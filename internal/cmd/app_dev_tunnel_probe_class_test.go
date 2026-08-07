package cmd

// A FILESYSTEM ERROR IS NOT A PROBE TIMEOUT.
//
// This file is the dev-tunnel half of the guard AGENTS.md item 24 describes —
// issue #246, alongside internal/appapi/submit_fs_not_timeout_test.go.
// classifyProbeErr used to end
//
//	var nerr net.Error
//	if errors.As(err, &nerr) && nerr.Timeout() { return "timeout" }
//	return "unreachable"
//
// and `syscall.Errno.Timeout()` is TRUE for ETIMEDOUT, EAGAIN and EWOULDBLOCK,
// so errors.As walked past a filesystem wrapper and read the errno underneath.
//
// The tag is advisory output, which is precisely why it matters: it is the only
// thing telling an author whether to WAIT (DNS propagation, a slow route) or to
// go looking. Manufacturing "timeout" for a problem on their own disk is the
// false-advice failure AGENTS.md item 10 spent four measured corrections
// avoiding.
//
// 🔴 REACHABILITY IS MEASURED, AND THE CLAIM IS KEPT AT THE MEASURED SIZE. A
// filesystem error DOES reach classifyProbeErr: probePublicURL and
// probeLocalHopURL pass whatever client.Do returns on an https URL, and the x509
// system-roots load surfaces an unreadable CA bundle as x509.SystemRootsError
// wrapping an *fs.PathError (reproduced live against an httptest TLS server with
// SSL_CERT_FILE at a mode-000 bundle). But today's outermost shape does NOT
// mislabel, for a reason two layers away from this function: url.Error.Timeout()
// TYPE-ASSERTS on its immediate .Err rather than unwrapping, so the errno is
// never consulted. Measured with an ETIMEDOUT bundle — bare
// x509.SystemRootsError tags "timeout", *tls.CertificateVerificationError around
// it tags "timeout", and only the outer *url.Error drops it to "unreachable".
// The gate makes the right answer structural instead of accidental; the rows
// below are labelled with which of those they are, so nobody reads an invariant
// guard as regression coverage.
//
// Two batteries, and neither subsumes the other: the filesystem table, and the
// transport table that would stay green if the fix simply deleted "timeout".

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"syscall"
	"testing"
	"time"
)

// naivelyLooksLikeAProbeTimeout is the branch classifyProbeErr USED to end with.
// Rows assert against it so a fixture that stopped exercising the trap cannot go
// green quietly.
func naivelyLooksLikeAProbeTimeout(err error) bool {
	var nerr net.Error
	return errors.As(err, &nerr) && nerr.Timeout()
}

