package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
)

// GOLDEN-OUTPUT PINNING FOR EVERY SPEND-COPY SURFACE.
//
// 🔴 THIS EXISTS BECAUSE THE PREVIOUS GUARD CLOSED DELETION AND LEFT ADDITION
// WIDE OPEN, AND THAT WAS MEASURED. Round 2 required each surface to render
// `buzzLedgerUnknownNote` verbatim and banned a list of directional phrases.
// Five audit mutants kept the constant word-for-word and simply APPENDED a
// sentence beside it — "The Buzz has gone straight back to your account", "In
// practice the Buzz comes straight back.", and a `. **Nothing is refunded**.`
// that the list missed because it banned `"not refunded"` and not
// `"nothing is refunded"`. All five survived; 18 packages ok.
//
// 🔴 AND THE FIX IS NOT A LONGER LIST. The lesson round 2 wrote down — "does
// this sentence answer the refund question" is not computable from text — is
// exactly why the list lost again. Any enumeration of forbidden wordings is
// beaten by the next wording. What is computable is whether the rendered output
// is EXACTLY what was reviewed, so these guards pin the whole normalised
// rendering of each surface. An added sentence fails regardless of what it says,
// which is the only property that actually holds.
//
// # THE COST, ACCEPTED DELIBERATELY
//
// A cosmetic reflow of any of these surfaces breaks these tests. That is the
// trade: these are the screens a user reads while deciding to spend money
// irreversibly, or immediately after having done so, and "someone reworded the
// spend warning and no test noticed" is the failure this repo has now hit three
// times. Re-approve the new text with:
//
//	go test ./internal/cmd -run TestGoldenSpendCopy -update
//
// and READ the resulting diff — that diff is the review. Do not run -update to
// make a red test green without reading what moved.
//
// # WHY NORMALISED, AND WHAT NORMALISATION HIDES
//
// Comparison is on whitespace-collapsed text, so tabwriter column padding and
// hard-wrap positions do not fail the guard while wording is unchanged. It
// therefore CANNOT see a pure re-wrap, and it cannot see ANSI styling (the test
// writers are not TTYs, so `internal/ui` emits plain text). Neither carries a
// claim, which is why the residual is acceptable — but do not read a green here
// as "the rendering is byte-identical".

var updateGolden = flag.Bool("update", false, "rewrite the spend-copy golden files from the current rendering")

const goldenDir = "testdata/golden"

// normaliseCopy collapses every whitespace run to one space. See the header for
// what this deliberately cannot see.
func normaliseCopy(s string) string { return strings.Join(strings.Fields(s), " ") }

// assertGolden compares one rendered surface against its approved text.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join(goldenDir, name+".txt")
	norm := normaliseCopy(got)

	// POSITIVE CONTROL. An empty rendering normalises to "" and would match an
	// empty golden file forever, reporting a serene pass over a surface that has
	// stopped printing anything at all.
	if strings.TrimSpace(norm) == "" {
		t.Fatalf("CONTROL failure, not a finding: surface %q rendered nothing, so there is no copy to pin. "+
			"An empty rendering matches an empty golden and would pass silently.", name)
	}

	if *updateGolden {
		if err := os.MkdirAll(goldenDir, 0o755); err != nil {
			t.Fatalf("create %s: %v", goldenDir, err)
		}
		if err := os.WriteFile(path, []byte(norm+"\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		t.Logf("updated %s", path)
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v\n\nThis surface has no approved copy. If it is new, add it deliberately:\n"+
			"  go test ./internal/cmd -run TestGoldenSpendCopy -update\nand review the file the run creates.", path, err)
	}
	if strings.TrimSpace(string(want)) != norm {
		t.Errorf("the %s copy changed.\n\n  want: %s\n\n  got:  %s\n\n"+
			"🔴 EVERY change here is a change to what a user is told about money, INCLUDING an addition that leaves the "+
			"existing sentences intact — that is the exact mutation five audit paraphrases used to survive the previous "+
			"guard (AGENTS.md item 28). If the new text is right, re-approve it with `-update` and put the diff in the "+
			"review; if it added a claim about whether Buzz comes back, it is wrong.",
			name, strings.TrimSpace(string(want)), norm)
	}
}

