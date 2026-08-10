package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/civitai/cli/internal/genapi"
	"github.com/civitai/cli/pkg/civitai"
)

// #341 — `civitai workflows cancel <id-that-does-not-exist> --yes` printed
// "Cancelled workflow …", printed the settlement note, and exited 0.
//
// 🔴 WHY THE OLD BEHAVIOUR WAS NOT VISIBLE TO THE EXISTING TESTS. Every cancel
// test drove a stubbed mutation seam, and the mutation seam is exactly the thing
// that cannot tell a real id from a typo: `orchestrator.cancelWorkflow` answers
// HTTP 200 with an empty body either way. A test asserting "the seam was handed
// the id and success was printed" passes identically for both, so the bug lived
// under a green suite. What is asserted here is therefore not the seam's reply
// but the RELATIONSHIP between the read and the mutation: which one runs, in
// which order, and which outcomes of the read may stop the other.
//
// 🔴 cancelWorkflow is a MUTATION. Every case below runs against httptest or a
// stubbed seam; nothing here may reach a real server.

// notFoundRead is a read seam that answers exactly as the live server answers
// for an unknown workflow id: a tRPC 404, built by the real client so the error
// carries the real classification rather than a hand-made sentinel.
func notFoundRead(t *testing.T, calls *int) getWorkflowFn {
	t.Helper()
	err := apiErrorWithStatus(t, http.StatusNotFound)
	return func(_ context.Context, _ string) (*genapi.Workflow, json.RawMessage, error) {
		if calls != nil {
			*calls++
		}
		return nil, nil, err
	}
}

// countingCancel records how many times the mutation was issued.
func countingCancel(calls *int) func(context.Context, string) (json.RawMessage, error) {
	return func(_ context.Context, _ string) (json.RawMessage, error) {
		*calls++
		return json.RawMessage(`{"result":{"data":{}}}`), nil
	}
}

// TestWorkflowsCancel_UnknownIDRefusesAndIssuesNoMutation is the defect itself.
//
// The three assertions are separate claims and all three matter: the command
// must FAIL (not exit 0), it must not have told the user the job was stopped,
// and it must not have sent the cancel at all — which is what makes the error's
// "nothing was cancelled" a fact rather than a promise.
func TestWorkflowsCancel_UnknownIDRefusesAndIssuesNoMutation(t *testing.T) {
	cancels, reads := 0, 0
	c, out, errb := genCmd("")
	deps := workflowsCancelDeps{
		cancelWorkflow: countingCancel(&cancels),
		getWorkflow:    notFoundRead(t, &reads),
	}

	err := runWorkflowsCancel(c, deps, workflowsCancelOpts{assumeYes: true}, "not-a-real-workflow-zzz")
	if err == nil {
		t.Fatal("cancelling an id the server does not know reported SUCCESS — this is #341")
	}
	// Item 7: the exit code is pinned by errors.Is, never by message text.
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("want civitai.ErrNotFound (exit 4), got %v", err)
	}
	if cancels != 0 {
		t.Errorf("the cancel mutation was issued %d time(s) for an unknown id; it must not be issued at all", cancels)
	}
	if reads != 1 {
		t.Errorf("the existence read ran %d time(s), want exactly 1", reads)
	}
	if out.Len() != 0 {
		t.Errorf("stdout must stay empty on a refused cancel, got %q", out.String())
	}
	// 🔴 The reassuring half of the defect: the settlement note implied the
	// server had acted on the job. Neither it nor the success line may render.
	stderr := errb.String()
	for _, forbidden := range []string{"Cancelled", "cancelled workflow", "already delivered", "re-prices"} {
		if strings.Contains(strings.ToLower(stderr), strings.ToLower(forbidden)) {
			t.Errorf("a refused cancel printed %q, which claims the server acted:\n%s", forbidden, stderr)
		}
	}
	if !strings.Contains(err.Error(), "not-a-real-workflow-zzz") {
		t.Errorf("the error should name the id that was not found: %v", err)
	}
}

