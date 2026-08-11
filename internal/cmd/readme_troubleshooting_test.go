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

// ----------------------------------------------------------------------------
// Attribution — issue #361
// ----------------------------------------------------------------------------
//
// 🔴 THE GUARD ABOVE ASKS "DOES THIS STRING EXIST?", WHICH IS NOT THE QUESTION A
// READER OF THE INDEX IS ASKING. They are holding an error from ONE command and
// want the row for THAT command.
//
// Measured drift (#361): the row for `is this an App project?` said it came from
// `civitai app validate` and linked to Validate fidelity. `app validate` has
// never printed it — it produces its own finding, `block.manifest.json not found
// at project root <dir>`, which the index did not carry at all. The string is
// real (`internal/manifest`, reached by `app submit` / `app listing` /
// `app dev-token` / `app dev-tunnel`), so the existence guard stayed green the
// whole time, and the one message a new author is most likely to hit was the one
// the index could not resolve.
//
// So this is a RELATIONSHIP guard, not a component one: for a curated set of
// rows it drives the REAL command through NewRootCmd and asserts the row's
// fragment is in what that command actually emitted — plus, where two rows are
// confusable, that the OTHER command does not emit it. A row can only pass by
// being about the command it claims.
//
// It is deliberately a ledger of a few rows rather than every row: proving
// reachability means driving each command into its failure mode, and several of
// them need a live server. Ledger membership is the claim "these rows are worth
// the run", and the confusable pair from #361 is why the ledger exists.

// attributionRun is one real invocation, driven through the same NewRootCmd path
// a user gets. `args` is a function so a row can point the command at a scratch
// directory the harness makes.
type attributionRun struct {
	name string
	args func(emptyProject, scratch string) []string
	env  map[string]string
}

// symptomAttribution is one Troubleshooting row and the commands it belongs to.
type symptomAttribution struct {
	// fragment is the row's first column, verbatim after the extractor's
	// backtick/ellipsis trim — i.e. exactly what documentedSymptoms returns.
	fragment string
	// anchor must appear in the row's "Where to read more" cell, and must be a
	// heading that exists. A right string under a wrong link is still a row that
	// sends the reader somewhere that does not explain their error.
	anchor string
	// emittedBy must each really print fragment.
	emittedBy []attributionRun
	// notEmittedBy must each really NOT print it. This is the half that names
	// the defect: `app validate` printing `is this an App project?` is precisely
	// the world the old row described.
	notEmittedBy []attributionRun
	why          string
}

func symptomAttributions() []symptomAttribution {
	appValidate := func(name string) attributionRun {
		return attributionRun{
			name: name,
			args: func(empty, _ string) []string { return []string{"app", "validate", empty} },
		}
	}
	return []symptomAttribution{
		{
			fragment: "not found at project root",
			anchor:   "#validate-fidelity",
			emittedBy: []attributionRun{
				appValidate("app validate"),
				{
					// --package-only never contacts the server, and this row
					// never gets that far anyway: validation fails first. It is
					// here because the README row claims `app submit` validates
					// first, and an unmeasured claim in the index is the thing
					// this file exists to stop.
					name: "app submit --package-only",
					args: func(empty, scratch string) []string {
						return []string{"app", "submit", empty, "--package-only", "--out", filepath.Join(scratch, "b.zip")}
					},
				},
			},
			why: "the manifest-missing verdict is the most likely error in the whole tool, and until #361 " +
				"the index did not contain it in any form",
		},
		{
			fragment: "is this an App project?",
			anchor:   "#listing-media-requirements",
			emittedBy: []attributionRun{
				{
					// The slug resolve fails on the manifest before any request
					// is built, so the token only has to be non-empty to get
					// past the credential gate.
					name: "app listing status",
					args: func(empty, _ string) []string {
						return []string{"app", "listing", "status", "--dir", empty}
					},
					env: map[string]string{"CIVITAI_TOKEN": "not-a-real-token"},
				},
			},
			notEmittedBy: []attributionRun{
				appValidate("app validate"),
				{
					// `app submit` DOES call manifest.Load — but only after
					// validation has passed, so a missing manifest never reaches
					// it. Listing the caller is not the same as measuring the
					// message, and this row is the difference: the first draft of
					// this fix credited `app submit`, `app dev-token` and
					// `app dev-tunnel` from a grep of manifest.Load callers, and
					// none of the three prints it for a missing manifest.
					name: "app submit --package-only",
					args: func(empty, scratch string) []string {
						return []string{"app", "submit", empty, "--package-only", "--out", filepath.Join(scratch, "b.zip")}
					},
				},
			},
			why: "#361 itself: the row credited this string to `app validate`, which has never printed it, " +
				"and sent the reader to a section that does not explain it",
		},
	}
}

// runAttribution executes one invocation and returns everything the user would
// see: stdout, stderr, and the error text `main` prints as `Error: …`.
func runAttribution(t *testing.T, r attributionRun, empty, scratch string) string {
	t.Helper()
	// Never let a README guard reach the network for a version check.
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "1")
	for k, v := range r.env {
		t.Setenv(k, v)
	}
	stdout, stderr, err := run(t, r.args(empty, scratch)...)
	var b strings.Builder
	b.WriteString(stdout)
	b.WriteString(stderr)
	if err != nil {
		b.WriteString(err.Error())
	}
	return b.String()
}

