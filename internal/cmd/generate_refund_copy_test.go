package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
)

// #278's guard, rebuilt. The first attempt policed the refund copy with a
// substring ledger of directional claims, and an audit mutant walked straight
// through it: rewriting the terminal-status error to "your Buzz returns to your
// balance automatically; confirm with `civitai buzz`" asserts a refund, matches
// no banned phrase, and left all eighteen packages green.
//
// 🔴 THE LESSON IS THE REPO'S OWN: a guard that can pass while the hazard exists
// IN A DIFFERENT SHAPE is a spelled guard, not a structural one. "This sentence
// decides the refund question" is not computable from text, so no phrase list
// can be the guard. What IS structural is the single constant every such surface
// must render, plus an asserted ledger of the surfaces. A paraphrase drops the
// constant (guard B); a new surface with its own wording grows the ledger
// (guard C); and editing the CONSTANT itself — the one mutation the
// consolidation would otherwise propagate everywhere silently — has to keep
// asserting non-observability (guard A).
//
// The three do not subsume one another. B alone passes if the constant is
// reworded; A alone passes if a site stops using it; C alone passes if both are
// wrong in the same place.

// refundDirectionalClaims is retained ONLY as guard A's input — a check on one
// reviewed literal, never on free-form output. Applied to arbitrary copy it is
// the spelled guard described above; applied to a single constant whose job is
// to be non-directional, an explicit list of the claims it must not make is
// exactly the right shape.
var refundDirectionalClaims = []string{
	"it was charged and",
	"was charged and produced",
	"is not refunded",
	"not refunded",
	"no refund",
	"was refunded",
	"has been refunded",
	"will be refunded",
	"returns to your balance",
	"back to your balance",
	"credited back",
	"you get it back",
	"automatically refund",
}

// buzzLedgerObservabilityClaim is the load-bearing half of the constant: it says
// what this PROCESS cannot see, which is true whatever the orchestrator decides.
// Guard A requires it, and that requirement is what defeats a paraphrase — every
// sentence that answers the refund question has to stop saying we cannot answer
// it.
const buzzLedgerObservabilityClaim = "cannot see your Buzz ledger"

// GUARD A — the constant itself may not decide the refund question.
//
// Consolidating three sentences into one constant buys a real property and
// creates one new risk: a single edit now reaches every surface at once. This is
// the guard on that edit.
func TestBuzzLedgerNote_AssertsNonObservabilityAndNoDirection(t *testing.T) {
	if !strings.Contains(buzzLedgerUnknownNote, buzzLedgerObservabilityClaim) {
		t.Fatalf("buzzLedgerUnknownNote no longer says %q.\n"+
			"That clause is the whole point: it is a claim about what this process can SEE, which stays true whatever the "+
			"orchestrator does with the charge. A note that drops it has started answering the refund question.\n  got: %s",
			buzzLedgerObservabilityClaim, buzzLedgerUnknownNote)
	}
	lower := strings.ToLower(buzzLedgerUnknownNote)
	for _, banned := range refundDirectionalClaims {
		if strings.Contains(lower, banned) {
			t.Errorf("buzzLedgerUnknownNote decides the refund question with %q. The orchestrator owns that rule and it is "+
				"not readable from the civitai monorepo (AGENTS.md item 28).\n  got: %s", banned, buzzLedgerUnknownNote)
		}
	}
	// It must also stay actionable: a note that says only "we cannot tell you"
	// leaves the user with nowhere to go.
	for _, want := range []string{"civitai buzz", "/user/transactions"} {
		if !strings.Contains(buzzLedgerUnknownNote, want) {
			t.Errorf("buzzLedgerUnknownNote no longer points at %q — the user is told the CLI cannot answer and given no way to", want)
		}
	}
}

// GUARD A' — the POSITIVE CONTROL, and the one the first attempt did not have.
//
// The original control compared two string literals to each other and could not
// fail. This one runs the REAL predicate — the body of guard A — over a corpus
// of sentences that must be rejected, including the exact audit mutant that
// survived, and over the real constant, which must be accepted. A ban list that
// has stopped matching, or a predicate wired to nothing, fails here.
func TestBuzzLedgerNote_GuardCanStillFire(t *testing.T) {
	rejects := func(s string) bool {
		if !strings.Contains(s, buzzLedgerObservabilityClaim) {
			return true
		}
		lower := strings.ToLower(s)
		for _, banned := range refundDirectionalClaims {
			if strings.Contains(lower, banned) {
				return true
			}
		}
		return false
	}

	mustReject := []struct{ name, text string }{
		{"the retracted claim", "it was charged and produced no usable result"},
		{"the retracted exclusion note", "These were still part of the workflow you were charged for — a blocked or missing output is not refunded."},
		{"the equal-and-opposite claim", "Your Buzz has been refunded."},
		// 🔴 THE AUDIT MUTANT. It survived the previous guard set with all
		// eighteen packages green. It asserts a refund and contains no banned
		// phrase from the original list; it is caught here because it stops
		// claiming non-observability.
		{"the audit paraphrase", "your Buzz returns to your balance automatically; confirm with `civitai buzz`"},
		{"a softer paraphrase", "the charge is reversed for you; check `civitai buzz`"},
		{"a hedge that still decides", "this CLI cannot see your Buzz ledger, but it will be refunded"},
	}
	for _, c := range mustReject {
		if !rejects(c.text) {
			t.Errorf("CONTROL failure: guard A accepts %s, which asserts a direction:\n  %s", c.name, c.text)
		}
	}
	// And it must ACCEPT the real thing — a guard that rejects everything is
	// equally useless, and would be satisfied by deleting the copy entirely.
	if rejects(buzzLedgerUnknownNote) {
		t.Fatalf("CONTROL failure: guard A rejects the constant it is written to accept:\n  %s", buzzLedgerUnknownNote)
	}
}

