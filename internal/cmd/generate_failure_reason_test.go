package cmd

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
	"github.com/civitai/cli/pkg/civitai"
)

// civitai/cli#367 — WHEN THE SERVER SAYS WHY, THE CLI MUST SAY WHY.
//
// The orchestrator records the reason on the step as `steps[].output.errors`
// (a []string nested inside `output`, a sibling of `images`), and `stepOutput`
// had no field for it, so it was dropped at unmarshal and EVERY failure printed
// the same generic sentence. Measured across 8 real workflows: 5 of 6 `failed`
// ones carried a reason and the CLI discarded all five.
//
// 🔴 THIS RECONCILES WITH #331/#339, IT DOES NOT REVERSE IT. `noFailureReasonNote`
// ("the orchestrator often supplies no failure reason, so it may not say why")
// came from a dogfood run that followed `civitai workflows get` on a failed job
// and saw no reason — an observation made THROUGH the struct that discarded the
// field, so it could not tell "the server sent nothing" from "the CLI threw it
// away". The caveat is therefore now CONDITIONAL, not deleted: the sixth failed
// workflow carried `"errors": []`, which is a measured branch where the sentence
// is still true. Both directions are asserted below, and the empty branch is
// asserted to be byte-for-byte what it was.
//
// 🔴 EVERY ASSERTION PINS THE REASON STRING SPECIFICALLY. "an error was
// returned" passes today and would pass after a broken fix, so it is not used
// anywhere in this file.

// serverSaid is the fixture reason. It is deliberately NOT the measured
// production string: nothing in the CLI may depend on the wording, and a fixture
// that reads like a real message invites a future guard to match on it.
const serverSaid = "the model refused the request for FIXTURE reasons"

// wfFailedJSON builds a terminal-status workflow whose single step carries the
// given `errors` array verbatim (pass "" to omit the key entirely).
func wfFailedJSON(status, errsJSON string) string {
	out := `"images":[{"id":"out_1","type":"image","available":false}]`
	if errsJSON != "" {
		out += `,"errors":` + errsJSON
	}
	return `{"id":"wf_123","status":"` + status + `","steps":[{"$type":"imageGen","name":"$0",
	  "status":"failed","metadata":{},"output":{` + out + `}}]}`
}

// runToTerminal drives a full `civitai generate` against a scripted poll reply
// and returns the error the dead-end path produced.
func runToTerminal(t *testing.T, payload string) error {
	t.Helper()
	withStdinTTY(t, false)
	clock := newFakeClock()
	calls := 0
	var s genSeams
	s.poll = clock.cfg()
	s.getWorkflow = scriptedWorkflows(&calls, payload)
	// Wired to a failing fetcher rather than left nil so a filter regression is a
	// readable failure instead of a panic.
	s.downloadBlob = func(context.Context, string) (*http.Response, error) {
		return nil, errors.New("nothing here is deliverable, so no fetch should happen")
	}
	c, _, _ := genCmd("")
	err := runGenerate(c, s.deps(t), waitOpts(t.TempDir()))
	if err == nil {
		t.Fatal("CONTROL failure, not a finding: a run that produced nothing reported success")
	}
	return err
}

// assertGenericExitCode is the item-7 pin. Everything else in this file reads
// message TEXT, which says nothing about what a script sees; #367 is a
// message-only change, so the classification must not move off the generic
// exit 1 that both dead-end errors publish.
func assertGenericExitCode(t *testing.T, err error) {
	t.Helper()
	for _, k := range []struct {
		sentinel error
		name     string
	}{
		{civitai.ErrBadRequest, "ErrBadRequest"},
		{civitai.ErrUnauthorized, "ErrUnauthorized"},
		{civitai.ErrNotFound, "ErrNotFound"},
		{civitai.ErrRateLimited, "ErrRateLimited"},
		{civitai.ErrNetwork, "ErrNetwork"},
		{ErrUsage, "cmd.ErrUsage"},
	} {
		if errors.Is(err, k.sentinel) {
			t.Errorf("this error is now classified %s, which moves its published exit code. Surfacing the server's "+
				"failure reason is a message change and must not reclassify anything (AGENTS.md item 7).", k.name)
		}
	}
}

// --- the non-succeeded terminal-status error ---------------------------------

