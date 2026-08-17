package antipattern

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/pkgzip"
)

// TestScanDirJudgesKeptDotenvNamesByDepth is the behavioural guard for #466.
//
// 🔴 THE DIVERGENCE IT PINS WAS INERT, AND MAKING IT REACHABLE IS THE WHOLE
// METHOD. #461 scoped the packager's dotenv allow-list to the project root, so
// `.env.production` ships and `app/.env.production` does not. ScanDir kept
// asking pkgzip.IsExcludedFile — the ROOT answer — at every depth. That could
// not bite, because `.example`, `.sample` and `.production` are absent from
// scannedExts and the extension gate skips those files before the exclusion
// answer matters. A test written against the SHIPPED extension table is
// therefore vacuous: it passes whichever predicate ScanDir calls.
//
// So this test widens scannedExts for its own duration — the one input that
// makes the file rule execute — and asserts the DEPTH answer through ScanDir's
// real output. Measured red against the base call site: with
// pkgzip.IsExcludedFile(d.Name()) the nested copies ARE scanned and reported.
//
// 🔴 The absence half is guarded by a positive control in the SAME run: the ROOT
// copy of each name must produce a finding. Without it the test is satisfied by
// a scanner that reads nothing at all — which is exactly what a wrong
// scannedExts override would produce.
func TestScanDirJudgesKeptDotenvNamesByDepth(t *testing.T) {
	names := pkgzip.KeptEnvFileNames()
	if len(names) == 0 {
		t.Fatal("CONTROL failure: pkgzip.KeptEnvFileNames() is empty, so this test has no subject")
	}

	// Roster derived from the packager's own allow-list, so a name added to or
	// removed from it enters or leaves this test automatically (#460's ledger
	// convention). Nothing below is a hardcoded copy of those three names.
	exts := make([]string, 0, len(names))
	for _, n := range names {
		e := strings.ToLower(filepath.Ext(n))
		if e == "" {
			t.Fatalf("kept name %q has no extension, so it cannot be routed through scannedExts", n)
		}
		exts = append(exts, e)
	}

	// Widen the extension gate for this test only. Package tests here run
	// sequentially (no t.Parallel anywhere in this package), and the original
	// map is restored on cleanup.
	original := scannedExts
	widened := make(map[string]bool, len(original)+len(exts))
	for k, v := range original {
		widened[k] = v
	}
	for i, e := range exts {
		if original[e] {
			t.Fatalf("CONTROL failure: scannedExts already contains %q (from kept name %q), so widening it "+
				"is not what makes the file rule reachable and this test no longer isolates the depth answer",
				e, names[i])
		}
		widened[e] = true
	}
	scannedExts = widened
	t.Cleanup(func() { scannedExts = original })

	// Fixture: every kept name at the ROOT (must be scanned) and the same name
	// one level down under an ordinary directory (must not be). `app` is not an
	// excluded directory, so the walk reaches the file and only the FILE rule can
	// drop it — a directory-rule skip would prove nothing about #466.
	root := t.TempDir()
	if pkgzip.IsExcluded("app") {
		t.Fatal("fixture error: pkgzip excludes the directory `app`, so the nested rows would be " +
			"skipped by the DIRECTORY rule and this test would pass without exercising the file rule")
	}
	for _, n := range names {
		writeFinding(t, root, n)
		writeFinding(t, root, "app/"+n)
	}
	writeFinding(t, root, "src/App.tsx") // ordinary control: an unrelated scanned file

	findings, err := ScanDir(root)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}
	reported := map[string]bool{}
	for _, f := range findings {
		reported[filepath.ToSlash(f.File)] = true
	}

	for _, n := range names {
		if !reported[n] {
			t.Errorf("CONTROL failure: the ROOT %s produced no finding. The packager UPLOADS it, so the "+
				"scanner must read it — an absence below cannot be credited to the depth rule when the "+
				"scanner is reading nothing.", n)
		}
		nested := "app/" + n
		if reported[nested] {
			t.Errorf("ScanDir reported a finding in %q, which the packager DROPS "+
				"(pkgzip.IsExcludedPath(%q) = %v). The dotenv allow-list is scoped to the project root, so "+
				"a base-name predicate answers the ROOT question at depth — ask IsExcludedPath on the "+
				"walk's rel (#466).", nested, nested, pkgzip.IsExcludedPath(nested))
		}
		// Guard the premise from the pkgzip side too: if the packager ever stopped
		// dropping the nested copy, the expectation above would be wrong rather
		// than the code.
		if !pkgzip.IsExcludedPath(nested) {
			t.Errorf("fixture error: pkgzip.IsExcludedPath(%q) = false, so the packager SHIPS it and this "+
				"row is asserting the wrong thing", nested)
		}
		if pkgzip.IsExcludedPath(n) {
			t.Errorf("fixture error: pkgzip.IsExcludedPath(%q) = true, so the packager drops the root copy "+
				"too and the positive control above is not a control", n)
		}
	}

	if !reported["src/App.tsx"] {
		t.Fatal("the ordinary control src/App.tsx produced no finding — this test would have passed " +
			"with scanning disabled entirely")
	}
}

// TestScanDirDepthAnswerIsUnchangedUnderTheShippedExtensionTable is the other
// half: with scannedExts as it actually ships, the #466 correction must be a
// measured no-op. #461 probed this against two built binaries; this pins it.
//
// It is an INVARIANT GUARD, not regression coverage — it was green before the
// correction too, which is precisely the property it exists to hold: the change
// removed a latent divergence and must not have moved live behaviour.
func TestScanDirDepthAnswerIsUnchangedUnderTheShippedExtensionTable(t *testing.T) {
	root := t.TempDir()
	for _, n := range pkgzip.KeptEnvFileNames() {
		writeFinding(t, root, n)
		writeFinding(t, root, "app/"+n)
	}
	writeFinding(t, root, "src/App.tsx")
	writeFinding(t, root, "app/lib.ts")

	findings, err := ScanDir(root)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}
	got := make([]string, 0, len(findings))
	for _, f := range findings {
		got = append(got, filepath.ToSlash(f.File))
	}
	// Literal expectation, written here rather than computed from the code under
	// test: the dotenv rows are invisible at BOTH depths because their extensions
	// are not scanned, and only the two source files are read.
	want := []string{"app/lib.ts", "src/App.tsx"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ScanDir reported %v, want %v — under the shipped scannedExts the kept dotenv names are "+
			"invisible at every depth, so #466's correction must not have changed what is scanned", got, want)
	}
}
