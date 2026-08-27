package main

// A FILESYSTEM ERROR IS NOT A NETWORK ERROR.
//
// This file is the guard for issue #241. The defect it pins was not a wrong
// branch anyone wrote — it was an accidental interface satisfaction in the
// standard library that made the OBVIOUS spelling of isNetworkErr wrong:
//
//	var netErr net.Error
//	return errors.As(err, &netErr)
//
// `syscall.Errno` declares `Timeout() bool` and `Temporary() bool`, so it IS a
// `net.Error`; `*fs.PathError` / `*os.LinkError` / `*os.SyscallError` are not,
// so errors.As unwrapped past them and matched the Errno underneath. Every
// untagged os.ReadFile / os.Stat / os.MkdirAll failure in the CLI therefore
// exited 5 — the code the README tells scripts to RETRY on.
//
// The walk that fixes it now lives in pkg/civitai (transport_error.go), reached
// through civitai.IsTransportError, because the identical spelling was ALSO
// open-coded in that package's retry loop and only this copy got fixed — see
// issue #244. These tests drive the CLI's classifier; pkg/civitai's
// retry_fs_test.go drives the retry loop through the same predicate.
//
// Three tests, and NONE subsumes the others:
//
//   - TestSyscallErrnoStillSatisfiesNetError pins the TRAP itself, so a later
//     "simplify" that deletes the guard has to first delete a test whose only
//     job is to say the hazard is still in the stdlib.
//   - TestFilesystemErrorsAreNotNetworkErrors is the table over the real error
//     SHAPES. Every fixture asserts BOTH halves: the naive predicate still
//     matches (the trap is live for this shape) and the real classifier does
//     not (the guard covers it). Asserting only the second half would pass
//     just as well against a Go release that quietly stopped satisfying the
//     interface — a guard green for a reason that has nothing to do with the
//     guard.
//   - TestTransportErrorsStillExitFive is the POSITIVE CONTROL battery.
//     Without it, deleting exit 5 outright would leave the first two tests
//     green. It deliberately mixes REAL errors produced by the net stack
//     (a genuine refused dial, a genuine read deadline, a real 503-after-
//     retries through pkg/civitai) with constructed shapes, because a
//     constructed *net.OpError proves nothing about what net/http hands us.
//
// Classification is asserted through exitCode — the process contract — never
// through message text, per AGENTS.md item 7. Message PRESERVATION is asserted
// separately and explicitly: the fix touches only the classifier, so every
// fixture's Error() must be byte-identical to the error the OS produced.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/civitai/cli/pkg/civitai"
)

// ---------------------------------------------------------------------------
// The trap
// ---------------------------------------------------------------------------

// naivelyLooksLikeANetError is the predicate isNetworkErr USED to end with. It
// exists so the tests below can assert, per fixture, that the hazard is still
// real — a fixture the naive predicate no longer matches is a fixture that
// proves nothing about the guard.
func naivelyLooksLikeANetError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr)
}

