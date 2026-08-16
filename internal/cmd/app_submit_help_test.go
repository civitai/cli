package cmd

import (
	"strings"
	"testing"

	"github.com/civitai/cli/internal/pkgzip"
)

// 🔴 THE HELP TEXT IS A SURFACE, AND UNTIL NOW NOTHING READ IT.
//
// `app submit --help` is the only exclusion documentation most authors ever see.
// It was measured, on the round that wrote it, that DELETING the entire
// paragraph describing the directory rule left the full 20-package suite green —
// the guards next door pin `pkgzip.DirectoryPatternSummary()`, which is a claim
// about a FUNCTION, not about anything a user is shown. A guard scoped to the
// producer cannot see the consumer drop it.
//
// It also caught a real inversion: the first version of this paragraph said
// "Those are matched against a FILE's name" under a list whose first 18 entries
// are DIRECTORY names, so it was false for 15 of the 18. A regular file named
// `build` is packaged; a directory named `node_modules` is dropped. The rows
// below pin both directions of that distinction, because getting it backwards
// is what the sentence did.
func TestSubmitHelpDocumentsBothExclusionShapes(t *testing.T) {
	long := newAppSubmitCmd().Long
	if strings.TrimSpace(long) == "" {
		t.Fatal("CONTROL failure: the submit command has no Long text, so every check " +
			"below would pass over an empty string")
	}

	// The directory PATTERN rules are not printed by any list, so the help must
	// carry the summary itself.
	if !strings.Contains(long, pkgzip.DirectoryPatternSummary()) {
		t.Error("`app submit --help` does not carry pkgzip.DirectoryPatternSummary() — " +
			"an author is told nothing about .env.d/ being dropped whole, nor about " +
			".env-backup/ NOT being dropped, which is the credential-direction half")
	}

	// Both fixed lists must appear, each under its own shape. Sampled rather
	// than exhaustive, but the samples are chosen to be the ones that read
	// wrongly if the two lists are conflated.
	for _, name := range pkgzip.ExcludedNames() {
		if !strings.Contains(long, name) {
			t.Errorf("`app submit --help` does not name the excluded directory %q", name)
		}
	}
	for _, pat := range pkgzip.ExcludedFilePatterns() {
		if !strings.Contains(long, pat) {
			t.Errorf("`app submit --help` does not name the excluded file pattern %q", pat)
		}
	}

	// 🔴 The shape distinction itself. `build` appears in BOTH lists' vicinity,
	// and it is the name whose two shapes behave oppositely, so the help has to
	// say which is which rather than presenting one flat list.
	if !strings.Contains(long, "DIRECTORIES") || !strings.Contains(long, "FILES") {
		t.Error("`app submit --help` does not distinguish DIRECTORIES from FILES — " +
			"the lists behave differently and a flat list reads as one rule")
	}

	// Pin the fact the inverted sentence got wrong, in the direction that costs
	// an author content rather than a secret.
	if !strings.Contains(long, "a regular file named\nbuild or dist IS packaged") &&
		!strings.Contains(long, "a regular file named build or dist IS packaged") {
		t.Error("`app submit --help` no longer states that a regular FILE named build " +
			"or dist is packaged — that is the half of the rule an author loses content to")
	}
}

// The negative control for the assertions above: they must be capable of
// failing. A help text that carried none of these would be caught — this proves
// the strings are really being searched rather than the checks being satisfied
// by something structural.
func TestSubmitHelpChecksCanFail(t *testing.T) {
	empty := ""
	if strings.Contains(empty, pkgzip.DirectoryPatternSummary()) {
		t.Fatal("an empty help text 'contains' the summary — the search is broken, so a " +
			"green TestSubmitHelpDocumentsBothExclusionShapes would prove nothing")
	}
	if len(pkgzip.ExcludedNames()) == 0 || len(pkgzip.ExcludedFilePatterns()) == 0 {
		t.Fatal("one of the exclusion lists is empty, so its loop above iterates zero " +
			"times and asserts nothing")
	}
}
