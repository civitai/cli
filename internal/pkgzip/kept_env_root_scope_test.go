package pkgzip

import (
	"sort"
	"strings"
	"testing"
)

// TESTS FOR THE ROOT SCOPE OF keptEnvFiles (issue #435, one row of the residual).
//
// The allow-list keeps `.env.example`, `.env.sample` and `.env.production`
// despite the `.env` catch-all, and both of its justifications are about the
// PROJECT ROOT: `vite build` reads env files from `envDir`, which defaults to
// the project root, and a template a reviewer reads is the one at the root.
// Applied by base name at any depth, the exemption reached files that can be
// neither — measured on a binary built at 8617a68, a planted
// `.env-backup/.env.production` holding a secret was packaged into a bundle that
// is committed to Forgejo and read by a human moderator reviewer.
//
// 🔴 EVERY ASSERTION HERE DRIVES THE REAL Build. The predicates are a seam
// (IsExcludedPath reproduces Build's decisions rather than sharing them), and
// only Build decides what is in the zip — a predicate-level suite is how a green
// run ships a walk that keeps the file anyway.

// rootScopeSecret is the sentinel these tests search the produced BYTES for.
// Distinct per-file so a positive result names which copy leaked; the search
// itself is proven to work by TestSecretSearchCanFindAKeptFile.
const rootScopeSecret = "API_SECRET=leak3-root-scope-sentinel"

// TestKeptEnvFilesAreRootScoped is the headline case, both directions in one
// tree: each of the three kept names at the ROOT is packaged, and the SAME three
// names at depth are dropped.
//
// 🔴 THE ROOT HALF IS THE POSITIVE CONTROL AND IT IS NOT OPTIONAL. Without it a
// mutant that deletes the allow-list outright — dropping the root files too —
// satisfies every "dropped at depth" assertion below and passes. The suite must
// be able to tell root-scoping apart from abolition.
//
// The depths are deliberately not adjacent: root, exactly one level, and three
// levels. A fixture whose only non-root case is one level deep cannot see a rule
// that scopes to "root or its immediate children".
func TestKeptEnvFilesAreRootScoped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "src/App.tsx", "export default 1")

	// AT THE ROOT — packaged. Contents are innocuous: these files ship, and a
	// test that plants a secret in a file it then asserts is uploaded would be
	// asserting the leak.
	writeFile(t, dir, ".env.example", "VITE_LIVE_BLOCK_TOKEN=\n")
	writeFile(t, dir, ".env.sample", "VITE_LIVE_BLOCK_TOKEN=\n")
	writeFile(t, dir, ".env.production", "VITE_BLOCK_ALLOWED_PARENT_ORIGINS=https://civitai.com\n")

	// AT DEPTH — dropped. The directories above them are ordinary content that
	// ships, so nothing but the root scope can drop these.
	writeFile(t, dir, ".env-backup/.env.production", rootScopeSecret+"\n")
	writeFile(t, dir, "app/.env.example", rootScopeSecret+"\n")
	writeFile(t, dir, "a/b/c/.env.sample", rootScopeSecret+"\n")

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := append([]string{}, res.Files...)
	sort.Strings(got)

	// A literal expected set, not one derived from keptEnvFiles or from the
	// walk: an expectation computed from the code under test agrees with it by
	// construction. `.env-backup/`, `app/` and `a/b/c/` themselves are ordinary
	// directories, so their absence here is entirely the file rule's doing.
	want := []string{
		".env.example",
		".env.production",
		".env.sample",
		"block.manifest.json",
		"src/App.tsx",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("bundle contents wrong\n got: %v\nwant: %v\n\n"+
			"🔴 A kept dotenv name is allow-listed at the PROJECT ROOT only. If a nested one is "+
			"present the leak is back; if a root one is missing the allow-list has been deleted "+
			"rather than scoped, and the server build loses the .env.production `vite build` reads.",
			got, want)
	}

	// The file list is the packager's own account of itself. Assert the secret is
	// absent from the produced BYTES too — a rule that listed the path
	// differently while still writing the content would pass the check above.
	if bytesContain(res.Zip, rootScopeSecret) {
		t.Fatal("the planted secret is present in the produced archive bytes although no nested " +
			"dotenv path appears in the file list — the bundle is committed to Forgejo and read " +
			"by a human moderator reviewer, so this is a durable disclosure")
	}
}

