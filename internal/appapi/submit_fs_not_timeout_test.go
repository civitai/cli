package appapi

// A FILESYSTEM ERROR IS NOT A REQUEST TIMEOUT, BECAUSE NOTHING WAS SENT.
//
// This file is the submit-path half of the guard AGENTS.md item 24 describes —
// issue #246, the THIRD copy of issues #241 and #244. isTimeoutErr used to read
//
//	if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) { return true }
//	var netErr net.Error
//	if errors.As(err, &netErr) { return netErr.Timeout() }
//
// and `syscall.Errno.Timeout()` is TRUE for ETIMEDOUT, EAGAIN and EWOULDBLOCK.
//
// 🔴 IT WAS FILESYSTEM-BROAD THROUGH TWO COMPLEMENTARY LINES, SO FIXING THE
// errors.As SPELLING ALONE IS A HALF-FIX. Measured on go1.25.12:
//
//   - os.IsTimeout unwraps *fs.PathError / *os.LinkError / *os.SyscallError at
//     the TOP LEVEL only. os.IsTimeout(&fs.PathError{Err: ETIMEDOUT}) is TRUE
//     while errors.As is what catches the same PathError under a fmt.Errorf
//     wrapper (os.IsTimeout of that is FALSE).
//
// So the table below carries BOTH shapes — DIRECT (only os.IsTimeout sees it)
// and WRAPPED (only errors.As sees it) — and each row records which half of the
// old predicate matched it. A gate placed above only one line leaves the other
// shape's rows red.
//
// The defect is reachable through the real submit path, not theoretical:
// internal/cmd/app_submit.go wires auth.New(cfg) into SubmitVersion →
// authedDoWith → auth.Source.Token(ctx) → refreshLocked → cfg.SetOAuthTokens →
// config.save(), and internal/auth/source.go:115 returns
// `persist refreshed tokens: %w` when that filesystem write fails. A config dir
// on NFS-soft / sshfs / CIFS fails with exactly those errnos, and the CLI then
// polls /submissions three times for a submission that never existed before
// telling the author the upload "may not have completed".
//
// Three batteries, and none subsumes the others:
//
//   - TestIsTimeoutErrRejectsFilesystemErrors asserts the PREDICATE, so a
//     failure names isTimeoutErr rather than only the flow around it.
//   - TestSubmitVersionDoesNotRecoverFromAFilesystemError drives the REAL
//     SubmitVersion and asserts what it DID (attempts, polls, error identity).
//     "An error came back" cannot see the defect — the buggy version also
//     returns an error, after three wasted polls and a misleading message.
//   - TestSubmitVersionStillRecoversFromARealTimeout is the POSITIVE CONTROL
//     battery. Without it, deleting the recovery branch outright would leave
//     both batteries above green.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/civitai/cli/pkg/civitai"
)

// ---------------------------------------------------------------------------
// The trap
// ---------------------------------------------------------------------------

// naivelyLooksLikeATimeout is the predicate isTimeoutErr USED to be. Rows assert
// against it so a fixture that stops exercising the trap cannot go green
// quietly — and, because it reports WHICH half matched, so the table can pin
// that the gate covers both lines rather than only the errors.As one.
func naivelyLooksLikeATimeout(err error) (viaOsIsTimeout, viaErrorsAs bool) {
	if err == nil {
		return false, false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return false, false
	}
	viaOsIsTimeout = os.IsTimeout(err)
	var netErr net.Error
	if errors.As(err, &netErr) {
		viaErrorsAs = netErr.Timeout()
	}
	return viaOsIsTimeout, viaErrorsAs
}

