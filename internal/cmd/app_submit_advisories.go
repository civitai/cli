package cmd

// app_submit_advisories.go prints the optional, NON-BLOCKING advisories an
// accepted submit may return (`advisories: [{code, message, docsUrl?}]`) — for
// example a recommendation to give the app its own look instead of Civitai's
// stock one.
//
// 🔴 ADVISORY, NEVER A GATE. They are printed after the success line, change no
// exit code and ask nothing. A server that sends none, or sends a shape this
// client does not understand, prints nothing here (appapi.SubmitAdvisories
// decodes tolerantly), so a submit reads exactly as it did before the field
// existed.
//
// `app submit` has no `--json` or quiet mode today, so there is no structured
// output to carry them in; if one is added, include Advisories in it rather than
// printing this prose.
//
// Every string here is SERVER text, so each goes through safeTermSingle before
// it reaches the terminal.

import (
	"fmt"
	"io"
	"strings"

	"github.com/civitai/cli/internal/appapi"
)

// submitAdvisoriesHeader opens an accepted submit's advisories; %d is their
// count.
const submitAdvisoriesHeader = "%d suggestion(s) from the server (advisory — they do not block this submit or its review):"

// Bounds on what one submit's advisories may put on the terminal. The server
// is trusted, but its text is still unbounded input to a fixed-format block.
const (
	maxSubmitAdvisories     = 5
	maxAdvisoryMessageRunes = 500
	maxAdvisoryDocsURLRunes = 300
)

// printSubmitAdvisories writes each advisory as one wrapped item, in the
// server's order: the message, its code in brackets, and the docs link on the
// next line when the server sent one.
//
// Each field is sanitised FIRST and judged after: an entry whose message is
// empty once sanitised (a zero-width space, a lone ESC — the decoder only drops
// messages blank BEFORE sanitising) is skipped, and the header counts only the
// entries kept. None kept prints nothing. At most maxSubmitAdvisories print,
// followed by one "(+N more)" line; messages and docs links are cut to their
// rune budgets with a trailing ellipsis.
func printSubmitAdvisories(w io.Writer, advisories []appapi.SubmitAdvisory) {
	type line struct{ msg, docs string }
	var kept []line
	for _, a := range advisories {
		msg := strings.TrimSpace(safeTermSingle(a.Message))
		if msg == "" {
			continue
		}
		msg = truncateRunes(msg, maxAdvisoryMessageRunes)
		if code := strings.TrimSpace(safeTermSingle(a.Code)); code != "" {
			msg += " [" + code + "]"
		}
		docs := truncateRunes(strings.TrimSpace(safeTermSingle(a.DocsURL)), maxAdvisoryDocsURLRunes)
		kept = append(kept, line{msg, docs})
	}
	if len(kept) == 0 {
		return
	}
	fmt.Fprintf(w, submitAdvisoriesHeader+"\n", len(kept))
	for i, l := range kept {
		if i == maxSubmitAdvisories {
			fmt.Fprintf(w, "%s(+%d more)\n", findingIndent, len(kept)-maxSubmitAdvisories)
			break
		}
		printFinding(w, l.msg)
		if l.docs != "" {
			fmt.Fprintln(w, findingIndent+"See: "+l.docs)
		}
	}
}

// truncateRunes cuts s to n runes, appending serverTextEllipsis when it cut.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + serverTextEllipsis
}
