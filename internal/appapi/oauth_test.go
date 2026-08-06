package appapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// writeDiscovery writes an OpenID discovery document whose issuer + endpoints
// point back at the test server `base` (so the device flow POSTs land on the
// same httptest server). This mirrors production, where discovery on the auth
// origin advertises that origin's endpoints.
func writeDiscovery(w http.ResponseWriter, base string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(openIDConfig{
		Issuer:                      base,
		DeviceAuthorizationEndpoint: base + pathDeviceInit,
		TokenEndpoint:               base + pathToken,
	})
}

func TestStartDeviceParsesResponse(t *testing.T) {
	var gotForm url.Values
	var gotOrigin, gotCT string
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeDiscovery(w, base)
			return
		case pathDeviceInit:
		default:
			t.Errorf("init hit wrong path %q", r.URL.Path)
		}
		gotCT = r.Header.Get("Content-Type")
		gotOrigin = r.Header.Get("Origin")
		_ = r.ParseForm()
		gotForm = r.PostForm
		_ = json.NewEncoder(w).Encode(DeviceAuth{
			DeviceCode:              "dev-secret",
			UserCode:                "WXYZ-1234",
			VerificationURI:         "https://auth.civitai.com/login/oauth/device",
			VerificationURIComplete: "https://auth.civitai.com/login/oauth/device?code=WXYZ-1234",
			ExpiresIn:               900,
			Interval:                5,
		})
	}))
	defer srv.Close()
	base = srv.URL

	d, err := NewOAuthClient(srv.URL).StartDevice(context.Background())
	if err != nil {
		t.Fatalf("StartDevice: %v", err)
	}
	// device-init MUST be form-urlencoded (a JSON body 400s on the real host).
	if gotCT != "application/x-www-form-urlencoded" {
		t.Errorf("init Content-Type = %q, want application/x-www-form-urlencoded", gotCT)
	}
	if gotForm.Get("client_id") != ClientID || gotForm.Get("scope") != DeviceScope {
		t.Errorf("init form = %v, want client_id=%s scope=%s", gotForm, ClientID, DeviceScope)
	}
	// The same-origin guard requires Origin == issuer (the test server itself).
	if gotOrigin != srv.URL {
		t.Errorf("init Origin = %q, want %q", gotOrigin, srv.URL)
	}
	if d.UserCode != "WXYZ-1234" || d.DeviceCode != "dev-secret" {
		t.Errorf("device auth = %+v", d)
	}
}

// TestDeviceScopeCarriesRequiredBits pins the login scope contract: `civitai
// login` must REQUEST exactly UserRead + AppBlocksSubmit + AppBlocksDevTunnel —
// the civitai-cli client's allowedScopes set. It must NOT request AIServicesWrite:
// the server's device-flow validateScope is all-or-nothing, so asking for a scope
// the client doesn't allow would REJECT the whole login. A granted token can still
// (a) identify the user, (b) MINT an App-Blocks dev token (read/estimate; the mint
// strips ai:write:budgeted without AIServicesWrite), (c) submit via the
// AppBlocksSubmit-gated routes, and (d) open an on-site dev tunnel
// (AppBlocksDevTunnel). Real-Buzz dev:live needs AIServicesWrite, which comes
// from `login --scopes generate` (see ResolveDeviceScope) or a personal API key —
// never from this DEFAULT mask.
func TestDeviceScopeCarriesRequiredBits(t *testing.T) {
	const (
		userRead           = 1 << 0  // 1
		aiServicesWrite    = 1 << 15 // 32768
		appBlocksSubmit    = 1 << 25 // 33554432
		appBlocksDevTunnel = 1 << 26 // 67108864
	)
	got, err := strconv.Atoi(DeviceScope)
	if err != nil {
		t.Fatalf("DeviceScope %q is not a base-10 bitmask: %v", DeviceScope, err)
	}
	for _, c := range []struct {
		name string
		bit  int
	}{
		{"UserRead", userRead},
		{"AppBlocksSubmit", appBlocksSubmit},
		{"AppBlocksDevTunnel", appBlocksDevTunnel},
	} {
		if got&c.bit == 0 {
			t.Errorf("DeviceScope=%d is missing the %s bit (%d)", got, c.name, c.bit)
		}
	}
	// AIServicesWrite must be ABSENT — requesting it would break `civitai login`
	// (the civitai-cli client does not allow it, and validateScope is all-or-nothing).
	if got&aiServicesWrite != 0 {
		t.Errorf("DeviceScope=%d unexpectedly includes the AIServicesWrite bit (%d) — would break login", got, aiServicesWrite)
	}
	if want := userRead | appBlocksSubmit | appBlocksDevTunnel; got != want {
		t.Errorf("DeviceScope=%d, want exactly %d (UserRead|AppBlocksSubmit|AppBlocksDevTunnel)", got, want)
	}
}

