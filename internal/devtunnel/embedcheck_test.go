package devtunnel

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// The behaviour pinned here was MEASURED against real Vite dev servers (6.4.3
// and 8.2.0 — identical on both) before any of it was written:
//
//	stock config,  GET /@vite/client, Origin: null  -> 200, Vary: Origin, NO ACAO
//	stock config,  Host: dev-*.civit.ai             -> 403
//	scaffold hdrs, GET /@vite/client, Origin: null  -> 200, ACAO: *, CSP frame-ancestors
//	scaffold hdrs, Host: dev-*.civit.ai             -> 200
//
// stockViteHandler and scaffoldViteHandler below reproduce exactly those two
// observations, and serve as the negative and positive controls for the whole
// check: the negative proves the harness can go red, the positive proves it can
// see a clean server — so a zero-finding result is a real zero and not a probe
// wired to nothing.

const probeTimeout = 2 * time.Second

// startProbeServer runs h on loopback and returns the host/port to probe, so the
// tests exercise the REAL DialLocalDevServer path rather than a stubbed dialer.
func startProbeServer(t *testing.T, h http.Handler) (string, int) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("split %s: %v", srv.URL, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("port %s: %v", portStr, err)
	}
	return host, port
}

// stockViteHandler mimics a DEFAULT Vite dev server: it serves /@vite/client,
// echoes ACAO only for loopback origins (so a null origin gets none), and 403s
// any Host that is not localhost.
func stockViteHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Host, "localhost") && !strings.HasPrefix(r.Host, "127.0.0.1") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Vary", "Origin")
		if origin := r.Header.Get("Origin"); strings.HasPrefix(origin, "http://localhost") {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.WriteHeader(http.StatusOK)
	})
}

// scaffoldViteHandler mimics a Vite dev server configured with the page-money
// scaffold's devServerSecurityHeaders() + DEV_ALLOWED_HOSTS.
func scaffoldViteHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Host, "localhost") &&
			!strings.HasPrefix(r.Host, "127.0.0.1") &&
			!strings.HasSuffix(hostOnly(r.Host), tunnelHostSuffix) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'self' "+ProdParentOrigin)
		w.WriteHeader(http.StatusOK)
	})
}

func hostOnly(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		return host
	}
	return h
}

func kinds(findings []Finding) []FindingKind {
	out := make([]FindingKind, 0, len(findings))
	for _, f := range findings {
		out = append(out, f.Kind)
	}
	return out
}

func hasKind(findings []Finding, k FindingKind) bool {
	for _, f := range findings {
		if f.Kind == k {
			return true
		}
	}
	return false
}

// TestCheckEmbeddableControls is the control PAIR, reported together: a stock
// server must produce findings and a scaffold-configured one must produce none.
// Either half alone is uninformative — the negative could pass with a check that
// always fires, the positive with a check wired to nothing.
func TestCheckEmbeddableControls(t *testing.T) {
	t.Run("negative control: stock vite is flagged", func(t *testing.T) {
		host, port := startProbeServer(t, stockViteHandler())
		got := CheckEmbeddable(host, port, probeTimeout)
		if !hasKind(got, FindingCORS) {
			t.Fatalf("stock vite must produce a %q finding; got %v", FindingCORS, kinds(got))
		}
		if !hasKind(got, FindingAllowedHosts) {
			t.Fatalf("stock vite must produce an %q finding; got %v", FindingAllowedHosts, kinds(got))
		}
	})

	t.Run("positive control: scaffold-configured vite is clean", func(t *testing.T) {
		host, port := startProbeServer(t, scaffoldViteHandler())
		if got := CheckEmbeddable(host, port, probeTimeout); len(got) != 0 {
			t.Fatalf("scaffold-configured vite must produce NO findings; got %v", kinds(got))
		}
	})
}

// TestCheckEmbeddableSendsNullOrigin pins the request shape. A probe that omits
// Origin is not equivalent — it is the null-origin case that breaks, and a
// server reflecting specific origins answers the two differently.
func TestCheckEmbeddableSendsNullOrigin(t *testing.T) {
	var sawOrigin string
	host, port := startProbeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/@vite/client" {
			sawOrigin = r.Header.Get("Origin")
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
	}))
	CheckEmbeddable(host, port, probeTimeout)
	if sawOrigin != "null" {
		t.Fatalf("probe must send Origin: null (the sandboxed iframe's origin); sent %q", sawOrigin)
	}
}

