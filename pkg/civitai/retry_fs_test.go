package civitai

// A FILESYSTEM ERROR IS NOT A TRANSIENT NETWORK ERROR.
//
// This file is the retry-loop half of the guard AGENTS.md item 24 describes for
// the exit-code classifier. It is issue #244, a SECOND COPY of issue #241: PR
// #242 fixed `cmd/civitai/main.go`'s isNetworkErr, while isTransientNetErr here
// still ended with the identical spelling —
//
//	var netErr net.Error
//	if errors.As(err, &netErr) { return netErr.Timeout() }
//
// — and `syscall.Errno.Timeout()` is TRUE for ETIMEDOUT, EAGAIN and EWOULDBLOCK.
// So a filesystem failure carrying one of those (a config write on NFS-soft,
// sshfs or CIFS) entered the retry loop: reachable through the TokenSource seam,
// because internal/auth's Token() returns `persist refreshed tokens: %w` when
// writing rotated OAuth tokens fails, and that error flows straight into
// getWithRetry's isTransientNetErr arm.
//
// Three batteries, and none subsumes the others:
//
//   - TestFilesystemErrorsAreNotRetried drives the REAL getWithRetry and asserts
//     the observed ATTEMPT COUNT (1 token call, 0 server hits, 0 retry notices).
//     Asserting only "an error came back" cannot see the defect at all — the
//     buggy loop also returns an error, after burning four attempts.
//   - TestTransientTransportErrorsStillRetry is the POSITIVE CONTROL battery.
//     Without it, a "fix" that deleted the isTransientNetErr net.Error branch
//     outright would leave the first battery green.
//   - TestSyscallErrnoTimeoutIsStillTrue pins the stdlib fact the whole thing
//     rests on, so a row that stops exercising the trap cannot go green quietly.
//
// Classification is asserted through OBSERVED BEHAVIOUR (attempt counts, errors.Is)
// and never through message text, per AGENTS.md item 7.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// The trap
// ---------------------------------------------------------------------------

// TestSyscallErrnoTimeoutIsStillTrue pins the two stdlib facts that make the
// naive spelling wrong, so a Go release that changes either fails HERE with an
// explanation rather than silently making the rows below vacuous.
func TestSyscallErrnoTimeoutIsStillTrue(t *testing.T) {
	if _, ok := any(syscall.ETIMEDOUT).(net.Error); !ok {
		t.Fatal("syscall.Errno no longer satisfies net.Error.\n" +
			"GOOD NEWS, not a failure to silence: the hazard has left the standard library.\n" +
			"Re-measure before deleting anything — the *fs.PathError/*os.LinkError terminators\n" +
			"guard the mirror-image hazard of a WRAPPER gaining Temporary().")
	}
	for _, e := range []syscall.Errno{syscall.ETIMEDOUT, syscall.EAGAIN, syscall.EWOULDBLOCK} {
		if !e.Timeout() {
			t.Errorf("%v.Timeout() is no longer true — the rows built on it no longer exercise the trap", e)
		}
	}
	// The controls' half: these were never mis-sorted, because Timeout() is false.
	for _, e := range []syscall.Errno{syscall.EACCES, syscall.ENOENT, syscall.EIO, syscall.ENOSPC} {
		if e.Timeout() {
			t.Errorf("%v.Timeout() is now true — this errno moved from the control group into the trap", e)
		}
	}
}

// ---------------------------------------------------------------------------
// Harness
// ---------------------------------------------------------------------------

// failingTokenSource is the seam the reachable defect arrives through:
// internal/auth's Tokens.Token(ctx) returns `persist refreshed tokens: %w` when
// persisting rotated OAuth tokens fails, and authedDoHdr surfaces that error
// verbatim into getWithRetry's transient arm.
type failingTokenSource struct {
	err   error
	calls int32
}

func (s *failingTokenSource) Token(context.Context) (string, error) {
	atomic.AddInt32(&s.calls, 1)
	return "", s.err
}

