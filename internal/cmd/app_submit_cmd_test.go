package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeStaticManifest writes a minimal valid static page manifest into dir.
func writeStaticManifest(t *testing.T, dir string) {
	t.Helper()
	m := `{
  "$schema": "https://civitai.com/schemas/app-block/v1.json",
  "blockId": "demo-block",
  "version": "0.1.0",
  "name": "Demo Block",
  "type": "block",
  "scopes": [],
  "page": { "path": "/", "title": "Demo Block", "icon": "bolt" },
  "iframe": { "minHeight": 400, "maxHeight": 4000, "resizable": true, "sandbox": "allow-scripts allow-forms" },
  "contentRating": "g",
  "minApiVersion": "1.0"
}`
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAppSubmitPackageOnly(t *testing.T) {
	tmp := t.TempDir()
	writeStaticManifest(t, tmp)
	if err := os.WriteFile(filepath.Join(tmp, "index.html"), []byte("<html></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(tmp, "bundle.zip")

	stdout, _, err := run(t, "app", "submit", tmp, "--package-only", "--out", out)
	if err != nil {
		t.Fatalf("submit --package-only: %v\n%s", err, stdout)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("zip should be written: %v", err)
	}
	if !strings.Contains(stdout, "Wrote canonical bundle") {
		t.Errorf("output should report the bundle: %s", stdout)
	}
}

// TestAppSubmitSurfacesWarnings pins that a non-fatal advisory REACHES the
// author on the submit path.
//
// `submit` used to branch only on res.OK() and drop res.Warnings on the floor.
// That is the highest-traffic command and the last point before an app goes to
// review, so the ready-ack advisory — which predicts a blank failure card in the
// real host — reached nobody who did not run `app validate` by hand.
//
// It must NOT block: warnings failing a submit is the --strict contract, and
// promoting a heuristic to a gate here would break correct projects.
func TestAppSubmitSurfacesWarnings(t *testing.T) {
	tmp := t.TempDir()
	writeStaticManifest(t, tmp)
	// A page app whose only source file never posts BLOCK_READY: the #206 shape.
	if err := os.WriteFile(filepath.Join(tmp, "index.html"), []byte("<html></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(tmp, "bundle.zip")

	stdout, stderr, err := run(t, "app", "submit", tmp, "--package-only", "--out", out)
	if err != nil {
		t.Fatalf("a warning must not fail the submit: %v\n%s", err, stderr)
	}
	if !strings.Contains(stderr, "BLOCK_READY") {
		t.Errorf("submit swallowed the ready-ack advisory; it must reach the author here, not only in "+
			"`app validate`.\nstderr: %s", stderr)
	}
	if !strings.Contains(stderr, "warning(s)") {
		t.Errorf("submit should print the warning header: %s", stderr)
	}
	// The submit still has to have happened.
	if _, err := os.Stat(out); err != nil {
		t.Errorf("the bundle must still be written: %v\n%s", err, stdout)
	}

	// NEGATIVE CONTROL: a project that is not in the #206 shape must produce no
	// such line, or the assertion above is satisfied by output that is always
	// printed regardless of the finding.
	clean := t.TempDir()
	writeStaticManifest(t, clean)
	if err := os.WriteFile(filepath.Join(clean, "index.html"),
		[]byte("<html><script>parent.postMessage({type:'BLOCK_READY',payload:{}},o)</script></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, stderr2, err := run(t, "app", "submit", clean, "--package-only", "--out", filepath.Join(clean, "b.zip"))
	if err != nil {
		t.Fatalf("clean submit: %v\n%s", err, stderr2)
	}
	if strings.Contains(stderr2, "nothing in this project's source posts") {
		t.Errorf("a correct app must not get the advisory on submit: %s", stderr2)
	}
}

func TestAppSubmitFallbackPrintsManualSteps(t *testing.T) {
	tmp := t.TempDir()
	writeStaticManifest(t, tmp)

	// No token => fallback to writing zip + manual steps.
	cfgdir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgdir)
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_SUBMIT_PATH", "")

	chdir(t, tmp)
	stdout, _, err := run(t, "app", "submit")
	if err != nil {
		t.Fatalf("submit fallback: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "NOT SUBMITTED") || !strings.Contains(stdout, "never uploaded") {
		t.Errorf("fallback must clearly read as not-submitted: %s", stdout)
	}
	if !strings.Contains(stdout, "/apps/submit") {
		t.Errorf("fallback should keep the web-UI upload option: %s", stdout)
	}
	if !strings.Contains(stdout, "civitai login") {
		t.Errorf("fallback should point at `civitai login`: %s", stdout)
	}
	// The dev believed they'd submitted with no token; point them at where a
	// successful submit will actually show up (mirrors the happy path's link).
	if !strings.Contains(stdout, "/apps/my-submissions") {
		t.Errorf("fallback should point at /apps/my-submissions: %s", stdout)
	}
	if !strings.Contains(stdout, "invite-only beta") {
		t.Errorf("fallback should keep the invite-only beta note: %s", stdout)
	}
}

func TestAppSubmitRefusesInvalidManifest(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "block.manifest.json"), []byte(`{"blockId":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, errOut, err := run(t, "app", "submit", tmp)
	if err == nil {
		t.Fatal("expected submit to fail on an invalid manifest")
	}
	if !strings.Contains(errOut, "validation failed") {
		t.Errorf("stderr should mention validation: %s", errOut)
	}
}

func TestAppSubmitUploadsWhenTokenAndPathConfigured(t *testing.T) {
	tmp := t.TempDir()
	writeStaticManifest(t, tmp)

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"publishRequestId": "pr_42",
			"slug":             "demo-block",
			"version":          "0.1.0",
			"status":           "pending",
		})
	}))
	defer srv.Close()

	cfgdir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgdir)
	t.Setenv("CIVITAI_TOKEN", "tok-xyz")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	t.Setenv("CIVITAI_SUBMIT_PATH", "/api/blocks/submit-version")

	// --yes bypasses the confirmation gate (non-TTY test shell would otherwise refuse).
	stdout, _, err := run(t, "app", "submit", tmp, "--yes")
	if err != nil {
		t.Fatalf("submit upload: %v\n%s", err, stdout)
	}
	if gotAuth != "Bearer tok-xyz" {
		t.Errorf("server saw auth %q, want Bearer tok-xyz", gotAuth)
	}
	if !strings.Contains(stdout, "pr_42") || !strings.Contains(stdout, "pending") {
		t.Errorf("output should report the publish request: %s", stdout)
	}
	// A successful submit surfaces actionable next-steps: the per-app deploy URL
	// and the submissions link — and drops the old, unrelated dev:live Buzz tip.
	if !strings.Contains(stdout, "https://demo-block.civit.ai") || !strings.Contains(stdout, "/apps/my-submissions") {
		t.Errorf("submit success should print the deploy URL + submissions link: %s", stdout)
	}
	if strings.Contains(stdout, "spends Buzz") || strings.Contains(stdout, "dev:live") {
		t.Errorf("submit success should not print the dev:live Buzz tip: %s", stdout)
	}
}

func TestAppSubmitUploadsToV1RouteByDefault(t *testing.T) {
	tmp := t.TempDir()
	writeStaticManifest(t, tmp)

	var gotPath, gotAuth, gotBundle string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var body struct {
			BundleBase64 string `json:"bundleBase64"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotBundle = body.BundleBase64
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"publishRequestId": "pr_77", "slug": "demo-block", "version": "0.1.0", "status": "pending",
		})
	}))
	defer srv.Close()

	cfgdir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgdir)
	t.Setenv("CIVITAI_TOKEN", "tok-default")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	// No CIVITAI_SUBMIT_PATH => must default to the v1 token route.
	t.Setenv("CIVITAI_SUBMIT_PATH", "")

	// --yes bypasses the confirmation gate (non-TTY test shell would otherwise refuse).
	stdout, _, err := run(t, "app", "submit", tmp, "--yes")
	if err != nil {
		t.Fatalf("submit default upload: %v\n%s", err, stdout)
	}
	if gotPath != "/api/v1/blocks/submit-version" {
		t.Errorf("submit path = %q, want /api/v1/blocks/submit-version", gotPath)
	}
	if gotAuth != "Bearer tok-default" {
		t.Errorf("auth = %q, want Bearer tok-default", gotAuth)
	}
	if gotBundle == "" {
		t.Error("server should have received a bundleBase64 body")
	}
	if !strings.Contains(stdout, "pr_77") {
		t.Errorf("output should report the publish request: %s", stdout)
	}
}

func TestAppSubmitSkipValidatePackagesAnyway(t *testing.T) {
	tmp := t.TempDir()
	// Invalid manifest (missing required fields) but parseable JSON.
	if err := os.WriteFile(filepath.Join(tmp, "block.manifest.json"),
		[]byte(`{"blockId":"x","version":"0.1.0"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(tmp, "out.zip")
	if _, _, err := run(t, "app", "submit", tmp, "--package-only", "--skip-validate", "--out", out); err != nil {
		t.Fatalf("submit --skip-validate: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("zip should be written with --skip-validate: %v", err)
	}
}