// TestCheckEmbeddableProbesTunnelHost pins that the host check actually exercises
// a tunnel-shaped Host, not the loopback one.
func TestCheckEmbeddableProbesTunnelHost(t *testing.T) {
	var hosts []string
	host, port := startProbeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hosts = append(hosts, r.Host)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
	}))
	CheckEmbeddable(host, port, probeTimeout)

	found := false
	for _, h := range hosts {
		if strings.HasSuffix(h, tunnelHostSuffix) {
			found = true
		}
	}
	if !found {
		t.Fatalf("probe must send a %s Host to exercise allowedHosts; sent %v", tunnelHostSuffix, hosts)
	}
}

func TestCheckEmbeddableCORS(t *testing.T) {
	tests := []struct {
		name     string
		acao     string // "" = header omitted
		wantCORS bool
	}{
		{name: "absent header blocks the null origin", acao: "", wantCORS: true},
		{name: "wildcard allows it", acao: "*", wantCORS: false},
		{name: "explicit null echo allows it", acao: "null", wantCORS: false},
		// CORS compares the header against the serialised origin byte-for-byte, so
		// `NULL` does NOT match `null` and the browser blocks the fetch.
		{name: "uppercase NULL echo does NOT allow it", acao: "NULL", wantCORS: true},
		{name: "specific localhost origin does not", acao: "http://localhost:5186", wantCORS: true},
		{name: "the prod parent origin does not help a null-origin fetch", acao: ProdParentOrigin, wantCORS: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			acao := tc.acao
			host, port := startProbeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if acao != "" {
					w.Header().Set("Access-Control-Allow-Origin", acao)
				}
				w.WriteHeader(http.StatusOK)
			}))
			got := CheckEmbeddable(host, port, probeTimeout)
			if hasKind(got, FindingCORS) != tc.wantCORS {
				t.Fatalf("acao=%q: want cors finding=%v, got kinds %v", tc.acao, tc.wantCORS, kinds(got))
			}
		})
	}
}

func TestCheckEmbeddableFraming(t *testing.T) {
	tests := []struct {
		name     string
		csp      string
		xfo      string
		wantKind FindingKind // "" = no framing finding
	}{
		{name: "no framing headers is fine", csp: "", xfo: ""},
		{name: "scaffold CSP is fine", csp: "frame-ancestors 'self' " + ProdParentOrigin},
		{name: "wildcard sources fine", csp: "frame-ancestors *"},
		{name: "https: scheme source fine", csp: "frame-ancestors https:"},
		// Per CSP, `*.civitai.com` matches SUBDOMAINS only — it does not match the
		// bare civitai.com, which is the origin that actually frames the tunnel. So
		// this is a genuine misconfiguration and must be flagged.
		{name: "wildcard subdomain does not cover the bare parent", csp: "frame-ancestors *.civitai.com", wantKind: FindingFrameAncestors},
		{name: "host without scheme fine", csp: "frame-ancestors civitai.com"},
		{name: "other directives ignored", csp: "default-src 'self'; script-src 'unsafe-inline'"},
		{name: "case-insensitive directive name", csp: "FRAME-ANCESTORS " + ProdParentOrigin},
		{name: "self only excludes the parent", csp: "frame-ancestors 'self'", wantKind: FindingFrameAncestors},
		{name: "none excludes the parent", csp: "frame-ancestors 'none'", wantKind: FindingFrameAncestors},
		{name: "a different origin excludes the parent", csp: "frame-ancestors https://example.com", wantKind: FindingFrameAncestors},
		{name: "empty source list is none", csp: "default-src 'self'; frame-ancestors", wantKind: FindingFrameAncestors},
		{name: "XFO DENY blocks the embed", xfo: "DENY", wantKind: FindingXFrameOptions},
		{name: "XFO SAMEORIGIN blocks the embed", xfo: "SAMEORIGIN", wantKind: FindingXFrameOptions},
		// CSP L3: frame-ancestors OBSOLETES X-Frame-Options. When both are present
		// browsers enforce frame-ancestors and ignore XFO, so a permissive
		// frame-ancestors means the embed works and there is nothing to report.
		// This case previously asserted the opposite and pinned a false positive.
		{
			name: "a permissive frame-ancestors supersedes XFO",
			csp:  "frame-ancestors " + ProdParentOrigin,
			xfo:  "DENY",
		},
		{
			name:     "XFO still reported when no frame-ancestors is present",
			csp:      "default-src 'self'",
			xfo:      "DENY",
			wantKind: FindingXFrameOptions,
		},
		{
			// Both bad: frame-ancestors is the enforced one, so report THAT.
			name:     "a restrictive frame-ancestors is reported over XFO",
			csp:      "frame-ancestors 'self'",
			xfo:      "DENY",
			wantKind: FindingFrameAncestors,
		},
		{name: "explicit default port matches", csp: "frame-ancestors https://civitai.com:443"},
		{name: "http source matches an https origin (CSP3 upgrade)", csp: "frame-ancestors http://civitai.com"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			csp, xfo := tc.csp, tc.xfo
			host, port := startProbeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Wildcard CORS so only the framing verdict is under test.
				w.Header().Set("Access-Control-Allow-Origin", "*")
				if csp != "" {
					w.Header().Set("Content-Security-Policy", csp)
				}
				if xfo != "" {
					w.Header().Set("X-Frame-Options", xfo)
				}
				w.WriteHeader(http.StatusOK)
			}))
			got := CheckEmbeddable(host, port, probeTimeout)

			if tc.wantKind == "" {
				if hasKind(got, FindingFrameAncestors) || hasKind(got, FindingXFrameOptions) {
					t.Fatalf("csp=%q xfo=%q: want no framing finding, got %v", tc.csp, tc.xfo, kinds(got))
				}
				return
			}
			if !hasKind(got, tc.wantKind) {
				t.Fatalf("csp=%q xfo=%q: want %q, got %v", tc.csp, tc.xfo, tc.wantKind, kinds(got))
			}
		})
	}
}

