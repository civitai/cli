package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, Filename), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPath(t *testing.T) {
	if got := Path("foo"); got != filepath.Join("foo", Filename) {
		t.Errorf("Path = %q", got)
	}
}

func TestLoadReadsFields(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `{
		"blockId": "demo",
		"version": "1.2.3",
		"name": "Demo",
		"buildCommand": "npm run build",
		"outputDir": "dist"
	}`)
	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.BlockID != "demo" || m.Version != "1.2.3" || m.Name != "Demo" {
		t.Errorf("unexpected manifest: %+v", m)
	}
	if m.BuildCommand != "npm run build" || m.OutputDir != "dist" {
		t.Errorf("build fields wrong: %+v", m)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
	if !contains(err.Error(), "civitai app init") {
		t.Errorf("error should hint at init: %v", err)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `{not json`)
	if _, err := Load(dir); err == nil {
		t.Fatal("expected JSON parse error")
	}
}

func TestLoadRaw(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `{"blockId":"x","version":"0.1.0","extra":true}`)
	generic, m, err := LoadRaw(dir)
	if err != nil {
		t.Fatalf("LoadRaw: %v", err)
	}
	if m.BlockID != "x" {
		t.Errorf("struct blockId = %q", m.BlockID)
	}
	gm, ok := generic.(map[string]any)
	if !ok {
		t.Fatalf("generic is %T, want map", generic)
	}
	if gm["extra"] != true {
		t.Errorf("generic should preserve unknown fields: %v", gm)
	}
}

func TestLoadRawMissingAndInvalid(t *testing.T) {
	if _, _, err := LoadRaw(t.TempDir()); err == nil {
		t.Error("expected error for missing manifest")
	}
	dir := t.TempDir()
	write(t, dir, `nope`)
	if _, _, err := LoadRaw(dir); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
