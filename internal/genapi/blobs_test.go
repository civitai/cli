package genapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// blobHarness stands up a fake Civitai (the AUTHED presign hop) and a fake
// storage host (the CREDENTIAL-FREE upload hop) as two SEPARATE servers, so the
// two hops' headers can never be confused for one another.
type blobHarness struct {
	presignCalls  int
	presignMethod string
	presignAuth   string
	presignPath   string

	uploadCalls  int
	uploadAuth   string
	uploadAuthOK bool
	uploadMethod string
	uploadCType  string
	uploadBody   []byte

	presignStatus int
	presignBody   string
	uploadStatus  int
	uploadBody2   string

	client *Client
	upl    civitai.PresignedUploader
}

func newBlobHarness(t *testing.T) *blobHarness {
	t.Helper()
	h := &blobHarness{presignStatus: 200, uploadStatus: 200}

	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.uploadCalls++
		h.uploadMethod = r.Method
		h.uploadAuth = r.Header.Get("Authorization")
		_, h.uploadAuthOK = r.Header["Authorization"]
		h.uploadCType = r.Header.Get("Content-Type")
		h.uploadBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(h.uploadStatus)
		body := h.uploadBody2
		if body == "" {
			body = `{"id":"BLOB1","type":"image","available":true,"url":"https://orchestration.civitai.com/v2/consumer/blobs/BLOB1.png"}`
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(storage.Close)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.presignCalls++
		h.presignMethod = r.Method
		h.presignPath = r.URL.Path
		h.presignAuth = r.Header.Get("Authorization")
		w.WriteHeader(h.presignStatus)
		body := h.presignBody
		if body == "" {
			body = fmt.Sprintf(`{"uploadUrl":%q,"expiresAt":"2030-01-01T00:00:00Z"}`, storage.URL+"/v2/consumer/blobs")
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(api.Close)

	h.client = New(api.URL, "secret-personal-key")
	// 🔴 The uploader's BaseURL is the STORAGE host, not the API host — on
	// purpose. In production the presigned upload URL lives on a *.civitai.com
	// host that isTrustedDownloadHost MATCHES, so a token-carrying code path
	// would happily attach the bearer. Pointing the test uploader at the API
	// host instead would make the predicate reject the storage URL for an
	// incidental reason (different host), and the no-credential assertion would
	// pass even against a client that folds the credential back in — measured:
	// a mutant attaching `Bearer <token>` behind isTrustedDownloadHost survived
	// this test until the BaseURL was moved here.
	up := civitai.NewWithSource(storage.URL, civitai.StaticToken("secret-personal-key"))
	up.AllowPrivateDownloadHosts = true // httptest is loopback http
	h.upl = up
	return h
}

func TestUploadImageBlob_HappyPath(t *testing.T) {
	h := newBlobHarness(t)

	url, err := h.client.UploadImageBlob(context.Background(), h.upl, "image/png", []byte("PNG"))
	if err != nil {
		t.Fatalf("UploadImageBlob: %v", err)
	}
	if url != "https://orchestration.civitai.com/v2/consumer/blobs/BLOB1.png" {
		t.Errorf("blob URL = %q", url)
	}
	if h.presignCalls != 1 || h.uploadCalls != 1 {
		t.Fatalf("presign=%d upload=%d, want 1 each", h.presignCalls, h.uploadCalls)
	}
	if h.presignMethod != http.MethodGet {
		t.Errorf("presign method = %q, want GET", h.presignMethod)
	}
	if h.presignPath != BlobUploadURLPath {
		t.Errorf("presign path = %q, want %q", h.presignPath, BlobUploadURLPath)
	}
	if h.uploadMethod != http.MethodPost {
		t.Errorf("upload method = %q, want POST", h.uploadMethod)
	}
	if h.uploadCType != "image/png" {
		t.Errorf("upload Content-Type = %q", h.uploadCType)
	}
	if string(h.uploadBody) != "PNG" {
		t.Errorf("upload body = %q", h.uploadBody)
	}
}

// 🔴 The two hops must differ: hop 1 authed, hop 2 NOT.
//
// Asserting only "hop 2 has no auth" cannot distinguish a correct
// credential-free upload from a harness that records nothing, so hop 1 is the
// in-test positive control — same run, same client, same token.
func TestUploadImageBlob_OnlyThePresignHopCarriesACredential(t *testing.T) {
	h := newBlobHarness(t)

	if _, err := h.client.UploadImageBlob(context.Background(), h.upl, "image/png", []byte("PNG")); err != nil {
		t.Fatalf("UploadImageBlob: %v", err)
	}

	// POSITIVE CONTROL: hop 1 DID carry the bearer.
	if h.presignAuth != "Bearer secret-personal-key" {
		t.Fatalf("positive control FAILED: the presign hop carried Authorization %q, want the bearer — with no credential in play the hop-2 assertion below is vacuous", h.presignAuth)
	}
	// THE GUARD: hop 2 carried nothing.
	if h.uploadAuthOK {
		t.Errorf("🔴 CREDENTIAL LEAK: the presigned upload hop carried Authorization %q", h.uploadAuth)
	}
}

func TestGetBlobUploadTarget_Errors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantIs  error
		wantSub string
	}{
		{"403 forbidden", http.StatusForbidden, "Not authorized to upload", civitai.ErrUnauthorized, "whoami"},
		{"401 unauthorized", http.StatusUnauthorized, "no token", civitai.ErrUnauthorized, "whoami"},
		{"500 server error", http.StatusInternalServerError, "boom", nil, "refused to issue"},
		{"200 but empty uploadUrl", http.StatusOK, `{"uploadUrl":"","expiresAt":"x"}`, nil, "unexpected"},
		{"200 but not JSON", http.StatusOK, `<html>nope</html>`, nil, "unexpected"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newBlobHarness(t)
			h.presignStatus, h.presignBody = tc.status, tc.body

			_, err := h.client.UploadImageBlob(context.Background(), h.upl, "image/png", []byte("PNG"))
			if err == nil {
				t.Fatal("want an error")
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Errorf("errors.Is(%v, %v) = false — the exit code is pinned by kind, never by text", err, tc.wantIs)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("message %q lacks %q", err, tc.wantSub)
			}
			// 🔴 A failed presign must never proceed to the upload.
			if h.uploadCalls != 0 {
				t.Errorf("upload ran %d time(s) after a failed presign", h.uploadCalls)
			}
		})
	}
}