// TestClassifyProbeErrRejectsFilesystemErrors is the filesystem table.
//
// trapped marks a row the OLD spelling mislabelled "timeout" — RED AT BASE
// (origin/main, 592a8a9). The others are CONTROLS: correct at base, kept so a
// future widening that starts mislabelling them fails loudly.
func TestClassifyProbeErrRejectsFilesystemErrors(t *testing.T) {
	caBundle := func(e syscall.Errno) error {
		return &fs.PathError{Op: "open", Path: "/etc/ssl/certs/ca-bundle.crt", Err: e}
	}
	// The real chain, rebuilt from its own types: url.Error → tls verification
	// error → x509.SystemRootsError → *fs.PathError → errno. It is constructed
	// rather than produced by SSL_CERT_FILE so the suite carries no dependency on
	// process-wide env or on the host's cert layout.
	systemRoots := func(e syscall.Errno) error { return x509.SystemRootsError{Err: caBundle(e)} }

	cases := []struct {
		name string
		err  error
		// trapped: the OLD predicate said "timeout" for this row.
		trapped bool
	}{
		// The reachable shape, at each layer of the real chain.
		{"x509.SystemRootsError(*fs.PathError ETIMEDOUT)", systemRoots(syscall.ETIMEDOUT), true},
		{"x509.SystemRootsError(*fs.PathError EAGAIN)", systemRoots(syscall.EAGAIN), true},
		{"tls.CertificateVerificationError(x509.SystemRootsError ETIMEDOUT)",
			&tls.CertificateVerificationError{Err: systemRoots(syscall.ETIMEDOUT)}, true},

		// Other filesystem shapes an error tree reaching here can carry.
		{"*fs.PathError(ETIMEDOUT)", caBundle(syscall.ETIMEDOUT), true},
		{"wrapped *fs.PathError(ETIMEDOUT)", fmt.Errorf("loading roots: %w", caBundle(syscall.ETIMEDOUT)), true},
		{"*os.SyscallError(ETIMEDOUT)", os.NewSyscallError("read", syscall.ETIMEDOUT), true},
		{"*os.LinkError(EWOULDBLOCK)",
			&os.LinkError{Op: "rename", Old: "/a", New: "/b", Err: syscall.EWOULDBLOCK}, true},
		{"bare syscall.ETIMEDOUT", syscall.ETIMEDOUT, true},
		{"multi-error tree (prose, *fs.PathError ETIMEDOUT)",
			errors.Join(errors.New("loading roots"), caBundle(syscall.ETIMEDOUT)), true},

		// CONTROLS — Timeout() is false for these errnos, so they tagged
		// "unreachable" at base too. Invariant guards, not regression coverage.
		{"control: x509.SystemRootsError(*fs.PathError EACCES)", systemRoots(syscall.EACCES), false},
		{"control: *fs.PathError(ENOENT)", caBundle(syscall.ENOENT), false},
		{"control: bare syscall.EIO", syscall.EIO, false},

		// The LIVE outermost shape. It tagged "unreachable" at base too — not
		// because classifyProbeErr was right, but because url.Error.Timeout()
		// type-asserts on its immediate .Err. Recorded as the invariant guard it
		// is, so nobody counts it as the regression.
		{"control (shielded by url.Error): *url.Error(tls(x509(*fs.PathError ETIMEDOUT)))",
			&url.Error{Op: "Get", URL: "https://dev-0123456789abcdef.civit.ai/",
				Err: &tls.CertificateVerificationError{Err: systemRoots(syscall.ETIMEDOUT)}}, false},
	}

	var trappedRows int
	for _, tc := range cases {
		if tc.trapped {
			trappedRows++
		}
	}
	if trappedRows < 6 {
		t.Fatalf("only %d row(s) exercise the trap; want >= 6 — a table trimmed down to controls\n"+
			"reports a serene pass while pinning nothing", trappedRows)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Half one: the row's own premise.
			if got := naivelyLooksLikeAProbeTimeout(tc.err); got != tc.trapped {
				t.Fatalf("the naive `errors.As(err, &net.Error) && Timeout()` said %v for %T, want %v —\n"+
					"this row no longer exercises what it claims", got, tc.err, tc.trapped)
			}
			// Half two: the real classifier.
			if got := classifyProbeErr(tc.err); got != "unreachable" {
				t.Errorf("classifyProbeErr = %q for a filesystem failure (%T: %v), want \"unreachable\".\n"+
					"\"timeout\" points the author at the network for a problem on their own disk — the\n"+
					"manufactured-advice failure AGENTS.md item 10 forbids for this output.",
					got, tc.err, tc.err)
			}
		})
	}
}

