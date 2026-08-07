package genapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// Decoding the substitution record off the three replies that carry it
// (civitai/civitai#3665 / #3692).
//
// These drive the REAL client against an httptest server returning the REAL
// wire shape, so a change to a json tag or to the metadata precedence fails
// here rather than at a user's expense.

// A substitution on the ESTIMATE must decode. This is the reply that matters
// most: it is the only one that can be read before any money moves.
func TestWhatIf_DecodesModelSubstitutions(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTRPC(w, map[string]any{
			"ready": true,
			"cost":  map[string]any{"total": 60},
			"modelSubstitutions": []map[string]any{
				{"requested": 999999999, "applied": 2436219, "reason": "unrecognized"},
			},
		})
	}))

	got, _, err := c.WhatIfFromGraph(context.Background(), Graph{Workflow: "txt2img", Prompt: "a cat"})
	if err != nil {
		t.Fatalf("WhatIfFromGraph: %v", err)
	}
	if len(got.ModelSubstitutions) != 1 {
		t.Fatalf("want 1 substitution, got %d (%+v)", len(got.ModelSubstitutions), got.ModelSubstitutions)
	}
	s := got.ModelSubstitutions[0]
	if s.Requested != 999999999 || s.Applied != 2436219 || s.Reason != SubstitutionUnrecognized {
		t.Errorf("decoded %+v, want {999999999 2436219 unrecognized}", s)
	}
}

// 🔴 THE BACKWARD-COMPATIBILITY CASE. A server that predates the field omits the
// key entirely; the reply must decode exactly as before with a nil record, and
// nothing may synthesise an "all clear".
func TestWhatIf_OldServerOmitsTheKeyAndDecodesClean(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTRPC(w, map[string]any{"ready": true, "cost": map[string]any{"total": 60}})
	}))

	got, _, err := c.WhatIfFromGraph(context.Background(), Graph{Workflow: "txt2img", Prompt: "a cat"})
	if err != nil {
		t.Fatalf("WhatIfFromGraph must still succeed against an older server: %v", err)
	}
	if got.ModelSubstitutions != nil {
		t.Errorf("an omitted key must decode to nil, got %+v", got.ModelSubstitutions)
	}
	if !got.Ready || got.Cost == nil || got.Cost.Total != 60 {
		t.Errorf("the rest of the reply must be unaffected: %+v", got)
	}
}

// The SUBMIT reply carries the record twice. The top-level copy is the one the
// server builds from the validation it just performed, so it wins.
func TestSubmit_DecodesTopLevelAndMetadata(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTRPC(w, map[string]any{
			"id":     "wf_1",
			"status": "queued",
			"modelSubstitutions": []map[string]any{
				{"requested": 111, "applied": 222, "reason": "gated"},
			},
			"metadata": map[string]any{
				"modelSubstitutions": []map[string]any{
					{"requested": 111, "applied": 222, "reason": "gated"},
				},
			},
		})
	}))

	got, _, _, err := c.GenerateFromGraph(context.Background(), Graph{Workflow: "txt2img"}, SubmitOptions{})
	if err != nil {
		t.Fatalf("GenerateFromGraph: %v", err)
	}
	subs := got.Substitutions()
	// 🔴 Not two. Both copies describe the SAME swap; concatenating would report
	// one substitution as two and inflate what the user thinks happened.
	if len(subs) != 1 {
		t.Fatalf("the two copies of one record must not be concatenated: got %d (%+v)", len(subs), subs)
	}
	if subs[0].Reason != SubstitutionGated || subs[0].Applied != 222 {
		t.Errorf("decoded %+v, want {111 222 gated}", subs[0])
	}
}

// When only the metadata copy is present — the shape every read AFTER the submit
// has — Substitutions() must fall back to it rather than reporting nothing.
func TestSubmit_FallsBackToMetadataWhenTopLevelAbsent(t *testing.T) {
	r := &SubmitResult{
		ID:       "wf_1",
		Metadata: json.RawMessage(`{"modelSubstitutions":[{"requested":7,"applied":8,"reason":"wrong-workflow"}]}`),
	}
	subs := r.Substitutions()
	if len(subs) != 1 || subs[0].Requested != 7 {
		t.Fatalf("metadata fallback lost the record: %+v", subs)
	}
}

// 🔴 ABSENT vs EMPTY at the top level. An EMPTY top-level array is not a record,
// so the metadata copy must still be consulted. This distinguishes `len(...) > 0`
// from `!= nil`: the `!= nil` mutant returns the empty slice and reports NOTHING,
// and it survived the suite until this test existed — the behaviour was
// implemented correctly and asserted nowhere.
func TestSubmit_EmptyTopLevelArrayStillFallsBackToMetadata(t *testing.T) {
	r := &SubmitResult{
		ID:                 "wf_1",
		ModelSubstitutions: []ModelSubstitution{}, // present, non-nil, EMPTY
		Metadata:           json.RawMessage(`{"modelSubstitutions":[{"requested":9,"applied":10,"reason":"gated"}]}`),
	}
	subs := r.Substitutions()
	if len(subs) != 1 || subs[0].Requested != 9 {
		t.Fatalf("an empty top-level array must not suppress the metadata record; got %+v", subs)
	}
}

