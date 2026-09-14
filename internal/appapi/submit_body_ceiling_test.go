package appapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Issue #423: a bundle that clears every local cap can still be too large for the
// server to RECEIVE, because a proxy-matched request body is truncated rather than
// refused and the author gets `400: Invalid JSON` — an error naming nothing about size.
//
// 🔴 REGRESSION GUARD, red against the pre-change client: it marshalled the body and
// uploaded it unconditionally, so the case below reached the transport and came back as
// whatever the server said. The assertion is that the upload NEVER HAPPENS.
func TestSubmitVersionRefusesABodyTheServerCannotReceive(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok123", "/submit")

	// One byte of zip over what the envelope + base64 expansion leaves room for.
	// Derived from the constant rather than hardcoded, so this cannot drift from it.
	oversize := make([]byte, (MaxSubmitBodyBytes/4)*3)
	_, err := c.SubmitVersion(context.Background(), oversize, "slug", "1.0.0", Provenance{})

	if err == nil {
		t.Fatal("expected a local refusal, got nil")
	}
	// The message must name the BODY size, which is the quantity the limit applies to
	// and the one the author cannot otherwise observe.
	for _, want := range []string{"submit body is", "server can receive"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message does not contain %q: %v", want, err)
		}
	}
	// 🔴 THE LOAD-BEARING ASSERTION. An error alone does not prove the upload was
	// avoided — the point of refusing locally is that the author does not wait out an
	// 11 MB POST first. A status code cannot witness that; the server's hit count can.
	if hits != 0 {
		t.Errorf("body was uploaded despite the refusal: server saw %d request(s)", hits)
	}
}

// The complement, so the guard above cannot pass by refusing everything.
func TestSubmitVersionStillUploadsABodyThatFits(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"publishRequestId":"x","slug":"slug","version":"1.0.0","status":"pending"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok123", "/submit")
	if _, err := c.SubmitVersion(context.Background(), []byte("a small zip"), "slug", "1.0.0", Provenance{}); err != nil {
		t.Fatalf("a small bundle must still upload: %v", err)
	}
	if hits != 1 {
		t.Errorf("expected exactly 1 upload, got %d", hits)
	}
}
