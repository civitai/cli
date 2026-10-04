package validate

import (
	"strings"
	"testing"
)

func TestToNumber(t *testing.T) {
	cases := []struct {
		in   any
		val  float64
		ok   bool
		name string
	}{
		{float64(42), 42, true, "float64"},
		{int(7), 7, true, "int"},
		{int64(9), 9, true, "int64"},
		{"40", 0, false, "string"},
		{true, 0, false, "bool"},
		{nil, 0, false, "nil"},
	}
	for _, tc := range cases {
		got, ok := toNumber(tc.in)
		if ok != tc.ok || (ok && got != tc.val) {
			t.Errorf("toNumber(%v [%s]) = %v,%v want %v,%v", tc.in, tc.name, got, ok, tc.val, tc.ok)
		}
	}
}

func TestSemanticChecksNonMap(t *testing.T) {
	if errs := semanticChecks([]any{1, 2}); errs != nil {
		t.Errorf("semanticChecks on non-map should be nil, got %v", errs)
	}
	if errs := targetChecks("not-a-map"); errs != nil {
		t.Errorf("targetChecks on non-map should be nil, got %v", errs)
	}
}

// TestUnknownSlotErrorEnumeratesKnownSlots asserts the unknown-slot error is
// self-documenting like the sibling scope enum error — it must append the full
// list of known slotIds so the developer sees the valid choices, phrased
// "value must be one of '…', '…'" (single-quoted, matching the JSON Schema
// enum error the scope check produces).
func TestUnknownSlotErrorEnumeratesKnownSlots(t *testing.T) {
	errs := targetChecks(map[string]any{
		"targets": []any{
			map[string]any{"slotId": "model.totally-made-up-slot"},
		},
	})
	if len(errs) != 1 {
		t.Fatalf("want exactly 1 error, got %d: %v", len(errs), errs)
	}
	got := errs[0].Message
	if !strings.Contains(got, "value must be one of ") {
		t.Errorf("error missing the enum phrasing: %q", got)
	}
	// Every known slotId must be enumerated, single-quoted.
	for id := range vendoredSlotIDs {
		if !strings.Contains(got, "'"+id+"'") {
			t.Errorf("error does not enumerate known slot %q: %q", id, got)
		}
	}
}

func TestIframeRequiredFieldsBadTypes(t *testing.T) {
	errs := iframeRequiredFields(map[string]any{
		"minHeight": "tall",  // not a number
		"resizable": "maybe", // not a bool
	})
	if len(errs) != 2 {
		t.Errorf("expected 2 errors for bad iframe field types, got %v", errs)
	}
}

func TestScopeJustificationChecks(t *testing.T) {
	cases := []struct {
		name    string
		generic map[string]any
		want    []string
	}{
		{
			name: "key present in scopes → no error (control)",
			generic: map[string]any{
				"scopes":              []any{"user:read:self"},
				"scopeJustifications": map[string]any{"user:read:self": "needed to greet the user"},
			},
			want: nil,
		},
		{
			name: "key not in scopes → specific error naming the key",
			generic: map[string]any{
				"scopes":              []any{"user:read:self"},
				"scopeJustifications": map[string]any{"buzz:read:self": "why I want buzz"},
			},
			want: []string{
				`scopeJustifications key "buzz:read:self" is not one of the manifest's declared scopes`,
			},
		},
		{
			name: "multiple unknown keys → one error each",
			generic: map[string]any{
				"scopes": []any{"user:read:self"},
				"scopeJustifications": map[string]any{
					"buzz:read:self":     "a",
					"media:read:owned":   "b",
					"apps:storage:write": "c",
				},
			},
			want: []string{
				`scopeJustifications key "apps:storage:write" is not one of the manifest's declared scopes`,
				`scopeJustifications key "buzz:read:self" is not one of the manifest's declared scopes`,
				`scopeJustifications key "media:read:owned" is not one of the manifest's declared scopes`,
			},
		},
		{
			name: "mix of known and unknown → only unknown errors",
			generic: map[string]any{
				"scopes": []any{"user:read:self", "buzz:read:self"},
				"scopeJustifications": map[string]any{
					"user:read:self":   "known-ok",
					"media:read:owned": "unknown",
				},
			},
			want: []string{
				`scopeJustifications key "media:read:owned" is not one of the manifest's declared scopes`,
			},
		},
		{
			name:    "scopeJustifications absent → no error",
			generic: map[string]any{"scopes": []any{"user:read:self"}},
			want:    nil,
		},
		{
			name: "empty scopeJustifications map → no error",
			generic: map[string]any{
				"scopes":              []any{"user:read:self"},
				"scopeJustifications": map[string]any{},
			},
			want: nil,
		},
		{
			name: "case matters — exact match only, no case-fold",
			generic: map[string]any{
				"scopes":              []any{"user:read:self"},
				"scopeJustifications": map[string]any{"User:Read:Self": "wrong case"},
			},
			want: []string{
				`scopeJustifications key "User:Read:Self" is not one of the manifest's declared scopes`,
			},
		},
		{
			name: "wrong-typed scopeJustifications (schema-handled) → no error",
			generic: map[string]any{
				"scopes":              []any{"user:read:self"},
				"scopeJustifications": "not-an-object",
			},
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scopeJustificationChecks(tc.generic)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d errors %v, want %d %v", len(got), got, len(tc.want), tc.want)
			}
			for i := range tc.want {
				if got[i].Message != tc.want[i] {
					t.Errorf("error[%d] = %q, want %q", i, got[i].Message, tc.want[i])
				}
			}
		})
	}
}

