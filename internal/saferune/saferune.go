// Package saferune owns the ONE rule this CLI has about which runes a
// SERVER-SUPPLIED string may put in front of a user, and nothing else.
//
// 🔴 SERVER-SUPPLIED. NOT "EVERY STRING THE CLI PRINTS". The CLI does not
// sanitise what the USER typed — a prompt, a flag value, a path, a workflow id
// the user passed on the command line is echoed back byte-for-byte, because the
// screen that precedes an irreversible spend has to show what will actually be
// sent. The first cut of civitai/cli#393 got this wrong: `safeTerm` was applied
// to `o.prompt`, so a typed Persian prompt (`می‌روم`, held apart by a ZWNJ)
// rendered joined on the approval screen while the graph on the wire carried
// the original. The screen stopped showing the job. Call sites are audited in
// both directions and pinned by TestApprovalScreenEchoesTypedInput_AndStripsServerText.
//
// 🔴 IT IS A PACKAGE RATHER THAN A FUNCTION BECAUSE TWO PACKAGES ASK THE SAME
// QUESTION AND MUST NOT ANSWER IT DIFFERENTLY. `internal/cmd`'s safeTerm asks
// "may this rune reach the terminal"; `internal/genapi`'s hasPrintableContent
// asks "would this string still say anything once a renderer has removed the
// runes it removes". The second is the first one's complement, and before #393
// they were answered by two tables that disagreed: safeTerm stripped C0/C1
// while hasPrintableContent used unicode.IsControl, which is Cc-only, so a
// reason made entirely of U+200B counted as content and rendered as
// `(the server reported: )` — an empty parenthetical. `internal/genapi` cannot
// import `internal/cmd` (the import runs the other way), so the shared answer
// lives below them both.
//
// # THE CLASS
//
//	Cc, minus \n and \t
//	+ Default_Ignorable_Code_Point
//	- U+FE0F VARIATION SELECTOR-16          (the one deliberate exception)
//	+ blankButGraphic                       (2 runes the property cannot reach)
//
// 🔴 IT IS DRAWN ON A UNICODE PROPERTY, AND THE FIRST CUT DREW IT ON A GENERAL
// CATEGORY INSTEAD — WRONG IN BOTH DIRECTIONS AT ONCE. That cut used
// `unicode.Cf`, while the argument for it was written as a property
// ("invisible, and not a space"). Category and property are not the same set:
//
//   - UNDER-INCLUSIVE, and the #393 symptom still reproduced through the gap.
//     U+034F COMBINING GRAPHEME JOINER is invisible and is `Mn`, so it was
//     KEPT: a failure reason of three CGJs rendered a `The server reported:`
//     heading with an invisible line under it, and `(the server reported: )` on
//     the excluded-output line — the exact defect #393 exists to close, in a
//     rune the fix did not cover. The same held for the 260 variation
//     selectors, U+17B4/17B5 and U+180B–U+180F.
//
//   - OVER-INCLUSIVE, and it deleted text Unicode requires be RENDERED. All 13
//     Prepended_Concatenation_Mark runes are `Cf` and all 13 are drawn: U+06DD
//     ARABIC END OF AYAH is the rosette around a Quranic verse number, U+070F
//     is the Syriac abbreviation mark. So were the 3 interlinear-annotation
//     (ruby) marks and the 16 Egyptian hieroglyph quadrat controls — 32 runes
//     in total, removed from text that needs them.
//
// Default_Ignorable_Code_Point is the Unicode property that means exactly
// "invisible; ignore in rendering", and its derivation already makes both
// corrections: it ADDS Other_Default_Ignorable_Code_Point (which carries
// U+034F, the Hangul fillers, U+17B4/17B5) and Variation_Selector, and it
// SUBTRACTS the three sets above. See defaultIgnorable for the derivation,
// quoted from UAX #44 and re-derived from Go's own tables rather than
// hand-listed.
//
// # THE ONE EXCEPTION, AND WHY IT IS ONE RATHER THAN NONE
//
// U+FE0F VARIATION SELECTOR-16 is Default_Ignorable and is KEPT. It is
// load-bearing: it is what makes an emoji render as an emoji rather than as
// monochrome text, so stripping it changes what the user sees of a legitimate
// message. Every other variation selector (VS1–15, VS17–256, the Mongolian
// free variation selectors) is stripped.
//
// 🔴 AN EARLIER VERSION OF THIS COMMENT ARGUED THERE COULD BE NO EXCEPTIONS —
// "a class with a hole in it is a class an attacker aims at" — and that
// argument was FALSE ON ITS OWN TERMS, because the class it defended had 263
// holes in it: every named rune with exactly the stated property that happened
// to be `Mn` rather than `Cf` was kept, unremarked. The rule is therefore not
// "no exceptions"; it is: the class is a PROPERTY, exceptions are explicit,
// enumerated and pinned, and there is currently exactly one. VS16 is defensible
// where a ZWJ carve-out would not be — VS16 changes how an adjacent glyph is
// drawn and cannot act as an invisible separator, while U+200D can and does.
//
// # WHAT IS DELIBERATELY NOT STRIPPED
//
//   - COMBINING MARKS that are not Default_Ignorable. `e`+U+0301 must survive.
//   - RIGHT-TO-LEFT SCRIPT. Arabic and Hebrew letters are `Lo`, ordinary
//     content; only the bidi CONTROLS are in the class. A filter that cannot
//     tell them apart mangles real messages.
//   - Zl/Zp/Zs (U+2028, U+2029, NBSP, ideographic space). They are
//     `unicode.IsSpace` — which is also why the DI derivation subtracts them —
//     so `strings.Fields` already splits on them and the unsplittable-token
//     hazard cannot arise; stripping would silently JOIN two words. Residual,
//     stated rather than rediscovered: a terminal that implemented U+2028 as a
//     line break would put its tail at column zero, where indentContinuation
//     cannot see it because there is no `\n`. Not measured; no terminal is
//     known to do it.
//   - Co (private use): renders as a vendor glyph or tofu. Visible, and neither
//     invisible nor reordering. Cn (unassigned) is likewise out of the class
//     EXCEPT where Unicode has explicitly reserved it as default-ignorable
//     (U+2065, U+FFF0–U+FFF8, U+E01F0–U+E0FFF); a blanket `!IsGraphic` rule was
//     rejected because it would delete any character newer than the toolchain's
//     Unicode tables from legitimate text.
//
// 🔴 THE ACCEPTED COST, WHICH IS REAL. The class contains U+200D ZERO WIDTH
// JOINER and U+200C ZERO WIDTH NON-JOINER, so text that depends on them renders
// differently: emoji ZWJ sequences break into their components, and at least
// nine scripts lose orthographic distinctions — Malayalam chillu (`ണ്‍` becomes
// `ണ്`, a DIFFERENT letter), Devanagari half-forms and conjuncts, Bengali,
// Tamil, Kannada, Sinhala repaya, Persian/Arabic ZWNJ, and Mongolian (both the
// vowel separator and the free variation selectors). The measured sheet is
// TestStripDocumentedDegradations. The maintainer chose STRIP over escaping for
// #393; this is what that costs, and it is written down rather than discovered.
package saferune

