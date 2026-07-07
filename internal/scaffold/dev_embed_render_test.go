package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPageMoneyEmitsEmbeddableDevServer asserts the scaffolded page-money project
// ships an iframe-embeddable dev server for `civitai app dev-tunnel`: the dev
// server sends a `frame-ancestors https://civitai.com` CSP, does NOT set
// X-Frame-Options, admits the `.civit.ai` tunnel host, and the block's dev
// parent-origin allowlist includes https://civitai.com — WITHOUT touching the
// production build. This is the Go-verifiable companion to the scaffold's own
// src/dev-embed.test.ts (which the scaffolded user's `vitest run` exercises).
func TestPageMoneyEmitsEmbeddableDevServer(t *testing.T) {
	dir := t.TempDir()
	if _, err := Render(PageMoney, dir, Data{Slug: "my-block", Name: "My Block"}); err != nil {
		t.Fatalf("render page-money: %v", err)
	}

	read := func(rel string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return string(b)
	}

	// 1. The dev-embed source (single source of truth) sets the frame-ancestors
	//    CSP + the tunnel host suffix. (The "no X-Frame-Options" invariant is
	//    asserted at the header-object level in src/dev-embed.test.ts.)
	devEmbed := read("src/dev-embed.ts")
	if !strings.Contains(devEmbed, "frame-ancestors") || !strings.Contains(devEmbed, "https://civitai.com") {
		t.Errorf("dev-embed.ts should set frame-ancestors https://civitai.com:\n%s", devEmbed)
	}
	if !strings.Contains(devEmbed, ".civit.ai") {
		t.Errorf("dev-embed.ts should admit the .civit.ai tunnel host")
	}

	// 2. vite.config wires the security headers + allowedHosts into the dev server.
	viteCfg := read("vite.config.ts")
	if !strings.Contains(viteCfg, "devServerSecurityHeaders()") {
		t.Errorf("vite.config.ts should wire devServerSecurityHeaders() into server.headers")
	}
	if !strings.Contains(viteCfg, "DEV_ALLOWED_HOSTS") {
		t.Errorf("vite.config.ts should set server.allowedHosts from DEV_ALLOWED_HOSTS")
	}

	// 3. The DEV allowlist carries civitai.com (env.development), and PRODUCTION is
	//    untouched (env.production must NOT gain a localhost/tunnel origin).
	envDev := read(".env.development")
	if !strings.Contains(envDev, "https://civitai.com") {
		t.Errorf(".env.development allowlist should include https://civitai.com:\n%s", envDev)
	}
	envProd := read(".env.production")
	if strings.Contains(envProd, "localhost") {
		t.Errorf(".env.production must NOT be loosened with a localhost origin:\n%s", envProd)
	}

	// 4. A dev:tunnel script exists to serve the embeddable dev server.
	pkg := read("package.json")
	if !strings.Contains(pkg, "dev:tunnel") {
		t.Errorf("package.json should provide a dev:tunnel script:\n%s", pkg)
	}

	// 5. The dev-embed test ships with the scaffold.
	if _, err := os.Stat(filepath.Join(dir, "src", "dev-embed.test.ts")); err != nil {
		t.Errorf("scaffold should ship src/dev-embed.test.ts: %v", err)
	}
}

