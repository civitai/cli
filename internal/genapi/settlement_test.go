package genapi

import (
	"encoding/json"
	"strings"
	"testing"
)

// wfWith wraps a `transactions` value in a minimal workflow so the tests below
// drive the SAME decode path production does — json.Unmarshal into Workflow,
// then Settlement() — rather than handing parseSettlement a pre-built
// RawMessage. A test that skips the outer decode cannot see a wrong struct tag.
func wfWith(transactions string) *Workflow {
	body := `{"id":"wf_1","status":"failed"`
	if transactions != "" {
		body += `,"transactions":` + transactions
	}
	body += `}`
	var w Workflow
	if err := json.Unmarshal([]byte(body), &w); err != nil {
		panic(err)
	}
	return &w
}

// The #346 payload, verbatim in shape: a debit and a matching credit.
func TestSettlement_DebitAndCredit(t *testing.T) {
	s, ok := wfWith(`{"list":[{"type":"debit","amount":8},{"type":"credit","amount":8}]}`).Settlement()
	if !ok {
		t.Fatal("the observed payload must decode to a settlement")
	}
	if s.Debits != 8 || s.Credits != 8 {
		t.Errorf("debits/credits = %v/%v, want 8/8", s.Debits, s.Credits)
	}
	if !s.NetKnown || s.Net != 0 {
		t.Errorf("net = %v (known %v), want 0/true — this is the run `workflows list` renders as COST 0", s.Net, s.NetKnown)
	}
	// Order is the SERVER's, first-seen: the CLI must not re-sort a record it
	// does not own.
	if len(s.Totals) != 2 || s.Totals[0].Type != "debit" || s.Totals[1].Type != "credit" {
		t.Errorf("totals = %+v, want debit then credit in payload order", s.Totals)
	}
	if s.Totals[0].Count != 1 || s.Totals[1].Count != 1 {
		t.Errorf("entry counts = %d/%d, want 1/1", s.Totals[0].Count, s.Totals[1].Count)
	}
	if len(s.Unclassified) != 0 {
		t.Errorf("unclassified = %v, want none", s.Unclassified)
	}
}

// A charge that stands: one debit, no credit.
func TestSettlement_DebitOnly(t *testing.T) {
	s, ok := wfWith(`{"list":[{"type":"debit","amount":12}]}`).Settlement()
	if !ok {
		t.Fatal("a single debit is still a settlement")
	}
	if !s.NetKnown || s.Net != 12 {
		t.Errorf("net = %v (known %v), want 12/true", s.Net, s.NetKnown)
	}
}

// Repeated entries of one type SUM, and the count survives so a renderer can say
// how many were folded together.
func TestSettlement_RepeatedTypesSum(t *testing.T) {
	s, ok := wfWith(`{"list":[{"type":"debit","amount":8},{"type":"debit","amount":4},{"type":"credit","amount":3}]}`).Settlement()
	if !ok {
		t.Fatal("want a settlement")
	}
	if s.Debits != 12 || s.Credits != 3 || s.Net != 9 {
		t.Errorf("debits/credits/net = %v/%v/%v, want 12/3/9", s.Debits, s.Credits, s.Net)
	}
	if len(s.Totals) != 2 {
		t.Fatalf("totals = %+v, want one row per TYPE", s.Totals)
	}
	if s.Totals[0].Count != 2 || s.Totals[0].Amount != 12 {
		t.Errorf("the debit row = %+v, want amount 12 over 2 entries", s.Totals[0])
	}
}

// 🔴 FAIL CLOSED. A type this build cannot place on either side of the
// subtraction must make the NET unreportable — not silently drop out of it. The
// alternative prints a confident wrong number for a run the user paid for.
func TestSettlement_UnknownTypeMakesTheNetUnknown(t *testing.T) {
	s, ok := wfWith(`{"list":[{"type":"debit","amount":8},{"type":"hold","amount":3}]}`).Settlement()
	if !ok {
		t.Fatal("an unknown type must still report the entries it saw")
	}
	if s.NetKnown {
		t.Errorf("net was reported known despite the unclassified type %v — the arithmetic silently dropped an entry", s.Unclassified)
	}
	if len(s.Unclassified) != 1 || s.Unclassified[0] != "hold" {
		t.Errorf("unclassified = %v, want [hold] verbatim", s.Unclassified)
	}
	// The entry itself is still REPORTED, verbatim and unrelabelled — it is the
	// arithmetic that is withheld, not the fact.
	if len(s.Totals) != 2 || s.Totals[1].Type != "hold" || s.Totals[1].Amount != 3 {
		t.Errorf("totals = %+v, want the unclassified entry reported as the server named it", s.Totals)
	}
	// The classified half is still summed, so a renderer can show it.
	if s.Debits != 8 {
		t.Errorf("debits = %v, want 8", s.Debits)
	}
}

