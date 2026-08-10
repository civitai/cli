package cmd

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/civitai/cli/internal/genapi"
	"github.com/civitai/cli/internal/ui"
)

// workflowSettlementPrintedNote is what a surface says INSTEAD of
// buzzLedgerUnknownNote once it has printed the workflow's own transactions
// (#346).
//
// 🔴 IT IS A POINTER AT FACTS, NOT A RULE. buzzLedgerUnknownNote exists because
// the CLI cannot answer "what became of this charge" and must not pretend to;
// #346 showed that on a workflow the server DOES itemise, printing that sentence
// sent the user to a website for a number that was in the payload the command
// had just parsed. So where the numbers are on screen, this replaces it — and it
// still asserts nothing: it names where the entries are, and the entries are the
// server's own.
//
// It deliberately does NOT re-point at /user/transactions. That pointer answers
// the ACCOUNT-WIDE question, which this block does not touch, and repeating it
// here is exactly the conflation #346 reported.
//
// It is a shared constant for the same reason buzzLedgerUnknownNote is: two
// hand-written variants drift, and the drift is invisible until someone reads
// both screens. Its call sites are an asserted ledger — see
// TestSettlementPrintedNoteLedger.
const workflowSettlementPrintedNote = "the transactions the server recorded for this workflow are printed above"

// reportWorkflowSettlement prints the orchestrator's own transaction record for
// ONE workflow, and reports whether it printed anything.
//
// 🔴 THE RETURN VALUE IS LOAD-BEARING. workflowSettlementPrintedNote says the
// entries are "printed above", so a caller may only use it when this function
// actually printed them. Returning the fact — rather than each caller re-deciding
// whether a settlement exists — is what keeps that sentence true by construction
// instead of by two predicates agreeing.
//
// rows go to `out` (they are DATA, the same class as the status and the output
// list) and the one line explaining the arithmetic goes to `errw`, per this
// repo's stream convention. A caller whose stdout must stay machine-clean passes
// errw for both.
//
// 🔴 It reports, it does not conclude. Every number is a sum of entries the
// server returned for this workflow id; nothing here generalises to what a
// failure or a cancel does, and nothing here is the account-wide Buzz ledger.
func reportWorkflowSettlement(out, errw io.Writer, wf *genapi.Workflow) bool {
	s, ok := wf.Settlement()
	if !ok {
		return false
	}
	entries := 0
	for _, t := range s.Totals {
		entries += t.Count
	}

	fmt.Fprintf(out, "\n%s\n", ui.For(out).Bold(fmt.Sprintf("Buzz transactions for this workflow (%d recorded)", entries)))
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, t := range s.Totals {
		// The type string is server-origin text, like every other field this
		// package prints, so it goes through safeTerm.
		// The third cell is omitted rather than emitted empty: a tab-terminated
		// empty cell pads to the column width, leaving trailing spaces on every
		// row of the commonest rendering.
		if t.Count > 1 {
			fmt.Fprintf(tw, "  %s\t%s\t(%d entries)\n", safeTerm(dashIfEmpty(t.Type)), buzzAmount(t.Amount), t.Count)
			continue
		}
		fmt.Fprintf(tw, "  %s\t%s\n", safeTerm(dashIfEmpty(t.Type)), buzzAmount(t.Amount))
	}
	if s.NetKnown {
		fmt.Fprintf(tw, "  net\t%s\n", buzzAmount(s.Net))
	} else {
		fmt.Fprintf(tw, "  net\t(not computed)\n")
	}
	_ = tw.Flush()

	st := ui.For(errw)
	if s.NetKnown {
		fmt.Fprintln(errw, st.Dim(
			"net is debit minus credit over the entries above, for this workflow only — it is not your account balance, which `civitai buzz` reports."))
	} else {
		fmt.Fprintln(errw, st.Dim(fmt.Sprintf(
			"net is not computed: this build does not know which way %s moves Buzz, so the entries above are listed without a total.",
			safeTerm(quotedList(s.Unclassified)))))
	}
	return true
}

// quotedList renders a type list as `"a", "b"` for the not-computed line.
func quotedList(items []string) string {
	out := ""
	for i, s := range items {
		if i > 0 {
			out += ", "
		}
		out += fmt.Sprintf("%q", s)
	}
	return out
}
