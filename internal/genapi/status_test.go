package genapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// trpcOK wraps a payload in the tRPC success envelope.
func trpcOK(payload string) string {
	return `{"result":{"data":{"json":` + payload + `}}}`
}

// statusServer serves one scripted getWorkflow reply and records the requests.
func statusServer(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := New(srv.URL, "tok")
	return c, srv
}

func TestGetWorkflow_SendsTheWorkflowIdInTheTRPCInput(t *testing.T) {
	var gotInput string
	var gotPath string
	c, _ := statusServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotInput = r.URL.Query().Get("input")
		_, _ = w.Write([]byte(trpcOK(`{"id":"wf_1","status":"processing","steps":[]}`)))
	})

	wf, raw, err := c.GetWorkflow(context.Background(), "wf_1")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if gotPath != GetWorkflowPath {
		t.Errorf("path = %q, want %q", gotPath, GetWorkflowPath)
	}
	// Pin the ENVELOPE by decoding it, not by substring-matching: a wrongly
	// nested input would still contain the id as a substring.
	var env struct {
		JSON map[string]any `json:"json"`
	}
	if err := json.Unmarshal([]byte(gotInput), &env); err != nil {
		t.Fatalf("input is not JSON: %q", gotInput)
	}
	if env.JSON["workflowId"] != "wf_1" {
		t.Errorf("input.json.workflowId = %v, want wf_1 (input was %q)", env.JSON["workflowId"], gotInput)
	}
	if wf.Status != "processing" {
		t.Errorf("status = %q", wf.Status)
	}
	if len(raw) == 0 {
		t.Error("raw payload must be returned for --json passthrough")
	}
}

