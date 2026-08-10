package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
	"github.com/civitai/cli/pkg/civitai"
)

// wfGetDeps wires a scripted reply into runWorkflowsGet.
func wfGetDeps(reply string, err error) workflowsGetDeps {
	return workflowsGetDeps{getWorkflow: func(ctx context.Context, id string) (*genapi.Workflow, json.RawMessage, error) {
		if err != nil {
			return nil, nil, err
		}
		var wf genapi.Workflow
		if uerr := json.Unmarshal([]byte(reply), &wf); uerr != nil {
			return nil, nil, uerr
		}
		return &wf, json.RawMessage(reply), nil
	}}
}

const wfGetPayload = `{
  "id":"wf_123","status":"succeeded","createdAt":"2026-08-05T12:00:00Z","completedAt":"2026-08-05T12:00:30Z",
  "steps":[{"$type":"textToImage","name":"s","status":"succeeded",
    "metadata":{"params":{"seed":42},"output":{"c":{"hidden":true}}},
    "output":{"images":[
      {"id":"a","type":"image","available":true,"url":"https://blobs.example/a.jpeg","urlExpiresAt":"2026-08-05T13:00:00Z","nsfwLevel":"pg","width":512,"height":768},
      {"id":"b","type":"image","available":true,"url":"https://blobs.example/b.jpeg","blockedReason":"minor"},
      {"id":"c","type":"image","available":true,"url":"https://blobs.example/c.jpeg"}
    ]}}]}`

func TestWorkflowsGet_Found(t *testing.T) {
	c, out, errb := genCmd("")
	if err := runWorkflowsGet(c, wfGetDeps(wfGetPayload, nil), workflowsGetOpts{baseURL: "https://civitai.com"}, "wf_123"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	stdout := out.String()
	for _, want := range []string{"wf_123", "succeeded", "512x768", "seed:    42", "https://blobs.example/a.jpeg", "2026-08-05T13:00:00Z"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
	// One deliverable, two excluded — and the excluded ones must be REPORTED,
	// not silently dropped.
	if !strings.Contains(stdout, "Outputs (1 deliverable, 2 excluded)") {
		t.Errorf("the deliverable/excluded counts are missing:\n%s", stdout)
	}
	stderr := errb.String()
	for _, want := range []string{"moderation", "minor", "hidden", "expire"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, stderr)
		}
	}
	// A blocked/hidden output's URL must not be presented as a result.
	if strings.Contains(stdout, "blobs.example/b.jpeg") || strings.Contains(stdout, "blobs.example/c.jpeg") {
		t.Errorf("an excluded output's URL was listed as a result:\n%s", stdout)
	}
}

func TestWorkflowsGet_NotFound(t *testing.T) {
	apiErr := apiErrorWithStatus(t, http.StatusNotFound)
	c, out, _ := genCmd("")
	err := runWorkflowsGet(c, wfGetDeps("", apiErr), workflowsGetOpts{}, "wf_missing")
	if err == nil {
		t.Fatal("a 404 must return an error")
	}
	// The exit code is pinned by errors.Is, never by the message text.
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("want civitai.ErrNotFound (exit 4), got %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("nothing should be printed on stdout for a 404: %q", out.String())
	}
}

// A 403 that is really a muted account must classify as ErrAccountRestricted, not
// as "log in again" — the same discrimination `generate` makes, via the same
// classifier.
func TestWorkflowsGet_RestrictedAccountIsNotAnAuthError(t *testing.T) {
	// Build the error from a REAL 403 reply carrying the muted-account wording,
	// so the classifier is exercised against the same *genapi.APIError
	// production produces.
	srv := trpcErrServer(t, http.StatusForbidden,
		"You cannot perform this action because your account has been restricted", "FORBIDDEN")
	_, _, apiErr := genapi.New(srv.URL, "tok").GetWorkflow(context.Background(), "wf_1")
	if apiErr == nil {
		t.Fatal("fixture: expected a 403 error")
	}

	c, _, _ := genCmd("")
	err := runWorkflowsGet(c, wfGetDeps("", apiErr), workflowsGetOpts{}, "wf_1")
	if !errors.Is(err, ErrAccountRestricted) {
		t.Fatalf("want ErrAccountRestricted, got %v", err)
	}
	if errors.Is(err, civitai.ErrUnauthorized) {
		t.Error("a restricted account must NOT read as an auth failure (exit 3) — `civitai login` will not help")
	}
}

