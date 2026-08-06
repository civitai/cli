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

// A cancelled context still warns — it just says "interrupted".
//
// 🔴 This test previously asserted the OPPOSITE (that a cancelled context must
// stay silent), and that revision was a money-safety regression: ctx.Err()
// cannot tell "cancelled before the POST" from "cancelled mid-flight", so
// suppressing on cancellation went silent in the window where the charge is
// real. See TestGenerate_CancelDuringSubmitStillWarnsAboutAPossibleCharge.
//
// The uncertainty belongs in the MESSAGE, not in the control flow: we cannot
// observe whether the request bytes were written, so we always warn, and the
// wording acknowledges the interruption instead of claiming the server was
// silent.
func TestGenerate_CancelledContextWarnsAsInterrupted(t *testing.T) {
	withStdinTTY(t, false)
	s := genSeams{submitErr: errors.New("context canceled")}
	o := baseOpts()
	o.assumeYes = true

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before the submit

	c, _, errOut := genCmd("")
	c.SetContext(ctx)
	err := runGenerate(c, s.deps(t), o)
	if err == nil {
		t.Fatal("cancelled context: want an error, got nil")
	}
	if !strings.Contains(errOut.String(), "MAY still have been accepted") {
		t.Errorf("a cancelled submit must still warn about a possible charge — silence strands the user.\nstderr:\n%s", errOut.String())
	}
	if !strings.Contains(errOut.String(), "interrupted") {
		t.Errorf("a cancelled submit should be worded as interrupted, not as server silence.\nstderr:\n%s", errOut.String())
	}
	if !strings.Contains(errOut.String(), "--external-id") {
		t.Errorf("the externalId must be handed back — this is the only surface that does.\nstderr:\n%s", errOut.String())
	}
}

// CONTRAST: a lost reply on a LIVE context uses the "no answer" wording, not the
// interrupted wording. Pins that the two cases are distinguishable in the text.
func TestGenerate_LostReplyOnLiveContextSaysNoAnswer(t *testing.T) {
	withStdinTTY(t, false)
	s := genSeams{submitErr: errors.New("EOF")}
	o := baseOpts()
	o.assumeYes = true

	c, _, errOut := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err == nil {
		t.Fatal("lost reply: want an error, got nil")
	}
	if !strings.Contains(errOut.String(), "got no answer from the server") {
		t.Errorf("a live-context lost reply should say the server gave no answer.\nstderr:\n%s", errOut.String())
	}
	if strings.Contains(errOut.String(), "interrupted") {
		t.Errorf("a live-context lost reply must not claim interruption.\nstderr:\n%s", errOut.String())
	}
}

// A lost reply on a LIVE context must hand back the externalId VALUE.
//
// This was previously labelled a "positive control" for a negative test that no
// longer exists (the one asserting cancellation stays silent, deleted because it
// pinned a regression). It earns its place on its own: it is the structural
// externalId assertion for the non-cancelled branch, complementing the
// wording contrast in TestGenerate_LostReplyOnLiveContextSaysNoAnswer.
func TestGenerate_LostReplyHandsBackTheExternalID(t *testing.T) {
	withStdinTTY(t, false)
	s := genSeams{submitErr: errors.New("EOF")} // no HTTP status, live context
	o := baseOpts()
	o.assumeYes = true

	c, _, errOut := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err == nil {
		t.Fatal("lost reply: want an error, got nil")
	}
	if !strings.Contains(errOut.String(), "MAY still have been accepted") {
		t.Fatalf("a lost reply on a LIVE context must warn about a possible charge.\nstderr:\n%s", errOut.String())
	}
	// Structural, not spelled — see the note in generate_charge_seam_test.go.
	if s.lastExternalID == "" {
		t.Fatal("POSITIVE CONTROL FAILED: no externalId was minted, so the assertion below is vacuous")
	}
	if !strings.Contains(errOut.String(), s.lastExternalID) {
		t.Errorf("the externalId VALUE %q must appear in the warning.\nstderr:\n%s", s.lastExternalID, errOut.String())
	}
}
