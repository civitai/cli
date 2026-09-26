package saferune

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/runenames"
)

// civitai/cli#393 — WHAT THIS FILE PINS IS A PROPERTY, NOT A LIST OF CODE
// POINTS.
//
// The issue named U+2800, U+200B and U+202E because those are the three that
// were measured. A test asserting those three are stripped passes while the
// hazard sits in a fourth rune nobody enumerated — which is exactly what
// happened to the first cut of this package: it drew the class on the general
// category `Cf`, and U+034F COMBINING GRAPHEME JOINER (invisible, `Mn`)
// reproduced the whole #393 symptom straight through it.
//
// So the guards below sweep EVERY code point and compare Stripped against an
// oracle built from a DIFFERENT data source than the implementation:
//
//   - the implementation reasons in Unicode PROPERTIES — Cc, and the UAX #44
//     derivation of Default_Ignorable_Code_Point — plus a two-rune residue;
//   - the oracle reasons in Unicode NAMES, read out of
//     `golang.org/x/text/unicode/runenames` (a test-only import of a module
//     this repo already requires for `x/text/message`).
//
// Both directions are asserted, because the category cut was wrong in both:
// TestStripsEveryInvisibleOrReorderingRune is under-inclusion (a rune whose
// published name says it is invisible must be stripped) and
// TestClassKeepsEveryRuneUnicodeSaysMustBeDrawn is over-inclusion (the 32 runes
// Unicode's own derivation carves OUT must survive).
//
// Every rune of the class is written as a `\u` escape in this file, U+2800
// included. The reason that applies to all of them is that a literal is
// invisible in the SOURCE too, so a reader cannot see what a fixture contains
// or that it contains anything. staticcheck's ST1018 enforces a strict subset
// of the same habit — it rejects a literal FORMAT character only, so it would
// not have flagged U+034F, a variation selector (both `Mn`) or U+2800 (`So`),
// and a green lint says nothing about those.

// suspectNameRe is the oracle. Every alternative is anchored and spelled out,
// because a loose pattern reddens on visible runes whose name merely contains a
// suspicious word — `DUPLOYAN AFFIX ATTACHED LEFT-TO-RIGHT SECANT` is a drawn
// glyph, not a bidi control, and an unanchored `LEFT-TO-RIGHT` matched it.
//
// 🔴 `COMBINING GRAPHEME JOINER` AND `VARIATION SELECTOR` ARE HERE BECAUSE THE
// AUDIT FOUND THEM MISSING. The previous pattern anchored `WORD JOINER` only,
// so the oracle could not see U+034F — and an oracle blind to the same runes as
// the implementation is not an oracle, it is a second copy. When adding to the
// class, add the name pattern too, or the sweep silently stops covering it.
var suspectNameRe = regexp.MustCompile(`^(?:` +
	`ZERO WIDTH .*|` +
	`INVISIBLE .*|` +
	`BRAILLE PATTERN BLANK|` +
	`SOFT HYPHEN|` +
	`(?:WORD|COMBINING GRAPHEME) JOINER|` +
	`MONGOLIAN (?:VOWEL SEPARATOR|FREE VARIATION SELECTOR .*)|` +
	`VARIATION SELECTOR(?:-[0-9]+)?|` +
	`ARABIC LETTER MARK|` +
	`FIRST STRONG ISOLATE|` +
	`POP DIRECTIONAL (?:FORMATTING|ISOLATE)|` +
	`(?:LEFT-TO-RIGHT|RIGHT-TO-LEFT) (?:MARK|EMBEDDING|OVERRIDE|ISOLATE)|` +
	`KHMER VOWEL INHERENT .*|` +
	`.*FILLER|` +
	`INTERLINEAR ANNOTATION .*|` +
	`TAG .*` +
	`)$`)

// mustBeDrawn is the set Unicode's own Default_Ignorable derivation SUBTRACTS —
// runes that are `Cf` and are nevertheless rendered. It is expressed as the
// property and the two literal ranges the UCD states, not as 32 hand-copied
// code points, so it tracks the toolchain's Unicode version.
func mustBeDrawn(r rune) bool {
	return unicode.Is(unicode.Prepended_Concatenation_Mark, r) ||
		(r >= 0xFFF9 && r <= 0xFFFB) || // interlinear annotation (ruby) marks
		(r >= 0x13430 && r <= 0x1343F) // Egyptian hieroglyph quadrat controls
}

// oracleFlags reports whether the Unicode NAME of r says it is invisible or
// reorders text, after the policy carve-outs.
//
// Each exclusion costs something and is stated rather than hidden:
//
//   - unicode.IsSpace. U+2028 LINE SEPARATOR and U+2029 PARAGRAPH SEPARATOR
//     match the name patterns and are deliberately NOT stripped (see the
//     package doc): they are whitespace, so every wrapper already splits on
//     them. Unicode's own derivation subtracts them for the same reason.
//   - unicode.IsPunct. `.*FILLER` also matches five DRAWN punctuation marks —
//     DEVANAGARI GAP FILLER, NEWA GAP FILLER, DIVES AKURU GAP FILLER, KAWI
//     PUNCTUATION SPACE FILLER, MANICHAEAN PUNCTUATION LINE FILLER.
//   - mustBeDrawn, for the same reason Unicode subtracts it.
//   - U+FE0F, the single documented exception, pinned on its own below.
func oracleFlags(r rune) bool {
	name := runenames.Name(r)
	if name == "" || strings.HasPrefix(name, "<") {
		return false // unnamed, unassigned, or a `<control>` placeholder
	}
	if !suspectNameRe.MatchString(name) {
		return false
	}
	if unicode.IsSpace(r) || unicode.IsPunct(r) || mustBeDrawn(r) {
		return false
	}
	return r != emojiPresentationSelector
}

