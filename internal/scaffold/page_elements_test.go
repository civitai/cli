package scaffold

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/civitai/cli/internal/validate"
)

// page-elements — the web-components template, and the default.
//
// It mirrors `starters/civitai-block-starter-elements` in
// civitai/civitai-app-starters: the same dependencies at the same pins, the same
// `overrides`, the same src/, test/ and index.html. Where it deliberately
// differs, the template's own README says so ("Page, not slot"): it declares a
// `page` like every template here rather than the starter's model-slot target,
// so it drops the starter's `app.host.autoResize(root)` (RESIZE_IFRAME is N/A on
// a page — AGENTS.md item 11) and its dev harness sends an `app.page` context.

func renderPageElements(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "elements-probe")
	if _, err := Render(PageElements, dir, Data{Slug: "elements-probe", Name: "Elements Probe"}); err != nil {
		t.Fatalf("render page-elements: %v", err)
	}
	return dir
}

func TestRenderPageElements(t *testing.T) {
	dir := renderPageElements(t)

	for _, f := range []string{
		"block.manifest.json", "package.json", "index.html", "tsconfig.json",
		"vite.config.ts", "vitest.config.ts", ".gitignore", ".env.example", ".env.development",
		"README.md", "assets/README.md",
		"src/main.ts", "src/block.ts", "src/directLoad.ts", "src/index.css", "src/dev/harness.ts",
		"test/block.test.ts",
	} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f))); err != nil {
			t.Errorf("page-elements scaffold is missing %s: %v", f, err)
		}
	}

	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	pkg := string(raw)
	// 🔴 THE PIN CANARY. These four lines are literal pin sites that
	// `go run ./internal/scaffold/cmd/bump-pins` rewrites in lockstep with
	// templates/page-elements/package.json.tmpl — keep their exact
	// `"@civitai/<pkg>": "^X.Y.Z"` shape, or the bumper stops finding them.
	// The values mirror the elements starter (civitai-app-starters #557).
	mustContain(t, pkg, `"@civitai/app-sdk": "^0.58.0"`)
	mustContain(t, pkg, `"@civitai/components": "^0.9.2"`)
	mustContain(t, pkg, `"@civitai/sdk": "^0.10.1"`)
	mustContain(t, pkg, `"@civitai/theme": "^0.5.2"`)

	var doc struct {
		Name            string            `json:"name"`
		Scripts         map[string]string `json:"scripts"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
		Overrides       map[string]string `json:"overrides"`
		PNPM            struct {
			Overrides map[string]string `json:"overrides"`
		} `json:"pnpm"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("rendered package.json is not valid JSON: %v\n%s", err, raw)
	}
	if doc.Name != "elements-probe" {
		t.Errorf("package name = %q, want the slug", doc.Name)
	}
	if len(doc.Dependencies) != 4 {
		t.Errorf("page-elements should depend on exactly the four @civitai/* packages, got %v", doc.Dependencies)
	}
	wantDev := map[string]string{
		"typescript": "^5.9.2", "vite": "^8.0.14", "vitest": "^4.1.11",
		"happy-dom": "^20.9.0", "@types/node": "^25.9.1", "ajv": "^8.17.1",
	}
	if len(doc.DevDependencies) != len(wantDev) {
		t.Errorf("devDependencies = %v, want exactly %v", doc.DevDependencies, wantDev)
	}
	for name, pin := range wantDev {
		if doc.DevDependencies[name] != pin {
			t.Errorf("devDependency %s = %q, want %q (the starter's pin)", name, doc.DevDependencies[name], pin)
		}
	}
	wantOverrides := map[string]string{"cookie@<0.7.0": "^0.7.0", "postcss@<8.5.10": ">=8.5.10"}
	for _, set := range []struct {
		name string
		got  map[string]string
	}{{"overrides", doc.Overrides}, {"pnpm.overrides", doc.PNPM.Overrides}} {
		if len(set.got) != len(wantOverrides) {
			t.Errorf("%s = %v, want %v", set.name, set.got, wantOverrides)
		}
		for k, v := range wantOverrides {
			if set.got[k] != v {
				t.Errorf("%s[%q] = %q, want %q", set.name, k, set.got[k], v)
			}
		}
	}
	for _, s := range []string{"dev", "dev:harness", "build", "typecheck", "test", "preview"} {
		if _, ok := doc.Scripts[s]; !ok {
			t.Errorf("missing script %q: %v", s, doc.Scripts)
		}
	}

	// A page app, like every template here — the surface the run host reveals
	// only after BLOCK_READY, which @civitai/sdk sends.
	manifest, err := os.ReadFile(filepath.Join(dir, "block.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Page         *json.RawMessage `json:"page"`
		BuildCommand string           `json:"buildCommand"`
		OutputDir    string           `json:"outputDir"`
		BootSkeleton bool             `json:"bootSkeleton"`
	}
	if err := json.Unmarshal(manifest, &m); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if m.Page == nil || m.BuildCommand != "npm run build" || m.OutputDir != "dist" || !m.BootSkeleton {
		t.Errorf("page-elements manifest should declare a page, the npm build recipe and bootSkeleton:\n%s", manifest)
	}

	// The SDK's transport performs the ready-ack; a vendored emitter would race it.
	if PageElements.ReadyAckPath() != "" {
		t.Errorf("page-elements ReadyAckPath() = %q, want \"\" — @civitai/sdk acks", PageElements.ReadyAckPath())
	}
	for _, f := range []string{"civitai-host.js", "src/civitai-host.js"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f))); err == nil {
			t.Errorf("page-elements ships %s beside @civitai/sdk — two handshakes race", f)
		}
	}
	// No autoResize call on a page surface (RESIZE_IFRAME is N/A there).
	block, err := os.ReadFile(filepath.Join(dir, "src", "block.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`(?m)^\s*app\.host\.autoResize\(`).Match(block) {
		t.Error("page-elements src/block.ts calls app.host.autoResize — a page fills the host surface, and RESIZE_IFRAME is N/A there")
	}
	if !strings.Contains(string(block), "initialize(") {
		t.Error("CONTROL failure, not a finding: src/block.ts no longer calls initialize(), so the check above read the wrong file")
	}

	// The display name reaches the document title; the slug reaches the harness.
	html, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	mustContain(t, string(html), "<title>Elements Probe</title>")
	harness, _ := os.ReadFile(filepath.Join(dir, "src", "dev", "harness.ts"))
	mustContain(t, string(harness), "const DEV_BLOCK_ID = 'elements-probe';")
}

