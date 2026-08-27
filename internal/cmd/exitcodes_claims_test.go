package cmd

// THE CONTRACT MUST SAY SOMETHING, NOT MERELY SAY IT TWICE.
//
// exitcodes_doc_test.go's guards are all AGREEMENT guards: the README table
// equals what exitCodeDocs renders, `--help` is rendered from the same slice,
// and the help text is a prefix of the README cell. Every one of them stays
// green if a Note is DELETED, because deleting it moves both surfaces together
// (edit exitCodeDocs, re-run the generator, paste the new table — the workflow
// the failure message itself prescribes). The two texts would agree perfectly
// about a contract that no longer exists.
//
// So this file is the other direction: a ledger of the DECISIONS the published
// contract has to carry, asserted against BOTH rendered surfaces. A row here is
// a claim someone measured and someone else must not silently drop.
//
// 🔴 A ROW IS NOT A SUBSTRING SEARCH FOR A NICE WORD. Each row names the CODE
// it belongs to and requires its phrases in that code's own text, so a phrase
// that drifts to a different row fails — otherwise "a resource that exists but
// is not ready is a 1" would be satisfied by the sentence appearing under
// code 4, which is the exact confusion it exists to prevent.

import (
	"strings"
	"testing"
)

// contractClaim is one published decision plus the reason it is published.
type contractClaim struct {
	// code is the exit code whose text must carry the claim.
	code int
	name string
	// phrases must ALL appear in that code's shared text. They are chosen to be
	// the load-bearing nouns of the decision, not incidental wording, so a
	// rewrite that keeps the meaning keeps the row green.
	phrases []string
	// why records what breaks if the claim goes. Derived from the decision, not
	// from the current sentence.
	why string
	// pinnedBy names the behavioural guard that proves the CODE honours the
	// claim, so a reader can tell a documented promise from a kept one.
	pinnedBy string
}

// exitCodeClaimsFloor is the KEEPER for exitCodeContractClaims: the row NAMES
// that must be in the ledger, whatever else it gains.
//
// 🔴 A COUNT WAS NOT ENOUGH, AND IT HAD FIVE ROWS OF SLACK. This started as
// `len(claims) < 5` against a ten-row ledger, which means five rows could be
// deleted with no addition at all and the guard stayed green. Add-one/delete-one
// defeats even a tight version of it: delete "a filesystem failure is 1, not 2
// and not 5" — the issue #241 row, the one that stops a script retrying a
// permissions error forever — add a row about some other code, count unchanged,
// green, and the README is free to drop the sentence.
//
// The per-row assertions in TestPublishedExitCodeClaims cannot cover this,
// because they iterate the rows that EXIST. A deleted row takes its own
// assertion with it, which is what makes row existence a separate question from
// row content, and the reason this file's header calls a row "a claim someone
// measured and someone else must not silently drop".
//
// Keyed on NAME, not on code: four rows share code 1 and four share code 2, so
// the code is not an identity. The names are the stable identities and are
// already the subtest names.
//
// 🔴 THREE RESIDUALS, STATED RATHER THAN GLOSSED. (1) Protection is OPT-IN PER
// ROW: the loop iterates THIS list, so a row added to the ledger and not named
// here is permanently tradeable, and nothing goes red at the moment of the
// omission. Append the name in the same commit as the row. (2) The two-line
// bypass is still open: delete the floor entry AND the row together. What this
// stops is the ONE-line trade; it does not stop a deliberate two-line removal,
// and no in-tree check can — that is what review is for. (3) It pins NAMES, not
// PHRASES: a row keeping its name while its `phrases` are watered down to
// something the contract trivially satisfies is not caught here. The phrases are
// asserted against both rendered surfaces, but nothing pins how load-bearing
// they are.
var exitCodeClaimsFloor = []string{
	"a filesystem failure is 1, not 2 and not 5",
	"a resource that EXISTS but is not READY is 1, not 4",
	`the not-ready 1 is not always "wait for approval"`,
	"a local image is refused at 2 for the enumerated reasons",
	"an unreadable-but-present file is NOT 2",
	"the missing-vs-unreadable split holds for a flag's value and a positional alike",
	"a project path that does not exist, or is not a directory, is 2",
	"a validation VERDICT is 1, and a manifest-less directory is a verdict",
	"`app validate --json` publishes a result only when it produced one",
	"5 is the retry code and a filesystem failure never lands there",
}

