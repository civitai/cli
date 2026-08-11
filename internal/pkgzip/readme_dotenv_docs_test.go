package pkgzip

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// 🔴 `.env.production` IS SHIPPED TO THE PLATFORM, AND FOR A LONG TIME NOTHING
// SAID SO. The README documented the exclusion of `.env.development*` and
// nothing else, so "the CLI excludes dotenv files" was the natural — and wrong —
// inference. Deciding what to put in a file is a decision an author can only
// make if they know where the file goes.
//
// This is a BIDIRECTIONAL ledger between the packager and the README, in the
// direction that matters: every dotenv file the packager treats specially must
// be NAMED in the README. Adding a new kept file, or a new excluded dotenv
// pattern, without documenting it fails here.
//
// 🔴 IT IS BOUND TO `isExcludedFile`, THE AUTHORITATIVE MATCHER — NOT TO
// `excludedFilePatterns`. The first revision of this guard read only the latter,
// which pkgzip.go's own comment labels "human-readable … for CLI messaging",
// so it was structurally blind to a change in the rule that actually decides
// what ships: the enumerated names could stay word-for-word while
// `isExcludedFile` started shipping every unlisted `.env*`, and this test would
// have reported a serene pass. The probes below are the repair — they are names
// in NEITHER list, so they can only be answered by the catch-all itself, and the
// README's claim to BE a catch-all is checked against what the matcher does with
// them.
//
// What it deliberately does NOT do: assert the README's *explanation* is
// correct. Only reading the code against the prose does that. (The explanation
// has been wrong here before — see the 🔴 note on `keptEnvFiles`.)

// readmeDotenvSection is the heading the documentation lives under. Asserting
// the section exists (rather than searching the whole README) is what stops an
// unrelated mention elsewhere in a 2,500-line file from satisfying the check.
const readmeDotenvHeading = "#### Which dotenv files end up in the bundle"

// catchAllProbes are `.env*` base names that appear in NEITHER keptEnvFiles NOR
// excludedFilePatterns, and that NO other requirement in this file mentions.
// They exist because an enumeration and a catch-all agree on every enumerated
// name and disagree only here: these are the only inputs that can tell "the rule
// is `.env*` minus an allow-list" apart from "the rule is these five names".
//
// 🔴 `.env.production.local` and `.env.development.local` are deliberately NOT
// here, even though the matcher does answer them by the catch-all. The first is
// separately REQUIRED to appear in the README by check (3) of the ledger test,
// and the second is a concrete instance of the enumerated `.env.*.local`
// pattern — so either of them would satisfy the documentation half of this test
// for a reason that has nothing to do with the catch-all. Measured, not
// theorised: with `.env.production.local` in this list, reverting the README to
// its enumeration-only wording left this test GREEN. They are probed as
// `catchAllMatcherOnlyProbes` below instead.
//
// 🔴 THE UNDOTTED ROWS ARE THE BOUNDARY, AND THEY ARE WHY THIS LIST GREW. Every
// probe here was `.env.`-DOTTED, so the matcher's real boundary — is the prefix
// `.env` or `.env.`? — was pinned by nothing: a mutant narrowing the match back
// to `name == ".env" || HasPrefix(name, ".env.")` was GREEN on the whole
// package, while `.envrc` (direnv, routinely holding exported credentials) was
// packaged and uploaded to the platform and to a human moderator reviewer,
// under a README sentence promising it was not. `.envrc` and `.env-local` are
// the rows that go red for that mutant.
var catchAllProbes = []string{
	".env.staging",
	".env.ci",
	".env.example.bak",
	".envrc",
	".env-local",
}

// catchAllMatcherOnlyProbes are answered by the catch-all too, but are named in
// the README for other reasons, so they check the MATCHER only.
var catchAllMatcherOnlyProbes = []string{
	".env.production.local",
	".env.development.local",
}

// repoRoot resolves the checkout root from THIS FILE's location rather than the
// process working directory, so the test does not depend on how it was invoked.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed — cannot locate the repository root")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func readmeBody(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	return string(b)
}

// dotenvSection returns the body of the README's dotenv subsection.
func dotenvSection(t *testing.T) string {
	t.Helper()
	body := readmeBody(t)
	i := strings.Index(body, readmeDotenvHeading)
	if i < 0 {
		t.Fatalf("README.md has no %q section — the packager's dotenv rule is undocumented, "+
			"which is the bug this guard exists for", readmeDotenvHeading)
	}
	rest := body[i+len(readmeDotenvHeading):]
	// The section runs to the next heading of ANY level. `####` is included
	// because this section is itself a `####`: stopping only at `##`/`###` let a
	// following sibling subsection be read as part of this one.
	for _, h := range []string{"\n## ", "\n### ", "\n#### "} {
		if j := strings.Index(rest, h); j >= 0 {
			rest = rest[:j]
		}
	}
	if strings.TrimSpace(rest) == "" {
		t.Fatal("CONTROL failure: the section heading exists but has no body, so every check below is vacuous")
	}
	return rest
}

