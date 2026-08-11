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
		{"strips C1 CSI U+009B", "a\u009bb", "ab"},
		{"strips C1 OSC U+009D", "a\u009db", "ab"},
		{"strips C1 low U+0080", "a\u0080b", "ab"},
		{"strips C1 high U+009F", "a\u009fb", "ab"},
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

// indentContinuation is the guard for the forgery class safeTerm CANNOT see:
// safeTerm keeps `\n`, so a server string is free to start a new line at column
// zero, where this CLI writes its own banners and headers. Every clause of the
// implementation is pinned here, because each one was separately mutable
// without any caller-level test noticing (civitai/cli#367, review round 3).
func TestIndentContinuation(t *testing.T) {
	const pad = "  "

	t.Run("the first line is never padded", func(t *testing.T) {
		// It is already positioned by whatever printed it — "  - id: <here>" —
		// so padding it would push it out of its own list item.
		if got := indentContinuation("only one line", pad); got != "only one line" {
			t.Errorf("a single-line string was modified: %q", got)
		}
		got := indentContinuation("first\nsecond", pad)
		if !strings.HasPrefix(got, "first\n") {
			t.Errorf("the first line was padded: %q", got)
		}
	})

	t.Run("EVERY continuation line is padded, not just the second", func(t *testing.T) {
		// A `break` after the first continuation leaves line 3 at column zero —
		// which is a forgery slot, and a two-line fixture cannot see it.
		got := indentContinuation("a\nb\nc\nd", pad)
		lines := strings.Split(got, "\n")
		if len(lines) != 4 {
			t.Fatalf("CONTROL failure, not a finding: %d lines, want 4: %q", len(lines), got)
		}
		for i, ln := range lines[1:] {
			if !strings.HasPrefix(ln, pad) {
				t.Errorf("continuation line %d is at column zero: %q (full: %q)", i+2, ln, got)
			}
		}
	})

	t.Run("an empty continuation line is left alone", func(t *testing.T) {
		// Padding it would emit a line of trailing whitespace, and the docstring
		// promises a trailing newline is not turned into one.
		if got := indentContinuation("a\n", pad); got != "a\n" {
			t.Errorf("a trailing newline was padded into whitespace: %q", got)
		}
		if got := indentContinuation("a\n\nb", pad); got != "a\n\n"+pad+"b" {
			t.Errorf("a blank interior line was padded: %q", got)
		}
	})

	t.Run("the pad is what the caller asked for", func(t *testing.T) {
		if got := indentContinuation("a\nb", "\t\t"); got != "a\n\t\tb" {
			t.Errorf("pad not applied verbatim: %q", got)
		}
	})

	t.Run("the text itself is unchanged", func(t *testing.T) {
		// AGENTS.md item 13: the reason passes through verbatim. This moves
		// where it sits, never what it says.
		in := "Résumé — 字\ttab\nsecond"
		got := indentContinuation(in, pad)
		if strings.ReplaceAll(got, "\n"+pad, "\n") != in {
			t.Errorf("indentContinuation altered the text: %q -> %q", in, got)
		}
	})
}

// controlBytes are the raw terminal-control byte sequences that must never
// survive into human-renderer output.
var controlBytes = []string{"\x1b", "\u009b", "\x07", "\x00", "\x7f"}

// assertNoControlBytes fails if s contains any raw terminal control sequence.
func assertNoControlBytes(t *testing.T, label, s string) {
	t.Helper()
	for _, cb := range controlBytes {
		if strings.Contains(s, cb) {
			t.Errorf("%s leaked a raw control sequence %q:\n%q", label, cb, s)
		}
	}
}
