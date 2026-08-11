package genapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// civitai/cli#367 — THE ORCHESTRATOR RECORDS WHY A STEP FAILED AND THE CLI
// DISCARDED IT AT UNMARSHAL.
//
// `errors` is a []string nested inside `output`, a SIBLING of `images` — not a
// field on the step and not an array of objects. The wire shape was confirmed
// against 8 live workflows before any of this was written, and no test here
// re-derives it.
//
// 🔴 MOST FIXTURES HERE ARE HAND-WRITTEN AND MINIMAL, WHICH ON ITS OWN WOULD BE
// A CLOSED LOOP — a hand-written fixture asserts that the code reads the shape
// the author believed, so a wrong belief is inert AND green. The one that closes
// it is TestDecodeRealCapturedFailedWorkflow, which reads a real, whole,
// identifier-redacted capture from testdata/. Read that one before trusting any
// of these.

// theReason is the exact string the measured failures carried. It is a fixture
// value, NOT a wording this code may depend on: all five populated samples were
// the same reproduction, so they prove the field is readable and nothing about
// how the text varies by cause.
const theReason = "Could not generate images with the given prompts and images. Please try again with different inputs."

// wfWithStepErrors builds a one-step failed workflow whose output container is
// named by `container` and whose `errors` array is `errsJSON` (pass "" to omit
// the key entirely).
func wfWithStepErrors(t *testing.T, container, blobsJSON, errsJSON string) *Workflow {
	t.Helper()
	out := `"` + container + `":` + blobsJSON
	if errsJSON != "" {
		out += `,"errors":` + errsJSON
	}
	payload := `{"id":"wf_1","status":"failed","steps":[{"$type":"imageGen","name":"$0","status":"failed",
	  "metadata":{},"output":{` + out + `}}]}`
	var wf Workflow
	if err := json.Unmarshal([]byte(payload), &wf); err != nil {
		t.Fatalf("fixture does not parse: %v\n%s", err, payload)
	}
	if len(wf.Steps) != 1 {
		t.Fatalf("CONTROL failure, not a finding: fixture decoded %d steps, want 1", len(wf.Steps))
	}
	return &wf
}

const oneUnavailableImage = `[{"id":"out_1","available":false,"width":1024,"height":1024}]`
const oneUnavailableBlob = `{"id":"out_1","available":false}`

// --- the unmarshal itself ----------------------------------------------------

// A populated array decodes, and it decodes for EVERY blob container: `errors`
// is not part of the images/blobs/blob union, so which container the step used
// must not decide whether the reason is read.
func TestStepOutputErrors_DecodeAcrossEveryBlobContainer(t *testing.T) {
	for _, c := range []struct{ container, blobs string }{
		{"images", oneUnavailableImage},
		{"blobs", oneUnavailableImage},
		{"blob", oneUnavailableBlob},
	} {
		t.Run(c.container, func(t *testing.T) {
			wf := wfWithStepErrors(t, c.container, c.blobs, `["`+theReason+`"]`)

			// POSITIVE CONTROL: the container really did populate, so a green
			// below cannot come from a workflow with no outputs at all.
			if got := len(wf.Outputs()); got != 1 {
				t.Fatalf("CONTROL failure, not a finding: the %q container yielded %d outputs, want 1", c.container, got)
			}
			got := wf.FailureReasons()
			if len(got) != 1 || got[0] != theReason {
				t.Fatalf("FailureReasons() = %#v, want exactly [%q]", got, theReason)
			}
			if o := wf.Outputs()[0]; len(o.StepErrors) != 1 || o.StepErrors[0] != theReason {
				t.Errorf("Output.StepErrors = %#v, want [%q]", o.StepErrors, theReason)
			}
		})
	}
}

