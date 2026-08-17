package pkgzip

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests drive the REAL Build walk, never the predicates directly.
//
// 🔴 THAT IS THE POINT, AND IT IS NOT A STYLE PREFERENCE. `Skipped` is produced
// BY the walk: a test that asks excludeFileReason("environment.env") what it
// returns proves the predicate works and says nothing about whether Build ever
// records the answer. A predicate-level suite is exactly how a green run ships a
// walk that collects nothing — the same shape as #442. Every assertion below
// therefore reads res.Skipped off a real tree on disk.

// skipKey renders one Skip for an assertion message: path, trailing slash for a
// directory, tag in parens. It is a TEST helper only — the production rendering
// lives in internal/cmd (renderSkippedLine) and must not gain a second copy
// here, so this deliberately does NOT implement the cap or the "… and K more".
func skipKey(s Skip) string {
	p := s.Path
	if s.Dir {
		p += "/"
	}
	if s.Rule != "" {
		p += " (" + s.Rule + ")"
	}
	return p
}

func skipKeys(skips []Skip) []string {
	out := make([]string, 0, len(skips))
	for _, s := range skips {
		out = append(out, skipKey(s))
	}
	return out
}

func findSkip(skips []Skip, path string) (Skip, bool) {
	for _, s := range skips {
		if s.Path == path {
			return s, true
		}
	}
	return Skip{}, false
}

// TestBuildReportsPatternMatchedFileDrop is the #435 headline case: `.env` is
// also Babylon.js's environment-texture format, so `public/environment.env` is
// dropped by the `*.env` suffix rule — and before drop-messaging the author
// found out at RUNTIME in the deployed app, because the submit output printed a
// file count and named nothing. The tag is what makes it recoverable: it says
// which rule matched, i.e. that renaming the file is the fix.
func TestBuildReportsPatternMatchedFileDrop(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "src/App.tsx", "export default 1")
	writeFile(t, dir, "public/environment.env", "TEXTURE")

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// CONTROL: the drop must really have happened, or an assertion about how it
	// is REPORTED is a fact about an empty list.
	for _, f := range res.Files {
		if f == "public/environment.env" {
			t.Fatal("CONTROL failure: public/environment.env is IN the bundle — " +
				"the *.env rule did not fire, so this test cannot be about reporting it")
		}
	}

	s, ok := findSkip(res.Skipped, "public/environment.env")
	if !ok {
		t.Fatalf("public/environment.env was dropped from the bundle and does NOT appear in Skipped %v — "+
			"that is the silent runtime break #435 exists to end", skipKeys(res.Skipped))
	}
	if s.Rule != RuleDotenvSuffix {
		t.Errorf("Skipped rule for public/environment.env = %q, want %q — the tag is what tells "+
			"the author WHICH rule matched, i.e. that renaming the file recovers it", s.Rule, RuleDotenvSuffix)
	}
	if s.Dir {
		t.Errorf("Skipped entry for public/environment.env has Dir=true, want false — it is a regular file")
	}
}

// TestBuildReportsExcludedDirectoryOnceUntagged pins the DECISION-POINT design.
//
// 🔴 ONE ENTRY, NOT ONE PER FILE. Build returns filepath.SkipDir at the subtree
// root, so the files beneath it are never enumerated and any count of them would
// be invented rather than measured. And no tag: `node_modules/` is a fixed name
// `app submit --help` already prints verbatim, so a tag there is noise on a line
// printed on every submit.
func TestBuildReportsExcludedDirectoryOnceUntagged(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "src/App.tsx", "export default 1")
	writeFile(t, dir, "node_modules/react/index.js", "x")
	writeFile(t, dir, "node_modules/react/lib/deep.js", "x")
	writeFile(t, dir, "node_modules/vite/index.js", "x")

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var hits int
	for _, s := range res.Skipped {
		if s.Path == "node_modules" {
			hits++
			if !s.Dir {
				t.Errorf("Skipped entry for node_modules has Dir=false, want true — the walk stopped at a directory")
			}
			if s.Rule != "" {
				t.Errorf("Skipped rule for node_modules = %q, want \"\" — a FIXED name carries no tag", s.Rule)
			}
		}
		if strings.HasPrefix(s.Path, "node_modules/") {
			t.Errorf("Skipped names %q, a CHILD of an excluded directory — the walk never descends, "+
				"so a per-file entry there is invented, not measured", s.Path)
		}
	}
	if hits != 1 {
		t.Errorf("node_modules appears %d time(s) in Skipped %v, want exactly 1 — "+
			"one skip DECISION POINT, not one entry per file under it", hits, skipKeys(res.Skipped))
	}
}

