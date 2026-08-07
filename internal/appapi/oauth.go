package appapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
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
	// scope `civitai login` requests when no --scopes set is named — i.e. the
	// DEFAULT login. 100663297 == (1<<0)|(1<<25)|(1<<26):
	//   - UserRead           (1<<0  = 1)        — whoami / identity.
	//   - AppBlocksSubmit     (1<<25 = 33554432) — `app submit` AND the dev-token mint
	//     gate (both require this on an OAuth token).
	//   - AppBlocksDevTunnel  (1<<26 = 67108864) — `app dev-tunnel` (start/stop/status).
	//     The dev-tunnel tRPC procs require this bit on an OAuth token; without it a
	//     login token 403s the scope gate and only a Full personal API key works.
	//
	// The DEFAULT deliberately omits AIServicesWrite. That is a product decision,
	// not a protocol limit: a plain `civitai login` must not silently hand every
	// stored credential general Buzz-SPEND authority. A user who wants generation
	// asks for it explicitly with `civitai login --scopes generate`, which ORs in
	// the ScopeSetGenerate bits (see deviceScopeSets) — the request is computed by
	// ResolveDeviceScope, not fixed.
	//
	// 🔴 SERVER DEPENDENCY, and it is all-or-nothing: the device-flow validateScope
	// REJECTS THE WHOLE LOGIN (400 invalid_scope) if the requested mask carries any
	// bit outside the civitai-cli OauthClient's allowedScopes. So every value this
	// package can produce must be a subset of the LIVE allowedScopes column:
	//   - DeviceScope (100663297) needs the AppBlocksDevTunnel widening — live.
	//   - DeviceScope|ScopeSetGenerate (100777985) needs the AIServicesRead |
	//     AIServicesWrite | BuzzRead widening — LIVE in production since
	//     civitai/civitai#3699 merged. Probed against auth.civitai.com on
	//     2026-08-06: scope=100777985 -> 200 with a device code, scope=100663297
	//     -> 200, and scope=100777987 (ONE bit outside) -> 400 invalid_scope.
	//     civitai-cli's allowedScopes is exactly 100777985.
	//
	// 100777985 is therefore a CEILING, not a floor: a new deviceScopeSets entry
	// whose bits fall outside it breaks EVERY login that names the set, so widen
	// allowedScopes server-side FIRST. The default (`civitai login`) must also stay
	// at 100663297 — that is the product decision above, not a server limit.
	//
	// An older release or a self-hosted server that predates the widening still
	// answers invalid_scope for `--scopes generate`; StartDevice maps that to an
	// actionable message (see InvalidScopeError) and plain `civitai login` keeps
	// working there.
	//
	// What a login token can do:
	//   - DEFAULT (`civitai login`, 100663297): identity/whoami, `app submit`,
	//     `app dev-tunnel`, and MINT an App-Blocks dev token. That dev token is
	//     read/estimate-only — the mint clamps the budgeted-spend scope against the
	//     BEARER's AIServicesWrite bit (keyCanSpend), which this mask lacks, so
	//     ai:write:budgeted is STRIPPED and `dev:live` cannot spend real Buzz.
	//   - GENERATE (`civitai login --scopes generate`, 100777985): all of the above
	//     PLUS AIServicesRead|AIServicesWrite|BuzzRead, so it clears the
	//     `civitai generate` scope gate and reads the Buzz balance. Because the
	//     bearer now carries AIServicesWrite, the dev-token mint's clamp no longer
	//     strips ai:write:budgeted — a dev token minted from it CAN arm real-Buzz
	//     `dev:live`. `civitai app dev-token` therefore only REQUESTS that scope
	//     when the user passes --spend (see internal/cmd/app_dev_token.go).
	//   - A full-scope personal API key (civitai.com/user/account) remains the other
	//     way to get AIServicesWrite, and is still the only credential carrying the
	//     rest of the Full mask.
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

// ScopeAppBlocksDevTunnel (1<<26) gates the on-site dev-tunnel tRPC procs. It is
// declared here rather than in the appblocks.go scope table because that table
// drives the `whoami --scopes` decode of a PERSONAL key's mask, whose upstream
// Full constant stops at bit 24; this bit only ever appears on an OAuth token.
const ScopeAppBlocksDevTunnel = 1 << 26

