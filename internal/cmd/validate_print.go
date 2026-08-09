package cmd

// validate_print.go owns the LAYOUT of a validation finding in the terminal.
//
// 🔴 THE SPLIT IS THE INVERSE OF AGENTS.md ITEM 23, AND IT IS LOAD-BEARING IN
// BOTH DIRECTIONS. Item 23 says a finding's `Field` must come from the PRODUCER
// and never be re-derived at the printer. Its layout is the other half of the
// same rule: `Finding.Message` is a `--json` string field, so it must stay ONE
// LINE on the wire, and the line breaking has to happen here. Wrapping inside
// the message would corrupt `--json` for exactly the consumers item 23 exists to
// serve — and hard-wrapping a message at the producer also fixes a width the
// producer cannot see.
//
// What drove it: the ready-ack advisory (internal/validate/readyack.go) is a
// single ~2 kB paragraph. Printed with `fmt.Fprintf(w, "  - %s\n", …)` it was
// ONE 1938-character line — unreadable in any terminal, and unreadable in a
// scrollback. Wrapping HERE fixes that message and every other long one at the
// same time, including any added later, which is why the fix is at the shared
// print site rather than in the one message that provoked it.

import (
	"fmt"
	"io"
	"strings"
)

const (
	// findingWrapWidth is the total column width a finding is wrapped to. 79
	// matches exitCodeHelpWidth: the CLI's two long-prose surfaces should not
	// disagree about how wide a terminal is.
	findingWrapWidth = 79
	// findingBullet leads the first line; findingIndent is the hanging indent
	// for continuations. They are the SAME WIDTH on purpose, so the text column
	// is straight and a wrapped finding still reads as one list item.
	findingBullet = "  - "
	findingIndent = "    "
)

// printFinding writes one finding's message as a wrapped bulleted list item.
//
// It is deliberately whitespace-normalising, so a message that acquired a stray
// newline still prints as a clean block rather than breaking the indent.
func printFinding(w io.Writer, msg string) {
	width := findingWrapWidth - len(findingIndent)
	for i, line := range wrapTokens(findingTokens(msg, width), width) {
		lead := findingIndent
		if i == 0 {
			lead = findingBullet
		}
		fmt.Fprintf(w, "%s%s\n", lead, line)
	}
}

// findingTokens splits msg into wrap units, keeping a DOUBLE-QUOTED span whole.
//
// 🔴 REMEDY TEXT IS MEANT TO BE PASTED, AND A GREEDY WRAP SPLIT IT. Measured on
// the lockfile finding: `"buildCommand": "pnpm run build"` — which the message
// tells the author to put in their manifest — came out as `"pnpm` / `run
// build"` across a line break, where the unwrapped base printed it whole. That
// is a user-visible regression introduced BY the wrapping fix.
//
// Two guards keep this from doing harm, and both are load-bearing:
//   - A span that would not FIT in `width` is split back into its words. An
//     atomic span wider than the budget would blow the width contract, which is
//     the very thing wrapping is for.
//   - An UNBALANCED quote (an odd `"` with no partner, e.g. a 6" measurement)
//     never swallows the tail: the accumulator is flushed as ordinary words at
//     end of input. Worst case is exactly today's greedy behaviour.
//
// Only `"` is grouped. An apostrophe cannot be: `project's` would open a span
// that never closes, and these messages are full of them.
func findingTokens(msg string, width int) []string {
	var (
		out  []string
		span []string
		open bool
	)
	flushUnbalanced := func() {
		out = append(out, span...)
		span, open = nil, false
	}
	for _, w := range strings.Fields(msg) {
		odd := strings.Count(w, `"`)%2 == 1
		if !open {
			if !odd {
				out = append(out, w)
				continue
			}
			span, open = []string{w}, true
			continue
		}
		span = append(span, w)
		if !odd {
			continue
		}
		// The span closed. Keep it whole only if it can fit on a line.
		if joined := strings.Join(span, " "); len([]rune(joined)) <= width {
			out = append(out, joined)
		} else {
			out = append(out, span...)
		}
		span, open = nil, false
	}
	if open {
		flushUnbalanced()
	}
	return out
}

// wrapTokens greedily fills lines of at most width RUNES from pre-split tokens.
//
// 🔴 RECONCILE NOTE: `wrapRunes` in exitcodes_doc.go (another agent's file this
// round) is exactly `wrapTokens(strings.Fields(s), width)`. The two are not
// duplicated logic by accident — the assembly is identical and only the
// TOKENIZER differs, which is the whole point of splitting them. If both land,
// collapse `wrapRunes` into a one-line call to this.
func wrapTokens(tokens []string, width int) []string {
	if len(tokens) == 0 {
		return []string{""}
	}
	var (
		lines []string
		cur   string
		n     int
	)
	for _, t := range tokens {
		lw := len([]rune(t))
		if cur == "" {
			cur, n = t, lw
			continue
		}
		if n+1+lw > width {
			lines = append(lines, cur)
			cur, n = t, lw
			continue
		}
		cur += " " + t
		n += 1 + lw
	}
	return append(lines, cur)
}

// unwrapFinding is the inverse, for tests: collapse the printed block back to
// the single logical line the producer emitted. It exists so an assertion about
// a message's CONTENT does not have to know where the layout chose to break —
// the failure mode a substring test hits the moment a width changes.
func unwrapFinding(s string) string { return strings.Join(strings.Fields(s), " ") }
