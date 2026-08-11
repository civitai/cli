package cmd

import "strings"

// safeTerm strips terminal control characters from a SERVER-ORIGIN string before
// it is printed to the user's terminal, so a malicious uploader cannot inject
// ANSI/OSC escape sequences through model/version names, usernames, tags,
// creators, base-model labels, descriptions, download/image URLs, article
// titles/authors, trained words, or file names.
//
// Without this, a hostile field can perform terminal OUTPUT FORGERY: cursor
// moves + line-clears (\x1b[1A\x1b[2K) to overwrite the CLI's own "SHA256
// verified" line or hide a warning, an OSC-52 clipboard-set (\x1b]52;c;...\x07),
// or a window-title spoof.
//
// Removed: every C0 control byte (0x00–0x1F) and DEL (0x7F) EXCEPT newline and
// tab (legitimate layout), plus the C1 control range (U+0080–U+009F) — the 8-bit
// forms of the CSI/OSC escape introducers (0x9B = CSI, 0x9D = OSC, …). Ordinary
// printable and multibyte UTF-8 text passes through byte-for-byte.
//
// Only the HUMAN (table/detail) renderers call this. `--json` output is emitted
// RAW: control characters are already \uXXXX-escaped in spec-compliant JSON, so a
// pipe is safe and sanitizing would corrupt machine-readable bytes.
func safeTerm(s string) string {
	if !strings.ContainsFunc(s, isStrippedControl) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, ch := range s {
		if isStrippedControl(ch) {
			continue
		}
		b.WriteRune(ch)
	}
	return b.String()
}

// indentContinuation prefixes every line of s AFTER the first with pad, so a
// multi-line SERVER string stays visibly inside the list item or block that
// introduced it.
//
// 🔴 THIS IS THE SECOND HALF OF safeTerm, NOT COSMETICS (civitai/cli#367
// review). safeTerm keeps `\n` deliberately — legitimate layout — which means a
// server-supplied string is free to contain them, and until #367 no server field
// this CLI printed was unbounded free text. It now is: the orchestrator's
// failure reason. A reason of
//
//	"Workflow ID:\tspoofed\nStatus:\tsucceeded"
//
// rendered its continuation flush against the left margin, where it was
// indistinguishable from the real `civitai workflows get` header two lines
// above — output forgery achieved without a single control byte, so safeTerm
// cannot see it. Indenting is what makes a continuation line unable to occupy
// column zero, and therefore unable to impersonate a line the CLI itself wrote.
//
// It deliberately does NOT collapse the newlines: the reason is passed through
// verbatim (AGENTS.md item 13), so the fix is where the text SITS, not what it
// says. A trailing newline is left alone rather than padded into a line of
// whitespace.
func indentContinuation(s, pad string) string {
	if !strings.Contains(s, "\n") {
		return s
	}
	lines := strings.Split(s, "\n")
	for i := 1; i < len(lines); i++ {
		if lines[i] == "" {
			continue
		}
		lines[i] = pad + lines[i]
	}
	return strings.Join(lines, "\n")
}

