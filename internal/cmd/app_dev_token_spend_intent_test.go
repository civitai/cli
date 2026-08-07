package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// budgetedScopeOnWire is the scope the two halves of the request have to agree
// about. Spelled out here rather than referencing the package constant so a
// rename of the constant cannot silently retarget these assertions at a string
// nobody sends.
const budgetedScopeOnWire = "ai:write:budgeted"

// devTokenIntentKey reports the `requestBudgetedSpend` member of the recorded
// body and whether the KEY was present at all.
//
// 🔴 Reading the raw bytes is not optional here. The server resolves an ABSENT
// key as `spendRequested = true` (#3703 step 1's `?? true`), so "absent" and
// "false" are OPPOSITE instructions — and a decoded `struct{… bool}` maps both
// to the same Go value. A test written against a decoded struct cannot see the
// bug this field exists to prevent.
func devTokenIntentKey(t *testing.T, rec *devTokenRec) (any, bool) {
	t.Helper()
	var generic map[string]any
	if err := json.Unmarshal(rec.rawBody, &generic); err != nil {
		t.Fatalf("request body is not a JSON object (%v): %q", err, string(rec.rawBody))
	}
	v, ok := generic["requestBudgetedSpend"]
	return v, ok
}

// devTokenIntentBool is devTokenIntentKey with the presence + JSON-type checks
// already applied, so callers assert on a plain bool.
func devTokenIntentBool(t *testing.T, rec *devTokenRec) bool {
	t.Helper()
	v, present := devTokenIntentKey(t, rec)
	if !present {
		t.Fatalf("the request body must ALWAYS carry a requestBudgetedSpend key — the server reads "+
			"an absent field as `?? true`, so omitting it states the OPPOSITE of a false Go value. "+
			"Body: %q", string(rec.rawBody))
	}
	b, isBool := v.(bool)
	if !isBool {
		t.Fatalf("requestBudgetedSpend must be a JSON boolean (the route's schema is z.boolean()), "+
			"got %v (%T) — body %q", v, v, string(rec.rawBody))
	}
	return b
}

// mintWithManifest runs `app dev-token my-block [args...]` against a recording
// server, from a temp CWD holding the given block.manifest.json scopes.
// manifestScopesJSON == nil means NO block.manifest.json at all (the case the
// `scopes` allow-list cannot express — see devTokenRequestScopes).
func mintWithManifest(t *testing.T, manifestScopesJSON *string, args ...string) *devTokenRec {
	t.Helper()
	dir := t.TempDir()
	if manifestScopesJSON != nil {
		body := `{"blockId":"my-block","version":"1.0.0","name":"My Block","scopes":` + *manifestScopesJSON + `}`
		if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)

	rec := &devTokenRec{}
	srv := devTokenServer(t, map[string]any{"token": "jwt-x"}, http.StatusOK, rec)
	t.Cleanup(srv.Close)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	full := append([]string{"app", "dev-token", "my-block"}, args...)
	if _, _, err := run(t, full...); err != nil {
		t.Fatalf("civitai %s: %v", strings.Join(full, " "), err)
	}
	if len(rec.rawBody) == 0 {
		t.Fatalf("the command sent no dev-token request at all — nothing to assert on")
	}
	return rec
}

// devTokenSpendMatrix is the full manifest × --spend cross-product. Both tests
// below walk the SAME matrix, deliberately: one pins each half's value, the
// other pins the relationship between them. A row added here is exercised by
// both.
var devTokenSpendMatrix = []struct {
	name string
	// manifest is the `scopes` array literal, or nil for "no manifest file".
	manifest *string
	spend    bool
	// wantIntent is the requestBudgetedSpend value expected on the wire.
	wantIntent bool
}{
	{"no manifest, no --spend", nil, false, false},
	{"no manifest, --spend", nil, true, true},
	{"manifest declares spend, no --spend", ptr(`["user:read:self","ai:write:budgeted"]`), false, false},
	{"manifest declares spend, --spend", ptr(`["user:read:self","ai:write:budgeted"]`), true, true},
	{"manifest declares ONLY spend, no --spend", ptr(`["ai:write:budgeted"]`), false, false},
	{"manifest declares ONLY spend, --spend", ptr(`["ai:write:budgeted"]`), true, true},
	{"manifest without spend, no --spend", ptr(`["user:read:self","apps:storage:read"]`), false, false},
	{"manifest without spend, --spend", ptr(`["user:read:self","apps:storage:read"]`), true, true},
	{"empty manifest scopes, no --spend", ptr(`[]`), false, false},
	{"empty manifest scopes, --spend", ptr(`[]`), true, true},
}

func ptr(s string) *string { return &s }

// TestAppDevTokenSpendIntentWireShape asserts the SERIALIZED value of
// requestBudgetedSpend for every manifest × flag combination.
//
// RED at 3c10afc: the field did not exist, so every row failed the presence
// check ("the request body must ALWAYS carry a requestBudgetedSpend key").
func TestAppDevTokenSpendIntentWireShape(t *testing.T) {
	for _, tc := range devTokenSpendMatrix {
		t.Run(tc.name, func(t *testing.T) {
			var args []string
			if tc.spend {
				args = append(args, "--spend")
			}
			rec := mintWithManifest(t, tc.manifest, args...)
			if got := devTokenIntentBool(t, rec); got != tc.wantIntent {
				t.Errorf("requestBudgetedSpend = %v, want %v (--spend=%v) — body %q",
					got, tc.wantIntent, tc.spend, string(rec.rawBody))
			}
			// Bare JSON literal, not a quoted string: the route's schema is
			// z.boolean() and rejects "false" outright.
			if !strings.Contains(string(rec.rawBody), fmt.Sprintf(`"requestBudgetedSpend":%v`, tc.wantIntent)) {
				t.Errorf("requestBudgetedSpend must serialize as a bare JSON boolean, got %q",
					string(rec.rawBody))
			}
		})
	}
}