// POSITIVE CONTROL for the uploadCalls==0 assertions above: the same counter
// reaches 1 on the success path.
func TestGetBlobUploadTarget_PositiveControl_UploadCounterMoves(t *testing.T) {
	h := newBlobHarness(t)
	if _, err := h.client.UploadImageBlob(context.Background(), h.upl, "image/png", []byte("PNG")); err != nil {
		t.Fatalf("UploadImageBlob: %v", err)
	}
	if h.uploadCalls != 1 {
		t.Fatalf("positive control FAILED: uploadCalls = %d, want 1 — the zeros asserted elsewhere prove nothing if this counter never moves", h.uploadCalls)
	}
}

func TestUploadImageBlob_UploadFailures(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantIs  error
		wantSub string
	}{
		{"403 expired signature", http.StatusForbidden, "signature expired", civitai.ErrUnauthorized, "short-lived"},
		{"400 bad blob", http.StatusBadRequest, "unsupported content type", civitai.ErrBadRequest, "rejected"},
		{"500 storage down", http.StatusInternalServerError, "oops", nil, "rejected"},
		{"200 but no url", http.StatusOK, `{"id":"B2","available":false}`, nil, "no usable URL"},
		{"200 but null url", http.StatusOK, `{"id":"B3","available":true,"url":null}`, nil, "no usable URL"},
		{"200 but not JSON", http.StatusOK, `not json`, nil, "unexpected image-upload response"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newBlobHarness(t)
			h.uploadStatus, h.uploadBody2 = tc.status, tc.body

			_, err := h.client.UploadImageBlob(context.Background(), h.upl, "image/png", []byte("PNG"))
			if err == nil {
				t.Fatal("want an error")
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Errorf("errors.Is(%v, %v) = false", err, tc.wantIs)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("message %q lacks %q", err, tc.wantSub)
			}
		})
	}
}

// A transport failure on the upload hop (host unreachable) is reported as such,
// not swallowed.
func TestUploadImageBlob_TransportFailure(t *testing.T) {
	h := newBlobHarness(t)
	// Point the presign reply at a port nothing listens on.
	h.presignBody = `{"uploadUrl":"http://127.0.0.1:1/v2/consumer/blobs","expiresAt":"2030-01-01T00:00:00Z"}`

	_, err := h.client.UploadImageBlob(context.Background(), h.upl, "image/png", []byte("PNG"))
	if err == nil {
		t.Fatal("want a transport error")
	}
	if !strings.Contains(err.Error(), "uploading the reference image failed") {
		t.Errorf("message %q should name the failing step", err)
	}
}

// 🔴 SSRF at the genapi layer: a SERVER-SUPPLIED upload URL pointing at a
// plain-http internal endpoint must be refused, with the guard ON.
func TestUploadImageBlob_RefusesHostileUploadURL(t *testing.T) {
	h := newBlobHarness(t)
	h.presignBody = `{"uploadUrl":"http://169.254.169.254/latest/meta-data","expiresAt":"2030-01-01T00:00:00Z"}`
	// Guard ON: a fresh uploader without AllowPrivateDownloadHosts.
	up := civitai.NewWithSource("https://civitai.com", civitai.StaticToken("k"))

	_, err := h.client.UploadImageBlob(context.Background(), up, "image/png", []byte("PNG"))
	if err == nil {
		t.Fatal("a plain-http/internal upload URL must be refused")
	}
	if !strings.Contains(err.Error(), "https") {
		t.Errorf("refusal should cite the https requirement, got %v", err)
	}
	if h.uploadCalls != 0 {
		t.Errorf("refused upload still reached storage %d time(s)", h.uploadCalls)
	}
}
