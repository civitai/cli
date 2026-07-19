package cmd

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/api"
)

// TestGetCmdsBadIdIsUsageTagged asserts that a garbage (non-integer / non-positive)
// positional id on `models get`, `articles get`, and `collections get` is
// classified as a USAGE error (→ exit 2) — matching `model-versions get` — rather
// than a bare error (→ exit 1). The bad id is caught client-side before any HTTP
// call, so no server is needed.
func TestGetCmdsBadIdIsUsageTagged(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"models", []string{"models", "get", "abc"}},
		{"articles", []string{"articles", "get", "abc"}},
		{"collections", []string{"collections", "get", "abc"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := run(t, tc.args...)
			if err == nil {
				t.Fatalf("%s get abc: expected an error for a non-integer id", tc.name)
			}
			if !errors.Is(err, ErrUsage) {
				t.Errorf("%s get abc should classify as usage (exit 2), got %T: %v", tc.name, err, err)
			}
			// The user-visible message must be preserved unchanged.
			if !strings.Contains(err.Error(), "must be a positive integer") {
				t.Errorf("%s get abc: message should be preserved, got: %v", tc.name, err)
			}
		})
	}
}

// TestModelsGetRealNotFoundStaysNotFound guards the boundary: a syntactically
// valid id that the API rejects with a 404 must STILL classify as not-found
// (exit 4) — the usage-tagging change only affects the local bad-id parse, not
// the remote 404 path.
func TestModelsGetRealNotFoundStaysNotFound(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})
	_, _, err := run(t, "models", "get", "999999999")
	if err == nil {
		t.Fatal("expected an error for a 404 model id")
	}
	if errors.Is(err, ErrUsage) {
		t.Errorf("a real 404 must NOT be tagged as usage, got: %v", err)
	}
	if !errors.Is(err, api.ErrNotFound) {
		t.Errorf("a real 404 should classify as not-found (exit 4), got: %v", err)
	}
}

// TestModelsSearchBaseModelZeroResultNote asserts that when --base-model is given
// and the search returns zero models, a one-line stderr note is printed while the
// exit stays 0 (an empty result set is not an error).
func TestModelsSearchBaseModelZeroResultNote(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[],"metadata":{}}`))
	})
	_, stderr, err := run(t, "models", "search", "--base-model", "Zzzzz", "--limit", "3")
	if err != nil {
		t.Fatalf("empty results must not be an error (exit 0), got: %v", err)
	}
	if !strings.Contains(stderr, "check --base-model spelling") {
		t.Errorf("expected the base-model zero-result note on stderr, got: %q", stderr)
	}
}

// TestModelsSearchNoBaseModelNoNote asserts the note is scoped to --base-model:
// a zero-result search WITHOUT --base-model must not emit it.
func TestModelsSearchNoBaseModelNoNote(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[],"metadata":{}}`))
	})
	_, stderr, err := run(t, "models", "search", "--query", "zzz-no-such-model", "--limit", "3")
	if err != nil {
		t.Fatalf("empty results must not be an error, got: %v", err)
	}
	if strings.Contains(stderr, "check --base-model spelling") {
		t.Errorf("the base-model note must not appear without --base-model, got: %q", stderr)
	}
}