func exitCodeContractClaims() []contractClaim {
	return []contractClaim{
		{
			code:    1,
			name:    "a filesystem failure is 1, not 2 and not 5",
			phrases: []string{"filesystem failure", "cannot be read"},
			why: "exit 5 is the code the README tells scripts to RETRY on, and a permissions or I/O " +
				"problem never clears — a retry loop on it does not terminate (issue #241)",
			pinnedBy: "cmd/civitai/fs_not_network_test.go",
		},
		{
			code:    1,
			name:    "a resource that EXISTS but is not READY is 1, not 4",
			phrases: []string{"exists but is not ready", "app metrics", "still in review", "app status"},
			why: "4 promises the resource does not exist. For an app awaiting approval the slug is RIGHT " +
				"and the app IS there — only its analytics are not — so collapsing the two onto 4 destroys " +
				"the only actionable distinction: fix your slug versus wait for approval. " +
				"🔴 THIS ROW IS THE 1-vs-4 SPLIT ONLY, NOT THE WHOLE OF WHAT 1 PROMISES HERE: its phrases " +
				"say \"still in review\", which is the PENDING case, and code 1 also covers a rejected or " +
				"withdrawn latest submission, where there is no review to wait for. The row below carries " +
				"that half (issue #378)",
			pinnedBy: "TestAppMetricsNoApprovedBlockYet (negative) + TestAppMetricsUnknownSlugIsActionableNotFound (positive control)",
		},
		{
			code: 1,
			name: "the not-ready 1 is not always \"wait for approval\"",
			// Bare nouns, no markdown emphasis: `**rejected**` would redden on a
			// meaning-preserving reword to `*rejected*`, and this file's own header
			// says a row is the load-bearing NOUNS, not incidental wording. The
			// bare word is a substring of the emphasised one either way.
			phrases: []string{"rejected", "withdrawn", "nothing is in review", "civitai app submit"},
			why: "the row above frames the whole state as \"still in review\", which was measured on the only " +
				"state anyone exercised (pending) and is false for the two the CLI can also be handed. " +
				"`app metrics` printed \"check where it is in review\" for a REJECTED app — a review that is " +
				"not happening — until pullReviewAdvice became shared with `app pull`, so what exit 1 " +
				"promises for this state is a next step chosen FROM the latest submission's own state, not a " +
				"blanket instruction to wait. Ledgered separately because the sentence above is verbatim " +
				"pre-split text that TestEveryPreSplitClauseSurvives forbids rewriting in place",
			pinnedBy: "TestAppMetricsTerminalSubmissionIsNotDescribedAsInReview + " +
				"TestAppMetricsAdviceNamesTheNewestSubmission (which row) + " +
				"TestPullAndMetricsGiveTheSameNextStepForTheSameState (the seam)",
		},
		{
			code:    2,
			name:    "a local image is refused at 2 for the enumerated reasons",
			phrases: []string{"set-icon", "generate --image", joinPhrases(imageUsageRefusals)},
			why: "the ledger in imageUsageRefusals is asserted against loadAndValidateImage in both " +
				"directions, so the published sentence is a claim the code has to keep",
			pinnedBy: "TestImageUsageRefusalLedger",
		},
		{
			code:    2,
			name:    "an unreadable-but-present file is NOT 2",
			phrases: []string{"cannot be **read**", "exits `1`, not `2`"},
			why: "this is the sentence `generate --input` contradicted for a whole release — every " +
				"os.ReadFile failure was asUsageError-tagged, so a mode-000 graph file exited 2 while " +
				"a mode-000 image exited 1, out of one binary",
			pinnedBy: "TestUnreadableImageIsNotAUsageError + TestReadGraphInputClassification",
		},
		{
			code:    2,
			name:    "the missing-vs-unreadable split holds for a flag's value and a positional alike",
			phrases: []string{"a flag's value and a positional argument alike", "generate --input"},
			why: "the shape of the rule, not its extent: a rule written only about images is one a future " +
				"path flag can be added beside without anyone noticing it disagrees, and --input was that " +
				"counterexample. It said \"every local path a FLAG names\" for a release, and the two " +
				"commands that broke it — `app validate <dir>` / `app submit <dir>`, issue #256 — take the " +
				"path POSITIONALLY, so the sentence excluded exactly the cases that disagreed with it. " +
				"🔴 The replacement then over-corrected to \"every local path the CLI is HANDED\", which is " +
				"ALSO false and has a live counterexample INSIDE its own scope: `app listing --dir <missing>` " +
				"exits 1, measured identical on base and on this branch. So the sentence now publishes the " +
				"shape over an enumerated ledger and states the residual, rather than quantifying over " +
				"paths nobody has audited",
			pinnedBy: "TestGenerateInputExitCodes (cmd/civitai) + TestReadGraphInputClassification + " +
				"TestProjectDirExitCodes + TestUngatedPathFlagsAreNotUsageErrors (the residual)",
		},
		{
			code:    2,
			name:    "a project path that does not exist, or is not a directory, is 2",
			phrases: []string{"app validate <dir>", "app submit <dir>", "or is not a directory"},
			why: "issue #256: `app validate /nope` reported the missing path as \"a project root without a " +
				"manifest\" and exited 1, so a script could not tell a typo'd path from an app that " +
				"genuinely fails validation — the one distinction the exit-code contract exists to draw",
			pinnedBy: "TestProjectDirExitCodes + TestResolveProjectDirClassification",
		},
		{
			code: 1,
			name: "a validation VERDICT is 1, and a manifest-less directory is a verdict",
			phrases: []string{"validation verdict", "app validate", "no `block.manifest.json` at its root",
				"app validate --json"},
			why: "the counterweight to the #256 fix: it would be easy to \"tidy\" the manifest-less " +
				"directory onto 2 alongside the nonexistent path. It must stay 1 — the user pointed at a " +
				"real place, so the invocation was right and the project is wrong, and that is the " +
				"answer `--json`'s `ok` field reports",
			pinnedBy: "TestProjectDirExitCodes (the control rows)",
		},
		{
			code: 1,
			name: "`app validate --json` publishes a result only when it produced one",
			phrases: []string{"When validation produces a result", "nothing on stdout",
				"branch on the exit code before parsing"},
			why: "the note said `--json` \"prints the full result … so a script never has to read stderr\", " +
				"full stop — and that is false for the failures that produce no Result at all. Measured on " +
				"this branch: `app validate <mode-000 project> --json` exits 1 with stdout EMPTY, because " +
				"validate.Dir returns an error rather than a Result for a non-ENOENT stat failure and for a " +
				"schema() failure. An unqualified promise here is worse than silence: it tells a script " +
				"author they may parse stdout unconditionally, on the one command whose whole job is to be " +
				"machine-read. 🔴 The first scoping was ALSO wrong, in the other direction: it read \"for a " +
				"project directory it could READ\", which still promises an object for the schema() arm — a " +
				"directory the CLI can read perfectly well that yields no Result. The condition is whether " +
				"validation PRODUCED a result, which is the thing the code actually branches on, so that is " +
				"what the sentence now says",
			pinnedBy: "TestValidateJSONOnlyEmitsAResultItActuallyProduced (+ its readable-dir positive control)",
		},
		{
			code:     5,
			name:     "5 is the retry code and a filesystem failure never lands there",
			phrases:  []string{"retry", "filesystem"},
			why:      "the whole point of issue #241: a retryable-looking errno is not a retryable failure",
			pinnedBy: "cmd/civitai/fs_not_network_test.go + pkg/civitai/retry_fs_test.go",
		},
	}
}

