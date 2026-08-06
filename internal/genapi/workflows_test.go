package genapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// --- queryGeneratedImages (list) --------------------------------------------

// The list envelope is a tRPC QUERY, so the input rides FLAT under "json" in the
// query string — the same shape whatIf uses, and NOT the nested shape the submit
// mutation uses. A wrong nesting here would 200 with the server's defaults
// (measured on whatIf: every nested body prices the default job), so the page
// size and cursor would be silently ignored and page 2 would repeat page 1.
func TestQueryWorkflows_EnvelopeIsFlatQueryInput(t *testing.T) {
	var got map[string]any
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("queryGeneratedImages must be a GET query, got %s", r.Method)
		}
		if r.URL.Path != QueryWorkflowsPath {
			t.Errorf("path = %s, want %s", r.URL.Path, QueryWorkflowsPath)
		}
		var env struct {
			JSON map[string]any `json:"json"`
		}
		if err := json.Unmarshal([]byte(r.URL.Query().Get("input")), &env); err != nil {
			t.Errorf("input param is not JSON: %v", err)
		}
		got = env.JSON
		writeTRPC(w, map[string]any{"items": []any{}, "nextCursor": nil})
	}))

	if _, _, err := c.QueryWorkflows(context.Background(), ListOptions{Take: 5, Cursor: "cur_1", Tags: []string{"x"}}); err != nil {
		t.Fatalf("QueryWorkflows: %v", err)
	}
	if _, nested := got["input"]; nested {
		t.Fatalf("the query input must be FLAT under \"json\", not nested under \"input\": %v", got)
	}
	if got["take"] != float64(5) {
		t.Errorf("take = %v, want 5", got["take"])
	}
	if got["cursor"] != "cur_1" {
		t.Errorf("cursor = %v, want cur_1", got["cursor"])
	}
}

// Unset options must be ABSENT so the server applies its own defaults. `take: 0`
// would ask for zero workflows and `tags: null` fails the server's zod schema
// (it declares a default of []), so a Go zero value leaking onto the wire is a
// broken request, not a harmless one.
func TestQueryWorkflows_UnsetOptionsAreAbsent(t *testing.T) {
	var got map[string]any
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env struct {
			JSON map[string]any `json:"json"`
		}
		_ = json.Unmarshal([]byte(r.URL.Query().Get("input")), &env)
		got = env.JSON
		writeTRPC(w, map[string]any{"items": []any{}})
	}))
	if _, _, err := c.QueryWorkflows(context.Background(), ListOptions{}); err != nil {
		t.Fatalf("QueryWorkflows: %v", err)
	}
	for _, k := range []string{"take", "cursor", "tags"} {
		if _, ok := got[k]; ok {
			t.Errorf("unset option %q is PRESENT on the wire as %#v — it must be absent", k, got[k])
		}
	}
}

const listPayload = `{
  "items":[
    {"id":"wf_1","status":"succeeded","createdAt":"2026-08-05T12:00:00Z","cost":{"base":8,"total":12},
     "steps":[{"$type":"textToImage","name":"s","status":"succeeded",
       "metadata":{"output":{"c":{"hidden":true}}},
       "output":[
         {"id":"a","available":true,"url":"https://blobs.example/a.jpeg"},
         {"id":"b","available":true,"url":"https://blobs.example/b.jpeg","blockedReason":"minor"},
         {"id":"c","available":true,"url":"https://blobs.example/c.jpeg"},
         {"id":"d","available":false,"url":"https://blobs.example/d.jpeg"}
       ]}]},
    {"id":"wf_2","status":"processing","createdAt":"2026-08-05T12:05:00Z","steps":[]}
  ],
  "nextCursor":"cur_2",
  "serverOnlyField":"kept"}`

