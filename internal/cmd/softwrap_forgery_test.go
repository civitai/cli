package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

// civitai/cli#605 and civitai/cli#624 — ONE CLASS: A SERVER OPERAND LONG ENOUGH
// TO SOFT-WRAP FORGES DISPLAY ROWS THAT NO EXISTING GUARD CAN SEE.
//
// `safeTermSingle` guarantees one line per LINE-BREAK RUNE it can see. A
// terminal's soft wrap has no rune, so a gate that closes the ANSI / `\n` / `\t`
// class leaves the LENGTH class wide open and the forged row lands anyway, with
// no control byte of any kind in the payload. Both issues carry the identical
// fork — bound the operand, or accept the residual in writing — and both say one
// decision governs both. This file is the measurement side of taking the first.
//
// # WHY THE ASSERTION IS A ROW COUNT AND NOT A SUBSTRING
//
// 🔴 A GUARD ON WORDS IS WALKABLE BY REWORDING — the repo's own rule, and both
// issues name it as the one closure that is NOT acceptable. The hazard is the
// GEOMETRY, so the guard asserts the geometry: how many display rows the emitted
// line occupies. The attacker's words appear below only in a CONTROL, where they
// establish that the fixture is load-bearing; no verdict is taken from them.
//
// # HOW A ROW COUNT IS ASSERTABLE BY A CLI THAT CANNOT MEASURE THE TERMINAL
//
// 🔴 `x/term.GetSize` APPEARS NOWHERE IN THIS REPO — only in comments noting its
// absence — so the CLI genuinely cannot ask how wide the terminal is. That looks
// like a blocker for pinning a row count and is not. Bound the OPERAND at N
// runes, and with a fixed CLI-owned format around it the emitted line is at most
// `overhead + N + 1` runes, hence at most `ceil((overhead+N+1)/W)` rows at EVERY
// width W. The tests below derive their expectation that way — `overhead` is
// MEASURED off a benign render rather than hand-counted, because a hand-counted
// bound is a second claim that can be wrong independently of the thing it checks
// (TestCheckTargetCollisionsCannotForgeARow's own first draft is the worked
// example). No terminal is measured and nothing here is timing-dependent.
//
// # THE SCOPE OF THE CLAIM, STATED RATHER THAN IMPLIED
//
// 🔴 THREE THINGS THIS DOES NOT PROVE, AND WRITING A SENTENCE THAT CLAIMS ANY OF
// THEM WOULD BE THE FOURTH FALSE ABSOLUTE IN THIS ARC:
//
//  1. **Runes are not display cells.** The cap counts runes; a double-width (CJK)
//     rune occupies two columns, so 120 runes can be 240 and every row count here
//     doubles. That gap is civitai/cli#397, and `wrapServerText`'s comment records
//     it for the surface IT guards. "Bounded in runes" is the honest phrase.
//  2. **A bounded tail can still begin at column zero.** Bounding caps HOW MANY
//     rows the server authors, not WHETHER one of them starts at column zero;
//     closing that needs the width. The value bought is the difference between one
//     forged continuation row and sixty-six of them.
//  3. **displayRows is a model, not a terminal capture.** It assumes a
//     fixed-width, non-reflowing terminal with no ANSI in play — exactly the
//     model both issues state for their own measurements. A terminal that reflows
//     on resize, or one honouring wide characters, lays the same bytes out
//     differently.

// swWidths are the widths every geometry assertion below is taken at, and they
// are NAMED rather than left as a bare list because one measurement is not a
// general claim.
//
//   - 80 is the BOUNDARY: the classic terminal width, and the one the #605
//     fixture is constructed against, so it is where a padded payload lands on a
//     row edge by design.
//   - 100 and 132 are the two widths #605's own negative control was measured at.
//   - 120 is the MIDDLE of the range people actually use (80–160).
//
// The derived bound holds at every width by construction; these four are where
// it is checked.
var swWidths = []int{80, 100, 120, 132}

// displayRows is how many display rows s occupies in a W-column terminal, under
// the model stated in this file's header. An empty logical line still occupies
// one row, which is why the max(1, …) is here and not folded into the division.
func displayRows(s string, width int) int {
	if width < 1 {
		panic("displayRows: width must be >= 1")
	}
	rows := 0
	for _, line := range strings.Split(s, "\n") {
		n := utf8.RuneCountInString(line)
		if n == 0 {
			rows++
			continue
		}
		rows += ceilDivRows(n, width)
	}
	return rows
}

// ceilDivRows is ceil(n/width) for positive n.
func ceilDivRows(n, width int) int { return (n + width - 1) / width }

