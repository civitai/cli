package genapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// ---------------------------------------------------------------------------
// Envelope shape — the core guard of this package.
//
// whatIf takes the graph FLAT; generate takes it NESTED under .input. A
// mismatch returns HTTP 200 with a bogus default cost, so a test that only
// asserts "it returned 200" is worthless here. Every assertion below decodes
// the ACTUAL outgoing bytes into map[string]any and inspects key positions.
// ---------------------------------------------------------------------------

// flatOnlyPricer reproduces the MEASURED civitai.com behaviour: whatIfFromGraph
// parses the graph only when it arrives FLAT under "json". A nested payload is
// never parsed at all and prices the default job at 8, byte-identically to {}.
//
// A client wired to the wrong nesting therefore still gets a clean 200 here —
// which is exactly the silent failure this handler exists to expose.
func flatOnlyPricer(w http.ResponseWriter, r *http.Request) {
	var env struct {
		JSON map[string]any `json:"json"`
	}
	if err := json.Unmarshal([]byte(r.URL.Query().Get("input")), &env); err != nil {
		http.Error(w, "bad input param", http.StatusBadRequest)
		return
	}
	total := 8.0 // the default-job price: what a wrongly-nested payload gets
	if _, nested := env.JSON["input"]; !nested {
		if _, ok := env.JSON["workflow"]; ok {
			qty := 1.0
			if q, ok := env.JSON["quantity"].(float64); ok {
				qty = q
			}
			total = 60 * qty
		}
	}
	writeTRPC(w, map[string]any{
		"ready": true,
		"cost":  map[string]any{"base": 60, "total": total, "factors": map[string]any{"quantity": 1}},
	})
}

func writeTRPC(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"result": map[string]any{"data": map[string]any{"json": payload}},
	})
}

func testClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(srv.URL, "test-key")
}

// TestWhatIf_GraphIsFlatInTheEnvelope pins the QUERY envelope: the graph's own
// keys sit at the top level of "json", with no "input" wrapper.
func TestWhatIf_GraphIsFlatInTheEnvelope(t *testing.T) {
	var got map[string]any
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env struct {
			JSON map[string]any `json:"json"`
		}
		if err := json.Unmarshal([]byte(r.URL.Query().Get("input")), &env); err != nil {
			t.Errorf("input param is not JSON: %v", err)
		}
		got = env.JSON
		writeTRPC(w, map[string]any{"ready": true, "cost": map[string]any{"total": 60}})
	}))

	if _, _, err := c.WhatIfFromGraph(context.Background(), Graph{
		Workflow: "txt2img", Ecosystem: "SDXL", Prompt: "a cat", Quantity: Ptr(2),
	}); err != nil {
		t.Fatalf("WhatIfFromGraph: %v", err)
	}

	if _, nested := got["input"]; nested {
		t.Errorf("whatIf envelope nests the graph under .input; it must be FLAT: %v", got)
	}
	for _, k := range []string{"workflow", "ecosystem", "quantity"} {
		if _, ok := got[k]; !ok {
			t.Errorf("whatIf envelope: key %q missing from the TOP level: %v", k, got)
		}
	}
	if got["workflow"] != "txt2img" {
		t.Errorf("whatIf workflow = %v, want txt2img", got["workflow"])
	}
}

// TestWhatIf_PricesTheGraphNotTheDefaultJob is the POSITIVE CONTROL for the
// envelope: against a handler that only prices a FLAT graph, the client must
// see the graph's real price AND that price must MOVE with quantity. A client
// in the wrong-nesting regime would read a constant 8 here.
func TestWhatIf_PricesTheGraphNotTheDefaultJob(t *testing.T) {
	c := testClient(t, http.HandlerFunc(flatOnlyPricer))

	one, _, err := c.WhatIfFromGraph(context.Background(), Graph{Workflow: "txt2img", Quantity: Ptr(1)})
	if err != nil {
		t.Fatalf("WhatIfFromGraph(quantity=1): %v", err)
	}
	four, _, err := c.WhatIfFromGraph(context.Background(), Graph{Workflow: "txt2img", Quantity: Ptr(4)})
	if err != nil {
		t.Fatalf("WhatIfFromGraph(quantity=4): %v", err)
	}
	if one.Cost.Total == 8 || four.Cost.Total == 8 {
		t.Fatalf("got the default-job price (8) — the graph was not parsed, i.e. the envelope is wrong: %v / %v",
			one.Cost.Total, four.Cost.Total)
	}
	if one.Cost.Total != 60 || four.Cost.Total != 240 {
		t.Fatalf("totals = %v / %v, want 60 / 240", one.Cost.Total, four.Cost.Total)
	}
	if four.Cost.Total <= one.Cost.Total {
		t.Errorf("total did not move with quantity (%v -> %v): a constant total is indistinguishable "+
			"from a harness wired to nothing", one.Cost.Total, four.Cost.Total)
	}
}

