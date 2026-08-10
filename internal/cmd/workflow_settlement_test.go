package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
)

// wfWithUnclassifiedTransaction carries a type this build cannot place on either
// side of the subtraction, which is the fail-closed branch.
const wfWithUnclassifiedTransaction = `{
  "id":"wf_346c","status":"failed","createdAt":"2026-08-10T09:00:00Z",
  "transactions":{"list":[
    {"type":"debit","amount":8},{"type":"debit","amount":4},{"type":"hold","amount":3}]},
  "steps":[]}`

func settlementOf(t *testing.T, payload string) (stdout, stderr string, printed bool) {
	t.Helper()
	var wf genapi.Workflow
	if err := json.Unmarshal([]byte(payload), &wf); err != nil {
		t.Fatalf("fixture does not decode: %v", err)
	}
	var out, errb bytes.Buffer
	printed = reportWorkflowSettlement(&out, &errb, &wf)
	return out.String(), errb.String(), printed
}

// The block reports the entries and the arithmetic over them, and nothing else.
func TestReportWorkflowSettlement_ReportsTheEntries(t *testing.T) {
	stdout, stderr, printed := settlementOf(t, wfWithTransactions)
	if !printed {
		t.Fatal("the #346 payload must render a settlement")
	}
	for _, want := range []string{"debit", "credit", "net", "8"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the block is missing %q:\n%s", want, stdout)
		}
	}
	// net 0 is the arithmetic `workflows list` renders as COST 0 for the same run.
	if !strings.Contains(normaliseCopy(stdout), "net 0") {
		t.Errorf("net is not reported as 0 for a matched debit/credit pair:\n%s", stdout)
	}
	// 🔴 The block itself must not answer the refund question in either
	// direction. It is a record; the rule stays server-side (AGENTS.md item 28).
	lower := strings.ToLower(stdout + stderr)
	for _, banned := range refundDirectionalClaims {
		if strings.Contains(lower, banned) {
			t.Errorf("the settlement block decides the refund question with %q:\n%s\n%s", banned, stdout, stderr)
		}
	}
	// And it must not re-point at the account-wide history: that is the
	// conflation #346 reported.
	if strings.Contains(stdout+stderr, "/user/transactions") {
		t.Errorf("the per-workflow block points at the account-wide history:\n%s\n%s", stdout, stderr)
	}
	// It must scope itself against the account balance, or "net 0" reads as a
	// balance rather than as this run's arithmetic.
	if !strings.Contains(stderr, "civitai buzz") {
		t.Errorf("nothing distinguishes this workflow's net from the account balance:\n%s", stderr)
	}
}

// FAIL CLOSED: an unclassifiable type withholds the net rather than quietly
// dropping the entry out of the subtraction.
func TestReportWorkflowSettlement_UnclassifiedTypeWithholdsTheNet(t *testing.T) {
	stdout, stderr, printed := settlementOf(t, wfWithUnclassifiedTransaction)
	if !printed {
		t.Fatal("an unclassifiable type must still render the entries")
	}
	if !strings.Contains(stdout, "not computed") {
		t.Errorf("the net was reported despite an unplaceable type:\n%s", stdout)
	}
	// The withheld figure must not appear anywhere: 8+4-0 = 12 would be the
	// number a silent drop produces.
	if strings.Contains(normaliseCopy(stdout), "net 12") {
		t.Errorf("the unclassified entry was dropped out of the arithmetic and a net was printed anyway:\n%s", stdout)
	}
	// The entry is still REPORTED, under the server's own name.
	if !strings.Contains(stdout, "hold") {
		t.Errorf("the unclassified entry was hidden rather than reported:\n%s", stdout)
	}
	if !strings.Contains(stderr, `"hold"`) {
		t.Errorf("the explanation does not name the type it could not place:\n%s", stderr)
	}
	// The multi-entry type reports how many entries it folded together.
	if !strings.Contains(normaliseCopy(stdout), "(2 entries)") {
		t.Errorf("two debits were summed into one row without saying so:\n%s", stdout)
	}
}

