package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
	"github.com/civitai/cli/pkg/civitai"
)

// 🔴 cancelWorkflow is a MUTATION. Every case here runs against httptest or a
// stubbed seam; nothing in this file may reach a real server.

// wfCancelDeps wires a scripted reply into runWorkflowsCancel and records what
// the seam was asked to cancel.
func wfCancelDeps(reply string, err error, seenID *string, calls *int) workflowsCancelDeps {
	return workflowsCancelDeps{cancelWorkflow: func(ctx context.Context, id string) (json.RawMessage, error) {
		if calls != nil {
			*calls++
		}
		if seenID != nil {
			*seenID = id
		}
		if err != nil {
			return nil, err
		}
		return json.RawMessage(reply), nil
	}}
}

func TestWorkflowsCancel_Success(t *testing.T) {
	var seen string
	c, out, errb := genCmd("")
	if err := runWorkflowsCancel(c, wfCancelDeps(`{"result":{"data":{}}}`, nil, &seen, nil), workflowsCancelOpts{assumeYes: true}, "  wf_1 "); err != nil {
		t.Fatalf("workflows cancel: %v", err)
	}
	if seen != "wf_1" {
		t.Errorf("the seam was handed %q, want the trimmed id", seen)
	}
	if !strings.Contains(out.String(), "wf_1") {
		t.Errorf("stdout does not confirm the cancelled id:\n%s", out.String())
	}
	// 🔴 The billing truth must be stated at the point of use, not only in
	// --help. #307 read the orchestrator and CHANGED what that truth is: the
	// delivered half is billed, the undelivered half is re-priced off, and the
	// settlement is server-side. So this asserts the half that IS certain (you
	// are billed for what was delivered) and still refuses any promise about an
	// amount, which the CLI cannot observe (AGENTS.md item 28).
	stderr := errb.String()
	if !strings.Contains(stderr, "billed for what it already delivered") {
		t.Errorf("stderr does not say the delivered work is billed:\n%s", stderr)
	}
	if !strings.Contains(stderr, "re-prices the rest") {
		t.Errorf("stderr does not say the rest is re-priced server-side:\n%s", stderr)
	}
	if !strings.Contains(stderr, buzzLedgerUnknownNote) {
		t.Errorf("stderr does not render the shared non-observability note:\n%s", stderr)
	}
	for _, forbidden := range []string{"you will not be charged", "fully refunded", "free"} {
		if strings.Contains(strings.ToLower(stderr), strings.ToLower(forbidden)) {
			t.Errorf("stderr promises an amount the CLI cannot observe (%q):\n%s", forbidden, stderr)
		}
	}
	// 🔴 THE RETRACTED SENTENCES, PINNED SO THEY CANNOT RETURN. This is NOT the
	// guard on this copy — the golden in generate_golden_copy_test.go is, because
	// a phrase list loses to the next paraphrase (item 28's two dead guards). It
	// is the item-3-style retraction check: these exact wordings were measured
	// FALSE against the orchestrator source in #307, so a re-introduction is a
	// revert rather than a rewording and deserves to name itself.
	assertNoRetractedCancelClaims(t, "the post-cancel note", stderr)
}

// retractedCancelClaims are the sentences #307 disproved. Each asserted that a
// cancel returns NOTHING; the orchestrator re-prices a cancelled workflow to the
// work it actually did and refunds the difference
// (`civitai/civitai-orchestration` → `WorkflowStepManager.CalculateCostAsync`'s
// undelivered-fraction subtraction, settled by
// `WorkflowGrain.EnsureCorrectBuzzChargedAsync(true)` on the final workflow
// event). See runWorkflowsCancel for the full chain.
var retractedCancelClaims = []string{
	"does not undo the charge",
	"did not undo the charge",
	"does not undo its cost",
	"bills the accrued cost",
	"bills the cost already accrued",
	"does not call that back",
	"is not a way to save money",
	"does not undo the buzz already spent",
	"already paid for",
	"non-refundab",
}