func TestGetWorkflow_ErrorsAreClassifiedByStatus(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   error
	}{
		{"unauthorized", http.StatusUnauthorized, civitai.ErrUnauthorized},
		{"forbidden", http.StatusForbidden, civitai.ErrUnauthorized},
		{"not found", http.StatusNotFound, civitai.ErrNotFound},
		{"rate limited", http.StatusTooManyRequests, civitai.ErrRateLimited},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := statusServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"error":{"json":{"message":"nope"}}}`))
			})
			_, _, err := c.GetWorkflow(context.Background(), "wf_1")
			if !errors.Is(err, tc.want) {
				t.Errorf("status %d: errors.Is(err, %v) = false; err = %v", tc.status, tc.want, err)
			}
			// The poll loop branches on this, not on message text.
			if got := StatusOf(err); got != tc.status {
				t.Errorf("StatusOf = %d, want %d", got, tc.status)
			}
		})
	}
}

func TestGetWorkflow_RejectsAnEmptyId(t *testing.T) {
	c := New("https://example.invalid", "tok")
	if _, _, err := c.GetWorkflow(context.Background(), "   "); err == nil {
		t.Fatal("empty workflow id: want an error, got nil")
	}
}

// A null tRPC payload must be rejected, not decoded into a zero Workflow that
// would render as "status (empty), 0 outputs" for a job the user paid for.
func TestGetWorkflow_RejectsANullPayload(t *testing.T) {
	c, _ := statusServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(trpcOK(`null`)))
	})
	if _, _, err := c.GetWorkflow(context.Background(), "wf_1"); err == nil {
		t.Fatal("null payload: want an error, got nil")
	}
}

func TestIsTerminalStatus(t *testing.T) {
	terminal := []string{StatusSucceeded, StatusFailed, StatusExpired, StatusCanceled, "SUCCEEDED", " failed "}
	for _, s := range terminal {
		if !IsTerminalStatus(s) {
			t.Errorf("IsTerminalStatus(%q) = false, want true", s)
		}
	}
	// Everything else keeps waiting — including a status this build has never
	// heard of. Reporting an unknown status as terminal would fabricate an
	// outcome; reporting it as non-terminal costs a --timeout at worst.
	nonTerminal := []string{StatusUnassigned, StatusPreparing, StatusScheduled, StatusProcessing, "", "quantum-superposition"}
	for _, s := range nonTerminal {
		if IsTerminalStatus(s) {
			t.Errorf("IsTerminalStatus(%q) = true, want false", s)
		}
	}
}

// The output filter is the guard that stops `generate` from reporting success
// while writing fewer files than the user paid for. Every exclusion reason is
// driven independently, and the kept set is checked by ID so a filter that
// dropped everything (or nothing) cannot pass.
func TestPartitionOutputs_FiltersBlockedUnavailableAndHidden(t *testing.T) {
	payload := `{
	  "id":"wf_1","status":"succeeded",
	  "steps":[{
	    "$type":"textToImage","name":"step-1","status":"succeeded",
	    "metadata":{"params":{"seed":100},"output":{"c":{"hidden":true}}},
	    "output":{"images":[
	      {"id":"a","type":"image","available":true,"url":"https://x/a.jpeg","width":512,"height":512},
	      {"id":"b","type":"image","available":true,"url":"https://x/b.jpeg","blockedReason":"minor"},
	      {"id":"c","type":"image","available":true,"url":"https://x/c.jpeg"},
	      {"id":"d","type":"image","available":false,"url":null},
	      {"id":"e","type":"image","available":true,"url":"https://x/e.jpeg"}
	    ]}
	  }]
	}`
	var wf Workflow
	if err := json.Unmarshal([]byte(payload), &wf); err != nil {
		t.Fatalf("decode: %v", err)
	}

	kept, excluded := PartitionOutputs(&wf)
	var keptIDs, exclIDs []string
	for _, o := range kept {
		keptIDs = append(keptIDs, o.ID)
	}
	for _, o := range excluded {
		exclIDs = append(exclIDs, o.ID)
	}
	if strings.Join(keptIDs, ",") != "a,e" {
		t.Errorf("kept = %v, want [a e]", keptIDs)
	}
	if strings.Join(exclIDs, ",") != "b,c,d" {
		t.Errorf("excluded = %v, want [b c d] (blocked, hidden, unavailable)", exclIDs)
	}

	// Each exclusion must report ITS OWN reason — a single generic string would
	// pass a "some reason was printed" assertion while telling the user nothing.
	reasons := map[string]string{}
	for _, o := range excluded {
		reasons[o.ID] = ExclusionReason(o)
	}
	if !strings.Contains(reasons["b"], "moderation") || !strings.Contains(reasons["b"], "minor") {
		t.Errorf("blocked reason = %q, want the moderation reason echoed", reasons["b"])
	}
	if !strings.Contains(reasons["c"], "hidden") {
		t.Errorf("hidden reason = %q", reasons["c"])
	}
	if !strings.Contains(reasons["d"], "not available") {
		t.Errorf("unavailable reason = %q", reasons["d"])
	}
	if r := ExclusionReason(kept[0]); r != "" {
		t.Errorf("a deliverable output must have no exclusion reason, got %q", r)
	}

	// Seed is per-OUTPUT (step seed + index), not per-step.
	seedOf := func(o Output) string {
		if o.Seed == nil {
			return "<nil>"
		}
		return fmt.Sprint(*o.Seed)
	}
	if kept[0].Seed == nil || *kept[0].Seed != 100 {
		t.Errorf("output %s seed = %s, want 100", kept[0].ID, seedOf(kept[0]))
	}
	if kept[1].Seed == nil || *kept[1].Seed != 104 {
		t.Errorf("output %s seed = %s, want 104 (index 4)", kept[1].ID, seedOf(kept[1]))
	}
	if kept[0].Width == nil || *kept[0].Width != 512 {
		t.Errorf("width not decoded: %v", kept[0].Width)
	}
}

// `hidden` lives in the step metadata under the CURRENT key `output` and the
// LEGACY key `images`. A reader that only knows the current key reports every
// output of a pre-rename workflow as visible — and then downloads results the
// user deleted.
func TestPartitionOutputs_HonoursTheLegacyHiddenMetadataKey(t *testing.T) {
	payload := `{
	  "id":"wf_1","status":"succeeded",
	  "steps":[{
	    "$type":"textToImage","name":"s","status":"succeeded",
	    "metadata":{"images":{"a":{"hidden":true}}},
	    "output":{"images":[{"id":"a","type":"image","available":true,"url":"https://x/a.jpeg"}]}
	  }]
	}`
	var wf Workflow
	if err := json.Unmarshal([]byte(payload), &wf); err != nil {
		t.Fatalf("decode: %v", err)
	}
	kept, excluded := PartitionOutputs(&wf)
	if len(kept) != 0 || len(excluded) != 1 {
		t.Fatalf("legacy hidden key ignored: kept=%d excluded=%d", len(kept), len(excluded))
	}
	if !strings.Contains(ExclusionReason(excluded[0]), "hidden") {
		t.Errorf("reason = %q", ExclusionReason(excluded[0]))
	}
}

// The three output containers a raw orchestrator step can use. `images` is the
// image path, `blobs` the comfy path, `blob` the upscaler path — reading only
// one of them silently yields zero outputs for the others.
func TestWorkflowOutputs_ReadsEveryStepOutputContainer(t *testing.T) {
	cases := []struct {
		name    string
		output  string
		wantIDs int
	}{
		{"images", `{"images":[{"id":"a","available":true},{"id":"b","available":true}]}`, 2},
		{"blobs", `{"blobs":[{"id":"a","available":true}]}`, 1},
		{"blob", `{"blob":{"id":"a","available":true}}`, 1},
		{"empty", `{}`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var wf Workflow
			payload := `{"id":"w","status":"succeeded","steps":[{"$type":"x","name":"s","status":"succeeded","output":` + tc.output + `}]}`
			if err := json.Unmarshal([]byte(payload), &wf); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got := len(wf.Outputs()); got != tc.wantIDs {
				t.Errorf("outputs = %d, want %d", got, tc.wantIDs)
			}
		})
	}
}
