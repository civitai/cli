package devtunnel

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Embeddability preflight for `civitai app dev-tunnel`.
//
// The tunnel can be perfectly healthy — ssh up, public host serving, local hop
// verified — while the app still never appears, because the failure is in the
// BROWSER: civitai.com/apps/dev/<blockId> iframes the tunneled dev server
// SANDBOXED (`allow-scripts allow-forms`, deliberately no `allow-same-origin`),
// so the child document runs at an opaque "null" origin. Three things silently
// break that embed, none of which the existing probes can see (they read status
// codes only):
//
//  1. no `Access-Control-Allow-Origin` — every ES module fetch from the null
//     origin is CORS-blocked, so NO JavaScript runs at all;
//  2. a `frame-ancestors` CSP (or any `X-Frame-Options`) that forbids
//     civitai.com framing the dev server;
//  3. Vite's DNS-rebinding host check 403ing the tunneled `*.civit.ai` Host
//     before the request ever reaches the app.
//
// Measured against real Vite dev servers (6.4.3 and 8.2.0, identical results):
// a STOCK config answers `GET /@vite/client` with `Origin: null` 200 + `Vary:
// Origin` and NO `Access-Control-Allow-Origin`, and 403s a `dev-*.civit.ai`
// Host. With the scaffold's `devServerSecurityHeaders()` + `DEV_ALLOWED_HOSTS`
// applied, ACAO is `*` and the tunneled Host is 200. So all three conditions are
// observable over one HTTP round-trip from the CLI.
//
// Everything here WARNS and never blocks: one HTTP response cannot rule out a
// reverse proxy, a non-Vite dev server, or a deliberately exotic setup, and
// hard-failing would regress flows that work today.

// ProdParentOrigin is the parent origin that embeds the dev tunnel — the
// `/apps/dev/<blockId>` route iframes the tunneled child from here. Mirrors
// PROD_PARENT_ORIGIN in the page-money scaffold's src/dev-embed.ts (the two are
// pinned together by a drift guard in internal/scaffold).
const ProdParentOrigin = "https://civitai.com"

// tunnelHostSuffix is the host suffix the tunnel serves the dev server on
// (`dev-<16hex>.civit.ai`). Mirrors DEV_TUNNEL_HOST_SUFFIX in the same file.
const tunnelHostSuffix = ".civit.ai"

// probeHostname is a REPRESENTATIVE tunnel host used to exercise the dev
// server's host check before a session exists (the check runs pre-mint, so the
// real `dev-<16hex>` host is not known yet). Vite's allowedHosts matches by
// suffix, so a synthetic label under the same suffix is equivalent for the
// scaffold's `['localhost', '.civit.ai']` config — an app that allowlists one
// EXACT host from a previous session would be a false positive, which is one
// more reason this only ever warns.
const probeHostname = "dev-preflight-probe" + tunnelHostSuffix

// ParentOriginsEnvVar is the Vite env var the SDK's IframeTransport reads to
// decide which parent origins may send BLOCK_INIT.
const ParentOriginsEnvVar = "VITE_BLOCK_ALLOWED_PARENT_ORIGINS"

// FindingKind identifies a check. It is a STABLE identifier: tests assert on the
// kind rather than on message text, so remediation copy can be reworded without
// silently unpinning the guard (the same reason the exit-code contract keys off
// errors.Is sentinels rather than message text).
type FindingKind string

const (
	// FindingCORS: the dev server sends no usable Access-Control-Allow-Origin,
	// so module scripts are blocked for the sandboxed (null-origin) iframe.
	FindingCORS FindingKind = "cors"
	// FindingFrameAncestors: a CSP frame-ancestors directive excludes civitai.com.
	FindingFrameAncestors FindingKind = "frame-ancestors"
	// FindingXFrameOptions: an X-Frame-Options header blocks the cross-origin embed
	// regardless of any CSP.
	FindingXFrameOptions FindingKind = "x-frame-options"
	// FindingAllowedHosts: the dev server rejects the tunneled *.civit.ai Host
	// (Vite's DNS-rebinding protection) before the app is reached.
	FindingAllowedHosts FindingKind = "allowed-hosts"
	// FindingParentOrigins: VITE_BLOCK_ALLOWED_PARENT_ORIGINS is unset or omits
	// the prod parent origin, so IframeTransport rejects the host's BLOCK_INIT.
	FindingParentOrigins FindingKind = "parent-origins"
)