// A blank type is unclassifiable in exactly the same way — it must not fall
// through into either side.
func TestSettlement_BlankTypeIsUnclassified(t *testing.T) {
	s, ok := wfWith(`{"list":[{"type":"","amount":5}]}`).Settlement()
	if !ok {
		t.Fatal("want a settlement")
	}
	if s.NetKnown || s.Debits != 0 || s.Credits != 0 {
		t.Errorf("a blank type was classified: net known %v, debits %v, credits %v", s.NetKnown, s.Debits, s.Credits)
	}
}

// Case and surrounding space do not change what a type IS.
func TestSettlement_TypeMatchIsCaseAndSpaceInsensitive(t *testing.T) {
	s, ok := wfWith(`{"list":[{"type":" Debit ","amount":8},{"type":"CREDIT","amount":2}]}`).Settlement()
	if !ok {
		t.Fatal("want a settlement")
	}
	if !s.NetKnown || s.Net != 6 {
		t.Errorf("net = %v (known %v), want 6/true", s.Net, s.NetKnown)
	}
	// The LABEL keeps the server's own casing — trimmed, never rewritten.
	if s.Totals[0].Type != "Debit" {
		t.Errorf("label = %q, want the server's own %q", s.Totals[0].Type, "Debit")
	}
}

// Every shape that means "nothing to report". 🔴 It means exactly that, and NOT
// "no money moved" — every caller keeps its existing wording in this case.
func TestSettlement_UnobservableShapes(t *testing.T) {
	for _, c := range []struct{ name, body string }{
		{"absent", ""},
		{"null", `null`},
		{"empty object", `{}`},
		{"empty list", `{"list":[]}`},
		{"null list", `{"list":null}`},
		{"a bare array, which was never observed on this route", `[{"type":"debit","amount":8}]`},
		{"a scalar", `7`},
		{"a string", `"none"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, ok := wfWith(c.body).Settlement()
			if ok || s != nil {
				t.Errorf("%s must read as unobservable, got %+v", c.name, s)
			}
		})
	}
}

// A nil workflow is unobservable rather than a panic: this is called from a
// render path that already tolerates a partially-populated read.
func TestSettlement_NilWorkflow(t *testing.T) {
	var w *Workflow
	if s, ok := w.Settlement(); ok || s != nil {
		t.Errorf("a nil workflow must report nothing, got %+v", s)
	}
}

// POSITIVE CONTROL for the whole file. The table above asserts a string of
// zeroes; without a case that MUST decode, a Settlement() wired to
// `return nil, false` would pass every one of them.
func TestSettlement_ControlTheDecoderCanReturnTrue(t *testing.T) {
	if _, ok := wfWith(`{"list":[{"type":"debit","amount":1}]}`).Settlement(); !ok {
		t.Fatal("CONTROL failure: the decoder never returns a settlement, so the unobservable cases above prove nothing")
	}
}

// The raw `transactions` bytes must survive on the struct untouched, because
// `--json` hands the whole payload to a script and this package promises not to
// relabel an orchestrator-owned shape.
func TestSettlement_RawTransactionsAreNotConsumed(t *testing.T) {
	w := wfWith(`{"list":[{"type":"debit","amount":8,"externalTransactionId":"xyz"}]}`)
	if !strings.Contains(string(w.Transactions), "externalTransactionId") {
		t.Errorf("a field the struct does not model was lost from the raw record: %s", w.Transactions)
	}
	if _, ok := w.Settlement(); !ok {
		t.Fatal("an entry carrying unmodelled fields must still decode")
	}
}
