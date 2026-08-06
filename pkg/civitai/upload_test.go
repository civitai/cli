package civitai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// uploadRecorder captures what a fake presigned upload host actually received.
type uploadRecorder struct {
	calls   int
	method  string
	auth    string
	ctype   string
	clen    int64
	body    []byte
	authSet bool
}

func newUploadServer(t *testing.T, rec *uploadRecorder, status int, reply string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.calls++
		rec.method = r.Method
		rec.auth = r.Header.Get("Authorization")
		_, rec.authSet = r.Header["Authorization"]
		rec.ctype = r.Header.Get("Content-Type")
		rec.clen = r.ContentLength
		rec.body, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// uploadClient builds a Client whose token source WOULD supply a credential.
// That is deliberate: a no-auth assertion made against a client with no token to
// leak proves nothing.
func uploadClient(baseURL, token string) *Client {
	c := NewWithSource(baseURL, StaticToken(token))
	c.AllowPrivateDownloadHosts = true // httptest is loopback http
	return c
}

// 🔴 THE guard: no credential ever reaches the presigned upload host.
//
// The token source is loaded with a real-looking key, and the assertion is on
// the RAW header map (not just the string), so a header present-but-empty would
// still fail.
func TestUploadPresigned_SendsNoCredential(t *testing.T) {
	var rec uploadRecorder
	srv := newUploadServer(t, &rec, http.StatusOK, `{"id":"b1","url":"https://x/blob"}`)

	c := uploadClient(srv.URL, "secret-personal-key")
	resp, err := c.UploadPresigned(context.Background(), srv.URL+"/v2/consumer/blobs", "image/png", []byte("PNGBYTES"))
	if err != nil {
		t.Fatalf("UploadPresigned: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if rec.calls != 1 {
		t.Fatalf("upload host saw %d requests, want 1 — a zero here would make every assertion below vacuous", rec.calls)
	}
	if rec.authSet {
		t.Errorf("🔴 CREDENTIAL LEAK: the presigned upload carried an Authorization header (%q)", rec.auth)
	}
	if rec.method != http.MethodPost {
		t.Errorf("method = %q, want POST (the orchestrator's presigned URL is POST /v2/consumer/blobs)", rec.method)
	}
	if rec.ctype != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", rec.ctype)
	}
	if rec.clen != int64(len("PNGBYTES")) {
		t.Errorf("Content-Length = %d, want %d — the signed endpoint rejects a chunked body", rec.clen, len("PNGBYTES"))
	}
	if string(rec.body) != "PNGBYTES" {
		t.Errorf("body = %q, want the bytes handed in", rec.body)
	}
}

// 🔴 POSITIVE CONTROL for the test above. The very same client, token and server
// DO produce an Authorization header on the path that is supposed to carry one.
//
// Without this, "0 auth headers" is indistinguishable from a recorder wired to
// nothing, a server never reached, or a Client that has no token at all.
func TestUploadPresigned_PositiveControl_AuthedPathDoesSendCredential(t *testing.T) {
	var rec uploadRecorder
	srv := newUploadServer(t, &rec, http.StatusOK, `{}`)
	c := uploadClient(srv.URL, "secret-personal-key")

	// DownloadFile targets the client's own BaseURL host, which
	// isTrustedDownloadHost trusts — so this request MUST carry the bearer.
	resp, err := c.DownloadFile(context.Background(), srv.URL+"/api/download/models/1")
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if rec.calls != 1 {
		t.Fatalf("server saw %d requests, want 1", rec.calls)
	}
	if !rec.authSet {
		t.Fatal("positive control FAILED: the authed download carried no Authorization header, so the no-credential assertion in the sibling test proves nothing")
	}
	if rec.auth != "Bearer secret-personal-key" {
		t.Errorf("Authorization = %q, want the bearer token", rec.auth)
	}
}

// 🔴 SSRF: a plain-http upload target is refused before any connection.
func TestUploadPresigned_RefusesNonHTTPS(t *testing.T) {
	var rec uploadRecorder
	srv := newUploadServer(t, &rec, http.StatusOK, `{}`)
	c := NewWithSource(srv.URL, StaticToken("k")) // guard ENABLED

	_, err := c.UploadPresigned(context.Background(), "http://169.254.169.254/latest/meta-data", "image/png", []byte("x"))
	if err == nil {
		t.Fatal("an http:// upload target must be refused")
	}
	if !strings.Contains(err.Error(), "https") {
		t.Errorf("refusal should cite the https requirement, got: %v", err)
	}
	if rec.calls != 0 {
		t.Errorf("refused upload still made %d request(s)", rec.calls)
	}
}

// 🔴 SSRF: an https URL that RESOLVES to an internal address is refused at dial
// time by the shared dial guard.
func TestUploadPresigned_RefusesInternalAddress(t *testing.T) {
	c := NewWithSource("https://civitai.com", StaticToken("k")) // guard ENABLED

	// 127.0.0.1 over https: the scheme check passes, so the refusal can only
	// come from the dialer — which is what this asserts.
	_, err := c.UploadPresigned(context.Background(), "https://127.0.0.1:9/v2/consumer/blobs", "image/png", []byte("x"))
	if err == nil {
		t.Fatal("an upload to a loopback address must be refused")
	}
	if !strings.Contains(err.Error(), "private/loopback") {
		t.Errorf("refusal should cite the private/loopback block, got: %v", err)
	}
}

// POSITIVE CONTROL for the SSRF guard: with the guard disabled the SAME loopback
// address is reachable. Without this, the refusal above could equally be a
// connection failure to port 9.
func TestUploadPresigned_PositiveControl_LoopbackReachableWithGuardOff(t *testing.T) {
	var rec uploadRecorder
	srv := newUploadServer(t, &rec, http.StatusOK, `{}`)
	c := uploadClient(srv.URL, "k") // AllowPrivateDownloadHosts = true

	resp, err := c.UploadPresigned(context.Background(), srv.URL+"/blobs", "image/png", []byte("x"))
	if err != nil {
		t.Fatalf("positive control FAILED: loopback upload errored with the guard off: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if rec.calls != 1 {
		t.Fatalf("positive control FAILED: the loopback server saw %d requests, want 1", rec.calls)
	}
}

func TestUploadPresigned_RefusesEmptyContentType(t *testing.T) {
	var rec uploadRecorder
	srv := newUploadServer(t, &rec, http.StatusOK, `{}`)
	c := uploadClient(srv.URL, "k")

	if _, err := c.UploadPresigned(context.Background(), srv.URL+"/blobs", "", []byte("x")); err == nil {
		t.Fatal("an upload with no Content-Type must be refused")
	}
	if rec.calls != 0 {
		t.Errorf("refused upload still reached the server %d time(s)", rec.calls)
	}
}

// A non-2xx upload is returned to the caller as a response, not an error — the
// genapi layer maps the status. This pins that contract.
func TestUploadPresigned_PassesNon2xxThrough(t *testing.T) {
	var rec uploadRecorder
	srv := newUploadServer(t, &rec, http.StatusInsufficientStorage, `nope`)
	c := uploadClient(srv.URL, "k")

	resp, err := c.UploadPresigned(context.Background(), srv.URL+"/blobs", "image/jpeg", []byte("x"))
	if err != nil {
		t.Fatalf("a 507 must come back as a response, not a transport error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusInsufficientStorage {
		t.Errorf("status = %d, want 507", resp.StatusCode)
	}
	if rec.authSet {
		t.Errorf("🔴 CREDENTIAL LEAK on the failure path: Authorization = %q", rec.auth)
	}
}
