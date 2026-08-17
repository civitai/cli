package antipattern

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/pkgzip"
)

// findingLine is a line that fires the buzz-rest rule (a REMOVED REST route).
// It is the same shape in every fixture below so a missing finding can only mean
// the file was never scanned, never that the rule failed to match it.
const findingLine = `const bal = await fetch('/api/v1/blocks/buzz');` + "\n"

// writeFinding creates <root>/<rel> and fills it with a line that MUST produce a
// finding if the scanner reads the file at all.
func writeFinding(t *testing.T, root, rel string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(p, []byte(findingLine), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// reportedDirs is the set of top-level directory names ScanDir produced a
// finding under, plus "" for findings at the root.
func reportedDirs(findings []Finding) map[string]bool {
	out := map[string]bool{}
	for _, f := range findings {
		parts := strings.Split(filepath.ToSlash(f.File), "/")
		if len(parts) == 1 {
			out[""] = true
			continue
		}
		out[parts[0]] = true
	}
	return out
}

// TestScanDirSkipsExactlyWhatThePackagerDrops is the SEAM GUARD for issue #435
// part 3: this package must keep NO exclusion list of its own, so the set of
// directories it refuses to descend into IS pkgzip's.
//
// 🔴 IT PINS A RELATIONSHIP, NOT A SNAPSHOT OF NAMES. The roster is built FROM
// pkgzip.ExcludedNames(), so a fixed name added or removed on the pkgzip side
// enters or leaves this test's tree automatically and is re-asserted against
// pkgzip's own predicate — the two cannot drift apart silently the way #409 and
// #420 did. Pattern-shaped names (dotenv, archive) have no enumeration to read,
// so they are listed by hand, exactly as pkgzip's own
// TestDirectoryRuleIsDocumentedForAuthors roster is.
//
// 🔴 THE ORACLE HALF CANNOT SEE A MUTANT THAT MOVES BOTH SIDES TOGETHER — a
// pkgzip.IsExcluded that returned true for everything would agree with a
// ScanDir that scanned nothing, and the whole test would pass while the scanner
// was inert. So there is a second half whose expectations are LITERAL: a fixed
// table of names, written down here, not computed from anything under test.
func TestScanDirSkipsExactlyWhatThePackagerDrops(t *testing.T) {
	// Pattern-shaped directory names (pkgzip's isDotenvShaped / isArchiveArtifact,
	// both case-insensitive) plus the documented lookalikes that are NOT dropped.
	patternNames := []string{
		".env", ".env.d", ".ENV.D", ".env.local", ".env.production",
		"old.zip", "X.ZIP",
		// Deliberately NOT excluded by pkgzip — the residual its isDotenvShaped
		// doc calls out. They must still be SCANNED, which is what makes this
		// test fail if antipattern ever widens BEYOND pkgzip.
		".envrc", ".env-backup", ".envs", ".environment",
	}
	// Ordinary source directories: the positive control. Without these the suite
	// cannot tell this change from one that disables scanning outright.
	ordinaryNames := []string{"src", "public", "lib", ".github", "components"}

	roster := append([]string{}, pkgzip.ExcludedNames()...)
	roster = append(roster, patternNames...)
	roster = append(roster, ordinaryNames...)

	root := t.TempDir()
	for _, name := range roster {
		writeFinding(t, root, name+"/probe.ts")
	}
	// A finding at the root itself — nothing may skip that.
	writeFinding(t, root, "probe.ts")

	findings, err := ScanDir(root)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}
	got := reportedDirs(findings)

	// ── half 1: the relationship, against pkgzip's own predicate ──────────────
	var skipped, scanned int
	for _, name := range roster {
		wantReported := !pkgzip.IsExcluded(name)
		if got[name] != wantReported {
			verb := "did NOT report"
			if got[name] {
				verb = "REPORTED"
			}
			t.Errorf("ScanDir %s a finding under %q, but pkgzip.IsExcluded(%q)=%v — "+
				"antipattern's skipped set must BE pkgzip's, not a copy of it (#435)",
				verb, name, name, pkgzip.IsExcluded(name))
		}
		if wantReported {
			scanned++
		} else {
			skipped++
		}
	}
	// Both arms must be non-empty, or "they agree" is a claim about nothing.
	if skipped == 0 {
		t.Fatal("no roster name was excluded — this test observed only the scanned arm")
	}
	if scanned == 0 {
		t.Fatal("no roster name was scanned — this test observed only the skipped arm")
	}
	if !got[""] {
		t.Error("no finding at the scanned root — the walk never read a file there")
	}

	// ── half 2: literal expectations, derived from NOTHING under test ─────────
	// If pkgzip.IsExcluded were mutated to answer the same wrong thing ScanDir
	// answers, half 1 stays green and this half does not.
	literal := map[string]bool{ // name -> must a finding under it be REPORTED?
		".venv":        false,
		".pnpm-store":  false,
		".turbo":       false,
		".cache":       false,
		"node_modules": false,
		".env.d":       false,
		".ENV.D":       false,
		"X.ZIP":        false,
		"src":          true,
		"public":       true,
		".envrc":       true,
		".environment": true,
	}
	for name, wantReported := range literal {
		if got[name] != wantReported {
			t.Errorf("finding under %q reported=%v, want %v (literal expectation, not computed)",
				name, got[name], wantReported)
		}
	}

	// The roster must cover every fixed pkgzip name, so a name added there is
	// automatically asserted here rather than quietly untested.
	for _, name := range pkgzip.ExcludedNames() {
		if !containsStr(roster, name) {
			t.Errorf("pkgzip excludes %q but the roster does not carry it", name)
		}
	}
	if len(pkgzip.ExcludedNames()) < 10 {
		t.Fatalf("pkgzip.ExcludedNames() returned %d names — too few to be the real list; "+
			"the roster was built from an empty or broken enumeration",
			len(pkgzip.ExcludedNames()))
	}
}

// TestScanDirSkipsFilesThePackagerDropsByName covers the FILE half of the same
// rule. scannedExts hides most of the divergence by accident — `.envrc`,
// `db.env` and `old.zip` all have extensions it does not scan — but `.env.json`
// has one it does, so without pkgzip.IsExcludedFile the scanner reports findings
// in a file the packager drops.
//
// The control is in the SAME directory with the SAME content and a name nothing
// excludes: if it also vanished, the test would be pinning "scanning is off"
// rather than "this name is dropped".
func TestScanDirSkipsFilesThePackagerDropsByName(t *testing.T) {
	root := t.TempDir()
	dropped := []string{".env.json", ".env.local.json", ".ENV.json", ".env.staging.md"}
	for _, name := range dropped {
		writeFinding(t, root, "src/"+name)
	}
	writeFinding(t, root, "src/config.json") // control: an ordinary scanned file

	findings, err := ScanDir(root)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}
	byBase := map[string]bool{}
	for _, f := range findings {
		byBase[filepath.Base(f.File)] = true
	}

	for _, name := range dropped {
		if !pkgzip.IsExcludedFile(name) {
			t.Fatalf("fixture error: pkgzip does NOT drop %q, so this case proves nothing", name)
		}
		if byBase[name] {
			t.Errorf("ScanDir reported a finding in %q, which pkgzip.IsExcludedFile drops — "+
				"a finding must name a file that ships (#435)", name)
		}
	}
	if !byBase["config.json"] {
		t.Fatal("the control file src/config.json produced no finding — this test would have " +
			"passed with scanning disabled entirely")
	}
}