func (s *failingTokenSource) Refresh(context.Context) (string, error) { return "", ErrNoRefresh }

// roundTripperFunc injects a transport-layer error into the REAL http.Client, so
// the error getWithRetry sees is the one net/http hands us (a *url.Error around
// the transport's error) rather than a constructed shape.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// observation is what the retry loop DID, which is the only thing that can tell
// a retried failure from a terminal one.
type observation struct {
	tokenCalls int
	serverHits int
	notices    int
	err        error
}

// noticeCount counts the one-line retry notices without depending on their
// wording: the loop emits exactly one line per retry.
func noticeCount(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	return len(strings.Split(s, "\n"))
}

// driveReadWithTokenError runs the REAL read GET path (getRaw → getWithRetry)
// against a live server, with a TokenSource that fails with tokenErr. The server
// counts hits so "0 requests reached the wire" is measured, not assumed.
func driveReadWithTokenError(t *testing.T, tokenErr error) observation {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	src := &failingTokenSource{err: tokenErr}
	c, buf := retryClient(srv)
	c.Tokens = src
	c.HTTP = srv.Client()

	_, _, err := c.getRaw(context.Background(), "/api/v1/probe", nil)
	return observation{
		tokenCalls: int(atomic.LoadInt32(&src.calls)),
		serverHits: int(atomic.LoadInt32(&hits)),
		notices:    noticeCount(buf.String()),
		err:        err,
	}
}

// driveReadWithTransportError runs the same real path with a healthy token and a
// transport that always fails with transportErr.
func driveReadWithTransportError(t *testing.T, transportErr error) observation {
	t.Helper()
	var hits int32
	c := New("http://civitai.invalid", "tok")
	d := time.Duration(0)
	c.RetryBackoffBase = &d
	var sb strings.Builder
	c.Stderr = &sb
	c.HTTP = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		atomic.AddInt32(&hits, 1)
		return nil, transportErr
	})}

	_, _, err := c.getRaw(context.Background(), "/api/v1/probe", nil)
	return observation{
		tokenCalls: 1,
		serverHits: int(atomic.LoadInt32(&hits)),
		notices:    noticeCount(sb.String()),
		err:        err,
	}
}

// ---------------------------------------------------------------------------
// Regression: a filesystem error must never enter the retry loop
// ---------------------------------------------------------------------------

// TestFilesystemErrorsAreNotRetried is the regression coverage.
//
// RED AT BASE (origin/main, 050d401): the three Timeout()==true rows observe
// tokenCalls=4 / notices=3 instead of 1 / 0.
// GREEN AT HEAD: every row observes exactly one attempt.
//
// The EACCES/ENOENT/EIO/ENOSPC rows are CONTROLS that already passed at base —
// invariant guards, not regression coverage. They are here so a future change to
// the walk that starts retrying them fails loudly.
func TestFilesystemErrorsAreNotRetried(t *testing.T) {
	cases := []struct {
		name string
		// errno is the errno the filesystem failure carries.
		errno syscall.Errno
		// trapped records whether this errno's Timeout() is true, i.e. whether
		// the row exercises the defect or is a control that never could.
		trapped bool
	}{
		{"ETIMEDOUT (Timeout()==true — the reachable defect)", syscall.ETIMEDOUT, true},
		{"EAGAIN (Timeout()==true — the reachable defect)", syscall.EAGAIN, true},
		{"EWOULDBLOCK (Timeout()==true — the reachable defect)", syscall.EWOULDBLOCK, true},

		{"EACCES (control: Timeout()==false, correct at base)", syscall.EACCES, false},
		{"ENOENT (control: Timeout()==false, correct at base)", syscall.ENOENT, false},
		{"EIO (control: Timeout()==false, correct at base)", syscall.EIO, false},
		{"ENOSPC (control: Timeout()==false, correct at base)", syscall.ENOSPC, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Half one: the row's premise. A row whose errno stopped being
			// trapped (or started being) says nothing about the guard.
			if got := tc.errno.Timeout(); got != tc.trapped {
				t.Fatalf("%v.Timeout() = %v, want %v — this row no longer exercises what it claims", tc.errno, got, tc.trapped)
			}

			// The shape internal/auth/source.go really produces: a persist
			// failure wrapped around the *fs.PathError os.WriteFile returns.
			tokenErr := fmt.Errorf("persist refreshed tokens: %w",
				&fs.PathError{Op: "write", Path: "/cfg/config.yaml", Err: tc.errno})

			got := driveReadWithTokenError(t, tokenErr)

			if got.tokenCalls != 1 {
				t.Errorf("Token() was called %d times, want 1.\n"+
					"A filesystem failure was fed to the transient-retry loop: the user watches\n"+
					"%d `network error from Civitai, retrying (n/4)…` lines plus backoff for a\n"+
					"problem that never clears.", got.tokenCalls, got.notices)
			}
			if got.serverHits != 0 {
				t.Errorf("%d request(s) reached the server, want 0 — the failure is before the wire", got.serverHits)
			}
			if got.notices != 0 {
				t.Errorf("%d retry notice(s) printed, want 0 — nothing about a filesystem failure is retriable", got.notices)
			}
			// The loop must hand the error back UNTOUCHED on the first attempt.
			// Identity is the structural form of "it was not re-wrapped as
			// `failed after N attempts`", asserted without reading any message.
			if got.err != tokenErr {
				t.Errorf("the returned error is not the one the TokenSource produced (%T) — it was\n"+
					"re-wrapped, which only the exhausted-retry path does", got.err)
			}
			if !errors.Is(got.err, tc.errno) {
				t.Errorf("the returned error no longer carries %v — the fixture is not exercising the errno path", tc.errno)
			}
		})
	}
}

