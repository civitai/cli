// Package auth bridges the persisted config and the api.TokenSource contract:
// it yields a Bearer token for the api client, refreshing device-flow OAuth
// tokens (and persisting the rotated refresh token) when they expire or after a
// 401. A personal API key is served as-is and never refreshes.
package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/config"
)

// expirySkew refreshes the access token slightly before its real expiry so an
// in-flight request doesn't race the boundary.
const expirySkew = 30 * time.Second

// refresher is the subset of api.OAuthClient the source needs (injectable in
// tests).
type refresher interface {
	Refresh(ctx context.Context, refreshToken string) (*api.TokenResponse, error)
}

// Source is an api.TokenSource backed by the on-disk config. For a personal-key
// config it returns the key and never refreshes; for an OAuth config it
// refreshes via the refresher and persists rotated tokens.
type Source struct {
	cfg *config.Config
	oc  refresher
	mu  sync.Mutex
}

// New builds a Source for cfg. The refresher defaults to an api.OAuthClient
// against the config's base URL.
func New(cfg *config.Config) *Source {
	return &Source{cfg: cfg, oc: api.NewOAuthClient(cfg.BaseURL())}
}

// newWithRefresher is the test seam.
func newWithRefresher(cfg *config.Config, oc refresher) *Source {
	return &Source{cfg: cfg, oc: oc}
}

// Token returns the current bearer token, refreshing an expired OAuth access
// token first.
func (s *Source) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cfg.AuthKind() != config.AuthKindOAuth {
		return s.cfg.Token(), nil
	}
	// OAuth: refresh if the access token is expired (or about to be).
	if exp := s.cfg.TokenExpiry(); !exp.IsZero() && time.Now().Add(expirySkew).After(exp) {
		if _, err := s.refreshLocked(ctx); err != nil {
			return "", err
		}
	}
	return s.cfg.AccessToken(), nil
}

// Refresh forces a refresh (used by the api client on a 401). A personal-key
// config returns api.ErrNoRefresh.
func (s *Source) Refresh(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cfg.AuthKind() != config.AuthKindOAuth {
		return "", api.ErrNoRefresh
	}
	return s.refreshLocked(ctx)
}

// refreshLocked performs the refresh grant and persists the result. Caller
// holds s.mu.
func (s *Source) refreshLocked(ctx context.Context) (string, error) {
	rt := s.cfg.RefreshToken()
	if rt == "" {
		return "", fmt.Errorf("no refresh token stored — run `civitai login` again")
	}
	tr, err := s.oc.Refresh(ctx, rt)
	if err != nil {
		return "", err
	}
	// The server may rotate the refresh token; keep the old one if it didn't.
	newRefresh := tr.RefreshToken
	if newRefresh == "" {
		newRefresh = rt
	}
	scope := tr.Scope
	if scope == "" {
		scope = s.cfg.Scope()
	}
	expiry := time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	if err := s.cfg.SetOAuthTokens(tr.AccessToken, newRefresh, expiry, scope); err != nil {
		return "", fmt.Errorf("persist refreshed tokens: %w", err)
	}
	return tr.AccessToken, nil
}