func TestCheckEmbeddableAllowedHosts(t *testing.T) {
	t.Run("403 on the tunnel host is flagged", func(t *testing.T) {
		host, port := startProbeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			if strings.HasSuffix(hostOnly(r.Host), tunnelHostSuffix) {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		got := CheckEmbeddable(host, port, probeTimeout)
		if !hasKind(got, FindingAllowedHosts) {
			t.Fatalf("want %q, got %v", FindingAllowedHosts, kinds(got))
		}
	})

	// Only 403 means "host check rejected it". A 404 is the app's own routing and
	// must NOT be reported as an allowedHosts problem.
	t.Run("404 on the tunnel host is not an allowedHosts problem", func(t *testing.T) {
		host, port := startProbeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			if strings.HasSuffix(hostOnly(r.Host), tunnelHostSuffix) {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		if got := CheckEmbeddable(host, port, probeTimeout); hasKind(got, FindingAllowedHosts) {
			t.Fatalf("404 must not be reported as an allowedHosts rejection; got %v", kinds(got))
		}
	})
}

// TestCheckEmbeddableNonVite covers a dev server that is not Vite: the header
// checks still run (via the / fallback) but the remediation must not tell the
// author to edit a vite.config.ts they do not have.
func TestCheckEmbeddableNonVite(t *testing.T) {
	host, port := startProbeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/@vite/client" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK) // no ACAO
	}))
	got := CheckEmbeddable(host, port, probeTimeout)
	if !hasKind(got, FindingCORS) {
		t.Fatalf("a non-Vite server missing ACAO must still be flagged; got %v", kinds(got))
	}
	for _, f := range got {
		joined := strings.Join(f.Fix, "\n")
		if strings.Contains(joined, "vite.config.ts") {
			t.Fatalf("non-Vite remediation must not name vite.config.ts:\n%s", joined)
		}
	}
}

// TestCheckEmbeddableVitePathNamesViteConfig is the counterpart: when Vite IS
// detected the author gets the config snippet. Without this, the non-Vite test
// above would pass against a build that never emits vite.config.ts at all.
func TestCheckEmbeddableVitePathNamesViteConfig(t *testing.T) {
	host, port := startProbeServer(t, stockViteHandler())
	got := CheckEmbeddable(host, port, probeTimeout)
	if len(got) == 0 {
		t.Fatal("expected findings from a stock vite server")
	}
	joined := strings.Join(got[0].Fix, "\n")
	if !strings.Contains(joined, "vite.config.ts") {
		t.Fatalf("vite remediation must name vite.config.ts:\n%s", joined)
	}
	if !strings.Contains(joined, tunnelHostSuffix) {
		t.Fatalf("vite remediation must include the %s allowedHosts entry:\n%s", tunnelHostSuffix, joined)
	}
}

// TestCheckEmbeddableUnobservable: when the probe cannot reach anything it must
// stay SILENT. probeLocalDevServer already owns "your dev server is down", and
// advice manufactured from a failed observation is worse than none.
func TestCheckEmbeddableUnobservable(t *testing.T) {
	// Bind then immediately release, so the port is almost certainly dead.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	if got := CheckEmbeddable("127.0.0.1", port, 250*time.Millisecond); got != nil {
		t.Fatalf("an unreachable server must produce no findings; got %v", kinds(got))
	}
}

