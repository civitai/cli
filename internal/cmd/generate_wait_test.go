package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/civitai/cli/internal/genapi"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// --- clock + sleep seam ------------------------------------------------------

// fakeClock records every sleep instead of performing it, and advances a virtual
// clock by the slept amount. A poll test that really slept would take minutes
// (the floor is 5s by design) and would be timing-flaky besides.
type fakeClock struct {
	now    time.Time
	slept  []time.Duration
	cancel context.CancelFunc
	// cancelAfter cancels the injected context once this many sleeps have
	// happened, which is how the interrupt path is driven deterministically.
	cancelAfter int
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.slept = append(c.slept, d)
	c.now = c.now.Add(d)
	if c.cancelAfter > 0 && len(c.slept) >= c.cancelAfter && c.cancel != nil {
		c.cancel()
		return context.Canceled
	}
	return nil
}

func (c *fakeClock) cfg() pollConfig {
	return pollConfig{now: c.Now, sleep: c.Sleep}
}

// scriptedWorkflows returns a getWorkflowFn that hands back one scripted reply
// per call, repeating the last one, and counts the calls.
func scriptedWorkflows(calls *int, replies ...any) getWorkflowFn {
	return func(ctx context.Context, id string) (*genapi.Workflow, json.RawMessage, error) {
		i := *calls
		*calls++
		if i >= len(replies) {
			i = len(replies) - 1
		}
		switch v := replies[i].(type) {
		case error:
			return nil, nil, v
		case string:
			var wf genapi.Workflow
			if err := json.Unmarshal([]byte(v), &wf); err != nil {
				return nil, nil, err
			}
			return &wf, json.RawMessage(v), nil
		}
		return nil, nil, fmt.Errorf("bad script entry %T", replies[i])
	}
}

func wfJSON(status string) string {
	return `{"id":"wf_1","status":"` + status + `","steps":[]}`
}

// apiErrorWithStatus builds a real genapi.APIError by driving a scripted server,
// so the poll loop's status classification is tested against the SAME error type
// production produces rather than a hand-rolled stand-in.
func apiErrorWithStatus(t *testing.T, status int) error {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":{"json":{"message":"scripted"}}}`))
	}))
	t.Cleanup(srv.Close)
	_, _, err := genapi.New(srv.URL, "tok").GetWorkflow(context.Background(), "wf_1")
	if err == nil {
		t.Fatalf("expected an error for status %d", status)
	}
	if genapi.StatusOf(err) != status {
		t.Fatalf("built error carries status %d, want %d", genapi.StatusOf(err), status)
	}
	return err
}

// --- terminal statuses -------------------------------------------------------

// Every terminal status must STOP the loop, and every non-terminal one must not.
func TestPollWorkflow_AllTerminalStatuses(t *testing.T) {
	for _, st := range []string{
		genapi.StatusSucceeded, genapi.StatusFailed, genapi.StatusExpired, genapi.StatusCanceled,
	} {
		t.Run(st, func(t *testing.T) {
			clock := newFakeClock()
			calls := 0
			wf, _, err := pollWorkflow(context.Background(),
				scriptedWorkflows(&calls, wfJSON(st)), "wf_1", clock.cfg(), &quietPollReporter{w: &bytes.Buffer{}, now: clock.Now, heartbeat: time.Second})
			if err != nil {
				t.Fatalf("%s: %v", st, err)
			}
			if wf.Status != st {
				t.Errorf("status = %q, want %q", wf.Status, st)
			}
			if calls != 1 {
				t.Errorf("%s should terminate on the FIRST poll, got %d calls", st, calls)
			}
			if len(clock.slept) != 0 {
				t.Errorf("%s must not sleep before returning, slept %v", st, clock.slept)
			}
		})
	}
}

