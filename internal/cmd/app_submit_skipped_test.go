package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/pkgzip"
)

// Issue #435, part 2: `app submit` printed a file COUNT and never named what it
// left out. Three classes were dropped silently — an excluded directory, a
// pattern-matched file, and any non-regular entry — and the cost case is
// `public/environment.env`: `.env` is also Babylon.js's environment-texture
// format, so a 3D block loses its texture and finds out at RUNTIME in the
// deployed app.
//
// The exclusion RULES are unchanged by this and must stay so; what changed is
// that the packager now says what it dropped.

// skippedLineRe-free by design: these tests read the whole line and compare it,
// because the line IS the contract. A substring match would pass on a line that
// named the right path with the wrong count beside it.
func skippedLineOf(t *testing.T, stdout string) string {
	t.Helper()
	for _, l := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(l, "Skipped ") {
			return l
		}
	}
	return ""
}

// submitPackageOnly runs the real command through the real root, and returns
// stdout. Driving the command (not renderSkippedLine directly) is what makes
// these tests evidence that the line is REACHED on the --package-only path —
// a renderer that is never called still renders perfectly in isolation.
func submitPackageOnly(t *testing.T, dir string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "bundle.zip")
	stdout, stderr, err := run(t, "app", "submit", dir, "--package-only", "--skip-validate", "--out", out)
	if err != nil {
		t.Fatalf("app submit --package-only: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Packaged ") {
		t.Fatalf("CONTROL failure: no `Packaged …` line in stdout, so the run did not reach the "+
			"printing code at all:\n%s", stdout)
	}
	return stdout
}

// TestSubmitPrintsSkippedLine is the end-to-end regression test: the real
// command, over a tree holding one of each silent-drop class, must print one
// line naming all three.
func TestSubmitPrintsSkippedLine(t *testing.T) {
	dir := t.TempDir()
	writeStaticManifest(t, dir)
	mustWrite(t, dir, "src/App.tsx", "export default 1")
	mustWrite(t, dir, "public/environment.env", "TEXTURE")
	mustWrite(t, dir, "node_modules/react/index.js", "x")
	mustWrite(t, dir, "node_modules/react/lib/deep.js", "x")
	symlinked := true
	if err := os.Symlink("src/App.tsx", filepath.Join(dir, "alias.tsx")); err != nil {
		symlinked = false
		t.Logf("symlinks unavailable here (%v) — the non-regular class is unobservable in this run", err)
	}

	stdout := submitPackageOnly(t, dir)
	line := skippedLineOf(t, stdout)
	if line == "" {
		t.Fatalf("no `Skipped …` line printed although three classes were dropped:\n%s", stdout)
	}

	if !symlinked {
		// Without the symlink the exact-line assertion below cannot hold; assert
		// the two classes that ARE observable rather than skipping silently.
		for _, want := range []string{"public/environment.env (*.env)", "node_modules/"} {
			if !strings.Contains(line, want) {
				t.Errorf("Skipped line %q does not name %q", line, want)
			}
		}
		return
	}

	want := "Skipped 3 path(s): alias.tsx (not a regular file), public/environment.env (*.env), node_modules/"
	if line != want {
		t.Errorf("Skipped line =\n  %s\nwant\n  %s\n\n"+
			"This line is the whole of #435 part 2: a bundle that silently loses "+
			"public/environment.env breaks at RUNTIME in the deployed app, and the (*.env) tag "+
			"is what tells the author renaming it is the fix.", line, want)
	}

	// The children of the excluded directory must NOT be listed individually —
	// the walk never descends, so naming them would be invented, not measured.
	if strings.Contains(line, "node_modules/react") {
		t.Errorf("Skipped line names a CHILD of an excluded directory: %q", line)
	}
}

// TestSubmitPrintsNoSkippedLineWhenNothingIsSkipped pins the empty case: no
// "Skipped 0 path(s)". A zero-line on a clean project is noise on every submit,
// and noise is what makes the non-zero line get skimmed.
//
// 🔴 THIS IS AN INVARIANT GUARD, NOT A REGRESSION TEST, AND IT IS LABELLED THAT
// WAY BECAUSE IT WAS MEASURED: run verbatim against origin/main it PASSES —
// trivially, because that build prints no such line under any circumstances. It
// pins a property this change must not break going forward; it is not evidence
// that this change fixed anything. TestSubmitPrintsSkippedLine is the one that
// goes red at the base.
func TestSubmitPrintsNoSkippedLineWhenNothingIsSkipped(t *testing.T) {
	dir := t.TempDir()
	writeStaticManifest(t, dir)
	mustWrite(t, dir, "src/App.tsx", "export default 1")
	mustWrite(t, dir, "package.json", "{}")
	// KEPT names, so the fixture is not merely empty.
	mustWrite(t, dir, ".env.production", "VITE_BLOCK_ALLOWED_PARENT_ORIGINS=x")
	mustWrite(t, dir, ".gitignore", "dist")

	stdout := submitPackageOnly(t, dir)
	if line := skippedLineOf(t, stdout); line != "" {
		t.Errorf("printed %q over a project with nothing excluded — the empty case must print nothing:\n%s",
			line, stdout)
	}
	// 🔴 NOT `strings.Contains(stdout, "Skipped")` — that spelled guard FAILED
	// here for a reason that has nothing to do with the code: t.TempDir() names
	// its directory after the TEST, so the word appears in the `Wrote canonical
	// bundle:` path. A guard on a WORD is walkable by another feature spelling
	// it. Assert the STRUCTURE instead: no line may begin with the prefix.
	for _, l := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(l, "Skipped") {
			t.Errorf("line %q begins with the skip prefix although nothing was skipped:\n%s", l, stdout)
		}
	}
}

