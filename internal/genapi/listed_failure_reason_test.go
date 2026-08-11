package genapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// civitai/cli#382 — the LISTING shape carries the failure reason too, at a
// DIFFERENT path from the one #367 fixed:
//
//	workflows get   ->  steps[].output.errors   (output is an OBJECT)
//	workflows list  ->  steps[].errors          (output is an ARRAY)
//
// #367 ruled this surface out of scope as "unverified" through four review
// rounds. These tests exist so nobody has to take that on trust again.

// --- the REAL capture, first, because everything else is a closed loop -------

// 🔴 EVERY HAND-WRITTEN FIXTURE BELOW ASSERTS THE SHAPE I BELIEVED. If `errors`
// sat somewhere else on the wire, `ListedStep.Errors` would decode to nothing,
// the feature would be inert, and every one of those tests would still pass —
// they would be pinning the code against a shape they invented themselves. That
// is exactly how #367 came to rule this surface out.
//
// testdata/listed_workflows_redacted.json breaks the loop: it is a REAL
// `orchestrator.queryGeneratedImages` page (2026-08-11, a read; it spends
// nothing), kept whole — `transactions`, `cost`, `tags`, `metadata.params`, the
// dual-emitted `output`/`images` arrays, `queuePosition`, the nulls and the keys
// this struct does not model — with only identifying values replaced (account
// id, signed blob URLs, orchestrator host, transaction ids, resource names).
// Nothing about the SHAPE was touched.
//
// The population it was cut from, over 30 real workflows: 5 `failed` carried one
// reason each, 1 `failed` carried none, 1 `canceled` and 23 `succeeded` carried
// none. That distribution is why the renderer prints nothing rather than an
// empty label for a failure with no reason: it is a MEASURED branch.
func TestDecodeRealCapturedWorkflowPage(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "listed_workflows_redacted.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	// CONTROL ON THE FIXTURE: the reason must really be at `steps[].errors`, a
	// SIBLING of `output`, and `output` must really be an ARRAY. Asserting only
	// that the struct decodes something cannot tell a correct struct from a
	// fixture that was quietly reshaped to match a wrong one.
	var probe struct {
		Items []struct {
			Steps []map[string]json.RawMessage `json:"steps"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("fixture is not JSON: %v", err)
	}
	if len(probe.Items) < 2 {
		t.Fatalf("CONTROL failure, not a finding: the fixture needs a workflow WITH a reason and one WITHOUT")
	}
	step := probe.Items[0].Steps[0]
	if _, ok := step["errors"]; !ok {
		t.Fatalf("CONTROL failure, not a finding: the fixture has no `errors` key on the step, so this test "+
			"cannot show where the reason lives. Step keys: %v", sortedKeys(step))
	}
	if got := string(step["output"]); !strings.HasPrefix(got, "[") {
		t.Errorf("CONTROL failure, not a finding: `output` is not an ARRAY in this shape (%.20s…). The whole "+
			"point of #382 is that `output.errors` CANNOT exist here", got)
	}
	if inner, ok := step["output"]; ok && strings.Contains(string(inner), `"errors"`) {
		t.Errorf("CONTROL failure, not a finding: the fixture put `errors` INSIDE output, which is the " +
			"`getWorkflow` shape, not this one")
	}

	var page WorkflowPage
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatalf("the real page does not decode: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("decoded %d items, want 2", len(page.Items))
	}
	failed := page.Items[0]
	if failed.Status != "failed" {
		t.Fatalf("CONTROL failure, not a finding: the first entry is %q, not the failed workflow this test "+
			"reads", failed.Status)
	}
	got := failed.FailureReasons()
	want := []string{
		"Google Gemini: Could not generate images with the given prompts and images. Please try again with different inputs.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FailureReasons on the real capture = %q, want %q — the reason the server handed back in the "+
			"SAME response that renders `failed … 0/1` is being dropped at unmarshal", got, want)
	}
	// The workflow with `"errors": null` must report none — and must still be a
	// workflow the renderer can otherwise render.
	if r := page.Items[1].FailureReasons(); len(r) != 0 {
		t.Errorf("the succeeded workflow reports %q; its `errors` is null", r)
	}
	if total, deliverable := page.Items[1].OutputCounts(); total != 1 || deliverable != 1 {
		t.Errorf("output counts on the succeeded workflow = %d/%d, want 1/1 — reading `errors` must not "+
			"disturb the output arrays it is a sibling of", deliverable, total)
	}
	// And reading it is INDEPENDENT of which blob array the step populated: the
	// failed step here carries both `output` and `images`, and its outputs are
	// counted while its reason is read.
	if total, deliverable := failed.OutputCounts(); total != 1 || deliverable != 0 {
		t.Errorf("output counts on the failed workflow = %d/%d, want 0/1", deliverable, total)
	}
}

func sortedKeys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// --- the shape, exhaustively ------------------------------------------------

// The four ways a step can present its reason, one of which — `null` — is what
// the server actually sends for "none": `formatGenerationResponse2` emits
// `errors: errors.length > 0 ? errors : undefined`, and all 25 unpopulated steps
// in the 30-workflow sample arrived as the literal `"errors": null`. An empty
// ARRAY is not what was measured, so it is pinned as a shape that must also
// behave, never as the shape to expect.
func TestListedWorkflow_FailureReasons_EveryPresentation(t *testing.T) {
	for _, c := range []struct {
		name  string
		steps string
		want  []string
	}{
		{"populated", `[{"status":"failed","output":[],"errors":["FIXTURE A"]}]`, []string{"FIXTURE A"}},
		{"null", `[{"status":"failed","output":[],"errors":null}]`, nil},
		{"empty array", `[{"status":"failed","output":[],"errors":[]}]`, nil},
		{"key absent", `[{"status":"failed","output":[]}]`, nil},
		{
			"two steps, two causes",
			`[{"status":"failed","output":[],"errors":["FIXTURE A"]},
			  {"status":"failed","output":[],"errors":["FIXTURE B"]}]`,
			[]string{"FIXTURE A", "FIXTURE B"},
		},
		{
			// The normalizer dedupes WITHIN a step and not across them
			// (`extractStepErrors` ends in `Array.from(new Set(...))`), so this is
			// the shape a multi-step workflow that died the same way twice hands
			// over. Printing it once per step buries whatever differs.
			"two steps, one cause",
			`[{"status":"failed","output":[],"errors":["FIXTURE A"]},
			  {"status":"failed","output":[],"errors":["FIXTURE A"]}]`,
			[]string{"FIXTURE A"},
		},
		{
			"order is wire order, and a shared entry does not merge the sets",
			`[{"status":"failed","output":[],"errors":["FIXTURE B","FIXTURE A"]},
			  {"status":"failed","output":[],"errors":["FIXTURE A","FIXTURE C"]}]`,
			[]string{"FIXTURE B", "FIXTURE A", "FIXTURE C"},
		},
		{
			"blank and control-only entries say nothing and are dropped",
			`[{"status":"failed","output":[],"errors":["  ","\u0000\u0007","FIXTURE A"]}]`,
			[]string{"FIXTURE A"},
		},
		{"no steps at all", `[]`, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			var w ListedWorkflow
			if err := json.Unmarshal([]byte(`{"id":"wf","status":"failed","steps":`+c.steps+`}`), &w); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got := w.FailureReasons(); !reflect.DeepEqual(got, c.want) {
				t.Errorf("FailureReasons = %q, want %q", got, c.want)
			}
		})
	}
}

// 🔴 THE REASON IS NOT PART OF THE BLOB-CONTAINER UNION, AND READING IT MUST NOT
// DEPEND ON WHICH ARRAY THE STEP FILLED. `ListedStep.blobs()` prefers `output`
// and falls back to the legacy `images` alias; a reader that went looking for
// `errors` "next to whichever array won" would silently return nothing for one
// of the two, and the rollout means both are live.
func TestListedWorkflow_FailureReasons_IndependentOfWhichBlobArrayIsPopulated(t *testing.T) {
	for _, c := range []struct{ name, step string }{
		{"output only", `{"status":"failed","output":[{"id":"a","available":true}],"errors":["FIXTURE A"]}`},
		{"legacy images only", `{"status":"failed","images":[{"id":"a","available":true}],"errors":["FIXTURE A"]}`},
		{"both", `{"status":"failed","output":[{"id":"a","available":true}],"images":[{"id":"a","available":true}],"errors":["FIXTURE A"]}`},
		{"neither", `{"status":"failed","errors":["FIXTURE A"]}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			var w ListedWorkflow
			if err := json.Unmarshal([]byte(`{"id":"wf","status":"failed","steps":[`+c.step+`]}`), &w); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got := w.FailureReasons(); !reflect.DeepEqual(got, []string{"FIXTURE A"}) {
				t.Errorf("FailureReasons = %q, want [FIXTURE A] — the reason is a SIBLING of the blob arrays, "+
					"not a member of their union", got)
			}
		})
	}
}