// TestScanDirReportsOrdinarySourceFindings is the standalone positive control:
// the whole rest of this file asserts ABSENCES, and an absence is what a
// scanner wired to nothing produces too.
func TestScanDirReportsOrdinarySourceFindings(t *testing.T) {
	root := t.TempDir()
	writeFinding(t, root, "src/App.tsx")
	writeFinding(t, root, "src/lib/api.ts")
	writeFinding(t, root, "README.md")

	findings, err := ScanDir(root)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}
	if len(findings) != 3 {
		t.Fatalf("got %d finding(s), want 3:\n%s", len(findings), Format(findings))
	}
	for _, f := range findings {
		if f.Rule.ID != "buzz-rest" {
			t.Errorf("finding %s:%d fired rule %q, want buzz-rest", f.File, f.Line, f.Rule.ID)
		}
	}
}

// TestScanDirNeverSkipsTheScannedRoot pins the `path == dir` guard, mirroring
// pkgzip.Build's own. Widening the rule to pkgzip's set made this reachable: a
// project directory named `dist`, `build` or `x.zip` would otherwise scan
// NOTHING and report a clean tree — a reassuring zero from a walk that never
// opened a file. The caller NAMED this tree, so it is the subject of the scan,
// not content found inside one.
func TestScanDirNeverSkipsTheScannedRoot(t *testing.T) {
	for _, rootName := range []string{"dist", "build", "node_modules", ".env.d", "app.zip"} {
		t.Run(rootName, func(t *testing.T) {
			if !pkgzip.IsExcluded(rootName) {
				t.Fatalf("fixture error: pkgzip does not exclude %q, so this case proves nothing", rootName)
			}
			root := filepath.Join(t.TempDir(), rootName)
			writeFinding(t, root, "src/App.tsx")

			findings, err := ScanDir(root)
			if err != nil {
				t.Fatalf("ScanDir: %v", err)
			}
			if len(findings) != 1 {
				t.Fatalf("scanning a root named %q produced %d finding(s), want 1 — the root "+
					"must never be skipped by the directory rule", rootName, len(findings))
			}
		})
	}
}