// wrapServerText hard-wraps a SERVER string to width runes per line, preserving
// the line breaks the string already contains, and returns the result as one
// string ready for indentContinuation.
//
// 🔴 IT IS THE THIRD HALF OF safeTerm, AND THE HAZARD IS THE TERMINAL'S OWN SOFT
// WRAP. indentContinuation stops a `\n` in server text from starting a line at
// column zero — but a single line longer than the terminal is wrapped by the
// TERMINAL, and the part that spills over starts at column zero too, with no
// newline anywhere in the string for indentContinuation to see. Under a table
// that is the same forgery: attacker-chosen text at the column every real row
// begins at.
//
// 🔴 WHAT THIS BUYS, STATED AT THE SCOPE IT WAS MEASURED — IT IS NOT "ANY REASON
// OF ANY LENGTH IS SAFE", AND AN EARLIER VERSION OF THIS COMMENT SAID THAT. The
// CLI never asks the terminal how wide it is (`x/term.GetSize` appears nowhere
// in this repo); it wraps to a FIXED budget. So this guarantees only that the
// CLI emits no logical line longer than the budget. In a terminal NARROWER than
// the budget, the terminal still soft-wraps and text still reaches column zero,
// and nothing here can prevent it. The residual is stated in the README too.
//
// A second, narrower case of the same gap: the budget counts RUNES, and a rune
// is not a display cell. 38 East Asian wide runes occupy 76 columns, so a line
// this function considers 38 wide is 76 wide on screen. Fixing that needs a
// character-width table (a new dependency, or a hand-rolled one) and is filed
// separately rather than smuggled in here.
//
// It reuses wrapTokens (validate_print.go), the CLI's one greedy line filler, so
// the wrapped surfaces cannot disagree about how a line is broken.
//
// 🔴 WHAT IT CHANGES ABOUT THE TEXT, STATED PRECISELY: within a line, runs of
// whitespace collapse to one space (strings.Fields), and line breaks are
// inserted. The WORDS are untouched — nothing here reads them, classifies them,
// truncates on what they say or matches their wording (AGENTS.md item 13). The
// string's OWN newlines survive as line breaks, so its paragraph structure is
// not flattened. This is deliberately more than printWorkflow does with the same
// text (that surface keeps whitespace verbatim): a reason there sits under a
// heading, while here it sits under a TABLE, where a tab is an alignment hazard
// and a soft wrap is a forgery one.
func wrapServerText(s string, width int) string {
	if width < 1 {
		width = 1
	}
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		lines = append(lines, wrapTokens(hardSplitOverlong(strings.Fields(line), width), width)...)
	}
	return strings.Join(lines, "\n")
}

// hardSplitOverlong breaks any token wider than width into width-rune chunks, so
// that wrapTokens — which never splits a token — cannot be handed one it must
// overflow with.
//
// 🔴 THIS REVERSES A DELIBERATE CHOICE, AND THE CHOICE WAS WRONG. The first cut
// left an over-long token whole and pinned that as "overflowing one line is the
// lesser harm", reasoning about a truncated id being worse than a long line.
// That framing missed the actual trade: an over-long token ALREADY breaks the
// layout, so the choice was never "clean line versus corrupted id" — it was
// "silently overflowing into a column an attacker can forge a row in, versus a
// visibly broken token". Demonstrated, not hypothesised: a single token of
// U+2800 BRAILLE PATTERN BLANK padding around row-shaped text renders as a
// spaced table row, is ONE token to strings.Fields, is above U+009F so safeTerm
// keeps it, and passes the server's own `isLikelySafeMessage` (under 300 chars,
// single line, matches no unsafe pattern) — so it arrives verbatim, produced a
// 266-rune emitted line, and the terminal placed its tail at column zero.
//
// It is applied HERE and not in wrapTokens on purpose. wrapTokens is shared with
// printFinding, where findingTokens deliberately keeps a quoted span whole so
// remedy text stays pasteable; that surface renders the CLI's OWN findings and
// does not sit under a table. Splitting there would corrupt text a user is meant
// to copy in order to fix a hazard that only exists on this path.
//
// Residual, stated rather than rediscovered: a chunk boundary can fall inside a
// grapheme cluster, so a combining mark can be separated from its base. That is
// a rendering blemish on hostile input; it is not a forgery vector, and the
// display-width gap above is the more consequential of the two.
func hardSplitOverlong(tokens []string, width int) []string {
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		r := []rune(t)
		for len(r) > width {
			out = append(out, string(r[:width]))
			r = r[width:]
		}
		out = append(out, string(r))
	}
	return out
}

// isStrippedControl reports whether r is a terminal control character safeTerm
// removes: any C0 control or DEL (other than newline and tab), or any C1 control
// (U+0080–U+009F).
func isStrippedControl(r rune) bool {
	if r == '\n' || r == '\t' {
		return false
	}
	if r < 0x20 || r == 0x7f {
		return true
	}
	return r >= 0x80 && r <= 0x9f
}
