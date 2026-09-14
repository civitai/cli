package civitai

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// zeroBackoff is a pointer to a zero duration for sleepless retries in tests.
func zeroBackoff() *time.Duration { d := time.Duration(0); return &d }

// retryClient builds an anonymous client pinned at srv with instant (sleepless)
// retries and a captured stderr, so the transient-retry path runs deterministically.
func retryClient(srv *httptest.Server) (*Client, *bytes.Buffer) {
	var buf bytes.Buffer
	c := New(srv.URL, "")
	c.RetryBackoffBase = zeroBackoff()
	c.Stderr = &buf
	return c, &buf
}

// countingServer returns a server that fails its first failTimes requests with
// failStatus (optionally setting Retry-After), then serves okBody. It reports the
// total request count via the returned counter.
func countingServer(t *testing.T, failStatus, failTimes int, retryAfter, okBody string) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if int(n) <= failTimes {
			if retryAfter != "" {
				w.Header().Set("Retry-After", retryAfter)
			}
			w.WriteHeader(failStatus)
			_, _ = w.Write([]byte(`{"error":"transient"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(okBody))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestReadRetriesTransientThenSucceeds(t *testing.T) {
	// 429 is intentionally excluded here: without a Retry-After header it is
	// terminal (deep-paging limit), covered separately below. A 429 WITH
	// Retry-After is retried — see TestReadHonorsRetryAfterOn429.
	for _, status := range []int{
		http.StatusBadGateway,         // 502
		http.StatusServiceUnavailable, // 503
		http.StatusGatewayTimeout,     // 504
	} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			// Fail twice, then succeed on the 3rd try.
			srv, hits := countingServer(t, status, 2, "", `{"id":42,"name":"M"}`)
			c, errBuf := retryClient(srv)

			m, _, err := c.GetModel(context.Background(), "42")
			if err != nil {
				t.Fatalf("GetModel should recover after retries, got %v", err)
			}
			if m.ID != 42 {
				t.Errorf("parsed model wrong: %+v", m)
			}
			if *hits != 3 {
				t.Errorf("expected 3 requests (2 fail + 1 ok), got %d", *hits)
			}
			// Exactly one retry note per retry (2 retries here).
			notes := strings.Count(errBuf.String(), "retrying")
			if notes != 2 {
				t.Errorf("expected 2 retry notes, got %d (%q)", notes, errBuf.String())
			}
			if !strings.Contains(errBuf.String(), "retrying (2/4)") {
				t.Errorf("retry note should number the try: %q", errBuf.String())
			}
		})
	}
}

func TestReadGivesUpAfterMaxAttempts(t *testing.T) {
	// Always 503 → exhaust all attempts.
	srv, hits := countingServer(t, http.StatusServiceUnavailable, 99, "", "")
	c, _ := retryClient(srv)

	_, _, err := c.GetModel(context.Background(), "42")
	if err == nil {
		t.Fatal("expected a terminal error after exhausting retries")
	}
	if !strings.Contains(err.Error(), "503") || !strings.Contains(err.Error(), "4 attempts") {
		t.Errorf("error should name the status + attempts, got %v", err)
	}
	if *hits != readMaxAttempts {
		t.Errorf("expected %d requests, got %d", readMaxAttempts, *hits)
	}
}

func TestReadDoesNotRetryTerminal4xx(t *testing.T) {
	for _, status := range []int{
		http.StatusBadRequest,   // 400
		http.StatusUnauthorized, // 401 (has its own refresh path; anon => no double-handle)
		http.StatusForbidden,    // 403
		http.StatusNotFound,     // 404
	} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			srv, hits := countingServer(t, status, 99, "", "")
			c, _ := retryClient(srv) // anonymous: a 401 can't refresh → no retry

			if _, _, err := c.GetModel(context.Background(), "42"); err == nil {
				t.Fatalf("status %d should surface an error", status)
			}
			if *hits != 1 {
				t.Errorf("terminal %d must make exactly 1 request, got %d", status, *hits)
			}
		})
	}
}

func TestReadHonorsRetryAfterOn429(t *testing.T) {
	// Retry-After: 0 exercises the header path without a real sleep.
	srv, hits := countingServer(t, http.StatusTooManyRequests, 1, "0", `{"id":7,"name":"M"}`)
	c, _ := retryClient(srv)

	m, _, err := c.GetModel(context.Background(), "7")
	if err != nil {
		t.Fatalf("GetModel should recover after a 429 + Retry-After, got %v", err)
	}
	if m.ID != 7 {
		t.Errorf("parsed model wrong: %+v", m)
	}
	if *hits != 2 {
		t.Errorf("expected 2 requests (1 x 429 + 1 ok), got %d", *hits)
	}
}

// A 429 WITHOUT a Retry-After header is Civitai's deterministic deep-paging
// limit, not a transient throttle: it must NOT be retried (exactly 1 request)
// and must surface the actionable --cursor guidance from readError's 429 branch
// — not the generic "temporarily unavailable" exhaustion message.
func TestRead429WithoutRetryAfterIsTerminalWithCursorHint(t *testing.T) {
	srv, hits := countingServer(t, http.StatusTooManyRequests, 99, "", `{"error":"You've paged too deep"}`)
	c, errBuf := retryClient(srv)

	_, _, err := c.GetModel(context.Background(), "42")
	if err == nil {
		t.Fatal("a Retry-After-less 429 should surface a terminal error")
	}
	if *hits != 1 {
		t.Errorf("a deterministic 429 must make exactly 1 request, got %d", *hits)
	}
	if !strings.Contains(err.Error(), "--cursor") {
		t.Errorf("error should preserve the --cursor guidance, got %v", err)
	}
	if strings.Contains(err.Error(), "temporarily unavailable") {
		t.Errorf("terminal 429 must NOT surface the retry-exhaustion message, got %v", err)
	}
	if strings.Contains(errBuf.String(), "retrying") {
		t.Errorf("a terminal 429 must not emit a retry notice, got %q", errBuf.String())
	}
}

// A 429 that KEEPS returning Retry-After is a genuine throttle: it is retried up
// to the cap and, if it never clears, exhausts with the generic message.
func TestRead429WithRetryAfterExhaustsGeneric(t *testing.T) {
	srv, hits := countingServer(t, http.StatusTooManyRequests, 99, "0", "")
	c, _ := retryClient(srv)

	_, _, err := c.GetModel(context.Background(), "42")
	if err == nil {
		t.Fatal("a persistent 429+Retry-After should exhaust with an error")
	}
	if *hits != readMaxAttempts {
		t.Errorf("expected %d requests, got %d", readMaxAttempts, *hits)
	}
	if !strings.Contains(err.Error(), "HTTP 429 after 4 attempts") {
		t.Errorf("exhaustion should name status + attempts, got %v", err)
	}
	if !strings.Contains(err.Error(), "temporarily unavailable") {
		t.Errorf("persistent throttle should surface the generic message, got %v", err)
	}
}

// 🔴 THE ORDERING ITSELF — the header is consulted BEFORE the message, so a
// CAP-WORDED 429 that carries Retry-After is retried to exhaustion and exits 5,
// NOT reclassified to 2. That is published as a 🔴 bullet in README's exit-code
// rows and in exitcodes_doc.go, and until this test it was UNGUARDED. Consulting
// the message first — return terminal on cap wording, whatever the header —
// inverts the bullet from 5 to 2, and with the mutant applied the ONLY `--- FAIL`
// in the whole repo is this test (20 ok packages otherwise), so nothing that
// existed before it could see the swap. Why not: retry_test.go's two neighbours
// each hold one half of the input fixed —
// TestRead429WithRetryAfterExhaustsGeneric sends an EMPTY body and
// TestRead429WithoutRetryAfterIsTerminalWithCursorHint sends no header — so
// neither can distinguish the two orderings. It takes BOTH at once.
//
// The VENDORED assumption (the server never attaches Retry-After to a cap 429)
// is genuinely unguardable here. The CLI's own ordering is not, and this is it.
//
// Asserted by errors.Is per AGENTS.md item 7: the sentinels carry no visible
// text, so the message assertions below cannot see a classification swap.
func TestCapWorded429WithRetryAfterIsRetriedNotReclassified(t *testing.T) {
	// Cap wording (isDeepPagingCap matches "too many pages" / "use cursors")
	// AND a Retry-After header, on every response. countingServer cannot express
	// this: its failure body is fixed.
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Retry-After", "0") // 0 exercises the header path sleeplessly
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"You've requested too many pages, please use cursors instead"}`))
	}))
	t.Cleanup(srv.Close)
	c, _ := retryClient(srv)

	_, _, err := c.GetModel(context.Background(), "42")
	if err == nil {
		t.Fatal("a persistent cap-worded 429 + Retry-After should surface an error")
	}
	// The header decided: retried to the cap, not terminal on the first try.
	if hits != readMaxAttempts {
		t.Errorf("header-before-message means this is RETRIED: expected %d requests, got %d "+
			"(1 would mean the cap wording was consulted first)", readMaxAttempts, hits)
	}
	// Exit 5, not 2. ErrNetwork is what retryExhaustedError tags; ErrBadRequest
	// is what readError's isDeepPagingCap branch would tag if the message won.
	if !errors.Is(err, ErrNetwork) {
		t.Errorf("must stay ErrNetwork (exit 5), got %v", err)
	}
	if errors.Is(err, ErrBadRequest) {
		t.Errorf("must NOT be reclassified to ErrBadRequest (exit 2) — that is the "+
			"published contract inverted, got %v", err)
	}
	if errors.Is(err, ErrRateLimited) {
		t.Errorf("must NOT be ErrRateLimited (exit 6), got %v", err)
	}
	// And the message the README's exit-5 row tells a reader to grep for, not
	// the exit-2/6 one. The cap wording still appears via snippet(raw), which is
	// why `rate limited (429)` and `--cursor` — readError's text, absent from the
	// body — are the discriminating absences.
	if !strings.Contains(err.Error(), "Civitai returned HTTP 429 after 4 attempts") {
		t.Errorf("should print the exhaustion message, got %v", err)
	}
	if strings.Contains(err.Error(), "rate limited (429)") || strings.Contains(err.Error(), "--cursor") {
		t.Errorf("must not print readError's 429 text — that is the terminal path, got %v", err)
	}
}