// TestREADMETroubleshootingRowsAreAttributedToTheEmittingCommand is the #361
// guard: existence -> attribution.
//
// Red/green matrix, measured: with README.md at `origin/main` and this file at
// HEAD, both ledger rows fail — the first because no row quotes the string at
// all, the second because its link cell says `#validate-fidelity`. Both pass
// with the README fix. The `notEmittedBy` half is an INVARIANT guard rather than
// regression coverage: `app validate` did not print the string before the fix
// either. It is here so a future "helpfully" shared manifest loader cannot make
// the old row retroactively true.
func TestREADMETroubleshootingRowsAreAttributedToTheEmittingCommand(t *testing.T) {
	md := readREADME(t)
	section := readmeTroubleshootingSection(t)
	ledger := symptomAttributions()

	// Positive control on the ledger itself: an empty (or accidentally emptied)
	// ledger passes every assertion below while checking nothing.
	if len(ledger) < 2 {
		t.Fatalf("the attribution ledger holds %d rows — it must at least cover the confusable pair from #361", len(ledger))
	}

	for _, a := range ledger {
		t.Run(a.fragment, func(t *testing.T) {
			row, ok := troubleshootingRowFor(section, a.fragment)
			if !ok {
				t.Fatalf("the Troubleshooting index has no row whose first column quotes %q.\n"+
					"Why it matters: %s.\n"+
					"A reader searching this page for the error in front of them finds nothing.", a.fragment, a.why)
			}
			if len(row) < 3 {
				t.Fatalf("row for %q has %d cells, want 3: %v", a.fragment, len(row), row)
			}
			if !strings.Contains(row[2], "("+a.anchor+")") {
				t.Errorf("the row for %q links to %q, but must point at %s.\n"+
					"Why it matters: %s.\n"+
					"A row whose string is right and whose link is wrong still lands the reader on a "+
					"section that does not explain their error — which is the #361 defect exactly.",
					a.fragment, strings.TrimSpace(row[2]), a.anchor, a.why)
			}
			// The link has to go somewhere. GitHub renders a dead anchor as a
			// silent no-op, so this cannot be left to review.
			if !readmeHasAnchor(md, a.anchor) {
				t.Errorf("the row for %q points at %s, which is not a heading in README.md", a.fragment, a.anchor)
			}

			for _, r := range a.emittedBy {
				t.Run("emitted by "+r.name, func(t *testing.T) {
					out := runAttribution(t, r, t.TempDir(), t.TempDir())
					// Positive control on the runner: a command that printed
					// nothing would satisfy notEmittedBy below unconditionally.
					if strings.TrimSpace(out) == "" {
						t.Fatalf("`civitai %s` emitted nothing at all — this harness is not observing the command",
							strings.Join(r.args("<dir>", "<scratch>"), " "))
					}
					if !strings.Contains(out, a.fragment) {
						t.Errorf("README credits %q to `%s`, but that command did not print it.\n"+
							"Why it matters: %s.\nWhat it printed:\n%s", a.fragment, r.name, a.why, out)
					}
				})
			}
			for _, r := range a.notEmittedBy {
				t.Run("not emitted by "+r.name, func(t *testing.T) {
					out := runAttribution(t, r, t.TempDir(), t.TempDir())
					if strings.TrimSpace(out) == "" {
						t.Fatalf("`civitai %s` emitted nothing at all — the absence below would be a fact "+
							"about the harness, not about the command",
							strings.Join(r.args("<dir>", "<scratch>"), " "))
					}
					if strings.Contains(out, a.fragment) {
						t.Errorf("`%s` prints %q, which the README attributes elsewhere.\n"+
							"Either the row is now wrong or this ledger is: %s\nWhat it printed:\n%s",
							r.name, a.fragment, a.why, out)
					}
				})
			}
		})
	}
}

// troubleshootingRowFor returns the cells of the first Troubleshooting row whose
// FIRST column quotes fragment. Matching on the first column only is the point:
// a fragment mentioned in another row's prose must not satisfy the lookup.
func troubleshootingRowFor(section, fragment string) ([]string, bool) {
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		if len(cells) == 0 {
			continue
		}
		if strings.Contains(cells[0], fragment) && strings.Contains(cells[0], "`") {
			return cells, true
		}
	}
	return nil, false
}

// readmeHasAnchor reports whether a GitHub-style `#anchor` resolves to a heading
// in md. The slug rules mirror GitHub's: lowercase, drop everything that is not
// alphanumeric/space/hyphen, spaces to hyphens — which is why a 🔴 heading's
// anchor carries a leading hyphen.
func readmeHasAnchor(md, anchor string) bool {
	want := strings.TrimPrefix(anchor, "#")
	for _, line := range strings.Split(md, "\n") {
		if !strings.HasPrefix(line, "#") {
			continue
		}
		text := strings.TrimSpace(strings.TrimLeft(line, "#"))
		if githubAnchorSlug(text) == want {
			return true
		}
	}
	return false
}

func githubAnchorSlug(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
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
