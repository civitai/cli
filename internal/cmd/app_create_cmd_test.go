package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/validate"
)

// simulateInstall writes the lockfile a scaffolded npm project gets from its
// first `npm install` (next step 1). `validate` requires it because the platform
// build installs strictly from the committed lockfile; the scaffold cannot ship
// one (a checked-in lockfile would be stale on arrival), so a test that asserts
// the FULL `validate` passes has to stand in for the author's install.
func simulateInstall(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err != nil {
		return // static template: no install step, no lockfile
	}
	if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write package-lock.json: %v", err)
	}
}

// assertScaffoldValid asserts the scaffolded dir is non-empty and passes the
// same self-validation the scaffold commands run.
func assertScaffoldValid(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read scaffold dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("scaffold dir is empty")
	}
	if _, err := os.Stat(filepath.Join(dir, "block.manifest.json")); err != nil {
		t.Errorf("block.manifest.json missing: %v", err)
	}
	simulateInstall(t, dir)
	res, err := validate.Dir(dir)
	if err != nil {
		t.Fatalf("validate.Dir: %v", err)
	}
	if !res.OK() {
		t.Errorf("scaffold failed self-validation: %v", res.Errors)
	}
}

func TestAppCreateDefaultsToPageMoney(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "my-block")

	stdout, _, err := run(t, "app", "create", "my-block", dest)
	if err != nil {
		t.Fatalf("app create: %v\n%s", err, stdout)
	}

	// Defaulted to the batteries-included page-money template (named in the
	// one-line summary).
	if !strings.Contains(stdout, "(page-money)") {
		t.Errorf("create should default to page-money:\n%s", stdout)
	}
	// The summary reports a file COUNT, not a per-file tree.
	if !strings.Contains(stdout, "files") {
		t.Errorf("create should report a file count in the summary:\n%s", stdout)
	}
	// The per-file tree is gone: no individual scaffold filename leaks into output.
	for _, leaked := range []string{"vite.config.ts", "block.manifest.json", "package.json"} {
		if strings.Contains(stdout, leaked) {
			t.Errorf("create should NOT list individual files (%q leaked):\n%s", leaked, stdout)
		}
	}
	// Numbered, scannable next steps.
	if !strings.Contains(stdout, "1. cd ") || !strings.Contains(stdout, "npm install") {
		t.Errorf("create should print a numbered cd+install step:\n%s", stdout)
	}
	// Onboarding must LEAD with the free, works-today path (`dev:harness`, mock
	// host, no beta access) and DEMOTE the invite-only beta surfaces (`civitai app
	// submit` + the dev tunnel) below it — a newcomer should hit the free path
	// first, not the gated wall. Assert the RELATIVE ORDER, not just presence.
	iHarness := strings.Index(stdout, "dev:harness")
	iSubmit := strings.Index(stdout, "civitai app submit")
	iTunnel := strings.Index(stdout, "civitai app dev-tunnel")
	if iHarness < 0 {
		t.Errorf("create should lead with the free dev:harness path:\n%s", stdout)
	}
	if iSubmit < 0 {
		t.Errorf("create should print the submit next step:\n%s", stdout)
	}
	if iTunnel < 0 {
		t.Errorf("create should print the dev-tunnel preview step:\n%s", stdout)
	}
	// dev:harness must come BEFORE submit AND before the dev tunnel.
	if iHarness >= 0 && iSubmit >= 0 && iHarness > iSubmit {
		t.Errorf("dev:harness (free path) must lead BEFORE civitai app submit (beta):\n%s", stdout)
	}
	if iHarness >= 0 && iTunnel >= 0 && iHarness > iTunnel {
		t.Errorf("dev:harness (free path) must lead BEFORE the dev-tunnel step (beta):\n%s", stdout)
	}
	// Among the demoted beta surfaces, submit still precedes the tunnel (the
	// tunnel resolves the app server-side, so it must be registered first).
	if iSubmit >= 0 && iTunnel >= 0 && iSubmit > iTunnel {
		t.Errorf("submit must be sequenced BEFORE the dev-tunnel step (register first):\n%s", stdout)
	}
	// The beta surfaces are gated behind an honest "beta access" heading.
	if !strings.Contains(stdout, "beta access") {
		t.Errorf("create should gate submit/dev-tunnel under a beta-access heading:\n%s", stdout)
	}
	// The Comfy Cloud (customComfy) sample is surfaced (finding #2: it was
	// undiscoverable) — honestly flagged as invite-only beta.
	if !strings.Contains(stdout, "Comfy Cloud") {
		t.Errorf("create should surface the Comfy Cloud sample in the output:\n%s", stdout)
	}
	// The trimmed message drops the old multi-line Buzz/OAuth/personal-key
	// paragraph (dev:live now lives in dev-token/README, not the scaffold banner).
	if strings.Contains(stdout, "dev:live") {
		t.Errorf("create next steps should NOT re-bloat with the dev:live Buzz paragraph:\n%s", stdout)
	}

	assertScaffoldValid(t, dest)

	// page-money signature files exist.
	for _, f := range []string{"package.json", "vite.config.ts", "tsconfig.json", "src/App.tsx"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(f))); err != nil {
			t.Errorf("page-money file %q missing: %v", f, err)
		}
	}
	// The page-money manifest carries the money-path scope + build fields.
	manifest, err := os.ReadFile(filepath.Join(dest, "block.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), "ai:write:budgeted") {
		t.Errorf("page-money manifest missing the budgeted scope:\n%s", manifest)
	}
}

