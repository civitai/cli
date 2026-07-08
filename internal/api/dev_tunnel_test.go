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
	sess, err := c.StartDevTunnel(context.Background(), "my-block", "ssh-ed25519 AAAAKEY", nil)
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
	// No scopes passed → declaredScopes is OMITTED entirely (omitempty), keeping
	// the wire shape identical to the pre-scopes request an old server expects.
	if strings.Contains(gotBody, "declaredScopes") {
		t.Errorf("empty scopes must omit the declaredScopes key: %s", gotBody)
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

// TestStartDevTunnelDeclaredScopes: when the caller passes the local manifest's
// scopes they are serialized as a `declaredScopes` string array under the tRPC
// `json` key — this is what lets the server grant them to an UNSUBMITTED app's
// tunnel token.
func TestStartDevTunnelDeclaredScopes(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{"data": map[string]any{"json": map[string]any{
				"sessionId":        "bki_abc",
				"host":             "dev-0123456789abcdef.civit.ai",
				"url":              "https://civitai.com/apps/dev/my-block",
				"expiresAt":        1893456000,
				"sshHostPublicKey": "ssh-ed25519 AAAAHOSTKEY",
			}}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok-1", "")
	scopes := []string{"ai:write:budgeted", "user:read:self"}
	if _, err := c.StartDevTunnel(context.Background(), "my-block", "ssh-ed25519 AAAAKEY", scopes); err != nil {
		t.Fatalf("StartDevTunnel: %v", err)
	}
	var wire struct {
		JSON startDevTunnelInput `json:"json"`
	}
	if err := json.Unmarshal([]byte(gotBody), &wire); err != nil {
		t.Fatalf("request body not {json:...}: %v (%s)", err, gotBody)
	}
	if len(wire.JSON.DeclaredScopes) != 2 ||
		wire.JSON.DeclaredScopes[0] != "ai:write:budgeted" ||
		wire.JSON.DeclaredScopes[1] != "user:read:self" {
		t.Errorf("declaredScopes = %#v, want the two manifest scopes in order", wire.JSON.DeclaredScopes)
	}
	// It rides under the tRPC `json` envelope (not top-level).
	if !strings.Contains(gotBody, `"declaredScopes":["ai:write:budgeted","user:read:self"]`) {
		t.Errorf("declaredScopes should serialize as a string array under json: %s", gotBody)
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
		{http.StatusNotFound, "can't tunnel this app"},
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
		_, err := c.StartDevTunnel(context.Background(), "my-block", "ssh-ed25519 K", nil)
		srv.Close()
		if err == nil {
			t.Fatalf("status %d: expected error", tc.status)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("status %d: error %q should contain %q", tc.status, err.Error(), tc.want)
		}
	}
}

// TestStartDevTunnel401DropsServerMessage: on this path a 401 is always a
// missing/expired/invalid credential, and the server's tRPC message is the
// misleading origin-gate string ("Please use the public API instead"). The
// mapped error must NOT echo it — it should just tell the user to log in.
func TestStartDevTunnel401DropsServerMessage(t *testing.T) {
	const serverMsg = "Please use the public API instead"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"json": map[string]any{"message": serverMsg}},
		})
	}))
	defer srv.Close()
	c := New(srv.URL, "tok", "")
	_, err := c.StartDevTunnel(context.Background(), "my-block", "ssh-ed25519 K", nil)
	if err == nil {
		t.Fatal("expected error on 401")
	}
	if strings.Contains(err.Error(), serverMsg) {
		t.Errorf("401 error %q should NOT echo the server message %q", err.Error(), serverMsg)
	}
	if !strings.Contains(err.Error(), "civitai login") {
		t.Errorf("401 error %q should tell the user to run `civitai login`", err.Error())
	}
}

// TestStartDevTunnel404SlugTakenOrInvalid: with the ephemeral pre-submit
// resolver deployed (civitai #2983/#2984), a new own app tunnels without
// submitting, so a 404 now means the slug is taken by another account or isn't
// a valid slug. The error must reflect that — NOT tell the user to submit
// first — and still point at `civitai app status`.
func TestStartDevTunnel404SlugTakenOrInvalid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"json": map[string]any{"message": "not found"}},
		})
	}))
	defer srv.Close()
	c := New(srv.URL, "tok", "")
	_, err := c.StartDevTunnel(context.Background(), "my-block", "ssh-ed25519 K", nil)
	if err == nil {
		t.Fatal("expected error on 404")
	}
	if strings.Contains(err.Error(), "civitai app submit") {
		t.Errorf("404 error %q should no longer tell the user to submit first (pre-submit tunneling shipped)", err.Error())
	}
	if !strings.Contains(err.Error(), "registered to a different account") {
		t.Errorf("404 error %q should say the slug is registered to a different account", err.Error())
	}
	if !strings.Contains(err.Error(), "isn't a valid app slug") {
		t.Errorf("404 error %q should say the slug may be invalid", err.Error())
	}
	if !strings.Contains(err.Error(), "civitai app status") {
		t.Errorf("404 error %q should still point at `civitai app status`", err.Error())
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
			_, err := c.StartDevTunnel(context.Background(), "my-block", "ssh-ed25519 K", nil)
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
