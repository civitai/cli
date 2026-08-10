package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The README's Troubleshooting section is a SYMPTOM INDEX: its first column is a
// fragment of a string the CLI really prints, so a reader can search the page
// for the words in front of them and land on the cause.
//
// That only works while the strings are real. The section it replaced was five
// bullets that had drifted to cover almost nothing the binary emits — the exact
// rot this file exists to make impossible. A documented symptom that no longer
// appears anywhere in the source is a row pointing at a message the user can
// never receive, and nothing else in the suite can see it: every other README
// guard here pins a command, a flag or a constant, never the prose of an error.
//
// 🔴 The guard is a DRIFT detector over the source, deliberately not an
// assertion that each string is reachable at runtime. Proving reachability
// would mean driving every one of these commands into its failure mode, which
// is a different (and much larger) test than "the README stopped describing
// this product". What it does catch is the change that actually happens:
// somebody rewords an error and the README keeps quoting the old sentence.

// symptomRowRe matches a Troubleshooting table row and captures its first cell.
// Rows are `| `symptom` | cause | link |`; the header and the `| --- |` divider
// do not start with a backtick run and so are skipped.
var symptomRowRe = regexp.MustCompile("(?m)^\\|\\s*(`{1,2}[^|]+?`{1,2})\\s*\\|")

// goStringConcatRe joins ADJACENT Go string literals, so that
//
//	"the host will not " +
//		"reveal a page app"
//
// is searchable as one phrase. Without this, a symptom quoted from a message
// that Go source happens to split across a `+` would be reported missing — a
// false failure with a confusing message, which is how a guard gets deleted.
var goStringConcatRe = regexp.MustCompile(`"\s*\+\s*"`)

// symptomSourceCorpus returns every non-test source file the CLI's user-facing
// strings can live in, concatenated and literal-joined.
//
// It spans `internal/`, `cmd/` and `pkg/` because the symptoms genuinely do:
// the scaffold refusals are in `internal/scaffold`, the ready-ack advisory in
// `internal/validate`, the transport errors in `pkg/civitai`. Template files are
// included because one documented symptom (`block lacks ai:write:budgeted
// scope`) is a message the scaffolded app itself surfaces.
//
// `_test.go` files are excluded on purpose, and it is load-bearing: this very
// file quotes several of these strings, so a corpus that included tests would
// find every symptom in its own source and pass unconditionally.
func symptomSourceCorpus(t *testing.T) string {
	t.Helper()
	root := repoRootDir(t)
	var b strings.Builder
	var files int
	for _, sub := range []string{"internal", "cmd", "pkg"} {
		err := filepath.Walk(filepath.Join(root, sub), func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			name := info.Name()
			if strings.HasSuffix(name, "_test.go") {
				return nil
			}
			switch {
			case strings.HasSuffix(name, ".go"),
				strings.HasSuffix(name, ".tmpl"),
				strings.HasSuffix(name, ".js"):
			default:
				return nil
			}
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			files++
			b.Write(raw)
			b.WriteString("\n")
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", sub, err)
		}
	}
	// Positive control on the corpus itself. A walk that read nothing would make
	// every lookup below fail for the wrong reason, and a walk that read only a
	// handful would fail intermittently as files move.
	if files < 100 {
		t.Fatalf("the symptom corpus read only %d source files — the walk is wrong, so any verdict "+
			"below would be a fact about the walk rather than about the README", files)
	}
	return goStringConcatRe.ReplaceAllString(b.String(), "")
}

// readmeTroubleshootingSection returns the README text under `## Troubleshooting`,
// raw, up to the next `##`.
func readmeTroubleshootingSection(t *testing.T) string {
	t.Helper()
	md := readREADME(t)
	const heading = "\n## Troubleshooting\n"
	i := strings.Index(md, heading)
	if i < 0 {
		t.Fatal("README.md has no `## Troubleshooting` section — the symptom index a reader " +
			"searches for their error message is gone")
	}
	body := md[i+len(heading):]
	if j := strings.Index(body, "\n## "); j >= 0 {
		body = body[:j]
	}
	return body
}