// TestSyscallErrnoStillSatisfiesNetError pins the stdlib fact the whole guard
// is built on, and the three NEGATIVE facts that make it bite.
//
// 🔴 Do not "simplify" civitai.IsTransportError back to a bare errors.As because
// the wrapper types look like they would stop it. They do not: measured on
// go1.25.12, NONE of *fs.PathError, *os.LinkError or *os.SyscallError declares
// Temporary(), so none of them satisfies net.Error and errors.As walks
// straight through to the Errno. That asymmetry is the entire bug.
func TestSyscallErrnoStillSatisfiesNetError(t *testing.T) {
	if _, ok := any(syscall.EACCES).(net.Error); !ok {
		t.Fatal("syscall.Errno no longer satisfies net.Error.\n" +
			"That is GOOD NEWS, not a test failure to silence: the hazard this file guards has\n" +
			"left the standard library. Re-measure before removing anything — the Errno skip in\n" +
			"pkg/civitai's transportError becomes dead code, but the *fs.PathError/*os.LinkError terminators\n" +
			"do NOT, they guard the mirror-image hazard of a wrapper GAINING Temporary().")
	}

	// The negative half: the wrappers do not satisfy it, which is why the walk
	// reaches the Errno at all.
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"*fs.PathError", &fs.PathError{Op: "open", Path: "/x", Err: syscall.EACCES}},
		{"*os.LinkError", &os.LinkError{Op: "rename", Old: "/a", New: "/b", Err: syscall.EACCES}},
		{"*os.SyscallError", os.NewSyscallError("read", syscall.EACCES)},
	} {
		if _, ok := tc.err.(net.Error); ok {
			t.Errorf("%s now satisfies net.Error directly. The Errno skip in transportError no longer\n"+
				"covers this shape — the wrapper itself matches. Verify the terminator list still\n"+
				"stops it (it covers *fs.PathError and *os.LinkError, NOT *os.SyscallError).", tc.name)
		}
	}
}

// ---------------------------------------------------------------------------
// The table
// ---------------------------------------------------------------------------

// fsFixture is one real filesystem failure plus the errno it must carry.
type fsFixture struct {
	name  string
	err   error
	errno syscall.Errno
}

// realFilesystemErrors produces the error SHAPES the CLI's ~80 filesystem call
// sites actually return, from real syscalls wherever the OS will cooperate.
//
// It is deliberately not a list of hand-built &fs.PathError{} literals: the
// defect was about what os.ReadFile/os.Stat/os.MkdirAll REALLY hand back, and a
// literal would still pass if the os package started wrapping differently.
func realFilesystemErrors(t *testing.T) []fsFixture {
	t.Helper()
	dir := t.TempDir()

	regular := filepath.Join(dir, "regular.png")
	if err := os.WriteFile(regular, []byte("not really a png"), 0o600); err != nil {
		t.Fatal(err)
	}
	unreadable := filepath.Join(dir, "unreadable.png")
	if err := os.WriteFile(unreadable, []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	sealed := filepath.Join(dir, "sealed")
	if err := os.Mkdir(sealed, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sealed, 0o700) })

	out := []fsFixture{}
	add := func(name string, err error, errno syscall.Errno) {
		if err == nil {
			t.Fatalf("fixture %q produced no error — it cannot prove anything about a failure path", name)
		}
		out = append(out, fsFixture{name, err, errno})
	}

	// *fs.PathError, from three different os entry points.
	_, err := os.Open(filepath.Join(dir, "definitely-absent.png"))
	add("os.Open ENOENT (*fs.PathError)", err, syscall.ENOENT)

	_, err = os.Stat(filepath.Join(regular, "under-a-regular-file"))
	add("os.Stat ENOTDIR (*fs.PathError)", err, syscall.ENOTDIR)

	_, err = os.ReadFile(dir)
	add("os.ReadFile EISDIR (*fs.PathError)", err, syscall.EISDIR)

	// EACCES is the headline case: a present-but-unreadable file. Root defeats
	// mode 000, so skip rather than assert something the OS is not doing.
	if os.Geteuid() != 0 {
		_, err = os.ReadFile(unreadable)
		add("os.ReadFile EACCES (*fs.PathError)", err, syscall.EACCES)

		err = os.MkdirAll(filepath.Join(sealed, "civitai"), 0o700)
		add("os.MkdirAll EACCES (*fs.PathError)", err, syscall.EACCES)
	}

	// *os.LinkError, from two different os entry points.
	err = os.Rename(filepath.Join(dir, "definitely-absent.png"), filepath.Join(dir, "dst.png"))
	add("os.Rename ENOENT (*os.LinkError)", err, syscall.ENOENT)

	err = os.Symlink(regular, regular)
	add("os.Symlink EEXIST (*os.LinkError)", err, syscall.EEXIST)

	// *os.SyscallError. os produces these through os.NewSyscallError for the
	// operations that are not about a path (Pipe, Hostname, …); constructing one
	// is the same value those return, and the point of the row is the SHAPE.
	add("os.NewSyscallError EACCES (*os.SyscallError)", os.NewSyscallError("read", syscall.EACCES), syscall.EACCES)

	// Bare errnos — nothing wrapping them at all.
	for _, e := range []syscall.Errno{syscall.ENOENT, syscall.EACCES, syscall.ENOTDIR, syscall.EISDIR} {
		add(fmt.Sprintf("bare syscall.Errno %s", e), e, e)
	}
	return out
}

