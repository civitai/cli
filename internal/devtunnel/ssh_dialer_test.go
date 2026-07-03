package devtunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// syncBuffer is a goroutine-safe io.Writer for capturing tunnel logs that a
// test reads concurrently.
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// TestSSHDialerDialErrorFast: dialing an unreachable endpoint returns an error
// quickly (covers the Dial failure path without a real server).
func TestSSHDialerDialErrorFast(t *testing.T) {
	key, err := GenerateEphemeralKey()
	if err != nil {
		t.Fatal(err)
	}
	d := NewSSHDialer(io.Discard)
	// 127.0.0.1:1 is reliably closed; DialContext should fail promptly.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	start := time.Now()
	_, err = d.Dial(ctx, DialOptions{Endpoint: "127.0.0.1:1", RemoteHost: "dev-x.civit.ai", LocalPort: 5186, Signer: key.Signer})
	if err == nil {
		t.Fatal("expected a dial error against a closed port")
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("dial to a closed port should fail fast, took %v", time.Since(start))
	}
}

// echoServer stands up a local TCP echo listener (stand-in for `npm run dev`)
// and returns its port + a stop func.
func echoServer(t *testing.T) (int, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) { _, _ = io.Copy(conn, conn); conn.Close() }(c)
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, func() { _ = ln.Close() }
}

// forwardServer is a minimal in-process SSH server that accepts any pubkey and
// honors a `tcpip-forward` request, exposing a way to push a forwarded-tcpip
// channel (simulating an inbound browser connection to the assigned host).
type forwardServer struct {
	ln           net.Listener
	hostSigner   ssh.Signer
	sconnCh      chan *ssh.ServerConn
	forwardReady chan struct{}
}

func newForwardServer(t *testing.T) *forwardServer {
	t.Helper()
	host, err := GenerateEphemeralKey()
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fs := &forwardServer{
		ln:           ln,
		hostSigner:   host.Signer,
		sconnCh:      make(chan *ssh.ServerConn, 1),
		forwardReady: make(chan struct{}, 1),
	}
	go fs.accept()
	return fs
}

func (fs *forwardServer) addr() string { return fs.ln.Addr().String() }
func (fs *forwardServer) close()       { _ = fs.ln.Close() }

func (fs *forwardServer) accept() {
	nconn, err := fs.ln.Accept()
	if err != nil {
		return
	}
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) {
			return &ssh.Permissions{}, nil
		},
	}
	cfg.AddHostKey(fs.hostSigner)
	sconn, chans, reqs, err := ssh.NewServerConn(nconn, cfg)
	if err != nil {
		return
	}
	// Reject any session channels (the client only requests a reverse forward).
	go func() {
		for nc := range chans {
			_ = nc.Reject(ssh.Prohibited, "no sessions")
		}
	}()
	// Service global requests; reply true to tcpip-forward and signal readiness.
	go func() {
		for req := range reqs {
			if req.Type == "tcpip-forward" {
				if req.WantReply {
					_ = req.Reply(true, nil)
				}
				select {
				case fs.forwardReady <- struct{}{}:
				default:
				}
				continue
			}
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}()
	fs.sconnCh <- sconn
}

// pushConnection opens a forwarded-tcpip channel to the client for (host, port),
// which the client's reverse listener will Accept — driving the dialer's
// serve/proxy path.
func (fs *forwardServer) pushConnection(t *testing.T, sconn *ssh.ServerConn, host string, port uint32) ssh.Channel {
	t.Helper()
	payload := ssh.Marshal(struct {
		Addr       string
		Port       uint32
		OriginAddr string
		OriginPort uint32
	}{host, port, "127.0.0.1", 40000})
	ch, reqs, err := sconn.OpenChannel("forwarded-tcpip", payload)
	if err != nil {
		t.Fatalf("open forwarded-tcpip: %v", err)
	}
	go ssh.DiscardRequests(reqs)
	return ch
}