// swForgedClaim is the text both fixtures forge. On the download path it is
// `(SHA256 verified)` — a claim about whether the bytes are the bytes, rendered
// before any bytes are verified — and it is quoted here ONLY by the controls.
const swForgedClaim = "Saved /home/u/model.safetensors (SHA256 verified)"

// swForgedRow is that claim padded to exactly 80 columns, so repeating it fills
// one 80-column display row per copy.
var swForgedRow = swForgedClaim + strings.Repeat(" ", 80-len(swForgedClaim))

// swRowsBeginningWith counts how many display rows of s START with prefix at the
// given width. It is the instrument behind the negative control, not behind any
// verdict: see this file's header on why no assertion may be spelled against the
// attacker's words.
func swRowsBeginningWith(s string, width int, prefix string) int {
	n := 0
	for _, line := range strings.Split(s, "\n") {
		r := []rune(line)
		for i := 0; i < len(r); i += width {
			end := i + width
			if end > len(r) {
				end = len(r)
			}
			if strings.HasPrefix(string(r[i:end]), prefix) {
				n++
			}
		}
	}
	return n
}

// swMaxRowsAt80 is the ABSOLUTE anchor — the only expectation in this file that
// does NOT scale with maxServerLineRunes.
//
// 🔴 WITHOUT IT EVERY ASSERTION HERE IS BLIND TO THE CAP BEING WIDENED, WHICH IS
// THE ONE EDIT MOST LIKELY TO UNDO #605/#624. Every other expectation is DERIVED
// from maxServerLineRunes, so a wider cap silently raises the bar it is checked
// against. With this constant REMOVED and the cap set to 5000, both geometry
// assertions stay green while the surfaces occupy 63 and 64 display rows at 80
// columns — the forgery essentially fully restored (66 before the bound). The
// realistic edit is not malice but tidiness: "two budgets is confusing,
// harmonise with pkg/civitai's snippet at 500".
//
// ⚠ NO PER-SURFACE ROW OR RUNE TOTAL IS QUOTED HERE, AND THREE DRAFTS OF THIS
// COMMENT ARE WHY. One said the cap-500 edit costs "7 rows directly above
// `Generate? [y/N]:`" — 7 is the DOWNLOAD line's figure; that surface costs 8
// (overhead 104 vs 21), i.e. a row count attached to the wrong surface, the
// exact defect round 1 fixed one file over. One quoted "225 runes / 142 runes"
// while safeterm.go, added by this same PR, forbids exactly that because 142 is
// one fixture's overhead and the same line reaches 155. One said "the only red
// at 5000 is …/multi-byte", which was true before this anchor existed and false
// in the tree it was written into — the anchor itself reds two more.
//
// So: the assertion computes every figure at run time and PRINTS it. Read the
// test output for today's numbers; do not carry them here. 3 is the value at
// which both surfaces pass today, and raising it is a deliberate act that has to
// be justified in this comment — which is the entire point of the constant.
//
// ⚠ IT PINS ROWS, NOT THE CAP, so it has slack and "the cap is pinned" would be
// the wrong sentence: the first cap value that reds the package is 136, because
// 135 still costs 3 rows on both surfaces. That is the contract working as
// stated, not a hole — but the headroom is real and worth knowing before anyone
// quotes this guard as pinning 120.
const swMaxRowsAt80 = 3