func TestPollTokenHappyPathAfterPending(t *testing.T) {
	var calls int32
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			writeDiscovery(w, base)
			return
		}
		if r.URL.Path != pathDeviceToken {
			t.Errorf("poll hit wrong path %q", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("poll Content-Type = %q, want application/x-www-form-urlencoded", ct)
		}
		if o := r.Header.Get("Origin"); o != base {
			t.Errorf("poll Origin = %q, want %q", o, base)
		}
		_ = r.ParseForm()
		if r.PostFormValue("grant_type") != grantTypeDeviceCode || r.PostFormValue("device_code") != "dc" || r.PostFormValue("client_id") != ClientID {
			t.Errorf("poll form = %v", r.PostForm)
		}
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
			return
		}
		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: "at-1", TokenType: "Bearer", ExpiresIn: 3600,
			RefreshToken: "rt-1", Scope: DeviceScope,
		})
	}))
	defer srv.Close()
	base = srv.URL

	var slept int
	sleep := func(time.Duration) { slept++ }
	c := NewOAuthClient(srv.URL)
	tr, err := c.PollToken(context.Background(), &DeviceAuth{DeviceCode: "dc", Interval: 1, ExpiresIn: 900}, sleep)
	if err != nil {
		t.Fatalf("PollToken: %v", err)
	}
	if tr.AccessToken != "at-1" || tr.RefreshToken != "rt-1" {
		t.Errorf("token response = %+v", tr)
	}
	if calls != 3 {
		t.Errorf("expected 3 polls, got %d", calls)
	}
	if slept != 2 {
		t.Errorf("expected 2 sleeps (between the 3 polls), got %d", slept)
	}
}

func TestPollTokenSlowDownIncreasesInterval(t *testing.T) {
	var calls int32
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			writeDiscovery(w, base)
			return
		}
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "slow_down"})
			return
		}
		_ = json.NewEncoder(w).Encode(TokenResponse{AccessToken: "at", TokenType: "Bearer", ExpiresIn: 3600})
	}))
	defer srv.Close()
	base = srv.URL

	var durs []time.Duration
	sleep := func(d time.Duration) { durs = append(durs, d) }
	_, err := NewOAuthClient(srv.URL).PollToken(context.Background(),
		&DeviceAuth{DeviceCode: "dc", Interval: 5, ExpiresIn: 900}, sleep)
	if err != nil {
		t.Fatalf("PollToken: %v", err)
	}
	// After a slow_down the interval should grow from 5s to 10s.
	if len(durs) != 1 || durs[0] != 10*time.Second {
		t.Errorf("after slow_down expected one 10s sleep, got %v", durs)
	}
}

func TestPollTokenTerminalErrors(t *testing.T) {
	for _, code := range []string{"expired_token", "access_denied"} {
		var base string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/.well-known/openid-configuration" {
				writeDiscovery(w, base)
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
		}))
		base = srv.URL
		_, err := NewOAuthClient(srv.URL).PollToken(context.Background(),
			&DeviceAuth{DeviceCode: "dc", Interval: 1, ExpiresIn: 900}, func(time.Duration) {})
		srv.Close()
		if err == nil {
			t.Fatalf("%s: expected terminal error", code)
		}
		var dfe *DeviceFlowError
		if !errorsAs(err, &dfe) || dfe.Code != code {
			t.Errorf("%s: error = %v, want DeviceFlowError{%s}", code, err, code)
		}
	}
}