// The exhaustion error must not emit a dangling ": " separator when the response
// body is empty.
func TestRetryExhaustedNoTrailingColonOnEmptyBody(t *testing.T) {
	// Always 503 with an EMPTY body → exhaust, empty snippet.
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	c, _ := retryClient(srv)

	_, _, err := c.GetModel(context.Background(), "42")
	if err == nil {
		t.Fatal("expected an exhaustion error")
	}
	if strings.HasSuffix(err.Error(), ": ") || strings.Contains(err.Error(), "shortly: ") {
		t.Errorf("empty body must not leave a dangling %q separator, got %q", ": ", err.Error())
	}
}

// backoffFor: a zero base yields exactly 0 (tests stay instant + deterministic);
// a positive base yields a jittered delay that always stays within [0, cap].
func TestBackoffForJitterBounds(t *testing.T) {
	// base == 0 → always 0, across all attempts.
	zeroC := &Client{RetryBackoffBase: zeroBackoff()}
	for attempt := 0; attempt < 5; attempt++ {
		if d := zeroC.backoffFor(attempt, nil); d != 0 {
			t.Fatalf("base=0 attempt=%d: got %v, want 0", attempt, d)
		}
	}

	// base > 0 → jittered, but always within [0, retryBackoffMax].
	base := 250 * time.Millisecond
	c := &Client{RetryBackoffBase: &base}
	for attempt := 0; attempt < 8; attempt++ {
		for i := 0; i < 200; i++ {
			d := c.backoffFor(attempt, nil)
			if d < 0 || d > retryBackoffMax {
				t.Fatalf("attempt=%d: jittered delay %v out of [0,%v]", attempt, d, retryBackoffMax)
			}
		}
	}
}

