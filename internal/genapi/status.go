package genapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// GetWorkflowPath is the read-back query for one workflow.
//
// 🔴 It is deliberately `getWorkflow` and NOT `statusUpdate`, even though
// statusUpdate returns a smaller projection that looks better suited to a poll.
// Two reasons, both measured against the platform source
// (civitai/civitai -> src/server/routers/orchestrator.router.ts):
//
//   - `statusUpdate` is an `orchestratorGuardedProcedure` (:593) while
//     `getWorkflow` is a plain `orchestratorProcedure` (:203). The guard rejects a
//     MUTED user — so polling through statusUpdate would leave a restricted user
//     unable to read a workflow they had ALREADY PAID FOR. Reading is not
//     spending; it must not be gated behind the spend guard.
//   - `statusUpdate` is `getWorkflow(...)` plus a projection that DROPS the
//     workflow `metadata` (:2854-2861), which is where per-output user state
//     (the `hidden` flag) lives.
//
// Neither procedure is cached: the "3s server-side feed cache" that an earlier
// design revision cited is `QUERIED_WORKFLOWS_CACHE_TTL`, which belongs to
// `queryGeneratedImages`, not to this route. `getWorkflow` proxies straight
// through to the orchestrator. See PollConfig for what follows from that.
const GetWorkflowPath = "/api/trpc/orchestrator.getWorkflow"

// The orchestrator's workflow statuses, verbatim from the generated client's
// WorkflowStatus enum (@civitai/client -> types.gen.d.ts). There is no "pending"
// — a workflow that has not started yet is `unassigned`, `preparing` or
// `scheduled`.
const (
	StatusUnassigned = "unassigned"
	StatusPreparing  = "preparing"
	StatusScheduled  = "scheduled"
	StatusProcessing = "processing"
	StatusSucceeded  = "succeeded"
	StatusFailed     = "failed"
	StatusExpired    = "expired"
	StatusCanceled   = "canceled"
)

// terminalStatuses are the four statuses a workflow never leaves.
var terminalStatuses = map[string]bool{
	StatusSucceeded: true,
	StatusFailed:    true,
	StatusExpired:   true,
	StatusCanceled:  true,
}

// IsTerminalStatus reports whether s is a status a workflow never leaves.
//
// It fails toward KEEPING WAITING: an unrecognised status is reported
// non-terminal, so a status the platform adds after this build makes the poll
// run to its --timeout and print the re-attach command, rather than declaring a
// result the CLI cannot actually interpret. A wrong "done" is much worse than a
// slow "still waiting" — the first reports a fabricated outcome for a job the
// user paid for.
func IsTerminalStatus(s string) bool {
	return terminalStatuses[strings.ToLower(strings.TrimSpace(s))]
}

// Blob is one orchestrator output blob as it appears on the RAW workflow.
//
// 🔴 The field set here is the RAW `Blob`/`ImageBlob` from the orchestrator
// client, NOT the site's `NormalizedWorkflowStepOutput`. `orchestrator.getWorkflow`
// returns the orchestrator's own workflow untouched (router :203-207 hands the
// client result straight back), while the normalized shape is only produced by
// `formatGenerationResponse2` on OTHER routes. Two consequences that are easy to
// get wrong, and that a normalized-shape reader would silently get zero values
// for:
//
//   - `hidden` is NOT a blob field. It lives in the STEP's metadata, keyed by
//     blob id (`metadata.output[<id>].hidden`, legacy `metadata.images[<id>]`).
//     See stepMetadata.
//   - `seed` is NOT a blob field either. It is `metadata.params.seed` plus the
//     blob's index within the step.
type Blob struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	// Available reports that the blob has landed and its URL is fetchable.
	Available bool `json:"available"`
	// URL is presigned and EXPIRES (see URLExpiresAt). It needs no bearer token
	// — and must not be sent one; see the download seam in internal/cmd.
	URL          *string `json:"url"`
	URLExpiresAt *string `json:"urlExpiresAt"`
	NSFWLevel    *string `json:"nsfwLevel"`
	// BlockedReason is set only when moderation blocked the blob.
	BlockedReason *string `json:"blockedReason"`
	Width         *int    `json:"width"`
	Height        *int    `json:"height"`
}