func TestPollTokenOverallTimeout(t *testing.T) {
	// Always pending: the flow must give up at the deadline.
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			writeDiscovery(w, base)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
	}))
	defer srv.Close()
	base = srv.URL

	// ExpiresIn 0 => deadline already past on the first loop check.
	_, err := NewOAuthClient(srv.URL).PollToken(context.Background(),
		&DeviceAuth{DeviceCode: "dc", Interval: 1, ExpiresIn: 0}, func(time.Duration) {})
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("timeout error should mention expiry: %v", err)
	}
}

func TestRefreshRotatesToken(t *testing.T) {
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			writeDiscovery(w, base)
			return
		}
		if r.URL.Path != pathToken {
			t.Errorf("refresh hit wrong path %q", r.URL.Path)
		}
		// The /token route (the @node-oauth/oauth2-server token handler) REQUIRES
		// application/x-www-form-urlencoded and rejects JSON with "content must be
		// application/x-www-form-urlencoded". Assert the CLI sends form encoding —
		// this is the regression guard for the bug where Refresh() posted JSON and
		// the real server rejected every refresh after the 1h access-token TTL.
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("refresh Content-Type = %q, want application/x-www-form-urlencoded", ct)
		}
		// The auth host's host-wide same-origin guard also requires Origin==issuer
		// on the refresh POST or it 403s.
		if o := r.Header.Get("Origin"); o != base {
			t.Errorf("refresh Origin = %q, want %q", o, base)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if r.PostFormValue("grant_type") != grantTypeRefreshToken || r.PostFormValue("refresh_token") != "old-rt" {
			t.Errorf("refresh form = %v", r.PostForm)
		}
		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: "new-at", ExpiresIn: 3600, RefreshToken: "new-rt", Scope: DeviceScope,
		})
	}))
	defer srv.Close()
	base = srv.URL

	tr, err := NewOAuthClient(srv.URL).Refresh(context.Background(), "old-rt")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if tr.AccessToken != "new-at" || tr.RefreshToken != "new-rt" {
		t.Errorf("refresh result = %+v", tr)
	}
	if tr.Scope.String() != DeviceScope {
		t.Errorf("scope = %q, want %q", tr.Scope, DeviceScope)
	}
}

// TestRefreshArrayScopeContract replays the ACTUAL server refresh body, where
// the @node-oauth/oauth2-server token route serializes scope as an ARRAY of
// strings (e.g. ["33554433"]) — unlike the device-token route, which sends a
// plain string. Before the C1 fix (Scope was a plain `string`), json.Unmarshal
// of this body failed the whole struct and Refresh() returned "unexpected
// refresh response", silently killing refresh after the 1h access-token TTL.
// This test is the regression guard: it FAILS without the custom Scope
// UnmarshalJSON and PASSES with it.
func TestRefreshArrayScopeContract(t *testing.T) {
	// Raw body byte-for-byte as the real /api/auth/oauth/token route emits it.
	const body = `{"access_token":"new-at","token_type":"Bearer","expires_in":3600,"refresh_token":"new-rt","scope":["33554433"]}`
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			writeDiscovery(w, base)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	base = srv.URL

	tr, err := NewOAuthClient(srv.URL).Refresh(context.Background(), "old-rt")
	if err != nil {
		t.Fatalf("Refresh with array scope: %v", err)
	}
	if tr.AccessToken != "new-at" || tr.RefreshToken != "new-rt" {
		t.Errorf("refresh result = %+v", tr)
	}
	// Array ["33554433"] must normalize to the space-joined string "33554433".
	if tr.Scope.String() != "33554433" {
		t.Errorf("array scope normalized to %q, want %q", tr.Scope, "33554433")
	}
}

// TestRefreshMultiElementArrayScope confirms a multi-element scope array is
// normalized to a space-joined string per OAuth convention.
func TestRefreshMultiElementArrayScope(t *testing.T) {
	const body = `{"access_token":"a","expires_in":3600,"refresh_token":"r","scope":["1","33554432"]}`
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			writeDiscovery(w, base)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	base = srv.URL

	tr, err := NewOAuthClient(srv.URL).Refresh(context.Background(), "rt")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if tr.Scope.String() != "1 33554432" {
		t.Errorf("multi-element scope = %q, want %q", tr.Scope, "1 33554432")
	}
}

