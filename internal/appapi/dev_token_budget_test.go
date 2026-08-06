package appapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// budgetRecorder stands up a dev-token endpoint that records the RAW request
// body. Assertions here read the bytes rather than a decoded struct: absent and
// zero are the same value once decoded into an int, and telling those two apart
// is the entire contract this flag has to honour.
func budgetRecorder(t *testing.T, raw *[]byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*raw = b
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "jwt-x"})
	}))
}

// TestMintDevTokenBudgetWireShape is the table that pins the absent/present
// distinction at the transport layer.
func TestMintDevTokenBudgetWireShape(t *testing.T) {
	i := func(n int) *int { return &n }
	cases := []struct {
		name       string
		budget     *int
		wantKey    bool
		wantNumber float64
	}{
		{"nil sends NO buzzBudget key (server default stays reachable)", nil, false, 0},
		{"the server cap", i(DevBuzzBudgetCap), true, float64(DevBuzzBudgetCap)},
		{"the server default, requested explicitly", i(DevBuzzBudgetDefault), true, float64(DevBuzzBudgetDefault)},
		{"the schema minimum", i(DevBuzzBudgetMin), true, float64(DevBuzzBudgetMin)},
		{"a mid value", i(120), true, 120},
		// A pointer to zero must still be SENT, not swallowed by omitempty:
		// "the caller explicitly asked for 0" and "the caller asked for
		// nothing" are different requests, and only the first should draw the
		// server's 400. Callers range-check first; this pins that the transport
		// does not quietly rewrite the request into the other one.
		{"an explicit zero is sent, not omitted", i(0), true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var raw []byte
			srv := budgetRecorder(t, &raw)
			defer srv.Close()

			if _, err := New(srv.URL, "tok", "").MintDevToken(context.Background(), "my-block", nil, tc.budget); err != nil {
				t.Fatalf("MintDevToken: %v", err)
			}
			var generic map[string]any
			if err := json.Unmarshal(raw, &generic); err != nil {
				t.Fatalf("body is not a JSON object (%v): %q", err, string(raw))
			}
			got, present := generic["buzzBudget"]
			if present != tc.wantKey {
				t.Fatalf("buzzBudget key present = %v, want %v — body %q", present, tc.wantKey, string(raw))
			}
			if tc.wantKey && got != tc.wantNumber {
				t.Errorf("buzzBudget = %v (%T), want %v — body %q", got, got, tc.wantNumber, string(raw))
			}
			// The slug must survive alongside the new field.
			if generic["slug"] != "my-block" {
				t.Errorf("slug = %v, want my-block — body %q", generic["slug"], string(raw))
			}
		})
	}
}

