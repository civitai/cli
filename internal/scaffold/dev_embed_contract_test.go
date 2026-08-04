package scaffold

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/civitai/cli/internal/devtunnel"
)

// The dev-server embeddability contract lives in TWO places and TWO languages:
// the scaffold emits it (page-money's src/dev-embed.ts + .env.development) and
// the CLI's dev-tunnel preflight checks for it (internal/devtunnel/embedcheck.go).
// Nothing in the compiler relates them, so either side can drift and silently
// un-fix the thing issue #196 was about — a scaffold that stops emitting the
// headers, or a preflight that stops recognising them.
//
// These guards pin the RELATIONSHIP rather than either side's spelling: they
// render the real template, extract the values it actually emits, stand up a
// server that behaves the way a Vite dev server configured with those values
// behaves, and require the CLI's own predicate to pass it. A `strings.Contains`
// check for the word "frame-ancestors" cannot do that — it stays green while the
// value drifts to something the CLI rejects.

// devEmbedConstants pulls the literal values out of the rendered dev-embed.ts,
// resolving the one template-literal interpolation it uses.
type devEmbedConstants struct {
	parentOrigin string
	hostSuffix   string
	csp          string
	acao         string
}

var (
	reParentOrigin = regexp.MustCompile(`PROD_PARENT_ORIGIN\s*=\s*'([^']+)'`)
	reHostSuffix   = regexp.MustCompile(`DEV_TUNNEL_HOST_SUFFIX\s*=\s*'([^']+)'`)
	// The header values are matched to END OF LINE, not with a character class:
	// the CSP value legitimately CONTAINS quotes (`frame-ancestors 'self' …`), and
	// a `[^']+` class silently truncates it at the first one — yielding a value the
	// preflight rejects and a drift report that blames the template.
	reCSP  = regexp.MustCompile(`(?m)^\s*'Content-Security-Policy':\s*(.+)$`)
	reACAO = regexp.MustCompile(`(?m)^\s*'Access-Control-Allow-Origin':\s*(.+)$`)
)

// unquoteTSLiteral strips a trailing comma and the surrounding ' or ` delimiters
// from a captured TypeScript string literal.
func unquoteTSLiteral(v string) string {
	v = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), ","))
	if len(v) >= 2 {
		if (v[0] == '`' && v[len(v)-1] == '`') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}

func extractDevEmbedConstants(t *testing.T, src string) devEmbedConstants {
	t.Helper()
	grab := func(re *regexp.Regexp, what string) string {
		m := re.FindStringSubmatch(src)
		if m == nil {
			t.Fatalf("could not find %s in dev-embed.ts — the contract guard can no longer read the template, which is itself a drift failure", what)
		}
		return m[1]
	}
	c := devEmbedConstants{
		parentOrigin: grab(reParentOrigin, "PROD_PARENT_ORIGIN"),
		hostSuffix:   grab(reHostSuffix, "DEV_TUNNEL_HOST_SUFFIX"),
		csp:          unquoteTSLiteral(grab(reCSP, "the Content-Security-Policy header")),
		acao:         unquoteTSLiteral(grab(reACAO, "the Access-Control-Allow-Origin header")),
	}
	// Resolve `${PROD_PARENT_ORIGIN}` in the CSP template literal.
	c.csp = strings.ReplaceAll(c.csp, "${PROD_PARENT_ORIGIN}", c.parentOrigin)
	if strings.Contains(c.csp, "${") {
		t.Fatalf("unresolved interpolation in the CSP value %q — extend the guard", c.csp)
	}
	// Sanity-check the EXTRACTOR before its output is used as evidence: a regex
	// that silently captured a fragment would otherwise produce a drift report
	// about the template when the fault is in this file.
	if !strings.HasPrefix(c.csp, "frame-ancestors ") || len(strings.Fields(c.csp)) < 2 {
		t.Fatalf("extracted CSP %q does not look like a frame-ancestors directive — fix the extractor, not the template", c.csp)
	}
	if c.acao == "" || c.hostSuffix == "" || c.parentOrigin == "" {
		t.Fatalf("extractor produced an empty value: %+v", c)
	}
	return c
}

