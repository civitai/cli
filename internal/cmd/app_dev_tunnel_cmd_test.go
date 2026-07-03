package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAppDevTunnelMissingBlockErrors: no blockId (positional or --block) is a
// clear, non-network error.
func TestAppDevTunnelMissingBlockErrors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	_, _, err := run(t, "app", "dev-tunnel")
	if err == nil {
		t.Fatal("expected error when no blockId is given")
	}
	if !strings.Contains(err.Error(), "blockId is required") {
		t.Errorf("missing-block error should explain: %v", err)
	}
}

// TestAppDevTunnelBadPortErrors: an out-of-range --port is rejected before any
// network call (flag validation).
func TestAppDevTunnelBadPortErrors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	_, _, err := run(t, "app", "dev-tunnel", "my-block", "--port", "70000")
	if err == nil {
		t.Fatal("expected error for an out-of-range port")
	}
	if !strings.Contains(err.Error(), "invalid --port") {
		t.Errorf("bad-port error should explain: %v", err)
	}
}

// TestAppDevTunnelBadIdleErrors: a non-positive --idle-timeout is rejected.
func TestAppDevTunnelBadIdleErrors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	_, _, err := run(t, "app", "dev-tunnel", "my-block", "--idle-timeout", "0s")
	if err == nil {
		t.Fatal("expected error for a non-positive idle timeout")
	}
	if !strings.Contains(err.Error(), "idle-timeout") {
		t.Errorf("bad-idle error should explain: %v", err)
	}
}

// TestAppDevTunnelMissingTokenErrors: no credential points the user at login.
func TestAppDevTunnelMissingTokenErrors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "")
	_, _, err := run(t, "app", "dev-tunnel", "my-block")
	if err == nil {
		t.Fatal("expected error with no token")
	}
	if !strings.Contains(err.Error(), "no token") || !strings.Contains(err.Error(), "civitai login") {
		t.Errorf("missing-token error should point to login: %v", err)
	}
}

// TestAppHelpListsDevTunnel: the subcommand is discoverable.
func TestAppHelpListsDevTunnel(t *testing.T) {
	out, _, err := run(t, "app", "--help")
	if err != nil {
		t.Fatalf("app --help: %v", err)
	}
	if !strings.Contains(out, "dev-tunnel") {
		t.Errorf("app help should list the dev-tunnel subcommand:\n%s", out)
	}
}

// TestAppDevTunnelForbiddenMapsToGuidance: when the server denies the mint (flag
// off / not an author), the CLI surfaces the actionable dark/pre-GA guidance.
// Exercises the FULL cobra path (config → api client → StartDevTunnel) against a
// local tRPC server, so it also proves the request path + error mapping.
func TestAppDevTunnelForbiddenMapsToGuidance(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"json": map[string]any{"message": "Dev tunnels are not available"}},
		})
	}))
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	_, _, err := run(t, "app", "dev-tunnel", "my-block")
	if err == nil {
		t.Fatal("expected a forbidden error while the flag is dark")
	}
	if !strings.Contains(err.Error(), "not available") || !strings.Contains(err.Error(), "403") {
		t.Errorf("403 should map to dark/pre-GA guidance: %v", err)
	}
	if gotPath != "/api/trpc/blocks.startDevTunnel" {
		t.Errorf("start path = %q, want the tRPC startDevTunnel route", gotPath)
	}
	// The request carries the ephemeral pubkey + blockId under the tRPC `json` key.
	if !strings.Contains(gotBody, `"blockId":"my-block"`) || !strings.Contains(gotBody, `ssh-ed25519 `) {
		t.Errorf("start body should carry blockId + an ed25519 pubkey: %s", gotBody)
	}
}