// assertRowsBounded is the ONE geometry assertion, shared by both surfaces so
// they cannot disagree about what "bounded" means (the repo's one-rule-one-place
// rule; a predicate open-coded at two sites is typically wrong at one of them).
//
// overhead is the rendered line's rune count with a ZERO-LENGTH operand, MEASURED
// by the caller off a benign render.
func assertRowsBounded(t *testing.T, issue, surface, got string, overhead int) {
	t.Helper()
	wantRunes := overhead + maxServerLineRunes + len([]rune(serverTextEllipsis))
	if rows := ceilDivRows(wantRunes, 80); rows > swMaxRowsAt80 {
		t.Errorf("%s: the DERIVED bound for %s is now %d display rows at 80 columns, and the anchor allows "+
			"at most %d. Every other expectation in this file scales with maxServerLineRunes (currently %d), "+
			"so widening the cap raises them all — this is the one check that does not move. A cap costing "+
			"more than %d rows at 80 columns hands the SERVER back control of how much screen it writes; if "+
			"that is intended, change swMaxRowsAt80 deliberately and say why.",
			issue, surface, rows, swMaxRowsAt80, maxServerLineRunes, swMaxRowsAt80)
	}
	if n := utf8.RuneCountInString(got); n > wantRunes {
		t.Errorf("%s FORGERY in %s: the emitted line is %d runes, and the operand cap allows at most %d "+
			"(%d of overhead + %d of operand + %d of ellipsis). An unbounded operand is what lets a SOFT "+
			"WRAP author display rows no line-break-rune guard can see:\n%q",
			issue, surface, n, wantRunes, overhead, maxServerLineRunes, len([]rune(serverTextEllipsis)), got)
		// The per-width rows are LOGGED before returning rather than asserted
		// again: one verdict per surface is enough, but a reader of the failure
		// needs the geometry it is about, and re-deriving it by hand from the
		// rune count is the "hand-counted bound" this file warns against.
		for _, w := range swWidths {
			t.Logf("%s %s: width %3d -> %d display row(s) (derived cap %d) [UNBOUNDED]",
				issue, surface, w, displayRows(got, w), ceilDivRows(wantRunes, w))
		}
		return
	}
	for _, w := range swWidths {
		want := ceilDivRows(wantRunes, w)
		rows := displayRows(got, w)
		if rows > want {
			t.Errorf("%s FORGERY in %s: at %d columns the emitted line occupies %d display rows, want at "+
				"most %d — the SERVER chose how many rows of your terminal this line takes:\n%q",
				issue, surface, w, rows, want, got)
		}
		t.Logf("%s %s: width %3d -> %d display row(s) (derived cap %d)", issue, surface, w, rows, want)
	}
}

// TestSafeTermBoundedBoundsTheOperand drives the shared helper directly, because
// the two call sites below can only show that SOMETHING bounded them.
//
// 🔴 THE RUNE-VERSUS-BYTE CASE IS THE POINT OF THIS TEST, NOT A COMPLETENESS
// SUBTEST. pkg/civitai's `snippet` — the repo's existing precedent for bounding
// server text, and the reason bounding here is consistency rather than invention
// — slices `s[:max]` on a BYTE index, so a cut landing inside a multi-byte UTF-8
// sequence emits invalid UTF-8 (civitai/cli#627). Copying that shape here would
// have been the obvious implementation. The `multi-byte` case fails on it, and
// carries its own control proving the fixture can tell the two apart.
func TestSafeTermBoundedBoundsTheOperand(t *testing.T) {
	t.Run("short values pass through byte-for-byte", func(t *testing.T) {
		const s = "weights.safetensors"
		if got := safeTermBounded(s); got != s {
			t.Errorf("safeTermBounded(%q) = %q, want it unchanged — the cap must not touch a value under it", s, got)
		}
	})

	t.Run("exactly at the cap is not truncated", func(t *testing.T) {
		s := strings.Repeat("a", maxServerLineRunes)
		got := safeTermBounded(s)
		if got != s {
			t.Errorf("safeTermBounded truncated a value of exactly %d runes; the cap is inclusive:\n got %q",
				maxServerLineRunes, got)
		}
		if strings.Contains(got, serverTextEllipsis) {
			t.Errorf("safeTermBounded marked a value of exactly %d runes as abbreviated", maxServerLineRunes)
		}
	})

	t.Run("one rune over the cap is truncated and marked", func(t *testing.T) {
		s := strings.Repeat("a", maxServerLineRunes+1)
		got := safeTermBounded(s)
		if n := utf8.RuneCountInString(got); n != maxServerLineRunes+1 {
			t.Errorf("safeTermBounded returned %d runes for a %d-rune input, want %d (cap + ellipsis)",
				n, maxServerLineRunes+1, maxServerLineRunes+1)
		}
		if !strings.HasSuffix(got, serverTextEllipsis) {
			t.Errorf("safeTermBounded truncated without marking it: %q does not end in %q — a silently "+
				"shortened value reads as the whole value", got, serverTextEllipsis)
		}
	})

	t.Run("multi-byte", func(t *testing.T) {
		// 🔴 THE FIXTURE'S ALIGNMENT IS THE WHOLE EXPERIMENT, AND THE FIRST
		// DRAFT OF IT PROVED NOTHING. A uniform repeat of a 3-byte rune puts byte
		// offset 120 exactly ON a rune boundary (120 = 3 x 40), so a byte-indexed
		// cut would have produced valid UTF-8 and this subtest would have passed
		// against the defect it exists to catch — and 120 is divisible by 2, 3 and
		// 4 alike, so EVERY uniform-width fixture has that problem. The 2-byte
		// ASCII prefix is what breaks the alignment: the cut then falls after
		// 118 bytes of 3-byte runes, which is 1 byte into a rune. The control
		// below is what makes that a measurement rather than an assumption.
		const cjk = "模型文件" // 4 runes, 12 bytes
		s := "v2" + strings.Repeat(cjk, 50)
		if utf8.RuneCountInString(s) <= maxServerLineRunes {
			t.Fatalf("CONTROL failure, not a finding: the fixture is %d runes, which is under the cap of "+
				"%d — this subtest would pass without the bound ever running",
				utf8.RuneCountInString(s), maxServerLineRunes)
		}
		// CONTROL on the fixture: a BYTE-indexed cut of this same value at the
		// same budget really does produce invalid UTF-8. Without this, "the
		// result is valid UTF-8" is indistinguishable from a fixture that no
		// byte-slicing implementation could have broken either.
		if utf8.ValidString(s[:maxServerLineRunes]) {
			t.Fatalf("CONTROL failure, not a finding: byte-slicing this fixture at %d produced VALID "+
				"UTF-8, so the assertion below cannot tell a rune bound from a byte bound",
				maxServerLineRunes)
		}
		got := safeTermBounded(s)
		if !utf8.ValidString(got) {
			t.Errorf("safeTermBounded emitted INVALID UTF-8 — the cut landed inside a multi-byte rune. "+
				"Bound by runes, never by bytes (this is pkg/civitai's `snippet` defect, civitai/cli#627):\n%q", got)
		}
		if n := utf8.RuneCountInString(got); n != maxServerLineRunes+1 {
			t.Errorf("safeTermBounded returned %d runes, want %d — the cap counts RUNES, and a %d-byte "+
				"value of %d runes must be cut at the same place a single-byte one is",
				n, maxServerLineRunes+1, len(s), utf8.RuneCountInString(s))
		}
	})

	t.Run("the strip still runs", func(t *testing.T) {
		// The cap is applied AFTER safeTermSingle, so the budget is spent on what
		// will actually be printed. A value under the cap must still lose the
		// class, or the cap has replaced the gate rather than joined it.
		got := safeTermBounded("weights\nSaved\ta\x1b[2Kb\u200b")
		if strings.ContainsAny(got, "\n\t") {
			t.Errorf("safeTermBounded left a line-break rune: %q", got)
		}
		if strings.ContainsRune(got, 0x1b) {
			t.Errorf("safeTermBounded left an ESC byte: %q", got)
		}
		if strings.ContainsRune(got, '\u200b') {
			t.Errorf("safeTermBounded left an invisible rune: %q", got)
		}
	})
}