// The two predicates that SHIPPED, kept as NEGATIVE CONTROLS. Every sweep
// reports how many runes each of them gets wrong, so a green run here is a
// claim about a test that has been watched to go red rather than about a test
// nobody has ever seen fail.
//
//   - legacyCcOnly is safeTerm's original C0/DEL/C1 table.
//   - firstCutCfOnly is the #393 fix as first written: category `Cf`, wrong in
//     both directions.
func legacyCcOnly(r rune) bool {
	if r == '\n' || r == '\t' {
		return false
	}
	if r < 0x20 || r == 0x7f {
		return true
	}
	return r >= 0x80 && r <= 0x9f
}

func firstCutCfOnly(r rune) bool {
	if legacyCcOnly(r) {
		return true
	}
	switch r {
	case 0x115F, 0x1160, 0x2800, 0x3164, 0xFFA0, 0x16FE4:
		return true
	}
	return unicode.Is(unicode.Cf, r)
}

// oracleSet is the flagged set, computed once — the sweep is 1.1M name lookups.
var oracleSet = sync.OnceValue(func() []rune {
	var out []rune
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if oracleFlags(r) {
			out = append(out, r)
		}
	}
	return out
})

// minOracleFlagged is the POSITIVE CONTROL on the oracle itself. A regex that
// has stopped matching — a renamed x/text API, a wrong table, a typo in an
// alternative — flags nothing, sweeps an empty set, finds no violations and
// reports a serene pass. Measured today: 385 (126 before the class moved off
// `Cf`). A floor of 300 leaves room for Unicode to retire some without letting
// an instrument wired to nothing through.
const minOracleFlagged = 300

func TestStripsEveryInvisibleOrReorderingRune(t *testing.T) {
	flagged := oracleSet()
	if len(flagged) < minOracleFlagged {
		t.Fatalf("CONTROL failure, not a finding: the name oracle flagged %d rune(s), want >= %d. "+
			"The oracle is broken, and every assertion below would pass by checking nothing.",
			len(flagged), minOracleFlagged)
	}

	// NEGATIVE CONTROLS, and they are the red half of the matrix. BOTH shipped
	// predicates must fail this sweep — including the `Cf` cut, which is what
	// makes this test a guard on the class MOVING rather than on the class
	// existing.
	missedByLegacy, missedByFirstCut := 0, 0
	for _, r := range flagged {
		if !legacyCcOnly(r) {
			missedByLegacy++
		}
		if !firstCutCfOnly(r) {
			missedByFirstCut++
		}
	}
	for _, c := range []struct {
		name string
		n    int
	}{{"the pre-#393 Cc-only predicate", missedByLegacy}, {"the first-cut Cf-only predicate", missedByFirstCut}} {
		if c.n == 0 {
			t.Fatalf("CONTROL failure, not a finding: %s strips every rune this sweep flags, so the sweep "+
				"cannot tell the current class from it (%d flagged)", c.name, len(flagged))
		}
	}
	t.Logf("oracle flagged %d rune(s); Cc-only missed %d, the Cf-only first cut missed %d",
		len(flagged), missedByLegacy, missedByFirstCut)

	var leaked []string
	for _, r := range flagged {
		if !Stripped(r) {
			leaked = append(leaked, fmt.Sprintf("U+%04X %s", r, runenames.Name(r)))
		}
	}
	if len(leaked) > 0 {
		t.Errorf("%d rune(s) whose Unicode NAME says they are invisible or reorder text survive Stripped:\n  %s\n\n"+
			"Any one of them is an invisible separator `strings.Fields` will not split on, or a control that "+
			"changes what the terminal DISPLAYS. Add the class, not the code point.",
			len(leaked), strings.Join(leaked, "\n  "))
	}
}

// 🔴 THE OVER-INCLUSION DIRECTION, WHICH THE `Cf` CUT FAILED SILENTLY. Thirteen
// Prepended_Concatenation_Marks, three interlinear-annotation marks and sixteen
// Egyptian quadrat controls are `Cf` AND are drawn — U+06DD ARABIC END OF AYAH
// is the rosette around a Quranic verse number. Stripping them deletes text
// Unicode requires be rendered, and no assertion in the first cut could see it
// because every test asked "is the invisible thing gone?".
func TestClassKeepsEveryRuneUnicodeSaysMustBeDrawn(t *testing.T) {
	var must []rune
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if mustBeDrawn(r) {
			must = append(must, r)
		}
	}
	if len(must) != 32 {
		t.Fatalf("CONTROL failure, not a finding: the must-be-drawn set has %d rune(s), want 32 "+
			"(13 Prepended_Concatenation_Mark + 3 interlinear + 16 Egyptian quadrat). The set is "+
			"mis-derived, so the assertions below prove nothing.", len(must))
	}

	// NEGATIVE CONTROL: the first cut stripped ALL of them. Without this the
	// test below cannot distinguish the fix from a predicate that never
	// stripped anything in this range.
	strippedByFirstCut := 0
	for _, r := range must {
		if firstCutCfOnly(r) {
			strippedByFirstCut++
		}
	}
	if strippedByFirstCut != len(must) {
		t.Fatalf("CONTROL failure, not a finding: the Cf-only first cut stripped %d of %d must-be-drawn runes, "+
			"want all of them — this test is not measuring the correction it claims to", strippedByFirstCut, len(must))
	}
	t.Logf("must-be-drawn: %d rune(s); the Cf-only first cut deleted all %d", len(must), strippedByFirstCut)

	for _, r := range must {
		if Stripped(r) {
			t.Errorf("U+%04X %s is stripped. Unicode's Default_Ignorable derivation subtracts it precisely "+
				"because it is a FORMAT character that is DRAWN; removing it deletes a mark the script needs.",
				r, runenames.Name(r))
		}
	}
}

