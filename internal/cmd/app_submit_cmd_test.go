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

func TestAppSubmitFallbackPrintsManualSteps(t *testing.T) {
	tmp := t.TempDir()
	writeStaticManifest(t, tmp)

	// No token + no submit path => fallback to writing zip + manual steps.
	cfgdir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgdir)
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_SUBMIT_PATH", "")

	chdir(t, tmp)
	stdout, _, err := run(t, "app", "submit")
	if err != nil {
		t.Fatalf("submit fallback: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "not yet automated") || !strings.Contains(stdout, "/apps/submit") {
		t.Errorf("fallback should print manual next steps: %s", stdout)
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

	stdout, _, err := run(t, "app", "submit", tmp)
	if err != nil {
		t.Fatalf("submit upload: %v\n%s", err, stdout)
	}
	if gotAuth != "Bearer tok-xyz" {
		t.Errorf("server saw auth %q, want Bearer tok-xyz", gotAuth)
	}
	if !strings.Contains(stdout, "pr_42") || !strings.Contains(stdout, "pending") {
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
