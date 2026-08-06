package genapi

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

// ---------------------------------------------------------------------------
// Silent model substitutions (civitai#3665, PRs #3692 / #3673).
//
// The server reports the same records on THREE read sites that do not agree on
// where they live, and a client that reads only one misses cases:
//
//	whatIfFromGraph   -> TOP-LEVEL only (nothing is persisted on that path)
//	generateFromGraph -> BOTH top-level and under `metadata`
//	getWorkflow       -> `metadata` only (the top-level copy is long gone)
//
// So every case below is paired: the carrier is exercised AND a no-substitution
// reply on the same carrier is asserted to produce nothing. A parser wired to
// nothing returns nil for both, which is why the populated half of each pair is
// the positive control for the empty half.
// ---------------------------------------------------------------------------

// wantSub is the fixture record. Its three fields are pairwise distinct so a
// transposition (requested/applied swapped) cannot pass.
var wantSub = ModelSubstitution{Requested: 128713, Applied: 999001, Reason: "unrecognized"}

const subJSON = `[{"requested":128713,"applied":999001,"reason":"unrecognized"}]`

func assertSubs(t *testing.T, site string, got []ModelSubstitution, want ...ModelSubstitution) {
	t.Helper()
	if len(want) == 0 {
		if len(got) != 0 {
			t.Errorf("%s: want no substitutions, got %+v", site, got)
		}
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: substitutions = %+v, want %+v", site, got, want)
	}
}

// --- carrier 1: the whatIf reply (top level, and the ONLY carrier there) -----

func TestWhatIf_ReadsTopLevelModelSubstitutions(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTRPC(w, map[string]any{
			"ready": true,
			"cost":  map[string]any{"base": 60, "total": 60},
			"modelSubstitutions": []any{
				map[string]any{"requested": 128713, "applied": 999001, "reason": "unrecognized"},
			},
		})
	}))
	res, raw, err := c.WhatIfFromGraph(context.Background(), Graph{Workflow: "txt2img"})
	if err != nil {
		t.Fatalf("whatIf: %v", err)
	}
	assertSubs(t, "whatIf top-level", res.ModelSubstitutions(), wantSub)
	// 🔴 --json passthrough: the raw payload must still carry the field, because
	// a script branches on it rather than on our rendering.
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("raw payload is not an object: %v", err)
	}
	if _, ok := m["modelSubstitutions"]; !ok {
		t.Errorf("the raw whatIf payload lost `modelSubstitutions`; --json would report nothing:\n%s", raw)
	}
}

// The overwhelmingly common case. Paired with the test above, which drives the
// same accessor non-empty through the same client.
func TestWhatIf_NoSubstitutionsReportsNone(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTRPC(w, map[string]any{"ready": true, "cost": map[string]any{"base": 60, "total": 60}})
	}))
	res, _, err := c.WhatIfFromGraph(context.Background(), Graph{Workflow: "txt2img"})
	if err != nil {
		t.Fatalf("whatIf: %v", err)
	}
	assertSubs(t, "whatIf without the field", res.ModelSubstitutions())
}

// --- carrier 2: the submit reply (BOTH top-level and metadata) ---------------

// 🔴 A submit reply carrying the record on ONE carrier only must still report
// it. The server writes both, but the top-level copy exists precisely because a
// graph that produced no workflow metadata round-trips nothing — so a reader
// that requires both, or reads only `metadata`, silently loses that case.
func TestSubmit_ReadsEitherCarrier(t *testing.T) {
	cases := []struct {
		name  string
		reply map[string]any
	}{
		{"top-level only", map[string]any{
			"id": "wf_1", "status": "queued",
			"modelSubstitutions": json.RawMessage(subJSON),
		}},
		{"metadata only", map[string]any{
			"id": "wf_1", "status": "queued",
			"metadata": map[string]any{"params": map[string]any{}, "modelSubstitutions": json.RawMessage(subJSON)},
		}},
		{"both carriers, reported once", map[string]any{
			"id": "wf_1", "status": "queued",
			"modelSubstitutions": json.RawMessage(subJSON),
			"metadata":           map[string]any{"modelSubstitutions": json.RawMessage(subJSON)},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeTRPC(w, tc.reply)
			}))
			res, _, _, err := c.GenerateFromGraph(context.Background(), Graph{Workflow: "txt2img"}, SubmitOptions{})
			if err != nil {
				t.Fatalf("submit: %v", err)
			}
			assertSubs(t, tc.name, res.ModelSubstitutions(), wantSub)
		})
	}
}

func TestSubmit_NoSubstitutionsReportsNone(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTRPC(w, map[string]any{"id": "wf_1", "status": "queued",
			"metadata": map[string]any{"params": map[string]any{"prompt": "a cat"}}})
	}))
	res, _, _, err := c.GenerateFromGraph(context.Background(), Graph{Workflow: "txt2img"}, SubmitOptions{})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	assertSubs(t, "submit without the field", res.ModelSubstitutions())
}

// --- carrier 3: a later read (getWorkflow -> `metadata` ONLY) ----------------

func TestGetWorkflow_ReadsMetadataModelSubstitutions(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTRPC(w, map[string]any{
			"id": "wf_1", "status": "succeeded", "createdAt": "2026-08-05T12:00:00Z",
			"metadata": map[string]any{"modelSubstitutions": json.RawMessage(subJSON)},
			"steps":    []any{},
		})
	}))
	wf, raw, err := c.GetWorkflow(context.Background(), "wf_1")
	if err != nil {
		t.Fatalf("getWorkflow: %v", err)
	}
	assertSubs(t, "getWorkflow metadata", wf.ModelSubstitutions(), wantSub)
	if !json.Valid(raw) {
		t.Fatalf("raw payload is not valid JSON: %s", raw)
	}
	// 🔴 The TOP-LEVEL field is absent on this path by design. Reading only
	// there would report nothing for every job the CLI did not itself submit.
	var m map[string]json.RawMessage
	_ = json.Unmarshal(raw, &m)
	if _, ok := m["modelSubstitutions"]; ok {
		t.Errorf("fixture drift: this path is metadata-only server-side, so the fixture must not carry a top-level copy")
	}
}