func TestREADMEDocumentsEveryDotenvFileThePackagerKeeps(t *testing.T) {
	rest := dotenvSection(t)

	// (1) Everything the AUTHORITATIVE matcher lets through must be named.
	//     Iterated over keptEnvFiles but asserted through isExcludedFile, so the
	//     two cannot drift apart silently.
	if len(keptEnvFiles) == 0 {
		t.Fatal("CONTROL failure: keptEnvFiles is empty, so this half of the ledger checks nothing")
	}
	for name := range keptEnvFiles {
		if isExcludedFile(name) {
			t.Errorf("🔴 SEAM: %s is in keptEnvFiles but isExcludedFile(%q) = true. The matcher decides what "+
				"ships; the map is what the README is documented from. They must agree.", name, name)
			continue
		}
		if !strings.Contains(rest, name) {
			t.Errorf("the packager SHIPS %s but the README section does not name it. "+
				"A file the author never learns is uploaded is a file they cannot decide what to put in.", name)
		}
	}

	// (2) Every dotenv pattern on the human-readable list must be named in the
	//     README AND actually excluded by the matcher. The second half is the
	//     seam: excludedFilePatterns is documentation, isExcludedFile is the rule.
	dotenvExclusions := 0
	for _, p := range excludedFilePatterns {
		if !strings.HasPrefix(p, ".env") {
			continue
		}
		dotenvExclusions++
		if !strings.Contains(rest, p) {
			t.Errorf("the packager EXCLUDES %q but the README section does not name it", p)
		}
		// `.env.*.local` is a glob in the human list; probe a concrete instance.
		concrete := strings.ReplaceAll(p, "*", "development")
		if !isExcludedFile(concrete) {
			t.Errorf("🔴 SEAM: excludedFilePatterns lists %q but isExcludedFile(%q) = false, so the file "+
				"the README calls excluded is actually UPLOADED. The pattern list is messaging; the matcher ships.", p, concrete)
		}
	}
	if dotenvExclusions == 0 {
		t.Fatal("CONTROL failure: excludedFilePatterns holds no .env pattern, so this half checks nothing")
	}

	// (3) The one file that is neither: `.env.production.local` is caught by the
	//     catch-all, which is exactly the distinction a reader gets wrong.
	if !strings.Contains(rest, ".env.production.local") {
		t.Error("the section must call out `.env.production.local` — a reader who learns `.env.production` " +
			"ships will assume its `.local` override does too, and that one carries secrets")
	}
	if _, kept := keptEnvFiles[".env.production.local"]; kept {
		t.Error("🔴 .env.production.local is in keptEnvFiles — `.local` is the dev-local override " +
			"convention and must never be uploaded")
	}
}

// TestDotenvRuleIsACatchAllAndTheREADMESaysSo is the half bound to nothing but
// `isExcludedFile`. The names it probes are in neither list, so the enumerated
// checks above are all satisfied whichever way the matcher answers them — which
// is precisely why an enumeration-shaped README (and an enumeration-bound guard)
// could both be green while `.env.staging` shipped.
func TestDotenvRuleIsACatchAllAndTheREADMESaysSo(t *testing.T) {
	rest := dotenvSection(t)

	// CONTROL, both directions: isExcludedFile must be able to answer BOTH ways,
	// or every assertion below passes on a constant.
	if !isExcludedFile(".env") {
		t.Fatal("CONTROL failure: isExcludedFile(\".env\") = false — the matcher excludes nothing, so the checks below are vacuous")
	}
	if isExcludedFile(".env.example") {
		t.Fatal("CONTROL failure: isExcludedFile(\".env.example\") = true — the matcher excludes everything, so the checks below are vacuous")
	}

	for _, name := range append(append([]string{}, catchAllProbes...), catchAllMatcherOnlyProbes...) {
		// Guard the premise: a probe that has been added to either list no longer
		// discriminates, and silently passing it would hollow this test out.
		if _, kept := keptEnvFiles[name]; kept {
			t.Errorf("CONTROL failure: probe %q is now in keptEnvFiles, so it no longer tests the catch-all — "+
				"replace it with a name in neither list (and document the new kept file)", name)
			continue
		}
		if !isExcludedFile(name) {
			t.Errorf("🔴 %s is UPLOADED. It is on no list, so only the catch-all can have excluded it — the rule "+
				"has become an enumeration, and every unrecognised dotenv file now ships to the platform.", name)
		}
	}

	// And the README must document the rule as a catch-all rather than as the
	// five names it happens to enumerate: a reader with `.env.staging` must be
	// able to answer the question from this section. Asserted by requiring the
	// section to name a file that ONLY the catch-all excludes.
	documented := 0
	for _, name := range catchAllProbes {
		if strings.Contains(rest, name) {
			documented++
		}
	}
	if documented == 0 {
		t.Errorf("the README section enumerates the listed names and stops, so a reader holding one of %v "+
			"concludes it ships — it does not. State the catch-all and name at least one file it is the only "+
			"thing covering.", catchAllProbes)
	}

	// 🔴 And it must name an UNDOTTED one. Every name in the section was
	// `.env.`-dotted, so the sentence "every file whose base name starts with
	// `.env`" was illustrated exclusively by examples that are ALSO consistent
	// with a `.env.`-prefix rule — the reader with `.envrc` in the project could
	// not answer their question from it, which is the only question this section
	// exists to answer for a name it does not list.
	var undottedProbes, undottedDocumented []string
	for _, name := range catchAllProbes {
		if strings.HasPrefix(name, ".env.") {
			continue
		}
		undottedProbes = append(undottedProbes, name)
		if strings.Contains(rest, name) {
			undottedDocumented = append(undottedDocumented, name)
		}
	}
	if len(undottedProbes) == 0 {
		t.Fatal("CONTROL failure: catchAllProbes holds no undotted `.env`-prefixed name, so the boundary " +
			"between a `.env` prefix and a `.env.` prefix is pinned by nothing above and unasserted here")
	}
	if len(undottedDocumented) == 0 {
		t.Errorf("every `.env` name the README section shows is `.env.`-dotted, so it illustrates a rule "+
			"narrower than the one it states. Name an undotted one (%v) — `.envrc` is the direnv convention "+
			"and routinely holds exported credentials, and it is EXCLUDED.", undottedProbes)
	}
}

