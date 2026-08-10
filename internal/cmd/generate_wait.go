package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/civitai/cli/internal/genapi"
	"github.com/civitai/cli/internal/ui"
)

// getWorkflowFn is the read-back seam. It matches genapi.Client.GetWorkflow.
type getWorkflowFn func(ctx context.Context, workflowID string) (*genapi.Workflow, json.RawMessage, error)

// errWaitTimeout is returned by pollWorkflow when --timeout elapsed before the
// workflow reached a terminal status.
//
// 🔴 It means the CLI STOPPED WAITING. It does NOT mean the job stopped: the
// generation keeps running server-side and finishes and bills exactly as if the
// wait had continued, and this path does not cancel anything. Every message
// built from this must say so. (What a deliberate `workflows cancel` then does
// to the charge is a different question, traced in runWorkflowsCancel — #307.)
var errWaitTimeout = errors.New("timed out waiting for the generation to finish")

// Poll cadence defaults.
//
// 🔴 The 5s floor is a deliberate lower bound, not a tuning knob. An earlier
// design revision argued for 3s on the strength of a "3s server-side feed
// cache" — that cache (QUERIED_WORKFLOWS_CACHE_TTL) belongs to
// `queryGeneratedImages`, a different route. `orchestrator.getWorkflow` proxies
// STRAIGHT THROUGH to the orchestrator with no cache, no cursor and no delta,
// and no tRPC-level rate limit is attached to any orchestrator procedure. So
// there is nothing between this loop and the orchestrator except this loop's own
// restraint: the CLI is the only thing that stops the CLI from being a 429
// storm. Back off, and back off HARDER when the server actually says 429.
const (
	defaultPollInterval     = 5 * time.Second
	defaultMaxPollInterval  = 60 * time.Second
	defaultPollFactor       = 1.5
	defaultRateLimitBackoff = 30 * time.Second
	defaultPollHeartbeat    = 15 * time.Second
	// defaultWaitTimeout bounds the default wait. It is a WAITING bound: an
	// unattended run must not block forever, and the re-attach path
	// (`civitai workflows get <id>`) makes giving up recoverable.
	//
	// 🔴 IT MUST OUTLAST THE QUEUE, NOT JUST THE EXECUTION — and it did not.
	// This was 10m, and the blind dogfood run of 2026-08-07 measured a healthy
	// job sitting in `scheduled` for 11m41s BEFORE execution began (created
	// 17:17:25.589Z, started 17:29:06.848Z, completed 17:31:11.339Z — 13m46s
	// end-to-end). So the default gave up 2m5s before the job even started,
	// and per this command's own help — "--timeout STOPS WAITING. IT DOES NOT
	// STOP PAYING" — the charge stood. The default put a user in exactly the
	// state the help exists to warn about, on nothing more unusual than a busy
	// queue. 30m is ~2.2x the measured p100. See civitai/cli#326.
	//
	// 🔴 REJECTED: excluding time spent in `scheduled` from the budget (the
	// dogfood report's other suggestion). The budget exists to stop an
	// unattended run blocking forever, and a job that never leaves the queue is
	// precisely the case that would then never time out — it removes the bound
	// in the one situation that motivated changing it. Raise the number; keep
	// the bound a bound. Both halves are pinned by
	// TestPollWorkflow_DefaultTimeoutOutlastsTheMeasuredQueueLatency and
	// TestPollWorkflow_DefaultTimeoutIsStillABound.
	defaultWaitTimeout = 30 * time.Minute
)

// pollConfig is the poll loop's cadence, with the clock and the sleep injected
// so a test can drive backoff and 429 handling without really sleeping (the
// precedent is app_dev_tunnel.go's interval seams).
type pollConfig struct {
	interval          time.Duration
	maxInterval       time.Duration
	factor            float64
	rateLimitInterval time.Duration
	heartbeat         time.Duration
	// timeout bounds the WAIT (not the job, and not the charge). 0 waits
	// indefinitely.
	timeout time.Duration

	// sleep waits d or returns ctx.Err(). Injected; nil means the real timer.
	sleep func(ctx context.Context, d time.Duration) error
	// now reads the clock. Injected; nil means time.Now.
	now func() time.Time
}