// swHostileBalanceLength is civitai/cli#624's measured payload: 160 copies of a
// complete, plausible counterfeit `Cost:` line, 5,120 characters, and NOT ONE
// CONTROL RUNE. It is the payload the issue drove through the real runGenerate
// and found rendering as one logical line of 5,224 runes — zero ESC, zero TAB —
// which an 80-column terminal laid out as ~66 rows, most of them beginning at
// column zero with a counterfeit cost, a few rows above the real one on the last
// screen before an irreversible spend.
var swHostileBalanceLength = strings.Repeat("Cost: 1 Buzz (balance 999999).  ", 160)

// TestGenerateBuzzBalanceWarningIsLengthBounded is civitai/cli#624.
//
// 🔴 IT IS A SECOND TEST ON A SURFACE THAT ALREADY HAS ONE, AND THAT IS THE
// DESIGN RATHER THAN AN OVERSIGHT. TestGenerateBuzzBalanceWarningCannotForgeALine
// asserts the ESCAPE CLASS and the LOGICAL-LINE geometry, and the payload above
// passes every one of its assertions — one logical line, zero ESC, the CLI's
// sentence opening and closing on the same line, the same line count a benign
// render produces. Widening that test would have destroyed the evidence; this one
// RE-RUNS its predicates on the hostile payload and requires them to stay GREEN,
// so the blindness is demonstrated in the same run as the fix. If those
// predicates ever go red here it is a CONTROL failure — a different defect — not
// this test finding something.
func TestGenerateBuzzBalanceWarningIsLengthBounded(t *testing.T) {
	render := func(t *testing.T, msg string) string {
		t.Helper()
		withStdinTTY(t, false)
		var s genSeams
		s.balance = func(ctx context.Context) (int64, error) { return 0, errors.New(msg) }
		o := baseOpts()
		o.assumeYes = true
		c, _, errb := genCmd("")
		if err := runGenerate(c, s.deps(t), o); err != nil {
			t.Fatalf("CONTROL failure, not a finding: the run failed before or after the balance "+
				"warning, so this test is not measuring that surface: %v", err)
		}
		if s.balanceCalls != 1 {
			t.Fatalf("CONTROL failure, not a finding: the balance seam was called %d time(s), want 1 — "+
				"the warning under test is only printed when it fails", s.balanceCalls)
		}
		return errb.String()
	}

	warningLine := func(t *testing.T, out string) string {
		t.Helper()
		for _, ln := range gupLines(out) {
			if strings.Contains(ln, gupBalanceOpen) {
				return ln
			}
		}
		t.Fatalf("CONTROL failure, not a finding: no line of the render opens the balance warning:\n%s", out)
		return ""
	}

	// --- the overhead, MEASURED off a benign render -------------------------
	benign := render(t, gupBenignServerMessage)
	benignLine := warningLine(t, benign)
	overhead := utf8.RuneCountInString(benignLine) - utf8.RuneCountInString(gupBenignServerMessage)
	if overhead < 1 {
		t.Fatalf("CONTROL failure, not a finding: the benign warning line (%d runes) is no longer than its "+
			"own operand (%d runes), so the overhead measurement is wrong and every bound derived from it "+
			"would be:\n%q", utf8.RuneCountInString(benignLine), utf8.RuneCountInString(gupBenignServerMessage), benignLine)
	}

	hostile := render(t, swHostileBalanceLength)
	hostileLine := warningLine(t, hostile)
	if !strings.Contains(hostileLine, "Cost: 1 Buzz") {
		t.Fatalf("CONTROL failure, not a finding: the hostile balance error never reached the warning, so "+
			"nothing below measures whether it is bounded:\n%q", hostileLine)
	}

	// --- the BLINDNESS DEMONSTRATION ----------------------------------------
	// Every predicate TestGenerateBuzzBalanceWarningCannotForgeALine takes a
	// verdict from, re-run on this payload. They must all be GREEN: that is the
	// evidence the new assertion below sees something the old one structurally
	// cannot.
	if got, want := len(gupLines(hostile)), len(gupLines(benign)); got != want {
		t.Errorf("CONTROL failure, not a finding: the 5,120-char payload changed the stderr LINE COUNT "+
			"(%d vs %d). The #612 guard already covers that; this test exists for the case where it does "+
			"not fire, so a red here means the payload is exercising a different defect", got, want)
	}
	if strings.ContainsRune(hostile, 0x1b) {
		t.Error("CONTROL failure, not a finding: the 5,120-char payload put an ESC byte on stderr. It " +
			"carries no control rune at all, so this is the #612 escape-class guard firing on something " +
			"else — the blindness this test demonstrates is about LENGTH")
	}
	// 🔴 TWO PREDICATES WERE DELETED HERE AND ONE ADDED, BECAUSE THE DOCSTRING
	// ABOVE CLAIMS THIS BLOCK RE-RUNS *EVERY* VERDICT THE #612 GUARD TAKES AND AN
	// EARLIER DRAFT DID NOT MATCH THAT SENTENCE IN EITHER DIRECTION.
	//
	// DELETED — a TAB check, which was UNREACHABLE, and whose own error text
	// admitted it ("such a guard could not discriminate anyway"). This surface is
	// ui-styled; lipgloss expands \t to four spaces before the bytes reach the
	// writer, which is exactly why the #612 guard has no tab predicate either. A
	// check that cannot go red for ANY input is not a control.
	//
	// DELETED — the `!Contains(hostileLine, gupBalanceOpen)` half. warningLine
	// SELECTS the line by containing gupBalanceOpen, so that half was tautological.
	// The gupBalanceClose half does real work and is kept.
	//
	// ADDED — the opener COUNT. The #612 guard takes a verdict from `opens != 1`;
	// warningLine returns the first match and never counts, so this block was
	// NARROWER than its own docstring until now.
	if !strings.Contains(hostileLine, gupBalanceClose) {
		t.Errorf("CONTROL failure, not a finding: the CLI's own sentence no longer CLOSES on the line it "+
			"opens on. That is the #612 logical-line guard, which this payload is supposed to PASS:\n%q", hostileLine)
	}
	if opens := strings.Count(hostile, gupBalanceOpen); opens != 1 {
		t.Errorf("CONTROL failure, not a finding: the render opens the balance warning %d time(s), want 1. "+
			"That is the #612 opener-count verdict, which this payload is supposed to PASS; a red here means "+
			"the payload is exercising the rune-class defect rather than the LENGTH one", opens)
	}

	// --- the new assertion: the GEOMETRY the old guard cannot see -----------
	assertRowsBounded(t, "#624", "runGenerate's Buzz-balance warning", hostileLine, overhead)

	t.Logf("#624 payload: %d runes of operand rendered a %d-rune warning line (benign: %d runes)",
		utf8.RuneCountInString(swHostileBalanceLength), utf8.RuneCountInString(hostileLine),
		utf8.RuneCountInString(benignLine))
}
