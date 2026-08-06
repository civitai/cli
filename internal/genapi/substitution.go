package genapi

import "encoding/json"

// ModelSubstitution is one silent checkpoint swap the server performed and now
// REPORTS (civitai/civitai issue #3665, PRs #3692 and #3673).
//
// It mirrors the server's `PersistedModelSubstitution`
// (civitai/civitai -> src/shared/data-graph/generation/model-substitution.ts):
// on a `modelLocked` ecosystem the graph replaces a checkpoint version id it
// does not recognise for the chosen workflow with that workflow's default,
// returns HTTP 200, and BILLS the caller. `Requested` is the id that was sent,
// `Applied` is the id that actually ran.
//
// 🔴 `Reason` is a plain string and is DELIBERATELY NOT validated against the
// server's three-value union (`wrong-workflow` / `unrecognized` / `gated`).
// The server narrows it on its own side precisely because the value feeds a
// bounded Prometheus label; the CLI has the opposite incentive. Vendoring the
// union here would mean a reason added server-side is DROPPED by an older CLI —
// turning a real, billed substitution back into the silence this whole feature
// exists to end. An unrecognised reason is rendered verbatim instead, which is
// the same anti-mirror judgement AGENTS.md item 13 makes about everything else
// on this path.
type ModelSubstitution struct {
	// Requested is the checkpoint model-VERSION id the caller sent.
	Requested int `json:"requested"`
	// Applied is the model-VERSION id the server ran instead.
	Applied int `json:"applied"`
	// Reason is the server's own token for why. Rendered verbatim.
	Reason string `json:"reason"`
}

// WorkflowMetadataModelSubstitutionsKey is the key silent substitutions are
// persisted under on an orchestrator workflow's `metadata` — the literal string
// of the server's `WORKFLOW_METADATA_MODEL_SUBSTITUTIONS_KEY`.
//
// It is the SAME key on both shapes the CLI reads: `orchestrator.getWorkflow`
// hands back the raw orchestrator workflow whose `metadata` is byte-for-byte
// what the submit persisted, and the normalized shape re-emits it under the
// same name from `formatGenerationResponse2`'s allowlist.
const WorkflowMetadataModelSubstitutionsKey = "modelSubstitutions"

// parseModelSubstitutions decodes a `modelSubstitutions` array LENIENTLY.
//
// 🔴 EVERY CARRIER OF THIS FIELD IS A json.RawMessage, AND THAT IS THE POINT.
// A typed slice on `SubmitResult` would make a malformed advisory field fail the
// whole `json.Unmarshal`, and the submit reply is the message that tells a user
// what they were just CHARGED for — an unparseable warning must never cost them
// the workflow id. So the field is captured raw and interpreted here, where a
// failure degrades to "no substitutions reported" instead of to an error.
//
// Individual entries are validated structurally and a bad one is SKIPPED, which
// mirrors the server's own `readModelSubstitutionsFromMetadata`. The one
// deliberate divergence is `reason`: the server narrows it against its union and
// drops anything else; this keeps any non-empty string. See ModelSubstitution.
//
// Returns nil (never an empty non-nil slice) when there is nothing to report, so
// every caller can test `len(...) > 0` and mean it.
func parseModelSubstitutions(raw json.RawMessage) []ModelSubstitution {
	if len(raw) == 0 {
		return nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil
	}
	var out []ModelSubstitution
	for _, e := range entries {
		var v struct {
			Requested *int   `json:"requested"`
			Applied   *int   `json:"applied"`
			Reason    string `json:"reason"`
		}
		if err := json.Unmarshal(e, &v); err != nil {
			continue
		}
		if v.Requested == nil || v.Applied == nil || v.Reason == "" {
			continue
		}
		out = append(out, ModelSubstitution{Requested: *v.Requested, Applied: *v.Applied, Reason: v.Reason})
	}
	return out
}

// substitutionsFromMetadata reads the persisted array off a workflow `metadata`
// object. A metadata blob that is absent, null, not an object, or carries no
// such key yields nothing — none of those is an error.
func substitutionsFromMetadata(metadata json.RawMessage) []ModelSubstitution {
	if len(metadata) == 0 {
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(metadata, &m); err != nil {
		return nil
	}
	return parseModelSubstitutions(m[WorkflowMetadataModelSubstitutionsKey])
}

// mergeModelSubstitutions concatenates carriers, dropping exact duplicates while
// preserving first-seen order.
//
// 🔴 It exists because `generateFromGraph` reports the SAME records on TWO
// carriers — top-level (authoritative for the validation it just performed) and
// under `metadata` (what survives to every later read). A caller that reads only
// one misses cases (the whatIf has no metadata; a poll has no top level), so the
// CLI reads both — and then has to not tell the user twice.
func mergeModelSubstitutions(groups ...[]ModelSubstitution) []ModelSubstitution {
	type key struct {
		requested int
		applied   int
		reason    string
	}
	seen := make(map[key]bool)
	var out []ModelSubstitution
	for _, g := range groups {
		for _, s := range g {
			k := key{s.Requested, s.Applied, s.Reason}
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, s)
		}
	}
	return out
}
