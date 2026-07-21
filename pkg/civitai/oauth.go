package civitai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// stderr is where low-noise security warnings go (e.g. a discovery doc that
// advertised an off-origin endpoint). It is a package var so tests can capture
// it. It MUST never receive tokens or codes — only the fact that a fallback
// occurred.
var stderr io.Writer = os.Stderr

// OAuth device-authorization-grant client for the civitai-cli public client.
//
// Contract: the OAuth provider lives on a dedicated auth origin (production:
// auth.civitai.com) discovered via OpenID well-known metadata. civitai.com
// itself does NOT serve the OAuth endpoints or the discovery document — it 404s
// to the SPA. The endpoints (resolved from the discovery doc) are:
//   - device init:   POST {issuer}/api/auth/oauth/device       (device_authorization_endpoint)
//   - device poll:   POST {issuer}/api/auth/oauth/device-token (issuer + pathDeviceToken; not in the doc)
//   - token refresh: POST {issuer}/api/auth/oauth/token        (token_endpoint)
//
// ALL THREE are application/x-www-form-urlencoded AND must carry an
// Origin: {issuer} header — the auth host enforces a host-wide same-origin
// guard on form POSTs (origin-less/cross-site form POST -> 403; a JSON body ->
// 400 "Missing client_id"). See resolveEndpoints / postForm.
//
// civitai-cli is a PUBLIC client (PKCE/device): no client secret.
const (
	// ClientID is the public OAuth client id for the CLI.
	ClientID = "civitai-cli"

	// DeviceScope is UserRead|AppBlocksSubmit|AppBlocksDevTunnel (bit flags), the
	// fixed scope the CLI requests on login. 100663297 == (1<<0)|(1<<25)|(1<<26):
	//   - UserRead           (1<<0  = 1)        — whoami / identity.
	//   - AppBlocksSubmit     (1<<25 = 33554432) — `app submit` AND the dev-token mint
	//     gate (both require this on an OAuth token).
	//   - AppBlocksDevTunnel  (1<<26 = 67108864) — `app dev-tunnel` (start/stop/status).
	//     The dev-tunnel tRPC procs require this bit on an OAuth token; without it a
	//     login token 403s the scope gate and only a Full personal API key works.
	// This is exactly the civitai-cli OauthClient.allowedScopes set (100663297). We
	// deliberately do NOT request AIServicesWrite: the server's device-flow
	// validateScope is all-or-nothing — requesting any scope the client doesn't
	// allow REJECTS the whole login — and we don't want every login token to carry
	// general Buzz-spend authority.
	//
	// 🔴 SERVER DEPENDENCY: AppBlocksDevTunnel (bit 26) + the widened civitai-cli
	// allowedScopes (100663297) must be LIVE on prod auth (migration applied) BEFORE
	// this ships — else the device request exceeds allowedScopes and login 400s
	// (invalid_scope). Do NOT release this ahead of the civitai server change.
	//
	// What a login token can do: MINT an App-Blocks dev token (the mint gate needs
	// AppBlocksSubmit), open an on-site dev tunnel (AppBlocksDevTunnel), and drive
	// the read/estimate harness paths — cost preview / whatif, catalog browsing, app
	// storage. It CANNOT run a real generation: the dev-token mint applies a uniform
	// AIServicesWrite ceiling on the budgeted-spend scope, so a login-minted dev
	// token has ai:write:budgeted STRIPPED (read/estimate only). Real-Buzz dev:live
	// needs a personal API key with full scope (civitai.com/user/account), which
	// carries AIServicesWrite.
	DeviceScope = "100663297"

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

	// endpoints, once resolved, hold the concrete OAuth endpoint URLs + issuer
	// used for the POSTs (and the Origin header). Resolved lazily via OpenID
	// discovery — see resolveEndpoints. nil until first resolution.
	endpoints *oauthEndpoints
}

// oauthEndpoints holds the concrete URLs the device flow POSTs to, plus the
// issuer (used as the same-origin Origin header value). Populated by OpenID
// discovery (resolveEndpoints).
type oauthEndpoints struct {
	Issuer      string // e.g. https://auth.civitai.com
	DeviceInit  string // device_authorization_endpoint
	DeviceToken string // issuer + pathDeviceToken (not in the discovery doc)
	Token       string // token_endpoint
}

// openIDConfig is the subset of the OpenID Provider Metadata we consume.
type openIDConfig struct {
	Issuer                      string `json:"issuer"`
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
	TokenEndpoint               string `json:"token_endpoint"`
}

