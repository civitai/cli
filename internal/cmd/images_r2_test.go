package cmd

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

// TestImagesSearchCollectionIDRemoved asserts the --collection-id flag is gone:
// the public /api/v1/images endpoint ignores collectionId (it silently returned
// the global feed as "the collection"), so the flag was removed. Passing it must
// now fail with an unknown-flag usage error (exit 2), not succeed at exit 0.
func TestImagesSearchCollectionIDRemoved(t *testing.T) {
	_, _, err := run(t, "images", "search", "--collection-id", "104")
	if err == nil {
		t.Fatal("--collection-id should be an unknown flag now, got no error")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("expected an unknown-flag error, got: %v", err)
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("unknown flag must be tagged ErrUsage (→ exit 2), got %v", err)
	}
	// The flag must not be registered on the command at all.
	if f := newImagesSearchCmd().Flags().Lookup("collection-id"); f != nil {
		t.Errorf("collection-id flag should be removed, but it is still registered")
	}
}

// TestImagesGetBadIdIsUsageError asserts a non-numeric positional id is a usage
// error (exit 2), mirroring `model-versions get` — it previously returned a bare
// error (exit 1).
func TestImagesGetBadIdIsUsageError(t *testing.T) {
	_, _, err := run(t, "images", "get", "abc")
	if err == nil {
		t.Fatal("expected error for non-numeric image id")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("bad image id must be tagged ErrUsage (→ exit 2), got %v", err)
	}
}

// TestImagesGetOutOfRangeIdIsUsageError asserts an oversized id (beyond the
// 32-bit image-id range) is caught locally as a usage error (exit 2) instead of
// being sent to the API and leaking a host 500 (exit 1). The server must never
// be hit.
func TestImagesGetOutOfRangeIdIsUsageError(t *testing.T) {
	hit := false
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusInternalServerError)
	})
	_, _, err := run(t, "images", "get", "99999999999")
	if err == nil {
		t.Fatal("expected error for out-of-range image id")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("out-of-range image id must be tagged ErrUsage (→ exit 2), got %v", err)
	}
	if hit {
		t.Error("an out-of-range id must be rejected locally, not sent to the API")
	}
}

// TestImagesSearchBaseModelZeroResultNote asserts a --base-model that matches
// nothing prints a spelling hint on stderr (the API OR-matches literally, so a
// typo just returns an empty page) while still exiting 0 — zero results is not
// an error.
func TestImagesSearchBaseModelZeroResultNote(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[],"metadata":{}}`))
	})
	out, errOut, err := run(t, "images", "search", "--base-model", "Zzzzz", "--limit", "3")
	if err != nil {
		t.Fatalf("zero results must not be an error (exit 0): %v", err)
	}
	if !strings.Contains(errOut, "check --base-model spelling") {
		t.Errorf("expected a --base-model spelling hint on stderr, got: %q", errOut)
	}
	if !strings.Contains(out, "No images found.") {
		t.Errorf("stdout should still report no images: %q", out)
	}
}

// TestImagesSearchNoBaseModelNoNote asserts the hint is NOT printed for an empty
// result when --base-model wasn't given (nothing to blame on spelling).
func TestImagesSearchNoBaseModelNoNote(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[],"metadata":{}}`))
	})
	_, errOut, err := run(t, "images", "search", "--limit", "3")
	if err != nil {
		t.Fatalf("images search: %v", err)
	}
	if strings.Contains(errOut, "check --base-model spelling") {
		t.Errorf("no --base-model given → no spelling hint, got: %q", errOut)
	}
}
