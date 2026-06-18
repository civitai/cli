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
