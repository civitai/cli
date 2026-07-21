// Package auth bridges the persisted config and the civitai.TokenSource contract:
// it yields a Bearer token for the api client, refreshing device-flow OAuth
// tokens (and persisting the rotated refresh token) when they expire or after a
// 401. A personal API key is served as-is and never refreshes.
package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/pkg/civitai"
)

// expirySkew refreshes the access token slightly before its real expiry so an
// in-flight request doesn't race the boundary.
const expirySkew = 30 * time.Second

// refresher is the subset of civitai.OAuthClient the source needs (injectable in
// tests).
type refresher interface {
	Refresh(ctx context.Context, refreshToken string) (*civitai.TokenResponse, error)
}

// Source is an civitai.TokenSource backed by the on-disk config. For a personal-key
// config it returns the key and never refreshes; for an OAuth config it
// refreshes via the refresher and persists rotated tokens.
type Source struct {
	cfg *config.Config
	oc  refresher
	mu  sync.Mutex
}

// New builds a Source for cfg. The refresher defaults to an civitai.OAuthClient
// against the config's base URL.
func New(cfg *config.Config) *Source {
	return &Source{cfg: cfg, oc: civitai.NewOAuthClient(cfg.BaseURL())}
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
// config returns civitai.ErrNoRefresh.
func (s *Source) Refresh(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cfg.AuthKind() != config.AuthKindOAuth {
		return "", civitai.ErrNoRefresh
	}
	return s.refreshLocked(ctx)
}

// refreshLocked performs the refresh grant and persists the result. Caller
// holds s.mu.
//
// FOLLOW-UP (M3, not fixed here): this refresh is not coordinated across
// concurrent CLI processes. The server rotates (and revokes the old) refresh
// token on each refresh, so two processes refreshing at once can race: the
// second sees its now-revoked token rejected and forces the user to re-login.
// The fix is a cross-process file lock around read-refresh-persist; left as a
// documented follow-up since the common single-process case is unaffected.
func (s *Source) refreshLocked(ctx context.Context) (string, error) {
	rt := s.cfg.RefreshToken()
	if rt == "" {
		return "", civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf("no refresh token stored — run `civitai login` again"))
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
	scope := tr.Scope.String()
	if scope == "" {
		scope = s.cfg.Scope()
	}
	// Floor-guard expires_in: a non-positive value would set the expiry to ~now
	// and trigger a refresh on every subsequent call (or, with skew, never serve
	// the token). The server always sends a positive lifetime, so treat a
	// non-positive one as a malformed response rather than silently persisting a
	// dead token.
	if tr.ExpiresIn <= 0 {
		return "", fmt.Errorf("refresh response had a non-positive expires_in (%d)", tr.ExpiresIn)
	}
	expiry := time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	if err := s.cfg.SetOAuthTokens(tr.AccessToken, newRefresh, expiry, scope); err != nil {
		return "", fmt.Errorf("persist refreshed tokens: %w", err)
	}
	return tr.AccessToken, nil
}