// TestSkippedReportsNestedKeptNames pins the half that makes the trade
// affordable. Dropping is the safe direction ONLY because the author is told: an
// app whose build really does read a nested `.env.production` (a `vite.config`
// pointing `envDir` at a subdirectory) learns on that very run, from the Skipped
// line, rather than at build time on the server.
//
// The tag matters as much as the path: `.env*` is what says "a dotenv rule
// reached it", i.e. that moving the file to the root recovers it.
func TestSkippedReportsNestedKeptNames(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "src/App.tsx", "export default 1")
	writeFile(t, dir, "app/.env.production", rootScopeSecret+"\n")
	writeFile(t, dir, ".env.production", "VITE_BLOCK_ALLOWED_PARENT_ORIGINS=https://civitai.com\n")

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// CONTROL: the drop must really have happened, or an assertion about how it
	// is REPORTED is a fact about an empty list.
	for _, f := range res.Files {
		if f == "app/.env.production" {
			t.Fatal("CONTROL failure: app/.env.production is IN the bundle — the root scope did " +
				"not fire, so this test cannot be about reporting it")
		}
	}
	// CONTROL, the other way: the ROOT copy must still ship, or the fixture is
	// testing abolition of the allow-list rather than its scope.
	rootShipped := false
	for _, f := range res.Files {
		if f == ".env.production" {
			rootShipped = true
		}
	}
	if !rootShipped {
		t.Fatal("CONTROL failure: the ROOT .env.production is NOT in the bundle — the allow-list " +
			"has been deleted rather than scoped, and the assertions below would pass for the wrong reason")
	}

	s, ok := findSkip(res.Skipped, "app/.env.production")
	if !ok {
		t.Fatalf("app/.env.production was dropped from the bundle and does NOT appear in Skipped %v — "+
			"an author whose vite.config points envDir at a subdirectory would find out at build "+
			"time on the server, which is the silent loss this scope is only affordable without",
			skipKeys(res.Skipped))
	}
	if s.Rule != RuleDotenvPrefix {
		t.Errorf("Skipped rule for app/.env.production = %q, want %q — the tag names the rule that "+
			"matched, i.e. that moving the file to the project root recovers it", s.Rule, RuleDotenvPrefix)
	}
	if s.Dir {
		t.Errorf("Skipped entry for app/.env.production has Dir=true, want false — it is a regular file")
	}
	// The ROOT copy is packaged, so it must NOT appear as a skip.
	if _, ok := findSkip(res.Skipped, ".env.production"); ok {
		t.Errorf("the ROOT .env.production appears in Skipped %v although it is in the bundle", skipKeys(res.Skipped))
	}
}

// TestNestedKeptNameUnderExcludedDirStaysDropped is the no-change control on the
// interaction that already worked: a kept name under an already-excluded
// directory was dropped by the DIRECTORY rule before the root scope existed, and
// still is. It is here so the root scope cannot be credited with a drop the walk
// was already making — and so a later change that removes the directory rule as
// "redundant now" goes red for its own reason.
func TestNestedKeptNameUnderExcludedDirStaysDropped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, ".env.d/.env.production", rootScopeSecret+"\n")
	writeFile(t, dir, "node_modules/.env.example", rootScopeSecret+"\n")

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, f := range res.Files {
		if f != "block.manifest.json" {
			t.Errorf("%q is in the bundle — a kept name inside an excluded DIRECTORY must stay "+
				"dropped by the directory rule, which returns SkipDir before any file name is read", f)
		}
	}
	// The walk never enumerates a skipped subtree, so the skip is recorded
	// against the DIRECTORY, not the file inside it. Pinning that here keeps the
	// two mechanisms distinguishable in the reporting as well as in the rule.
	for _, p := range []string{".env.d", "node_modules"} {
		s, ok := findSkip(res.Skipped, p)
		if !ok {
			t.Errorf("%q is not in Skipped %v — the directory decision is the one that happened", p, skipKeys(res.Skipped))
			continue
		}
		if !s.Dir {
			t.Errorf("Skipped entry for %q has Dir=false, want true", p)
		}
	}
	if _, ok := findSkip(res.Skipped, ".env.d/.env.production"); ok {
		t.Error("Skipped names a file under a skipped directory — the walk cannot have enumerated it")
	}
}

