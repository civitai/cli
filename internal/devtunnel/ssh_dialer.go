package devtunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
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

	// Register OUR OWN handler for inbound forwarded-tcpip channels BEFORE asking
	// sish to forward, so no early channel is missed.
	//
	// WHY NOT client.Listen: x/crypto's client.Listen installs a forwardList
	// handler that, for every inbound `forwarded-tcpip` channel, first calls
	// parseTCPAddr(payload.OriginAddr, payload.OriginPort) — which runs
	// netip.ParseAddr on OriginAddr and REJECTS the channel with
	// "cannot parse IP address" if it is not an IP literal (x/crypto tcpip.go).
	// sish sets OriginAddr to the tunnel HOSTNAME (`dev-<16hex>`), not an IP — so
	// x/crypto rejects EVERY channel before it ever reaches the forward match.
	// Accept() therefore never fires, the proxy never runs, and sish returns 502.
	//
	// CONFIRMED LIVE (PR): with a live sish tunnel, sish stamps the channel
	// Addr="dev-<16hex>" Port=80 (which DOES match our `<label>:80` bind — so the
	// address match is NOT the problem), but OriginAddr="dev-<16hex>" — a hostname,
	// not an IP — which is exactly what parseTCPAddr rejects. Stock CLI → 502 on
	// every real request; this handler → 200 (see PR evidence).
	//
	// Since we only ever request a SINGLE reverse forward, EVERY inbound
	// forwarded-tcpip channel on this connection is unambiguously ours — accept
	// them ALL, without inspecting the Addr/Port/Origin sish stamps on them.
	newChans := client.HandleChannelOpen("forwarded-tcpip")
	if newChans == nil {
		_ = client.Close()
		return nil, fmt.Errorf("forwarded-tcpip channel handler already registered")
	}

	// Request the reverse forward ourselves. Bind the SUBDOMAIN LABEL, not the
	// full host: sish appends its own domain to the requested subdomain, so
	// requesting the full `dev-<16hex>.civit.ai` double-appends to
	// `dev-<16hex>.civit.ai.civit.ai` and the browser's real
	// `Host: dev-<16hex>.civit.ai` request 404s. See subdomainLabel. opts.RemoteHost
	// stays the full host for logging / the ready-URL / upstream validation.
	bindLabel := subdomainLabel(opts.RemoteHost)
	req := tcpipForwardRequest{BindAddr: bindLabel, BindPort: remoteBindPort}
	ok, _, err := client.SendRequest("tcpip-forward", true, ssh.Marshal(&req))
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("request reverse forward for %s:%d: %w", bindLabel, remoteBindPort, err)
	}
	if !ok {
		_ = client.Close()
		return nil, fmt.Errorf("reverse forward for %s:%d denied by sish", bindLabel, remoteBindPort)
	}

	t := &sshTunnel{
		client:    client,
		newChans:  newChans,
		bindLabel: bindLabel,
		done:      make(chan struct{}),
		activity:  make(chan struct{}, 1),
		log:       d.log,
		localHost: opts.LocalHost,
		localPort: opts.LocalPort,
		debug:     os.Getenv(devTunnelDebugEnv) != "",
	}
	go t.serve()
	return t, nil
}

// tcpipForwardRequest is the RFC 4254 §7.1 `tcpip-forward` global-request
// payload (string bind address, uint32 bind port). Marshalled with ssh.Marshal,
// which emits the fields in declaration order — matching the wire format.
type tcpipForwardRequest struct {
	BindAddr string
	BindPort uint32
}

// forwardedTCPPayload is the RFC 4254 §7.2 `forwarded-tcpip` channel-open
// payload sish sends for each inbound connection. We parse it only for the
// diagnostic log (see devTunnelDebugEnv) — the proxy accepts the channel
// regardless of what these fields say, because our single reverse forward owns
// every forwarded-tcpip channel on this connection. OriginAddr is the field that
// broke stock delivery: sish sets it to the tunnel hostname (not an IP), which
// x/crypto's built-in handler rejects via netip.ParseAddr.
type forwardedTCPPayload struct {
	Addr       string
	Port       uint32
	OriginAddr string
	OriginPort uint32
}

