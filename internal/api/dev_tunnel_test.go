package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestStartDevTunnelRequestResponse asserts the CLI POSTs the exact tRPC input
// shape ({"json":{blockId,sshPublicKey}}) and decodes the superjson result
// envelope into a DevTunnelSession matching the P1 server contract.
func TestStartDevTunnelRequestResponse(t *testing.T) {
	var gotAuth, gotCT, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{"data": map[string]any{"json": map[string]any{
				"sessionId":        "bki_abc",
				"host":             "dev-0123456789abcdef.civit.ai",
				"url":              "https://civitai.com/apps/dev/my-block",
				"expiresAt":        1893456000,
				"spendCapBuzz":     5000,
				"sshHostPublicKey": "ssh-ed25519 AAAAHOSTKEY",
			}}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok-1", "")
	sess, err := c.StartDevTunnel(context.Background(), "my-block", "ssh-ed25519 AAAAKEY")
	if err != nil {
		t.Fatalf("StartDevTunnel: %v", err)
	}

	if gotPath != "/api/trpc/blocks.startDevTunnel" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer tok-1" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type = %q", gotCT)
	}
	// Exact tRPC input envelope.
	var wire struct {
		JSON startDevTunnelInput `json:"json"`
	}
	if err := json.Unmarshal([]byte(gotBody), &wire); err != nil {
		t.Fatalf("request body not {json:...}: %v (%s)", err, gotBody)
	}
	if wire.JSON.BlockID != "my-block" || wire.JSON.SSHPublicKey != "ssh-ed25519 AAAAKEY" {
		t.Errorf("input = %+v, want blockId=my-block + the pubkey", wire.JSON)
	}
	// Decoded result.
	if sess.SessionID != "bki_abc" || sess.Host != "dev-0123456789abcdef.civit.ai" {
		t.Errorf("session = %+v", sess)
	}
	if sess.URL != "https://civitai.com/apps/dev/my-block" || sess.SpendCapBuzz != 5000 {
		t.Errorf("session url/cap = %q/%d", sess.URL, sess.SpendCapBuzz)
	}
	if sess.ExpiresAt != 1893456000 {
		t.Errorf("expiresAt = %d", sess.ExpiresAt)
	}
	// R1: the sish host public key the CLI pins is decoded from the mint response.
	if sess.SSHHostPublicKey != "ssh-ed25519 AAAAHOSTKEY" {
		t.Errorf("sshHostPublicKey = %q, want the pinned host key line", sess.SSHHostPublicKey)
	}
}

// TestStopDevTunnelBySession asserts stop POSTs {"json":{sessionId}} and decodes
// the {ok,stopped} result.
func TestStopDevTunnelBySession(t *testing.T) {
	var gotBody, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{"data": map[string]any{"json": map[string]any{
				"ok": true, "stopped": true,
			}}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok-1", "")
	stopped, err := c.StopDevTunnel(context.Background(), "bki_abc", "")
	if err != nil {
		t.Fatalf("StopDevTunnel: %v", err)
	}
	if !stopped {
		t.Error("stopped should be true")
	}
	if gotPath != "/api/trpc/blocks.stopDevTunnel" {
		t.Errorf("path = %q", gotPath)
	}
	if !strings.Contains(gotBody, `"sessionId":"bki_abc"`) {
		t.Errorf("stop body should carry sessionId: %s", gotBody)
	}
	// A sessionId selector must NOT also send a blockId key (omitempty).
	if strings.Contains(gotBody, `"blockId"`) {
		t.Errorf("stop-by-session body must omit blockId: %s", gotBody)
	}
}

// TestStopDevTunnelByBlock asserts blockId selection when no sessionId is known.
func TestStopDevTunnelByBlock(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{"data": map[string]any{"json": map[string]any{"ok": true, "stopped": false}}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok-1", "")
	stopped, err := c.StopDevTunnel(context.Background(), "", "my-block")
	if err != nil {
		t.Fatalf("StopDevTunnel: %v", err)
	}
	if stopped {
		t.Error("stopped should be false (nothing active)")
	}
	if !strings.Contains(gotBody, `"blockId":"my-block"`) || strings.Contains(gotBody, `"sessionId"`) {
		t.Errorf("stop-by-block body wrong: %s", gotBody)
	}
}

// TestStopDevTunnelNeedsSelector guards the client-side precondition.
func TestStopDevTunnelNeedsSelector(t *testing.T) {
	c := New("http://unused", "tok", "")
	if _, err := c.StopDevTunnel(context.Background(), "", ""); err == nil {
		t.Fatal("expected an error when neither sessionId nor blockId is given")
	}
}

// TestStartDevTunnelErrorMapping asserts the tRPC error statuses map to
// actionable CLI guidance.
func TestStartDevTunnelErrorMapping(t *testing.T) {
	cases := []struct {
		status int
		want   string
	}{
		{http.StatusForbidden, "not available"},
		{http.StatusNotFound, "app not found"},
		{http.StatusUnauthorized, "not logged in"},
		{http.StatusServiceUnavailable, "unavailable (503)"},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"json": map[string]any{"message": "denied"}},
			})
		}))
		c := New(srv.URL, "tok", "")
		_, err := c.StartDevTunnel(context.Background(), "my-block", "ssh-ed25519 K")
		srv.Close()
		if err == nil {
			t.Fatalf("status %d: expected error", tc.status)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("status %d: error %q should contain %q", tc.status, err.Error(), tc.want)
		}
	}
}

// TestStartDevTunnelForbiddenClassifiesScope: a 403 is split by its server
// message into a token-SCOPE refusal (fix = full-scope key) vs the author/flag
// gate (fix = enrolled account). Both are FORBIDDEN with no distinct code, so
// the message is the only signal.
func TestStartDevTunnelForbiddenClassifiesScope(t *testing.T) {
	cases := []struct {
		name      string
		serverMsg string
		wantScope bool
		wantText  string
	}{
		{"scope", "Your API key does not have the required scope for this action", true, "full-scope credential"},
		{"authorFlag", "Dev tunnels are not available", false, "not available for your account"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{"json": map[string]any{"message": tc.serverMsg}},
				})
			}))
			defer srv.Close()
			c := New(srv.URL, "tok", "")
			_, err := c.StartDevTunnel(context.Background(), "my-block", "ssh-ed25519 K")
			var forbidden *DevTunnelForbiddenError
			if !errors.As(err, &forbidden) {
				t.Fatalf("403 should map to *DevTunnelForbiddenError, got %v", err)
			}
			if forbidden.InsufficientScope != tc.wantScope {
				t.Errorf("InsufficientScope = %v, want %v (msg %q)", forbidden.InsufficientScope, tc.wantScope, tc.serverMsg)
			}
			if !strings.Contains(err.Error(), tc.wantText) || !strings.Contains(err.Error(), "403") {
				t.Errorf("error %q should contain %q and 403", err.Error(), tc.wantText)
			}
		})
	}
}