// 🔴 ONE OF THE DERIVATION'S FOUR SUBTRACTIONS IS CURRENTLY A NO-OP, AND
// SAYING SO IS THE POINT OF THIS TEST.
//
// The mutation battery for this change deleted each subtraction in turn. Three
// of them (Prepended_Concatenation_Mark: 13 runes, interlinear: 3, Egyptian
// quadrat: 16) turn a test red. Deleting the White_Space subtraction changes
// NOTHING — the intersection is empty with this toolchain's Unicode tables,
// because U+180E MONGOLIAN VOWEL SEPARATOR stopped being White_Space in Unicode
// 6.3 and no other format character is whitespace.
//
// So that clause is an UNREACHABLE guard: no behavioural test can kill a mutant
// that removes it, and counting it as covered would be a false claim. It stays
// because the derivation is quoted from UAX #44 and a faithful copy is worth
// more than a minimal one — if a future Unicode version puts a format character
// back into White_Space, the clause is what stops the CLI deleting a space. This
// test pins the emptiness so that the day it stops being empty, someone is told
// the clause has become live and owes a behavioural case.
func TestDerivationSubtractionsAreLiveExceptWhiteSpace(t *testing.T) {
	base := func(r rune) bool {
		return unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r) ||
			unicode.Is(unicode.Cf, r) ||
			unicode.Is(unicode.Variation_Selector, r)
	}
	ws, pcm, interlinear, egyptian := 0, 0, 0, 0
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if !base(r) {
			continue
		}
		if unicode.Is(unicode.White_Space, r) {
			ws++
		}
		if unicode.Is(unicode.Prepended_Concatenation_Mark, r) {
			pcm++
		}
		if r >= 0xFFF9 && r <= 0xFFFB {
			interlinear++
		}
		if r >= 0x13430 && r <= 0x1343F {
			egyptian++
		}
	}
	if ws != 0 {
		t.Errorf("the White_Space subtraction now removes %d rune(s) from the class, so it has become "+
			"REACHABLE. It was documented as a no-op that no mutation test could kill; that is no longer "+
			"true and it needs a behavioural case of its own.", ws)
	}
	for _, c := range []struct {
		name string
		got  int
		want int
	}{
		{"Prepended_Concatenation_Mark", pcm, 13},
		{"interlinear annotation", interlinear, 3},
		{"Egyptian hieroglyph quadrat", egyptian, 16},
	} {
		if c.got != c.want {
			t.Errorf("the %s subtraction covers %d rune(s), want %d — the number moved, so the "+
				"must-be-drawn ledger is measuring a different set than the one it was written against",
				c.name, c.got, c.want)
		}
	}
	t.Logf("subtractions: White_Space %d (empty: unreachable by construction), PCM %d, interlinear %d, Egyptian %d",
		ws, pcm, interlinear, egyptian)
}

// 🔴 ONE EXCEPTION, AND EXACTLY ONE. U+FE0F is what makes an emoji render as an
// emoji; it is Default_Ignorable and is kept anyway. Every other variation
// selector is stripped. Asserted in both directions so the exception cannot
// quietly grow a second member, and so it cannot quietly disappear either.
func TestEmojiPresentationSelectorIsTheOnlyException(t *testing.T) {
	if Stripped(emojiPresentationSelector) {
		t.Errorf("U+FE0F VARIATION SELECTOR-16 is stripped. It is load-bearing: without it an emoji renders " +
			"as monochrome text, which changes what the user sees of a legitimate message.")
	}
	kept, strippedN := []string{}, 0
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if !unicode.Is(unicode.Variation_Selector, r) {
			continue
		}
		if Stripped(r) {
			strippedN++
			continue
		}
		if r != emojiPresentationSelector {
			kept = append(kept, fmt.Sprintf("U+%04X %s", r, runenames.Name(r)))
		}
	}
	if len(kept) > 0 {
		t.Errorf("the exception has grown to %d more variation selector(s):\n  %s\nThe package doc says there "+
			"is exactly one; a second undocumented one is how a class becomes a denylist.",
			len(kept), strings.Join(kept, "\n  "))
	}
	if strippedN < 200 {
		t.Fatalf("CONTROL failure, not a finding: only %d variation selector(s) are stripped, so the "+
			"exception above is not being measured against a populated set", strippedN)
	}
	t.Logf("variation selectors: %d stripped, 1 kept (U+FE0F)", strippedN)
}

