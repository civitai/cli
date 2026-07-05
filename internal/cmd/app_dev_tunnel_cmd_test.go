package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAppDevTunnelMissingBlockErrors: no blockId (positional, --block, or a
// manifest in the CWD) is a clear, non-network error. Run from a temp dir with
// NO block.manifest.json so the manifest fallback can't accidentally resolve one.
func TestAppDevTunnelMissingBlockErrors(t *testing.T) {
	chdir(t, t.TempDir())
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

// TestAppDevTunnelBlockIdFromManifest: with NO flag/arg, run from an App project
// dir containing block.manifest.json, the blockId is resolved from the manifest
// (mirroring `civitai app submit`) — the dev never has to retype it. Guard: this
// fails without the manifest fallback (it would short-circuit on "blockId is
// required" before ever reaching the network). We point the CLI at a local tRPC
// server so no real network is touched, and assert (a) the request carried the
// manifest's blockId and (b) the error is NOT the blockId-required one (it fails
// later, on the dark/forbidden mint, which is expected pre-GA).
func TestAppDevTunnelBlockIdFromManifest(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"json": map[string]any{"message": "Dev tunnels are not available"}},
		})
	}))
	defer srv.Close()

	// App project dir with a valid manifest; blockId "demo".
	dir := t.TempDir()
	const m = `{
  "$schema": "https://civitai.com/schemas/app-block/v1.json",
  "blockId": "demo",
  "version": "0.1.0",
  "name": "Demo",
  "type": "block",
  "page": { "path": "/", "title": "Demo" }
}`
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	_, _, err := run(t, "app", "dev-tunnel")
	// It must get PAST the blockId check — the only remaining failure is the
	// (expected, pre-GA) forbidden mint, never "blockId is required".
	if err != nil && strings.Contains(err.Error(), "blockId is required") {
		t.Fatalf("blockId should have been resolved from the manifest, got: %v", err)
	}
	// Proof the resolved blockId came from the manifest and reached the request.
	if !strings.Contains(gotBody, `"blockId":"demo"`) {
		t.Errorf("start body should carry the manifest blockId 'demo': %s", gotBody)
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
