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
