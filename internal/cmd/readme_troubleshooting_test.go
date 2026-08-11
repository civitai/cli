package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"

	"github.com/spf13/cobra"
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
// confusable, that the OTHER command does not emit it.
//
// 🔴 AND IT READS THE ROW'S OWN PROSE, WHICH THE FIRST VERSION DID NOT. That
// version examined `row[0]` (the fragment) and `row[2]` (the link), never
// `row[1]` — the cell that says WHICH COMMAND PRINTS IT. So `emittedBy` /
// `notEmittedBy` were Go-side labels decoupled from the sentence a reader
// actually reads, and the #361 defect could be re-typed verbatim under a green
// suite: rewriting the cell to "Printed by `civitai app validate` … `app
// listing` never prints it" left every assertion passing. The red/green matrix
// that shipped with it reddened on the LINK cell, so attribution was never the
// discriminator. `attributionProseCheck` below is the repair: the cell's
// command mentions are parsed out and must AGREE with the measured ledger, in
// both directions and with nothing unmeasured left over.
//
// It is deliberately a ledger of a few rows rather than every row: proving
// reachability means driving each command into its failure mode, and several of
// them need a live server. Ledger membership is the claim "these rows are worth
// the run", and the confusable pair from #361 is why the ledger exists.

// attributionRun is one real invocation, driven through the same NewRootCmd path
// a user gets. `args` is a function so a row can point the command at a scratch
// directory the harness makes.
//
// 🔴 There is deliberately no `readmeName` field any more, and #373 is why. It
// held the EXACT inline-code text the cause cell had to use, matched by string
// EQUALITY on the normalised span — which reddened on three classes of CORRECT
// prose, including this ledger's OWN measured argv (`app listing status --dir
// <path>` is what the harness runs, and the cell was not allowed to say so
// because the field said `app listing`). A prose span is now matched against
// the run's REAL argv instead; see resolveSpan for the rule and for how F1's
// `app submit` / `app submit --skip-validate` distinction survives the change.
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
	appValidate := func() attributionRun {
		return attributionRun{
			name: "app validate",
			args: func(empty, _ string) []string { return []string{"app", "validate", empty} },
		}
	}
	// `app submit` on a manifest-less directory, with and without the flag that
	// waives validation. THE TWO PRINT DIFFERENT MESSAGES, and the pair is the
	// whole of F1: the shipped rows said "`app validate` and `app submit` never
	// print [`is this an App project?`]", which is true only for the default
	// argv. `--skip-validate` is documented on this same page, and `app submit`'s
	// own validation failure hands the reader the falsifier in one hop ("fix
	// before submitting, or pass --skip-validate").
	appSubmit := func(skipValidate bool) attributionRun {
		name := "app submit"
		if skipValidate {
			name = "app submit --skip-validate"
		}
		return attributionRun{
			// --package-only never contacts the server. Without --skip-validate
			// the run never gets that far anyway (validation fails first); WITH
			// it, packaging is reached and manifest.Load is what fails.
			name: name + " --package-only",
			args: func(empty, scratch string) []string {
				a := []string{"app", "submit", empty}
				if skipValidate {
					a = append(a, "--skip-validate")
				}
				return append(a, "--package-only", "--out", filepath.Join(scratch, "b.zip"))
			},
		}
	}
	return []symptomAttribution{
		{
			fragment: "not found at project root",
			// NOT #validate-fidelity: that section is about how faithfully the
			// LOCAL mirror reproduces the server's approve-time validator, and
			// never mentions a missing manifest, a project root, or what to do
			// about one. Exit code 1 states this exact case in so many words —
			// "a real directory with no block.manifest.json at its root — you
			// pointed at a real place, so the invocation was right and the
			// project is wrong" — which is what the row's last sentence is
			// about. The anchor assertion below is tautological on its own (the
			// ledger constant IS the expected value), so the choice is argued
			// here rather than pinned there.
			anchor: "#exit-code-1",
			emittedBy: []attributionRun{
				appValidate(),
				appSubmit(false),
			},
			notEmittedBy: []attributionRun{
				appSubmit(true),
			},
			why: "the manifest-missing verdict is the most likely error in the whole tool, and until #361 " +
				"the index did not contain it in any form",
		},
		{
			fragment: "is this an App project?",
			// NOT #listing-media-requirements: that section is image formats,
			// byte caps and aspect ratios, and says nothing about how `app
			// listing` decides WHICH app you mean. "After you submit" is where
			// the `app listing` flow is walked through, and it states both the
			// working-directory resolution and the --slug/--dir remedy this row
			// sends the reader to use.
			//
			// 🔴 It did not GAIN that remedy with this row, and #373 corrects
			// the claim that it did (#368's F5: the section "now documents …
			// the --slug/--dir remedy"). Measured: `--slug <blockId>` and
			// `--dir` were already documented in this same section BEFORE that
			// change — README@fc03d33:1255, a bullet still shipping today at
			// README:1306, about twenty lines below the block #368 added, which
			// now says it a second time. What #368 really added is the worked
			// failure above it: the transcript of `app listing status` run from
			// the wrong directory. So the anchor is the right choice for a
			// weaker reason than the one that was recorded — the section
			// explained the remedy all along, and the row could have pointed
			// here from the start.
			anchor: "#after-you-submit-review--approve--deploy",
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
				appSubmit(true),
			},
			notEmittedBy: []attributionRun{
				appValidate(),
				// `app submit` DOES call manifest.Load — but only after
				// validation has passed, so a missing manifest never reaches it
				// UNLESS --skip-validate waives the validation. Listing the
				// caller is not the same as measuring the message, and this row
				// is the difference: the first draft of this fix credited `app
				// submit`, `app dev-token` and `app dev-tunnel` from a grep of
				// manifest.Load callers. dev-token and dev-tunnel discard the
				// load error and still print nothing; `app submit` prints it on
				// exactly one argv, which is why the SAME constructor appears on
				// both rows — row 1 as an emitter, here as a non-emitter.
				//
				// It is a call, not a field-by-field copy of one: #373 found the
				// copy byte-equivalent today and a drift seam on the exact
				// identity this guard resolves prose spans against.
				appSubmit(false),
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
// Red/green matrix, re-measured for #373 against a PINNED ref rather than
// `origin/main` — which is what this said, and which has since moved past the
// fix, so the sentence had stopped describing anything. With README.md at
// `9c7baed` (the commit before #361 was repaired) and this file at HEAD, both
// ledger rows fail: row 1 because no row quotes the string at all, row 2 on TWO
// counts — its cause cell names no invocation the parser can see, and its link
// cell says `#validate-fidelity`. Both pass with the README at HEAD.
//
// The `notEmittedBy` half is an INVARIANT guard rather than regression coverage:
// `app validate` did not print the string before the fix either. It is here so a
// future "helpfully" shared manifest loader cannot make the old row
// retroactively true.
//
// The `attributes` subtest is the one that reads `row[1]`. Without it every
// assertion here is about a cell a reader never reads for attribution, and the
// #361 defect can be re-typed into the prose under a green suite.
func TestREADMETroubleshootingRowsAreAttributedToTheEmittingCommand(t *testing.T) {
	md := readREADME(t)
	section := readmeTroubleshootingSection(t)
	ledger := symptomAttributions()
	paths := knownCommandPaths(t)

	// Positive control on the ledger itself: an empty (or accidentally emptied)
	// ledger passes every assertion below while checking nothing.
	if len(ledger) < 2 {
		t.Fatalf("the attribution ledger holds %d rows — it must at least cover the confusable pair from #361", len(ledger))
	}

	for _, a := range ledger {
		t.Run(a.fragment, func(t *testing.T) {
			row, n := troubleshootingRowFor(section, a.fragment)
			if n == 0 {
				t.Fatalf("the Troubleshooting index has no row whose first column quotes %q.\n"+
					"Why it matters: %s.\n"+
					"A reader searching this page for the error in front of them finds nothing.", a.fragment, a.why)
			}
			if n > 1 {
				// Two rows quoting the same fragment means this test picked one
				// of them and said nothing about the other — and a reader
				// searching the page has the same problem.
				t.Fatalf("%d Troubleshooting rows quote %q in their first column. "+
					"This guard can only speak about one of them, so the others would drift unwatched: "+
					"merge them, or make each fragment name exactly one row.", n, a.fragment)
			}
			if len(row) < 3 {
				t.Fatalf("row for %q has %d cells, want 3: %v", a.fragment, len(row), row)
			}
			t.Run("attributes it to the ledger's commands", func(t *testing.T) {
				attributionProseCheck(t, a, row[1], paths)
			})
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

// troubleshootingRowFor returns the cells of the Troubleshooting row whose FIRST
// column quotes fragment, plus HOW MANY rows matched. Matching on the first
// column only is the point: a fragment mentioned in another row's prose must not
// satisfy the lookup. Returning the count matters too — the first version
// returned the first match, so a later row whose first cell is a superset of an
// earlier one's would silently shadow the row this ledger means.
func troubleshootingRowFor(section, fragment string) ([]string, int) {
	var first []string
	var n int
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
			n++
			if first == nil {
				first = cells
			}
		}
	}
	return first, n
}

// ----------------------------------------------------------------------------
// Reading the attribution out of the row's own prose
// ----------------------------------------------------------------------------

// The cause cell is English prose, and this parser has to answer two questions
// about it: WHICH measured invocation each inline-code span names, and WHETHER
// the sentence says that invocation prints the message or says it does not.
//
// 🔴 #373 — the first version answered both with flat string matching, and all
// three of those decisions reddened on CORRECT prose:
//
//  1. A span had to EQUAL a hand-written `readmeName`. So `app listing status
//     --dir <path>` — the ledger's OWN measured argv — was reported as "a
//     command this ledger never runs", because the field said `app listing`.
//     So was a bare `app` used as an English noun ("your `app` directory"):
//     19 of the tree's command paths are single words, several of them ordinary
//     nouns.
//  2. A clause was negative iff it contained one of seven literal phrases, all
//     of them `…print`. `do not emit it` was therefore a POSITIVE attribution;
//     `do not print it` passed.
//  3. That polarity coloured the whole clause, so "`app validate` never prints
//     it, but `app listing` does" made both negative.
//
// This matters more than the friction: the file's own F4 note says a guard that
// fails on correct input is one somebody deletes, and the two cause cells this
// runs on are the highest-traffic prose on that page.
//
// The repair is structural on all three counts. A span is resolved against the
// run's REAL argv (resolveSpan) rather than against a name somebody typed twice.
// Negation is a property of a PREDICATE — a printing verb with a negator on it —
// and each mention takes the polarity of the NEAREST predicate, so one negation
// no longer colours a sentence that changes direction. A contrastive conjunction
// ends a predicate's scope, which is what "…, but X does" means in English.
//
// What remains is vocabulary: two closed word sets, matched with English
// inflection rather than as whole phrases, so rewording around the same verb
// needs no new entry.
//
// 🔴 The direction it fails in is NOT uniform, and an earlier draft of this
// comment claimed it was. A denial phrased with a verb this file does not list
// leaves the mention at its DEFAULT polarity, which is POSITIVE — measured:
//
//	`app listing` does not output it.   -> read as a POSITIVE attribution
//	`app listing` never produces it.    -> read as a POSITIVE attribution
//
// So for a NON-emitter an unlisted denial goes red (the required negative
// never appears), but for an EMITTER it goes GREEN — appended to the shipped
// row-2 cell, both sentences above yield ZERO problems while `never prints it`
// yields one. That is a live false negative, not a regression: the seven-phrase
// predecessor behaved identically. It is the honest boundary of the guard —
// it catches a wrong attribution, not every wrong SENTENCE — and the fix for a
// missed phrasing is a word in printVerbStems, never a special case.
//
// 🔴 The same boundary has a DISTANCE half, and it fails the other way. A
// negator reaches its verb across at most three tokens, so a denial that pushes
// them further apart reads POSITIVE — and for a NON-emitter that reddens
// correct prose. All four of these are natural English and all four are read as
// attributions today:
//
//	`app validate` does not, as of today, print it.
//	`app validate` does not, on this path, ever print it.
//	`app validate` will not, under any circumstances, print it.
//	`app validate` never actually goes on to print it.
//
// Widening the window is not free: it is bounded from the other side by
// sentences where the negator belongs to a different word ("finds there is no
// manifest, so it prints this"), and both boundaries now carry a fixture. Three
// is where those two natural readings stop colliding, not a number with a
// theory behind it. Pre-existing since the first cut of #373; the remedy in
// prose is to put the negator next to its verb.
//
// ONE rule here is deliberately not pinned: the SPACE requirement on the `.`/`;`
// clause split, which exists so a filename or a version number in unfenced prose
// ("block.manifest.json", "v1.2.3") cannot cut a clause in half. No such text is
// in either cell today and every fixture that would create one is contrived, so
// it survives mutation on purpose.
//
// 🔴 The nearest-predicate TIE-BREAK used to be on that list, with the reason
// "both readings are arbitrary and no natural sentence distinguishes them". That
// was false, and the #387 delta audit built the counterexample — a rephrase of
// the shipped cell's own sentence, with a predicate two tokens away on each
// side:
//
//	Not printed by `app validate`, which reports the row above.
//
// The earlier predicate is the negated one, so keeping it on a tie is the
// correct English reading. Flipping to `<=` hands the tie to `reports` and the
// guard announces "credits `app validate` with printing it" about CORRECT
// prose — the exact false-positive class this file exists to remove. It is a
// load-bearing claim, and it is now pinned by a fixture.
//
// Rejected alternative: require the cause cell to carry its attribution in a
// fixed machine-readable syntax. That is fully deterministic and needs no
// vocabulary at all — and it would dictate the wording of the sentence the
// reader reads, which is the thing this row exists to get right. The guard is
// here to follow the prose, not to author it.

// printVerbStems are the verbs a cause cell uses to say a command PRODUCES the
// message. Matched with regular English inflection (-s/-es/-ed/-d/-ing) plus the
// irregulars below, so "prints", "printed", "emits" and "reported" all count
// without being enumerated.
var printVerbStems = []string{"print", "emit", "report", "say", "show", "surface", "display"}

// printVerbIrregulars are the inflections the suffix rule does not generate.
var printVerbIrregulars = map[string]bool{"said": true, "shown": true}

// attributionNegators flip a printing predicate. This is the closed English
// negator set, NOT a list of printing phrasings: these words negate whatever
// verb they attach to, and the parser only ever consults them next to a verb
// from printVerbStems. That is what keeps "which did not validate first" — a
// real sentence in the shipped README — from reading as a claim about printing.
var attributionNegators = map[string]bool{
	"not": true, "never": true, "no": true, "neither": true, "nor": true,
	"nothing": true, "cannot": true, "can't": true, "won't": true,
	"doesn't": true, "don't": true, "didn't": true, "isn't": true, "aren't": true,
	"without": true,
}

// contrastConjunctions end the scope of a preceding predicate: in "X never
// prints it, but Y does", the negation stops at "but". Only unambiguously
// contrastive words are listed — "while" and "yet" have common non-contrastive
// readings ("while waiting", "no approved version yet") and would cut a clause
// in half for the wrong reason.
var contrastConjunctions = map[string]bool{
	"but": true, "whereas": true, "although": true, "though": true,
}

// proseToken is one word or one inline-code span of a clause, kept in order so
// that "the nearest predicate" is a distance a machine can compute.
type proseToken struct {
	code bool   // an inline-code span — a candidate command mention
	text string // code: the span's contents; word: lowercased, punctuation stripped
}

// tokenizeClause splits a clause into words and inline-code spans.
func tokenizeClause(clause string) []proseToken {
	var out []proseToken
	rs := []rune(clause)
	for i := 0; i < len(rs); i++ {
		switch {
		case rs[i] == '`':
			j := i + 1
			for j < len(rs) && rs[j] != '`' {
				j++
			}
			out = append(out, proseToken{code: true, text: string(rs[i+1 : j])})
			i = j
		case unicode.IsSpace(rs[i]):
			// separator
		default:
			j := i
			for j < len(rs) && !unicode.IsSpace(rs[j]) && rs[j] != '`' {
				j++
			}
			w := strings.ToLower(strings.Trim(string(rs[i:j]), `.,;:()*_—"'`))
			if w != "" {
				out = append(out, proseToken{text: w})
			}
			i = j - 1
		}
	}
	return out
}

// isPrintVerb reports whether a word claims the message was PRODUCED.
func isPrintVerb(w string) bool {
	if printVerbIrregulars[w] {
		return true
	}
	for _, stem := range printVerbStems {
		switch w {
		case stem, stem + "s", stem + "es", stem + "ed", stem + "d", stem + "ing":
			return true
		}
	}
	return false
}

// spanPolarity is one inline-code span of a clause with the polarity the
// sentence gives it.
type spanPolarity struct {
	span    string
	negated bool
}

// attributionSpansIn assigns a polarity to every inline-code span in one clause.
// The clause is first cut at contrastive conjunctions, because a predicate
// governs only its own side of a contrast.
func attributionSpansIn(clause string) []spanPolarity {
	tokens := tokenizeClause(clause)
	var out []spanPolarity
	start := 0
	for i := 0; i <= len(tokens); i++ {
		if i == len(tokens) || (!tokens[i].code && contrastConjunctions[tokens[i].text]) {
			out = append(out, segmentSpanPolarities(tokens[start:i])...)
			start = i + 1
		}
	}
	return out
}

// segmentSpanPolarities is the polarity rule itself, over one contrast-free
// segment.
//
// The default is POSITIVE, because a cause cell often attributes with no
// printing verb at all — "**`civitai app validate`** found no
// `block.manifest.json` in the directory you named" is the shipped row 1 — and
// because negation is the marked case in English. A span is negative when a
// negator sits immediately in front of it ("but not `app listing`"), and
// otherwise when the NEAREST printing predicate in the segment is negated. The
// nearest-predicate rule is what stops a single negation from colouring spans
// that a later predicate speaks for.
func segmentSpanPolarities(tokens []proseToken) []spanPolarity {
	type predicate struct {
		at      int
		negated bool
	}
	var preds []predicate
	for i, tk := range tokens {
		if tk.code || !isPrintVerb(tk.text) {
			continue
		}
		neg := false
		for j := i - 1; j >= 0 && j >= i-3; j-- {
			if !tokens[j].code && attributionNegators[tokens[j].text] {
				neg = true
				break
			}
		}
		// "prints neither X nor Y" puts the negator AFTER the verb.
		if i+1 < len(tokens) && !tokens[i+1].code && attributionNegators[tokens[i+1].text] {
			neg = true
		}
		preds = append(preds, predicate{at: i, negated: neg})
	}
	var out []spanPolarity
	for i, tk := range tokens {
		if !tk.code {
			continue
		}
		negated := false
		switch {
		case i > 0 && !tokens[i-1].code && attributionNegators[tokens[i-1].text]:
			negated = true
		case len(preds) > 0:
			best := preds[0]
			for _, p := range preds[1:] {
				if absInt(p.at-i) < absInt(best.at-i) {
					best = p
				}
			}
			negated = best.negated
		}
		out = append(out, spanPolarity{span: tk.text, negated: negated})
	}
	return out
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// commandMention is one invocation a cause cell names, which measured run it
// resolves to, and whether the sentence naming it was a negation.
type commandMention struct {
	span    string
	run     int // index into the row's runs, or spanUnmeasured / spanAmbiguous
	negated bool
}

// splitAttributionClauses cuts a cause cell into clauses at sentence ends,
// semicolons and spaced em-dashes — never inside an inline-code span, so a
// documented message containing ". " cannot be torn in half. A COMMA is
// deliberately not a separator: "`app validate` and a plain `app submit` never
// print it, because …" is ONE claim about two commands.
//
// That rationale had no case behind it until #373 — adding ',' here left the
// whole suite green. TestAttributionProseParser now carries a fixture ("`app
// validate`, a plain `app submit` and `app listing` never print it") whose
// leading command falls into a predicate-less segment, and so flips to POSITIVE,
// the moment a comma separates.
func splitAttributionClauses(cell string) []string {
	var clauses []string
	var cur []rune
	inCode := false
	rs := []rune(cell)
	flush := func() {
		if s := strings.TrimSpace(string(cur)); s != "" {
			clauses = append(clauses, s)
		}
		cur = cur[:0]
	}
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == '`' {
			inCode = !inCode
			cur = append(cur, r)
			continue
		}
		if !inCode {
			isBreak := (r == '.' || r == ';') && i+1 < len(rs) && rs[i+1] == ' '
			isDash := r == '—' && i > 0 && rs[i-1] == ' ' && i+1 < len(rs) && rs[i+1] == ' '
			if isBreak || isDash {
				flush()
				i++
				continue
			}
		}
		cur = append(cur, r)
	}
	flush()
	return clauses
}

// normalizeCommandSpan renders an inline-code span the way an argv is written:
// no `civitai ` prefix, no trailing ellipsis, single-spaced.
func normalizeCommandSpan(span string) string {
	s := strings.Join(strings.Fields(span), " ")
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(s, "…"), "..."))
	return strings.TrimSpace(strings.TrimPrefix(s, "civitai "))
}

// knownCommandPaths is every command path in the REAL Cobra tree, minus the root
// name — "app", "app validate", "app listing status", "generate", … Derived from
// the tree rather than a hand-list so a renamed command cannot leave this
// recogniser quietly matching nothing.
//
// The VALUE says whether the path is a FAMILY — a parent that has subcommands.
// That distinction is load-bearing since #373: a family is not an invocation, so
// the `app` in "your `app` directory" is English rather than an attribution.
//
// Why HasSubCommands is the right test, and the scope of the claim: `root.go`
// gives every parent that has subcommands AND no action of its own a RunE that
// prints help (or rejects an unknown subcommand). It cannot be read back off the
// built tree — after that injection every node reports Runnable() — so the
// claim was measured instead, on the two families these cause cells name:
// `civitai app` and `civitai app listing` each print their help and exit 0.
// A future parent that also did work of its own would be misread as a family;
// that fails CLOSED for the run underneath it, which is still matched by its own
// path and flags.
func knownCommandPaths(t *testing.T) map[string]bool {
	t.Helper()
	root := NewRootCmd()
	prefix := root.Name() + " "
	out := map[string]bool{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			out[strings.TrimPrefix(sub.CommandPath(), prefix)] = sub.HasSubCommands()
			walk(sub)
		}
	}
	walk(root)
	// Positive control on the walk: a tree read as empty would make every span
	// below "not a command", and the completeness check would pass vacuously.
	if len(out) < 20 {
		t.Fatalf("the Cobra walk found %d command paths — it is not reading the real tree, so "+
			"no verdict below would be about the README", len(out))
	}
	// Positive control on the family flag itself. `app` and `app listing` are
	// both parents; a walk that recorded none would leave the family rule in
	// resolveSpan switched off rather than satisfied, and nothing else here
	// would notice.
	families := 0
	for _, isFamily := range out {
		if isFamily {
			families++
		}
	}
	if families < 2 {
		t.Fatalf("the Cobra walk found %d parent commands — `app` and `app listing` are both parents, "+
			"so anything below 2 means HasSubCommands is not being read", families)
	}
	return out
}

// commandPathIn returns the longest leading run of WORDS in fields that is a
// real command path. A flag ends the search, so `app submit --skip-validate`
// names `app submit`, while `--json` and `block.manifest.json not found …` name
// nothing.
func commandPathIn(fields []string, paths map[string]bool) (string, bool) {
	var found string
	var ok bool
	for n := 1; n <= len(fields); n++ {
		if strings.HasPrefix(fields[n-1], "-") {
			break
		}
		p := strings.Join(fields[:n], " ")
		if _, exists := paths[p]; exists {
			found, ok = p, true
		}
	}
	return found, ok
}

// argvFlags is the set of flag names in an argv or a prose span, with any
// `=value` cut off.
func argvFlags(fields []string) map[string]bool {
	out := map[string]bool{}
	for _, f := range fields {
		if !strings.HasPrefix(f, "-") {
			continue
		}
		if i := strings.Index(f, "="); i > 0 {
			f = f[:i]
		}
		out[f] = true
	}
	return out
}

// ledgerRun is one measured invocation of a row, plus the argv shape a prose
// span has to match to be read as naming it. Both are derived from `args` — the
// argv IS the measurement, so it is what the prose is held against.
type ledgerRun struct {
	run   attributionRun
	emits bool
	argv  []string
	path  string
	flags map[string]bool
}

// rowRuns flattens a row's ledger into its emitting and non-emitting runs.
func rowRuns(a symptomAttribution, paths map[string]bool) []ledgerRun {
	var out []ledgerRun
	add := func(r attributionRun, emits bool) {
		argv := r.args("<dir>", "<scratch>")
		path, _ := commandPathIn(argv, paths)
		out = append(out, ledgerRun{run: r, emits: emits, argv: argv, path: path, flags: argvFlags(argv)})
	}
	for _, r := range a.emittedBy {
		add(r, true)
	}
	for _, r := range a.notEmittedBy {
		add(r, false)
	}
	return out
}

// Outcomes of resolveSpan that are not an index into the row's runs.
const (
	spanUnmeasured = -1
	spanAmbiguous  = -2
)

// resolveSpan maps one normalised inline-code span onto the row's measured runs:
//
//	(i, true)               — the span names runs[i]
//	(spanUnmeasured, true)  — it names a command this ledger never runs
//	(spanAmbiguous, true)   — it names several measured runs and picks none
//	(0, false)              — it is not an invocation at all
//
// The rules, in order:
//
//   - No command path in front (`--json`, `block.manifest.json not found …`):
//     not an invocation.
//   - A FAMILY with no flags (`app`, `app listing`) is not an invocation either
//     — running it prints help. Its reading depends on how many of the row's
//     runs it spans, and the three cases are NOT interchangeable:
//     EXACTLY ONE, it is shorthand for that run; SEVERAL, it identifies none of
//     them and is not read at all ("your `app` directory", and equally "`app`
//     prints it"); NONE, it is an attribution to something the row never ran and
//     is reported as unmeasured. 🔴 An earlier draft of this comment said a
//     family spanning "none or several" went unchecked, and for one commit the
//     code agreed with it — that was the measured regression the #387 audit
//     caught, and this sentence is the one that outlived the fix by 30 lines.
//     The residual is only the SEVERAL case. Every leaf mention is checked, and
//     #361 was a leaf.
//   - Otherwise the span is an invocation claim, matched against each run whose
//     path it names or prefixes, keeping the runs whose flags are a superset of
//     the span's. Ties are broken on the row's DISTINGUISHING flags — those
//     present on one candidate and absent on another. `--skip-validate` is one,
//     so `app submit` cannot resolve to the --skip-validate run and F1's
//     distinction survives the move off string equality; `--package-only` and
//     `--out` are on both submit runs, so declining to name them is not a claim
//     about anything and the bare `app submit` still lands on the run it means.
func resolveSpan(span string, runs []ledgerRun, paths map[string]bool) (int, bool) {
	fields := strings.Fields(span)
	path, ok := commandPathIn(fields, paths)
	if !ok {
		return 0, false
	}
	flags := argvFlags(fields)
	var candidates []int
	for i, r := range runs {
		if r.path == path || strings.HasPrefix(r.path, path+" ") {
			candidates = append(candidates, i)
		}
	}
	if isFamily := paths[path]; isFamily && len(flags) == 0 {
		switch {
		case len(candidates) == 1:
			// Unambiguous shorthand for the one run underneath it.
			return candidates[0], true
		case len(candidates) == 0:
			// 🔴 NOT the same situation as several, and lumping the two together
			// was a measured REGRESSION in the first cut of this fix (#387
			// audit). A family the row measures NOTHING under is an attribution
			// to a command nobody ran — the #361 shape — and crediting row 1's
			// `not found at project root` to `app listing` is the single most
			// plausible authoring error on that page. The pre-#373 guard caught
			// it; the exemption written for `app` silently un-attributed all
			// eleven families.
			return spanUnmeasured, true
		default:
			// Several: the span picks none of them, so it is not an attribution
			// to any measured invocation. This is the `app` in "your `app`
			// directory" — and equally "`app` prints it", which identifies no
			// invocation either.
			return 0, false
		}
	}
	distinguishing := map[string]bool{}
	for _, i := range candidates {
		for f := range runs[i].flags {
			for _, j := range candidates {
				if !runs[j].flags[f] {
					distinguishing[f] = true
				}
			}
		}
	}
	var matches []int
	for _, i := range candidates {
		match := true
		for f := range flags {
			if !runs[i].flags[f] {
				match = false
			}
		}
		for f := range distinguishing {
			if flags[f] != runs[i].flags[f] {
				match = false
			}
		}
		if match {
			matches = append(matches, i)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], true
	case 0:
		return spanUnmeasured, true
	default:
		return spanAmbiguous, true
	}
}

// commandMentionsIn parses a cause cell into the invocations it names and the
// polarity the sentence gives each.
func commandMentionsIn(cell string, paths map[string]bool, runs []ledgerRun) []commandMention {
	var out []commandMention
	for _, clause := range splitAttributionClauses(cell) {
		for _, sp := range attributionSpansIn(clause) {
			name := normalizeCommandSpan(sp.span)
			if name == "" {
				continue
			}
			idx, ok := resolveSpan(name, runs, paths)
			if !ok {
				continue
			}
			out = append(out, commandMention{span: name, run: idx, negated: sp.negated})
		}
	}
	return out
}

// attributionProseCheck is the F2 repair: the row's CAUSE cell — the sentence a
// reader actually reads to learn which command printed their error — must agree
// with the measured ledger, in both directions and with nothing left over.
//
//   - every emittedBy command is named in a POSITIVE clause and in no negative one
//   - every notEmittedBy command is named in a NEGATIVE clause and in no positive one
//   - every command the cell names at all is in the ledger, so the row cannot
//     carry an attribution nothing measured
//
// Matching is against the run's measured ARGV (resolveSpan), so `app submit`
// never stands in for `app submit --skip-validate` and the ledger's own argv is
// something the cell is allowed to write out in full.
//
// 🔴 Two exemptions make those three lines narrower than they read, and both
// are measured rather than inferred. (a) A bare FAMILY name that spans SEVERAL
// of the row's runs identifies no invocation, so it is not read at all —
// "`app` prints it" is unchecked, though `app` naming nothing the row measures
// is caught (resolveSpan). (b) A denial phrased with a verb outside
// printVerbStems reads as POSITIVE, so an EMITTER wrongly denied in such a
// sentence passes; see the vocabulary note above for the two measured cases.
// Neither is a licence to widen: they are where the guard stops, written down
// so nobody reads a green here as more than it is.
func attributionProseCheck(t *testing.T, a symptomAttribution, cell string, paths map[string]bool) {
	t.Helper()
	for _, p := range attributionProseProblems(a, cell, paths) {
		t.Error(p)
	}
}

// attributionProseProblems returns everything wrong with a cause cell, as
// messages.
//
// It is a function of the cell rather than a bundle of t.Errorf calls so that
// BOTH controls can be exercised directly: a cell that must be clean is asserted
// to produce ZERO problems, and a cell carrying a real mis-attribution is
// asserted to produce a specific NON-ZERO count. A checker wired to nothing
// returns zero for both, and only the pair tells them apart.
func attributionProseProblems(a symptomAttribution, cell string, paths map[string]bool) []string {
	runs := rowRuns(a, paths)
	mentions := commandMentionsIn(cell, paths, runs)
	// Positive control on the parser: a cell it reads as naming no invocation
	// would satisfy every "must not appear positively" check below for free.
	if len(mentions) == 0 {
		return []string{fmt.Sprintf("the cause cell for %q names no invocation this parser can see, so "+
			"nothing else here is a fact about the README.\ncell: %s", a.fragment, cell)}
	}
	pos := map[int]bool{}
	neg := map[int]bool{}
	for _, m := range mentions {
		if m.run < 0 {
			continue
		}
		if m.negated {
			neg[m.run] = true
		} else {
			pos[m.run] = true
		}
	}
	var probs []string
	for i, lr := range runs {
		argv := "civitai " + strings.Join(lr.argv, " ")
		if lr.emits {
			if !pos[i] {
				probs = append(probs, fmt.Sprintf("the row for %q is MEASURED to be printed by `%s`, but no "+
					"code span in its cause cell resolves to that invocation (measured argv: %s).\n"+
					"Why it matters: %s.\n"+
					"The cell is what a reader reads to find their command; a ledger that agrees with itself "+
					"and not with the prose is #361 with the labels moved into Go.\ncell: %s",
					a.fragment, lr.run.name, argv, a.why, cell))
			}
			if neg[i] {
				probs = append(probs, fmt.Sprintf("the row for %q says `%s` does NOT print it, but running that "+
					"command printed it (measured argv: %s).\nWhy it matters: %s.\ncell: %s",
					a.fragment, lr.run.name, argv, a.why, cell))
			}
			continue
		}
		if !neg[i] {
			probs = append(probs, fmt.Sprintf("the row for %q is measured NOT to be printed by `%s`, but its "+
				"cause cell does not say so: no span resolving to that invocation (measured argv: %s) sits "+
				"near a NEGATED printing verb — `never prints`, `does not emit`, `will not report`, and so "+
				"on, from %v negated by %v.\nWhy it matters: %s.\ncell: %s",
				a.fragment, lr.run.name, argv, printVerbStems, sortedKeys(attributionNegators), a.why, cell))
		}
		if pos[i] {
			probs = append(probs, fmt.Sprintf("the row for %q credits `%s` with printing it, and running that "+
				"command did not (measured argv: %s).\nWhy it matters: %s.\n"+
				"This is the #361 defect exactly: a row about a command that never emits the string.\ncell: %s",
				a.fragment, lr.run.name, argv, a.why, cell))
		}
	}
	for _, m := range mentions {
		switch m.run {
		case spanUnmeasured:
			// 🔴 "never runs" is the wrong diagnosis when the row DOES run that
			// command path and the span merely fails to say WHICH invocation —
			// a bare `app submit` against a row measuring two flagged submits
			// reported "which this ledger never runs" for a command run twice.
			// A guard that cries wolf is bad; one that cries wolf with a
			// misleading message is worse, so the message names the argvs it is
			// actually choosing between.
			if near := ledgerArgvsUnder(m.span, runs, paths); len(near) > 0 {
				probs = append(probs, fmt.Sprintf("the cause cell for %q names `%s`, which does not identify any "+
					"invocation this ledger measures. Under that command path it runs:\n  %s\n"+
					"Name the flag that picks one of them out, or add a measured run to "+
					"symptomAttributions().\ncell: %s",
					a.fragment, m.span, strings.Join(near, "\n  "), cell))
				continue
			}
			probs = append(probs, fmt.Sprintf("the cause cell for %q names `%s`, which this ledger never runs.\n"+
				"An unmeasured attribution in the index is exactly what #361 was. Either add a measured "+
				"run for `%s` to symptomAttributions(), or stop naming it here.\ncell: %s",
				a.fragment, m.span, m.span, cell))
		case spanAmbiguous:
			probs = append(probs, fmt.Sprintf("the cause cell for %q names `%s`, which matches more than one of "+
				"this row's measured invocations, so it attributes the message to none of them.\n"+
				"Name the flag that tells them apart.\ncell: %s", a.fragment, m.span, cell))
		}
	}
	return probs
}

// ledgerArgvsUnder returns the measured argvs whose command path the span names
// or prefixes — i.e. the invocations an unresolved span was choosing between.
// Empty when the row genuinely never runs that command, which is the case the
// "never runs" wording is for.
func ledgerArgvsUnder(span string, runs []ledgerRun, paths map[string]bool) []string {
	path, ok := commandPathIn(strings.Fields(span), paths)
	if !ok {
		return nil
	}
	var out []string
	for _, r := range runs {
		if r.path == path || strings.HasPrefix(r.path, path+" ") {
			out = append(out, "civitai "+strings.Join(r.argv, " "))
		}
	}
	sort.Strings(out)
	return out
}

// sortedKeys renders a set deterministically, so a failure message does not
// shuffle between runs.
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// readmeHasAnchor reports whether a GitHub-style `#anchor` resolves to a heading
// in md. Lines inside fenced code blocks are skipped: a shell comment or a
// Markdown sample beginning with `#` is not a heading, and counting one would
// let a dead link pass.
func readmeHasAnchor(md, anchor string) bool {
	want := strings.TrimPrefix(anchor, "#")
	inFence := false
	for _, line := range strings.Split(md, "\n") {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || !strings.HasPrefix(line, "#") {
			continue
		}
		text := strings.TrimSpace(strings.TrimLeft(line, "#"))
		if githubAnchorSlug(text) == want {
			return true
		}
	}
	return false
}

// githubAnchorSlug mirrors GitHub's heading-slug rules: lowercase, drop
// everything that is not alphanumeric/space/hyphen/UNDERSCORE, spaces to
// hyphens — which is why a 🔴 heading's anchor carries a leading hyphen.
//
// 🔴 The underscore is not cosmetic and it was wrong here. Dropping it turned
// "The host handshake (`BLOCK_READY`)" into `the-host-handshake-blockready`, so
// readmeHasAnchor reported `#the-host-handshake-block_ready` — a link that
// exists and works, and that several places on the page use — as a dead anchor.
// A guard that fails on correct input is one somebody deletes.
//
// How many places is COUNTED by the test below rather than written down here.
// #373: this comment said "five times" and the measured number is FOUR.
func githubAnchorSlug(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}

// TestAttributionProseParser validates the INSTRUMENT before its verdict above
// is read. A parser that saw no command, or that let `app submit` stand in for
// `app submit --skip-validate`, would make attributionProseCheck report a
// confident pass about a cell it never understood.
//
// The second case is the mutation the F2 audit demonstrated, verbatim: the #361
// defect re-typed into the prose while the link cell stayed correct.
func TestAttributionProseParser(t *testing.T) {
	paths := knownCommandPaths(t)
	// Resolution is per-ROW, so the parser is exercised against a real row's
	// measured runs. This one covers all four invocations the two rows share.
	row := symptomAttributions()[1]
	runs := rowRuns(row, paths)
	if len(runs) != 4 {
		t.Fatalf("the `is this an App project?` row measures %d invocations, want 4 — the fixtures below "+
			"name specific ones, so a changed ledger must move them too", len(runs))
	}
	resolved := func(i int) string {
		switch i {
		case spanUnmeasured:
			return "<not in this ledger>"
		case spanAmbiguous:
			return "<ambiguous>"
		default:
			return runs[i].run.name
		}
	}
	// got maps each span the parser saw to "<polarity><the run it resolved to>".
	got := func(cell string) map[string]string {
		m := map[string]string{}
		for _, x := range commandMentionsIn(cell, paths, runs) {
			pol := "+"
			if x.negated {
				pol = "-"
			}
			m[x.span] = pol + resolved(x.run)
		}
		return m
	}
	want := func(t *testing.T, m map[string]string, span, expect string) {
		t.Helper()
		if m[span] != expect {
			t.Errorf("span `%s` read as %q, want %q; whole parse: %v", span, m[span], expect, m)
		}
	}

	t.Run("positive and negative clauses are told apart", func(t *testing.T) {
		m := got("Printed by `civitai app listing …`. `app validate` never prints it — it reports the row above.")
		want(t, m, "app listing", "+app listing status")
		want(t, m, "app validate", "-app validate")
	})

	t.Run("a flagged invocation does not stand in for the bare command", func(t *testing.T) {
		m := got("Printed by `app submit --skip-validate`, which waived validation.")
		if _, ok := m["app submit"]; ok {
			t.Errorf("`app submit --skip-validate` must NOT register as a mention of `app submit` — "+
				"they print different messages, and conflating them is F1; got %v", m)
		}
		want(t, m, "app submit --skip-validate", "+app submit --skip-validate --package-only")
	})

	t.Run("the bare command resolves to the run without the distinguishing flag", func(t *testing.T) {
		// The other half of the same rule: `--package-only` and `--out` are on
		// BOTH submit runs, so a cell that names neither is still unambiguous.
		m := got("`app submit` prints it, because it validates first.")
		want(t, m, "app submit", "+app submit --package-only")
	})

	t.Run("the ledger's own measured argv is something the cell may write out", func(t *testing.T) {
		// #373 class 1: this exact span was reported as "a command this ledger
		// never runs", while being the argv the harness executes.
		m := got("Run `app listing status --dir <path>` from anywhere.")
		want(t, m, "app listing status --dir <path>", "+app listing status")
	})

	t.Run("a family name spanning several measured runs is English, not an attribution", func(t *testing.T) {
		// #373 class 1: `app` is a parent — `civitai app` prints help — and it
		// prefixes every run this row measures, so it identifies none of them.
		m := got("Run `app listing` from your `app` directory.")
		if _, ok := m["app"]; ok {
			t.Errorf("a bare `app` used as an English noun must not register as a command mention; got %v", m)
		}
		want(t, m, "app listing", "+app listing status")
	})

	t.Run("a family the row measures NOTHING under is an unmeasured attribution", func(t *testing.T) {
		// 🔴 The #387 audit's finding, and a measured REGRESSION in the first
		// cut of #373: the exemption written for "your `app` directory" also
		// swallowed families the row runs nothing under, which is the #361
		// shape. Crediting row 1's `not found at project root` to `app listing`
		// is the single most plausible authoring error on that page; it was
		// caught before #373, silently not caught after, and is caught again.
		row1 := rowRuns(symptomAttributions()[0], paths)
		if idx, ok := resolveSpan("app listing", row1, paths); !ok || idx != spanUnmeasured {
			t.Errorf("row 1 credits `app listing`: resolveSpan = (%d, %v), want (%d, true) — row 1 measures "+
				"nothing under `app listing`, so naming it is an attribution nobody ran", idx, ok, spanUnmeasured)
		}
		// And a family with no relationship to the App path at all.
		if idx, ok := resolveSpan("workflows", runs, paths); !ok || idx != spanUnmeasured {
			t.Errorf("row 2 credits `workflows`: resolveSpan = (%d, %v), want (%d, true)", idx, ok, spanUnmeasured)
		}
		// The exemption that remains: `app` spans ALL FOUR of row 2's runs, so
		// it identifies none of them and stays English.
		if _, ok := resolveSpan("app", runs, paths); ok {
			t.Error("a bare `app`, which prefixes every run this row measures, must still be exempt")
		}
	})

	t.Run("a family carrying a flag is an invocation claim, not a family reference", func(t *testing.T) {
		// The `len(flags) == 0` half of the family rule. `--slug` is real and
		// documented on this page, but no run in this ledger passes it, so the
		// span names an argv nobody measured rather than the `--dir` run.
		if idx, ok := resolveSpan("app listing --slug <blockId>", runs, paths); !ok || idx != spanUnmeasured {
			t.Errorf("resolveSpan(`app listing --slug <blockId>`) = (%d, %v), want (%d, true) — a flag the "+
				"ledger never passed cannot ride in on the family exemption", idx, ok, spanUnmeasured)
		}
	})

	t.Run("an unresolved span under a measured path says so, instead of `never runs`", func(t *testing.T) {
		// A cry-wolf with a MISLEADING message is worse than a cry-wolf. Two
		// runs sharing no flags: a bare `app submit` picks neither, and the old
		// wording called a command the row runs twice one it "never runs".
		synth := symptomAttribution{
			fragment: "synthetic row, for the message only",
			emittedBy: []attributionRun{
				{name: "app submit --package-only", args: func(e, _ string) []string {
					return []string{"app", "submit", e, "--package-only"}
				}},
				{name: "app submit --skip-validate", args: func(e, _ string) []string {
					return []string{"app", "submit", e, "--skip-validate"}
				}},
			},
		}
		joined := strings.Join(attributionProseProblems(synth, "Printed by `app submit`.", paths), "\n")
		if strings.Contains(joined, "never runs") {
			t.Errorf("`app submit` is run twice by this row; the message must not say the ledger never "+
				"runs it.\n%s", joined)
		}
		for _, want := range []string{"does not identify any invocation", "--package-only", "--skip-validate"} {
			if !strings.Contains(joined, want) {
				t.Errorf("the message must name %q so the author can see what to disambiguate.\n%s", want, joined)
			}
		}
	})

	t.Run("negation is a property of the verb, not of a listed phrase", func(t *testing.T) {
		// #373 class 2: `do not print` passed and `do not emit` did not.
		for _, cell := range []string{
			"`app validate` does not print it.",
			"`app validate` do not emit it.",
			"`app validate` will not report it.",
			"`app validate` never surfaces it.",
			"`app validate` cannot display it.",
			"`app validate` prints neither it nor the row above.",
			// Each of the next four pins one word or rule that an independently
			// built mutation sweep found unpinned (#387 audit finding 4).
			"`app validate` will print no such line.",     // the negator "no", after the verb
			"`app validate` exits without printing it.",   // the negator "without"
			"`app validate` has never shown it.",          // an irregular inflection
			"`app validate` never said it.",               // the other irregular
			"`app validate` **never** prints it.",         // emphasis marks must not hide the negator
			"`app validate` does not, in fact, print it.", // the negator three tokens ahead of the verb
		} {
			if m := got(cell); m["app validate"] != "-app validate" {
				t.Errorf("cell %q must read as a NEGATIVE attribution; got %v", cell, m)
			}
		}
	})

	t.Run("a negator further from the verb than the window does not reach it", func(t *testing.T) {
		// The other boundary of the same rule, and the reason it is a window at
		// all: "no" here negates the MANIFEST, not the printing. Without an
		// upper bound this reads as a denial and the row's real emitter goes
		// red — the false-positive class #373 exists to remove.
		m := got("`app listing` finds there is no manifest, so it prints this.")
		want(t, m, "app listing", "+app listing status")
	})

	t.Run("an em-dash starts a new clause, so a predicate does not reach across it", func(t *testing.T) {
		// splitAttributionClauses states this and #387 found it unfixtured —
		// the same "rationale with no case behind it" defect fixed for the
		// comma. Merged into one clause, `app listing` binds to the NEGATED
		// `prints` one token away instead of the `show` in its own sentence.
		m := got("`app validate` never prints — `app listing` does show it.")
		want(t, m, "app validate", "-app validate")
		want(t, m, "app listing", "+app listing status")
	})

	t.Run("a negator directly in front of a command negates it with no verb of its own", func(t *testing.T) {
		// English elides the second verb phrase. The contrast segment here has
		// no predicate at all, so without this rule it would default to POSITIVE
		// and credit a command the row measures as silent — a mutant that
		// survived until this case existed.
		m := got("`app listing status` prints it, but not `app validate`.")
		want(t, m, "app listing status", "+app listing status")
		want(t, m, "app validate", "-app validate")
	})

	t.Run("a later predicate speaks for the commands nearer to it", func(t *testing.T) {
		// One clause, two predicates of opposite polarity and no contrast word
		// to cut it. Binding every mention to the FIRST predicate — which is
		// what "the clause is positive or negative" amounts to — makes `app
		// validate` positive here, and that mutant survived the rest of this
		// table until this case was added.
		m := got("Reported by `app listing`, and `app validate` never prints it.")
		want(t, m, "app listing", "+app listing status")
		want(t, m, "app validate", "-app validate")
	})

	t.Run("on a tie the EARLIER predicate keeps the mention", func(t *testing.T) {
		// A rephrase of the shipped cell's own sentence, with a predicate two
		// tokens away on either side: `printed` (negated by the leading `Not`)
		// before, `reports` after. English reads this as a denial. Handing the
		// tie to the later predicate instead makes the guard report "credits
		// `app validate` with printing it" about correct prose — measured, 3
		// problems becomes 5 — so this is a claim, not an implementation detail.
		m := got("Not printed by `app validate`, which reports the row above.")
		want(t, m, "app validate", "-app validate")
	})

	t.Run("a contrast conjunction ends the negation's scope", func(t *testing.T) {
		// #373 class 3: one negation coloured the whole clause.
		m := got("`app validate` never prints it, but `app listing` does.")
		want(t, m, "app validate", "-app validate")
		want(t, m, "app listing", "+app listing status")
	})

	t.Run("a comma does not separate a shared subject list", func(t *testing.T) {
		// This is the rationale splitAttributionClauses states and #373 found
		// unpinned: adding ',' as a separator left the suite green. It cannot
		// now — comma-splitting puts `app validate` in a segment with no
		// predicate, which defaults to POSITIVE.
		m := got("`app validate`, a plain `app submit` and `app listing` never print it.")
		want(t, m, "app validate", "-app validate")
		want(t, m, "app submit", "-app submit --package-only")
		want(t, m, "app listing", "-app listing status")
	})

	t.Run("non-command code spans are ignored", func(t *testing.T) {
		m := got("The full line is `block.manifest.json not found at project root <dir>`; `--json` carries it as one `message` string.")
		if len(m) != 0 {
			t.Errorf("no command is named in that cell, but the parser found %v", m)
		}
	})

	t.Run("a negation about something other than printing is not a negation", func(t *testing.T) {
		m := got("Reported by `app listing`, which did not validate first.")
		want(t, m, "app listing", "+app listing status")
	})

	t.Run("a span matching several measured runs is reported, not guessed", func(t *testing.T) {
		// Reachable as soon as a row measures one command twice with the same
		// flags and a different operand — a plausible next ledger entry, and
		// nothing in today's ledger does it, so without this fixture the branch
		// is unreachable and deleting it is invisible.
		twice := symptomAttribution{
			fragment: "synthetic row, for the ambiguity branch only",
			emittedBy: []attributionRun{
				{name: "app validate <project>", args: func(empty, _ string) []string {
					return []string{"app", "validate", empty}
				}},
				{name: "app validate <scratch>", args: func(_, scratch string) []string {
					return []string{"app", "validate", scratch}
				}},
			},
		}
		runsTwice := rowRuns(twice, paths)
		if idx, ok := resolveSpan("app validate", runsTwice, paths); !ok || idx != spanAmbiguous {
			t.Errorf("resolveSpan(`app validate`) = (%d, %v), want (%d, true): a span matching two measured "+
				"runs attributes the message to neither of them", idx, ok, spanAmbiguous)
		}
		probs := attributionProseProblems(twice, "Printed by `app validate`.", paths)
		if len(probs) == 0 || !strings.Contains(strings.Join(probs, "\n"), "matches more than one") {
			t.Errorf("an ambiguous mention must be REPORTED rather than silently resolved; got %v", probs)
		}
	})

	t.Run("a command outside the ledger is still reported", func(t *testing.T) {
		// The leftover rule survives the move off equality: a leaf command this
		// row never measured is an unmeasured attribution, which is #361.
		m := got("`generate` prints it too.")
		want(t, m, "generate", "+<not in this ledger>")
	})
}

// TestAttributionProseCheckAcceptsCorrectProseAndRejectsMisattribution is the
// #373 regression guard, run as a PAIR.
//
// The defect it pins is a FALSE POSITIVE: three classes of correct prose that
// reddened the row. So the regression test is the correct prose — but a checker
// that returns "no problems" for everything would pass that half perfectly,
// which is why the same table carries mis-attributed cells and asserts a
// non-zero count with the right message. Neither half means anything alone.
//
// Every fixture is built by editing the cell the README really ships, and each
// edit asserts it matched something first: a fixture built by a replacement that
// silently did nothing is the shipped cell again, and would report a clean pass
// about a case it never constructed.
func TestAttributionProseCheckAcceptsCorrectProseAndRejectsMisattribution(t *testing.T) {
	paths := knownCommandPaths(t)
	row := symptomAttributions()[1]
	section := readmeTroubleshootingSection(t)
	cells, n := troubleshootingRowFor(section, row.fragment)
	if n != 1 || len(cells) < 3 {
		t.Fatalf("expected exactly one Troubleshooting row quoting %q with 3 cells, got n=%d cells=%v",
			row.fragment, n, cells)
	}
	shipped := cells[1]

	// edit applies old/new pairs to the shipped cell, asserting each one matched.
	edit := func(t *testing.T, oldNew ...string) string {
		t.Helper()
		out := shipped
		for i := 0; i+1 < len(oldNew); i += 2 {
			if !strings.Contains(out, oldNew[i]) {
				t.Fatalf("the shipped cause cell no longer contains %q, so this fixture would be the "+
					"shipped cell unchanged and would prove nothing.\ncell: %s", oldNew[i], out)
			}
			out = strings.Replace(out, oldNew[i], oldNew[i+1], 1)
		}
		return out
	}

	t.Run("correct prose", func(t *testing.T) {
		cases := []struct {
			name          string
			cell          func(t *testing.T) string
			whyItReddened string
		}{
			{"the cell the README ships", func(*testing.T) string { return shipped },
				"the baseline: if this is not clean, nothing below is about #373"},
			{"class 1a — the ledger's own measured argv", func(t *testing.T) string {
				return edit(t, "Run `app listing` from the app directory",
					"Run `app listing status --dir <path>` from anywhere")
			}, "`app listing status --dir <path>` is what the harness runs, and the guard called it unmeasured"},
			{"class 1b — a bare `app` as an English noun", func(t *testing.T) string {
				return edit(t, "from the app directory", "from your `app` directory")
			}, "`app` is one of 19 single-word command paths, and several are ordinary nouns"},
			{"class 2 — a negation outside the seven listed phrases", func(t *testing.T) string {
				return edit(t, "never print it", "do not emit it")
			}, "only `…print` phrasings counted, so `do not emit it` read as a POSITIVE attribution"},
			{"class 3 — two polarities in one comma-joined clause", func(t *testing.T) string {
				return edit(t, "never print it, because validation reports the row above first.",
					"never print it, but `app listing` does.")
			}, "one negation coloured the whole clause, so `app listing` came out negated"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				cell := tc.cell(t)
				probs := attributionProseProblems(row, cell, paths)
				if len(probs) != 0 {
					t.Errorf("this cause cell is CORRECT and the guard reported %d problem(s).\n"+
						"Why this case exists: %s.\n"+
						"A guard that fails on correct input is one somebody deletes — that is the stake, "+
						"not the friction.\ncell: %s\nproblems:\n%s",
						len(probs), tc.whyItReddened, cell, strings.Join(probs, "\n"))
				}
			})
		}
	})

	// The other control. Without it, "0 problems" above is indistinguishable
	// from a checker wired to nothing.
	t.Run("misattribution", func(t *testing.T) {
		cases := []struct {
			name, mustSay string
			cell          func(t *testing.T) string
		}{
			{"#361 re-typed: a non-emitter credited with printing it", "credits `app validate`",
				func(t *testing.T) string {
					return edit(t, "`app validate` and a plain `app submit` never print it",
						"`app validate` and a plain `app submit` print it")
				}},
			{"F1 re-typed: the bare command standing in for the flagged one", "`app submit --package-only`",
				func(t *testing.T) string {
					return edit(t, "and `app submit --skip-validate`, which waived", "and `app submit`, which waived")
				}},
			// A cell that contradicts itself: clause 1 still credits `app
			// listing`, so the "is MEASURED to be printed by" arm stays quiet
			// and only the emitter-must-not-be-denied arm can catch this one.
			{"an emitter the cell also denies", "does NOT print it, but running that command printed it",
				func(t *testing.T) string {
					return edit(t, "Run `app listing` from the app directory",
						"`app listing` never prints it. Run it from the app directory")
				}},
			// Not the same defect as crediting a non-emitter: here the cell says
			// nothing at all about the two commands a reader is most likely to
			// have just run. Every other row of this table also trips the
			// "credits X" arm, so without this one the "must be named in a
			// NEGATED clause" arm could be deleted and the suite stays green —
			// measured.
			{"a non-emitter the cell simply never mentions", "is measured NOT to be printed by",
				func(t *testing.T) string {
					return edit(t, "`app validate` and a plain `app submit` never print it, "+
						"because validation reports the row above first. ", "")
				}},
			// End-to-end cover for the #387 audit's finding: the family
			// exemption must not swallow a family the row runs nothing under.
			{"a family the row measures nothing under", "which this ledger never runs",
				func(t *testing.T) string {
					return edit(t, "Run `app listing`", "`workflows` prints it too. Run `app listing`")
				}},
			{"an attribution nothing measured", "which this ledger never runs",
				func(t *testing.T) string {
					return edit(t, "Run `app listing`", "`generate` prints it too. Run `app listing`")
				}},
			// Both mentions have to go: the guard reads the WHOLE cell, so the
			// remedy sentence at the end still credits `app listing` on its own.
			// Deleting only the first one is not a defect and does not redden —
			// measured while writing this table.
			{"an emitter the cell never names", "MEASURED to be printed by",
				func(t *testing.T) string {
					return edit(t,
						"`civitai app listing …`", "the listing commands",
						"Run `app listing` from the app directory", "Run it from the app directory")
				}},
			{"a cell that names no invocation at all", "names no invocation",
				func(*testing.T) string { return "The same cause, reported by a command that did not validate first." }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				cell := tc.cell(t)
				probs := attributionProseProblems(row, cell, paths)
				if len(probs) == 0 {
					t.Fatalf("POSITIVE CONTROL FAILED: this cause cell is WRONG and the guard reported "+
						"nothing. A zero from this checker is only meaningful once this number can "+
						"move.\ncell: %s", cell)
				}
				if !strings.Contains(strings.Join(probs, "\n"), tc.mustSay) {
					t.Errorf("the guard went red, but for the wrong reason — no problem mentions %q, so a "+
						"green elsewhere could be green for an unrelated reason.\nproblems:\n%s",
						tc.mustSay, strings.Join(probs, "\n"))
				}
				t.Logf("positive control: %d problem(s), first is:\n%s", len(probs), probs[0])
			})
		}
	})
}