func TestGetWorkflow_NoSubstitutionsReportsNone(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTRPC(w, map[string]any{
			"id": "wf_1", "status": "succeeded", "createdAt": "2026-08-05T12:00:00Z",
			"metadata": map[string]any{"params": map[string]any{"seed": 1}},
			"steps":    []any{},
		})
	}))
	wf, _, err := c.GetWorkflow(context.Background(), "wf_1")
	if err != nil {
		t.Fatalf("getWorkflow: %v", err)
	}
	assertSubs(t, "getWorkflow without the field", wf.ModelSubstitutions())
}

// --- leniency: a malformed advisory field must never break a paid reply ------

// 🔴 The submit reply is the message that tells a user what they were just
// CHARGED for. If an unparseable `modelSubstitutions` failed the whole decode,
// a server-side shape change would cost them the workflow id — the only handle
// to a job they have already paid for. Every one of these must still yield a
// usable SubmitResult.
func TestSubmit_MalformedSubstitutionsNeverBreakTheReply(t *testing.T) {
	cases := []struct {
		name  string
		field string
	}{
		{"an object instead of an array", `{"requested":1}`},
		{"a string", `"nope"`},
		{"null", `null`},
		{"an array of primitives", `[1,2,3]`},
		{"entries missing applied", `[{"requested":1,"reason":"gated"}]`},
		{"entries missing requested", `[{"applied":2,"reason":"gated"}]`},
		{"entries with an empty reason", `[{"requested":1,"applied":2,"reason":""}]`},
		{"a non-numeric requested", `[{"requested":"1","applied":2,"reason":"gated"}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"id":"wf_1","status":"queued","modelSubstitutions":` + tc.field + `}`
			c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"result":{"data":{"json":` + body + `}}}`))
			}))
			res, _, _, err := c.GenerateFromGraph(context.Background(), Graph{Workflow: "txt2img"}, SubmitOptions{})
			if err != nil {
				t.Fatalf("a malformed advisory field must not fail the submit reply: %v", err)
			}
			if res.ID != "wf_1" {
				t.Fatalf("the workflow id was lost: %+v", res)
			}
			assertSubs(t, tc.name, res.ModelSubstitutions())
		})
	}
}

// 🔴 A reason the server ADDS after this build must survive. The server narrows
// `reason` against its own union because the value doubles as a bounded metric
// label; vendoring that union here would DROP a new reason and turn a real,
// billed substitution back into silence. Kept verbatim instead.
func TestParseModelSubstitutions_KeepsAnUnknownReason(t *testing.T) {
	got := parseModelSubstitutions(json.RawMessage(
		`[{"requested":1,"applied":2,"reason":"a-reason-invented-after-this-build"}]`))
	assertSubs(t, "unknown reason", got,
		ModelSubstitution{Requested: 1, Applied: 2, Reason: "a-reason-invented-after-this-build"})
}

// A bad entry is skipped; its good siblings survive. The server's own reader
// does the same, and dropping the whole array would lose real records.
func TestParseModelSubstitutions_SkipsOnlyTheBadEntry(t *testing.T) {
	got := parseModelSubstitutions(json.RawMessage(
		`[{"requested":1,"applied":2,"reason":"gated"},{"requested":3},{"requested":4,"applied":5,"reason":"wrong-workflow"}]`))
	assertSubs(t, "mixed array", got,
		ModelSubstitution{Requested: 1, Applied: 2, Reason: "gated"},
		ModelSubstitution{Requested: 4, Applied: 5, Reason: "wrong-workflow"})
}

// The metadata key is a cross-repo constant. Pin its literal value: a typo here
// is silent — every payload simply reports no substitutions, forever.
func TestWorkflowMetadataKeyLiteral(t *testing.T) {
	if WorkflowMetadataModelSubstitutionsKey != "modelSubstitutions" {
		t.Errorf("metadata key = %q, want %q (civitai/civitai WORKFLOW_METADATA_MODEL_SUBSTITUTIONS_KEY)",
			WorkflowMetadataModelSubstitutionsKey, "modelSubstitutions")
	}
}

// mergeModelSubstitutions must dedupe EXACT records only. Two swaps that differ
// in any field are two separate facts and both must be reported.
func TestMergeModelSubstitutions_DedupesExactOnly(t *testing.T) {
	a := ModelSubstitution{Requested: 1, Applied: 2, Reason: "gated"}
	b := ModelSubstitution{Requested: 1, Applied: 2, Reason: "unrecognized"}
	c := ModelSubstitution{Requested: 3, Applied: 2, Reason: "gated"}
	got := mergeModelSubstitutions([]ModelSubstitution{a, b}, []ModelSubstitution{a, c})
	assertSubs(t, "merge", got, a, b, c)
}

// Nil receivers are reachable: `submit` can hand back a nil result alongside a
// nil error on a reply the CLI could not model.
func TestModelSubstitutions_NilReceiversAreSafe(t *testing.T) {
	var (
		w  *WhatIfResult
		s  *SubmitResult
		wf *Workflow
	)
	assertSubs(t, "nil WhatIfResult", w.ModelSubstitutions())
	assertSubs(t, "nil SubmitResult", s.ModelSubstitutions())
	assertSubs(t, "nil Workflow", wf.ModelSubstitutions())
}
