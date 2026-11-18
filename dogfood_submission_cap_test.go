package cli_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// ACCOUNTING of the dogfood submission cap (scripts/dogfood/runner.py).
//
// 🔴 THE MEASUREMENT. Trial `ab-ship-mimo-01` (2026-09-25), evidence at
// ~/.cache/dogfood-runs-2026-09-25/ab-ship-mimo-01/. The cap charged ATTEMPTS
// rather than submissions, so a submit the CLI refused BEFORE contacting anything
// spent the whole budget:
//
//	step 49  cd ab-generate-and-post && civitai app submit 2>&1
//	         → verdict=run, exit 1, "Error: refusing to submit without --yes in a
//	           non-interactive shell (…)". NOTHING was contacted. `sub` went 0→1.
//	step 50  cd ab-generate-and-post && civitai app submit --yes 2>&1
//	         → REFUSED by the harness: "the submission cap for this trial is 1 and
//	           it has been reached". This is the retry that would have worked.
//	step 51  cd ab-generate-and-post && civitai app submit --package-only 2>&1
//	         → REFUSED, and --package-only writes a .zip and contacts nothing.
//	step 54  the --yes retry again → REFUSED.
//
// The account afterwards held 0 pending requests, 0 `ab-*` listings and unchanged
// Buzz: nothing was ever submitted. The model recovered correctly — it reached for
// `--yes` on its very next step — and the harness blocked it. That cell's
// `SHIP=no` was an artifact of this accounting, not a fact about the model.
//
// 🔴 AND THE DIRECTION OF THE FIX, WHICH MATTERS MORE THAN THE MECHANISM. The
// obvious repair decides up front: charge only a submit carrying `--yes`. That
// FAILS OPEN — it makes the harness depend on the CLI continuing to refuse a
// non-interactive submit without `--yes`, so the day that stops (auto-confirm
// config, a TTY, a renamed flag) the harness silently under-counts and permits
// unlimited REAL submissions against a live account. So the charge stays
// unconditional and is REFUNDED on proof that nothing was contacted: if the CLI
// changes, the refund stops firing and the harness over-refuses, which is the safe
// direction. The two facts that proof rests on live in Go, and the seam guards
// that pin them are in dogfood_submission_cap_seam_test.go.
//
// Everything here runs offline through testdata/fake_trial.py: no Docker, no
// OpenRouter, no money, no account.

// The exact bytes step 49 came back with, stripped of the `exit code:` line the
// runner adds. Used as the planted output so the trial replays the measurement
// rather than a paraphrase of it.
const measuredPreflightRefusal = "Error: refusing to submit without --yes in a non-interactive shell " +
	"(submitting creates a real moderator-review request; pass --yes to confirm, or " +
	"--package-only to just write the .zip)"

// results is the FAKE_TOOL_RESULTS_JSON env entry for a set of planted results.
type plantedResult struct {
	RC  int    `json:"rc,omitempty"`
	Out string `json:"out,omitempty"`
	Err string `json:"err,omitempty"`
}

func plantedResults(t *testing.T, m map[string]plantedResult) string {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("encoding the planted results: %v", err)
	}
	return "FAKE_TOOL_RESULTS_JSON=" + string(raw)
}

// ── the regression: the measured sequence ────────────────────────────────────

