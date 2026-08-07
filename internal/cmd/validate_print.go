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
// It is deliberately whitespace-normalising (wrapRunes splits on
// `strings.Fields`), so a message that acquired a stray newline still prints as
// a clean block rather than breaking the indent.
func printFinding(w io.Writer, msg string) {
	for i, line := range wrapRunes(msg, findingWrapWidth-len(findingIndent)) {
		lead := findingIndent
		if i == 0 {
			lead = findingBullet
		}
		fmt.Fprintf(w, "%s%s\n", lead, line)
	}
}

// unwrapFinding is the inverse, for tests: collapse the printed block back to
// the single logical line the producer emitted. It exists so an assertion about
// a message's CONTENT does not have to know where the layout chose to break —
// the failure mode a substring test hits the moment a width changes.
func unwrapFinding(s string) string { return strings.Join(strings.Fields(s), " ") }
