package appapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// devTokenSpendKey decodes rec-style raw bytes as a generic object and reports
// the `requestBudgetedSpend` member plus whether the KEY was present at all.
//
// Reading the raw body is the whole point, and for THIS field it matters more
// than for buzzBudget: the server resolves an absent key as `?? true`, so
// "absent" and "false" are OPPOSITE instructions that a decoded
// `struct{RequestBudgetedSpend bool}` renders identical. Only the serialized
// bytes can tell a stated "no" from an unstated "yes".
func devTokenSpendKey(t *testing.T, raw []byte) (any, bool) {
	t.Helper()
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("request body is not a JSON object (%v): %q", err, string(raw))
	}
	v, ok := generic["requestBudgetedSpend"]
	return v, ok
}

// devTokenSpendBool is devTokenSpendKey with the presence + JSON-type checks
// applied, so callers assert on a plain bool and a type violation reports itself
// as one instead of surfacing as a confusing value mismatch.
func devTokenSpendBool(t *testing.T, raw []byte) bool {
	t.Helper()
	v, present := devTokenSpendKey(t, raw)
	if !present {
		t.Fatalf("body must ALWAYS carry a requestBudgetedSpend key (absent is read by the "+
			"server as `?? true`) — got %q", string(raw))
	}
	b, isBool := v.(bool)
	if !isBool {
		t.Fatalf("requestBudgetedSpend must be a JSON boolean, got %v (%T) — body %q",
			v, v, string(raw))
	}
	return b
}

// TestDevTokenBodyZeroValueStatesFalse is the tightest possible guard on the
// `omitempty` decision, asserted on the struct's own marshalling rather than
// through a request.
//
// 🔴 Adding `,omitempty` to RequestBudgetedSpend compiles, passes review, and
// INVERTS the field's meaning: `false` would serialize to nothing, and the
// server reads an absent field as `spendRequested = true` (the #3703 step-1
// `?? true` default). The CLI would say "no spend" and be heard as "yes".
// The zero value must therefore appear ON THE WIRE.
func TestDevTokenBodyZeroValueStatesFalse(t *testing.T) {
	raw, err := json.Marshal(devTokenBody{Slug: "my-block"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"requestBudgetedSpend":false`) {
		t.Errorf("the zero-valued body must carry requestBudgetedSpend:false — got %q.\n"+
			"An absent key is read by the server as `?? true`, i.e. the OPPOSITE of what a "+
			"false Go field means. Do not add `omitempty` to this field.", string(raw))
	}
	// The two neighbouring optional fields keep their omitempty — this test must
	// not be read as licence to serialize them too.
	if strings.Contains(string(raw), "buzzBudget") || strings.Contains(string(raw), "scopes") {
		t.Errorf("scopes/buzzBudget must stay omitempty; got %q", string(raw))
	}
}

// TestMintDevTokenSendsSpendIntentAlways pins the outgoing wire shape of the
// spend-intent field for both values, and independently of the other two
// optional fields.
//
// The `present` assertion and the `want` assertion are BOTH load-bearing and
// they fail for different reasons: presence dies to an `omitempty`/deleted
// field, the value dies to an inverted or hardcoded flag.
func TestMintDevTokenSendsSpendIntentAlways(t *testing.T) {
	budget := 200
	cases := []struct {
		name   string
		scopes []string
		budget *int
		intent bool
		want   bool
	}{
		{"no scopes, no budget, no spend asked", nil, nil, false, false},
		{"no scopes, no budget, spend asked", []string{"ai:write:budgeted"}, nil, true, true},
		{"narrowed scopes without spend", []string{"user:read:self"}, nil, false, false},
		{"budget present must not suppress the intent", []string{"ai:write:budgeted"}, &budget, true, true},
		{"budget present must not invent an intent", []string{"user:read:self"}, &budget, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var raw []byte
			srv := budgetRecorder(t, &raw)
			defer srv.Close()

			if _, err := New(srv.URL, "tok", "").MintDevToken(
				context.Background(), "my-block", tc.scopes, tc.budget, tc.intent); err != nil {
				t.Fatalf("MintDevToken: %v", err)
			}
			got := devTokenSpendBool(t, raw)
			if got != tc.want {
				t.Errorf("requestBudgetedSpend = %v, want %v — body %q", got, tc.want, string(raw))
			}
			// A bare JSON literal, not a quoted string: the route's schema is
			// z.boolean() and 400s on "false".
			if !strings.Contains(string(raw), fmt.Sprintf(`"requestBudgetedSpend":%v`, tc.want)) {
				t.Errorf("requestBudgetedSpend must serialize as a bare JSON boolean, got %q", string(raw))
			}
		})
	}
}

// TestMintDevTokenSpendIntentIsNotDerivedFromScopes: MintDevToken must send what
// it is TOLD, not what it can infer. Reconciling the two client-side would hide
// a caller bug that the cmd-layer agreement test is there to catch — the
// transport must stay a faithful mirror of the caller's argument.
func TestMintDevTokenSpendIntentIsNotDerivedFromScopes(t *testing.T) {
	// Deliberately incoherent input: scopes ask for spend, the intent says no.
	// The wire must show exactly that, so the disagreement is OBSERVABLE.
	var raw []byte
	srv := budgetRecorder(t, &raw)
	defer srv.Close()

	if _, err := New(srv.URL, "tok", "").MintDevToken(
		context.Background(), "my-block", []string{"ai:write:budgeted"}, nil, false); err != nil {
		t.Fatalf("MintDevToken: %v", err)
	}
	if devTokenSpendBool(t, raw) {
		t.Errorf("MintDevToken must send the intent it was given (false), not one inferred from "+
			"scopes; got true — body %q", string(raw))
	}
}