// assertNoRetractedCancelClaims fails when a cancel surface has grown back one of
// the wordings #307 retracted.
func assertNoRetractedCancelClaims(t *testing.T, surface, got string) {
	t.Helper()
	if strings.TrimSpace(got) == "" {
		t.Fatalf("CONTROL failure, not a finding: %s rendered nothing, so this scan reads no copy at all", surface)
	}
	lower := strings.ToLower(strings.Join(strings.Fields(got), " "))
	for _, claim := range retractedCancelClaims {
		if strings.Contains(lower, claim) {
			t.Errorf("%s carries the RETRACTED claim %q. It asserts that a cancel returns nothing, which #307 measured "+
				"false against the orchestrator source: a cancelled workflow is re-priced to the work it actually did and "+
				"the difference is refunded. Say what is billed, not what is lost.\n%s", surface, claim, got)
		}
	}
}

// 🔴 The help text is part of the money contract. It is asserted structurally
// rather than by eyeball, because this is precisely the sentence a later edit
// would "tidy".
//
// 🔴 WHAT IT ASSERTS WAS INVERTED BY #307. It used to require "is not a way to
// save money", "bills the accrued cost" and "does not call that back" — three
// wordings the orchestrator source contradicts, held in place by this test. A
// test that pins a false claim is worse than no test: it is why the claim
// survived two rounds of copy review. What survives is the half that is still
// true — delivered work is billed — plus the re-pricing that replaces the rest.
func TestWorkflowsCancel_HelpStatesTheBillingTruth(t *testing.T) {
	// Normalised: the claim is about the SENTENCE, not about where the help text
	// happens to wrap or which case it shouts in. A guard that broke on a re-wrap
	// would be deleted the first time someone reflowed the paragraph.
	long := strings.ToLower(strings.Join(strings.Fields(newWorkflowsCancelCmd().Long), " "))
	for _, want := range []string{
		"charged up front",
		"re-prices the workflow",
		"already delivered is billed",
		"cannot see from here",
	} {
		if !strings.Contains(long, want) {
			t.Errorf("`workflows cancel --help` is missing %q:\n%s", want, long)
		}
	}
	assertNoRetractedCancelClaims(t, "`workflows cancel --help`", newWorkflowsCancelCmd().Long)
	assertNoRetractedCancelClaims(t, "the `workflows` group --help", newWorkflowsCmd().Long)
	if !strings.Contains(newWorkflowsCancelCmd().Short, "billed for what it delivered") {
		t.Errorf("the one-line summary must carry the billing half too: %q", newWorkflowsCancelCmd().Short)
	}
}

// TestRetractedCancelClaimScannerCanFire is the POSITIVE CONTROL on the
// retraction scanner. Without it, a typo in a pattern, a normalisation that
// lower-cases nothing, or a scan wired to an empty string reports a serene pass
// over every surface — the "reassuring zero" this repo has now been bitten by
// repeatedly.
//
// It runs the REAL predicate over the exact sentences that shipped before #307,
// each of which MUST be rejected, and over the current copy, which MUST be
// accepted. The rejected corpus is built from the pre-fix strings, not from the
// pattern list, so a pattern that has stopped matching its own target fails here.
func TestRetractedCancelClaimScannerCanFire(t *testing.T) {
	rejects := func(s string) bool {
		lower := strings.ToLower(strings.Join(strings.Fields(s), " "))
		for _, claim := range retractedCancelClaims {
			if strings.Contains(lower, claim) {
				return true
			}
		}
		return false
	}

	// Verbatim from the pre-#307 tree.
	mustReject := map[string]string{
		"the old Short":            "Cancel a running generation workflow (DOES NOT UNDO THE CHARGE)",
		"the old --help lead":      "🔴 CANCELLING IS NOT A WAY TO SAVE MONEY. A mid-run cancel BILLS THE ACCRUED COST, orchestrator-side: by the time a workflow is running the money has already moved, and stopping it does not call that back.",
		"the old prompt warning":   "This does NOT undo the charge — a mid-run cancel bills the cost already accrued.",
		"the old prompt deterrent": "You are throwing away a job you have already paid for. It cannot be un-cancelled.",
		"the old post-cancel note": "This did NOT undo the charge — a mid-run cancel bills the cost already accrued.",
		"the old non-TTY refusal":  "cancelling wf_1 is irreversible and does not undo the Buzz already spent on it.",
		"the old group help":       "`cancel` stops a job but does not undo its cost — a mid-run cancel bills what has already accrued.",
		"an older phrasing still":  "a mid-run cancel is non-refundable",
	}
	for name, text := range mustReject {
		if !rejects(text) {
			t.Errorf("CONTROL failure: the scanner accepts %s, which is one of the sentences #307 retracted:\n  %s", name, text)
		}
	}
	// And it must ACCEPT what ships now — a scanner that rejects everything would
	// be satisfied by deleting the copy.
	for name, text := range map[string]string{
		"the cancel --help":      newWorkflowsCancelCmd().Long,
		"the cancel Short":       newWorkflowsCancelCmd().Short,
		"the workflows --help":   newWorkflowsCmd().Long,
		"the shared ledger note": buzzLedgerUnknownNote,
	} {
		if rejects(text) {
			t.Errorf("CONTROL failure: the scanner rejects %s, which is the copy it is written to accept:\n  %s", name, text)
		}
	}
}