// TestSensitiveScopeJustificationChecks mirrors the backend enforcement: a
// declared SENSITIVE scope without a non-empty scopeJustifications entry is a
// hard error naming every offender, byte-identical to the server's submit-time
// 400 message. Compliant manifests (non-sensitive scopes, or sensitive-with-
// justification) produce no error.
func TestSensitiveScopeJustificationChecks(t *testing.T) {
	const prefix = "sensitive scopes require a justification — add a non-empty scopeJustifications entry for: "
	cases := []struct {
		name    string
		generic map[string]any
		want    []string
	}{
		{
			name: "sensitive scope, no justifications at all → hard error naming it",
			generic: map[string]any{
				"scopes": []any{"ai:write:budgeted"},
			},
			want: []string{prefix + "ai:write:budgeted"},
		},
		{
			name: "sensitive scope, empty scopeJustifications map → hard error",
			generic: map[string]any{
				"scopes":              []any{"buzz:read:self"},
				"scopeJustifications": map[string]any{},
			},
			want: []string{prefix + "buzz:read:self"},
		},
		{
			name: "sensitive scope with a whitespace-only justification → still unjustified",
			generic: map[string]any{
				"scopes":              []any{"social:tip:self"},
				"scopeJustifications": map[string]any{"social:tip:self": "   "},
			},
			want: []string{prefix + "social:tip:self"},
		},
		{
			name: "sensitive scope with a non-string justification → still unjustified",
			generic: map[string]any{
				"scopes":              []any{"apps:storage:shared:write"},
				"scopeJustifications": map[string]any{"apps:storage:shared:write": 123},
			},
			want: []string{prefix + "apps:storage:shared:write"},
		},
		{
			name: "sensitive scope WITH a real justification → no error",
			generic: map[string]any{
				"scopes":              []any{"ai:write:budgeted"},
				"scopeJustifications": map[string]any{"ai:write:budgeted": "needed to run generations for the user"},
			},
			want: nil,
		},
		{
			name: "non-sensitive scope, no justification → no error",
			generic: map[string]any{
				"scopes": []any{"user:read:self", "apps:storage:shared:read"},
			},
			want: nil,
		},
		{
			// goods:purchase:self spends the viewer's Buzz. Membership in
			// SENSITIVE_BLOCK_SCOPES is asserted by TestIsSensitiveBlockScope;
			// this is the BEHAVIOURAL half — that being in the set actually
			// reaches a finding through unjustifiedSensitiveScopes. The two are
			// different claims and the set can be right while the seam is not.
			name: "goods:purchase:self unjustified → hard error",
			generic: map[string]any{
				"scopes": []any{"goods:purchase:self"},
			},
			want: []string{prefix + "goods:purchase:self"},
		},
		{
			// The PAIR case, and the one that would catch goods:read:self being
			// wrongly added to the sensitive set: both goods scopes declared,
			// only the purchase half justified ⇒ NO error. If the read half
			// were sensitive this row fails, naming it.
			name: "goods:read:self alongside a justified purchase → no error",
			generic: map[string]any{
				"scopes": []any{"goods:read:self", "goods:purchase:self"},
				"scopeJustifications": map[string]any{
					"goods:purchase:self": "lets the viewer buy the extra credits this app sells",
				},
			},
			want: nil,
		},
		{
			// posts:write:self publishes a PUBLIC post under the viewer's byline.
			// It must behave like every other sensitive scope: unjustified is a
			// hard local failure, matching the 400 the server would return.
			name: "posts:write:self unjustified → hard error",
			generic: map[string]any{
				"scopes": []any{"posts:write:self"},
			},
			want: []string{prefix + "posts:write:self"},
		},
		{
			name: "posts:write:self WITH a real justification → no error",
			generic: map[string]any{
				"scopes": []any{"posts:write:self"},
				"scopeJustifications": map[string]any{
					"posts:write:self": "lets the viewer publish the images this app generated for them",
				},
			},
			want: nil,
		},
		{
			name: "multiple sensitive, some unjustified → lists all offenders in declaration order",
			generic: map[string]any{
				"scopes": []any{
					"ai:write:budgeted", "user:read:self", "buzz:read:self",
					"collections:read:private", "apps:storage:shared:write",
				},
				"scopeJustifications": map[string]any{
					// buzz:read:self is justified; the other three are not.
					"buzz:read:self": "read balance to show the user their spend",
				},
			},
			want: []string{prefix + "ai:write:budgeted, collections:read:private, apps:storage:shared:write"},
		},
		{
			name: "duplicate sensitive scope, unjustified → named once",
			generic: map[string]any{
				"scopes": []any{"ai:write:budgeted", "ai:write:budgeted"},
			},
			want: []string{prefix + "ai:write:budgeted"},
		},
		{
			name:    "no scopes at all → no error",
			generic: map[string]any{},
			want:    nil,
		},
		{
			name: "non-array scopes → no error (schema-handled)",
			generic: map[string]any{
				"scopes": "not-an-array",
			},
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sensitiveScopeJustificationChecks(tc.generic)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d errors %v, want %d %v", len(got), got, len(tc.want), tc.want)
			}
			for i := range tc.want {
				if got[i].Message != tc.want[i] {
					t.Errorf("error[%d] = %q, want %q", i, got[i].Message, tc.want[i])
				}
			}
		})
	}
}

