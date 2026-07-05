package cmd

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/devtunnel"
	"github.com/spf13/cobra"
)

// devHostPrefixRE mirrors the P1 DEV_HOST_LABEL_REGEX shape (dev-tunnel-session.ts):
// the server-assigned host MUST be `dev-<16hex>.<domain>`. Defense-in-depth — a
// compromised/malicious mint response must not be able to steer the reverse-
// forward bind name to an arbitrary host.
var devHostPrefixRE = regexp.MustCompile(`^dev-[a-f0-9]{16}\.`)

// Dev-tunnel defaults.
const (
	// defaultDevTunnelPort matches the scaffold's pinned dev-server port
	// (page-money `dev:harness`/`dev:live`/`dev:tunnel` all use 5186).
	defaultDevTunnelPort = 5186
	// defaultDevTunnelIdle is the client-side idle timeout (design §9: 30m). The
	// server reaper is the authoritative backstop; this is the CLI doing its part.
	defaultDevTunnelIdle = 30 * time.Minute
	// defaultDevTunnelEndpoint is the public sish SSH listener. ⚠️ PLACEHOLDER —
	// the real endpoint is provisioned in P3 (a fresh port on the shared proxy,
	// design Revision #1/#2). Override with --tunnel-endpoint or
	// CIVITAI_DEV_TUNNEL_ENDPOINT until then; the command is inert end-to-end
	// without it.
	defaultDevTunnelEndpoint = "sish.civitai.com:2224"
	// devTunnelEndpointEnv overrides the endpoint from the environment.
	devTunnelEndpointEnv = "CIVITAI_DEV_TUNNEL_ENDPOINT"
)

// tunnelAPI is the subset of the API client the session core needs (seam for a
// mock in tests).
type tunnelAPI interface {
	StartDevTunnel(ctx context.Context, blockID, sshPublicKey string) (*api.DevTunnelSession, error)
	StopDevTunnel(ctx context.Context, sessionID, blockID string) (bool, error)
}

// tunnelSessionDeps are the injectable dependencies of runTunnelSession — every
// side effect (API, keygen, tunnel, clock, signals, IO) goes through here so the
// lifecycle is unit-testable with no network.
type tunnelSessionDeps struct {
	api      tunnelAPI
	keygen   func() (*devtunnel.EphemeralKey, error)
	dialer   devtunnel.Dialer
	blockID  string
	port     int
	endpoint string
	// baseURL is the configured Civitai origin; the mint response's URL must be
	// same-origin as this (defense-in-depth against an attacker-influenced URL).
	baseURL  string
	idle     time.Duration
	newTimer func(d time.Duration) devtunnel.Timer
	signals  <-chan os.Signal
	out      io.Writer
	errw     io.Writer
}

