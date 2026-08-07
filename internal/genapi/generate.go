package genapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// The two orchestrator generation-graph procedures.
//
// 🔴 They take DIFFERENT ENVELOPES for the SAME graph, and the mismatch is
// SILENT — it returns HTTP 200 with a plausible, wrong cost. This is the single
// most important fact in this package.
//
//	whatIfFromGraph   GET   ?input={"json": <graph>}              graph FLAT
//	generateFromGraph POST   {"json":{"input": <graph>, ...}}     graph NESTED
//
// The server destructures the graph out of a wrapper on the mutation
// (`const { input: formInput, ..., externalId } = input`) and takes it flat on
// the query (civitai/civitai -> src/server/routers/orchestrator.router.ts).
// The web mirrors the asymmetry deliberately.
//
// Measured against civitai.com: a NESTED payload sent to whatIf is never parsed
// at all — every nested body prices the default job (total 8) byte-identically
// to `{}`, while the same graph sent flat priced 60. The discriminating control
// was an ecosystem that 500s "Unknown ecosystem" when sent flat and returns a
// clean `200 ready:true` when sent nested. So a builder wired to the wrong
// nesting keeps every "did it 200?" assertion green while quoting a constant —
// which would let a `--max-cost` pre-flight wave through an arbitrarily
// expensive job. The envelope shapes are pinned by tests for this reason.
const (
	// WhatIfPath is the cost-estimate query. It spends nothing.
	WhatIfPath = "/api/trpc/orchestrator.whatIfFromGraph"
	// GeneratePath is the SUBMIT mutation. It SPENDS BUZZ.
	GeneratePath = "/api/trpc/orchestrator.generateFromGraph"
)

// WorkflowCost is the orchestrator's cost breakdown. Factors are multipliers
// (e.g. quantity/size/steps/scheduler/popularity) and Fixed are flat additions
// (e.g. additionalNetworks/priority/format); both are open maps because the key
// set is server-owned and must be echoed VERBATIM, never relabelled.
//
// Total is documented server-side as "including tips" — but the whatIf request
// prices a strictly smaller body than a submit (it sends no tips), so a caller
// that passes any tip cannot trust this total by arithmetic. This package does
// not send tips at all (see generateEnvelope).
type WorkflowCost struct {
	Base    float64            `json:"base"`
	Total   float64            `json:"total"`
	Factors map[string]float64 `json:"factors,omitempty"`
	Fixed   map[string]float64 `json:"fixed,omitempty"`
	Tips    map[string]float64 `json:"tips,omitempty"`
}

// WhatIfResult is the whatIfFromGraph reply.
//
// 🔴 It is an ESTIMATE, not a quote: it carries no id, nonce, signed price or
// expiry, so there is nothing to hand back to the submit. No server-side spend
// ceiling is reachable from a personal API key, and realized cost can exceed
// this figure with no refund. Never present it as a cap.
type WhatIfResult struct {
	// Ready is false when some job in the workflow has no available support
	// (i.e. the resources are not currently generatable).
	Ready              bool          `json:"ready"`
	Cost               *WorkflowCost `json:"cost"`
	AllowMatureContent *bool         `json:"allowMatureContent,omitempty"`
	// Transactions is passed through untyped; its shape is orchestrator-owned.
	Transactions json.RawMessage `json:"transactions,omitempty"`
	// ModelSubstitutions records checkpoints the server swapped out for this
	// graph. See substitution.go.
	//
	// 🔴 THIS IS THE MOST VALUABLE FIELD ON THIS REPLY. A whatIf spends nothing
	// and persists nothing, so it is the ONLY place a caller can discover — while
	// it can still back out — that the model it named is not the model that will
	// run. It is also the only signal for a SAME-PRICE substitution: comparing
	// costs only helps if you already knew the expected price, and a swap between
	// similarly-priced checkpoints is otherwise completely invisible.
	//
	// Nothing is persisted on this path, so unlike the submit there is no
	// metadata round-trip to recover it from later. Read it here or never.
	ModelSubstitutions []ModelSubstitution `json:"modelSubstitutions,omitempty"`
}