// TestMintDevTokenBudgetIsIndependentOfScopes: the two optional fields must not
// interfere — sending scopes must not suppress the budget, and vice versa.
func TestMintDevTokenBudgetIsIndependentOfScopes(t *testing.T) {
	var raw []byte
	srv := budgetRecorder(t, &raw)
	defer srv.Close()

	n := 200
	if _, err := New(srv.URL, "tok", "").MintDevToken(
		context.Background(), "my-block", []string{"ai:write:budgeted"}, &n); err != nil {
		t.Fatalf("MintDevToken: %v", err)
	}
	var body struct {
		Slug       string   `json:"slug"`
		Scopes     []string `json:"scopes"`
		BuzzBudget *int     `json:"buzzBudget"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode: %v (%q)", err, string(raw))
	}
	if body.BuzzBudget == nil || *body.BuzzBudget != 200 {
		t.Errorf("buzzBudget = %v, want 200 — body %q", body.BuzzBudget, string(raw))
	}
	if len(body.Scopes) != 1 || body.Scopes[0] != "ai:write:budgeted" {
		t.Errorf("scopes = %v, want [ai:write:budgeted] — body %q", body.Scopes, string(raw))
	}
}

// TestDevTokenValidationDetail exercises the 400-detail extractor directly,
// including the shapes that must NOT produce a dangling parenthetical.
func TestDevTokenValidationDetail(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			"a rejected budget names the field and the reason",
			`{"message":"Invalid request body","details":{"formErrors":[],"fieldErrors":{"buzzBudget":["Too small: expected number to be >0"]}}}`,
			"buzzBudget: Too small: expected number to be >0",
		},
		{
			"multiple messages on one field are joined",
			`{"details":{"fieldErrors":{"buzzBudget":["a","b"]}}}`,
			"buzzBudget: a; b",
		},
		{
			"multiple fields are reported in a stable order",
			`{"details":{"fieldErrors":{"slug":["bad slug"],"buzzBudget":["bad budget"]}}}`,
			"buzzBudget: bad budget | slug: bad slug",
		},
		{"form-level errors are carried too", `{"details":{"formErrors":["one of appBlockId or slug is required"]}}`,
			"one of appBlockId or slug is required"},
		{"no details member yields nothing", `{"message":"Invalid request body"}`, ""},
		{"an empty fieldErrors map yields nothing", `{"details":{"fieldErrors":{}}}`, ""},
		{"a field with no messages yields nothing", `{"details":{"fieldErrors":{"buzzBudget":[]}}}`, ""},
		{"a non-JSON body yields nothing", `<html>502</html>`, ""},
		{"a JSON array body yields nothing", `[1,2,3]`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := devTokenValidationDetail([]byte(tc.body)); got != tc.want {
				t.Errorf("devTokenValidationDetail = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestMintDevToken400SurfacesServerMessage: a 400 must carry the server's own
// verdict — the generic message AND the per-field reason — and classify as a
// usage-class failure so the process exit code matches a bad invocation.
func TestMintDevToken400SurfacesServerMessage(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		want     []string
		dontWant []string
	}{
		{
			name: "zod field detail is surfaced",
			body: `{"message":"Invalid request body","details":{"formErrors":[],"fieldErrors":{"buzzBudget":["Too small: expected number to be >0"]}}}`,
			want: []string{"400", "Invalid request body", "buzzBudget", "Too small"},
		},
		{
			name: "a detail-free 400 still reports the message, with no empty tail",
			body: `{"message":"Invalid request body"}`,
			want: []string{"400", "Invalid request body"},
			// The em-dash only introduces a detail; without one it must not appear.
			dontWant: []string{"—"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			_, err := New(srv.URL, "tok", "").MintDevToken(context.Background(), "my-block", nil, nil)
			if err == nil {
				t.Fatal("expected a 400 error")
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("400 error %q should contain %q", err.Error(), w)
				}
			}
			for _, w := range tc.dontWant {
				if strings.Contains(err.Error(), w) {
					t.Errorf("400 error %q should NOT contain %q", err.Error(), w)
				}
			}
			if !errors.Is(err, civitai.ErrBadRequest) {
				t.Errorf("a 400 must classify as ErrBadRequest, got %T: %v", err, err)
			}
			// A 400 is NOT a slug collision — it must never trigger the
			// auto-rename retry loop that the 404 sentinel drives.
			if errors.Is(err, ErrSlugRegisteredToOtherAccount) {
				t.Error("a 400 must not wrap the rename-retry sentinel")
			}
		})
	}
}

// TestDevBuzzBudgetBoundsMirrorServer pins the vendored literals. It cannot see
// the server, so it is a change-detector by design: whoever edits a bound is
// forced through this test and its pointer to the file the numbers came from.
// The relationships below are what actually make the values usable, so they are
// asserted rather than left implicit.
func TestDevBuzzBudgetBoundsMirrorServer(t *testing.T) {
	// civitai/civitai src/server/services/blocks/dev-scoped-mint.service.ts
	//   export const DEV_BUZZ_BUDGET_CAP = 250;
	//   export const DEV_BUZZ_BUDGET_DEFAULT = 50;
	// and the route's schema field `buzzBudget: z.number().int().positive()`.
	if DevBuzzBudgetCap != 250 {
		t.Errorf("DevBuzzBudgetCap = %d, want 250 (DEV_BUZZ_BUDGET_CAP)", DevBuzzBudgetCap)
	}
	if DevBuzzBudgetDefault != 50 {
		t.Errorf("DevBuzzBudgetDefault = %d, want 50 (DEV_BUZZ_BUDGET_DEFAULT)", DevBuzzBudgetDefault)
	}
	if DevBuzzBudgetMin != 1 {
		t.Errorf("DevBuzzBudgetMin = %d, want 1 (zod .positive() on an int)", DevBuzzBudgetMin)
	}
	if DevBuzzBudgetDefault > DevBuzzBudgetCap || DevBuzzBudgetDefault < DevBuzzBudgetMin {
		t.Errorf("the default (%d) must lie inside [%d, %d] — otherwise omitting --budget "+
			"yields a budget the CLI would reject if asked for explicitly",
			DevBuzzBudgetDefault, DevBuzzBudgetMin, DevBuzzBudgetCap)
	}
}