// TestFlatOnlyPricer_NegativeControl validates the HARNESS above: fed a NESTED
// payload directly (bypassing the client), it must report the bogus default
// price of 8. Without this, the green result in the previous test could come
// from a handler that always answers 60.
func TestFlatOnlyPricer_NegativeControl(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(flatOnlyPricer))
	defer srv.Close()

	get := func(input string) float64 {
		t.Helper()
		q := url.Values{}
		q.Set("input", input)
		resp, err := http.Get(srv.URL + "?" + q.Encode())
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		payload, err := unwrapTRPC("test", raw)
		if err != nil {
			t.Fatalf("unwrap: %v", err)
		}
		var out WhatIfResult
		if err := json.Unmarshal(payload, &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out.Cost.Total
	}

	if got := get(`{"json":{"input":{"workflow":"txt2img","quantity":4}}}`); got != 8 {
		t.Errorf("nested payload priced %v, want the bogus default 8 — the harness cannot detect a wrong envelope", got)
	}
	if got := get(`{"json":{}}`); got != 8 {
		t.Errorf("empty payload priced %v, want 8", got)
	}
	if got := get(`{"json":{"workflow":"txt2img","quantity":4}}`); got != 240 {
		t.Errorf("flat payload priced %v, want 240 — the harness cannot observe a correct envelope", got)
	}
}

// checkNestedEnvelope asserts the SUBMIT envelope shape on a decoded body: the
// graph must sit under .json.input, and its keys must NOT be at .json's top
// level beside externalId.
func checkNestedEnvelope(body map[string]any) error {
	j, ok := body["json"].(map[string]any)
	if !ok {
		return fmt.Errorf("body has no object at .json: %v", body)
	}
	inner, ok := j["input"].(map[string]any)
	if !ok {
		return fmt.Errorf("submit envelope has no object at .json.input — the graph must be NESTED: %v", j)
	}
	if _, ok := inner["workflow"]; !ok {
		return fmt.Errorf(".json.input carries no graph keys: %v", inner)
	}
	if _, leaked := j["workflow"]; leaked {
		return fmt.Errorf("graph keys leaked to .json's top level (that is the whatIf shape): %v", j)
	}
	return nil
}

// TestGenerate_GraphIsNestedUnderInput pins the MUTATION envelope.
func TestGenerate_GraphIsNestedUnderInput(t *testing.T) {
	var body map[string]any
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("submit body is not JSON: %v", err)
		}
		writeTRPC(w, map[string]any{"id": "wf_123", "status": "processing"})
	}))

	res, _, _, err := c.GenerateFromGraph(context.Background(),
		Graph{Workflow: "txt2img", Prompt: "a cat"}, SubmitOptions{Tags: []string{"cli"}})
	if err != nil {
		t.Fatalf("GenerateFromGraph: %v", err)
	}
	if err := checkNestedEnvelope(body); err != nil {
		t.Fatalf("submit envelope: %v", err)
	}
	if res.ID != "wf_123" {
		t.Errorf("workflow id = %q, want wf_123", res.ID)
	}
}