// 🔴 A REASON NESTED UNDER `output` MUST NOT BE READ HERE, and this is the
// symmetry assumption that cost #367 this surface. If `ListedStep` ever grows a
// fallback that reaches into the output objects, a listing payload would start
// reporting reasons the listing endpoint never lifted — and the two commands
// would disagree about the same workflow for a reason nobody could see.
func TestListedWorkflow_DoesNotReadTheGetShapesNestedPath(t *testing.T) {
	var w ListedWorkflow
	raw := `{"id":"wf","status":"failed","steps":[{"status":"failed","output":[{"id":"a","errors":["FIXTURE NESTED"]}]}]}`
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := w.FailureReasons(); len(got) != 0 {
		t.Errorf("FailureReasons = %q from a reason nested inside the output array. In THIS shape `output` is "+
			"the output list; `errors` lives on the step", got)
	}
}

// The two shapes share ONE de-duplication rule, and the last time they did not,
// one payload rendered two ways on one screen (#381). This asserts the property
// on both entry points at once rather than trusting that dedupeReasons is called
// from both.
func TestBothWorkflowShapesApplyTheSameReasonRule(t *testing.T) {
	// Identical reason material, expressed at each shape's own wire path.
	const reasons = `["  FIXTURE A  ","FIXTURE A","","FIXTURE B"]`
	var listed ListedWorkflow
	if err := json.Unmarshal([]byte(`{"id":"wf","steps":[{"errors":`+reasons+`}]}`), &listed); err != nil {
		t.Fatalf("decode listed: %v", err)
	}
	var got Workflow
	if err := json.Unmarshal([]byte(`{"id":"wf","steps":[{"output":{"errors":`+reasons+`}}]}`), &got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if !reflect.DeepEqual(listed.FailureReasons(), got.FailureReasons()) {
		t.Errorf("the two shapes disagree about identical reason material: list=%q get=%q — one payload, "+
			"two answers, which is the #381 defect at a new seam",
			listed.FailureReasons(), got.FailureReasons())
	}
	if want := []string{"FIXTURE A", "FIXTURE B"}; !reflect.DeepEqual(listed.FailureReasons(), want) {
		// POSITIVE CONTROL: agreeing on nothing at all would satisfy the check
		// above while proving the rule is not running.
		t.Errorf("CONTROL failure, not a finding: the shared rule produced %q, want %q — the equality above "+
			"would be satisfied by both shapes reading nothing", listed.FailureReasons(), want)
	}
}