// ScopeSetGenerate is the name a user types to opt a login into generation:
//
//	civitai login --scopes generate
//
// Named sets exist so nobody ever types bit arithmetic on the command line, and
// so a future set can be added without changing the flag's shape.
const ScopeSetGenerate = "generate"

// deviceScopeSet is one named, user-facing bundle of extra bits a login may
// opt into. Bits are ADDITIVE on top of DeviceScope: opting into generation
// must not cost the user app-submit or dev-tunnel ability, because a login
// yields ONE credential.
type deviceScopeSet struct {
	Name string
	Bits int
	// Summary is shown in `login --help` and echoed at login time. It must name
	// the CONSEQUENCE (Buzz-spend authority), not just the bit names — the point
	// of use is the only place a user reliably reads it.
	Summary string
}

// deviceScopeSets is the registry of named --scopes sets, in listing order.
// Adding a set here is the whole extension point: the flag help, the validation
// error, and ResolveDeviceScope all read from it.
var deviceScopeSets = []deviceScopeSet{
	{
		Name: ScopeSetGenerate,
		Bits: ScopeAIServicesRead | ScopeAIServicesWrite | ScopeBuzzRead,
		Summary: "run `civitai generate` and read your Buzz balance " +
			"(AIServicesRead|AIServicesWrite|BuzzRead) — this login WILL be able to SPEND your Buzz",
	},
}

// deviceScopeBase is DeviceScope as an int. Pinned against the string constant
// by TestDeviceScopeStringMatchesBits so the two can never drift.
const deviceScopeBase = ScopeUserRead | ScopeAppBlocksSubmit | ScopeAppBlocksDevTunnel

// DeviceScopeSetNames returns the valid --scopes set names in listing order.
func DeviceScopeSetNames() []string {
	names := make([]string, 0, len(deviceScopeSets))
	for _, s := range deviceScopeSets {
		names = append(names, s.Name)
	}
	return names
}

// DeviceScopeSetSummary returns the human summary for a named set, and whether
// the name is known.
func DeviceScopeSetSummary(name string) (string, bool) {
	for _, s := range deviceScopeSets {
		if s.Name == normalizeScopeSetName(name) {
			return s.Summary, true
		}
	}
	return "", false
}

// normalizeScopeSetName canonicalizes a user-typed set name (trim + lowercase)
// so `--scopes Generate` and `--scopes " generate "` behave like `generate`.
func normalizeScopeSetName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// ResolveDeviceScope computes the `scope` value the device-authorization request
// must carry, given zero or more named scope sets from `login --scopes`.
//
// It is ADDITIVE and starts from DeviceScope: passing no sets (or only
// empty/whitespace entries) returns DeviceScope unchanged, so the default login
// is bit-for-bit what it has always been. Each recognized set ORs its bits in.
// An unrecognized name is a hard error naming the valid sets — a typo'd
// `--scopes generte` must not silently log the user in with fewer scopes than
// they asked for.
func ResolveDeviceScope(sets []string) (string, error) {
	bits := deviceScopeBase
	for _, raw := range sets {
		name := normalizeScopeSetName(raw)
		if name == "" {
			continue // a stray empty element (e.g. `--scopes ""`) means "no set"
		}
		found := false
		for _, s := range deviceScopeSets {
			if s.Name == name {
				bits |= s.Bits
				found = true
				break
			}
		}
		if !found {
			// A FLAG-SHAPED name is not a typo, it is a swallowed flag: --scopes
			// takes a value and has no NoOptDefVal, so `login --scopes --no-browser`
			// hands "--no-browser" here as the set name. Reporting that as an
			// unknown SET sends the user hunting for a name they never typed, so
			// name the real mistake instead. (It already fails SAFE — the login is
			// refused, nothing is stored — this is purely the message.)
			if trimmed := strings.TrimSpace(raw); strings.HasPrefix(trimmed, "-") {
				return "", fmt.Errorf(
					"--scopes consumed %q as its VALUE, not as a flag — it always takes the next argument. "+
						"Write the set name immediately after it (e.g. `--scopes %s %s`); "+
						"valid scope sets: %s",
					trimmed, ScopeSetGenerate, trimmed, strings.Join(DeviceScopeSetNames(), ", "))
			}
			return "", fmt.Errorf("unknown login scope set %q — valid scope sets: %s",
				strings.TrimSpace(raw), strings.Join(DeviceScopeSetNames(), ", "))
		}
	}
	return strconv.Itoa(bits), nil
}

