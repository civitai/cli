package cmd

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestTruncateCutsOnRuneBoundaries — `truncate` used to slice BYTES
// (`s[:max]`), so a cut landing inside a multi-byte rune emitted the leading
// bytes of that rune on their own. Go renders those as U+FFFD, so the CLI
// printed a replacement character into a tagline/description/base-model cell:
// corrupted output, not merely a short one.
//
// The fixtures are genuinely multi-byte on purpose. An ASCII fixture cannot
// fail this test at all — every byte is its own rune — which is exactly why the
// bug survived: nothing exercised the only input shape that can show it.
func TestTruncateCutsOnRuneBoundaries(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
	}{
		// 3 bytes per rune, so max=4 lands one byte into the second rune.
		{"cjk", strings.Repeat("漢", 10), 4},
		// 4 bytes per rune (astral plane): max=6 lands two bytes into the
		// second rune.
		{"emoji", strings.Repeat("🐈", 10), 6},
		// 2 bytes per rune, mixed with ASCII — the shape a real tagline has.
		{"accented latin", "café résumé naïve " + strings.Repeat("ü", 40), 25},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncate(tc.in, tc.max)

			// CONTROL: the fixture must actually be truncated, or this case
			// asserts nothing. A pass over an untouched string would look
			// identical to a pass over a correct cut.
			if got == tc.in {
				t.Fatalf("CONTROL failure: %q was not truncated at max=%d, so this case exercises nothing", tc.in, tc.max)
			}
			if !strings.HasSuffix(got, "…") {
				t.Errorf("a truncated value must keep its ellipsis, got %q", got)
			}
			if !utf8.ValidString(got) {
				t.Errorf("truncate(%q, %d) = %q — not valid UTF-8", tc.in, tc.max, got)
			}
			if strings.ContainsRune(got, utf8.RuneError) {
				t.Errorf("truncate(%q, %d) = %q — the cut landed inside a rune and produced U+FFFD",
					tc.in, tc.max, got)
			}
			// Every rune of the result (bar the ellipsis) must be a whole rune
			// taken from the input, in order.
			body := strings.TrimSuffix(got, "…")
			if !strings.HasPrefix(tc.in, body) {
				t.Errorf("truncate(%q, %d) = %q — the kept part is not a prefix of the input", tc.in, tc.max, got)
			}
		})
	}
}

// TestTruncateLeavesShortValuesAlone is the other direction: a value that fits
// must come back byte-identical, with NO ellipsis appended. A rune-aware
// rewrite that measured the wrong unit could start truncating values that were
// previously left whole, which is a silent data loss rather than a visible one.
func TestTruncateLeavesShortValuesAlone(t *testing.T) {
	for _, s := range []string{"", "short", "café", "漢字", strings.Repeat("a", 200)} {
		if got := truncate(s, 200); got != s {
			t.Errorf("truncate(%q, 200) = %q, want it unchanged", s, got)
		}
	}
}

// 🔴 TestTruncateInTheByteOverRuneUnderWindow IS THE CASE EVERY OTHER TEST HERE
// MISSES, AND ITS ABSENCE LEFT A SURVIVING MUTANT.
//
// `truncate` short-circuits on BYTE length first, then measures RUNES. Between
// those two bounds sits a window — `len(s) > max >= len([]rune(s))` — reachable
// only by a non-ASCII string just over the byte bound. Every case above is
// either far past both bounds (the truncation cases) or far under both
// (max=200 over ≤200-byte inputs), so all of them returned at the FIRST check
// and the rune-length branch never executed. Measured: replacing it with
// `if false && len(r) <= max` left the whole package green while
// `truncate("café", 4)` returned `"café…"` — an ellipsis appended to a value
// that fits.
//
// The implementation now closes the window with a single walk rather than a
// second branch, so this test is what proves the walk (not just the byte
// short-circuit) gets the answer right. It is also the mutation target: break
// the loop's bound and this is the case that goes red.
func TestTruncateInTheByteOverRuneUnderWindow(t *testing.T) {
	cases := []struct {
		in       string
		max      int
		want     string
		whyInWin string
	}{
		// 5 bytes, 4 runes.
		{"café", 4, "café", "byte len 5 > 4 >= rune len 4"},
		// 6 bytes, 2 runes.
		{"漢字", 2, "漢字", "byte len 6 > 2 >= rune len 2"},
		// 8 bytes, 2 runes.
		{"🐈🐈", 2, "🐈🐈", "byte len 8 > 2 >= rune len 2"},
		// Exactly one rune past the window: it MUST be cut, which is what stops
		// a "never truncate anything non-ASCII" fix from passing this test.
		{"漢字漢", 2, "漢字…", "rune len 3 > max 2 — outside the window, must cut"},
	}
	for _, tc := range cases {
		// CONTROL: assert the fixture really is where the comment claims. A
		// fixture that drifted out of the window would exercise the byte
		// short-circuit and pass while proving nothing.
		if len(tc.in) <= tc.max {
			t.Fatalf("CONTROL failure: %q has %d bytes, which is not > max=%d — this case no longer reaches "+
				"past the byte short-circuit (%s)", tc.in, len(tc.in), tc.max, tc.whyInWin)
		}
		if got := truncate(tc.in, tc.max); got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q (%s)", tc.in, tc.max, got, tc.want, tc.whyInWin)
		}
	}
}
