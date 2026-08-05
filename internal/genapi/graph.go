package genapi

// Graph is the generation-graph payload — the ONE payload struct both
// orchestrator procedures carry. The two procedures wrap it in DIFFERENT
// envelopes (see generate.go); the graph itself is identical.
//
// 🔴 EVERY optional field is a pointer or `omitempty`, so an unset field is
// ABSENT from the marshalled JSON rather than a Go zero value. This is not
// style — the server is permissive, not a validator:
//
//   - `steps: 0` is ACCEPTED and prices a degenerate, cheaper, wrong job
//     (measured: the steps cost factor drops to 0.333 from 1).
//   - `quantity: 0` / out-of-range values are CLAMPED silently, with no error.
//   - `cfgScale: 0` is accepted the same way.
//
// A value-typed field would therefore turn "the user did not pass --steps"
// into "the user asked for a broken job", HTTP 200, billed. Pointers also keep
// "unset" distinguishable from "explicitly zero", which a later `--input`
// round-trip needs.
//
// Field names track the server's declared graph nodes EXACTLY
// (civitai/civitai -> src/shared/data-graph/generation/). An UNDECLARED key is
// silently DROPPED server-side (`_validate` copies only declared node keys), so
// a typo here is a no-op that reports success — never add a key without
// checking it against the graph definitions.
type Graph struct {
	// Workflow is the generation mode, e.g. "txt2img".
	Workflow string `json:"workflow,omitempty"`
	// Ecosystem selects the model family, e.g. "SDXL". It is OMITTABLE, but its
	// default is NOT a stable fact: the server resolves it against the caller's
	// usable (non-disabled, non-memberOnly) ecosystems, so a free-tier account
	// and a member account can resolve to different models at different prices.
	// Callers must report which ecosystem the server actually used.
	Ecosystem string `json:"ecosystem,omitempty"`
	// Prompt is the positive prompt. It is STRIPPED from whatIf calls (see
	// whatIfGraph) — it does not affect cost, and the server defaults it.
	Prompt string `json:"prompt,omitempty"`
	// NegativePrompt is stripped from whatIf calls for the same reason.
	NegativePrompt string `json:"negativePrompt,omitempty"`
	// Quantity is the number of outputs. The server CLAMPS out-of-range values
	// (measured: 10000 -> 10, -5 -> 1) without an error, so the effective value
	// must be read back from the response rather than assumed.
	Quantity *int `json:"quantity,omitempty"`
	// AspectRatio is a bucket string, e.g. "1:1"; width/height derive from it.
	AspectRatio string `json:"aspectRatio,omitempty"`
	// Model is the checkpoint. Unlike Resources, this node coerces a bare id,
	// so {id} alone is accepted. A NONEXISTENT id is still accepted with HTTP
	// 200 and the ecosystem default silently substituted — resolve it first
	// (see ResolveModelVersion).
	Model *Resource `json:"model,omitempty"`
	// Resources are the additional networks (LoRAs). 🔴 Each entry REQUIRES
	// `model:{type}`: a bare id, and `{id}` alone, are both rejected with
	// 400 "expected object, received undefined". The type comes from
	// ResolveModelVersion.
	Resources []Resource `json:"resources,omitempty"`
	// Steps, CfgScale, Sampler and Seed are the silently-degrading parameters
	// (see the type comment). They are carried here so the transport is
	// complete and the absence rule is pinned; no flag sets them yet.
	Steps    *int     `json:"steps,omitempty"`
	CfgScale *float64 `json:"cfgScale,omitempty"`
	Sampler  string   `json:"sampler,omitempty"`
	Seed     *int     `json:"seed,omitempty"`
}

// Resource is one graph resource reference (a checkpoint or an additional
// network).
type Resource struct {
	ID int `json:"id"`
	// Model carries the resource's model TYPE. It is REQUIRED inside
	// Graph.Resources and optional on Graph.Model. A WRONG type is silently
	// accepted (a LoRA sent as {type:"Checkpoint"} returns 200), so it must come
	// from a live lookup, never a guess.
	Model *ResourceModel `json:"model,omitempty"`
	// Strength is the additional-network weight; unset means the server default.
	Strength *float64 `json:"strength,omitempty"`
}

// ResourceModel is the `model` sub-object of a graph resource.
type ResourceModel struct {
	Type string `json:"type"`
}

// whatIfGraph returns a COPY of g suitable for a cost estimate: the prompt
// fields are removed.
//
// The web deliberately strips them ("they don't affect cost and shouldn't be
// sent to the server until actual submission"), and the server's whatIf path
// substitutes its own defaults for exactly this case — `whatIfFromGraph`
// spreads the caller's input over its own defaults (prompt "cost-estimation",
// negativePrompt empty) before
// validating (civitai/civitai ->
// src/server/services/orchestrator/orchestration-new.service.ts). So dropping
// them cannot change the quote, and it stops shipping the user's prompt on
// every estimate.
//
// It returns a copy: the caller's Graph must be unchanged, because the SAME
// Graph is submitted afterwards and it needs its prompt.
func whatIfGraph(g Graph) Graph {
	out := g
	out.Prompt = ""
	out.NegativePrompt = ""
	return out
}

// Ptr returns a pointer to v. Every optional Graph field is a pointer so that
// "unset" is absent from the wire (see the Graph doc comment); this is how a
// caller sets one from a literal.
func Ptr[T any](v T) *T { return &v }