// TestOsIsTimeoutAndErrorsAsAreComplementaryHalves pins the two stdlib facts the
// gate's PLACEMENT rests on. If either changes, the rows built on it stop
// exercising what they claim and this fails HERE with an explanation.
func TestOsIsTimeoutAndErrorsAsAreComplementaryHalves(t *testing.T) {
	if _, ok := any(syscall.ETIMEDOUT).(net.Error); !ok {
		t.Fatal("syscall.Errno no longer satisfies net.Error.\n" +
			"GOOD NEWS, not a failure to silence: the hazard has left the standard library.\n" +
			"Re-measure before deleting the gate — os.IsTimeout's own unwrapping of\n" +
			"*fs.PathError/*os.LinkError/*os.SyscallError is a SEPARATE half and does not go away.")
	}
	direct := &fs.PathError{Op: "write", Path: "/cfg/config.yaml", Err: syscall.ETIMEDOUT}
	wrapped := fmt.Errorf("persist refreshed tokens: %w", direct)

	if !os.IsTimeout(direct) {
		t.Error("os.IsTimeout no longer unwraps a *fs.PathError to its errno — the DIRECT rows below\n" +
			"no longer exercise the os.IsTimeout half of the trap")
	}
	if os.IsTimeout(wrapped) {
		t.Error("os.IsTimeout now walks through a fmt.Errorf wrapper — the WRAPPED rows below no longer\n" +
			"isolate the errors.As half, so the table stops pinning the gate's placement")
	}
	var ne net.Error
	if !errors.As(wrapped, &ne) || !ne.Timeout() {
		t.Error("errors.As no longer reaches the errno under a fmt.Errorf wrapper — the WRAPPED rows\n" +
			"no longer exercise the trap at all")
	}
}

// ---------------------------------------------------------------------------
// The table
// ---------------------------------------------------------------------------

// half records which line of the OLD predicate a fixture was caught by, and the
// two values are DISJOINT by construction:
//
//   - halfOsIsTimeout: os.IsTimeout matched it, so a gate placed BELOW that line
//     leaves this row broken.
//   - halfErrorsAs: os.IsTimeout did NOT match and errors.As did, so this row is
//     evidence about the errors.As line ALONE.
//
// Both premises are asserted per row, which is what makes the table able to tell
// a complete fix from a gate placed above the wrong line.
type half int

const (
	halfNone half = iota // a CONTROL: neither line matched, correct at base
	halfOsIsTimeout
	halfErrorsAs
)

func (h half) String() string {
	switch h {
	case halfOsIsTimeout:
		return "os.IsTimeout"
	case halfErrorsAs:
		return "errors.As only"
	default:
		return "neither (control)"
	}
}

// fsErrCases is the shared corpus: the filesystem error shapes the submit path's
// TokenSource seam really produces, plus the errnos that were never mis-sorted.
func fsErrCases() []struct {
	name string
	err  error
	half half
} {
	pathErr := func(e syscall.Errno) error {
		return &fs.PathError{Op: "write", Path: "/cfg/civitai/config.yaml", Err: e}
	}
	return []struct {
		name string
		err  error
		half half
	}{
		// WRAPPED — the exact shape internal/auth/source.go:115 produces. Only
		// errors.As saw these.
		{"persist refreshed tokens: *fs.PathError(ETIMEDOUT)",
			fmt.Errorf("persist refreshed tokens: %w", pathErr(syscall.ETIMEDOUT)), halfErrorsAs},
		{"persist refreshed tokens: *fs.PathError(EAGAIN)",
			fmt.Errorf("persist refreshed tokens: %w", pathErr(syscall.EAGAIN)), halfErrorsAs},
		{"persist refreshed tokens: *fs.PathError(EWOULDBLOCK)",
			fmt.Errorf("persist refreshed tokens: %w", pathErr(syscall.EWOULDBLOCK)), halfErrorsAs},
		{"multi-error tree (prose, *fs.PathError(ETIMEDOUT))",
			errors.Join(errors.New("persisting rotated tokens"), pathErr(syscall.ETIMEDOUT)), halfErrorsAs},

		// DIRECT — os.IsTimeout matches these on its own, so a gate placed only
		// above errors.As leaves them broken.
		{"bare syscall.ETIMEDOUT", syscall.ETIMEDOUT, halfOsIsTimeout},
		{"*fs.PathError(ETIMEDOUT), unwrapped", pathErr(syscall.ETIMEDOUT), halfOsIsTimeout},
		{"*os.SyscallError(ETIMEDOUT)", os.NewSyscallError("fsync", syscall.ETIMEDOUT), halfOsIsTimeout},
		{"*os.LinkError(ETIMEDOUT)",
			&os.LinkError{Op: "rename", Old: "/cfg/.tmp", New: "/cfg/config.yaml", Err: syscall.ETIMEDOUT}, halfOsIsTimeout},

		// CONTROLS — Timeout() is false for these errnos, so they were sorted
		// correctly at base too. Invariant guards, not regression coverage.
		{"control: *fs.PathError(EACCES)", pathErr(syscall.EACCES), halfNone},
		{"control: *fs.PathError(ENOENT)", pathErr(syscall.ENOENT), halfNone},
		{"control: persist refreshed tokens: *fs.PathError(ENOSPC)",
			fmt.Errorf("persist refreshed tokens: %w", pathErr(syscall.ENOSPC)), halfNone},
	}
}

