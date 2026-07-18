package cmd

import (
	"strings"
	"testing"
)

func TestSafeTerm(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain ascii", "dreamshaper-8", "dreamshaper-8"},
		{"keeps newline and tab", "a\tb\nc", "a\tb\nc"},
		{"strips CR", "a\rb", "ab"},
		{"strips ESC/ANSI color", "red\x1b[31mtext\x1b[0m", "red[31mtext[0m"},
		{"strips cursor-up + line-clear (output forgery)", "x\x1b[1A\x1b[2Ky", "x[1A[2Ky"},
		{"strips OSC-52 clipboard set", "n\x1b]52;c;AAAA\x07m", "n]52;c;AAAAm"},
		{"strips BEL", "a\x07b", "ab"},
		{"strips NUL", "a\x00b", "ab"},
		{"strips DEL", "a\x7fb", "ab"},
		{"strips C1 CSI U+009B", "ab", "ab"},
		{"strips C1 OSC U+009D", "ab", "ab"},
		{"strips C1 low U+0080", "ab", "ab"},
		{"strips C1 high U+009F", "ab", "ab"},
		{"keeps multibyte UTF-8 (accents/CJK/emoji)", "café — 日本語 🚀", "café — 日本語 🚀"},
		{"keeps printable just above C1 (U+00A0 NBSP)", "a z", "a z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := safeTerm(tc.in); got != tc.want {
				t.Errorf("safeTerm(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestSafeTermRemovesEveryControlRune exhaustively checks that every C0 (except
// \n/\t), DEL, and C1 code point is removed, and that a sampling of ordinary
// runes survives.
func TestSafeTermRemovesEveryControlRune(t *testing.T) {
	for r := rune(0); r <= 0x9f; r++ {
		if r == '\n' || r == '\t' {
			if got := safeTerm(string(r)); got != string(r) {
				t.Errorf("safeTerm should KEEP %#x", r)
			}
			continue
		}
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			if got := safeTerm("a" + string(r) + "b"); got != "ab" {
				t.Errorf("safeTerm should strip %#x, got %q", r, got)
			}
		}
	}
	// A printable ASCII + multibyte runes must be untouched.
	for _, keep := range []string{"A", "~", " ", "¡", "字"} {
		if got := safeTerm(keep); got != keep {
			t.Errorf("safeTerm stripped a printable rune %q -> %q", keep, got)
		}
	}
}

// controlBytes are the raw terminal-control byte sequences that must never
// survive into human-renderer output.
var controlBytes = []string{"\x1b", "", "\x07", "\x00", "\x7f"}

// assertNoControlBytes fails if s contains any raw terminal control sequence.
func assertNoControlBytes(t *testing.T, label, s string) {
	t.Helper()
	for _, cb := range controlBytes {
		if strings.Contains(s, cb) {
			t.Errorf("%s leaked a raw control sequence %q:\n%q", label, cb, s)
		}
	}
}
