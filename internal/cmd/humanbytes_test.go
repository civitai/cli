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
// humanBytesLadderFloor is the KEEPER for the case table in
// TestHumanBytesLabelsItsOwnArithmetic: the set of case NAMES that must have a
// row, whatever else the table gains.
//
// 🔴 A COUNT WAS NOT ENOUGH, AND THE ROWS IT PROTECTS ARE NOT INTERCHANGEABLE.
// This started as `len(cases) < 13`, reasoning that the table only grows so any
// deletion drops below the floor. Add-one/delete-one defeats it in one move:
// delete "one decimal MB" and add any unrelated row — "two bytes", say — and the
// count is still 13, the guard is green, and the rung is uncovered.
//
// That trade is not hypothetical bookkeeping, because this table's rows are
// individually load-bearing and the doc comment above says which: the IEC-labels
// -over-a-1000-divisor mutant is survived by the 1024 row alone, so "one decimal
// MB" is the row that fails LEGIBLY (976.6 KiB vs 1.0 MiB — a disagreement about
// which RUNG applies, not a last digit). Trading it for a row that only re-pins
// an already-covered rung keeps the count and silently downgrades the mutation
// matrix this test's credibility rests on. Membership cannot be traded that way:
// removing a NAME from this list, or a row from the table, is red by name.
//
// 🔴 TWO RESIDUALS, STATED RATHER THAN GLOSSED. (1) Protection is OPT-IN PER
// ROW: the loop iterates THIS list, so a row added to the table and not named
// here is permanently tradeable, and nothing goes red at the moment of the
// omission. Append the name in the same commit as the row. (2) The two-line
// bypass is still open: delete the floor entry AND the row together. What this
// stops is the ONE-line trade — a deletion paid for by an unrelated addition,
// which is exactly the shape a "widen the table" commit takes; it does not stop
// a deliberate two-line removal, and no in-tree check can. That is review's job.
//
// It pins NAMES, not values. A row keeping its name while its `want` is edited
// to match a regression is not caught here — TestHumanBytesNeverPrintsAnSILabel
// is the property half that covers the ladder independently of any row.
var humanBytesLadderFloor = []string{
	"zero",
	"one",
	"just under a KiB",
	"exactly a KiB",
	"one decimal MB",
	"icon cap",
	"cover cap",
	"screenshot cap",
	"a MiB and a half",
	"GiB",
	"TiB",
	"PiB",
	"EiB",
}

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
	have := map[string]bool{}
	for _, tc := range cases {
		have[tc.name] = true
	}
	// POSITIVE CONTROL: an empty floor would make every check below vacuous.
	if len(humanBytesLadderFloor) == 0 {
		t.Fatal("CONTROL failure: humanBytesLadderFloor is empty, so this test asserts nothing")
	}
	for _, name := range humanBytesLadderFloor {
		if !have[name] {
			t.Errorf("the %q row is gone from the unit-ladder table.\n"+
				"Rows are only ever ADDED; deleting one stops this guard covering that rung — and "+
				"because the old guard was a COUNT (`len(cases) < 13`), deleting this row while "+
				"adding any unrelated one kept the count at 13 and stayed green.\n"+
				"Adding rows is free; this list only forbids REMOVING one.", name)
		}
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
