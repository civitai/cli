package devtunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// remoteBindPort is the port the reverse forward binds on the sish side. sish
// routes the assigned `dev-<16hex>.<APPS_DOMAIN>` vhost to this HTTP backend; the
// public edge (Traefik `*.civit.ai` on :443) fronts it. Fixed here; P3 confirms
// it against the real sish image.
const remoteBindPort = 80

// subdomainLabel returns the first DNS label of a host — the piece BEFORE the
// first `.` (e.g. "dev-0123456789abcdef.civit.ai" → "dev-0123456789abcdef").
//
// WHY this matters for the `ssh -R` bind: sish forms the served HTTP vhost as
// `<requested-bind-subdomain>.<sish-domain>` (sish is configured `domain: civit.ai`,
// `force-requested-subdomains: true`). So the reverse forward must request the
// SUBDOMAIN LABEL, not the full assigned host: binding the full host
// `dev-<16hex>.civit.ai` makes sish register the vhost as
// `dev-<16hex>.civit.ai.civit.ai` (domain DOUBLED) — the SSH auth + the forward
// reservation both succeed, but the browser's real `Host: dev-<16hex>.civit.ai`
// (via Traefik) 404s and the tunnel never serves. Binding the label
// `dev-<16hex>` makes sish form `dev-<16hex>.civit.ai`, which matches.
//
// The assigned host is always `dev-<16hex>.<domain>` (regex-validated upstream in
// app_dev_tunnel.go before we ever dial), so the label is the first component. A
// bare host with no `.` (shouldn't happen) is returned unchanged.
func subdomainLabel(host string) string {
	return strings.SplitN(host, ".", 2)[0]
}

// sshDialUser is the SSH username presented on the bind. sish authorizes by the
// PUBLIC KEY (the callback), not the username, so this is cosmetic/log-only.
const sshDialUser = "civitai-dev"

// dialTimeout bounds the initial SSH handshake so a dead/unreachable endpoint
// (the common case pre-P3) fails fast instead of hanging.
const dialTimeout = 15 * time.Second

// localDialTimeout bounds each attempt to reach the developer's local dev server.
// Loopback connects/refusals resolve in well under a millisecond, so this only
// matters for a wedged (accepting-but-never-completing) local server.
const localDialTimeout = 3 * time.Second

// loopbackHosts are the addresses a local dev server may be bound to. A dev
// server launched with `--host localhost` (the scaffold's `dev:tunnel`) binds to
// whichever family `localhost` resolves to — `::1` on a dual-stack box, else
// `127.0.0.1` — and to that ONE family only. So the CLI must try both loopback
// families rather than assume IPv4; otherwise a perfectly-running dev server on
// `::1` looks "unreachable" to both the pre-flight probe and the tunnel proxy.
// IPv4 is tried first (still the common bind), IPv6 second.
var loopbackHosts = []string{"127.0.0.1", "::1"}

// isLoopbackHost reports whether `host` should use the both-loopback-families
// behavior: an empty host or the literal "localhost" (the scaffold's
// `dev:tunnel` binds `localhost`). Any explicit address/hostname is dialed as-is.
func isLoopbackHost(host string) bool {
	h := strings.TrimSpace(host)
	return h == "" || strings.EqualFold(h, "localhost")
}

// DialLocalDevServer connects to the developer's local dev server on `host:port`.
//
//   - host "" or "localhost" (the SAFE default) → the loopback behavior: try each
//     family in turn (127.0.0.1 then ::1) with a per-attempt timeout and return
//     the first successful connection. A `--host localhost` dev server binds only
//     ONE family (`::1` on a dual-stack box), so both must be tried.
//   - any other host (e.g. a container/pod-netns IP like 10.42.0.100, a VM, or a
//     specific bound interface) → dial EXACTLY that host:port (single target; the
//     both-families logic is loopback-specific).
//
// Shared by the pre-flight probe (which closes the returned conn) and the live
// tunnel proxy (which uses it) so the two always agree on whether the dev server
// is reachable.
func DialLocalDevServer(host string, port int, timeout time.Duration) (net.Conn, error) {
	if isLoopbackHost(host) {
		var lastErr error
		for _, lh := range loopbackHosts {
			conn, err := net.DialTimeout("tcp", net.JoinHostPort(lh, strconv.Itoa(port)), timeout)
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}
	return net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), timeout)
}

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

