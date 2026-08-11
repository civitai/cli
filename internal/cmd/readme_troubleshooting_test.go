package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

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
type attributionRun struct {
	// readmeName is the EXACT inline-code text the row's cause cell must use to
	// name this invocation, after normalisation (a leading `civitai ` and a
	// trailing `…` are stripped). It is matched by EQUALITY, never substring:
	// `app submit` and `app submit --skip-validate` are DIFFERENT claims — the
	// first prints `not found at project root` and the second prints `is this an
	// App project?` — and a substring match would let either satisfy the other's
	// row, which is the shadowing that hid #361's sibling defect (F1).
	readmeName string
	name       string
	args       func(emptyProject, scratch string) []string
	env        map[string]string
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
			readmeName: "app validate",
			name:       "app validate",
			args:       func(empty, _ string) []string { return []string{"app", "validate", empty} },
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
			readmeName: name,
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
			// the `app listing` flow is walked through, and now carries the
			// working-directory resolution and the --slug/--dir remedy this row
			// tells the reader to use.
			anchor: "#after-you-submit-review--approve--deploy",
			emittedBy: []attributionRun{
				{
					// The slug resolve fails on the manifest before any request
					// is built, so the token only has to be non-empty to get
					// past the credential gate.
					readmeName: "app listing",
					name:       "app listing status",
					args: func(empty, _ string) []string {
						return []string{"app", "listing", "status", "--dir", empty}
					},
					env: map[string]string{"CIVITAI_TOKEN": "not-a-real-token"},
				},
				appSubmit(true),
			},
			notEmittedBy: []attributionRun{
				appValidate(),
				{
					// `app submit` DOES call manifest.Load — but only after
					// validation has passed, so a missing manifest never reaches
					// it UNLESS --skip-validate waives the validation. Listing
					// the caller is not the same as measuring the message, and
					// this row is the difference: the first draft of this fix
					// credited `app submit`, `app dev-token` and `app dev-tunnel`
					// from a grep of manifest.Load callers. dev-token and
					// dev-tunnel discard the load error and still print nothing;
					// `app submit` prints it on exactly one argv, which is why it
					// now appears on BOTH rows under two different readmeNames.
					readmeName: "app submit",
					name:       appSubmit(false).name,
					args:       appSubmit(false).args,
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

// inlineCodeSpanRe captures the text inside a markdown inline-code span.
var inlineCodeSpanRe = regexp.MustCompile("`([^`]+)`")

// attributionNegations is the CLOSED set of phrases that make a clause a
// NEGATIVE attribution ("X never prints it"). It is deliberately about PRINTING
// and nothing else: a clause may legitimately say a command "did not validate
// first" or "does not exist", and neither is a claim about the message.
var attributionNegations = []string{
	"never print",
	"does not print",
	"do not print",
	"did not print",
	"will not print",
	"cannot print",
	"prints neither",
}

// commandMention is one command a cause cell names, and whether the clause
// naming it was a negation.
type commandMention struct {
	name    string
	negated bool
}

// splitAttributionClauses cuts a cause cell into clauses at sentence ends,
// semicolons and spaced em-dashes — never inside an inline-code span, so a
// documented message containing ". " cannot be torn in half. A COMMA is
// deliberately not a separator: "`app validate` and a plain `app submit` never
// print it, because …" is ONE claim about two commands.
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

// normalizeCommandSpan renders an inline-code span the way a readmeName is
// written: no `civitai ` prefix, no trailing ellipsis, single-spaced.
func normalizeCommandSpan(span string) string {
	s := strings.Join(strings.Fields(span), " ")
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(s, "…"), "..."))
	return strings.TrimSpace(strings.TrimPrefix(s, "civitai "))
}

// knownCommandPaths is every command path in the REAL Cobra tree, minus the root
// name — "app", "app validate", "app listing status", "generate", … Derived from
// the tree rather than a hand-list so a renamed command cannot leave this
// recogniser quietly matching nothing.
func knownCommandPaths(t *testing.T) map[string]bool {
	t.Helper()
	root := NewRootCmd()
	prefix := root.Name() + " "
	out := map[string]bool{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			out[strings.TrimPrefix(sub.CommandPath(), prefix)] = true
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
	return out
}

// namesAKnownCommand reports whether a normalised span STARTS with a real
// command path, so `app submit --skip-validate` counts as naming a command while
// `--json` and `block.manifest.json not found …` do not.
func namesAKnownCommand(name string, paths map[string]bool) bool {
	words := strings.Fields(name)
	for n := len(words); n > 0; n-- {
		if paths[strings.Join(words[:n], " ")] {
			return true
		}
	}
	return false
}

// commandMentionsIn parses a cause cell into the commands it names and the
// polarity of the clause naming each.
func commandMentionsIn(cell string, paths map[string]bool) []commandMention {
	var out []commandMention
	for _, clause := range splitAttributionClauses(cell) {
		lower := strings.ToLower(clause)
		negated := false
		for _, p := range attributionNegations {
			if strings.Contains(lower, p) {
				negated = true
				break
			}
		}
		for _, m := range inlineCodeSpanRe.FindAllStringSubmatch(clause, -1) {
			name := normalizeCommandSpan(m[1])
			if name == "" || !namesAKnownCommand(name, paths) {
				continue
			}
			out = append(out, commandMention{name: name, negated: negated})
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
// Matching is by EQUALITY on the normalised span, so `app submit` never stands
// in for `app submit --skip-validate`.
func attributionProseCheck(t *testing.T, a symptomAttribution, cell string, paths map[string]bool) {
	t.Helper()
	mentions := commandMentionsIn(cell, paths)
	// Positive control on the parser: a cell it reads as naming no command would
	// satisfy every "must not appear positively" clause below for free.
	if len(mentions) == 0 {
		t.Fatalf("the cause cell for %q names no command this parser can see, so nothing below is a "+
			"fact about the README.\ncell: %s", a.fragment, cell)
	}
	pos := map[string]bool{}
	neg := map[string]bool{}
	for _, m := range mentions {
		if m.negated {
			neg[m.name] = true
		} else {
			pos[m.name] = true
		}
	}
	for _, r := range a.emittedBy {
		if !pos[r.readmeName] {
			t.Errorf("the row for %q is MEASURED to be printed by `%s`, but its cause cell never says so.\n"+
				"Why it matters: %s.\n"+
				"The cell is what a reader reads to find their command; a ledger that agrees with itself "+
				"and not with the prose is #361 with the labels moved into Go.\ncell: %s",
				a.fragment, r.readmeName, a.why, cell)
		}
		if neg[r.readmeName] {
			t.Errorf("the row for %q says `%s` does NOT print it, but running that command printed it.\n"+
				"Why it matters: %s.\ncell: %s", a.fragment, r.readmeName, a.why, cell)
		}
	}
	for _, r := range a.notEmittedBy {
		if !neg[r.readmeName] {
			t.Errorf("the row for %q is measured NOT to be printed by `%s`, but its cause cell does not "+
				"say so in a negated clause (it must, in one of %v).\n"+
				"Why it matters: %s.\ncell: %s",
				a.fragment, r.readmeName, attributionNegations, a.why, cell)
		}
		if pos[r.readmeName] {
			t.Errorf("the row for %q credits `%s` with printing it, and running that command did not.\n"+
				"Why it matters: %s.\n"+
				"This is the #361 defect exactly: a row about a command that never emits the string.\ncell: %s",
				a.fragment, r.readmeName, a.why, cell)
		}
	}
	ledgered := map[string]bool{}
	for _, r := range a.emittedBy {
		ledgered[r.readmeName] = true
	}
	for _, r := range a.notEmittedBy {
		ledgered[r.readmeName] = true
	}
	for _, m := range mentions {
		if !ledgered[m.name] {
			t.Errorf("the cause cell for %q names `%s`, which this ledger never runs.\n"+
				"An unmeasured attribution in the index is exactly what #361 was. Either add a measured "+
				"run for `%s` to symptomAttributions(), or stop naming it here.\ncell: %s",
				a.fragment, m.name, m.name, cell)
		}
	}
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
// exists and works, and is used five times on the page — as a dead anchor. A
// guard that fails on correct input is one somebody deletes.
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
	got := func(cell string) map[string]bool {
		m := map[string]bool{}
		for _, x := range commandMentionsIn(cell, paths) {
			m[x.name] = x.negated
		}
		return m
	}

	t.Run("positive and negative clauses are told apart", func(t *testing.T) {
		m := got("Printed by `civitai app listing …`. `app validate` never prints it — it reports the row above.")
		if neg, ok := m["app listing"]; !ok || neg {
			t.Errorf("`app listing` must be read as a POSITIVE mention; got %v", m)
		}
		if neg, ok := m["app validate"]; !ok || !neg {
			t.Errorf("`app validate` must be read as a NEGATIVE mention; got %v", m)
		}
	})

	t.Run("a flagged invocation does not stand in for the bare command", func(t *testing.T) {
		m := got("Printed by `app submit --skip-validate`, which waived validation.")
		if _, ok := m["app submit"]; ok {
			t.Errorf("`app submit --skip-validate` must NOT register as a mention of `app submit` — "+
				"they print different messages, and conflating them is F1; got %v", m)
		}
		if neg, ok := m["app submit --skip-validate"]; !ok || neg {
			t.Errorf("the flagged form must register positively under its own name; got %v", m)
		}
	})

	t.Run("non-command code spans are ignored", func(t *testing.T) {
		m := got("The full line is `block.manifest.json not found at project root <dir>`; `--json` carries it as one `message` string.")
		if len(m) != 0 {
			t.Errorf("no command is named in that cell, but the parser found %v", m)
		}
	})

	t.Run("a negation about something other than printing is not a negation", func(t *testing.T) {
		m := got("Reported by `app listing`, which did not validate first.")
		if neg, ok := m["app listing"]; !ok || neg {
			t.Errorf("`did not validate` is not a claim about printing; got %v", m)
		}
	})
}

// TestGithubAnchorSlugKeepsUnderscores is the F4 regression guard.
//
// Measured red before the fix: githubAnchorSlug dropped `_`, so the anchor for
// the `BLOCK_READY` heading came back `the-host-handshake-blockready` and
// readmeHasAnchor called a live, five-times-used link dead. Both of the ledger
// rows most likely to be added next (the two `BLOCK_READY` advisory rows) point
// at that heading.
func TestGithubAnchorSlugKeepsUnderscores(t *testing.T) {
	if got, want := githubAnchorSlug("The host handshake (`BLOCK_READY`)"), "the-host-handshake-block_ready"; got != want {
		t.Errorf("githubAnchorSlug = %q, want %q — GitHub preserves underscores", got, want)
	}
	md := readREADME(t)
	if !readmeHasAnchor(md, "#the-host-handshake-block_ready") {
		t.Error("readmeHasAnchor cannot see `#the-host-handshake-block_ready`, a heading README.md really has " +
			"and links to five times")
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
