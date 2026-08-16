package pkgzip

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TESTS FOR IsExcludedPath (issue #411).
//
// The dirty-tree guard asks this one question of every path `git status`
// reports: would the packager put it in the bundle? A wrong answer is not a
// cosmetic bug in either direction — too broad and the guard fires on an
// untracked `dist/` that ships nowhere, training everyone to pass --allow-dirty;
// too narrow and an uncommitted source file submits silently, which is the
// defect the guard exists to prevent.

func TestIsExcludedPathDirectoryComponents(t *testing.T) {
	excluded := []string{
		"node_modules/react/index.js",
		"dist/assets/app.js",
		"src/.git/config",
		"a/b/.venv/lib/x.py",
		"coverage/lcov.info",
		".next/server/page.js",
		// git's spelling for an untracked DIRECTORY — the trailing slash makes
		// the last component a directory, so the directory rule applies to it.
		"dist/",
		"node_modules/",
		"deep/nested/build/",
	}
	for _, p := range excluded {
		if !IsExcludedPath(p) {
			t.Errorf("IsExcludedPath(%q) = false, want true", p)
		}
	}

	included := []string{
		"src/App.tsx",
		"block.manifest.json",
		"package.json",
		"src/components/Button.tsx",
		"newfeature/",
		"docs/dist.md",
		// A plain FILE named after an excluded DIRECTORY is bundle content —
		// Build applies the directory rule only to directories, and so does this.
		"dist",
		"src/build",
		"./src/App.tsx",
	}
	for _, p := range included {
		if IsExcludedPath(p) {
			t.Errorf("IsExcludedPath(%q) = true, want false", p)
		}
	}
}

func TestIsExcludedPathFileNames(t *testing.T) {
	excluded := []string{
		"prev-package.zip",
		"src/thing.zip",
		".env",
		".env.local",
		".env.development",
		".envrc",
		"nested/.env.production.local",
	}
	for _, p := range excluded {
		if !IsExcludedPath(p) {
			t.Errorf("IsExcludedPath(%q) = false, want true", p)
		}
	}
	// The allow-listed dotenv templates are BUNDLE CONTENT, so an uncommitted
	// one is a real difference between the bundle and HEAD.
	for _, p := range []string{".env.example", ".env.sample", ".env.production", "src/.env.example"} {
		if IsExcludedPath(p) {
			t.Errorf("IsExcludedPath(%q) = true, but the packager KEEPS it, so the guard must see it", p)
		}
	}
}

// TestIsExcludedEntryJudgesFileType pins the half IsExcludedPath structurally
// cannot answer: Build drops every non-regular file, and no spelling of a path
// reveals that it is a symlink.
func TestIsExcludedEntryJudgesFileType(t *testing.T) {
	// The same path, three types, three answers.
	if IsExcludedEntry("src/App.tsx", 0) {
		t.Error("a regular src/App.tsx is bundle content")
	}
	if !IsExcludedEntry("src/App.tsx", fs.ModeSymlink) {
		t.Error("Build skips non-regular files, so a symlink is in NO bundle — the guard must not " +
			"refuse a submit over one, and its message must not claim it 'goes into the bundle'")
	}
	for _, mode := range []fs.FileMode{fs.ModeSocket, fs.ModeNamedPipe, fs.ModeDevice, fs.ModeCharDevice} {
		if !IsExcludedEntry("src/App.tsx", mode) {
			t.Errorf("mode %v is not a regular file and is in no bundle", mode)
		}
	}
	// A DIRECTORY takes the directory rule even without git's trailing slash.
	if !IsExcludedEntry("dist", fs.ModeDir) {
		t.Error("a directory named dist is excluded by name")
	}
	if IsExcludedEntry("src", fs.ModeDir) {
		t.Error("an ordinary directory is not excluded")
	}
	// NEGATIVE CONTROL: type is not the ONLY rule — the path still decides.
	if !IsExcludedEntry("dist/assets/app.js", 0) {
		t.Error("a regular file under dist/ is still excluded by path")
	}
}

// TestIsExcludedPathAgreesWithBuild is the SEAM guard.
//
// 🔴 IsExcludedPath reproduces Build's decisions rather than sharing them (Build
// walks a filesystem; this takes a string), so the two can drift silently — an
// exclusion added to one and not the other makes the dirty-tree guard fire on
// files that are not in the bundle, or miss files that are. Verifying each side
// in isolation cannot see that: the defect lives in the relationship.
//
// So this walks ONE tree twice — once through Build, once through the
// path-judging predicate — and asserts the two file sets are identical.
//
// 🔴 IT USED TO COPY Build'S OWN FILE-TYPE FILTER INTO ITS ORACLE, and that made
// it structurally blind to the one dimension the real caller exercises. The
// prediction walk skipped `!d.Type().IsRegular()` entries, so the fixture could
// never contain a symlink that the two sides disagreed about — and they DID
// disagree: Build drops a symlink, IsExcludedPath (a string predicate) cannot
// know, and the dirty-tree guard refused a submit over an untracked symlink
// that is in no zip. A seam guard that reproduces one side's filter in its
// oracle is only testing the half it already agreed on. The oracle now walks
// every non-directory entry and judges it with IsExcludedEntry, which is the
// predicate that HAS the type.
func TestIsExcludedPathAgreesWithBuild(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		"block.manifest.json",
		"package.json",
		"src/App.tsx",
		"src/components/Button.tsx",
		".env.example",
		".env.production",
		"docs/dist.md",
		// Every one of these must be dropped by BOTH sides.
		"node_modules/react/index.js",
		"dist/assets/app.js",
		"build/out.js",
		"coverage/lcov.info",
		".venv/lib/x.py",
		"src/.turbo/cache.bin",
		"prev-package.zip",
		"src/inner.zip",
		".env",
		".env.local",
		".envrc",
		"nested/.env.development.local",
		// 🔴 #409: a linked worktree's / submodule's `.git` is a regular FILE.
		// It is here because the two sides used to AGREE about it — and agree
		// that it ships. Both now drop it, and a fix applied to only one side
		// (an inline check in Build's walk instead of in the shared
		// isExcludedFile) makes this test red, which is what makes the fix
		// observable through the seam rather than only through Build.
		".git",
		"sub/.git",
		// 🔴 #420: the mirror of the line above, with the shapes swapped —
		// `.env.local` and `*.zip` were excluded as FILES only, so the same
		// names as DIRECTORIES shipped their contents. These are here for the
		// same reason `.git` is: a fix applied to Build's walk alone, instead of
		// to the shared isExcludedDir, makes this test red.
		//
		// Note the base names inside: `db.env` does NOT start with ".env", so
		// isExcludedFile keeps it. The directory rule is the only thing that
		// drops it, which is why a seam row for it is worth having.
		// (`.env.local` cannot appear here as a directory — this fixture already
		// plants it as a FILE above, and one tree cannot hold both shapes of one
		// name. TestBuildExcludesDotenvShapedDirectories owns that shape, in its
		// own tree. `.env.d` is the same rule with no such collision.)
		".env.d/db.env",
		"src/.env.d/api.env",
		"artifact.zip/payload.bin",
	}
	// Near-miss DIRECTORY names that must keep shipping — here for the same
	// reason `.gitignore` is, and with the same limitation, stated honestly:
	// this is an AGREEMENT oracle, so it catches an ASYMMETRIC change only. A
	// directory rule widened to the bare ".env" prefix deletes both subtrees
	// from every submission silently, and both sides would agree about it, so
	// these rows stay GREEN through exactly that mistake. The guards that catch
	// it are the near-miss rows in TestIsExcludedPathDotenvShapedDirectories and
	// the `kept` half of TestDirectoryRuleIsDocumentedForAuthors.
	files = append(files, ".environment/config.yaml", ".envoy/bootstrap.yaml")
	// The names that merely START with ".git" stay in the fixture as content:
	// both sides must keep shipping them, and a prefix match would make Build
	// and the predicate agree on a WRONG answer, which only the exact-name
	// assertions in git_metadata_file_test.go can catch.
	files = append(files, ".gitignore", ".gitattributes", "src/.gitkeep")
	for _, f := range files {
		writeFile(t, dir, f, "x")
	}
	// A SYMLINK, which is the entry the oracle used to filter out of its own
	// walk. Build drops it (non-regular); a string predicate cannot tell.
	symlinked := false
	if err := os.Symlink("src/App.tsx", filepath.Join(dir, "alias.tsx")); err == nil {
		symlinked = true
	}

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	built := append([]string{}, res.Files...)
	sort.Strings(built)

	// The same tree, judged only by the predicate — and WITHOUT reproducing
	// Build's file-type filter here, which is what made this oracle blind.
	var predicted []string
	var walked, nonRegular int
	err = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		walked++
		if !d.Type().IsRegular() {
			nonRegular++
		}
		if !IsExcludedEntry(rel, d.Type()) {
			predicted = append(predicted, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	sort.Strings(predicted)

	// POSITIVE CONTROL: the fixture must actually exercise the exclusions, or
	// the agreement below is a fact about two functions that exclude nothing.
	if walked <= len(built) {
		t.Fatalf("the fixture excludes nothing (%d files on disk, %d in the bundle) — "+
			"an agreement over it would be vacuous", walked, len(built))
	}
	// POSITIVE CONTROL on the WIDENING: a non-regular entry must really have
	// reached the oracle, or this test is the blind one it used to be.
	if symlinked && nonRegular == 0 {
		t.Fatal("the oracle walked no non-regular entry although the fixture created a symlink — " +
			"the file-type dimension is unobservable again")
	}

	if strings.Join(built, "\n") != strings.Join(predicted, "\n") {
		t.Errorf("Build and IsExcludedPath disagree about this tree.\n"+
			"One side gained an exclusion the other lacks — the dirty-tree guard "+
			"(#411) then fires on paths that never reach the bundle, or misses paths that do.\n"+
			"Build:\n  %s\nIsExcludedPath:\n  %s",
			strings.Join(built, "\n  "), strings.Join(predicted, "\n  "))
	}
}