// TestIsSensitiveBlockScope pins the sensitive set to the server's 7 scopes.
//
// This list is spelled out rather than derived, and that is correct even though
// the sibling enum guard in pattern_test.go now derives its content: the
// sensitive set is NOT in the vendored schema. It mirrors a separate server
// constant (SENSITIVE_BLOCK_SCOPES in block-scope.constants.ts), so there is
// nothing local to derive it from, and a hand-kept list is the honest shape.
// The cost is that a scope turning sensitive upstream is silent here until
// someone checks — re-read the DEPLOYED constant (civitai `origin/release`; the
// site serves `release`, not `main`) when the schema gains a scope.
func TestIsSensitiveBlockScope(t *testing.T) {
	sensitive := []string{
		"ai:write:budgeted", "social:tip:self", "buzz:read:self",
		"collections:read:private", "apps:storage:shared:write",
		"posts:write:self",
		// Spends the viewer's Buzz on the app's own catalog.
		"goods:purchase:self",
	}
	if len(SENSITIVE_BLOCK_SCOPES) != len(sensitive) {
		t.Fatalf("SENSITIVE_BLOCK_SCOPES has %d entries, want %d", len(SENSITIVE_BLOCK_SCOPES), len(sensitive))
	}
	for _, s := range sensitive {
		if !isSensitiveBlockScope(s) {
			t.Errorf("isSensitiveBlockScope(%q) = false, want true", s)
		}
	}
	// goods:read:self is the NEAR MISS and the reason it is listed: it arrived in
	// the same canonical change as goods:purchase:self and reads like its twin,
	// but the server scopes it to the calling app's own sales, so treating it as
	// sensitive would demand a justification the server does not ask for.
	for _, s := range []string{"user:read:self", "apps:storage:shared:read", "collections:read:self", "goods:read:self", "AI:WRITE:BUDGETED"} {
		if isSensitiveBlockScope(s) {
			t.Errorf("isSensitiveBlockScope(%q) = true, want false", s)
		}
	}
}

