package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// assertGenericExitCode is the item-7 pin, and it is the ONE copy of this
// sentinel list.
//
// Everything around it reads message TEXT, which says nothing about what a
// script sees; both #331 and #367 are message-only changes, so the
// classification must not move off the generic exit 1 that both dead-end errors
// publish. It is reached by carrying no classification sentinel at all, which is
// why this asserts an absence per sentinel rather than a code.
//
// 🔴 IT LIVES HERE BECAUSE assertDeadEndCopy HAD ITS OWN COPY. Two enumerations
// of the same published contract drift, and the one that drifts is always the
// one nobody edited — a sentinel added to `pkg/civitai` would have had to be
// remembered in two files. `assertDeadEndCopy` now calls this.
func assertGenericExitCode(t *testing.T, err error) {
	t.Helper()
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

// --- the excluded-output line, and the ELEVEN-COPIES rule ---------------------

// 🔴 THE SERVER RECORDS ITS REASON ON THE STEP, SO EVERY OUTPUT OF THAT STEP
// CARRIES THE IDENTICAL STRING. The first cut of #367 printed it once per
// excluded output on top of the caller's own rendering: at `serverQuantityClamp`
// (10) that is ELEVEN copies of one sentence on one screen, which is the exact
// argument genapi.FailureReasons makes for collapsing byte-identical repeats,
// violated one layer up. This is the end-to-end count.
func TestGenerate_ServerReasonIsPrintedOnceNotOncePerOutput(t *testing.T) {
	var images []string
	for i := 1; i <= 10; i++ {
		images = append(images, fmt.Sprintf(`{"id":"out_%d","type":"image","available":false}`, i))
	}
	payload := `{"id":"wf_123","status":"succeeded","steps":[{"$type":"imageGen","name":"$0","status":"failed",
	  "metadata":{},"output":{"images":[` + strings.Join(images, ",") + `],
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
	err := runGenerate(c, s.deps(t), waitOpts(t.TempDir()))
	if err == nil {
		t.Fatal("CONTROL failure, not a finding: a run with no deliverable outputs reported success")
	}

	// POSITIVE CONTROL: all ten outputs really were listed, so a low count below
	// cannot come from a report that rendered nothing.
	stderr := errb.String()
	for _, id := range []string{"out_1", "out_10"} {
		if !strings.Contains(stderr, id) {
			t.Fatalf("CONTROL failure, not a finding: the excluded-output report did not list %s:\n%s", id, stderr)
		}
	}

	// The whole screen: everything printed, plus the error the user then reads.
	screen := stderr + "\n" + err.Error()
	if got := strings.Count(screen, serverSaid); got != 1 {
		t.Errorf("the server's reason appears %d times on one screen, want exactly 1 — ten of them say nothing "+
			"the first did not, and they bury anything that differs.\n%s", got, screen)
	}
}

// The other half of the rule: where NOTHING else carries the reason, the
// per-output line must still state it. A partial run — some outputs saved,
// others excluded — returns no error, so this list is the only carrier.
func TestReportExcludedOutputs_KeepsTheReasonWhenNothingElseCarriesIt(t *testing.T) {
	var b bytes.Buffer
	reportExcludedOutputs(&b, []genapi.Output{
		{Blob: genapi.Blob{ID: "out_1"}, StepErrors: []string{serverSaid}},
	}, false, nil)
	got := b.String()
	if !strings.Contains(got, serverSaid) {
		t.Errorf("no other surface reported this reason and the list dropped it too — that is a regression, "+
			"not a de-duplication:\n%s", got)
	}
	if strings.Contains(got, "the job finished without producing a usable file") {
		t.Errorf("the generic sentence is printed beside the specific one:\n%s", got)
	}
}

// …and where the caller HAS already rendered it, the list falls back to the
// categorical wording rather than repeating the sentence.
func TestReportExcludedOutputs_SuppressesAReasonTheCallerAlreadyPrinted(t *testing.T) {
	var b bytes.Buffer
	reportExcludedOutputs(&b, []genapi.Output{
		{Blob: genapi.Blob{ID: "out_1"}, StepErrors: []string{serverSaid}},
	}, false, []string{serverSaid})
	got := b.String()
	if strings.Contains(got, serverSaid) {
		t.Errorf("the caller had already printed this reason and the list repeated it:\n%s", got)
	}
	// ANTI-VACUITY: suppression must not delete the LINE, only the duplicated
	// sentence — the output is still excluded and still has to be reported.
	if !strings.Contains(got, "out_1") {
		t.Errorf("suppressing the reason removed the excluded output's line entirely:\n%s", got)
	}
	if !strings.Contains(got, "the job finished without producing a usable file") {
		t.Errorf("the fallback wording is not the shared generic sentence:\n%s", got)
	}
}

// 🔴 SUPPRESSION MUST NOT COST ATTRIBUTION. Two steps that died for DIFFERENT
// reasons are the case where a per-output reason carries information the
// workflow-level list cannot: which paid output died of what. Both survive, each
// against its own output, and each still only once.
func TestReportExcludedOutputs_KeepsDISTINCTReasonsPerOutput(t *testing.T) {
	var b bytes.Buffer
	reportExcludedOutputs(&b, []genapi.Output{
		{Blob: genapi.Blob{ID: "out_1"}, StepErrors: []string{"FIXTURE reason one"}},
		{Blob: genapi.Blob{ID: "out_2"}, StepErrors: []string{"FIXTURE reason one"}},
		{Blob: genapi.Blob{ID: "out_3"}, StepErrors: []string{"FIXTURE reason two"}},
	}, false, nil)
	got := b.String()
	for _, want := range []string{"FIXTURE reason one", "FIXTURE reason two"} {
		if n := strings.Count(got, want); n != 1 {
			t.Errorf("reason %q appears %d times, want exactly 1 — distinct reasons are kept, repeats are not:\n%s", want, n, got)
		}
	}
	// The output that shares the first reason keeps its line and falls back.
	if !strings.Contains(got, "out_2") {
		t.Errorf("the output sharing a reason lost its line:\n%s", got)
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

// 🔴 THREE SITES PRINT THIS TEXT, AND THE FIRST ROUND OF #367 MUTATION-TESTED
// ONE. Dropping safeTerm at the `workflows get` block or at the excluded-output
// line both survived, because the only sanitization assertion drove the
// `generate` error. Every site that prints server free text is covered here, and
// each case is built so the mutant it kills is that site's own.
//
// escapeReason carries a real ESC (0x1b) plus a CSI cursor-up + line-clear —
// the sequence that overwrites the line above, i.e. the CLI's own output.
const escapeReason = "danger\x1b[1A\x1b[2Kforged"

// assertNoEscapeBytes is the paired predicate: the escape is gone AND the words
// arrived, so a green cannot come from the whole string being dropped.
func assertNoEscapeBytes(t *testing.T, surface, got string) {
	t.Helper()
	if strings.ContainsRune(got, 0x1b) {
		t.Errorf("%s: an ESC byte from the server survived into the terminal — safeTerm is not applied here: %q", surface, got)
	}
	for _, want := range []string{"danger", "forged"} {
		if !strings.Contains(got, want) {
			t.Errorf("CONTROL failure, not a finding: %s never rendered the reason at all, so the check above is vacuous: %q", surface, got)
		}
	}
}

// Site 2 of 3: the `civitai workflows get` reason block.
func TestWorkflowsGet_ReasonBlockIsSanitized(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"id": "wf_123", "status": "failed",
		"steps": []any{map[string]any{
			"$type": "imageGen", "name": "$0", "status": "failed", "metadata": map[string]any{},
			"output": map[string]any{"errors": []string{escapeReason}},
		}},
	})
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	c, out, _ := genCmd("")
	if err := runWorkflowsGet(c, wfGetDeps(string(payload), nil), workflowsGetOpts{baseURL: "https://civitai.com"}, "wf_123"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	assertNoEscapeBytes(t, "the `workflows get` reason block", out.String())
}

// Site 3 of 3: the per-excluded-output line. Pre-existing code path, but #367 is
// what starts routing unbounded server free text through it.
func TestReportExcludedOutputs_ReasonIsSanitized(t *testing.T) {
	var b bytes.Buffer
	reportExcludedOutputs(&b, []genapi.Output{
		{Blob: genapi.Blob{ID: "out_1"}, StepErrors: []string{escapeReason}},
	}, false, nil)
	assertNoEscapeBytes(t, "the excluded-output line", b.String())
}

// 🔴 A FORGED HEADER NEEDS NO CONTROL BYTE, SO safeTerm CANNOT SEE IT. safeTerm
// keeps `\n` by design. A reason containing one renders its continuation flush
// left, where it is indistinguishable from a line `civitai workflows get` wrote
// itself — here, its own `Workflow ID:` / `Status:` header two lines above.
// Continuation lines are indented so they cannot occupy column zero.
func TestWorkflowsGet_MultiLineReasonCannotForgeAHeaderLine(t *testing.T) {
	forgery := "Workflow ID:\tspoofed\nStatus:\tsucceeded"
	payload, err := json.Marshal(map[string]any{
		"id": "wf_123", "status": "failed",
		"steps": []any{map[string]any{
			"$type": "imageGen", "name": "$0", "status": "failed", "metadata": map[string]any{},
			"output": map[string]any{"errors": []string{forgery}},
		}},
	})
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	c, out, _ := genCmd("")
	if err := runWorkflowsGet(c, wfGetDeps(string(payload), nil), workflowsGetOpts{baseURL: "https://civitai.com"}, "wf_123"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	stdout := out.String()

	// POSITIVE CONTROL: the forged text did render, so the absence below is
	// about WHERE it sits and not about it being dropped.
	if !strings.Contains(stdout, "spoofed") {
		t.Fatalf("CONTROL failure, not a finding: the reason never rendered:\n%s", stdout)
	}
	// The real header is written by this command and starts at column zero. A
	// server string must never be able to produce a line that does.
	var forged []string
	for _, line := range strings.Split(stdout, "\n") {
		if line == "" || strings.HasPrefix(line, " ") {
			continue
		}
		if strings.HasPrefix(line, "Status:") && strings.Contains(line, "succeeded") {
			forged = append(forged, line)
		}
		if strings.HasPrefix(line, "Workflow ID:") && strings.Contains(line, "spoofed") {
			forged = append(forged, line)
		}
	}
	if len(forged) > 0 {
		t.Errorf("a server-supplied reason produced %d flush-left line(s) impersonating this command's own header — "+
			"safeTerm keeps newlines, so indentation is the guard:\n  %s\nfull output:\n%s",
			len(forged), strings.Join(forged, "\n  "), stdout)
	}
	// And the real Status line must still say what it really is.
	if !strings.Contains(stdout, "Status:") || !strings.Contains(stdout, "failed") {
		t.Errorf("CONTROL failure, not a finding: the genuine header is missing:\n%s", stdout)
	}
}

// #5: `workflows get` renders EVERY reason, not just the first. The
// single-reason fixture elsewhere in this file cannot tell `range reasons` from
// `reasons[0]`.
func TestWorkflowsGet_RendersEveryReasonNotJustTheFirst(t *testing.T) {
	payload := `{"id":"wf_123","status":"failed","createdAt":"2026-08-10T22:37:15Z","steps":[
	  {"$type":"imageGen","name":"$0","status":"failed","metadata":{},
	   "output":{"images":[{"id":"out_1","type":"image","available":false}],"errors":["FIXTURE reason one"]}},
	  {"$type":"imageGen","name":"$1","status":"failed","metadata":{},
	   "output":{"images":[{"id":"out_2","type":"image","available":false}],"errors":["FIXTURE reason two"]}}]}`
	c, out, _ := genCmd("")
	if err := runWorkflowsGet(c, wfGetDeps(payload, nil), workflowsGetOpts{baseURL: "https://civitai.com"}, "wf_123"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	stdout := out.String()
	for _, want := range []string{"FIXTURE reason one", "FIXTURE reason two"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("`workflows get` dropped %q — a second failed step's reason is not optional detail, it is a "+
				"different cause for a different paid output:\n%s", want, stdout)
		}
	}
}

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