// devTunnelDebugEnv, when set to any non-empty value, makes the tunnel log the
// exact (Addr:Port / OriginAddr:OriginPort) sish stamps on each inbound
// forwarded-tcpip channel — the evidence for WHY x/crypto's built-in
// client.Listen handler rejected it (a non-IP OriginAddr → "cannot parse IP
// address"), the root cause of the historical 502s. Off by default; zero cost
// when unset.
const devTunnelDebugEnv = "CIVITAI_DEVTUNNEL_DEBUG"

// sshTunnel is a live reverse tunnel from the sish endpoint to the local dev
// server.
type sshTunnel struct {
	client    *ssh.Client
	newChans  <-chan ssh.NewChannel
	bindLabel string
	done      chan struct{}
	activity  chan struct{}
	log       io.Writer
	localHost string
	localPort int
	debug     bool
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

// serve accepts inbound forwarded-tcpip channels until the connection drops (the
// channel stream closes) or Close tears it down, proxying each to the local dev
// server. It accepts EVERY channel unconditionally: our single reverse forward
// owns them all, so we deliberately do NOT strict-match the (Addr:Port) sish
// stamps on the channel — that strict match (x/crypto's client.Listen) is
// exactly what broke delivery and 502'd. It closes done on exit so the command's
// select loop learns the tunnel terminated.
func (t *sshTunnel) serve() {
	defer close(t.done)
	for newCh := range t.newChans {
		if t.debug {
			var p forwardedTCPPayload
			if err := ssh.Unmarshal(newCh.ExtraData(), &p); err == nil {
				_, originErr := netip.ParseAddr(p.OriginAddr)
				t.logf("  [debug] forwarded-tcpip: sish stamped Addr=%q Port=%d OriginAddr=%q OriginPort=%d (we bound %q:%d) — accepting; x/crypto client.Listen would have REJECTED it because OriginAddr is not an IP: %v\n",
					p.Addr, p.Port, p.OriginAddr, p.OriginPort, t.bindLabel, remoteBindPort, originErr != nil)
			} else {
				t.logf("  [debug] forwarded-tcpip: could not parse ExtraData (%v); accepting anyway\n", err)
			}
		}
		ch, reqs, err := newCh.Accept()
		if err != nil {
			// The connection is going away; the range will end on the next close.
			continue
		}
		go ssh.DiscardRequests(reqs)
		// Best-effort, coalesced activity signal (drop if the buffer is full — one
		// pending signal is enough to reset the idle timer).
		select {
		case t.activity <- struct{}{}:
		default:
		}
		go t.proxy(ch)
	}
}

// proxy bridges one forwarded channel to the local dev server on
// <localHost>:<localPort> (loopback both-families when localHost is
// empty/"localhost", else the exact host). remote is an ssh.Channel (an
// io.ReadWriteCloser whose CloseWrite half-closes the SSH channel).
func (t *sshTunnel) proxy(remote io.ReadWriteCloser) {
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
func closeWrite(c io.ReadWriteCloser) error {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}

// Close tears the tunnel down (idempotent): closing the client drops the SSH
// connection, which closes the forwarded-tcpip channel stream and unblocks
// serve's range. A best-effort cancel-tcpip-forward lets sish drop the vhost
// promptly instead of waiting for the connection teardown.
func (t *sshTunnel) Close() error {
	t.closeOnce.Do(func() {
		req := tcpipForwardRequest{BindAddr: t.bindLabel, BindPort: remoteBindPort}
		_, _, _ = t.client.SendRequest("cancel-tcpip-forward", false, ssh.Marshal(&req))
		_ = t.client.Close()
	})
	return nil
}
