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
	// --help. What is TRUE is that the accrued cost has been billed and stopping
	// the job does not call it back; what nobody has evidenced is that nothing
	// ever comes back, so this asserts the first and refuses the second (#278,
	// AGENTS.md item 28 — civitai/cli#307 owns the substance).
	stderr := errb.String()
	if !strings.Contains(stderr, "did NOT undo the charge") {
		t.Errorf("stderr does not say the accrued cost still stands:\n%s", stderr)
	}
	if !strings.Contains(stderr, buzzLedgerUnknownNote) {
		t.Errorf("stderr does not render the shared non-observability note:\n%s", stderr)
	}
	for _, forbidden := range []string{"saved", "refund" + "ed you", "you will not be charged"} {
		if strings.Contains(strings.ToLower(stderr), strings.ToLower(forbidden)) {
			t.Errorf("stderr implies cancelling saves money (%q):\n%s", forbidden, stderr)
		}
	}
}

// 🔴 The help text is part of the money contract: `cancel` must never read as a
// way to stop paying. It is asserted structurally rather than by eyeball,
// because this is precisely the sentence a later edit would "tidy".
func TestWorkflowsCancel_HelpStatesTheBillingTruth(t *testing.T) {
	// Normalised: the claim is about the SENTENCE, not about where the help text
	// happens to wrap or which case it shouts in. A guard that broke on a re-wrap
	// would be deleted the first time someone reflowed the paragraph.
	long := strings.ToLower(strings.Join(strings.Fields(newWorkflowsCancelCmd().Long), " "))
	// "does not get your buzz back" and "non-refundab" were pinned here and are
	// deliberately gone: both assert that NOTHING comes back, which no source in
	// this repo establishes. The accrued-cost half is what survives.
	for _, want := range []string{"is not a way to save money", "bills the accrued cost", "does not call that back"} {
		if !strings.Contains(long, want) {
			t.Errorf("`workflows cancel --help` is missing %q:\n%s", want, long)
		}
	}
	if !strings.Contains(newWorkflowsCancelCmd().Short, "DOES NOT UNDO THE CHARGE") {
		t.Errorf("the one-line summary must carry the warning too: %q", newWorkflowsCancelCmd().Short)
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
