package scaffold

import (
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

	// package.json name should use the slug.
	pkg := readFile(t, filepath.Join(dest, "package.json"))
	mustContain(t, pkg, `"name": "vite-block"`)
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
