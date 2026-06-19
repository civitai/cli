package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["grant_type"] != grantTypeRefreshToken || body["refresh_token"] != "old-rt" {
			t.Errorf("refresh body = %v", body)
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
