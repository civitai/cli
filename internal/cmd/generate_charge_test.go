package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
)

// The realized charge must be visible on the DEFAULT (waiting) path, not only on
// --no-wait. The user approves an ESTIMATE and this command's whole spend story
// is that the realized cost can exceed it with no refund, so the number that
// settles what actually happened cannot be reachable only via a flag.
//
// Audit finding 🟡#1 on PR #210: `Charged:` had exactly one call site, inside
// printSubmitResult, which the waiting path never reaches.
func TestPrintSubmitted_ShowsTheRealizedCharge(t *testing.T) {
	var buf bytes.Buffer
	printSubmitted(&buf, "wf_123", "ext-1", "https://civitai.com",
		&genapi.WorkflowCost{Total: 137})

	got := buf.String()
	if !strings.Contains(got, "137") {
		t.Errorf("waiting path must print the realized charge 137; got:\n%s", got)
	}
	if !strings.Contains(got, "Charged") {
		t.Errorf("waiting path must label the charge; got:\n%s", got)
	}
}

// POSITIVE CONTROL for the test above: the same assertion must be able to see a
// DIFFERENT number, so a hardcoded "137" in the renderer could not pass.
func TestPrintSubmitted_ChargeIsTheServersNumber(t *testing.T) {
	var buf bytes.Buffer
	printSubmitted(&buf, "wf_123", "ext-1", "https://civitai.com",
		&genapi.WorkflowCost{Total: 9})
	if !strings.Contains(buf.String(), "9") {
		t.Errorf("charge line must echo the server's number; got:\n%s", buf.String())
	}
	if strings.Contains(buf.String(), "137") {
		t.Errorf("charge line is not reading the cost it was given; got:\n%s", buf.String())
	}
}

// A nil cost means the server reported none. Print NOTHING rather than a
// fabricated "Charged 0 Buzz" — a false zero on a money line is worse than
// silence, and is the same failure `unwrapTRPC`'s null-rejection exists to stop.
func TestPrintSubmitted_NilCostPrintsNoChargeLine(t *testing.T) {
	var buf bytes.Buffer
	printSubmitted(&buf, "wf_123", "ext-1", "https://civitai.com", nil)
	got := buf.String()
	if strings.Contains(got, "Charged") {
		t.Errorf("nil cost must not print a charge line; got:\n%s", got)
	}
	if strings.Contains(got, " 0 Buzz") {
		t.Errorf("nil cost must never render as 0 Buzz; got:\n%s", got)
	}
	// It still has to announce the submission and the re-attach handle.
	if !strings.Contains(got, "wf_123") || !strings.Contains(got, "ext-1") {
		t.Errorf("submission announcement lost; got:\n%s", got)
	}
}

// A CANCELLED context must NOT produce "you MAY have been charged".
//
// Audit finding 🟡#2 on PR #210: StatusOf(err)==0 conflates "the request went out
// and the answer was lost" (money may have moved) with "the request never left
// this process" (it cannot have). Warning on the second sends the user to
// re-attach with an externalId no workflow exists behind — which submits a NEW
// job and charges them for real.
func TestGenerate_CancelledContextDoesNotClaimAPossibleCharge(t *testing.T) {
	withStdinTTY(t, false)
	s := genSeams{submitErr: errors.New("context canceled")}
	o := baseOpts()
	o.assumeYes = true

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled BEFORE the submit — nothing can have reached the server

	c, _, errOut := genCmd("")
	c.SetContext(ctx)
	err := runGenerate(c, s.deps(t), o)
	if err == nil {
		t.Fatal("cancelled context: want an error, got nil")
	}
	if strings.Contains(errOut.String(), "MAY still have been accepted") {
		t.Errorf("cancelled context must NOT claim a possible charge — nothing was sent.\nstderr:\n%s", errOut.String())
	}
	if strings.Contains(errOut.String(), "--external-id") {
		t.Errorf("cancelled context must NOT send the user to re-attach: that externalId has no workflow, so re-attaching submits a NEW job.\nstderr:\n%s", errOut.String())
	}
}

// POSITIVE CONTROL for the test above. Without this, the assertions could pass
// against a build that never emits the warning at all — the warning is REQUIRED
// when the request genuinely went out and the reply was lost.
func TestGenerate_LostReplyStillWarnsAboutAPossibleCharge(t *testing.T) {
	withStdinTTY(t, false)
	s := genSeams{submitErr: errors.New("EOF")} // no HTTP status, live context
	o := baseOpts()
	o.assumeYes = true

	c, _, errOut := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err == nil {
		t.Fatal("lost reply: want an error, got nil")
	}
	if !strings.Contains(errOut.String(), "MAY still have been accepted") {
		t.Fatalf("POSITIVE CONTROL FAILED: a lost reply on a LIVE context must warn about a possible charge.\nstderr:\n%s", errOut.String())
	}
}