// SubmitResult is the generateFromGraph reply — one normalized workflow.
//
// ✅ VERIFIED against a live submit (one SDXL-default txt2img, quantity 1:
// estimated 8 Buzz, charged exactly 8). The field names below were originally
// derived by reading the server's `NormalizedWorkflow` interface
// (civitai/civitai -> src/server/services/orchestrator/
// orchestration-new.service.ts) and carried an UNVERIFIED warning, because
// exercising the mutation spends real money; that warning is now discharged.
//
// Only the fields a caller needs to poll and report are modelled; the raw
// payload is returned alongside so nothing is lost. Note this is the NORMALIZED
// shape — `orchestrator.getWorkflow` returns the RAW orchestrator workflow, a
// different shape again (see status.go). Do not share a struct between them.
type SubmitResult struct {
	// ID is the workflow id — the handle for polling and for re-attaching after
	// an interrupt.
	ID     string        `json:"id"`
	Status string        `json:"status"`
	Cost   *WorkflowCost `json:"cost"`
	Tags   []string      `json:"tags,omitempty"`
	// ModelSubstitutions is the TOP-LEVEL record of checkpoints the server
	// swapped out for this submit — the money has already moved by the time this
	// is read. See substitution.go.
	ModelSubstitutions []ModelSubstitution `json:"modelSubstitutions,omitempty"`
	// Metadata carries the SAME record a second time, under the key the server
	// persists onto the orchestrator workflow. Both are populated on this reply;
	// only this one survives to a later read.
	//
	// 🔴 Read neither directly — use Substitutions(), which fixes the precedence.
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// SubmitOptions carries the ENVELOPE siblings of the graph — the fields that
// live beside `input`, not inside it.
//
// 🔴 This struct is a WHITELIST and must stay one. The envelope also accepts
// `civitaiTip` / `creatorTip`, which are real money and are deliberately NOT
// modelled: if a caller ever splats a user-supplied graph file into the
// envelope's top level, a committed `graph.json` carrying `civitaiTip: 5000`
// would charge a tip that a cost pre-flight structurally cannot see.
type SubmitOptions struct {
	// ExternalID overrides the minted idempotency key. Empty means "mint one",
	// which is the normal path — see GenerateFromGraph.
	ExternalID string
	// Tags are orchestrator workflow tags.
	Tags []string
}

// whatIfEnvelope is the tRPC input wrapper for the QUERY: the graph sits FLAT
// under "json".
type whatIfEnvelope struct {
	JSON Graph `json:"json"`
}

// generateEnvelope is the tRPC input wrapper for the MUTATION: the graph is
// NESTED under "json"."input", beside the envelope-only siblings.
type generateEnvelope struct {
	JSON generateInput `json:"json"`
}

type generateInput struct {
	Input Graph `json:"input"`
	// ExternalID is the orchestrator idempotency key. It has no `omitempty` on
	// purpose: it is unconditional, and a silently-absent key is the exact
	// failure this field exists to prevent.
	ExternalID string   `json:"externalId"`
	Tags       []string `json:"tags,omitempty"`
}

// NewExternalID mints a RFC 4122 version-4 UUID for use as the orchestrator
// idempotency key.
//
// 🔴 One MUST be sent on every submit. The platform's submit wrapper retries
// 3x on a 5xx AND on a network error / no response, reusing the same body, and
// adds NO idempotency key of its own; there is no server-side minting fallback.
// The orchestrator dedupes on (userId, externalId), so without one a single
// user action can be charged up to three times. The web client is safe only
// because it always mints one.
//
// It uses crypto/rand + manual v4 formatting rather than a uuid dependency —
// this is a small, focused project and a new module needs maintainer approval.
func NewExternalID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("mint externalId: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// WhatIfFromGraph estimates the cost of g WITHOUT submitting it. It spends
// nothing (verified live: the Buzz balance was byte-identical before and after
// every probe).
//
// 🔴 It never sends an externalId. A whatIf carrying a matching key would
// return the pre-existing workflow, burning the key the subsequent submit needs
// — which is why the server's own whatIf body omits it.
//
// It returns the decoded result AND the raw unwrapped payload, so a `--json`
// surface can pass through server fields this struct does not model.
func (c *Client) WhatIfFromGraph(ctx context.Context, g Graph) (*WhatIfResult, json.RawMessage, error) {
	inputJSON, err := json.Marshal(whatIfEnvelope{JSON: whatIfGraph(g)})
	if err != nil {
		return nil, nil, err
	}
	q := url.Values{}
	q.Set("input", string(inputJSON))
	reqURL := c.BaseURL + WhatIfPath + "?" + q.Encode()

	build := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return nil, nil, err
	}
	if status != http.StatusOK {
		return nil, nil, generateError("whatIfFromGraph", status, raw)
	}
	payload, err := unwrapTRPC("orchestrator.whatIfFromGraph", raw)
	if err != nil {
		return nil, nil, err
	}
	var out WhatIfResult
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, nil, fmt.Errorf("unexpected orchestrator.whatIfFromGraph payload: %s", string(payload))
	}
	return &out, payload, nil
}