func TestParsePageElements(t *testing.T) {
	got, err := ParseTemplate("page-elements")
	if err != nil || got != PageElements {
		t.Fatalf("ParseTemplate(page-elements) = %q, %v", got, err)
	}
	if DefaultTemplate != PageElements {
		t.Errorf("DefaultTemplate = %q, want page-elements", DefaultTemplate)
	}
	if AllTemplates()[0] != PageElements {
		t.Errorf("AllTemplates() should list the default first, got %v", AllTemplates())
	}
}

// TestPageElementsScaffoldBuildsTestsAndValidates is the end-to-end guard: render
// the template, install, and run the app's own typecheck, tests and build, then
// the CLI's full `validate` (lockfile included) — the out-of-the-box contract.
// The `template-page-elements` CI job runs the same steps through the built CLI.
//
// Gated on CIVITAI_SCAFFOLD_TYPECHECK=1 (network + ~1 min), like
// TestPageMoneyScaffoldTypechecksItsOwnShippedTests. Plain `npm install`, the
// shape CI and the next-steps output use.
func TestPageElementsScaffoldBuildsTestsAndValidates(t *testing.T) {
	if os.Getenv(typecheckEnv) != "1" {
		t.Skipf("set %s=1 to render page-elements, npm install, and run its typecheck, tests, build "+
			"and `validate` (network, ~1 min). NOTHING about the scaffold's build was verified by this run.",
			typecheckEnv)
	}
	npm, err := exec.LookPath("npm")
	if err != nil {
		t.Fatalf("%s=1 was set and npm is not on PATH: %v", typecheckEnv, err)
	}
	dir := renderPageElements(t)

	step := func(name string, d time.Duration, args ...string) string {
		t.Helper()
		c := exec.Command(npm, args...)
		c.Dir = dir
		c.Env = append(os.Environ(), "npm_config_update_notifier=false", "CI=1", "NO_COLOR=1")
		out, err := runWithTimeout(c, d)
		if err != nil {
			t.Fatalf("%s failed in the rendered page-elements scaffold: %v\n%s", name, err, tail(out, 4000))
		}
		return out
	}

	step("npm install", 10*time.Minute, "install", "--no-audit", "--no-fund")
	step("npm run typecheck", 5*time.Minute, "run", "typecheck")

	// COUNT the tests rather than trusting vitest's exit code: a suite that
	// collected nothing exits 0 too.
	testOut := step("npm test", 5*time.Minute, "test")
	m := regexp.MustCompile(`Tests\s+(\d+) passed`).FindStringSubmatch(testOut)
	if m == nil {
		t.Fatalf("CONTROL failure: could not find vitest's `Tests N passed` summary:\n%s", tail(testOut, 3000))
	}
	if n, _ := strconv.Atoi(m[1]); n < 10 {
		t.Fatalf("vitest passed only %s test(s); the shipped test/block.test.ts carries 12 — the suite "+
			"is not running what the template ships:\n%s", m[1], tail(testOut, 3000))
	}
	if strings.Contains(testOut, " failed") {
		t.Fatalf("vitest reported failures:\n%s", tail(testOut, 3000))
	}
	t.Logf("vitest: %s tests passed", m[1])

	step("npm run build", 5*time.Minute, "run", "build")
	// The ack must survive bundling: Vite output is what the platform serves.
	var js []string
	_ = filepath.WalkDir(filepath.Join(dir, "dist"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".js") {
			js = append(js, p)
		}
		return nil
	})
	if len(js) == 0 {
		t.Fatal("the build emitted no JS into dist/")
	}
	var bundle strings.Builder
	for _, p := range js {
		b, _ := os.ReadFile(p)
		bundle.Write(b)
	}
	if !strings.Contains(bundle.String(), "civitai-text") {
		t.Fatal("CONTROL failure: the bundle does not mention civitai-text, so it is not the app's bundle")
	}
	if !strings.Contains(bundle.String(), "BLOCK_READY") {
		t.Error("the built bundle carries no BLOCK_READY — the SDK's ready-ack did not survive bundling")
	}

	res, err := validate.Dir(dir)
	if err != nil {
		t.Fatalf("validate.Dir: %v", err)
	}
	if !res.OK() {
		t.Fatalf("the installed page-elements scaffold fails `civitai app validate`: %v", validate.Messages(res.Errors))
	}
}
