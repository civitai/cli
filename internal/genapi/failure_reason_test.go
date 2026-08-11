package genapi

import (
	"encoding/json"
	"strings"
	"testing"
)

// civitai/cli#367 — THE ORCHESTRATOR RECORDS WHY A STEP FAILED AND THE CLI
// DISCARDED IT AT UNMARSHAL.
//
// `errors` is a []string nested inside `output`, a SIBLING of `images` — not a
// field on the step and not an array of objects. The shape below is a redacted
// real capture; the wire shape was confirmed against 8 live workflows before any
// of this was written, and no test here re-derives it.
//
// 🔴 EVERY FIXTURE IS HAND-WRITTEN AND MINIMAL. The capture it was read from
// carried signed blob URLs and is deliberately not in the repo.

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