// stepOutput covers the output containers an image-producing step can use. The
// orchestrator's step output is a discriminated union keyed on the step's
// `$type`; rather than vendor the 51-engine table (this repo's anti-mirror
// rule), all three image-shaped containers are read and whichever is populated
// wins. `textToImage`/`imageGen` fill `images`, `comfy` fills `blobs`, and the
// upscaler/preprocess steps fill the single `blob`.
type stepOutput struct {
	Images []Blob `json:"images"`
	Blobs  []Blob `json:"blobs"`
	Blob   *Blob  `json:"blob"`
}

// stepMetadata is the per-step user metadata. It carries the two fields the raw
// blob does not: the per-output `hidden` flag and the job's seed.
type stepMetadata struct {
	// Output is the current per-blob metadata map, keyed by blob id.
	Output map[string]outputMeta `json:"output"`
	// Images is the LEGACY name for the same map (pre-rename workflows). The
	// site merges both with the new key winning per field; a workflow submitted
	// before the rename only has this one, so ignoring it would read `hidden`
	// as false for every output of an older workflow.
	Images map[string]outputMeta `json:"images"`
	Params struct {
		Seed *int64 `json:"seed"`
	} `json:"params"`
}

type outputMeta struct {
	Hidden bool `json:"hidden"`
}

// hiddenFor reports the effective `hidden` flag for a blob id, with the current
// key winning over the legacy one.
func (m stepMetadata) hiddenFor(id string) bool {
	if v, ok := m.Output[id]; ok {
		return v.Hidden
	}
	if v, ok := m.Images[id]; ok {
		return v.Hidden
	}
	return false
}

// Step is one workflow step.
type Step struct {
	Type     string       `json:"$type"`
	Name     string       `json:"name"`
	Status   string       `json:"status"`
	Metadata stepMetadata `json:"metadata"`
	Output   stepOutput   `json:"output"`
}

// blobs returns the step's outputs from whichever container the step populated.
func (s Step) blobs() []Blob {
	switch {
	case len(s.Output.Images) > 0:
		return s.Output.Images
	case len(s.Output.Blobs) > 0:
		return s.Output.Blobs
	case s.Output.Blob != nil:
		return []Blob{*s.Output.Blob}
	}
	return nil
}

