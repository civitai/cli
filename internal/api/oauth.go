package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OAuth device-authorization-grant client for the civitai-cli public client.
//
// Contract (civitai/civitai PR #2644):
//   - device init:  POST {BaseURL}/api/auth/oauth/device
//   - device poll:  POST {BaseURL}/api/auth/oauth/device-token
//   - token refresh: POST {BaseURL}/api/auth/oauth/token
//
// civitai-cli is a PUBLIC client (PKCE/device): no client secret.
const (
	// ClientID is the public OAuth client id for the CLI.
	ClientID = "civitai-cli"

	// DeviceScope is UserRead|AppBlocksSubmit (bit flags), the fixed scope the
	// CLI requests. 33554433 == (1<<0)|(1<<25) in the server's scope bitset.
	DeviceScope = "33554433"

	grantTypeDeviceCode   = "urn:ietf:params:oauth:grant-type:device_code"
	grantTypeRefreshToken = "refresh_token"

	pathDeviceInit  = "/api/auth/oauth/device"
	pathDeviceToken = "/api/auth/oauth/device-token"
	pathToken       = "/api/auth/oauth/token"

	// maxPollInterval caps the RFC 8628 +5s-per-slow_down backoff so a
	// misbehaving (or repeated) slow_down can't stretch the poll unboundedly.
	maxPollInterval = 60 * time.Second
)

// OAuthClient talks the device-flow + refresh endpoints.
type OAuthClient struct {
	BaseURL string
	HTTP    *http.Client
}

// NewOAuthClient builds an OAuthClient with sane defaults.
func NewOAuthClient(baseURL string) *OAuthClient {
	return &OAuthClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// DeviceAuth is the device-init response.
type DeviceAuth struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// Scope is a token scope that tolerates BOTH JSON shapes the server emits:
// the device-token (login) route returns a plain string (`scope.toString()`),
// while the token/refresh route returns the @node-oauth/oauth2-server shape
// where scope is an ARRAY of strings (e.g. ["33554433"]). Declaring scope as a
// plain `string` made json.Unmarshal of the refresh response fail the whole
// struct, killing Refresh() after the 1h access-token TTL. UnmarshalJSON
// normalizes either shape to a single space-joined string (OAuth convention).
// The CLI only stores/displays scope, it never enforces it, so this is safe.
type Scope string

// UnmarshalJSON accepts a JSON string OR a JSON array of strings.
func (s *Scope) UnmarshalJSON(b []byte) error {
	// Null/absent -> empty.
	if len(b) == 0 || string(b) == "null" {
		*s = ""
		return nil
	}
	// Array shape (refresh route): ["33554433"] -> "33554433".
	if b[0] == '[' {
		var arr []string
		if err := json.Unmarshal(b, &arr); err != nil {
			return err
		}
		*s = Scope(strings.Join(arr, " "))
		return nil
	}
	// String shape (device-token route).
	var str string
	if err := json.Unmarshal(b, &str); err != nil {
		return err
	}
	*s = Scope(str)
	return nil
}

// String returns the scope as a plain string for storage/display.
func (s Scope) String() string { return string(s) }

// TokenResponse is the successful device-token / refresh response. RefreshToken
// may be empty on a refresh that doesn't rotate.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        Scope  `json:"scope"`
}

// oauthErr is the OAuth error body ({"error": "..."}).
type oauthErr struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// DeviceFlowError is a terminal OAuth error (expired_token, access_denied, …)
// surfaced to the caller. authorization_pending / slow_down are handled inline
// by PollToken and never returned as this.
type DeviceFlowError struct {
	Code        string
	Description string
}

func (e *DeviceFlowError) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("device login failed: %s (%s)", e.Description, e.Code)
	}
	switch e.Code {
	case "expired_token":
		return "device login expired before it was approved — run `civitai login` again"
	case "access_denied":
		return "device login was denied in the browser"
	default:
		return "device login failed: " + e.Code
	}
}

// StartDevice initiates the device-authorization grant.
func (c *OAuthClient) StartDevice(ctx context.Context) (*DeviceAuth, error) {
	body, _ := json.Marshal(map[string]string{
		"client_id": ClientID,
		"scope":     DeviceScope,
	})
	resp, raw, err := c.post(ctx, pathDeviceInit, body)
	if err != nil {
		return nil, err
	}
	if resp != http.StatusOK {
		return nil, fmt.Errorf("device init failed (status %d): %s", resp, oauthMsg(raw))
	}
	var d DeviceAuth
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("unexpected device-init response: %s", string(raw))
	}
	if d.DeviceCode == "" || d.UserCode == "" {
		return nil, fmt.Errorf("device-init response missing device_code/user_code")
	}
	if d.Interval <= 0 {
		d.Interval = 5
	}
	if d.ExpiresIn <= 0 {
		d.ExpiresIn = 900
	}
	return &d, nil
}

// pollOutcome is the internal result of a single device-token poll.
type pollOutcome int

const (
	pollPending  pollOutcome = iota // authorization_pending — keep polling
	pollSlowDown                    // slow_down — increase the interval, keep polling
	pollDone                        // success
)

