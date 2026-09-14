package appapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// TestAllowOversizeBodySubmitsAnyway pins the escape hatch on the vendored body
// ceiling.
//
// 🔴 WHY A FLAG AT ALL, AND WHY IT NEEDS ITS OWN TEST. MaxSubmitBodyBytes is the
// framework default civitai's proxy runs under — sourced, and it correctly
// predicts both of #423's measured submissions — but it is VENDORED. Nothing
// local observes the day that default is raised, and civitai/civitai#4793 names
// raising it as the change to make if larger bundles are ever wanted. On that
// day every already-shipped CLI refuses at the old number, and without an
// override the author has nothing to do about it. That is precisely the failure
// claudedocs/decisions/31 was written to prevent ("a local refusal at a guessed
// limit would reject bundles the server accepts, with no override").
//
// So the refusal must hold by default AND be escapable, and BOTH halves need
// pinning: a flag that is wired but inert reads exactly like a working one.
func TestAllowOversizeBodySubmitsAnyway(t *testing.T) {
	// One byte of zip base64-encodes to more than one byte, so a zip this size
	// is guaranteed to produce a body over the ceiling without allocating an
	// enormous fixture twice.
	big := make([]byte, MaxSubmitBodyBytes)

	t.Run("refused by default", func(t *testing.T) {
		var hits int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":"1"}`)
		}))
		defer srv.Close()

		c := NewWithSource(srv.URL, civitai.StaticToken("t"), "/api/blocks/submit-version")
		_, err := c.SubmitVersion(context.Background(), big, "slug", "1.0.0", Provenance{})
		if err == nil {
			t.Fatal("an over-ceiling body was submitted without a refusal — the guard is gone")
		}
		if !strings.Contains(err.Error(), "the server can receive") {
			t.Errorf("refused, but not by the size guard: %v", err)
		}
		// The point of a PREFLIGHT refusal is that the upload never happens.
		if hits != 0 {
			t.Errorf("the server was contacted %d time(s); the refusal must cost no upload", hits)
		}
		// The refusal must name the way out, or an author who believes the
		// ceiling is stale has no route that is not editing their bundle.
		if !strings.Contains(err.Error(), "--allow-oversize") {
			t.Errorf("the refusal does not name --allow-oversize, so the escape hatch is undiscoverable "+
				"from the only place it is needed:\n  %v", err)
		}
	})

	t.Run("submitted anyway with AllowOversizeBody", func(t *testing.T) {
		var hits int
		var got int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			b, _ := io.ReadAll(r.Body)
			got = len(b)
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":"1"}`)
		}))
		defer srv.Close()

		c := NewWithSource(srv.URL, civitai.StaticToken("t"), "/api/blocks/submit-version")
		c.AllowOversizeBody = true
		if _, err := c.SubmitVersion(context.Background(), big, "slug", "1.0.0", Provenance{}); err != nil {
			t.Fatalf("--allow-oversize did not bypass the refusal: %v", err)
		}
		// 🔴 POSITIVE CONTROL. Without this the test passes if the override
		// happened to make SubmitVersion succeed for some other reason — the
		// upload must actually have carried the over-ceiling body.
		if hits != 1 {
			t.Fatalf("the server was contacted %d time(s), want 1 — the override must SUBMIT, not skip", hits)
		}
		if got <= MaxSubmitBodyBytes {
			t.Errorf("the body that arrived was %d bytes, not over the %d ceiling — this case is not "+
				"exercising the override at all", got, MaxSubmitBodyBytes)
		}
	})
}