func TestGenerate_TerminalFailurePrintsTheServerReason(t *testing.T) {
	for _, st := range []string{genapi.StatusFailed, genapi.StatusExpired, genapi.StatusCanceled} {
		t.Run(st, func(t *testing.T) {
			err := runToTerminal(t, wfFailedJSON(st, `["`+serverSaid+`"]`))
			msg := err.Error()

			if !strings.Contains(msg, serverSaid) {
				t.Errorf("the server recorded why this run failed and the error does not repeat it — that is civitai/cli#367.\n"+
					"  want to contain: %s\n  got: %s", serverSaid, msg)
			}
			// 🔴 THE #339 CAVEAT MUST BE GONE HERE. It says the orchestrator often
			// supplies no reason "so it may not say why"; the reason is in the same
			// sentence, so keeping it would tell the user their handle cannot answer
			// a question it has just answered.
			if strings.Contains(msg, noFailureReasonNote) {
				t.Errorf("the #339 caveat is still printed beside a reason the server DID supply, which is now false:\n  got: %s", msg)
			}
			// The capability half of #331: qualifying the pointer must never remove
			// it, and neither may replacing the qualification.
			for _, want := range []string{"civitai workflows get", "wf_123"} {
				if !strings.Contains(msg, want) {
					t.Errorf("the error lost %q — the re-attach command is the user's only handle on a paid run:\n  got: %s", want, msg)
				}
			}
			// Item 28: the reason explains the OUTCOME and must not become a claim
			// about the charge in either direction.
			lower := strings.ToLower(msg)
			for _, banned := range refundDirectionalClaims {
				if strings.Contains(lower, banned) {
					t.Errorf("this error now decides the refund question with %q:\n  %s", banned, msg)
				}
			}
			if !strings.Contains(msg, buzzLedgerUnknownNote) && !strings.Contains(msg, workflowSettlementPrintedNote) {
				t.Errorf("the fate-of-charge sentence disappeared along with the caveat:\n  got: %s", msg)
			}
			assertGenericExitCode(t, err)
		})
	}
}

// The measured EMPTY branch: one of six failed workflows carried `"errors": []`.
// It must read exactly as it did before #367, caveat included.
func TestGenerate_TerminalFailureWithNoServerReasonKeepsTheCaveat(t *testing.T) {
	for _, c := range []struct{ name, errs string }{
		{"empty_array", `[]`},
		{"key_absent", ``},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := runToTerminal(t, wfFailedJSON(genapi.StatusFailed, c.errs))
			msg := err.Error()
			if !strings.Contains(msg, noFailureReasonNote) {
				t.Errorf("the server supplied no reason and the caveat is gone — #339 was reconciled, not reverted.\n"+
					"  want to contain: %s\n  got: %s", noFailureReasonNote, msg)
			}
			if strings.Contains(msg, "The server reported") {
				t.Errorf("this run has no server reason and the error claims one:\n  got: %s", msg)
			}
			assertGenericExitCode(t, err)
		})
	}
}

// Several failed steps: every reason is carried, not just the first.
func TestGenerate_TerminalFailureCarriesEveryStepsReason(t *testing.T) {
	payload := `{"id":"wf_123","status":"failed","steps":[
	  {"$type":"imageGen","name":"$0","status":"failed","metadata":{},"output":{"errors":["FIXTURE reason one"]}},
	  {"$type":"imageGen","name":"$1","status":"failed","metadata":{},"output":{"errors":["FIXTURE reason two"]}}]}`
	msg := runToTerminal(t, payload).Error()
	for _, want := range []string{"FIXTURE reason one", "FIXTURE reason two"} {
		if !strings.Contains(msg, want) {
			t.Errorf("a step's reason was dropped — printing only the first is silent data loss.\n  want to contain: %s\n  got: %s", want, msg)
		}
	}
}

// --- the succeeded-but-zero-deliverables error -------------------------------

func TestGenerate_SucceededWithNoDeliverablesPrintsTheServerReason(t *testing.T) {
	payload := `{"id":"wf_123","status":"succeeded","steps":[{"$type":"imageGen","name":"$0","status":"failed",
	  "metadata":{},"output":{"images":[{"id":"out_1","type":"image","available":false}],
	  "errors":["` + serverSaid + `"]}}]}`
	err := runToTerminal(t, payload)
	msg := err.Error()
	if !strings.Contains(msg, "succeeded but produced no deliverable outputs") {
		t.Fatalf("CONTROL failure, not a finding: this fixture did not reach the succeeded-but-empty path:\n  %s", msg)
	}
	if !strings.Contains(msg, serverSaid) {
		t.Errorf("the server said why nothing landed and the error does not repeat it.\n  want to contain: %s\n  got: %s", serverSaid, msg)
	}
	if strings.Contains(msg, noFailureReasonNote) {
		t.Errorf("the #339 caveat is printed beside a reason the server DID supply:\n  got: %s", msg)
	}
	assertGenericExitCode(t, err)
}

// --- the excluded-output line -------------------------------------------------