// NewOAuthClient builds an OAuthClient with sane defaults.
func NewOAuthClient(baseURL string) *OAuthClient {
	return &OAuthClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// discoveryURL returns the OpenID well-known URL for a given origin (scheme +
// host[:port]), e.g. https://auth.civitai.com/.well-known/openid-configuration.
func discoveryURL(origin string) string {
	return strings.TrimRight(origin, "/") + "/.well-known/openid-configuration"
}

// isLocalOrExemptHost reports whether host is one we must NOT treat as a real,
// public, multi-label DNS host: an IP literal, a loopback name, or a
// single-label host (localhost, httptest's 127.0.0.1, a bare service name).
// These are exempt from BOTH the "auth." derivation AND the https upgrade so
// local-dev and httptest (which serve http on 127.0.0.1 / localhost) keep
// working. Everything else is a real dotted public host that must use https.
func isLocalOrExemptHost(host string) bool {
	if host == "" {
		return true
	}
	if net.ParseIP(host) != nil { // IPv4/IPv6 literal (covers 127.0.0.1, ::1)
		return true
	}
	if !strings.Contains(host, ".") { // single-label (localhost, bare name)
		return true
	}
	return false
}

// authOrigin maps the configured BaseURL to the origin that serves the OpenID
// discovery document (and the OAuth endpoints).
//
// On production civitai.com the OAuth provider lives on a dedicated subdomain
// (auth.civitai.com); civitai.com itself does NOT serve the discovery document
// (it 404s to the SPA). So for a real, dotted public host we prepend "auth."
// to reach the provider. For hosts that are already an auth.* host, an IP, or a
// single-label host (localhost, httptest's 127.0.0.1) we use BaseURL as-is —
// prepending would break local/test servers and the discovery doc is served
// from the same origin in those cases.
//
// SECURITY (FIX 2): for the derived public auth host we FORCE https. The CLI
// POSTs the device_code and the refresh token (long-lived credentials) to this
// origin; an http:// public BaseURL would otherwise send them in cleartext.
// Loopback / single-label / IP hosts keep their input scheme (http allowed) so
// local-dev + httptest still work.
func authOrigin(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return strings.TrimRight(baseURL, "/")
	}
	host := u.Hostname()
	// Already the auth host, an IP, or a single-label host: use as-is (scheme
	// preserved — http is allowed for local-dev/test).
	if strings.HasPrefix(host, "auth.") || isLocalOrExemptHost(host) {
		return u.Scheme + "://" + u.Host
	}
	// Real dotted public host: derive auth.<host> AND force https so the
	// device_code / refresh-token POSTs are never sent in cleartext.
	newHost := "auth." + host
	if u.Port() != "" {
		newHost += ":" + u.Port()
	}
	return "https://" + newHost
}

