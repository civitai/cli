package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// run executes the root command with args, capturing stdout+stderr.
func run(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	root := NewRootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errb.String(), err
}

func TestRootHelpListsCommands(t *testing.T) {
	out, _, err := run(t, "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	for _, want := range []string{"app", "login", "whoami", "buzz", "version", "completion"} {
		if !strings.Contains(out, want) {
			t.Errorf("root help missing command %q\n%s", want, out)
		}
	}
}

func TestVersionCommand(t *testing.T) {
	// Keep this test hermetic — don't reach out to GitHub for the update check.
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "1")
	out, _, err := run(t, "version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	for _, want := range []string{"civitai", "commit:", "built:", "go:"} {
		if !strings.Contains(out, want) {
			t.Errorf("version output missing %q: %s", want, out)
		}
	}
}

func TestSetBuildInfo(t *testing.T) {
	origV, origC, origD := version, commit, date
	t.Cleanup(func() { version, commit, date = origV, origC, origD })

	SetBuildInfo("1.2.3", "abc123", "2026-01-01")
	if version != "1.2.3" || commit != "abc123" || date != "2026-01-01" {
		t.Fatalf("SetBuildInfo did not apply: %s %s %s", version, commit, date)
	}
	// Empty values must not clobber existing.
	SetBuildInfo("", "", "")
	if version != "1.2.3" || commit != "abc123" || date != "2026-01-01" {
		t.Errorf("empty SetBuildInfo clobbered values: %s %s %s", version, commit, date)
	}
}

func TestCompletionGeneratesForEachShell(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		out, _, err := run(t, "completion", shell)
		if err != nil {
			t.Fatalf("completion %s: %v", shell, err)
		}
		if len(out) == 0 {
			t.Errorf("completion %s produced no output", shell)
		}
	}
}

func TestCompletionRejectsUnknownShell(t *testing.T) {
	if _, _, err := run(t, "completion", "tcsh"); err == nil {
		t.Fatal("expected error for unknown shell")
	}
}

func TestAppHelp(t *testing.T) {
	out, _, err := run(t, "app", "--help")
	if err != nil {
		t.Fatalf("app --help: %v", err)
	}
	for _, want := range []string{"init", "validate", "submit"} {
		if !strings.Contains(out, want) {
			t.Errorf("app help missing subcommand %q", want)
		}
	}
}

func TestAppInitScaffoldsAndValidates(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)

	out, _, err := run(t, "app", "init", "my-block")
	if err != nil {
		t.Fatalf("app init: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Created App") {
		t.Errorf("unexpected init output: %s", out)
	}
	// Manifest must exist and validate clean.
	if _, err := os.Stat(filepath.Join(tmp, "my-block", "block.manifest.json")); err != nil {
		t.Fatalf("scaffolded manifest missing: %v", err)
	}
	if _, _, err := run(t, "app", "validate", filepath.Join(tmp, "my-block")); err != nil {
		t.Errorf("scaffolded project should validate: %v", err)
	}
}

