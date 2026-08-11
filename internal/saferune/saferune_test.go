package saferune

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"unicode"

	"golang.org/x/text/unicode/runenames"
)

// civitai/cli#393 — WHAT THIS FILE PINS IS A PROPERTY, NOT THREE CODE POINTS.
//
// The issue named U+2800, U+200B and U+202E because those are the three that
// were measured. A test asserting those three are stripped passes while the
// hazard sits in a fourth rune nobody enumerated, which is the SPELLED guard
// this repo keeps relearning to avoid — see AGENTS.md item 28's two dead phrase
// lists, and TestDistinctReasonSets_KeyIsInjective for the shape that worked.
//
// So the guards below sweep EVERY code point and compare Stripped against an
// oracle built from a DIFFERENT data source than the implementation:
//
//   - the implementation reasons in Unicode CATEGORIES (Cc, Cf) plus a
//     six-rune residue;
//   - the oracle reasons in Unicode NAMES, read out of
//     `golang.org/x/text/unicode/runenames` (a test-only import of a module
//     this repo already requires for `x/text/message`).
//
// A rune whose published NAME says it is zero-width, invisible, a filler or a
// bidi control, and which is not whitespace, must be stripped. That catches the
// fourth rune: if a future Unicode revision adds one, it arrives with a name and
// this fails, naming it.
//
// The opposite direction — "can it redden on correct input?" — is
// TestStripKeepsLegitimateText and TestStripDocumentedDegradations: ordinary
// CJK, emoji, accented Latin and right-to-left SCRIPT must all survive
// byte-for-byte, and the two places where legitimate text is knowingly changed
// are asserted as intended rather than left to be discovered.
//
// Every invisible rune below is written as a `\u` escape on purpose: a literal
// one is invisible in the source too, and staticcheck's ST1018 rejects it.

// suspectNameRe is the oracle. Every alternative is anchored and spelled out,
// because a loose pattern reddens on visible runes whose name merely contains a
// suspicious word — `DUPLOYAN AFFIX ATTACHED LEFT-TO-RIGHT SECANT` is a drawn
// glyph, not a bidi control, and an unanchored `LEFT-TO-RIGHT` matched it.
var suspectNameRe = regexp.MustCompile(`^(?:` +
	`ZERO WIDTH .*|` +
	`INVISIBLE .*|` +
	`BRAILLE PATTERN BLANK|` +
	`SOFT HYPHEN|` +
	`WORD JOINER|` +
	`MONGOLIAN VOWEL SEPARATOR|` +
	`ARABIC LETTER MARK|` +
	`FIRST STRONG ISOLATE|` +
	`POP DIRECTIONAL (?:FORMATTING|ISOLATE)|` +
	`(?:LEFT-TO-RIGHT|RIGHT-TO-LEFT) (?:MARK|EMBEDDING|OVERRIDE|ISOLATE)|` +
	`.*FILLER|` +
	`INTERLINEAR ANNOTATION .*|` +
	`TAG .*` +
	`)$`)

// oracleFlags reports whether the Unicode NAME of r says it is invisible or
// reorders text.
//
// Two refinements, both of which cost something and are stated rather than
// hidden:
//
//   - unicode.IsSpace is excluded. U+2028 LINE SEPARATOR and U+2029 PARAGRAPH
//     SEPARATOR match the name patterns and are deliberately NOT stripped (see
//     the package doc): they are whitespace, so every wrapper in this CLI
//     already splits on them and the "one unsplittable token" hazard cannot
//     arise there.
//
//   - unicode.IsPunct is excluded. `.*FILLER` also matches five drawn
//     punctuation marks — DEVANAGARI GAP FILLER, NEWA GAP FILLER, DIVES AKURU
//     GAP FILLER, KAWI PUNCTUATION SPACE FILLER and MANICHAEAN PUNCTUATION LINE
//     FILLER — which have glyphs. Punctuation is visible by construction, so it
//     is not part of the class.
func oracleFlags(r rune) bool {
	name := runenames.Name(r)
	if name == "" || strings.HasPrefix(name, "<") {
		return false // unnamed, unassigned, or a `<control>` placeholder
	}
	return suspectNameRe.MatchString(name) && !unicode.IsSpace(r) && !unicode.IsPunct(r)
}