// TestAppDevTokenSpendIntentAndScopesAgree is the SEAM test, and the reason the
// two halves are not tested only in isolation.
//
// The request states its spend intent TWICE, in two encodings that a reader has
// to reconcile: the `scopes` allow-list (which works against a server that has
// not flipped its `?? true` default) and `requestBudgetedSpend` (which works
// after). A change that updates one and forgets the other type-checks, passes
// every single-half test, and ships a request that asks for spend while
// stripping the scope — or the reverse.
//
// The invariant is a BICONDITIONAL, asserted on the wire:
//
//	requestBudgetedSpend == true  ⟺  scopes contains ai:write:budgeted
//
// (an absent `scopes` key contains nothing). Note this test derives BOTH sides
// from the recorded body and compares them to EACH OTHER — it never consults
// wantIntent — so it stays meaningful even if the expectations in the sibling
// test are wrong, and it fires for a desynchronisation in either direction.
//
// RED/GREEN, stated honestly: all 10 rows fail at 3c10afc, but they fail in the
// PRESENCE precondition (there was no field to read), not in the biconditional —
// at base the two halves could not disagree because only one existed. So treat
// this as an INVARIANT GUARD against a future desync rather than as regression
// coverage for a desync that ever shipped. Its killing mutations are the
// deliberate desynchronisations in the commit's mutation matrix, not the absence
// of the field.
func TestAppDevTokenSpendIntentAndScopesAgree(t *testing.T) {
	for _, tc := range devTokenSpendMatrix {
		t.Run(tc.name, func(t *testing.T) {
			var args []string
			if tc.spend {
				args = append(args, "--spend")
			}
			rec := mintWithManifest(t, tc.manifest, args...)

			intent := devTokenIntentBool(t, rec)
			scopesAsk := slices.Contains(rec.scopes, budgetedScopeOnWire)

			if intent != scopesAsk {
				t.Errorf("the two halves of the request DISAGREE about budgeted spend: "+
					"requestBudgetedSpend=%v but scopes%s contain %q (scopes=%#v). "+
					"Both must derive from the same --spend value; one asking while the other "+
					"strips is the desynchronisation this test exists to catch. Body: %q",
					intent, map[bool]string{true: " DO", false: " do NOT"}[scopesAsk],
					budgetedScopeOnWire, rec.scopes, string(rec.rawBody))
			}
			// Cross-check against the raw bytes: rec.scopes is a decoded []string
			// and could in principle agree while the scope rides along elsewhere
			// in the body. The wire is the contract.
			if mentioned := strings.Contains(string(rec.rawBody), budgetedScopeOnWire); mentioned != intent {
				t.Errorf("the raw body %s mention %q while requestBudgetedSpend=%v — body %q",
					map[bool]string{true: "DOES", false: "does NOT"}[mentioned],
					budgetedScopeOnWire, intent, string(rec.rawBody))
			}
		})
	}
}

// TestAppDevTokenSpendIntentSurvivesRename: the anti-shadow 404 retries the mint
// against a renamed slug. A rename changes WHICH slug is minted for and nothing
// else — the retry must carry the same spend intent, or `--spend` becomes a
// coin-flip on a colliding slug.
func TestAppDevTokenSpendIntentSurvivesRename(t *testing.T) {
	for _, spend := range []bool{false, true} {
		name := "no --spend"
		if spend {
			name = "--spend"
		}
		t.Run(name, func(t *testing.T) {
			var bodies [][]byte
			srv := devTokenIntentRenameServer(t, map[string]bool{"my-block": true}, &bodies)
			defer srv.Close()

			prev := devTokenSuffixGen
			devTokenSuffixGen = func() string { return "abc12" }
			defer func() { devTokenSuffixGen = prev }()

			dir := t.TempDir()
			writeDevTokenManifest(t, dir, `{
  "blockId": "my-block",
  "version": "0.1.0",
  "name": "My Block",
  "scopes": ["identity:read"]
}`)
			chdir(t, dir)

			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("CIVITAI_TOKEN", "tok")
			t.Setenv("CIVITAI_BASE_URL", srv.URL)

			args := []string{"app", "dev-token", "my-block"}
			if spend {
				args = append(args, "--spend")
			}
			if _, _, err := run(t, args...); err != nil {
				t.Fatalf("app dev-token: %v", err)
			}
			if len(bodies) < 2 {
				t.Fatalf("expected a collision + a retry (2 mint requests), got %d", len(bodies))
			}
			for i, raw := range bodies {
				rec := &devTokenRec{rawBody: raw}
				if got := devTokenIntentBool(t, rec); got != spend {
					t.Errorf("request %d/%d sent requestBudgetedSpend=%v, want %v — body %q",
						i+1, len(bodies), got, spend, string(raw))
				}
			}
		})
	}
}

// devTokenIntentRenameServer is devTokenRenameServer with every request BODY
// recorded (the sibling only counts them), so the retry's payload is assertable.
func devTokenIntentRenameServer(t *testing.T, collide map[string]bool, bodies *[][]byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/me") {
			_ = json.NewEncoder(w).Encode(map[string]any{"username": "u", "id": 1, "tokenScope": 0})
			return
		}
		raw, _ := io.ReadAll(r.Body)
		*bodies = append(*bodies, raw)
		var body struct {
			Slug string `json:"slug"`
		}
		_ = json.Unmarshal(raw, &body)
		if collide[body.Slug] {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "App not found"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "jwt-x"})
	}))
}