// TestCheckEmbeddableServerError: a 5xx on both probe paths means the server is
// answering but broken — header advice would be noise.
func TestCheckEmbeddableServerError(t *testing.T) {
	host, port := startProbeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	if got := CheckEmbeddable(host, port, probeTimeout); len(got) != 0 {
		t.Fatalf("a 5xx dev server must produce no header findings; got %v", kinds(got))
	}
}

// TestFindingsCarryRemediation guards the thing that makes this feature useful:
// every finding must name what was observed AND what to do. A summary-only
// warning would reproduce the original complaint in a new form.
func TestFindingsCarryRemediation(t *testing.T) {
	host, port := startProbeServer(t, stockViteHandler())
	got := CheckEmbeddable(host, port, probeTimeout)
	if len(got) == 0 {
		t.Fatal("expected findings")
	}
	for _, f := range got {
		if strings.TrimSpace(f.Summary) == "" {
			t.Errorf("%s: empty summary", f.Kind)
		}
		if len(f.Evidence) == 0 {
			t.Errorf("%s: no evidence", f.Kind)
		}
		if len(f.Fix) == 0 {
			t.Errorf("%s: no remediation", f.Kind)
		}
		if !strings.Contains(strings.Join(f.Fix, "\n"), "Fix") {
			t.Errorf("%s: remediation names no fix:\n%s", f.Kind, strings.Join(f.Fix, "\n"))
		}
	}
}

func TestFrameAncestors(t *testing.T) {
	tests := []struct {
		csp         string
		wantPresent bool
		wantAllow   bool
	}{
		{csp: "", wantPresent: false},
		{csp: "default-src 'self'", wantPresent: false},
		{csp: "frame-ancestors 'self' " + ProdParentOrigin, wantPresent: true, wantAllow: true},
		{csp: "frame-ancestors 'self'", wantPresent: true, wantAllow: false},
		{csp: "frame-ancestors 'none'", wantPresent: true, wantAllow: false},
		{csp: "frame-ancestors *", wantPresent: true, wantAllow: true},
		{csp: "frame-ancestors https:", wantPresent: true, wantAllow: true},
		{csp: "frame-ancestors https://civitai.com/", wantPresent: true, wantAllow: true},
		{csp: "frame-ancestors *.civitai.com", wantPresent: true, wantAllow: false},
		{csp: "default-src 'self'; frame-ancestors " + ProdParentOrigin, wantPresent: true, wantAllow: true},
		{csp: "frame-ancestors https://evil.com; default-src 'self'", wantPresent: true, wantAllow: false},
		// `'none'` is decisive only as the SOLE source: the CSP grammar does not
		// admit it beside others, so a browser ignores the invalid `'none'` and
		// honours the rest.
		{csp: "frame-ancestors 'none' " + ProdParentOrigin, wantPresent: true, wantAllow: true},
		{csp: "frame-ancestors https://civitai.com:443", wantPresent: true, wantAllow: true},
		{csp: "frame-ancestors http://civitai.com", wantPresent: true, wantAllow: true},
	}
	for _, tc := range tests {
		t.Run(tc.csp, func(t *testing.T) {
			sources, present := frameAncestors(tc.csp)
			if present != tc.wantPresent {
				t.Fatalf("present: got %v want %v", present, tc.wantPresent)
			}
			if !present {
				return
			}
			if got := frameAncestorsAllow(sources); got != tc.wantAllow {
				t.Fatalf("allow(%v): got %v want %v", sources, got, tc.wantAllow)
			}
		})
	}
}

// TestHostSourceAllows exercises the host-source matcher directly, including the
// `*.` wildcard branch — which frameAncestorsAllow can never reach a TRUE result
// through for the bare civitai.com parent, so without this it would be untested.
func TestHostSourceAllows(t *testing.T) {
	tests := []struct {
		source string
		origin string
		want   bool
	}{
		{source: "https://civitai.com", origin: "https://civitai.com", want: true},
		{source: "civitai.com", origin: "https://civitai.com", want: true},
		{source: "https://civitai.com/", origin: "https://civitai.com", want: true},
		{source: "HTTPS://CIVITAI.COM", origin: "https://civitai.com", want: true},
		{source: "https://example.com", origin: "https://civitai.com", want: false},
		// Wildcard: matches a subdomain, NOT the bare registrable domain.
		{source: "*.civitai.com", origin: "https://www.civitai.com", want: true},
		{source: "*.civitai.com", origin: "https://civitai.com", want: false},
		// A wildcard must not match a lookalike suffix.
		{source: "*.civitai.com", origin: "https://evilcivitai.com", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.source+"|"+tc.origin, func(t *testing.T) {
			if got := hostSourceAllows(tc.source, tc.origin); got != tc.want {
				t.Fatalf("hostSourceAllows(%q, %q) = %v want %v", tc.source, tc.origin, got, tc.want)
			}
		})
	}
}

