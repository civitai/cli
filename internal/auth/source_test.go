package auth

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/config"
)

// stubRefresher returns a fixed response and records the refresh token it saw.
type stubRefresher struct {
	calls int
	sawRT string
	resp  *api.TokenResponse
	rerr  error
}

func (s *stubRefresher) Refresh(_ context.Context, rt string) (*api.TokenResponse, error) {
	s.calls++
	s.sawRT = rt
	if s.rerr != nil {
		return nil, s.rerr
	}
	return s.resp, nil
}

func loadCfg(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

func TestSourceExpiredAccessTokenTriggersRefresh(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	os.Unsetenv("CIVITAI_TOKEN")

	cfg := loadCfg(t)
	// Already-expired access token + a refresh token.
	if err := cfg.SetOAuthTokens("old-at", "old-rt", time.Now().Add(-time.Minute), "s"); err != nil {
		t.Fatal(err)
	}

	ref := &stubRefresher{resp: &api.TokenResponse{
		AccessToken: "new-at", ExpiresIn: 3600, RefreshToken: "rotated-rt", Scope: "s",
	}}
	src := newWithRefresher(cfg, ref)

	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok != "new-at" {
		t.Errorf("Token() = %q, want new-at", tok)
	}
	if ref.calls != 1 || ref.sawRT != "old-rt" {
		t.Errorf("refresh calls=%d sawRT=%q", ref.calls, ref.sawRT)
	}
	// The rotated refresh token must be persisted.
	reloaded := loadCfg(t)
	if reloaded.RefreshToken() != "rotated-rt" {
		t.Errorf("persisted refresh token = %q, want rotated-rt", reloaded.RefreshToken())
	}
	if reloaded.AccessToken() != "new-at" {
		t.Errorf("persisted access token = %q, want new-at", reloaded.AccessToken())
	}
}

func TestSourceValidTokenSkipsRefresh(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	os.Unsetenv("CIVITAI_TOKEN")

	cfg := loadCfg(t)
	if err := cfg.SetOAuthTokens("good-at", "rt", time.Now().Add(time.Hour), "s"); err != nil {
		t.Fatal(err)
	}
	ref := &stubRefresher{resp: &api.TokenResponse{AccessToken: "nope"}}
	src := newWithRefresher(cfg, ref)

	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok != "good-at" {
		t.Errorf("Token() = %q, want the non-expired good-at", tok)
	}
	if ref.calls != 0 {
		t.Errorf("a valid token should not refresh, got %d calls", ref.calls)
	}
}

func TestSourceRefreshKeepsOldRefreshTokenWhenNotRotated(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	os.Unsetenv("CIVITAI_TOKEN")

	cfg := loadCfg(t)
	if err := cfg.SetOAuthTokens("old-at", "keep-rt", time.Now().Add(-time.Minute), "s"); err != nil {
		t.Fatal(err)
	}
	// Server returns no refresh_token (no rotation).
	ref := &stubRefresher{resp: &api.TokenResponse{AccessToken: "new-at", ExpiresIn: 3600}}
	src := newWithRefresher(cfg, ref)

	if _, err := src.Token(context.Background()); err != nil {
		t.Fatalf("Token: %v", err)
	}
	reloaded := loadCfg(t)
	if reloaded.RefreshToken() != "keep-rt" {
		t.Errorf("refresh token should be retained when not rotated, got %q", reloaded.RefreshToken())
	}
}

func TestSourceRefreshRejectsNonPositiveExpiresIn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	os.Unsetenv("CIVITAI_TOKEN")

	cfg := loadCfg(t)
	if err := cfg.SetOAuthTokens("old-at", "old-rt", time.Now().Add(-time.Minute), "s"); err != nil {
		t.Fatal(err)
	}
	// A refresh that returns expires_in<=0 must be treated as an error, not
	// persisted with an ~now expiry (which would refresh on every call).
	ref := &stubRefresher{resp: &api.TokenResponse{AccessToken: "new-at", ExpiresIn: 0, RefreshToken: "new-rt"}}
	src := newWithRefresher(cfg, ref)

	if _, err := src.Token(context.Background()); err == nil {
		t.Fatal("expected an error for non-positive expires_in")
	}
	// The dead token must NOT have been persisted.
	reloaded := loadCfg(t)
	if reloaded.AccessToken() == "new-at" {
		t.Errorf("dead token should not have been persisted")
	}
}

func TestSourceRefreshPersistsArrayScope(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	os.Unsetenv("CIVITAI_TOKEN")

	cfg := loadCfg(t)
	if err := cfg.SetOAuthTokens("old-at", "old-rt", time.Now().Add(-time.Minute), "s"); err != nil {
		t.Fatal(err)
	}
	// Scope arriving as the array-normalized string (what the refresh route
	// yields via the custom Scope unmarshaler) must round-trip into config.
	ref := &stubRefresher{resp: &api.TokenResponse{
		AccessToken: "new-at", ExpiresIn: 3600, RefreshToken: "new-rt", Scope: "33554433",
	}}
	src := newWithRefresher(cfg, ref)

	if _, err := src.Token(context.Background()); err != nil {
		t.Fatalf("Token: %v", err)
	}
	reloaded := loadCfg(t)
	if reloaded.Scope() != "33554433" {
		t.Errorf("persisted scope = %q, want 33554433", reloaded.Scope())
	}
}

func TestSourcePersonalKeyNeverRefreshes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	os.Unsetenv("CIVITAI_TOKEN")

	cfg := loadCfg(t)
	if err := cfg.SetToken("personal-key"); err != nil {
		t.Fatal(err)
	}
	ref := &stubRefresher{resp: &api.TokenResponse{AccessToken: "should-not-be-used"}}
	src := newWithRefresher(cfg, ref)

	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok != "personal-key" {
		t.Errorf("Token() = %q, want personal-key", tok)
	}
	if _, err := src.Refresh(context.Background()); err != api.ErrNoRefresh {
		t.Errorf("personal-key Refresh err = %v, want ErrNoRefresh", err)
	}
	if ref.calls != 0 {
		t.Errorf("personal key must not call the refresher, got %d", ref.calls)
	}
}
