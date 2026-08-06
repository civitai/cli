package cmd

import (
	"strings"
	"testing"
)

// 🔴 cancelWorkflow is a MUTATION and it is IRREVERSIBLE — it destroys a job the
// user has already paid for. Every case in this file drives a stubbed seam and
// counts what reached it; nothing here may touch a real server.
//
// The claim under test is the confirmation gate added for `workflows cancel`,
// mirroring confirmSubmit (app_submit.go) and confirmGenerate (generate.go):
//
//	--yes/-y              → proceed
//	non-TTY without --yes → REFUSE
//	interactive TTY       → prompt, and DEFAULT TO NO
//
// Every "0 cancels" assertion below is paired with a case in the SAME test
// driving the SAME counter to 1. A counter wired to nothing reads zero too.

// TestWorkflowsCancel_NonTTYWithoutYesRefusesAndNeverCancels is the headline
// property, with its positive control.
func TestWorkflowsCancel_NonTTYWithoutYesRefusesAndNeverCancels(t *testing.T) {
	withStdinTTY(t, false)

	// (A) negative: no --yes on a non-TTY.
	refuseCalls := 0
	c, out, errb := genCmd("")
	err := runWorkflowsCancel(c, wfCancelDeps(`{}`, nil, nil, &refuseCalls), workflowsCancelOpts{}, "wf_1")
	if err == nil {
		t.Fatal("non-TTY without --yes: want a refusal, got nil")
	}
	if refuseCalls != 0 {
		t.Errorf("non-TTY without --yes: the cancel seam was reached %d time(s), want 0", refuseCalls)
	}
	// The refusal must be actionable: it names the flag that proceeds and the
	// read-only command that shows what would be thrown away.
	for _, want := range []string{"--yes", "workflows get"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not mention %q: %v", want, err)
		}
	}
	if strings.Contains(errb.String(), "[y/N]") {
		t.Error("a non-TTY must not print a prompt (nobody can answer it)")
	}
	if out.Len() != 0 {
		t.Errorf("a refused cancel printed to stdout: %q", out.String())
	}

	// 🔴 (A2) The case that makes the COUNTER the discriminator.
	//
	// Case (A) pipes empty stdin, so if the non-TTY refusal were removed the run
	// would fall through to the prompt, read EOF and abort anyway — the seam
	// stays at 0 and only the message assertions above notice. That is a guard
	// killed by a DIFFERENT guard's error.
	//
	// A CI script that pipes "y" is the real shape of the hazard: with the
	// non-TTY branch gone, that answer satisfies the prompt and the job is
	// destroyed with no --yes anywhere. A non-TTY must refuse REGARDLESS of what
	// is on stdin, because "a terminal is attached" is the thing being asserted,
	// not "somebody typed something".
	for _, piped := range []string{"y\n", "yes\n", "Y\n"} {
		pipedCalls := 0
		c3, _, errb3 := genCmd(piped)
		err := runWorkflowsCancel(c3, wfCancelDeps(`{}`, nil, nil, &pipedCalls), workflowsCancelOpts{}, "wf_1")
		if err == nil {
			t.Errorf("non-TTY with %q piped to stdin: want a refusal, got nil", piped)
		}
		if pipedCalls != 0 {
			t.Errorf("🔴 non-TTY with %q piped to stdin CANCELLED the workflow (%d seam call(s)) — piped input is not a confirmation",
				piped, pipedCalls)
		}
		if strings.Contains(errb3.String(), "[y/N]") {
			t.Errorf("non-TTY with %q piped: a prompt was printed", piped)
		}
	}

	// (B) POSITIVE CONTROL: identical wiring, --yes set. Without this, the zero
	// in (A) is indistinguishable from a seam that is never called at all.
	proceedCalls := 0
	c2, _, _ := genCmd("")
	if err := runWorkflowsCancel(c2, wfCancelDeps(`{}`, nil, nil, &proceedCalls), workflowsCancelOpts{assumeYes: true}, "wf_1"); err != nil {
		t.Fatalf("positive control (--yes): %v", err)
	}
	if proceedCalls != 1 {
		t.Fatalf("POSITIVE CONTROL FAILED: cancel seam called %d times, want 1 — the 0 in case (A) proves nothing", proceedCalls)
	}
}

// 🔴 THE default-deny guard for the cancel prompt.
//
// The mutation this exists to kill is the switch inversion — rewriting
//
//	case "y", "yes": proceed;  default: abort
//
// as
//
//	case "n", "no": abort;     default: proceed
//
// which silently turns `[y/N]` into `[Y/n]`. It type-checks, it keeps the "n"
// and "y" tests green, and it destroys a paid-for job on a bare Enter or a
// typo. Only inputs that are NEITHER "y"/"yes" NOR "n"/"no" can see it.
func TestWorkflowsCancel_PromptDefaultsToNo(t *testing.T) {
	withStdinTTY(t, true)

	// Every one of these must ABORT. "n"/"no" are deliberately NOT in this list:
	// they abort under the mutation too, so they cannot discriminate.
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
	}
	for _, tc := range deny {
		t.Run("deny/"+tc.name, func(t *testing.T) {
			calls := 0
			c, out, errb := genCmd(tc.stdin)
			err := runWorkflowsCancel(c, wfCancelDeps(`{}`, nil, nil, &calls), workflowsCancelOpts{}, "wf_1")
			if err == nil {
				t.Fatalf("answer %q: want an abort, got nil", tc.stdin)
			}
			// 🔴 The load-bearing assertion: the irreversible seam was NOT reached.
			if calls != 0 {
				t.Errorf("answer %q: the cancel seam was reached %d time(s) — this answer must NOT cancel", tc.stdin, calls)
			}
			if out.Len() != 0 {
				t.Errorf("answer %q: an aborted cancel printed to stdout: %q", tc.stdin, out.String())
			}
			// The prompt must have actually been shown, or the case above proves
			// nothing about the switch (it could have refused earlier).
			if !strings.Contains(errb.String(), "[y/N]") {
				t.Errorf("answer %q: no prompt was printed, so this case never reached the switch:\n%s", tc.stdin, errb.String())
			}
		})
	}

	// POSITIVE CONTROLS in the same test: the accepting answers DO proceed, so
	// the zeros above cannot come from a gate that refuses everything.
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
			calls := 0
			c, _, _ := genCmd(tc.stdin)
			if err := runWorkflowsCancel(c, wfCancelDeps(`{}`, nil, nil, &calls), workflowsCancelOpts{}, "wf_1"); err != nil {
				t.Fatalf("answer %q: want the cancel to proceed, got %v", tc.stdin, err)
			}
			if calls != 1 {
				t.Fatalf("POSITIVE CONTROL FAILED: answer %q reached the cancel seam %d times, want 1", tc.stdin, calls)
			}
		})
	}
}

