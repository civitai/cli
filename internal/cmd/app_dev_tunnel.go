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
	"unicode/utf8"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/devtunnel"
	"github.com/civitai/cli/internal/dnsprobe"
	"github.com/civitai/cli/internal/manifest"
	"github.com/spf13/cobra"
	"golang.org/x/term"
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
	// ~1–2 min. The DEFAULT is 0 = wait INDEFINITELY (until ready, Ctrl-C, or the
	// tunnel drops) — the host reliably comes up, and aborting mid-propagation only
	// hands the dev a URL that NXDOMAINs/404s. A POSITIVE --ready-timeout re-imposes
	// a cap (warn + print the URL anyway on expiry, non-fatal); --no-wait skips the
	// wait entirely.
	defaultReadyTimeout = 0
	// defaultReadyPollInterval is how often the readiness wait re-probes the public
	// host. ~5s balances responsiveness against hammering CF/Traefik while a route
	// is still being built.
	defaultReadyPollInterval = 5 * time.Second
	// defaultNudgeAfter is how long the wait runs before printing the one-time
	// "taking longer than usual — the tunnel IS live, try the URL or keep waiting"
	// nudge. It fires exactly once, then the wait keeps going.
	defaultNudgeAfter = 2 * time.Minute
	// defaultSpinnerInterval is the TTY spinner frame cadence (smooth animation even
	// though probes are ~readyPollInterval apart). defaultQuietInterval is the
	// non-TTY (piped/CI) "still waiting" heartbeat cadence — a deterministic,
	// greppable line at most this often (NOT one-per-probe-attempt). Both are
	// injectable via deps so tests drive them fast without sleeps.
	defaultSpinnerInterval = 100 * time.Millisecond
	defaultQuietInterval   = 15 * time.Second
	// probePublicTimeout bounds a single unauthenticated public-host GET so a hung
	// connection (SYN blackhole during propagation) can't stall a poll iteration.
	probePublicTimeout = 8 * time.Second
	// defaultLocalHost is the local dev-server host the tunnel reaches by default:
	// "localhost" = loopback (127.0.0.1/::1), which PRESERVES the pre-flag behavior.
	// The scaffold's `dev:tunnel` binds localhost, so most users need nothing.
	defaultLocalHost = "localhost"
	// defaultLocalHopNoticeInterval rate-limits the readiness wait's "tunnel up but
	// local dev server unreachable" notice.
	defaultLocalHopNoticeInterval = 15 * time.Second
	// defaultDNSPendingGrace is how long the readiness wait shows the generic
	// "waiting for the host" indicator before switching to the DNS-specific
	// "waiting for DNS to publish for <host>…" line, once DoH reports the record
	// isn't published yet. A short grace avoids flipping the message on the very
	// first probe (the record is expected to be missing for the first second or
	// two right after mint).
	defaultDNSPendingGrace = 5 * time.Second
)

