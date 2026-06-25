package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMintDevTokenSendsBearerAndBody(t *testing.T) {
	var gotAuth, gotMethod, gotPath, gotCT, gotSlug string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Slug string `json:"slug"`
		}
		_ = json.Unmarshal(raw, &body)
		gotSlug = body.Slug
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "jwt-x", "expiresAt": "soon"})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok123", "")
	tok, err := c.MintDevToken(context.Background(), "my-block")
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
}

func TestMintDevTokenEmptyTokenErrors(t *testing.T) {
	// A 200 with no token field is a malformed response, not a success.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"expiresAt": "soon"})
	}))
	defer srv.Close()
	c := New(srv.URL, "tok", "")
	if _, err := c.MintDevToken(context.Background(), "my-block"); err == nil {
		t.Fatal("expected error when response has no token")
	}
}

func TestMintDevTokenErrorMapping(t *testing.T) {
	cases := []struct {
		status int
		body   map[string]string
		want   string
	}{
		{http.StatusNotFound, map[string]string{"message": "no such app"}, "app not found (or not yours)"},
		{http.StatusForbidden, map[string]string{"message": "mods only"}, "not authorized (403)"},
		{http.StatusUnauthorized, map[string]string{"message": "bad key"}, "not logged in"},
		{http.StatusTooManyRequests, map[string]string{"message": "slow"}, "rate limited"},
		{http.StatusServiceUnavailable, map[string]string{"message": "flag off"}, "App Blocks unavailable (503)"},
		{http.StatusInternalServerError, map[string]string{"message": "boom"}, "server returned 500"},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_ = json.NewEncoder(w).Encode(tc.body)
		}))
		_, err := New(srv.URL, "tok", "").MintDevToken(context.Background(), "my-block")
		srv.Close()
		if err == nil {
			t.Fatalf("status %d: expected error", tc.status)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("status %d: error %q should contain %q", tc.status, err.Error(), tc.want)
		}
	}
}
