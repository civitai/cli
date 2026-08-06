package genapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// 🔴 UploadImageBlob must close the upload response body on EVERY path.
//
// Surviving mutation: delete `defer func() { _ = resp.Body.Close() }()` in
// UploadImageBlob. Deleting it leaves the whole suite green — every existing
// test reads the returned URL or the returned error and never asks what happened
// to the handle. A leaked body keeps the connection out of the pool, and this is
// the one hop in the feature that runs once PER REFERENCE IMAGE.
//
// The uploader seam here is a fake that returns a response THIS TEST built, so
// the close is observed by a counter in this process at the moment
// UploadImageBlob returns: no server, no goroutine, no timing dependency.
// Deliberately not measured on the server side — a count of what a handler
// observes is a race, not an assertion.

// closingBody is a response body that counts its own Close calls.
type closingBody struct {
	mu     sync.Mutex
	reader io.Reader
	closed int
}

func newClosingBody(s string) *closingBody {
	return &closingBody{reader: bytes.NewReader([]byte(s))}
}

func (b *closingBody) Read(p []byte) (int, error) { return b.reader.Read(p) }

func (b *closingBody) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed++
	return nil
}

func (b *closingBody) closeCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

// fakeUploader is a civitai.PresignedUploader returning a scripted response.
//
// It takes no token parameter — that is the interface's whole point (AGENTS.md
// item 19(e)) — so this fake cannot accidentally weaken the credential-free
// contract the sibling tests assert.
type fakeUploader struct {
	status int
	body   *closingBody
	calls  int
}

func (f *fakeUploader) UploadPresigned(_ context.Context, _, _ string, _ []byte) (*http.Response, error) {
	f.calls++
	return &http.Response{StatusCode: f.status, Body: f.body, Header: make(http.Header)}, nil
}

func TestUploadImageBlob_ClosesTheUploadResponseBody(t *testing.T) {
	// The presign hop is a real httptest server; only the upload hop is faked,
	// so the two-hop shape under test is the real one.
	presign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"uploadUrl":"https://storage.example/v2/consumer/blobs","expiresAt":"2030-01-01T00:00:00Z"}`)
	}))
	defer presign.Close()

	cases := []struct {
		name    string
		status  int
		reply   string
		wantErr bool
	}{
		{"success", http.StatusOK, `{"id":"B1","available":true,"url":"https://storage.example/blobs/B1.png"}`, false},
		{"rejected (413)", http.StatusRequestEntityTooLarge, `too big`, true},
		{"expired signature (403)", http.StatusForbidden, `denied`, true},
		{"2xx with unparseable body", http.StatusOK, `<html>not json</html>`, true},
		{"2xx with a null url", http.StatusOK, `{"id":"B1","available":false,"url":null}`, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := newClosingBody(tc.reply)
			up := &fakeUploader{status: tc.status, body: body}
			c := New(presign.URL, "test-key")

			_, err := c.UploadImageBlob(context.Background(), up, "image/png", []byte("PNGBYTES"))
			if tc.wantErr && err == nil {
				t.Fatal("want an error for this case")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if up.calls != 1 {
				t.Fatalf("the upload seam was reached %d time(s), want 1 — the close assertion below would be vacuous", up.calls)
			}

			// 🔴 THE assertion, on every path.
			if got := body.closeCount(); got != 1 {
				t.Errorf("🔴 HANDLE LEAK: the upload response body was closed %d time(s), want exactly 1", got)
			}
		})
	}

	// POSITIVE/NEGATIVE CONTROL for the counter itself: it reads 0 until
	// something closes it and 1 afterwards, so the 1s above are observations
	// rather than a constant.
	t.Run("control: the counter moves", func(t *testing.T) {
		b := newClosingBody("x")
		if got := b.closeCount(); got != 0 {
			t.Fatalf("CONTROL FAILED: an untouched body reports %d closes, want 0", got)
		}
		_ = b.Close()
		if got := b.closeCount(); got != 1 {
			t.Fatalf("CONTROL FAILED: after one Close() the counter reads %d, want 1", got)
		}
	})
}

// The fake must satisfy the real interface, or this file is testing a shape the
// production code does not use.
var _ civitai.PresignedUploader = (*fakeUploader)(nil)
