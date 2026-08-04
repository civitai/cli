package devtunnel

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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
		{name: "uppercase NULL echo allows it", acao: "NULL", wantCORS: false},
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
		{
			name:     "XFO wins over an otherwise-fine CSP",
			csp:      "frame-ancestors " + ProdParentOrigin,
			xfo:      "DENY",
			wantKind: FindingXFrameOptions,
		},
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
		// A 'none' anywhere in the list is decisive even alongside a matching source.
		{csp: "frame-ancestors 'none' " + ProdParentOrigin, wantPresent: true, wantAllow: false},
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