// TestCheckNestedEnvelope_NegativeControl validates the assertion above: fed
// the whatIf (flat) shape and a graph-keys-leaked shape, it must FAIL. A
// checker that cannot go red says nothing about the real envelope.
func TestCheckNestedEnvelope_NegativeControl(t *testing.T) {
	cases := map[string]string{
		"flat (the whatIf shape)": `{"json":{"workflow":"txt2img","externalId":"x"}}`,
		"no json wrapper":         `{"input":{"workflow":"txt2img"}}`,
		"input present but empty": `{"json":{"input":{}}}`,
		"graph keys duplicated":   `{"json":{"input":{"workflow":"txt2img"},"workflow":"txt2img"}}`,
	}
	for name, raw := range cases {
		var m map[string]any
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatalf("%s: fixture is not JSON: %v", name, err)
		}
		if err := checkNestedEnvelope(m); err == nil {
			t.Errorf("%s: checkNestedEnvelope accepted a bad shape", name)
		}
	}
	// Positive control: the correct shape must pass.
	var good map[string]any
	if err := json.Unmarshal([]byte(`{"json":{"input":{"workflow":"txt2img"},"externalId":"x"}}`), &good); err != nil {
		t.Fatal(err)
	}
	if err := checkNestedEnvelope(good); err != nil {
		t.Errorf("checkNestedEnvelope rejected the correct shape: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Unset fields must be ABSENT from the wire, never Go zero values.
// ---------------------------------------------------------------------------

func marshalToMap(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	return m
}

// TestGraph_UnsetFieldsAreAbsent is the key-absence guard. `steps: 0` is
// ACCEPTED by the server and silently prices a degenerate half-price job, so a
// Go zero value leaking onto the wire is a money bug, not a cosmetic one.
//
// It checks KEY PRESENCE on map[string]any, never strings.Contains: "cfg"
// substring-matches "cfgScale", so a text search cannot tell the two apart.
func TestGraph_UnsetFieldsAreAbsent(t *testing.T) {
	m := marshalToMap(t, Graph{Workflow: "txt2img", Prompt: "a cat"})

	for _, k := range []string{"steps", "cfgScale", "quantity", "seed", "sampler", "ecosystem",
		"aspectRatio", "negativePrompt", "model", "resources"} {
		if _, ok := m[k]; ok {
			t.Errorf("unset field %q is PRESENT on the wire as %#v — it must be absent", k, m[k])
		}
	}
	for _, k := range []string{"workflow", "prompt"} {
		if _, ok := m[k]; !ok {
			t.Errorf("set field %q is missing from the wire: %v", k, m)
		}
	}
}

// TestGraph_ZeroValueMarshalsToEmptyObject is the same guard in its widest
// form: it covers every field, including ones added later, so a new value-typed
// field without omitempty fails here without anyone remembering to list it.
func TestGraph_ZeroValueMarshalsToEmptyObject(t *testing.T) {
	m := marshalToMap(t, Graph{})
	if len(m) != 0 {
		t.Errorf("zero Graph marshalled to %d keys (%v); every optional field must be absent when unset", len(m), m)
	}
}

// TestGraph_ExplicitZeroIsSent is the other half of the pointer rationale: a
// caller that DELIBERATELY sets 0 must be able to, and that is exactly what a
// value-typed field with omitempty could not express.
func TestGraph_ExplicitZeroIsSent(t *testing.T) {
	m := marshalToMap(t, Graph{Steps: Ptr(0), CfgScale: Ptr(0.0)})
	for _, k := range []string{"steps", "cfgScale"} {
		v, ok := m[k]
		if !ok {
			t.Fatalf("explicitly-zero %q was dropped; unset and explicit-zero must stay distinguishable", k)
		}
		if v != 0.0 {
			t.Errorf("%q = %v, want 0", k, v)
		}
	}
}

// TestWhatIf_StripsPrompts pins that a cost estimate does not ship the user's
// prompt. The server substitutes its own placeholder for whatIf, so this cannot
// change the quote.
func TestWhatIf_StripsPrompts(t *testing.T) {
	var got map[string]any
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env struct {
			JSON map[string]any `json:"json"`
		}
		_ = json.Unmarshal([]byte(r.URL.Query().Get("input")), &env)
		got = env.JSON
		writeTRPC(w, map[string]any{"ready": true, "cost": map[string]any{"total": 60}})
	}))

	g := Graph{Workflow: "txt2img", Prompt: "a secret prompt", NegativePrompt: "blurry"}
	if _, _, err := c.WhatIfFromGraph(context.Background(), g); err != nil {
		t.Fatalf("WhatIfFromGraph: %v", err)
	}
	for _, k := range []string{"prompt", "negativePrompt"} {
		if _, ok := got[k]; ok {
			t.Errorf("whatIf sent %q (%v); prompts must be stripped from a cost estimate", k, got[k])
		}
	}
	// The caller's graph must be untouched — the same value is submitted next.
	if g.Prompt != "a secret prompt" || g.NegativePrompt != "blurry" {
		t.Errorf("WhatIfFromGraph mutated the caller's graph: %+v", g)
	}
}

