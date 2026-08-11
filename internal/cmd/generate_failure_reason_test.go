package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// 🔴 THE CALLER'S DECISION, PINNED THROUGH THE CALLER (F4). waitAndCollect
// suppresses the list's copy only when `len(kept) == 0`, i.e. only when the
// error it is about to return will state the reason on the SAME stream. The
// direct-call tests below pass `reportedElsewhere` themselves and so cannot
// observe that choice at all: an "always suppress" mutant there prints the
// reason ZERO times anywhere on a partial run, which is the regression
// generate.go's own comment forbids.
//
// A PARTIAL run: one deliverable output that downloads, one that never landed.
func TestGenerate_PartialRunStillNamesTheReasonSomewhere(t *testing.T) {
	payload := `{"id":"wf_123","status":"succeeded","steps":[{"$type":"imageGen","name":"$0","status":"failed",
	  "metadata":{},"output":{"images":[
	    {"id":"out_ok","type":"image","available":true,"url":"https://blobs.example/a.jpeg"},
	    {"id":"out_gone","type":"image","available":false}],
	  "errors":["` + serverSaid + `"]}}]}`
	withStdinTTY(t, false)
	clock := newFakeClock()
	calls := 0
	var s genSeams
	s.poll = clock.cfg()
	s.getWorkflow = scriptedWorkflows(&calls, payload)
	fetched := 0
	s.downloadBlob = func(context.Context, string) (*http.Response, error) {
		fetched++
		return &http.Response{
			StatusCode:    http.StatusOK,
			Body:          io.NopCloser(strings.NewReader("PNGDATA")),
			ContentLength: 7,
		}, nil
	}
	c, _, errb := genCmd("")
	err := runGenerate(c, s.deps(t), waitOpts(t.TempDir()))
	if err != nil {
		t.Fatalf("a partial run must still succeed for what it delivered: %v", err)
	}
	// POSITIVE CONTROLS: this really is the partial path — something downloaded,
	// and no error was returned, so the error tail cannot be the carrier.
	if fetched != 1 {
		t.Fatalf("CONTROL failure, not a finding: %d blob(s) fetched, want 1 — this is not the partial-run path", fetched)
	}
	stderr := errb.String()
	if !strings.Contains(stderr, "out_gone") {
		t.Fatalf("CONTROL failure, not a finding: the excluded output was not reported:\n%s", stderr)
	}
	if !strings.Contains(stderr, serverSaid) {
		t.Errorf("nothing on this run carries the server's reason: no error is returned and the list suppressed it. "+
			"That is a regression, not a de-duplication.\n%s", stderr)
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

// 🔴 SUPPRESSION MUST NOT COST ATTRIBUTION, AND FOR ONE ROUND IT DID.
// `reportedElsewhere` was fed the whole workflow's reason set, every output's
// set is a subset of that by construction, so EVERY line fell back to the
// generic sentence — on a workflow whose steps died of two different causes.
// This is the shape that can see it: each excluded output must name ITS OWN
// cause, and must not name another output's.
func TestReportExcludedOutputs_EachOutputNamesItsOwnCause(t *testing.T) {
	outs := []genapi.Output{
		{Blob: genapi.Blob{ID: "out_1"}, StepErrors: []string{"FIXTURE reason ALPHA"}},
		{Blob: genapi.Blob{ID: "out_2"}, StepErrors: []string{"FIXTURE reason BETA"}},
	}
	// Passed the whole workflow's reasons, exactly as the regression did.
	for _, reported := range [][]string{nil, {"FIXTURE reason ALPHA", "FIXTURE reason BETA"}} {
		var b bytes.Buffer
		reportExcludedOutputs(&b, outs, false, reported)
		for _, line := range strings.Split(b.String(), "\n") {
			for _, c := range []struct{ id, own, other string }{
				{"out_1", "FIXTURE reason ALPHA", "FIXTURE reason BETA"},
				{"out_2", "FIXTURE reason BETA", "FIXTURE reason ALPHA"},
			} {
				if !strings.Contains(line, "- "+c.id+":") {
					continue
				}
				if !strings.Contains(line, c.own) {
					t.Errorf("reportedElsewhere=%v: %s does not name its own cause %q — a user cannot tell which "+
						"paid output died of what:\n  %s", reported, c.id, c.own, line)
				}
				if strings.Contains(line, c.other) {
					t.Errorf("reportedElsewhere=%v: %s names another output's cause %q:\n  %s", reported, c.id, c.other, line)
				}
			}
		}
	}
}

// 🔴 THE SUBSET SHAPE — WHERE ONE OUTPUT'S SET IS CONTAINED IN ANOTHER'S. It is
// the shape no other test uses, and it is what distinguishes "the gate reads the
// EXCLUDED OUTPUTS" from "the gate reads something else that happens to
// correlate". Two mutants live only here, and being handed the wrong data is
// exactly how the round-2 regression was introduced:
//
//   - keying the gate on `len(reportedElsewhere)` instead of the outputs, which
//     is nil on this call (and on every printWorkflow call), so the gate reads
//     false and out_2 loses its own cause;
//   - keying a reason set on `StepErrors[0]`, which is "A" for both outputs, so
//     they read as one set.
//
// Both leave every other test in this file green.
func TestReportExcludedOutputs_SubsetReasonSetsStillDiscriminate(t *testing.T) {
	var b bytes.Buffer
	reportExcludedOutputs(&b, []genapi.Output{
		{Blob: genapi.Blob{ID: "out_1"}, StepErrors: []string{"FIXTURE A", "FIXTURE B"}},
		{Blob: genapi.Blob{ID: "out_2"}, StepErrors: []string{"FIXTURE A"}},
	}, false, nil)
	for _, line := range strings.Split(b.String(), "\n") {
		switch {
		case strings.Contains(line, "- out_1:"):
			for _, want := range []string{"FIXTURE A", "FIXTURE B"} {
				if !strings.Contains(line, want) {
					t.Errorf("out_1 lost %q from its own set:\n  %s", want, line)
				}
			}
		case strings.Contains(line, "- out_2:"):
			if !strings.Contains(line, "FIXTURE A") {
				t.Errorf("out_2 does not name its own cause — a set that is a SUBSET of another output's is still "+
					"a different cause, and this output's line is the only place it is attributable:\n  %s", line)
			}
			if strings.Contains(line, "FIXTURE B") {
				t.Errorf("out_2 names a cause that belongs only to out_1:\n  %s", line)
			}
		}
	}
}

// 🔴 THE GATE'S KEY MUST BE INJECTIVE — AGAINST EVERY SEPARATOR, NOT ONE.
// A key built by joining elements on a delimiter collides whenever that
// delimiter can occur inside an element, and the elements here are server free
// text. Two separators are exercised, and the SPACE is the sharper of the two:
//
//   - a SPACE needs no claim about the server at all. Orchestrator prose
//     contains spaces; `%v`, `fmt.Sprint` and `strings.Join(…, " ")` are all
//     plausible tidying edits of this line, and every one of them collides
//     ["A B"] with ["A","B"].
//   - a NUL is the case that motivated the change: JSON permits a \\u0000
//     escape, encoding/json decodes it to a real NUL, and safeTerm strips it
//     only at RENDER. That proves the FORMAT permits it — it is NOT an
//     observation that the orchestrator emits one, and nothing here should be
//     read as one.
//
// The first version of this test pinned only the NUL, and `%q` → `%v` survived
// it. A test that rejects one spelling of a hazard has not rejected the hazard.
//
// 🔴 `reportedElsewhere` IS SEEDED, AND WITHOUT THAT THIS TEST IS VACUOUS. An
// earlier version passed nil and the NUL-join mutant SURVIVED: with an empty
// seed nothing is suppressed whatever the gate decides, because the intra-call
// `seen` map keys on INDIVIDUAL reasons and still tells the two sets apart. The
// collision only has consequences once a caller has reported the union — which
// is exactly what waitAndCollect passes, and what is seeded here.
func TestReportExcludedOutputs_ReasonKeyIsInjectiveAgainstASeparatorInTheText(t *testing.T) {
	for _, c := range []struct{ name, sep string }{
		{"space", " "},
		{"nul", "\x00"},
	} {
		t.Run(c.name, func(t *testing.T) {
			joined := "FIXTURE A" + c.sep + "FIXTURE B"
			outs := []genapi.Output{
				{Blob: genapi.Blob{ID: "out_1"}, StepErrors: []string{joined}},
				{Blob: genapi.Blob{ID: "out_2"}, StepErrors: []string{"FIXTURE A", "FIXTURE B"}},
			}
			var b bytes.Buffer
			reportExcludedOutputs(&b, outs, false, []string{joined, "FIXTURE A", "FIXTURE B"})
			got := b.String()

			// POSITIVE CONTROL: a reason really did render. Asserting only that
			// the generic sentence is ABSENT is a reassuring zero — it cannot be
			// told apart from "no reason rendered at all", which is how a
			// stubbed-out ExclusionReason passed the first version of this test.
			if !strings.Contains(got, "the server reported") {
				t.Fatalf("CONTROL failure, not a finding: no reason rendered at all, so the absence "+
					"asserted below proves nothing:\n%s", got)
			}
			for _, id := range []string{"out_1", "out_2"} {
				if !strings.Contains(got, id) {
					t.Fatalf("CONTROL failure, not a finding: %s did not render:\n%s", id, got)
				}
			}
			// Two DISTINCT sets, so the gate must discriminate and neither
			// output may fall back to the categorical sentence.
			if n := strings.Count(got, "the job finished without producing a usable file"); n != 0 {
				t.Errorf("%d output(s) fell back to the generic sentence. The two sets are different causes "+
					"that render differently, so they collided on the gate's key — joining on %q is not "+
					"injective when %q can occur in the text:\n%s", n, c.sep, c.sep, got)
			}
		})
	}
}

// 🔴 INJECTIVITY AS A PROPERTY OF THE KEY, NOT AS A LIST OF SPELLINGS. The
// render-level test above pins two separators, and pinning separators one at a
// time is the losing shape this repo has already documented twice (AGENTS.md
// item 28's two dead phrase lists): a join on "\x00\x00" survived a fixture that
// only used "\x00". So this asserts the property directly on the key function —
// N pairwise-distinct reason sets must produce N distinct keys.
//
// The table is built so that EVERY fixed-separator join collides on at least one
// pair: for each candidate separator, ["A"+sep+"B"] and ["A","B"] are different
// sets that any join on that separator maps together. Extending the separator
// list is cheap; a key that is not a join (%q, or any escaping encoder) passes
// all of them at once, which is the point.
func TestDistinctReasonSets_KeyIsInjective(t *testing.T) {
	sets := [][]string{
		{"A", "B"},
		{"A"},
		{"B"},
		{"A", ""},
	}
	// One "looks like the joined form" element per plausible separator.
	for _, sep := range []string{
		" ", "  ", ",", ", ", ";", "; ", "|", "\t", "\n", "\x00", "\x00\x00", "\x1f", "-", "_", "",
	} {
		sets = append(sets, []string{"A" + sep + "B"})
	}

	// The table must be pairwise distinct AS SLICES, or the expectation below is
	// wrong rather than the code. This is the control on the fixture itself.
	for i := range sets {
		for j := i + 1; j < len(sets); j++ {
			if fmt.Sprintf("%q", sets[i]) == fmt.Sprintf("%q", sets[j]) {
				t.Fatalf("CONTROL failure, not a finding: table entries %d and %d are the same value %q — "+
					"the expected count below would be wrong", i, j, sets[i])
			}
		}
	}

	var outs []genapi.Output
	for i, s := range sets {
		outs = append(outs, genapi.Output{
			Blob: genapi.Blob{ID: fmt.Sprintf("out_%d", i)}, StepErrors: s,
		})
	}
	if got := distinctReasonSets(outs); got != len(sets) {
		t.Errorf("distinctReasonSets = %d over %d pairwise-distinct sets — the key maps two different causes to "+
			"one, so an output would be told another output's story or lose its own. A key built by joining "+
			"elements on ANY fixed separator collides as soon as that separator appears in an element, and "+
			"these are server free text.", got, len(sets))
	}
}

// Order is part of the rendering, so it is part of the identity of a reason set:
// joinReasons emits the slice in order. Unasserted, a "tidying" sort would
// quietly merge two outputs that said different things.
//
// (Repetition is significant to the key too, but is currently UNREACHABLE —
// Step.failureReasons dedupes and is the only producer of StepErrors — so it is
// not asserted here. See distinctReasonSets' comment.)
func TestReportExcludedOutputs_ReasonSetIdentityIsOrderSensitive(t *testing.T) {
	var b bytes.Buffer
	reportExcludedOutputs(&b, []genapi.Output{
		{Blob: genapi.Blob{ID: "out_1"}, StepErrors: []string{"FIXTURE A", "FIXTURE B"}},
		{Blob: genapi.Blob{ID: "out_2"}, StepErrors: []string{"FIXTURE B", "FIXTURE A"}},
	}, false, nil)
	got := b.String()
	// POSITIVE CONTROL, same reason: the absence below must not be satisfiable
	// by nothing having rendered.
	if !strings.Contains(got, "the server reported") {
		t.Fatalf("CONTROL failure, not a finding: no reason rendered at all:\n%s", got)
	}
	if n := strings.Count(got, "the job finished without producing a usable file"); n != 0 {
		t.Errorf("a reversed set was treated as the same cause, but joinReasons renders the two in different "+
			"orders — they are different sentences:\n%s", got)
	}
}

// Where the outputs AGREE, attribution is vacuous and the sentence collapses to
// one copy — the eleven-copies rule. The line for every other output survives.
func TestReportExcludedOutputs_CollapsesOneSharedReason(t *testing.T) {
	var outs []genapi.Output
	for i := 1; i <= 10; i++ {
		outs = append(outs, genapi.Output{
			Blob: genapi.Blob{ID: fmt.Sprintf("out_%d", i)}, StepErrors: []string{serverSaid},
		})
	}
	var b bytes.Buffer
	reportExcludedOutputs(&b, outs, false, nil)
	got := b.String()
	if n := strings.Count(got, serverSaid); n != 1 {
		t.Errorf("one shared reason across ten outputs appears %d times, want 1:\n%s", n, got)
	}
	for _, id := range []string{"out_1", "out_10"} {
		if !strings.Contains(got, id) {
			t.Errorf("collapsing the reason removed %s's line entirely:\n%s", id, got)
		}
	}
}

// 🔴 A REASONLESS OUTPUT IS NOT A SECOND OPINION. A moderation-blocked output
// beside ones that never landed carries no server account at all, so it must not
// count as a differing cause — counting it flips `attributionDiscriminates` and
// restores the eleven copies for every mixed list, which is the commonest shape
// of all. The collapse fixture above is all-reasoned and cannot see this.
func TestReportExcludedOutputs_AReasonlessOutputDoesNotDefeatTheCollapse(t *testing.T) {
	blocked := "minor"
	outs := []genapi.Output{
		{Blob: genapi.Blob{ID: "out_blocked", Available: true, BlockedReason: &blocked}},
	}
	for i := 1; i <= 5; i++ {
		outs = append(outs, genapi.Output{
			Blob: genapi.Blob{ID: fmt.Sprintf("out_%d", i)}, StepErrors: []string{serverSaid},
		})
	}
	var b bytes.Buffer
	reportExcludedOutputs(&b, outs, false, nil)
	got := b.String()
	if n := strings.Count(got, serverSaid); n != 1 {
		t.Errorf("one shared reason appears %d times, want 1 — the reasonless output must not count as a "+
			"differing cause:\n%s", n, got)
	}
	// POSITIVE CONTROLS: both kinds really are in the list.
	if !strings.Contains(got, "blocked by moderation: minor") {
		t.Errorf("CONTROL failure, not a finding: the blocked output lost its own reason:\n%s", got)
	}
	if !strings.Contains(got, "out_5") {
		t.Errorf("CONTROL failure, not a finding: the last reasoned output lost its line:\n%s", got)
	}
}

// Overlapping-but-different sets: [R1,R2] and [R2,R3] disagree, so both are
// stated in full and R2 is printed twice. That is the documented consequence of
// allSeen being ALL rather than ANY — pinned in both directions so the choice
// cannot be flipped silently.
func TestReportExcludedOutputs_OverlappingReasonSets(t *testing.T) {
	var b bytes.Buffer
	reportExcludedOutputs(&b, []genapi.Output{
		{Blob: genapi.Blob{ID: "out_1"}, StepErrors: []string{"FIXTURE R1", "FIXTURE R2"}},
		{Blob: genapi.Blob{ID: "out_2"}, StepErrors: []string{"FIXTURE R2", "FIXTURE R3"}},
	}, false, nil)
	got := b.String()
	if n := strings.Count(got, "FIXTURE R2"); n != 2 {
		t.Errorf("the shared entry appears %d times, want 2 — differing sets are each stated in full, because "+
			"there is no rendering for 'the part you have not read yet':\n%s", n, got)
	}
	for _, want := range []string{"FIXTURE R1", "FIXTURE R3"} {
		if !strings.Contains(got, want) {
			t.Errorf("the distinguishing reason %q was dropped:\n%s", want, got)
		}
	}
}

// 🔴 END-TO-END, THROUGH THE COMMAND, ON THE STREAM A USER READS. The unit tests
// above call the presenter directly and so cannot see what printWorkflow decides
// to pass it — which is precisely where the regression lived.
func TestWorkflowsGet_ExcludedOutputsStillNameTheirOwnCause(t *testing.T) {
	payload := `{"id":"wf_123","status":"failed","createdAt":"2026-08-10T22:37:15Z","steps":[
	  {"$type":"imageGen","name":"$0","status":"failed","metadata":{},
	   "output":{"images":[{"id":"out_1","type":"image","available":false}],"errors":["FIXTURE reason ALPHA"]}},
	  {"$type":"imageGen","name":"$1","status":"failed","metadata":{},
	   "output":{"images":[{"id":"out_2","type":"image","available":false}],"errors":["FIXTURE reason BETA"]}}]}`
	c, out, errb := genCmd("")
	if err := runWorkflowsGet(c, wfGetDeps(payload, nil), workflowsGetOpts{baseURL: "https://civitai.com"}, "wf_123"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	stderr := errb.String()
	// POSITIVE CONTROL: the excluded list rendered on stderr at all.
	if !strings.Contains(stderr, "out_1") || !strings.Contains(stderr, "out_2") {
		t.Fatalf("CONTROL failure, not a finding: the excluded-output list did not render:\n%s", stderr)
	}
	for _, want := range []string{"FIXTURE reason ALPHA", "FIXTURE reason BETA"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr does not carry %q. The block naming it goes to STDOUT, so `civitai workflows get X >file` "+
				"leaves the terminal with nothing but the generic sentence:\n%s", want, stderr)
		}
	}
	// And stdout still carries the block, so this is not a case of the reason
	// having simply moved streams.
	if !strings.Contains(out.String(), "FIXTURE reason ALPHA") {
		t.Errorf("the stdout reason block lost its content:\n%s", out.String())
	}
}

// 🔴 THE CROSS-STREAM CASE, WHICH IS WHERE THE REGRESSION ACTUALLY SURVIVES A
// DIFFERING-CAUSES TEST. When every excluded output died of the SAME cause,
// attribution is vacuous and the list is allowed to collapse the sentence — so
// the differing-causes test above passes even if printWorkflow hands over the
// whole workflow's reason set. What it must NOT do is collapse it against a
// block printed on a DIFFERENT STREAM: stdout and stderr are independently
// redirectable, and `civitai workflows get X >file` would then leave the
// terminal holding nothing but "the job finished without producing a usable
// file".
func TestWorkflowsGet_StderrCarriesTheReasonEvenWhenStdoutAlreadyDid(t *testing.T) {
	payload := `{"id":"wf_123","status":"failed","createdAt":"2026-08-10T22:37:15Z","steps":[
	  {"$type":"imageGen","name":"$0","status":"failed","metadata":{},
	   "output":{"images":[
	     {"id":"out_1","type":"image","available":false},
	     {"id":"out_2","type":"image","available":false}],
	    "errors":["` + serverSaid + `"]}}]}`
	c, out, errb := genCmd("")
	if err := runWorkflowsGet(c, wfGetDeps(payload, nil), workflowsGetOpts{baseURL: "https://civitai.com"}, "wf_123"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	// POSITIVE CONTROL: stdout really did print the block, so this is the
	// "already reported" situation and not a workflow with no reason at all.
	if !strings.Contains(out.String(), serverSaid) {
		t.Fatalf("CONTROL failure, not a finding: the stdout block did not render:\n%s", out.String())
	}
	stderr := errb.String()
	if !strings.Contains(stderr, "out_1") {
		t.Fatalf("CONTROL failure, not a finding: the excluded list did not render on stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, serverSaid) {
		t.Errorf("stderr suppressed the reason against a block printed on STDOUT. Redirect stdout and the terminal "+
			"is left with only the categorical sentence:\n%s", stderr)
	}
	// …and it is still collapsed WITHIN stderr: two outputs, one cause, one copy.
	if n := strings.Count(stderr, serverSaid); n != 1 {
		t.Errorf("the shared reason appears %d times on stderr, want 1:\n%s", n, stderr)
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
//
// The fixture is THREE lines, not two: a guard measured at one continuation
// point is a guard that a `break`-after-the-first-line implementation passes,
// and that mutant survived a 2-line fixture.
func TestWorkflowsGet_MultiLineReasonCannotForgeAHeaderLine(t *testing.T) {
	forgery := "Workflow ID:\tspoofed\nStatus:\tsucceeded\nCompleted:\tnever"
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
		if strings.HasPrefix(line, "Completed:") && strings.Contains(line, "never") {
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

// 🔴 THE OTHER TWO SITES THAT PRINT THIS TEXT, EACH OF WHICH SHIPPED WITHOUT
// THE INDENT GUARD OR WITHOUT A TEST FOR IT. `firstColumnLines` is the shared
// predicate: no line a SERVER string produced may begin at column zero, because
// that is where this CLI writes its own banners, headers and money lines.
//
// Golden pinning does not close this — the goldens' fixture reasons are
// single-line, so a runtime injection happens below the golden's resolution.
func firstColumnLines(s string, needles ...string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line == "" || strings.HasPrefix(line, " ") {
			continue
		}
		for _, n := range needles {
			if strings.HasPrefix(line, n) {
				out = append(out, line)
				break
			}
		}
	}
	return out
}

// The `generate` dead-end error: it had safeTerm and NO indentContinuation at
// all, so a multi-line reason rendered flush left one line under `Error: `,
// forging this CLI's own success banner and a money line.
func TestGenerate_MultiLineReasonCannotForgeTheSuccessBanner(t *testing.T) {
	forgery := "real failure\n✓ Generation submitted — workflow wf_999\nCharged 0 Buzz"
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
	msg := runToTerminal(t, string(payload)).Error()
	if !strings.Contains(msg, "real failure") {
		t.Fatalf("CONTROL failure, not a finding: the reason never reached the error:\n%s", msg)
	}
	if forged := firstColumnLines(msg, "✓", "Charged"); len(forged) > 0 {
		t.Errorf("a server reason produced %d flush-left line(s) forging this CLI's own output:\n  %s\nfull error:\n%s",
			len(forged), strings.Join(forged, "\n  "), msg)
	}
}

// The excluded-output line: it HAD indentContinuation, and removing it left the
// whole package green.
func TestReportExcludedOutputs_MultiLineReasonCannotForgeARefundClaim(t *testing.T) {
	forgery := "real failure\nNothing is refunded.\nCharged 0 Buzz"
	var b bytes.Buffer
	reportExcludedOutputs(&b, []genapi.Output{
		{Blob: genapi.Blob{ID: "out_1"}, StepErrors: []string{forgery}},
	}, false, nil)
	got := b.String()
	if !strings.Contains(got, "real failure") {
		t.Fatalf("CONTROL failure, not a finding: the reason never rendered:\n%s", got)
	}
	if forged := firstColumnLines(got, "Nothing is refunded", "Charged"); len(forged) > 0 {
		t.Errorf("a server reason produced %d flush-left line(s) beside the charge copy — a refund claim the CLI "+
			"never wrote and no golden can see:\n  %s\nfull output:\n%s", len(forged), strings.Join(forged, "\n  "), got)
	}
}

// F10: the output ID is server-origin too, and it is on the same line as the
// reason. The claim that every site printing server text is covered here has to
// include it.
func TestReportExcludedOutputs_OutputIDIsSanitized(t *testing.T) {
	var b bytes.Buffer
	reportExcludedOutputs(&b, []genapi.Output{
		{Blob: genapi.Blob{ID: "out\x1b[2K_1"}},
	}, false, nil)
	got := b.String()
	if strings.ContainsRune(got, 0x1b) {
		t.Errorf("an ESC byte in the SERVER-supplied output id survived into the terminal: %q", got)
	}
	if !strings.Contains(got, "out") || !strings.Contains(got, "_1") {
		t.Errorf("CONTROL failure, not a finding: the id did not render at all: %q", got)
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