// legacyStripped is the predicate that SHIPPED before #393 — safeTerm's
// C0/DEL/C1 table, spelled exactly as it stood. It is kept as the NEGATIVE
// CONTROL: the sweep reports how many runes it misses, so a green run here is a
// claim about a test that has been watched to go red rather than about a test
// nobody has ever seen fail.
func legacyStripped(r rune) bool {
	if r == '\n' || r == '\t' {
		return false
	}
	if r < 0x20 || r == 0x7f {
		return true
	}
	return r >= 0x80 && r <= 0x9f
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
// reports a serene pass. Measured today: 126. A floor of 100 leaves room for
// Unicode to retire a few without letting an instrument wired to nothing
// through.
const minOracleFlagged = 100

func TestStripsEveryInvisibleOrReorderingRune(t *testing.T) {
	flagged := oracleSet()
	if len(flagged) < minOracleFlagged {
		t.Fatalf("CONTROL failure, not a finding: the name oracle flagged %d rune(s), want >= %d. "+
			"The oracle is broken, and every assertion below would pass by checking nothing.",
			len(flagged), minOracleFlagged)
	}

	// NEGATIVE CONTROL, and it is the red half of the matrix: the predicate that
	// shipped before #393 must FAIL this sweep. If it passes, the sweep cannot
	// tell the fix from the bug it replaced.
	missedByLegacy := 0
	for _, r := range flagged {
		if !legacyStripped(r) {
			missedByLegacy++
		}
	}
	if missedByLegacy == 0 {
		t.Fatalf("CONTROL failure, not a finding: the pre-#393 predicate strips every rune this sweep flags, "+
			"so the sweep cannot tell the fix from the bug it replaced (%d flagged)", len(flagged))
	}
	t.Logf("oracle flagged %d rune(s); the pre-#393 predicate missed %d of them", len(flagged), missedByLegacy)

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

// The other direction of the same ledger: blankButGraphic must be EXACTLY the
// residue the categories do not reach. It fails when Unicode grows a seventh
// such rune (a name arrives that no category test catches) and equally when a
// rune is listed there that the oracle does not recognise — a hand-written list
// nobody re-derives is how a denylist starts.
func TestBlankButGraphicIsExactlyTheResidue(t *testing.T) {
	want := map[rune]bool{}
	for _, r := range oracleSet() {
		if !unicode.Is(unicode.Cc, r) && !unicode.Is(unicode.Cf, r) {
			want[r] = true
		}
	}
	if len(want) == 0 {
		t.Fatal("CONTROL failure, not a finding: the oracle flagged nothing outside Cc/Cf, so this ledger " +
			"compares an empty set with an empty set and proves nothing")
	}

	have := map[rune]bool{}
	for _, r := range blankButGraphic {
		have[r] = true
	}
	for r := range want {
		if !have[r] {
			t.Errorf("U+%04X %s renders as nothing and is neither Cc nor Cf, so no category test reaches it — "+
				"add it to blankButGraphic", r, runenames.Name(r))
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
// must survive byte-for-byte. RIGHT-TO-LEFT SCRIPT is the case that matters
// most: Arabic and Hebrew letters are ordinary text, only the bidi CONTROLS are
// not, and a filter that cannot tell them apart mangles real messages.
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
		{"emoji with a variation selector", "✌\ufe0f"},
		{"skin-tone modifier", "\U0001F44D\U0001F3FD"},
		{"box drawing and braille that is not blank", "┌──┐ ± × ÷ ≈ ⠿"},
		{"NBSP and ideographic space", "a\u00a0b\u3000c"},
		{"line and paragraph separators", "a\u2028b\u2029c"},
		{"private use", "a\uf8ffb"},
		{"replacement character", "a�b"},
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

// 🔴 THE ACCEPTED COST, PINNED SO IT CANNOT CHANGE SILENTLY IN EITHER
// DIRECTION. Stripping the whole of Cf takes ZWJ and ZWNJ with it. These are
// the cases where legitimate text renders differently afterwards, and they are
// asserted as INTENDED — if someone carves an exemption for U+200D, this fails
// and points at the package doc's argument for why a class with a hole in it is
// not a class.
func TestStripDocumentedDegradations(t *testing.T) {
	t.Run("an emoji ZWJ sequence degrades into its components", func(t *testing.T) {
		family := "\U0001F468\u200d\U0001F469\u200d\U0001F467"
		want := "\U0001F468\U0001F469\U0001F467"
		if got := Strip(family); got != want {
			t.Errorf("Strip(family emoji) = %q, want %q", got, want)
		}
	})
	t.Run("a subdivision flag degrades to the black flag", func(t *testing.T) {
		// The black flag plus the tag characters spelling "gbsct" and CANCEL
		// TAG. Every tag character is Cf.
		scotland := "\U0001F3F4\U000E0067\U000E0062\U000E0073\U000E0063\U000E0074\U000E007F"
		if got := Strip(scotland); got != "\U0001F3F4" {
			t.Errorf("Strip(Scotland flag) = %q, want the bare black flag", got)
		}
	})
	t.Run("Persian text loses its ZWNJ and renders joined", func(t *testing.T) {
		// می\u200cروم — "I go", whose prefix is held apart by a ZWNJ.
		withZWNJ := "می\u200cروم"
		joined := "میروم"
		if got := Strip(withZWNJ); got != joined {
			t.Errorf("Strip = %q, want %q", got, joined)
		}
	})
}

// Strip only ever REMOVES: no rune is added, replaced or reordered, and running
// it twice changes nothing. Without this a "helpful" future edit could replace
// the class with U+FFFD or with a visible escape and every other test here would
// still pass.
func TestStripOnlyRemovesAndIsIdempotent(t *testing.T) {
	corpus := []string{
		"", "plain", "a\u200bb", "⠀⠀wf_forged⠀⠀succeeded",
		"\u202emirror", "café 日本語 🚀", "a\x1b[2Kb", "\u200b", "\t\n",
		"می\u200cر", "mixed \u00ad soft \u2060 hyphen",
		"\u3164\u115f\u1160\uffa0\U00016FE4",
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
		{"⠀", false},
		{"⠀ ⠀", false}, // TrimSpace leaves this intact: U+2800 is not a space
		{"\u202e\u202c", false},
		{"\u200bx", true},
		{"\u00a0", false},
		{"\u3000x\u3000", true},
		{"\uf8ff", true}, // private use is not part of the class and is kept
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
	forged := strings.Repeat("⠀", 8) + "wf_forged" + strings.Repeat("⠀", 2) + "succeeded"

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