func TestSandboxChecksNonStringIgnored(t *testing.T) {
	if errs := sandboxChecks(map[string]any{"sandbox": 123}); errs != nil {
		t.Errorf("non-string sandbox is schema-handled, got %v", errs)
	}
	if errs := sandboxChecks(map[string]any{}); errs != nil {
		t.Errorf("absent sandbox should yield no semantic errors, got %v", errs)
	}
	if errs := sandboxChecks(map[string]any{"sandbox": "   "}); len(errs) == 0 {
		t.Error("empty/whitespace sandbox should error")
	}
}

// TestGoodsChecksMirrorTheServerRulesJSONSchemaCannotExpress pins the three
// `goods` rules that live in `parseManifestGoods` and nowhere in the vendored
// schema, with the server's own message text.
//
// 🔴 EVERY `want` HERE IS SPELLED OUT, NOT BUILT FROM THE CODE UNDER TEST. These
// strings are a contract with the server's 400: the whole value of the mirror is
// that an author sees the same sentence locally, so a `want` derived from
// `goodsChecks` would move with a reword and assert nothing.
func TestGoodsChecksMirrorTheServerRulesJSONSchemaCannotExpress(t *testing.T) {
	good := func(id, title string) map[string]any {
		return map[string]any{"id": id, "title": title, "priceBuzz": float64(10)}
	}
	// A payload whose SERIALIZED size is what matters: two short keys and one
	// long value. Deliberately not a fixture whose length equals the bound, so
	// a mutant that hardcodes 2048 cannot survive on it.
	bigPayload := map[string]any{"note": strings.Repeat("x", 4096)}
	okPayload := map[string]any{"note": strings.Repeat("x", 100)}

	cases := []struct {
		name string
		in   map[string]any
		want []string
	}{
		{
			name: "duplicate id → the server's duplicate message, on the SECOND entry",
			in:   map[string]any{"goods": []any{good("credits", "A"), good("credits", "B")}},
			want: []string{`goods[1].id duplicates an earlier good id ("credits")`},
		},
		{
			name: "three entries, the third repeats the first → indexed at 2",
			in:   map[string]any{"goods": []any{good("a", "A"), good("b", "B"), good("a", "C")}},
			want: []string{`goods[2].id duplicates an earlier good id ("a")`},
		},
		{
			name: "whitespace-only title → non-empty message (schema minLength:1 accepts it)",
			in:   map[string]any{"goods": []any{good("credits", "   ")}},
			want: []string{"goods[0].title must be a non-empty string"},
		},
		{
			name: "absent title → same message",
			in:   map[string]any{"goods": []any{map[string]any{"id": "credits", "priceBuzz": float64(10)}}},
			want: []string{"goods[0].title must be a non-empty string"},
		},
		{
			name: "payload over the serialized bound → byte message",
			in: map[string]any{"goods": []any{
				map[string]any{"id": "credits", "title": "A", "priceBuzz": float64(10), "payload": bigPayload},
			}},
			want: []string{"goods[0].payload must serialize to at most 2048 bytes"},
		},
		// ---- negative controls: each rule must ACCEPT its legal neighbour ----
		{
			name: "distinct ids → nothing",
			in:   map[string]any{"goods": []any{good("credits", "A"), good("boosts", "B")}},
			want: nil,
		},
		{
			name: "a title with surrounding space but real content → nothing",
			in:   map[string]any{"goods": []any{good("credits", "  Extra credits  ")}},
			want: nil,
		},
		{
			name: "a payload well under the bound → nothing",
			in: map[string]any{"goods": []any{
				map[string]any{"id": "credits", "title": "A", "priceBuzz": float64(10), "payload": okPayload},
			}},
			want: nil,
		},
		{
			name: "no goods key at all sells nothing and is valid",
			in:   map[string]any{},
			want: nil,
		},
		{
			name: "a non-array goods is the schema's to reject, not ours",
			in:   map[string]any{"goods": "nope"},
			want: nil,
		},
		{
			name: "an entry that is not an object is the schema's too",
			in:   map[string]any{"goods": []any{"nope"}},
			want: nil,
		},
		{
			name: "a non-string id cannot key a duplicate test",
			in:   map[string]any{"goods": []any{map[string]any{"id": float64(7), "title": "A"}}},
			want: nil,
		},
		// ---- the one-finding-per-entry contract ----
		{
			// The server's forEach RETURNS on the duplicate, so the blank title
			// of the same entry is never reported. Two findings here would make
			// the local set differ from the 400.
			name: "duplicate id AND blank title on one entry → only the duplicate",
			in:   map[string]any{"goods": []any{good("credits", "A"), good("credits", "   ")}},
			want: []string{`goods[1].id duplicates an earlier good id ("credits")`},
		},
		{
			// Two independent offenders in one manifest DO both report, in
			// declaration order — the per-entry rule is not a per-manifest one.
			name: "two different entries each offend → both, in order",
			in: map[string]any{"goods": []any{
				good("credits", "   "),
				good("boosts", "B"),
				good("boosts", "C"),
			}},
			want: []string{
				"goods[0].title must be a non-empty string",
				`goods[2].id duplicates an earlier good id ("boosts")`,
			},
		},
	}

	produced := 0
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := goodsChecks(tc.in)
			produced += len(got)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d finding(s), want %d\n  got:  %v\n  want: %v",
					len(got), len(tc.want), got, tc.want)
			}
			for i := range tc.want {
				if got[i].Message != tc.want[i] {
					t.Errorf("finding[%d].Message\n  want: %s\n  got:  %s", i, tc.want[i], got[i].Message)
				}
			}
		})
	}
	// POSITIVE CONTROL for the table as a whole. Every `want: nil` row passes
	// against a goodsChecks that returns nil unconditionally, and they are the
	// majority here — so assert the table did make it emit something.
	if produced == 0 {
		t.Fatal("no case produced a finding — goodsChecks may be wired to nothing and every nil row would still pass")
	}
}