// 🔴 THE REGRESSION TEST, and it is the measured sequence: a bare `submit` the
// CLI refuses, then the `--yes` retry that must be ALLOWED. Red on origin/main,
// where the bare submit charges the cap and the retry is refused.
func TestDogfoodASubmitTheCLIRefusedDoesNotSpendTheCap(t *testing.T) {
	credPath, _ := credentialFile(t)

	// The commands as the model actually wrote them, `cd` prefix and `2>&1` and
	// all — the classifier reads command TEXT, so the shape is part of the case.
	const bare = "cd ab-generate-and-post && civitai app submit 2>&1"
	const withYes = "cd ab-generate-and-post && civitai app submit --yes 2>&1"
	const packageOnly = "cd ab-generate-and-post && civitai app submit --package-only 2>&1"

	for _, tc := range []struct {
		name     string
		commands []string
		// where the CLI's refusal landed. `2>&1` put it on stdout in the measured
		// trial; without that redirect the real CLI writes it to stderr, which
		// sh() appends under a `[stderr]` header — both must refund.
		onStderr bool
		want     []string
	}{
		{
			name:     "the measured sequence, refusal on stdout",
			commands: []string{bare, withYes},
			want:     []string{"run", "run"},
		},
		{
			name:     "the same refusal on stderr",
			commands: []string{bare, withYes},
			onStderr: true,
			want:     []string{"run", "run"},
		},
		{
			name:     "and the --package-only probe the trial also lost",
			commands: []string{bare, packageOnly, withYes},
			want:     []string{"run", "run", "run"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			planted := plantedResult{RC: 1, Out: measuredPreflightRefusal}
			if tc.onStderr {
				planted = plantedResult{RC: 1, Err: measuredPreflightRefusal}
			}
			tr := runFakeTrialEnv(t,
				[]string{plantedResults(t, map[string]plantedResult{bare: planted})},
				tc.commands, "",
				"--credential-file", credPath, "--app-prefix", "ab-",
				"--max-submissions", "1")
			body := readFile(t, tr.transcript)
			got := stepVerdicts(t, body)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("verdicts %v, want %v.\nThe bare submit was refused by the CLI before it "+
					"contacted anything — charging it spends the budget on nothing and blocks the "+
					"retry that works. That is what made ab-ship-mimo-01 grade SHIP=no.\n%s",
					got, tc.want, body)
			}
			// The cap must report the NET count, or a grader reading the summary
			// concludes a submission happened when none did.
			if n := endSubmissions(t, body); n != 1 {
				t.Errorf("the run's recorded `submissions` is %d, want 1 — one real submit "+
					"(`--yes`) happened; the refused attempt and the --package-only probe cost "+
					"nothing.\n%s", n, body)
			}
			// The refund is EVIDENCE, not silence: an operator reading the command
			// log must be able to see the budget go back.
			if !strings.Contains(readFile(t, tr.commandLog), "verdict=refund") {
				t.Errorf("the command log records no refund, so nothing in the artifacts explains "+
					"why the cap did not advance:\n%s", readFile(t, tr.commandLog))
			}
		})
	}
}

// ── the --package-only exemption ─────────────────────────────────────────────

// 🔴 `--package-only` CONTACTS NOTHING BY CONSTRUCTION, so a cap on it is pure
// over-refusal. Red on origin/main, where it is both capped and charged.
func TestDogfoodPackageOnlyIsNotASubmission(t *testing.T) {
	credPath, _ := credentialFile(t)

	// (a) It is EXEMPT: allowed even with the budget already exhausted.
	t.Run("allowed at a cap of zero", func(t *testing.T) {
		tr := runFakeTrial(t, []string{"civitai app submit --package-only"}, "",
			"--credential-file", credPath, "--app-prefix", "ab-", "--max-submissions", "0")
		if got := stepVerdicts(t, readFile(t, tr.transcript)); len(got) != 1 || got[0] != "run" {
			t.Fatalf("`--package-only` was refused at --max-submissions 0 (verdicts %v). It writes "+
				"a .zip and reaches no API — refusing it costs the trial a safe preview and "+
				"protects nothing.\n%s", got, readFile(t, tr.transcript))
		}
	})

	// (b) It is not CHARGED either — the half that cost step 51 its retry.
	t.Run("does not spend the budget", func(t *testing.T) {
		tr := runFakeTrial(t, []string{"civitai app submit --package-only", "civitai app submit --yes"}, "",
			"--credential-file", credPath, "--app-prefix", "ab-", "--max-submissions", "1")
		got := stepVerdicts(t, readFile(t, tr.transcript))
		want := []string{"run", "run"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("verdicts %v, want %v — a `--package-only` preview consumed the one real "+
				"submission the trial was allowed.\n%s", got, want, readFile(t, tr.transcript))
		}
	})

	// (c) 🔴 THE EXEMPTION IS A HOLE IN A CAP, so it must be granted only where
	// cobra would really read the flag as set. Each of these LOOKS like
	// `--package-only` on the command line and is not it.
	//
	// ⚠ INVARIANT GUARD, NOT REGRESSION COVERAGE — every arm is refused at
	// origin/main too, because there NOTHING is exempt. It is here so the
	// exemption cannot be satisfied by a substring search.
	t.Run("but only when cobra would read it as set", func(t *testing.T) {
		for _, tc := range []struct{ name, command string }{
			{"explicitly false", "civitai app submit --package-only=false"},
			{"another flag's value", "civitai app submit --caption --package-only"},
			{"a positional after --", "civitai app submit -- --package-only"},
			{"a value cobra cannot parse", "civitai app submit --package-only=maybe"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				tr := runFakeTrial(t, []string{tc.command}, "",
					"--credential-file", credPath, "--app-prefix", "ab-", "--max-submissions", "0")
				got := stepVerdicts(t, readFile(t, tr.transcript))
				if len(got) != 1 || got[0] != "refused" {
					t.Fatalf("%q was ALLOWED past a cap of 0 (verdicts %v) — the exemption was granted "+
						"by a token cobra does not read as `--package-only`, which is a hole in the "+
						"cap on real submissions.\n%s", tc.command, got, readFile(t, tr.transcript))
				}
			})
		}
	})
}

