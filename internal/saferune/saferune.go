// Package saferune owns the ONE rule this CLI has about which runes a
// SERVER-SUPPLIED string may put in front of a user, and nothing else.
//
// 🔴 IT IS A PACKAGE RATHER THAN A FUNCTION BECAUSE TWO PACKAGES ASK THE SAME
// QUESTION AND MUST NOT ANSWER IT DIFFERENTLY. `internal/cmd`'s safeTerm asks
// "may this rune reach the terminal"; `internal/genapi`'s hasPrintableContent
// asks "would this string still say anything once a renderer has removed the
// runes it removes". The second question is the first one's complement, and
// before civitai/cli#393 they were answered by two tables that disagreed:
// safeTerm stripped C0/C1 while hasPrintableContent used unicode.IsControl,
// which is Cc-only, so a reason made entirely of U+200B counted as content,
// displaced the categorical fallback, and rendered as `(the server reported: )`
// — an empty parenthetical. `internal/genapi` cannot import `internal/cmd`
// (the import runs the other way), so the shared answer lives below them both.
//
// # THE CLASS, AND WHY EACH PART OF IT IS IN
//
//   - unicode.Cc — every C0 control, DEL and C1, except newline and tab. This
//     is what safeTerm always removed: the ANSI/OSC introducers a hostile field
//     uses for terminal OUTPUT FORGERY (cursor-up + line-clear to overwrite the
//     CLI's own "SHA256 verified" line, an OSC-52 clipboard set, a window-title
//     spoof). The range test below IS unicode.Cc, exactly and by measurement —
//     TestCcFastPathIsExactlyTheCategory pins the equivalence.
//
//   - unicode.Cf — all 170 format characters. They are what #393 is about, and
//     Go already agrees they are not text: `unicode.IsPrint` and
//     `unicode.IsGraphic` are BOTH false for every one of them. Two hazards
//     live here. INVISIBILITY: U+200B ZERO WIDTH SPACE and friends are not
//     unicode.IsSpace, so `strings.Fields` does not split on them — text that
//     looks like several columns to a reader is ONE token to every wrapper in
//     this CLI, which is how a reason becomes a forged table row. REORDERING:
//     U+202A–U+202E and U+2066–U+2069 (the bidi embeddings, overrides and
//     isolates) change the order in which a terminal DISPLAYS a line, so what
//     the user reads is not what the bytes say. No other guard addressed that.
//
//   - blankButGraphic — the residue, six runes that render as nothing while
//     Unicode classifies them as a letter, a symbol or a mark. No category
//     captures "has no glyph", and Go ships no Default_Ignorable table, so this
//     is the one hand-written list in the package. It is not a denylist of
//     hazards seen in the wild: it is the complete set that a sweep of every
//     assigned code point flags, cross-checked against the Unicode NAMES in
//     `golang.org/x/text/unicode/runenames` — see saferune_test.go, which
//     re-derives it from those names on every run and fails if Unicode grows a
//     seventh.
//
// # WHAT IS DELIBERATELY NOT STRIPPED, AND WHAT THAT COSTS
//
//   - Mn/Mc/Me COMBINING MARKS. `é` written as `e` + U+0301 must survive, and
//     so must VARIATION SELECTOR-16 (U+FE0F, category Mn) — which is why an
//     emoji keeps its emoji presentation through this filter.
//
//   - RIGHT-TO-LEFT SCRIPT. Arabic and Hebrew LETTERS are Lo and are ordinary
//     content; only the bidi CONTROLS are Cf. Conflating the two would mangle
//     real text, and a guard that cannot tell them apart is not a guard.
//
//   - Zl/Zp (U+2028 LINE SEPARATOR, U+2029 PARAGRAPH SEPARATOR) and Zs. They
//     are unicode.IsSpace, so `strings.Fields` already splits on them and the
//     token hazard above cannot arise; stripping them would silently JOIN two
//     words. Residual, stated rather than rediscovered: a terminal that
//     implemented U+2028 as a line break would put its tail at column zero,
//     where indentContinuation cannot see it because there is no `\n`. No
//     terminal is known to do that, and it has not been measured here.
//
//   - Cn (unassigned) and Co (private use). Both are non-graphic to Go, so a
//     `!unicode.IsGraphic` rule would have swept them up — and that rule was
//     rejected for it. Cn is "unassigned IN GO'S TABLE", so stripping it
//     deletes any character newer than the toolchain's Unicode version,
//     silently, from legitimate text. Co renders as a vendor glyph or tofu:
//     visible, and neither invisible nor reordering.
//
// 🔴 THE ACCEPTED COST, WHICH IS REAL AND IS NOT A BUG REPORT. Stripping the
// whole of Cf takes U+200D ZERO WIDTH JOINER and U+200C ZERO WIDTH NON-JOINER
// with it. So a multi-person emoji ZWJ sequence degrades into its components
// (👨‍👩‍👧 renders as three people, not one family), a subdivision flag degrades to
// 🏴 (its tag characters are Cf), and — the sharpest one — Persian and Arabic
// text that uses ZWNJ to keep a prefix from joining renders JOINED. That is a
// content change to legitimate text, and it is the price of having no
// carve-outs: every exemption is a rune with exactly the property the class
// exists to remove (invisible, not a space, so an invisible separator), and a
// class with a hole in it is a class an attacker aims at. The maintainer chose
// STRIP over escaping for civitai/cli#393; escaping would keep the bytes at the
// cost of every surface having to render an escape.
package saferune