// GenerateFromGraph SUBMITS g. 🔴 THIS SPENDS BUZZ.
//
// An externalId is minted unconditionally when opts.ExternalID is empty, and is
// always present in the body (see NewExternalID for why). The caller gets it
// back so it can be recorded before/around the call and used to re-attach: a
// duplicate externalId returns HTTP 200 with the PRE-EXISTING workflow rather
// than a 409, so re-attachment has to be inferred locally.
//
// It returns the decoded result AND the raw unwrapped payload.
func (c *Client) GenerateFromGraph(ctx context.Context, g Graph, opts SubmitOptions) (*SubmitResult, string, json.RawMessage, error) {
	externalID := opts.ExternalID
	if externalID == "" {
		var err error
		if externalID, err = NewExternalID(); err != nil {
			return nil, "", nil, err
		}
	}
	body, err := json.Marshal(generateEnvelope{JSON: generateInput{
		Input:      g,
		ExternalID: externalID,
		Tags:       opts.Tags,
	}})
	if err != nil {
		return nil, externalID, nil, err
	}
	build := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+GeneratePath, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}
	status, raw, err := c.authedDoWith(ctx, c.submitClient(), build)
	if err != nil {
		return nil, externalID, nil, err
	}
	if status != http.StatusOK {
		return nil, externalID, nil, generateError("generateFromGraph", status, raw)
	}
	payload, err := unwrapTRPC("orchestrator.generateFromGraph", raw)
	if err != nil {
		return nil, externalID, nil, err
	}
	var out SubmitResult
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, externalID, nil, fmt.Errorf("unexpected orchestrator.generateFromGraph payload: %s", string(payload))
	}
	return &out, externalID, payload, nil
}

// unwrapTRPC extracts the payload from a tRPC success envelope
// {"result":{"data":{"json":{...}}}}.
//
// A literal `null` payload unmarshals CLEANLY into a zero struct, which for a
// cost estimate would render as "total 0" — a fabricated free quote. Reject it
// as malformed alongside a missing/undecodable envelope, exactly as the App
// Blocks analytics reader does.
func unwrapTRPC(proc string, raw []byte) (json.RawMessage, error) {
	var env struct {
		Result struct {
			Data struct {
				JSON json.RawMessage `json:"json"`
			} `json:"data"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil ||
		len(env.Result.Data.JSON) == 0 ||
		string(bytes.TrimSpace(env.Result.Data.JSON)) == "null" {
		return nil, fmt.Errorf("unexpected %s response: %s", proc, string(raw))
	}
	return env.Result.Data.JSON, nil
}
