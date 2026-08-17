package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// THE PUBLISH-FLOOR LEDGER — one definition, and a guard that notices a second.
//
// `floorMet` is THE publish-floor predicate (app_listing.go). It was open-coded
// at four sites; #437 folded in three and #440 the fourth. The behavioural cases
// that pin those four sites — TestAppListingStatusRendersFloor,
// TestSetMediaBelowFloorOnLiveListing, TestAppListingReorderBelowFloorOnLiveListing,
// TestAppListingSubmitRevisionBelowFloorStillFails, TestAppListingSetIconDraft —
// each go red when its own site is inverted. Not one of them goes red when a
// FIFTH site is added, because a new site is new code that no existing case
// reaches. That is what this ledger is for.
//
// Why it is worth a file. Two of `floorMet`'s callers answer this one question
// with OPPOSITE outcomes on purpose (AGENTS.md item 30): below the floor is
// PROGRESS on the attach/reorder path and a FAILURE on `submit-revision`. A
// predicate whose call sites are meant to disagree is precisely the one where a
// re-open-coded copy looks like a deliberate divergence and reads as normal in
// review.
//
// 🔴 IT IS A COUNT ASSERTION OVER THE WHOLE MODULE, NOT A HAND-MAINTAINED LIST
// OF SITES. A list covers the sites somebody thought of. The rule here is
// structural: across every non-test .go file in the module, a single line may
// mention BOTH `…Icon.Present()` and `…Cover.Present()` exactly once — inside
// `floorMet`'s own body — and no line may conjoin an icon-ish with a cover-ish
// local. The expected direct count is ONE rather than zero on purpose: it is the
// positive control riding on real data. A zero there would mean the scanner
// stopped seeing the definition it is calibrated against — a wrong pattern, a
// renamed method, a walk rooted at the wrong directory — and a scanner that sees
// nothing reports "no offenders" just as loudly as a clean tree does.
//
// It is line-level rather than `&&`-level so the De Morgan spelling
// (`!Icon.Present() || !Cover.Present()`) cannot walk it by rewording.
//
// 🔴 WHAT IT CANNOT SEE, stated rather than waved at: a conjunction split across
// two lines; a copy routed through further locals whose names contain neither
// "icon" nor "cover"; and the floor asked at the field level
// (`Assets.Icon.ImageID != nil && …`), bypassing `Present()` entirely. The
// local-name shape IS covered because it is the cheapest evasion — two sites in
// app_listing.go already compute `iconOK`/`coverOK` for their per-slot messages
// and would only have to conjoin them.

// floorSlotCall matches a call of `.Present()` on the Icon or Cover asset,
// capturing which slot. The identifier class allows `.`, `[`, `]` and `*` so a
// selector, an index and a pointer deref are all caught.
var floorSlotCall = regexp.MustCompile(`[A-Za-z0-9_.\[\]*]*\.(Icon|Cover)\.Present\(\)`)

// floorLocalConjunction matches an icon-ish local conjoined with a cover-ish
// local in either order. The identifier class excludes `.`, so it cannot also
// match the selector shape floorSlotCall handles — the two classes are disjoint.
var floorLocalConjunction = regexp.MustCompile(`(?i)\b([a-z0-9_]*icon[a-z0-9_]*)\s*&&\s*([a-z0-9_]*cover[a-z0-9_]*)\b|` +
	`\b([a-z0-9_]*cover[a-z0-9_]*)\s*&&\s*([a-z0-9_]*icon[a-z0-9_]*)\b`)

// floorHit is one line that asks the publish-floor question.
type floorHit struct {
	file string
	line int // 1-based
	text string
}

// scanFloorQuestion returns every line of src that asks the publish-floor
// question directly (both slots' Present() on one line) and every line that asks
// it through locals. Full-line comments are skipped: this file and app_listing.go
// both discuss the shape in prose, and a guard that flags its own documentation
// is a guard people delete.
func scanFloorQuestion(file, src string) (direct, viaLocals []floorHit) {
	for i, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		slots := map[string]bool{}
		for _, m := range floorSlotCall.FindAllStringSubmatch(line, -1) {
			slots[m[1]] = true
		}
		if slots["Icon"] && slots["Cover"] {
			direct = append(direct, floorHit{file, i + 1, trimmed})
		}
		if floorLocalConjunction.MatchString(line) {
			viaLocals = append(viaLocals, floorHit{file, i + 1, trimmed})
		}
	}
	return direct, viaLocals
}

// moduleGoFilesFromCmd returns every non-test .go file in the module, relative
// to the module root, sorted. Tests run with cwd = internal/cmd, so the walk is
// rooted two levels up: scoping it to this package would let the predicate be
// re-open-coded in appapi and stay invisible.
func moduleGoFilesFromCmd(t *testing.T) (root string, files []string) {
	t.Helper()
	root = filepath.Join("..", "..")
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("module root not where expected (%s): %v", root, err)
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "node_modules" || name == "dist" || name == "bin") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		t.Fatalf("walk module: %v", err)
	}
	sort.Strings(files)
	return root, files
}