// The transition case, and the POSITIVE CONTROL for the "terminated on the first
// poll" assertions above: this one MUST take three polls and two sleeps. Without
// it, a `calls == 1` result is indistinguishable from a loop that never runs.
func TestPollWorkflow_PendingThenProcessingThenSucceeded(t *testing.T) {
	clock := newFakeClock()
	calls := 0
	var errb bytes.Buffer
	rep := &quietPollReporter{w: &errb, now: clock.Now, heartbeat: time.Nanosecond}

	wf, raw, err := pollWorkflow(context.Background(),
		scriptedWorkflows(&calls,
			wfJSON(genapi.StatusScheduled),
			wfJSON(genapi.StatusProcessing),
			wfJSON(genapi.StatusSucceeded)),
		"wf_1", clock.cfg(), rep)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if calls != 3 {
		t.Fatalf("POSITIVE CONTROL FAILED: want 3 polls across the transition, got %d", calls)
	}
	if len(clock.slept) != 2 {
		t.Errorf("want 2 sleeps between 3 polls, got %v", clock.slept)
	}
	if wf.Status != genapi.StatusSucceeded {
		t.Errorf("final status = %q", wf.Status)
	}
	if len(raw) == 0 {
		t.Error("the terminal raw payload must be returned for --json")
	}
	// The non-TTY reporter is the test-drivable path; it must report the
	// intermediate statuses it observed.
	for _, want := range []string{genapi.StatusScheduled, genapi.StatusProcessing} {
		if !strings.Contains(errb.String(), want) {
			t.Errorf("stderr does not mention status %q: %q", want, errb.String())
		}
	}
}

// --- backoff + 429 -----------------------------------------------------------

// Backoff must be exponential, must start no faster than the 5s floor, and must
// be capped. The floor is the point: getWorkflow proxies straight through to the
// orchestrator with no cache and no rate limit, so nothing but this stops the
// CLI being a 429 storm.
func TestPollWorkflow_BackoffIsExponentialAboveTheFloor(t *testing.T) {
	clock := newFakeClock()
	calls := 0
	replies := make([]any, 0, 9)
	for i := 0; i < 8; i++ {
		replies = append(replies, wfJSON(genapi.StatusProcessing))
	}
	replies = append(replies, wfJSON(genapi.StatusSucceeded))

	cfg := clock.cfg()
	cfg.maxInterval = 20 * time.Second
	if _, _, err := pollWorkflow(context.Background(),
		scriptedWorkflows(&calls, replies...), "wf_1", cfg, &quietPollReporter{w: &bytes.Buffer{}, now: clock.Now, heartbeat: time.Hour}); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(clock.slept) != 8 {
		t.Fatalf("want 8 sleeps, got %d (%v)", len(clock.slept), clock.slept)
	}
	if clock.slept[0] != defaultPollInterval {
		t.Errorf("first wait = %s, want the %s floor", clock.slept[0], defaultPollInterval)
	}
	for i := 1; i < len(clock.slept); i++ {
		if clock.slept[i] < clock.slept[i-1] {
			t.Errorf("wait %d (%s) is shorter than wait %d (%s) — backoff must not shrink", i, clock.slept[i], i-1, clock.slept[i-1])
		}
		if clock.slept[i] > cfg.maxInterval {
			t.Errorf("wait %d = %s exceeds the cap %s", i, clock.slept[i], cfg.maxInterval)
		}
	}
	if clock.slept[len(clock.slept)-1] <= clock.slept[0] {
		t.Errorf("backoff never grew: %v", clock.slept)
	}
}

// The floor is not negotiable from the caller side: a config asking for a
// sub-floor interval is clamped UP, not honoured.
func TestPollConfig_ClampsTheIntervalUpToTheFloor(t *testing.T) {
	got := pollConfig{interval: 100 * time.Millisecond}.resolved()
	if got.interval != defaultPollInterval {
		t.Errorf("interval = %s, want it clamped to %s", got.interval, defaultPollInterval)
	}
	if got.maxInterval < got.interval {
		t.Errorf("maxInterval %s < interval %s", got.maxInterval, got.interval)
	}
}

