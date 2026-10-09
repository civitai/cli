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
// (synced to dbd5fca): the same dependencies (the @civitai/* ones pinned in this
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
		"test/block.test.ts", "test/elements.test.ts",
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
	mustContain(t, pkg, `"@civitai/app-sdk": "^0.60.0"`)
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

// TestPageElementsDevAllowlistAndPort pins the dev-server origin and the parent
// origins the bridge accepts in dev. Both must match page-money: port 5186 is
// `civitai app dev-tunnel`'s default, and `https://civitai.com` is the origin
// the REAL host posts BLOCK_INIT from when the tunnel embeds the dev server.
// Without it the bridge drops that BLOCK_INIT and the app waits forever.
func TestPageElementsDevAllowlistAndPort(t *testing.T) {
	const want = "VITE_BLOCK_ALLOWED_PARENT_ORIGINS=http://localhost:5186,https://civitai.com"
	for _, tmpl := range []Template{PageElements, PageMoney} {
		dir := filepath.Join(t.TempDir(), "probe")
		if _, err := Render(tmpl, dir, Data{Slug: "probe-app", Name: "Probe App"}); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(filepath.Join(dir, ".env.development"))
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "VITE_BLOCK_ALLOWED_PARENT_ORIGINS=") {
				got = append(got, line)
			}
		}
		if len(got) != 1 || got[0] != want {
			t.Errorf("%s .env.development allowlist = %q, want exactly [%q]", tmpl, got, want)
		}
		vite, err := os.ReadFile(filepath.Join(dir, "vite.config.ts"))
		if err != nil {
			t.Fatal(err)
		}
		if !regexp.MustCompile(`(?m)^\s*port: 5186,`).Match(vite) {
			t.Errorf("%s vite.config.ts does not pin the dev server to port 5186", tmpl)
		}
	}
}

// TestPageElementsAsksForNoScopes: the page app reads nothing through the API,
// so it declares no scope — like `static` and `page-vite` — and neither its
// harness nor its tests mint a token claiming one.
func TestPageElementsAsksForNoScopes(t *testing.T) {
	dir := renderPageElements(t)
	raw, err := os.ReadFile(filepath.Join(dir, "block.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Scopes *[]string `json:"scopes"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.Scopes == nil || len(*m.Scopes) != 0 {
		t.Errorf("page-elements manifest scopes = %v, want an explicit empty list", m.Scopes)
	}
	for _, f := range []string{"src/dev/harness.ts", "test/block.test.ts"} {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f)))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "models:read:self") {
			t.Errorf("%s still mints a token claiming models:read:self", f)
		}
		if !strings.Contains(string(b), "scopes: []") {
			t.Errorf("CONTROL failure: %s has no `scopes: []` token fixture — the check above read the wrong file", f)
		}
	}
}

// TestPageElementsNeverAdvisesThePlainMeRoute: a block cannot read the viewer
// through the plain `me` route. On civitai, `/api/v1/me` is an AuthedEndpoint
// (session cookie or API key) with no block-scope wrapper, so a block token gets
// a 401; the block route is `/api/v1/blocks/me`, wrapped in withBlockScope with
// `requiredScope: 'user:read:self'` — a consent-gated scope. 0.1.114 shipped
// `app.site.get('me')` as the advice in src/block.ts.
//
// The scan covers EVERY rendered file (comments included — a comment is the
// advice an author copies). A positive control proves the pattern matches the
// exact spelling that shipped, and the replacement advice must name both the
// block route and the scope the manifest has to declare.
func TestPageElementsNeverAdvisesThePlainMeRoute(t *testing.T) {
	plainMe := regexp.MustCompile("site\\.get(<[^>]*>)?\\(\\s*['\"`]/?me['\"`?]")
	if !plainMe.MatchString("read it from the API (`app.site.get('me')`)") {
		t.Fatal("CONTROL failure: the pattern does not match the spelling 0.1.114 shipped")
	}
	if plainMe.MatchString("app.site.get('blocks/me')") {
		t.Fatal("CONTROL failure: the pattern matches the CORRECT route")
	}

	dir := renderPageElements(t)
	files := 0
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		files++
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			if plainMe.MatchString(line) {
				rel, _ := filepath.Rel(dir, p)
				t.Errorf("%s:%d advises the plain `me` route, which refuses a block token (401) — use `blocks/me` and declare `user:read:self`:\n  %s",
					rel, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 15 {
		t.Fatalf("CONTROL failure: scanned only %d rendered file(s)", files)
	}

	block, err := os.ReadFile(filepath.Join(dir, "src", "block.ts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"app.site.get('blocks/me')", "user:read:self"} {
		if !strings.Contains(string(block), want) {
			t.Errorf("src/block.ts's viewer-identity advice should name %q", want)
		}
	}
}