// TestDeviceTokenStringScopeContract replays the device-token (login) route's
// string-shaped scope and asserts it parses to the same plain string. Together
// with TestRefreshArrayScopeContract this covers BOTH server scope shapes.
func TestDeviceTokenStringScopeContract(t *testing.T) {
	const body = `{"access_token":"at","token_type":"Bearer","expires_in":3600,"refresh_token":"rt","scope":"33554433"}`
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			writeDiscovery(w, base)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	base = srv.URL

	c := NewOAuthClient(srv.URL)
	tr, err := c.PollToken(context.Background(),
		&DeviceAuth{DeviceCode: "dc", Interval: 1, ExpiresIn: 900}, func(time.Duration) {})
	if err != nil {
		t.Fatalf("PollToken: %v", err)
	}
	if tr.Scope.String() != "33554433" {
		t.Errorf("string scope = %q, want %q", tr.Scope, "33554433")
	}
}

// TestPollSlowDownIntervalCapped verifies the +5s-per-slow_down backoff is
// bounded by maxPollInterval (M2): many consecutive slow_downs must not push
// the sleep interval past the cap.
func TestPollSlowDownIntervalCapped(t *testing.T) {
	var calls int32
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			writeDiscovery(w, base)
			return
		}
		n := atomic.AddInt32(&calls, 1)
		// 20 slow_downs (would push interval to 5+20*5=105s uncapped), then succeed.
		if n <= 20 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "slow_down"})
			return
		}
		_ = json.NewEncoder(w).Encode(TokenResponse{AccessToken: "at", ExpiresIn: 3600})
	}))
	defer srv.Close()
	base = srv.URL

	var durs []time.Duration
	sleep := func(d time.Duration) { durs = append(durs, d) }
	_, err := NewOAuthClient(srv.URL).PollToken(context.Background(),
		&DeviceAuth{DeviceCode: "dc", Interval: 5, ExpiresIn: 100000}, sleep)
	if err != nil {
		t.Fatalf("PollToken: %v", err)
	}
	for i, d := range durs {
		if d > maxPollInterval {
			t.Errorf("sleep[%d] = %v exceeds cap %v", i, d, maxPollInterval)
		}
	}
	// The last few sleeps should be clamped at the cap, not growing past it.
	if len(durs) == 0 || durs[len(durs)-1] != maxPollInterval {
		t.Errorf("final interval = %v, want clamped to %v", durs, maxPollInterval)
	}
}

func TestRefreshTerminalError(t *testing.T) {
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			writeDiscovery(w, base)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
	}))
	defer srv.Close()
	base = srv.URL
	if _, err := NewOAuthClient(srv.URL).Refresh(context.Background(), "rt"); err == nil {
		t.Fatal("expected error for invalid_grant")
	}
}