// TestClassifyProbeErrStillTagsTransportFailures is the POSITIVE CONTROL
// battery. Without it, a "fix" that returned "unreachable" unconditionally would
// leave the whole table above green.
//
// 🔴 INVARIANT GUARD — every row passes at base too. That is exactly its job.
// It mixes REAL errors from the net stack with constructed ones, because a
// constructed *net.OpError proves nothing about what net/http hands us.
func TestClassifyProbeErrStillTagsTransportFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"REAL http.Client.Timeout (*url.Error)", realProbeClientTimeout(t), "timeout"},
		{"REAL read deadline exceeded (*net.OpError from conn.Read)", realProbeReadDeadline(t), "timeout"},
		{"REAL refused dial (*url.Error from http.Client.Do)", realProbeRefusedDial(t), "unreachable"},

		{"context.DeadlineExceeded", context.DeadlineExceeded, "timeout"},
		{"wrapped context.DeadlineExceeded", fmt.Errorf("probe: %w", context.DeadlineExceeded), "timeout"},
		{"*net.DNSError (not found)",
			&net.DNSError{Err: "no such host", Name: "dev-0123456789abcdef.civit.ai", IsNotFound: true}, "dns"},
		{"*url.Error wrapping *net.DNSError",
			&url.Error{Op: "Get", URL: "https://dev-0123456789abcdef.civit.ai/",
				Err: &net.DNSError{Err: "no such host", Name: "dev-0123456789abcdef.civit.ai", IsNotFound: true}}, "dns"},
		{"*net.OpError nesting *os.SyscallError(ETIMEDOUT)",
			&net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ETIMEDOUT)}, "timeout"},
		{"*url.Error wrapping a timeout *net.OpError",
			&url.Error{Op: "Get", URL: "https://dev-0123456789abcdef.civit.ai/",
				Err: &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ETIMEDOUT)}}, "timeout"},
		{"*net.OpError nesting *os.SyscallError(ECONNREFUSED)",
			&net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}, "unreachable"},
		{"a plain error", errors.New("boom"), "unreachable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyProbeErr(tc.err); got != tc.want {
				t.Errorf("classifyProbeErr = %q, want %q for %T: %v.\n"+
					"A genuine transport failure must keep its tag — the filesystem gate must not have\n"+
					"flattened the classification.", got, tc.want, tc.err, tc.err)
			}
		})
	}
}

// TestClassifyProbeErrRealDNSFailure uses a REAL resolver failure when one is
// observable. `.invalid` is reserved by RFC 2606 and can never resolve, but a
// sandbox with no resolver at all answers differently — so the row SKIPS rather
// than asserting something the environment is not doing. It never depends on
// reaching the public internet.
func TestClassifyProbeErrRealDNSFailure(t *testing.T) {
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get("http://dev-0123456789abcdef.invalid/")
	if err == nil {
		_ = resp.Body.Close()
		t.Skip("a reserved .invalid name resolved here — this environment cannot show a real DNS failure")
	}
	var dnsErr *net.DNSError
	if !errors.As(err, &dnsErr) {
		t.Skipf("the resolver did not produce a *net.DNSError here (%T: %v) — nothing real to assert", err, err)
	}
	if got := classifyProbeErr(err); got != "dns" {
		t.Errorf("classifyProbeErr = %q for a REAL resolver failure, want \"dns\": %v", got, err)
	}
}

// realProbeClientTimeout produces the error net/http really returns when
// http.Client.Timeout fires against a hung handler.
func realProbeClientTimeout(t *testing.T) error {
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

// realProbeRefusedDial produces the *url.Error http.Client.Do really returns for
// a refused connection — the commonest not-ready state of a dev tunnel.
func realProbeRefusedDial(t *testing.T) error {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not bind a loopback listener: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("could not release the listener: %v", err)
	}
	c := &http.Client{Timeout: 5 * time.Second}
	resp, derr := c.Get("http://" + addr + "/")
	if derr == nil {
		_ = resp.Body.Close()
		t.Skipf("something else grabbed %s between close and dial — the refusal is not observable here", addr)
	}
	var op *net.OpError
	if !errors.As(derr, &op) {
		t.Fatalf("expected a *net.OpError from a refused dial, got %T: %v", derr, derr)
	}
	return derr
}

// realProbeReadDeadline produces a genuine timeout *net.OpError from the net
// stack: a live loopback connection whose read deadline has already passed.
func realProbeReadDeadline(t *testing.T) error {
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
