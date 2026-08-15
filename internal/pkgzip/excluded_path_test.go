package pkgzip

import (
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

// TestIsExcludedPathAgreesWithBuild is the SEAM guard.
//
// 🔴 IsExcludedPath reproduces Build's decisions rather than sharing them (Build
// walks a filesystem; this takes a string), so the two can drift silently — an
// exclusion added to one and not the other makes the dirty-tree guard fire on
// files that are not in the bundle, or miss files that are. Verifying each side
// in isolation cannot see that: the defect lives in the relationship.
//
// So this walks ONE tree twice — once through Build, once through
// IsExcludedPath — and asserts the two file sets are identical.
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
	}
	for _, f := range files {
		writeFile(t, dir, f, "x")
	}

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	built := append([]string{}, res.Files...)
	sort.Strings(built)

	// The same tree, judged only by IsExcludedPath.
	var predicted []string
	var walked int
	err = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		walked++
		if !IsExcludedPath(rel) {
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

	if strings.Join(built, "\n") != strings.Join(predicted, "\n") {
		t.Errorf("Build and IsExcludedPath disagree about this tree.\n"+
			"One side gained an exclusion the other lacks — the dirty-tree guard "+
			"(#411) then fires on paths that never reach the bundle, or misses paths that do.\n"+
			"Build:\n  %s\nIsExcludedPath:\n  %s",
			strings.Join(built, "\n  "), strings.Join(predicted, "\n  "))
	}
}