// A 429 must drive a LONGER wait than the normal cadence, and must keep the
// raised floor for subsequent polls rather than decaying straight back.
func TestPollWorkflow_RateLimitDrivesALongerWait(t *testing.T) {
	clock := newFakeClock()
	calls := 0
	var errb bytes.Buffer

	cfg := clock.cfg()
	cfg.rateLimitInterval = 45 * time.Second
	cfg.maxInterval = 5 * time.Minute

	_, _, err := pollWorkflow(context.Background(),
		scriptedWorkflows(&calls,
			apiErrorWithStatus(t, http.StatusTooManyRequests),
			wfJSON(genapi.StatusProcessing),
			wfJSON(genapi.StatusSucceeded)),
		"wf_1", cfg, &quietPollReporter{w: &errb, now: clock.Now, heartbeat: time.Nanosecond})
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(clock.slept) != 2 {
		t.Fatalf("want 2 sleeps, got %v", clock.slept)
	}
	if clock.slept[0] != 45*time.Second {
		t.Errorf("wait after a 429 = %s, want the %s rate-limit backoff", clock.slept[0], cfg.rateLimitInterval)
	}
	// CONTRAST (the positive control for "longer"): a non-429 first poll waits
	// only the floor. Same loop, same config, one different reply.
	clock2 := newFakeClock()
	calls2 := 0
	cfg2 := clock2.cfg()
	cfg2.rateLimitInterval = 45 * time.Second
	if _, _, err := pollWorkflow(context.Background(),
		scriptedWorkflows(&calls2, wfJSON(genapi.StatusProcessing), wfJSON(genapi.StatusSucceeded)),
		"wf_1", cfg2, &quietPollReporter{w: &bytes.Buffer{}, now: clock2.Now, heartbeat: time.Hour}); err != nil {
		t.Fatalf("contrast poll: %v", err)
	}
	if clock2.slept[0] != defaultPollInterval {
		t.Fatalf("CONTRAST FAILED: a non-429 poll waited %s — if this were also 45s the 429 case would prove nothing", clock2.slept[0])
	}
	// And the raised floor must persist.
	if clock.slept[1] < 45*time.Second {
		t.Errorf("the wait after the 429 decayed to %s — the raised floor must persist", clock.slept[1])
	}
	if !strings.Contains(errb.String(), "429") {
		t.Errorf("the 429 must be reported to the user, stderr = %q", errb.String())
	}
}

// A definite 4xx cannot be fixed by polling again — return it at once instead of
// hammering the endpoint until --timeout.
func TestPollWorkflow_NonRetryableStatusReturnsImmediately(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			clock := newFakeClock()
			calls := 0
			_, _, err := pollWorkflow(context.Background(),
				scriptedWorkflows(&calls, apiErrorWithStatus(t, status)), "wf_1", clock.cfg(),
				&quietPollReporter{w: &bytes.Buffer{}, now: clock.Now, heartbeat: time.Hour})
			if err == nil {
				t.Fatalf("status %d: want an error", status)
			}
			if calls != 1 {
				t.Errorf("status %d polled %d times, want 1", status, calls)
			}
			if len(clock.slept) != 0 {
				t.Errorf("status %d slept %v, want none", status, clock.slept)
			}
		})
	}
	// POSITIVE CONTROL: a 5xx from the SAME builder IS retried, so the zero
	// sleeps above are a property of the classification, not of the harness.
	clock := newFakeClock()
	calls := 0
	_, _, err := pollWorkflow(context.Background(),
		scriptedWorkflows(&calls, apiErrorWithStatus(t, http.StatusServiceUnavailable), wfJSON(genapi.StatusSucceeded)),
		"wf_1", clock.cfg(), &quietPollReporter{w: &bytes.Buffer{}, now: clock.Now, heartbeat: time.Hour})
	if err != nil {
		t.Fatalf("503 should be retried: %v", err)
	}
	if calls != 2 || len(clock.slept) != 1 {
		t.Fatalf("POSITIVE CONTROL FAILED: 503 gave %d calls / %v sleeps, want 2 / one sleep", calls, clock.slept)
	}
}