// GUARD B — every surface that speaks about the fate of a charge renders the
// constant VERBATIM.
//
// This is what the audit mutant trips: it replaced the sentence, so the constant
// is simply not there. Asserting the constant rather than a phrase means the
// guard does not care how the surrounding sentence is worded, only that the one
// reviewed claim is the one being made.
func TestBuzzLedgerNote_IsRenderedOnEverySpendOutcomeSurface(t *testing.T) {
	withStdinTTY(t, false)

	t.Run("terminal_status_error", func(t *testing.T) {
		for _, st := range []string{genapi.StatusFailed, genapi.StatusExpired, genapi.StatusCanceled} {
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
			if !strings.Contains(err.Error(), buzzLedgerUnknownNote) {
				t.Errorf("%s: the error does not render buzzLedgerUnknownNote verbatim — it has grown its own wording for the "+
					"money question, which is how a paraphrase that ASSERTS a refund gets in (#278).\n  got:  %v\n  want: …%s…",
					st, err, buzzLedgerUnknownNote)
			}
		}
	})

	t.Run("excluded_outputs_note", func(t *testing.T) {
		blocked := "minor"
		var b bytes.Buffer
		reportExcludedOutputs(&b, []genapi.Output{{Blob: genapi.Blob{ID: "out_1", BlockedReason: &blocked}}})
		if b.Len() == 0 {
			t.Fatal("CONTROL failure: reportExcludedOutputs printed nothing")
		}
		if !strings.Contains(b.String(), buzzLedgerUnknownNote) {
			t.Errorf("the exclusion note does not render buzzLedgerUnknownNote verbatim:\n%s", b.String())
		}
		// The charge half must SURVIVE — a succeeded workflow did run and was
		// billed, and softening that would over-fix a true statement.
		if !strings.Contains(b.String(), "charged") {
			t.Errorf("the exclusion note no longer says the workflow was charged; that half is not in doubt:\n%s", b.String())
		}
	})

	t.Run("help_text", func(t *testing.T) {
		long := newGenerateCmd().Long
		if len(long) < 500 {
			t.Fatalf("CONTROL failure: the generate Long text is %d bytes — too short to be the real help", len(long))
		}
		if !strings.Contains(long, buzzLedgerUnknownNote) {
			t.Errorf("`civitai generate --help` does not render buzzLedgerUnknownNote verbatim, so the screen can drift from "+
				"what the command actually prints:\n---\n%s", long)
		}
	})

	// The cancel surfaces joined the ledger in round 3. `generate`'s
	// terminal-status error already covered `canceled`, so leaving `workflows
	// cancel` asserting "does NOT refund anything" made the two commands
	// contradict each other about the same event.
	t.Run("cancel_help_text", func(t *testing.T) {
		long := newWorkflowsCancelCmd().Long
		if len(long) < 400 {
			t.Fatalf("CONTROL failure: the cancel Long text is %d bytes — too short to be the real help", len(long))
		}
		if !strings.Contains(long, buzzLedgerUnknownNote) {
			t.Errorf("`civitai workflows cancel --help` does not render buzzLedgerUnknownNote verbatim:\n---\n%s", long)
		}
	})

	t.Run("cancel_prompt_and_result", func(t *testing.T) {
		withStdinTTY(t, true)
		c, _, errb := genCmd("n\n")
		if err := confirmCancel(c, "01JABCXYZ", false); err == nil {
			t.Fatal("declining the cancel prompt must not report success")
		}
		if !strings.Contains(errb.String(), buzzLedgerUnknownNote) {
			t.Errorf("the cancel confirmation does not render buzzLedgerUnknownNote verbatim:\n%s", errb.String())
		}
	})
}

