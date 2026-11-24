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

// printSubmitAdvisories writes each advisory as one wrapped item, in the
// server's order: the message, its code in brackets, and the docs link on the
// next line when the server sent one. An empty list prints nothing.
func printSubmitAdvisories(w io.Writer, advisories []appapi.SubmitAdvisory) {
	if len(advisories) == 0 {
		return
	}
	fmt.Fprintf(w, submitAdvisoriesHeader+"\n", len(advisories))
	for _, a := range advisories {
		msg := strings.TrimSpace(safeTermSingle(a.Message))
		if code := strings.TrimSpace(safeTermSingle(a.Code)); code != "" {
			msg += " [" + code + "]"
		}
		printFinding(w, msg)
		if docs := strings.TrimSpace(safeTermSingle(a.DocsURL)); docs != "" {
			fmt.Fprintln(w, findingIndent+"See: "+docs)
		}
	}
}
