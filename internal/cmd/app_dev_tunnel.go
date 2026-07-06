package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
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
	// defaultReadyTimeout bounds the post-dial readiness wait. The PUBLIC host
	// `dev-<16hex>.civit.ai` isn't reachable the instant the reverse tunnel binds —
	// external-dns + Cloudflare propagation + Traefik router build take up to
	// ~1–2 min. We poll until the server path is live (or this deadline), so the dev
	// isn't handed a URL that NXDOMAINs/404s. Non-fatal on expiry (the host may
	// still come up shortly) — override with --ready-timeout, skip with --no-wait.
	defaultReadyTimeout = 120 * time.Second
	// defaultReadyPollInterval is how often the readiness wait re-probes the public
	// host. ~5s balances responsiveness against hammering CF/Traefik while a route
	// is still being built.
	defaultReadyPollInterval = 5 * time.Second
	// probePublicTimeout bounds a single unauthenticated public-host GET so a hung
	// connection (SYN blackhole during propagation) can't stall a poll iteration.
	probePublicTimeout = 8 * time.Second
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
	// probePublic checks whether the PUBLIC tunnel host is serving yet: it does an
	// unauthenticated GET of https://<host>/ and reports ready per the classification
	// in probePublicTunnel (HTTP 200/401/403 = the whole server path works — a naked
	// no-token request is DENIED with 401 by the forwardAuth gate, which proves
	// DNS→Traefik-router→gate are all up; 404/5xx/52x/DNS/refused = not up yet). Wired
	// to probePublicTunnel in production; injected in tests. Nil disables the wait.
	probePublic func(ctx context.Context, host string) (ready bool, detail string, err error)
	keygen      func() (*devtunnel.EphemeralKey, error)
	dialer      devtunnel.Dialer
	blockID     string
	port        int
	endpoint    string
	// baseURL is the configured Civitai origin; the mint response's URL must be
	// same-origin as this (defense-in-depth against an attacker-influenced URL).
	baseURL string
	idle    time.Duration
	// readyTimeout bounds the post-dial readiness wait (default defaultReadyTimeout);
	// readyPollInterval is the re-probe cadence (default defaultReadyPollInterval).
	// noWait skips the wait entirely (print the URL immediately, as before).
	readyTimeout      time.Duration
	readyPollInterval time.Duration
	noWait            bool
	newTimer          func(d time.Duration) devtunnel.Timer
	signals           <-chan os.Signal
	out               io.Writer
	errw              io.Writer
}