// fsFixtureFloor is the KEEPER for realFilesystemErrors: the fixture NAMES that
// must always be produced, whatever else the table gains.
//
// 🔴 A COUNT WAS NOT ENOUGH, AND THE FIXTURES IT PROTECTS ARE NOT
// INTERCHANGEABLE. This started as `len(fixtures) < 8`, reasoning that the table
// only grows so any deletion drops below the floor. Add-one/delete-one defeats
// it in one move: delete `os.Symlink EEXIST (*os.LinkError)` and add any
// unrelated row — a fifth bare errno, say — and the count is still above 8, the
// guard is green, and `*os.LinkError` has lost half its coverage.
//
// The trade matters here more than a row count suggests, because the fixtures
// are not samples of one thing: each pins a distinct WRAPPING SHAPE
// (`*fs.PathError`, `*os.LinkError`, `*os.SyscallError`, bare `syscall.Errno`),
// and the defect in issue #241 was precisely that `errors.As` unwrapped PAST
// some wrappers and not others. A table that keeps its size while collapsing
// onto one wrapper still reports "12 fixtures, all green" while the shape that
// actually regressed is no longer represented. Membership cannot be traded that
// way: removing a NAME from this list, or its `add(...)` call, is red by name.
//
// 🔴 THREE RESIDUALS, STATED RATHER THAN GLOSSED. (1) The two EACCES fixtures
// are DELIBERATELY ABSENT from this floor: realFilesystemErrors skips them under
// `os.Geteuid() == 0` because root defeats mode 000, so naming them here would
// make the guard red in a root container rather than catching anything. Their
// wrapping shape (`*fs.PathError`) is held by three other named fixtures, so the
// omission costs no shape coverage — but it does mean the EACCES rows themselves
// stay tradeable. (2) Protection is OPT-IN PER FIXTURE: the loop iterates THIS
// list, so a fixture added and not named here is permanently tradeable, and
// nothing goes red at the moment of the omission. Append the name in the same
// commit. (3) The two-line bypass is still open: delete the floor entry AND the
// `add(...)` call together. What this stops is the ONE-line trade — a deletion
// paid for by an unrelated addition; it does not stop a deliberate two-line
// removal, and no in-tree check can. That is review's job.
//
// The bare-errno names are BUILT the way realFilesystemErrors builds them rather
// than typed out, because `%s` on a syscall.Errno renders the OS's message text
// ("no such file or directory"), which is not portable across GOOS. The identity
// being pinned is the ERRNO, not the English string.
var fsFixtureFloor = []string{
	// *fs.PathError, from three unconditional os entry points.
	"os.Open ENOENT (*fs.PathError)",
	"os.Stat ENOTDIR (*fs.PathError)",
	"os.ReadFile EISDIR (*fs.PathError)",
	// *os.LinkError, from two.
	"os.Rename ENOENT (*os.LinkError)",
	"os.Symlink EEXIST (*os.LinkError)",
	// *os.SyscallError.
	"os.NewSyscallError EACCES (*os.SyscallError)",
	// Bare errnos — nothing wrapping them at all.
	fmt.Sprintf("bare syscall.Errno %s", syscall.ENOENT),
	fmt.Sprintf("bare syscall.Errno %s", syscall.EACCES),
	fmt.Sprintf("bare syscall.Errno %s", syscall.ENOTDIR),
	fmt.Sprintf("bare syscall.Errno %s", syscall.EISDIR),
}

