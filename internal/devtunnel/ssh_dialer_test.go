package devtunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
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
	hostKey, err := GenerateEphemeralKey()
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = d.Dial(ctx, DialOptions{Endpoint: "127.0.0.1:1", RemoteHost: "dev-x.civit.ai", LocalPort: 5186, Signer: key.Signer, SSHHostPublicKey: hostKey.AuthorizedKey})
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
	boundAddr    chan string // the address the client requested in `tcpip-forward`
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
		boundAddr:    make(chan string, 1),
	}
	go fs.accept()
	return fs
}

func (fs *forwardServer) addr() string { return fs.ln.Addr().String() }
func (fs *forwardServer) close()       { _ = fs.ln.Close() }

// hostAuthorizedKey is the server's host public key in OpenSSH authorized-key
// line form — the value the dialer must pin (opts.SSHHostPublicKey) to complete
// the handshake against this server.
func (fs *forwardServer) hostAuthorizedKey() string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(fs.hostSigner.PublicKey())))
}

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
				// The `tcpip-forward` payload is `string addr, uint32 port`
				// (RFC 4254 §7.1). Capture the requested bind ADDRESS so the test
				// can assert the client bound the subdomain LABEL, not the full host.
				var fwd struct {
					Addr string
					Port uint32
				}
				if err := ssh.Unmarshal(req.Payload, &fwd); err == nil {
					select {
					case fs.boundAddr <- fwd.Addr:
					default:
					}
				}
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

// pushConnectionWithOrigin is pushConnection with control over the OriginAddr /
// OriginPort of the forwarded-tcpip payload. sish stamps OriginAddr with the
// tunnel HOSTNAME (not an IP), which is exactly the shape x/crypto's built-in
// client.Listen handler rejects (netip.ParseAddr fails → "cannot parse IP
// address" → channel rejected → 502). This lets the regression test reproduce
// that real-world channel and assert our handler proxies it anyway.
func (fs *forwardServer) pushConnectionWithOrigin(t *testing.T, sconn *ssh.ServerConn, addr string, port uint32, originAddr string, originPort uint32) ssh.Channel {
	t.Helper()
	payload := ssh.Marshal(struct {
		Addr       string
		Port       uint32
		OriginAddr string
		OriginPort uint32
	}{addr, port, originAddr, originPort})
	ch, reqs, err := sconn.OpenChannel("forwarded-tcpip", payload)
	if err != nil {
		t.Fatalf("open forwarded-tcpip: %v", err)
	}
	go ssh.DiscardRequests(reqs)
	return ch
}

