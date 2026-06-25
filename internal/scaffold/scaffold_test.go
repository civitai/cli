package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderStatic(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "my-block")
	written, err := Render(Static, dest, Data{Slug: "my-block", Name: "My Block"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	want := []string{
		".gitignore",
		"README.md",
		"app.js",
		"block.manifest.json",
		"index.html",
		"style.css",
	}
	got := relNames(t, dest, written)
	assertSameSet(t, want, got)

	// Manifest must contain the slug + name and NOT carry build fields.
	manifest := readFile(t, filepath.Join(dest, "block.manifest.json"))
	mustContain(t, manifest, `"blockId": "my-block"`)
	mustContain(t, manifest, `"name": "My Block"`)
	if contains(manifest, "buildCommand") {
		t.Error("static template manifest should not declare buildCommand")
	}
}

func TestRenderPageVite(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "vite-block")
	written, err := Render(PageVite, dest, Data{Slug: "vite-block", Name: "Vite Block"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := relNames(t, dest, written)
	for _, expect := range []string{
		"block.manifest.json", "package.json", "vite.config.js", "index.html",
		"src/main.jsx", "src/App.jsx", "src/index.css", "README.md", ".gitignore",
	} {
		if !containsStr(got, expect) {
			t.Errorf("page-vite missing expected file %q (got %v)", expect, got)
		}
	}

	manifest := readFile(t, filepath.Join(dest, "block.manifest.json"))
	mustContain(t, manifest, `"buildCommand": "npm run build"`)
	mustContain(t, manifest, `"outputDir": "dist"`)

	// package.json name should use the slug, carry a postinstall next-step hint,
	// and stay valid JSON after rendering.
	pkg := readFile(t, filepath.Join(dest, "package.json"))
	mustContain(t, pkg, `"name": "vite-block"`)
	mustContain(t, pkg, `"postinstall"`)
	mustContain(t, pkg, "npm run dev")
	if !json.Valid([]byte(pkg)) {
		t.Errorf("page-vite package.json is not valid JSON:\n%s", pkg)
	}
}

func TestRenderPageMoney(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "money-block")
	written, err := Render(PageMoney, dest, Data{Slug: "money-block", Name: "Money Block"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := relNames(t, dest, written)
	for _, expect := range []string{
		"block.manifest.json", "package.json", "vite.config.ts", "tsconfig.json", "index.html",
		"src/main.tsx", "src/App.tsx", "src/Harness.tsx", "src/generation.ts",
		"src/generation.test.ts", "src/App.pollretry.test.tsx", "src/index.css",
		"README.md", ".gitignore", ".env.development", ".env.production", ".env.example",
	} {
		if !containsStr(got, expect) {
			t.Errorf("page-money missing expected file %q (got %v)", expect, got)
		}
	}

	// Money-path manifest: budgeted scope + per-gen budget + build fields.
	manifest := readFile(t, filepath.Join(dest, "block.manifest.json"))
	mustContain(t, manifest, `"ai:write:budgeted"`)
	mustContain(t, manifest, `"buzzBudgetPerGen": 10`)
	mustContain(t, manifest, `"buildCommand": "npm run build"`)
	mustContain(t, manifest, `"outputDir": "dist"`)
	// Server-owned fields must NOT be set.
	if contains(manifest, "iframe.src") || contains(manifest, `"src"`) {
		t.Error("page-money manifest must not declare iframe.src")
	}
	if contains(manifest, "trustTier") {
		t.Error("page-money manifest must not declare trustTier")
	}

	// package.json: SDK deps + the dev:harness script + slug name + a postinstall
	// next-step hint, all while staying valid JSON after rendering.
	pkg := readFile(t, filepath.Join(dest, "package.json"))
	mustContain(t, pkg, `"@civitai/blocks-react"`)
	mustContain(t, pkg, `"@civitai/app-sdk"`)
	mustContain(t, pkg, `"dev:harness"`)
	mustContain(t, pkg, `"build"`)
	mustContain(t, pkg, `"test"`)
	mustContain(t, pkg, `"name": "money-block"`)
	mustContain(t, pkg, `"postinstall"`)
	mustContain(t, pkg, "dev:harness")
	if !json.Valid([]byte(pkg)) {
		t.Errorf("page-money package.json is not valid JSON:\n%s", pkg)
	}

	// App.tsx must use the SDK, never raw postMessage('*').
	app := readFile(t, filepath.Join(dest, "src", "App.tsx"))
	mustContain(t, app, "useRequestConsent")
	mustContain(t, app, "useBuzzWorkflow")
	mustContain(t, app, "useBlockResize")
	if contains(app, "window.parent.postMessage") {
		t.Error("page-money App.tsx should not use raw window.parent.postMessage")
	}
	// The poll loop must retry transient transport errors (a throw), not turn
	// them into a terminal failure — guards the round-5 dogfood regression where
	// a poll blip marked a server-side SUCCESS as FAILED.
	mustContain(t, app, "MAX_TRANSIENT_ERRORS")
	mustContain(t, app, "consecutiveErrors")

	// The display name should land in the manifest + App heading.
	mustContain(t, manifest, `"name": "Money Block"`)
	mustContain(t, app, "Money Block")

	// The LiveUnavailable screen (shown by dev:live with no token) must tell the
	// user HOW to get one: the CLI one-liner with the real slug, a copy handler,
	// the VITE_LIVE_BLOCK_TOKEN paste instruction, and the personal-key note.
	mainTSX := readFile(t, filepath.Join(dest, "src", "main.tsx"))
	// The required order is submit-first (the token is minted against the
	// submitted/pending app) — the screen must surface `civitai app submit`
	// as the prerequisite step, not just the dev-token mint.
	mustContain(t, mainTSX, "civitai app submit")
	mustContain(t, mainTSX, "civitai app dev-token money-block")
	mustContain(t, mainTSX, "clipboard?.writeText")
	mustContain(t, mainTSX, "VITE_LIVE_BLOCK_TOKEN")
	mustContain(t, mainTSX, ".env.development.local")
	mustContain(t, mainTSX, "full-scope personal API key")
	mustContain(t, mainTSX, "civitai whoami")
	// Curl fallback for users without the CLI, carrying the real slug.
	mustContain(t, mainTSX, `"slug":"money-block"`)
	mustContain(t, mainTSX, "/api/v1/blocks/dev-token")

	// README docs the live-mode Buzz-balance recipe (#30) — the only working path
	// is the tRPC procedure with a bearer key (no public REST buzz endpoint).
	readme := readFile(t, filepath.Join(dest, "README.md"))
	mustContain(t, readme, "buzz.getBuzzAccount")
	mustContain(t, readme, "no public REST")
	// README docs the submission lifecycle + the withdraw escape hatch (#29).
	mustContain(t, readme, "Submission lifecycle")
	mustContain(t, readme, "civitai app withdraw")
	mustContain(t, readme, "pending")
}

func TestPageMoneyNeedsHarness(t *testing.T) {
	if !PageMoney.NeedsHarness() {
		t.Error("page-money should need the dev harness")
	}
	if Static.NeedsHarness() || PageVite.NeedsHarness() {
		t.Error("static/page-vite should not need the dev harness")
	}
}

func TestOutputNameMapsEnvFiles(t *testing.T) {
	cases := map[string]string{
		"env.development.tmpl": ".env.development",
		"env.production.tmpl":  ".env.production",
		"env.example.tmpl":     ".env.example",
	}
	for in, want := range cases {
		if got := outputName(in); got != want {
			t.Errorf("outputName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderRefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "existing"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Render(Static, dir, Data{Slug: "abc", Name: "Abc"}); err == nil {
		t.Fatal("expected error rendering into a non-empty dir")
	}
}

func TestParseTemplate(t *testing.T) {
	if _, err := ParseTemplate("static"); err != nil {
		t.Errorf("static should parse: %v", err)
	}
	if _, err := ParseTemplate("page-vite"); err != nil {
		t.Errorf("page-vite should parse: %v", err)
	}
	if _, err := ParseTemplate("page-money"); err != nil {
		t.Errorf("page-money should parse: %v", err)
	}
	if _, err := ParseTemplate("nope"); err == nil {
		t.Error("unknown template should error")
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"My Cool Block": "my-cool-block",
		"weird__Name!!": "weird-name",
		"already-slug":  "already-slug",
	}
	for in, want := range cases {
		got, err := Slugify(in)
		if err != nil {
			t.Errorf("Slugify(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := Slugify("a"); err == nil {
		t.Error("too-short name should error")
	}
}

func TestValidateSlug(t *testing.T) {
	if err := ValidateSlug("good-slug"); err != nil {
		t.Errorf("good-slug rejected: %v", err)
	}
	for _, bad := range []string{"ab", "Bad", "1starts", "-leading", "trailing-", "has space"} {
		if err := ValidateSlug(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

// --- helpers ---

func relNames(t *testing.T, dest string, written []string) []string {
	t.Helper()
	var out []string
	for _, w := range written {
		rel, err := filepath.Rel(dest, w)
		if err != nil {
			t.Fatalf("rel: %v", err)
		}
		out = append(out, filepath.ToSlash(rel))
	}
	return out
}

func assertSameSet(t *testing.T, want, got []string) {
	t.Helper()
	if len(want) != len(got) {
		t.Errorf("file count = %d, want %d\n got=%v\nwant=%v", len(got), len(want), got, want)
	}
	for _, w := range want {
		if !containsStr(got, w) {
			t.Errorf("missing expected file %q (got %v)", w, got)
		}
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !contains(haystack, needle) {
		t.Errorf("expected to contain %q\n--- content ---\n%s", needle, haystack)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func containsStr(set []string, s string) bool {
	for _, x := range set {
		if x == s {
			return true
		}
	}
	return false
}
