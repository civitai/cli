package scaffold

import (
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
// to conditionally emit `server.hmr = { clientPort: 443, protocol: 'wss' }` with
// `host` LEFT UNSET (so the client derives the runtime-minted tunnel host from
// location.hostname). Plain dev/dev:harness/dev:live must NOT force wss:443 (that
// breaks local HMR) — hence the env gate.
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
	//    emits clientPort:443 + wss — WITHOUT hardcoding an `hmr.host` (the tunnel
	//    host is minted at runtime; the client must fall back to location.hostname).
	viteCfg := read("vite.config.ts")
	if !strings.Contains(viteCfg, "CIVITAI_DEV_TUNNEL_HMR") {
		t.Errorf("vite.config.ts should gate tunnel HMR on CIVITAI_DEV_TUNNEL_HMR:\n%s", viteCfg)
	}
	if !strings.Contains(viteCfg, "clientPort: 443") {
		t.Errorf("vite.config.ts should set hmr.clientPort: 443 for the tunnel")
	}
	if !strings.Contains(viteCfg, "protocol: 'wss'") {
		t.Errorf("vite.config.ts should set hmr.protocol: 'wss' for the tunnel")
	}
	// The client MUST derive the (runtime-minted) tunnel host from location.hostname
	// — a hardcoded `hmr: { host: ... }` would pin the wrong host and break render.
	if strings.Contains(viteCfg, "hmr: { host:") || strings.Contains(viteCfg, "hmr:{host:") {
		t.Errorf("vite.config.ts must NOT hardcode hmr.host (tunnel host is runtime-minted):\n%s", viteCfg)
	}
}