// TestAntipatternKeepsNoExclusionListOfItsOwn is the structural half of the seam
// guard: the behavioural test above proves the two AGREE today, and this proves
// there is nothing here that COULD disagree tomorrow.
//
// It walks the package's own non-test sources and fails on any string literal
// that pkgzip would exclude — the shape a reintroduced `skipDirs` (or a slice,
// or a switch) inevitably takes. It is not a grep: the AST sees literals
// wherever they are written, and ignores comments, which is why the doc comment
// above ScanDir may name the eight dead entries without tripping it.
func TestAntipatternKeepsNoExclusionListOfItsOwn(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	sort.Strings(files)

	examined, literals := 0, 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		examined++
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		bad, n := rivalExclusionLiterals(t, f, string(src))
		literals += n
		for _, b := range bad {
			t.Errorf("%s declares the string literal %q, which pkgzip already excludes — "+
				"this package must delegate to pkgzip.IsExcluded / pkgzip.IsExcludedFile, "+
				"not keep a rival list (#435)", f, b)
		}
	}

	// Positive controls: a zero here must mean "no rival literal", never "the
	// detector read nothing".
	if examined == 0 {
		t.Fatal("examined 0 non-test source files — the glob observed nothing")
	}
	if literals < 10 {
		t.Fatalf("examined only %d string literal(s) across %d file(s) — the AST walk is not "+
			"reading the source", literals, examined)
	}
	// …and the detector must be able to FIRE. This source is not the code under
	// test and its expectation is written here, literally.
	planted := "package antipattern\n\nvar skipDirs = map[string]bool{\"node_modules\": true, \".git\": true}\n"
	bad, _ := rivalExclusionLiterals(t, "planted.go", planted)
	if len(bad) != 2 {
		t.Fatalf("the detector found %v in a source that plants two rival names — it cannot "+
			"observe the thing this test reports zero of", bad)
	}
}

// rivalExclusionLiterals returns every string literal in src that pkgzip already
// excludes (as a directory or as a file), plus the total number of string
// literals it examined.
func rivalExclusionLiterals(t *testing.T, name, src string) ([]string, int) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	var bad []string
	total := 0
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		total++
		v, err := strconv.Unquote(lit.Value)
		if err != nil || v == "" {
			return true
		}
		if pkgzip.IsExcluded(v) || pkgzip.IsExcludedFile(v) {
			bad = append(bad, v)
		}
		return true
	})
	sort.Strings(bad)
	return bad, total
}

func containsStr(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