// TestFilesystemErrorsAreNotNetworkErrors is the regression coverage.
//
// RED AT BASE (origin/main): every subtest fails with exit 5.
// GREEN AT HEAD: every subtest gets exit 1.
func TestFilesystemErrorsAreNotNetworkErrors(t *testing.T) {
	fixtures := realFilesystemErrors(t)

	have := map[string]bool{}
	for _, f := range fixtures {
		have[f.name] = true
	}
	// POSITIVE CONTROL: an empty floor would make every check below vacuous.
	if len(fsFixtureFloor) == 0 {
		t.Fatal("CONTROL failure: fsFixtureFloor is empty, so this test asserts nothing")
	}
	for _, name := range fsFixtureFloor {
		if !have[name] {
			t.Errorf("the %q fixture is gone.\n"+
				"Each fixture pins a distinct WRAPPING SHAPE that must not be classified as a network "+
				"error (AGENTS.md items 7 and 24); deleting one drops that shape's coverage. The old "+
				"guard was a COUNT (`len(fixtures) < 8`), so deleting this fixture while adding any "+
				"unrelated one kept the count above 8 and stayed green.\n"+
				"Adding fixtures is free; this list only forbids REMOVING one.", name)
		}
	}

	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			if !errors.Is(f.err, f.errno) {
				t.Fatalf("fixture does not carry %v (got %v) — it is testing something other than the errno path", f.errno, f.err)
			}

			// Half one: the trap is LIVE for this shape. Without this the row
			// could go green because the stdlib changed, not because we did.
			if !naivelyLooksLikeANetError(f.err) {
				t.Fatalf("the naive `errors.As(err, &net.Error)` no longer matches %T — this row no longer\n"+
					"exercises the trap, so its green says nothing about hasTransportError", f.err)
			}

			// Half two: the real classifier rejects it.
			if isNetworkErr(f.err) {
				t.Errorf("isNetworkErr said TRUE for a filesystem failure (%T: %v)", f.err, f.err)
			}
			if got := exitCode(f.err); got != exitGeneric {
				t.Errorf("exitCode = %d, want %d (generic).\n"+
					"%d is the code the README tells scripts to RETRY on; a permissions or I/O\n"+
					"problem never fixes itself, so a retry loop on it does not terminate.", got, exitGeneric, got)
			}

			// The classifier must not touch the message. Compare against the
			// error's own text captured before classification.
			before := f.err.Error()
			_ = exitCode(f.err)
			if after := f.err.Error(); after != before {
				t.Errorf("classification changed the message: %q -> %q", before, after)
			}

			// Wrapped exactly as the CLI wraps it (`--image %s: %w`,
			// `writing config: %w`, …) must classify identically.
			wrapped := fmt.Errorf("--image %s: %w", "icon.png", f.err)
			if got := exitCode(wrapped); got != exitGeneric {
				t.Errorf("wrapped: exitCode = %d, want %d", got, exitGeneric)
			}
		})
	}
}

// TestFilesystemErrorUnderAMultiErrorTree pins that the walk handles the
// `Unwrap() []error` shape — the shape internal/cmd's usageError and
// civitai.Tag both use — rather than only the single-unwrap chain.
//
// It runs BOTH directions from one tree shape, which is the point: an
// implementation that stops walking at the first Errno would pass the negative
// row and fail the positive one, and an implementation that only handles
// `Unwrap() error` would fail the positive row too.
func TestFilesystemErrorUnderAMultiErrorTree(t *testing.T) {
	fsErr := &fs.PathError{Op: "open", Path: "/x/icon.png", Err: syscall.EACCES}

	if got := exitCode(multiErr{errs: []error{errors.New("context"), fsErr}}); got != exitGeneric {
		t.Errorf("multi-error tree of (prose, filesystem): exitCode = %d, want %d", got, exitGeneric)
	}
	// A genuine net.Error as a SIBLING of the errno must still be found — the
	// walk skips an Errno, it does not stop at one.
	opErr := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("boom")}
	if got := exitCode(multiErr{errs: []error{syscall.EACCES, opErr}}); got != exitNetwork {
		t.Errorf("multi-error tree of (errno, *net.OpError): exitCode = %d, want %d", got, exitNetwork)
	}
}

