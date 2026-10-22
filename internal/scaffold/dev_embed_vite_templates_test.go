package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// Every template that ships a Vite dev server must serve it embeddably for
// `civitai app dev-tunnel`. page-vite once shipped a vite.config.js with no
// `server` block at all, so Vite's DNS-rebinding check 403'd the tunnel's
// `dev-<hex>.civit.ai` Host and the app never rendered through the tunnel —
// while page-money and page-elements, which carried the config, stayed green
// under guards that only ever iterated those two.
//
// So the set under test here is DERIVED from AllTemplates() — "renders a
// vite.config.*" — never a hand list: a future Vite scaffold that forgets the
// tunnel config fails these tests by existing, instead of being left out of a
// list nobody remembered to extend.

// viteTemplate is a template that renders a Vite config, plus the rendered paths
// the dev-tunnel wiring lives at.
type viteTemplate struct {
	tmpl     Template
	dir      string // the rendered project
	config   string // vite.config.ts | vite.config.js, relative to dir
	devEmbed string // src/dev-embed.ts | src/dev-embed.js, relative to dir
}

// viteConfigNames are the config filenames a rendered template may carry.
var viteConfigNames = []string{"vite.config.ts", "vite.config.js"}

// renderViteTemplates renders every template and returns those that ship a Vite
// config. It enforces a floor so the derivation cannot quietly go vacuous (a
// renamed config file would otherwise drop a template from every guard below).
func renderViteTemplates(t *testing.T) []viteTemplate {
	t.Helper()
	var out []viteTemplate
	for _, tmpl := range AllTemplates() {
		dir := t.TempDir()
		if _, err := Render(tmpl, dir, Data{Slug: "my-block", Name: "My Block"}); err != nil {
			t.Fatalf("render %s: %v", tmpl, err)
		}
		var cfg string
		for _, name := range viteConfigNames {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				if cfg != "" {
					t.Fatalf("%s renders BOTH %s and %s — Vite would load only one; the guard cannot tell which", tmpl, cfg, name)
				}
				cfg = name
			}
		}
		if cfg == "" {
			continue
		}
		// The dev-embed module sits beside the config in the config's language.
		ext := strings.TrimPrefix(filepath.Ext(cfg), ".")
		out = append(out, viteTemplate{tmpl: tmpl, dir: dir, config: cfg, devEmbed: "src/dev-embed." + ext})
	}
	// page-elements, page-money and page-vite. Lower this on purpose only.
	const floor = 3
	if len(out) < floor {
		var names []string
		for _, v := range out {
			names = append(names, string(v.tmpl))
		}
		t.Fatalf("found only %d template(s) rendering a vite.config.* (%v), expected at least %d — "+
			"a template lost its config, or viteConfigNames no longer matches what they render", len(out), names, floor)
	}
	return out
}

func (v viteTemplate) read(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(v.dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("%s: read %s: %v", v.tmpl, rel, err)
	}
	return string(b)
}

// serverBlock returns the body of the `server: { … }` object literal in a Vite
// config, matched by brace depth so a nested `proxy: { … }` does not end it
// early. "" when there is no server block.
func serverBlock(cfg string) string {
	loc := regexp.MustCompile(`(?m)^\s*server:\s*\{`).FindStringIndex(cfg)
	if loc == nil {
		return ""
	}
	depth := 0
	for i := loc[1] - 1; i < len(cfg); i++ {
		switch cfg[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return cfg[loc[1]:i]
			}
		}
	}
	return ""
}