func TestOriginsInclude(t *testing.T) {
	tests := []struct {
		list string
		want bool
	}{
		{list: "", want: false},
		{list: ProdParentOrigin, want: true},
		{list: "http://localhost:5186," + ProdParentOrigin, want: true},
		{list: " http://localhost:5186 , " + ProdParentOrigin + " ", want: true},
		{list: ProdParentOrigin + "/", want: true},
		{list: "HTTPS://CIVITAI.COM", want: true},
		{list: "http://localhost:5186", want: false},
		{list: "https://www.civitai.com", want: false},
		{list: "https://civitai.com.evil.test", want: false},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%q", tc.list), func(t *testing.T) {
			if got := originsInclude(tc.list, ProdParentOrigin); got != tc.want {
				t.Fatalf("originsInclude(%q) = %v want %v", tc.list, got, tc.want)
			}
		})
	}
}

// ── regressions from the PR #198 audit ───────────────────────────────────────

// TestCheckEmbeddableNonInterpretableBaseline: the probe may reach something
// that is NOT the app — an authenticating proxy, a blanket-deny gate, a redirect
// to a login page. Its headers say nothing about how the app would be served, so
// reading CORS off them reported "your modules are CORS-blocked" at servers whose
// CORS was fine, and reading the follow-up 403 blamed `allowedHosts` for a proxy
// that rejects every request identically.
func TestCheckEmbeddableNonInterpretableBaseline(t *testing.T) {
	for _, status := range []int{
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusFound,
		http.StatusNotFound,
		http.StatusBadGateway,
		http.StatusInternalServerError,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			host, port := startProbeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			if got := CheckEmbeddable(host, port, probeTimeout); len(got) != 0 {
				t.Fatalf("a uniform %d must produce no findings, got %v", status, kinds(got))
			}
		})
	}
}

// TestCheckEmbeddableHostCheckStillCaught is the counterpart control: the 2xx
// gate above must NOT swallow the real allowedHosts case, where the baseline
// succeeds and only the tunnel-shaped Host is refused.
func TestCheckEmbeddableHostCheckStillCaught(t *testing.T) {
	host, port := startProbeServer(t, stockViteHandler())
	got := CheckEmbeddable(host, port, probeTimeout)
	if !hasKind(got, FindingAllowedHosts) {
		t.Fatalf("a host-specific 403 must still be reported, got %v", kinds(got))
	}
}

// TestCheckEmbeddableAllCSPHeaders: policies combine restrictively and a response
// may carry several. Header.Get sees only the first, so a second policy saying
// frame-ancestors 'none' used to be reported clean.
func TestCheckEmbeddableAllCSPHeaders(t *testing.T) {
	host, port := startProbeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Add("Content-Security-Policy", "frame-ancestors "+ProdParentOrigin)
		w.Header().Add("Content-Security-Policy", "frame-ancestors 'none'")
		w.WriteHeader(http.StatusOK)
	}))
	if got := CheckEmbeddable(host, port, probeTimeout); !hasKind(got, FindingFrameAncestors) {
		t.Fatalf("every CSP header must be evaluated, got %v", kinds(got))
	}
}