// TestIsTimeoutErrRejectsFilesystemErrors asserts the PREDICATE directly, so a
// regression names isTimeoutErr rather than only the flow.
//
// RED AT BASE (origin/main, 592a8a9): every non-control row returns true.
func TestIsTimeoutErrRejectsFilesystemErrors(t *testing.T) {
	cases := fsErrCases()

	// A count floor per half, so a trimmed table cannot leave one line of the
	// old predicate unpinned and still report a serene pass.
	var viaOs, viaAs int
	for _, tc := range cases {
		switch tc.half {
		case halfOsIsTimeout:
			viaOs++
		case halfErrorsAs:
			viaAs++
		}
	}
	if viaOs < 2 || viaAs < 3 {
		t.Fatalf("the table pins %d os.IsTimeout row(s) and %d errors.As row(s); want >= 2 and >= 3.\n"+
			"The old predicate was filesystem-broad through BOTH lines — a table covering only one\n"+
			"cannot tell a complete fix from a gate placed above the wrong line.", viaOs, viaAs)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Half one: the row's own premise. Which line of the OLD predicate
			// matched is what makes this row evidence about the gate's placement.
			gotOs, gotAs := naivelyLooksLikeATimeout(tc.err)
			switch tc.half {
			case halfOsIsTimeout:
				if !gotOs {
					t.Fatalf("os.IsTimeout no longer matches %T — this row no longer pins that the gate\n"+
						"sits ABOVE os.IsTimeout, which is the half a spelling-only fix misses", tc.err)
				}
			case halfErrorsAs:
				if !gotAs {
					t.Fatalf("the naive errors.As no longer reports a timeout for %T — this row no longer\n"+
						"exercises the trap", tc.err)
				}
				if gotOs {
					t.Fatalf("os.IsTimeout now ALSO matches %T — this row was the errors.As line's own\n"+
						"evidence, and it no longer isolates it", tc.err)
				}
			default:
				if gotOs || gotAs {
					t.Fatalf("control row %T is now matched by the old predicate (os=%v as=%v) — it moved\n"+
						"out of the control group and into the trap", tc.err, gotOs, gotAs)
				}
			}

			// Half two: the real predicate rejects it.
			if isTimeoutErr(tc.err) {
				t.Errorf("isTimeoutErr said TRUE for a filesystem failure (%T: %v).\n"+
					"Nothing was sent, so there is no request to have timed out: this routes the submit\n"+
					"into recoverTimedOutSubmit, which polls for a submission that never existed and then\n"+
					"reports that the upload \"may not have completed\".\n"+
					"The old predicate caught this via %s.", tc.err, tc.err, tc.half)
			}
			// The predicate must not touch the message.
			before := tc.err.Error()
			_ = isTimeoutErr(tc.err)
			if after := tc.err.Error(); after != before {
				t.Errorf("classification changed the message: %q -> %q", before, after)
			}
		})
	}
}

// TestIsTimeoutErrStillAcceptsRealTimeouts is the predicate-level positive
// control. Without it, `return false` would pass the whole table above.
//
// 🔴 INVARIANT GUARD — every row passes at base too. That is exactly its job.
func TestIsTimeoutErrStillAcceptsRealTimeouts(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"context.DeadlineExceeded", context.DeadlineExceeded},
		{"wrapped context.DeadlineExceeded", fmt.Errorf("submit: %w", context.DeadlineExceeded)},
		{"REAL http.Client.Timeout (*url.Error)", realClientTimeoutErr(t)},
		{"REAL read deadline exceeded (*net.OpError)", realReadDeadlineErr(t)},
		{"*net.OpError nesting *os.SyscallError(ETIMEDOUT)",
			&net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ETIMEDOUT)}},
		{"*net.DNSError (IsTimeout)",
			&net.DNSError{Err: "i/o timeout", Name: "civitai.com", IsTimeout: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !isTimeoutErr(tc.err) {
				t.Errorf("isTimeoutErr said FALSE for a genuine request timeout (%T: %v) — the filesystem\n"+
					"gate has disabled the recovery this predicate exists to trigger", tc.err, tc.err)
			}
		})
	}
	if isTimeoutErr(nil) {
		t.Error("nil is not a timeout")
	}
	if isTimeoutErr(errors.New("boom")) {
		t.Error("a plain error is not a timeout")
	}
}