// --timeout stops WAITING and returns errWaitTimeout with the last status seen.
func TestPollWorkflow_TimeoutReturnsTheLastStatus(t *testing.T) {
	clock := newFakeClock()
	calls := 0
	cfg := clock.cfg()
	cfg.timeout = 12 * time.Second // two 5s waits, then over the line

	wf, _, err := pollWorkflow(context.Background(),
		scriptedWorkflows(&calls, wfJSON(genapi.StatusProcessing)), "wf_1", cfg,
		&quietPollReporter{w: &bytes.Buffer{}, now: clock.Now, heartbeat: time.Hour})
	if !errors.Is(err, errWaitTimeout) {
		t.Fatalf("want errWaitTimeout, got %v", err)
	}
	if wf == nil || wf.Status != genapi.StatusProcessing {
		t.Errorf("the last-seen workflow must be returned so the caller can report its status, got %+v", wf)
	}
	var total time.Duration
	for _, d := range clock.slept {
		total += d
	}
	if total > cfg.timeout {
		t.Errorf("slept %s in total, which overshoots the %s timeout", total, cfg.timeout)
	}
}

// Ctrl-C (a cancelled context) unblocks the wait promptly.
func TestPollWorkflow_ContextCancellationStopsTheWait(t *testing.T) {
	clock := newFakeClock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clock.cancel, clock.cancelAfter = cancel, 1

	calls := 0
	_, _, err := pollWorkflow(ctx, scriptedWorkflows(&calls, wfJSON(genapi.StatusProcessing)), "wf_1",
		clock.cfg(), &quietPollReporter{w: &bytes.Buffer{}, now: clock.Now, heartbeat: time.Hour})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

// --- the command-level wait path --------------------------------------------

// waitOpts is a full (non---no-wait) invocation writing into dir.
func waitOpts(dir string) generateOpts {
	o := baseOpts()
	o.noWait = false
	o.assumeYes = true
	o.outDir = dir
	o.timeout = 5 * time.Minute
	return o
}

// A --timeout expiry must NOT report success, and must print the exact re-attach
// command plus both ids — the run was charged and the ids are the only way back
// to what was bought.
func TestGenerate_TimeoutPrintsReattachAndFails(t *testing.T) {
	withStdinTTY(t, false)
	clock := newFakeClock()
	calls := 0

	var s genSeams
	s.getWorkflow = scriptedWorkflows(&calls, wfJSON(genapi.StatusProcessing))
	s.poll = clock.cfg()

	o := waitOpts(t.TempDir())
	o.timeout = 11 * time.Second

	c, out, errb := genCmd("")
	err := runGenerate(c, s.deps(t), o)
	if err == nil {
		t.Fatal("--timeout expiry must return an error, not report success")
	}
	if !errors.Is(err, errWaitTimeout) {
		t.Errorf("want errWaitTimeout, got %v", err)
	}
	// The error must be explicit that the job and the charge both survive.
	for _, want := range []string{"still running", "charged", "not cancelled"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("timeout error does not say %q: %v", want, err)
		}
	}
	stderr := errb.String()
	for _, want := range []string{"wf_123", "civitai workflows get wf_123", "External ID"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, stderr)
		}
	}
	if strings.Contains(out.String(), "Saved") {
		t.Errorf("nothing should have been saved on a timeout: %q", out.String())
	}
}