// An interactive decline is the ordinary case, kept separate so a failure names
// it directly. The prompt goes to STDERR so a --json stdout stays machine-clean.
func TestWorkflowsCancel_TTYDeclineDoesNotCancel(t *testing.T) {
	withStdinTTY(t, true)
	calls := 0
	c, out, errb := genCmd("n\n")
	err := runWorkflowsCancel(c, wfCancelDeps(`{}`, nil, nil, &calls), workflowsCancelOpts{}, "wf_1")
	if err == nil {
		t.Fatal("declined prompt: want an abort, got nil")
	}
	if calls != 0 {
		t.Errorf("declined prompt: the cancel seam was reached %d time(s), want 0", calls)
	}
	if !strings.Contains(errb.String(), "[y/N]") {
		t.Errorf("confirmation prompt missing from stderr: %q", errb.String())
	}
	if strings.Contains(out.String(), "[y/N]") {
		t.Errorf("confirmation leaked to stdout: %q", out.String())
	}
	// The prompt must state what is lost — it is the last point at which the
	// user can find out that cancelling does not get the money back.
	if !strings.Contains(errb.String(), "does NOT refund") {
		t.Errorf("the prompt does not say cancelling is not refunded:\n%s", errb.String())
	}
}

// The gate runs BEFORE the mutation even with --json, so a scripted `--json`
// cancel without --yes cannot destroy a job and then fail to print.
func TestWorkflowsCancel_JSONStillGated(t *testing.T) {
	withStdinTTY(t, false)
	calls := 0
	c, out, _ := genCmd("")
	if err := runWorkflowsCancel(c, wfCancelDeps(`{}`, nil, nil, &calls), workflowsCancelOpts{jsonOut: true}, "wf_1"); err == nil {
		t.Fatal("--json without --yes on a non-TTY: want a refusal, got nil")
	}
	if calls != 0 {
		t.Errorf("--json without --yes: the cancel seam was reached %d time(s), want 0", calls)
	}
	if out.Len() != 0 {
		t.Errorf("a refused --json cancel wrote to stdout: %q", out.String())
	}

	// POSITIVE CONTROL: with --yes the same wiring both cancels and prints JSON.
	okCalls := 0
	c2, out2, _ := genCmd("")
	if err := runWorkflowsCancel(c2, wfCancelDeps(`{}`, nil, nil, &okCalls), workflowsCancelOpts{jsonOut: true, assumeYes: true}, "wf_1"); err != nil {
		t.Fatalf("positive control (--json --yes): %v", err)
	}
	if okCalls != 1 || out2.Len() == 0 {
		t.Fatalf("POSITIVE CONTROL FAILED: calls=%d, stdout=%q", okCalls, out2.String())
	}
}

// The flag must exist with its short form, and it must be a real bool the
// command wires to the gate. Asserted on the flag's VALUE after parsing, not on
// its name: a flag that is registered but never read looks identical to one that
// works if you only check that the name is present.
func TestWorkflowsCancel_YesFlagIsRegisteredAndWired(t *testing.T) {
	cmd := newWorkflowsCancelCmd()
	f := cmd.Flags().Lookup("yes")
	if f == nil {
		t.Fatal("`workflows cancel` has no --yes flag")
	}
	if f.Shorthand != "y" {
		t.Errorf("--yes shorthand = %q, want \"y\" (matches `app submit`)", f.Shorthand)
	}
	if f.Value.Type() != "bool" {
		t.Errorf("--yes type = %q, want bool", f.Value.Type())
	}
	if got := f.Value.String(); got != "false" {
		t.Errorf("--yes default = %q, want false — the gate must be ON by default", got)
	}
	// Parsing -y must actually flip the value the RunE closure reads.
	if err := cmd.Flags().Parse([]string{"-y"}); err != nil {
		t.Fatalf("parse -y: %v", err)
	}
	if got := f.Value.String(); got != "true" {
		t.Fatalf("after -y the flag value is %q, want true", got)
	}
}

// The help text must tell a scripter how to get past the gate, or the refusal is
// a dead end. Normalised so a re-wrap does not break it.
func TestWorkflowsCancel_HelpDocumentsTheConfirmation(t *testing.T) {
	long := strings.ToLower(strings.Join(strings.Fields(newWorkflowsCancelCmd().Long), " "))
	for _, want := range []string{"--yes", "non-interactive", "refuses"} {
		if !strings.Contains(long, want) {
			t.Errorf("`workflows cancel --help` does not document the confirmation (%q missing):\n%s", want, long)
		}
	}
}