// Finding is one embeddability problem. Evidence (what was observed) and Fix
// (what to do) are SEPARATE because a single vite.config.ts block typically
// resolves three findings at once — CORS, framing and allowedHosts all come from
// the same missing `server` config. Folding the remediation into each finding
// made the real output repeat the same eight-line snippet three times; keeping
// them apart lets the renderer print each distinct fix once. The caller owns
// styling; Summary is a single line, the slices are pre-wrapped lines.
type Finding struct {
	Kind     FindingKind
	Summary  string
	Evidence []string
	Fix      []string
}

// viteConfigFix is the remediation block for a detected Vite dev server. Keep it
// in lockstep with the page-money scaffold (src/dev-embed.ts + vite.config.ts) —
// this is the copy an author with an OLDER app pastes to catch up.
func viteConfigFix() []string {
	return []string{
		"Fix — in vite.config.ts (dev-only; `server.*` never affects `vite build`):",
		"",
		"    server: {",
		"      allowedHosts: ['localhost', '" + tunnelHostSuffix + "'],",
		"      headers: {",
		"        'Access-Control-Allow-Origin': '*',",
		"        'Content-Security-Policy': \"frame-ancestors 'self' " + ProdParentOrigin + "\",",
		"      },",
		"    }",
	}
}

// genericServerFix is the remediation for a dev server we could NOT identify as
// Vite — same requirements, no framework-specific config snippet to offer.
func genericServerFix() []string {
	return []string{
		"Fix — your dev server must, in development, send:",
		"",
		"    Access-Control-Allow-Origin: *",
		"    Content-Security-Policy: frame-ancestors 'self' " + ProdParentOrigin,
		"",
		"  and it must accept requests whose Host is a " + tunnelHostSuffix + " subdomain.",
	}
}