func TestRetryAfterDelayParsesAndCaps(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   time.Duration
		ok     bool
	}{
		{"seconds", "2", 2 * time.Second, true},
		{"caps-large", "3600", retryAfterCap, true}, // capped to 5s
		{"zero", "0", 0, true},
		{"absent", "", 0, false},
		{"garbage", "soon", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			if tc.header != "" {
				h.Set("Retry-After", tc.header)
			}
			got, ok := retryAfterDelay(h, retryAfterCap)
			if ok != tc.ok || got != tc.want {
				t.Errorf("retryAfterDelay(%q) = (%v,%v), want (%v,%v)", tc.header, got, ok, tc.want, tc.ok)
			}
		})
	}
	// An HTTP-date in the far future caps to the ceiling.
	h := http.Header{}
	h.Set("Retry-After", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat))
	if got, ok := retryAfterDelay(h, retryAfterCap); !ok || got != retryAfterCap {
		t.Errorf("future HTTP-date should cap to %v, got (%v,%v)", retryAfterCap, got, ok)
	}
}

// failFirstTransport fails the first `fails` round-trips with err, then delegates.
type failFirstTransport struct {
	calls int32
	fails int32
	err   error
	base  http.RoundTripper
}

func (t *failFirstTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if atomic.AddInt32(&t.calls, 1) <= t.fails {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: t.err}
	}
	return t.base.RoundTrip(r)
}

func TestReadRetriesTransientNetworkError(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(`{"id":5,"name":"M"}`))
	}))
	defer srv.Close()

	tr := &failFirstTransport{fails: 1, err: syscall.ECONNREFUSED, base: http.DefaultTransport}
	c := New(srv.URL, "")
	c.RetryBackoffBase = zeroBackoff()
	c.Stderr = &bytes.Buffer{}
	c.HTTP = &http.Client{Transport: tr}

	m, _, err := c.GetModel(context.Background(), "5")
	if err != nil {
		t.Fatalf("a transient connection-refused should be retried, got %v", err)
	}
	if m.ID != 5 {
		t.Errorf("parsed model wrong: %+v", m)
	}
	if tr.calls != 2 {
		t.Errorf("expected 2 transport calls (1 refused + 1 ok), got %d", tr.calls)
	}
	if hits != 1 {
		t.Errorf("server should be reached once (after the refused dial), got %d", hits)
	}
}

func TestIsTransientNetErr(t *testing.T) {
	if isTransientNetErr(nil) {
		t.Error("nil is not transient")
	}
	if !isTransientNetErr(&net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}) {
		t.Error("ECONNREFUSED should be transient")
	}
	if !isTransientNetErr(&net.OpError{Op: "read", Err: syscall.ECONNRESET}) {
		t.Error("ECONNRESET should be transient")
	}
	if isTransientNetErr(context.Canceled) {
		t.Error("a user cancellation must NOT be retried")
	}
}
