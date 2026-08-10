package genapi

import (
	"encoding/json"
	"strings"
)

// The two transaction types this build knows the DIRECTION of.
//
// 🔴 They are the only two ever OBSERVED on the wire, and the direction is
// corroborated rather than assumed: a failed workflow read back on 2026-08-10
// carried `{"type":"debit","amount":8}` and `{"type":"credit","amount":8}`, and
// the SAME workflow rendered `COST 0` in `civitai workflows list` — i.e. the
// server's own cost for that run equals debit minus credit. Anything else is
// left UNCLASSIFIED on purpose; see Settlement.NetKnown.
const (
	TransactionDebit  = "debit"
	TransactionCredit = "credit"
)

// WorkflowTransaction is one entry of the orchestrator's per-workflow money
// record.
//
// Only the two fields the CLI reports are modelled. The shape is
// orchestrator-owned, so the type string is echoed VERBATIM and never relabelled
// (the same rule Workflow.Transactions' own comment states).
type WorkflowTransaction struct {
	Type   string  `json:"type"`
	Amount float64 `json:"amount"`
}

// TransactionTotal is every entry of one type, summed.
type TransactionTotal struct {
	// Type is the server's own string, untouched.
	Type string
	// Amount is the sum of that type's entries.
	Amount float64
	// Count is how many entries were summed into Amount.
	Count int
}

// Settlement is what the orchestrator recorded against ONE workflow.
//
// 🔴 IT IS A RECORD, NOT A RULE. Every field here is arithmetic over entries the
// server returned for this workflow id. It says nothing about whether a failure
// or a cancel is refunded in general, and nothing about the account-wide Buzz
// ledger, which this CLI still cannot read. That distinction is the whole of
// AGENTS.md item 28: reporting a ledger you were handed is the opposite of
// claiming an unobservable one.
type Settlement struct {
	// Totals is one entry per distinct type, in the order the types were first
	// seen in the payload — the server's order, not one this package invents.
	Totals []TransactionTotal
	// Debits and Credits are the sums of the two classified types.
	Debits  float64
	Credits float64
	// Net is Debits - Credits. 🔴 Read it ONLY when NetKnown is true.
	Net float64
	// NetKnown is false when the payload carried at least one type this build
	// cannot place on either side of the subtraction. It FAILS CLOSED: an
	// unrecognised type makes the net unreportable rather than silently dropping
	// the entry out of the arithmetic, which would print a confident wrong
	// number for a run the user paid for.
	NetKnown bool
	// Unclassified lists those types, verbatim and de-duplicated, so a caller can
	// name what it could not place instead of saying only "unknown".
	Unclassified []string
}

// Settlement decodes the workflow's `transactions` record into a reportable
// summary.
//
// The second return is false — meaning "nothing to report", NOT "nothing
// happened" — when the field is absent, null, not the observed
// `{"list":[…]}` envelope, or carries an empty list. A caller must keep whatever
// it says when the settlement is unobservable; an absent record is not evidence
// that no money moved.
//
// 🔴 ONLY the measured envelope is accepted. `orchestrator.getWorkflow` returned
// `"transactions":{"list":[…]}`; a bare array or some other wrapper is NOT
// speculatively supported, because guessing a shape here would turn a parse this
// package got wrong into a money figure on a user's screen. An unrecognised
// shape reads as unobservable, which is the pre-existing behaviour.
func (w *Workflow) Settlement() (*Settlement, bool) {
	if w == nil {
		return nil, false
	}
	return parseSettlement(w.Transactions)
}

func parseSettlement(raw json.RawMessage) (*Settlement, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var env struct {
		List []WorkflowTransaction `json:"list"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, false
	}
	if len(env.List) == 0 {
		return nil, false
	}

	s := &Settlement{NetKnown: true}
	at := map[string]int{}
	seenUnclassified := map[string]bool{}
	for _, t := range env.List {
		label := strings.TrimSpace(t.Type)
		key := strings.ToLower(label)
		if i, ok := at[key]; ok {
			s.Totals[i].Amount += t.Amount
			s.Totals[i].Count++
		} else {
			at[key] = len(s.Totals)
			s.Totals = append(s.Totals, TransactionTotal{Type: label, Amount: t.Amount, Count: 1})
		}
		switch key {
		case TransactionDebit:
			s.Debits += t.Amount
		case TransactionCredit:
			s.Credits += t.Amount
		default:
			s.NetKnown = false
			if !seenUnclassified[key] {
				seenUnclassified[key] = true
				s.Unclassified = append(s.Unclassified, label)
			}
		}
	}
	s.Net = s.Debits - s.Credits
	return s, true
}