// ---------------------------------------------------------------------------
// Harness: the REAL SubmitVersion, driven through the real TokenSource seam
// ---------------------------------------------------------------------------

// fsTokenSource is the seam the reachable defect arrives through: internal/auth's
// Token(ctx) returns `persist refreshed tokens: %w` when persisting rotated
// OAuth tokens fails, and authedDoWith surfaces that verbatim into
// SubmitVersion's isTimeoutErr arm.
type fsTokenSource struct {
	err   error
	calls int32
}

func (s *fsTokenSource) Token(context.Context) (string, error) {
	atomic.AddInt32(&s.calls, 1)
	return "", s.err
}

func (s *fsTokenSource) Refresh(context.Context) (string, error) { return "", civitai.ErrNoRefresh }

// submitObservation is what SubmitVersion DID. A returned error alone cannot
// tell a refusal from a recovery — the buggy version also returns an error.
type submitObservation struct {
	tokenCalls int
	submitPOST int
	listCalls  int
	err        error
	res        *SubmitResult
}

// countingSubmitServer answers the submit POST and the submissions GET, counting
// both, so "0 requests reached the wire" and "0 recovery polls" are MEASURED
// rather than assumed. hangPOST makes the POST block past the client's
// SubmitTimeout, producing a real http.Client.Timeout *url.Error.
func countingSubmitServer(t *testing.T, hangPOST bool, submissions func() []Submission) (*httptest.Server, *int32, *int32) {
	t.Helper()
	release := make(chan struct{})
	var posts, lists int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "submit-version"):
			atomic.AddInt32(&posts, 1)
			if hangPOST {
				<-release
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"publishRequestId": "pubreq_live"})
		case r.Method == http.MethodGet && r.URL.Path == SubmissionsPath:
			atomic.AddInt32(&lists, 1)
			_ = json.NewEncoder(w).Encode(map[string]any{"submissions": submissions()})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})
	return srv, &posts, &lists
}

// driveSubmitWithTokenError runs the REAL SubmitVersion with a TokenSource that
// fails with tokenErr, against a live server that counts what reached it.
func driveSubmitWithTokenError(t *testing.T, tokenErr error) submitObservation {
	t.Helper()
	srv, posts, lists := countingSubmitServer(t, false, func() []Submission {
		// A row that WOULD satisfy the recovery matcher, so a run that reaches
		// the poll is unmistakable: it returns a success rather than an error.
		return []Submission{{ID: "pubreq_recovered", BlockID: "my-block", Version: "0.2.0", Status: "pending"}}
	})

	src := &fsTokenSource{err: tokenErr}
	c := NewWithSource(srv.URL, src, "")
	zero := time.Duration(0)
	c.SubmitPollDelay = &zero

	res, err := c.SubmitVersion(context.Background(), []byte("ZIP"), "my-block", "0.2.0")
	return submitObservation{
		tokenCalls: int(atomic.LoadInt32(&src.calls)),
		submitPOST: int(atomic.LoadInt32(posts)),
		listCalls:  int(atomic.LoadInt32(lists)),
		err:        err,
		res:        res,
	}
}

// ---------------------------------------------------------------------------
// Regression: a filesystem error must never enter the recovery poll
// ---------------------------------------------------------------------------