// The other direction of the residue ledger: blankButGraphic must be EXACTLY
// what the property cannot reach. It fails when Unicode grows a third such rune
// and equally when a rune is listed there that the oracle does not recognise —
// a hand-written list nobody re-derives is how a denylist starts.
//
// It shrank from six to two when the class moved to Default_Ignorable: the four
// Hangul fillers are Other_Default_Ignorable_Code_Point, so the property now
// covers them and the ledger says so rather than leaving them duplicated.
func TestBlankButGraphicIsExactlyTheResidue(t *testing.T) {
	want := map[rune]bool{}
	for _, r := range oracleSet() {
		if !unicode.Is(unicode.Cc, r) && !defaultIgnorable(r) {
			want[r] = true
		}
	}
	if len(want) == 0 {
		t.Fatal("CONTROL failure, not a finding: the oracle flagged nothing outside Cc/Default_Ignorable, so " +
			"this ledger compares an empty set with an empty set and proves nothing")
	}

	have := map[rune]bool{}
	for _, r := range blankButGraphic {
		have[r] = true
	}
	for r := range want {
		if !have[r] {
			t.Errorf("U+%04X %s renders as nothing, is not Cc and is not Default_Ignorable, so no property "+
				"test reaches it — add it to blankButGraphic", r, runenames.Name(r))
		}
	}
	for r := range have {
		if !want[r] {
			t.Errorf("blankButGraphic lists U+%04X %q, which the name oracle does not recognise as invisible. "+
				"Either the oracle is too narrow or this entry is a guess; a list nobody can re-derive is a denylist.",
				r, runenames.Name(r))
		}
	}
}

// The C0/DEL/C1 range test in Stripped claims to BE unicode.Cc. If that stops
// being true the package doc is wrong, and the doc is what a reader trusts
// instead of re-deriving it.
func TestCcRangeTestIsExactlyTheCategory(t *testing.T) {
	n := 0
	for r := rune(0); r <= unicode.MaxRune; r++ {
		inRange := r < 0x20 || (r >= 0x7f && r <= 0x9f)
		if inRange {
			n++
		}
		if inRange != unicode.Is(unicode.Cc, r) {
			t.Fatalf("U+%04X: the range test says %v, unicode.Cc says %v", r, inRange, unicode.Is(unicode.Cc, r))
		}
	}
	if n != 65 {
		t.Errorf("the range covers %d code points, want 65 (0x00-0x1F, 0x7F-0x9F)", n)
	}
}

// 🔴 CAN IT REDDEN ON CORRECT INPUT? Everything here is legitimate content and
// must survive byte-for-byte. Two groups matter most: RIGHT-TO-LEFT SCRIPT
// (Arabic and Hebrew letters are ordinary text; only the bidi CONTROLS are not)
// and the runes Unicode's derivation carves out, which the previous class
// deleted.
func TestStripKeepsLegitimateText(t *testing.T) {
	for _, tc := range []struct{ name, in string }{
		{"ascii", "dreamshaper-8 failed: try again"},
		{"newline and tab", "a\tb\nc"},
		{"precomposed accents", "café — Résumé"},
		{"decomposed accents (combining marks)", "café — Résumé"},
		{"CJK", "日本語のモデルが見つかりません"},
		{"Korean", "한국어 텍스트"},
		{"Cyrillic and Greek", "Привет — Ελληνικά"},
		{"Arabic script", "لم يتم العثور على النموذج"},
		{"Hebrew script", "הדגם לא נמצא"},
		{"emoji", "🚀 done"},
		{"emoji with the presentation selector", "✌️"},
		{"skin-tone modifier", "\U0001F44D\U0001F3FD"},
		{"box drawing and braille that is not blank", "┌──┐ ± × ÷ ≈ ⠿"},
		{"NBSP and ideographic space", "a b　c"},
		{"line and paragraph separators", "a b c"},
		{"private use", "ab"},
		{"replacement character", "a�b"},
		// The carve-outs, each a rune the Cf-only class deleted.
		{"Arabic end of ayah", "القرآن\u06dd١"},
		{"Arabic number sign", "\u0600١٢"},
		{"Syriac abbreviation mark", "ܐ\u070fܒ"},
		{"Kaithi number sign", "\U000110bd\U00011066"},
		{"interlinear annotation (ruby)", "\ufff9漢\ufffa kan \ufffb"},
		{"Egyptian hieroglyph quadrat controls", "\U00013000\U00013430\U00013001"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Strip(tc.in); got != tc.in {
				t.Errorf("Strip mangled legitimate text:\n  in:  %q\n  got: %q", tc.in, got)
			}
			if !HasVisibleContent(tc.in) {
				t.Errorf("HasVisibleContent says %q says nothing", tc.in)
			}
		})
	}
}

// The RTL pair, asserted as a pair because that is the distinction: the SCRIPT
// survives, the OVERRIDE does not, and the words on either side of the override
// are still there.
func TestStripRemovesTheBidiControlAndKeepsTheScript(t *testing.T) {
	const arabic = "النموذج"
	in := "prompt \u202e" + arabic + "\u202c rejected"
	got := Strip(in)
	if strings.ContainsRune(got, 0x202e) || strings.ContainsRune(got, 0x202c) {
		t.Errorf("a bidi control survived: %q", got)
	}
	// POSITIVE CONTROL: nothing else was dropped, so the absence above is not
	// the whole string having been eaten.
	for _, want := range []string{"prompt ", arabic, " rejected"} {
		if !strings.Contains(got, want) {
			t.Errorf("Strip dropped %q along with the control: %q", want, got)
		}
	}
	if want := "prompt " + arabic + " rejected"; got != want {
		t.Errorf("Strip = %q, want %q — exactly the text with the two controls removed", got, want)
	}
}