// --no-wait prints the workflow id and exits 0 without ever polling.
func TestGenerate_NoWaitSkipsThePoll(t *testing.T) {
	withStdinTTY(t, false)
	polled := 0
	var s genSeams
	s.getWorkflow = func(ctx context.Context, id string) (*genapi.Workflow, json.RawMessage, error) {
		polled++
		return nil, nil, errors.New("the poll seam must not be reached with --no-wait")
	}
	o := baseOpts()
	o.assumeYes = true

	c, out, _ := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("--no-wait: %v", err)
	}
	if polled != 0 {
		t.Errorf("--no-wait polled %d times, want 0", polled)
	}
	if !strings.Contains(out.String(), "wf_123") {
		t.Errorf("workflow id missing from stdout: %q", out.String())
	}

	// POSITIVE CONTROL for that zero: the SAME seam, without --no-wait, IS
	// reached. Otherwise "0 polls" could just mean the seam was never wired.
	var s2 genSeams
	reached := 0
	clock := newFakeClock()
	s2.poll = clock.cfg()
	s2.getWorkflow = func(ctx context.Context, id string) (*genapi.Workflow, json.RawMessage, error) {
		reached++
		var wf genapi.Workflow
		_ = json.Unmarshal([]byte(wfJSON(genapi.StatusFailed)), &wf)
		return &wf, json.RawMessage(wfJSON(genapi.StatusFailed)), nil
	}
	c2, _, _ := genCmd("")
	_ = runGenerate(c2, s2.deps(t), waitOpts(t.TempDir()))
	if reached != 1 {
		t.Fatalf("POSITIVE CONTROL FAILED: the poll seam was reached %d times without --no-wait, want 1", reached)
	}
}

// A workflow that ends failed/expired/canceled is an ERROR, and the message must
// say it was charged anyway.
func TestGenerate_NonSucceededTerminalStatusFails(t *testing.T) {
	withStdinTTY(t, false)
	for _, st := range []string{genapi.StatusFailed, genapi.StatusExpired, genapi.StatusCanceled} {
		t.Run(st, func(t *testing.T) {
			clock := newFakeClock()
			calls := 0
			var s genSeams
			s.poll = clock.cfg()
			s.getWorkflow = scriptedWorkflows(&calls, wfJSON(st))
			c, _, _ := genCmd("")
			err := runGenerate(c, s.deps(t), waitOpts(t.TempDir()))
			if err == nil {
				t.Fatalf("%s must not report success", st)
			}
			if !strings.Contains(err.Error(), "charged") {
				t.Errorf("%s error must say it was charged: %v", st, err)
			}
			if !strings.Contains(err.Error(), "civitai workflows get") {
				t.Errorf("%s error must name the re-attach command: %v", st, err)
			}
		})
	}
}

// --- the pre-submit state file ----------------------------------------------

// 🔴 The record must exist BEFORE the POST, not merely afterwards. This asserts
// ORDERING: the observer runs INSIDE the submit seam and reads the file from
// there, so a record written after the call would be missing at that moment.
func TestGenerate_StateFileIsWrittenBeforeTheSubmit(t *testing.T) {
	withStdinTTY(t, false)
	dir := t.TempDir()

	var s genSeams
	s.pendingDirOverride = dir
	var seenAtSubmit []pendingGeneration
	s.submitObserver = func() {
		entries, err := recordsIn(dir)
		if err != nil {
			t.Errorf("reading the pending dir from inside the submit: %v", err)
			return
		}
		seenAtSubmit = entries
	}

	o := baseOpts()
	o.assumeYes = true
	c, _, _ := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(seenAtSubmit) != 1 {
		t.Fatalf("at the moment of the POST there were %d recovery records, want 1 — the record must be written BEFORE the request", len(seenAtSubmit))
	}
	rec := seenAtSubmit[0]
	if rec.ExternalID == "" {
		t.Error("the record must carry the idempotency key")
	}
	if rec.ExternalID != s.lastExternalID {
		t.Errorf("the recorded key %q is not the key the submit was given (%q) — a record of a DIFFERENT key cannot re-attach anything", rec.ExternalID, s.lastExternalID)
	}
	if rec.PayloadHash == "" || strings.HasPrefix(rec.PayloadHash, "unhashable") {
		t.Errorf("payloadHash = %q", rec.PayloadHash)
	}
	if rec.SubmittedAt == "" {
		t.Error("the record must carry submittedAt")
	}
	if rec.WorkflowID != "" {
		t.Errorf("workflowId must be empty before the reply arrives, got %q", rec.WorkflowID)
	}
}