func TestWorkflowsGet_JSONIsRawPassthrough(t *testing.T) {
	c, out, _ := genCmd("")
	if err := runWorkflowsGet(c, wfGetDeps(wfGetPayload, nil), workflowsGetOpts{jsonOut: true}, "wf_123"); err != nil {
		t.Fatalf("--json: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("--json stdout is not valid JSON: %v\n%s", err, out.String())
	}
	if decoded["id"] != "wf_123" {
		t.Errorf("--json id = %v", decoded["id"])
	}
	// Raw passthrough: a field the CLI's struct never models must survive, so a
	// script can branch on the server payload rather than on this CLI's view.
	steps, ok := decoded["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("--json lost the steps array: %v", decoded["steps"])
	}
	if strings.Contains(out.String(), "\x1b[") {
		t.Error("--json must emit no ANSI styling")
	}
}

func TestWorkflowsGet_EmptyIDIsAUsageError(t *testing.T) {
	c, _, _ := genCmd("")
	err := runWorkflowsGet(c, wfGetDeps(wfGetPayload, nil), workflowsGetOpts{}, "   ")
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("want ErrUsage (exit 2), got %v", err)
	}
}

// An unfinished workflow says so instead of rendering "0 outputs" as a result.
func TestWorkflowsGet_UnfinishedWorkflowSaysSo(t *testing.T) {
	c, out, errb := genCmd("")
	payload := `{"id":"wf_9","status":"processing","createdAt":"2026-08-05T12:00:00Z","steps":[]}`
	if err := runWorkflowsGet(c, wfGetDeps(payload, nil), workflowsGetOpts{}, "wf_9"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	if !strings.Contains(out.String(), "not finished") {
		t.Errorf("stdout should say the workflow has not finished:\n%s", out.String())
	}
	if !strings.Contains(errb.String(), "spends nothing") {
		t.Errorf("stderr should say re-checking is free:\n%s", errb.String())
	}
}

// ---------------------------------------------------------------------------
// #280 — the excluded-output report is a CHARGE claim, so it must only be
// printed for a workflow that actually reached a terminal state.
//
// 🔴 The fixture above (TestWorkflowsGet_UnfinishedWorkflowSaysSo) is
// STRUCTURALLY BLIND to this bug: its payload is `"steps":[]`, i.e. zero
// outputs, so it only ever exercises the "(none yet — the workflow has not
// finished)" branch and never reaches reportExcludedOutputs at all. Reproducing
// #280 needs BOTH conditions at once — a non-terminal status AND a
// non-deliverable output — or the contradicting line is unreachable.
// ---------------------------------------------------------------------------

// A running workflow whose blob has not landed yet: non-terminal status AND one
// non-available output.
const wfGetRunningWithPendingOutput = `{
  "id":"wf_280","status":"processing","createdAt":"2026-08-05T12:00:00Z",
  "steps":[{"$type":"textToImage","name":"s","status":"processing",
    "output":{"images":[
      {"id":"pending","type":"image","available":false}
    ]}}]}`

// The same shape after it finishes, plus one deliverable output so the
// presigned-URL note also fires and the ORDER of the two stderr blocks can be
// asserted.
const wfGetSucceededWithPendingOutput = `{
  "id":"wf_280t","status":"succeeded","createdAt":"2026-08-05T12:00:00Z","completedAt":"2026-08-05T12:00:30Z",
  "steps":[{"$type":"textToImage","name":"s","status":"succeeded",
    "output":{"images":[
      {"id":"ok","type":"image","available":true,"url":"https://blobs.example/ok.jpeg"},
      {"id":"pending","type":"image","available":false}
    ]}}]}`

// excludedOutputReport renders exactly what reportExcludedOutputs prints for
// payload's excluded outputs.
//
// The assertions below pin WHERE that block is printed, not how it is worded:
// the charge/refund sentence lives in generate_output.go and is being revised
// separately, so a literal copy of it here would be a spelling assertion that
// breaks on a rewording and — worse — would silently stop matching while the
// hazard remained. Deriving the block from the real function keeps the check
// structural. Two positive controls guard against the derivation going vacuous
// (`strings.Contains(x, "")` is always true): the fixture must actually have an
// excluded output, and the rendered block must be non-empty.
func excludedOutputReport(t *testing.T, payload string) string {
	t.Helper()
	var wf genapi.Workflow
	if err := json.Unmarshal([]byte(payload), &wf); err != nil {
		t.Fatalf("fixture does not decode: %v", err)
	}
	_, excluded := genapi.PartitionOutputs(&wf)
	if len(excluded) == 0 {
		t.Fatalf("fixture is blind to #280: it carries no excluded output, so the charge claim is unreachable")
	}
	var b bytes.Buffer
	// Both #280 fixtures carry no `transactions` record, so the surface they
	// render is the unsettled one. Deriving the block with settled=false keeps
	// these assertions matching what runWorkflowsGet prints for them; a fixture
	// that GAINED a record would make this derivation stop matching the real
	// stderr, which fails LOUD ("must still get the excluded-output report")
	// rather than passing silently.
	reportExcludedOutputs(&b, excluded, false)
	block := b.String()
	if strings.TrimSpace(block) == "" {
		t.Fatalf("positive control failed: reportExcludedOutputs printed nothing for %d excluded output(s)", len(excluded))
	}
	return block
}

// #280: a queued/running workflow must not be told it was charged and will not
// be refunded one line before being told it has not finished.
func TestWorkflowsGet_RunningWorkflowIsNotToldItWasChargedAndUnrefunded(t *testing.T) {
	chargeClaim := excludedOutputReport(t, wfGetRunningWithPendingOutput)

	c, out, errb := genCmd("")
	if err := runWorkflowsGet(c, wfGetDeps(wfGetRunningWithPendingOutput, nil), workflowsGetOpts{}, "wf_280"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	stderr := errb.String()

	idxNotFinished := strings.Index(stderr, "has not finished")
	if idxNotFinished < 0 {
		t.Fatalf("a non-terminal workflow must be told it has not finished; stderr:\n%s", stderr)
	}
	// The charge claim is a statement about a job that RAN. It must be absent
	// entirely — not merely printed later.
	if idx := strings.Index(stderr, chargeClaim); idx >= 0 {
		rel := "before"
		if idx > idxNotFinished {
			rel = "after"
		}
		t.Errorf("#280: a processing workflow was told it was charged and will not be refunded (index %d, %s "+
			"the \"has not finished\" line at index %d) — the excluded-output report must run only inside "+
			"the IsTerminalStatus guard.\nstderr:\n%s", idx, rel, idxNotFinished, stderr)
	}
	// State, not spelling: the report's own header must not appear either, so a
	// reworded final sentence cannot let the block through unnoticed.
	if strings.Contains(stderr, "will NOT be saved") {
		t.Errorf("#280: the excluded-output report was printed for a non-terminal workflow:\n%s", stderr)
	}
	// The counts themselves are honest at any status and must survive: they are
	// a count, not a claim about money.
	if !strings.Contains(out.String(), "Outputs (0 deliverable, 1 excluded)") {
		t.Errorf("the deliverable/excluded counts must still be printed:\n%s", out.String())
	}
}

// The counterpart, so the fix cannot silently delete a real warning: a TERMINAL
// workflow with a non-deliverable output still gets the excluded-output report,
// and it still comes before the presigned-URL note.
func TestWorkflowsGet_TerminalWorkflowStillReportsExcludedOutputs(t *testing.T) {
	chargeClaim := excludedOutputReport(t, wfGetSucceededWithPendingOutput)

	c, _, errb := genCmd("")
	if err := runWorkflowsGet(c, wfGetDeps(wfGetSucceededWithPendingOutput, nil), workflowsGetOpts{}, "wf_280t"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	stderr := errb.String()

	idxClaim := strings.Index(stderr, chargeClaim)
	if idxClaim < 0 {
		t.Fatalf("a terminal workflow with a non-deliverable output must still get the excluded-output "+
			"report; stderr:\n%s", stderr)
	}
	idxPresigned := strings.Index(stderr, "presigned and expire")
	if idxPresigned < 0 {
		t.Fatalf("the presigned-URL note must still print when a deliverable output exists; stderr:\n%s", stderr)
	}
	if idxClaim > idxPresigned {
		t.Errorf("the excluded-output report (index %d) must precede the presigned-URL note (index %d):\n%s",
			idxClaim, idxPresigned, stderr)
	}
	if strings.Contains(stderr, "has not finished") {
		t.Errorf("a succeeded workflow must not be told it has not finished:\n%s", stderr)
	}
}

// The command group must reject an unknown subcommand as a usage error rather
// than printing help and exiting 0 (root.go's enforceUsageExitCodes only covers
// non-runnable parents — this pins that `workflows` stays one).
func TestWorkflowsGroup_UnknownSubcommandIsAUsageError(t *testing.T) {
	_, _, err := run(t, "workflows", "frobnicate")
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("unknown subcommand: want ErrUsage (exit 2), got %v", err)
	}
}