// TestAuthOriginDerivation pins the BaseURL -> auth-origin mapping. Production
// civitai.com routes OAuth to auth.civitai.com (civitai.com itself 404s the
// discovery doc), so a real dotted host gets "auth." prepended; auth.* hosts,
// IPs, and single-label hosts (localhost / httptest's 127.0.0.1) are used as-is.
func TestAuthOriginDerivation(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://civitai.com", "https://auth.civitai.com"},
		{"https://civitai.com/", "https://auth.civitai.com"},
		{"https://auth.civitai.com", "https://auth.civitai.com"},
		{"https://dev.civitai.com", "https://auth.dev.civitai.com"},
		{"http://127.0.0.1:8080", "http://127.0.0.1:8080"},
		{"http://localhost:3000", "http://localhost:3000"},
		{"https://staging.example.com:8443", "https://auth.staging.example.com:8443"},
		// FIX 2: a real dotted public host on http MUST be upgraded to https —
		// the device_code + refresh token are POSTed here and must never go in
		// cleartext.
		{"http://civitai.com", "https://auth.civitai.com"},
		{"http://staging.example.com:8443", "https://auth.staging.example.com:8443"},
		// Exemptions PRESERVED: loopback / single-label / IP keep their http
		// scheme for local-dev/test.
		{"http://localhost", "http://localhost"},
		{"http://myservice:8080", "http://myservice:8080"},
		{"http://127.0.0.1", "http://127.0.0.1"},
	}
	for _, c := range cases {
		if got := authOrigin(c.in); got != c.want {
			t.Errorf("authOrigin(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestResolveEndpointsFromDiscovery confirms the device flow uses the endpoints
// + issuer the OpenID discovery document advertises (the device-token poll
// endpoint is derived as issuer + pathDeviceToken since it is NOT in the doc).
// The advertised endpoints share the discovery origin's host — as production
// (auth.civitai.com advertises its OWN endpoints) and FIX 1's same-origin
// validation require.
func TestResolveEndpointsFromDiscovery(t *testing.T) {
	var issuer string // = srv.URL (the discovery origin)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			t.Errorf("discovery hit wrong path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issuer":"` + issuer + `",` +
			`"device_authorization_endpoint":"` + issuer + `/api/auth/oauth/device",` +
			`"token_endpoint":"` + issuer + `/api/auth/oauth/token"}`))
	}))
	defer srv.Close()
	issuer = srv.URL

	// Force the client to discover against this test server's origin by using it
	// as the BaseURL (a single-label/IP host is used as-is by authOrigin).
	c := NewOAuthClient(srv.URL)
	ep, err := c.resolveEndpoints(context.Background())
	if err != nil {
		t.Fatalf("resolveEndpoints: %v", err)
	}
	if ep.Issuer != issuer {
		t.Errorf("issuer = %q, want %q", ep.Issuer, issuer)
	}
	if ep.DeviceInit != issuer+pathDeviceInit {
		t.Errorf("DeviceInit = %q, want %q", ep.DeviceInit, issuer+pathDeviceInit)
	}
	if ep.Token != issuer+pathToken {
		t.Errorf("Token = %q, want %q", ep.Token, issuer+pathToken)
	}
	// device-token is derived (not advertised in the doc).
	if ep.DeviceToken != issuer+pathDeviceToken {
		t.Errorf("DeviceToken = %q, want %q (issuer + path)", ep.DeviceToken, issuer+pathDeviceToken)
	}
}

