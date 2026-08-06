package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// 🔴 Nothing in this file may reach a real server: every case drives the stubbed
// genSeams submit counter, which is what makes "0 submits" a MEASURED zero.

// --- the prompt defaults to NO ------------------------------------------------

// 🔴 THE default-deny guard for the spend prompt.
//
// The existing tests feed only "y" and "n", and that pair CANNOT see the
// mutation this test exists to kill — rewriting confirmGenerate's
//
//	case "y", "yes": proceed;  default: cancel
//
// as
//
//	case "n", "no": cancel;    default: proceed
//
// keeps both of those cases green while silently turning `[y/N]` into `[Y/n]`.
// The difference is only observable on an answer that is NEITHER accepting NOR
// explicitly refusing: a bare Enter, a typo, a stray "1". Under the mutation
// every one of those SPENDS REAL BUZZ.
//
// "n" and "no" are deliberately absent from the deny table below: they abort
// under the mutation too, so they discriminate nothing.
func TestGenerate_PromptDefaultsToNoAndNeverSubmits(t *testing.T) {
	withStdinTTY(t, true)

	deny := []struct {
		name  string
		stdin string
	}{
		{"bare Enter", "\n"},
		{"empty stdin (EOF)", ""},
		{"whitespace only", "   \n"},
		{"maybe", "maybe\n"},
		{"yy", "yy\n"},
		{"Ye (prefix of yes, not yes)", "Ye\n"},
		{"1", "1\n"},
		{"ok", "ok\n"},
		{"sure", "sure\n"},
		{"yeah", "yeah\n"},
		{"y-with-trailing-junk", "y!\n"},
	}
	for _, tc := range deny {
		t.Run("deny/"+tc.name, func(t *testing.T) {
			var s genSeams
			c, out, errb := genCmd(tc.stdin)
			err := runGenerate(c, s.deps(t), baseOpts())
			if err == nil {
				t.Fatalf("answer %q: want a cancellation, got nil", tc.stdin)
			}
			// 🔴 The load-bearing assertion: no money moved. Under the inverted
			// switch this is 1.
			if s.submitCalls != 0 {
				t.Errorf("🔴 answer %q SPENT BUZZ: submit called %d time(s), want 0", tc.stdin, s.submitCalls)
			}
			// The case must actually have reached the switch. Without this, a
			// refusal from some earlier guard would look like a default-deny.
			if !strings.Contains(errb.String(), "[y/N]") {
				t.Errorf("answer %q: no prompt printed, so this case never reached the switch:\n%s", tc.stdin, errb.String())
			}
			if strings.Contains(out.String(), "wf_123") {
				t.Errorf("answer %q: a workflow id was printed for a cancelled run: %q", tc.stdin, out.String())
			}
		})
	}

	// 🔴 POSITIVE CONTROLS, in the same test. Every zero above is only meaningful
	// because these drive the SAME counter, through the SAME deps constructor, to
	// 1 — a submit seam wired to nothing would report 0 for the whole table.
	for _, tc := range []struct {
		name  string
		stdin string
	}{
		{"y", "y\n"},
		{"yes", "yes\n"},
		{"Y uppercase", "Y\n"},
		{"YES uppercase", "YES\n"},
		{"y with surrounding spaces", "  y  \n"},
	} {
		t.Run("accept/"+tc.name, func(t *testing.T) {
			var s genSeams
			c, out, _ := genCmd(tc.stdin)
			if err := runGenerate(c, s.deps(t), baseOpts()); err != nil {
				t.Fatalf("answer %q: want the generation to proceed, got %v", tc.stdin, err)
			}
			if s.submitCalls != 1 {
				t.Fatalf("POSITIVE CONTROL FAILED: answer %q submitted %d times, want 1 — the zeros in the deny table prove nothing",
					tc.stdin, s.submitCalls)
			}
			if !strings.Contains(out.String(), "wf_123") {
				t.Errorf("answer %q: accepted run printed no workflow id: %q", tc.stdin, out.String())
			}
		})
	}
}