// 🔴 THE DEGRADATION SHEET: WHAT LEGITIMATE TEXT LOSES, MEASURED AND PINNED SO
// IT CANNOT CHANGE SILENTLY IN EITHER DIRECTION.
//
// The class contains the join controls, so every script that uses them to make
// an orthographic distinction loses that distinction. The first version of this
// sheet listed two cases (emoji, Persian) and the audit measured eight more.
// Each expectation is written as the surviving SEQUENCE, spelled out — never as
// `Strip(in)` — so it cannot be derived from the implementation it tests.
//
// Malayalam is the sharpest: the chillu is a different LETTER, not a different
// shape of the same one.
//
// 🔴 EVERY ROW DECLARES ITS `script`, and that field is not decoration: it is
// what makes the package doc's COUNT derivable instead of hand-maintained.
// TestDegradationScriptsMatchThePackageDoc reads it. A row whose script is ""
// asserts "this degradation is not a script losing an orthographic distinction",
// and has to be named in nonScriptDegradations to say so out loud.
var documentedDegradations = []struct{ name, script, in, want, note string }{
	{
		"emoji ZWJ sequence", "", "\U0001F468\u200d\U0001F469\u200d\U0001F467",
		"\U0001F468\U0001F469\U0001F467", "one family becomes three people",
	},
	{
		"subdivision flag", "",
		"\U0001F3F4\U000E0067\U000E0062\U000E0073\U000E0063\U000E0074\U000E007F",
		"\U0001F3F4", "the tag characters are Cf; the flag falls back to black",
	},
	{
		"emoji TEXT presentation selector", "", "✈︎", "✈",
		"VS15 is stripped, so a deliberately-monochrome glyph may render as emoji",
	},
	{
		"Persian ZWNJ", "Persian/Arabic", "می\u200cروم", "میروم", "the prefix joins the stem",
	},
	{
		"Malayalam chillu", "Malayalam", "ണ്\u200d", "ണ്",
		"chillu-N becomes NA + virama — a DIFFERENT letter, not a variant shape",
	},
	{
		"Devanagari half-form", "Devanagari", "क्\u200dष", "क्ष",
		"the explicit half-form request is lost; the renderer picks its own conjunct",
	},
	{
		"Devanagari forced conjunct-break", "Devanagari", "क्\u200cष", "क्ष",
		"the explicit NON-joining request is lost, which is the opposite change",
	},
	{
		"Bengali conjunct", "Bengali", "ক্\u200dষ", "ক্ষ", "same mechanism as Devanagari",
	},
	{
		"Tamil non-joining", "Tamil", "க்\u200cஷ", "க்ஷ", "same mechanism",
	},
	{
		"Kannada half-form", "Kannada", "ಕ್\u200d", "ಕ್", "same mechanism",
	},
	{
		"Sinhala repaya", "Sinhala", "ර්\u200dය", "ර්ය", "the repaya form is lost",
	},
	{
		"Mongolian vowel separator", "Mongolian", "ᠮᠣᠩ\u180eᠭᠣᠯ", "ᠮᠣᠩᠭᠣᠯ", "U+180E is Cf",
	},
	{
		"Mongolian free variation selector", "Mongolian", "ᠨ᠋", "ᠨ",
		"FVS1 is Variation_Selector, newly in the class — it selects a letter's shape",
	},
}

// nonScriptDegradations names the rows that are NOT a script losing an
// orthographic distinction, so they do not count toward the package doc's
// number. Ledgered rather than inferred from `script == ""`, because a script row
// that merely FORGOT its label would otherwise be absorbed here silently and the
// published count would drift DOWN with the whole suite green — the same
// direction of failure as the "at least nine" this comment's subject used to be.
var nonScriptDegradations = map[string]string{
	"emoji ZWJ sequence":               "an emoji sequence, not a script",
	"subdivision flag":                 "an emoji flag sequence, not a script",
	"emoji TEXT presentation selector": "a presentation choice on one glyph, not an orthographic distinction",
}

func TestStripDocumentedDegradations(t *testing.T) {
	// CONTROL: the sheet is the thing under test, so an emptied or truncated
	// slice must fail here rather than pass vacuously.
	if len(documentedDegradations) < 13 {
		t.Fatalf("CONTROL failure, not a finding: the degradation sheet carries only %d row(s) — "+
			"rows were removed, so every assertion below covers less than it reads as covering",
			len(documentedDegradations))
	}
	for _, tc := range documentedDegradations {
		t.Run(tc.name, func(t *testing.T) {
			// CONTROL: the fixture really does carry something to lose.
			if tc.in == tc.want {
				t.Fatalf("CONTROL failure, not a finding: fixture and expectation are identical, so this row "+
					"asserts nothing (%s)", tc.note)
			}
			if got := Strip(tc.in); got != tc.want {
				t.Errorf("Strip(%q) = %q, want %q\n  %s", tc.in, got, tc.want, tc.note)
			}
		})
	}
}