func TestQueryWorkflows_DecodesPageAndCounts(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"data":{"json":` + listPayload + `}}}`))
	}))
	page, raw, err := c.QueryWorkflows(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("QueryWorkflows: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(page.Items))
	}
	if page.NextCursor == nil || *page.NextCursor != "cur_2" {
		t.Errorf("nextCursor = %v, want cur_2", page.NextCursor)
	}
	// 4 outputs; one blocked, one hidden (via step metadata), one unavailable.
	total, deliverable := page.Items[0].OutputCounts()
	if total != 4 || deliverable != 1 {
		t.Errorf("OutputCounts = %d/%d, want 1/4 deliverable/total", deliverable, total)
	}
	if page.Items[0].Cost == nil || page.Items[0].Cost.Total != 12 {
		t.Errorf("cost = %v, want total 12", page.Items[0].Cost)
	}
	// The raw payload must survive for --json.
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("raw payload is not JSON: %v", err)
	}
	if decoded["serverOnlyField"] != "kept" {
		t.Error("a field the CLI does not model was dropped from the raw passthrough")
	}
}

// The legacy `images` alias must be counted when `output` is absent — a server
// mid-rollout emits both, one before the rename emits only `images`, and reading
// only `output` would report every older workflow as having produced nothing.
func TestQueryWorkflows_LegacyImagesAliasIsCounted(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"data":{"json":{"items":[{"id":"wf_old","status":"succeeded",
		  "steps":[{"name":"s","images":[{"id":"a","available":true},{"id":"b","available":true}]}]}]}}}}`))
	}))
	page, _, err := c.QueryWorkflows(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("QueryWorkflows: %v", err)
	}
	total, deliverable := page.Items[0].OutputCounts()
	if total != 2 || deliverable != 2 {
		t.Errorf("legacy `images` outputs = %d/%d, want 2/2", deliverable, total)
	}
}

func TestQueryWorkflows_ErrorStatusIsClassified(t *testing.T) {
	srv := trpcErrJSON(t, http.StatusForbidden, "Your API key does not have the required scope", "FORBIDDEN")
	_, _, err := New(srv, "tok").QueryWorkflows(context.Background(), ListOptions{})
	if err == nil {
		t.Fatal("a 403 must return an error")
	}
	if !errors.Is(err, civitai.ErrUnauthorized) {
		t.Errorf("want civitai.ErrUnauthorized, got %v", err)
	}
}

// --- cancelWorkflow ---------------------------------------------------------

// 🔴 cancelWorkflow is a MUTATION. Every assertion here runs against httptest;
// nothing in this file may reach a real server.
func TestCancelWorkflow_PostsWorkflowIdEnvelope(t *testing.T) {
	var method, path string
	var body map[string]any
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		// The real procedure returns undefined, so superjson emits an envelope
		// with NO `json` key at all.
		_, _ = w.Write([]byte(`{"result":{"data":{}}}`))
	}))
	raw, err := c.CancelWorkflow(context.Background(), "  wf_1  ")
	if err != nil {
		t.Fatalf("CancelWorkflow: %v", err)
	}
	if method != http.MethodPost {
		t.Errorf("cancel must be a POST mutation, got %s", method)
	}
	if path != CancelWorkflowPath {
		t.Errorf("path = %s, want %s", path, CancelWorkflowPath)
	}
	inner, _ := body["json"].(map[string]any)
	if inner == nil || inner["workflowId"] != "wf_1" {
		t.Fatalf("body = %v, want {\"json\":{\"workflowId\":\"wf_1\"}} with the id trimmed", body)
	}
	if len(raw) == 0 {
		t.Error("the raw reply must be returned for --json")
	}
}

// 🔴 The regression this pins: unwrapTRPC REJECTS an envelope with no
// `result.data.json`, and cancelWorkflow's success reply is exactly that (the
// server procedure returns undefined). Routing cancel through unwrapTRPC would
// turn every successful cancel into "unexpected response".
func TestCancelWorkflow_EmptyPayloadIsSuccessNotAnError(t *testing.T) {
	for _, reply := range []string{
		`{"result":{"data":{}}}`,
		`{"result":{"data":{"json":null}}}`,
		`{}`,
	} {
		c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(reply))
		}))
		if _, err := c.CancelWorkflow(context.Background(), "wf_1"); err != nil {
			t.Errorf("a 200 with reply %s must be a successful cancel, got %v", reply, err)
		}
	}
	// Negative control: the same code path DOES surface a non-200.
	srv := trpcErrJSON(t, http.StatusNotFound, "No workflow with that id", "NOT_FOUND")
	if _, err := New(srv, "tok").CancelWorkflow(context.Background(), "wf_gone"); err == nil {
		t.Error("a 404 must still be an error — otherwise the success check above proves nothing")
	}
}