// TestBuildReportsPatternMatchedDirectoryDrops covers the two DIRECTORY pattern
// rules (#420): a dotenv-shaped directory and an archive-shaped one. Both carry
// a tag, because unlike `node_modules/` the name alone does not say which rule
// reached it — and for the dotenv directory the tag must NOT be the file rule's
// `.env*`, since `.envrc/` and `.env-backup/` are deliberately NOT dropped.
func TestBuildReportsPatternMatchedDirectoryDrops(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "src/App.tsx", "export default 1")
	writeFile(t, dir, ".env.d/credentials.json", "secret")
	writeFile(t, dir, "artifact.zip/payload.bin", "x")
	// A near-miss that must NOT be reported as skipped, because it is not:
	// a directory merely STARTING with ".env" still ships (isDotenvShaped
	// requires the dot). Its contents are ordinary bundle content.
	writeFile(t, dir, ".envoy/bootstrap.yaml", "x")

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, tc := range []struct {
		path string
		rule string
	}{
		{".env.d", RuleDotenvDir},
		{"artifact.zip", RuleArchive},
	} {
		s, ok := findSkip(res.Skipped, tc.path)
		if !ok {
			t.Errorf("%s/ was dropped whole and does NOT appear in Skipped %v", tc.path, skipKeys(res.Skipped))
			continue
		}
		if !s.Dir {
			t.Errorf("Skipped entry for %s has Dir=false, want true", tc.path)
		}
		if s.Rule != tc.rule {
			t.Errorf("Skipped rule for %s/ = %q, want %q", tc.path, s.Rule, tc.rule)
		}
	}

	// The near-miss: reporting it would be a claim that a subtree was dropped
	// when it was uploaded — the direction that costs an author a leak.
	if _, ok := findSkip(res.Skipped, ".envoy"); ok {
		t.Errorf(".envoy/ appears in Skipped %v, but it SHIPS — reporting a drop that did not happen "+
			"tells an author their content is safely out of the bundle when it is in it", skipKeys(res.Skipped))
	}
	var sawEnvoyContent bool
	for _, f := range res.Files {
		if f == ".envoy/bootstrap.yaml" {
			sawEnvoyContent = true
		}
	}
	if !sawEnvoyContent {
		t.Fatalf("CONTROL failure: .envoy/bootstrap.yaml is not in the bundle %v — the near-miss "+
			"assertion above is then vacuous", res.Files)
	}
}

// TestBuildReportsNonRegularEntries covers the third silent-drop class. It is
// not a name rule at all: Build drops every non-regular entry, and
// IsExcludedEntry's own doc comment records the measured wrong answer that a
// symlink's invisibility caused (#411 refused a submit over a path in no zip).
func TestBuildReportsNonRegularEntries(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "src/App.tsx", "export default 1")
	if err := os.Symlink("src/App.tsx", filepath.Join(dir, "alias.tsx")); err != nil {
		t.Skipf("symlinks unavailable on this filesystem: %v", err)
	}

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	s, ok := findSkip(res.Skipped, "alias.tsx")
	if !ok {
		t.Fatalf("the symlink alias.tsx was dropped and does NOT appear in Skipped %v", skipKeys(res.Skipped))
	}
	if s.Rule != RuleNonRegular {
		t.Errorf("Skipped rule for alias.tsx = %q, want %q", s.Rule, RuleNonRegular)
	}
	if s.Dir {
		t.Errorf("Skipped entry for alias.tsx has Dir=true, want false")
	}
}

