package redact

import "testing"

// TestServerTextRefusesAPIKeyShapedHex pins the 32-hex rule: a Civitai API key
// is 32 hex characters (civitai's generateKey(length = 32)), below the 40-char
// token-run threshold, so before this rule a bare key inside server text was
// shown. Any unbroken all-hex run of 32 or more letters/digits is now withheld.
//
// The 31/32 pair pins the threshold from both sides; the mixed-case 32-run is
// NOT hex and must still follow the 40 rule (shown at 32, refused at 40), so the
// new rule cannot be met by lowering every threshold to 32. Fixtures are
// pairwise distinct.
func TestServerTextRefusesAPIKeyShapedHex(t *testing.T) {
	key := "9f86d081884c7d659a2feaa0c55ad015"      // a realistic lowercase 32-hex key
	upperKey := "B94D27B9934D3E08A52E52D7DA7DABFA" // the same shape, uppercase
	hex31 := "2c26b46b68ffc68ff99b453c1d30413"
	mixed32 := "Q2xpZW50U2VjcmV0VmFsdWVGb3JUZXN0" // mixed case, holds non-hex letters
	mixed40 := mixed32 + "aW5nT25s"
	if len(key) != 32 || len(upperKey) != 32 || len(hex31) != 31 || len(mixed32) != 32 || len(mixed40) != 40 {
		t.Fatalf("PREMISE BROKEN: fixture lengths %d/%d/%d/%d/%d", len(key), len(upperKey), len(hex31), len(mixed32), len(mixed40))
	}
	if hexRun.MatchString(mixed32) {
		t.Fatalf("PREMISE BROKEN: %q is all hex, so it cannot test the non-hex path", mixed32)
	}

	refused := []struct{ name, in string }{
		{"a bare 32-hex API key", key},
		{"a 32-hex API key inside a message", "API key " + key + " is not valid"},
		{"an uppercase 32-hex key", "token " + upperKey + " was revoked"},
		{"a 32-hex key behind a prefix", "invalid civitai_" + key},
		{"a 32-hex key in a path", "no such key at /api/keys/" + key + ".json"},
		{"a 33-hex run", "hash " + key + "e"},
		{"a mixed-case non-hex 40-run (the 40 rule)", "secret " + mixed40},
	}
	for _, tc := range refused {
		t.Run("refused/"+tc.name, func(t *testing.T) {
			if got, ok := ServerText(tc.in); ok || got != "" {
				t.Errorf("ServerText(%q) = (%q, %v), want it withheld", tc.in, got, ok)
			}
		})
	}

	shown := []string{
		"a 31-hex run " + hex31 + " is below the threshold",
		"a mixed-case 32-run " + mixed32 + " is not hex and follows the 40 rule",
		"request 3f2504e0-4f89-11d3-9a0c-0305e82c3301 failed", // a UUID breaks at its hyphens
	}
	for _, in := range shown {
		t.Run("shown/"+in[:20], func(t *testing.T) {
			if got, ok := ServerText(in); !ok || got != in {
				t.Errorf("ServerText(%q) = (%q, %v), want it shown whole", in, got, ok)
			}
		})
	}

	// The key must also be withheld on the route-message path, not only by
	// ServerText called directly.
	if got := CredentialRouteMessage([]byte(`{"message":"API key ` + key + ` is not valid"}`)); got != "67-byte response body not shown because it may carry credentials" {
		t.Errorf("CredentialRouteMessage showed %q, want the byte count", got)
	}
}