func TestWorkflowsCancel_NotFound(t *testing.T) {
	apiErr := apiErrorWithStatus(t, http.StatusNotFound)
	c, out, _ := genCmd("")
	err := runWorkflowsCancel(c, wfCancelDeps("", apiErr, nil, nil), workflowsCancelOpts{assumeYes: true}, "wf_gone")
	if err == nil {
		t.Fatal("a 404 must return an error")
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("want civitai.ErrNotFound (exit 4), got %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("nothing should be printed on stdout for a failed cancel: %q", out.String())
	}
}

func TestWorkflowsCancel_EmptyIDIsAUsageError(t *testing.T) {
	calls := 0
	c, _, _ := genCmd("")
	err := runWorkflowsCancel(c, wfCancelDeps("", nil, nil, &calls), workflowsCancelOpts{assumeYes: true}, "   ")
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("want ErrUsage (exit 2), got %v", err)
	}
	if calls != 0 {
		t.Errorf("a mutation was issued for an empty id (%d calls)", calls)
	}
	// Positive control: the same seam IS reached for a real id.
	if err := runWorkflowsCancel(c, wfCancelDeps(`{}`, nil, nil, &calls), workflowsCancelOpts{assumeYes: true}, "wf_1"); err != nil {
		t.Fatalf("control cancel: %v", err)
	}
	if calls != 1 {
		t.Errorf("control: seam calls = %d, want 1", calls)
	}
}

// --json must emit a parseable document even though the server returns nothing
// on success — an empty stdout is a hard error for `jq`.
func TestWorkflowsCancel_JSONEmitsAParseableDocument(t *testing.T) {
	c, out, _ := genCmd("")
	if err := runWorkflowsCancel(c, wfCancelDeps("", nil, nil, nil), workflowsCancelOpts{jsonOut: true, assumeYes: true}, "wf_1"); err != nil {
		t.Fatalf("--json: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("--json stdout is not valid JSON: %v\n%q", err, out.String())
	}
}

// End-to-end over httptest: the real client, the real envelope, no live server.
func TestWorkflowsCancel_AgainstHTTPTest(t *testing.T) {
	var method, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"data":{}}}`))
	}))
	defer srv.Close()

	gen := genapi.New(srv.URL, "test-key")
	c, out, _ := genCmd("")
	if err := runWorkflowsCancel(c, workflowsCancelDeps{cancelWorkflow: gen.CancelWorkflow}, workflowsCancelOpts{assumeYes: true}, "wf_1"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if method != http.MethodPost || path != genapi.CancelWorkflowPath {
		t.Errorf("request = %s %s, want POST %s", method, path, genapi.CancelWorkflowPath)
	}
	if !strings.Contains(out.String(), "wf_1") {
		t.Errorf("stdout = %q", out.String())
	}
}

// The command group must expose all three verbs, and `workflows` itself must
// stay NON-runnable: root.go only installs the unknown-subcommand guard on
// parents that define no Run/RunE, so making it runnable would turn
// `civitai workflows frobnicate` into a silent exit 0.
func TestWorkflowsGroup_HasListGetCancelAndIsNotRunnable(t *testing.T) {
	g := newWorkflowsCmd()
	if g.Runnable() {
		t.Error("the `workflows` group must not be runnable")
	}
	want := map[string]bool{"list": false, "get": false, "cancel": false}
	for _, sub := range g.Commands() {
		want[sub.Name()] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("`civitai workflows %s` is not registered", name)
		}
	}
}
