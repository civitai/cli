package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// devTokenRenameServer emulates POST /api/v1/blocks/dev-token: it 404s (bare
// "App not found" — the anti-shadow collision) for any request whose slug is in
// collide, and 200s with a token otherwise. It also answers GET .../me for the
// read-only warning path. reqs counts mint calls.
func devTokenRenameServer(t *testing.T, collide map[string]bool, reqs *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/me") {
			_ = json.NewEncoder(w).Encode(map[string]any{"username": "u", "id": 1, "tokenScope": 0})
			return
		}
		atomic.AddInt32(reqs, 1)
		var body struct {
			Slug string `json:"slug"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		if collide[body.Slug] {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "App not found"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "jwt-x"})
	}))
}

func readBlockID(t *testing.T, dir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "block.manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m struct {
		BlockID string `json:"blockId"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	return m.BlockID
}

// TestAppDevTokenRenamesOnCollisionAndRetries: on the anti-shadow 404 the command
// appends a random suffix (deterministic here), rewrites block.manifest.json's
// blockId, prints a notice, retries, and succeeds on the free slug.
func TestAppDevTokenRenamesOnCollisionAndRetries(t *testing.T) {
	var reqs int32
	srv := devTokenRenameServer(t, map[string]bool{"my-block": true}, &reqs)
	defer srv.Close()

	prev := devTokenSuffixGen
	devTokenSuffixGen = func() string { return "abc12" }
	defer func() { devTokenSuffixGen = prev }()

	dir := t.TempDir()
	writeDevTokenManifest(t, dir, `{
  "blockId": "my-block",
  "version": "0.1.0",
  "name": "My Block",
  "scopes": ["identity:read"]
}`)
	chdir(t, dir)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, errOut, err := run(t, "app", "dev-token", "my-block")
	if err != nil {
		t.Fatalf("app dev-token: %v\n%s", err, errOut)
	}
	if strings.TrimSpace(out) != "jwt-x" {
		t.Errorf("stdout should be the token after retry, got %q", out)
	}
	if reqs != 2 {
		t.Errorf("expected 2 mint calls (collision + retry), got %d", reqs)
	}
	// The manifest on disk now carries the renamed slug.
	if got := readBlockID(t, dir); got != "my-block-abc12" {
		t.Errorf("manifest blockId = %q, want my-block-abc12", got)
	}
	// Other fields survive the rewrite.
	raw, _ := os.ReadFile(filepath.Join(dir, "block.manifest.json"))
	if !strings.Contains(string(raw), `"name": "My Block"`) || !strings.Contains(string(raw), `"identity:read"`) {
		t.Errorf("rewrite should preserve other fields:\n%s", raw)
	}
	// The rename notice goes to stderr.
	if !strings.Contains(errOut, `Slug "my-block" is registered to another account`) ||
		!strings.Contains(errOut, `renamed to "my-block-abc12"`) {
		t.Errorf("expected a rename notice on stderr:\n%s", errOut)
	}
}

// TestAppDevTokenRenameIsBounded: when every generated alternative also collides,
// the retry loop gives up after maxDevTokenRenameAttempts with a clear message.
func TestAppDevTokenRenameIsBounded(t *testing.T) {
	var reqs int32
	// Collide on EVERY slug (the map lookup below always returns true).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqs, 1)
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "App not found"})
	}))
	defer srv.Close()

	var n int32
	prev := devTokenSuffixGen
	devTokenSuffixGen = func() string { n++; return "s" + string(rune('a'+n)) } // sb, sc, ...
	defer func() { devTokenSuffixGen = prev }()

	dir := t.TempDir()
	writeDevTokenManifest(t, dir, `{"blockId":"my-block","version":"0.1.0","name":"My Block"}`)
	chdir(t, dir)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	_, _, err := run(t, "app", "dev-token", "my-block")
	if err == nil {
		t.Fatal("expected a give-up error when every alternative collides")
	}
	if !strings.Contains(err.Error(), "all registered to other accounts") {
		t.Errorf("give-up error should be actionable: %v", err)
	}
	// original attempt + maxDevTokenRenameAttempts renamed attempts.
	if want := int32(1 + maxDevTokenRenameAttempts); reqs != want {
		t.Errorf("expected %d mint calls, got %d", want, reqs)
	}
}

// TestAppDevTokenNonCollision404DoesNotRename: a 404 that is NOT the anti-shadow
// collision (e.g. an owned-but-undeployed app: "no live deployment") must surface
// the raw error WITHOUT renaming the manifest.
func TestAppDevTokenNonCollision404DoesNotRename(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "block 'my-block' has no live deployment — dev:live requires an approved + deployed version",
		})
	}))
	defer srv.Close()

	prev := devTokenSuffixGen
	devTokenSuffixGen = func() string { t.Fatal("suffix generator must not be called for a non-collision 404"); return "" }
	defer func() { devTokenSuffixGen = prev }()

	dir := t.TempDir()
	writeDevTokenManifest(t, dir, `{"blockId":"my-block","version":"0.1.0","name":"My Block"}`)
	chdir(t, dir)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	_, _, err := run(t, "app", "dev-token", "my-block")
	if err == nil {
		t.Fatal("expected the raw 404 error")
	}
	if !strings.Contains(err.Error(), "no live deployment") {
		t.Errorf("non-collision 404 should surface verbatim: %v", err)
	}
	// The manifest must be untouched.
	if got := readBlockID(t, dir); got != "my-block" {
		t.Errorf("manifest blockId changed to %q; a non-collision 404 must not rename", got)
	}
}

// TestAppDevTokenCollisionNoManifestSurfacesError: without a local manifest to
// rewrite, the collision 404 is surfaced verbatim (nothing to rename).
func TestAppDevTokenCollisionNoManifestSurfacesError(t *testing.T) {
	var reqs int32
	srv := devTokenRenameServer(t, map[string]bool{"my-block": true}, &reqs)
	defer srv.Close()

	chdir(t, t.TempDir()) // empty dir, no manifest

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	_, _, err := run(t, "app", "dev-token", "my-block")
	if err == nil {
		t.Fatal("expected the collision 404 error")
	}
	if !strings.Contains(err.Error(), "registered to a different account") {
		t.Errorf("collision error should surface: %v", err)
	}
	if reqs != 1 {
		t.Errorf("expected exactly 1 mint call with no manifest to rename, got %d", reqs)
	}
}