// TestGithubAnchorSlugKeepsUnderscores is the F4 regression guard.
//
// Measured red before the fix: githubAnchorSlug dropped `_`, so the anchor for
// the `BLOCK_READY` heading came back `the-host-handshake-blockready` and
// readmeHasAnchor called a live link dead. Both of the ledger rows most likely
// to be added next (the two `BLOCK_READY` advisory rows) point at that heading.
//
// 🔴 The link COUNT is measured here, not asserted from memory. #373: the
// comment above githubAnchorSlug, the failure message below, and commit
// ae58a47's own F4 paragraph all said the anchor was "used five times"; there
// are FOUR links to it — the table of contents, one prose link in the
// dev-tunnel walkthrough, and the two Troubleshooting rows. The commit message
// cannot be rewritten, so this is the correction of record. Counting it means
// the number in the failure a maintainer reads is whatever is true that day.
func TestGithubAnchorSlugKeepsUnderscores(t *testing.T) {
	const anchor = "#the-host-handshake-block_ready"
	if got, want := githubAnchorSlug("The host handshake (`BLOCK_READY`)"), strings.TrimPrefix(anchor, "#"); got != want {
		t.Errorf("githubAnchorSlug = %q, want %q — GitHub preserves underscores", got, want)
	}
	md := readREADME(t)
	// Positive control on the count: an anchor nothing links to would make the
	// assertion below a fact about a string rather than about a live link.
	links := strings.Count(md, "]("+anchor+")")
	if links == 0 {
		t.Fatalf("no link in README.md targets %s, so whether readmeHasAnchor can resolve it says "+
			"nothing about any reader's click", anchor)
	}
	if !readmeHasAnchor(md, anchor) {
		t.Errorf("readmeHasAnchor cannot see `%s`, a heading README.md really has and links to %d times",
			anchor, links)
	}
	// Negative control: the searcher must be able to report a miss at all.
	if readmeHasAnchor(md, "#there-is-no-such-heading-in-this-readme") {
		t.Error("readmeHasAnchor found a heading that does not exist — it cannot report a miss")
	}
	// And a `#` line inside a fenced block is not a heading.
	const fenced = "## Real Heading\n\n```sh\n# not a heading\n```\n"
	if !readmeHasAnchor(fenced, "#real-heading") {
		t.Error("POSITIVE CONTROL FAILED: readmeHasAnchor cannot see a heading outside a fence, " +
			"so the negative below would be a fact about the walk")
	}
	if readmeHasAnchor(fenced, "#not-a-heading") {
		t.Error("readmeHasAnchor counted a shell comment inside a code fence as a heading — " +
			"README.md really contains such lines (`# …` inside ```sh blocks), so a dead anchor could pass")
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
		{"refusing to withdraw without --yes",
			"the #350 gate, and the only refusal in this index that BROKE a working script — a scripted " +
				"`app withdraw` used to exit 0, so an author who never reads --help meets it as a CI failure"},
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