// pollOnce performs a single device-token POST.
func (c *OAuthClient) pollOnce(ctx context.Context, deviceCode string) (pollOutcome, *TokenResponse, error) {
	body, _ := json.Marshal(map[string]string{
		"grant_type":  grantTypeDeviceCode,
		"device_code": deviceCode,
		"client_id":   ClientID,
	})
	status, raw, err := c.post(ctx, pathDeviceToken, body)
	if err != nil {
		return pollPending, nil, err
	}
	if status == http.StatusOK {
		var tr TokenResponse
		if err := json.Unmarshal(raw, &tr); err != nil {
			return pollPending, nil, fmt.Errorf("unexpected device-token response: %s", string(raw))
		}
		if tr.AccessToken == "" {
			return pollPending, nil, fmt.Errorf("device-token success response had no access_token")
		}
		return pollDone, &tr, nil
	}
	// Non-200: an OAuth error code drives the polling state machine.
	var oe oauthErr
	_ = json.Unmarshal(raw, &oe)
	switch oe.Error {
	case "authorization_pending":
		return pollPending, nil, nil
	case "slow_down":
		return pollSlowDown, nil, nil
	case "expired_token", "access_denied", "invalid_grant", "invalid_client", "unauthorized_client":
		return pollPending, nil, &DeviceFlowError{Code: oe.Error, Description: oe.ErrorDescription}
	default:
		if oe.Error != "" {
			return pollPending, nil, &DeviceFlowError{Code: oe.Error, Description: oe.ErrorDescription}
		}
		return pollPending, nil, fmt.Errorf("device-token poll failed (status %d): %s", status, oauthMsg(raw))
	}
}

// PollToken polls device-token until approval, a terminal error, or the
// device-flow deadline (auth.ExpiresIn). It blocks for `interval` seconds
// between polls and increases the interval by 5s on slow_down. sleep is
// injectable for tests; pass nil for the real time.Sleep.
func (c *OAuthClient) PollToken(ctx context.Context, auth *DeviceAuth, sleep func(time.Duration)) (*TokenResponse, error) {
	if sleep == nil {
		sleep = time.Sleep
	}
	interval := time.Duration(auth.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(auth.ExpiresIn) * time.Second)

	for {
		if time.Now().After(deadline) {
			return nil, &DeviceFlowError{Code: "expired_token"}
		}
		outcome, tr, err := c.pollOnce(ctx, auth.DeviceCode)
		if err != nil {
			return nil, err
		}
		switch outcome {
		case pollDone:
			return tr, nil
		case pollSlowDown:
			interval += 5 * time.Second
			if interval > maxPollInterval {
				interval = maxPollInterval
			}
		}
		// Don't sleep past the deadline.
		if time.Now().Add(interval).After(deadline) {
			// One more short wait then a final loop check will expire it.
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return nil, &DeviceFlowError{Code: "expired_token"}
			}
			sleep(remaining)
			return nil, &DeviceFlowError{Code: "expired_token"}
		}
		sleep(interval)
	}
}

// Refresh exchanges a refresh token for a new access token. The server may
// rotate the refresh token; callers must persist tr.RefreshToken if non-empty.
func (c *OAuthClient) Refresh(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	// The /token endpoint is the @node-oauth/oauth2-server token handler, which
	// REQUIRES application/x-www-form-urlencoded (it rejects JSON with "content
	// must be application/x-www-form-urlencoded"). Unlike the custom device-flow
	// endpoints (/device, /device-token) which accept JSON via post(), the refresh
	// grant must be form-encoded.
	form := url.Values{
		"grant_type":    {grantTypeRefreshToken},
		"refresh_token": {refreshToken},
		"client_id":     {ClientID},
	}
	status, raw, err := c.postForm(ctx, pathToken, form)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		var oe oauthErr
		_ = json.Unmarshal(raw, &oe)
		if oe.Error != "" {
			return nil, &DeviceFlowError{Code: oe.Error, Description: oe.ErrorDescription}
		}
		return nil, fmt.Errorf("token refresh failed (status %d): %s", status, oauthMsg(raw))
	}
	var tr TokenResponse
	if err := json.Unmarshal(raw, &tr); err != nil {
		return nil, fmt.Errorf("unexpected refresh response: %s", string(raw))
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("refresh response had no access_token")
	}
	return &tr, nil
}

// post sends a JSON POST and returns the status + body.
func (c *OAuthClient) post(ctx context.Context, path string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, raw, nil
}

// postForm sends an application/x-www-form-urlencoded POST. Used for the OAuth
// /token endpoint (the refresh grant), whose @node-oauth/oauth2-server handler
// requires form encoding and rejects JSON.
func (c *OAuthClient) postForm(ctx context.Context, path string, form url.Values) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, raw, nil
}

// oauthMsg extracts a human message from an OAuth error body.
func oauthMsg(raw []byte) string {
	var oe oauthErr
	if json.Unmarshal(raw, &oe) == nil && oe.Error != "" {
		if oe.ErrorDescription != "" {
			return oe.Error + ": " + oe.ErrorDescription
		}
		return oe.Error
	}
	return strings.TrimSpace(string(raw))
}