// --external-id overrides the minted key, which is what makes the documented
// re-attach command real rather than aspirational.
func TestGenerate_ExternalIDOverrideReachesTheSubmit(t *testing.T) {
	withStdinTTY(t, false)
	var s genSeams
	o := baseOpts()
	o.assumeYes = true
	o.externalID = "11111111-2222-4333-8444-555555555555"
	c, _, _ := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if s.lastExternalID != o.externalID {
		t.Errorf("submit got externalId %q, want the --external-id override %q", s.lastExternalID, o.externalID)
	}
}

// recordsIn reads every pending-generation record in dir.
func recordsIn(dir string) ([]pendingGeneration, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	out := make([]pendingGeneration, 0, len(matches))
	for _, m := range matches {
		p, err := readPendingGeneration(m)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, nil
}

// A submit that never got an answer is the charged-or-not-charged case: warn
// loudly and hand over the re-attach command instead of inviting a re-run.
func TestGenerate_SubmitWithNoResponseWarnsAboutAPossibleCharge(t *testing.T) {
	withStdinTTY(t, false)
	var s genSeams
	s.submitErr = errors.New("Post \"https://civitai.com/api/trpc/...\": context deadline exceeded")
	o := baseOpts()
	o.assumeYes = true
	c, _, errb := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err == nil {
		t.Fatal("a failed submit must return an error")
	}
	stderr := errb.String()
	if !strings.Contains(stderr, "MAY still have been accepted and charged") {
		t.Errorf("stderr must not let a no-answer submit read as 'nothing happened':\n%s", stderr)
	}
	if !strings.Contains(stderr, "--external-id") {
		t.Errorf("stderr must name the re-attach flag:\n%s", stderr)
	}

	// CONTRAST: a submit that DID get an answer (a classified 400) must not
	// print that warning — otherwise the message is noise on every failure.
	var s2 genSeams
	s2.submitErr = apiErrorWithStatus(t, http.StatusBadRequest)
	c2, _, errb2 := genCmd("")
	if err := runGenerate(c2, s2.deps(t), o); err == nil {
		t.Fatal("a 400 submit must return an error")
	}
	if strings.Contains(errb2.String(), "MAY still have been accepted and charged") {
		t.Errorf("a definite 400 must NOT claim a possible charge:\n%s", errb2.String())
	}
}

// pflag's UnquoteUsage treats the first back-quoted span in a usage string as
// the flag's VALUE NAME. A boolean flag whose usage quoted a command therefore
// rendered as `--no-wait civitai workflows get <id>` in --help — measured on
// this phase's first draft, not theorised. The existing --max-cost usage
// carries the same note; this pins it for every flag on both commands at once.
func TestGenerateFlagUsageHasNoBackquotes(t *testing.T) {
	checked := 0
	for _, cmd := range []*cobra.Command{newGenerateCmd(), newWorkflowsGetCmd()} {
		name := cmd.Name()
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			checked++
			if strings.Contains(f.Usage, "`") {
				t.Errorf("%s --%s: usage contains a back-quote, which pflag turns into the flag's VALUE NAME: %q", name, f.Name, f.Usage)
			}
		})
	}
	// POSITIVE CONTROL (a): a zero here would mean VisitAll walked nothing.
	if checked < 10 {
		t.Fatalf("POSITIVE CONTROL FAILED: only %d flags inspected — the walk found nothing to check", checked)
	}
	// POSITIVE CONTROL (b): prove the predicate CAN fire, so the clean result
	// above is a fact about the flags rather than about a check that never
	// matches anything.
	probe := &cobra.Command{Use: "probe"}
	probe.Flags().Bool("bad", false, "see `civitai thing`")
	hits := 0
	probe.Flags().VisitAll(func(f *pflag.Flag) {
		if strings.Contains(f.Usage, "`") {
			hits++
		}
	})
	if hits != 1 {
		t.Fatalf("POSITIVE CONTROL FAILED: the back-quote predicate matched %d times on a deliberately bad flag, want 1", hits)
	}
}
