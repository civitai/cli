package cmd

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
	"github.com/civitai/cli/pkg/civitai"
)

// #331 / dogfood finding F13 — THE TWO DEAD-END ERRORS MAY NOT PROMISE A
// DIAGNOSIS THEY CANNOT DELIVER.
//
// Both errors below hand the user `civitai workflows get <id>` and stop there.
// The blind, credentialed dogfood run of 2026-08-07 followed that pointer on a
// failed job and got `Status: failed`, zero deliverable outputs and NO reason,
// because the orchestrator supplies none. The remaining strategy was to
// re-submit hypotheses at 8 Buzz each.
//
// 🔴 THE ASSERTIONS ARE PAIRED, AND THE SECOND HALF IS THE IMPORTANT ONE. The
// fix reduces an over-promise; it must not reduce CAPABILITY. The workflow id
// and the re-attach command are the user's only handle on a run they have
// already paid for, so each surface is checked to still carry both — a "fix"
// that deleted the pointer instead of qualifying it would satisfy the caveat
// assertion alone.
//
// The caveat is pinned as a LITERAL rather than by referencing the production
// constant on purpose: a test that imports the constant it is checking passes by
// construction, and cannot be watched to fail against the pre-#331 tree because
// it would not compile there.
//
// 🔴 #367 MADE THE CAVEAT CONDITIONAL, AND THIS FILE NOW COVERS ONE BRANCH OF
// IT. Every fixture below carries no `steps[].output.errors`, which is the
// measured branch where the orchestrator genuinely supplied nothing and the
// sentence is still true. Where it DID supply a reason the reason is printed
// instead, and asserting the caveat there would be asserting a falsehood — that
// direction lives in generate_failure_reason_test.go. Do not "repair" this file
// by making its assertion unconditional.
const deadEndReasonCaveat = "the orchestrator often supplies no failure reason"

// The exact pre-#331 wording of both errors, kept as the POSITIVE CONTROL for
// the predicate below: if `Contains(msg, deadEndReasonCaveat)` ever stops being
// able to distinguish qualified copy from unqualified copy, these fail first.
var deadEndPre331Wording = []string{
	"the generation finished with status \"failed\" and produced no usable result; " +
		buzzLedgerUnknownNote + ". Inspect the run with `civitai workflows get wf_123`",
	"the generation succeeded but produced no deliverable outputs — it was charged; " +
		"inspect it with `civitai workflows get wf_123`",
}

func TestGenerate_DeadEndErrorsDoNotPromiseADiagnosis(t *testing.T) {
	withStdinTTY(t, false)

	// POSITIVE CONTROL first: the predicate must REJECT the copy this change
	// replaced. A guard that accepts the old text is wired to nothing.
	for _, old := range deadEndPre331Wording {
		if strings.Contains(old, deadEndReasonCaveat) {
			t.Fatalf("CONTROL failure, not a finding: the pre-#331 wording already contains the caveat, so every "+
				"assertion below would pass without the fix:\n  %s", old)
		}
		// …and the control corpus must be the right shape: it has to carry the
		// pointer whose survival is asserted, or the capability half is vacuous.
		if !strings.Contains(old, "civitai workflows get") {
			t.Fatalf("CONTROL failure, not a finding: the control corpus lost the re-attach command:\n  %s", old)
		}
	}

	// --- the non-succeeded terminal-status error, per status ----------------
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
			assertDeadEndCopy(t, err)
		})
	}

	// --- the succeeded-but-zero-deliverables error --------------------------
	t.Run("succeeded_no_deliverables", func(t *testing.T) {
		payload := `{"id":"wf_123","status":"succeeded","steps":[{
		  "$type":"textToImage","name":"s","status":"succeeded","metadata":{},
		  "output":{"images":[{"id":"a","type":"image","available":true,"url":"https://x/a.jpeg","blockedReason":"minor"}]}}]}`
		clock := newFakeClock()
		calls := 0
		var s genSeams
		s.poll = clock.cfg()
		s.getWorkflow = scriptedWorkflows(&calls, payload)
		// Wired to a failing fetcher rather than left nil, so a filter regression
		// is a readable failure instead of a panic.
		s.downloadBlob = func(context.Context, string) (*http.Response, error) {
			return nil, errors.New("no deliverable output should ever be fetched here")
		}
		c, _, _ := genCmd("")
		err := runGenerate(c, s.deps(t), waitOpts(t.TempDir()))
		if err == nil {
			t.Fatal("a workflow with no deliverable outputs must not report success")
		}
		assertDeadEndCopy(t, err)
	})
}

// assertDeadEndCopy is the paired predicate: the promise is qualified AND the
// handle survives — plus the exit-code pin that message assertions cannot carry.
func assertDeadEndCopy(t *testing.T, err error) {
	t.Helper()
	msg := err.Error()
	if strings.TrimSpace(msg) == "" {
		t.Fatal("CONTROL failure, not a finding: the surface rendered no message at all")
	}
	if !strings.Contains(msg, deadEndReasonCaveat) {
		t.Errorf("this error names `civitai workflows get` without saying that it often cannot answer. The 2026-08-07 "+
			"dogfood run followed exactly this pointer on a failed job and got a status and no reason, and paid 8 Buzz "+
			"per further hypothesis (#331).\n  want: …%s…\n  got:  %s", deadEndReasonCaveat, msg)
	}
	// 🔴 THE CAPABILITY HALF. #331 reduces an over-promise; it must not reduce
	// what the user can still do with a run they have already been charged for.
	for _, want := range []string{"civitai workflows get", "wf_123"} {
		if !strings.Contains(msg, want) {
			t.Errorf("this error no longer carries %q — the caveat was supposed to QUALIFY the re-attach pointer, not "+
				"remove it; it is the user's only handle on a paid run.\n  got: %s", want, msg)
		}
	}
	// And the caveat must not have smuggled in a direction on the charge
	// (AGENTS.md item 28). Defence in depth: the golden files are the guard.
	lower := strings.ToLower(msg)
	for _, banned := range refundDirectionalClaims {
		if strings.Contains(lower, banned) {
			t.Errorf("this error decides the refund question with %q, which the CLI cannot observe:\n  %s", banned, msg)
		}
	}

	// 🔴 EXIT-CODE PIN (AGENTS.md item 7). Everything above reads message TEXT,
	// which says nothing about what a script sees — and #331 is a message-only
	// change, so the classification must not move. The published contract for
	// both of these failures is the GENERIC code, reached by carrying no
	// classification sentinel at all. The terminal-status error already has this
	// pin in generate_wait_test.go; the succeeded-but-empty error had NONE until
	// this change edited its text, which is exactly the seam item 7 names.
	for _, k := range []struct {
		sentinel error
		name     string
	}{
		{civitai.ErrBadRequest, "ErrBadRequest"},
		{civitai.ErrUnauthorized, "ErrUnauthorized"},
		{civitai.ErrNotFound, "ErrNotFound"},
		{civitai.ErrRateLimited, "ErrRateLimited"},
		{civitai.ErrNetwork, "ErrNetwork"},
	} {
		if errors.Is(err, k.sentinel) {
			t.Errorf("this error is now classified %s, which changes its published exit code. Rewording a message must "+
				"not reclassify it.", k.name)
		}
	}
	if errors.Is(err, ErrUsage) {
		t.Errorf("this error is now cmd.ErrUsage (exit 2) — a generation that produced nothing is not a usage mistake")
	}
}
