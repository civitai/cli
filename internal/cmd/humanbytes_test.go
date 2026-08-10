package cmd

import (
	"strings"
	"testing"
)

// TestHumanBytesLabelsItsOwnArithmetic pins the unit SUFFIX to the divisor the
// function actually uses.
//
// The regression it exists for: humanBytes divided by 1024 and labelled the
// result with the SI suffixes (KB/MB/GB), so 2 MiB rendered as "2.0 MB" — a
// figure 4.9% below the bytes it described. That was invisible while the string
// only ever reached download progress, and #275 put it in `--help`: the
// set-icon / set-cover one-liners interpolate listingSourceRule(kind), which
// renders the cap through this function, so two commands advertised "at most
// 2.0 MB" / "at most 4.0 MB" for caps the README documents as 2 MiB / 4 MiB.
//
// 🔴 The table has to pin the DIVISOR as well as the LABEL, because "relabel"
// and "re-base the arithmetic" are two different fixes and only one of them was
// wanted. Both mutants were run against this table rather than reasoned about:
//
//	SI labels over a 1024 divisor (the bug)  -> every prefixed row reddens.
//	IEC labels over a 1000 divisor           -> 1023, 1,000,000, all three cap
//	                                            rows, 1.5 MiB and every rung
//	                                            from GiB up redden; only the
//	                                            1024 row survives, because
//	                                            %.1f rounds 1.024 back to "1.0".
//
// The 1,000,000 row is the loudest of those and is here on purpose: it is the
// value where the two systems disagree about which RUNG to use at all
// ("976.6 KiB" vs "1.0 MiB"), so it fails legibly rather than by a last digit.
func TestHumanBytesLabelsItsOwnArithmetic(t *testing.T) {
	cases := []struct {
		name string
		in   int64
		want string
	}{
		{"zero", 0, "0 B"},
		{"one", 1, "1 B"},
		{"just under a KiB", 1023, "1023 B"},
		{"exactly a KiB", 1024, "1.0 KiB"},
		// A decimal megabyte. Binary-labelled it is NOT "1.0 M<anything>".
		{"one decimal MB", 1_000_000, "976.6 KiB"},
		{"icon cap", maxIconBytes, "2.0 MiB"},
		{"cover cap", maxCoverBytes, "4.0 MiB"},
		{"screenshot cap", maxScreenshotBytes, "2.0 MiB"},
		{"a MiB and a half", 1536 * 1024, "1.5 MiB"},
		{"GiB", 1 << 30, "1.0 GiB"},
		{"TiB", 1 << 40, "1.0 TiB"},
		{"PiB", 1 << 50, "1.0 PiB"},
		{"EiB", 1 << 60, "1.0 EiB"},
	}
	if len(cases) < 13 {
		t.Fatalf("the table lost rows (%d) — a shrunken table is how this guard stops "+
			"covering the unit ladder without anything going red", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := humanBytes(tc.in); got != tc.want {
				t.Errorf("humanBytes(%d) = %q, want %q — the divisor is 1024, so the label "+
					"must be the IEC one (KiB/MiB/GiB); an SI label over binary math "+
					"understates the value it is describing", tc.in, got, tc.want)
			}
		})
	}
}

// TestHumanBytesNeverPrintsAnSILabel is the shape half of the assertion above.
//
// The table pins thirteen points; this pins the property, so a new unit added to
// the ladder cannot arrive SI-labelled just because nobody added a row for it.
// It sweeps the whole ladder (including the sub-KiB "N B" case, which is
// correctly unprefixed in both systems) and requires every prefixed answer to
// carry the "i".
func TestHumanBytesNeverPrintsAnSILabel(t *testing.T) {
	var checked int
	for exp := 0; exp <= 6; exp++ {
		n := int64(1)
		for i := 0; i < exp; i++ {
			n *= 1024
		}
		// A value comfortably inside the band, so rounding cannot push it up a rung.
		for _, mult := range []int64{1, 3} {
			got := humanBytes(n * mult)
			checked++
			unit := got[strings.LastIndex(got, " ")+1:]
			if unit == "B" {
				continue // bare bytes carry no prefix in either system
			}
			if !strings.HasSuffix(unit, "iB") {
				t.Errorf("humanBytes(%d) = %q — unit %q is an SI label on a 1024-based "+
					"divisor. Either label it IEC (%siB) or change the divisor to 1000, "+
					"but the two must agree.", n*mult, got, unit, strings.TrimSuffix(unit, "B"))
			}
		}
	}
	// Positive control: a sweep that walked nothing would report the same serene
	// pass as a correct one.
	if checked != 14 {
		t.Fatalf("swept %d values, want 14 — the ladder walk is wrong, so its clean "+
			"verdict is a fact about the loop, not about humanBytes", checked)
	}
}