func TestAppInitPageViteTemplate(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	out, _, err := run(t, "app", "init", "vite-app", "--template", "page-vite")
	if err != nil {
		t.Fatalf("app init page-vite: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(tmp, "vite-app", "package.json")); err != nil {
		t.Errorf("page-vite should scaffold package.json: %v", err)
	}
	if !strings.Contains(out, "npm install") {
		t.Errorf("page-vite next steps should mention npm install: %s", out)
	}
}

func TestAppInitPageMoneyTemplate(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	out, _, err := run(t, "app", "init", "money-app", "--template", "page-money")
	if err != nil {
		t.Fatalf("app init page-money: %v\n%s", err, out)
	}
	// SDK-wired files present.
	for _, f := range []string{"package.json", "src/App.tsx", "src/Harness.tsx", "src/generation.ts", ".env.example"} {
		if _, err := os.Stat(filepath.Join(tmp, "money-app", filepath.FromSlash(f))); err != nil {
			t.Errorf("page-money should scaffold %s: %v", f, err)
		}
	}
	// Next steps center the dev-tunnel (prod-fidelity preview) and keep dev:harness
	// as the offline/mock fallback tip. Plain `dev` renders blank without a host.
	if !strings.Contains(out, "civitai app dev-tunnel") {
		t.Errorf("page-money next steps should highlight the dev-tunnel: %s", out)
	}
	if !strings.Contains(out, "npm run dev:harness") {
		t.Errorf("page-money next steps should keep the dev:harness fallback: %s", out)
	}
	if strings.Contains(out, "npm run dev\n") {
		t.Errorf("page-money next steps should NOT suggest plain `npm run dev`: %s", out)
	}
	// And it must validate clean (the scaffold sets a budget so no warnings)
	// once the author has installed — `validate` requires the committed lockfile
	// the platform build installs strictly from.
	simulateInstall(t, filepath.Join(tmp, "money-app"))
	if _, _, err := run(t, "app", "validate", filepath.Join(tmp, "money-app")); err != nil {
		t.Errorf("scaffolded page-money should validate: %v", err)
	}
}

func TestAppInitDirFlag(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	target := filepath.Join("nested", "custom-dir")
	out, _, err := run(t, "app", "init", "my-block", "--dir", target)
	if err != nil {
		t.Fatalf("app init --dir: %v\n%s", err, out)
	}
	// Project lands in the custom dir, not ./<slug>.
	if _, err := os.Stat(filepath.Join(tmp, target, "block.manifest.json")); err != nil {
		t.Errorf("expected manifest in --dir target: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "my-block")); err == nil {
		t.Error("--dir should NOT create the default ./<slug> dir")
	}
	// The slug is still my-block (independent of the dir).
	manifest := readManifest(t, filepath.Join(tmp, target))
	if !strings.Contains(manifest, `"blockId": "my-block"`) {
		t.Errorf("slug should remain my-block: %s", manifest)
	}
	// Next steps should cd into the target dir, not the slug.
	if !strings.Contains(out, filepath.ToSlash(target)) && !strings.Contains(out, target) {
		t.Errorf("next steps should reference the target dir: %s", out)
	}
}

func TestAppInitPositionalDir(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	_, _, err := run(t, "app", "init", "my-block", "out-dir")
	if err != nil {
		t.Fatalf("app init <name> <dir>: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "out-dir", "block.manifest.json")); err != nil {
		t.Errorf("positional dir should be used: %v", err)
	}
}

func TestAppInitNameFlagSeparatesNameFromSlug(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	_, _, err := run(t, "app", "init", "my-block", "--name", "My Fancy Block")
	if err != nil {
		t.Fatalf("app init --name: %v", err)
	}
	manifest := readManifest(t, filepath.Join(tmp, "my-block"))
	// Display name from --name, slug from the positional arg — they differ.
	if !strings.Contains(manifest, `"name": "My Fancy Block"`) {
		t.Errorf("display name should be from --name: %s", manifest)
	}
	if !strings.Contains(manifest, `"blockId": "my-block"`) {
		t.Errorf("slug should remain my-block: %s", manifest)
	}
}

func TestAppInitConflictingDir(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	_, _, err := run(t, "app", "init", "my-block", "posdir", "--dir", "flagdir")
	if err == nil {
		t.Fatal("expected error for conflicting positional dir and --dir")
	}
	if !strings.Contains(err.Error(), "conflicting") {
		t.Errorf("error should mention the conflict: %v", err)
	}
}

func TestAppValidateWarningsExitZero(t *testing.T) {
	tmp := t.TempDir()
	// A budgeted page with no budget: warns but is otherwise valid -> exit 0.
	writeBudgetedNoBudgetManifest(t, tmp)
	out, errOut, err := run(t, "app", "validate", tmp)
	if err != nil {
		t.Fatalf("warning-only validate should exit 0: %v\nstderr: %s", err, errOut)
	}
	if !strings.Contains(errOut, "warning(s)") {
		t.Errorf("stderr should list warnings: %s", errOut)
	}
	if !strings.Contains(out, "is valid") || !strings.Contains(out, "warning(s)") {
		t.Errorf("stdout should report valid-with-warnings: %s", out)
	}
}

func TestAppValidateStrictFailsOnWarnings(t *testing.T) {
	tmp := t.TempDir()
	writeBudgetedNoBudgetManifest(t, tmp)
	_, errOut, err := run(t, "app", "validate", tmp, "--strict")
	if err == nil {
		t.Fatal("expected --strict to fail on warnings")
	}
	if !strings.Contains(errOut, "warning(s)") {
		t.Errorf("stderr should list warnings under --strict: %s", errOut)
	}
}

func TestAppInitRequiresName(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	if _, _, err := run(t, "app", "init"); err == nil {
		t.Fatal("expected error when no name is given")
	}
}

func TestAppInitFromIsNotWired(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	_, _, err := run(t, "app", "init", "forked", "--from", "some-slug")
	if err == nil {
		t.Fatal("expected --from to be reported as not wired")
	}
	if !strings.Contains(err.Error(), "not yet wired") {
		t.Errorf("error should say not yet wired: %v", err)
	}
}

func TestAppInitUnknownTemplate(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	if _, _, err := run(t, "app", "init", "x", "--template", "nope"); err == nil {
		t.Fatal("expected error for unknown template")
	}
}

func TestAppInitSlugifiesDisplayName(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	out, _, err := run(t, "app", "init", "My Cool Block")
	if err != nil {
		t.Fatalf("app init: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(tmp, "my-cool-block", "block.manifest.json")); err != nil {
		t.Errorf("expected slug dir my-cool-block: %v", err)
	}
}

func TestAppValidateReportsErrors(t *testing.T) {
	tmp := t.TempDir()
	// A manifest missing required fields.
	if err := os.WriteFile(filepath.Join(tmp, "block.manifest.json"), []byte(`{"blockId":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, errOut, err := run(t, "app", "validate", tmp)
	if err == nil {
		t.Fatal("expected validation to fail for a bad manifest")
	}
	if !strings.Contains(errOut, "validation error") {
		t.Errorf("stderr should list validation errors: %s", errOut)
	}
}

func TestAppValidateMissingManifest(t *testing.T) {
	tmp := t.TempDir()
	_, _, err := run(t, "app", "validate", tmp)
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
}

func TestWhoAmIWithoutToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "")
	_, _, err := run(t, "whoami")
	if err == nil {
		t.Fatal("expected error when no token configured")
	}
	if !strings.Contains(err.Error(), "no token") {
		t.Errorf("error should mention missing token: %v", err)
	}
}

func TestWhoAmISuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"username":"bob","id":99}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, _, err := run(t, "whoami")
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	if !strings.Contains(out, "bob") || !strings.Contains(out, "99") {
		t.Errorf("whoami output should report the user: %s", out)
	}
}

// TestWhoAmISubmitAppsCapability drives the "Submit Apps: yes/no" capability
// line off the token-scope bitmask whoami already fetches. Bit 25
// (ScopeAppBlocksSubmit = 1<<25 = 33554432) is the App-Blocks submit scope; a
// token carrying it must report "yes", one lacking it "no".
func TestWhoAmISubmitAppsCapability(t *testing.T) {
	const submitBit = 1 << 25 // ScopeAppBlocksSubmit
	cases := []struct {
		name       string
		tokenScope int
		want       string
	}{
		{"has submit scope", submitBit | 1, "Submit Apps:              yes"},
		{"lacks submit scope", 1, "Submit Apps:              no"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"username":"bob","id":99,"tokenScope":%d}`, tc.tokenScope)
			}))
			defer srv.Close()

			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", dir)
			t.Setenv("CIVITAI_TOKEN", "tok-1")
			t.Setenv("CIVITAI_BASE_URL", srv.URL)

			out, _, err := run(t, "whoami")
			if err != nil {
				t.Fatalf("whoami: %v", err)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("whoami output should contain %q, got:\n%s", tc.want, out)
			}
		})
	}
}

func TestLoginStoresToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "")
	out, _, err := run(t, "login", "--token", "abc123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !strings.Contains(out, "Token saved") {
		t.Errorf("login should confirm save: %s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "civitai", "config.yaml")); err != nil {
		t.Errorf("config file should be written: %v", err)
	}
}

// TestLoginDeviceStartFailureReturnsError drives the DEFAULT `civitai login`
// (no --token => OAuth device flow) against an httptest server whose device-init
// endpoint fails, and asserts login returns an error FAST.
//
// This replaces the old TestLoginEmptyTokenViaStdinFails, which assumed the
// default path read a token from stdin ("no token provided"). It no longer
// does: with no --token, login runs the device flow and makes a real network
// call to base_url. With no CIVITAI_BASE_URL override that call hit the REAL
// https://civitai.com — when egress was open, device-init succeeded and the
// poll looped until the device-code expired (~10 min), tripping Go's 10-min
// test timeout (network-dependent flake). Here we point the CLI at a local
// server (t.Setenv CIVITAI_BASE_URL = srv.URL) that returns 4xx on device-init,
// so StartDevice errors immediately, no polling, no real network, sub-second.
func TestLoginDeviceStartFailureReturnsError(t *testing.T) {
	var gotDeviceInit bool
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The device flow first resolves endpoints via OpenID discovery, which
		// advertises the endpoints back at this server.
		if r.URL.Path == "/.well-known/openid-configuration" {
			writeLoginDiscovery(w, base)
			return
		}
		// Device-init is the first (and here, only) OAuth request the device flow makes.
		if r.URL.Path == "/api/auth/oauth/device" {
			gotDeviceInit = true
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_client"})
			return
		}
		// Reaching the poll endpoint would mean StartDevice wrongly succeeded.
		t.Errorf("unexpected path %q — login should fail at device-init, never poll", r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	base = srv.URL

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "")
	// Hermetic: pin the CLI at the local server so it NEVER touches civitai.com.
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	start := time.Now()
	// --no-browser so a failure can't spawn anything; the device flow still runs.
	_, _, err := run(t, "login", "--no-browser")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error when device-init fails")
	}
	if !strings.Contains(err.Error(), "device init failed") {
		t.Errorf("error should report the device-init failure, got: %v", err)
	}
	if !gotDeviceInit {
		t.Error("login should have hit the local device-init endpoint")
	}
	// Deterministic + fast: a clean device-init failure must not poll/wait.
	if elapsed > 5*time.Second {
		t.Errorf("login took %v — should fail fast without polling", elapsed)
	}
	// Nothing persisted on a failed login.
	if _, statErr := os.Stat(filepath.Join(dir, "civitai", "config.yaml")); statErr == nil {
		t.Error("no config should be written on a failed login")
	}
}

