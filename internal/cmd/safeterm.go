package cmd

import (
	"strings"

	"github.com/civitai/cli/internal/saferune"
)

// safeTerm strips the runes a SERVER-ORIGIN string may not put on the user's
// terminal, before it is printed, so a malicious uploader cannot inject
// ANSI/OSC escape sequences — or invisible and direction-altering characters —
// through model/version names, usernames, tags, creators, base-model labels,
// descriptions, download/image URLs, article titles/authors, trained words,
// file names, or an orchestrator failure reason.
//
// Without this, a hostile field can perform terminal OUTPUT FORGERY: cursor
// moves + line-clears (\x1b[1A\x1b[2K) to overwrite the CLI's own "SHA256
// verified" line or hide a warning, an OSC-52 clipboard-set (\x1b]52;c;...\x07),
// or a window-title spoof.
//
// 🔴 IT IS THE ONE GATE, AND WHAT IT REMOVES IS DEFINED ONE LAYER DOWN.
// `internal/saferune` owns the class — every C0 control and DEL except newline
// and tab, the C1 range (the 8-bit CSI/OSC introducers), all of `unicode.Cf`
// (zero-width spaces and joiners, the bidi overrides/embeddings/isolates that
// reverse the DISPLAYED order of a line) and the handful of runes that render
// as blank while Unicode calls them a letter or a symbol. Read that package's
// doc comment before changing what is stripped: it states what is deliberately
// KEPT (combining marks, right-to-left script, private-use, whitespace
// separators) and the cost the strip knowingly accepts. Do not add a second
// table here — civitai/cli#393 was two tables that disagreed.
//
// Ordinary printable and multibyte UTF-8 text passes through byte-for-byte.
//
// Only the HUMAN (table/detail) renderers call this. `--json` output is emitted
// RAW: control characters are already \uXXXX-escaped in spec-compliant JSON, so a
// pipe is safe and sanitizing would corrupt machine-readable bytes.
func safeTerm(s string) string {
	return saferune.Strip(s)
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
// is not a display cell. Measured at listReasonWrapWidth: 75 CJK runes occupy
// 150 columns, so a line this function considers inside a 75-wide budget is
// twice that on screen. Fixing it needs a character-width table (a new direct
// dependency, or a hand-rolled one) and is civitai/cli#397 — filed separately
// rather than smuggled in here.
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
// spaced table row, is ONE token to strings.Fields, and passes the server's own
// `isLikelySafeMessage` (under 300 chars, single line, matches no unsafe
// pattern) — so it arrived verbatim, produced a 266-rune emitted line, and the
// terminal placed its tail at column zero.
//
// 🔴 THAT PARTICULAR PAYLOAD NO LONGER REACHES HERE, AND THIS IS STILL LOAD
// BEARING. civitai/cli#393 added U+2800 (and every other invisible rune) to
// what safeTerm strips, so the demonstration above now arrives as
// `wf_forgedsucceeded` — one visibly-joined token, not a row. What it
// demonstrated is unchanged: an over-long single token, which a 200-character
// URL, an id or an unspaced foreign-script sentence produces without any
// invisible rune at all. Deleting this because its original fixture was closed
// upstream would reopen the overflow for every one of those.
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

// (isStrippedControl lived here. It WAS safeTerm's table; it is now
// saferune.Stripped, so that internal/genapi can ask the same question and get
// the same answer — civitai/cli#393 was two spellings of one rule that
// disagreed.)
