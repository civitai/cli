package pkgzip

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 🔴 `.env.production` IS SHIPPED TO THE PLATFORM, AND FOR A LONG TIME NOTHING
// SAID SO. The README documented the exclusion of `.env.development*` and
// nothing else, so "the CLI excludes dotenv files" was the natural — and wrong —
// inference. Deciding what to put in a file is a decision an author can only
// make if they know where the file goes.
//
// This is a BIDIRECTIONAL ledger between the packager's two lists and the
// README, in the direction that matters: every dotenv file the packager treats
// specially must be NAMED in the README. Adding a new kept file, or a new
// excluded dotenv pattern, without documenting it fails here.
//
// What it deliberately does NOT do: assert the README's *explanation* is
// correct. Only reading the code against the prose does that.

// readmeDotenvSection is the heading the documentation lives under. Asserting
// the section exists (rather than searching the whole README) is what stops an
// unrelated mention elsewhere in a 2,500-line file from satisfying the check.
const readmeDotenvHeading = "#### Which dotenv files end up in the bundle"

func readmeBody(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	return string(b)
}

func TestREADMEDocumentsEveryDotenvFileThePackagerKeeps(t *testing.T) {
	body := readmeBody(t)
	i := strings.Index(body, readmeDotenvHeading)
	if i < 0 {
		t.Fatalf("README.md has no %q section — the packager's dotenv rule is undocumented, "+
			"which is the bug this guard exists for", readmeDotenvHeading)
	}
	// The section runs to the next heading of the same or higher level.
	rest := body[i+len(readmeDotenvHeading):]
	if j := strings.Index(rest, "\n## "); j >= 0 {
		rest = rest[:j]
	}
	if k := strings.Index(rest, "\n### "); k >= 0 {
		rest = rest[:k]
	}
	if strings.TrimSpace(rest) == "" {
		t.Fatal("CONTROL failure: the section heading exists but has no body, so every check below is vacuous")
	}

	// (1) Everything the packager deliberately KEEPS must be named.
	if len(keptEnvFiles) == 0 {
		t.Fatal("CONTROL failure: keptEnvFiles is empty, so this half of the ledger checks nothing")
	}
	for name := range keptEnvFiles {
		if !strings.Contains(rest, name) {
			t.Errorf("the packager SHIPS %s but the README section does not name it. "+
				"A file the author never learns is uploaded is a file they cannot decide what to put in.", name)
		}
	}

	// (2) Every dotenv pattern it EXCLUDES must be named too — otherwise the
	// section reads as a complete account while omitting half the rule.
	dotenvExclusions := 0
	for _, p := range excludedFilePatterns {
		if !strings.HasPrefix(p, ".env") {
			continue
		}
		dotenvExclusions++
		if !strings.Contains(rest, p) {
			t.Errorf("the packager EXCLUDES %q but the README section does not name it", p)
		}
	}
	if dotenvExclusions == 0 {
		t.Fatal("CONTROL failure: excludedFilePatterns holds no .env pattern, so this half checks nothing")
	}

	// (3) The one file that is neither: `.env.production.local` is caught by the
	// `.env.*.local` rule, which is exactly the distinction a reader gets wrong.
	if !strings.Contains(rest, ".env.production.local") {
		t.Error("the section must call out `.env.production.local` — a reader who learns `.env.production` " +
			"ships will assume its `.local` override does too, and that one carries secrets")
	}
	if _, kept := keptEnvFiles[".env.production.local"]; kept {
		t.Error("🔴 .env.production.local is in keptEnvFiles — `.local` is the dev-local override " +
			"convention and must never be uploaded")
	}
}
