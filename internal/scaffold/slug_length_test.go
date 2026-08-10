package scaffold

import (
	"strings"
	"testing"
)

// Issue #291. Derivation used to TRUNCATE a name whose slug ran past the 40-char
// cap (`s = strings.Trim(s[:40], "-")`), while an EXPLICIT `--slug` of the same
// length was refused with "must be 3-40 chars". The asymmetry is the bug: the
// derived path is the one a first-time author walks, and the value being cut is
// the app's PERMANENT public id.
//
// 🔴 THE COLLISION IS WHAT RAISES THIS ABOVE COSMETIC. A truncation on its own is
// ugly but recognisable; two DIFFERENT names silently minting the SAME id is not
// something the author can detect locally, and a blockId cannot be renamed.
//
// The tests below pin the refusal from BOTH sides of the boundary, because a
// one-sided length guard is where an off-by-one lives:
//   - exactly 40 must still derive, byte-identical (positive control), and
//   - 41 must be refused.

// #291's own reproduction: two 54-character names that differ only from
// character 44 onward — well inside the region the old code cut away.
const (
	longNameA = "aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd eeeeeeeeee"
	longNameB = "aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd ZZZZZZZZZZ"
	// collidingSlug is what BOTH of the above derived before the refusal, at
	// exit 0. It is spelled out rather than described so a row here is evidence
	// about THIS defect and not merely "an error came back".
	collidingSlug = "aaaaaaaaaa-bbbbbbbbbb-cccccccccc-ddddddd"

	// exactlyFortyName derives a 40-character slug — the boundary that must keep
	// working untouched.
	exactlyFortyName = "aaaaaaaaaa bbbbbbbbbb cccccccccc ddddddd"
	exactlyFortySlug = "aaaaaaaaaa-bbbbbbbbbb-cccccccccc-ddddddd"

	// fortyOneName derives a 41-character slug — one past the cap.
	fortyOneName = "aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddd"
)

// TestOverLengthNamesDoNotSilentlyMintTheSameBlockID is the headline of #291.
func TestOverLengthNamesDoNotSilentlyMintTheSameBlockID(t *testing.T) {
	// The fixture has to actually BE the collision, or this test is about
	// nothing. Both controls run before any assertion about the refusal.
	if longNameA == longNameB {
		t.Fatal("CONTROL failure: the two names must differ")
	}
	if len(collidingSlug) != 40 {
		t.Fatalf("CONTROL failure: the pre-fix id is %d chars, want the 40-char truncation", len(collidingSlug))
	}
	for _, n := range []string{longNameA, longNameB} {
		if got := legacyTruncate(n); got != collidingSlug {
			t.Fatalf("CONTROL failure: %q used to derive %q, not the pinned %q — the fixture no longer reproduces #291",
				n, got, collidingSlug)
		}
	}

	var derived []string
	for _, name := range []string{longNameA, longNameB} {
		got, err := Slugify(name)
		if err == nil {
			derived = append(derived, got)
			t.Errorf("Slugify(%q) = %q with no error — an over-length name must be refused, not truncated into a "+
				"permanent public id the author did not type", name, got)
			if got == collidingSlug {
				t.Errorf("…and it is the pre-fix truncation %q, which the OTHER name derives too: two apps, one "+
					"un-renameable blockId", collidingSlug)
			}
			continue
		}
		assertLengthRefusal(t, name, err)
	}
	if len(derived) == 2 && derived[0] == derived[1] {
		t.Fatalf("two different names minted the identical blockId %q", derived[0])
	}
}

// TestSlugifyRefusalNamesTheLengthAndTheEscapeHatch: the refusal is only useful
// if the author can act on it. It must say how long the derived id was, what the
// limit is, and which flag settles it.
func TestSlugifyRefusalNamesTheLengthAndTheEscapeHatch(t *testing.T) {
	_, err := Slugify(longNameA)
	if err == nil {
		t.Fatalf("Slugify(%q) should be refused", longNameA)
	}
	assertLengthRefusal(t, longNameA, err)
}