// GUARD C — the ASSERTED LEDGER of call sites.
//
// B checks the surfaces we know about. It is structurally blind to a NEW one: a
// future error path that invents its own sentence about the charge is never
// rendered by any test here, so nothing notices. The ledger closes that by
// pinning the set of production files that reference the constant, failing when
// it GROWS (a site added without review) and when it SHRINKS (a site that
// stopped using it — the paraphrase case, caught a second way).
//
// This is the idiom neterr_ledger_test.go (item 24) and item 26's caller ledger
// already use, for the same reason.
func TestBuzzLedgerNoteIsTheOnlyRefundWording(t *testing.T) {
	// file -> number of references, DECLARATION INCLUDED.
	want := map[string]int{
		"generate.go":         3, // the const declaration, the --help header, the terminal-status error
		"generate_output.go":  1, // the excluded-outputs note
		"workflows_cancel.go": 3, // the cancel --help, the pre-cancel prompt, the post-cancel note
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot read the package directory: %v", err)
	}
	got := map[string]int{}
	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, rerr := os.ReadFile(filepath.Clean(name))
		if rerr != nil {
			t.Fatalf("CONTROL failure: read %s: %v", name, rerr)
		}
		scanned++
		// COMMENT LINES ARE EXCLUDED. The constant's own doc comment names it,
		// and so should any future comment explaining a site — counting those
		// would make the ledger fail on a prose edit, which is a false failure
		// at correct content and the fastest way to get a docs-adjacent guard
		// deleted. What is pinned is the set of places that RENDER it.
		n := 0
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			n += strings.Count(line, "buzzLedgerUnknownNote")
		}
		if n > 0 {
			got[name] = n
		}
	}
	// POSITIVE CONTROL. A scan that read no files, or a renamed identifier,
	// produces an empty map — which would compare unequal and report the right
	// answer for the wrong reason, or (if want were ever emptied) pass while
	// checking nothing.
	if scanned < 10 {
		t.Fatalf("CONTROL failure, not a finding: scanned only %d non-test .go files in internal/cmd. "+
			"The ledger below would be comparing against almost nothing.", scanned)
	}
	if len(got) == 0 {
		t.Fatalf("CONTROL failure, not a finding: the identifier buzzLedgerUnknownNote appears in 0 of %d scanned files. "+
			"It was renamed or the scan is broken — do not read this as 'no sites'.", scanned)
	}

	if fmt.Sprint(sortedPairs(got)) != fmt.Sprint(sortedPairs(want)) {
		t.Fatalf("the buzzLedgerUnknownNote call-site ledger changed.\n  got:  %v\n  want: %v\n\n"+
			"🔴 It fails in BOTH directions on purpose. A file that GAINED a reference is a new surface talking about the fate "+
			"of a charge — add it here and to TestBuzzLedgerNote_IsRenderedOnEverySpendOutcomeSurface so it is actually read. A "+
			"file that LOST one has started wording the money question itself, which is the #278 defect returning: an audit "+
			"mutant did exactly that and left every package green (AGENTS.md item 28).",
			sortedPairs(got), sortedPairs(want))
	}
}

func sortedPairs(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, fmt.Sprintf("%s=%d", k, v))
	}
	sort.Strings(out)
	return out
}

// 🔴 THE SPEND CONFIRMATION HAD NO COVERAGE AT ALL, and this PR edits it.
//
// `confirmGenerate` prints the last line a user reads before `Generate? [y/N]`
// on the CLI's only irreversibly-spending path. An audit mutant replaced that
// line with "This is free and fully refundable." and the whole suite stayed
// green: rc=0, zero failing packages, zero failing tests. Both neighbouring
// surfaces gained paired guards in this PR while the one in the middle — the
// one an actual human reads with their finger over the y key — was unpinned.
//
// The assertion is paired, like its neighbours: the deterrent must be PRESENT
// (it spends real Buzz, it cannot be undone) and no refund direction may be
// asserted. The prompt must also still be a prompt.
func TestConfirmGenerate_SpendWarningIsPresentAndNonDirectional(t *testing.T) {
	withStdinTTY(t, true)

	var s genSeams
	s.whatIf = func(context.Context, genapi.Graph) (*genapi.WhatIfResult, json.RawMessage, error) {
		return okQuote(12), okQuoteRaw(12), nil
	}
	o := baseOpts()
	// "n" declines: the assertions are about what was PRINTED before the
	// decision, and declining keeps the submit seam out of the picture entirely.
	c, _, errb := genCmd("n\n")
	if err := runGenerate(c, s.deps(t), o); err == nil {
		t.Fatal("declining at the prompt must not report success")
	}
	if s.submitCalls != 0 {
		t.Fatalf("declining still submitted %d time(s)", s.submitCalls)
	}
	out := errb.String()
	if out == "" {
		t.Fatal("CONTROL failure: the confirmation printed nothing, so every assertion below is vacuous")
	}

	// The prompt itself, so a mutation that guts the whole block cannot pass by
	// printing nothing.
	if !strings.Contains(out, "Generate? [y/N]") {
		t.Errorf("the confirmation prompt is gone:\n%s", out)
	}
	// The deterrent, asserted as STATE rather than as one sentence: the screen
	// must say it spends real Buzz AND that it cannot be undone.
	lower := strings.ToLower(out)
	for _, want := range []string{"spends real buzz", "cannot be undone"} {
		if !strings.Contains(lower, want) {
			t.Errorf("the spend confirmation no longer says %q. This is the last line before the y/N on the only path in this "+
				"CLI that spends money irreversibly.\n%s", want, out)
		}
	}
	// And the negative half — the same rule item 28 applies everywhere else.
	for _, banned := range refundDirectionalClaims {
		if strings.Contains(lower, banned) {
			t.Errorf("the spend confirmation decides the refund question with %q, before the run has even been submitted:\n%s", banned, out)
		}
	}
}