// documentedSymptoms returns the literal symptom strings the Troubleshooting
// table's first column names.
//
// A cell may hold several alternatives separated by ` / ` (e.g. `insufficient
// Buzz` / `generation disabled`), and each is a separate claim, so each is
// returned separately.
//
// An ellipsis at EITHER end is presentational, marking where the real message
// interpolates a value: the README writes `cannot derive a slug from …` because
// the name follows, and `… cannot appear in a blockId` because the offending
// characters precede. Both are trimmed. Trimming only the trailing one is not a
// harmless simplification — it reported the leading-ellipsis row as a missing
// string, which is a false failure, and a guard that cries wolf is a guard
// somebody deletes.
func documentedSymptoms(t *testing.T, section string) []string {
	t.Helper()
	var out []string
	for _, m := range symptomRowRe.FindAllStringSubmatch(section, -1) {
		for _, part := range strings.Split(m[1], "` / `") {
			s := strings.TrimSpace(strings.Trim(strings.TrimSpace(part), "`"))
			s = strings.TrimSpace(strings.Trim(s, "…"))
			if s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// TestREADMETroubleshootingSymptomsExistInTheSource is the anti-rot guard for
// the symptom index.
//
// Mutation-verified: rewording a documented message in its source file (e.g.
// `refusing to submit without --yes` -> `refusing to submit without --confirm`
// in app_submit.go) reddens this test, and the failure NAMES the string that no
// longer exists rather than reporting a count.
func TestREADMETroubleshootingSymptomsExistInTheSource(t *testing.T) {
	section := readmeTroubleshootingSection(t)
	symptoms := documentedSymptoms(t, section)

	// Positive control on the extractor. A regex typo, a reformatted table or a
	// section that lost its rows all look identical to "everything passed", and
	// this is the shape that reads most reassuringly while checking nothing.
	if len(symptoms) < 15 {
		t.Fatalf("extracted only %d symptom strings from the Troubleshooting section, want >= 15 — "+
			"the row extractor is reading the wrong text (pattern: %s).\nsection:\n%s",
			len(symptoms), symptomRowRe, section)
	}

	// Validate the INSTRUMENT before reading its verdict: the searcher must be
	// able to report a miss at all. A corpus builder wired to something huge and
	// irrelevant would find every short phrase and pass over anything.
	corpus := symptomSourceCorpus(t)
	const absent = "this sentence is not in the civitai CLI source anywhere at all"
	if strings.Contains(corpus, absent) {
		t.Fatalf("the negative control %q was found in the corpus — the searcher cannot report a miss", absent)
	}

	for _, s := range symptoms {
		if !strings.Contains(corpus, s) {
			t.Errorf("README Troubleshooting documents the symptom %q, but no non-test source file "+
				"contains that string.\n"+
				"Either the message was reworded (update the README row — a reader searching for the "+
				"text in front of them will not find it) or the row was invented. "+
				"The index is only navigable while every row quotes a string the CLI really prints.", s)
		}
	}
}

// TestREADMETroubleshootingCoversTheRefusalsAuthorsActuallyHit pins the FLOOR of
// what the section must cover, independently of the guard above.
//
// The two are not redundant, and neither subsumes the other: the sibling asks
// "is every row real?", which a section that had been cut down to three
// uncontroversial rows would satisfy perfectly. This asks "are the expensive
// ones still here?" — the refusals that stop an author mid-task and that the
// pre-index section documented none of. Deleting a row to make the sibling green
// is exactly the repair this forbids.
//
// Each entry is derived from the CODE's own constant where one is exported, so a
// reword moves the expectation and the README together rather than leaving this
// test asserting a string nothing produces.
func TestREADMETroubleshootingCoversTheRefusalsAuthorsActuallyHit(t *testing.T) {
	section := readmeTroubleshootingSection(t)

	cases := []struct {
		want, why string
	}{
		{"refusing to submit without --yes",
			"the CI-blocking refusal; a scripted `app submit` cannot get past it and the README never mentioned it"},
		{"cannot derive a slug from",
			"the blockId refusal — it stops `app create` dead, and the identity it guards can never be renamed"},
		{"no such directory — pass the path to an App project root",
			"the exit-2 path classification, whose whole point is being distinguishable from a validation verdict"},
		{"refusing to overwrite. Scaffold somewhere else",
			"the not-empty-directory refusal, which has no --force and so must name its remedy"},
		{"block lacks ai:write:budgeted scope",
			"the dev:live money-path dead end, reached with a credential that looks entirely valid"},
		{"it did NOT check that the file is loaded",
			"the ready-ack advisory's weak tier — the disclosure that keeps `valid` from reading as `wired`"},
	}
	for _, tc := range cases {
		if !strings.Contains(section, tc.want) {
			t.Errorf("the Troubleshooting symptom index no longer documents %q.\n"+
				"It is on the floor because: %s.\n"+
				"If it was removed deliberately, remove this row too — but do not let the index "+
				"quietly stop covering the messages that halt an author.", tc.want, tc.why)
		}
	}
}