import (
	"strings"
	"unicode"
)

// emojiPresentationSelector is U+FE0F, the single documented exception to the
// class. See the package doc.
const emojiPresentationSelector = 0xFE0F

// blankButGraphic is what the property cannot reach: runes that occupy a cell
// and paint nothing, which Unicode classifies as a symbol or a mark and does
// NOT mark default-ignorable.
//
// U+2800 is the MEASURED counterexample from civitai/cli#382 — a single token
// of it padded around row-shaped text rendered as a spaced table row, was one
// token to strings.Fields, and passed the server's own `isLikelySafeMessage`.
//
// It was six entries before the class moved to Default_Ignorable_Code_Point;
// the four Hangul fillers are Other_Default_Ignorable_Code_Point and are now
// covered by the property itself. What remains is re-derived from Unicode NAMES
// on every test run by TestBlankButGraphicIsExactlyTheResidue, which fails both
// when the list grows a guess and when Unicode grows a third.
var blankButGraphic = [...]rune{
	0x2800,  // BRAILLE PATTERN BLANK
	0x16FE4, // KHITAN SMALL SCRIPT FILLER
}

// defaultIgnorable reports the Unicode Default_Ignorable_Code_Point property.
//
// 🔴 IT IS THE UAX #44 DERIVATION, RE-DERIVED FROM GO'S TABLES RATHER THAN
// HAND-LISTED, because a hand-listed copy is a snapshot that stops being true
// the next time the toolchain's Unicode version moves. DerivedCoreProperties'
// own derivation reads:
//
//	Default_Ignorable_Code_Point =
//	    Other_Default_Ignorable_Code_Point
//	  + Cf (Format characters)
//	  + Variation_Selector
//	  - White_Space
//	  - FFF9..FFFB   (Interlinear annotation format characters)
//	  - 13430..1343F (Egyptian hieroglyph format characters)
//	  - Prepended_Concatenation_Mark (Exceptional format characters that should be visible)
//
// Go ships every one of those as a range table except the two literal ranges,
// which the UCD itself states as literals. Three of the four subtractions are
// the entire over-inclusion fix: they are what keeps ARABIC END OF AYAH (13
// runes), the ruby marks (3) and the hieroglyph quadrat controls (16) out of
// the class.
//
// The White_Space subtraction is CURRENTLY A NO-OP — measured: the intersection
// is empty, because U+180E stopped being White_Space in Unicode 6.3 and no
// other format character is whitespace. It is kept because this is a quoted
// derivation rather than a minimised one, and it is labelled here rather than
// counted as coverage: no mutation test can kill a mutant that deletes it.
// TestDerivationSubtractionsAreLiveExceptWhiteSpace pins the emptiness, so the
// day it becomes live somebody is told.
func defaultIgnorable(r rune) bool {
	if unicode.Is(unicode.White_Space, r) {
		return false
	}
	if unicode.Is(unicode.Prepended_Concatenation_Mark, r) {
		return false
	}
	if r >= 0xFFF9 && r <= 0xFFFB {
		return false
	}
	if r >= 0x13430 && r <= 0x1343F {
		return false
	}
	return unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r) ||
		unicode.Is(unicode.Cf, r) ||
		unicode.Is(unicode.Variation_Selector, r)
}

// Stripped reports whether r must be removed from SERVER-SUPPLIED text before a
// human renderer prints it. It is not asked about text the user typed.
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
	if r == emojiPresentationSelector {
		return false
	}
	if defaultIgnorable(r) {
		return true
	}
	for _, b := range blankButGraphic {
		if r == b {
			return true
		}
	}
	return false
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
// pins the relationship over every code point rather than over a fixture.
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