func TestCancelWorkflow_NotFoundIsClassified(t *testing.T) {
	srv := trpcErrJSON(t, http.StatusNotFound, "No workflow with that id", "NOT_FOUND")
	_, err := New(srv, "tok").CancelWorkflow(context.Background(), "wf_gone")
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("want civitai.ErrNotFound (exit 4), got %v", err)
	}
}

func TestCancelWorkflow_EmptyIDIsRejectedLocally(t *testing.T) {
	calls := 0
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		writeTRPC(w, map[string]any{})
	}))
	if _, err := c.CancelWorkflow(context.Background(), "   "); err == nil {
		t.Fatal("an empty workflow id must be refused")
	}
	if calls != 0 {
		t.Errorf("a mutation was sent for an empty id (%d calls)", calls)
	}
	// Positive control: the same seam DOES issue a request for a real id.
	if _, err := c.CancelWorkflow(context.Background(), "wf_1"); err != nil {
		t.Fatalf("control cancel: %v", err)
	}
	if calls != 1 {
		t.Errorf("control: request count = %d, want 1 — a zero above would otherwise prove nothing", calls)
	}
}

// trpcErrJSON serves the platform's tRPC error envelope at a given status and
// returns the server URL.
func trpcErrJSON(t *testing.T, status int, message, code string) string {
	t.Helper()
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		body, _ := json.Marshal(map[string]any{
			"error": map[string]any{"json": map[string]any{"message": message, "code": code}},
		})
		_, _ = w.Write(body)
	}))
	return c.BaseURL
}

// --- raw-graph passthrough ---------------------------------------------------

// A raw graph must reach the wire BYTE-FOR-BYTE (modulo whitespace), including
// keys this package does not model. Round-tripping it through the typed Graph
// would delete them silently, which is the failure `--input` exists to avoid.
func TestGraph_RawPassthroughKeepsUnmodelledKeys(t *testing.T) {
	raw := json.RawMessage(`{"workflow":"txt2img","prompt":"a cat","someFutureNode":{"shift":3},"foobar":123}`)
	m := marshalToMap(t, Graph{Raw: raw})
	for _, k := range []string{"workflow", "prompt", "someFutureNode", "foobar"} {
		if _, ok := m[k]; !ok {
			t.Errorf("raw key %q was dropped: %v", k, m)
		}
	}
	// The typed fields must NOT leak in beside the raw payload.
	if len(m) != 4 {
		t.Errorf("raw graph marshalled to %d keys, want exactly the 4 supplied: %v", len(m), m)
	}
}

// The typed fields are IGNORED when Raw is set — a half-merged graph would be a
// payload neither the caller nor the file author wrote.
func TestGraph_RawWinsOverTypedFields(t *testing.T) {
	m := marshalToMap(t, Graph{
		Raw:      json.RawMessage(`{"workflow":"txt2img","prompt":"from the file"}`),
		Prompt:   "from a flag",
		Quantity: Ptr(9),
	})
	if m["prompt"] != "from the file" {
		t.Errorf("prompt = %v, want the raw graph's value", m["prompt"])
	}
	if _, ok := m["quantity"]; ok {
		t.Errorf("a typed field leaked into a raw graph: %v", m)
	}
}

func TestGraph_RawInvalidJSONFailsMarshal(t *testing.T) {
	if _, err := json.Marshal(Graph{Raw: json.RawMessage(`{"workflow":`)}); err == nil {
		t.Fatal("a malformed raw graph must fail to marshal rather than corrupt the envelope")
	}
}

