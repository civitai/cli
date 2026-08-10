package cmd

import (
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
)

// #346 — THE ONE VIEW THAT DISCLAIMED THE KNOWLEDGE WAS THE ONE VIEW THAT
// DISCARDED IT.
//
// A blind credentialed dogfood run read a failed workflow back with
// `civitai workflows get` and was told *"this CLI cannot see your Buzz ledger …
// settle it against your Buzz transaction history (/user/transactions)"* — while
// the payload the command had just parsed itemised the run:
//
//	"transactions":{"list":[{"type":"debit","amount":8,…},
//	                        {"type":"credit","amount":8,…}]}
//
// `civitai workflows list` had been rendering that same settlement as `COST 0`
// all along. The command sent the user to a website for a number on their screen.
//
// 🔴 EVERY ASSERTION BELOW IS ABOUT BEHAVIOUR THIS PACKAGE ALREADY EXPOSED —
// runWorkflowsGet, runGenerate, the shared constant — and NOT about any symbol
// #346 introduced. That is deliberate: this file compiles against the
// pre-change tree, so it can be watched to FAIL there. A regression test that
// only compiles after the fix proves the fix exists, not that it fixed anything.
//
// It asserts NO refund rule, in either direction. What it pins is that a
// disclaimer of knowledge is not printed alongside the knowledge, and that the
// disclaimer SURVIVES wherever the payload really is silent.

// wfWithTransactions is the #346 payload: a failed run with one non-deliverable
// output (so the excluded-output note, which carried the disclaimer, fires) and
// the orchestrator's own debit/credit pair.
const wfWithTransactions = `{
  "id":"wf_346","status":"failed","createdAt":"2026-08-10T09:00:00Z","completedAt":"2026-08-10T09:00:20Z",
  "transactions":{"list":[
    {"type":"debit","amount":8,"accountType":"user","createdAt":"2026-08-10T09:00:00Z"},
    {"type":"credit","amount":8,"accountType":"user","createdAt":"2026-08-10T09:00:19Z"}]},
  "steps":[{"$type":"textToImage","name":"s","status":"failed",
    "output":{"images":[{"id":"gone","type":"image","available":false}]}}]}`

// wfWithoutTransactions is the SAME workflow with the record absent — the case
// the disclaimer is actually for.
const wfWithoutTransactions = `{
  "id":"wf_346b","status":"failed","createdAt":"2026-08-10T09:00:00Z","completedAt":"2026-08-10T09:00:20Z",
  "steps":[{"$type":"textToImage","name":"s","status":"failed",
    "output":{"images":[{"id":"gone","type":"image","available":false}]}}]}`

// TestWorkflowsGet_RendersTheTransactionsItParsed is #346 itself.
func TestWorkflowsGet_RendersTheTransactionsItParsed(t *testing.T) {
	c, out, errb := genCmd("")
	if err := runWorkflowsGet(c, wfGetDeps(wfWithTransactions, nil), workflowsGetOpts{}, "wf_346"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	stdout, stderr := out.String(), errb.String()
	both := stdout + stderr

	// POSITIVE CONTROL. If the excluded-output report stopped rendering, the
	// disclaimer assertion below would pass for the wrong reason — the surface
	// that carried it would simply be gone.
	if !strings.Contains(stderr, "will NOT be saved") {
		t.Fatalf("CONTROL failure: the excluded-output report did not render, so the disclaimer assertion below is "+
			"vacuous.\nstderr:\n%s", stderr)
	}

	// The record itself: both entries and the arithmetic over them.
	for _, want := range []string{"debit", "credit", "net"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("#346: `workflows get` did not render the %q the payload contains.\nstdout:\n%s", want, stdout)
		}
	}
	// The amounts, so a heading with no numbers under it cannot pass.
	if !strings.Contains(stdout, "8") {
		t.Errorf("#346: the transaction amounts are missing.\nstdout:\n%s", stdout)
	}

	// THE DEFECT: the disclaimer must not be printed beside the data.
	if strings.Contains(both, buzzLedgerUnknownNote) {
		t.Errorf("#346: `workflows get` printed %q for a workflow whose own transactions it had already parsed. "+
			"That sentence sends the user to a website for numbers that are on their screen.\nstdout:\n%s\nstderr:\n%s",
			buzzLedgerUnknownNote, stdout, stderr)
	}
	// …and neither may the pointer on its own: /user/transactions answers the
	// ACCOUNT-WIDE question, which is not the one this view was asked.
	if strings.Contains(both, "/user/transactions") {
		t.Errorf("#346: the account-wide transaction-history pointer is still on the per-workflow view.\n"+
			"stdout:\n%s\nstderr:\n%s", stdout, stderr)
	}

	// The half that is NOT in doubt must survive: the workflow was charged.
	if !strings.Contains(stderr, "charged") {
		t.Errorf("the excluded-output note no longer says the workflow was charged; that half was never in doubt.\n%s", stderr)
	}
}

