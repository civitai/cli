package genapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// The two orchestrator workflow-collection procedures.
//
//	queryGeneratedImages GET  ?input={"json":{take,cursor,tags}}  READ, spends nothing
//	cancelWorkflow       POST  {"json":{"workflowId":"…"}}         MUTATION
//
// 🔴 cancelWorkflow is a MUTATION and must never be pointed at a live server
// from a test. It does not undo the charge — see CancelWorkflow.
const (
	// QueryWorkflowsPath is the paged list of the caller's own workflows.
	QueryWorkflowsPath = "/api/trpc/orchestrator.queryGeneratedImages"
	// CancelWorkflowPath cancels one workflow. It is a mutation.
	CancelWorkflowPath = "/api/trpc/orchestrator.cancelWorkflow"
)

// ListedWorkflow is one entry of the workflow feed.
//
// 🔴 It is the server's NORMALIZED workflow shape, which is NOT the raw
// orchestrator shape `GetWorkflow` returns. `queryGeneratedImages` runs its
// items through `formatGenerationResponse2` (civitai/civitai ->
// src/server/services/orchestrator/orchestration-new.service.ts) while
// `orchestrator.getWorkflow` hands the orchestrator's own workflow straight
// back. The two overlap on id/status/createdAt/cost but differ underneath, so
// this deliberately does not reuse the Workflow struct — a shared struct would
// read zero values for whichever fields the other shape names differently, and
// a fabricated zero is exactly the class of bug this package exists to avoid.
type ListedWorkflow struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	// CreatedAt is a Date server-side; superjson serialises it to an ISO string
	// inside the `json` payload, so it decodes as a string here.
	CreatedAt string        `json:"createdAt"`
	Cost      *WorkflowCost `json:"cost"`
	Tags      []string      `json:"tags,omitempty"`
	Steps     []ListedStep  `json:"steps"`
}

// ListedStep is one step of a normalized (listed) workflow.
type ListedStep struct {
	Type   string `json:"$type"`
	Name   string `json:"name"`
	Status string `json:"status"`
	// Metadata carries the per-output `hidden` flag, which is NOT a field of the
	// output itself in either shape.
	Metadata stepMetadata `json:"metadata"`
	// Output is the normalized output array.
	Output []Blob `json:"output"`
	// Images is the server's TEMPORARY legacy alias of Output, emitted during
	// the rename rollout and documented as always identical to it. It is read as
	// a fallback so a server that has finished the rollout and one that has not
	// both count correctly.
	Images []Blob `json:"images"`
}

// blobs returns the step's outputs from whichever array the server populated.
func (s ListedStep) blobs() []Blob {
	if len(s.Output) > 0 {
		return s.Output
	}
	return s.Images
}

// OutputCounts returns the total number of outputs and how many of them are
// DELIVERABLE, using the same predicate `workflows get` and the download path
// use (available && !blockedReason && !hidden).
//
// Reporting both is the point: a listing that showed only the total would
// present a workflow whose outputs were all moderation-blocked as a normal
// four-image result, and one that showed only the deliverable count would hide
// that anything had been produced and paid for at all.
func (w ListedWorkflow) OutputCounts() (total, deliverable int) {
	for _, st := range w.Steps {
		for i, b := range st.blobs() {
			total++
			o := Output{Blob: b, StepName: st.Name, StepStatus: st.Status, Index: i,
				Hidden: st.Metadata.hiddenFor(b.ID)}
			if Deliverable(o) {
				deliverable++
			}
		}
	}
	return total, deliverable
}

// WorkflowPage is one page of the workflow feed.
type WorkflowPage struct {
	Items []ListedWorkflow `json:"items"`
	// NextCursor is absent on the last page. It is a POINTER so "no more pages"
	// stays distinguishable from "an empty cursor string".
	NextCursor *string `json:"nextCursor"`
}