// POSITIVE CONTROL for the precedence rule: when the two copies DISAGREE, the
// top-level one must win. Without this, a Substitutions() that simply returned
// the metadata copy would pass every test above.
func TestSubmit_TopLevelWinsOverMetadata(t *testing.T) {
	r := &SubmitResult{
		ModelSubstitutions: []ModelSubstitution{{Requested: 1, Applied: 2, Reason: SubstitutionUnrecognized}},
		Metadata:           json.RawMessage(`{"modelSubstitutions":[{"requested":3,"applied":4,"reason":"gated"}]}`),
	}
	subs := r.Substitutions()
	if len(subs) != 1 || subs[0].Applied != 2 {
		t.Fatalf("top-level record must win, got %+v", subs)
	}
}

// A nil result must not panic — runGenerate calls Substitutions() on a reply
// that can legitimately be nil when the server answered without a body.
func TestSubmit_NilResultSubstitutionsIsSafe(t *testing.T) {
	var r *SubmitResult
	if got := r.Substitutions(); got != nil {
		t.Errorf("nil result must yield nil, got %+v", got)
	}
}

// The POLL path — the only carrier once the submit reply is gone. `getWorkflow`
// is a straight passthrough of the raw orchestrator workflow, so the key arrives
// under `metadata` exactly as the submit stored it.
func TestGetWorkflow_DecodesPersistedSubstitutions(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTRPC(w, map[string]any{
			"id":     "wf_1",
			"status": "succeeded",
			"metadata": map[string]any{
				"modelSubstitutions": []map[string]any{
					{"requested": 5, "applied": 6, "reason": "wrong-workflow"},
				},
			},
		})
	}))

	wf, _, err := c.GetWorkflow(context.Background(), "wf_1")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	subs := wf.Substitutions()
	if len(subs) != 1 || subs[0].Reason != SubstitutionWrongWorkflow {
		t.Fatalf("persisted record lost on read-back: %+v", subs)
	}
}

// A workflow with no metadata (or an older server) must read as nil, not panic.
func TestGetWorkflow_NoMetadataIsNil(t *testing.T) {
	var wf *Workflow
	if got := wf.Substitutions(); got != nil {
		t.Errorf("nil workflow must yield nil, got %+v", got)
	}
	if got := (&Workflow{ID: "wf_1"}).Substitutions(); got != nil {
		t.Errorf("workflow without metadata must yield nil, got %+v", got)
	}
}

// 🔴 An UNKNOWN reason from a newer server must SURVIVE decoding. The ids are the
// load-bearing part of the record; dropping the entry because its label is
// unfamiliar would restore the exact silence this feature removes.
func TestSubstitution_UnknownReasonIsPreservedNotDropped(t *testing.T) {
	var out WhatIfResult
	err := json.Unmarshal([]byte(
		`{"ready":true,"modelSubstitutions":[{"requested":1,"applied":2,"reason":"some-future-reason"}]}`), &out)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.ModelSubstitutions) != 1 {
		t.Fatalf("an unknown reason must not drop the record: %+v", out.ModelSubstitutions)
	}
	if out.ModelSubstitutions[0].Reason != "some-future-reason" {
		t.Errorf("the server's own token must be preserved verbatim, got %q", out.ModelSubstitutions[0].Reason)
	}
}

// 🔴 A GARBLED WARNING MUST NEVER COST THE WORKFLOW ITSELF.
//
// `metadata` is an orchestrator-owned free-form bag. While it was modelled as a
// typed struct, a malformed ADVISORY field failed the unmarshal of the WHOLE
// reply — and `GetWorkflow` turns that into
// `unexpected orchestrator.getWorkflow payload`, discarding a workflow the user
// has already been charged for. Measured before the fix: all three shapes below
// lost the entire workflow.
//
// None is producible by today's server, which only writes well-formed records.
// That is exactly why it is worth pinning: this feature exists so a user can
// discover they were billed for the wrong model, so its read path must fail soft
// rather than take the workflow down with the warning.
//
// The well-formed case is the POSITIVE CONTROL — without it, "everything decodes"
// would also be satisfied by a decoder that silently returns nothing for every
// input.
func TestWorkflow_MalformedMetadataNeverCostsTheWorkflow(t *testing.T) {
	cases := []struct {
		name     string
		payload  string
		wantSubs int
	}{
		{"entry field has the wrong type",
			`{"id":"wf-1","status":"succeeded","metadata":{"modelSubstitutions":[{"requested":"x","applied":2,"reason":"unrecognized"}]}}`, 0},
		{"the key is not an array",
			`{"id":"wf-1","status":"succeeded","metadata":{"modelSubstitutions":"nope"}}`, 0},
		{"the metadata bag is not an object",
			`{"id":"wf-1","status":"succeeded","metadata":"nope"}`, 0},
		{"POSITIVE CONTROL: well-formed still decodes",
			`{"id":"wf-1","status":"succeeded","metadata":{"modelSubstitutions":[{"requested":1,"applied":2,"reason":"unrecognized"}]}}`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var w Workflow
			if err := json.Unmarshal([]byte(tc.payload), &w); err != nil {
				t.Fatalf("a malformed advisory field must not fail the whole reply: %v", err)
			}
			if w.ID != "wf-1" {
				t.Fatalf("workflow id lost: %q", w.ID)
			}
			if got := len(w.Substitutions()); got != tc.wantSubs {
				t.Fatalf("Substitutions() = %d records, want %d", got, tc.wantSubs)
			}
		})
	}
}