// TestSSHDialerReverseForwardProxies is the real dialer end-to-end against an
// in-process SSH forward server + a local echo server: it proves Dial binds the
// reverse forward, an inbound forwarded connection is proxied to the local dev
// port (round-trips), Activity fires, and Close/Done work.
func TestSSHDialerReverseForwardProxies(t *testing.T) {
	localPort, stopEcho := echoServer(t)
	defer stopEcho()

	fs := newForwardServer(t)
	defer fs.close()

	key, err := GenerateEphemeralKey()
	if err != nil {
		t.Fatal(err)
	}
	const host = "dev-0123456789abcdef.civit.ai"

	d := NewSSHDialer(io.Discard)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tunnel, err := d.Dial(ctx, DialOptions{Endpoint: fs.addr(), RemoteHost: host, LocalPort: localPort, Signer: key.Signer})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer tunnel.Close()

	// Wait for the server to have registered the reverse forward + grab the conn.
	var sconn *ssh.ServerConn
	select {
	case sconn = <-fs.sconnCh:
	case <-time.After(3 * time.Second):
		t.Fatal("server never completed handshake")
	}
	select {
	case <-fs.forwardReady:
	case <-time.After(3 * time.Second):
		t.Fatal("server never saw the tcpip-forward request")
	}

	// Simulate a browser connection: open a forwarded channel on the assigned host.
	ch := fs.pushConnection(t, sconn, host, uint32(remoteBindPort))
	defer ch.Close()

	// The dialer should proxy this to the local echo server → round-trip.
	msg := "GET /apps/dev HTTP/1.1\r\n\r\n"
	if _, err := ch.Write([]byte(msg)); err != nil {
		t.Fatalf("write to forwarded channel: %v", err)
	}
	buf := make([]byte, len(msg))
	_ = ch.CloseWrite() // let the echo copy unwind after our bytes
	if _, err := io.ReadFull(ch, buf); err != nil {
		t.Fatalf("read echo back through the tunnel: %v", err)
	}
	if got := string(buf); !strings.HasPrefix(got, "GET /apps/dev") {
		t.Errorf("proxied round-trip mismatch: got %q", got)
	}

	// Activity should have fired for the inbound connection.
	select {
	case <-tunnel.Activity():
	case <-time.After(time.Second):
		t.Error("expected an Activity signal for the inbound connection")
	}

	// Close is idempotent and unblocks the serve loop → Done closes.
	if err := tunnel.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	_ = tunnel.Close() // idempotent
	select {
	case <-tunnel.Done():
	case <-time.After(3 * time.Second):
		t.Error("Done was not signaled after Close")
	}
}

// TestSSHDialerLocalDevDown: when the local dev server is NOT running, a
// forwarded connection is accepted but the proxy can't reach localhost — the
// tunnel logs guidance and stays up (doesn't crash).
func TestSSHDialerLocalDevDown(t *testing.T) {
	// Pick a port that is (almost certainly) closed: bind then immediately free.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadPort := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	fs := newForwardServer(t)
	defer fs.close()
	key, _ := GenerateEphemeralKey()
	const host = "dev-0123456789abcdef.civit.ai"

	var logbuf syncBuffer
	d := NewSSHDialer(&logbuf)
	tunnel, err := d.Dial(context.Background(), DialOptions{Endpoint: fs.addr(), RemoteHost: host, LocalPort: deadPort, Signer: key.Signer})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer tunnel.Close()

	sconn := <-fs.sconnCh
	<-fs.forwardReady
	ch := fs.pushConnection(t, sconn, host, uint32(remoteBindPort))
	defer ch.Close()

	// The proxy dial to the dead local port fails; the channel closes. Poll the
	// log for the guidance line.
	deadline := time.After(2 * time.Second)
	for {
		if strings.Contains(logbuf.String(), fmt.Sprintf("127.0.0.1:%d", deadPort)) {
			return
		}
		select {
		case <-deadline:
			t.Errorf("expected an unreachable-local-dev log line, got %q", logbuf.String())
			return
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}