// The two EMPTY branches, which are measured and not defensive: one of six
// failed workflows carried `"errors": []`, and the canceled/succeeded pair
// carried empty arrays too. Neither may invent a reason.
func TestStepOutputErrors_EmptyArrayAndAbsentKeyBothReportNoReason(t *testing.T) {
	for _, c := range []struct{ name, errs string }{
		{"empty_array", `[]`},
		{"key_absent", ``},
		{"blank_entries_only", `["", "   ", "\t\n"]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			wf := wfWithStepErrors(t, "images", oneUnavailableImage, c.errs)
			if got := wf.FailureReasons(); len(got) != 0 {
				t.Errorf("FailureReasons() = %#v, want none — this branch is measured and must not fabricate one", got)
			}
			if got := wf.FailureReasonText(); got != "" {
				t.Errorf("FailureReasonText() = %q, want %q", got, "")
			}
		})
	}
}

// Several steps each carrying reasons: every one is reported, in step order.
// "print the first one" is the failure this pins.
func TestFailureReasons_EveryStepIsReportedInOrder(t *testing.T) {
	payload := `{"id":"wf_1","status":"failed","steps":[
	  {"$type":"imageGen","name":"$0","status":"failed","metadata":{},
	   "output":{"images":[],"errors":["first reason","second reason"]}},
	  {"$type":"imageGen","name":"$1","status":"failed","metadata":{},
	   "output":{"images":[],"errors":["third reason"]}}]}`
	var wf Workflow
	if err := json.Unmarshal([]byte(payload), &wf); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	want := []string{"first reason", "second reason", "third reason"}
	got := wf.FailureReasons()
	if len(got) != len(want) {
		t.Fatalf("FailureReasons() = %#v, want %#v — a later step's reason must not be dropped", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("reason %d = %q, want %q", i, got[i], want[i])
		}
	}
	if joined := wf.FailureReasonText(); joined != strings.Join(want, "; ") {
		t.Errorf("FailureReasonText() = %q, want %q", joined, strings.Join(want, "; "))
	}
}

// De-duplication is EXACT-MATCH ONLY. A four-step run that died the same way
// repeats one string per step; two reasons that differ at all are both kept.
func TestFailureReasons_DedupesExactRepeatsAndNothingElse(t *testing.T) {
	payload := `{"id":"wf_1","status":"failed","steps":[
	  {"$type":"imageGen","name":"$0","status":"failed","metadata":{},"output":{"errors":["same","different"]}},
	  {"$type":"imageGen","name":"$1","status":"failed","metadata":{},"output":{"errors":["same","Different"]}}]}`
	var wf Workflow
	if err := json.Unmarshal([]byte(payload), &wf); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	want := []string{"same", "different", "Different"}
	got := wf.FailureReasons()
	if len(got) != len(want) {
		t.Fatalf("FailureReasons() = %#v, want %#v — only byte-identical repeats collapse; case is not normalised", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("reason %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// 🔴 EACH OUTPUT MUST CARRY ITS OWN STEP'S REASON, NOT THE FIRST STEP'S.
// TestFailureReasons_EveryStepIsReportedInOrder uses `"images":[]`, so it says
// nothing about ATTRIBUTION: computing `reasons` once, outside the step loop,
// left the whole suite green while telling a user the wrong cause for a specific
// output they paid for. This fixture is the one shape that can see it — several
// steps, each with BOTH outputs AND a distinct reason.
func TestOutputs_AttributeEachStepsReasonToThatStepsOwnOutputs(t *testing.T) {
	payload := `{"id":"wf_1","status":"failed","steps":[
	  {"$type":"imageGen","name":"$0","status":"failed","metadata":{},
	   "output":{"images":[{"id":"a1","available":false},{"id":"a2","available":false}],"errors":["reason A"]}},
	  {"$type":"imageGen","name":"$1","status":"failed","metadata":{},
	   "output":{"images":[{"id":"b1","available":false}],"errors":["reason B"]}},
	  {"$type":"imageGen","name":"$2","status":"succeeded","metadata":{},
	   "output":{"images":[{"id":"c1","available":true}]}}]}`
	var wf Workflow
	if err := json.Unmarshal([]byte(payload), &wf); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	outs := wf.Outputs()
	if len(outs) != 4 {
		t.Fatalf("CONTROL failure, not a finding: %d outputs, want 4 — the fixture must give every step outputs "+
			"or attribution cannot be observed", len(outs))
	}
	want := map[string][]string{
		"a1": {"reason A"},
		"a2": {"reason A"},
		"b1": {"reason B"},
		"c1": nil, // a step that carried no reason must not inherit one
	}
	for _, o := range outs {
		w := want[o.ID]
		if len(o.StepErrors) != len(w) {
			t.Errorf("output %s carries %#v, want %#v — a stale binding hands every output the first step's cause",
				o.ID, o.StepErrors, w)
			continue
		}
		for i := range w {
			if o.StepErrors[i] != w[i] {
				t.Errorf("output %s reason %d = %q, want %q", o.ID, i, o.StepErrors[i], w[i])
			}
		}
	}
	// And the per-output SENTENCE follows the attribution, which is what a user
	// actually reads.
	_, excluded := PartitionOutputs(&wf)
	for _, o := range excluded {
		got := ExclusionReason(o)
		wantReason := want[o.ID][0]
		if !strings.Contains(got, wantReason) {
			t.Errorf("output %s should report %q, got: %s", o.ID, wantReason, got)
		}
		for other, rs := range want {
			if other == o.ID || len(rs) == 0 || rs[0] == wantReason {
				continue
			}
			if strings.Contains(got, rs[0]) {
				t.Errorf("output %s reports another step's cause %q: %s", o.ID, rs[0], got)
			}
		}
	}
}

// --- the REAL capture --------------------------------------------------------

// 🔴 EVERY OTHER FIXTURE IN THIS FILE IS HAND-WRITTEN FROM THE SHAPE I BELIEVED,
// AND THAT IS A CLOSED LOOP. If `steps[].output.errors` were nested differently
// on the wire, the field would decode to nothing, the whole feature would be
// inert, and every test here would still pass — they would be asserting that the
// code reads the shape they themselves invented.
//
// testdata/failed_workflow_redacted.json breaks the loop: it is a REAL
// `orchestrator.getWorkflow` payload from a failed run, kept whole — the
// transactions envelope, `metadata.params`, the JSON-in-JSON `imageMetadata`
// string, `cost`, `tips`, `callbacks`, the nulls and the keys this struct does
// not model — with only the identifying values replaced (account id, signed blob
// URLs, orchestrator hosts, job/transaction/external ids). Nothing about the
// SHAPE was touched, which is the only thing this test reads.
func TestDecodeRealCapturedFailedWorkflow(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "failed_workflow_redacted.json"))
	if err != nil {
		t.Fatalf("read the captured payload: %v", err)
	}
	var wf Workflow
	if err := json.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("the real payload does not decode into Workflow: %v", err)
	}

	// The capture is a failed run with one step and one undelivered output.
	if wf.Status != StatusFailed {
		t.Fatalf("CONTROL failure, not a finding: the capture is %q, not %q — it is no longer the shape this test reads",
			wf.Status, StatusFailed)
	}
	if len(wf.Steps) != 1 {
		t.Fatalf("CONTROL failure, not a finding: %d steps in the capture, want 1", len(wf.Steps))
	}

	// THE CLAIM UNDER TEST: the reason comes out of a real payload.
	got := wf.FailureReasons()
	if len(got) != 1 {
		t.Fatalf("the real capture yielded %#v — if this is empty the wire shape is not what internal/genapi models, "+
			"and the whole of civitai/cli#367 is inert", got)
	}
	const want = "Could not generate images with the given prompts and images. Please try again with different inputs."
	if got[0] != want {
		t.Errorf("reason decoded as %q, want %q", got[0], want)
	}

	// …and it reaches the sentence a user reads, through the same path
	// production uses.
	_, excluded := PartitionOutputs(&wf)
	if len(excluded) != 1 {
		t.Fatalf("CONTROL failure, not a finding: %d excluded outputs in the capture, want 1", len(excluded))
	}
	if r := ExclusionReason(excluded[0]); !strings.Contains(r, want) {
		t.Errorf("the captured reason does not reach ExclusionReason: %s", r)
	}

	// The surrounding record still decodes too — evidence that the capture is a
	// whole payload and not a hand-trimmed one shaped to this struct.
	if len(wf.Transactions) == 0 {
		t.Error("the capture lost its transactions envelope, so it no longer exercises the fields around `errors`")
	}
	if s, ok := wf.Settlement(); !ok || s == nil || s.Debits == 0 {
		t.Errorf("the captured transactions did not settle, so the surrounding shape is not being exercised: ok=%v %#v", ok, s)
	}
}

// --- ExclusionReason ---------------------------------------------------------
//
// 🔴 THESE TWO USE ONLY PRE-#367 SYMBOLS (json + PartitionOutputs +
// ExclusionReason), so they compile against the pre-fix tree and can be watched
// to FAIL there rather than failing to build. That is the point: the defect was
// that the reason never reached this sentence.

// The generic parenthetical, pinned as a literal so it cannot drift silently.
const genericUnavailableReason = "not available (the job finished without producing a usable file)"

func TestExclusionReason_UsesTheServerReasonWhenThereIsOne(t *testing.T) {
	wf := wfWithStepErrors(t, "images", oneUnavailableImage, `["`+theReason+`"]`)
	_, excluded := PartitionOutputs(wf)
	if len(excluded) != 1 {
		t.Fatalf("CONTROL failure, not a finding: %d excluded outputs, want 1", len(excluded))
	}
	got := ExclusionReason(excluded[0])
	if !strings.Contains(got, theReason) {
		t.Errorf("the server said why and this sentence does not repeat it — that is civitai/cli#367.\n  want to contain: %s\n  got: %s", theReason, got)
	}
	// ANTI-VACUITY: "an exclusion reason was returned" passes today and would
	// pass after a broken fix. The generic sentence must be GONE, not merely
	// joined by a second one.
	if strings.Contains(got, genericUnavailableReason) {
		t.Errorf("the generic sentence is still here beside the specific one — one exclusion gets one explanation:\n  got: %s", got)
	}
}

func TestExclusionReason_KeepsTheGenericWordingWhenTheServerSaidNothing(t *testing.T) {
	wf := wfWithStepErrors(t, "images", oneUnavailableImage, `[]`)
	_, excluded := PartitionOutputs(wf)
	if len(excluded) != 1 {
		t.Fatalf("CONTROL failure, not a finding: %d excluded outputs, want 1", len(excluded))
	}
	if got := ExclusionReason(excluded[0]); got != genericUnavailableReason {
		t.Errorf("the empty-array branch is measured and its wording is unchanged.\n  want: %s\n  got:  %s", genericUnavailableReason, got)
	}
}

// The other two branches carry their own specific cause and a step-level failure
// reason does not explain either, so they are unchanged even when one is present.
func TestExclusionReason_BlockedAndHiddenAreUnaffectedByAStepReason(t *testing.T) {
	payload := `{"id":"wf_1","status":"succeeded","steps":[{"$type":"imageGen","name":"$0","status":"failed",
	  "metadata":{"output":{"out_2":{"hidden":true}}},
	  "output":{"images":[
	    {"id":"out_1","available":true,"blockedReason":"minor"},
	    {"id":"out_2","available":true}],
	   "errors":["` + theReason + `"]}}]}`
	var wf Workflow
	if err := json.Unmarshal([]byte(payload), &wf); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	_, excluded := PartitionOutputs(&wf)
	if len(excluded) != 2 {
		t.Fatalf("CONTROL failure, not a finding: %d excluded outputs, want 2", len(excluded))
	}
	for _, o := range excluded {
		got := ExclusionReason(o)
		if strings.Contains(got, theReason) {
			t.Errorf("output %s reports the step's failure reason, but it is excluded for its own specific cause:\n  got: %s", o.ID, got)
		}
	}
	if got := ExclusionReason(excluded[0]); got != "blocked by moderation: minor" {
		t.Errorf("blocked wording changed: %s", got)
	}
	if got := ExclusionReason(excluded[1]); got != "hidden (you deleted this result on the website)" {
		t.Errorf("hidden wording changed: %s", got)
	}
}