// multiErr is the `Unwrap() []error` shape (what errors.Join and the CLI's own
// tagging produce).
type multiErr struct{ errs []error }

func (m multiErr) Error() string   { return "joined" }
func (m multiErr) Unwrap() []error { return m.errs }

// TestPathErrorTerminatesTheWalk pins the SECOND rule in transportError,
// which the Errno skip cannot cover: an error ABOUT A PATH is a filesystem
// error, and nothing beneath it is transport evidence.
//
// It is written against a fake net.Error rather than an errno precisely so it
// fails if the terminator is deleted — with an errno inside, the Errno skip
// would carry the row and the deletion would survive. Today the terminator is
// belt-and-braces (real PathErrors bottom out in an Errno); it is what keeps a
// future Go release that adds Temporary() to *fs.PathError from silently
// re-opening the bug.
func TestPathErrorTerminatesTheWalk(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"*fs.PathError", &fs.PathError{Op: "read", Path: "/x", Err: fakeNetErr{timeout: true}}},
		{"*os.LinkError", &os.LinkError{Op: "rename", Old: "/a", New: "/b", Err: fakeNetErr{timeout: true}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if isNetworkErr(tc.err) {
				t.Errorf("a %s is an error about a PATH — nothing inside it may classify as transport", tc.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Positive controls
// ---------------------------------------------------------------------------

// TestTransportErrorsStillExitFive is the other half of the guard. Every row
// here must be exit 5 AFTER the fix; without them, a "fix" that disabled the
// network branch entirely would leave the whole file above green.
//
// 🔴 INVARIANT GUARD, not regression coverage — every row passes at base too.
// That is exactly its job.
//
// 🔴 viaNetErrorBranch IS THE BACKSTOP, AND IT USED TO REST ON ONE ROW. The
// mutation that matters here is re-spelling transportError's Errno skip as
// `errors.As(ne, &errno)` — precisely the mistake the doc comment warns about,
// because errors.As unwraps a *net.OpError down to its errno and then rejects
// the OpError. Measured before this file was widened, that mutant reddened
// exactly ONE subtest (`*net.OpError nesting *os.SyscallError(ECONNRESET)`);
// the real refused dial survived it because the ECONNREFUSED sentinel carried
// the row. Deleting or reshaping that single row would have made the whole
// regression invisible. So: the real refused dial now ALSO asserts the walk saw
// it (its exit 5 still comes from the sentinel — the two assertions are
// independent), and two sentinel-free *net.OpError rows were added, whose exit
// 5 has no sentinel to fall back on at all.
func TestTransportErrorsStillExitFive(t *testing.T) {
	realRefused := realRefusedDial(t)
	realDeadline := realReadDeadlineExceeded(t)

	cases := []struct {
		name string
		err  error
		// viaNetErrorBranch requires the row to be classified by the net.Error
		// walk rather than by one of the three explicit sentinel checks. Without
		// it, gutting the walk would leave rows green on the sentinels alone.
		viaNetErrorBranch bool
		// sentinelFree additionally requires that NO sentinel could have carried
		// the row, so its exit-5 assertion is itself evidence about the walk.
		// Asserted, not just declared — see the check below.
		sentinelFree bool
	}{
		{"REAL refused dial (*net.OpError from net.Dial)", realRefused, true, false},
		{"REAL read deadline exceeded (*net.OpError from conn.Read)", realDeadline, true, true},
		{"REAL 503 after retries (pkg/civitai read GET)", real503AfterRetries(t), false, false},

		{"bare syscall.ECONNREFUSED", syscall.ECONNREFUSED, false, false},
		{"bare syscall.ECONNRESET", syscall.ECONNRESET, false, false},
		{"wrapped syscall.ECONNREFUSED", fmt.Errorf("dial: %w", syscall.ECONNREFUSED), false, false},
		{"context.DeadlineExceeded", fmt.Errorf("request: %w", context.DeadlineExceeded), false, false},

		{"*net.OpError nesting *os.SyscallError(ECONNRESET)",
			&net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)}, true, false},
		{"*net.OpError nesting *os.SyscallError(ETIMEDOUT)",
			&net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ETIMEDOUT)}, true, true},
		{"*net.OpError nesting *os.SyscallError(EHOSTUNREACH)",
			&net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.EHOSTUNREACH)}, true, true},
		{"*net.DNSError", &net.DNSError{Err: "no such host", Name: "civitai.com", IsNotFound: true}, true, true},
		{"*url.Error wrapping *net.OpError",
			&url.Error{Op: "Get", URL: "https://civitai.com/api/v1/models",
				Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("i/o timeout")}}, true, true},
		{"civitai.ErrNetwork sentinel", civitai.ErrNetwork, false, false},
	}

	// A count floor, so a table someone trimmed cannot report a serene pass with
	// the backstop back down to one row.
	var sentinelFreeWalkRows int
	for _, tc := range cases {
		if tc.viaNetErrorBranch && tc.sentinelFree {
			sentinelFreeWalkRows++
		}
	}
	if sentinelFreeWalkRows < 4 {
		t.Fatalf("only %d row(s) pin the walk with no sentinel to fall back on; want >= 4.\n"+
			"This backstop was ONE row once, and the errors.As mutant it exists to catch was\n"+
			"one row-deletion away from invisible.", sentinelFreeWalkRows)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil {
				t.Fatal("nil fixture — this row proves nothing")
			}
			if got := exitCode(tc.err); got != exitNetwork {
				t.Errorf("exitCode = %d, want %d (network).\n"+
					"A real transport failure must stay retryable — the filesystem fix must not\n"+
					"have disabled exit 5. err = %#v", got, exitNetwork, tc.err)
			}
			if tc.viaNetErrorBranch && !civitai.IsTransportError(tc.err) {
				t.Errorf("this row is meant to be classified by the net.Error walk, but\n" +
					"civitai.IsTransportError said false — it is passing on a sentinel shortcut\n" +
					"instead, so it cannot see a regression in the walk itself")
			}
			if tc.sentinelFree {
				// The row's own premise. A "sentinel-free" row that quietly
				// gained a sentinel would stop being evidence about the walk.
				for _, s := range []error{context.DeadlineExceeded, syscall.ECONNREFUSED, syscall.ECONNRESET} {
					if errors.Is(tc.err, s) {
						t.Fatalf("row is marked sentinel-free but errors.Is finds %v — its exit 5 no longer\n"+
							"depends on the walk, so it cannot back the errors.As mutation up", s)
					}
				}
			}
		})
	}
}