// 🔴 The non-TTY refusal must hold REGARDLESS of what is on stdin.
//
// TestGenerate_NonTTYWithoutYesRefusesAndNeverSubmits pipes EMPTY stdin, so if
// the `!stdinIsTTY()` refusal were removed the run would fall through to the
// prompt, read EOF, and cancel anyway — the submit counter stays at 0 and only
// the message assertions notice. The counter never discriminates, so the guard
// is pinned by its wording rather than by its behaviour.
//
// Piping "y" is the real shape of the hazard: a CI job that answers the prompt
// on stdin instead of passing --yes. With the non-TTY branch gone, that SPENDS
// REAL BUZZ with no --yes anywhere in the command line. The claim is "a terminal
// is attached", not "somebody typed something".
func TestGenerate_NonTTYIgnoresPipedYes(t *testing.T) {
	withStdinTTY(t, false)

	for _, piped := range []string{"y\n", "yes\n", "Y\n", "YES\n"} {
		var s genSeams
		c, _, errb := genCmd(piped)
		err := runGenerate(c, s.deps(t), baseOpts())
		if err == nil {
			t.Errorf("non-TTY with %q piped to stdin: want a refusal, got nil", piped)
		}
		if s.submitCalls != 0 {
			t.Errorf("🔴 non-TTY with %q piped to stdin SPENT BUZZ (%d submit(s)) — piped input is not a confirmation",
				piped, s.submitCalls)
		}
		if strings.Contains(errb.String(), "[y/N]") {
			t.Errorf("non-TTY with %q piped: a prompt was printed to a shell that cannot answer it", piped)
		}
	}

	// POSITIVE CONTROL: the same seam, same non-TTY, with --yes — submits.
	var ok genSeams
	o := baseOpts()
	o.assumeYes = true
	c, _, _ := genCmd("")
	if err := runGenerate(c, ok.deps(t), o); err != nil {
		t.Fatalf("positive control (--yes on a non-TTY): %v", err)
	}
	if ok.submitCalls != 1 {
		t.Fatalf("POSITIVE CONTROL FAILED: submit called %d times, want 1", ok.submitCalls)
	}
}

// --- the cost-vs-balance boundary --------------------------------------------

// 🔴 The `cost > balance` comparison at its BOUNDARY.
//
// Surviving mutation: `>` → `>=`. Every existing case sits far from the edge
// (balance 3 vs cost 12, or balance 1,000,000), so none of them can tell the two
// operators apart. Spending your last Buzz on a job you can exactly afford is a
// legitimate, and not especially rare, thing to do — under `>=` it is refused
// with "exceeds your balance of 12", which is simply false.
//
// The three points are measured together so the claim carries its own scope:
// just below the balance, exactly at it, and just above it.
func TestGenerate_CostEqualToBalanceIsAffordable(t *testing.T) {
	withStdinTTY(t, false)

	const cost = 12 // genSeams' default quote (okQuote(12))

	cases := []struct {
		name        string
		balance     int64
		wantSubmits int
		wantErr     bool
	}{
		// Below the cost: refused. This is the POSITIVE CONTROL for the guard —
		// it proves the balance check is wired up and CAN fire, so the zeros in
		// the "want a refusal" direction are real.
		{"balance one below the cost", cost - 1, 0, true},
		// 🔴 AT the boundary: affordable. This is the case `>=` gets wrong.
		{"balance exactly equal to the cost", cost, 1, false},
		// Above: affordable (guards against `<` style inversions too).
		{"balance one above the cost", cost + 1, 1, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var s genSeams
			bal := tc.balance
			s.balance = func(context.Context) (int64, error) { return bal, nil }
			o := baseOpts()
			o.assumeYes = true
			c, _, _ := genCmd("")
			err := runGenerate(c, s.deps(t), o)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("balance %d vs cost %d: want a refusal, got nil", tc.balance, cost)
				}
				// Pinned by errors.Is, never by message text.
				if !errors.Is(err, ErrInsufficientBuzz) {
					t.Errorf("balance %d: want ErrInsufficientBuzz, got %v", tc.balance, err)
				}
			} else if err != nil {
				t.Fatalf("balance %d vs cost %d: this job IS affordable and must not be refused: %v",
					tc.balance, cost, err)
			}

			if s.submitCalls != tc.wantSubmits {
				t.Errorf("balance %d vs cost %d: submit called %d time(s), want %d",
					tc.balance, cost, s.submitCalls, tc.wantSubmits)
			}
			// The balance seam must have been consulted, or the comparison under
			// test never ran and the whole case is vacuous.
			if s.balanceCalls != 1 {
				t.Errorf("balance seam called %d time(s), want 1 — the comparison under test did not run", s.balanceCalls)
			}
		})
	}
}