// pinnedHostKeyCallback builds a FixedHostKey callback from the mint-provided
// OpenSSH host public-key line, PINNING the sish host key so an on-path attacker
// impersonating the sish endpoint is rejected at the SSH handshake (a MITM there
// would otherwise reach the dev's localhost + read/tamper tunneled traffic).
//
// FAIL CLOSED: an empty/absent host key is a hard error — the dialer refuses to
// connect rather than fall back to ssh.InsecureIgnoreHostKey. A malformed key
// yields a clear parse error. There is NO code path here that accepts an
// unverified host key.
func pinnedHostKeyCallback(hostPublicKey string) (ssh.HostKeyCallback, error) {
	if strings.TrimSpace(hostPublicKey) == "" {
		return nil, fmt.Errorf("server did not provide a sish host key to pin; refusing to connect")
	}
	pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(hostPublicKey))
	if err != nil {
		return nil, fmt.Errorf("parse sish host key %q: %w", hostPublicKey, err)
	}
	return ssh.FixedHostKey(pub), nil
}

// Dial opens the SSH connection to the sish endpoint and requests a reverse
// forward for the assigned host, then proxies inbound connections to the local
// dev server.
//
// SECURITY (host key): the sish host key is PINNED from the mint's
// sshHostPublicKey (opts.SSHHostPublicKey) via ssh.FixedHostKey — this closes
// MITM on the first-party SSH hop. If the mint did not supply a host key the
// dial FAILS CLOSED (see pinnedHostKeyCallback); it never uses
// InsecureIgnoreHostKey.
func (d *sshDialer) Dial(ctx context.Context, opts DialOptions) (Tunnel, error) {
	hostKeyCallback, err := pinnedHostKeyCallback(opts.SSHHostPublicKey)
	if err != nil {
		return nil, err
	}
	cfg := &ssh.ClientConfig{
		User:            sshDialUser,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(opts.Signer)},
		HostKeyCallback: hostKeyCallback,
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

	// Bind the SUBDOMAIN LABEL, not the full host: sish appends its own domain to
	// the requested subdomain, so requesting the full `dev-<16hex>.civit.ai`
	// double-appends to `dev-<16hex>.civit.ai.civit.ai` and the browser's real
	// `Host: dev-<16hex>.civit.ai` request 404s. See subdomainLabel. opts.RemoteHost
	// stays the full host for logging / the ready-URL / upstream validation.
	bindAddr := fmt.Sprintf("%s:%d", subdomainLabel(opts.RemoteHost), remoteBindPort)
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
		localHost: opts.LocalHost,
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
	localHost string
	localPort int
	closeOnce sync.Once

	// localErrMu guards lastLocalErr so the per-connection proxy path can
	// rate-limit the "local dev server unreachable" notice: a broken local server
	// makes the browser retry many subresources, which would otherwise flood the
	// log and scroll the guidance off-screen. We emit it at most once per
	// localErrLogEvery.
	localErrMu   sync.Mutex
	lastLocalErr time.Time
}

// localErrLogEvery rate-limits the proxy's local-unreachable notice (see
// localErrMu) so a retrying browser doesn't flood stderr.
const localErrLogEvery = 5 * time.Second

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

// proxy bridges one forwarded connection to the local dev server on
// <localHost>:<localPort> (loopback both-families when localHost is
// empty/"localhost", else the exact host).
func (t *sshTunnel) proxy(remote net.Conn) {
	defer remote.Close()
	local, err := DialLocalDevServer(t.localHost, t.localPort, localDialTimeout)
	if err != nil {
		t.logLocalUnreachable(err)
		return
	}
	defer local.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(local, remote); _ = closeWrite(local) }()
	go func() { defer wg.Done(); _, _ = io.Copy(remote, local); _ = closeWrite(remote) }()
	wg.Wait()
}

// displayLocalHost renders the local host for user-facing messages: an
// empty/"localhost" host is shown as "localhost" (the loopback default).
func displayLocalHost(host string) string {
	if isLoopbackHost(host) {
		return "localhost"
	}
	return host
}

// logLocalUnreachable emits the prominent, rate-limited notice that the tunnel is
// up but the local dev server can't be reached. It names the resolved
// --local-host so a wrong host (the silent-502 dogfood failure: dev server on a
// container IP, not loopback) is obvious. Rate-limited via localErrMu so a
// retrying browser doesn't scroll it off-screen.
func (t *sshTunnel) logLocalUnreachable(err error) {
	if t.log == nil {
		return
	}
	t.localErrMu.Lock()
	now := time.Now()
	if !t.lastLocalErr.IsZero() && now.Sub(t.lastLocalErr) < localErrLogEvery {
		t.localErrMu.Unlock()
		return
	}
	t.lastLocalErr = now
	t.localErrMu.Unlock()

	host := displayLocalHost(t.localHost)
	t.logf("\n  ⚠ tunnel: local dev server on port %d unreachable at %s:%d (%v)\n"+
		"    → is your dev server running (`npm run dev:tunnel`), and is --local-host %q correct?\n\n",
		t.localPort, host, t.localPort, err, host)
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