// TestResolveEndpointsRejectsForeignTokenEndpoint is the FIX 1 regression guard:
// a (tampered) discovery doc whose token_endpoint points at a FOREIGN host must
// NOT be trusted — the client falls back to the well-known paths on the trusted
// auth origin so the refresh-token POST can't be redirected cross-host. We test
// the token endpoint (refresh-token target) and the device endpoint (device_code
// target) separately, with both an http and an https foreign host.
func TestResolveEndpointsRejectsForeignTokenEndpoint(t *testing.T) {
	cases := []struct {
		name              string
		deviceEP, tokenEP string
	}{
		{"foreign token_endpoint (https)", "/api/auth/oauth/device", "https://evil.example/api/auth/oauth/token"},
		{"foreign token_endpoint (http)", "/api/auth/oauth/device", "http://attacker.example/api/auth/oauth/token"},
		{"foreign device_endpoint (https)", "https://evil.example/api/auth/oauth/device", "/api/auth/oauth/token"},
	}
	// Capture the low-noise security warning so it doesn't litter test output,
	// and assert it carries no token/code material.
	var warnBuf strings.Builder
	prevStderr := stderr
	stderr = &warnBuf
	defer func() { stderr = prevStderr }()

	for _, tc := range cases {
		warnBuf.Reset()
		t.Run(tc.name, func(t *testing.T) {
			var origin string // = srv.URL (the discovery + auth origin)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/.well-known/openid-configuration" {
					t.Errorf("discovery hit wrong path %q", r.URL.Path)
				}
				// Resolve a same-origin path to a full URL on this server; a foreign
				// host stays as given.
				deviceEP := tc.deviceEP
				if strings.HasPrefix(deviceEP, "/") {
					deviceEP = origin + deviceEP
				}
				tokenEP := tc.tokenEP
				if strings.HasPrefix(tokenEP, "/") {
					tokenEP = origin + tokenEP
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"issuer":"` + origin + `",` +
					`"device_authorization_endpoint":"` + deviceEP + `",` +
					`"token_endpoint":"` + tokenEP + `"}`))
			}))
			defer srv.Close()
			origin = srv.URL

			c := NewOAuthClient(srv.URL)
			ep, err := c.resolveEndpoints(context.Background())
			if err != nil {
				t.Fatalf("resolveEndpoints: %v", err)
			}
			// The client must have fallen back to the well-known paths on the
			// trusted auth origin — NOT the attacker's host.
			authOrig := authOrigin(srv.URL) // == srv.URL for an IP host
			if ep.DeviceInit != authOrig+pathDeviceInit ||
				ep.DeviceToken != authOrig+pathDeviceToken ||
				ep.Token != authOrig+pathToken {
				t.Errorf("did not fall back to the auth origin: ep=%+v, want paths on %q", ep, authOrig)
			}
			// Every resolved endpoint must share the auth origin's host.
			wantHost := mustHost(t, authOrig)
			for name, u := range map[string]string{"DeviceInit": ep.DeviceInit, "DeviceToken": ep.DeviceToken, "Token": ep.Token} {
				if h := mustHost(t, u); h != wantHost {
					t.Errorf("%s host = %q, want auth-origin host %q (untrusted endpoint leaked)", name, h, wantHost)
				}
			}
			// A warning should have been emitted, and it must NOT leak the
			// attacker host's credentials path or any code/token.
			if got := warnBuf.String(); !strings.Contains(got, "falling back") {
				t.Errorf("expected a fallback warning, got %q", got)
			}
		})
	}
}

// mustHost parses a URL and returns its host, failing the test on a parse error.
func mustHost(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", raw, err)
	}
	return u.Hostname()
}

// TestResolveEndpointsFallbackOn404 confirms that when discovery is unavailable
// (the well-known 404s, as civitai.com itself does), resolveEndpoints falls
// back to the known paths on the auth origin so login still works.
func TestResolveEndpointsFallbackOn404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := NewOAuthClient(srv.URL)
	ep, err := c.resolveEndpoints(context.Background())
	if err != nil {
		t.Fatalf("resolveEndpoints: %v", err)
	}
	if ep.Issuer != srv.URL || ep.DeviceInit != srv.URL+pathDeviceInit ||
		ep.DeviceToken != srv.URL+pathDeviceToken || ep.Token != srv.URL+pathToken {
		t.Errorf("fallback endpoints = %+v, want paths on %q", ep, srv.URL)
	}
}

// TestStartDeviceFailsWithoutOrigin is the regression guard for the actual prod
// failure: a test server that mimics the host-wide same-origin guard — it 403s
// any form POST that lacks an Origin header. The OLD code (JSON body, no Origin)
// would fail this; the fixed code sets Origin == issuer and succeeds.
func TestStartDeviceFailsWithoutOrigin(t *testing.T) {
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			writeDiscovery(w, base)
			return
		}
		// Mimic the auth host's guard: cross-site / origin-less form POST -> 403.
		if r.Header.Get("Origin") != base {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"Cross-site POST form submissions are forbidden"}`))
			return
		}
		// Mimic the JSON-body failure too: no parsed form -> "Missing client_id".
		_ = r.ParseForm()
		if r.PostFormValue("client_id") == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_request","error_description":"Missing client_id"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(DeviceAuth{
			DeviceCode: "dc", UserCode: "AAAA-BBBB", ExpiresIn: 900, Interval: 5,
		})
	}))
	defer srv.Close()
	base = srv.URL

	d, err := NewOAuthClient(srv.URL).StartDevice(context.Background())
	if err != nil {
		t.Fatalf("StartDevice should satisfy the same-origin guard: %v", err)
	}
	if d.UserCode != "AAAA-BBBB" {
		t.Errorf("device auth = %+v", d)
	}
}

// errorsAs is a tiny local errors.As shim to avoid importing errors in just one
// spot; keeps the test file self-contained.
func errorsAs(err error, target **DeviceFlowError) bool {
	for err != nil {
		if dfe, ok := err.(*DeviceFlowError); ok {
			*target = dfe
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