// The COUNTERPART, and the reason this is not a deletion: a payload that really
// carries no transaction record still gets the disclaimer. Without this, "remove
// the sentence everywhere" would pass the test above.
func TestWorkflowsGet_KeepsTheDisclaimerWhenThePayloadIsSilent(t *testing.T) {
	c, out, errb := genCmd("")
	if err := runWorkflowsGet(c, wfGetDeps(wfWithoutTransactions, nil), workflowsGetOpts{}, "wf_346b"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	stdout, stderr := out.String(), errb.String()
	if !strings.Contains(stderr, "will NOT be saved") {
		t.Fatalf("CONTROL failure: the excluded-output report did not render.\nstderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, buzzLedgerUnknownNote) {
		t.Errorf("a workflow with NO transaction record must keep the ledger disclaimer — an absent record is not "+
			"evidence that no money moved.\nstderr:\n%s", stderr)
	}
	// And no settlement block may be invented for it.
	if strings.Contains(stdout, "Buzz transactions") {
		t.Errorf("a settlement block was rendered for a payload that carries none:\n%s", stdout)
	}
}

// The same defect on the money-spending path: `generate` polls the workflow back
// and its terminal-status error carried the same disclaimer.
func TestGenerate_TerminalStatusRendersTheTransactionsItPolled(t *testing.T) {
	withStdinTTY(t, false)
	clock := newFakeClock()
	calls := 0
	var s genSeams
	s.poll = clock.cfg()
	s.getWorkflow = scriptedWorkflows(&calls, wfWithTransactions)

	c, out, errb := genCmd("")
	err := runGenerate(c, s.deps(t), waitOpts(t.TempDir()))
	if err == nil {
		t.Fatal("a failed workflow must not report success")
	}
	stderr := errb.String()
	if strings.Contains(err.Error(), buzzLedgerUnknownNote) {
		t.Errorf("#346: the terminal-status error disclaims a ledger the poll reply itemises.\n  got: %v", err)
	}
	if strings.Contains(err.Error(), "/user/transactions") {
		t.Errorf("#346: the terminal-status error still points at the account-wide history.\n  got: %v", err)
	}
	for _, want := range []string{"debit", "credit", "net"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("#346: `generate` did not render the %q from the workflow it polled.\nstderr:\n%s", want, stderr)
		}
	}
	// 🔴 STDOUT MUST STAY CLEAN. This branch returns an error and `--json` puts a
	// machine payload on stdout; a money block written there would corrupt it.
	if out.Len() != 0 {
		t.Errorf("the settlement block leaked onto stdout on the failure path: %q", out.String())
	}
}

// The counterpart on the generate path.
func TestGenerate_TerminalStatusKeepsTheDisclaimerWhenSilent(t *testing.T) {
	withStdinTTY(t, false)
	for _, st := range []string{genapi.StatusFailed, genapi.StatusExpired, genapi.StatusCanceled} {
		t.Run(st, func(t *testing.T) {
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
				t.Errorf("%s: a poll reply with no transaction record must keep the disclaimer.\n  got: %v", st, err)
			}
		})
	}
}