// TestAppUnlockGoodsChecksMirrorTheServerRulesTheSchemaWontExpress pins the
// three rules a `kind: "app_unlock"` good carries and an ordinary one does not.
//
// 🔴 WHY THESE BELONG IN THE CLI AT ALL. Unlike the general price range, the
// vendored schema does not reject any of them and deliberately will not: its
// `priceBuzz.maximum` stays 50000 for every kind with the narrow bound in a
// prose `description`, `justification` is absent from `required` (the
// requirement is conditional on `kind`, which a per-item subschema cannot
// state), and arity is a whole-array property. So before this, an `app_unlock`
// priced at 5001 — or a second one, or one with no justification — passed
// `civitai app validate` and was rejected at submit. A clean local verdict for a
// manifest the server refuses is the failure this closes.
//
// Every `want` is spelled out, never built from the code under test, for the
// same reason as the sibling table above: these strings are a contract with the
// server's 400.
func TestAppUnlockGoodsChecksMirrorTheServerRulesTheSchemaWontExpress(t *testing.T) {
	// 7_777 and 5_001: neither equals a bound nor is a multiple of one, so a
	// mutant hardcoding 5000 — or substituting the general 50000 ceiling — cannot
	// survive on them. 4_999/5_000 pin the boundary from below and exactly on it.
	unlock := func(id string, priceBuzz int, extra map[string]any) map[string]any {
		e := map[string]any{"id": id, "title": "Full access", "priceBuzz": float64(priceBuzz), "kind": "app_unlock"}
		for k, v := range extra {
			e[k] = v
		}
		return e
	}
	justified := map[string]any{"justification": "reviewed: unlocks the editor"}

	cases := []struct {
		name string
		in   map[string]any
		want []string
	}{
		// ---- justification: required only for app_unlock ----
		{
			name: "app_unlock with NO justification → the server's required message",
			in:   map[string]any{"goods": []any{unlock("access", 500, nil)}},
			want: []string{"goods[0].justification is required for an app_unlock good — it makes the app paid, so a moderator must be told why"},
		},
		{
			name: "an ORDINARY good with no justification → nothing (the rule is kind-keyed)",
			in:   map[string]any{"goods": []any{map[string]any{"id": "credits", "title": "Credits", "priceBuzz": float64(500)}}},
			want: nil,
		},
		{
			name: "kind omitted entirely resolves to `good` → still nothing",
			in:   map[string]any{"goods": []any{map[string]any{"id": "credits", "title": "Credits", "priceBuzz": float64(7777)}}},
			want: nil,
		},
		{
			name: "app_unlock WITH a justification → nothing",
			in:   map[string]any{"goods": []any{unlock("access", 500, justified)}},
			want: nil,
		},
		{
			name: "whitespace-only justification → non-empty message, NOT the required one (schema minLength:1 accepts it)",
			in:   map[string]any{"goods": []any{unlock("access", 500, map[string]any{"justification": "   "})}},
			want: []string{"goods[0].justification must be a non-empty string"},
		},
		{
			name: "an ORDINARY good's justification is shape-checked too (explaining an item must not be punished, but an empty explanation is still wrong)",
			in: map[string]any{"goods": []any{map[string]any{
				"id": "credits", "title": "Credits", "priceBuzz": float64(500), "justification": ""}}},
			want: []string{"goods[0].justification must be a non-empty string"},
		},

		// ---- priceBuzz: the kind's own ceiling, named in the message ----
		{
			name: "app_unlock at 5001 → the message names 5000, the bound it broke, not 50000",
			in:   map[string]any{"goods": []any{unlock("access", 5001, justified)}},
			want: []string{"goods[0].priceBuzz must be a whole number between 2 and 5000 Buzz for an app_unlock good"},
		},
		{
			name: "app_unlock at 7777 → same message (a mutant using the general ceiling passes 7777)",
			in:   map[string]any{"goods": []any{unlock("access", 7777, justified)}},
			want: []string{"goods[0].priceBuzz must be a whole number between 2 and 5000 Buzz for an app_unlock good"},
		},
		{
			name: "app_unlock exactly AT 5000 → nothing (the bound is inclusive)",
			in:   map[string]any{"goods": []any{unlock("access", 5000, justified)}},
			want: nil,
		},
		{
			name: "app_unlock at 4999 → nothing",
			in:   map[string]any{"goods": []any{unlock("access", 4999, justified)}},
			want: nil,
		},
		{
			name: "an ORDINARY good at 20000 → nothing; the general ceiling is the schema's business",
			in: map[string]any{"goods": []any{map[string]any{
				"id": "credits", "title": "Credits", "priceBuzz": float64(20000)}}},
			want: nil,
		},
		{
			// 🔴 REGRESSION GUARD for the defect a round-0 audit found. An earlier
			// draft clamped this check to 2..50000 "so as not to double-report with
			// the schema", so a 60000 unlock got ONLY the schema's
			// `maximum: got 60,000, want 50,000` — 10x the real ceiling, telling a
			// developer to come down to a bound they are still 55000 over. The
			// schema fires here too; naming the right bound twice beats naming the
			// wrong one once.
			name: "app_unlock at 60000 → still names 5000, NOT the general 50000 the schema would quote",
			in:   map[string]any{"goods": []any{unlock("access", 60000, justified)}},
			want: []string{"goods[0].priceBuzz must be a whole number between 2 and 5000 Buzz for an app_unlock good"},
		},
		{
			name: "app_unlock at 1 → below the floor, and the message states the real range",
			in:   map[string]any{"goods": []any{unlock("access", 1, justified)}},
			want: []string{"goods[0].priceBuzz must be a whole number between 2 and 5000 Buzz for an app_unlock good"},
		},
		{
			name: "app_unlock at a FRACTIONAL price → not a whole number, same message",
			in: map[string]any{"goods": []any{map[string]any{
				"id": "access", "title": "T", "priceBuzz": 500.5, "kind": "app_unlock",
				"justification": "reviewed"}}},
			want: []string{"goods[0].priceBuzz must be a whole number between 2 and 5000 Buzz for an app_unlock good"},
		},
		{
			name: "a NON-NUMERIC priceBuzz is left to the schema's `type` — nothing to mirror",
			in: map[string]any{"goods": []any{map[string]any{
				"id": "access", "title": "T", "priceBuzz": "500", "kind": "app_unlock",
				"justification": "reviewed"}}},
			want: nil,
		},

		// ---- the order contract: first hit ends the entry ----
		{
			name: "app_unlock over-priced AND unjustified → ONLY the price finding, because the server returns there",
			in:   map[string]any{"goods": []any{unlock("access", 7777, nil)}},
			want: []string{"goods[0].priceBuzz must be a whole number between 2 and 5000 Buzz for an app_unlock good"},
		},
		{
			name: "a whitespace title beats both, mirroring the server's earlier return",
			in: map[string]any{"goods": []any{map[string]any{
				"id": "access", "title": "  ", "priceBuzz": float64(7777), "kind": "app_unlock"}}},
			want: []string{"goods[0].title must be a non-empty string"},
		},

		// ---- arity: a whole-array property, path `goods`, additive ----
		{
			name: "two app_unlock goods → the arity message, at path `goods`",
			in:   map[string]any{"goods": []any{unlock("a", 500, justified), unlock("b", 500, justified)}},
			want: []string{"goods may declare at most 1 app_unlock good (found 2) — app access is one question with one answer"},
		},
		{
			name: "three app_unlock goods → the count reported is 3 (a mutant hardcoding 2 dies here)",
			in: map[string]any{"goods": []any{
				unlock("a", 500, justified), unlock("b", 500, justified), unlock("c", 500, justified)}},
			want: []string{"goods may declare at most 1 app_unlock good (found 3) — app access is one question with one answer"},
		},
		{
			name: "one app_unlock beside several ordinary goods → nothing; only unlocks are counted",
			in: map[string]any{"goods": []any{
				map[string]any{"id": "c1", "title": "C1", "priceBuzz": float64(500)},
				unlock("access", 500, justified),
				map[string]any{"id": "c2", "title": "C2", "priceBuzz": float64(500)},
			}},
			want: nil,
		},
		{
			name: "arity is ADDITIVE, not entry-terminating: a per-entry finding and the arity finding both appear",
			in: map[string]any{"goods": []any{
				unlock("a", 500, justified),
				unlock("b", 500, justified),
				unlock("c", 7777, justified),
			}},
			want: []string{
				"goods[2].priceBuzz must be a whole number between 2 and 5000 Buzz for an app_unlock good",
				"goods may declare at most 1 app_unlock good (found 2) — app access is one question with one answer",
			},
		},
		{
			name: "an entry that FAILED is not counted toward arity, mirroring the server's parsed-goods array",
			in: map[string]any{"goods": []any{
				unlock("a", 500, justified),
				unlock("b", 7777, justified), // fails on price, so it never counts
			}},
			want: []string{"goods[1].priceBuzz must be a whole number between 2 and 5000 Buzz for an app_unlock good"},
		},

		// ---- an invalid kind is the schema's enum to reject ----
		{
			name: "kind present but not a member → skipped here, so no app_unlock rule is applied to it",
			in: map[string]any{"goods": []any{map[string]any{
				"id": "access", "title": "Access", "priceBuzz": float64(7777), "kind": "app_subscription"}}},
			want: nil,
		},
		{
			// 🔴 PINS AN AUDIT FINDING THAT WAS **REJECTED**, so nobody "fixes" it
			// into a divergence. A round-0 pass read the invalid-kind skip as
			// suppressing the payload byte-size finding. It does — and the SERVER
			// does the same: its kind check returns at
			// block-goods.constants.ts:385-387, before its payload check at :464.
			// Mirroring that is this file's contract. If this case ever starts
			// reporting the payload finding, the CLI has diverged from the server.
			name: "an invalid kind suppresses the payload finding, exactly as the server's early return does",
			in: map[string]any{"goods": []any{map[string]any{
				"id": "access", "title": "T", "priceBuzz": float64(500), "kind": "nope",
				"payload": map[string]any{"note": strings.Repeat("x", 4096)}}},
			},
			want: nil,
		},
		{
			name: "the SAME oversized payload under a VALID kind DOES report — the control proving the case above is about `kind`, not about payload detection",
			in: map[string]any{"goods": []any{map[string]any{
				"id": "access", "title": "T", "priceBuzz": float64(500), "kind": "good",
				"payload": map[string]any{"note": strings.Repeat("x", 4096)}}},
			},
			want: []string{"goods[0].payload must serialize to at most 2048 bytes"},
		},
	}

	produced := 0
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := goodsChecks(tc.in)
			produced += len(got)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d finding(s), want %d\n  got:  %v\n  want: %v",
					len(got), len(tc.want), got, tc.want)
			}
			for i := range tc.want {
				if got[i].Message != tc.want[i] {
					t.Errorf("finding %d message mismatch\n  got:  %q\n  want: %q",
						i, got[i].Message, tc.want[i])
				}
			}
		})
	}

	// A table whose rows mostly expect nil passes against a goodsChecks wired to
	// nothing, so assert the table did make it speak.
	if produced == 0 {
		t.Fatal("no case produced a finding — the app_unlock checks may be unreachable and every nil row would still pass")
	}
}