// ListOptions are the query parameters for QueryWorkflows.
type ListOptions struct {
	// Take is the page size. Zero omits the key and lets the server apply its
	// own default (20) rather than asking for zero workflows.
	Take int
	// Cursor is the opaque page cursor from a previous page's NextCursor.
	Cursor string
	// Tags filters the feed. Empty omits the key: the server's schema declares
	// `tags` with a default of [], and sending an explicit null would fail
	// validation rather than mean "no filter".
	Tags []string
}

// listInput is the tRPC input object. Every field is omitempty so an unset
// option is ABSENT from the wire and the server's own default applies — the
// same absence rule the generation graph follows, for the same reason.
type listInput struct {
	Take   int      `json:"take,omitempty"`
	Cursor string   `json:"cursor,omitempty"`
	Tags   []string `json:"tags,omitempty"`
}

// QueryWorkflows reads one page of the caller's own workflows. It is a READ and
// spends nothing.
//
// It returns the decoded page AND the raw unwrapped payload, so a `--json`
// surface can pass through server fields this struct does not model.
func (c *Client) QueryWorkflows(ctx context.Context, opts ListOptions) (*WorkflowPage, json.RawMessage, error) {
	inputJSON, err := json.Marshal(struct {
		JSON listInput `json:"json"`
	}{JSON: listInput{
		Take:   opts.Take,
		Cursor: strings.TrimSpace(opts.Cursor),
		Tags:   opts.Tags,
	}})
	if err != nil {
		return nil, nil, err
	}
	q := url.Values{}
	q.Set("input", string(inputJSON))
	reqURL := c.BaseURL + QueryWorkflowsPath + "?" + q.Encode()

	build := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return nil, nil, err
	}
	if status != http.StatusOK {
		return nil, nil, generateError("queryGeneratedImages", status, raw)
	}
	payload, err := unwrapTRPC("orchestrator.queryGeneratedImages", raw)
	if err != nil {
		return nil, nil, err
	}
	var out WorkflowPage
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, nil, fmt.Errorf("unexpected orchestrator.queryGeneratedImages payload: %s", string(payload))
	}
	return &out, payload, nil
}

// CancelWorkflow cancels one workflow. 🔴 IT IS A MUTATION, AND IT DOES NOT
// REFUND ANYTHING.
//
// The server-side handler resolves to `updateWorkflow({status:'canceled'})` and
// a mid-run cancel BILLS THE ACCRUED COST orchestrator-side. What the ledger
// does with it afterwards is not readable from this repo (AGENTS.md item 28).
// Cancelling is how you stop a job you no longer want the *output* of; it is
// never a way to get Buzz back, and no caller of this may present it as one.
//
// 🔴 It deliberately does NOT go through unwrapTRPC. The server procedure
// returns `undefined` (its service function awaits the orchestrator call and
// returns nothing), so a SUCCESSFUL reply carries no `result.data.json` at all —
// unwrapTRPC would reject the success case as a malformed response. HTTP 200 is
// therefore the success signal, and the raw body is returned for `--json`.
//
// It sends NO externalId, and must not: the orchestrator dedupes generation
// submits on (userId, externalId) and this is not a submit — but note that means
// the client-side retry safety the submit path relies on does not apply here.
// authedDo replays only on a 401, and a replayed cancel is idempotent by nature
// (the workflow is already canceled), so that is safe.
func (c *Client) CancelWorkflow(ctx context.Context, workflowID string) (json.RawMessage, error) {
	id := strings.TrimSpace(workflowID)
	if id == "" {
		return nil, fmt.Errorf("a workflow id is required")
	}
	body, err := json.Marshal(struct {
		JSON struct {
			WorkflowID string `json:"workflowId"`
		} `json:"json"`
	}{JSON: struct {
		WorkflowID string `json:"workflowId"`
	}{WorkflowID: id}})
	if err != nil {
		return nil, err
	}
	build := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+CancelWorkflowPath, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, generateError("cancelWorkflow", status, raw)
	}
	return raw, nil
}
