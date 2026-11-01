package devtunnel

import (
	"net/http"
	"strings"
	"testing"
)

// The Civitai host serves App previews (`/apps/dev/<blockId>`) on EVERY Civitai
// domain, not just civitai.com. A tester on civitai.red got a blank preview: the
// dev server's `frame-ancestors 'self' https://civitai.com` refused the frame,
// and the app's allowlist dropped the host's BLOCK_INIT from civitai.red. The
// preflight said nothing, because it only ever asked about civitai.com.
//
// These values are spelled out here on purpose, NOT read from the constant the
// code uses: a test that derives its expectation from the implementation passes
// whatever the implementation says.
const (
	civitaiCom        = "https://civitai.com"
	civitaiOriginsCSV = "https://civitai.com,https://civitai.red,https://civitai.green"
	civitaiOriginsCSP = "https://civitai.com https://civitai.red https://civitai.green"
)

// TestFramingRequiresEveryCivitaiDomain: a frame-ancestors list that leaves out
// any Civitai domain is a finding, and a lookalike does not stand in for one.
func TestFramingRequiresEveryCivitaiDomain(t *testing.T) {
	tests := []struct {
		name     string
		csp      string
		wantWarn bool
	}{
		{name: "every Civitai domain", csp: "frame-ancestors 'self' " + civitaiOriginsCSP},
		{name: "every Civitai domain, any order", csp: "frame-ancestors https://civitai.green https://civitai.red 'self' https://civitai.com"},
		{name: "a wildcard admits all of them", csp: "frame-ancestors *"},
		{name: "civitai.com alone (the old scaffold)", csp: "frame-ancestors 'self' https://civitai.com", wantWarn: true},
		{name: "civitai.red missing", csp: "frame-ancestors 'self' https://civitai.com https://civitai.green", wantWarn: true},
		{name: "civitai.green missing", csp: "frame-ancestors 'self' https://civitai.com https://civitai.red", wantWarn: true},
		{name: "civitai.com missing", csp: "frame-ancestors 'self' https://civitai.red https://civitai.green", wantWarn: true},
		{name: "a lookalike does not stand in for civitai.red", csp: "frame-ancestors https://civitai.com https://civitai.red.evil.com https://civitai.green", wantWarn: true},
		{name: "a subdomain wildcard does not cover the bare civitai.red", csp: "frame-ancestors https://civitai.com *.civitai.red https://civitai.green", wantWarn: true},
		{name: "a non-Civitai origin admits none of them", csp: "frame-ancestors https://evil.example", wantWarn: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			csp := tc.csp
			host, port := startProbeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				w.Header().Set("Content-Security-Policy", csp)
				w.WriteHeader(http.StatusOK)
			}))
			got := CheckEmbeddable(host, port, probeTimeout)
			if hasKind(got, FindingFrameAncestors) != tc.wantWarn {
				t.Fatalf("csp=%q: want frame-ancestors finding=%v, got %v", tc.csp, tc.wantWarn, kinds(got))
			}
		})
	}
}

// TestFramingFindingNamesTheMissingDomain: the evidence says WHICH domain is not
// admitted, so an author with the old scaffold sees what to add.
func TestFramingFindingNamesTheMissingDomain(t *testing.T) {
	host, port := startProbeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'self' https://civitai.com")
		w.WriteHeader(http.StatusOK)
	}))
	var f *Finding
	for _, x := range CheckEmbeddable(host, port, probeTimeout) {
		if x.Kind == FindingFrameAncestors {
			f = &x
		}
	}
	if f == nil {
		t.Fatal("want a frame-ancestors finding for a CSP that admits civitai.com alone")
	}
	ev := strings.Join(f.Evidence, "\n")
	if want := "That directive does not admit https://civitai.red, https://civitai.green, so the browser"; !strings.Contains(ev, want) {
		t.Errorf("evidence does not name the missing domains (want %q):\n%s", want, ev)
	}
	fix := strings.Join(f.Fix, "\n")
	// startProbeServer answers every path 200, /@vite/client included, so this
	// reads as a Vite dev server and gets the vite.config.ts snippet.
	if want := `        'Content-Security-Policy': "frame-ancestors 'self' ` + civitaiOriginsCSP + `",`; !strings.Contains(fix, want) {
		t.Errorf("the fix does not carry the full CSP line (want %q):\n%s", want, fix)
	}
}

// TestParentOriginsRequireEveryCivitaiDomain: the allowlist must name every
// Civitai domain exactly. The SDK matches origins exactly, so a lookalike, the
// http:// scheme or a subdomain of a Civitai domain does not count.
func TestParentOriginsRequireEveryCivitaiDomain(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		wantWarn bool
	}{
		{name: "the scaffold's list", value: "http://localhost:5186," + civitaiOriginsCSV},
		{name: "every Civitai domain, any order", value: "https://civitai.green,https://civitai.com,https://civitai.red"},
		{name: "civitai.com alone (the old scaffold)", value: "http://localhost:5186,https://civitai.com", wantWarn: true},
		{name: "civitai.red missing", value: "https://civitai.com,https://civitai.green", wantWarn: true},
		{name: "civitai.green missing", value: "https://civitai.com,https://civitai.red", wantWarn: true},
		{name: "a lookalike is not civitai.red", value: "https://civitai.com,https://civitai.red.evil.com,https://civitai.green", wantWarn: true},
		{name: "http:// is not https://civitai.red", value: "https://civitai.com,http://civitai.red,https://civitai.green", wantWarn: true},
		{name: "http://civitai.com is not https://civitai.com", value: "http://civitai.com,https://civitai.red,https://civitai.green", wantWarn: true},
		{name: "a subdomain is not civitai.red", value: "https://civitai.com,https://www.civitai.red,https://civitai.green", wantWarn: true},
		{name: "a non-Civitai origin", value: "https://evil.example", wantWarn: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearParentOrigins(t)
			dir := writeAppDir(t, appDirOpts{
				manifest: true, pkg: sdkPkg,
				envFiles: map[string]string{".env.development": ParentOriginsEnvVar + "=" + tc.value + "\n"},
			})
			got := CheckParentOrigins(dir)
			if hasKind(got, FindingParentOrigins) != tc.wantWarn {
				t.Fatalf("%s=%s: want parent-origins finding=%v, got %v", ParentOriginsEnvVar, tc.value, tc.wantWarn, kinds(got))
			}
		})
	}
}

// TestParentOriginsFixAddsOnlyTheMissingDomains: the suggested line keeps what
// the author has and appends exactly the Civitai domains it lacks, in order.
func TestParentOriginsFixAddsOnlyTheMissingDomains(t *testing.T) {
	clearParentOrigins(t)
	dir := writeAppDir(t, appDirOpts{
		manifest: true, pkg: sdkPkg,
		envFiles: map[string]string{".env.development": ParentOriginsEnvVar + "=http://localhost:5186,https://civitai.com\n"},
	})
	got := CheckParentOrigins(dir)
	if len(got) != 1 {
		t.Fatalf("want 1 finding for an allowlist with civitai.com alone, got %v", kinds(got))
	}
	want := "    " + ParentOriginsEnvVar + "=http://localhost:5186," + civitaiOriginsCSV
	found := false
	for _, line := range got[0].Fix {
		if line == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("fix does not carry the exact line %q:\n%s", want, strings.Join(got[0].Fix, "\n"))
	}
}
