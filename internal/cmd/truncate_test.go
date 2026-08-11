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