var (
	// `export const DEV_ALLOWED_HOSTS: string[] = [...]` (TS) or
	// `export const DEV_ALLOWED_HOSTS = [...]` (JS).
	reAllowedHosts   = regexp.MustCompile(`DEV_ALLOWED_HOSTS(?:\s*:\s*[^=]+)?\s*=\s*\[([^\]]*)\]`)
	reDevEmbedImport = regexp.MustCompile(
		`(?m)^import\s*\{\s*DEV_ALLOWED_HOSTS,\s*devServerSecurityHeaders,\s*isDevTunnel\s*\}\s*from\s*'\./src/dev-embed\.js';`)
	// The ONE env flag that marks a dev-tunnel session: `npm run dev:tunnel` sets
	// it, and so does the App Assistant sandbox's `CIVITAI_DEV_TUNNEL_HMR=1 npm run
	// dev`. Named for its first use (HMR over the tunnel).
	reDevTunnelEnv = regexp.MustCompile(`DEV_TUNNEL_ENV\s*=\s*'([^']+)'`)
	// The predicate's whole body, so '1' and 'true' both count and nothing else does.
	reIsDevTunnel = regexp.MustCompile(
		`(?s)export function isDevTunnel\(env[^)]*\)[^{]*\{\s*const v = env\[DEV_TUNNEL_ENV\];\s*return v === '1' \|\| v === 'true';\s*\}`)
	reLineComment = regexp.MustCompile(`(?m)^\s*//.*$`)
)