// spinnerFrames is the braille animation cycle for the TTY readiness indicator.
var spinnerFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

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
	// actually listening on <localHost>:<port> BEFORE anything is minted
	// server-side. A non-nil error aborts the session before keygen/StartDevTunnel,
	// so a not-running dev server fails fast with clear guidance instead of minting
	// a rate-limited/reaper-tracked session that would only serve connection-refused.
	probeLocal func(host string, port int) error
	// probePublic checks whether the PUBLIC tunnel host is serving yet: it does an
	// unauthenticated GET of https://<host>/ and reports ready per the classification
	// in probePublicTunnel (HTTP 200/401/403 = the whole server path works — a naked
	// no-token request is DENIED with 401 by the forwardAuth gate, which proves
	// DNS→Traefik-router→gate are all up; 404/5xx/52x/DNS/refused = not up yet). Wired
	// to probePublicTunnel in production; injected in tests. Nil disables the wait.
	probePublic func(ctx context.Context, host string) (ready bool, detail string, err error)
	// probeLocalHop checks whether a request can actually TRAVERSE the tunnel to the
	// local dev server — the gate-only probePublic can't (a naked GET is denied 401
	// by the forwardAuth gate before it ever reaches the backend). It sends a GET to
	// the public host with a NON-entry Sec-Fetch-Dest, which the gate treats as a
	// SUBRESOURCE and forwards to sish → the local server; a 502/503/504 means the
	// tunnel is up but the local server is unreachable (returns false), any other
	// status means the local hop works (returns true). Wired to probeLocalHopTunnel
	// in production; injected in tests. Nil skips the local-hop check (gate-only
	// readiness — used by lifecycle tests that don't exercise it).
	probeLocalHop func(ctx context.Context, host string) (localReachable bool, detail string, err error)
	keygen        func() (*devtunnel.EphemeralKey, error)
	dialer        devtunnel.Dialer
	blockID       string
	port          int
	// localHost is the resolved host the developer's dev server is bound to
	// ("localhost" by default = loopback; e.g. 10.42.0.100 for a container). Used
	// by BOTH the pre-flight probe and the live tunnel proxy so the two agree.
	localHost string
	endpoint  string
	// baseURL is the configured Civitai origin; the mint response's URL must be
	// same-origin as this (defense-in-depth against an attacker-influenced URL).
	baseURL string
	idle    time.Duration
	// readyTimeout bounds the post-dial readiness wait: 0 (the default) = wait
	// INDEFINITELY (until ready / Ctrl-C / tunnel-close); >0 = cap it (warn + print
	// the URL anyway on expiry, non-fatal). readyPollInterval is the re-probe cadence
	// (default defaultReadyPollInterval). noWait skips the wait entirely (print the
	// URL immediately, as before). nudgeAfter is the one-time "taking longer than
	// usual" nudge delay (default defaultNudgeAfter). spinnerInterval /
	// quietInterval tune the TTY spinner animation cadence and the non-TTY heartbeat
	// cadence respectively (defaults defaultSpinnerInterval / defaultQuietInterval);
	// all three are injectable so tests drive them fast without real sleeps.
	readyTimeout      time.Duration
	readyPollInterval time.Duration
	noWait            bool
	nudgeAfter        time.Duration
	spinnerInterval   time.Duration
	quietInterval     time.Duration
	// localHopNoticeInterval rate-limits the "tunnel up but local dev server
	// unreachable" notice during the readiness wait (default
	// defaultLocalHopNoticeInterval), so a stuck local hop is surfaced clearly and
	// repeatedly without spamming a line per poll. Injectable so tests drive it fast.
	localHopNoticeInterval time.Duration
	// dnsPendingGrace is how long the wait shows the generic indicator before
	// switching to the "waiting for DNS to publish" line once DoH reports the host
	// isn't published yet (default defaultDNSPendingGrace). Injectable so tests
	// drive it fast.
	dnsPendingGrace time.Duration
	newTimer        func(d time.Duration) devtunnel.Timer
	signals         <-chan os.Signal
	out             io.Writer
	errw            io.Writer
}

