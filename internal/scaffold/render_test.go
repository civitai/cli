package scaffold

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenderRefusesFileAsDir(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "afile")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Render(Static, f, Data{Slug: "abc", Name: "Abc"}); err == nil {
		t.Fatal("expected error rendering into a path that is a file")
	}
}

func TestRenderCreatesNestedDir(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "a", "b", "block")
	if _, err := Render(Static, dest, Data{Slug: "block", Name: "Block"}); err != nil {
		t.Fatalf("Render into nested non-existent dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "block.manifest.json")); err != nil {
		t.Errorf("manifest not written: %v", err)
	}
}

func TestOutputNameMapsGitignore(t *testing.T) {
	if got := outputName("gitignore.tmpl"); got != ".gitignore" {
		t.Errorf("outputName(gitignore.tmpl) = %q", got)
	}
	if got := outputName("src/App.jsx.tmpl"); got != filepath.FromSlash("src/App.jsx") {
		t.Errorf("outputName = %q", got)
	}
}