// TestIsExcludedPathAgreesWithBuildAtBothDepths is the SEAM guard for the new
// dimension. TestIsExcludedPathAgreesWithBuild walks one tree and compares the
// sets; this asks the narrower question that tree cannot ask on its own — the
// SAME base name, at the root and at depth, must get the same answer from Build
// and from both string predicates.
//
// 🔴 An agreement oracle catches an ASYMMETRIC change only. Build learns depth
// from its `rel`; IsExcludedPath counts the components it walked past. Those are
// two derivations of one fact, which is exactly the shape that drifts (#409,
// #420), so the answers are compared rather than the implementations read.
func TestIsExcludedPathAgreesWithBuildAtBothDepths(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	// One name, three positions. Written literally so no expectation below is
	// derived from keptEnvFiles.
	paths := []string{
		".env.production",
		"app/.env.production",
		"a/b/c/.env.production",
		".env.example",
		"docs/.env.example",
		".env.sample",
		".env-backup/.env.sample",
	}
	for _, p := range paths {
		writeFile(t, dir, p, "x")
	}

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	inBundle := map[string]bool{}
	for _, f := range res.Files {
		inBundle[f] = true
	}

	// Literal expectations: root keeps, depth drops.
	want := map[string]bool{
		".env.production":         true,
		"app/.env.production":     false,
		"a/b/c/.env.production":   false,
		".env.example":            true,
		"docs/.env.example":       false,
		".env.sample":             true,
		".env-backup/.env.sample": false,
	}
	kept, dropped := 0, 0
	for p, wantKept := range want {
		if inBundle[p] != wantKept {
			t.Errorf("Build: %q in bundle = %v, want %v (bundle: %v)", p, inBundle[p], wantKept, res.Files)
		}
		// 🔴 The seam is judged against what Build ACTUALLY did, never against
		// the expectation: "the predicate agrees with the table" and "the
		// predicate agrees with the walk" are different claims, and only the
		// second one is the seam. (When both sides are wrong together they agree,
		// and these two lines stay green — that is the known limit of an
		// agreement oracle, and the Build assertion above is what covers it.)
		if IsExcludedPath(p) == inBundle[p] {
			t.Errorf("🔴 SEAM: IsExcludedPath(%q) = %v while Build %s it. The dirty-tree guard (#411) "+
				"reads this predicate, so a disagreement makes it refuse a submit over a file that is "+
				"in no zip, or wave through an uncommitted one that is",
				p, IsExcludedPath(p), map[bool]string{true: "PACKAGES", false: "DROPS"}[inBundle[p]])
		}
		// IsExcludedEntry delegates, but it is what the guard actually calls.
		if IsExcludedEntry(p, 0) == inBundle[p] {
			t.Errorf("🔴 SEAM: IsExcludedEntry(%q, regular) = %v while Build put it in the bundle = %v — "+
				"this is the predicate the dirty-tree guard actually calls",
				p, IsExcludedEntry(p, 0), inBundle[p])
		}
		if wantKept {
			kept++
		} else {
			dropped++
		}
	}
	// CONTROL: both answers must be represented, or this loop pins a constant.
	if kept == 0 || dropped == 0 {
		t.Fatalf("the table has %d kept and %d dropped rows — an agreement over one answer is vacuous", kept, dropped)
	}
}
