package scaffold_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/civitai/cli/internal/credscan"
	"github.com/civitai/cli/internal/pkgzip"
	"github.com/civitai/cli/internal/scaffold"
)

// 🔴 A FRESH SCAFFOLD MUST PACKAGE WITHOUT A CREDENTIAL WARNING (issue #937).
//
// `civitai app submit --package-only` on an untouched page-money app printed
//
//	⚠ 1 packaged file(s) look like they hold credentials:
//	    src/setup-dev-live.test.ts:25  VITE_LIVE_BLOCK_TOKEN
//
// for the template's OWN synthetic mergeEnvFile fixture: the expected output
// `'VITE_LIVE_BLOCK_TOKEN=tok123\nCIVITAI_HOST_KEY=key456\n'` is an unquoted
// assignment whose value clears credscan's length, letter+digit and entropy
// gates. An advisory that fires on every fresh scaffold trains authors to
// ignore it, which is the failure credscan's design optimises against.
//
// The fix is in the FIXTURE, never the scanner: credscan deliberately has no
// test-file carve-out (TestNoTestFileCarveOut), so the fixture now names
// itself synthetic (`fake-…`), the spelling credscan already documents as
// non-real. This test is the seam guard — both legs are the REAL
// implementations `app submit` runs: pkgzip.Build decides what ships, and
// credscan.Scan reads exactly that list (see internal/cmd/app_submit.go).
func TestFreshScaffoldsPackageWithNoCredentialFinding(t *testing.T) {
	examined := 0
	for _, tmpl := range scaffold.AllTemplates() {
		dir, uploaded := renderedProject(t, tmpl)
		files := sortedFiles(uploaded)
		rep := credscan.Scan(dir, files)
		if rep.Truncated || rep.FilesScanned != rep.FilesTotal || rep.FilesTotal != len(files) {
			t.Fatalf("%s: the scan did not read every packaged file (scanned %d of %d, truncated=%v) — "+
				"a zero below would be a fact about the budget, not the scaffold",
				tmpl, rep.FilesScanned, rep.FilesTotal, rep.Truncated)
		}
		for _, f := range rep.Findings {
			t.Errorf("%s: a FRESH scaffold packages %s:%d, which credscan reports as a credential (%s). "+
				"`civitai app submit` would warn on an app the author has not touched yet — fix the "+
				"template's fixture, not the scanner.", tmpl, f.Path, f.Line, f.Label)
		}
		examined++
	}
	if examined < len(scaffold.AllTemplates()) || examined < 3 {
		t.Fatalf("CONTROL failure: examined %d template(s), want every one of AllTemplates()", examined)
	}
}

// TestScaffoldCredentialScanStillFlagsARealTokenInTheShippedTestFile is the
// positive control for the zero above, run through the SAME render → package →
// scan path: the fixture file must actually be in the package, and a
// real-looking token planted in it must still be reported. Without this, a
// pkgzip change that stopped shipping `*.test.ts` — or a scanner that stopped
// reading test files — would turn the guard above into a vacuous green.
func TestScaffoldCredentialScanStillFlagsARealTokenInTheShippedTestFile(t *testing.T) {
	const fixture = "src/setup-dev-live.test.ts"
	dir, uploaded := renderedProject(t, scaffold.PageMoney)
	if _, ok := uploaded[fixture]; !ok {
		t.Fatalf("CONTROL failure: %s is not in page-money's package, so the no-finding guard never "+
			"scanned the fixture issue #937 is about", fixture)
	}
	path := filepath.Join(dir, filepath.FromSlash(fixture))
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Same shape as the original false positive (an unquoted env assignment in
	// a test string), but a value with no synthetic tell.
	if _, err := f.WriteString("const leaked = 'VITE_LIVE_BLOCK_TOKEN=Q8vN2mR7kX4zL9pW3tB6';\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	res, err := pkgzip.Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	rep := credscan.Scan(dir, res.Files)
	var hits int
	for _, fd := range rep.Findings {
		if fd.Path == fixture {
			hits++
		}
	}
	if hits != 1 {
		t.Fatalf("a real-looking token appended to the shipped %s produced %d finding(s) there, want "+
			"exactly 1 (all findings: %+v). Either the planted line is not seen (the scanner or the "+
			"package lost test files) or the synthetic fixture fires again.", fixture, hits, rep.Findings)
	}
}

func sortedFiles(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for f := range set {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}