// readmeExitCodePublished is everything README publishes about exit codes: the
// index table plus the per-code `### Exit code N` subsections. After the
// summary/detail split a claim reaches the README through one or the other, so
// asserting against the table alone would have quietly stopped observing every
// clause the split moved — the failure this whole file exists to catch, arrived
// at from the guard's own side.
func readmeExitCodePublished() string {
	return readmeExitCodeTable() + "\n" + readmeExitCodeSections()
}

// TestPublishedExitCodeClaims requires every ledgered decision to appear in the
// PUBLISHED text of its own code (Summary plus Detail) and to reach the README,
// which publishes both halves.
//
// 🔴 IT NO LONGER REQUIRES EVERY CLAIM TO REACH `--help`, AND THAT IS THE
// SUMMARY/DETAIL TRADE, NOT AN EROSION OF THE LEDGER. The terminal now prints
// one summary line per code plus a pointer at README (see the file header in
// exitcodes_doc.go); the ledger's rows are the DETAIL, and most of them live
// where README publishes them. What replaces the lost `--help` assertion is
// TestEveryCodesSummaryReachesTheTerminal below plus
// TestHelpPointsAtTheREADMESection — i.e. the terminal is still asserted to
// carry every code and to say where the rest is, which is what it now promises.
func TestPublishedExitCodeClaims(t *testing.T) {
	claims := exitCodeContractClaims()

	have := map[string]bool{}
	for _, c := range claims {
		if have[c.name] {
			// Two rows sharing a name would let one be deleted while its twin
			// satisfies the floor below, which is the trade this floor exists
			// to stop.
			t.Fatalf("two ledger rows are both named %q — the floor keys on the name, so a duplicate "+
				"lets one of them be deleted silently", c.name)
		}
		have[c.name] = true
	}
	// POSITIVE CONTROL: an empty floor would make every check below vacuous.
	if len(exitCodeClaimsFloor) == 0 {
		t.Fatal("CONTROL failure: exitCodeClaimsFloor is empty, so this test asserts nothing")
	}
	for _, name := range exitCodeClaimsFloor {
		if !have[name] {
			t.Errorf("the ledger row %q is gone.\n"+
				"A row is a decision someone MEASURED and someone else must not silently drop; the "+
				"per-row assertions below only check rows that still exist, so a deleted row takes its "+
				"own coverage with it. The old guard was a COUNT (`len(claims) < 5`) against ten rows, "+
				"so five could be deleted outright — and deleting this one while adding any unrelated "+
				"row kept the count and stayed green.\n"+
				"Adding rows is free; this list only forbids REMOVING one.", name)
		}
	}

	byCode := make(map[int]ExitCodeDoc, len(exitCodeDocs))
	for _, d := range exitCodeDocs {
		byCode[d.Code] = d
	}

	readme := flattenWS(readmeExitCodePublished())

	for _, c := range claims {
		t.Run(c.name, func(t *testing.T) {
			doc, ok := byCode[c.code]
			if !ok {
				t.Fatalf("the ledger claims something about exit code %d, which exitCodeDocs does not document", c.code)
			}
			published := flattenWS(doc.published())

			for _, p := range c.phrases {
				if strings.TrimSpace(p) == "" {
					t.Fatalf("empty phrase in the ledger — strings.Contains(x, \"\") is always true, which disarms the row")
				}
				// The claim must live under ITS OWN code, not merely somewhere
				// in the contract.
				if !strings.Contains(published, flattenWS(p)) {
					t.Errorf("exit code %d no longer publishes %q.\nWhy it is published: %s\nHonoured by: %s\n\ncode %d says: %s",
						c.code, p, c.why, c.pinnedBy, c.code, doc.published())
					continue
				}
				// And it must reach the rendered README, which is the surface
				// that publishes the ledger in full.
				if !strings.Contains(readme, flattenWS(p)) {
					t.Errorf("exit code %d's claim %q does not reach the rendered README (table + `### Exit code %d`)", c.code, p, c.code)
				}
			}
		})
	}
}

