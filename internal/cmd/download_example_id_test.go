package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// knownAmbiguousExampleID is the id that shipped in the `download` examples until
// issue #227. It is BOTH DreamShaper's v8 version id AND the model id of an
// unrelated NSFW-titled model, so `civitai download 128713` refuses to guess and
// exits 2 — the first command a new user copy-pastes, failing.
//
// It is named explicitly (rather than only being absent from
// unambiguousExampleIDs) so a reader of a failure knows what the guard is about.
const knownAmbiguousExampleID = "128713"

// bareDownloadPositionalRe matches a `civitai download <digits>` example — an id
// passed as the BARE POSITIONAL, which is the only form that can hit the
// ambiguity stop. `civitai download --version 128713` and
// `civitai download --model 4384` deliberately do NOT match: naming the kind
// explicitly can never be ambiguous, which is exactly why the help still uses
// 128713 to demonstrate the escape hatch.
var bareDownloadPositionalRe = regexp.MustCompile(`civitai download ([0-9]+)`)

// repoRootDir locates the module root from this test file's own path.
func repoRootDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// …/internal/cmd/download_example_id_test.go -> the repo root is two dirs up.
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// downloadExampleSources returns every surface that teaches a `civitai download`
// invocation: the two command helps a user reaches with --help, and the README.
// Each entry carries the MINIMUM number of bare-positional examples it must
// contain, so a source that stops matching (a renamed command, a moved file, a
// regex that no longer fires) fails loudly instead of passing on zero hits.
func downloadExampleSources(t *testing.T) map[string]struct {
	text    string
	minHits int
} {
	t.Helper()
	root := NewRootCmd()
	dl := newDownloadCmd()

	readme, err := os.ReadFile(filepath.Join(repoRootDir(t), "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}

	return map[string]struct {
		text    string
		minHits int
	}{
		"root Long":        {root.Long, 1},
		"root Example":     {root.Example, 1},
		"download Long":    {dl.Long, 2},
		"download Example": {dl.Example, 5},
		"README.md":        {string(readme), 8},
	}
}

// TestDownloadExamplesUseUnambiguousIDs is the regression guard for issue #227.
//
// Every id handed to `civitai download` as a BARE POSITIONAL — in the root help,
// in the download help, and in the README — must be one whose ambiguity was
// actually checked against the live API and recorded in unambiguousExampleIDs.
// The check is an allowlist rather than a "not 128713" denylist on purpose: the
// hazard is the CLASS (a number that is both a model id and a version id), and a
// denylist would wave through the next one someone picks at random.
func TestDownloadExamplesUseUnambiguousIDs(t *testing.T) {
	// The known-bad id must never be smuggled into the allowlist.
	if what, ok := unambiguousExampleIDs[knownAmbiguousExampleID]; ok {
		t.Fatalf("%s is ambiguous (both a model id and a version id) but is listed in unambiguousExampleIDs as %q",
			knownAmbiguousExampleID, what)
	}

	total := 0
	for name, src := range downloadExampleSources(t) {
		hits := bareDownloadPositionalRe.FindAllStringSubmatch(src.text, -1)
		// Positive control: a source that yields zero matches proves nothing —
		// it is indistinguishable from a guard wired to the wrong text.
		if len(hits) < src.minHits {
			t.Errorf("%s: found %d `civitai download <id>` examples, want >= %d — "+
				"the guard is reading the wrong text, or the examples were removed",
				name, len(hits), src.minHits)
			continue
		}
		total += len(hits)
		for _, h := range hits {
			id := h[1]
			if id == knownAmbiguousExampleID {
				t.Errorf("%s: `civitai download %s` is the issue-#227 example — %s is BOTH "+
					"a model id and a version id, so it exits 2 with an ambiguity error. "+
					"Use %s, or name the kind explicitly (--version/--model).",
					name, id, id, downloadExampleVersionID)
				continue
			}
			if _, ok := unambiguousExampleIDs[id]; !ok {
				t.Errorf("%s: `civitai download %s` uses an id that has not been verified "+
					"unambiguous. Check that /api/v1/models/%s and /api/v1/model-versions/%s "+
					"do not BOTH return 200, then add it to unambiguousExampleIDs.",
					name, id, id, id)
			}
		}
	}
	if total == 0 {
		t.Fatal("no `civitai download <id>` examples found anywhere — the guard is inert")
	}
}

// TestDownloadExampleVersionIDIsUsedEverywhere pins the headline example id
// itself. downloadExampleVersionID must be in the verified allowlist, and both
// root-help surfaces must actually render it — a constant nothing interpolates
// would let the help drift back to a literal while the constant stayed correct.
func TestDownloadExampleVersionIDIsUsedEverywhere(t *testing.T) {
	if _, ok := unambiguousExampleIDs[downloadExampleVersionID]; !ok {
		t.Fatalf("downloadExampleVersionID = %q is not listed in unambiguousExampleIDs; "+
			"verify it against the live API and record what it resolved as",
			downloadExampleVersionID)
	}
	root := NewRootCmd()
	want := "civitai download " + downloadExampleVersionID
	for name, text := range map[string]string{"root Long": root.Long, "root Example": root.Example} {
		if !strings.Contains(text, want) {
			t.Errorf("%s should show %q:\n%s", name, want, text)
		}
	}
}

// TestDownloadHelpKeepsTheExplicitKindEscapes proves the ambiguity ESCAPES stay
// documented. Fixing #227 by swapping the id would be hollow if the help stopped
// showing how to name the kind explicitly — that is what a user hitting the stop
// on their own id needs. Both forms must survive, and the ambiguous id is the
// right value to demonstrate --version with.
func TestDownloadHelpKeepsTheExplicitKindEscapes(t *testing.T) {
	dl := newDownloadCmd()
	blob := dl.Long + "\n" + dl.Example
	for _, want := range []string{
		"civitai download --version " + knownAmbiguousExampleID,
		"civitai download --model ",
	} {
		if !strings.Contains(blob, want) {
			t.Errorf("download help should still document the explicit-kind escape %q:\n%s", want, blob)
		}
	}
}

// TestUnambiguousExampleIDsAreDocumented keeps the allowlist self-describing:
// every entry must carry a non-empty note saying what it resolved as, so adding
// one is a recorded measurement rather than a silent widening.
func TestUnambiguousExampleIDsAreDocumented(t *testing.T) {
	if len(unambiguousExampleIDs) == 0 {
		t.Fatal("unambiguousExampleIDs is empty — the allowlist guard would reject every example")
	}
	ids := make([]string, 0, len(unambiguousExampleIDs))
	for id := range unambiguousExampleIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if strings.TrimSpace(unambiguousExampleIDs[id]) == "" {
			t.Errorf("unambiguousExampleIDs[%q] has no note recording what it resolved as", id)
		}
	}
}
