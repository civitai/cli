package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// TestMintDevToken404SentinelWrapping: the bare "App not found" 404 wraps
// ErrSlugRegisteredToOtherAccount (rename-retriable); the owned-but-undeployed
// "no live deployment" 404 does NOT.
func TestMintDevToken404SentinelWrapping(t *testing.T) {
	cases := []struct {
		name        string
		message     string
		wantWrapped bool
	}{
		{"bare app-not-found is the collision", "App not found", true},
		{"generic 404 is treated as a collision", "no such app", true},
		{"no-live-deployment is NOT a collision", "block 'x' has no live deployment — deploy first", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": tc.message})
			}))
			defer srv.Close()
			_, err := New(srv.URL, "tok", "").MintDevToken(context.Background(), "my-block", nil)
			if err == nil {
				t.Fatal("expected a 404 error")
			}
			if got := errors.Is(err, ErrSlugRegisteredToOtherAccount); got != tc.wantWrapped {
				t.Errorf("errors.Is(err, sentinel) = %v, want %v (err=%q)", got, tc.wantWrapped, err)
			}
		})
	}
}

func TestMintDevTokenSendsBearerAndBody(t *testing.T) {
	var gotAuth, gotMethod, gotPath, gotCT, gotSlug string
	var gotScopes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Slug   string   `json:"slug"`
			Scopes []string `json:"scopes"`
		}
		_ = json.Unmarshal(raw, &body)
		gotSlug = body.Slug
		gotScopes = body.Scopes
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "jwt-x", "expiresAt": "soon"})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok123", "")
	tok, err := c.MintDevToken(context.Background(), "my-block", []string{"ai:write:budgeted"})
	if err != nil {
		t.Fatalf("MintDevToken: %v", err)
	}
	if tok != "jwt-x" {
		t.Errorf("token = %q, want jwt-x", tok)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth header = %q, want Bearer tok123", gotAuth)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != DevTokenPath {
		t.Errorf("path = %q, want %q", gotPath, DevTokenPath)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type = %q, want application/json", gotCT)
	}
	if gotSlug != "my-block" {
		t.Errorf("slug = %q, want my-block", gotSlug)
	}
	if !reflect.DeepEqual(gotScopes, []string{"ai:write:budgeted"}) {
		t.Errorf("scopes = %v, want [ai:write:budgeted]", gotScopes)
	}
}

// TestMintDevTokenOmitsEmptyScopes proves the body carries NO `scopes` key when
// no scopes are passed (registered-app + read-only paths must be unchanged).
func TestMintDevTokenOmitsEmptyScopes(t *testing.T) {
	var hasScopesKey bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var generic map[string]any
		_ = json.Unmarshal(raw, &generic)
		_, hasScopesKey = generic["scopes"]
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "jwt-x"})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	if _, err := c.MintDevToken(context.Background(), "my-block", nil); err != nil {
		t.Fatalf("MintDevToken: %v", err)
	}
	if hasScopesKey {
		t.Error("body should NOT carry a scopes key when scopes is nil")
	}

	// An empty (non-nil) slice must also be omitted (omitempty).
	if _, err := c.MintDevToken(context.Background(), "my-block", []string{}); err != nil {
		t.Fatalf("MintDevToken: %v", err)
	}
	if hasScopesKey {
		t.Error("body should NOT carry a scopes key when scopes is empty")
	}
}

func TestMintDevTokenEmptyTokenErrors(t *testing.T) {
	// A 200 with no token field is a malformed response, not a success.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"expiresAt": "soon"})
	}))
	defer srv.Close()
	c := New(srv.URL, "tok", "")
	if _, err := c.MintDevToken(context.Background(), "my-block", nil); err == nil {
		t.Fatal("expected error when response has no token")
	}
}

func TestMintDevTokenErrorMapping(t *testing.T) {
	cases := []struct {
		status int
		body   map[string]string
		want   string
	}{
		{http.StatusNotFound, map[string]string{"message": "no such app"}, "registered to a different account"},
		{http.StatusForbidden, map[string]string{"message": "mods only"}, "not authorized (403)"},
		{http.StatusUnauthorized, map[string]string{"message": "bad key"}, "not logged in"},
		{http.StatusTooManyRequests, map[string]string{"message": "slow"}, "rate limited"},
		{http.StatusServiceUnavailable, map[string]string{"message": "flag off"}, "apps unavailable (503)"},
		{http.StatusInternalServerError, map[string]string{"message": "boom"}, "server returned 500"},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_ = json.NewEncoder(w).Encode(tc.body)
		}))
		_, err := New(srv.URL, "tok", "").MintDevToken(context.Background(), "my-block", nil)
		srv.Close()
		if err == nil {
			t.Fatalf("status %d: expected error", tc.status)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("status %d: error %q should contain %q", tc.status, err.Error(), tc.want)
		}
	}
}

// TestMintDevToken404DropsSubmitFirst asserts the 404 message no longer tells
// the user to submit first (the no-row path means a 404 is a slug collision).
func TestMintDevToken404DropsSubmitFirst(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "not found"})
	}))
	defer srv.Close()
	_, err := New(srv.URL, "tok", "").MintDevToken(context.Background(), "my-block", nil)
	if err == nil {
		t.Fatal("expected a 404 error")
	}
	if strings.Contains(err.Error(), "civitai app submit") {
		t.Errorf("404 message should no longer mention submit-first: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "block.manifest.json") {
		t.Errorf("404 message should mention the local manifest: %q", err.Error())
	}
}
