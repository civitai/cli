package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `app submit` MUST run the STRICT validator (validate.Dir), not the weaker
// validate.ManifestOnly the scaffold self-check uses. Submitting creates a real
// pending-moderator-review request, so a silent downgrade there would let an app
// the platform build cannot build reach a moderator — and the author, who cannot
// read the server-side build log, would only see it fail later.
//
// These are BEHAVIOURAL, not symbol assertions: they drive `app submit` on a
// project whose only defect is a project-state one (a pnpm lockfile under the
// scaffold's `npm run build`), which ManifestOnly by construction does not
// report. Swapping app_submit.go's validate.Dir for validate.ManifestOnly makes
// both of them fail.

// It must refuse, and must not do the packaging work.
func TestAppSubmitRefusesLockfileMismatchAndDoesNotPackage(t *testing.T) {
	dir := scaffoldWithLockfiles(t, "pnpm-lock.yaml")
	out := filepath.Join(t.TempDir(), "bundle.zip")

	stdout, stderr, err := run(t, "app", "submit", dir, "--package-only", "--out", out)
	if err == nil {
		t.Fatalf("submit must refuse a lockfile the platform build cannot install from\nstdout:%s\nstderr:%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "validation failed") {
		t.Errorf("stderr should report validation failure: %s", stderr)
	}
	// Specifically the project-state check — a generic manifest error would not
	// prove submit still runs the strict validator.
	if !strings.Contains(stderr, "pnpm-lock.yaml is committed") {
		t.Errorf("submit must surface the lockfile ↔ buildCommand error: %s", stderr)
	}
	if _, serr := os.Stat(out); serr == nil {
		t.Error("submit must not package a project it refused to validate")
	}
	if strings.Contains(stdout, "Packaged") {
		t.Errorf("submit must not report packaging after refusing: %s", stdout)
	}
}

// And with a token configured (the path that actually creates the review
// request), it must refuse BEFORE anything reaches the server.
func TestAppSubmitRefusesLockfileMismatchAndDoesNotUpload(t *testing.T) {
	dir := scaffoldWithLockfiles(t, "pnpm-lock.yaml")

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-lock")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	t.Setenv("CIVITAI_SUBMIT_PATH", "/api/blocks/submit-version")

	stdout, stderr, err := run(t, "app", "submit", dir, "--yes")
	if err == nil {
		t.Fatalf("submit must refuse before uploading\nstdout:%s\nstderr:%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "pnpm-lock.yaml is committed") {
		t.Errorf("submit must surface the lockfile ↔ buildCommand error: %s", stderr)
	}
	if hits != 0 {
		t.Errorf("submit sent %d request(s) to the server; a failed validation must never reach it", hits)
	}
}

// The control: the SAME project with the matching lockfile submits fine. Without
// this, a submit that refused everything would satisfy the tests above.
func TestAppSubmitAcceptsMatchingLockfile(t *testing.T) {
	dir := scaffoldWithLockfiles(t, "package-lock.json")
	out := filepath.Join(t.TempDir(), "bundle.zip")

	if stdout, stderr, err := run(t, "app", "submit", dir, "--package-only", "--out", out); err != nil {
		t.Fatalf("a matching lockfile must submit: %v\nstdout:%s\nstderr:%s", err, stdout, stderr)
	}
	if _, serr := os.Stat(out); serr != nil {
		t.Errorf("bundle should have been written: %v", serr)
	}
}