// resolved applies the documented defaults to any unset field, so a zero-value
// config (a focused unit test) still polls sanely rather than hot-looping.
//
// 🔴 interval is CLAMPED UP to the floor, never merely defaulted: a caller
// passing 100ms would otherwise hammer an uncached orchestrator route. Tests
// that need a fast loop inject `sleep` instead — that is what the seam is for,
// and it keeps the floor from being negotiable in production code.
func (c pollConfig) resolved() pollConfig {
	if c.interval < defaultPollInterval {
		c.interval = defaultPollInterval
	}
	if c.maxInterval < c.interval {
		c.maxInterval = maxDuration(defaultMaxPollInterval, c.interval)
	}
	if c.factor <= 1 {
		c.factor = defaultPollFactor
	}
	if c.rateLimitInterval <= 0 {
		c.rateLimitInterval = defaultRateLimitBackoff
	}
	if c.heartbeat <= 0 {
		c.heartbeat = defaultPollHeartbeat
	}
	if c.sleep == nil {
		c.sleep = sleepCtx
	}
	if c.now == nil {
		c.now = time.Now
	}
	return c
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

// sleepCtx waits d, returning early with ctx.Err() on cancellation (Ctrl-C).
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// pollEvent is one observation handed to the reporter.
type pollEvent struct {
	attempt     int
	elapsed     time.Duration
	status      string
	wait        time.Duration
	rateLimited bool
	// err is the (retryable) failure this attempt hit, if any.
	err error
}

// pollReporter renders poll progress. Two implementations exist so the non-TTY
// path is fully test-drivable without a terminal, as internal/ui/CONVENTION.md
// requires (the precedent is waitTunnelQuiet / waitTunnelTTY).
type pollReporter interface {
	tick(pollEvent)
	finish(status string)
}

// isRetryablePollStatus reports whether an HTTP status justifies another poll.
//
// Status 0 means the request never got a response (transport/DNS/timeout) and IS
// retried: the money has already moved, so the expensive mistake is giving up on
// a workflow the user paid for, not polling a few more times. A definite 4xx
// (401/403/404) is returned immediately — more polls cannot change it.
func isRetryablePollStatus(status int) bool {
	switch {
	case status == 0: // no response at all
		return true
	case status == http.StatusRequestTimeout,
		status == http.StatusTooEarly,
		status == http.StatusTooManyRequests:
		return true
	case status >= 500:
		return true
	}
	return false
}

// pollWorkflow polls until the workflow reaches a terminal status, the wait
// times out, the context is cancelled, or a non-retryable error arrives.
//
// It returns the LAST workflow it managed to read even on a timeout, so the
// caller can report the status the job had reached rather than "unknown".
func pollWorkflow(ctx context.Context, get getWorkflowFn, workflowID string, cfg pollConfig, rep pollReporter) (*genapi.Workflow, json.RawMessage, error) {
	cfg = cfg.resolved()
	start := cfg.now()
	wait := cfg.interval

	var last *genapi.Workflow
	var lastRaw json.RawMessage

	for attempt := 1; ; attempt++ {
		wf, raw, err := get(ctx, workflowID)
		rateLimited := false

		switch {
		case err == nil:
			last, lastRaw = wf, raw
			if genapi.IsTerminalStatus(wf.Status) {
				rep.finish(wf.Status)
				return wf, raw, nil
			}
		default:
			// A cancelled context beats every other classification: the user
			// pressed Ctrl-C and the request failed BECAUSE of that.
			if ctxErr := ctx.Err(); ctxErr != nil {
				return last, lastRaw, ctxErr
			}
			st := genapi.StatusOf(err)
			if !isRetryablePollStatus(st) {
				return last, lastRaw, err
			}
			rateLimited = st == http.StatusTooManyRequests
		}

		// The server said 429. Back off to at least the rate-limit interval —
		// never merely to the next exponential step, which on an early attempt
		// is still only a few seconds.
		thisWait := wait
		if rateLimited {
			thisWait = maxDuration(thisWait, cfg.rateLimitInterval)
		}

		elapsed := cfg.now().Sub(start)
		if cfg.timeout > 0 {
			if elapsed >= cfg.timeout {
				rep.finish(statusOrUnknown(last))
				return last, lastRaw, errWaitTimeout
			}
			// Don't overshoot the deadline by a whole backoff step.
			if remaining := cfg.timeout - elapsed; remaining < thisWait {
				thisWait = remaining
			}
		}

		rep.tick(pollEvent{
			attempt:     attempt,
			elapsed:     elapsed,
			status:      statusOrUnknown(last),
			wait:        thisWait,
			rateLimited: rateLimited,
			err:         err,
		})

		if serr := cfg.sleep(ctx, thisWait); serr != nil {
			return last, lastRaw, serr
		}

		// Grow from the wait actually used, so a 429 raises the floor for every
		// subsequent poll rather than decaying back to the fast cadence.
		wait = time.Duration(float64(thisWait) * cfg.factor)
		if wait > cfg.maxInterval {
			wait = cfg.maxInterval
		}
	}
}

func statusOrUnknown(w *genapi.Workflow) string {
	if w == nil || w.Status == "" {
		return "unknown"
	}
	return w.Status
}

// ── reporters ────────────────────────────────────────────────────────────────

// newPollReporter picks the renderer for w. bubbletea/animation NEVER runs on a
// non-TTY writer; a pipe, a CI log and a test buffer all take the quiet path.
func newPollReporter(w io.Writer, now func() time.Time, heartbeat time.Duration) pollReporter {
	if now == nil {
		now = time.Now
	}
	if heartbeat <= 0 {
		heartbeat = defaultPollHeartbeat
	}
	if writerIsTTY(w) {
		return &ttyPollReporter{w: w}
	}
	return &quietPollReporter{w: w, now: now, heartbeat: heartbeat}
}

// quietPollReporter is the non-TTY path: greppable, throttled, no animation.
// This is the path every poll unit test drives, so its behaviour is
// load-bearing.
type quietPollReporter struct {
	w         io.Writer
	now       func() time.Time
	heartbeat time.Duration
	lastPrint time.Time
	printed   bool
}

func (r *quietPollReporter) tick(e pollEvent) {
	// A 429 is always reported: it is the one event where the user's next
	// question ("why is this so slow?") has a real answer.
	if e.rateLimited {
		fmt.Fprintln(r.w, ui.For(r.w).Warn(fmt.Sprintf(
			"the server rate-limited the status poll (429) — backing off %s before the next check", e.wait.Round(time.Second))))
		r.lastPrint, r.printed = r.now(), true
		return
	}
	now := r.now()
	if r.printed && now.Sub(r.lastPrint) < r.heartbeat {
		return
	}
	r.lastPrint, r.printed = now, true
	if e.err != nil {
		fmt.Fprintf(r.w, "  waiting… status %s after %s (the last status check failed: %v; retrying in %s)\n",
			safeTerm(e.status), e.elapsed.Round(time.Second), e.err, e.wait.Round(time.Second))
		return
	}
	fmt.Fprintf(r.w, "  waiting… status %s after %s (next check in %s)\n",
		safeTerm(e.status), e.elapsed.Round(time.Second), e.wait.Round(time.Second))
}

func (r *quietPollReporter) finish(status string) {
	if r.printed {
		fmt.Fprintf(r.w, "  status %s\n", safeTerm(status))
	}
}

// ttyPollReporter is the interactive path: one carriage-return-redrawn line, the
// same technique the download progress meter already uses in this binary. It
// emits no ANSI of its own.
type ttyPollReporter struct {
	w    io.Writer
	live bool
}

func (r *ttyPollReporter) tick(e pollEvent) {
	r.live = true
	suffix := fmt.Sprintf("next check in %s", e.wait.Round(time.Second))
	if e.rateLimited {
		suffix = fmt.Sprintf("rate-limited (429) — backing off %s", e.wait.Round(time.Second))
	} else if e.err != nil {
		suffix = fmt.Sprintf("status check failed, retrying in %s", e.wait.Round(time.Second))
	}
	fmt.Fprintf(r.w, "\r  generating… %s  [%s]  %s   ", fmtMMSS(e.elapsed), safeTerm(e.status), suffix)
}

func (r *ttyPollReporter) finish(status string) {
	if !r.live {
		return
	}
	fmt.Fprintf(r.w, "\r  generation %s%s\n", safeTerm(status), "                                        ")
}