// TestSSHDialerProxiesNonIPOriginAddr is the regression guard for the CONFIRMED
// delivery bug: sish opens each forwarded-tcpip channel with OriginAddr set to
// the tunnel HOSTNAME (`dev-<16hex>`, NOT an IP) — and, defensively, an Addr that
// need not equal the exact `<label>:80` we registered. x/crypto's built-in
// client.Listen handler runs netip.ParseAddr(OriginAddr) and REJECTS every such
// channel ("cannot parse IP address"), so Accept() never fires and sish 502s
// (verified live against the real sish: stock CLI → 502, this handler → 200).
// Our dialer accepts ALL forwarded-tcpip channels regardless of Addr/Origin, so
// the inbound connection must still round-trip to the local dev server.
func TestSSHDialerProxiesNonIPOriginAddr(t *testing.T) {
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
	tunnel, err := d.Dial(ctx, DialOptions{Endpoint: fs.addr(), RemoteHost: host, LocalPort: localPort, Signer: key.Signer, SSHHostPublicKey: fs.hostAuthorizedKey()})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer tunnel.Close()

	sconn := <-fs.sconnCh
	<-fs.forwardReady

	// Reproduce EXACTLY what live sish sends: a non-IP OriginAddr (the tunnel
	// hostname). Also stamp a MISMATCHED Addr (the full host, not the bound label)
	// to prove we don't strict-match the address either. Both would make
	// client.Listen reject the channel.
	ch := fs.pushConnectionWithOrigin(t, sconn, host, uint32(remoteBindPort), subdomainLabel(host), uint32(remoteBindPort))
	defer ch.Close()

	msg := "GET /@vite/client HTTP/1.1\r\n\r\n"
	if _, err := ch.Write([]byte(msg)); err != nil {
		t.Fatalf("write to forwarded channel: %v", err)
	}
	buf := make([]byte, len(msg))
	_ = ch.CloseWrite()
	if _, err := io.ReadFull(ch, buf); err != nil {
		t.Fatalf("read echo back through the tunnel (non-IP OriginAddr must still proxy): %v", err)
	}
	if got := string(buf); !strings.HasPrefix(got, "GET /@vite/client") {
		t.Errorf("proxied round-trip mismatch for non-IP-origin channel: got %q", got)
	}

	select {
	case <-tunnel.Activity():
	case <-time.After(time.Second):
		t.Error("expected an Activity signal for the inbound connection")
	}
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
	tunnel, err := d.Dial(ctx, DialOptions{Endpoint: fs.addr(), RemoteHost: host, LocalPort: localPort, Signer: key.Signer, SSHHostPublicKey: fs.hostAuthorizedKey()})
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

	// Simulate a browser connection: sish opens the forwarded-tcpip channel keyed
	// by the bound SUBDOMAIN LABEL (the address the client requested), so push with
	// the label — the full host would not match the client's registered forward.
	ch := fs.pushConnection(t, sconn, subdomainLabel(host), uint32(remoteBindPort))
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

// TestSSHDialerBindsSubdomainLabel is the regression guard for the sish
// domain-doubling bug: the reverse forward MUST request the subdomain LABEL
// (`dev-<16hex>`), NOT the full assigned host (`dev-<16hex>.civit.ai`). sish
// forms the served vhost as `<requested-subdomain>.<sish-domain>`, so binding the
// full host makes sish register `dev-<16hex>.civit.ai.civit.ai` — the SSH forward
// succeeds but the browser's real `Host: dev-<16hex>.civit.ai` 404s. This test
// captures the `tcpip-forward` bind address the client actually requested and
// asserts it is the label. It FAILS against the old full-host bind.
func TestSSHDialerBindsSubdomainLabel(t *testing.T) {
	fs := newForwardServer(t)
	defer fs.close()

	key, err := GenerateEphemeralKey()
	if err != nil {
		t.Fatal(err)
	}
	const fullHost = "dev-0123456789abcdef.civit.ai"
	const wantLabel = "dev-0123456789abcdef"

	d := NewSSHDialer(io.Discard)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tunnel, err := d.Dial(ctx, DialOptions{Endpoint: fs.addr(), RemoteHost: fullHost, LocalPort: 5186, Signer: key.Signer, SSHHostPublicKey: fs.hostAuthorizedKey()})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer tunnel.Close()

	select {
	case got := <-fs.boundAddr:
		if got != wantLabel {
			t.Errorf("reverse forward bound %q; want the subdomain LABEL %q (binding the full host %q makes sish double-append its domain → 404)", got, wantLabel, fullHost)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server never saw the tcpip-forward bind address")
	}
}

// TestSubdomainLabel unit-covers the label extraction across the shapes the
// dialer can see (the full assigned host, and defensively a bare host with no dot).
func TestSubdomainLabel(t *testing.T) {
	cases := map[string]string{
		"dev-0123456789abcdef.civit.ai":         "dev-0123456789abcdef",
		"dev-0123456789abcdef.apps.example.com": "dev-0123456789abcdef",
		"dev-0123456789abcdef":                  "dev-0123456789abcdef", // no dot → unchanged
	}
	for host, want := range cases {
		if got := subdomainLabel(host); got != want {
			t.Errorf("subdomainLabel(%q) = %q; want %q", host, got, want)
		}
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
	tunnel, err := d.Dial(context.Background(), DialOptions{Endpoint: fs.addr(), RemoteHost: host, LocalPort: deadPort, Signer: key.Signer, SSHHostPublicKey: fs.hostAuthorizedKey()})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer tunnel.Close()

	sconn := <-fs.sconnCh
	<-fs.forwardReady
	ch := fs.pushConnection(t, sconn, subdomainLabel(host), uint32(remoteBindPort))
	defer ch.Close()

	// The proxy dial to the dead local port fails; the channel closes. Poll the
	// log for the guidance line — it must name the port AND the --local-host so a
	// wrong host is obvious (the silent-502 dogfood failure).
	deadline := time.After(2 * time.Second)
	for {
		if strings.Contains(logbuf.String(), fmt.Sprintf("on port %d unreachable", deadPort)) {
			if !strings.Contains(logbuf.String(), "--local-host") {
				t.Errorf("unreachable notice must reference --local-host, got %q", logbuf.String())
			}
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

// TestSSHDialerProxyHonorsLocalHost proves the live proxy dials the configured
// LocalHost (not just loopback): with an explicit LocalHost that points at the
// echo server's address, an inbound forwarded connection round-trips. This is the
// fix for the silent-502 dogfood failure — a dev server not on the CLI's loopback
// (e.g. a container IP) is now reachable through the tunnel.
func TestSSHDialerProxyHonorsLocalHost(t *testing.T) {
	localPort, stopEcho := echoServer(t) // binds 127.0.0.1
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
	// Explicit LocalHost "127.0.0.1" — the non-loopback code path (exact single
	// target), pointed at where the echo server actually listens.
	tunnel, err := d.Dial(ctx, DialOptions{Endpoint: fs.addr(), RemoteHost: host, LocalPort: localPort, LocalHost: "127.0.0.1", Signer: key.Signer, SSHHostPublicKey: fs.hostAuthorizedKey()})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer tunnel.Close()

	sconn := <-fs.sconnCh
	<-fs.forwardReady
	ch := fs.pushConnection(t, sconn, subdomainLabel(host), uint32(remoteBindPort))
	defer ch.Close()

	msg := "GET /apps/dev HTTP/1.1\r\n\r\n"
	if _, err := ch.Write([]byte(msg)); err != nil {
		t.Fatalf("write to forwarded channel: %v", err)
	}
	buf := make([]byte, len(msg))
	_ = ch.CloseWrite()
	if _, err := io.ReadFull(ch, buf); err != nil {
		t.Fatalf("read echo back through the tunnel (explicit LocalHost): %v", err)
	}
	if got := string(buf); !strings.HasPrefix(got, "GET /apps/dev") {
		t.Errorf("proxied round-trip via explicit LocalHost mismatch: got %q", got)
	}
}

// TestSSHDialerProxyLocalHostUnreachableNamesHost proves a wrong/unreachable
// LocalHost surfaces the host in the guidance line (so the dev sees WHICH host
// the proxy tried and can correct --local-host).
func TestSSHDialerProxyLocalHostUnreachableNamesHost(t *testing.T) {
	// A closed port on an explicit loopback host.
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
	tunnel, err := d.Dial(context.Background(), DialOptions{Endpoint: fs.addr(), RemoteHost: host, LocalPort: deadPort, LocalHost: "127.0.0.1", Signer: key.Signer, SSHHostPublicKey: fs.hostAuthorizedKey()})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer tunnel.Close()

	sconn := <-fs.sconnCh
	<-fs.forwardReady
	ch := fs.pushConnection(t, sconn, subdomainLabel(host), uint32(remoteBindPort))
	defer ch.Close()

	deadline := time.After(2 * time.Second)
	for {
		got := logbuf.String()
		if strings.Contains(got, fmt.Sprintf("on port %d unreachable", deadPort)) {
			if !strings.Contains(got, "127.0.0.1") {
				t.Errorf("unreachable notice must name the tried host 127.0.0.1, got %q", got)
			}
			return
		}
		select {
		case <-deadline:
			t.Errorf("expected an unreachable-local-dev log line naming the host, got %q", got)
			return
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// TestDialLocalDevServerBothFamilies proves the loopback dialer reaches a dev
// server bound to EITHER family. The IPv6-only case is the real regression: a
// `--host localhost` dev server binds `::1` only on a dual-stack box, and the
// old 127.0.0.1-hardcoded dialer/probe falsely reported it unreachable.
func TestDialLocalDevServerBothFamilies(t *testing.T) {
	// The loopback-default cases run for BOTH host values that select loopback
	// behavior: "" (unset) and "localhost". Both must try both families.
	for _, loopbackHost := range []string{"", "localhost"} {
		loopbackHost := loopbackHost
		name := "host-empty"
		if loopbackHost != "" {
			name = "host-" + loopbackHost
		}
		t.Run(name, func(t *testing.T) {
			t.Run("ipv4", func(t *testing.T) {
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer ln.Close()
				port := ln.Addr().(*net.TCPAddr).Port
				conn, err := DialLocalDevServer(loopbackHost, port, 2*time.Second)
				if err != nil {
					t.Fatalf("expected to reach the IPv4 dev server on port %d: %v", port, err)
				}
				_ = conn.Close()
			})

			t.Run("ipv6", func(t *testing.T) {
				ln, err := net.Listen("tcp", "[::1]:0")
				if err != nil {
					t.Skipf("IPv6 loopback unavailable on this host: %v", err)
				}
				defer ln.Close()
				port := ln.Addr().(*net.TCPAddr).Port
				// Regression guard: on the old code this returned "connection refused"
				// because it only ever dialed 127.0.0.1.
				conn, err := DialLocalDevServer(loopbackHost, port, 2*time.Second)
				if err != nil {
					t.Fatalf("expected to reach the IPv6-only dev server on port %d (this is the localhost→::1 fix): %v", port, err)
				}
				_ = conn.Close()
			})
		})
	}

	t.Run("nothing-listening", func(t *testing.T) {
		// A port that is (almost certainly) closed: bind then immediately free.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		deadPort := ln.Addr().(*net.TCPAddr).Port
		_ = ln.Close()
		if conn, err := DialLocalDevServer("localhost", deadPort, 500*time.Millisecond); err == nil {
			_ = conn.Close()
			t.Fatalf("expected an error dialing the closed port %d", deadPort)
		}
	})
}

// TestDialLocalDevServerSpecificHost proves the non-loopback path: a specific
// host (the container/pod-netns / VM / bound-interface case, e.g. 10.42.0.100)
// is dialed EXACTLY as given — reaching a server bound to 127.0.0.1 ONLY when the
// caller asks for 127.0.0.1, and erroring when nothing listens there.
func TestDialLocalDevServerSpecificHost(t *testing.T) {
	t.Run("reaches-explicit-127.0.0.1", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		port := ln.Addr().(*net.TCPAddr).Port
		conn, err := DialLocalDevServer("127.0.0.1", port, 2*time.Second)
		if err != nil {
			t.Fatalf("expected to reach the explicit-host dev server on 127.0.0.1:%d: %v", port, err)
		}
		_ = conn.Close()
	})

	t.Run("explicit-ipv4-does-not-fall-back-to-ipv6", func(t *testing.T) {
		// Bind ONLY ::1, then ask for 127.0.0.1 explicitly. Unlike the loopback
		// default (which tries both families), an explicit 127.0.0.1 must NOT reach
		// an IPv6-only server — it is a single, exact target.
		//
		// Binding [::1]:0 does NOT reserve the same port number on IPv4: the two
		// families share one ephemeral range, so an unrelated `127.0.0.1:0` listener
		// elsewhere in this test binary can already hold it. That made this case fail
		// for the WRONG reason (the dial succeeded — against somebody else's server),
		// observed once after the embeddability tests added many loopback listeners.
		// So: keep picking ports until we have one that is genuinely IPv4-free.
		var ln net.Listener
		var port int
		for attempt := 0; attempt < 20; attempt++ {
			candidate, err := net.Listen("tcp", "[::1]:0")
			if err != nil {
				t.Skipf("IPv6 loopback unavailable on this host: %v", err)
			}
			p := candidate.Addr().(*net.TCPAddr).Port
			// If we can bind the same port on IPv4, nothing else holds it. Release it
			// immediately — the assertion below needs it EMPTY, not listening.
			probe, perr := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p)))
			if perr != nil {
				// Occupied on IPv4 by another test's server; this port is unusable here.
				_ = candidate.Close()
				continue
			}
			_ = probe.Close()
			ln, port = candidate, p
			break
		}
		if ln == nil {
			t.Skip("could not find a port free on IPv4 while bound on IPv6")
		}
		defer ln.Close()

		if conn, err := DialLocalDevServer("127.0.0.1", port, 500*time.Millisecond); err == nil {
			_ = conn.Close()
			t.Fatalf("explicit 127.0.0.1 must NOT reach an IPv6-only server on port %d", port)
		}
	})

	t.Run("unreachable-host-errors", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		deadPort := ln.Addr().(*net.TCPAddr).Port
		_ = ln.Close()
		// A specific host with nothing listening is a hard error (no both-families
		// masking).
		if conn, err := DialLocalDevServer("127.0.0.1", deadPort, 500*time.Millisecond); err == nil {
			_ = conn.Close()
			t.Fatalf("expected an error dialing the closed 127.0.0.1:%d", deadPort)
		}
	})
}

// TestPinnedHostKeyCallback proves the host-key pin: a valid host key yields a
// FixedHostKey callback that ACCEPTS the matching host key and REJECTS a
// DIFFERENT one; an empty key fails closed; a malformed key gives a parse error.
func TestPinnedHostKeyCallback(t *testing.T) {
	pinned, err := GenerateEphemeralKey()
	if err != nil {
		t.Fatal(err)
	}
	other, err := GenerateEphemeralKey()
	if err != nil {
		t.Fatal(err)
	}

	// Valid key → non-nil callback.
	cb, err := pinnedHostKeyCallback(pinned.AuthorizedKey)
	if err != nil {
		t.Fatalf("valid host key should build a callback, got %v", err)
	}
	if cb == nil {
		t.Fatal("expected a non-nil HostKeyCallback for a valid host key")
	}
	addr := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 2224}
	// Accepts the matching host key.
	if err := cb("sish.example:2224", addr, pinned.Signer.PublicKey()); err != nil {
		t.Errorf("callback should ACCEPT the matching host key, got %v", err)
	}
	// Rejects a different host key (the MITM case).
	if err := cb("sish.example:2224", addr, other.Signer.PublicKey()); err == nil {
		t.Error("callback must REJECT a different host key (MITM), got nil")
	}

	// Empty → fail closed, NO callback.
	if _, err := pinnedHostKeyCallback(""); err == nil || !strings.Contains(err.Error(), "sish host key to pin") {
		t.Errorf("empty host key must fail closed, got %v", err)
	}
	if _, err := pinnedHostKeyCallback("   "); err == nil || !strings.Contains(err.Error(), "sish host key to pin") {
		t.Errorf("blank host key must fail closed, got %v", err)
	}

	// Malformed → clear parse error, NO callback.
	if _, err := pinnedHostKeyCallback("not-a-valid-openssh-key"); err == nil || !strings.Contains(err.Error(), "parse sish host key") {
		t.Errorf("malformed host key must yield a parse error, got %v", err)
	}
}

// TestSSHDialerFailsClosedWithoutHostKey proves the dialer refuses to connect
// when the mint provided no host key to pin — it NEVER falls back to
// InsecureIgnoreHostKey. The error surfaces before any TCP dial.
func TestSSHDialerFailsClosedWithoutHostKey(t *testing.T) {
	key, err := GenerateEphemeralKey()
	if err != nil {
		t.Fatal(err)
	}
	d := NewSSHDialer(io.Discard)
	// Endpoint is irrelevant: the fail-closed check precedes the dial.
	_, err = d.Dial(context.Background(), DialOptions{
		Endpoint: "sish.example:2224", RemoteHost: "dev-0123456789abcdef.civit.ai",
		LocalPort: 5186, Signer: key.Signer, SSHHostPublicKey: "",
	})
	if err == nil || !strings.Contains(err.Error(), "sish host key to pin") {
		t.Fatalf("expected a fail-closed error with no host key, got %v", err)
	}
}

// TestSSHDialerMalformedHostKey proves a malformed pinned host key is a clear
// parse error (still no InsecureIgnoreHostKey path).
func TestSSHDialerMalformedHostKey(t *testing.T) {
	key, err := GenerateEphemeralKey()
	if err != nil {
		t.Fatal(err)
	}
	d := NewSSHDialer(io.Discard)
	_, err = d.Dial(context.Background(), DialOptions{
		Endpoint: "sish.example:2224", RemoteHost: "dev-0123456789abcdef.civit.ai",
		LocalPort: 5186, Signer: key.Signer, SSHHostPublicKey: "ssh-ed25519 not-base64!!!",
	})
	if err == nil || !strings.Contains(err.Error(), "parse sish host key") {
		t.Fatalf("expected a parse error for a malformed host key, got %v", err)
	}
}

// TestSSHDialerRejectsWrongPinnedHostKey is the end-to-end MITM guard: the
// dialer pins a host key that does NOT match the server's actual host key, so
// the SSH handshake is rejected and Dial fails (the impersonated endpoint can't
// complete). Proves FixedHostKey is enforced on the real handshake.
func TestSSHDialerRejectsWrongPinnedHostKey(t *testing.T) {
	fs := newForwardServer(t)
	defer fs.close()

	key, err := GenerateEphemeralKey()
	if err != nil {
		t.Fatal(err)
	}
	// Pin a DIFFERENT key than the server presents.
	wrong, err := GenerateEphemeralKey()
	if err != nil {
		t.Fatal(err)
	}

	d := NewSSHDialer(io.Discard)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = d.Dial(ctx, DialOptions{
		Endpoint: fs.addr(), RemoteHost: "dev-0123456789abcdef.civit.ai",
		LocalPort: 5186, Signer: key.Signer, SSHHostPublicKey: wrong.AuthorizedKey,
	})
	if err == nil {
		t.Fatal("expected the handshake to FAIL when the pinned host key does not match the server")
	}
	if !strings.Contains(err.Error(), "ssh handshake") {
		t.Errorf("expected an ssh handshake failure, got %v", err)
	}
}
