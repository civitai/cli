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
				"the only actionable distinction: fix your slug versus wait for approval",
			pinnedBy: "TestAppMetricsNoApprovedBlockYet (negative) + TestAppMetricsUnknownSlugIsActionableNotFound (positive control)",
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
			phrases: []string{"For a project directory it could", "nothing on stdout",
				"branch on the exit code before parsing"},
			why: "the note said `--json` \"prints the full result … so a script never has to read stderr\", " +
				"full stop — and that is false for the failures that produce no Result at all. Measured on " +
				"this branch: `app validate <mode-000 project> --json` exits 1 with stdout EMPTY, because " +
				"validate.Dir returns an error rather than a Result for a non-ENOENT stat failure and for a " +
				"schema() failure. An unqualified promise here is worse than silence: it tells a script " +
				"author they may parse stdout unconditionally, on the one command whose whole job is to be " +
				"machine-read",
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

// TestPublishedExitCodeClaims requires every ledgered decision to appear in the
// SHARED text of its own code — which means it reaches `--help` and the README
// table alike, since both are rendered from exactly that string.
func TestPublishedExitCodeClaims(t *testing.T) {
	claims := exitCodeContractClaims()
	if len(claims) < 5 {
		t.Fatalf("only %d claims in the ledger — a shrunken ledger is a guard wired to less than it says", len(claims))
	}

	byCode := make(map[int]ExitCodeDoc, len(exitCodeDocs))
	for _, d := range exitCodeDocs {
		byCode[d.Code] = d
	}

	// Rendered once; every row is checked against both surfaces.
	help := flattenWS(rootExitCodeHelp())
	readme := flattenWS(readmeExitCodeTable())

	for _, c := range claims {
		t.Run(c.name, func(t *testing.T) {
			doc, ok := byCode[c.code]
			if !ok {
				t.Fatalf("the ledger claims something about exit code %d, which exitCodeDocs does not document", c.code)
			}
			shared := flattenWS(doc.shared())

			for _, p := range c.phrases {
				if strings.TrimSpace(p) == "" {
					t.Fatalf("empty phrase in the ledger — strings.Contains(x, \"\") is always true, which disarms the row")
				}
				// The claim must live under ITS OWN code, not merely somewhere
				// in the contract.
				if !strings.Contains(shared, flattenWS(p)) {
					t.Errorf("exit code %d no longer publishes %q.\nWhy it is published: %s\nHonoured by: %s\n\ncode %d says: %s",
						c.code, p, c.why, c.pinnedBy, c.code, doc.shared())
					continue
				}
				// And it must reach both rendered surfaces. `--help` is
				// de-emphasised, so compare the plainified form there.
				if !strings.Contains(help, flattenWS(plainify(p))) {
					t.Errorf("exit code %d's claim %q does not reach `--help`", c.code, p)
				}
				if !strings.Contains(readme, flattenWS(p)) {
					t.Errorf("exit code %d's claim %q does not reach the README table", c.code, p)
				}
			}
		})
	}
}

// TestExitCodeClaimsLedgerIsNotVacuous is the ledger's own positive control: a
// phrase that is NOT in the contract must fail the same comparison the rows
// above use. Without it, a green run could mean the comparison never rejects
// anything (a flattenWS/plainify bug that normalised everything to "", say).
func TestExitCodeClaimsLedgerIsNotVacuous(t *testing.T) {
	help := flattenWS(rootExitCodeHelp())
	readme := flattenWS(readmeExitCodeTable())
	const absent = "ZZ-NOT-IN-THE-EXIT-CODE-CONTRACT-ZZ"

	for name, surface := range map[string]string{"--help": help, "README": readme} {
		if strings.Contains(surface, flattenWS(plainify(absent))) {
			t.Errorf("%s appears to contain %q — the comparison used by TestPublishedExitCodeClaims cannot reject anything", name, absent)
		}
		if surface == "" {
			t.Errorf("%s rendered to the empty string — every Contains assertion against it is vacuous", name)
		}
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
	if strings.Contains(flattenWS(six.shared()), "filesystem failure") {
		t.Error("code 6's text carries code 1's claim — the per-code scoping in the ledger is not discriminating")
	}
}