// degradationScriptSet returns the distinct scripts the sheet measures, and
// fails when a row is neither labelled with one nor declared non-script.
func degradationScriptSet(t *testing.T) map[string]bool {
	t.Helper()
	set := map[string]bool{}
	for _, tc := range documentedDegradations {
		if tc.script == "" {
			if _, declared := nonScriptDegradations[tc.name]; !declared {
				t.Errorf("UNLABELLED ROW: degradation %q carries no `script` and is not in "+
					"nonScriptDegradations. Either name the script it costs a distinction in — which moves "+
					"the number the package doc publishes — or say here why it is not a script degradation. "+
					"An unlabelled row silently LOWERS the published count.", tc.name)
			}
			continue
		}
		if why, declared := nonScriptDegradations[tc.name]; declared {
			t.Errorf("CONTRADICTORY ROW: degradation %q is labelled script %q AND declared non-script "+
				"(%q). One of the two is wrong.", tc.name, tc.script, why)
		}
		set[tc.script] = true
	}
	// Bidirectional: a nonScriptDegradations entry for a row that no longer
	// exists reads as coverage of a case the sheet has stopped measuring.
	have := map[string]bool{}
	for _, tc := range documentedDegradations {
		have[tc.name] = true
	}
	for name := range nonScriptDegradations {
		if !have[name] {
			t.Errorf("STALE non-script declaration: %q is not a row in documentedDegradations. Remove the "+
				"entry, or restore the row it exempts.", name)
		}
	}
	return set
}

// numberWords is the spelling the package doc uses for the count. Only the range
// the sheet could plausibly occupy is listed; a count outside it must be spelled
// deliberately rather than guessed.
var numberWords = map[int]string{
	5: "FIVE", 6: "SIX", 7: "SEVEN", 8: "EIGHT", 9: "NINE",
	10: "TEN", 11: "ELEVEN", 12: "TWELVE", 13: "THIRTEEN", 14: "FOURTEEN", 15: "FIFTEEN",
}