// ── the refund's own boundary ────────────────────────────────────────────────

// 🔴 THE FAIL-CLOSED HALF, AND THE MUTATION CONTROL FOR THE REFUND CONDITION.
// Everything that is not a proof must charge. A refund keyed on "the output
// mentions the refusal" or on "the command was a submit" passes the regression
// test above and every arm here goes red.
//
// ⚠ INVARIANT GUARD, NOT REGRESSION COVERAGE, AND COUNTED SEPARATELY. At
// origin/main every arm passes, because there NOTHING is ever refunded. Its job
// is to stop the fix from over-refunding, which the regression test cannot see.
func TestDogfoodOnlyAProvenNonContactIsRefunded(t *testing.T) {
	credPath, _ := credentialFile(t)
	const first = "civitai app submit --yes"
	const retry = "civitai app submit --yes --allow-dirty"

	for _, tc := range []struct {
		name    string
		planted plantedResult
		why     string
	}{
		{
			name:    "a submission that succeeded",
			planted: plantedResult{RC: 0, Out: "Submitted demo-block@0.1.0 — publish request pubreq_01ABC (pending)"},
			why:     "it reached the API and created a real moderator-review request",
		},
		{
			name:    "a submission that failed AT the server",
			planted: plantedResult{RC: 1, Out: "Error: submit failed: 500 Internal Server Error"},
			why:     "it exited non-zero, but it contacted the API to find that out",
		},
		{
			name:    "the refusal text on a command that exited ZERO",
			planted: plantedResult{RC: 0, Out: measuredPreflightRefusal},
			why: "exit 0 means the invocation did something and returned happy, so the sentence " +
				"came from somewhere else (an `|| true`, a --help dump, an echo) — not from the CLI refusing",
		},
		{
			name:    "a DIFFERENT refusal",
			planted: plantedResult{RC: 1, Out: "Error: refusing to submit: the working tree is dirty"},
			why:     "only the pre-flight gate is known to run before the network; this one is not it",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := runFakeTrialEnv(t,
				[]string{plantedResults(t, map[string]plantedResult{first: tc.planted})},
				[]string{first, retry}, "",
				"--credential-file", credPath, "--app-prefix", "ab-",
				"--max-submissions", "1")
			body := readFile(t, tr.transcript)
			got := stepVerdicts(t, body)
			want := []string{"run", "refused"}
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("verdicts %v, want %v — this attempt was REFUNDED and it must not be: %s.\n%s",
					got, want, tc.why, body)
			}
			if n := endSubmissions(t, body); n != 1 {
				t.Errorf("recorded `submissions` is %d, want 1 — the charge was given back.\n%s", n, body)
			}
		})
	}
}

// 🔴 A REFUND CAN ONLY GIVE BACK WHAT THE COMMAND JUST TOOK. Two submits in one
// command charge twice; a single pre-flight refusal in that output cannot say
// which of the two produced it, so there is no attribution and nothing is
// refunded.
//
// ⚠ INVARIANT GUARD at origin/main (nothing is refunded there), counted
// separately. It exists because the cheapest wrong implementation — "if the
// output says the CLI refused, decrement" — is blind to the count.
func TestDogfoodAnUnattributableRefusalRefundsNothing(t *testing.T) {
	credPath, _ := credentialFile(t)
	const both = "civitai app submit --yes; civitai app submit"
	tr := runFakeTrialEnv(t,
		[]string{plantedResults(t, map[string]plantedResult{
			both: {RC: 1, Out: measuredPreflightRefusal},
		})},
		[]string{both}, "",
		"--credential-file", credPath, "--app-prefix", "ab-",
		"--max-submissions", "2")
	body := readFile(t, tr.transcript)
	if got := stepVerdicts(t, body); len(got) != 1 || got[0] != "run" {
		t.Fatalf("CONTROL failure, not a finding: the command did not run (verdicts %v), so nothing "+
			"below is about the refund.\n%s", got, body)
	}
	if n := endSubmissions(t, body); n != 2 {
		t.Fatalf("recorded `submissions` is %d, want 2. One refusal in the output of a command that "+
			"charged TWICE is not attributable to either attempt — refunding on it gives back a "+
			"submission that may really have been sent.\n%s", n, body)
	}
}