// TestAppCreateHelpMentionsComfy guards finding #2: the Comfy Cloud sample must
// be discoverable from `civitai app create --help` (it was previously described
// as a txt2img-only app).
func TestAppCreateHelpMentionsComfy(t *testing.T) {
	out, _, err := run(t, "app", "create", "--help")
	if err != nil {
		t.Fatalf("app create --help: %v", err)
	}
	for _, want := range []string{"Comfy Cloud", "customComfy"} {
		if !strings.Contains(out, want) {
			t.Errorf("app create help should mention %q (Comfy Cloud discoverability):\n%s", want, out)
		}
	}
}

func TestAppCreateRespectsTemplateOverride(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "static-block")

	stdout, _, err := run(t, "app", "create", "static-block", dest, "--template", "static")
	if err != nil {
		t.Fatalf("app create --template static: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "(static)") {
		t.Errorf("--template static should override the page-money default:\n%s", stdout)
	}
	assertScaffoldValid(t, dest)
	// Static is no-build: no package.json.
	if _, err := os.Stat(filepath.Join(dest, "package.json")); err == nil {
		t.Error("static template should not produce a package.json")
	}
}

func TestAppCreateDerivesSlugFromDisplayName(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "out")

	stdout, _, err := run(t, "app", "create", "My Cool Block", dest)
	if err != nil {
		t.Fatalf("app create with display name: %v\n%s", err, stdout)
	}
	// The display name slugifies to my-cool-block in the manifest (the
	// summary line no longer prints the slug separately).
	manifest := readManifest(t, dest)
	if !strings.Contains(manifest, `"blockId": "my-cool-block"`) {
		t.Errorf("display name should slugify to my-cool-block:\n%s", manifest)
	}
	if !strings.Contains(stdout, `"My Cool Block"`) {
		t.Errorf("output should report the display name:\n%s", stdout)
	}
}

func TestAppCreateRefusesNonEmptyDir(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "existing"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "app", "create", "my-block", tmp); err == nil {
		t.Fatal("expected create to refuse a non-empty dir")
	}
}

func TestAppCreateFromIsNotWired(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "out")
	_, errOut, err := run(t, "app", "create", "my-block", dest, "--from", "some-slug")
	if err == nil {
		t.Fatal("expected --from to error (not yet wired)")
	}
	if !strings.Contains(err.Error()+errOut, "not yet wired") {
		t.Errorf("--from should report it is not wired: err=%v stderr=%s", err, errOut)
	}
}

// TestAppInitStillDefaultsToStatic guards the back-compat alias: the refactor to
// the shared helper must not change init's default template.
func TestAppInitStillDefaultsToStatic(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "init-block")

	stdout, _, err := run(t, "app", "init", "init-block", dest)
	if err != nil {
		t.Fatalf("app init: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "(static)") {
		t.Errorf("init should still default to static:\n%s", stdout)
	}
	assertScaffoldValid(t, dest)
	if _, err := os.Stat(filepath.Join(dest, "package.json")); err == nil {
		t.Error("init's static default should not produce a package.json")
	}
}

func TestAppInitPageMoneyViaFlag(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "init-money")

	stdout, _, err := run(t, "app", "init", "init-money", dest, "--template", "page-money")
	if err != nil {
		t.Fatalf("app init --template page-money: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "(page-money)") {
		t.Errorf("init --template page-money should scaffold page-money:\n%s", stdout)
	}
	assertScaffoldValid(t, dest)
}