// Workflow is the orchestrator workflow as `orchestrator.getWorkflow` returns
// it. Only the fields the CLI reports on are modelled; the raw payload travels
// alongside for `--json`.
type Workflow struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"createdAt"`
	StartedAt   *string `json:"startedAt"`
	CompletedAt *string `json:"completedAt"`
	Tags        []string
	Steps       []Step `json:"steps"`
	// Transactions is the orchestrator's own money record, passed through
	// untyped: its shape is orchestrator-owned and the CLI must not relabel it.
	Transactions json.RawMessage `json:"transactions,omitempty"`
	// Metadata carries the workflow-level record the SUBMIT persisted, which is
	// how a checkpoint substitution survives to be read back. `getWorkflow` is a
	// straight passthrough of the raw orchestrator workflow, so the key arrives
	// exactly as it was stored.
	//
	// 🔴 Read it via Substitutions(), not directly. This is the ONLY carrier once
	// the submit reply is gone — which is precisely the situation
	// `civitai generate --no-wait` creates by design.
	Metadata *substitutionMetadata `json:"metadata,omitempty"`
}

// Output is one workflow output with the two step-derived fields folded in.
type Output struct {
	Blob
	// StepName / StepStatus identify the producing step.
	StepName   string
	StepStatus string
	// Index is the blob's position within its step, which is what makes Seed
	// per-output rather than per-step.
	Index int
	// Hidden is the user's per-output "deleted" flag, read from the step
	// metadata (it is not on the blob).
	Hidden bool
	// Seed is the step's seed offset by Index, or nil when the step recorded
	// none.
	Seed *int64
}

// Outputs flattens every step's outputs, folding in the step-derived `hidden`
// and `seed` fields. It applies NO filtering — see Deliverable.
func (w *Workflow) Outputs() []Output {
	var out []Output
	for _, st := range w.Steps {
		blobs := st.blobs()
		for i, b := range blobs {
			o := Output{
				Blob:       b,
				StepName:   st.Name,
				StepStatus: st.Status,
				Index:      i,
				Hidden:     st.Metadata.hiddenFor(b.ID),
			}
			if s := st.Metadata.Params.Seed; s != nil {
				v := *s + int64(i)
				o.Seed = &v
			}
			out = append(out, o)
		}
	}
	return out
}

// Deliverable reports whether an output is a real, fetchable result.
//
// 🔴 This is the site's own predicate, verbatim:
//
//	output.filter((x) => x.available && !x.blockedReason && !x.hidden)
//
// (civitai/civitai -> src/shared/orchestrator/workflow-data.ts:222,
// `succeededOutput`.) A `succeeded` workflow can legitimately contain outputs
// that are none of those things: moderation-blocked, not-yet-landed (the worker
// finished but the blob upload did not), or user-hidden. A caller that skips
// this reports success and silently writes fewer files than --quantity asked
// for, with no signal — which is why the CLI reports the excluded ones by REASON
// rather than just downloading what is left.
func Deliverable(o Output) bool {
	return o.Available && !hasBlockedReason(o) && !o.Hidden
}

func hasBlockedReason(o Output) bool {
	return o.BlockedReason != nil && strings.TrimSpace(*o.BlockedReason) != ""
}

// ExclusionReason returns why an output is NOT deliverable, or "" when it is.
// The reasons are checked in the same order the predicate ANDs them, so an
// output failing several reports the first — the one closest to the cause.
func ExclusionReason(o Output) string {
	switch {
	case !o.Available:
		return "not available (the job finished without producing a usable file)"
	case hasBlockedReason(o):
		return "blocked by moderation: " + strings.TrimSpace(*o.BlockedReason)
	case o.Hidden:
		return "hidden (you deleted this result on the website)"
	}
	return ""
}

// PartitionOutputs splits a workflow's outputs into the deliverable ones and the
// excluded ones, so a caller can report BOTH rather than quietly shipping the
// remainder.
func PartitionOutputs(w *Workflow) (kept, excluded []Output) {
	for _, o := range w.Outputs() {
		if Deliverable(o) {
			kept = append(kept, o)
		} else {
			excluded = append(excluded, o)
		}
	}
	return kept, excluded
}

// GetWorkflow reads one workflow by id. It is a READ — it spends nothing.
//
// It returns the decoded workflow AND the raw unwrapped payload, so a `--json`
// surface can pass through server fields this struct does not model.
func (c *Client) GetWorkflow(ctx context.Context, workflowID string) (*Workflow, json.RawMessage, error) {
	id := strings.TrimSpace(workflowID)
	if id == "" {
		return nil, nil, fmt.Errorf("a workflow id is required")
	}
	inputJSON, err := json.Marshal(struct {
		JSON struct {
			WorkflowID string `json:"workflowId"`
		} `json:"json"`
	}{JSON: struct {
		WorkflowID string `json:"workflowId"`
	}{WorkflowID: id}})
	if err != nil {
		return nil, nil, err
	}
	q := url.Values{}
	q.Set("input", string(inputJSON))
	reqURL := c.BaseURL + GetWorkflowPath + "?" + q.Encode()

	build := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return nil, nil, err
	}
	if status != http.StatusOK {
		return nil, nil, generateError("getWorkflow", status, raw)
	}
	payload, err := unwrapTRPC("orchestrator.getWorkflow", raw)
	if err != nil {
		return nil, nil, err
	}
	var out Workflow
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, nil, fmt.Errorf("unexpected orchestrator.getWorkflow payload: %s", string(payload))
	}
	if out.ID == "" {
		// The orchestrator's id field is nullable; fall back to what was asked
		// for so downstream output never renders an empty handle.
		out.ID = id
	}
	return &out, payload, nil
}

// StatusOf returns an APIError's HTTP status, or 0 when err is not one. It backs
// the poll loop's 429/5xx retry decision without re-parsing message text.
func StatusOf(err error) int {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status
	}
	return 0
}