// TestGoldenSpendCopy pins every surface that talks about money on the generate
// and cancel paths.
//
// 🔴 The set is the point as much as the contents. A surface missing from here
// is a surface where a sentence can be added silently; when you add one, add it
// here in the same change.
func TestGoldenSpendCopy(t *testing.T) {
	withStdinTTY(t, false)

	// --- `civitai generate --help` ------------------------------------------
	// The whole Long, not a chosen paragraph: N3 found that the four overage
	// claims reframed in this PR (`--max-cost` / `--timeout` flag usage, the
	// Long's own overage sentence, the --dry-run caveat) all reverted cleanly to
	// their retracted wording with the suite green, precisely because nothing
	// read them.
	t.Run("generate_help_long", func(t *testing.T) {
		assertGolden(t, "generate_help_long", newGenerateCmd().Long)
	})

	// --- the two flag usage strings -----------------------------------------
	t.Run("generate_flag_usage_max_cost", func(t *testing.T) {
		f := newGenerateCmd().Flags().Lookup("max-cost")
		if f == nil {
			t.Fatal("CONTROL failure: --max-cost is gone")
		}
		assertGolden(t, "generate_flag_usage_max_cost", f.Usage)
	})
	t.Run("generate_flag_usage_timeout", func(t *testing.T) {
		f := newGenerateCmd().Flags().Lookup("timeout")
		if f == nil {
			t.Fatal("CONTROL failure: --timeout is gone")
		}
		assertGolden(t, "generate_flag_usage_timeout", f.Usage)
	})

	// --- the interactive spend confirmation ---------------------------------
	t.Run("generate_confirmation", func(t *testing.T) {
		withStdinTTY(t, true)
		var s genSeams
		s.whatIf = func(context.Context, genapi.Graph) (*genapi.WhatIfResult, json.RawMessage, error) {
			return okQuote(12), okQuoteRaw(12), nil
		}
		c, _, errb := genCmd("n\n")
		if err := runGenerate(c, s.deps(t), baseOpts()); err == nil {
			t.Fatal("declining must not report success")
		}
		assertGolden(t, "generate_confirmation", errb.String())
	})

	// --- the --dry-run quote, both streams ----------------------------------
	// stderr carries the "Estimate only …" caveat, which prints on EVERY
	// --dry-run and had no coverage at all before this file.
	t.Run("generate_dry_run_streams", func(t *testing.T) {
		var s genSeams
		o := baseOpts()
		o.dryRun = true
		c, out, errb := genCmd("")
		if err := runGenerate(c, s.deps(t), o); err != nil {
			t.Fatalf("--dry-run: %v", err)
		}
		assertGolden(t, "generate_dry_run_stdout", out.String())
		assertGolden(t, "generate_dry_run_stderr", errb.String())
	})

	// --- the --dry-run quote WITH a model substitution ----------------------
	//
	// 🔴 THIS SURFACE EXISTS BECAUSE A GUARD WITHOUT IT WAS UNREACHABLE. Round 3
	// banned the word "generatable" from both --dry-run streams, and an audit
	// mutant re-added it to `substitutionAdvice` — which survived, because
	// reportModelSubstitutions prints on the ESTIMATE path and the only dry-run
	// fixture had no substitutions, so the banned word never rendered and the
	// ban never executed. "I wrote a guard" and "the guard runs" are different
	// claims; this fixture is what makes the second one true.
	//
	// It doubles as the FALSE-POSITIVE control for that ban: a legitimate
	// substitution notice must pass it, so if the guard ever starts rejecting
	// correct copy this goes red on real output rather than in review.
	t.Run("generate_dry_run_with_substitution_stderr", func(t *testing.T) {
		var s genSeams
		s.whatIf = func(context.Context, genapi.Graph) (*genapi.WhatIfResult, json.RawMessage, error) {
			q := okQuote(12)
			q.ModelSubstitutions = []genapi.ModelSubstitution{
				{Requested: 111, Applied: 222, Reason: genapi.SubstitutionUnrecognized},
			}
			return q, okQuoteRaw(12), nil
		}
		o := baseOpts()
		o.dryRun = true
		c, _, errb := genCmd("")
		if err := runGenerate(c, s.deps(t), o); err != nil {
			t.Fatalf("--dry-run with a substitution: %v", err)
		}
		if !strings.Contains(errb.String(), "222") {
			t.Fatalf("CONTROL failure: the substitution notice did not render, so this surface pins nothing:\n%s", errb.String())
		}
		assertGolden(t, "generate_dry_run_with_substitution_stderr", errb.String())
	})

	// --- the terminal-status error, per status ------------------------------
	for _, st := range []string{genapi.StatusFailed, genapi.StatusExpired, genapi.StatusCanceled} {
		t.Run("terminal_status_"+st, func(t *testing.T) {
			clock := newFakeClock()
			calls := 0
			var s genSeams
			s.poll = clock.cfg()
			s.getWorkflow = scriptedWorkflows(&calls, wfJSON(st))
			c, _, _ := genCmd("")
			err := runGenerate(c, s.deps(t), waitOpts(t.TempDir()))
			if err == nil {
				t.Fatalf("%s must not report success", st)
			}
			assertGolden(t, "terminal_status_"+st, err.Error())
		})
	}

	// --- the excluded-outputs note ------------------------------------------
	t.Run("excluded_outputs_note", func(t *testing.T) {
		blocked := "minor"
		var b bytes.Buffer
		reportExcludedOutputs(&b, []genapi.Output{{Blob: genapi.Blob{ID: "out_1", BlockedReason: &blocked}}})
		assertGolden(t, "excluded_outputs_note", b.String())
	})

	// --- the succeeded-but-empty error --------------------------------------
	// It is NOT in the buzzLedgerUnknownNote ledger (it states that a charge
	// happened, which is not in doubt) — but it is money copy, so it is pinned
	// here. That is the difference between the two guards: the ledger is about
	// which surfaces make the FATE claim; this is about none of them changing.
	t.Run("succeeded_no_deliverables", func(t *testing.T) {
		payload := `{"id":"wf_123","status":"succeeded","steps":[{
		  "$type":"textToImage","name":"s","status":"succeeded","metadata":{},
		  "output":{"images":[{"id":"a","type":"image","available":true,"url":"https://x/a.jpeg","blockedReason":"minor"}]}}]}`
		clock := newFakeClock()
		calls := 0
		var s genSeams
		s.poll = clock.cfg()
		s.getWorkflow = scriptedWorkflows(&calls, payload)
		// Wired to a failing fetcher rather than left nil: a nil seam turns a
		// filter regression into a panic instead of a readable failure.
		s.downloadBlob = func(context.Context, string) (*http.Response, error) {
			return nil, errors.New("no deliverable output should ever be fetched here")
		}
		c, _, _ := genCmd("")
		err := runGenerate(c, s.deps(t), waitOpts(t.TempDir()))
		if err == nil {
			t.Fatal("a workflow with no deliverable outputs must not report success")
		}
		assertGolden(t, "succeeded_no_deliverables", err.Error())
	})

	// --- `civitai workflows cancel` -----------------------------------------
	t.Run("cancel_help_long", func(t *testing.T) {
		assertGolden(t, "cancel_help_long", newWorkflowsCancelCmd().Long)
	})
	t.Run("cancel_group_help_long", func(t *testing.T) {
		assertGolden(t, "cancel_group_help_long", newWorkflowsCmd().Long)
	})

	// The two cancel RUNTIME surfaces. `--help` is what a user reads before
	// deciding; these are what they read at the moment of the act and just
	// after it, which is where a reassuring addition would do the most damage.
	t.Run("cancel_confirmation", func(t *testing.T) {
		withStdinTTY(t, true)
		c, _, errb := genCmd("n\n")
		if err := confirmCancel(c, "01JABCXYZ", false); err == nil {
			t.Fatal("declining the cancel prompt must not report success")
		}
		assertGolden(t, "cancel_confirmation", errb.String())
	})
	t.Run("cancel_non_tty_refusal", func(t *testing.T) {
		withStdinTTY(t, false)
		c, _, _ := genCmd("")
		err := confirmCancel(c, "01JABCXYZ", false)
		if err == nil {
			t.Fatal("a non-TTY cancel without --yes must be refused")
		}
		assertGolden(t, "cancel_non_tty_refusal", err.Error())
	})
}
