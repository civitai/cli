package pkgzip

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExcludedNamesAndJoin(t *testing.T) {
	names := ExcludedNames()
	if len(names) < 3 {
		t.Fatalf("ExcludedNames = %v, want at least the core entries", names)
	}
	// Sorted.
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("ExcludedNames not sorted: %v", names)
		}
	}
	// Must include the original core dirs.
	have := map[string]bool{}
	for _, n := range names {
		have[n] = true
	}
	for _, core := range []string{".git", "node_modules", "dist"} {
		if !have[core] {
			t.Errorf("ExcludedNames missing core entry %q: %v", core, names)
		}
	}
	if JoinExcluded() == "" {
		t.Error("JoinExcluded should be non-empty")
	}
}

func TestBuildMissingManifest(t *testing.T) {
	if _, err := Build(t.TempDir()); err == nil {
		t.Fatal("expected error when no manifest present")
	}
}

func TestBuildOnlyManifestStillPackages(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(res.Files) != 1 || res.Files[0] != "block.manifest.json" {
		t.Errorf("files = %v", res.Files)
	}
	if res.DecompressedBy <= 0 {
		t.Errorf("DecompressedBy = %d, want > 0", res.DecompressedBy)
	}
}

func TestBuildRejectsTooManyFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	for i := 0; i < MaxFiles+1; i++ {
		writeFile(t, dir, filepath.Join("many", "f"+itoa(i)+".txt"), "x")
	}
	if _, err := Build(dir); err == nil {
		t.Fatal("expected error when file count exceeds the server cap")
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func TestBuildSkipsSymlinks(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "real.txt", "hi")
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(filepath.Join(dir, "real.txt"), link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, f := range res.Files {
		if f == "link.txt" {
			t.Errorf("symlink should be skipped, got %v", res.Files)
		}
	}
}