// TestFilesystemErrorShapesAreNotRetried covers the wrapper shapes the CLI's
// filesystem call sites actually return, beyond the *fs.PathError of the
// reachable case: a bare errno, an *os.LinkError, and an *os.SyscallError.
//
// RED AT BASE for every row (all three carry ETIMEDOUT, whose Timeout() is true).
func TestFilesystemErrorShapesAreNotRetried(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"bare syscall.ETIMEDOUT", syscall.ETIMEDOUT},
		{"*fs.PathError", &fs.PathError{Op: "write", Path: "/cfg/config.yaml", Err: syscall.ETIMEDOUT}},
		{"*os.LinkError", &os.LinkError{Op: "rename", Old: "/cfg/.tmp", New: "/cfg/config.yaml", Err: syscall.ETIMEDOUT}},
		{"*os.SyscallError", os.NewSyscallError("fsync", syscall.ETIMEDOUT)},
		{"multi-error tree (prose, *fs.PathError)", errors.Join(
			errors.New("persisting rotated tokens"),
			&fs.PathError{Op: "write", Path: "/cfg/config.yaml", Err: syscall.ETIMEDOUT})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := driveReadWithTokenError(t, tc.err)
			if got.tokenCalls != 1 || got.notices != 0 || got.serverHits != 0 {
				t.Errorf("tokenCalls=%d notices=%d serverHits=%d, want 1/0/0 — a %T entered the retry loop",
					got.tokenCalls, got.notices, got.serverHits, tc.err)
			}
		})
	}
}

// TestContextCancelledIsNeverRetried pins the one exclusion that predates this
// fix: a user SIGINT is not transient. INVARIANT GUARD — green at base.
func TestContextCancelledIsNeverRetried(t *testing.T) {
	got := driveReadWithTokenError(t, fmt.Errorf("token: %w", context.Canceled))
	if got.tokenCalls != 1 || got.notices != 0 {
		t.Errorf("tokenCalls=%d notices=%d, want 1/0 — a cancelled context was retried", got.tokenCalls, got.notices)
	}
}

// ---------------------------------------------------------------------------
// Positive controls: what MUST still retry
// ---------------------------------------------------------------------------