func newAppDevTunnelCmd() *cobra.Command {
	var blockFlag string
	var port int
	var endpoint string
	var idle time.Duration

	cmd := &cobra.Command{
		Use:   "dev-tunnel [blockId]",
		Short: "Preview your LOCAL dev server inside the real Civitai host via a hardened tunnel",
		Long: `Run your app locally (` + "`npm run dev:tunnel`" + `) and see it rendered INSIDE the
real production host at civitai.com/apps/dev/<blockId> — the actual page host
bridge, your real Buzz, real pickers, real session — but with the iframe pointing
at YOUR local code instead of a deployed bundle. A prod-fidelity inner-dev-loop.

How it works: this mints an EPHEMERAL ssh keypair (in memory — never written to
~/.ssh), calls blocks.startDevTunnel with the PUBLIC key, opens a reverse tunnel
(ssh -R) from your local dev-server port to the Civitai tunnel endpoint, and
prints the /apps/dev/<blockId> URL to open in your browser. On Ctrl-C (or an idle
timeout) it tears the tunnel down and revokes the session server-side.

Start your dev server first, in another terminal:

  npm run dev:tunnel            # serves your app on 127.0.0.1:5186, embeddable

then run this against the SAME port. Authentication uses your stored credential
(` + "`civitai login`" + ` or a personal API key); you can only tunnel your OWN app.

⚠️ PRE-GA / DARK: dev tunnels are gated behind an Apps-author invite AND a
kill-switch flag that is OFF today, and the public tunnel endpoint is not exposed
yet — so end-to-end this will report "not available" until it ships. The command,
its contract, and its teardown are complete; only the server flag + endpoint are
pending.`,
		Example: `  # In terminal 1: start the embeddable dev server.
  npm run dev:tunnel
  # In terminal 2: open the tunnel (Ctrl-C to tear down).
  civitai app dev-tunnel my-block
  civitai app dev-tunnel my-block --port 5173
  civitai app dev-tunnel --block my-block --idle-timeout 15m`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			blockID := strings.TrimSpace(blockFlag)
			if blockID == "" && len(args) == 1 {
				blockID = strings.TrimSpace(args[0])
			}
			if blockID == "" {
				return fmt.Errorf("a blockId is required — e.g. `civitai app dev-tunnel my-block` (find it with `civitai app status`)")
			}
			if port < 1 || port > 65535 {
				return fmt.Errorf("invalid --port %d (must be 1-65535)", port)
			}
			if idle <= 0 {
				return fmt.Errorf("--idle-timeout must be positive (got %s)", idle)
			}

			// Endpoint: flag > env > documented placeholder default.
			ep := strings.TrimSpace(endpoint)
			if ep == "" {
				ep = strings.TrimSpace(os.Getenv(devTunnelEndpointEnv))
			}
			if ep == "" {
				ep = defaultDevTunnelEndpoint
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Token() == "" {
				return fmt.Errorf("no token configured — run `civitai login` (or set CIVITAI_TOKEN)")
			}

			client := api.NewWithSource(cfg.BaseURL(), auth.New(cfg), "")

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
			defer signal.Stop(sigCh)

			return runTunnelSession(context.Background(), tunnelSessionDeps{
				api:      client,
				keygen:   devtunnel.GenerateEphemeralKey,
				dialer:   devtunnel.NewSSHDialer(cmd.ErrOrStderr()),
				blockID:  blockID,
				port:     port,
				endpoint: ep,
				baseURL:  cfg.BaseURL(),
				idle:     idle,
				newTimer: devtunnel.NewRealTimer,
				signals:  sigCh,
				out:      cmd.OutOrStdout(),
				errw:     cmd.ErrOrStderr(),
			})
		},
	}

	cmd.Flags().StringVar(&blockFlag, "block", "", "the blockId (app slug) to tunnel (or pass it positionally)")
	cmd.Flags().IntVar(&port, "port", defaultDevTunnelPort, "local dev-server port to tunnel (matches the scaffold's dev:tunnel)")
	cmd.Flags().StringVar(&endpoint, "tunnel-endpoint", "", "sish SSH endpoint host:port (default "+defaultDevTunnelEndpoint+", or $"+devTunnelEndpointEnv+"; P3-provisioned)")
	cmd.Flags().DurationVar(&idle, "idle-timeout", defaultDevTunnelIdle, "tear the tunnel down after this much inactivity")
	return cmd
}