// ---------------------------------------------------------------------------
// externalId — unconditional on submit, never on whatIf.
// ---------------------------------------------------------------------------

var uuidV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// TestGenerate_MintsExternalIDUnconditionally: the platform retries a submit 3x
// with the same body and adds no idempotency key of its own, so a missing
// externalId risks charging one user action up to three times.
func TestGenerate_MintsExternalIDUnconditionally(t *testing.T) {
	var bodies []map[string]any
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &m)
		bodies = append(bodies, m)
		writeTRPC(w, map[string]any{"id": "wf_1", "status": "processing"})
	}))

	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		_, returned, _, err := c.GenerateFromGraph(context.Background(), Graph{Workflow: "txt2img"}, SubmitOptions{})
		if err != nil {
			t.Fatalf("GenerateFromGraph: %v", err)
		}
		j := bodies[i]["json"].(map[string]any)
		id, ok := j["externalId"].(string)
		if !ok || id == "" {
			t.Fatalf("submit body has no externalId: %v", j)
		}
		if id != returned {
			t.Errorf("returned externalId %q != the one sent %q", returned, id)
		}
		if !uuidV4Re.MatchString(id) {
			t.Errorf("externalId %q is not a v4 UUID", id)
		}
		if seen[id] {
			t.Errorf("externalId %q reused across submits — each submit needs its own key", id)
		}
		seen[id] = true
	}
}

// TestGenerate_ExternalIDOverride: an explicit id is passed through verbatim
// (this is how a re-attach after an interrupt reuses the original key).
func TestGenerate_ExternalIDOverride(t *testing.T) {
	var body map[string]any
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		writeTRPC(w, map[string]any{"id": "wf_1"})
	}))
	_, returned, _, err := c.GenerateFromGraph(context.Background(),
		Graph{Workflow: "txt2img"}, SubmitOptions{ExternalID: "fixed-key-1"})
	if err != nil {
		t.Fatalf("GenerateFromGraph: %v", err)
	}
	if returned != "fixed-key-1" {
		t.Errorf("returned externalId = %q, want fixed-key-1", returned)
	}
	if got := body["json"].(map[string]any)["externalId"]; got != "fixed-key-1" {
		t.Errorf("sent externalId = %v, want fixed-key-1", got)
	}
}

// TestWhatIf_NeverSendsExternalID: a whatIf carrying a matching key would
// return the pre-existing workflow and burn the key the submit needs.
func TestWhatIf_NeverSendsExternalID(t *testing.T) {
	var rawQuery string
	var input map[string]any
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		var env map[string]any
		_ = json.Unmarshal([]byte(r.URL.Query().Get("input")), &env)
		input, _ = env["json"].(map[string]any)
		writeTRPC(w, map[string]any{"ready": true, "cost": map[string]any{"total": 60}})
	}))

	g := Graph{Workflow: "txt2img", Prompt: "a cat"}
	if _, _, err := c.WhatIfFromGraph(context.Background(), g); err != nil {
		t.Fatalf("WhatIfFromGraph: %v", err)
	}
	if _, ok := input["externalId"]; ok {
		t.Errorf("whatIf graph carries externalId: %v", input)
	}
	// Widest form: the key must not appear ANYWHERE in the request line —
	// neither as its own param nor inside the encoded input blob.
	decoded, err := url.QueryUnescape(rawQuery)
	if err != nil {
		t.Fatalf("query unescape: %v", err)
	}
	if strings.Contains(strings.ToLower(decoded), "externalid") {
		t.Errorf("whatIf request mentions externalId: %s", decoded)
	}
	// Positive control for that scan: it must be able to SEE the token when one
	// is present, otherwise the clean result above proves nothing.
	if !strings.Contains(strings.ToLower(decoded+"externalId"), "externalid") {
		t.Fatal("the externalId scan cannot detect the token at all")
	}
}

// ---------------------------------------------------------------------------
// Error classification — pinned by errors.Is, never by message text.
// ---------------------------------------------------------------------------