// TestDegradationScriptsMatchThePackageDoc is the guard the "at least nine
// scripts" defect needed and did not have: the package doc published a COUNT of
// scripts, the measured sheet enumerated a different number, and nothing in the
// tree compared the two.
//
// 🔴 IT PINS THE RELATIONSHIP, NOT A WORD. Three legs, because a check on any one
// of them passes while the published sentence is false:
//
//  1. NAMING, both directions — every script the sheet measures must be named in
//     the package doc, and the doc's count must equal the number of distinct
//     scripts the sheet measures. A doc that names seven of eight, or says eight
//     and names eight while the sheet measures nine, reddens.
//  2. RETRACTION — the doc must not restate the retracted "nine scripts" /
//     "at least nine", in any spelling, and must not put an exhaustiveness word
//     on the set. The sheet is a FLOOR on the damage: every script that uses the
//     join controls orthographically loses the same distinctions whether or not a
//     row here names it, so "all"/"only"/"exhaustive"/"complete" would be a
//     sharper claim than anything measured.
//  3. ROWS ≠ SCRIPTS — the doc must say so, because that conflation IS the defect:
//     Devanagari and Mongolian each contribute two rows and one script, and three
//     rows contribute no script at all.
//
// The doc comment is read out of saferune.go rather than restated here; restating
// it would make this test pass against its own copy of the prose.
func TestDegradationScriptsMatchThePackageDoc(t *testing.T) {
	src, err := os.ReadFile("saferune.go")
	if err != nil {
		t.Fatalf("read saferune.go: %v", err)
	}
	doc := string(src)
	if i := strings.Index(doc, "\npackage saferune"); i >= 0 {
		doc = doc[:i]
	}
	// CONTROL: we really are holding the package doc, not an empty slice of it.
	if !strings.Contains(doc, "THE ACCEPTED COST") || len(doc) < 2000 {
		t.Fatalf("CONTROL failure, not a finding: the extracted package doc is %d byte(s) and does not "+
			"contain THE ACCEPTED COST — the extractor is reading the wrong region, so every assertion "+
			"below is about the wrong text", len(doc))
	}

	scripts := degradationScriptSet(t)
	// CONTROL: a non-trivial set, or leg 1 checks nothing.
	if len(scripts) < 5 {
		t.Fatalf("CONTROL failure, not a finding: the sheet measures only %d distinct script(s) (%v) — "+
			"too few for this guard to be meaningful", len(scripts), sortedKeys(scripts))
	}

	// The doc is a wrapped comment, so every assertion about a PHRASE has to run
	// against the whitespace-normalised form: "EIGHT\n// scripts" is the same
	// claim as "EIGHT scripts" and a raw Contains misses it. Measured — the first
	// cut of this guard read `doc` and reported the fixed comment as unfixed.
	flat := strings.Join(strings.Fields(strings.ReplaceAll(doc, "//", " ")), " ")

	// --- Leg 1: naming, both directions. ---
	for _, s := range sortedKeys(scripts) {
		if !strings.Contains(flat, s) {
			t.Errorf("the package doc does not name %q, which TestStripDocumentedDegradations MEASURES as "+
				"losing an orthographic distinction. The doc's list is what a reader takes as the cost of "+
				"the strip; a measured script missing from it is an undocumented cost.\n  measured: %v",
				s, sortedKeys(scripts))
		}
	}
	word, ok := numberWords[len(scripts)]
	if !ok {
		t.Fatalf("the sheet now measures %d distinct scripts (%v), which numberWords does not spell. Add "+
			"the spelling AND update the package doc — do not drop the count", len(scripts), sortedKeys(scripts))
	}
	if !strings.Contains(flat, word+" scripts") {
		t.Errorf("the package doc does not state %q. The sheet measures %d distinct script(s) (%v), and the "+
			"doc publishes a count — they have to be the same number. That they were not is civitai/cli's "+
			"'at least nine scripts' defect: the doc said nine and enumerated eight.",
			word+" scripts", len(scripts), sortedKeys(scripts))
	}

	// --- Leg 2: the retraction, and no exhaustiveness word. ---
	//
	// 🔴 THE BAN IS SCOPED TO AN ASSERTION, because a comment that RETRACTS a
	// claim has to be able to quote it. This is the "too STRICT" half of the bug
	// internal/cmd's readmeAssertsFlagWinsRule documents: a bare substring ban on
	// the retracted sentence also forbids the sentence retracting it, and the
	// retraction is the most useful prose in the comment. Same structural rule as
	// there: an occurrence bracketed by quotation delimiters is the doc talking
	// ABOUT the claim; anything else is the doc making it.
	for _, banned := range []string{"at least nine scripts", "nine scripts lose"} {
		if assertsRetractedCount(flat, banned) {
			t.Errorf("the package doc ASSERTS the retracted claim %q (unquoted). Nine was never a count of "+
				"scripts — it is the non-emoji ROW count of a ten-row sheet. Say what the enumeration "+
				"supports; quote the old claim only to retract it.", banned)
		}
	}
	// `\b` on both ends, so the RETRACTION's own word "exhaustiveness" — which is
	// the doc telling an editor not to add one — does not trip the ban on
	// "exhaustive".
	for _, claim := range []string{
		`all scripts`, `only scripts`, `exhaustive`, `the complete set`, `a complete list`,
	} {
		re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(claim) + `\b`)
		if re.MatchString(flat) {
			t.Errorf("the package doc claims %q about the degradation set. The sheet is a FLOOR: any script "+
				"using the join controls orthographically loses the same distinctions whether or not a row "+
				"names it, so no closed-set word is supported.", claim)
		}
	}

	// --- Leg 3: rows are not scripts, and the doc must say so. ---
	if len(documentedDegradations) == len(scripts) {
		t.Fatalf("CONTROL failure, not a finding: the sheet has %d rows and %d scripts. Leg 3 exists because "+
			"those numbers DIFFER; equal, the conflation it guards against cannot happen and the assertion "+
			"below is vacuous", len(documentedDegradations), len(scripts))
	}
	for _, want := range []string{"NOT THE ROW COUNT", "NOT A CEILING"} {
		if !strings.Contains(flat, want) {
			t.Errorf("the package doc does not state %q. It publishes a script count over a sheet with %d "+
				"rows and %d distinct scripts; a reader who reads the count as the row count, or as a "+
				"ceiling on the damage, is wrong in a way the doc has to pre-empt.",
				want, len(documentedDegradations), len(scripts))
		}
	}
}

// retractedCountDelims are the characters that, bracketing an occurrence, mark it
// as a MENTION of the claim rather than an ASSERTION of it.
var retractedCountDelims = map[rune]bool{
	'"': true, '“': true, '”': true, '\'': true, '`': true, '‘': true, '’': true,
}

// assertsRetractedCount reports whether `flat` STATES the retracted count rather
// than quoting it in order to retract it.
func assertsRetractedCount(flat, claim string) bool {
	re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(claim))
	for _, loc := range re.FindAllStringIndex(flat, -1) {
		before, _ := utf8.DecodeLastRuneInString(flat[:loc[0]])
		after, _ := utf8.DecodeRuneInString(flat[loc[1]:])
		if retractedCountDelims[before] && retractedCountDelims[after] {
			continue // quoted: the doc is retracting the claim, not making it
		}
		return true
	}
	return false
}