// TestWorkflowsCancel_RealWorkflowStillCancels is the MANDATORY CONTROL.
//
// 🔴 This is the shape of #256's near-miss, where a path-classification fix
// nearly broke the valid case. A guard in front of the only lever that stops a
// job burning Buzz is worth having only if it cannot hold that lever shut, so
// the case the guard must NOT touch is asserted at the same level of detail as
// the case it must catch.
func TestWorkflowsCancel_RealWorkflowStillCancels(t *testing.T) {
	cancels, reads := 0, 0
	c, out, errb := genCmd("")
	deps := workflowsCancelDeps{
		cancelWorkflow: countingCancel(&cancels),
		getWorkflow:    existingWorkflowRead(&reads),
	}

	if err := runWorkflowsCancel(c, deps, workflowsCancelOpts{assumeYes: true}, "wf_live"); err != nil {
		t.Fatalf("cancelling a real, cancellable workflow must still succeed: %v", err)
	}
	if cancels != 1 {
		t.Errorf("the cancel mutation was issued %d time(s), want exactly 1", cancels)
	}
	if reads != 1 {
		t.Errorf("the existence read ran %d time(s), want exactly 1", reads)
	}
	if !strings.Contains(out.String(), "Cancelled workflow wf_live") {
		t.Errorf("stdout does not confirm the cancel:\n%s", out.String())
	}
	if !strings.Contains(errb.String(), "billed for what it already delivered") {
		t.Errorf("the post-cancel billing note did not render:\n%s", errb.String())
	}
}

// TestWorkflowsCancel_NonNotFoundReadFailuresFailOpen pins the fail-open rule.
//
// 🔴 ONLY a 404 is evidence that the id is bad. Every other read outcome —
// a 5xx, a rate limit, an auth failure, a dead socket — is evidence about the
// READ, and must leave the cancel exactly as reachable as it was before this
// guard existed. A user whose job is spending Buzz right now must not be told
// "could not verify" while it keeps spending.
func TestWorkflowsCancel_NonNotFoundReadFailuresFailOpen(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"500 from the read", apiErrorWithStatus(t, http.StatusInternalServerError)},
		{"503 from the read", apiErrorWithStatus(t, http.StatusServiceUnavailable)},
		{"429 from the read", apiErrorWithStatus(t, http.StatusTooManyRequests)},
		{"401 from the read", apiErrorWithStatus(t, http.StatusUnauthorized)},
		{"403 from the read", apiErrorWithStatus(t, http.StatusForbidden)},
		{"400 from the read", apiErrorWithStatus(t, http.StatusBadRequest)},
		{"a transport error", errors.New("dial tcp: connection refused")},
		{"a cancelled context", context.Canceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cancels := 0
			readErr := tc.err
			c, out, _ := genCmd("")
			deps := workflowsCancelDeps{
				cancelWorkflow: countingCancel(&cancels),
				getWorkflow: func(context.Context, string) (*genapi.Workflow, json.RawMessage, error) {
					return nil, nil, readErr
				},
			}
			if err := runWorkflowsCancel(c, deps, workflowsCancelOpts{assumeYes: true}, "wf_live"); err != nil {
				t.Fatalf("a read failure that is not a 404 must not block the cancel, got: %v", err)
			}
			if cancels != 1 {
				t.Fatalf("the cancel was issued %d time(s); a failed read must not hold the lever shut", cancels)
			}
			if !strings.Contains(out.String(), "Cancelled workflow wf_live") {
				t.Errorf("stdout does not confirm the cancel:\n%s", out.String())
			}
		})
	}
}

// TestWorkflowsCancel_ReadErrorThatOnlyLOOKSLikeANotFound is the false-positive
// control on the discrimination. The refusal keys on the classification
// civitai.TagStatus attached to a real 404, NOT on words in the message — so a
// 400 whose text happens to say "not found" must still cancel.
func TestWorkflowsCancel_ReadErrorThatOnlyLOOKSLikeANotFound(t *testing.T) {
	cancels := 0
	c, out, _ := genCmd("")
	deps := workflowsCancelDeps{
		cancelWorkflow: countingCancel(&cancels),
		getWorkflow: func(context.Context, string) (*genapi.Workflow, json.RawMessage, error) {
			return nil, nil, errors.New("no such workflow: not found, 404")
		},
	}
	if err := runWorkflowsCancel(c, deps, workflowsCancelOpts{assumeYes: true}, "wf_live"); err != nil {
		t.Fatalf("an untagged error whose TEXT says 404 must not refuse the cancel: %v", err)
	}
	if cancels != 1 {
		t.Errorf("the cancel was issued %d time(s), want 1", cancels)
	}
	if !strings.Contains(out.String(), "Cancelled workflow wf_live") {
		t.Errorf("stdout does not confirm the cancel:\n%s", out.String())
	}
}