// realRefusedDial produces a genuine ECONNREFUSED by dialling a port that was
// bound and then released — no constructed error, no outbound network.
func realRefusedDial(t *testing.T) error {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not bind a loopback listener: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("could not release the listener: %v", err)
	}
	conn, derr := net.DialTimeout("tcp", addr, 5*time.Second)
	if derr == nil {
		_ = conn.Close()
		t.Skipf("something else grabbed %s between close and dial — the refusal is not observable here", addr)
	}
	var op *net.OpError
	if !errors.As(derr, &op) {
		t.Fatalf("expected a *net.OpError from a refused dial, got %T: %v", derr, derr)
	}
	return derr
}

// realReadDeadlineExceeded produces a genuine timeout *net.OpError from the net
// stack: a live loopback connection whose read deadline has already passed.
// This is the "real dial timeout" control in a form that cannot flake on a
// slow box or a sandbox with no route out.
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

// real503AfterRetries drives the REAL pkg/civitai read path against a server
// that only ever answers 503, so the returned error is the one a user gets from
// an overloaded backend — tagged civitai.ErrNetwork by retryExhaustedError.
// A constructed civitai.ErrNetwork could not tell us the tagging still happens.
func real503AfterRetries(t *testing.T) error {
	t.Helper()
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"upstream unavailable"}`))
	}))
	t.Cleanup(srv.Close)

	zero := time.Duration(0)
	c := &civitai.Client{
		BaseURL:          srv.URL,
		Tokens:           civitai.StaticToken(""),
		HTTP:             srv.Client(),
		RetryBackoffBase: &zero,
		Stderr:           discardWriter{},
	}
	_, err := c.SearchModels(context.Background(), url.Values{})
	if err == nil {
		t.Fatal("a server answering only 503 produced no error")
	}
	// Positive control on the control: the retry loop really ran, so this is
	// "after retries" and not a single-shot failure that happens to look alike.
	if hits < 2 {
		t.Fatalf("the retry loop did not run — only %d request(s) reached the server", hits)
	}
	return err
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// ---------------------------------------------------------------------------
// End to end, against the real binary
// ---------------------------------------------------------------------------

// TestFilesystemErrorsExitGenericEndToEnd is the reproduction of the reported
// symptom, through the actual process exit status rather than through
// exitCode() in-process. exitCode() unit tests cannot see a command that never
// reaches the classifier, or a RunE that tags the error on the way out.
//
// RED AT BASE (origin/main): every case exits 5.
// GREEN AT HEAD: every case exits 1.
//
// The two controls at the end are what stop this from being a test that would
// pass against a binary hard-wired to exit 1.
func TestFilesystemErrorsExitGenericEndToEnd(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: mode-000 fixtures are still readable, so the symptom is not observable")
	}
	bin := buildCLI(t)
	dir := t.TempDir()

	// A present-but-unreadable file.
	unreadable := filepath.Join(dir, "icon.png")
	if err := os.WriteFile(unreadable, []byte("bytes that will never be read"), 0o000); err != nil {
		t.Fatal(err)
	}
	// A regular file, so any path BELOW it is ENOTDIR.
	notADir := filepath.Join(dir, "notadir")
	if err := os.WriteFile(notADir, []byte("regular file"), 0o600); err != nil {
		t.Fatal(err)
	}
	// An unwritable config root, so the config write cannot mkdir.
	sealed := filepath.Join(dir, "sealed")
	if err := os.Mkdir(sealed, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sealed, 0o700) })

	writableConfig := filepath.Join(dir, "config")
	// Every subprocess talks to a port nothing is listening on, so a case that
	// wrongly proceeds to the network fails loudly instead of reaching civitai.com.
	deadURL := "http://" + closedLoopbackAddr(t)

	cases := []struct {
		name string
		env  []string
		args []string
		// reason is the OS's own text for the failure. Asserting it is a
		// MESSAGE-PRESERVATION check, not a classification check: the exit code
		// is asserted separately and structurally, per AGENTS.md item 7.
		reason string
	}{
		{
			name:   "app listing set-icon (unreadable file)",
			args:   []string{"app", "listing", "set-icon", unreadable, "--slug", "demo-app"},
			reason: "permission denied",
		},
		{
			name:   "app listing set-cover (unreadable file)",
			args:   []string{"app", "listing", "set-cover", unreadable, "--slug", "demo-app"},
			reason: "permission denied",
		},
		{
			name:   "app listing add-screenshot (unreadable file)",
			args:   []string{"app", "listing", "add-screenshot", unreadable, "--slug", "demo-app"},
			reason: "permission denied",
		},
		{
			name:   "generate --image (unreadable file)",
			args:   []string{"generate", "make it winter", "--ecosystem", "Qwen", "--image", unreadable, "--dry-run"},
			reason: "permission denied",
		},
		{
			name:   "app validate (ENOTDIR)",
			args:   []string{"app", "validate", filepath.Join(notADir, "civitai-app.json")},
			reason: "not a directory",
		},
		{
			name:   "login (unwritable config dir)",
			env:    []string{"XDG_CONFIG_HOME=" + filepath.Join(sealed, "config"), "CIVITAI_TOKEN="},
			args:   []string{"login", "--token", "abc123"},
			reason: "permission denied",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := append([]string{
				"HOME=" + dir,
				"XDG_CONFIG_HOME=" + writableConfig,
				"CIVITAI_TOKEN=abc123",
				"CIVITAI_BASE_URL=" + deadURL,
				"NO_COLOR=1",
			}, tc.env...)

			rc, stderr := runCLI(t, bin, env, tc.args...)
			if rc != exitGeneric {
				t.Errorf("exit status = %d, want %d (generic).\n"+
					"%d is what this reported as before the fix, and %d is the code the README\n"+
					"tells scripts to RETRY on — a permissions or I/O problem never clears.\n"+
					"stderr: %s", rc, exitGeneric, exitNetwork, exitNetwork, stderr)
			}
			// The fix must be invisible in the output.
			if !strings.Contains(stderr, tc.reason) {
				t.Errorf("the OS's own reason %q is gone from the message — classification must\n"+
					"never alter what the user reads.\nstderr: %s", tc.reason, stderr)
			}
		})
	}

	// 🔴 POSITIVE CONTROLS. Without these, a binary that returned 1 for
	// everything would pass every row above.
	t.Run("control: a real transport failure still exits 5", func(t *testing.T) {
		rc, stderr := runCLI(t, bin, []string{
			"HOME=" + dir,
			"XDG_CONFIG_HOME=" + writableConfig,
			"CIVITAI_TOKEN=abc123",
			"CIVITAI_BASE_URL=" + deadURL,
			"NO_COLOR=1",
		}, "models", "search", "--limit", "1")
		if rc != exitNetwork {
			t.Errorf("a refused dial exited %d, want %d — the fix disabled exit 5, or this\n"+
				"harness cannot observe it at all, in which case the rows above prove nothing.\n"+
				"stderr: %s", rc, exitNetwork, stderr)
		}
	})

	t.Run("control: a usage mistake still exits 2", func(t *testing.T) {
		rc, stderr := runCLI(t, bin, []string{"HOME=" + dir, "NO_COLOR=1"}, "models", "search", "--limit", "99999")
		if rc != exitUsage {
			t.Errorf("an out-of-range --limit exited %d, want %d — the harness is not observing\n"+
				"differentiated codes.\nstderr: %s", rc, exitUsage, stderr)
		}
	})
}

// buildCLI compiles the binary under test into a temp dir and returns its path.
// It builds THIS package's source, so the test can never be measuring a stale
// ./bin/civitai from an earlier tree.
func buildCLI(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "civitai")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the CLI failed: %v\n%s", err, out)
	}
	return bin
}

// runCLI runs the built binary with a CLEAN environment plus env, and returns
// its exit status and stderr. The environment is replaced rather than extended
// so an ambient CIVITAI_TOKEN / XDG_CONFIG_HOME on the developer's machine
// cannot change what the subprocess does.
func runCLI(t *testing.T, bin string, env []string, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH")}, env...)
	var stderr, stdout bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout

	err := cmd.Run()
	if err == nil {
		t.Fatalf("`civitai %s` SUCCEEDED — it must fail for the exit code to mean anything.\nstdout: %s", strings.Join(args, " "), stdout.String())
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("could not run the binary: %v", err)
	}
	return ee.ExitCode(), strings.TrimSpace(stderr.String())
}

// closedLoopbackAddr returns a loopback host:port that was bound and released,
// so connecting to it is refused rather than routed anywhere real.
func closedLoopbackAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not bind a loopback listener: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("could not release the listener: %v", err)
	}
	return addr
}