import (
	"strings"
	"unicode"
)

// blankButGraphic is the hand-written residue described above: runes that
// occupy no visible glyph while Unicode calls them a letter (Lo), a symbol (So)
// or a mark (Mn), so no category test finds them.
//
// U+2800 is the MEASURED counterexample from civitai/cli#382 — a single token of
// it padded around row-shaped text rendered as a spaced table row, was one token
// to strings.Fields, was above U+009F so safeTerm kept it, and passed the
// server's own `isLikelySafeMessage`. The Hangul fillers are the same trick with
// a different code point (U+3164 is the classic "invisible username"), which is
// the whole reason this is derived from a property rather than from the three
// runes the issue happened to name.
//
// Keep it sorted; the test that re-derives it from Unicode names reports the
// difference as a set, not as a diff.
var blankButGraphic = [...]rune{
	0x115F,  // HANGUL CHOSEONG FILLER
	0x1160,  // HANGUL JUNGSEONG FILLER
	0x2800,  // BRAILLE PATTERN BLANK
	0x3164,  // HANGUL FILLER
	0xFFA0,  // HALFWIDTH HANGUL FILLER
	0x16FE4, // KHITAN SMALL SCRIPT FILLER
}

// Stripped reports whether r must be removed from server-supplied text before a
// human renderer prints it.
//
// Newline and tab are kept: they are legitimate layout, and the guards against
// what a server can do WITH them live in internal/cmd (indentContinuation stops
// a `\n` from starting a line at column zero; wrapServerText stops a long line
// from being broken by the terminal at column zero). Stripping them here would
// silently repeal a promise those two make about paragraph structure.
func Stripped(r rune) bool {
	if r == '\n' || r == '\t' {
		return false
	}
	// unicode.Cc, spelled as a range test: the whole category is C0, DEL and
	// C1, so this is an exact restatement and not an approximation of it.
	if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
		return true
	}
	// Everything below U+00A0 that is left is printable ASCII.
	if r < 0xa0 {
		return false
	}
	for _, b := range blankButGraphic {
		if r == b {
			return true
		}
	}
	return unicode.Is(unicode.Cf, r)
}

// Strip removes every Stripped rune from s. It only ever removes: what survives
// is a subsequence of the input, in the input's order, byte-for-byte.
func Strip(s string) string {
	if !strings.ContainsFunc(s, Stripped) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, ch := range s {
		if Stripped(ch) {
			continue
		}
		b.WriteRune(ch)
	}
	return b.String()
}

// HasVisibleContent reports whether s would still show a reader anything: at
// least one rune that survives Strip and is not whitespace.
//
// 🔴 IT IS THE COMPLEMENT OF Strip AND MUST STAY THAT WAY. The two are used by
// different packages for different questions, and the seam between them is
// where civitai/cli#393 lived: whatever Strip removes, this must not count as
// content, or a string that renders as nothing is treated as something and
// displaces a fallback that said more. TestHasVisibleContentAgreesWithStrip
// pins the relationship over every rune rather than over a fixture.
//
// The whitespace clause is the second half of the same idea and is not new
// behaviour for the caller: `strings.TrimSpace` already emptied an all-space
// string before this was reached. What it adds is the mixed case — `"⠀ ⠀"`
// trims to itself, because U+2800 is not a space, and renders as three blanks.
func HasVisibleContent(s string) bool {
	for _, r := range s {
		if !Stripped(r) && !unicode.IsSpace(r) {
			return true
		}
	}
	return false
}
