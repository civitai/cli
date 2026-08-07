package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/civitai/cli/internal/genapi"
	"github.com/spf13/cobra"
)

// The workflow id must reach the user BEFORE any advisory output.
//
// 🔴 THE PROPERTY, AND WHY IT IS NOT ABOUT SUBSTITUTIONS. By the time the submit
// reply lands the job is CHARGED, and the workflow id is the user's only handle
// on what they paid for. The post-spend substitution report explains a charge
// that has already happened — it is advisory. An advisory step that runs BEFORE
// the handle is emitted can delay it (an enrichment resolving the substituted
// ids to names would inherit getWithRetry's 4 attempts against a 30s-timeout
// client) or, if it panics or blocks, withhold it entirely.
//
// The tests below drive the ordering through the REAL runGenerate rather than
// asserting it about a renderer, because "the handle comes first" is a property
// of the path, not of either printer.

// --- a writer that can stall exactly one line --------------------------------

// gateWriter is an io.Writer that BLOCKS the first write containing trigger
// until release is closed, signalling reached when it does.
//
// 🔴 THIS IS HOW THE ORDERING IS MEASURED WITHOUT A WALL-CLOCK SLEEP. A test
// that slept and then looked would be timing-flaky and would prove nothing about
// ordering; blocking the advisory's own write and then reading what is already
// in the buffer makes "the handle was out first" an observation rather than an
// inference. Nothing here depends on how long anything takes.
type gateWriter struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	trigger string
	reached chan struct{}
	release chan struct{}
	fired   bool
}