// TestEveryCodesSummaryReachesTheTerminal is what the `--help` half of
// TestPublishedExitCodeClaims became. The terminal's promise shrank from "the
// whole ledger" to "every code, named" — so that is what is asserted, per code,
// against the RENDERED help rather than the generator's return value.
func TestEveryCodesSummaryReachesTheTerminal(t *testing.T) {
	help := flattenWS(renderRootHelp(t))
	if help == "" {
		t.Fatal("the rendered help is empty — every assertion below would be vacuous")
	}
	for _, d := range exitCodeDocs {
		if !strings.Contains(help, flattenWS(plainify(d.Summary))) {
			t.Errorf("exit code %d's summary %q does not reach the rendered `civitai --help`", d.Code, d.Summary)
		}
	}
}

// TestExitCodeClaimsLedgerIsNotVacuous is the ledger's own positive control: a
// phrase that is NOT in the contract must fail the same comparison the rows
// above use. Without it, a green run could mean the comparison never rejects
// anything (a flattenWS/plainify bug that normalised everything to "", say).
func TestExitCodeClaimsLedgerIsNotVacuous(t *testing.T) {
	help := flattenWS(rootExitCodeHelp())
	readme := flattenWS(readmeExitCodePublished())
	const absent = "ZZ-NOT-IN-THE-EXIT-CODE-CONTRACT-ZZ"

	for name, surface := range map[string]string{"--help": help, "README": readme} {
		if strings.Contains(surface, flattenWS(plainify(absent))) {
			t.Errorf("%s appears to contain %q — the comparison used by TestPublishedExitCodeClaims cannot reject anything", name, absent)
		}
		if surface == "" {
			t.Errorf("%s rendered to the empty string — every Contains assertion against it is vacuous", name)
		}
	}

	// The README surface must really carry BOTH halves, or "a claim reaches the
	// README" is satisfied by a renderer wired to the table alone.
	if !strings.Contains(readme, flattenWS("### Exit code 2")) {
		t.Error("the README surface used by the ledger carries no per-code subsection — it is wired to the table only")
	}

	// And the per-code scoping really scopes: code 6's text must NOT carry
	// code 1's headline claim, or "the claim is under its own code" is not
	// something this file can observe.
	var six ExitCodeDoc
	for _, d := range exitCodeDocs {
		if d.Code == 6 {
			six = d
		}
	}
	if strings.Contains(flattenWS(six.published()), "filesystem failure") {
		t.Error("code 6's text carries code 1's claim — the per-code scoping in the ledger is not discriminating")
	}
}