// TestBuildSkippedOrderIsTaggedFirstThenPath pins the ORDER exactly, over one
// tree holding both groups. Tagged first because the tag is the actionable half
// (`public/environment.env (*.env)` is the line an author must not skim past);
// sorted within each group because the line is something tests and humans
// compare across runs, and filepath.WalkDir's lexical order is its behaviour,
// not its contract.
func TestBuildSkippedOrderIsTaggedFirstThenPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "src/App.tsx", "export default 1")
	// Untagged (fixed names), deliberately written in an order that is neither
	// sorted nor the reverse of it.
	writeFile(t, dir, "node_modules/react/index.js", "x")
	writeFile(t, dir, "dist/app.js", "x")
	writeFile(t, dir, "coverage/lcov.info", "x")
	// Tagged (pattern rules), same treatment.
	writeFile(t, dir, "public/environment.env", "TEXTURE")
	writeFile(t, dir, "prev-package.zip", "x")
	writeFile(t, dir, ".envrc", "export SECRET=1")

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	got := skipKeys(res.Skipped)
	want := []string{
		".envrc (.env*)",
		"prev-package.zip (*.zip)",
		"public/environment.env (*.env)",
		"coverage/",
		"dist/",
		"node_modules/",
	}
	if strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Errorf("Skipped order =\n  %s\nwant\n  %s\n\n"+
			"Tagged entries must come FIRST (the tag is the actionable half), then untagged, "+
			"each group sorted by path. A drifting order makes the printed line unpinnable.",
			strings.Join(got, " | "), strings.Join(want, " | "))
	}
}

// TestBuildSkippedIsEmptyWhenNothingIsExcluded is the negative case behind "if
// nothing was skipped, print nothing": the CLI's suppression is keyed on this
// list being empty, so an empty tree must really produce an empty list rather
// than a one-element list nobody looked at.
func TestBuildSkippedIsEmptyWhenNothingIsExcluded(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "src/App.tsx", "export default 1")
	writeFile(t, dir, "package.json", "{}")
	writeFile(t, dir, ".env.production", "VITE_BLOCK_ALLOWED_PARENT_ORIGINS=x")
	writeFile(t, dir, ".gitignore", "dist")

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(res.Skipped) != 0 {
		t.Errorf("Skipped = %v over a tree with nothing excluded, want empty", skipKeys(res.Skipped))
	}
	// CONTROL: the fixture must include the near-miss names, or "nothing was
	// excluded" is a fact about a tree with nothing IN it.
	if len(res.Files) != 5 {
		t.Errorf("bundle has %d file(s) %v, want all 5 — .env.production and .gitignore are KEPT, "+
			"and a fixture that lost them makes the empty Skipped vacuous", len(res.Files), res.Files)
	}
}

// TestSkippedEntriesAgreeWithTheStringSeam is an INVARIANT GUARD, not a
// regression test: it pins a relationship that has never been violated.
//
// IsExcludedPath / IsExcludedEntry reproduce Build's decisions rather than
// sharing them (Build walks a filesystem; they take strings), and
// TestIsExcludedPathAgreesWithBuild pins the INCLUDED set across that seam. This
// pins the other side of the same seam: everything Build reports as SKIPPED must
// also be excluded according to the string predicate. A Skipped list that named
// a path the predicate keeps would mean drop-messaging and the dirty-tree guard
// (#411) disagree about what is in the bundle.
func TestSkippedEntriesAgreeWithTheStringSeam(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "src/App.tsx", "export default 1")
	writeFile(t, dir, "node_modules/react/index.js", "x")
	writeFile(t, dir, "dist/app.js", "x")
	writeFile(t, dir, ".env.d/credentials.json", "secret")
	writeFile(t, dir, "artifact.zip/payload.bin", "x")
	writeFile(t, dir, "public/environment.env", "TEXTURE")
	writeFile(t, dir, ".envrc", "export SECRET=1")
	writeFile(t, dir, "prev-package.zip", "x")
	writeFile(t, dir, "sub/.git", "gitdir: /elsewhere")

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(res.Skipped) == 0 {
		t.Fatal("CONTROL failure: nothing was skipped over a tree built to exclude eight things — " +
			"an agreement over an empty list is vacuous")
	}
	for _, s := range res.Skipped {
		p := s.Path
		if s.Dir {
			p += "/"
		}
		if s.Rule == RuleNonRegular {
			// A string predicate cannot see a file type; IsExcludedEntry is the
			// one that can, and this fixture plants no non-regular entry.
			continue
		}
		if !IsExcludedPath(p) {
			t.Errorf("Build skipped %q but IsExcludedPath(%q) = false — drop-messaging and the "+
				"dirty-tree guard (#411) disagree about what reaches the bundle", skipKey(s), p)
		}
	}
}

