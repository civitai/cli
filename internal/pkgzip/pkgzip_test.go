package pkgzip

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildExcludesArtifacts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "src/main.jsx", "export default 1")
	writeFile(t, dir, "package.json", "{}")
	// These must be excluded:
	writeFile(t, dir, "node_modules/react/index.js", "module.exports = {}")
	writeFile(t, dir, "dist/bundle.js", "console.log(1)")
	writeFile(t, dir, ".git/HEAD", "ref: refs/heads/main")

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	got := namesInZip(t, res.Zip)
	want := []string{"block.manifest.json", "package.json", "src/main.jsx"}
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("zip contents = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("zip[%d] = %q, want %q (all: %v)", i, got[i], want[i], got)
		}
	}

	for _, bad := range got {
		if bad == "dist/bundle.js" || bad == ".git/HEAD" {
			t.Errorf("artifact %q should have been excluded", bad)
		}
		if strings.HasPrefix(bad, "node_modules") {
			t.Errorf("node_modules entry %q should have been excluded", bad)
		}
	}
}

func TestBuildRequiresManifestAtRoot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src/block.manifest.json", `{"blockId":"x"}`) // nested, not root
	if _, err := Build(dir); err == nil {
		t.Fatal("expected error when manifest is not at the root")
	}
}

func TestBuildRejectsOversizeFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	big := bytes.Repeat([]byte("a"), MaxFileSizeBytes+1)
	if err := os.WriteFile(filepath.Join(dir, "big.bin"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(dir); err == nil {
		t.Fatal("expected error for oversize file")
	}
}

func TestBuildDeterministicOrder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "z.txt", "z")
	writeFile(t, dir, "a.txt", "a")
	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := []string{"a.txt", "block.manifest.json", "z.txt"}
	if len(res.Files) != len(want) {
		t.Fatalf("files = %v, want %v", res.Files, want)
	}
	for i := range want {
		if res.Files[i] != want[i] {
			t.Errorf("Files[%d] = %q, want %q", i, res.Files[i], want[i])
		}
	}
}

func TestExcludedNames(t *testing.T) {
	for _, n := range []string{
		".git", ".hg", ".svn",
		"node_modules", ".venv", "venv", ".pnpm-store",
		"dist", "build", "out", ".next",
		".vite", ".cache", "coverage", ".pytest_cache", ".mypy_cache", ".ruff_cache", ".turbo",
	} {
		if !IsExcluded(n) {
			t.Errorf("%q should be excluded", n)
		}
	}
	for _, n := range []string{"src", "public", "assets", "lib"} {
		if IsExcluded(n) {
			t.Errorf("%q should NOT be excluded", n)
		}
	}
}

// TestBuildExcludesVenvAndCaches guards the regression that bloated the
// gen-matrix bundle to 888 files: a stray Python .venv plus build/test caches
// leaking into the SOURCE bundle. Only real source must survive.
func TestBuildExcludesVenvAndCaches(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "package.json", "{}")
	writeFile(t, dir, "src/main.tsx", "export default 1")
	// Junk that must all be excluded:
	writeFile(t, dir, ".venv/lib/python3.12/site-packages/foo/__init__.py", "x")
	writeFile(t, dir, "venv/bin/activate", "x")
	writeFile(t, dir, "build/index.js", "x")
	writeFile(t, dir, "out/page.html", "x")
	writeFile(t, dir, ".next/server/app.js", "x")
	writeFile(t, dir, ".vite/deps/chunk.js", "x")
	writeFile(t, dir, "coverage/lcov.info", "x")
	writeFile(t, dir, ".pytest_cache/v/cache/lastfailed", "x")

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := namesInZip(t, res.Zip)
	sort.Strings(got)
	want := []string{"block.manifest.json", "package.json", "src/main.tsx"}
	if len(got) != len(want) {
		t.Fatalf("zip contents = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("zip[%d] = %q, want %q (all: %v)", i, got[i], want[i], got)
		}
	}
}

func namesInZip(t *testing.T, b []byte) []string {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	var out []string
	for _, f := range r.File {
		out = append(out, f.Name)
	}
	return out
}