// TestWorkflowsCancel_UnknownIDClassifiesLikeWorkflowsGet pins the two commands
// TOGETHER rather than pinning a number twice.
//
// The reported control for #341 was that `workflows get <bogus>` exits 4 while
// `workflows cancel <bogus>` exited 0 — a disagreement between two commands
// about the same id. Asserting each one's sentinel separately would let them
// drift apart again; asserting that they AGREE is the property that was broken.
func TestWorkflowsCancel_UnknownIDClassifiesLikeWorkflowsGet(t *testing.T) {
	read := notFoundRead(t, nil)

	c1, _, _ := genCmd("")
	getErr := runWorkflowsGet(c1, workflowsGetDeps{getWorkflow: read}, workflowsGetOpts{}, "wf_gone")
	c2, _, _ := genCmd("")
	cancelErr := runWorkflowsCancel(c2, workflowsCancelDeps{
		cancelWorkflow: countingCancel(new(int)),
		getWorkflow:    read,
	}, workflowsCancelOpts{assumeYes: true}, "wf_gone")

	if getErr == nil || cancelErr == nil {
		t.Fatalf("CONTROL failure: both commands must fail on an unknown id (get=%v, cancel=%v)", getErr, cancelErr)
	}
	for _, tc := range []struct {
		sentinel error
		name     string
	}{
		{civitai.ErrNotFound, "civitai.ErrNotFound"},
		{civitai.ErrUnauthorized, "civitai.ErrUnauthorized"},
		{ErrUsage, "cmd.ErrUsage"},
	} {
		if got, want := errors.Is(cancelErr, tc.sentinel), errors.Is(getErr, tc.sentinel); got != want {
			t.Errorf("cancel and get disagree on %s for the same unknown id: cancel=%v, get=%v\n  cancel err: %v\n  get err:    %v",
				tc.name, got, want, cancelErr, getErr)
		}
	}
}

// TestWorkflowsCancel_TraversalIDIsRefused covers the second half of the report:
// `civitai workflows cancel "../../etc/passwd" --yes` answered "Cancelled".
// Nothing about the string makes it special — it is refused because the server
// does not know it, which is the same reason every typo is refused.
func TestWorkflowsCancel_TraversalIDIsRefused(t *testing.T) {
	cancels := 0
	c, out, _ := genCmd("")
	deps := workflowsCancelDeps{
		cancelWorkflow: countingCancel(&cancels),
		getWorkflow:    notFoundRead(t, nil),
	}
	err := runWorkflowsCancel(c, deps, workflowsCancelOpts{assumeYes: true}, "../../etc/passwd")
	if err == nil {
		t.Fatal(`cancelling "../../etc/passwd" reported success`)
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("want civitai.ErrNotFound (exit 4), got %v", err)
	}
	if cancels != 0 {
		t.Errorf("a mutation was issued for %q", "../../etc/passwd")
	}
	if out.Len() != 0 {
		t.Errorf("stdout must stay empty, got %q", out.String())
	}
}