// Nothing is printed, and nothing is claimed, when the payload carries no
// record. 🔴 The return value is what every caller's "printed above" pointer
// depends on, so a false positive here would make that sentence a lie.
func TestReportWorkflowSettlement_SilentWhenUnobservable(t *testing.T) {
	stdout, stderr, printed := settlementOf(t, wfWithoutTransactions)
	if printed {
		t.Error("a payload with no transaction record must report that it printed nothing")
	}
	if stdout != "" || stderr != "" {
		t.Errorf("something was printed for a payload with no record:\nstdout: %q\nstderr: %q", stdout, stderr)
	}
}

// A server-origin type string is terminal-escaped like every other field this
// package prints.
func TestReportWorkflowSettlement_SanitisesTheServerType(t *testing.T) {
	stdout, _, printed := settlementOf(t, `{"id":"w","status":"failed","transactions":{"list":[
	  {"type":"deb\u001b[31mit","amount":1}]}}`)
	if !printed {
		t.Fatal("CONTROL failure: the fixture rendered nothing, so the escape assertion is vacuous")
	}
	if strings.Contains(stdout, "\x1b[31m") {
		t.Errorf("a raw control sequence from the server reached the terminal:\n%q", stdout)
	}
}

// GUARD A, for #346's constant — the same shape as
// TestBuzzLedgerNote_AssertsNonObservabilityAndNoDirection.
//
// workflowSettlementPrintedNote is what a user reads INSTEAD of "this CLI cannot
// see your Buzz ledger". The whole basis for dropping that disclaimer is that
// the entries are on screen, so this constant must do two things and no more:
// point at them, and decide nothing.
func TestSettlementPrintedNote_PointsAtTheRecordAndDecidesNothing(t *testing.T) {
	if !strings.Contains(workflowSettlementPrintedNote, "printed above") {
		t.Errorf("the note no longer says the entries are printed above. That clause is the whole licence for omitting "+
			"buzzLedgerUnknownNote — without it the CLI has simply stopped answering.\n  got: %s", workflowSettlementPrintedNote)
	}
	if !strings.Contains(workflowSettlementPrintedNote, "the server recorded") {
		t.Errorf("the note no longer attributes the entries to the SERVER. They are the orchestrator's record, not this "+
			"CLI's conclusion, and the attribution is what keeps it a report.\n  got: %s", workflowSettlementPrintedNote)
	}
	// It is per-workflow, and must not be mistaken for the account ledger.
	if strings.Contains(workflowSettlementPrintedNote, "/user/transactions") {
		t.Errorf("the per-workflow note points at the account-wide history — the conflation #346 reported.\n  got: %s",
			workflowSettlementPrintedNote)
	}
	lower := strings.ToLower(workflowSettlementPrintedNote)
	for _, banned := range refundDirectionalClaims {
		if strings.Contains(lower, banned) {
			t.Errorf("the note decides the refund question with %q. Reporting a record is allowed; stating the rule is "+
				"not (AGENTS.md item 28).\n  got: %s", banned, workflowSettlementPrintedNote)
		}
	}
}

