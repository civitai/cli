package config

import (
	"os"
	"testing"
	"time"
)

func TestSetOAuthTokensPersists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	os.Unsetenv("CIVITAI_TOKEN")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	exp := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	if err := cfg.SetOAuthTokens("at-1", "rt-1", exp, "33554433"); err != nil {
		t.Fatalf("SetOAuthTokens: %v", err)
	}

	cfg2, err := Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cfg2.AuthKind() != AuthKindOAuth {
		t.Errorf("AuthKind = %q, want oauth", cfg2.AuthKind())
	}
	if cfg2.AccessToken() != "at-1" || cfg2.RefreshToken() != "rt-1" {
		t.Errorf("tokens = %q/%q", cfg2.AccessToken(), cfg2.RefreshToken())
	}
	if cfg2.Scope() != "33554433" {
		t.Errorf("scope = %q", cfg2.Scope())
	}
	if !cfg2.TokenExpiry().Equal(exp) {
		t.Errorf("expiry = %v, want %v", cfg2.TokenExpiry(), exp)
	}
	// Token() should serve the access token for OAuth configs.
	if cfg2.Token() != "at-1" {
		t.Errorf("Token() = %q, want the access token", cfg2.Token())
	}
	// File must be owner-only.
	info, err := os.Stat(cfg2.Path())
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("config perms = %o, want 600", perm)
	}
}

func TestSetTokenClearsOAuthState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	os.Unsetenv("CIVITAI_TOKEN")

	cfg, _ := Load()
	if err := cfg.SetOAuthTokens("at", "rt", time.Now().Add(time.Hour), "s"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetToken("personal-key"); err != nil {
		t.Fatal(err)
	}
	cfg2, _ := Load()
	if cfg2.AuthKind() != AuthKindToken {
		t.Errorf("AuthKind = %q, want token", cfg2.AuthKind())
	}
	if cfg2.RefreshToken() != "" || cfg2.AccessToken() != "" {
		t.Errorf("OAuth state should be cleared: at=%q rt=%q", cfg2.AccessToken(), cfg2.RefreshToken())
	}
	if cfg2.Token() != "personal-key" {
		t.Errorf("Token() = %q", cfg2.Token())
	}
}

func TestEnvTokenIsAlwaysPersonalKind(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "env-key")

	cfg, _ := Load()
	if cfg.AuthKind() != AuthKindToken {
		t.Errorf("env CIVITAI_TOKEN should be a personal key, got %q", cfg.AuthKind())
	}
	if cfg.Token() != "env-key" {
		t.Errorf("Token() = %q", cfg.Token())
	}
}