// CheckEmbeddable probes the LOCAL dev server and reports why the host would
// fail to embed it. It returns nil when everything checks out — and nil, too,
// when the server cannot be reached or behaves in a way we cannot interpret:
// probeLocalDevServer already owns "the dev server is down", and a probe that
// cannot observe must not manufacture advice.
//
// The probe dials through DialLocalDevServer, the SAME dialer the tunnel proxy
// uses, so "what the probe measured" is by construction "what the tunnel will
// serve" — including the loopback-family choice when a host listens on only one
// of 127.0.0.1 / ::1 (or, as seen in the wild, when DIFFERENT servers listen on
// each).
func CheckEmbeddable(host string, port int, timeout time.Duration) []Finding {
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return DialLocalDevServer(host, port, timeout)
			},
			DisableKeepAlives: true,
		},
		// A dev server may redirect /; judge the first response we get.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// The baseline Host must be one the dev server ACCEPTS, or its own host check
	// 403s every probe and we misread that as broken CORS. (An early draft used a
	// placeholder hostname here; against a real Vite server that tripped the
	// DNS-rebinding check on every request and manufactured findings for a healthy
	// server.) It is the authority the CLI dials — NOT the Host the tunnel later
	// forwards, which is the `dev-<hex>.civit.ai` name checkAllowedHosts probes
	// with. A dev server whose allowlist admits neither (a hostname --local-host
	// such as a compose service name) answers 403 here, which the 2xx gate below
	// turns into silence rather than into invented advice.
	authority := probeAuthority(host, port)

	// `/@vite/client` is always served by a Vite dev server and is a real ES
	// module, so this is the faithful reproduction of what the sandboxed iframe
	// does — and its presence doubles as Vite detection, which decides whether we
	// can offer a vite.config.ts snippet or only generic advice.
	resp, err := getWithOrigin(client, authority, "/@vite/client", "")
	if err != nil {
		return nil
	}
	isVite := resp.StatusCode == http.StatusOK
	if !isVite {
		// Not Vite (or Vite not serving that path) — fall back to the app root so
		// the header checks still run against something meaningful.
		resp, err = getWithOrigin(client, authority, "/", "")
		if err != nil {
			return nil
		}
	}

	// ONLY a 2xx baseline is interpretable. Anything else means the response we
	// are holding was produced by something OTHER than the app — an authenticating
	// proxy (401), a blanket-deny gate or a host check we already failed (403), a
	// redirect to a login page (302), a broken server (5xx) — and its headers say
	// nothing about how the app would be served. Reading CORS off such a response
	// reported "your modules are CORS-blocked" at servers whose CORS was fine, and
	// reading the follow-up 403 as a host-check failure blamed `allowedHosts` for
	// an authenticating proxy that rejects every request identically. Staying
	// silent is the correct failure mode here: this check exists to end a silent
	// failure, and it would be self-defeating to replace it with confident noise.
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil
	}

	// Capitalised: serverDesc opens a sentence in the finding summary.
	fix := viteConfigFix()
	serverDesc := "Your Vite dev server"
	if !isVite {
		fix = genericServerFix()
		serverDesc = "Your dev server"
	}

	var findings []Finding
	probed := probedPath(isVite)

	if f := checkCORS(resp, probed, serverDesc, fix); f != nil {
		findings = append(findings, *f)
	}
	if f := checkFraming(resp, probed, fix); f != nil {
		findings = append(findings, *f)
	}
	if f := checkAllowedHosts(client, authority, isVite, fix); f != nil {
		findings = append(findings, *f)
	}
	return findings
}

func probedPath(isVite bool) string {
	if isVite {
		return "/@vite/client"
	}
	return "/"
}

// probeAuthority renders the host:port the probe addresses. An empty host means
// the loopback default, which is spelled `localhost` so a dev server's host
// allowlist (Vite's default is localhost + loopback IPs) accepts it.
func probeAuthority(host string, port int) string {
	h := strings.TrimSpace(host)
	if h == "" {
		h = "localhost"
	}
	return net.JoinHostPort(h, fmt.Sprint(port))
}

