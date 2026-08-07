package genapi

import "encoding/json"

// Model substitution — the server's record of a checkpoint it swapped out.
//
// 🔴 THE FAILURE THIS EXISTS TO MAKE VISIBLE. When a submitted `model.id` is not
// valid for a `modelLocked` ecosystem, the server does NOT reject it: it
// substitutes that ecosystem's default checkpoint, runs the job, and bills for
// what actually ran. The reply was byte-indistinguishable from success — a
// nonexistent version id came back HTTP 200 at the default price, and the caller
// had no way to learn it got a different model than it named
// (civitai/civitai#3665).
//
// The server now records each swap. This package models that record so
// `civitai generate` can say so before the money moves.
//
// 🔴 ABSENCE IS AMBIGUOUS AND MUST NEVER BE REPORTED AS ASSURANCE. The key is
// OMITTED when nothing was substituted, so an empty slice means EITHER "no
// substitution happened" OR "this server predates the field". The two are
// indistinguishable over the wire. Nothing in this CLI may render absence as
// "no substitutions" — see reportModelSubstitutions, which prints nothing at
// all when the record is empty.
//
// 🔴 WHY A PLAIN SLICE AND NOT A POINTER, given item 9's `AppAnalytics.Views`.
// Item 9 uses a pointer because that field has THREE reachable states —
// measured, explicitly unavailable, and absent — and a value type would collapse
// "absent" into "measured zero". This field has only TWO: the server attaches
// the key ONLY when there is at least one record (`modelSubstitutions?.length`
// guards every attach site), so an EMPTY array is never sent. `nil` therefore
// already means exactly one thing on the wire — "no record was carried" — and a
// `*[]ModelSubstitution` would add a third state that no server can produce.
// The ambiguity item 9 warns about is real here, but it is INHERENT to the
// protocol (no-substitution and old-server are the same bytes) and no Go type
// can resolve it. It is resolved the only way it can be: by never claiming the
// negative. If the server ever starts sending `[]` to mean "checked, none", this
// decision must be revisited — that would be the third state appearing.

// The three reasons the server records, as of civitai/civitai#3692. They are
// server-owned and bounded (the server narrows them at runtime against its own
// union before they ever reach a wire), which is why they are safe to switch on.
//
// 🔴 They are NOT exhaustive from this CLI's point of view, and the renderer
// must not assume they are — see ModelSubstitution.Reason.
const (
	// SubstitutionWrongWorkflow — the id is a real version of this ecosystem but
	// scoped to a DIFFERENT workflow (e.g. an edit-only version sent to txt2img).
	// There is usually a specific right answer the caller wanted.
	SubstitutionWrongWorkflow = "wrong-workflow"
	// SubstitutionUnrecognized — the id is in no scoped list of this ecosystem: a
	// community checkpoint, or a version retired since the caller's script was
	// written.
	SubstitutionUnrecognized = "unrecognized"
	// SubstitutionGated — the id IS offered for this workflow, but a gate rule
	// hides it from THIS user. An entitlement problem, not an authoring one.
	SubstitutionGated = "gated"
)

// ModelSubstitution is one recorded silent checkpoint substitution.
//
// The server drops the `ecosystem`/`workflow` fields it tracks internally before
// putting a record on the wire, on the grounds that the caller already knows what
// it submitted. What survives is the pair of ids plus the reason.
type ModelSubstitution struct {
	// Requested is the checkpoint version id the caller actually sent.
	Requested int `json:"requested"`
	// Applied is the version id the graph ran INSTEAD — and the one that was
	// billed.
	Applied int `json:"applied"`
	// Reason is one of the three Substitution* constants above.
	//
	// 🔴 A PLAIN STRING, DELIBERATELY — not a typed enum, and not validated
	// against the constants on decode. A newer server that adds a fourth reason
	// must still produce a VISIBLE warning from an older CLI: the ids are the
	// load-bearing part of the record, and dropping the whole entry because its
	// label is unfamiliar would restore exactly the silence this feature exists
	// to remove. The renderer falls back to generic advice for an unknown reason
	// rather than discarding the substitution.
	Reason string `json:"reason"`
}

// substitutionMetadata is the `metadata` envelope that carries the record on
// every read AFTER the submit.
//
// The submit persists the record into the orchestrator's workflow metadata, so
// it survives to later reads; the reply-only copy does not. Only the one key is
// modelled — the raw payload travels alongside for `--json`.
type substitutionMetadata struct {
	ModelSubstitutions []ModelSubstitution `json:"modelSubstitutions,omitempty"`
}

// substitutionsFromMetadata decodes the record out of a RAW `metadata` blob.
//
// 🔴 WHY THE CARRIER IS json.RawMessage AND NOT A TYPED FIELD. `metadata` is an
// orchestrator-owned free-form bag, exactly like the `transactions` field
// declared a few lines above it in `Workflow`. Modelling it as a struct made a
// malformed ADVISORY field fail the unmarshal of the whole reply — and the
// replies this rides on are the ones that tell a user what they were just
// CHARGED for. Measured on the typed version, all three shapes losing the entire
// workflow to `unexpected orchestrator.getWorkflow payload`:
//
//	{"metadata":{"modelSubstitutions":[{"requested":"x",…}]}}  entry type wrong
//	{"metadata":{"modelSubstitutions":"nope"}}                 key not an array
//	{"metadata":"nope"}                                        bag not an object
//
// None is producible by today's server, which only ever writes well-formed
// records. That is the point: a feature that exists so a user can discover they
// were billed for the wrong model must not have a read path where a
// garbled WARNING costs them the workflow itself. Failing soft here degrades to
// "no substitutions reported", which is the same thing an older server produces.
//
// Absent / null / not-an-object / no such key all yield nil, none of them an
// error. Entry-level validation stays where it was — a bad entry is skipped, not
// fatal.
func substitutionsFromMetadata(raw json.RawMessage) []ModelSubstitution {
	if len(raw) == 0 {
		return nil
	}
	var env substitutionMetadata
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil
	}
	return env.ModelSubstitutions
}

// Substitutions returns the substitution record for this submit, or nil.
//
// 🔴 IT READS TWO PLACES, AND THE ORDER MATTERS. `generateFromGraph` attaches the
// record BOTH at the top level and under `metadata`. The top-level copy is the
// one the server builds from the validation it just performed, so it is
// authoritative for this request; the metadata copy is what the orchestrator
// echoed back and is the only copy that survives to a later read. Prefer the
// top-level, fall back to metadata — never concatenate, or the same swap is
// reported twice.
func (r *SubmitResult) Substitutions() []ModelSubstitution {
	if r == nil {
		return nil
	}
	// 🔴 len() > 0, NOT != nil. A top-level key decoded as an EMPTY array is not a
	// record — falling back to metadata is what surfaces the swap. `!= nil` would
	// return the empty slice and report nothing, and that mutation survived the
	// suite until it was pinned: absent-vs-empty was implemented correctly here
	// and asserted nowhere.
	if len(r.ModelSubstitutions) > 0 {
		return r.ModelSubstitutions
	}
	return substitutionsFromMetadata(r.Metadata)
}

// Substitutions returns the substitution record persisted on this workflow, or
// nil.
//
// This is the ONLY carrier on a later read. `civitai generate --no-wait` prints
// a workflow id and tells the user to collect the results with
// `civitai workflows get <id>`; without this, that documented path is the one
// place a substituted run stays silent forever.
func (w *Workflow) Substitutions() []ModelSubstitution {
	if w == nil {
		return nil
	}
	return substitutionsFromMetadata(w.Metadata)
}