// TestPublishFloorHasOneDefinition asserts the module-wide count: exactly one
// line asks the floor question directly and it is inside floorMet, and no line
// asks it through locals.
func TestPublishFloorHasOneDefinition(t *testing.T) {
	root, files := moduleGoFilesFromCmd(t)
	if len(files) < 20 {
		t.Fatalf("walked only %d non-test .go files — the scan is measuring the wrong tree", len(files))
	}

	var direct, viaLocals []floorHit
	for _, rel := range files {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		d, l := scanFloorQuestion(rel, string(b))
		direct = append(direct, d...)
		viaLocals = append(viaLocals, l...)
	}

	// The positive control, on real data: the definition itself must be found.
	// Report the pair — a non-zero here is what makes the zero below mean
	// something.
	t.Logf("scanned %d non-test .go files; direct floor-question lines: %d; via-locals: %d", len(files), len(direct), len(viaLocals))

	if len(direct) != 1 {
		t.Fatalf("expected exactly ONE line asking the publish-floor question directly (floorMet's own body); got %d: %+v\n"+
			"A second one is the predicate re-open-coded. Call floorMet instead — see its doc comment and AGENTS.md item 30.", len(direct), direct)
	}
	if len(viaLocals) != 0 {
		t.Fatalf("the publish-floor question is conjoined from locals at %d site(s): %+v\n"+
			"Call floorMet instead — see its doc comment and AGENTS.md item 30.", len(viaLocals), viaLocals)
	}

	hit := direct[0]
	wantFile := filepath.Join("internal", "cmd", "app_listing.go")
	if hit.file != wantFile {
		t.Fatalf("the one direct floor question is in %s, not %s: %+v", hit.file, wantFile, hit)
	}
	lo, hi := floorMetLineRange(t, filepath.Join(root, hit.file))
	if hit.line < lo || hit.line > hi {
		t.Fatalf("the one direct floor question is at %s:%d, outside floorMet's body (lines %d–%d): %q\n"+
			"Either the definition moved or this is a second copy.", hit.file, hit.line, lo, hi, hit.text)
	}
}

// floorMetLineRange returns the 1-based line span of func floorMet in path. It
// fails rather than returning a zero range when the function is absent, so a
// rename cannot turn this guard into a vacuous pass.
func floorMetLineRange(t *testing.T, path string) (lo, hi int) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "floorMet" || fn.Recv != nil {
			continue
		}
		return fset.Position(fn.Pos()).Line, fset.Position(fn.End()).Line
	}
	t.Fatalf("no func floorMet in %s — the ledger is calibrated against a definition that no longer exists", path)
	return 0, 0
}

// TestFloorQuestionScannerSeesShapesNotInTree is the calibration. Every case is
// a shape the tree does NOT contain, because a control built from the shape you
// already handle proves nothing.
func TestFloorQuestionScannerSeesShapesNotInTree(t *testing.T) {
	flagged := []struct {
		name, src string
		local     bool
	}{
		{"reversed order", "\tif v.Assets.Cover.Present() && v.Assets.Icon.Present() {\n", false},
		{"de morgan", "\tif !v.Assets.Icon.Present() || !v.Assets.Cover.Present() {\n", false},
		{"different receiver, pointer deref", "\treturn (*ed).Assets.Icon.Present() && ed.Assets.Cover.Present()\n", false},
		{"indexed receiver", "\tok := rows[i].Assets.Icon.Present() && rows[i].Assets.Cover.Present()\n", false},
		{"locals conjoined", "\tif iconOK && coverOK {\n", true},
		{"locals conjoined, reversed and renamed", "\tif hasCover && hasIcon {\n", true},
	}
	for _, tc := range flagged {
		t.Run(tc.name, func(t *testing.T) {
			direct, viaLocals := scanFloorQuestion("x.go", tc.src)
			got := len(direct)
			if tc.local {
				got = len(viaLocals)
			}
			if got != 1 {
				t.Fatalf("scanner did not flag %q (direct=%d viaLocals=%d) — it cannot see this shape", tc.src, len(direct), len(viaLocals))
			}
		})
	}

	clean := []struct{ name, src string }{
		{"one slot only", "\tif v.Assets.Icon.Present() {\n"},
		{"two slots on separate lines", "\ticonOK := v.Assets.Icon.Present()\n\tcoverOK := v.Assets.Cover.Present()\n"},
		{"the shape quoted in a comment", "\t// view.Assets.Icon.Present() && view.Assets.Cover.Present()\n"},
		{"an unrelated conjunction", "\tif ok && done {\n"},
	}
	for _, tc := range clean {
		t.Run(tc.name, func(t *testing.T) {
			direct, viaLocals := scanFloorQuestion("x.go", tc.src)
			if len(direct) != 0 || len(viaLocals) != 0 {
				t.Fatalf("scanner flagged clean source %q (direct=%+v viaLocals=%+v)", tc.src, direct, viaLocals)
			}
		})
	}
}
