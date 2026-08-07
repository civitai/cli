package genapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

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

	// Images are the img2img reference images. Their PRESENCE is what promotes
	// a `txt2img` workflow to `img2img:edit` server-side — the CLI never sends
	// an img2img workflow value itself (see AGENTS.md item 19(a)).
	//
	// 🔴 The promotion also requires Ecosystem to be set. `normalizeImageWorkflow`
	// reads `input.ecosystem` off the RAW request body, BEFORE the graph's own
	// ecosystem defaulting runs, so an absent ecosystem means no promotion — and
	// the default ecosystem's graph has no `images` node, so the key is then
	// dropped with zero diagnostics and a plain txt2img is billed. Measured
	// against civitai.com: `{workflow:txt2img, images:[…]}` with no ecosystem
	// priced 8 with factors {base,pixels,steps,quantity} — byte-identical to the
	// same graph with no images at all.
	Images []GraphImage `json:"images,omitempty"`

	// Raw, when non-nil, IS the graph: MarshalJSON emits it VERBATIM and every
	// typed field above is ignored. It is the `--input` passthrough.
	//
	// 🔴 It exists so a caller-supplied graph reaches the server UNCHANGED. The
	// whole point of a raw-graph surface is that the CLI carries no per-engine
	// code for the platform's ~51 ecosystem graphs; round-tripping such a file
	// through the typed struct above would silently DELETE every key this struct
	// does not model, which is worse than the server's own silent-drop behaviour
	// because it happens before the request is even sent.
	//
	// It is `json:"-"` so the typed path can never emit it as a field, and it is
	// deliberately NOT settable from a decoded server payload — nothing in this
	// package unmarshals into a Graph.
	Raw json.RawMessage `json:"-"`
}

// MarshalJSON emits Raw verbatim when it is set, and the typed fields otherwise.
//
// Raw is passed through json.Compact rather than returned as-is: Compact both
// VALIDATES it (encoding/json does not check what a MarshalJSON implementation
// returns beyond re-scanning it, and a malformed byte slice would corrupt the
// whole envelope) and normalises the whitespace of a hand-edited file.
//
// The `graphAlias` indirection is what stops this from recursing: a named type
// with the same fields has no methods, so json.Marshal falls back to the
// reflect-based encoder for it.
func (g Graph) MarshalJSON() ([]byte, error) {
	if g.Raw != nil {
		var buf bytes.Buffer
		if err := json.Compact(&buf, g.Raw); err != nil {
			return nil, fmt.Errorf("raw graph is not valid JSON: %w", err)
		}
		return buf.Bytes(), nil
	}
	type graphAlias Graph
	return json.Marshal(graphAlias(g))
}

// GraphImage is one entry of Graph.Images.
//
// 🔴 Width and Height are VALUE ints with NO `omitempty`, which contradicts
// item 14's "unset must be absent" rule on every other optional field. That is
// deliberate and is the opposite situation: the server REQUIRES both. The
// graph's images node accepts them as optional on INPUT and then validates the
// same array against an output schema where both are required numbers, so
// omitting them is a hard 400 — measured against civitai.com:
// `images:[{"url":"…"}]` returns
// `Validation failed: images: Invalid input: expected number, received undefined`,
// and a bare URL string (which the input union also accepts) fails identically
// because the input transform turns it into `{url}` with no dimensions.
//
// So there is no "unset" state to preserve here: a GraphImage that reached the
// wire without dimensions would be rejected, not silently mispriced. `omitempty`
// would additionally erase a legitimate value — it drops the key for 0, and a
// 0-dimension image is a caller bug that should surface as the server's own
// error rather than as a missing key.
type GraphImage struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
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
//
// A RAW graph (--input) is stripped the same way, by deleting the two keys from
// the decoded object rather than by re-marshalling a typed struct — every other
// key, including ones this package does not model, must survive untouched. A raw
// graph that does not decode as a JSON object is passed through unchanged: it is
// the server's business to reject it, and inventing a local parse error here
// would be a validation rule this CLI has no authority to enforce.
func whatIfGraph(g Graph) Graph {
	out := g
	if g.Raw != nil {
		out.Raw = stripPromptKeys(g.Raw)
		return out
	}
	out.Prompt = ""
	out.NegativePrompt = ""
	return out
}

// stripPromptKeys removes `prompt`/`negativePrompt` from a raw graph object,
// leaving every other key byte-identical. It returns the input unchanged when it
// is not a JSON object.
func stripPromptKeys(raw json.RawMessage) json.RawMessage {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return raw
	}
	delete(obj, "prompt")
	delete(obj, "negativePrompt")
	out, err := json.Marshal(obj)
	if err != nil {
		return raw
	}
	return out
}

// KnownGraphKeys returns the set of top-level JSON keys this CLI's Graph models.
//
// 🔴 It is derived by REFLECTION over the struct tags, never hand-listed. A
// second hand-maintained list would drift from the struct the moment a field is
// added, and the only consumer is a warning about keys the CLI does not
// recognise — a warning built from a stale list warns about the wrong things.
//
// This is emphatically NOT a mirror of the server's node registry, and must
// never become one: it answers "does the CLI model this key?", not "does the
// server accept it?". The server owns ~51 ecosystem graphs whose node sets this
// package deliberately does not vendor.
func KnownGraphKeys() map[string]bool {
	out := make(map[string]bool)
	t := reflect.TypeOf(Graph{})
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		out[name] = true
	}
	return out
}

// Ptr returns a pointer to v. Every optional Graph field is a pointer so that
// "unset" is absent from the wire (see the Graph doc comment); this is how a
// caller sets one from a literal.
func Ptr[T any](v T) *T { return &v }