// TestTransientTransportErrorsStillRetry is the other half of the guard. Without
// it, a "fix" that simply deleted isTransientNetErr's net.Error branch would
// leave every filesystem row above green.
//
// 🔴 INVARIANT GUARD, not regression coverage — every row passes at base too.
// That is exactly its job.
//
// Rows carrying an errno that is NOT one of the two explicit sentinels
// (ECONNREFUSED / ECONNRESET) are the ones that pin the WALK rather than the
// shortcut: gutting the walk leaves a sentinel-backed row green.
func TestTransientTransportErrorsStillRetry(t *testing.T) {
	realDeadline := realReadDeadlineExceeded(t)

	for _, tc := range []struct {
		name string
		err  error
		// sentinelFree marks a row no errors.Is sentinel can carry, so its
		// green is evidence about the net.Error walk itself.
		sentinelFree bool
	}{
		{"*net.OpError nesting *os.SyscallError(ECONNRESET)",
			&net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)}, false},
		{"*net.OpError nesting *os.SyscallError(ETIMEDOUT)",
			&net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ETIMEDOUT)}, true},
		{"*net.OpError nesting *os.SyscallError(EAGAIN)",
			&net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.EAGAIN)}, true},
		{"REAL read deadline exceeded (*net.OpError from conn.Read)", realDeadline, true},
		{"*net.DNSError (IsTimeout)",
			&net.DNSError{Err: "i/o timeout", Name: "civitai.com", IsTimeout: true}, true},
		{"*url.Error wrapping a timeout *net.OpError",
			&url.Error{Op: "Get", URL: "https://civitai.com/api/v1/models",
				Err: &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ETIMEDOUT)}}, true},
		{"context.DeadlineExceeded", context.DeadlineExceeded, true},
		{"bare syscall.ECONNREFUSED", syscall.ECONNREFUSED, false},
		{"bare syscall.ECONNRESET", syscall.ECONNRESET, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := driveReadWithTransportError(t, tc.err)
			if got.serverHits != readMaxAttempts {
				t.Errorf("the transport was tried %d time(s), want %d.\n"+
					"A genuine transport failure must stay retryable — the filesystem fix must not\n"+
					"have disabled the retry loop. err = %#v", got.serverHits, readMaxAttempts, tc.err)
			}
			if got.notices != readMaxAttempts-1 {
				t.Errorf("%d retry notice(s), want %d", got.notices, readMaxAttempts-1)
			}
			if got.err == nil {
				t.Fatal("exhausted retries returned no error")
			}
			if tc.sentinelFree {
				// Independent of the loop: the shared predicate itself must see
				// this shape. A row that only passes because errors.Is found
				// ECONNREFUSED/ECONNRESET cannot report a regression in the walk.
				if errors.Is(tc.err, syscall.ECONNREFUSED) || errors.Is(tc.err, syscall.ECONNRESET) {
					t.Fatalf("row is marked sentinel-free but carries a sentinel — it cannot pin the walk")
				}
				if !isTransientNetErr(tc.err) {
					t.Errorf("isTransientNetErr said false for a sentinel-free transport failure — the\n" +
						"net.Error walk is not seeing it, so the loop above is passing for some other reason")
				}
			}
		})
	}
}

// TestRealRefusedDialStillRetries drives the retry loop with a genuinely refused
// loopback dial — the error net/http really produces, rather than a constructed
// one. INVARIANT GUARD.
func TestRealRefusedDialStillRetries(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not bind a loopback listener: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("could not release the listener: %v", err)
	}

	c := New("http://"+addr, "tok")
	d := time.Duration(0)
	c.RetryBackoffBase = &d
	var sb strings.Builder
	c.Stderr = &sb
	c.HTTP = &http.Client{Timeout: 5 * time.Second}

	_, _, gerr := c.getRaw(context.Background(), "/api/v1/probe", nil)
	if gerr == nil {
		t.Skip("something else grabbed the port between close and dial — the refusal is not observable here")
	}
	if n := noticeCount(sb.String()); n != readMaxAttempts-1 {
		t.Errorf("a refused dial produced %d retry notice(s), want %d", n, readMaxAttempts-1)
	}
	// It is classified by the shared predicate, not only by a sentinel: assert
	// both, so gutting the walk cannot hide behind errors.Is(ECONNREFUSED).
	var ue *url.Error
	if !errors.As(gerr, &ue) {
		t.Fatalf("expected net/http's *url.Error, got %T", gerr)
	}
	if _, ok := transportError(ue); !ok {
		t.Error("transportError did not see a real refused dial — the walk is broken and this row\n" +
			"is only green because errors.Is found ECONNREFUSED")
	}
}