// TestAppUnlockFindingsCarryTheOffendingFieldPath pins the Field. The arity
// finding is the interesting one: it is the only goods finding whose path is the
// ARRAY rather than an element, because the rule is a property of the whole
// catalog and there is no single entry to blame.
//
// It also pins the subtlety that cost this test a round: TWO entries must
// SURVIVE for arity to fire, because a failing entry is excluded from the count
// (mirroring the server's parsed-goods array). A fixture of three unlocks where
// two of them fail produces no arity finding at all.
func TestAppUnlockFindingsCarryTheOffendingFieldPath(t *testing.T) {
	justified := map[string]any{"justification": "reviewed"}
	unlock := func(id string, price int, extra map[string]any) map[string]any {
		e := map[string]any{"id": id, "title": "T", "priceBuzz": float64(price), "kind": "app_unlock"}
		for k, v := range extra {
			e[k] = v
		}
		return e
	}

	got := goodsChecks(map[string]any{"goods": []any{
		unlock("a", 500, justified),  // survives → counted
		unlock("b", 500, nil),        // missing justification → NOT counted
		unlock("c", 500, justified),  // survives → counted, so the count reaches 2
		unlock("d", 7777, justified), // over the unlock ceiling → NOT counted
	}})

	want := []string{"goods[1].justification", "goods[3].priceBuzz", "goods"}
	if len(got) != len(want) {
		t.Fatalf("got %d finding(s), want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Field != want[i] {
			t.Errorf("finding %d field: got %q, want %q", i, got[i].Field, want[i])
		}
	}
}