// `reportExcludedOutputs` prints ExclusionReason per output. Before #367 an
// output that never landed always read "not available (the job finished without
// producing a usable file)"; the server's own account now replaces it.
func TestGenerate_ExcludedOutputLineCarriesTheServerReason(t *testing.T) {
	payload := `{"id":"wf_123","status":"succeeded","steps":[{"$type":"imageGen","name":"$0","status":"failed",
	  "metadata":{},"output":{"images":[{"id":"out_1","type":"image","available":false}],
	  "errors":["` + serverSaid + `"]}}]}`
	withStdinTTY(t, false)
	clock := newFakeClock()
	calls := 0
	var s genSeams
	s.poll = clock.cfg()
	s.getWorkflow = scriptedWorkflows(&calls, payload)
	s.downloadBlob = func(context.Context, string) (*http.Response, error) {
		return nil, errors.New("nothing here is deliverable, so no fetch should happen")
	}
	c, _, errb := genCmd("")
	if err := runGenerate(c, s.deps(t), waitOpts(t.TempDir())); err == nil {
		t.Fatal("CONTROL failure, not a finding: a run with no deliverable outputs reported success")
	}
	stderr := errb.String()
	if !strings.Contains(stderr, "out_1") {
		t.Fatalf("CONTROL failure, not a finding: the excluded-output report did not render:\n%s", stderr)
	}
	if !strings.Contains(stderr, serverSaid) {
		t.Errorf("the excluded-output line does not name the reason the server gave:\n%s", stderr)
	}
	if strings.Contains(stderr, "the job finished without producing a usable file") {
		t.Errorf("the generic exclusion sentence is still printed beside the specific one:\n%s", stderr)
	}
}

// --- `civitai workflows get` --------------------------------------------------

// The command `generate` sends the user to after a failure is the one that most
// needs the reason: #331's dogfood run followed exactly this pointer and got a
// status and nothing else.
func TestWorkflowsGet_RendersTheServerFailureReason(t *testing.T) {
	payload := `{"id":"wf_123","status":"failed","createdAt":"2026-08-10T22:37:15Z",
	  "steps":[{"$type":"imageGen","name":"$0","status":"failed","metadata":{},
	  "output":{"images":[{"id":"out_1","type":"image","available":false}],
	  "errors":["` + serverSaid + `"]}}]}`
	c, out, _ := genCmd("")
	if err := runWorkflowsGet(c, wfGetDeps(payload, nil), workflowsGetOpts{baseURL: "https://civitai.com"}, "wf_123"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	stdout := out.String()
	if !strings.Contains(stdout, serverSaid) {
		t.Errorf("`workflows get` on a failed workflow still does not say why — that is civitai/cli#367.\n"+
			"  want to contain: %s\n  got:\n%s", serverSaid, stdout)
	}
}

// The same view must not invent a heading when the array is empty: this is the
// measured branch #339's caveat still describes.
func TestWorkflowsGet_SaysNothingWhenTheServerReportedNothing(t *testing.T) {
	payload := `{"id":"wf_123","status":"failed","createdAt":"2026-08-10T22:37:15Z",
	  "steps":[{"$type":"imageGen","name":"$0","status":"failed","metadata":{},
	  "output":{"images":[{"id":"out_1","type":"image","available":false}],"errors":[]}}]}`
	c, out, _ := genCmd("")
	if err := runWorkflowsGet(c, wfGetDeps(payload, nil), workflowsGetOpts{baseURL: "https://civitai.com"}, "wf_123"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	stdout := out.String()
	// POSITIVE CONTROL: the view rendered, so the absence below is meaningful.
	if !strings.Contains(stdout, "wf_123") {
		t.Fatalf("CONTROL failure, not a finding: the workflow did not render at all:\n%s", stdout)
	}
	if strings.Contains(stdout, "The server reported") {
		t.Errorf("an empty `errors` array produced a reason heading with nothing under it:\n%s", stdout)
	}
}

// --- the untrusted-text seam --------------------------------------------------

// The reason is server-origin text headed for a terminal. It goes through
// safeTerm, which strips C0/C1 controls and DEL — without it a hostile reason
// could clear the line above and overwrite the CLI's own output.
func TestGenerate_ServerReasonIsSanitizedForTheTerminal(t *testing.T) {
	// A real ESC (0x1b, JSON-escaped) plus a CSI sequence inside the reason.
	payload := `{"id":"wf_123","status":"failed","steps":[{"$type":"imageGen","name":"$0","status":"failed",
	  "metadata":{},"output":{"errors":["danger\u001b[1A\u001b[2Kforged"]}}]}`
	msg := runToTerminal(t, payload).Error()
	if strings.ContainsRune(msg, 0x1b) {
		t.Errorf("an ESC byte from the server survived into the terminal error — safeTerm was not applied: %q", msg)
	}
	// POSITIVE CONTROL: the reason really did reach the message, so the absence
	// above is not simply the whole string being dropped.
	if !strings.Contains(msg, "danger") || !strings.Contains(msg, "forged") {
		t.Errorf("CONTROL failure, not a finding: the reason did not reach the message at all: %q", msg)
	}
}