// GUARD A' — the POSITIVE CONTROL. The predicate above must reject sentences
// that assert a direction or that stop pointing at the record, and accept the
// real constant. Without this, a ban list that has stopped matching passes.
//
// 🔴 WHAT THIS CONTROL CANNOT DO, MEASURED WHILE WRITING IT. The first version
// of the table below carried the case *"the transactions the server recorded are
// printed above and nothing is refunded"* — and the predicate ACCEPTED it: both
// required clauses are present, and `refundDirectionalClaims` bans
// `"not refunded"` but not `"nothing is refunded"`. That is not a bug in this
// test, it is item 28's finding reproduced a third time: a sentence that keeps
// every required clause and APPENDS a claim is not detectable from text, and no
// length of phrase list changes that (the item's own "DO NOT REACH FOR A THIRD
// PHRASE LIST"). The offending case was removed rather than the list extended.
//
// The APPEND mutation is caught by TestGoldenSpendCopy instead, which compares
// the whole normalised rendering of every surface this constant lands on. That
// is the guard; this one only proves the enumerated wordings still bite.
func TestSettlementPrintedNote_GuardCanStillFire(t *testing.T) {
	rejects := func(s string) bool {
		if !strings.Contains(s, "printed above") || !strings.Contains(s, "the server recorded") {
			return true
		}
		lower := strings.ToLower(s)
		for _, banned := range refundDirectionalClaims {
			if strings.Contains(lower, banned) {
				return true
			}
		}
		return false
	}
	for _, c := range []struct{ name, text string }{
		{"a direction bolted onto the pointer",
			"the transactions the server recorded for this workflow are printed above, so it was refunded"},
		{"the paraphrase that beat the first guard",
			"your Buzz returns to your balance automatically; confirm with `civitai buzz`"},
		{"a pointer that names no record", "check the numbers above"},
		{"an enumerated wording appended to the real constant",
			workflowSettlementPrintedNote + "; the rest will be refunded"},
	} {
		if !rejects(c.text) {
			t.Errorf("CONTROL failure: the guard accepts %s:\n  %s", c.name, c.text)
		}
	}
	if rejects(workflowSettlementPrintedNote) {
		t.Fatalf("CONTROL failure: the guard rejects the constant it is written to accept:\n  %s", workflowSettlementPrintedNote)
	}
}

// GUARD C — the asserted call-site ledger, the same idiom as
// TestBuzzLedgerNoteIsTheOnlyRefundWording.
//
// It fails in BOTH directions. A file that GAINED a reference is a new surface
// speaking about the fate of a charge; a file that LOST one has started wording
// the money question itself. #346's constant needs this for exactly the reason
// #278's did: the goldens cover the surfaces we know about and are structurally
// blind to a new one.
func TestSettlementPrintedNoteLedger(t *testing.T) {
	want := map[string]int{
		"workflow_settlement.go": 1, // the const declaration
		"generate_output.go":     1, // the settled excluded-outputs note
		"generate.go":            1, // the settled terminal-status error
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot read the package directory: %v", err)
	}
	got := map[string]int{}
	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, rerr := os.ReadFile(filepath.Clean(name))
		if rerr != nil {
			t.Fatalf("CONTROL failure: read %s: %v", name, rerr)
		}
		scanned++
		// Comment lines are excluded for the same reason the #278 ledger excludes
		// them: what is pinned is where the constant is RENDERED.
		n := 0
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			n += strings.Count(line, "workflowSettlementPrintedNote")
		}
		if n > 0 {
			got[name] = n
		}
	}
	if scanned < 10 {
		t.Fatalf("CONTROL failure, not a finding: scanned only %d non-test .go files in internal/cmd", scanned)
	}
	if len(got) == 0 {
		t.Fatalf("CONTROL failure, not a finding: the identifier appears in 0 of %d scanned files — it was renamed or the "+
			"scan is broken. Do not read this as 'no sites'.", scanned)
	}
	if fmt.Sprint(sortedPairs(got)) != fmt.Sprint(sortedPairs(want)) {
		t.Fatalf("the workflowSettlementPrintedNote call-site ledger changed.\n  got:  %v\n  want: %v\n\n"+
			"🔴 A file that GAINED a reference is a new surface that stops disclaiming the ledger — pin its rendering in "+
			"TestGoldenSpendCopy in the same change, or the sentence beside it is unreviewed. A file that LOST one has grown "+
			"its own wording for the money question (AGENTS.md item 28).",
			sortedPairs(got), sortedPairs(want))
	}
}
