package pkgzip

import (
	"testing"
)

// `scratch/` AT THE PROJECT ROOT is the page-elements scaffold's throwaway
// directory (probes, debug scripts), git-ignored and outside the typecheck and
// the test run. It is not app source, so `app submit` must not package it.
//
// 🔴 THE RULE IS ROOT-ONLY, AND THE NESTED CASE IS THE HALF THAT CAN GO WRONG
// QUIETLY. Adding "scratch" to excludedDirs (any depth) would pass the first
// assertion below and silently drop an app's own `src/x/scratch/` source — a
// directory the author named, holding code the build imports. So the fixture
// plants both, and the nested one must SHIP.
func TestBuildDropsRootScratchOnlyNotNestedScratch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "src/App.tsx", "export default 1")
	writeFile(t, dir, "scratch/README.md", "# scratch")
	writeFile(t, dir, "scratch/probe.ts", "const n: number = 'x'")
	writeFile(t, dir, "src/x/scratch/file.ts", "export const kept = 1")
	// A FILE named scratch at the root is not the directory rule's business,
	// exactly as a file named `dist` is not (see IsExcludedPath).
	writeFile(t, dir, "src/scratch", "a file, not a directory")

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	have := map[string]bool{}
	for _, f := range res.Files {
		have[f] = true
	}
	for _, gone := range []string{"scratch/README.md", "scratch/probe.ts"} {
		if have[gone] {
			t.Errorf("%s was packaged; the project-root scratch/ must not be (files: %v)", gone, res.Files)
		}
	}
	for _, kept := range []string{"src/x/scratch/file.ts", "src/scratch", "src/App.tsx", "block.manifest.json"} {
		if !have[kept] {
			t.Errorf("%s was dropped; only the PROJECT-ROOT scratch/ is excluded (files: %v)", kept, res.Files)
		}
	}

	// The drop is reported, once, as the directory — the same shape every other
	// directory skip takes.
	var skips []Skip
	for _, s := range res.Skipped {
		if s.Path == "scratch" || s.Path == "src/x/scratch" {
			skips = append(skips, s)
		}
	}
	if len(skips) != 1 || skips[0] != (Skip{Path: "scratch", Dir: true}) {
		t.Errorf("Skipped entries for scratch = %+v, want exactly [{Path:scratch Dir:true Rule:}]", skips)
	}
}

// IsExcludedPath is the dirty-tree guard's and internal/antipattern's view of
// the same rule, reached through a string rather than a walk. It must agree
// with Build at both depths — TestIsExcludedPathAgreesWithBuild carries the
// same rows as seam coverage; these are the direct answers.
func TestIsExcludedPathRootScratchOnly(t *testing.T) {
	for _, c := range []struct {
		rel  string
		want bool
	}{
		{"scratch/probe.ts", true},
		{"scratch/", true},
		{"./scratch/deep/x.ts", true},
		{"src/x/scratch/file.ts", false},
		{"src/scratch/", false},
		{"scratch", false}, // a FILE named scratch at the root: the file rule decides
		{"Scratch/probe.ts", false},
	} {
		if got := IsExcludedPath(c.rel); got != c.want {
			t.Errorf("IsExcludedPath(%q) = %v, want %v", c.rel, got, c.want)
		}
	}
	// The any-depth base-name predicate must NOT have gained the name: that is
	// the over-broad fix this rule exists instead of.
	if IsExcluded("scratch") {
		t.Error(`IsExcluded("scratch") = true; scratch is excluded at the project root only, not at any depth`)
	}
}