// updateDotenvGolden re-approves the pinned section below.
var updateDotenvGolden = flag.Bool("update", false, "rewrite the pinned README dotenv section")

const dotenvGoldenPath = "testdata/readme_dotenv_section.golden.txt"

// 🔴 TestREADMEDotenvSectionIsTheReviewedCopy PINS THE SECTION WHOLE, because
// the thing that has been wrong here twice is the EXPLANATION, and no ledger
// can check an explanation.
//
// This section shipped, in successive revisions, two false safety claims about a
// file the CLI UPLOADS: that `.env.production` "by construction cannot hold a
// secret" (wrong — Vite inlines only `VITE_`-prefixed variables, so a plain
// `API_SECRET=…` is neither public nor excluded, and this repo itself treats the
// VITE_-prefixed `VITE_LIVE_BLOCK_TOKEN` as a spending secret), and that
// `.env.example` / `.env.sample` hold "no secret" (the allow-list is by NAME;
// `page-money/env.example.tmpl` invites pasting a live token into one).
//
// A banned-phrase list loses to the next phrasing — item 28 records five audit
// paraphrases beating two such lists. What IS computable is whether the prose is
// EXACTLY what was reviewed, so an edit here fails until someone re-approves it
// and puts the diff in the review:
//
//	go test ./internal/pkgzip -run TestREADMEDotenvSectionIsTheReviewedCopy -update
//
// READ that diff. The question it must answer is not "does this read well" but
// "does this sentence assert a property of the FILE'S CONTENTS that nothing in
// isExcludedFile checks". If it does, it is wrong however it is worded.
//
// Residual: comparison is whitespace-collapsed, so a pure re-wrap passes. That
// carries no claim, which is why it is acceptable here.
func TestREADMEDotenvSectionIsTheReviewedCopy(t *testing.T) {
	got := strings.Join(strings.Fields(dotenvSection(t)), " ")
	if got == "" {
		t.Fatal("CONTROL failure: the section normalised to nothing, so an empty golden would match it forever")
	}
	if *updateDotenvGolden {
		if err := os.MkdirAll(filepath.Dir(dotenvGoldenPath), 0o755); err != nil {
			t.Fatalf("create testdata: %v", err)
		}
		if err := os.WriteFile(dotenvGoldenPath, []byte(got+"\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", dotenvGoldenPath, err)
		}
		t.Logf("updated %s — READ THE DIFF", dotenvGoldenPath)
		return
	}
	want, err := os.ReadFile(dotenvGoldenPath)
	if err != nil {
		t.Fatalf("read %s: %v\n\nRe-approve deliberately with -update and review the file it writes.", dotenvGoldenPath, err)
	}
	if strings.TrimSpace(string(want)) != got {
		t.Errorf("the README's dotenv section changed.\n\n  want: %s\n\n  got:  %s\n\n"+
			"🔴 This section tells an author what reaches the platform, and it has carried a FALSE safety claim "+
			"twice. Re-approve with `-update` only after checking the new text asserts nothing about the CONTENTS "+
			"of a file that `isExcludedFile` allow-lists by NAME.",
			strings.TrimSpace(string(want)), got)
	}
}