// runTunnelSession is the testable core: mint an ephemeral key, start the server
// tunnel session, open the reverse tunnel, print the URL, then block until a
// signal / idle timeout / tunnel termination — tearing everything down (revoke
// the server session + close the tunnel) on every exit path. Every dependency is
// injected via deps so this is exercised with mocks (no SSH, no network, no real
// clock).
func runTunnelSession(ctx context.Context, d tunnelSessionDeps) error {
	key, err := d.keygen()
	if err != nil {
		return fmt.Errorf("generate ephemeral tunnel key: %w", err)
	}

	sess, err := d.api.StartDevTunnel(ctx, d.blockID, key.AuthorizedKey)
	if err != nil {
		return err
	}

	// Defense-in-depth: never trust the mint response blindly. The assigned host's
	// subdomain LABEL feeds the `ssh -R` bind (the dialer binds
	// subdomainLabel(host), NOT the full host — sish appends its own domain to the
	// requested subdomain, so binding the full `dev-<16hex>.civit.ai` would
	// double-append to `dev-<16hex>.civit.ai.civit.ai` and 404 the browser), and the
	// URL is what the developer clicks — so a compromised/malicious response must
	// not be able to steer either. Validate the host shape + that the URL is
	// same-origin as the configured base, and revoke the just-minted session on
	// rejection so nothing is orphaned.
	if verr := validateMintResponse(sess, d.baseURL); verr != nil {
		_, _ = d.api.StopDevTunnel(context.Background(), sess.SessionID, "")
		return verr
	}

	// From here a server session exists — guarantee teardown on every path.
	stop := func(reason string) {
		fmt.Fprintf(d.errw, "\nTearing down dev tunnel (%s)…\n", reason)
		// Use a fresh context so teardown still runs if ctx was canceled.
		if _, serr := d.api.StopDevTunnel(context.Background(), sess.SessionID, ""); serr != nil {
			fmt.Fprintf(d.errw, "warning: stopDevTunnel failed (the server reaper will reclaim it by %s): %v\n",
				time.Unix(sess.ExpiresAt, 0).Format(time.RFC3339), serr)
		}
	}

	tunnel, err := d.dialer.Dial(ctx, devtunnel.DialOptions{
		Endpoint:         d.endpoint,
		RemoteHost:       sess.Host,
		LocalPort:        d.port,
		Signer:           key.Signer,
		SSHHostPublicKey: sess.SSHHostPublicKey,
	})
	if err != nil {
		// The mint succeeded but we couldn't bind — revoke so we don't orphan the
		// route/credential.
		stop("tunnel dial failed")
		return fmt.Errorf("open reverse tunnel to %s: %w", d.endpoint, err)
	}

	printTunnelReady(d.out, sess, d.port)

	idleTimer := d.newTimer(d.idle)
	defer idleTimer.Stop()

	reason := "interrupt"
loop:
	for {
		select {
		case <-d.signals:
			reason = "interrupt"
			break loop
		case <-idleTimer.C():
			reason = fmt.Sprintf("idle for %s", d.idle)
			break loop
		case <-tunnel.Done():
			reason = "tunnel closed"
			break loop
		case <-tunnel.Activity():
			// A connection came through — the session is not idle; reset the timer.
			// Stop + drain any already-fired tick first (the standard time.Timer
			// idiom) so a stale tick can't trigger a spurious early teardown on the
			// next loop iteration.
			if !idleTimer.Stop() {
				select {
				case <-idleTimer.C():
				default:
				}
			}
			idleTimer.Reset(d.idle)
			continue
		case <-ctx.Done():
			reason = "canceled"
			break loop
		}
	}

	_ = tunnel.Close()
	stop(reason)
	return nil
}

// printTunnelReady prints the actionable "tunnel is up" block, with the
// /apps/dev URL made prominent (it is the one thing the developer must open).
func printTunnelReady(out io.Writer, sess *api.DevTunnelSession, port int) {
	fmt.Fprintf(out, "\nDev tunnel ready — serving 127.0.0.1:%d as %s\n\n", port, sess.Host)
	fmt.Fprintf(out, "  ▶ Open this in your browser (you must be logged in):\n\n")
	fmt.Fprintf(out, "      %s\n\n", sess.URL)
	fmt.Fprintf(out, "  Make sure your dev server is running (`npm run dev:tunnel`) on port %d.\n", port)
	fmt.Fprintf(out, "  Press Ctrl-C to tear the tunnel down.\n\n")
}

// validateMintResponse asserts the server-returned session is well-formed before
// the CLI acts on it: the host matches the P1 `dev-<16hex>.<domain>` shape (it
// becomes the reverse-forward bind name), the URL is same-origin (scheme + host)
// as the configured Civitai base (it is what the developer opens), and a sish
// host public key is present to PIN (it secures the `ssh -R` hop). Any failing
// is a hard error — a mint that tries to steer the bind name, hand the dev an
// attacker-influenced URL, or omit the host key to pin is refused, not followed.
//
// FAIL CLOSED (host key): an absent sshHostPublicKey is rejected here so the CLI
// never dials without a pinned host key. The dialer independently fails closed
// too (pinnedHostKeyCallback), so there is no reachable InsecureIgnoreHostKey
// path from either layer.
func validateMintResponse(sess *api.DevTunnelSession, baseURL string) error {
	if !devHostPrefixRE.MatchString(sess.Host) {
		return fmt.Errorf("server returned an unexpected tunnel host %q (want dev-<16hex>.<domain>) — refusing to bind", sess.Host)
	}
	if strings.TrimSpace(sess.SSHHostPublicKey) == "" {
		return fmt.Errorf("server did not provide a sish host key to pin; refusing to connect")
	}
	base, err := url.Parse(baseURL)
	if err != nil || base.Host == "" {
		return fmt.Errorf("cannot validate the tunnel URL against base %q: %v", baseURL, err)
	}
	u, err := url.Parse(sess.URL)
	if err != nil {
		return fmt.Errorf("server returned an unparseable URL %q — refusing to open", sess.URL)
	}
	if u.Scheme != base.Scheme || u.Host != base.Host {
		return fmt.Errorf("server returned a cross-origin URL %q (expected same origin as %s) — refusing to open", sess.URL, baseURL)
	}
	return nil
}