// getWithOrigin issues a GET carrying `Origin: null` — the origin a document
// inside a sandbox WITHOUT allow-same-origin actually sends. A probe that omits
// Origin is NOT equivalent: a server configured to reflect specific origins can
// answer a no-Origin request differently from a null-origin one, and it is the
// null-origin case that breaks. hostHeader, when set, overrides the Host.
func getWithOrigin(client *http.Client, authority, path, hostHeader string) (*http.Response, error) {
	// The Transport's DialContext ignores the address and always dials the local
	// dev server, so the URL's authority serves only to set the Host header.
	req, err := http.NewRequest(http.MethodGet, "http://"+authority+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Origin", "null")
	req.Header.Set("Sec-Fetch-Dest", "script")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	if hostHeader != "" {
		req.Host = hostHeader
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	// Drain so the connection can be reused/closed cleanly; we only want headers.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	_ = resp.Body.Close()
	return resp, nil
}

// checkCORS reports whether a null-origin module fetch would be allowed. Only
// `*` and an explicit `null` echo work for an opaque origin; anything else
// (typically Vite's default localhost-only policy, which sends NO header at all
// to a null origin) blocks every module script.
func checkCORS(resp *http.Response, probed, serverDesc string, fix []string) *Finding {
	// Case-SENSITIVE on purpose: CORS compares the header against the request's
	// serialised origin byte-for-byte, so `NULL` does not match the `null` origin
	// and the browser blocks the fetch. Accepting it case-insensitively would call
	// a genuinely broken server clean.
	acao := strings.TrimSpace(resp.Header.Get("Access-Control-Allow-Origin"))
	if acao == "*" || acao == "null" {
		return nil
	}
	observed := "no Access-Control-Allow-Origin header"
	if acao != "" {
		observed = fmt.Sprintf("Access-Control-Allow-Origin: %s", acao)
	}
	return &Finding{
		Kind:    FindingCORS,
		Summary: fmt.Sprintf("%s will not load in the Civitai host — its modules are CORS-blocked.", serverDesc),
		Evidence: []string{
			fmt.Sprintf("  GET %s  (Origin: null)  →  %d, %s", probed, resp.StatusCode, observed),
			"",
			"  The host iframes your app sandboxed, so it runs at an opaque \"null\"",
			"  origin. Without a wildcard CORS header every ES module fetch is blocked",
			"  and no JavaScript runs — the iframe stays blank and the host reports",
			"  \"This app didn't load in time\".",
		},
		Fix: fix,
	}
}

// checkFraming reports headers that forbid civitai.com framing the dev server.
// An ABSENT CSP is fine — no CSP means no framing restriction — so this warns
// only when a frame-ancestors directive is present AND excludes the parent.
//
// Order matters and is NOT interchangeable: CSP Level 3 says frame-ancestors
// OBSOLETES X-Frame-Options, and that when a resource carries both, browsers
// enforce frame-ancestors and IGNORE X-Frame-Options. So a permissive
// frame-ancestors makes any XFO irrelevant, and reporting XFO anyway warns at a
// server that embeds perfectly well. XFO is therefore only consulted when NO
// frame-ancestors directive is present at all.
//
// Every Content-Security-Policy header is evaluated, not just the first: a
// response may carry several, they combine RESTRICTIVELY, and Header.Get would
// see only one — so a second policy saying `frame-ancestors 'none'` used to be
// reported clean.
func checkFraming(resp *http.Response, probed string, fix []string) *Finding {
	sawFrameAncestors := false
	for _, csp := range resp.Header.Values("Content-Security-Policy") {
		ancestors, present := frameAncestors(csp)
		if !present {
			continue
		}
		sawFrameAncestors = true
		if frameAncestorsAllow(ancestors) {
			continue
		}
		return &Finding{
			Kind:    FindingFrameAncestors,
			Summary: "Your dev server's CSP forbids the Civitai host from framing it.",
			Evidence: []string{
				fmt.Sprintf("  GET %s  →  Content-Security-Policy: frame-ancestors %s", probed, strings.Join(ancestors, " ")),
				"",
				fmt.Sprintf("  That directive does not admit %s, so the browser refuses to render", ProdParentOrigin),
				"  the iframe at all.",
			},
			Fix: fix,
		}
	}
	if sawFrameAncestors {
		// A frame-ancestors that admits the parent is the enforced policy; any XFO
		// alongside it is ignored by the browser, so there is nothing to report.
		return nil
	}

	if xfo := strings.TrimSpace(resp.Header.Get("X-Frame-Options")); xfo != "" {
		return &Finding{
			Kind:    FindingXFrameOptions,
			Summary: "Your dev server sends X-Frame-Options, which blocks the Civitai host from framing it.",
			Evidence: []string{
				fmt.Sprintf("  GET %s  →  X-Frame-Options: %s", probed, xfo),
				"",
				"  X-Frame-Options cannot express a cross-origin allowlist, and this",
				"  response sets no frame-ancestors policy to supersede it.",
			},
			Fix: fix,
		}
	}
	return nil
}

// frameAncestors extracts the frame-ancestors source list from a CSP header,
// reporting whether the directive was present at all. CSP directive names are
// case-insensitive and separated by ';'.
func frameAncestors(csp string) (sources []string, present bool) {
	for _, directive := range strings.Split(csp, ";") {
		fields := strings.Fields(strings.TrimSpace(directive))
		if len(fields) == 0 {
			continue
		}
		if !strings.EqualFold(fields[0], "frame-ancestors") {
			continue
		}
		return fields[1:], true
	}
	return nil, false
}

// frameAncestorsAllow reports whether a frame-ancestors source list admits the
// prod parent. Deliberately GENEROUS — it answers "could this plausibly allow
// civitai.com?", because a false warning on an exotic-but-working CSP is worse
// than a missed one (the browser console still shows a real violation). A
// `'none'` list is the one unambiguous rejection.
func frameAncestorsAllow(sources []string) bool {
	nonEmpty := make([]string, 0, len(sources))
	for _, src := range sources {
		if s := strings.TrimSpace(src); s != "" {
			nonEmpty = append(nonEmpty, s)
		}
	}
	if len(nonEmpty) == 0 {
		// `frame-ancestors` with no sources matches nothing — equivalent to 'none'.
		return false
	}
	// `'none'` is decisive ONLY as the sole source. The CSP grammar does not admit
	// it alongside other sources, so a browser parsing `'none' https://civitai.com`
	// ignores the invalid `'none'` and honours the rest — treating it as decisive
	// there would warn at a policy that actually permits the embed.
	if len(nonEmpty) == 1 && strings.EqualFold(nonEmpty[0], "'none'") {
		return false
	}
	for _, s := range nonEmpty {
		if s == "*" || strings.EqualFold(s, "https:") {
			return true
		}
		if hostSourceAllows(s, ProdParentOrigin) {
			return true
		}
	}
	return false
}

// hostSourceAllows reports whether one CSP host-source admits origin. Handles the
// exact origin, a scheme-less host, and a `*.` wildcard prefix.
func hostSourceAllows(source, origin string) bool {
	// Scheme and host are both case-insensitive in a CSP source expression, so
	// normalise BEFORE stripping — a case-sensitive TrimPrefix leaves an uppercase
	// `HTTPS://` attached and turns a valid source into a spurious warning.
	originHost := stripScheme(strings.ToLower(strings.TrimSpace(origin)))
	host := stripScheme(strings.ToLower(strings.TrimSuffix(strings.TrimSpace(source), "/")))

	if host == originHost {
		return true
	}
	// `*.example.com` matches SUBDOMAINS of example.com — not example.com itself.
	// The leading "*" is dropped rather than the whole "*.", keeping the dot so
	// `*.civitai.com` cannot match `evilcivitai.com`.
	if strings.HasPrefix(host, "*.") {
		return strings.HasSuffix(originHost, host[1:])
	}
	return false
}

// stripScheme removes an http(s):// prefix from an already-lowercased value, then
// drops an explicit DEFAULT port. `https://civitai.com:443` and
// `https://civitai.com` denote the same origin, so a source spelling the port
// out must not read as a different host. (An http:// source is accepted for an
// https origin because CSP3 upgrades http sources to their https equivalent.)
func stripScheme(v string) string {
	v = strings.TrimPrefix(strings.TrimPrefix(v, "https://"), "http://")
	v = strings.TrimSuffix(v, ":443")
	return strings.TrimSuffix(v, ":80")
}

// checkAllowedHosts reports a dev server that rejects the tunneled Host outright.
// Vite's DNS-rebinding protection answers an unlisted Host with 403 BEFORE the
// request reaches the app, so the browser gets an error page rather than the
// bundle. Only a 403 counts: any other status means the host check passed (or
// the server has none), and a transport error means we could not observe.
func checkAllowedHosts(client *http.Client, authority string, isVite bool, fix []string) *Finding {
	resp, err := getWithOrigin(client, authority, probedPath(isVite), probeHostname)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		return nil
	}
	return &Finding{
		Kind:    FindingAllowedHosts,
		Summary: "Your dev server rejects the tunnel's hostname, so requests never reach your app.",
		Evidence: []string{
			fmt.Sprintf("  GET %s  (Host: %s)  →  403", probedPath(isVite), probeHostname),
			"",
			fmt.Sprintf("  The tunnel serves your dev server as a %s subdomain. Vite's", tunnelHostSuffix),
			"  DNS-rebinding host check 403s any hostname not in `server.allowedHosts`,",
			"  before your app sees the request.",
		},
		Fix: fix,
	}
}

// CheckParentOrigins reports a missing/incomplete VITE_BLOCK_ALLOWED_PARENT_ORIGINS
// in dir. Unlike CheckEmbeddable this CANNOT be observed over HTTP — Vite inlines
// `import.meta.env.*` into the bundle at transform time — so it reads what Vite
// itself reads. That makes it a HEURISTIC, and it is gated hard to keep it quiet
// when it cannot be confident: dir must hold both a block manifest AND a
// package.json depending on the App SDK. `dev-tunnel` can be run with an explicit
// blockId from anywhere, so an unrelated CWD must produce no advice.
func CheckParentOrigins(dir string) []Finding {
	if !isSDKAppDir(dir) {
		return nil
	}
	value, found := resolveViteEnv(dir, ParentOriginsEnvVar)
	if found && originsInclude(value, ProdParentOrigin) {
		return nil
	}

	observed := fmt.Sprintf("%s is not set", ParentOriginsEnvVar)
	if found {
		observed = fmt.Sprintf("%s=%s — no %s", ParentOriginsEnvVar, value, ProdParentOrigin)
	}
	suggested := ProdParentOrigin
	if found && strings.TrimSpace(value) != "" {
		suggested = strings.TrimSpace(value) + "," + ProdParentOrigin
	}
	return []Finding{{
		Kind:    FindingParentOrigins,
		Summary: "Your app will reject the host's handshake — no allowed parent origin.",
		Evidence: []string{
			"  " + observed,
			"",
			"  The App SDK's IframeTransport drops any postMessage whose origin is not",
			fmt.Sprintf("  allowlisted, so BLOCK_INIT from %s is ignored and your app", ProdParentOrigin),
			"  never signals ready. The error is swallowed by the error boundary, so",
			"  nothing appears in the terminal.",
		},
		Fix: []string{
			"Fix — in .env.development:",
			"",
			"    " + ParentOriginsEnvVar + "=" + suggested,
		},
	}}
}

// isSDKAppDir reports whether dir looks like an App project built on the SDK —
// the only shape for which the parent-origins advice is meaningful.
func isSDKAppDir(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "block.manifest.json")); err != nil {
		return false
	}
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return false
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return false
	}
	const sdk = "@civitai/app-sdk"
	if _, ok := pkg.Dependencies[sdk]; ok {
		return true
	}
	_, ok := pkg.DevDependencies[sdk]
	return ok
}