// topLevel returns the parts of an object-literal body that sit at brace depth
// 0, with whole-line `//` comments removed — so a key nested inside `proxy: {…}`
// or inside a conditional spread `...(x ? { headers: … } : {})` is not seen as
// one of the object's own keys.
func topLevel(body string) string {
	body = reLineComment.ReplaceAllString(body, "")
	var b strings.Builder
	depth := 0
	for _, r := range body {
		switch r {
		case '{':
			depth++
			continue
		case '}':
			depth--
			continue
		}
		if depth == 0 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// extractAllowedHosts reads DEV_ALLOWED_HOSTS from a rendered dev-embed module,
// resolving the DEV_TUNNEL_HOST_SUFFIX identifier it references. Any other
// identifier is a fatal — the guard must not guess at a value it cannot read.
func extractAllowedHosts(t *testing.T, src, hostSuffix string) []string {
	t.Helper()
	m := reAllowedHosts.FindStringSubmatch(src)
	if m == nil {
		t.Fatal("could not find DEV_ALLOWED_HOSTS in the dev-embed module")
	}
	var hosts []string
	for _, el := range strings.Split(m[1], ",") {
		el = strings.TrimSpace(el)
		switch {
		case el == "":
		case el == "DEV_TUNNEL_HOST_SUFFIX":
			hosts = append(hosts, hostSuffix)
		case len(el) >= 2 && el[0] == '\'' && el[len(el)-1] == '\'':
			hosts = append(hosts, el[1:len(el)-1])
		default:
			t.Fatalf("DEV_ALLOWED_HOSTS element %q is neither a string literal nor DEV_TUNNEL_HOST_SUFFIX — extend the guard", el)
		}
	}
	return hosts
}

// TestEveryViteTemplateServesTheDevTunnel: every Vite template's dev server
// admits the `.civit.ai` tunnel host and sends the same framing/CORS headers —
// only during a dev-tunnel session — and those values are page-elements' (the
// default template's) exactly. The CI template jobs run the same contract
// against each REAL dev server (scripts/ci/dev-tunnel-embed-probe.sh); this
// test reads the rendered text, so it covers every Vite template without npm.
func TestEveryViteTemplateServesTheDevTunnel(t *testing.T) {
	templates := renderViteTemplates(t)

	// The reference values: page-elements, the default template.
	var ref *viteTemplate
	for i := range templates {
		if templates[i].tmpl == PageElements {
			ref = &templates[i]
		}
	}
	if ref == nil {
		t.Fatal("page-elements no longer renders a vite.config.* — the reference for this guard is gone")
	}
	refSrc := ref.read(t, ref.devEmbed)
	refConsts := extractDevEmbedConstants(t, refSrc)
	refHosts := extractAllowedHosts(t, refSrc, refConsts.hostSuffix)

	// Pin the reference's own values literally, so the cross-template comparison
	// below cannot be satisfied by every template drifting together.
	if refConsts.hostSuffix != ".civit.ai" {
		t.Fatalf("page-elements DEV_TUNNEL_HOST_SUFFIX = %q, want %q", refConsts.hostSuffix, ".civit.ai")
	}
	if want := []string{"localhost", ".civit.ai"}; !reflect.DeepEqual(refHosts, want) {
		t.Fatalf("page-elements DEV_ALLOWED_HOSTS = %q, want %q", refHosts, want)
	}
	if want := "frame-ancestors 'self' https://civitai.com"; refConsts.csp != want {
		t.Fatalf("page-elements Content-Security-Policy = %q, want %q", refConsts.csp, want)
	}
	if refConsts.acao != "*" {
		t.Fatalf("page-elements Access-Control-Allow-Origin = %q, want %q", refConsts.acao, "*")
	}

	for _, v := range templates {
		t.Run(string(v.tmpl), func(t *testing.T) {
			cfg := v.read(t, v.config)

			// 1. The config imports the shared values from the dev-embed module…
			if !reDevEmbedImport.MatchString(cfg) {
				t.Errorf("%s does not import { DEV_ALLOWED_HOSTS, devServerSecurityHeaders, isDevTunnel } from './src/dev-embed.js'", v.config)
			}
			// 2. …and wires them INTO `server` (dev-only). A server block that
			//    exists but omits allowedHosts is the 403 page-vite once shipped.
			srv := serverBlock(cfg)
			if srv == "" {
				t.Fatalf("%s has no `server: { … }` block — Vite will 403 the dev-tunnel host `dev-<hex>.civit.ai`:\n%s", v.config, cfg)
			}
			// 3. The embed headers are sent ONLY in a dev-tunnel session. Sent on a
			//    plain `npm run dev`, `Access-Control-Allow-Origin: *` lets any page
			//    in the browser read the local dev server's source. The session
			//    flag is read through loadEnv(…, '') so a value exported in the
			//    shell (the App Assistant sandbox) counts as well as one in a script.
			for _, re := range []string{
				`(?m)^\s*const env = loadEnv\(mode, process\.cwd\(\), ''\);`,
				`(?m)^\s*const devTunnel = isDevTunnel\(env\);`,
			} {
				if !regexp.MustCompile(re).MatchString(cfg) {
					t.Errorf("%s does not match %s", v.config, re)
				}
			}
			for _, re := range []string{
				`(?m)^\s*allowedHosts: DEV_ALLOWED_HOSTS,`,
				`(?m)^\s*\.\.\.\(devTunnel \? \{ headers: devServerSecurityHeaders\(\) \} : \{\}\),`,
			} {
				if !regexp.MustCompile(re).MatchString(srv) {
					t.Errorf("%s `server` block does not match %s:\n%s", v.config, re, srv)
				}
			}
			if regexp.MustCompile(`(?m)^\s*headers\s*:`).MatchString(topLevel(srv)) {
				t.Errorf("%s sets `server.headers` UNCONDITIONALLY — the embed headers must be sent only when isDevTunnel(env):\n%s", v.config, srv)
			}
			// 4. The session flag: one name, one predicate, and a dev:tunnel script
			//    that sets it while plain `dev` does not.
			embed := v.read(t, v.devEmbed)
			if m := reDevTunnelEnv.FindStringSubmatch(embed); m == nil || m[1] != "CIVITAI_DEV_TUNNEL_HMR" {
				t.Errorf("%s DEV_TUNNEL_ENV = %v, want 'CIVITAI_DEV_TUNNEL_HMR' (the App Assistant sandbox sets exactly that name)", v.devEmbed, m)
			}
			if !reIsDevTunnel.MatchString(embed) {
				t.Errorf("%s has no isDevTunnel(env) of the shared shape (env[DEV_TUNNEL_ENV] is '1' or 'true')", v.devEmbed)
			}
			var pkg struct {
				Scripts map[string]string `json:"scripts"`
			}
			if err := json.Unmarshal([]byte(v.read(t, "package.json")), &pkg); err != nil {
				t.Fatalf("parse package.json: %v", err)
			}
			if !strings.Contains(pkg.Scripts["dev:tunnel"], "CIVITAI_DEV_TUNNEL_HMR=1 ") {
				t.Errorf("package.json dev:tunnel = %q, want it to set CIVITAI_DEV_TUNNEL_HMR=1", pkg.Scripts["dev:tunnel"])
			}
			if strings.Contains(pkg.Scripts["dev"], "CIVITAI_DEV_TUNNEL_HMR") {
				t.Errorf("package.json dev = %q must NOT set CIVITAI_DEV_TUNNEL_HMR", pkg.Scripts["dev"])
			}

			// 5. The module behind those names carries page-elements' values.
			src := v.read(t, v.devEmbed)
			got := extractDevEmbedConstants(t, src)
			if got != refConsts {
				t.Errorf("%s dev-embed values %+v differ from page-elements' %+v", v.devEmbed, got, refConsts)
			}
			if hosts := extractAllowedHosts(t, src, got.hostSuffix); !reflect.DeepEqual(hosts, refHosts) {
				t.Errorf("%s DEV_ALLOWED_HOSTS = %q, want page-elements' %q", v.devEmbed, hosts, refHosts)
			}
			// Never X-Frame-Options: it blocks the cross-origin embed whatever the CSP says.
			if strings.Contains(src, "X-Frame-Options'") || strings.Contains(src, `"X-Frame-Options"`) {
				t.Errorf("%s sets an X-Frame-Options header, which blocks civitai.com from framing the dev server", v.devEmbed)
			}
		})
	}
}

// TestPageViteDevServerAdmitsTunnelHost is the named regression guard for the
// page-vite scaffold specifically: its rendered JS config must admit `.civit.ai`
// hosts and send page-elements' headers. TestEveryViteTemplateServesTheDevTunnel
// covers this too; this one fails with page-vite in its name if page-vite ever
// stops rendering a vite.config.js (which would silently drop it from the
// derived set above).
func TestPageViteDevServerAdmitsTunnelHost(t *testing.T) {
	var vite *viteTemplate
	templates := renderViteTemplates(t)
	for i := range templates {
		if templates[i].tmpl == PageVite {
			vite = &templates[i]
		}
	}
	if vite == nil {
		t.Fatal("page-vite does not render a vite.config.* at all")
	}
	if vite.config != "vite.config.js" || vite.devEmbed != "src/dev-embed.js" {
		t.Fatalf("page-vite renders %s / %s, want the JS form vite.config.js / src/dev-embed.js", vite.config, vite.devEmbed)
	}
	src := vite.read(t, vite.devEmbed)
	c := extractDevEmbedConstants(t, src)
	hosts := extractAllowedHosts(t, src, c.hostSuffix)
	if !reflect.DeepEqual(hosts, []string{"localhost", ".civit.ai"}) {
		t.Errorf("page-vite DEV_ALLOWED_HOSTS = %q, want [localhost .civit.ai]", hosts)
	}
	if c.csp != "frame-ancestors 'self' https://civitai.com" || c.acao != "*" {
		t.Errorf("page-vite headers CSP=%q ACAO=%q, want page-elements' frame-ancestors + `*`", c.csp, c.acao)
	}
	if srv := serverBlock(vite.read(t, vite.config)); !strings.Contains(srv, "allowedHosts: DEV_ALLOWED_HOSTS") ||
		!strings.Contains(srv, "headers: devServerSecurityHeaders()") {
		t.Errorf("page-vite vite.config.js `server` block does not wire allowedHosts + headers:\n%s", srv)
	}
}