// TestSubmitVersionDoesNotRecoverFromAFilesystemError is the regression coverage,
// through the real command path rather than the predicate alone.
//
// RED AT BASE (origin/main, 592a8a9): every non-control row observes
// tokenCalls=4 (one submit + three recovery polls, each re-asking the source)
// and comes back with the misleading timedOutSubmitError.
// GREEN AT HEAD: exactly one attempt, and the filesystem error itself.
func TestSubmitVersionDoesNotRecoverFromAFilesystemError(t *testing.T) {
	for _, tc := range fsErrCases() {
		t.Run(tc.name, func(t *testing.T) {
			got := driveSubmitWithTokenError(t, tc.err)

			if got.tokenCalls != 1 {
				t.Errorf("Token() was called %d times, want 1.\n"+
					"A filesystem failure was routed into recoverTimedOutSubmit: the CLI polled for a\n"+
					"submission that never existed, on an upload where ZERO bytes were sent.",
					got.tokenCalls)
			}
			if got.submitPOST != 0 {
				t.Errorf("%d submit POST(s) reached the server, want 0 — the failure is before the wire", got.submitPOST)
			}
			if got.listCalls != 0 {
				t.Errorf("%d recovery poll(s) of %s, want 0 — nothing was submitted, so there is nothing\n"+
					"to recover", got.listCalls, SubmissionsPath)
			}
			if got.res != nil {
				t.Errorf("SubmitVersion reported SUCCESS (%+v) for a failure that never left the machine —\n"+
					"the recovery poll matched an unrelated pending row", got.res)
			}
			// The error must be handed back untouched. Identity is the structural
			// form of "it was not re-wrapped as timedOutSubmitError", asserted
			// without reading any message: timedOutSubmitError interpolates its
			// cause with %v, so errors.Is cannot find it through one.
			if !errors.Is(got.err, tc.err) {
				t.Errorf("the returned error is not the one the TokenSource produced (%T: %v) — it was\n"+
					"re-wrapped, which only the post-timeout recovery path does", got.err, got.err)
			}
		})
	}
}

// TestSubmitVersionFilesystemErrorMessageIsNotTheTimeoutAdvice pins the
// USER-VISIBLE cost separately from the classification: the misleading sentence
// must not appear at all. Classification itself is asserted structurally above,
// per AGENTS.md item 7 — this row is about the advice, not the code path.
func TestSubmitVersionFilesystemErrorMessageIsNotTheTimeoutAdvice(t *testing.T) {
	tokenErr := fmt.Errorf("persist refreshed tokens: %w",
		&fs.PathError{Op: "write", Path: "/cfg/civitai/config.yaml", Err: syscall.ETIMEDOUT})
	got := driveSubmitWithTokenError(t, tokenErr)
	if got.err == nil {
		t.Fatal("a failing token source produced no error")
	}
	for _, forbidden := range []string{"may not have completed", "civitai app status"} {
		if strings.Contains(got.err.Error(), forbidden) {
			t.Errorf("the error tells the author %q about an upload that was never attempted.\n"+
				"full message: %s", forbidden, got.err)
		}
	}
	// Positive control on this assertion: the real timeout path DOES say it, so
	// a green above cannot mean the sentence was simply deleted from the product.
	if !strings.Contains(timedOutSubmitError("my-block", context.DeadlineExceeded).Error(), "may not have completed") {
		t.Fatal("timedOutSubmitError no longer carries the sentence this test asserts is absent —\n" +
			"the assertion above is now vacuous")
	}
}

// ---------------------------------------------------------------------------
// Positive controls: what MUST still recover
// ---------------------------------------------------------------------------

// TestSubmitVersionStillRecoversFromARealTimeout is the other half of the guard.
// Without it, deleting the recovery branch at SubmitVersion outright would leave
// every filesystem row above green.
//
// 🔴 INVARIANT GUARD — green at base. It drives a REAL http.Client.Timeout
// against a server that hangs the POST, because a constructed error proves
// nothing about what net/http hands SubmitVersion.
func TestSubmitVersionStillRecoversFromARealTimeout(t *testing.T) {
	landed := Submission{ID: "pubreq_01KVY45JGYSAXK7DKMQ19T2HGE", BlockID: "my-block", Version: "0.2.0", Status: "pending"}
	srv, posts, lists := countingSubmitServer(t, true, func() []Submission { return []Submission{landed} })

	c := New(srv.URL, "tok", "")
	c.SubmitTimeout = 50 * time.Millisecond
	zero := time.Duration(0)
	c.SubmitPollDelay = &zero

	res, err := c.SubmitVersion(context.Background(), []byte("ZIP"), "my-block", "0.2.0")
	if err != nil {
		t.Fatalf("a REAL request timeout must still recover, got error: %v", err)
	}
	if res == nil || res.PublishRequestID != landed.ID {
		t.Fatalf("recovered result = %+v, want the landed pubreq id %q", res, landed.ID)
	}
	if got := int(atomic.LoadInt32(posts)); got != 1 {
		t.Errorf("%d submit POST(s) reached the server, want 1 — the upload really was attempted", got)
	}
	if got := int(atomic.LoadInt32(lists)); got < 1 {
		t.Errorf("%d recovery poll(s), want >= 1 — the recovery branch is gone, so the filesystem rows\n"+
			"above would pass against a build that never recovers anything", got)
	}
}