// TestGoodsFindingsCarryTheOffendingFieldPath pins the Field, not the Message.
// The two are separate claims: `--json` consumers key on Field, and a finding
// whose text names goods[1] while its Field says something else sends a tool to
// the wrong place.
func TestGoodsFindingsCarryTheOffendingFieldPath(t *testing.T) {
	in := map[string]any{"goods": []any{
		map[string]any{"id": "credits", "title": "A", "priceBuzz": float64(10)},
		map[string]any{"id": "credits", "title": "B", "priceBuzz": float64(10)},
		map[string]any{"id": "boosts", "title": "  ", "priceBuzz": float64(10)},
	}}
	got := goodsChecks(in)
	want := []struct{ field, msgHas string }{
		{"goods[1].id", "duplicates an earlier good id"},
		{"goods[2].title", "must be a non-empty string"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d finding(s), want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Field != w.field {
			t.Errorf("finding[%d].Field = %q, want %q", i, got[i].Field, w.field)
		}
		if !strings.Contains(got[i].Message, w.msgHas) {
			t.Errorf("finding[%d] message %q does not mention %q", i, got[i].Message, w.msgHas)
		}
	}
}

// TestGoodsChecksReachTheRealValidator is the SEAM. goodsChecks being correct in
// isolation says nothing about whether semanticChecks calls it — the defect that
// would ship is a perfect function nobody invokes, and every test above would
// still pass.
func TestGoodsChecksReachTheRealValidator(t *testing.T) {
	const body = `{"blockId":"ok-app","name":"x","version":"1.0.0","contentRating":"g","scopes":[],` +
		`"kind":"page","iframe":{"sandbox":"allow-scripts","minHeight":100,"resizable":false},` +
		`"goods":[{"id":"credits","title":"A","priceBuzz":10},{"id":"credits","title":"B","priceBuzz":10}]}`
	var hit bool
	for _, f := range manifestOnlyFindings(t, body) {
		if f.Message == `goods[1].id duplicates an earlier good id ("credits")` {
			hit = true
		}
	}
	if !hit {
		t.Fatal("the duplicate-id finding did not come out of the real ManifestOnly path — " +
			"goodsChecks is not wired into semanticChecks")
	}

	// NEGATIVE CONTROL on the same path: the identical manifest with distinct
	// ids must validate with NO findings at all, or the row above could be
	// passing because this fixture fails for some unrelated reason.
	const ok = `{"blockId":"ok-app","name":"x","version":"1.0.0","contentRating":"g","scopes":[],` +
		`"kind":"page","iframe":{"sandbox":"allow-scripts","minHeight":100,"resizable":false},` +
		`"goods":[{"id":"credits","title":"A","priceBuzz":10},{"id":"boosts","title":"B","priceBuzz":10}]}`
	for _, f := range manifestOnlyFindings(t, ok) {
		t.Errorf("unexpected finding on the legal manifest: %s: %s", f.Field, f.Message)
	}
}
