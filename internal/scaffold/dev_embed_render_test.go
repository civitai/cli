package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// devEmbedTemplates are the templates whose dev server `civitai app dev-tunnel`
// must be able to embed: both SDK templates, page-elements being the default.
// Every guard in this file and in dev_embed_contract_test.go runs over each.
var devEmbedTemplates = []Template{PageMoney, PageElements}

// devEmbedTestPath is where each template's vitest suite for src/dev-embed.ts
// lives (page-elements keeps every test under test/).
var devEmbedTestPath = map[Template]string{
	PageMoney:    "src/dev-embed.test.ts",
	PageElements: "test/dev-embed.test.ts",
}

// TestPageElementsSharesPageMoneyDevEmbed is the drift guard between the two
// copies. The template system renders each template from its own directory, so
// page-elements carries its own src/dev-embed.ts — and it must be BYTE-IDENTICAL
// to page-money's, the single source the CLI's preflight (internal/devtunnel)
// is kept in step with. Its vitest suite must match page-money's too, except the
// one import line its test/ location changes. The embed WIRING in vite.config.ts
// is pinned per template by the tests below.
func TestPageElementsSharesPageMoneyDevEmbed(t *testing.T) {
	read := func(p string) string {
		t.Helper()
		b, err := templatesFS.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		return string(b)
	}
	money := read("templates/page-money/src/dev-embed.ts.tmpl")
	if got := read("templates/page-elements/src/dev-embed.ts.tmpl"); got != money {
		t.Errorf("templates/page-elements/src/dev-embed.ts.tmpl has drifted from page-money's copy — " +
			"make it byte-identical (the CLI's dev-tunnel preflight is kept in step with that one source)")
	}
	moneyTest := read("templates/page-money/src/dev-embed.test.ts.tmpl")
	elTest := read("templates/page-elements/test/dev-embed.test.ts.tmpl")
	const moneyImport, elImport = "} from './dev-embed.js';", "} from '../src/dev-embed.js';"
	if !strings.Contains(moneyTest, moneyImport) {
		t.Fatalf("CONTROL failure: page-money's dev-embed test no longer imports %q", moneyImport)
	}
	if strings.Replace(elTest, elImport, moneyImport, 1) != moneyTest {
		t.Errorf("templates/page-elements/test/dev-embed.test.ts.tmpl has drifted from page-money's " +
			"src/dev-embed.test.ts.tmpl beyond the import path")
	}
	// The same constants reach the same Vite options in both configs.
	for _, tmpl := range devEmbedTemplates {
		cfg := read("templates/" + string(tmpl) + "/vite.config.ts.tmpl")
		for _, re := range []string{
			`(?m)^\s*allowedHosts: DEV_ALLOWED_HOSTS,`,
			`(?m)^\s*headers: devServerSecurityHeaders\(\),`,
			`\.\.\.\(tunnelHmr \? \{ ws: \{ clientPort: 443, protocol: 'wss' \} \} : \{\}\),`,
			`(?m)^\s*port: 5186,`,
			`(?m)^\s*strictPort: true,`,
		} {
			if !regexp.MustCompile(re).MatchString(cfg) {
				t.Errorf("%s vite.config.ts does not match %s", tmpl, re)
			}
		}
	}
}