// TestSubmitVersionStillRecoversFromARealTimeoutWithNothingLanded is the second
// positive control: the same real timeout with NO matching submission must burn
// the full poll budget and end in the actionable timeout error. It pins the
// EXHAUSTED branch, which the recovering control above never reaches.
//
// 🔴 INVARIANT GUARD — green at base.
func TestSubmitVersionStillRecoversFromARealTimeoutWithNothingLanded(t *testing.T) {
	srv, _, lists := countingSubmitServer(t, true, func() []Submission { return nil })

	c := New(srv.URL, "tok", "")
	c.SubmitTimeout = 50 * time.Millisecond
	zero := time.Duration(0)
	c.SubmitPollDelay = &zero

	_, err := c.SubmitVersion(context.Background(), []byte("ZIP"), "my-block", "0.2.0")
	if err == nil {
		t.Fatal("a real timeout with nothing landed produced no error")
	}
	if got := int(atomic.LoadInt32(lists)); got != submitPollAttempts {
		t.Errorf("%d recovery poll(s), want %d — the bounded poll is what distinguishes a lost response\n"+
			"from a failure that never reached the wire", got, submitPollAttempts)
	}
	if !strings.Contains(err.Error(), "may not have completed") {
		t.Errorf("a genuinely lost response must still carry the actionable advice, got: %v", err)
	}
}

// TestSubmitVersionRecoversFromADeadlineExceededTokenError is the positive
// control on the INJECTION POINT the regression uses. Without it, "no filesystem
// error recovers" would also be satisfied by never recovering from anything a
// TokenSource returns — and a ctx deadline really does arrive through that seam,
// because internal/auth's refresh makes its own HTTP call.
//
// The recovery poll re-enters the same failing source, so the observable is the
// ATTEMPT COUNT (1 submit + submitPollAttempts polls), not a server hit.
//
// 🔴 INVARIANT GUARD — green at base.
func TestSubmitVersionRecoversFromADeadlineExceededTokenError(t *testing.T) {
	got := driveSubmitWithTokenError(t, fmt.Errorf("refreshing tokens: %w", context.DeadlineExceeded))
	want := 1 + submitPollAttempts
	if got.tokenCalls != want {
		t.Errorf("Token() was called %d times, want %d (one submit + %d recovery polls) — a genuine\n"+
			"deadline must still enter the recovery poll", got.tokenCalls, want, submitPollAttempts)
	}
	if got.err == nil {
		t.Fatal("an exhausted recovery produced no error")
	}
}

// ---------------------------------------------------------------------------
// Real errors from the net stack
// ---------------------------------------------------------------------------

// realClientTimeoutErr produces the error net/http really returns when
// http.Client.Timeout fires: a *url.Error whose Timeout() is true. A constructed
// one proves nothing about what the client hands isTimeoutErr.
func realClientTimeoutErr(t *testing.T) error {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})

	c := &http.Client{Timeout: 50 * time.Millisecond}
	resp, err := c.Get(srv.URL + "/")
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("a hung handler answered inside the client timeout — no timeout error to test with")
	}
	var ne net.Error
	if !errors.As(err, &ne) || !ne.Timeout() {
		t.Fatalf("expected a timeout net.Error from http.Client.Timeout, got %T: %v", err, err)
	}
	return err
}

// realReadDeadlineErr produces a genuine timeout *net.OpError from the net stack:
// a live loopback connection whose read deadline has already passed. It cannot
// flake on a slow box or in a sandbox with no route out.
func realReadDeadlineErr(t *testing.T) error {
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
	return rerr
}
