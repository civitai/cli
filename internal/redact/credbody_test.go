package redact

import (
	"strings"
	"testing"
)

// TestServerText pins which server error messages may be printed on a
// credential route. Each refusal row is a shape a credential takes on the wire
// (a JSON payload, a URL) or one a terminal acts on (control and format
// characters); the accept rows are real error messages.
func TestServerText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"oauth code", "invalid_grant", "invalid_grant", true},
		{"trimmed message", "  App not found \t", "App not found", true},
		// No length limit: the dev-token route's real owned-but-not-deployed 404
		// is 179 runes plus the blockId, and a 200-rune cap withheld it.
		{"a long message is not refused for its length", strings.Repeat("é", 500), strings.Repeat("é", 500), true},
		{"empty", "   ", "", false},
		{"json object", `{"access_token":"x"}`, "", false},
		{"json array", `["x"]`, "", false},
		{"signed url", "see https://up.example/p?sig=abc", "", false},
		{"userinfo url", "git://dev:tok@host/repo", "", false},
		{"newline", "line one\nline two", "", false},
		{"escape sequence", "a\x1b[2Jb", "", false},
		{"bidi override", "abc\u202edef", "", false},
		{"invalid utf-8", "a\xffb", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ServerText(tc.in)
			if got != tc.want || ok != tc.ok {
				t.Errorf("ServerText(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// TestWithheldBodyNeverShowsTheBody pins the two shapes the size-only report
// takes, with a fixture whose length is not a constant the text names.
func TestWithheldBodyNeverShowsTheBody(t *testing.T) {
	body := []byte(`{"refresh_token":"SENTINEL-WITHHELD-5a1f"}`)
	got := WithheldBody(body)
	if want := "42-byte response body not shown because it may carry credentials"; got != want {
		t.Errorf("WithheldBody = %q, want %q", got, want)
	}
	if got := WithheldBody([]byte(" \n")); got != "empty response body" {
		t.Errorf("WithheldBody(blank) = %q", got)
	}
}

// TestErrorMessageAndWhatMayBeShown pins the split between the two readers of a
// credential route's error body: ErrorMessage returns the server's wording
// UNFILTERED (for a caller that classifies on it), CredentialRouteMessage shows
// it only when ServerText accepts it, and CredentialRouteText additionally shows
// a bare-text body. Each row's message text is distinct, so a row cannot pass by
// reading another field.
func TestErrorMessageAndWhatMayBeShown(t *testing.T) {
	const withheld20 = "20-byte response body not shown because it may carry credentials"
	urlMsg := `{"message":"see https://x.example/?sig=Q"}`
	cases := []struct {
		name      string
		body      string
		reason    string // ErrorMessage
		shown     string // CredentialRouteMessage
		shownText string // CredentialRouteText
	}{
		{"trpc message", `{"error":{"json":{"message":"tRPC said no","code":-32603}}}`, "tRPC said no", "tRPC said no", "tRPC said no"},
		{"plain message", `{"message":"plain said no"}`, "plain said no", "plain said no", "plain said no"},
		{"string error", `{"error":"error said no"}`, "error said no", "error said no", "error said no"},
		{"message wins over error", `{"message":"first","error":"second"}`, "first", "first", "first"},
		{"message beside an object error", `{"message":"beside object","error":{"code":1}}`, "beside object", "beside object", "beside object"},
		{"a URL-bearing message is classified but not shown", urlMsg, "see https://x.example/?sig=Q",
			"42-byte response body not shown because it may carry credentials",
			"42-byte response body not shown because it may carry credentials"},
		{"no message field", `{"token":"tok-xyz1"}`, "", withheld20, withheld20},
		{"bare text", "bare text refusal", "", "17-byte response body not shown because it may carry credentials", "bare text refusal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(tc.body)
			if got := ErrorMessage(raw); got != tc.reason {
				t.Errorf("ErrorMessage = %q, want %q", got, tc.reason)
			}
			if got := CredentialRouteMessage(raw); got != tc.shown {
				t.Errorf("CredentialRouteMessage = %q, want %q", got, tc.shown)
			}
			if got := CredentialRouteText(raw); got != tc.shownText {
				t.Errorf("CredentialRouteText = %q, want %q", got, tc.shownText)
			}
		})
	}
}

// TestServerTextRefusesSingleCaseTokens pins the third token shape: an unbroken
// run of 40 or more ASCII letters and digits, whatever its case. Before it, a
// lowercase-only (or uppercase-only) non-hex token of any length passed, because
// the run rule only refused hex or mixed case. English words and identifiers do
// not reach 40 letters without a separator, and slugs, snake_case names, paths
// and ids break into shorter pieces at `-`, `_`, `/` and `.`, so the accept rows
// must still be shown whole. Each fixture is distinct, and the 39/40 pair pins
// the threshold from both sides for both cases.
func TestServerTextRefusesSingleCaseTokens(t *testing.T) {
	lower40 := "qkzvwmxprtnbjhlgfdscyaueoiqpwlzmxkcnvbrt" // letters only, not hex
	lowerDigits40 := "k7m2p9q4x8w3n6r1t5v0z8c3b6d9f2g4h7j1l5s0"
	upper40 := "MFRGGZDFMZTWQ2LKNNWG23TPOBYXE43UOV3HO6DZ" // a base32 shape
	lower39 := "zpxqwmvkrnbtjhlgfdscyaueoimqpwlzmxkcnvb"
	if len(lower40) != 40 || len(lowerDigits40) != 40 || len(upper40) != 40 || len(lower39) != 39 {
		t.Fatalf("PREMISE BROKEN: fixture lengths %d/%d/%d/%d", len(lower40), len(lowerDigits40), len(upper40), len(lower39))
	}
	refused := []struct{ name, in string }{
		{"a 40-letter lowercase run", "your token is " + lower40},
		{"a 40-char lowercase letters-and-digits run", "key " + lowerDigits40 + " was revoked"},
		{"a 40-char uppercase run", "secret " + upper40},
		{"a lowercase token behind a prefix", "invalid civitai_" + lowerDigits40},
		{"a lowercase token in a path", "no blob at /v2/blobs/" + lower40 + ".png"},
		{"a lowercase run longer than 40", "t=" + lower40 + "abcdefgh"},
	}
	for _, tc := range refused {
		t.Run("refused/"+tc.name, func(t *testing.T) {
			if got, ok := ServerText(tc.in); ok || got != "" {
				t.Errorf("ServerText(%q) = (%q, %v), want it withheld", tc.in, got, ok)
			}
		})
	}
	shown := []string{
		"a 39-char lowercase run " + lower39 + " is below the threshold",
		"field this_is_a_very_long_snake_case_identifier_for_the_request_body is required",
		"block 'the-quick-brown-fox-jumps-over-the-lazy-dog-again-and-again' was not found",
		"the checkpoint models/sdxl/checkpoints/realisticvisionv60b1.safetensors is unavailable",
		"workflow 0-20261120-clx3k9z0q0000abcd1234efgh is still processing on the orchestrator",
		"Insufficient Buzz: this generation costs 240 and your account holds 12 (internationalization pending)",
		"see civitai.com/user/account for your personal API keys and scopes",
	}
	for _, in := range shown {
		t.Run("shown/"+in[:24], func(t *testing.T) {
			if got, ok := ServerText(in); !ok || got != in {
				t.Errorf("ServerText(%q) = (%q, %v), want it shown whole", in, got, ok)
			}
		})
	}
}

// TestServerTextRefusesTokenShapes pins the token shapes ServerText withholds
// now that it has no length cap: a JWT anywhere in the text, and a run of 40+
// token characters that is all hex or mixes upper- and lower-case letters. The
// accept rows are the server's real long messages and a long lowercase slug, so
// the refusal cannot be met by withholding every long message; the 39/40 pair
// pins the threshold from both sides.
func TestServerTextRefusesTokenShapes(t *testing.T) {
	const jwt = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiI0MiJ9.c2lnbmF0dXJlLXZhbHVl_-Q"
	const unsignedJWT = "eyJhbGciOiJub25lIn0.eyJzdWIiOiI0MiJ9."
	hex40 := "3f9a0c7e5b1d2468ace013579bdf2468ace01357"
	mixed40 := "Q2xpZW50U2VjcmV0VmFsdWVGb3JUZXN0aW5nT25s"
	mixed39 := mixed40[:39]
	slug := "my-really-long-dev-block-name1"
	refused := []struct{ name, in string }{
		{"a JWT inside a message", `minted ` + jwt + ` for you`},
		{"a bearer JWT", "Bearer " + jwt},
		{"a bare JWT", jwt},
		{"an unsigned JWT", "token " + unsignedJWT},
		{"a 40-char hex run", "your key is " + hex40},
		{"a 40-char mixed-case run", "secret=" + mixed40},
		{"a 52-char mixed-case run with base64 padding", "k " + mixed40 + "AbCdEfGh0123=="},
	}
	for _, tc := range refused {
		t.Run("refused/"+tc.name, func(t *testing.T) {
			if got, ok := ServerText(tc.in); ok || got != "" {
				t.Errorf("ServerText(%q) = (%q, %v), want it withheld", tc.in, got, ok)
			}
		})
	}
	shown := []string{
		"block '" + slug + "' has no live deployment — dev:live requires an " +
			"approved + deployed version (deployState: live). Until your first " +
			"version is live, validate locally with dev:harness (mock).",
		"dev-token needs a personal API key (full scope, create at " +
			"civitai.com/user/account) OR an OAuth login whose token carries the " +
			"Apps submit scope; real Buzz spend additionally needs AI Services scope",
		"block 'a-very-long-lowercase-app-slug-with-many-words-2026' was not found",
		"a 39-char run " + mixed39 + " is below the threshold",
		"a lowercase path src/server/services/blocks/offsite-listing.service.ts failed",
	}
	if len(mixed39) != 39 || len(mixed40) != 40 || len(hex40) != 40 {
		t.Fatalf("PREMISE BROKEN: fixture lengths %d/%d/%d", len(mixed39), len(mixed40), len(hex40))
	}
	for _, in := range shown {
		t.Run("shown/"+in[:20], func(t *testing.T) {
			if got, ok := ServerText(in); !ok || got != in {
				t.Errorf("ServerText(%q) = (%q, %v), want it shown whole", in, got, ok)
			}
		})
	}
}
