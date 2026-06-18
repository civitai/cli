// Package api is the (thin) HTTP client for the Civitai App Blocks surface.
//
// Auth/submit contract (investigated against civitai/civitai @ main, 2026-06):
//
//   - The live bundle-upload route is POST /api/blocks/submit-version. It is
//     wrapped in ModEndpoint = SESSION-COOKIE auth + isModerator + appBlocks
//     flag. It does NOT accept an API key / bearer token today, so a clean
//     programmatic "civitai app submit" is blocked on a server change.
//   - The git-push flow (blocks.getMyAppRepo, #2587) provisions a scoped
//     Forgejo repo but only AFTER the first version has been ZIP-approved, and
//     is itself session-auth tRPC.
//
// So: this client speaks the submit-version payload shape and sends the token
// as a Bearer header against a configurable path (CIVITAI_SUBMIT_PATH), which
// is exactly what the companion server endpoint needs to accept. Until that
// endpoint exists, `civitai app submit` falls back to writing the canonical
// ZIP + printing next steps (see internal/cmd/app_submit.go). Keeping the
// network call behind the Submitter interface makes the whole thing testable
// without a live server.
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

// Client is the default HTTP implementation.
type Client struct {
	BaseURL    string
	Token      string
	SubmitPath string // route for submit-version; companion endpoint, configurable
	HTTP       *http.Client
}

// New builds a Client with sane defaults.
func New(baseURL, token, submitPath string) *Client {
	if submitPath == "" {
		submitPath = "/api/blocks/submit-version"
	}
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Token:      token,
		SubmitPath: submitPath,
		HTTP:       &http.Client{Timeout: 120 * time.Second},
	}
}

// submitBody mirrors submitVersionSchema: a base64-encoded ZIP.
type submitBody struct {
	BundleBase64 string `json:"bundleBase64"`
}

// SubmitVersion uploads the bundle. The server route this targets must accept
// a Bearer token (the companion change); see package doc.
func (c *Client) SubmitVersion(ctx context.Context, zipBytes []byte) (*SubmitResult, error) {
	body, err := json.Marshal(submitBody{
		BundleBase64: base64.StdEncoding.EncodeToString(zipBytes),
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+c.SubmitPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode != http.StatusOK {
		return nil, serverError(resp.StatusCode, raw)
	}
	var out SubmitResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("unexpected response (status %d): %s", resp.StatusCode, string(raw))
	}
	return &out, nil
}

// WhoAmI verifies the token. The public Civitai REST surface exposes
// authenticated user details; we hit a configurable path and tolerate either
// {username,id} directly or wrapped. Minimal by design.
func (c *Client) WhoAmI(ctx context.Context) (*Identity, error) {
	if c.Token == "" {
		return nil, fmt.Errorf("no token configured — run `civitai login` first")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/v1/me", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, serverError(resp.StatusCode, raw)
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