// TestWorkflowsCancel_UnknownIDWithJSONEmitsNoSuccessDocument. `--json` prints
// `{}` on a successful cancel precisely so `jq` has something to parse; a
// refused cancel must not hand a script that same document, or every scripted
// caller reads a typo as a completed cancel.
func TestWorkflowsCancel_UnknownIDWithJSONEmitsNoSuccessDocument(t *testing.T) {
	cancels := 0
	c, out, _ := genCmd("")
	deps := workflowsCancelDeps{
		cancelWorkflow: countingCancel(&cancels),
		getWorkflow:    notFoundRead(t, nil),
	}
	err := runWorkflowsCancel(c, deps, workflowsCancelOpts{jsonOut: true, assumeYes: true}, "wf_gone")
	if err == nil {
		t.Fatal("--json on an unknown id reported success")
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("want civitai.ErrNotFound (exit 4), got %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("--json stdout must stay empty on a refused cancel, got %q", out.String())
	}
	// Positive control on the same options: the success path DOES emit a document.
	c2, out2, _ := genCmd("")
	if err := runWorkflowsCancel(c2, wfCancelDeps("", nil, nil, nil), workflowsCancelOpts{jsonOut: true, assumeYes: true}, "wf_live"); err != nil {
		t.Fatalf("control --json cancel: %v", err)
	}
	if out2.Len() == 0 {
		t.Error("CONTROL failure: the successful --json cancel emitted nothing, so the assertion above proves nothing")
	}
}

// TestWorkflowsCancel_MissingReadSeamRefuses. A nil read seam is unreachable
// from the command tree, but a nil seam that SKIPPED the check would silently
// restore #341 for whoever forgot to wire it — the "unreachable guard" failure
// mode. It refuses loudly instead, and issues no mutation.
func TestWorkflowsCancel_MissingReadSeamRefuses(t *testing.T) {
	cancels := 0
	c, out, _ := genCmd("")
	deps := workflowsCancelDeps{cancelWorkflow: countingCancel(&cancels)}
	err := runWorkflowsCancel(c, deps, workflowsCancelOpts{assumeYes: true}, "wf_live")
	if err == nil {
		t.Fatal("a cancel with no read seam must refuse rather than report success")
	}
	if cancels != 0 {
		t.Errorf("a mutation was issued with no way to check the id (%d calls)", cancels)
	}
	if out.Len() != 0 {
		t.Errorf("stdout must stay empty, got %q", out.String())
	}
}

// TestWorkflowsCancel_ConfirmationGateStillRunsFirst. The existence read is a
// network call, so it must not move ahead of the gate: a non-interactive shell
// without --yes still refuses offline, touching neither seam.
func TestWorkflowsCancel_ConfirmationGateStillRunsFirst(t *testing.T) {
	withStdinTTY(t, false)
	cancels, reads := 0, 0
	c, out, _ := genCmd("")
	deps := workflowsCancelDeps{
		cancelWorkflow: countingCancel(&cancels),
		getWorkflow:    existingWorkflowRead(&reads),
	}
	if err := runWorkflowsCancel(c, deps, workflowsCancelOpts{}, "wf_live"); err == nil {
		t.Fatal("a non-TTY cancel without --yes must be refused")
	}
	if reads != 0 || cancels != 0 {
		t.Errorf("the refusal touched the network (reads=%d, cancels=%d); the gate must run before either seam", reads, cancels)
	}
	if out.Len() != 0 {
		t.Errorf("stdout must stay empty, got %q", out.String())
	}
}

// --- end to end, through the real command tree -------------------------------

// cancelHTTPTest serves the two orchestrator procedures this command uses and
// records every path it was asked for.
//
// 🔴 The cancel handler answers 200 with an empty tRPC success — the live
// server's behaviour for ANY id, valid or not. That is the whole point: an
// end-to-end test whose fake 404'd the mutation would be testing a server that
// does not exist, and would pass with the guard deleted.
func cancelHTTPTest(t *testing.T, getStatus int) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case genapi.GetWorkflowPath:
			if getStatus != http.StatusOK {
				w.WriteHeader(getStatus)
				_, _ = w.Write([]byte(`{"error":{"json":{"message":"Not Found"}}}`))
				return
			}
			_, _ = fmt.Fprintf(w, `{"result":{"data":{"json":{"id":"wf_live","status":%q,"steps":[]}}}}`, genapi.StatusProcessing)
		case genapi.CancelWorkflowPath:
			_, _ = w.Write([]byte(`{"result":{"data":{}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}
}

// TestWorkflowsCancelCmd_UnknownIDEndToEnd is the reported reproduction, driven
// through NewRootCmd against a fake server.
//
// 🔴 IT EXISTS BECAUSE runWorkflowsCancel BEING CORRECT IS NOT THE CLAIM. The
// guard lives behind a seam that newWorkflowsCancelCmd has to wire; a fix that
// verified only the core would be green with the constructor still handing over
// a nil read seam, i.e. with the shipped binary unchanged. This is the case that
// fails if the wiring is dropped.
func TestWorkflowsCancelCmd_UnknownIDEndToEnd(t *testing.T) {
	srv, paths := cancelHTTPTest(t, http.StatusNotFound)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, _, err := run(t, "workflows", "cancel", "not-a-real-workflow-zzz", "--yes")
	if err == nil {
		t.Fatal("the shipped command reported success for an unknown workflow id — this is #341")
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("want civitai.ErrNotFound (exit 4), got %v", err)
	}
	if strings.Contains(out, "Cancelled") {
		t.Errorf("stdout claimed the workflow was cancelled:\n%s", out)
	}
	for _, p := range paths() {
		if p == genapi.CancelWorkflowPath {
			t.Errorf("the cancel mutation was sent for an id the server 404s; requests were %v", paths())
		}
	}
	if len(paths()) == 0 {
		t.Error("CONTROL failure: no request reached the server, so this case proves nothing about the wiring")
	}
}

// TestWorkflowsCancelCmd_RealWorkflowEndToEnd is the same wiring on the case
// that must keep working: the read answers, and the mutation is sent.
func TestWorkflowsCancelCmd_RealWorkflowEndToEnd(t *testing.T) {
	srv, paths := cancelHTTPTest(t, http.StatusOK)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, _, err := run(t, "workflows", "cancel", "wf_live", "--yes")
	if err != nil {
		t.Fatalf("cancelling a real workflow through the command tree: %v", err)
	}
	if !strings.Contains(out, "Cancelled workflow wf_live") {
		t.Errorf("stdout does not confirm the cancel:\n%s", out)
	}
	var sawGet, sawCancel bool
	for _, p := range paths() {
		sawGet = sawGet || p == genapi.GetWorkflowPath
		sawCancel = sawCancel || p == genapi.CancelWorkflowPath
	}
	if !sawGet {
		t.Errorf("the existence read never reached the server; requests were %v", paths())
	}
	if !sawCancel {
		t.Errorf("the cancel mutation never reached the server; requests were %v", paths())
	}
}