func newAppDevTunnelCmd() *cobra.Command {
	var blockFlag string
	var port int
	var endpoint string
	var idle time.Duration
	var readyTimeout time.Duration
	var noWait bool

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
			if readyTimeout <= 0 {
				return fmt.Errorf("--ready-timeout must be positive (got %s)", readyTimeout)
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
				api:               client,
				probeLocal:        probeLocalDevServer,
				probePublic:       probePublicTunnel,
				keygen:            devtunnel.GenerateEphemeralKey,
				dialer:            devtunnel.NewSSHDialer(cmd.ErrOrStderr()),
				blockID:           blockID,
				port:              port,
				endpoint:          ep,
				baseURL:           cfg.BaseURL(),
				idle:              idle,
				readyTimeout:      readyTimeout,
				readyPollInterval: defaultReadyPollInterval,
				noWait:            noWait,
				newTimer:          devtunnel.NewRealTimer,
				signals:           sigCh,
				out:               cmd.OutOrStdout(),
				errw:              cmd.ErrOrStderr(),
			})
		},
	}

	cmd.Flags().StringVar(&blockFlag, "block", "", "the blockId (app slug) to tunnel (or pass it positionally; defaults to the blockId in "+manifest.Filename+" in the CWD)")
	cmd.Flags().IntVar(&port, "port", defaultDevTunnelPort, "local dev-server port to tunnel (matches the scaffold's dev:tunnel)")
	cmd.Flags().StringVar(&endpoint, "tunnel-endpoint", "", "sish SSH endpoint host:port (default "+defaultDevTunnelEndpoint+", or $"+devTunnelEndpointEnv+")")
	cmd.Flags().DurationVar(&idle, "idle-timeout", defaultDevTunnelIdle, "tear the tunnel down after this much inactivity")
	cmd.Flags().DurationVar(&readyTimeout, "ready-timeout", defaultReadyTimeout, "how long to wait for the public host to start serving before printing the URL anyway")
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "skip the readiness wait and print the URL immediately (it may 404/NXDOMAIN for ~1–2 min while DNS/route propagate)")
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

	// The reverse tunnel is bound, but the PUBLIC host is not reachable yet —
	// external-dns + Cloudflare + Traefik router readiness lag the bind by up to
	// ~1–2 min. Wait for the server path to actually serve before telling the dev to
	// open it, so they don't hit NXDOMAIN/404/502 on a too-early click. --no-wait
	// skips this and prints the URL immediately (old behavior).
	if d.noWait {
		printTunnelReady(d.out, sess, d.port)
	} else {
		fmt.Fprintf(d.errw, "\nDev tunnel established — serving 127.0.0.1:%d as %s. Waiting for it to become reachable…\n", d.port, sess.Host)
		ready, abortReason := waitForTunnelReachable(ctx, d, tunnel, sess.Host)
		if abortReason != "" {
			// The dev aborted (Ctrl-C / ctx) or the tunnel dropped DURING the wait —
			// tear down now rather than fall into the idle loop.
			_ = tunnel.Close()
			stop(abortReason)
			return nil
		}
		if ready {
			printTunnelReadyConfirmed(d.out, sess, d.port)
		} else {
			// Readiness timeout is NON-fatal: the host may still come up shortly, so
			// keep the tunnel and print the URL with a clear "not up yet" warning.
			printTunnelReadyTimeout(d.out, sess, d.port, resolveReadyTimeout(d))
		}
	}

	// Start the idle timer AFTER the readiness wait so the wait doesn't count
	// against the idle-timeout budget.
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
	// A token-SCOPE refusal is a different fix from the author/flag gate: the
	// credential needs FULL scope, not a different account. Don't misdirect the
	// user to account-switching — an OAuth `civitai login` token can't open dev
	// tunnels; a full-scope personal API key can.
	if forbidden.InsufficientScope {
		return fmt.Errorf("%w — this credential lacks Full scope. Create a full-scope personal API key at https://civitai.com/user/account, then `civitai login --token <key>` (an OAuth `civitai login` token can't open dev tunnels); check your credential + scopes with `civitai whoami`", err)
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

// printTunnelReadyConfirmed is the ready path: the public host answered healthy,
// so lead with a ✓ confirmation then the standard "open this" block.
func printTunnelReadyConfirmed(out io.Writer, sess *api.DevTunnelSession, port int) {
	fmt.Fprintf(out, "\n✓ Ready — %s is reachable.\n", sess.Host)
	printTunnelReady(out, sess, port)
}

// printTunnelReadyTimeout is the readiness-timeout path: the host did not answer
// healthy within the deadline, but that is NON-fatal — DNS/Cloudflare/Traefik
// propagation can still land shortly. Print the URL with a clear warning so the
// dev knows to retry the browser in a moment rather than assume it's broken.
func printTunnelReadyTimeout(out io.Writer, sess *api.DevTunnelSession, port int, waited time.Duration) {
	fmt.Fprintf(out, "\n⚠ %s isn't resolving/serving yet (waited %s).\n", sess.Host, waited)
	fmt.Fprintf(out, "  This is usually just DNS + Cloudflare + route propagation — it can take a minute.\n")
	fmt.Fprintf(out, "  The tunnel is UP; open the URL and retry in a bit if it NXDOMAINs / 404s / 502s:\n\n")
	fmt.Fprintf(out, "      %s\n\n", sess.URL)
	fmt.Fprintf(out, "  Make sure your dev server is running (`npm run dev:tunnel`) on port %d.\n", port)
	fmt.Fprintf(out, "  Press Ctrl-C to tear the tunnel down.\n\n")
}

// resolveReadyTimeout / resolveReadyPollInterval apply the documented defaults
// when a dep leaves the value unset (zero), so a zero-value deps set (e.g. a
// focused unit test) still behaves sanely.
func resolveReadyTimeout(d tunnelSessionDeps) time.Duration {
	if d.readyTimeout > 0 {
		return d.readyTimeout
	}
	return defaultReadyTimeout
}

func resolveReadyPollInterval(d tunnelSessionDeps) time.Duration {
	if d.readyPollInterval > 0 {
		return d.readyPollInterval
	}
	return defaultReadyPollInterval
}

// waitForTunnelReachable polls the PUBLIC tunnel host until it starts serving
// (d.probePublic reports ready) or the readiness deadline elapses, printing an
// unobtrusive progress line to d.errw on each not-ready attempt. It probes
// immediately, then every readyPollInterval.
//
// Returns:
//   - (true, "")           the host became reachable — print the ready block.
//   - (false, "")          the deadline elapsed (NON-fatal) — print the URL with a
//     "not up yet" warning and keep the tunnel.
//   - (false, <reason>)    the dev aborted (Ctrl-C / ctx canceled) or the tunnel
//     dropped mid-wait — the caller tears the session down with <reason>.
//
// A nil d.probePublic disables the wait (returns ready immediately).
func waitForTunnelReachable(ctx context.Context, d tunnelSessionDeps, tunnel devtunnel.Tunnel, host string) (bool, string) {
	if d.probePublic == nil {
		return true, ""
	}
	interval := resolveReadyPollInterval(d)
	deadline := time.NewTimer(resolveReadyTimeout(d))
	defer deadline.Stop()
	// Fire immediately for the first probe, then re-arm at `interval` after each
	// not-ready result.
	tick := time.NewTimer(0)
	defer tick.Stop()

	for {
		select {
		case <-d.signals:
			return false, "interrupt"
		case <-ctx.Done():
			return false, "canceled"
		case <-tunnel.Done():
			return false, "tunnel closed"
		case <-deadline.C:
			return false, "" // timeout — non-fatal
		case <-tick.C:
			ready, detail, err := d.probePublic(ctx, host)
			if ready {
				return true, ""
			}
			if detail == "" {
				if err != nil {
					detail = err.Error()
				} else {
					detail = "not ready"
				}
			}
			fmt.Fprintf(d.errw, "  … waiting for %s to come up (%s)\n", host, detail)
			tick.Reset(interval)
		}
	}
}

// probePublicTunnel is the production probePublic: an UNAUTHENTICATED GET of
// https://<host>/ (a fresh client that does NOT carry the CLI's credential), with
// the response classified by classifyReadyStatus.
//
// WHY 401/403 mean healthy: a naked (un-embedded, no-token) request to the tunnel
// host is DENIED by the forwardAuth gate with 401/403 precisely WHEN the whole
// server path works — DNS resolved, Traefik built the router, and the forwardAuth
// gate is reachable. 200 also = ready (the path serves). Everything else means the
// path isn't up yet: 404 (Traefik has no route), 502/503/504 (backend/gate down),
// Cloudflare origin errors 520–526, or a DNS/connection error.
func probePublicTunnel(ctx context.Context, host string) (bool, string, error) {
	// A dedicated client with NO auth transport — this is a public probe; it must
	// not reuse the CLI credential. Redirect-following is fine (default).
	client := &http.Client{Timeout: probePublicTimeout}
	return probePublicURL(ctx, client, "https://"+host+"/")
}

// probePublicURL is the testable core of probePublicTunnel: GET rawURL with the
// given client and classify. Split out so tests can point it at an httptest
// server (real statuses) without needing a real https host.
func probePublicURL(ctx context.Context, client *http.Client, rawURL string) (bool, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false, "bad-url", err
	}
	resp, err := client.Do(req)
	if err != nil {
		// DNS failure, connection refused, timeout — the host/route/backend isn't up.
		return false, classifyProbeErr(err), err
	}
	defer resp.Body.Close()
	// Drain a little so the connection can be reused / closed cleanly.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return classifyReadyStatus(resp.StatusCode), fmt.Sprintf("http %d", resp.StatusCode), nil
}

// classifyReadyStatus maps an HTTP status to readiness: only 200/401/403 mean the
// full server path is live (see probePublicTunnel). 404 (no route yet), 5xx, and
// Cloudflare 520–526 are explicitly NOT ready.
func classifyReadyStatus(code int) bool {
	switch code {
	case http.StatusOK, http.StatusUnauthorized, http.StatusForbidden:
		return true
	default:
		return false
	}
}

// classifyProbeErr gives a short human tag for a transport-level probe failure,
// for the progress line ("dns", "timeout", "unreachable").
func classifyProbeErr(err error) string {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return "timeout"
	}
	return "unreachable"
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