func newAppDevTunnelCmd() *cobra.Command {
	var blockFlag string
	var port int
	var localHost string
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
  # Dev server NOT on the CLI's loopback (a container/pod, VM, or bound interface):
  civitai app dev-tunnel my-block --local-host 10.42.0.100
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
			// Resolve the local host: empty falls back to the loopback default so a
			// `--local-host ""` can't accidentally break dialing.
			lh := strings.TrimSpace(localHost)
			if lh == "" {
				lh = defaultLocalHost
			}
			if idle <= 0 {
				return fmt.Errorf("--idle-timeout must be positive (got %s)", idle)
			}
			if readyTimeout < 0 {
				return fmt.Errorf("--ready-timeout must be >= 0 (0 = wait indefinitely until ready or Ctrl-C; got %s)", readyTimeout)
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
				api:                    client,
				probeLocal:             probeLocalDevServer,
				probePublic:            probePublicTunnel,
				probeLocalHop:          probeLocalHopTunnel,
				keygen:                 devtunnel.GenerateEphemeralKey,
				dialer:                 devtunnel.NewSSHDialer(cmd.ErrOrStderr()),
				blockID:                blockID,
				port:                   port,
				localHost:              lh,
				endpoint:               ep,
				baseURL:                cfg.BaseURL(),
				idle:                   idle,
				readyTimeout:           readyTimeout,
				readyPollInterval:      defaultReadyPollInterval,
				noWait:                 noWait,
				nudgeAfter:             defaultNudgeAfter,
				spinnerInterval:        defaultSpinnerInterval,
				quietInterval:          defaultQuietInterval,
				localHopNoticeInterval: defaultLocalHopNoticeInterval,
				dnsPendingGrace:        defaultDNSPendingGrace,
				newTimer:               devtunnel.NewRealTimer,
				signals:                sigCh,
				out:                    cmd.OutOrStdout(),
				errw:                   cmd.ErrOrStderr(),
			})
		},
	}

	cmd.Flags().StringVar(&blockFlag, "block", "", "the blockId (app slug) to tunnel (or pass it positionally; defaults to the blockId in "+manifest.Filename+" in the CWD)")
	cmd.Flags().IntVar(&port, "port", defaultDevTunnelPort, "local dev-server port to tunnel (matches the scaffold's dev:tunnel)")
	cmd.Flags().StringVar(&localHost, "local-host", defaultLocalHost, "host your local dev server is bound to. Default `localhost` (loopback) — the scaffold's `dev:tunnel` binds localhost, so most users need nothing. Set this for a dev server NOT on the CLI's loopback: a container/pod (e.g. --local-host 10.42.0.100), a VM, or a specific bound interface")
	cmd.Flags().StringVar(&endpoint, "tunnel-endpoint", "", "sish SSH endpoint host:port (default "+defaultDevTunnelEndpoint+", or $"+devTunnelEndpointEnv+")")
	cmd.Flags().DurationVar(&idle, "idle-timeout", defaultDevTunnelIdle, "tear the tunnel down after this much inactivity")
	cmd.Flags().DurationVar(&readyTimeout, "ready-timeout", defaultReadyTimeout, "cap the wait for the public host to start serving (0 = wait indefinitely until ready or Ctrl-C; a positive value warns + prints the URL anyway on expiry)")
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
		if err := d.probeLocal(d.localHost, d.port); err != nil {
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
		LocalHost:        d.localHost,
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
		printTunnelReady(d.out, sess, d.localHost, d.port)
	} else {
		fmt.Fprintf(d.errw, "\nDev tunnel established — serving %s:%d as %s. Waiting for it to become reachable…\n", localHostForDisplay(d.localHost), d.port, sess.Host)
		ready, abortReason := waitForTunnelReachable(ctx, d, tunnel, sess.Host, sess.URL)
		if abortReason != "" {
			// The dev aborted (Ctrl-C / ctx) or the tunnel dropped DURING the wait —
			// tear down now rather than fall into the idle loop.
			_ = tunnel.Close()
			stop(abortReason)
			return nil
		}
		if ready {
			printTunnelReadyConfirmed(d.out, sess, d.localHost, d.port)
		} else {
			// A POSITIVE --ready-timeout cap elapsed (the default is indefinite, which
			// never lands here). NON-fatal: the host may still come up shortly, so keep
			// the tunnel and print the URL with a clear "not up yet" warning.
			printTunnelReadyTimeout(d.out, sess, d.localHost, d.port, d.readyTimeout)
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
func probeLocalDevServer(host string, port int) error {
	conn, err := devtunnel.DialLocalDevServer(host, port, 2*time.Second)
	if err != nil {
		return fmt.Errorf("no local dev server is listening on %s:%d (%s) — start it first (e.g. `npm run dev:tunnel`), then re-run. (override the port with --port, or the host with --local-host)",
			localHostForDisplay(host), port, localDialTargets(host))
	}
	_ = conn.Close()
	return nil
}

// localHostForDisplay renders the local host for user-facing messages: an
// empty/"localhost" host shows as "localhost" (the loopback default).
func localHostForDisplay(host string) string {
	h := strings.TrimSpace(host)
	if h == "" || strings.EqualFold(h, "localhost") {
		return "localhost"
	}
	return h
}

// localDialTargets describes, for an error message, exactly what the dialer
// tried: both loopback families for the loopback default, else the single host.
func localDialTargets(host string) string {
	if h := strings.TrimSpace(host); h == "" || strings.EqualFold(h, "localhost") {
		return "tried 127.0.0.1 and ::1"
	}
	return "tried " + strings.TrimSpace(host)
}

// printTunnelReady prints the actionable "tunnel is up" block, with the
// /apps/dev URL made prominent (it is the one thing the developer must open).
func printTunnelReady(out io.Writer, sess *api.DevTunnelSession, localHost string, port int) {
	fmt.Fprintf(out, "\nDev tunnel ready — serving %s:%d as %s\n\n", localHostForDisplay(localHost), port, sess.Host)
	fmt.Fprintf(out, "  ▶ Open this in your browser (you must be logged in):\n\n")
	fmt.Fprintf(out, "      %s\n\n", sess.URL)
	fmt.Fprintf(out, "  Make sure your dev server is running (`npm run dev:tunnel`) on %s:%d.\n", localHostForDisplay(localHost), port)
	fmt.Fprintf(out, "  Press Ctrl-C to tear the tunnel down.\n\n")
}

// printTunnelReadyConfirmed is the ready path: the public host answered healthy,
// so lead with a ✓ confirmation then the standard "open this" block.
func printTunnelReadyConfirmed(out io.Writer, sess *api.DevTunnelSession, localHost string, port int) {
	fmt.Fprintf(out, "\n✓ Ready — %s is reachable (local hop verified).\n", sess.Host)
	printTunnelReady(out, sess, localHost, port)
}

// printTunnelReadyTimeout is the readiness-timeout path: the host did not answer
// healthy within the deadline, but that is NON-fatal — DNS/Cloudflare/Traefik
// propagation can still land shortly. Print the URL with a clear warning so the
// dev knows to retry the browser in a moment rather than assume it's broken.
func printTunnelReadyTimeout(out io.Writer, sess *api.DevTunnelSession, localHost string, port int, waited time.Duration) {
	fmt.Fprintf(out, "\n⚠ %s isn't resolving/serving yet (waited %s).\n", sess.Host, waited)
	fmt.Fprintf(out, "  This is usually just DNS + Cloudflare + route propagation — it can take a minute.\n")
	fmt.Fprintf(out, "  The tunnel is UP; open the URL and retry in a bit if it NXDOMAINs / 404s / 502s:\n\n")
	fmt.Fprintf(out, "      %s\n\n", sess.URL)
	fmt.Fprintf(out, "  Make sure your dev server is running (`npm run dev:tunnel`) on %s:%d.\n", localHostForDisplay(localHost), port)
	fmt.Fprintf(out, "  Press Ctrl-C to tear the tunnel down.\n\n")
}

// resolveReadyPollInterval / resolveNudgeAfter / resolveSpinnerInterval /
// resolveQuietInterval apply the documented defaults when a dep leaves the value
// unset (zero), so a zero-value deps set (e.g. a focused unit test) still behaves
// sanely. (readyTimeout intentionally has NO resolver — 0 is the meaningful
// "indefinite" value, not "unset".)
func resolveReadyPollInterval(d tunnelSessionDeps) time.Duration {
	if d.readyPollInterval > 0 {
		return d.readyPollInterval
	}
	return defaultReadyPollInterval
}

func resolveNudgeAfter(d tunnelSessionDeps) time.Duration {
	if d.nudgeAfter > 0 {
		return d.nudgeAfter
	}
	return defaultNudgeAfter
}

func resolveSpinnerInterval(d tunnelSessionDeps) time.Duration {
	if d.spinnerInterval > 0 {
		return d.spinnerInterval
	}
	return defaultSpinnerInterval
}

func resolveQuietInterval(d tunnelSessionDeps) time.Duration {
	if d.quietInterval > 0 {
		return d.quietInterval
	}
	return defaultQuietInterval
}

func resolveLocalHopNoticeInterval(d tunnelSessionDeps) time.Duration {
	if d.localHopNoticeInterval > 0 {
		return d.localHopNoticeInterval
	}
	return defaultLocalHopNoticeInterval
}

func resolveDNSPendingGrace(d tunnelSessionDeps) time.Duration {
	if d.dnsPendingGrace > 0 {
		return d.dnsPendingGrace
	}
	return defaultDNSPendingGrace
}

// writerIsTTY reports whether w is a terminal we can animate on — true only when
// w is an *os.File whose fd is a TTY. A bytes.Buffer / syncBuffer (tests) or a
// pipe (CI) is NOT a TTY, so it takes the quiet non-animated path.
func writerIsTTY(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// fmtMMSS renders an elapsed duration as M:SS (e.g. 2:07).
func fmtMMSS(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Seconds())
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}

// waitForTunnelReachable polls the PUBLIC tunnel host until it starts serving
// (d.probePublic reports ready), showing a live progress indicator on d.errw. By
// DEFAULT (readyTimeout == 0) it waits INDEFINITELY — until ready, Ctrl-C, or the
// tunnel drops; a POSITIVE readyTimeout re-imposes a non-fatal cap.
//
// The indicator degrades by destination:
//   - TTY (d.errw is a terminal): a braille spinner that rewrites ONE line in place
//     via \r, advancing every spinnerInterval, showing elapsed M:SS. Cleared before
//     any other block (nudge / ready / abort) is printed.
//   - non-TTY (piped/CI/tests): NO \r/animation — a quiet, greppable heartbeat line
//     at most every quietInterval ("  … still waiting for <host> (M:SS)").
//
// Each probe runs in its own goroutine on a cancelable context, so an abort
// (signal / tunnel-close / ctx) returns PROMPTLY even mid-probe (the in-flight
// probe's context is canceled rather than riding out its ~8s HTTP timeout).
//
// After nudgeAfter it prints a one-time "taking longer than usual" nudge (with the
// URL) and keeps waiting.
//
// Returns:
//   - (true, "")           the host became reachable — print the ready block.
//   - (false, "")          a positive readyTimeout cap elapsed (NON-fatal) — print
//     the URL with a "not up yet" warning and keep the tunnel.
//   - (false, <reason>)    the dev aborted (Ctrl-C / ctx canceled) or the tunnel
//     dropped mid-wait — the caller tears the session down with <reason>.
//
// A nil d.probePublic disables the wait (returns ready immediately).
func waitForTunnelReachable(ctx context.Context, d tunnelSessionDeps, tunnel devtunnel.Tunnel, host, tunnelURL string) (bool, string) {
	if d.probePublic == nil {
		return true, ""
	}
	interval := resolveReadyPollInterval(d)
	tty := writerIsTTY(d.errw)
	start := time.Now()

	// A single reusable probe goroutine + buffered result channel. Only one probe is
	// ever outstanding, so a cap-1 channel means the goroutine never blocks on send
	// after we've returned (no goroutine leak).
	//
	// Each probe first checks the GATE (probePublic). If the gate is reachable AND a
	// probeLocalHop is wired, it then checks the LOCAL HOP (a subresource-shaped
	// request that the forwardAuth gate forwards to sish → the local dev server):
	//   - gateReady=false                 → propagation not done; keep waiting.
	//   - gateReady + localHopDone+localOK → the FULL path works → ready.
	//   - gateReady + localHopDone+!localOK→ tunnel up but LOCAL server unreachable
	//     (502/503/504): surface the clear notice, keep waiting (NOT ready).
	//   - gateReady + !localHopDone        → local-hop probe errored transiently
	//     (or no probeLocalHop wired → treated as OK): re-probe.
	type probeResult struct {
		gateReady    bool
		localHopDone bool // the local-hop probe returned a definitive status (err==nil)
		localOK      bool // the local hop responded non-bad-gateway
		detail       string
		err          error
	}
	resultCh := make(chan probeResult, 1)
	var probeCancel context.CancelFunc
	startProbe := func() {
		pctx, cancel := context.WithCancel(ctx)
		probeCancel = cancel
		go func() {
			gateReady, detail, err := d.probePublic(pctx, host)
			res := probeResult{gateReady: gateReady, detail: detail, err: err}
			if gateReady && d.probeLocalHop != nil {
				ok, ldetail, lerr := d.probeLocalHop(pctx, host)
				if lerr == nil {
					res.localHopDone = true
					res.localOK = ok
					res.detail = ldetail
				}
				// A local-hop transport error (lerr != nil) is left inconclusive
				// (localHopDone=false) → re-probed, not reported as local-unreachable.
			}
			resultCh <- res
		}()
	}
	cancelProbe := func() {
		if probeCancel != nil {
			probeCancel()
			probeCancel = nil
		}
	}

	// Progress-indicator state (TTY only tracks lastLen so it can clear its line).
	// localHopDown is set once the gate is reachable but the local hop 502s, so the
	// indicator switches from "waiting for the host" to "waiting for your local dev
	// server" — the actionable state.
	frame := 0
	lastLen := 0
	localHopDown := false
	// dnsPending is set once DoH AUTHORITATIVELY reports the tunnel host isn't
	// published yet (external-dns + Cloudflare haven't propagated the record). It
	// switches the indicator to the DNS-specific line — but only after a short
	// grace period, so a single first-probe NXDOMAIN doesn't flip the message
	// instantly.
	dnsPending := false
	dnsGrace := resolveDNSPendingGrace(d)
	showDNSPending := func() bool { return dnsPending && time.Since(start) >= dnsGrace }
	localTarget := fmt.Sprintf("%s:%d", localHostForDisplay(d.localHost), d.port)
	renderSpinner := func() {
		if !tty {
			return
		}
		var line string
		switch {
		case localHopDown:
			line = fmt.Sprintf("%c Tunnel up — waiting for your local dev server on %s… %s elapsed",
				spinnerFrames[frame%len(spinnerFrames)], localTarget, fmtMMSS(time.Since(start)))
		case showDNSPending():
			line = fmt.Sprintf("%c Waiting for DNS to publish for %s (external-dns + Cloudflare, usually <1 min)… %s elapsed",
				spinnerFrames[frame%len(spinnerFrames)], host, fmtMMSS(time.Since(start)))
		default:
			line = fmt.Sprintf("%c Waiting for %s to come up… %s elapsed",
				spinnerFrames[frame%len(spinnerFrames)], host, fmtMMSS(time.Since(start)))
		}
		fmt.Fprintf(d.errw, "\r%s", line)
		lastLen = utf8.RuneCountInString(line)
	}
	clearLine := func() {
		if tty && lastLen > 0 {
			fmt.Fprintf(d.errw, "\r%s\r", strings.Repeat(" ", lastLen))
			lastLen = 0
		}
	}

	// localUnreachableNotice prints the CLEAR, persistent "tunnel up but local dev
	// server unreachable" guidance, rate-limited to at most every
	// localHopNoticeInterval so a stuck local hop is surfaced repeatedly without
	// one line per poll. On a TTY it clears the spinner line first, then repaints.
	lastLocalNotice := time.Time{}
	localNoticeEvery := resolveLocalHopNoticeInterval(d)
	localUnreachableNotice := func() {
		now := time.Now()
		if !lastLocalNotice.IsZero() && now.Sub(lastLocalNotice) < localNoticeEvery {
			return
		}
		lastLocalNotice = now
		clearLine()
		fmt.Fprintf(d.errw, "⚠ Tunnel is up, but your local dev server on %s is unreachable through the tunnel\n", localTarget)
		fmt.Fprintf(d.errw, "  — is it running, and is --local-host correct? (currently %q)\n", localHostForDisplay(d.localHost))
		if tty {
			renderSpinner()
		}
	}

	// UI ticker: fast spinner frames on a TTY, slow heartbeat otherwise.
	uiEvery := resolveSpinnerInterval(d)
	if !tty {
		uiEvery = resolveQuietInterval(d)
	}
	ui := time.NewTicker(uiEvery)
	defer ui.Stop()

	// Optional cap (only when readyTimeout > 0). A nil channel never fires, giving
	// the indefinite default.
	var deadlineC <-chan time.Time
	if d.readyTimeout > 0 {
		dt := time.NewTimer(d.readyTimeout)
		defer dt.Stop()
		deadlineC = dt.C
	}

	// One-time nudge timer.
	nudge := time.NewTimer(resolveNudgeAfter(d))
	defer nudge.Stop()

	// Scheduler for the NEXT probe after a not-ready result. Start it stopped/drained
	// — the first probe is kicked immediately below.
	schedule := time.NewTimer(0)
	if !schedule.Stop() {
		<-schedule.C
	}
	defer schedule.Stop()

	startProbe()    // probe immediately
	renderSpinner() // paint the initial spinner frame on a TTY

	for {
		select {
		case <-d.signals:
			cancelProbe()
			clearLine()
			return false, "interrupt"
		case <-ctx.Done():
			cancelProbe()
			clearLine()
			return false, "canceled"
		case <-tunnel.Done():
			cancelProbe()
			clearLine()
			return false, "tunnel closed"
		case <-deadlineC:
			cancelProbe()
			clearLine()
			return false, "" // positive-cap timeout — non-fatal
		case <-nudge.C:
			// Fire exactly once (the timer is one-shot; this arm is only reachable the
			// first time). Clear the spinner, print the nudge block, resume the spinner.
			clearLine()
			fmt.Fprintf(d.errw, "! Taking longer than usual. The tunnel IS live — you can try opening the URL now,\n")
			fmt.Fprintf(d.errw, "  or keep waiting. (Ctrl-C to tear down.)\n\n")
			fmt.Fprintf(d.errw, "      %s\n\n", tunnelURL)
			renderSpinner()
		case <-ui.C:
			switch {
			case tty:
				frame++
				renderSpinner()
			case localHopDown:
				fmt.Fprintf(d.errw, "  … tunnel up; still waiting for your local dev server on %s (%s)\n", localTarget, fmtMMSS(time.Since(start)))
			case showDNSPending():
				fmt.Fprintf(d.errw, "  … waiting for DNS to publish for %s (external-dns + Cloudflare, usually <1 min) (%s)\n", host, fmtMMSS(time.Since(start)))
			default:
				fmt.Fprintf(d.errw, "  … still waiting for %s (%s)\n", host, fmtMMSS(time.Since(start)))
			}
		case res := <-resultCh:
			// The probe returned — cancel its context (release resources) rather than
			// dropping the CancelFunc, honoring the always-call-cancel contract (a no-op
			// on the completed ctx today, but leak-safe if a cancelable parent is ever
			// threaded through runTunnelSession).
			cancelProbe()
			// FULLY ready only when the gate is reachable AND the local hop is not a
			// bad-gateway (or no local-hop probe is wired). This is the fix: gate-only
			// readiness (200/401/403) declared "ready" while the local dev server was
			// dead behind sish → the browser silently 502'd.
			if res.gateReady && (d.probeLocalHop == nil || res.localOK) {
				clearLine()
				return true, ""
			}
			if res.gateReady && res.localHopDone && !res.localOK {
				// Tunnel/gate up, but the local hop 502s — the local dev server is
				// unreachable through the tunnel. Surface the clear, persistent notice
				// and KEEP waiting (don't declare ready, don't hang silently).
				localHopDown = true
				dnsPending = false
				localUnreachableNotice()
			} else {
				// Gate not up yet (or the local-hop probe was inconclusive) — back to
				// the plain "waiting for the host" indicator. If DoH reports the record
				// isn't published yet, switch to the DNS-specific indicator (after the
				// grace period) so "DNS not up" reads differently from "route not built".
				localHopDown = false
				dnsPending = isDNSPending(res.detail, res.err)
			}
			// Not ready — re-probe after the poll interval. (The detail/err is folded
			// into the indicator, not spammed per-attempt.)
			schedule.Reset(interval)
		case <-schedule.C:
			startProbe()
		}
	}
}

// devTunnelResolver is the DoH resolver the readiness probes use to resolve the
// ephemeral tunnel host. It is a package var so tests can inject a fake DoH
// endpoint; production uses Cloudflare DoH JSON.
var devTunnelResolver dnsprobe.Resolver = dnsprobe.DefaultResolver

// dnsPendingDetail is the probe `detail` reported when DoH AUTHORITATIVELY says
// the tunnel host is not published yet (NXDOMAIN / no address answer) — DISTINCT
// from a transient "dns" transport error. The readiness wait keys off this (or the
// dnsprobe.ErrNotPublished sentinel) to show the "waiting for DNS to publish" line
// instead of the generic "still waiting".
const dnsPendingDetail = "dns-pending"

// probeHTTPClientForHost builds the probe HTTP client for host, resolving host via
// DNS-over-HTTPS to Cloudflare (NOT the OS resolver). This is the crux of the fix:
// a premature GET of the just-minted `dev-<16hex>.civit.ai` used to resolve via the
// OS resolver, which NXDOMAIN-negative-caches the not-yet-published record for up to
// the civit.ai SOA minimum TTL (1800s, CF-unlowerable) — hanging the CLI for ~30 min
// AND poisoning the OS cache so the browser can't later resolve the now-live host.
// Resolving over DoH sees the record the moment CF has it and NEVER poisons the OS
// negative cache. On an authoritative not-published result it returns
// dnsprobe.ErrNotPublished; if DoH itself fails it falls back to the OS resolver so
// behavior is never worse than before.
func probeHTTPClientForHost(ctx context.Context, host string) (*http.Client, error) {
	return dnsprobe.DialClient(ctx, devTunnelResolver, host, probePublicTimeout)
}

// probePublicTunnel is the production probePublic: an UNAUTHENTICATED GET of
// https://<host>/ (a fresh client that does NOT carry the CLI's credential), with
// the response classified by classifyReadyStatus. The host is resolved via DoH
// (see probeHTTPClientForHost) so the OS resolver is never queried for the
// not-yet-published host.
//
// WHY 401/403 mean healthy: a naked (un-embedded, no-token) request to the tunnel
// host is DENIED by the forwardAuth gate with 401/403 precisely WHEN the whole
// server path works — DNS resolved, Traefik built the router, and the forwardAuth
// gate is reachable. 200 also = ready (the path serves). Everything else means the
// path isn't up yet: 404 (Traefik has no route), 502/503/504 (backend/gate down),
// Cloudflare origin errors 520–526, or a DNS/connection error.
func probePublicTunnel(ctx context.Context, host string) (bool, string, error) {
	client, err := probeHTTPClientForHost(ctx, host)
	if err != nil {
		if errors.Is(err, dnsprobe.ErrNotPublished) {
			// DoH says the record isn't published yet — the expected state right after
			// mint. Report the DISTINCT dns-pending signal (not a hard error) so the
			// wait keeps polling and shows the DNS-specific message.
			return false, dnsPendingDetail, err
		}
		// Should not happen (DialClient falls back to the OS resolver on transient DoH
		// failure), but be defensive: treat an unexpected resolver error as not-ready.
		return false, "dns", err
	}
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

// probeLocalHopTunnel is the production probeLocalHop: it verifies a request can
// actually TRAVERSE the tunnel to the local dev server, which the gate-only
// probePublicTunnel cannot (a naked GET is denied 401 by the forwardAuth gate
// BEFORE it reaches the backend — so gate-reachable ≠ local-server-reachable, the
// silent-502 dogfood failure).
//
// HOW: it GETs https://<host>/ with a NON-entry Sec-Fetch-Dest ("image"). The
// forwardAuth gate (dev-tunnel-gate.ts) classifies ENTRY dests
// (document/iframe/frame/nested-document, or ABSENT) as token-required → 401, and
// treats ANY OTHER Sec-Fetch-Dest as a SUBRESOURCE → allowed through to the
// backend (sish → the local dev server). So the response status reflects the
// LOCAL hop: 502/503/504 = tunnel up but local server unreachable (not ready);
// any other status (200/404/…) = the local server responded (ready).
func probeLocalHopTunnel(ctx context.Context, host string) (bool, string, error) {
	// Resolve via DoH too (the gate probe already proved the host resolves, but
	// keeping the local-hop probe off the OS resolver means NEITHER probe can poison
	// the OS negative cache). A transient DoH failure falls back to the OS resolver.
	client, err := probeHTTPClientForHost(ctx, host)
	if err != nil {
		// Inconclusive (e.g. a transient not-published between gate-ready and this
		// probe) — report not-reachable-yet so the caller re-probes rather than
		// declaring the local hop down.
		return false, classifyProbeErr(err), err
	}
	return probeLocalHopURL(ctx, client, "https://"+host+"/")
}

// probeLocalHopURL is the testable core of probeLocalHopTunnel: GET rawURL with a
// subresource-shaped Sec-Fetch-Dest and classify per localHopReachable. Split out
// so tests can point it at an httptest server (real statuses) without a real host.
func probeLocalHopURL(ctx context.Context, client *http.Client, rawURL string) (bool, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false, "bad-url", err
	}
	// A non-entry Sec-Fetch-Dest makes the forwardAuth gate forward this to the
	// backend as a SUBRESOURCE (no token needed) — so the status reflects the local
	// hop, not the gate. Also mark it fetch-mode so it never looks like a navigation.
	req.Header.Set("Sec-Fetch-Dest", "image")
	req.Header.Set("Sec-Fetch-Mode", "no-cors")
	resp, err := client.Do(req)
	if err != nil {
		return false, classifyProbeErr(err), err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return localHopReachable(resp.StatusCode), fmt.Sprintf("http %d", resp.StatusCode), nil
}

// localHopReachable classifies a SUBRESOURCE-probe status: only the bad-gateway
// family (502/503/504) means the tunnel is up but the local dev server is
// unreachable; ANY other status means the local server responded (the hop works).
func localHopReachable(code int) bool {
	switch code {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return false
	default:
		return true
	}
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

// isDNSPending reports whether a probe result means "DNS isn't published yet"
// (DoH returned NXDOMAIN / no address answer) as opposed to any other not-ready
// reason. It accepts EITHER the dnsPendingDetail marker (so injected test probes
// need not import the sentinel) OR the dnsprobe.ErrNotPublished sentinel.
func isDNSPending(detail string, err error) bool {
	return detail == dnsPendingDetail || errors.Is(err, dnsprobe.ErrNotPublished)
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