// TestFindingEvidenceNamesTheProbe pins the user-visible evidence strings. Every
// other test drives handlers that behave identically on all paths and asserts
// only on Kind, so mutating the probed path or leaking the directive name into
// the evidence survived the whole suite.
func TestFindingEvidenceNamesTheProbe(t *testing.T) {
	t.Run("cors evidence names the vite module path and the null origin", func(t *testing.T) {
		host, port := startProbeServer(t, stockViteHandler())
		got := CheckEmbeddable(host, port, probeTimeout)
		var cors *Finding
		for i := range got {
			if got[i].Kind == FindingCORS {
				cors = &got[i]
			}
		}
		if cors == nil {
			t.Fatalf("expected a cors finding, got %v", kinds(got))
		}
		ev := strings.Join(cors.Evidence, "\n")
		if !strings.Contains(ev, "GET /@vite/client") {
			t.Errorf("evidence must name the probed path:\n%s", ev)
		}
		if !strings.Contains(ev, "Origin: null") {
			t.Errorf("evidence must name the origin it sent:\n%s", ev)
		}
	})

	t.Run("non-vite evidence names the root path", func(t *testing.T) {
		host, port := startProbeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/@vite/client" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK) // 200 baseline, no ACAO
		}))
		got := CheckEmbeddable(host, port, probeTimeout)
		if len(got) == 0 {
			t.Fatal("expected findings")
		}
		ev := strings.Join(got[0].Evidence, "\n")
		if !strings.Contains(ev, "GET /  ") {
			t.Errorf("non-Vite evidence must name the root path, not the vite one:\n%s", ev)
		}
		if strings.Contains(ev, "@vite/client") {
			t.Errorf("non-Vite evidence must not name the vite path:\n%s", ev)
		}
	})

	t.Run("frame-ancestors evidence lists only the sources", func(t *testing.T) {
		host, port := startProbeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Security-Policy", "frame-ancestors 'self'")
			w.WriteHeader(http.StatusOK)
		}))
		got := CheckEmbeddable(host, port, probeTimeout)
		if len(got) == 0 {
			t.Fatal("expected a frame-ancestors finding")
		}
		ev := strings.Join(got[0].Evidence, "\n")
		// The rendered line is "…: frame-ancestors 'self'" — the directive name is
		// printed by the format string, so it must NOT also appear in the sources.
		if strings.Contains(ev, "frame-ancestors frame-ancestors") {
			t.Errorf("the directive name leaked into the source list:\n%s", ev)
		}
		if !strings.Contains(ev, "frame-ancestors 'self'") {
			t.Errorf("evidence must show the observed source list:\n%s", ev)
		}
	})

	t.Run("allowed-hosts evidence names the tunnel host", func(t *testing.T) {
		host, port := startProbeServer(t, stockViteHandler())
		got := CheckEmbeddable(host, port, probeTimeout)
		var ah *Finding
		for i := range got {
			if got[i].Kind == FindingAllowedHosts {
				ah = &got[i]
			}
		}
		if ah == nil {
			t.Fatalf("expected an allowed-hosts finding, got %v", kinds(got))
		}
		ev := strings.Join(ah.Evidence, "\n")
		if !strings.Contains(ev, probeHostname) || !strings.Contains(ev, "403") {
			t.Errorf("evidence must name the Host it sent and the status:\n%s", ev)
		}
	})
}

// ── regressions from the PR #198 DELTA re-audit ──────────────────────────────

// TestCheckEmbeddableFollowsRedirect: a Vite project with a `base` path 404s
// /@vite/client and redirects /, so refusing to follow redirects made the 2xx
// gate report a genuinely un-embeddable server as CLEAN — the gate over-
// corrected into silence. The browser follows the redirect, so the target is the
// app and its headers are the interpretable ones.
func TestCheckEmbeddableFollowsRedirect(t *testing.T) {
	basePathHandler := func(w http.ResponseWriter, r *http.Request) {
		if hostOnly(r.Host) != "127.0.0.1" && hostOnly(r.Host) != "localhost" {
			w.WriteHeader(http.StatusForbidden) // real allowedHosts behaviour
			return
		}
		switch r.URL.Path {
		case "/app/", "/app/@vite/client":
			w.WriteHeader(http.StatusOK) // real content, NO ACAO
		case "/":
			w.Header().Set("Location", "/app/")
			w.WriteHeader(http.StatusFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	t.Run("the broken server behind a redirect is still reported", func(t *testing.T) {
		host, port := startProbeServer(t, http.HandlerFunc(basePathHandler))
		got := CheckEmbeddable(host, port, probeTimeout)
		if !hasKind(got, FindingCORS) || !hasKind(got, FindingAllowedHosts) {
			t.Fatalf("want cors + allowed-hosts through the redirect, got %v", kinds(got))
		}
	})

	t.Run("vite is still detected under a base path", func(t *testing.T) {
		host, port := startProbeServer(t, http.HandlerFunc(basePathHandler))
		got := CheckEmbeddable(host, port, probeTimeout)
		if len(got) == 0 {
			t.Fatal("expected findings")
		}
		joined := strings.Join(got[0].Fix, "\n")
		if !strings.Contains(joined, "vite.config.ts") {
			t.Errorf("a base-path Vite project must still get the vite remediation:\n%s", joined)
		}
	})

	// Control: a HEALTHY server behind a redirect must stay clean, so the redirect
	// following cannot be passing the case above by warning indiscriminately.
	t.Run("a healthy server behind a redirect stays clean", func(t *testing.T) {
		host, port := startProbeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				w.Header().Set("Location", "/app/")
				w.WriteHeader(http.StatusFound)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Security-Policy", "frame-ancestors 'self' "+ProdParentOrigin)
			w.WriteHeader(http.StatusOK)
		}))
		if got := CheckEmbeddable(host, port, probeTimeout); len(got) != 0 {
			t.Fatalf("a correctly configured server behind a redirect must be clean, got %v", kinds(got))
		}
	})

	// A redirect LOOP must terminate and stay silent rather than hang: the
	// transport always dials the local server, so an unbounded chase would spin.
	t.Run("a redirect loop terminates without findings", func(t *testing.T) {
		host, port := startProbeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "/loop")
			w.WriteHeader(http.StatusFound)
		}))
		done := make(chan []Finding, 1)
		go func() { done <- CheckEmbeddable(host, port, probeTimeout) }()
		select {
		case got := <-done:
			if len(got) != 0 {
				t.Fatalf("an endless redirect is not interpretable; got %v", kinds(got))
			}
		case <-time.After(15 * time.Second):
			t.Fatal("CheckEmbeddable did not terminate on a redirect loop")
		}
	})
}

