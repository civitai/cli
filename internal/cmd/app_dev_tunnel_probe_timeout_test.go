package cmd

import (
	"bufio"
	"errors"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// The dev-tunnel command tests that reach the mint do it with a "dev server"
// that is only a TCP listener. probeLocalDevServer's dial succeeds against it,
// and then the embeddability probe sends `GET /@vite/client` and waits for an
// answer that never comes — so each of those tests used to cost exactly
// embedProbeTimeout, two seconds, without asserting anything about the wait.
//
// silentDevServer makes that wait short AND makes it a claim. It accepts, reads
// what the client sends, never writes a byte, and records how each connection
// ended. A timeout is the only way the probe's request can end against it, so
// "one request arrived and the CLIENT hung up on it" is the timeout path seen
// from the other side of the wire — which is where it has to be seen, because
// CheckEmbeddable reports a probe it could not complete as no findings at all
// (AGENTS.md item 10) and the command prints nothing for it.

// testEmbedProbeTimeout is the budget listenLocal injects.
const testEmbedProbeTimeout = 50 * time.Millisecond

// silentServerHangBound is how long the silent server holds a request open
// before deciding the client is never going to give up. It is the watchdog that
// turns "the probe ignores its timeout" into a failed assertion instead of a
// hung test binary: at the bound the server closes the connection itself, which
// unblocks the command, and the test then fails by name. Two orders of
// magnitude above the injected budget, so load does not reach it.
const silentServerHangBound = 5 * time.Second

type silentDevServer struct {
	port int

	mu sync.Mutex
	// requests holds the request line of every connection that sent one.
	requests []string
	// abandoned counts requests the CLIENT closed while still unanswered.
	abandoned int
	// outlasted counts requests still open at silentServerHangBound.
	outlasted int
}

// startSilentDevServer listens on an ephemeral 127.0.0.1 port. wait closes the
// listener and blocks until every accepted connection has ended; the recorded
// fields are only meaningful after it returns.
func startSilentDevServer(t *testing.T) (srv *silentDevServer, wait func()) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("open local listener: %v", err)
	}
	srv = &silentDevServer{port: l.Addr().(*net.TCPAddr).Port}

	var conns sync.WaitGroup
	accepting := make(chan struct{})
	go func() {
		defer close(accepting)
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			conns.Add(1)
			go func() {
				defer conns.Done()
				srv.hold(c)
			}()
		}
	}()

	var once sync.Once
	wait = func() {
		once.Do(func() {
			_ = l.Close()
			<-accepting
			conns.Wait()
		})
	}
	t.Cleanup(wait)
	return srv, wait
}

// hold reads one connection to its end without ever answering it.
func (s *silentDevServer) hold(c net.Conn) {
	defer func() { _ = c.Close() }()
	_ = c.SetReadDeadline(time.Now().Add(silentServerHangBound))
	r := bufio.NewReader(c)

	// A connection that closes before sending a full line is probeLocalDevServer's
	// dial, which connects and hangs up. It is not a request and is not counted.
	line, err := r.ReadString('\n')
	if err != nil {
		return
	}
	s.mu.Lock()
	s.requests = append(s.requests, strings.TrimRight(line, "\r\n"))
	s.mu.Unlock()

	// Keep reading until the connection ends. The server never writes, so the
	// read ends in exactly one of two ways: the client closed (it gave up), or
	// the read deadline passed (it did not).
	for {
		if _, err = r.ReadString('\n'); err != nil {
			break
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if errors.Is(err, os.ErrDeadlineExceeded) {
		s.outlasted++
		return
	}
	s.abandoned++
}

// assertOneProbeTimedOut is the timeout path, asserted from the server's side:
// exactly one embeddability request arrived, the client abandoned it, and
// nothing followed it.
func (s *silentDevServer) assertOneProbeTimedOut(t *testing.T) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()

	const want = "GET " + "/@vite/client" + " HTTP/1.1"
	if len(s.requests) == 0 {
		t.Errorf("no HTTP request reached the silent dev server: the embeddability probe never ran, so this test did not take its timeout path")
		return
	}
	if s.outlasted > 0 {
		t.Errorf("the embeddability probe was still waiting %s after sending %q to a server that never answers: embedProbeTimeout is not bounding it",
			silentServerHangBound, s.requests[0])
	}
	if len(s.requests) != 1 || s.requests[0] != want {
		t.Errorf("silent dev server saw requests %q, want exactly [%q]: a probe that times out must stop, not go on to probe something else",
			s.requests, want)
	}
	if s.abandoned != len(s.requests)-s.outlasted {
		t.Errorf("silent dev server saw %d request(s), %d abandoned by the client and %d held to the %s bound: every request must end one of those two ways",
			len(s.requests), s.abandoned, s.outlasted, silentServerHangBound)
	}
}

// setProbeTimeout assigns one of the two dev-tunnel probe timeouts for the
// length of a test. The package's tests are not parallel, which is what makes a
// package variable an acceptable seam here.
func setProbeTimeout(t *testing.T, timeout *time.Duration, d time.Duration) (previous time.Duration) {
	t.Helper()
	previous = *timeout
	*timeout = d
	t.Cleanup(func() { *timeout = previous })
	return previous
}

// TestDevTunnelProbeTimeoutsDefaultToTwoSeconds pins the production budgets.
// They are variables so tests can shorten them; this is what notices one left
// shortened, or changed, in the source.
func TestDevTunnelProbeTimeoutsDefaultToTwoSeconds(t *testing.T) {
	if embedProbeTimeout != 2*time.Second {
		t.Errorf("embedProbeTimeout = %s, want 2s", embedProbeTimeout)
	}
	if localProbeDialTimeout != 2*time.Second {
		t.Errorf("localProbeDialTimeout = %s, want 2s", localProbeDialTimeout)
	}
}

// TestEmbedProbeHonoursItsInjectedTimeout drives the production probe at a
// server that never answers and checks the injected value is the one that
// bounded it: the probe waits at least that long, returns before the production
// default would have fired, and reports nothing.
//
// The lower bound cannot flake, since a timer does not fire early. The upper
// bound is the production default rather than a multiple of the injected value,
// so it asks only "was it the injected budget or the default one?" and leaves a
// loaded machine more than ten times the budget to answer.
func TestEmbedProbeHonoursItsInjectedTimeout(t *testing.T) {
	// Distinct from both the production default and testEmbedProbeTimeout, so
	// neither constant can satisfy the bounds below by coincidence.
	const injected = 150 * time.Millisecond
	production := setProbeTimeout(t, &embedProbeTimeout, injected)
	if injected*10 > production {
		t.Fatalf("the production embedProbeTimeout is %s, too close to the injected %s for the upper bound to mean anything", production, injected)
	}

	srv, wait := startSilentDevServer(t)

	start := time.Now()
	findings := probeEmbeddableDevServer("", srv.port)
	elapsed := time.Since(start)
	wait()

	if elapsed < injected {
		t.Errorf("the probe returned after %s against a server that never answers, before its %s timeout: it did not wait for an answer", elapsed, injected)
	}
	if elapsed >= production {
		t.Errorf("the probe took %s with a %s timeout injected, which is the production %s or longer: the injected value is not the one bounding it", elapsed, injected, production)
	}
	// Item 10: a probe that could not observe says nothing.
	if len(findings) != 0 {
		t.Errorf("a probe that timed out returned %d finding(s), want none: %+v", len(findings), findings)
	}
	srv.assertOneProbeTimedOut(t)
}
