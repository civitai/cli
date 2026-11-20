package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// useTempConfigDir points EVERY variable os.UserConfigDir reads — XDG_CONFIG_HOME
// on Linux, HOME on macOS, APPDATA on Windows — at a fresh temp dir and returns
// the resolved config dir (<base>/civitai). XDG_CONFIG_HOME alone is honoured on
// Linux only, which is how TestSetTokenPersists overwrote a Mac developer's real
// token (issue #931). TestMain's configtest.Run is the package-wide backstop; this
// makes each test own its directory so fixtures can be placed in it on any OS.
func useTempConfigDir(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	for _, k := range []string{"XDG_CONFIG_HOME", "HOME", "APPDATA", "USERPROFILE"} {
		t.Setenv(k, tmp)
	}
	d, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if !within(realPath(t, d), realPath(t, tmp)) {
		t.Fatalf("PREMISE BROKEN: Dir() = %s is outside the test-owned %s", d, tmp)
	}
	return d
}

func TestEnvOverridesToken(t *testing.T) {
	useTempConfigDir(t)
	t.Setenv("CIVITAI_TOKEN", "from-env")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Token() != "from-env" {
		t.Errorf("Token() = %q, want from-env", cfg.Token())
	}
	if cfg.BaseURL() != DefaultBaseURL {
		t.Errorf("BaseURL() = %q, want default %q", cfg.BaseURL(), DefaultBaseURL)
	}
}

func TestSetTokenPersists(t *testing.T) {
	cfgDir := useTempConfigDir(t)
	// Ensure no env override masks the file.
	os.Unsetenv("CIVITAI_TOKEN")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.SetToken("saved-token"); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	if want := filepath.Join(cfgDir, "config.yaml"); cfg.Path() != want {
		t.Fatalf("config path = %s, want the test-owned %s", cfg.Path(), want)
	}

	// Re-load from disk.
	cfg2, err := Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cfg2.Token() != "saved-token" {
		t.Errorf("reloaded Token() = %q, want saved-token", cfg2.Token())
	}

	// File should be owner-only.
	info, err := os.Stat(cfg.Path())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("config perms = %o, want 600", perm)
	}
}

func TestLoadMalformedConfigErrors(t *testing.T) {
	cfgDir := useTempConfigDir(t)
	os.Unsetenv("CIVITAI_TOKEN")

	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Not valid YAML.
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte("::: not: yaml: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("expected a parse error for a malformed config file")
	}
}

func TestDirHonoursXDG(t *testing.T) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		t.Skip("os.UserConfigDir does not consult XDG_CONFIG_HOME on " + runtime.GOOS)
	}
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if got != filepath.Join(dir, "civitai") {
		t.Errorf("Dir = %q, want %q", got, filepath.Join(dir, "civitai"))
	}
}

func TestSaveErrorOnUnwritableDir(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("permission-based test unreliable as root / on windows")
	}
	cfgParent := useTempConfigDir(t)
	os.Unsetenv("CIVITAI_TOKEN")

	// Make the config parent dir unwritable so MkdirAll/CreateTemp fails. Its
	// parents are created writable first: on macOS the config dir sits two levels
	// down (Library/Application Support/civitai), and MkdirAll applies the mode to
	// every directory it creates.
	if err := os.MkdirAll(filepath.Dir(cfgParent), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(cfgParent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cfgParent, 0o700) })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.SetToken("x"); err == nil {
		t.Error("expected SetToken to fail writing into an unwritable dir")
	}
}

func TestSetBaseURL(t *testing.T) {
	useTempConfigDir(t)
	os.Unsetenv("CIVITAI_BASE_URL")

	cfg, _ := Load()
	if err := cfg.SetBaseURL("https://staging.example.com"); err != nil {
		t.Fatalf("SetBaseURL: %v", err)
	}
	cfg2, _ := Load()
	if cfg2.BaseURL() != "https://staging.example.com" {
		t.Errorf("BaseURL() = %q", cfg2.BaseURL())
	}
}