// scaffoldHeaderServer stands up a server that answers the way a Vite dev server
// configured from these constants would: it honours the allowedHosts suffix and
// emits the two headers.
func scaffoldHeaderServer(t *testing.T, c devEmbedConstants, emitHeaders bool) (string, int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(r.Host); err == nil {
			host = h
		}
		allowed := host == "localhost" || host == "127.0.0.1" || strings.HasSuffix(host, c.hostSuffix)
		if !allowed {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if emitHeaders {
			w.Header().Set("Content-Security-Policy", c.csp)
			w.Header().Set("Access-Control-Allow-Origin", c.acao)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	h, p, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("split %s: %v", srv.URL, err)
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		t.Fatalf("port %s: %v", p, err)
	}
	return h, port
}

// TestScaffoldHeadersSatisfyDevTunnelPreflight is the seam guard for the header
// half: what page-money emits must be exactly what `civitai app dev-tunnel`
// accepts. It reports a control PAIR — with the headers the preflight must be
// silent, without them it must complain — so a preflight that degenerated into
// "always returns nil" fails here instead of passing as a clean bill of health.
func TestScaffoldHeadersSatisfyDevTunnelPreflight(t *testing.T) {
	dir := t.TempDir()
	if _, err := Render(PageMoney, dir, Data{Slug: "my-block", Name: "My Block"}); err != nil {
		t.Fatalf("render page-money: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "src", "dev-embed.ts"))
	if err != nil {
		t.Fatalf("read dev-embed.ts: %v", err)
	}
	c := extractDevEmbedConstants(t, string(raw))

	t.Run("positive: scaffold headers pass the preflight", func(t *testing.T) {
		host, port := scaffoldHeaderServer(t, c, true)
		if got := devtunnel.CheckEmbeddable(host, port, 2*time.Second); len(got) != 0 {
			var kinds []string
			for _, f := range got {
				kinds = append(kinds, string(f.Kind)+": "+f.Summary)
			}
			t.Fatalf("the scaffold's own dev-server config must satisfy the CLI preflight, but it reported:\n  %s\n"+
				"either the template drifted or the preflight no longer recognises it — reconcile "+
				"internal/scaffold/templates/page-money/src/dev-embed.ts with internal/devtunnel/embedcheck.go",
				strings.Join(kinds, "\n  "))
		}
	})

	t.Run("negative: without the headers the preflight complains", func(t *testing.T) {
		host, port := scaffoldHeaderServer(t, c, false)
		if got := devtunnel.CheckEmbeddable(host, port, 2*time.Second); len(got) == 0 {
			t.Fatal("preflight reported nothing for a server with no embed headers — " +
				"the positive case above therefore proves nothing")
		}
	})
}

// TestScaffoldEnvSatisfiesDevTunnelPreflight is the seam guard for the
// parent-origins half, driven by the REAL rendered .env.development.
func TestScaffoldEnvSatisfiesDevTunnelPreflight(t *testing.T) {
	if v, ok := os.LookupEnv(devtunnel.ParentOriginsEnvVar); ok {
		t.Setenv(devtunnel.ParentOriginsEnvVar, v)
		_ = os.Unsetenv(devtunnel.ParentOriginsEnvVar)
	}

	dir := t.TempDir()
	if _, err := Render(PageMoney, dir, Data{Slug: "my-block", Name: "My Block"}); err != nil {
		t.Fatalf("render page-money: %v", err)
	}

	t.Run("positive: the scaffolded project is silent", func(t *testing.T) {
		if got := devtunnel.CheckParentOrigins(dir); len(got) != 0 {
			body, _ := os.ReadFile(filepath.Join(dir, ".env.development"))
			t.Fatalf("a freshly scaffolded page-money app must not trip the parent-origins check.\n"+
				".env.development:\n%s", body)
		}
	})

	// Control: the check must be able to fire on this very project — otherwise the
	// positive result above could come from a gate that silently excludes it (e.g.
	// the SDK-dependency detection breaking against the real package.json).
	t.Run("negative: removing the origin trips it", func(t *testing.T) {
		envPath := filepath.Join(dir, ".env.development")
		body, err := os.ReadFile(envPath)
		if err != nil {
			t.Fatalf("read .env.development: %v", err)
		}
		broken := regexp.MustCompile(`(?m)^(`+regexp.QuoteMeta(devtunnel.ParentOriginsEnvVar)+`=).*$`).
			ReplaceAllString(string(body), "${1}http://localhost:5186")
		if broken == string(body) {
			t.Fatalf("could not rewrite %s in .env.development — the guard cannot construct its control:\n%s",
				devtunnel.ParentOriginsEnvVar, body)
		}
		if err := os.WriteFile(envPath, []byte(broken), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := devtunnel.CheckParentOrigins(dir); len(got) == 0 {
			t.Fatal("dropping the parent origin from the scaffolded .env.development must trip the check — " +
				"the positive case above therefore proves nothing")
		}
	})
}