func TestGenerateErrors_Classification(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"401", http.StatusUnauthorized, `{"error":{"json":{"message":"UNAUTHORIZED"}}}`, civitai.ErrUnauthorized},
		{"403", http.StatusForbidden, `{"error":{"json":{"message":"Insufficient funds"}}}`, civitai.ErrUnauthorized},
		{"400", http.StatusBadRequest, `{"error":{"json":{"message":"request is invalid"}}}`, civitai.ErrBadRequest},
		{"404", http.StatusNotFound, `{"message":"not found"}`, civitai.ErrNotFound},
		{"429", http.StatusTooManyRequests, `{"message":"slow down"}`, civitai.ErrRateLimited},
		{"503", http.StatusServiceUnavailable, `{"message":"down"}`, civitai.ErrNetwork},
	}
	for _, tc := range cases {
		t.Run("whatIf/"+tc.name, func(t *testing.T) {
			c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			_, _, err := c.WhatIfFromGraph(context.Background(), Graph{Workflow: "txt2img"})
			if err == nil {
				t.Fatalf("status %d returned no error", tc.status)
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("errors.Is(err, %v) = false for status %d; err = %v", tc.want, tc.status, err)
			}
		})
		t.Run("generate/"+tc.name, func(t *testing.T) {
			c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			_, _, _, err := c.GenerateFromGraph(context.Background(), Graph{Workflow: "txt2img"}, SubmitOptions{})
			if err == nil {
				t.Fatalf("status %d returned no error", tc.status)
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("errors.Is(err, %v) = false for status %d; err = %v", tc.want, tc.status, err)
			}
		})
	}
}

// TestGenerateErrors_500IsNotMisclassified: an unknown ecosystem arrives as a
// bare 500. It must NOT be tagged as an auth failure (which would tell a user
// to re-run `civitai login` for a typo). Design §7 wants it re-mapped to usage
// at the command surface; this layer must at least not lie about it.
func TestGenerateErrors_500IsNotMisclassified(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":{"json":{"message":"Unknown ecosystem: SDXLL"}}}`)
	}))
	_, _, err := c.WhatIfFromGraph(context.Background(), Graph{Workflow: "txt2img", Ecosystem: "SDXLL"})
	if err == nil {
		t.Fatal("500 returned no error")
	}
	for _, kind := range []error{civitai.ErrUnauthorized, civitai.ErrBadRequest, civitai.ErrNotFound,
		civitai.ErrRateLimited, civitai.ErrNetwork} {
		if errors.Is(err, kind) {
			t.Errorf("500 was classified as %v; it carries no kind today", kind)
		}
	}
	if !strings.Contains(err.Error(), "Unknown ecosystem") {
		t.Errorf("500 error dropped the server message: %v", err)
	}
}

// TestGenerateErrors_NoToken refuses before any request is made.
func TestGenerateErrors_NoToken(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("a request was sent without a token")
	}))
	c.Tokens = civitai.StaticToken("")
	if _, _, err := c.WhatIfFromGraph(context.Background(), Graph{}); !errors.Is(err, civitai.ErrUnauthorized) {
		t.Errorf("empty token: err = %v, want ErrUnauthorized", err)
	}
	if _, _, _, err := c.GenerateFromGraph(context.Background(), Graph{}, SubmitOptions{}); !errors.Is(err, civitai.ErrUnauthorized) {
		t.Errorf("empty token: err = %v, want ErrUnauthorized", err)
	}
}