// viteEnvFiles is Vite's dev-mode env file order, LOWEST precedence first. A
// later file overrides an earlier one; see resolveViteEnv for the process-env
// rule that sits above all of them.
var viteEnvFiles = []string{".env", ".env.local", ".env.development", ".env.development.local"}

// resolveViteEnv mirrors how Vite resolves a VITE_-prefixed variable in dev.
//
// This is a deliberate VENDORED MIRROR of another project's behaviour (see
// AGENTS.md). Two rules matter and both are load-bearing: `.env.<mode>` beats
// plain `.env`, and a REAL process environment variable beats every file —
// dotenv does not overwrite variables that already exist. Getting the second one
// backwards would warn at an author who exported the value in their shell.
func resolveViteEnv(dir, key string) (string, bool) {
	if v, ok := os.LookupEnv(key); ok {
		return v, true
	}
	value := ""
	found := false
	for _, name := range viteEnvFiles {
		vars, err := parseDotEnv(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if v, ok := vars[key]; ok {
			value, found = v, true
		}
	}
	return value, found
}

// parseDotEnv reads the dotenv syntax Vite actually accepts. Each rule below was
// verified against real `dotenv` (17.4.2) rather than assumed — the first draft
// got four of them wrong, and every one produced a FALSE WARNING at a correctly
// configured project, which is the worst failure mode for advisory output:
//
//   - `KEY=value`. NOT `KEY: value` — measured against Vite's own loadEnv, the
//     colon form resolves to NOTHING, so honouring it would make this checker
//     find a value the app will never see and stay silent on a broken project;
//   - an optional `export ` prefix;
//   - single, double AND BACKTICK quoted values (backticks are dotenv ≥16);
//   - for UNQUOTED values, `#` starts a comment ANYWHERE, not only after a space
//     (dotenv's capture is `[^#\r\n]+`); inside quotes a `#` is literal.
//
// `${VAR}` interpolation is applied separately by expandDotEnv — Vite runs
// dotenv-expand over the parsed result.
func parseDotEnv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")

		key, rest, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if key = strings.TrimSpace(key); key == "" {
			continue
		}
		out[key] = unquoteEnvValue(strings.TrimSpace(rest))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return expandDotEnv(out), nil
}