// OAuthClient talks the device-flow + refresh endpoints.
type OAuthClient struct {
	BaseURL string
	HTTP    *http.Client

	// Scope is the exact `scope` value StartDevice puts on the wire. Empty means
	// "the default login scope" (DeviceScope) — callers that don't care about
	// scope sets leave it zero and get today's behaviour. Callers honouring
	// `login --scopes` set it from ResolveDeviceScope.
	Scope string

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

// RequestedScope returns the scope value the device request will carry: the
// caller-supplied Scope, or DeviceScope when it is unset/blank.
func (c *OAuthClient) RequestedScope() string {
	if s := strings.TrimSpace(c.Scope); s != "" {
		return s
	}
	return DeviceScope
}

// InvalidScopeError is the device-init `invalid_scope` rejection, rendered as
// something a user can act on. The server's scope validation is ALL-OR-NOTHING:
// one bit outside the civitai-cli client's allowedScopes rejects the entire
// login.
//
// civitai.com production permits 100777985 (see DeviceScope), so on production
// this error means a set was added client-side ahead of an allowedScopes
// widening. Against ANY OTHER auth origin — a self-hosted deployment, an older
// release, a non-default CIVITAI_BASE_URL — it means that server predates the
// widening. Either way the message names the concrete fallback (plain `civitai
// login`) rather than echoing an OAuth code.
type InvalidScopeError struct {
	// Requested is the scope mask the CLI put on the wire.
	Requested string
	// Description is the server's error_description, if any.
	Description string
}

func (e *InvalidScopeError) Error() string {
	var b strings.Builder
	b.WriteString("the Civitai auth server rejected this login's scopes (invalid_scope)")
	if e.Description != "" {
		b.WriteString(": " + e.Description)
	}
	if e.Requested != "" && e.Requested != DeviceScope {
		// A widened request — the actionable case. Name the fallback that works
		// TODAY and why the wide one may not.
		b.WriteString(fmt.Sprintf(
			".\nThis login asked for extra scopes (requested %s; the default login requests %s), "+
				"and this server does not permit them for the %s client.\n"+
				"civitai.com does permit them, so check CIVITAI_BASE_URL — a self-hosted or older "+
				"auth server predates that widening.\n"+
				"Run `civitai login` (no --scopes) — it still works — and use `civitai login --scopes %s` "+
				"against civitai.com when you need Buzz-spend.",
			e.Requested, DeviceScope, ClientID, ScopeSetGenerate))
	} else {
		b.WriteString(fmt.Sprintf(
			".\nThis server does not permit the default login scope (%s) for the %s client — "+
				"your CLI is likely newer than the auth server it is pointed at (check CIVITAI_BASE_URL). "+
				"Use a personal API key instead: `civitai login --token <key>` "+
				"(create one at https://civitai.com/user/account).",
			e.Requested, ClientID))
	}
	return b.String()
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
	scope := c.RequestedScope()
	form := url.Values{
		"client_id": {ClientID},
		"scope":     {scope},
	}
	resp, raw, err := c.postForm(ctx, ep.DeviceInit, form, ep.Issuer)
	if err != nil {
		return nil, err
	}
	if resp != http.StatusOK {
		// invalid_scope is the ONE device-init failure a user can act on, and the
		// raw OAuth code says nothing about what to do. It means the requested mask
		// carried a bit outside the civitai-cli client's allowedScopes — which is
		// exactly what happens when the CLI is newer than the server's scope
		// widening. Map it to a message that names the fallback.
		var oe oauthErr
		_ = json.Unmarshal(raw, &oe)
		if oe.Error == "invalid_scope" {
			return nil, &InvalidScopeError{Requested: scope, Description: oe.ErrorDescription}
		}
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
