package validate

import "testing"

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