// TestCheckEmbeddableDuplicateCORSHeaders: more than one
// Access-Control-Allow-Origin is a CORS failure in the browser whatever the
// values say, so reading only the first reported a blocked server as fine.
func TestCheckEmbeddableDuplicateCORSHeaders(t *testing.T) {
	host, port := startProbeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")
		w.Header().Add("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
	}))
	got := CheckEmbeddable(host, port, probeTimeout)
	if !hasKind(got, FindingCORS) {
		t.Fatalf("duplicate ACAO headers must be reported, got %v", kinds(got))
	}
}

// TestHostSourceAllowsDefaultPorts: only the default port for the source's OWN
// scheme may be dropped — `https://civitai.com:80` is a different origin.
func TestHostSourceAllowsDefaultPorts(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{source: "https://civitai.com:443", want: true},
		{source: "http://civitai.com:80", want: true},
		{source: "civitai.com:443", want: true},
		{source: "https://civitai.com:80", want: false},
		{source: "http://civitai.com:443", want: false},
		{source: "https://civitai.com:8443", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.source, func(t *testing.T) {
			if got := hostSourceAllows(tc.source, ProdParentOrigin); got != tc.want {
				t.Fatalf("hostSourceAllows(%q) = %v want %v", tc.source, got, tc.want)
			}
		})
	}
}

// TestCheckEmbeddableRedirectSafety pins the two limits on redirect following.
// Both were unpinned when redirect following was introduced: a mutant removing
// the hop bound and a mutant following cross-host redirects each survived.
func TestCheckEmbeddableRedirectSafety(t *testing.T) {
	t.Run("cross-host redirects are not followed", func(t *testing.T) {
		// Observable is the PATH, not the Host: the redirect-following code forces
		// Request.Host back to the original authority, so a foreign Host never
		// reaches the server even when the cross-host guard is defeated. Asserting
		// on Host therefore could not detect the guard being removed.
		var mu sync.Mutex
		var paths []string
		host, port := startProbeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			paths = append(paths, r.URL.Path)
			mu.Unlock()
			w.Header().Set("Location", "http://elsewhere.example/somewhere")
			w.WriteHeader(http.StatusFound)
		}))
		CheckEmbeddable(host, port, probeTimeout)

		mu.Lock()
		defer mu.Unlock()
		if len(paths) == 0 {
			t.Fatal("no request reached the server — the harness observes nothing")
		}
		for _, p := range paths {
			// The transport ALWAYS dials the local dev server whatever the URL says,
			// so chasing an external Location just probes THIS server at someone
			// else's path and proves nothing about it.
			if p == "/somewhere" {
				t.Fatalf("a cross-host redirect must not be followed; requested %v", paths)
			}
		}
	})

	t.Run("redirect hops are bounded", func(t *testing.T) {
		var mu sync.Mutex
		hops := 0
		host, port := startProbeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			hops++
			n := hops
			mu.Unlock()
			w.Header().Set("Location", fmt.Sprintf("/hop%d", n))
			w.WriteHeader(http.StatusFound)
		}))
		CheckEmbeddable(host, port, probeTimeout)
		mu.Lock()
		defer mu.Unlock()
		// Two probe requests may each redirect up to the bound; anything far above
		// that means the bound is not being applied.
		if max := 4 * (maxProbeRedirects + 1); hops > max {
			t.Fatalf("redirect following is unbounded: %d hops (bound implies <= %d)", hops, max)
		}
		if hops == 0 {
			t.Fatal("no requests reached the server — the harness is not observing anything")
		}
	})
}

