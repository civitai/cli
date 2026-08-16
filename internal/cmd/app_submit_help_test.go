package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/pkgzip"
)

// Deliberately reuses this package's existing golden machinery — `goldenDir`,
// `normaliseCopy` and the `-update` flag from generate_golden_copy_test.go —
// rather than minting a second flag. A private flag would mean
// `go test ./internal/cmd -update` silently does not refresh this file, which
// is exactly the kind of "the gate exists but nobody's habit reaches it" gap
// this test was written to close.
const submitHelpGoldenName = "app_submit_help_exclusions"

// 🔴 THE HELP TEXT IS A SURFACE, AND A SUBSTRING CHECK CANNOT GUARD PROSE.
//
// `app submit --help` is the only exclusion documentation most authors ever
// see. Two rounds of guards failed here, in the same way, one after the other:
//
//   - Round 1 pinned nothing at all, so DELETING the whole paragraph left the
//     full 20-package suite green.
//   - Round 2 pinned `strings.Contains(long, DirectoryPatternSummary())` and
//     each list entry — assertions against strings the help is CONCATENATED
//     FROM, so they detect deletion and nothing else. Two wrong-but-passing
//     help texts were constructed against it: one that SWAPPED the DIRECTORIES
//     and FILES headings (re-creating the exact inversion the round was fixing)
//     and one whose closing paragraph read "The two lists are ONE rule … it is
//     a MYTH that a regular file named build or dist IS packaged". Both green.
//
// A guard on WORDS is walkable by choosing different words, so this pins the
// WHOLE normalised exclusion block. That is the shape that already works for
// `pkgzip.DirectoryPatternSummary` and for the README's dotenv section.
//
// Comparison is whitespace-collapsed, so re-wrapping the lists or the summary
// costs nothing. Any other edit fails here and must be re-read against the
// packager before it is re-approved:
//
//	go test ./internal/cmd -run TestSubmitHelpIsTheReviewedCopy -update
//
// READ that diff. The question it must answer is not "does this read well" but
// "is every sentence TRUE of pkgzip.Build" — checked by building a fixture, not
// by reasoning. Three separate sentences here have been measurably false, each
// caught only by running the packager: one about which SHAPE a rule applies to,
// one in the credential direction, and one universal ("wherever they sit") that
// contradicted a sentence two lines above it.
func TestSubmitHelpIsTheReviewedCopy(t *testing.T) {
	got := normaliseCopy(submitHelpExclusionBlock(t))
	path := filepath.Join(goldenDir, submitHelpGoldenName+".txt")

	// POSITIVE CONTROL: an empty block normalises to "" and would match an empty
	// golden forever, reporting a serene pass over a help text that has stopped
	// describing anything.
	if strings.TrimSpace(got) == "" {
		t.Fatal("CONTROL failure: the exclusion block normalised to nothing, so an " +
			"empty golden would match it forever")
	}

	if *updateGolden {
		if err := os.MkdirAll(goldenDir, 0o755); err != nil {
			t.Fatalf("create %s: %v", goldenDir, err)
		}
		if err := os.WriteFile(path, []byte(got+"\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		t.Logf("updated %s — READ THE DIFF", path)
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v\n\nRe-approve deliberately with -update and review the "+
			"file it writes.", path, err)
	}
	if strings.TrimSpace(string(want)) != got {
		t.Errorf("`app submit --help`'s exclusion block changed.\n\n want: %s\n\n got:  %s\n\n"+
			"🔴 This text tells an author what reaches the platform, and it has been "+
			"WRONG three times: about which shape a rule applies to (a regular file "+
			"named `build` IS packaged; a directory named `node_modules` is not), in "+
			"the credential direction, and by claiming kept dotenv files survive "+
			"\"wherever they sit\" when a kept name inside .env.d/ is dropped with the "+
			"directory. Re-approve with -update only after checking every sentence "+
			"against a real `Build` run.",
			strings.TrimSpace(string(want)), got)
	}
}

// The golden pins the WORDS. This pins the words to the CODE: every list the
// help renders must be the one pkgzip actually exports, so a rule added to
// pkgzip without touching the help fails here rather than shipping an
// exclusion list that quietly omits it.
func TestSubmitHelpRendersTheRealExclusionLists(t *testing.T) {
	block := submitHelpExclusionBlock(t)

	for _, name := range pkgzip.ExcludedNames() {
		if !strings.Contains(block, name) {
			t.Errorf("`app submit --help` does not name the excluded directory %q — "+
				"a rule was added to pkgzip without updating the help", name)
		}
	}
	for _, pat := range pkgzip.ExcludedFilePatterns() {
		if !strings.Contains(block, pat) {
			t.Errorf("`app submit --help` does not name the excluded file pattern %q", pat)
		}
	}
	// The kept files are the credential-direction half: an author who reads only
	// the exclusion list puts a token in the one dotenv file that IS uploaded.
	for _, kept := range pkgzip.KeptEnvFileNames() {
		if !strings.Contains(block, kept) {
			t.Errorf("`app submit --help` does not say %q is KEPT and uploaded — that is "+
				"the file an author is most likely to put a secret in", kept)
		}
	}
	if !strings.Contains(block, pkgzip.DirectoryPatternSummary()) {
		t.Error("`app submit --help` does not carry pkgzip.DirectoryPatternSummary() — " +
			"the directory PATTERN rules are printed by no list, so dropping it leaves " +
			"them undocumented")
	}

	// CONTROL: these loops assert nothing if the lists are empty.
	if len(pkgzip.ExcludedNames()) == 0 || len(pkgzip.ExcludedFilePatterns()) == 0 ||
		len(pkgzip.KeptEnvFileNames()) == 0 {
		t.Fatal("an exclusion list is empty, so its loop above iterated zero times")
	}
}

// submitHelpExclusionBlock is the part of the command's Long text that
// describes what is packaged — from the opening sentence to the next section.
// Scoped rather than whole-Long so unrelated edits to the submission-path prose
// do not force a re-approval of the exclusion copy.
func submitHelpExclusionBlock(t *testing.T) string {
	t.Helper()
	long := newAppSubmitCmd().Long
	if strings.TrimSpace(long) == "" {
		t.Fatal("CONTROL failure: the submit command has no Long text")
	}
	const start = "The package is the SOURCE tree"
	const end = "Submission path:"
	i := strings.Index(long, start)
	if i < 0 {
		t.Fatalf("`app submit --help` no longer contains %q — the exclusion block was "+
			"renamed or removed, and this guard cannot see what replaced it", start)
	}
	rest := long[i:]
	if j := strings.Index(rest, end); j >= 0 {
		rest = rest[:j]
	}
	if strings.TrimSpace(rest) == "" {
		t.Fatal("CONTROL failure: the exclusion block is empty")
	}
	return rest
}
