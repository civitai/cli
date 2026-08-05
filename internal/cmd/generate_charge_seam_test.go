package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
)

// 🔴 The DEFAULT (waiting) path must print the realized charge.
//
// Delta-audit finding NEW-2 on PR #210: the renderer tests in
// generate_charge_test.go call printSubmitted DIRECTLY, so a mutant that makes
// the CALL SITE always pass a nil cost survived the entire package — the claim
// "the default path shows the charge" was asserted about the renderer, never
// about the path. This drives runGenerate down the real waiting path.
func TestGenerate_WaitingPathPrintsTheRealizedCharge(t *testing.T) {
	withStdinTTY(t, false)
	clock := newFakeClock()
	var s genSeams
	s.poll = clock.cfg()
	s.submitReply = &genapi.SubmitResult{
		ID: "wf_123", Status: "queued",
		Cost: &genapi.WorkflowCost{Total: 137},
	}
	s.getWorkflow = func(ctx context.Context, id string) (*genapi.Workflow, json.RawMessage, error) {
		var wf genapi.Workflow
		_ = json.Unmarshal([]byte(wfJSON(genapi.StatusFailed)), &wf)
		return &wf, json.RawMessage(wfJSON(genapi.StatusFailed)), nil
	}

	c, _, errOut := genCmd("")
	// A failed terminal status returns an error; the charge notice is emitted
	// before the wait begins, which is exactly what we are pinning.
	_ = runGenerate(c, s.deps(t), waitOpts(t.TempDir()))

	if !strings.Contains(errOut.String(), "137") {
		t.Errorf("waiting path must print the realized charge 137 via runGenerate.\nstderr:\n%s", errOut.String())
	}
	if !strings.Contains(errOut.String(), "Charged") {
		t.Errorf("waiting path must label the charge.\nstderr:\n%s", errOut.String())
	}
}

// CONTRAST CONTROL for the test above: a submit reply with NO cost must print no
// charge line on the same path. Without this, an implementation that hardcoded a
// charge line would pass the test above.
func TestGenerate_WaitingPathNilCostPrintsNoCharge(t *testing.T) {
	withStdinTTY(t, false)
	clock := newFakeClock()
	var s genSeams
	s.poll = clock.cfg()
	s.submitReply = &genapi.SubmitResult{ID: "wf_123", Status: "queued"} // no Cost
	s.getWorkflow = func(ctx context.Context, id string) (*genapi.Workflow, json.RawMessage, error) {
		var wf genapi.Workflow
		_ = json.Unmarshal([]byte(wfJSON(genapi.StatusFailed)), &wf)
		return &wf, json.RawMessage(wfJSON(genapi.StatusFailed)), nil
	}

	c, _, errOut := genCmd("")
	_ = runGenerate(c, s.deps(t), waitOpts(t.TempDir()))

	if strings.Contains(errOut.String(), "Charged") {
		t.Errorf("nil cost must not print a charge line on the waiting path.\nstderr:\n%s", errOut.String())
	}
	if !strings.Contains(errOut.String(), "wf_123") {
		t.Errorf("submission announcement lost.\nstderr:\n%s", errOut.String())
	}
}

// 🔴 Ctrl-C DURING the submit round trip must STILL warn about a possible charge.
//
// Delta-audit finding NEW-1 on PR #210. An earlier revision returned early on
// ctx.Err() != nil, which suppressed this warning for every cancellation — but
// ctx.Err() cannot distinguish "cancelled before the POST" (microseconds, no
// charge possible) from "cancelled mid-flight" (up to the submit timeout, charge
// entirely possible). This is the case that regression made silent, and it is
// the dominant one: a slow orchestrator hop is exactly why a user hits Ctrl-C.
//
// The cancel fires INSIDE the submit seam, i.e. after the request has notionally
// gone out — submitCalls == 1 is the positive control proving that.
func TestGenerate_CancelDuringSubmitStillWarnsAboutAPossibleCharge(t *testing.T) {
	withStdinTTY(t, false)

	ctx, cancel := context.WithCancel(context.Background())
	s := genSeams{
		submitErr:      errors.New("context canceled"),
		submitObserver: cancel, // cancels once the request is already in flight
	}
	o := baseOpts()
	o.assumeYes = true

	c, _, errOut := genCmd("")
	c.SetContext(ctx)
	if err := runGenerate(c, s.deps(t), o); err == nil {
		t.Fatal("cancel during submit: want an error, got nil")
	}
	if s.submitCalls != 1 {
		t.Fatalf("POSITIVE CONTROL FAILED: submit seam reached %d times, want 1 — the cancel must happen AFTER the request went out", s.submitCalls)
	}
	if !strings.Contains(errOut.String(), "MAY still have been accepted") {
		t.Errorf("cancel DURING the submit must still warn about a possible charge — the request went out.\nstderr:\n%s", errOut.String())
	}
	if !strings.Contains(errOut.String(), "--external-id") {
		t.Errorf("cancel DURING the submit must hand back the externalId — this is the only surface that does, and nothing reads the crash-recovery record.\nstderr:\n%s", errOut.String())
	}
	// The wording must acknowledge the interruption rather than claiming the
	// server was silent, since we cannot observe whether the bytes were written.
	if !strings.Contains(errOut.String(), "interrupted") {
		t.Errorf("a cancelled submit should say it was interrupted.\nstderr:\n%s", errOut.String())
	}
}
