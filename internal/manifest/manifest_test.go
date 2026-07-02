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

func TestLoadScopes(t *testing.T) {
	// Present scopes are returned.
	dir := t.TempDir()
	write(t, dir, `{"blockId":"x","scopes":["ai:write:budgeted","identity:read"]}`)
	got := LoadScopes(dir)
	if len(got) != 2 || got[0] != "ai:write:budgeted" || got[1] != "identity:read" {
		t.Errorf("LoadScopes = %v", got)
	}

	// No scopes field → nil.
	dir2 := t.TempDir()
	write(t, dir2, `{"blockId":"x"}`)
	if got := LoadScopes(dir2); got != nil {
		t.Errorf("LoadScopes (no scopes) = %v, want nil", got)
	}

	// Missing manifest → nil, no error (graceful degrade).
	if got := LoadScopes(t.TempDir()); got != nil {
		t.Errorf("LoadScopes (missing) = %v, want nil", got)
	}

	// Malformed JSON → nil, no error (graceful degrade).
	dir3 := t.TempDir()
	write(t, dir3, `{ not json `)
	if got := LoadScopes(dir3); got != nil {
		t.Errorf("LoadScopes (malformed) = %v, want nil", got)
	}
}

func TestSetBlockIDPreservesOrderAndFields(t *testing.T) {
	dir := t.TempDir()
	src := `{
  "blockId": "my-block",
  "version": "0.1.0",
  "name": "My Block",
  "scopes": ["identity:read"]
}`
	write(t, dir, src)
	if err := SetBlockID(dir, "my-block-abc12"); err != nil {
		t.Fatalf("SetBlockID: %v", err)
	}
	raw, _ := os.ReadFile(Path(dir))
	out := string(raw)
	if !contains(out, `"blockId": "my-block-abc12"`) {
		t.Errorf("blockId not updated:\n%s", out)
	}
	// Other fields + their order preserved (surgical value replace).
	if !contains(out, `"name": "My Block"`) || !contains(out, `"identity:read"`) {
		t.Errorf("other fields not preserved:\n%s", out)
	}
	if indexOf(out, "blockId") >= indexOf(out, "version") {
		t.Errorf("field order not preserved:\n%s", out)
	}
	m, err := Load(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if m.BlockID != "my-block-abc12" {
		t.Errorf("reloaded blockId = %q", m.BlockID)
	}
	// Mode preserved at 0600.
	info, _ := os.Stat(Path(dir))
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 600", perm)
	}
}

func TestSetBlockIDMissingKeyFallsBack(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `{"version":"0.1.0","name":"X"}`)
	if err := SetBlockID(dir, "new-slug"); err != nil {
		t.Fatalf("SetBlockID (no key): %v", err)
	}
	m, err := Load(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if m.BlockID != "new-slug" {
		t.Errorf("blockId = %q, want new-slug", m.BlockID)
	}
	if m.Name != "X" {
		t.Errorf("other fields lost: name = %q", m.Name)
	}
}

func TestSetBlockIDMissingFileErrors(t *testing.T) {
	if err := SetBlockID(t.TempDir(), "x-slug"); err == nil {
		t.Error("expected an error when no manifest exists")
	}
}

// TestSetBlockIDRefusesInvalidResultDoesNotClobber: a blockId key inside otherwise
// malformed JSON is value-replaced, but the result fails the json.Valid guard, so
// SetBlockID errors WITHOUT writing (the original file is left intact).
func TestSetBlockIDRefusesInvalidResultDoesNotClobber(t *testing.T) {
	dir := t.TempDir()
	broken := `{ "blockId": "old-slug" this is not valid json`
	write(t, dir, broken)
	if err := SetBlockID(dir, "new-slug"); err == nil {
		t.Fatal("expected an error when the rewrite would produce invalid JSON")
	}
	raw, _ := os.ReadFile(Path(dir))
	if string(raw) != broken {
		t.Errorf("file must be untouched on a refused write:\n%s", raw)
	}
}

// TestSetBlockIDInvalidJSONNoKeyErrors: with no blockId key AND invalid JSON, the
// fallback structural rewrite fails to parse and returns an error (no write).
func TestSetBlockIDInvalidJSONNoKeyErrors(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `{ not valid and no key `)
	if err := SetBlockID(dir, "x-slug"); err == nil {
		t.Error("expected an error rewriting invalid JSON with no blockId key")
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
