package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Bounded transient-failure retry for the IDEMPOTENT read GET path only
// (getRaw → getWithRetry). It deliberately does NOT wrap the mutating calls
// (submit / dev-token / withdraw / dev-tunnel), which share authedDo but must
// not be replayed, nor the download stream (a mid-stream 5xx on a multi-GB body
// is out of scope). A read GET is safe to replay, so a brief backing-off retry
// turns the observed 5–8 consecutive 503s from the search backend into a
// transparent recovery instead of a hard failure.

const (
	// readMaxAttempts bounds the total number of tries (1 initial + up to 3
	// retries) for a transient failure on a read GET.
	readMaxAttempts = 4
	// defaultRetryBackoff is the base of the exponential backoff between tries
	// (base, 2·base, 4·base…). Kept modest so a genuinely-down backend fails
	// fast; tests inject 0 via Client.RetryBackoffBase for sleepless runs.
	defaultRetryBackoff = 250 * time.Millisecond
	// retryBackoffMax caps a single computed backoff delay.
	retryBackoffMax = 5 * time.Second
	// retryAfterCap caps how long a 429 Retry-After header can make us wait, so
	// a hostile/huge value can't hang the CLI.
	retryAfterCap = 5 * time.Second
)

// isRetriableStatus reports whether an HTTP status is a TRANSIENT server-side
// condition worth retrying on an idempotent GET: 429 (rate limited) and the
// 502/503/504 gateway/overload family. Other 4xx (400/401/403/404) are terminal
// — a retry can't change them — and 401 already has its own refresh path.
func isRetriableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, // 429
		http.StatusBadGateway,         // 502
		http.StatusServiceUnavailable, // 503
		http.StatusGatewayTimeout:     // 504
		return true
	default:
		return false
	}
}

// isTransientNetErr reports whether a request error is a transient network
// condition worth retrying (timeout, connection refused, connection reset). A
// context cancellation (user SIGINT) is NOT transient and is never retried.
func isTransientNetErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}

// retryAfterDelay parses a Retry-After header (delta-seconds or an HTTP-date)
// into a delay, clamped to [0, cap]. The bool reports whether a usable value was
// present.
func retryAfterDelay(h http.Header, cap time.Duration) (time.Duration, bool) {
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(v); err == nil {
		return clampDelay(time.Duration(secs)*time.Second, cap), true
	}
	if t, err := http.ParseTime(v); err == nil {
		return clampDelay(time.Until(t), cap), true
	}
	return 0, false
}

// clampDelay bounds d to [0, cap].
func clampDelay(d, cap time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	if d > cap {
		return cap
	}
	return d
}

// backoffFor returns the delay before the retry following a 0-indexed failed
// attempt. A non-nil override (a 429's Retry-After) wins; otherwise it is an
// exponential base·2^attempt capped at retryBackoffMax. The base defaults to
// defaultRetryBackoff and is overridden by Client.RetryBackoffBase (tests set it
// to 0 for instant, sleepless retries).
func (c *Client) backoffFor(attempt int, override *time.Duration) time.Duration {
	if override != nil {
		return clampDelay(*override, retryBackoffMax)
	}
	base := defaultRetryBackoff
	if c.RetryBackoffBase != nil {
		base = *c.RetryBackoffBase
	}
	if base <= 0 {
		return 0
	}
	d := base << attempt
	if d <= 0 { // overflow guard
		return retryBackoffMax
	}
	return clampDelay(d, retryBackoffMax)
}

// retryStderr is where the one-line retry notices go; nil defaults to os.Stderr.
func (c *Client) retryStderr() io.Writer {
	if c.Stderr != nil {
		return c.Stderr
	}
	return os.Stderr
}

// retryNote renders the one-line, once-per-retry stderr notice. attempt is the
// 0-indexed attempt that just failed, so the human-facing try number is
// attempt+2 (the initial try is 1).
func retryNote(reason string, attempt, max int) string {
	return fmt.Sprintf("%s from Civitai, retrying (%d/%d)…", reason, attempt+2, max)
}

// retryExhaustedError is returned when a retriable status persisted through all
// attempts; it names the status and the attempt count so the failure is
// unambiguous.
func retryExhaustedError(status, attempts int, raw []byte) error {
	return fmt.Errorf("Civitai returned HTTP %d after %d attempts — the service is temporarily unavailable, try again shortly: %s", status, attempts, snippet(raw))
}

// sleepCtx waits for d or until ctx is cancelled, returning ctx.Err() on
// cancellation. A non-positive d returns immediately.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
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

// getWithRetry issues the read GET built by build, retrying transient failures
// (retriable statuses + transient network errors) with bounded exponential
// backoff. It reuses authedDoHdr so the existing single 401-refresh still
// happens inside each try. Terminal statuses (incl. 400/401/403/404) return on
// the first try, so a non-transient read makes exactly one request.
func (c *Client) getWithRetry(ctx context.Context, path string, build func() (*http.Request, error)) (int, []byte, error) {
	for attempt := 0; ; attempt++ {
		status, hdr, raw, err := c.authedDoHdr(ctx, build)
		switch {
		case err != nil && isTransientNetErr(err):
			if attempt >= readMaxAttempts-1 {
				return 0, nil, fmt.Errorf("request to %s failed after %d attempts: %w", path, readMaxAttempts, err)
			}
			fmt.Fprintln(c.retryStderr(), retryNote("network error", attempt, readMaxAttempts))
			if serr := sleepCtx(ctx, c.backoffFor(attempt, nil)); serr != nil {
				return 0, nil, serr
			}
		case err != nil:
			return 0, nil, err
		case isRetriableStatus(status):
			if attempt >= readMaxAttempts-1 {
				return status, raw, retryExhaustedError(status, readMaxAttempts, raw)
			}
			var override *time.Duration
			if status == http.StatusTooManyRequests {
				if d, ok := retryAfterDelay(hdr, retryAfterCap); ok {
					override = &d
				}
			}
			fmt.Fprintln(c.retryStderr(), retryNote(fmt.Sprintf("transient %d", status), attempt, readMaxAttempts))
			if serr := sleepCtx(ctx, c.backoffFor(attempt, override)); serr != nil {
				return 0, nil, serr
			}
		default:
			return status, raw, nil
		}
	}
}

// authedDoHdr is authedDo but also returns the response headers (needed to read
// a 429's Retry-After for the read-retry path). Like authedDo it attaches the
// bearer token and does a single transparent 401-refresh + retry; a
// non-refreshable source keeps the original 401.
func (c *Client) authedDoHdr(ctx context.Context, build func() (*http.Request, error)) (int, http.Header, []byte, error) {
	token, err := c.Tokens.Token(ctx)
	if err != nil {
		return 0, nil, nil, err
	}
	status, hdr, raw, err := c.doOnceHdr(build, token)
	if err != nil {
		return 0, nil, nil, err
	}
	if status == http.StatusUnauthorized {
		if newTok, rerr := c.Tokens.Refresh(ctx); rerr == nil && newTok != "" {
			return c.doOnceHdr(build, newTok)
		}
	}
	return status, hdr, raw, nil
}

// doOnceHdr is doOnceWith against the shared client, additionally returning the
// response headers.
func (c *Client) doOnceHdr(build func() (*http.Request, error), token string) (int, http.Header, []byte, error) {
	req, err := build()
	if err != nil {
		return 0, nil, nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, resp.Header, raw, nil
}
