package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestStartDeviceParsesResponse(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathDeviceInit {
			t.Errorf("init hit wrong path %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(DeviceAuth{
			DeviceCode:              "dev-secret",
			UserCode:                "WXYZ-1234",
			VerificationURI:         "https://civitai.com/oauth/device",
			VerificationURIComplete: "https://civitai.com/oauth/device?user_code=WXYZ-1234",
			ExpiresIn:               900,
			Interval:                5,
		})
	}))
	defer srv.Close()

	d, err := NewOAuthClient(srv.URL).StartDevice(context.Background())
	if err != nil {
		t.Fatalf("StartDevice: %v", err)
	}
	if gotBody["client_id"] != ClientID || gotBody["scope"] != DeviceScope {
		t.Errorf("init body = %v, want client_id=%s scope=%s", gotBody, ClientID, DeviceScope)
	}
	if d.UserCode != "WXYZ-1234" || d.DeviceCode != "dev-secret" {
		t.Errorf("device auth = %+v", d)
	}
}

// TestDeviceScopeCarriesRequiredBits pins the login scope contract: `civitai
// login` must REQUEST UserRead + AIServicesWrite + AppBlocksSubmit so a granted
// token can (a) identify the user, (b) mint a SPEND-capable App-Blocks dev token
// for dev:live, and (c) submit/mint via the AppBlocksSubmit-gated routes. The
// effective grant is still capped server-side by the civitai-cli client's
// allowedScopes, but the CLI must ASK for these or dev:live can never spend.
func TestDeviceScopeCarriesRequiredBits(t *testing.T) {
	const (
		userRead        = 1 << 0  // 1
		aiServicesWrite = 1 << 15 // 32768
		appBlocksSubmit = 1 << 25 // 33554432
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
		{"AIServicesWrite", aiServicesWrite},
		{"AppBlocksSubmit", appBlocksSubmit},
	} {
		if got&c.bit == 0 {
			t.Errorf("DeviceScope=%d is missing the %s bit (%d)", got, c.name, c.bit)
		}
	}
	if want := userRead | aiServicesWrite | appBlocksSubmit; got != want {
		t.Errorf("DeviceScope=%d, want exactly %d (UserRead|AIServicesWrite|AppBlocksSubmit)", got, want)
	}
}

func TestPollTokenHappyPathAfterPending(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathDeviceToken {
			t.Errorf("poll hit wrong path %q", r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["grant_type"] != grantTypeDeviceCode || body["device_code"] != "dc" || body["client_id"] != ClientID {
			t.Errorf("poll body = %v", body)
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "slow_down"})
			return
		}
		_ = json.NewEncoder(w).Encode(TokenResponse{AccessToken: "at", TokenType: "Bearer", ExpiresIn: 3600})
	}))
	defer srv.Close()

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
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
		}))
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
	}))
	defer srv.Close()

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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
	}))
	defer srv.Close()
	if _, err := NewOAuthClient(srv.URL).Refresh(context.Background(), "rt"); err == nil {
		t.Fatal("expected error for invalid_grant")
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