// TestAssertsRetractedCountPredicate validates the INSTRUMENT before its verdict
// is read in TestDegradationScriptsMatchThePackageDoc. Both directions, because
// the predicate it replaces was wrong in both: a bare substring ban forbids the
// retraction (false positive), and no ban at all lets the claim back in (false
// negative).
func TestAssertsRetractedCountPredicate(t *testing.T) {
	const claim = "at least nine scripts"
	for _, tc := range []struct {
		name  string
		in    string
		asrts bool
	}{
		// NEGATIVE CONTROLS: the predicate must be able to say "no".
		{"quoted retraction", `used to say "at least nine scripts" and enumerated eight`, false},
		{"backticked retraction", "the `at least nine scripts` wording was never a script count", false},
		{"absent", "EIGHT scripts are MEASURED to lose orthographic distinctions", false},
		// POSITIVE CONTROLS: it must be able to say "yes" — including the exact
		// shape the defect shipped in, and a re-WRAPPED restatement, which is what
		// a comment reflow produces.
		{"bare assertion", "emoji sequences break, and at least nine scripts lose distinctions", true},
		{"capitalised", "At least nine scripts lose orthographic distinctions", true},
		{"half-quoted", `we said "at least nine scripts and more`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flat := strings.Join(strings.Fields(tc.in), " ")
			if got := assertsRetractedCount(flat, claim); got != tc.asrts {
				t.Errorf("assertsRetractedCount(%q) = %v, want %v", tc.in, got, tc.asrts)
			}
		})
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Strip only ever REMOVES: no rune is added, replaced or reordered, and running
// it twice changes nothing. Without this a "helpful" future edit could replace
// the class with U+FFFD or with a visible escape and every other test here
// would still pass.
func TestStripOnlyRemovesAndIsIdempotent(t *testing.T) {
	corpus := []string{
		"", "plain", "a\u200bb", "\u2800\u2800wf_forged\u2800\u2800succeeded",
		"\u202emirror", "café 日本語 🚀", "a\x1b[2Kb", "\u200b", "\t\n",
		"می\u200cر", "mixed \u00ad soft \u2060 hyphen",
		"\u3164\u115f\u1160\uffa0\U00016FE4", "a͏b", "ا\u06dd١",
	}
	for _, in := range corpus {
		got := Strip(in)
		if !isSubsequence(got, in) {
			t.Errorf("Strip(%q) = %q, which is not a subsequence of the input — it added or reordered runes", in, got)
		}
		if again := Strip(got); again != got {
			t.Errorf("Strip is not idempotent: %q -> %q -> %q", in, got, again)
		}
		for _, r := range got {
			if Stripped(r) {
				t.Errorf("Strip(%q) left U+%04X, which Stripped rejects", in, r)
			}
		}
	}
}

func isSubsequence(sub, of string) bool {
	i := 0
	subr := []rune(sub)
	for _, r := range of {
		if i < len(subr) && subr[i] == r {
			i++
		}
	}
	return i == len(subr)
}

// 🔴 THE SEAM, WHICH IS WHERE #393 ACTUALLY LIVED. internal/cmd decides what a
// terminal may receive and internal/genapi decides whether a string still says
// anything; they were two tables and they disagreed. This asserts the
// RELATIONSHIP over every code point rather than over a fixture: a string says
// something exactly when what survives Strip is not all whitespace.
//
// It carries its own negative control — `unicode.IsControl`, the predicate
// genapi used — so the number of code points on which the old answer was wrong
// is reported rather than assumed.
func TestHasVisibleContentAgreesWithStrip(t *testing.T) {
	legacyWrong := 0
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue // surrogates cannot appear in a Go string as themselves
		}
		s := string(r)
		want := strings.TrimSpace(Strip(s)) != ""
		if got := HasVisibleContent(s); got != want {
			t.Fatalf("U+%04X %s: HasVisibleContent = %v but what survives Strip is %q. A string that renders "+
				"as nothing must not count as content — that is the empty parenthetical in civitai/cli#393.",
				r, runenames.Name(r), got, Strip(s))
		}
		if (!unicode.IsControl(r)) != want {
			legacyWrong++
		}
	}
	if legacyWrong == 0 {
		t.Fatal("CONTROL failure, not a finding: unicode.IsControl gives the same answer as the fix on every " +
			"code point, so this test cannot tell them apart")
	}
	t.Logf("unicode.IsControl (the pre-#393 predicate) disagrees with the rendered result on %d code point(s)", legacyWrong)

	// Multi-rune strings, including the shapes TrimSpace alone cannot reach.
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"", false},
		{"x", true},
		{" x ", true},
		{"   ", false},
		{"\u200b", false},
		{"\u200b\u200b\u200b", false},
		{"\u2800", false},
		{"\u2800 \u2800", false}, // TrimSpace leaves this: U+2800 is not a space
		{"\u202e\u202c", false},
		{"͏͏͏", false}, // the rune the Cf-only class kept
		{"\u200bx", true},
		{" ", false},
		{"　x　", true},
		{"", true},      // private use is not in the class and is kept
		{"\u06dd", true}, // must-be-drawn: a rune, therefore content
		{"️", true},      // the documented exception
	} {
		if got := HasVisibleContent(tc.in); got != tc.want {
			t.Errorf("HasVisibleContent(%q) = %v, want %v (survives Strip as %q)", tc.in, got, tc.want, Strip(tc.in))
		}
	}
}

// The measured vector from civitai/cli#382, restated as what #393 changes about
// it: U+2800-padded text READS as a table row and is ONE token to strings.Fields.
// After the strip it is neither.
func TestBlankPaddedRowShapedTextIsNoLongerRowShaped(t *testing.T) {
	forged := strings.Repeat("\u2800", 8) + "wf_forged" + strings.Repeat("\u2800", 2) + "succeeded"

	// CONTROL: before the strip it really is one token that looks like a row.
	if n := len(strings.Fields(forged)); n != 1 {
		t.Fatalf("CONTROL failure, not a finding: the fixture is %d token(s), not the single unsplittable one the "+
			"hazard needs", n)
	}
	got := Strip(forged)
	if strings.ContainsRune(got, 0x2800) {
		t.Errorf("the padding survived: %q", got)
	}
	if got != "wf_forgedsucceeded" {
		t.Errorf("Strip = %q, want the two words with the invisible padding gone — visibly one word, which is "+
			"the point: it can no longer pose as two aligned columns", got)
	}
}