// resolveEndpoints lazily resolves the concrete OAuth endpoints via OpenID
// discovery and caches them on the client. It fetches the discovery document
// from the auth origin (authOrigin(BaseURL)); on any failure it falls back to
// the well-known paths on that same origin so login still works if discovery is
// briefly unavailable. The DeviceToken (poll) endpoint is always derived as
// issuer + pathDeviceToken — it is not advertised in the discovery document.
func (c *OAuthClient) resolveEndpoints(ctx context.Context) (*oauthEndpoints, error) {
	if c.endpoints != nil {
		return c.endpoints, nil
	}
	origin := authOrigin(c.BaseURL)

	fallback := func() *oauthEndpoints {
		return &oauthEndpoints{
			Issuer:      origin,
			DeviceInit:  origin + pathDeviceInit,
			DeviceToken: origin + pathDeviceToken,
			Token:       origin + pathToken,
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL(origin), nil)
	if err != nil {
		c.endpoints = fallback()
		return c.endpoints, nil
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		c.endpoints = fallback()
		return c.endpoints, nil
	}
	defer resp.Body.Close()
	raw, err := readResponseBody(resp.Body, maxResponseBody)
	if err != nil || resp.StatusCode != http.StatusOK {
		c.endpoints = fallback()
		return c.endpoints, nil
	}
	var oc openIDConfig
	if err := json.Unmarshal(raw, &oc); err != nil || oc.Issuer == "" ||
		oc.DeviceAuthorizationEndpoint == "" || oc.TokenEndpoint == "" {
		c.endpoints = fallback()
		return c.endpoints, nil
	}
	// SECURITY (FIX 1): the discovery document is fetched over the network and
	// could be tampered with. We POST the device_code AND the refresh token to
	// device_authorization_endpoint / token_endpoint, so a doc pointing either at
	// a FOREIGN host (or downgrading to http) could exfiltrate those credentials.
	// Require every discovered endpoint to live on the SAME host as the discovery
	// origin AND to be https (exempting loopback/single-label/IP hosts so local
	// dev + httptest keep working). On any mismatch, fall back to the well-known
	// paths on the trusted auth origin rather than trusting the doc.
	if !sameOriginEndpoint(origin, oc.DeviceAuthorizationEndpoint) ||
		!sameOriginEndpoint(origin, oc.TokenEndpoint) {
		fmt.Fprintln(stderr, "warning: discovery document advertised an off-origin or non-https OAuth endpoint; falling back to default endpoints")
		c.endpoints = fallback()
		return c.endpoints, nil
	}
	issuer := strings.TrimRight(oc.Issuer, "/")
	c.endpoints = &oauthEndpoints{
		Issuer:     issuer,
		DeviceInit: oc.DeviceAuthorizationEndpoint,
		// The poll (device-token) endpoint is NOT advertised in the OpenID doc;
		// it is the issuer + the known custom path.
		DeviceToken: issuer + pathDeviceToken,
		Token:       oc.TokenEndpoint,
	}
	return c.endpoints, nil
}

// sameOriginEndpoint reports whether a discovered endpoint URL is safe to POST
// credentials to: it must parse, share the discovery origin's host (so the
// refresh-token / device_code POST can't be redirected cross-host), and use
// https. As in authOrigin, loopback / single-label / IP origins are exempt from
// the https requirement (local-dev + httptest serve http on 127.0.0.1 /
// localhost) — but the host must still match the discovery origin's host.
func sameOriginEndpoint(origin, endpoint string) bool {
	ou, err := url.Parse(origin)
	if err != nil {
		return false
	}
	eu, err := url.Parse(endpoint)
	if err != nil {
		return false
	}
	if eu.Hostname() == "" || !strings.EqualFold(eu.Hostname(), ou.Hostname()) {
		return false
	}
	// Public (real dotted) hosts MUST be https; loopback/single-label/IP may
	// stay http for local-dev/test.
	if !isLocalOrExemptHost(eu.Hostname()) && eu.Scheme != "https" {
		return false
	}
	return true
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
//
// The device endpoint (auth.civitai.com) requires application/x-www-form-urlencoded
// AND a same-origin Origin header — a JSON body 400s ("Missing client_id") and a
// form POST without a matching Origin 403s ("Cross-site POST form submissions are
// forbidden"). The endpoint URL + Origin come from OpenID discovery.
func (c *OAuthClient) StartDevice(ctx context.Context) (*DeviceAuth, error) {
	ep, err := c.resolveEndpoints(ctx)
	if err != nil {
		return nil, err
	}
	form := url.Values{
		"client_id": {ClientID},
		"scope":     {DeviceScope},
	}
	resp, raw, err := c.postForm(ctx, ep.DeviceInit, form, ep.Issuer)
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

// pollOnce performs a single device-token POST. Like the device-init POST it
// must be application/x-www-form-urlencoded with a same-origin Origin header
// (the host-wide guard applies to every OAuth form POST).
func (c *OAuthClient) pollOnce(ctx context.Context, deviceCode string) (pollOutcome, *TokenResponse, error) {
	ep, err := c.resolveEndpoints(ctx)
	if err != nil {
		return pollPending, nil, err
	}
	form := url.Values{
		"grant_type":  {grantTypeDeviceCode},
		"device_code": {deviceCode},
		"client_id":   {ClientID},
	}
	status, raw, err := c.postForm(ctx, ep.DeviceToken, form, ep.Issuer)
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
	// must be application/x-www-form-urlencoded"). The auth host (auth.civitai.com)
	// ALSO enforces a host-wide same-origin guard on form POSTs, so the refresh
	// must carry an Origin header matching the issuer or it 403s. Endpoint URL +
	// Origin come from OpenID discovery.
	ep, err := c.resolveEndpoints(ctx)
	if err != nil {
		return nil, err
	}
	form := url.Values{
		"grant_type":    {grantTypeRefreshToken},
		"refresh_token": {refreshToken},
		"client_id":     {ClientID},
	}
	status, raw, err := c.postForm(ctx, ep.Token, form, ep.Issuer)
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

// postForm sends an application/x-www-form-urlencoded POST to an absolute URL,
// setting the Origin header to satisfy the auth host's same-origin guard. ALL
// OAuth POSTs (device-init, device-token poll, refresh) go through this: the
// @node-oauth/oauth2-server token handler requires form encoding (rejects JSON)
// and the auth host (auth.civitai.com) rejects any cross-site / origin-less form
// POST with 403 "Cross-site POST form submissions are forbidden". origin should
// be the issuer (the host being POSTed to).
func (c *OAuthClient) postForm(ctx context.Context, rawURL string, form url.Values, origin string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := readResponseBody(resp.Body, maxResponseBody)
	if err != nil {
		return resp.StatusCode, raw, err
	}
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