// ── regressions from the PR #198 round-3 delta re-audit ──────────────────────

// TestCheckEmbeddableRedirectedProbeIsNotVite: following redirects made a 200
// reached via a redirect look like a 200 from the path we asked for. Any server
// that bounces unknown paths to an index (an auth gate, an SPA dev server) was
// then classified Vite and handed vite.config.ts advice, and the evidence line
// asserted that /@vite/client returned 200 when it returned a redirect.
func TestCheckEmbeddableRedirectedProbeIsNotVite(t *testing.T) {
	host, port := startProbeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			w.WriteHeader(http.StatusOK) // 200, no ACAO
			return
		}
		w.Header().Set("Location", "/login")
		w.WriteHeader(http.StatusFound)
	}))
	got := CheckEmbeddable(host, port, probeTimeout)
	if !hasKind(got, FindingCORS) {
		t.Fatalf("the server is still un-embeddable and must be reported; got %v", kinds(got))
	}
	for _, f := range got {
		ev := strings.Join(f.Evidence, "\n")
		if strings.Contains(ev, "GET "+viteClientPath) {
			t.Errorf("evidence must name the path actually answered, not the one requested:\n%s", ev)
		}
		if strings.Contains(strings.Join(f.Fix, "\n"), "vite.config.ts") {
			t.Errorf("a non-Vite server must not get vite.config.ts remediation:\n%s", strings.Join(f.Fix, "\n"))
		}
	}
}

// TestCheckAllowedHostsKeepsHostAcrossAbsoluteRedirect: net/http carries a custom
// Request.Host across a RELATIVE redirect but drops it for an absolute one, which
// silently turned the tunnel-Host probe into an ordinary loopback request.
func TestCheckAllowedHostsKeepsHostAcrossAbsoluteRedirect(t *testing.T) {
	// The baseline must land on /@vite/client directly (so `probed` is that path),
	// and ONLY the tunnel-Host request may be redirected — otherwise the host probe
	// never traverses a redirect and this guard is unreachable.
	var srvURL string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tunnelHost := strings.HasSuffix(hostOnly(r.Host), tunnelHostSuffix)
		switch {
		case r.URL.Path == "/gate":
			// The gate denies a tunnel Host and admits anything else. Reaching it
			// with the Host dropped silently turns a 403 into a 200.
			if tunnelHost {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusOK)
		case tunnelHost:
			w.Header().Set("Location", srvURL+"/gate") // ABSOLUTE, same host
			w.WriteHeader(http.StatusFound)
		default:
			w.WriteHeader(http.StatusOK) // baseline: 200, no ACAO
		}
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	srvURL = srv.URL

	h, p, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := CheckEmbeddable(h, port, probeTimeout); !hasKind(got, FindingAllowedHosts) {
		t.Fatalf("the tunnel Host must survive an absolute redirect; got %v", kinds(got))
	}
}

// TestCheckAllowedHostsProbesTheAnsweringPath: the host check must exercise the
// path the baseline actually came from. A handler that refuses a foreign Host at
// EVERY path cannot observe which path is probed, which is why this needs its own
// case rather than riding on the base-path test.
func TestCheckAllowedHostsProbesTheAnsweringPath(t *testing.T) {
	var tunnelHostPaths []string
	var mu sync.Mutex
	host, port := startProbeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(hostOnly(r.Host), tunnelHostSuffix) {
			mu.Lock()
			tunnelHostPaths = append(tunnelHostPaths, r.URL.Path)
			mu.Unlock()
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.URL.Path == viteClientPath {
			w.WriteHeader(http.StatusNotFound) // not Vite
			return
		}
		w.WriteHeader(http.StatusOK) // "/" is the answering path, no ACAO
	}))
	CheckEmbeddable(host, port, probeTimeout)

	mu.Lock()
	defer mu.Unlock()
	if len(tunnelHostPaths) == 0 {
		t.Fatal("no tunnel-Host request reached the server — the harness observes nothing")
	}
	for _, p := range tunnelHostPaths {
		if p == viteClientPath {
			t.Errorf("host check probed %s, but the baseline was answered at /", viteClientPath)
		}
	}
}
