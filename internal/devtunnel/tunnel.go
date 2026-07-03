package devtunnel

import (
	"context"
	"time"

	"golang.org/x/crypto/ssh"
)

// DialOptions parametrizes a reverse-tunnel dial.
type DialOptions struct {
	// Endpoint is the public sish SSH listener `host:port` (P3-provisioned).
	Endpoint string
	// RemoteHost is the server-assigned `dev-<16hex>.<APPS_DOMAIN>` the reverse
	// tunnel binds to (from StartDevTunnel — never client-chosen).
	RemoteHost string
	// LocalPort is the developer's running dev server port on 127.0.0.1.
	LocalPort int
	// Signer is the ephemeral private key authenticating the `ssh -R` bind.
	Signer ssh.Signer
	// SSHHostPublicKey is the sish endpoint's OpenSSH host public-key line
	// (`ssh-ed25519 AAAA...`, from StartDevTunnel's sshHostPublicKey) that the
	// dialer PINS as its HostKeyCallback. The dialer FAILS CLOSED when this is
	// empty (refuses to connect) — it NEVER falls back to InsecureIgnoreHostKey.
	SSHHostPublicKey string
}

// Tunnel is a live reverse tunnel. Consumers select on Done (terminated),
// Activity (a connection arrived — resets the idle timer), and Close it on
// teardown.
type Tunnel interface {
	// Done is closed (or receives) when the tunnel terminates on its own (the
	// SSH connection dropped, the remote closed the forward, etc.).
	Done() <-chan struct{}
	// Activity fires (best-effort, coalesced) when a browser connection is
	// proxied through the tunnel, so the caller can treat the session as
	// non-idle. May be nil (a nil channel simply never fires).
	Activity() <-chan struct{}
	// Close tears the tunnel down (idempotent).
	Close() error
}

// Dialer opens a reverse tunnel. Behind an interface so the command's lifecycle
// is testable with a mock (no SSH, no network).
type Dialer interface {
	Dial(ctx context.Context, opts DialOptions) (Tunnel, error)
}

// Timer is the minimal clock seam the idle-timeout loop needs, so tests drive
// the timeout deterministically instead of sleeping.
type Timer interface {
	C() <-chan time.Time
	Reset(d time.Duration) bool
	Stop() bool
}

// realTimer adapts time.Timer to Timer.
type realTimer struct{ t *time.Timer }

func (r *realTimer) C() <-chan time.Time        { return r.t.C }
func (r *realTimer) Reset(d time.Duration) bool { return r.t.Reset(d) }
func (r *realTimer) Stop() bool                 { return r.t.Stop() }

// NewRealTimer is the production Timer factory (a wall-clock time.Timer).
func NewRealTimer(d time.Duration) Timer { return &realTimer{t: time.NewTimer(d)} }