// TestPageMoneyDevTunnelRoutesHmrOverTunnel asserts the scaffold routes Vite HMR
// over the reverse tunnel (`wss://dev-<hex>.civit.ai:443`) instead of the dev
// server's own `ws://localhost:5186` — which the browser inside the tunneled
// iframe can't reach, so live-reload silently fails on the tunnel. The wiring is:
// the `dev:tunnel` script sets CIVITAI_DEV_TUNNEL_HMR=1, and vite.config reads it
// to conditionally emit `server.ws = { clientPort: 443, protocol: 'wss' }` with
// `host` LEFT UNSET (so the client derives the runtime-minted tunnel host from
// location.hostname). Plain dev/dev:harness/dev:live must NOT force wss:443 (that
// breaks local HMR) — hence the env gate. (Vite 8.1 renamed the websocket options
// from `server.hmr` to `server.ws`; the scaffold pins vite ^8.0.0 → 8.1.x+.)
func TestPageMoneyDevTunnelRoutesHmrOverTunnel(t *testing.T) {
	dir := t.TempDir()
	if _, err := Render(PageMoney, dir, Data{Slug: "my-block", Name: "My Block"}); err != nil {
		t.Fatalf("render page-money: %v", err)
	}

	read := func(rel string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return string(b)
	}

	// 1. dev:tunnel sets the HMR gate env var (so config knows it's tunneling).
	pkg := read("package.json")
	if !strings.Contains(pkg, "CIVITAI_DEV_TUNNEL_HMR=1") {
		t.Errorf("dev:tunnel script should set CIVITAI_DEV_TUNNEL_HMR=1:\n%s", pkg)
	}

	// 2. vite.config gates the tunnel HMR block on that env var and, when set,
	//    emits clientPort:443 + wss on `server.ws` — WITHOUT hardcoding a `ws.host`
	//    (the tunnel host is minted at runtime; the client must fall back to
	//    location.hostname).
	viteCfg := read("vite.config.ts")
	if !strings.Contains(viteCfg, "CIVITAI_DEV_TUNNEL_HMR") {
		t.Errorf("vite.config.ts should gate tunnel HMR on CIVITAI_DEV_TUNNEL_HMR:\n%s", viteCfg)
	}
	if !strings.Contains(viteCfg, "clientPort: 443") {
		t.Errorf("vite.config.ts should set ws.clientPort: 443 for the tunnel")
	}
	if !strings.Contains(viteCfg, "protocol: 'wss'") {
		t.Errorf("vite.config.ts should set ws.protocol: 'wss' for the tunnel")
	}
	// Vite 8.1 renamed the websocket options from `server.hmr` to `server.ws`; the
	// scaffold pins vite ^8.0.0 (→ 8.1.x+) where `server.hmr`'s ws options are
	// deprecated aliases. The tunnel block must use the current `ws:` key.
	if !strings.Contains(viteCfg, "ws: { clientPort: 443, protocol: 'wss' }") {
		t.Errorf("vite.config.ts should route tunnel HMR via `server.ws` (Vite 8.1), not the deprecated `server.hmr` ws options:\n%s", viteCfg)
	}
	// The client MUST derive the (runtime-minted) tunnel host from location.hostname
	// — a hardcoded `ws: { host: ... }` would pin the wrong host and break render.
	if strings.Contains(viteCfg, "ws: { host:") || strings.Contains(viteCfg, "ws:{host:") {
		t.Errorf("vite.config.ts must NOT hardcode ws.host (tunnel host is runtime-minted):\n%s", viteCfg)
	}
}

// TestPageMoneyPlainDevKeepsLocalHmr locks in the "gated OFF" side of the tunnel
// HMR wiring: ONLY `dev:tunnel` may carry CIVITAI_DEV_TUNNEL_HMR. If the flag
// leaked into plain `dev` / `dev:harness` / `dev:live` — or into ANY committed
// `.env` file that `loadEnv(mode, cwd, ”)` reads (it applies `.env*` to EVERY
// mode, plain dev included) — Vite would force wss:443 for local dev too and
// silently break local ws HMR (the browser can't reach `wss://localhost:443`).
// This is the counterpart to TestPageMoneyDevTunnelRoutesHmrOverTunnel, which
// asserts the flag IS present on `dev:tunnel`.
func TestPageMoneyPlainDevKeepsLocalHmr(t *testing.T) {
	dir := t.TempDir()
	if _, err := Render(PageMoney, dir, Data{Slug: "my-block", Name: "My Block"}); err != nil {
		t.Fatalf("render page-money: %v", err)
	}

	read := func(rel string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return string(b)
	}

	const gate = "CIVITAI_DEV_TUNNEL_HMR"

	// Parse scripts so we assert PER-SCRIPT — the flag legitimately lives on
	// `dev:tunnel`, so a whole-file grep can't distinguish a leak from the intended
	// use. Only the three plain-dev scripts must be clean.
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal([]byte(read("package.json")), &pkg); err != nil {
		t.Fatalf("parse package.json: %v", err)
	}
	// Control assertion: dev:tunnel DOES carry the gate (proves the negative checks
	// below aren't passing simply because the flag was dropped everywhere).
	if !strings.Contains(pkg.Scripts["dev:tunnel"], gate) {
		t.Errorf("dev:tunnel script should set %s (control):\n%q", gate, pkg.Scripts["dev:tunnel"])
	}
	for _, name := range []string{"dev", "dev:harness", "dev:live"} {
		script, ok := pkg.Scripts[name]
		if !ok {
			t.Errorf("package.json missing expected %q script", name)
			continue
		}
		if strings.Contains(script, gate) {
			t.Errorf("plain %q script must NOT set %s (would force wss:443 → break local HMR):\n%q", name, gate, script)
		}
	}

	// The gate must NOT appear in ANY committed .env file — loadEnv(mode, cwd, '')
	// applies .env* to EVERY mode incl. plain dev, so a leak here would break local
	// HMR for dev/dev:harness/dev:live regardless of the per-script gate above.
	for _, envFile := range []string{".env.development", ".env.production", ".env.example"} {
		if body := read(envFile); strings.Contains(body, gate) {
			t.Errorf("%s must NOT contain %s (loadEnv applies it to plain dev too):\n%s", envFile, gate, body)
		}
	}
}
