package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// page-elements — the web-components template, and the default.
//
// It mirrors `starters/civitai-block-starter` in civitai/civitai-app-starters
// (synced to b3213cd): the same dependencies (the @civitai/* ones pinned in this
// repo's `^X.Y.0` form), the same devDependencies and `overrides`, the same
// main.ts, harness, index.html, index.css and config. Where it deliberately
// differs, the template's own README says so ("Page, not slot"): it declares a
// `page` like every template here rather than the starter's model-slot target,
// so it drops the starter's `app.host.autoResize(root)` (RESIZE_IFRAME is N/A on
// a page — AGENTS.md item 11), narrows `app.page` instead of a model context,
// and — like page-money — ships no direct-load fallback (src/directLoad.ts).
//
// The end-to-end check (install, typecheck, the app's own tests, build,
// validate) is the `template-page-elements` CI job, which scaffolds with no
// --template; `bump-scaffold-pins.yml` repeats it after a pin bump.

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
		"src/main.ts", "src/block.ts", "src/index.css", "src/dev/harness.ts",
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
	// The minors mirror the starter (civitai-app-starters #557), in the
	// `^X.Y.0` form bump-pins writes.
	mustContain(t, pkg, `"@civitai/app-sdk": "^0.58.0"`)
	mustContain(t, pkg, `"@civitai/components": "^0.9.0"`)
	mustContain(t, pkg, `"@civitai/sdk": "^0.10.0"`)
	mustContain(t, pkg, `"@civitai/theme": "^0.5.0"`)

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

	// No direct-load fallback, matching page-money (which ships none).
	if _, err := os.Stat(filepath.Join(dir, "src", "directLoad.ts")); err == nil {
		t.Error("page-elements ships src/directLoad.ts — page-money has no direct-load fallback, so neither does this")
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
