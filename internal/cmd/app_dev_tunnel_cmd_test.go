package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// startDevTunnelRoute is the non-batched tRPC mint route. The forbidden-mint
// tests must record ONLY this request's body/path — the 403 now triggers a
// follow-up WhoAmI (/api/v1/me), which would otherwise clobber the recorders.
const startDevTunnelRoute = "/api/trpc/blocks.startDevTunnel"

// listenLocal opens a listener on an ephemeral 127.0.0.1 port and returns the
// port, so a full-path dev-tunnel test can pass `--port <port>` and satisfy the
// pre-flight local-dev-server probe (which TCP-dials 127.0.0.1:<port> before the
// mint). The listener is closed at test end.
func listenLocal(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("open local listener: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l.Addr().(*net.TCPAddr).Port
}

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
		// The forbidden mint now triggers a follow-up WhoAmI (/api/v1/me) to enrich
		// the error — only record the START request so WhoAmI can't clobber gotBody.
		if r.URL.Path == startDevTunnelRoute {
			raw, _ := io.ReadAll(r.Body)
			gotBody = string(raw)
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"json": map[string]any{"message": "Dev tunnels are not available"}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"username": "tester", "id": 7})
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

	// A live local listener so the pre-flight probe passes and we reach the mint.
	port := listenLocal(t)
	_, _, err := run(t, "app", "dev-tunnel", "--port", fmt.Sprint(port))
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

// TestAppDevTunnelBlockFlagWinsOverPositionalWithWarning: passing BOTH --block
// and a DIFFERENT positional blockId warns on stderr that --block wins, and the
// resolved blockId (carried to the mint request) is the --block value.
func TestAppDevTunnelBlockFlagWinsOverPositionalWithWarning(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == startDevTunnelRoute {
			raw, _ := io.ReadAll(r.Body)
			gotBody = string(raw)
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"json": map[string]any{"message": "Dev tunnels are not available"}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"username": "tester", "id": 7})
	}))
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	port := listenLocal(t)
	_, errOut, _ := run(t, "app", "dev-tunnel", "positional-block", "--block", "flag-block", "--port", fmt.Sprint(port))

	// The conflict warning names both values and says --block wins.
	if !strings.Contains(errOut, "flag-block") || !strings.Contains(errOut, "positional-block") ||
		!strings.Contains(strings.ToLower(errOut), "using --block") {
		t.Errorf("expected a --block-wins conflict warning on stderr, got: %s", errOut)
	}
	// --block wins: the mint request carries flag-block, NOT positional-block.
	if !strings.Contains(gotBody, `"blockId":"flag-block"`) {
		t.Errorf("resolved blockId should be the --block value: %s", gotBody)
	}
	if strings.Contains(gotBody, `"blockId":"positional-block"`) {
		t.Errorf("the positional blockId must be ignored when --block is set: %s", gotBody)
	}
}

// TestAppDevTunnelBlockFlagEqualsPositionalNoWarning: when --block and the
// positional are the SAME value they are redundant, not a conflict — no warning.
func TestAppDevTunnelBlockFlagEqualsPositionalNoWarning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"json": map[string]any{"message": "Dev tunnels are not available"}},
		})
	}))
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	port := listenLocal(t)
	_, errOut, _ := run(t, "app", "dev-tunnel", "same-block", "--block", "same-block", "--port", fmt.Sprint(port))
	if strings.Contains(strings.ToLower(errOut), "using --block") {
		t.Errorf("equal --block and positional must NOT warn: %s", errOut)
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
		// Record ONLY the mint request — the 403 now triggers a follow-up WhoAmI
		// (/api/v1/me) to name the signed-in account, which we serve a valid
		// identity for so the enriched error can be asserted.
		if r.URL.Path == startDevTunnelRoute {
			gotPath = r.URL.Path
			raw, _ := io.ReadAll(r.Body)
			gotBody = string(raw)
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"json": map[string]any{"message": "Dev tunnels are not available"}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"username": "tester", "id": 7})
	}))
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	// A live local listener so the pre-flight probe passes and we reach the mint.
	port := listenLocal(t)
	_, _, err := run(t, "app", "dev-tunnel", "my-block", "--port", fmt.Sprint(port))
	if err == nil {
		t.Fatal("expected a forbidden error while the flag is dark")
	}
	if !strings.Contains(err.Error(), "not available") || !strings.Contains(err.Error(), "403") {
		t.Errorf("403 should map to dark/pre-GA guidance: %v", err)
	}
	// The 403 is enriched with the signed-in identity + how to switch/check accounts.
	if !strings.Contains(err.Error(), "tester") {
		t.Errorf("403 should name the signed-in account (tester): %v", err)
	}
	if !strings.Contains(err.Error(), "civitai login") || !strings.Contains(err.Error(), "civitai whoami") {
		t.Errorf("403 should carry account-switch guidance (civitai login / civitai whoami): %v", err)
	}
	if gotPath != startDevTunnelRoute {
		t.Errorf("start path = %q, want the tRPC startDevTunnel route", gotPath)
	}
	// The request carries the ephemeral pubkey + blockId under the tRPC `json` key.
	if !strings.Contains(gotBody, `"blockId":"my-block"`) || !strings.Contains(gotBody, `ssh-ed25519 `) {
		t.Errorf("start body should carry blockId + an ed25519 pubkey: %s", gotBody)
	}
}
