package config

import (
	"os"
	"testing"
)

func TestEnvOverridesToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	// Ensure no env override masks the file.
	os.Unsetenv("CIVITAI_TOKEN")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.SetToken("saved-token"); err != nil {
		t.Fatalf("SetToken: %v", err)
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

func TestSetBaseURL(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
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