// TestBuildSkippedRulesAreOnlyTheDeclaredTags is an INVARIANT GUARD. It stops a new rule inventing an
// ad-hoc tag string that no consumer, README line or test knows about: every tag
// Build produces must be one of the exported constants (or empty).
func TestBuildSkippedRulesAreOnlyTheDeclaredTags(t *testing.T) {
	declared := map[string]struct{}{
		"":               {},
		RuleArchive:      {},
		RuleDotenvPrefix: {},
		RuleDotenvSuffix: {},
		RuleDotenvDir:    {},
		RuleNonRegular:   {},
	}
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "src/App.tsx", "export default 1")
	writeFile(t, dir, "node_modules/react/index.js", "x")
	writeFile(t, dir, ".env.d/credentials.json", "secret")
	writeFile(t, dir, "artifact.zip/payload.bin", "x")
	writeFile(t, dir, "public/environment.env", "TEXTURE")
	writeFile(t, dir, ".envrc", "export SECRET=1")
	writeFile(t, dir, "prev-package.zip", "x")
	writeFile(t, dir, "sub/.git", "gitdir: /elsewhere")

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	seen := map[string]struct{}{}
	for _, s := range res.Skipped {
		if _, ok := declared[s.Rule]; !ok {
			t.Errorf("Skipped entry %q carries an undeclared rule tag %q — add a Rule* constant "+
				"so the README and the CLI's line can name it", s.Path, s.Rule)
		}
		seen[s.Rule] = struct{}{}
	}
	// POSITIVE CONTROL: the fixture must exercise more than one tag, or an
	// "every tag is declared" pass says nothing.
	if len(seen) < 4 {
		t.Errorf("the fixture produced only %d distinct rule tag(s) %v — too few for this to be "+
			"a claim about the tag set", len(seen), seen)
	}
}

// TestBuildSkippedIsDeterministicAcrossRuns is an INVARIANT GUARD: it pins that two Builds of one tree
// produce the identical list — the property the printed line's testability rests
// on, and the one a map-ordered implementation would break intermittently.
func TestBuildSkippedIsDeterministicAcrossRuns(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "src/App.tsx", "export default 1")
	for i := 0; i < 8; i++ {
		writeFile(t, dir, fmt.Sprintf("pkg%d/asset.env", i), "TEXTURE")
		writeFile(t, dir, fmt.Sprintf("out%d/dist/app.js", i), "x")
	}

	first, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	second, err := Build(dir)
	if err != nil {
		t.Fatalf("Build (second): %v", err)
	}
	a, b := skipKeys(first.Skipped), skipKeys(second.Skipped)
	if strings.Join(a, " | ") != strings.Join(b, " | ") {
		t.Errorf("two Builds of one tree produced different Skipped lists:\n  %s\n  %s",
			strings.Join(a, " | "), strings.Join(b, " | "))
	}
	// CONTROL: the list must be long enough for an ordering difference to be
	// observable at all.
	if len(a) < 8 {
		t.Fatalf("only %d skip(s) %v — too few for a determinism claim", len(a), a)
	}
}
