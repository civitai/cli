package devtunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// remoteBindPort is the port the reverse forward binds on the sish side. sish
// routes the assigned `dev-<16hex>.<APPS_DOMAIN>` vhost to this HTTP backend; the
// public edge (Traefik `*.civit.ai` on :443) fronts it. Fixed here; P3 confirms
// it against the real sish image.
const remoteBindPort = 80

// sshDialUser is the SSH username presented on the bind. sish authorizes by the
// PUBLIC KEY (the callback), not the username, so this is cosmetic/log-only.
const sshDialUser = "civitai-dev"

// dialTimeout bounds the initial SSH handshake so a dead/unreachable endpoint
// (the common case pre-P3) fails fast instead of hanging.
const dialTimeout = 15 * time.Second

// sshDialer is the production Dialer: an in-process `ssh -R` via
// golang.org/x/crypto/ssh, so the ephemeral private key never touches disk.
type sshDialer struct {
	log io.Writer // tunnel/connection status stream (stderr)
}

// NewSSHDialer builds the production reverse-tunnel dialer, logging connection
// status to log.
func NewSSHDialer(log io.Writer) Dialer { return &sshDialer{log: log} }

func (d *sshDialer) logf(format string, a ...any) {
	if d.log == nil {
		return
	}
	fmt.Fprintf(d.log, format, a...)
}

// Dial opens the SSH connection to the sish endpoint and requests a reverse
// forward for the assigned host, then proxies inbound connections to the local
// dev server.
//
// SECURITY (host key): host-key verification is deferred to P3 — the sish
// endpoint + its host key are not provisioned yet. Until then this uses
// InsecureIgnoreHostKey; P3 MUST pin the sish host key (known_hosts) before the
// public listener is exposed. The tunnel's security boundary is the edge
// forwardAuth + the pubkey-bound credential (design §5/§6), not this leg, but a
// pinned host key closes MITM on the first-party SSH hop.
func (d *sshDialer) Dial(ctx context.Context, opts DialOptions) (Tunnel, error) {
	cfg := &ssh.ClientConfig{
		User:            sshDialUser,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(opts.Signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO(P3): pin the sish host key
		Timeout:         dialTimeout,
	}

	dialer := &net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", opts.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", opts.Endpoint, err)
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, opts.Endpoint, cfg)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ssh handshake with %s: %w", opts.Endpoint, err)
	}
	client := ssh.NewClient(sshConn, chans, reqs)

	bindAddr := fmt.Sprintf("%s:%d", opts.RemoteHost, remoteBindPort)
	listener, err := client.Listen("tcp", bindAddr)
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("request reverse forward for %s: %w", bindAddr, err)
	}

	t := &sshTunnel{
		client:    client,
		listener:  listener,
		done:      make(chan struct{}),
		activity:  make(chan struct{}, 1),
		log:       d.log,
		localPort: opts.LocalPort,
	}
	go t.serve()
	return t, nil
}

// sshTunnel is a live reverse tunnel from the sish endpoint to the local dev
// server.
type sshTunnel struct {
	client    *ssh.Client
	listener  net.Listener
	done      chan struct{}
	activity  chan struct{}
	log       io.Writer
	localPort int
	closeOnce sync.Once
}

func (t *sshTunnel) Done() <-chan struct{}     { return t.done }
func (t *sshTunnel) Activity() <-chan struct{} { return t.activity }

func (t *sshTunnel) logf(format string, a ...any) {
	if t.log == nil {
		return
	}
	fmt.Fprintf(t.log, format, a...)
}

// serve accepts forwarded connections until the listener closes, proxying each
// to the local dev server. It closes done on exit so the command's select loop
// learns the tunnel terminated.
func (t *sshTunnel) serve() {
	defer close(t.done)
	for {
		remote, err := t.listener.Accept()
		if err != nil {
			// Listener closed (teardown) or the SSH connection dropped.
			return
		}
		// Best-effort, coalesced activity signal (drop if the buffer is full — one
		// pending signal is enough to reset the idle timer).
		select {
		case t.activity <- struct{}{}:
		default:
		}
		go t.proxy(remote)
	}
}

// proxy bridges one forwarded connection to 127.0.0.1:<localPort>.
func (t *sshTunnel) proxy(remote net.Conn) {
	defer remote.Close()
	local, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", t.localPort))
	if err != nil {
		t.logf("  tunnel: local dev server 127.0.0.1:%d unreachable (%v) — is `npm run dev` running?\n", t.localPort, err)
		return
	}
	defer local.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(local, remote); _ = closeWrite(local) }()
	go func() { defer wg.Done(); _, _ = io.Copy(remote, local); _ = closeWrite(remote) }()
	wg.Wait()
}

// closeWrite half-closes the write side when supported (TCP), so the peer sees
// EOF and the copy unwinds; otherwise a no-op.
func closeWrite(c net.Conn) error {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}

// Close tears the tunnel down (idempotent): closing the listener unblocks
// serve, and closing the client drops the SSH connection.
func (t *sshTunnel) Close() error {
	t.closeOnce.Do(func() {
		_ = t.listener.Close()
		_ = t.client.Close()
	})
	return nil
}