// TestSubmitSkippedLineTruncatesKeepingTheTotal covers the monorepo tail-guard:
// past the cap the LIST is elided but the COUNT is not. A truncation that also
// truncated the count would understate the loss, which is the one direction this
// message must never fail in.
//
// 🔴 EVERY EXPECTATION HERE IS A LITERAL, NOT skippedListCap, AND THAT IS A
// MEASURED FIX RATHER THAN A STYLE CHOICE. The first version of this test
// derived its numbers from the constant — cap names shown, total-cap in the
// tail — so the mutation `skippedListCap: 12 → 3` SURVIVED a fully green run:
// the assertion moved with the mutant and could not see it. A test whose
// expectation is computed from the value under test cannot test that value.
// 20 dropped paths, 12 shown, 8 elided, written out. Changing the cap therefore
// turns this red on purpose: the cap is a user-facing contract, and moving it
// is a re-approval, not a refactor.
func TestSubmitSkippedLineTruncatesKeepingTheTotal(t *testing.T) {
	dir := t.TempDir()
	writeStaticManifest(t, dir)
	mustWrite(t, dir, "src/App.tsx", "export default 1")
	// 20 tagged drops — comfortably past the cap, all of one class so the
	// ordering within the tagged group is a plain path sort.
	for i := 0; i < 20; i++ {
		mustWrite(t, dir, fmt.Sprintf("assets/tex%02d.env", i), "TEXTURE")
	}

	stdout := submitPackageOnly(t, dir)
	line := skippedLineOf(t, stdout)
	if line == "" {
		t.Fatalf("no `Skipped …` line printed:\n%s", stdout)
	}

	// The COUNT is of every decision, not of the shown ones.
	const prefix = "Skipped 20 path(s): "
	if !strings.HasPrefix(line, prefix) {
		t.Errorf("Skipped line = %q, want it to start %q — the count must stay accurate under truncation",
			line, prefix)
	}
	const tail = "… and 8 more"
	if !strings.HasSuffix(line, tail) {
		t.Errorf("Skipped line = %q, want it to end %q", line, tail)
	}
	// Exactly 12 names are shown.
	body := strings.TrimSuffix(strings.TrimPrefix(line, prefix), " "+tail)
	if n := len(strings.Split(body, ", ")); n != 12 {
		t.Errorf("Skipped line lists %d name(s), want 12 (the cap):\n  %s", n, line)
	}
	// And they are the FIRST twelve in the pinned order, not an arbitrary
	// subset — a truncation that dropped the wrong end would still satisfy the
	// count assertions above. Written out whole: this is the one assertion that
	// can see both the cap and the ordering at once.
	want := make([]string, 0, 12)
	for i := 0; i < 12; i++ {
		want = append(want, fmt.Sprintf("assets/tex%02d.env (*.env)", i))
	}
	if body != strings.Join(want, ", ") {
		t.Errorf("truncated list =\n  %s\nwant\n  %s", body, strings.Join(want, ", "))
	}
	// CONTROL: the count field parses, and equals the number of files planted.
	countStr := strings.Fields(line)[1]
	if n, err := strconv.Atoi(countStr); err != nil || n != 20 {
		t.Errorf("count field = %q (parsed %v, err %v), want 20", countStr, n, err)
	}
}

// TestRenderSkippedLineIsTheOnlyRenderer is a unit-level pin on the renderer's
// edge cases, which a walk cannot cheaply produce: an empty list, and a list
// exactly AT the cap (where nothing may be elided and no "… and 0 more" tail
// may appear). It is an INVARIANT GUARD, not a regression test — no shipped
// version ever had these behaviours to regress from.
func TestRenderSkippedLineIsTheOnlyRenderer(t *testing.T) {
	if got := renderSkippedLine(nil); got != "" {
		t.Errorf("renderSkippedLine(nil) = %q, want \"\"", got)
	}
	if got := renderSkippedLine([]pkgzip.Skip{}); got != "" {
		t.Errorf("renderSkippedLine(empty) = %q, want \"\"", got)
	}

	// Exactly at the cap: no tail.
	atCap := make([]pkgzip.Skip, 0, skippedListCap)
	for i := 0; i < skippedListCap; i++ {
		atCap = append(atCap, pkgzip.Skip{Path: fmt.Sprintf("d%02d", i), Dir: true})
	}
	got := renderSkippedLine(atCap)
	if strings.Contains(got, "more") {
		t.Errorf("a list exactly at the cap was truncated: %q", got)
	}
	if !strings.HasPrefix(got, fmt.Sprintf("Skipped %d path(s): ", skippedListCap)) {
		t.Errorf("line = %q, want the cap-sized count in the prefix", got)
	}

	// One past the cap: a tail of exactly 1.
	got = renderSkippedLine(append(atCap, pkgzip.Skip{Path: "zz", Dir: true}))
	if !strings.HasSuffix(got, "… and 1 more") {
		t.Errorf("line = %q, want it to end \"… and 1 more\"", got)
	}
}

// mustWrite writes rel (slash-separated) under dir, creating parents.
func mustWrite(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