// TestRetriableStatusesStillRetry pins that the STATUS side of the loop is
// untouched by the error-classification fix. INVARIANT GUARD.
func TestRetriableStatusesStillRetry(t *testing.T) {
	for _, status := range []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var hits int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&hits, 1)
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"transient"}`))
			}))
			t.Cleanup(srv.Close)

			c, buf := retryClient(srv)
			c.HTTP = srv.Client()
			_, _, err := c.getRaw(context.Background(), "/api/v1/probe", nil)
			if err == nil {
				t.Fatal("a server answering only this status produced no error")
			}
			if got := int(atomic.LoadInt32(&hits)); got != readMaxAttempts {
				t.Errorf("%d request(s) reached the server, want %d", got, readMaxAttempts)
			}
			if n := noticeCount(buf.String()); n != readMaxAttempts-1 {
				t.Errorf("%d retry notice(s), want %d", n, readMaxAttempts-1)
			}
			if !errors.Is(err, ErrNetwork) {
				t.Errorf("exhausted retries on HTTP %d must stay tagged ErrNetwork (exit 5)", status)
			}
		})
	}
}

// TestTransportErrorFromTheTokenSourceStillRetries is the positive control for
// the INJECTION POINT the regression uses. Without it, "no filesystem error is
// retried" would also be satisfied by never retrying anything a TokenSource
// returns — and internal/auth's refresh really does make an HTTP call, so a
// transport failure genuinely arrives through this seam.
//
// INVARIANT GUARD — green at base.
func TestTransportErrorFromTheTokenSourceStillRetries(t *testing.T) {
	tokenErr := fmt.Errorf("refreshing tokens: %w", &url.Error{
		Op:  "Post",
		URL: "https://civitai.com/api/auth/token",
		Err: &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ETIMEDOUT)},
	})
	got := driveReadWithTokenError(t, tokenErr)
	if got.tokenCalls != readMaxAttempts {
		t.Errorf("Token() was called %d times, want %d — a transport failure from the token source\n"+
			"must still be retried", got.tokenCalls, readMaxAttempts)
	}
	if got.notices != readMaxAttempts-1 {
		t.Errorf("%d retry notice(s), want %d", got.notices, readMaxAttempts-1)
	}
}

// realReadDeadlineExceeded produces a genuine timeout *net.OpError from the net
// stack: a live loopback connection whose read deadline has already passed. A
// constructed *net.OpError proves nothing about what the net stack really hands
// us; this cannot flake on a slow box or a sandbox with no route out.
func realReadDeadlineExceeded(t *testing.T) error {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not bind a loopback listener: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	accepted := make(chan net.Conn, 1)
	go func() {
		c, aerr := ln.Accept()
		if aerr == nil {
			accepted <- c
		}
		close(accepted)
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("could not dial the loopback listener: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if c := <-accepted; c != nil {
		t.Cleanup(func() { _ = c.Close() })
	}

	if err := conn.SetReadDeadline(time.Now().Add(-time.Second)); err != nil {
		t.Fatalf("could not set a read deadline: %v", err)
	}
	_, rerr := conn.Read(make([]byte, 1))
	if rerr == nil {
		t.Fatal("a read past its deadline returned no error")
	}
	var ne net.Error
	if !errors.As(rerr, &ne) || !ne.Timeout() {
		t.Fatalf("expected a timeout net.Error, got %T: %v", rerr, rerr)
	}
	return rerr
}
