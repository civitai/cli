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
	got := errs[0]
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
				if got[i] != tc.want[i] {
					t.Errorf("error[%d] = %q, want %q", i, got[i], tc.want[i])
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
				if got[i] != tc.want[i] {
					t.Errorf("error[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestIsSensitiveBlockScope pins the sensitive set to the server's 5 scopes.
func TestIsSensitiveBlockScope(t *testing.T) {
	sensitive := []string{
		"ai:write:budgeted", "social:tip:self", "buzz:read:self",
		"collections:read:private", "apps:storage:shared:write",
	}
	if len(SENSITIVE_BLOCK_SCOPES) != len(sensitive) {
		t.Fatalf("SENSITIVE_BLOCK_SCOPES has %d entries, want %d", len(SENSITIVE_BLOCK_SCOPES), len(sensitive))
	}
	for _, s := range sensitive {
		if !isSensitiveBlockScope(s) {
			t.Errorf("isSensitiveBlockScope(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"user:read:self", "apps:storage:shared:read", "collections:read:self", "AI:WRITE:BUDGETED"} {
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