// TestFlagErrorTaggedAsUsage exercises the REAL SetFlagErrorFunc → asUsageError
// wiring on the root command (not a reimplemented stand-in): a bad flag must
// yield an error that satisfies errors.Is(err, ErrUsage) — which the entrypoint
// maps to exit code 2 — while the user-visible message stays Cobra's verbatim.
func TestFlagErrorTaggedAsUsage(t *testing.T) {
	_, _, err := run(t, "images", "search", "--nope")
	if err == nil {
		t.Fatal("expected a flag error")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("flag error must be tagged ErrUsage (→ exit 2), got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("message should be Cobra's verbatim flag error, got: %v", err)
	}
}

func TestJoinLines(t *testing.T) {
	if got := joinLines([]string{"a", "b"}); got != "a\n  b" {
		t.Errorf("joinLines = %q", got)
	}
	if got := joinLines(nil); got != "" {
		t.Errorf("joinLines(nil) = %q", got)
	}
}

func readManifest(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "block.manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	return string(b)
}

func writeBudgetedNoBudgetManifest(t *testing.T, dir string) {
	t.Helper()
	const m = `{
  "$schema": "https://civitai.com/schemas/app-block/v1.json",
  "blockId": "no-budget",
  "version": "0.1.0",
  "name": "No Budget",
  "type": "block",
  "scopes": ["ai:write:budgeted"],
  "scopeJustifications": { "ai:write:budgeted": "spends the page's per-gen Buzz budget to run generations" },
  "page": { "path": "/", "title": "No Budget" },
  "iframe": { "minHeight": 600, "resizable": true, "sandbox": "allow-scripts allow-forms" },
  "contentRating": "g",
  "buildCommand": "npm run build",
  "outputDir": "dist"
}`
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}
}

// chdir changes into dir for the duration of the test.
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}