func unquoteEnvValue(v string) string {
	if len(v) >= 2 {
		q := v[0]
		if (q == '"' || q == '\'' || q == '`') && v[len(v)-1] == q {
			// Quoted: the value is literal, `#` included.
			return v[1 : len(v)-1]
		}
	}
	// Unquoted: `#` begins a comment wherever it appears.
	if i := strings.IndexByte(v, '#'); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}

// expandDotEnv applies the `${VAR}` / `$VAR` interpolation dotenv-expand performs
// (Vite runs it over every parsed env file), resolving against the file's own
// values first and the process environment second. Bounded to a few passes so a
// self-referential file cannot loop; an unresolvable reference expands to the
// empty string, matching dotenv-expand.
func expandDotEnv(vars map[string]string) map[string]string {
	lookup := func(name string) (string, bool) {
		if v, ok := vars[name]; ok {
			return v, true
		}
		return os.LookupEnv(name)
	}
	for pass := 0; pass < 8; pass++ {
		changed := false
		for k, v := range vars {
			expanded := expandEnvRefs(v, lookup)
			if expanded != v {
				vars[k] = expanded
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return vars
}

// expandEnvRefs substitutes `${NAME}` and `$NAME` references using lookup.
func expandEnvRefs(v string, lookup func(string) (string, bool)) string {
	var b strings.Builder
	for i := 0; i < len(v); {
		if v[i] != '$' {
			b.WriteByte(v[i])
			i++
			continue
		}
		// `\$` escapes the sigil.
		if i > 0 && v[i-1] == '\\' {
			b.WriteByte(v[i])
			i++
			continue
		}
		name, next, ok := readEnvRef(v, i)
		if !ok {
			b.WriteByte(v[i])
			i++
			continue
		}
		// An UNRESOLVED reference expands to the empty string, not to its own text
		// — measured against Vite's loadEnv, which resolves `${NOPE}` to "".
		if val, found := lookup(name); found {
			b.WriteString(val)
		}
		i = next
	}
	return b.String()
}

// readEnvRef reads a `${NAME}` or `$NAME` reference starting at v[i] == '$'.
func readEnvRef(v string, i int) (name string, next int, ok bool) {
	j := i + 1
	if j < len(v) && v[j] == '{' {
		end := strings.IndexByte(v[j:], '}')
		if end < 0 {
			return "", 0, false
		}
		return v[j+1 : j+end], j + end + 1, true
	}
	start := j
	for j < len(v) && (v[j] == '_' ||
		(v[j] >= 'A' && v[j] <= 'Z') || (v[j] >= 'a' && v[j] <= 'z') ||
		(j > start && v[j] >= '0' && v[j] <= '9')) {
		j++
	}
	if j == start {
		return "", 0, false
	}
	return v[start:j], j, true
}

// originsInclude reports whether a comma-separated origin allowlist contains
// origin. Entries are trimmed and compared case-insensitively; a trailing slash
// is tolerated because authors write it and the SDK normalises it away.
func originsInclude(list, origin string) bool {
	for _, entry := range strings.Split(list, ",") {
		e := strings.TrimSuffix(strings.TrimSpace(entry), "/")
		if strings.EqualFold(e, origin) {
			return true
		}
	}
	return false
}