// TestSlugifyAtExactlyFortyStillDerives is the BOUNDARY CONTROL. `>= 40` instead
// of `> 40` is the off-by-one this pins, and it is invisible to every other test
// in the suite.
func TestSlugifyAtExactlyFortyStillDerives(t *testing.T) {
	if len(exactlyFortySlug) != 40 {
		t.Fatalf("CONTROL failure: the fixture slug is %d chars, want exactly 40", len(exactlyFortySlug))
	}
	got, err := Slugify(exactlyFortyName)
	if err != nil {
		t.Fatalf("Slugify(%q) = %v — a 40-char derivation is AT the cap, not over it, and must still succeed",
			exactlyFortyName, err)
	}
	if got != exactlyFortySlug {
		t.Errorf("Slugify(%q) = %q, want %q", exactlyFortyName, got, exactlyFortySlug)
	}
	if err := ValidateSlug(got); err != nil {
		t.Errorf("the derived 40-char slug %q must pass the same contract an explicit --slug does: %v", got, err)
	}
}

// TestSlugifyAtFortyOneIsRefused pins the other side of the same boundary, so a
// mutant that widens the cap (`> 40` -> `> 41`) has somewhere to die.
func TestSlugifyAtFortyOneIsRefused(t *testing.T) {
	if got := legacyTruncate(fortyOneName); len(got) != 40 {
		t.Fatalf("CONTROL failure: %q used to derive %d chars, want a 41-char slug truncated to 40", fortyOneName, len(got))
	}
	got, err := Slugify(fortyOneName)
	if err == nil {
		t.Fatalf("Slugify(%q) = %q — 41 chars is one PAST the cap and must be refused", fortyOneName, got)
	}
	assertLengthRefusal(t, fortyOneName, err)
}

// TestShortAsciiDerivationIsUntouchedByTheLengthGuard is the positive control for
// the inverted mutant ("refuse every derived slug"). These rows have always been
// green; their job is to go RED if the refusal ever fires on an ordinary name.
func TestShortAsciiDerivationIsUntouchedByTheLengthGuard(t *testing.T) {
	for name, want := range map[string]string{
		"My Cool Block":     "my-cool-block",
		"Notepad":           "notepad",
		"  Spaced   Out  ":  "spaced-out",
		"Widget (v2) — Pro": "widget-v2-pro",
	} {
		got, err := Slugify(name)
		if err != nil {
			t.Errorf("Slugify(%q) = %v — the length guard must not touch a short name", name, err)
			continue
		}
		if got != want {
			t.Errorf("Slugify(%q) = %q, want %q", name, got, want)
		}
	}
}

// TestExplicitSlugKeepsItsOwnLengthMessage is the CONTROL for the behaviour the
// derived path is being made consistent WITH: it must not move. ValidateSlug is
// the server contract's mirror and answers on the slug, not on a name.
func TestExplicitSlugKeepsItsOwnLengthMessage(t *testing.T) {
	over := strings.Repeat("a", 41)
	err := ValidateSlug(over)
	if err == nil {
		t.Fatalf("ValidateSlug(%q) must refuse a 41-char slug", over)
	}
	if !strings.Contains(err.Error(), "must be 3-40 chars") {
		t.Errorf("the explicit-slug message moved: %q — #291 makes the DERIVED path agree with this one, it does not "+
			"rewrite this one", err)
	}
	// And the boundary on this side too, so the two paths provably share a cap.
	if err := ValidateSlug(strings.Repeat("a", 40)); err != nil {
		t.Errorf("ValidateSlug(40 chars) = %v, want nil — both paths cap at 40", err)
	}
}

// assertLengthRefusal checks the error is THIS guard's, not another one that
// happens to fire on the same input. A mutation test whose mutant dies on a
// different guard's error is green for the wrong reason.
func assertLengthRefusal(t *testing.T, name string, err error) {
	t.Helper()
	msg := err.Error()
	for _, want := range []string{
		"cannot derive a slug from", // the derivation family
		"the limit is 40",           // THIS guard: the length cap
		"--slug",                    // the escape hatch, named as the next thing to do
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("the refusal of %q does not contain %q — got: %s", name, want, msg)
		}
	}
	if strings.Contains(msg, "cannot appear in a blockId") {
		t.Errorf("the refusal of %q came from the LOSSY-CHARACTER guard, not the length guard: %s", name, msg)
	}
	if strings.Contains(msg, "not valid UTF-8") {
		t.Errorf("the refusal of %q came from the UTF-8 guard, not the length guard: %s", name, msg)
	}
}

// legacyTruncate reproduces the pre-#291 derivation tail — lowercase, fold
// non-slug runs to a hyphen, collapse, then CUT to 40 — so every "this used to
// produce X" claim above is a MEASUREMENT of the old algorithm rather than a
// remembered string. (Same reason `legacySlugify` exists in slug_lossy_test.go;
// this one is deliberately the length half only, so it stays readable next to
// the guard it is about.)
func legacyTruncate(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = nonSlugChars.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	return s
}