// TestSDKTemplatesEmitEmbeddableDevServer asserts each scaffolded SDK project
// ships an iframe-embeddable dev server for `civitai app dev-tunnel`: the dev
// server sends a `frame-ancestors https://civitai.com` CSP, does NOT set
// X-Frame-Options, admits the `.civit.ai` tunnel host, and the block's dev
// parent-origin allowlist includes https://civitai.com — WITHOUT touching the
// production build. This is the Go-verifiable companion to the scaffold's own
// src/dev-embed.test.ts (which the scaffolded user's `vitest run` exercises).
func TestSDKTemplatesEmitEmbeddableDevServer(t *testing.T) {
	for _, tmpl := range devEmbedTemplates {
		t.Run(string(tmpl), func(t *testing.T) {
			dir := t.TempDir()
			if _, err := Render(tmpl, dir, Data{Slug: "my-block", Name: "My Block"}); err != nil {
				t.Fatalf("render %s: %v", tmpl, err)
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
			// page-elements ships no .env.production: its production allowlist is the
			// platform-injected value. Where the file exists it must stay un-loosened.
			if _, err := os.Stat(filepath.Join(dir, ".env.production")); err == nil {
				if envProd := read(".env.production"); strings.Contains(envProd, "localhost") {
					t.Errorf(".env.production must NOT be loosened with a localhost origin:\n%s", envProd)
				}
			}

			// 4. A dev:tunnel script exists to serve the embeddable dev server.
			pkg := read("package.json")
			if !strings.Contains(pkg, "dev:tunnel") {
				t.Errorf("package.json should provide a dev:tunnel script:\n%s", pkg)
			}

			// 5. The dev-embed test ships with the scaffold.
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(devEmbedTestPath[tmpl]))); err != nil {
				t.Errorf("scaffold should ship %s: %v", devEmbedTestPath[tmpl], err)
			}
		})
	}
}

// TestSDKTemplatesRouteTunnelHmrOverTunnel asserts the scaffold routes Vite HMR
// over the reverse tunnel (`wss://dev-<hex>.civit.ai:443`) instead of the dev
// server's own `ws://localhost:5186` — which the browser inside the tunneled
// iframe can't reach, so live-reload silently fails on the tunnel. The wiring is:
// the `dev:tunnel` script sets CIVITAI_DEV_TUNNEL_HMR=1, and vite.config reads it
// to conditionally emit `server.ws = { clientPort: 443, protocol: 'wss' }` with
// `host` LEFT UNSET (so the client derives the runtime-minted tunnel host from
// location.hostname). Plain dev/dev:harness/dev:live must NOT force wss:443 (that
// breaks local HMR) — hence the env gate. (Vite 8.1 renamed the websocket options
// from `server.hmr` to `server.ws`; the scaffold pins vite ^8.0.0 → 8.1.x+.)
func TestSDKTemplatesRouteTunnelHmrOverTunnel(t *testing.T) {
	for _, tmpl := range devEmbedTemplates {
		t.Run(string(tmpl), func(t *testing.T) {
			dir := t.TempDir()
			if _, err := Render(tmpl, dir, Data{Slug: "my-block", Name: "My Block"}); err != nil {
				t.Fatalf("render %s: %v", tmpl, err)
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
		})
	}
}

// TestSDKTemplatesPlainDevKeepsLocalHmr locks in the "gated OFF" side of the tunnel
// HMR wiring: ONLY `dev:tunnel` may carry CIVITAI_DEV_TUNNEL_HMR. If the flag
// leaked into plain `dev` / `dev:harness` / `dev:live` — or into ANY committed
// `.env` file that `loadEnv(mode, cwd, ”)` reads (it applies `.env*` to EVERY
// mode, plain dev included) — Vite would force wss:443 for local dev too and
// silently break local ws HMR (the browser can't reach `wss://localhost:443`).
// This is the counterpart to TestSDKTemplatesRouteTunnelHmrOverTunnel, which
// asserts the flag IS present on `dev:tunnel`.
func TestSDKTemplatesPlainDevKeepsLocalHmr(t *testing.T) {
	for _, tmpl := range devEmbedTemplates {
		t.Run(string(tmpl), func(t *testing.T) {
			dir := t.TempDir()
			if _, err := Render(tmpl, dir, Data{Slug: "my-block", Name: "My Block"}); err != nil {
				t.Fatalf("render %s: %v", tmpl, err)
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
					// dev:live is page-money's alone (its live host is React-only).
					if name != "dev:live" || tmpl == PageMoney {
						t.Errorf("package.json missing expected %q script", name)
					}
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
				if _, err := os.Stat(filepath.Join(dir, envFile)); err != nil && envFile == ".env.production" && tmpl != PageMoney {
					continue // page-elements ships no .env.production
				}
				if body := read(envFile); strings.Contains(body, gate) {
					t.Errorf("%s must NOT contain %s (loadEnv applies it to plain dev too):\n%s", envFile, gate, body)
				}
			}
		})
	}
}