// TestMalformedEnvelope covers the shapes that unmarshal CLEANLY into a zero
// struct: a `null` payload would render as a free (total 0) quote.
func TestMalformedEnvelope(t *testing.T) {
	cases := map[string]string{
		"null payload":  `{"result":{"data":{"json":null}}}`,
		"empty object":  `{}`,
		"no json key":   `{"result":{"data":{}}}`,
		"not json":      `<html>502 bad gateway</html>`,
		"bare array":    `[]`,
		"result is nul": `{"result":null}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, body)
			}))
			res, raw, err := c.WhatIfFromGraph(context.Background(), Graph{Workflow: "txt2img"})
			if err == nil {
				t.Fatalf("malformed envelope %q accepted: result=%+v raw=%s", body, res, raw)
			}
			if res != nil {
				t.Errorf("malformed envelope returned a non-nil result: %+v", res)
			}
			sub, _, _, err := c.GenerateFromGraph(context.Background(), Graph{Workflow: "txt2img"}, SubmitOptions{})
			if err == nil {
				t.Fatalf("malformed submit envelope %q accepted: %+v", body, sub)
			}
		})
	}
}

// TestUnwrapTRPC_PositiveControl proves unwrapTRPC can actually observe a good
// payload — otherwise the rejections above could come from a function that
// rejects everything.
func TestUnwrapTRPC_PositiveControl(t *testing.T) {
	payload, err := unwrapTRPC("test", []byte(`{"result":{"data":{"json":{"ready":true,"cost":{"total":60}}}}}`))
	if err != nil {
		t.Fatalf("unwrapTRPC rejected a well-formed envelope: %v", err)
	}
	var out WhatIfResult
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !out.Ready || out.Cost.Total != 60 {
		t.Errorf("decoded = %+v, want ready + total 60", out)
	}
}

// ---------------------------------------------------------------------------
// Transport
// ---------------------------------------------------------------------------

// refreshOnceSource returns a stale token until Refresh is called.
type refreshOnceSource struct {
	refreshed bool
	calls     int
}

func (s *refreshOnceSource) Token(context.Context) (string, error) {
	if s.refreshed {
		return "good", nil
	}
	return "stale", nil
}

func (s *refreshOnceSource) Refresh(context.Context) (string, error) {
	s.calls++
	s.refreshed = true
	return "good", nil
}

// TestAuthedDo_RefreshesOn401 pins the transparent-refresh replay for both the
// GET and the POST path.
func TestAuthedDo_RefreshesOn401(t *testing.T) {
	var tokens []string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		tokens = append(tokens, tok)
		if tok != "good" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":{"json":{"message":"expired"}}}`)
			return
		}
		writeTRPC(w, map[string]any{"ready": true, "cost": map[string]any{"total": 60}, "id": "wf_1"})
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	src := &refreshOnceSource{}
	c := NewWithSource(srv.URL, src)
	if _, _, err := c.WhatIfFromGraph(context.Background(), Graph{Workflow: "txt2img"}); err != nil {
		t.Fatalf("WhatIfFromGraph after refresh: %v", err)
	}
	if len(tokens) != 2 || tokens[0] != "stale" || tokens[1] != "good" {
		t.Fatalf("tokens = %v, want [stale good]", tokens)
	}

	// The POST body must survive the replay.
	var replayed []map[string]any
	src2 := &refreshOnceSource{}
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &m)
		replayed = append(replayed, m)
		if strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") != "good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeTRPC(w, map[string]any{"id": "wf_1"})
	}))
	defer srv2.Close()
	c2 := NewWithSource(srv2.URL, src2)
	_, id, _, err := c2.GenerateFromGraph(context.Background(), Graph{Workflow: "txt2img"}, SubmitOptions{})
	if err != nil {
		t.Fatalf("GenerateFromGraph after refresh: %v", err)
	}
	if len(replayed) != 2 {
		t.Fatalf("submit attempts = %d, want 2", len(replayed))
	}
	// 🔴 The replayed body must carry the SAME externalId, or the refresh retry
	// would be a second charge.
	first := replayed[0]["json"].(map[string]any)["externalId"]
	second := replayed[1]["json"].(map[string]any)["externalId"]
	if first != second || first != id {
		t.Errorf("replayed submit changed externalId (%v -> %v, returned %v); the retry must dedupe", first, second, id)
	}
}

// TestNewExternalID_Format/uniqueness — the key itself.
func TestNewExternalID(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 64; i++ {
		id, err := NewExternalID()
		if err != nil {
			t.Fatalf("NewExternalID: %v", err)
		}
		if !uuidV4Re.MatchString(id) {
			t.Fatalf("NewExternalID = %q, not an RFC 4122 v4 UUID", id)
		}
		if seen[id] {
			t.Fatalf("NewExternalID repeated %q", id)
		}
		seen[id] = true
	}
}

// TestResponseBodyCap: an over-cap body is an error, not silently truncated
// JSON.
func TestResponseBodyCap(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"result":{"data":{"json":{"ready":true,"cost":{"total":60,"pad":"`+
			strings.Repeat("x", 4096)+`"}}}}}`)
	}))
	c.MaxResponseBody = 128
	if _, _, err := c.WhatIfFromGraph(context.Background(), Graph{Workflow: "txt2img"}); err == nil {
		t.Fatal("over-cap body was accepted")
	} else if !strings.Contains(err.Error(), "limit") {
		t.Errorf("over-cap error = %v, want a limit error", err)
	}
	// Positive control: under the cap the same client succeeds.
	c.MaxResponseBody = 1 << 20
	if _, _, err := c.WhatIfFromGraph(context.Background(), Graph{Workflow: "txt2img"}); err != nil {
		t.Errorf("under-cap body rejected: %v", err)
	}
}