func newGateWriter(trigger string) *gateWriter {
	return &gateWriter{
		trigger: trigger,
		reached: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (g *gateWriter) Write(p []byte) (int, error) {
	// fired is only ever touched from the goroutine running the command, which is
	// the only writer; the buffer itself is mutex-guarded because the test
	// goroutine snapshots it while this one is parked.
	if !g.fired && g.trigger != "" && strings.Contains(string(p), g.trigger) {
		g.fired = true
		close(g.reached)
		<-g.release
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.buf.Write(p)
}

// snapshot returns everything written so far.
func (g *gateWriter) snapshot() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.buf.String()
}

// wireIdlePoll gives a --no-wait case a WORKING poll seam it should never use.
//
// 🔴 IT IS THERE SO A CONTROL-FLOW MUTANT FAILS AN ASSERTION RATHER THAN
// PANICKING. With the seam left nil, any mutant that sends a --no-wait run into
// waitAndCollect crashes on a nil poll function — and a panic aborts the whole
// test binary, so the guard that would have named the defect
// (TestGenerate_NoWaitEmitsTheHandleAndNeverPolls) never runs. Measured: three
// such mutants were detected only as a segfault until this was wired.
func wireIdlePoll(s *genSeams) {
	s.poll = newFakeClock().cfg()
	s.getWorkflow = func(ctx context.Context, id string) (*genapi.Workflow, json.RawMessage, error) {
		var wf genapi.Workflow
		_ = json.Unmarshal([]byte(wfJSON(genapi.StatusSucceeded)), &wf)
		return &wf, json.RawMessage(wfJSON(genapi.StatusSucceeded)), nil
	}
}

// subSubmitReply is a submit reply carrying one top-level substitution record.
func subSubmitReply(id string) *genapi.SubmitResult {
	return &genapi.SubmitResult{
		ID:     id,
		Status: "queued",
		ModelSubstitutions: []genapi.ModelSubstitution{
			{Requested: 999999999, Applied: 2436219, Reason: genapi.SubstitutionUnrecognized},
		},
	}
}

// 🔴 THE HEADLINE ORDERING ASSERTION, on the --no-wait path.
//
// The advisory's own first write is stalled; while it is stalled the test reads
// what the user has already been shown. Both ids the user needs to get back to a
// paid-for job — the workflow id and the externalId — must already be there.
func TestGenerate_SubmitHandleReachesTheUserBeforeTheSubstitutionAdvisory(t *testing.T) {
	withStdinTTY(t, false)

	var s genSeams
	s.submitReply = subSubmitReply("wf_handle_1")
	wireIdlePoll(&s)
	o := baseOpts() // --no-wait
	o.assumeYes = true
	o.externalID = "ext-handle-1"

	gate := newGateWriter(substitutionLead(substitutionAfterSubmit))
	c := &cobra.Command{}
	c.SetOut(gate)
	c.SetErr(gate)
	c.SetIn(strings.NewReader(""))
	d := s.deps(t)

	done := make(chan error, 1)
	go func() { done <- runGenerate(c, d, o) }()

	// POSITIVE CONTROL built into the wait: if the advisory is never emitted the
	// command simply finishes, and this test would otherwise hang or (worse)
	// silently assert nothing.
	select {
	case <-gate.reached:
	case err := <-done:
		t.Fatalf("POSITIVE CONTROL FAILED: the post-submit advisory was never written (run returned %v), "+
			"so this test could not observe the ordering it claims to check.\ngot:\n%s", err, gate.snapshot())
	}
	atAdvisory := gate.snapshot()
	close(gate.release)

	if err := <-done; err != nil {
		t.Fatalf("submit must succeed: %v", err)
	}

	if !strings.Contains(atAdvisory, "wf_handle_1") {
		t.Errorf("the WORKFLOW ID must reach the user before the substitution advisory runs — "+
			"the job is already charged and the id is the only handle on it.\nseen before the advisory:\n%s", atAdvisory)
	}
	if !strings.Contains(atAdvisory, "ext-handle-1") {
		t.Errorf("the EXTERNAL ID must reach the user before the substitution advisory runs — "+
			"it is what re-attaches to a submit without paying twice.\nseen before the advisory:\n%s", atAdvisory)
	}

	// ...and the advisory itself must still be reported, with the ids. Ordering
	// is only an improvement if nothing was dropped to get it.
	full := gate.snapshot()
	assertPhase(t, full, substitutionAfterSubmit)
	if !strings.Contains(full, "999999999") || !strings.Contains(full, "2436219") {
		t.Errorf("the substitution must still be reported with BOTH ids after the handle; got:\n%s", full)
	}
}

// CONTRAST CONTROL for the test above: with no substitution the gate's trigger
// is never written, so the run completes without ever parking. This proves the
// gate fires on the advisory specifically rather than on any output at all — if
// it fired on something else, the ordering assertion above would be measuring
// the wrong line.
func TestGenerate_GateOnlyFiresOnTheAdvisory(t *testing.T) {
	withStdinTTY(t, false)

	var s genSeams
	s.submitReply = &genapi.SubmitResult{ID: "wf_handle_2", Status: "queued"} // no substitution
	wireIdlePoll(&s)
	o := baseOpts()
	o.assumeYes = true
	o.externalID = "ext-handle-2"

	gate := newGateWriter(substitutionLead(substitutionAfterSubmit))
	c := &cobra.Command{}
	c.SetOut(gate)
	c.SetErr(gate)
	c.SetIn(strings.NewReader(""))

	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("submit must succeed: %v", err)
	}
	if gate.fired {
		t.Fatal("the gate fired with no substitution in the reply — it is not keyed on the advisory")
	}
	got := gate.snapshot()
	if !strings.Contains(got, "wf_handle_2") {
		t.Fatalf("POSITIVE CONTROL FAILED: the run produced no workflow id at all; got:\n%s", got)
	}
	if strings.Contains(got, substitutionLead(substitutionAfterSubmit)) {
		t.Errorf("no substitution must produce no advisory; got:\n%s", got)
	}
}

// The same ordering on the DEFAULT (waiting) path, which uses a different
// renderer (printSubmitted, not printSubmitResult).
//
// 🔴 A one-path assertion would not have caught the `Charged:` regression this
// repo already wrote generate_charge_seam_test.go about: the claim was asserted
// about the renderer the OTHER branch never reaches. Both branches emit the
// handle through emitSubmitHandle, and both are checked.
func TestGenerate_WaitingPathEmitsTheHandleBeforeTheAdvisory(t *testing.T) {
	withStdinTTY(t, false)

	clock := newFakeClock()
	var s genSeams
	s.poll = clock.cfg()
	s.submitReply = subSubmitReply("wf_handle_3")

	gate := newGateWriter(substitutionLead(substitutionAfterSubmit))
	c := &cobra.Command{}
	c.SetOut(gate)
	c.SetErr(gate)
	c.SetIn(strings.NewReader(""))

	polls := 0
	s.getWorkflow = func(ctx context.Context, id string) (*genapi.Workflow, json.RawMessage, error) {
		polls++
		var wf genapi.Workflow
		_ = json.Unmarshal([]byte(wfJSON(genapi.StatusFailed)), &wf)
		return &wf, json.RawMessage(wfJSON(genapi.StatusFailed)), nil
	}

	o := waitOpts(t.TempDir())
	o.externalID = "ext-handle-3"
	d := s.deps(t)

	done := make(chan error, 1)
	go func() { done <- runGenerate(c, d, o) }()

	select {
	case <-gate.reached:
	case err := <-done:
		t.Fatalf("POSITIVE CONTROL FAILED: the post-submit advisory was never written (run returned %v).\ngot:\n%s",
			err, gate.snapshot())
	}
	atAdvisory := gate.snapshot()
	pollsAtAdvisory := polls
	close(gate.release)
	<-done // a failed terminal status returns an error; the ordering is the claim here

	if !strings.Contains(atAdvisory, "wf_handle_3") {
		t.Errorf("waiting path: the workflow id must precede the advisory.\nseen before the advisory:\n%s", atAdvisory)
	}
	if !strings.Contains(atAdvisory, "ext-handle-3") {
		t.Errorf("waiting path: the external id must precede the advisory.\nseen before the advisory:\n%s", atAdvisory)
	}
	// 🔴 The advisory must also run BEFORE the poll loop, not after it — a wait
	// can last the whole --timeout, and a receipt delivered at the end of it is
	// not a receipt. If polling had already begun, the advisory parked too late.
	if pollsAtAdvisory != 0 {
		t.Errorf("the advisory must be emitted before the wait begins; %d poll(s) had already run", pollsAtAdvisory)
	}
	// POSITIVE CONTROL for that zero: the same counter must be able to move.
	if polls == 0 {
		t.Fatal("POSITIVE CONTROL FAILED: the poll seam was never reached, so the zero above is not evidence of ordering")
	}
	if !strings.Contains(gate.snapshot(), "2436219") {
		t.Errorf("waiting path: the substitution must still be reported; got:\n%s", gate.snapshot())
	}
}

// 🔴 emitSubmitHandle MUST STILL DECIDE WHICH BRANCH THE RUN IS ON.
//
// Extracting the handle emission into one function moved three decisions with it
// — "is this --no-wait", "is there anything left to wait for", and "did the
// handle write fail" — and the ordering assertions above cannot see any of them:
// three separate control-flow mutants (`||`->`&&` on the --no-wait test,
// `return true`->`false` on the terminal branch, `||`->`&&` on the caller's
// early return) each turned a --no-wait run into a POLLING run, and each was
// detected only as a nil-seam panic rather than by any guard. A panic is a
// crash, not an assertion — so this pins the behaviour those mutants break.
//
// Case A asserts a MEASURED zero on the poll seam; case B drives the SAME
// counter, through the same seam constructor, above zero.
func TestGenerate_NoWaitEmitsTheHandleAndNeverPolls(t *testing.T) {
	withStdinTTY(t, false)

	newSeams := func(polls *int) *genSeams {
		s := &genSeams{}
		s.poll = newFakeClock().cfg()
		s.submitReply = subSubmitReply("wf_handle_5")
		s.getWorkflow = func(ctx context.Context, id string) (*genapi.Workflow, json.RawMessage, error) {
			*polls++
			var wf genapi.Workflow
			_ = json.Unmarshal([]byte(wfJSON(genapi.StatusSucceeded)), &wf)
			return &wf, json.RawMessage(wfJSON(genapi.StatusSucceeded)), nil
		}
		return s
	}

	// (A) --no-wait: the handle is the end of the command.
	noWaitPolls := 0
	a := newSeams(&noWaitPolls)
	o := baseOpts() // --no-wait
	o.assumeYes = true
	o.externalID = "ext-handle-5"

	c, out, errb := genCmd("")
	err := runGenerate(c, a.deps(t), o)

	// 🔴 THE POLL-SEAM ASSERTION COMES FIRST, AND IS NOT FATAL-GATED ON err.
	// Every mutant that sends --no-wait into the wait ALSO changes the returned
	// error, so a `t.Fatalf` on err above would kill the test before this line
	// and the failure would be attributed to the wrong guard — a green-for-the-
	// wrong-reason inversion of the same trap.
	if noWaitPolls != 0 {
		t.Errorf("🔴 --no-wait reached the poll seam %d time(s) — the run did not stop at the handle", noWaitPolls)
	}
	if err != nil {
		t.Errorf("--no-wait submit must succeed: %v", err)
	}
	if !strings.Contains(out.String(), "wf_handle_5") || !strings.Contains(out.String(), "ext-handle-5") {
		t.Errorf("--no-wait must print both ids:\nstdout:\n%s", out.String())
	}
	if !strings.Contains(errb.String(), "--no-wait") {
		t.Errorf("--no-wait must say it is not waiting and how to collect:\nstderr:\n%s", errb.String())
	}

	// (B) POSITIVE CONTROL: the waiting path drives the same counter above zero,
	// so the zero above is a measurement rather than an unwired seam.
	waitPolls := 0
	b := newSeams(&waitPolls)
	// The wait's own outcome is irrelevant here (a workflow with no output steps
	// ends in an error); the counter is the claim.
	_ = runGenerate(mustGenCmd(), b.deps(t), waitOpts(t.TempDir()))
	if waitPolls == 0 {
		t.Fatal("POSITIVE CONTROL FAILED: the poll seam is never reached at all, so case A's zero proves nothing")
	}
}

// mustGenCmd is genCmd with the streams discarded — used where only a seam
// counter is being read.
func mustGenCmd() *cobra.Command {
	c, _, _ := genCmd("")
	return c
}

// --json is a raw stdout passthrough, and the reorder must not have changed
// either half of that contract: the record still reaches stderr, and stdout is
// still exactly one parseable document.
func TestGenerate_JSONStillReportsTheSubstitutionAfterTheHandle(t *testing.T) {
	withStdinTTY(t, false)

	var s genSeams
	s.submitReply = subSubmitReply("wf_handle_4")
	wireIdlePoll(&s)
	s.submitRaw = json.RawMessage(`{"id":"wf_handle_4","status":"queued","modelSubstitutions":[{"requested":999999999,"applied":2436219,"reason":"unrecognized"}]}`)
	o := baseOpts()
	o.assumeYes = true
	o.jsonOut = true

	c, out, errb := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("submit must succeed: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("stdout must stay one parseable document: %v\n%s", err, out.String())
	}
	if doc["id"] != "wf_handle_4" {
		t.Errorf("stdout must be the raw submit reply; got %v", doc)
	}
	assertPhase(t, errb.String(), substitutionAfterSubmit)
}

// 🔴 SIGNATURE LEDGER. The ordering above is what makes it SAFE to enrich the
// post-spend report later (resolving the substituted ids to names, say). This
// assignment fails to compile the moment reportModelSubstitutions grows a
// context or a resolver — which is precisely the change that would make it able
// to block — and sends whoever makes it to the ordering comment in runGenerate
// before they wire it up.
var _ func(io.Writer, []genapi.ModelSubstitution, substitutionPhase) = reportModelSubstitutions