// 🔴 AND IT CANNOT REACH BACK. A submit charges; a LATER command that merely
// prints the refusal sentence must not hand that earlier charge back.
//
// ⚠ INVARIANT GUARD at origin/main, counted separately. This is the arm that
// fails if the pending charge is not reset on every judged command.
func TestDogfoodARefundCannotGiveBackAnEarlierCommandsCharge(t *testing.T) {
	credPath, _ := credentialFile(t)
	const submit = "civitai app submit --yes"
	const echoIt = "echo " + `'` + measuredPreflightRefusal + `'` + "; exit 1"
	const retry = "civitai app submit --yes --allow-dirty"
	tr := runFakeTrialEnv(t,
		[]string{plantedResults(t, map[string]plantedResult{
			echoIt: {RC: 1, Out: measuredPreflightRefusal},
		})},
		[]string{submit, echoIt, retry}, "",
		"--credential-file", credPath, "--app-prefix", "ab-",
		"--max-submissions", "1")
	body := readFile(t, tr.transcript)
	got := stepVerdicts(t, body)
	want := []string{"run", "run", "refused"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("verdicts %v, want %v — a command that was never a submit gave back the submission "+
			"the first command spent.\n%s", got, want, body)
	}
}

// 🔴 THE PREDICATE'S PARSE, EXERCISED DIRECTLY, because two of its branches are
// NOT REACHABLE through a trial. `sh()` always prepends its own `exit code: N`
// line, so a result whose exit code cannot be read — and a timeout, whose result
// the runner synthesises rather than receiving — cannot be planted by the
// fixture. Running the trial for those would assert something about the fixture.
//
// ⚠ NOT REGRESSION COVERAGE, AND NOT COUNTED AS SUCH. `submit_contacted_nothing`
// does not exist at origin/main, so at base this dies with an AttributeError — a
// DIFFERENT failure from the one it tests. The red-at-base evidence is
// TestDogfoodASubmitTheCLIRefusedDoesNotSpendTheCap.
func TestDogfoodSubmitNonContactPredicate(t *testing.T) {
	type arm struct {
		name   string
		result string
		want   bool
	}
	arms := []arm{
		{"the measured refusal", "exit code: 1\n" + measuredPreflightRefusal, true},
		{"under a [stderr] header", "exit code: 1\nfake\n[stderr]\n" + measuredPreflightRefusal, true},
		{"exit zero", "exit code: 0\n" + measuredPreflightRefusal, false},
		{"no refusal", "exit code: 1\nError: 500 Internal Server Error", false},
		// The two unreachable-through-a-trial branches, which is why this test
		// exists at all.
		{"a timeout", "exit code: -1\n[command timed out after 300s]", false},
		{"no exit-code line", measuredPreflightRefusal, false},
		{"an empty result", "", false},
	}
	// POSITIVE CONTROL: at least one arm must come back true, or a predicate wired
	// to `return False` satisfies every other row here.
	sawTrue := false
	for _, a := range arms {
		if a.want {
			sawTrue = true
		}
	}
	if !sawTrue {
		t.Fatal("CONTROL failure, not a finding: no arm expects a refund, so a predicate that never " +
			"refunds would pass this table")
	}
	for _, a := range arms {
		t.Run(a.name, func(t *testing.T) {
			if got := runnerSaysContactedNothing(t, a.result); got != a.want {
				t.Errorf("submit_contacted_nothing(%q) = %v, want %v", a.result, got, a.want)
			}
		})
	}
}

// endSubmissions reads the run's final `submissions` count off the `end` record —
// the number a grader and the summary both report.
func endSubmissions(t *testing.T, transcript string) int {
	t.Helper()
	got := -1
	found := false
	for _, line := range strings.Split(strings.TrimSpace(transcript), "\n") {
		var r struct {
			Kind        string `json:"kind"`
			Submissions *int   `json:"submissions"`
		}
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("transcript line is not JSON: %q", line)
		}
		if r.Kind == "end" {
			found = true
			if r.Submissions != nil {
				got = *r.Submissions
			}
		}
	}
	if !found {
		t.Fatalf("CONTROL failure, not a finding: the transcript has no `end` record, so there is no "+
			"count to read:\n%s", transcript)
	}
	if got < 0 {
		t.Fatalf("CONTROL failure, not a finding: the `end` record carries no `submissions` key — the "+
			"caps were not armed, so every count assertion in this file is vacuous:\n%s", transcript)
	}
	return got
}
