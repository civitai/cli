// Package api is the (thin) HTTP client for the Civitai App Blocks surface.
//
// Auth/submit contract (civitai/civitai PR #2644):
//
//   - The token-authenticated bundle-upload route is
//     POST /api/v1/blocks/submit-version (Bearer access token). This is the
//     default the CLI targets; CIVITAI_SUBMIT_PATH overrides it.
//   - The legacy /api/blocks/submit-version route is session-cookie + moderator
//     authenticated and not used by the CLI.
//
// Auth is supplied via a TokenSource so OAuth (device-flow) access tokens are
// refreshed transparently before a request and once on a 401; a personal API
// key is a static source with no refresh. Keeping the network call behind the
// Submitter interface makes the whole thing testable without a live server.
package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TokenSource yields the Bearer token to send. Refresh, if supported, refreshes
// the token (e.g. after a 401) and returns the new one; a static personal-key
// source returns ErrNoRefresh.
type TokenSource interface {
	// Token returns the current bearer token, refreshing first if it is known
	// to be expired. An empty token means "unauthenticated".
	Token(ctx context.Context) (string, error)
	// Refresh forces a refresh (used on a 401) and returns the new token.
	// Sources that cannot refresh return ("", ErrNoRefresh).
	Refresh(ctx context.Context) (string, error)
}

// ErrNoRefresh is returned by a non-refreshable (personal-key) TokenSource.
var ErrNoRefresh = fmt.Errorf("token source cannot refresh")

// StaticToken is a TokenSource for a fixed token (personal API key). It never
// refreshes.
type StaticToken string

// Token returns the static token.
func (s StaticToken) Token(context.Context) (string, error) { return string(s), nil }

// Refresh always fails: a personal key has no refresh path.
func (s StaticToken) Refresh(context.Context) (string, error) { return "", ErrNoRefresh }

// Submitter submits a packaged bundle and returns the server's response.
type Submitter interface {
	SubmitVersion(ctx context.Context, zipBytes []byte) (*SubmitResult, error)
}

// Verifier verifies a token and returns the authenticated identity.
type Verifier interface {
	WhoAmI(ctx context.Context) (*Identity, error)
}

// Identity is the minimal authenticated-user view `whoami` reports.
type Identity struct {
	Username string `json:"username"`
	ID       int    `json:"id"`
}

// SubmitResult is the publish-request result the server returns.
type SubmitResult struct {
	PublishRequestID string `json:"publishRequestId"`
	Slug             string `json:"slug"`
	Version          string `json:"version"`
	Status           string `json:"status"`
}

// DefaultSubmitPath is the token-authenticated submit-version route.
const DefaultSubmitPath = "/api/v1/blocks/submit-version"

// Client is the default HTTP implementation.
type Client struct {
	BaseURL    string
	Tokens     TokenSource
	SubmitPath string // route for submit-version; CIVITAI_SUBMIT_PATH overrides
	HTTP       *http.Client
}

// New builds a Client with sane defaults from a static token (personal API key
// or a one-shot access token). For refreshable OAuth credentials use
// NewWithSource.
func New(baseURL, token, submitPath string) *Client {
	return NewWithSource(baseURL, StaticToken(token), submitPath)
}

// NewWithSource builds a Client backed by a TokenSource (which may refresh).
func NewWithSource(baseURL string, src TokenSource, submitPath string) *Client {
	if submitPath == "" {
		submitPath = DefaultSubmitPath
	}
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Tokens:     src,
		SubmitPath: submitPath,
		HTTP:       &http.Client{Timeout: 120 * time.Second},
	}
}

// authedDo runs build() (which builds a fresh *http.Request each call, since a
// retried request needs a fresh body) with a Bearer token from the source,
// refreshing once on a 401 and retrying. The returned response body is fully
// read into raw and the response is closed.
func (c *Client) authedDo(ctx context.Context, build func() (*http.Request, error)) (int, []byte, error) {
	token, err := c.Tokens.Token(ctx)
	if err != nil {
		return 0, nil, err
	}
	status, raw, err := c.doOnce(ctx, build, token)
	if err != nil {
		return 0, nil, err
	}
	if status == http.StatusUnauthorized {
		// Try a single refresh + retry. A non-refreshable source returns
		// ErrNoRefresh and we keep the original 401.
		newTok, rerr := c.Tokens.Refresh(ctx)
		if rerr == nil && newTok != "" {
			return c.doOnce(ctx, build, newTok)
		}
	}
	return status, raw, nil
}

func (c *Client) doOnce(ctx context.Context, build func() (*http.Request, error), token string) (int, []byte, error) {
	req, err := build()
	if err != nil {
		return 0, nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, raw, nil
}

// submitBody mirrors submitVersionSchema: a base64-encoded ZIP.
type submitBody struct {
	BundleBase64 string `json:"bundleBase64"`
}

// SubmitVersion uploads the bundle to the token-authenticated submit route,
// refreshing the OAuth access token transparently if needed.
func (c *Client) SubmitVersion(ctx context.Context, zipBytes []byte) (*SubmitResult, error) {
	body, err := json.Marshal(submitBody{
		BundleBase64: base64.StdEncoding.EncodeToString(zipBytes),
	})
	if err != nil {
		return nil, err
	}
	build := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+c.SubmitPath, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, serverError(status, raw)
	}
	var out SubmitResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("unexpected response (status %d): %s", status, string(raw))
	}
	return &out, nil
}

// WhoAmI verifies the token against /api/v1/me, refreshing the OAuth access
// token transparently if needed.
func (c *Client) WhoAmI(ctx context.Context) (*Identity, error) {
	tok, err := c.Tokens.Token(ctx)
	if err != nil {
		return nil, err
	}
	if tok == "" {
		return nil, fmt.Errorf("no token configured — run `civitai login` first")
	}
	build := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/v1/me", nil)
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, serverError(status, raw)
	}
	var id Identity
	if err := json.Unmarshal(raw, &id); err != nil {
		return nil, fmt.Errorf("unexpected /api/v1/me response: %s", string(raw))
	}
	return &id, nil
}

// serverError turns a non-2xx response into a clear, actionable error.
func serverError(status int, raw []byte) error {
	msg := strings.TrimSpace(string(raw))
	// The server returns {"message": "..."} for these routes.
	var wrapped struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(raw, &wrapped) == nil {
		if wrapped.Message != "" {
			msg = wrapped.Message
		} else if wrapped.Error != "" {
			msg = wrapped.Error
		}
	}
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("unauthorized (401): %s — check your token with `civitai login`", msg)
	case http.StatusForbidden:
		return fmt.Errorf("forbidden (403): %s — your account may lack App Blocks access", msg)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("service unavailable (503): %s — App Blocks may not be enabled", msg)
	default:
		return fmt.Errorf("server returned %d: %s", status, msg)
	}
}