// whatIf strips the prompts from a RAW graph too — and leaves everything else
// untouched. Stripping by re-marshalling a typed struct would have deleted the
// unmodelled keys, so this is also a guard on HOW the strip is done.
func TestWhatIf_StripsPromptsFromRawGraphOnly(t *testing.T) {
	var got map[string]any
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env struct {
			JSON map[string]any `json:"json"`
		}
		_ = json.Unmarshal([]byte(r.URL.Query().Get("input")), &env)
		got = env.JSON
		writeTRPC(w, map[string]any{"ready": true, "cost": map[string]any{"total": 60}})
	}))
	g := Graph{Raw: json.RawMessage(`{"workflow":"txt2img","prompt":"a secret prompt","negativePrompt":"blurry","someFutureNode":{"shift":3}}`)}
	if _, _, err := c.WhatIfFromGraph(context.Background(), g); err != nil {
		t.Fatalf("WhatIfFromGraph: %v", err)
	}
	for _, k := range []string{"prompt", "negativePrompt"} {
		if _, ok := got[k]; ok {
			t.Errorf("%q was shipped on a cost estimate: %v", k, got)
		}
	}
	if _, ok := got["someFutureNode"]; !ok {
		t.Errorf("stripping the prompts deleted an unmodelled key: %v", got)
	}
	// The caller's Graph must be unchanged — the SAME graph is submitted next.
	if !strings.Contains(string(g.Raw), `"prompt":"a secret prompt"`) {
		t.Errorf("whatIf mutated the caller's graph: %s", g.Raw)
	}
}

// The SUBMIT envelope keeps the raw graph nested under "input", beside the
// CLI-owned externalId — the asymmetry phase 0 pinned, now exercised with a raw
// payload.
func TestGenerate_RawGraphKeepsNestedEnvelope(t *testing.T) {
	var body map[string]any
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		writeTRPC(w, map[string]any{"id": "wf_1", "status": "queued"})
	}))
	raw := json.RawMessage(`{"workflow":"txt2img","prompt":"a cat","someFutureNode":{"shift":3}}`)
	if _, _, _, err := c.GenerateFromGraph(context.Background(), Graph{Raw: raw}, SubmitOptions{}); err != nil {
		t.Fatalf("GenerateFromGraph: %v", err)
	}
	inner, _ := body["json"].(map[string]any)
	if inner == nil {
		t.Fatalf("body has no \"json\" wrapper: %v", body)
	}
	graph, _ := inner["input"].(map[string]any)
	if graph == nil {
		t.Fatalf("the raw graph must sit NESTED under \"input\": %v", inner)
	}
	if _, ok := graph["someFutureNode"]; !ok {
		t.Errorf("an unmodelled key was dropped from the submit body: %v", graph)
	}
	if inner["externalId"] == nil || inner["externalId"] == "" {
		t.Error("a raw-graph submit must still carry the minted externalId")
	}
}

// KnownGraphKeys must be derived from the struct, not hand-listed: a hand-list
// is what goes stale and makes the unknown-key warning warn about the wrong
// things.
func TestKnownGraphKeys_TracksTheStruct(t *testing.T) {
	keys := KnownGraphKeys()
	for _, k := range []string{"workflow", "ecosystem", "prompt", "negativePrompt", "quantity",
		"aspectRatio", "model", "resources", "steps", "cfgScale", "sampler", "seed"} {
		if !keys[k] {
			t.Errorf("KnownGraphKeys is missing %q, which Graph models", k)
		}
	}
	// `Raw` is json:"-" and must never appear as a graph key.
	if keys["-"] || keys["Raw"] || keys["raw"] {
		t.Errorf("KnownGraphKeys leaked the Raw carrier field: %v", keys)
	}
	// Negative control: it must not claim to know a key the struct does not have.
	if keys["foobar"] {
		t.Error("KnownGraphKeys reports an unmodelled key as known")
	}
}
