package api

import (
	"encoding/json"
	"testing"
)

// TestScopeUnmarshalShapes covers the two valid wire shapes plus the null/empty
// degradation and the malformed-input error paths of Scope.UnmarshalJSON — the
// device-login route emits a plain string, the refresh route emits an array.
func TestScopeUnmarshalShapes(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"string shape", `"33554433"`, "33554433", false},
		{"single-element array", `["33554433"]`, "33554433", false},
		{"multi-element array", `["a","b","c"]`, "a b c", false},
		{"empty array", `[]`, "", false},
		{"json null degrades to empty", `null`, "", false},
		{"malformed array element", `[123]`, "", true},
		{"non-string non-array", `123`, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var s Scope
			err := json.Unmarshal([]byte(tc.in), &s)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error unmarshaling %q", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unmarshal %q: %v", tc.in, err)
			}
			if s.String() != tc.want {
				t.Errorf("scope = %q, want %q", s.String(), tc.want)
			}
		})
	}
}

// TestScopeUnmarshalWithinStruct confirms the custom unmarshaler engages when the
// field is decoded as part of the token response envelope (both shapes).
func TestScopeUnmarshalWithinStruct(t *testing.T) {
	var arr TokenResponse
	if err := json.Unmarshal([]byte(`{"access_token":"a","scope":["1","2"]}`), &arr); err != nil {
		t.Fatalf("array-scope response: %v", err)
	}
	if arr.Scope.String() != "1 2" {
		t.Errorf("array scope = %q, want '1 2'", arr.Scope.String())
	}

	var str TokenResponse
	if err := json.Unmarshal([]byte(`{"access_token":"a","scope":"7"}`), &str); err != nil {
		t.Fatalf("string-scope response: %v", err)
	}
	if str.Scope.String() != "7" {
		t.Errorf("string scope = %q, want '7'", str.Scope.String())
	}
}
