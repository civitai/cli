package cmd

import (
	"context"
	"errors"
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
	"github.com/civitai/cli/internal/manifest"
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
	// defaultDevTunnelEndpoint is the live public sish SSH listener. Override with
	// --tunnel-endpoint or CIVITAI_DEV_TUNNEL_ENDPOINT for a different deployment.
	defaultDevTunnelEndpoint = "sish.civitai.com:2224"
	// devTunnelEndpointEnv overrides the endpoint from the environment.
	devTunnelEndpointEnv = "CIVITAI_DEV_TUNNEL_ENDPOINT"
)

// tunnelAPI is the subset of the API client the session core needs (seam for a
// mock in tests).
type tunnelAPI interface {
	StartDevTunnel(ctx context.Context, blockID, sshPublicKey string) (*api.DevTunnelSession, error)
	StopDevTunnel(ctx context.Context, sessionID, blockID string) (bool, error)
	// WhoAmI resolves the signed-in identity — used to enrich a 403 mint refusal
	// with which account the CLI is authenticated as (the usual cause is being
	// logged in as the wrong account). The real *api.Client already implements it.
	WhoAmI(ctx context.Context) (*api.Identity, error)
}

// tunnelSessionDeps are the injectable dependencies of runTunnelSession — every
// side effect (API, keygen, tunnel, clock, signals, IO) goes through here so the
// lifecycle is unit-testable with no network.
type tunnelSessionDeps struct {
	api tunnelAPI
	// probeLocal pre-flight-checks that the developer's local dev server is
	// actually listening on 127.0.0.1:<port> BEFORE anything is minted server-side.
	// A non-nil error aborts the session before keygen/StartDevTunnel, so a
	// not-running dev server fails fast with clear guidance instead of minting a
	// rate-limited/reaper-tracked session that would only serve connection-refused.
	probeLocal func(port int) error
	keygen     func() (*devtunnel.EphemeralKey, error)
	dialer     devtunnel.Dialer
	blockID    string
	port       int
	endpoint   string
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

The blockId is resolved from (in order): the ` + "`--block`" + ` flag, the positional
argument, then the ` + "`blockId`" + ` in ` + "`block.manifest.json`" + ` in the current
directory. Run it from your App project dir and you can omit the blockId entirely.

⚠️ GATED: dev tunnels are limited to invited Apps authors / moderators and are
guarded by a server kill-switch flag. The tunnel endpoint is live; if you are not
enrolled the mint reports "not available" — ask to be added to the cohort.`,
		Example: `  # In terminal 1: start the embeddable dev server.
  npm run dev:tunnel
  # In terminal 2: open the tunnel (Ctrl-C to tear down).
  civitai app dev-tunnel                 # blockId from block.manifest.json in the CWD
  civitai app dev-tunnel my-block
  civitai app dev-tunnel my-block --port 5173
  civitai app dev-tunnel --block my-block --idle-timeout 15m`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Precedence: --block flag > positional arg > block.manifest.json in the
			// CWD. The manifest fallback mirrors `civitai app submit` (which defaults
			// to the current directory) so a dev in their App project dir can run
			// `civitai app dev-tunnel` with no args.
			blockID := strings.TrimSpace(blockFlag)
			// If BOTH --block and a positional blockId were given and they DIFFER,
			// the positional is silently ignored (--block wins). Warn so the dev
			// isn't confused about which one took effect. Equal values are redundant,
			// not a conflict — stay silent.
			if blockFlag != "" && len(args) == 1 && strings.TrimSpace(args[0]) != strings.TrimSpace(blockFlag) {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"warning: both --block %q and positional blockId %q were given; using --block (%q)\n",
					strings.TrimSpace(blockFlag), strings.TrimSpace(args[0]), strings.TrimSpace(blockFlag))
			}
			if blockID == "" && len(args) == 1 {
				blockID = strings.TrimSpace(args[0])
			}
			if blockID == "" {
				// Fall back to the local project manifest. A missing/unreadable/
				// malformed manifest is NOT fatal here — fall through to the error
				// below so an absent manifest still yields the clear guidance.
				if m, merr := manifest.Load("."); merr == nil {
					blockID = strings.TrimSpace(m.BlockID)
				}
			}
			if blockID == "" {
				return fmt.Errorf("a blockId is required — pass it (`civitai app dev-tunnel my-block`), or run from an App directory containing %s (list your submitted apps with `civitai app status`)", manifest.Filename)
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
				api:        client,
				probeLocal: probeLocalDevServer,
				keygen:     devtunnel.GenerateEphemeralKey,
				dialer:     devtunnel.NewSSHDialer(cmd.ErrOrStderr()),
				blockID:    blockID,
				port:       port,
				endpoint:   ep,
				baseURL:    cfg.BaseURL(),
				idle:       idle,
				newTimer:   devtunnel.NewRealTimer,
				signals:    sigCh,
				out:        cmd.OutOrStdout(),
				errw:       cmd.ErrOrStderr(),
			})
		},
	}

	cmd.Flags().StringVar(&blockFlag, "block", "", "the blockId (app slug) to tunnel (or pass it positionally; defaults to the blockId in "+manifest.Filename+" in the CWD)")
	cmd.Flags().IntVar(&port, "port", defaultDevTunnelPort, "local dev-server port to tunnel (matches the scaffold's dev:tunnel)")
	cmd.Flags().StringVar(&endpoint, "tunnel-endpoint", "", "sish SSH endpoint host:port (default "+defaultDevTunnelEndpoint+", or $"+devTunnelEndpointEnv+")")
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
	// Pre-flight FIRST: fail fast if the local dev server isn't listening, BEFORE
	// minting anything server-side. Nothing to tear down yet — return directly.
	if d.probeLocal != nil {
		if err := d.probeLocal(d.port); err != nil {
			return err
		}
	}

	key, err := d.keygen()
	if err != nil {
		return fmt.Errorf("generate ephemeral tunnel key: %w", err)
	}

	sess, err := d.api.StartDevTunnel(ctx, d.blockID, key.AuthorizedKey)
	if err != nil {
		// A 403 mint refusal is the common "wrong account" case — enrich it with
		// the signed-in identity + how to switch accounts. This runs on the
		// mint-error path, BEFORE any tunnel dial, so a forbidden mint never dials.
		return enrichDevTunnelAuthError(ctx, d, err)
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

// enrichDevTunnelAuthError augments a 403 mint refusal with the identity the CLI is
// signed in as + how to switch accounts (the usual cause is "wrong account"). Best
// effort: a failed WhoAmI just omits the name. Non-forbidden errors pass through.
func enrichDevTunnelAuthError(ctx context.Context, d tunnelSessionDeps, err error) error {
	var forbidden *api.DevTunnelForbiddenError
	if !errors.As(err, &forbidden) {
		return err
	}
	who := ""
	if id, werr := d.api.WhoAmI(ctx); werr == nil && id != nil {
		who = fmt.Sprintf(" You are signed in as %s (id %d).", id.Username, id.ID)
	}
	return fmt.Errorf("%w.%s Switch accounts with `civitai login` (browser) or `civitai login --token <key>`, then re-run; check the active account any time with `civitai whoami`", err, who)
}

// probeLocalDevServer is the production probeLocal: a short TCP dial of the local
// dev server on <port>, trying BOTH loopback families (127.0.0.1 and ::1) so a
// server bound only to IPv6 (`--host localhost` → `::1` on a dual-stack box) is
// not falsely reported down. Uses the SAME dialer as the live tunnel proxy, so
// "probe passed" implies "the proxy can reach it too." A successful connect means
// the dev server is up; a refused connection / timeout is a HARD error with
// actionable guidance. Run BEFORE the mint so a not-running dev server never
// burns a rate-limited/reaper-tracked server session (which would only serve the
// browser connection-refused).
func probeLocalDevServer(port int) error {
	conn, err := devtunnel.DialLocalDevServer(port, 2*time.Second)
	if err != nil {
		return fmt.Errorf("no local dev server is listening on port %d (tried 127.0.0.1 and ::1) — start it first (e.g. `npm run dev:tunnel`), then re-run. (override the port with --port)", port)
	}
	_ = conn.Close()
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
