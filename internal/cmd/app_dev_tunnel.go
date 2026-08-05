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
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/devtunnel"
	"github.com/civitai/cli/internal/dnsprobe"
	"github.com/civitai/cli/internal/manifest"
	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/pkg/civitai"
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

// tunnelAPI is the subset of the API client the session core needs (seam for a
// mock in tests).
type tunnelAPI interface {
	StartDevTunnel(ctx context.Context, blockID, sshPublicKey string, declaredScopes []string) (*appapi.DevTunnelSession, error)
	StopDevTunnel(ctx context.Context, sessionID, blockID string) (bool, error)
	// WhoAmI resolves the signed-in identity — used to enrich a 403 mint refusal
	// with which account the CLI is authenticated as (the usual cause is being
	// logged in as the wrong account). The real *appapi.Client already implements it.
	WhoAmI(ctx context.Context) (*appapi.Identity, error)
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
	// probeEmbeddable inspects the local dev server's RESPONSE HEADERS to catch the
	// failures that leave a perfectly healthy tunnel serving an app the browser
	// refuses to run: no wildcard CORS (so the sandboxed null-origin iframe can't
	// fetch a single ES module), a framing header that forbids civitai.com, or a
	// host check that 403s the tunneled *.civit.ai Host. These are pure WARNINGS —
	// unlike probeLocal they never abort the session, because one HTTP response
	// can't rule out a proxy or an exotic-but-working setup. Nil skips the check.
	probeEmbeddable func(host string, port int) []devtunnel.Finding
	// checkParentOrigins reports a missing VITE_BLOCK_ALLOWED_PARENT_ORIGINS in the
	// project dir, which makes the SDK's IframeTransport ignore the host's
	// BLOCK_INIT — an error the app's error boundary swallows, so it is invisible
	// both in the browser and here. Unlike probeEmbeddable this is a STATIC read of
	// the project (Vite inlines the value at transform time, so it cannot be
	// observed over HTTP) and is gated to SDK app dirs. Nil skips the check.
	checkParentOrigins func(dir string) []devtunnel.Finding
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
	// declaredScopes are the LOCAL manifest's `scopes`, forwarded to
	// StartDevTunnel so the server can grant them to an UNSUBMITTED app's tunnel
	// token. Empty/nil = read-only (no spend) — never fatal.
	declaredScopes []string
	port           int
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

// maxDeclaredScopes / maxDeclaredScopeLen mirror the server's zod bound on
// blocks.startDevTunnel's declaredScopes input
// (z.array(z.string().min(1).max(64)).max(32)). Enforcing them CLIENT-side means
// an oversized/garbage local manifest degrades to the valid subset instead of
// being rejected with an opaque BAD_REQUEST — preserving the CLI's promise that a
// malformed manifest never blocks tunneling.
const (
	maxDeclaredScopes   = 32
	maxDeclaredScopeLen = 64
)

// boundDeclaredScopes filters the local manifest scopes down to what the server
// will accept: drop empty/whitespace-only entries and any over maxDeclaredScopeLen
// chars, dedupe (preserving first-seen order), and cap at maxDeclaredScopes. Returns
// nil when nothing valid remains so the request omits declaredScopes entirely.
func boundDeclaredScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(scopes))
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		if strings.TrimSpace(s) == "" || len(s) > maxDeclaredScopeLen {
			continue // empty/whitespace-only or too long → the server would reject it
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
		if len(out) == maxDeclaredScopes {
			break // at the cap — extras are silently dropped, not sent
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// sanitizeScopeForDisplay strips non-printable/control runes from a scope before
// it is echoed to the dev's terminal, so a crafted manifest can't inject ANSI /
// control sequences into their session. Display-only — the value SENT to the
// server is unchanged (the server sanitizes independently).
func sanitizeScopeForDisplay(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, s)
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
				return civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf("no token configured — run `civitai login` (or set CIVITAI_TOKEN)"))
			}

			client := appapi.NewWithSource(cfg.BaseURL(), auth.New(cfg), "")

			// Read the LOCAL manifest scopes (from the CWD — the dev runs this from
			// their App project dir) so the server can grant them to the tunnel token
			// of an UNSUBMITTED app (no submit needed). Degrade gracefully: a missing/
			// unreadable/malformed manifest (or one with no scopes) sends nothing — the
			// tunnel still works, just read-only for spend. This is NON-fatal even when
			// the blockId was given explicitly (flag/positional), so a bare directory
			// never blocks tunneling. boundDeclaredScopes enforces the server's zod
			// bound CLIENT-side so an oversized/garbage scopes array degrades to the
			// valid subset instead of 400ing the mint (keeping that "never blocks"
			// promise).
			declaredScopes := boundDeclaredScopes(manifest.LoadScopes("."))

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
			defer signal.Stop(sigCh)

			return runTunnelSession(context.Background(), tunnelSessionDeps{
				api:                    client,
				probeLocal:             probeLocalDevServer,
				probeEmbeddable:        probeEmbeddableDevServer,
				checkParentOrigins:     devtunnel.CheckParentOrigins,
				probePublic:            probePublicTunnel,
				probeLocalHop:          probeLocalHopTunnel,
				keygen:                 devtunnel.GenerateEphemeralKey,
				dialer:                 devtunnel.NewSSHDialer(cmd.ErrOrStderr()),
				blockID:                blockID,
				declaredScopes:         declaredScopes,
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

	// Embeddability checks run HERE — after probeLocal has established the server
	// is up (so a transport error means "can't observe", not "not running"), and
	// before the mint, so they cost nothing on the failure path. The findings are
	// deliberately NOT printed yet: they are rendered immediately before the
	// "open this URL" block below, because that is the moment the dev acts on
	// them. Printed here they would scroll away behind the readiness wait — the
	// silent-failure this whole check exists to end.
	var embedFindings []devtunnel.Finding
	if d.probeEmbeddable != nil {
		embedFindings = append(embedFindings, d.probeEmbeddable(d.localHost, d.port)...)
	}
	if d.checkParentOrigins != nil {
		embedFindings = append(embedFindings, d.checkParentOrigins(".")...)
	}

	// Immediate feedback: the mint round-trip + SSH dial below take a few seconds
	// with NOTHING printed otherwise, so the terminal looks hung. Emit a status line
	// the instant the session starts. Written to errw (status stream) as a single
	// plain line so the TTY and non-TTY paths are identical + greppable — we
	// deliberately do NOT wrap the mint+dial in an animated bubbletea spinner here:
	// runTunnelSession has an active signal.Notify(d.signals), and a default
	// bubbletea program (without WithoutSignalHandler) would race that handler for
	// the SIGINT/SIGTERM during this pre-serving phase. The reachability wait below
	// already animates the longer wait (waitTunnelTTY) with the signal-safe watcher
	// design.
	fmt.Fprintf(d.errw, "%s\n", ui.Dim(fmt.Sprintf("Establishing dev tunnel for %s… (minting session + opening SSH tunnel)", d.blockID)))

	// Transparency: surface exactly which scopes the tunnel is requesting (a
	// spend-consent signal, and it catches manifest typos before a click). Only
	// when the local manifest declares any — an empty set is read-only and needs
	// no notice. Sanitize for DISPLAY so a crafted manifest can't inject ANSI/
	// control sequences into the dev's terminal (the value SENT to the server is
	// unchanged — the server sanitizes independently).
	if len(d.declaredScopes) > 0 {
		display := make([]string, len(d.declaredScopes))
		for i, s := range d.declaredScopes {
			display[i] = sanitizeScopeForDisplay(s)
		}
		fmt.Fprintf(d.errw, "%s\n", ui.Dim(fmt.Sprintf("Declaring scopes: %s", strings.Join(display, ", "))))
	}

	key, err := d.keygen()
	if err != nil {
		return fmt.Errorf("generate ephemeral tunnel key: %w", err)
	}

	sess, err := d.api.StartDevTunnel(ctx, d.blockID, key.AuthorizedKey, d.declaredScopes)
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
		fmt.Fprintf(d.errw, "\n%s\n", ui.Dim(fmt.Sprintf("Tearing down dev tunnel (%s)…", reason)))
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
		printEmbedWarnings(d.out, embedFindings)
		printTunnelReady(d.out, sess, d.localHost, d.port)
	} else {
		fmt.Fprintf(d.errw, "\nDev tunnel established — serving %s:%d as %s. Waiting for %s…\n", localHostForDisplay(d.localHost), d.port, sess.Host, sess.Host)
		ready, abortReason := waitForTunnelReachable(ctx, d, tunnel, sess.Host, sess.URL)
		if abortReason != "" {
			// The dev aborted (Ctrl-C / ctx) or the tunnel dropped DURING the wait —
			// tear down now rather than fall into the idle loop.
			_ = tunnel.Close()
			stop(abortReason)
			return nil
		}
		if ready {
			printEmbedWarnings(d.out, embedFindings)
			printTunnelReadyConfirmed(d.out, sess, d.localHost, d.port)
		} else {
			// A POSITIVE --ready-timeout cap elapsed (the default is indefinite, which
			// never lands here). NON-fatal: the host may still come up shortly, so keep
			// the tunnel and print the URL with a clear "not up yet" warning.
			printEmbedWarnings(d.out, embedFindings)
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
	var forbidden *appapi.DevTunnelForbiddenError
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

// probeEmbeddableDevServer is the production probeEmbeddable: inspect the local
// dev server's response headers for the conditions that make the browser refuse
// to run the app even though the tunnel is healthy. Warn-only by construction —
// CheckEmbeddable returns no findings when it cannot observe.
func probeEmbeddableDevServer(host string, port int) []devtunnel.Finding {
	return devtunnel.CheckEmbeddable(host, port, embedProbeTimeout)
}

// embedProbeTimeout caps the embeddability probe. Matches probeLocalDevServer's
// dial timeout: this runs against loopback right after that dial succeeded, so a
// slow answer means something is wrong, not far away.
const embedProbeTimeout = 2 * time.Second

// printEmbedWarnings renders embeddability findings immediately before the
// "open this URL" block. Placement is the point: `dev-tunnel` reporting "Ready"
// while the app never appears is the exact failure this ends, so the LAST thing
// on screen before the URL must be the reason it won't work.
func printEmbedWarnings(out io.Writer, findings []devtunnel.Finding) {
	for _, f := range findings {
		fmt.Fprintf(out, "\n%s\n", ui.Warn(f.Summary))
		for _, line := range f.Evidence {
			fmt.Fprintf(out, "%s\n", line)
		}
	}
	// One vite.config.ts block fixes CORS, framing and allowedHosts together, so
	// the findings usually SHARE a remediation. Print each DISTINCT fix once,
	// after all the evidence — repeating an eight-line snippet per finding buries
	// the evidence and reads like three unrelated problems.
	seen := make(map[string]bool, len(findings))
	for _, f := range findings {
		fix := strings.Join(f.Fix, "\n")
		if fix == "" || seen[fix] {
			continue
		}
		seen[fix] = true
		fmt.Fprintf(out, "\n%s\n", fix)
	}
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
func printTunnelReady(out io.Writer, sess *appapi.DevTunnelSession, localHost string, port int) {
	fmt.Fprintf(out, "\nDev tunnel ready — serving %s:%d as %s\n\n", localHostForDisplay(localHost), port, ui.Bold(sess.Host))
	fmt.Fprintf(out, "  ▶ Open this in your browser (you must be logged in):\n\n")
	fmt.Fprintf(out, "      %s\n\n", ui.URL(sess.URL))
	fmt.Fprintf(out, "  Press Ctrl-C to tear the tunnel down.\n\n")
}

// printTunnelReadyConfirmed is the ready path: the public host answered healthy,
// so lead with a ✓ confirmation then the standard "open this" block.
func printTunnelReadyConfirmed(out io.Writer, sess *appapi.DevTunnelSession, localHost string, port int) {
	fmt.Fprintf(out, "\n%s\n", ui.Success(fmt.Sprintf("Ready — %s is reachable (local hop verified).", sess.Host)))
	printTunnelReady(out, sess, localHost, port)
}

// printTunnelReadyTimeout is the readiness-timeout path: the host did not answer
// healthy within the deadline, but that is NON-fatal — DNS/Cloudflare/Traefik
// propagation can still land shortly. Print the URL with a clear warning so the
// dev knows to retry the browser in a moment rather than assume it's broken.
func printTunnelReadyTimeout(out io.Writer, sess *appapi.DevTunnelSession, localHost string, port int, waited time.Duration) {
	fmt.Fprintf(out, "\n%s\n", ui.Warn(fmt.Sprintf("%s isn't resolving/serving yet (waited %s).", sess.Host, waited)))
	fmt.Fprintf(out, "  This is usually just DNS + Cloudflare + route propagation — it can take a minute.\n")
	fmt.Fprintf(out, "  The tunnel is UP; open the URL and retry in a bit if it NXDOMAINs / 404s / 502s:\n\n")
	fmt.Fprintf(out, "      %s\n\n", ui.URL(sess.URL))
	fmt.Fprintf(out, "  Press Ctrl-C to tear the tunnel down.\n\n")
}

// resolveReadyPollInterval / resolveNudgeAfter / resolveQuietInterval apply the
// documented defaults when a dep leaves the value unset (zero), so a zero-value
// deps set (e.g. a focused unit test) still behaves sanely. (readyTimeout
// intentionally has NO resolver — 0 is the meaningful "indefinite" value, not
// "unset". The TTY spinner cadence is owned by bubbles/spinner, so spinnerInterval
// has no resolver either — the field is retained only for deps API compatibility.)
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

// waitForTunnelReachable polls the PUBLIC tunnel host until it starts serving,
// showing a live progress indicator on d.errw. By DEFAULT (readyTimeout == 0) it
// waits INDEFINITELY — until ready, Ctrl-C, or the tunnel drops; a POSITIVE
// readyTimeout re-imposes a non-fatal cap.
//
// Each probe checks the GATE (probePublic) and, when the gate is reachable and a
// probeLocalHop is wired, the LOCAL HOP (a subresource-shaped request the gate
// forwards to sish → the local dev server). FULL readiness = gate reachable AND the
// local hop is not a bad-gateway (or no local-hop probe wired). The indicator has
// three states — "waiting for <host> to come up", "waiting for DNS to publish" (DoH
// says the record isn't published yet, after a short grace), and "tunnel up —
// waiting for your local dev server" (gate up but the hop 502s) — plus a clear,
// rate-limited local-unreachable notice.
//
// The indicator degrades by destination:
//   - TTY (d.errw is a terminal): a bubbletea + bubbles/spinner program
//     (waitTunnelTTY) renders those states as a live spinner.
//   - non-TTY (piped/CI/tests): NO animation — a quiet, greppable heartbeat line at
//     most every quietInterval (waitTunnelQuiet). This is the path all the
//     readiness-wait unit tests drive, so its behavior + the (ready, reason) exit
//     contract are load-bearing. bubbletea NEVER runs on a non-TTY writer.
//
// Each probe runs on a cancelable context, so an abort (signal / tunnel-close /
// ctx) returns PROMPTLY even mid-probe. After nudgeAfter a one-time "taking longer
// than usual" nudge (with the URL) prints and the wait keeps going.
//
// Returns:
//   - (true, "")        the host became reachable (local hop verified) — ready block.
//   - (false, "")       a positive readyTimeout cap elapsed (NON-fatal).
//   - (false, <reason>) the dev aborted (Ctrl-C / ctx) or the tunnel dropped.
//
// A nil d.probePublic disables the wait (returns ready immediately).
func waitForTunnelReachable(ctx context.Context, d tunnelSessionDeps, tunnel devtunnel.Tunnel, host, tunnelURL string) (bool, string) {
	if d.probePublic == nil {
		return true, ""
	}
	if writerIsTTY(d.errw) {
		return waitTunnelTTY(ctx, d, tunnel, host, tunnelURL)
	}
	return waitTunnelQuiet(ctx, d, tunnel, host, tunnelURL)
}

// tunnelProbeResult is one combined gate+local-hop readiness probe outcome. Shared
// by both the quiet loop and the bubbletea model so the readiness decision + state
// classification are computed identically on both paths.
type tunnelProbeResult struct {
	gateReady    bool
	localHopDone bool // the local-hop probe returned a definitive status (err==nil)
	localOK      bool // the local hop responded non-bad-gateway
	detail       string
	err          error
}

// runTunnelReadinessProbe runs the gate probe and, when the gate is reachable and a
// probeLocalHop is wired, the local-hop probe, folding both into a tunnelProbeResult.
// A local-hop transport error (lerr != nil) is left inconclusive (localHopDone=false)
// so it is re-probed, not reported as local-unreachable.
func runTunnelReadinessProbe(pctx context.Context, d tunnelSessionDeps, host string) tunnelProbeResult {
	gateReady, detail, err := d.probePublic(pctx, host)
	res := tunnelProbeResult{gateReady: gateReady, detail: detail, err: err}
	if gateReady && d.probeLocalHop != nil {
		ok, ldetail, lerr := d.probeLocalHop(pctx, host)
		if lerr == nil {
			res.localHopDone = true
			res.localOK = ok
			res.detail = ldetail
		}
	}
	return res
}

// tunnelProbeReady reports FULL readiness: the gate is reachable AND the local hop
// is not a bad-gateway (or no local-hop probe is wired). This is the core fix —
// gate-only readiness (200/401/403) declared "ready" while the local dev server was
// dead behind sish → the browser silently 502'd.
func tunnelProbeReady(d tunnelSessionDeps, res tunnelProbeResult) bool {
	return res.gateReady && (d.probeLocalHop == nil || res.localOK)
}

// classifyNotReady maps a NOT-ready probe result to the two indicator flags:
// localHopDown (gate reachable but the local hop returned a bad-gateway) takes
// precedence; otherwise dnsPending is set when DoH reports the record isn't
// published yet. Shared by the quiet loop and the bubbletea model.
func classifyNotReady(res tunnelProbeResult) (localHopDown, dnsPending bool) {
	if res.gateReady && res.localHopDone && !res.localOK {
		return true, false
	}
	return false, isDNSPending(res.detail, res.err)
}

// printTunnelNudge writes the one-time "taking longer than usual" block (with the
// URL). Shared by the quiet loop; the TTY path renders the same wording above the
// spinner via tea.Printf.
func printTunnelNudge(w io.Writer, tunnelURL string) {
	fmt.Fprintf(w, "! Taking longer than usual. The tunnel IS live — you can try opening the URL now,\n")
	fmt.Fprintf(w, "  or keep waiting. (Ctrl-C to tear down.)\n\n")
	fmt.Fprintf(w, "      %s\n\n", tunnelURL)
}

// waitTunnelQuiet is the non-TTY readiness wait: a select loop that re-probes on a
// cancelable context and emits a quiet, greppable heartbeat at most every
// quietInterval (NOT one line per probe). No animation. It surfaces all three
// states — generic "still waiting", DNS-pending, and "tunnel up; waiting for your
// local dev server" — plus the rate-limited local-unreachable notice. This is the
// path all the readiness-wait unit tests drive; its behavior + the (ready, reason)
// exit contract are load-bearing.
func waitTunnelQuiet(ctx context.Context, d tunnelSessionDeps, tunnel devtunnel.Tunnel, host, tunnelURL string) (bool, string) {
	interval := resolveReadyPollInterval(d)
	start := time.Now()

	// One reusable probe goroutine + a cap-1 result channel (only one probe is ever
	// outstanding, so the goroutine never blocks on send after we've returned).
	resultCh := make(chan tunnelProbeResult, 1)
	var probeCancel context.CancelFunc
	startProbe := func() {
		pctx, cancel := context.WithCancel(ctx)
		probeCancel = cancel
		go func() { resultCh <- runTunnelReadinessProbe(pctx, d, host) }()
	}
	cancelProbe := func() {
		if probeCancel != nil {
			probeCancel()
			probeCancel = nil
		}
	}

	// Indicator state. localHopDown = gate reachable but the local hop 502s;
	// dnsPending = DoH authoritatively reports the host isn't published yet (shown
	// only after a short grace so a single first-probe NXDOMAIN doesn't flip it).
	localHopDown := false
	dnsPending := false
	dnsGrace := resolveDNSPendingGrace(d)
	showDNSPending := func() bool { return dnsPending && time.Since(start) >= dnsGrace }
	localTarget := fmt.Sprintf("%s:%d", localHostForDisplay(d.localHost), d.port)

	// localUnreachableNotice prints the clear, persistent "tunnel up but local dev
	// server unreachable" guidance, rate-limited to at most every localHopNoticeInterval.
	lastLocalNotice := time.Time{}
	localNoticeEvery := resolveLocalHopNoticeInterval(d)
	localUnreachableNotice := func() {
		now := time.Now()
		if !lastLocalNotice.IsZero() && now.Sub(lastLocalNotice) < localNoticeEvery {
			return
		}
		lastLocalNotice = now
		fmt.Fprintf(d.errw, "⚠ Tunnel is up, but your local dev server on %s is unreachable through the tunnel\n", localTarget)
		fmt.Fprintf(d.errw, "  — is it running, and is --local-host correct? (currently %q)\n", localHostForDisplay(d.localHost))
	}

	heartbeat := time.NewTicker(resolveQuietInterval(d))
	defer heartbeat.Stop()

	// Optional cap (only when readyTimeout > 0). A nil channel never fires, giving
	// the indefinite default.
	var deadlineC <-chan time.Time
	if d.readyTimeout > 0 {
		dt := time.NewTimer(d.readyTimeout)
		defer dt.Stop()
		deadlineC = dt.C
	}

	nudge := time.NewTimer(resolveNudgeAfter(d))
	defer nudge.Stop()

	// Scheduler for the NEXT probe after a not-ready result. Start it stopped/drained.
	schedule := time.NewTimer(0)
	if !schedule.Stop() {
		<-schedule.C
	}
	defer schedule.Stop()

	startProbe()

	for {
		select {
		case <-d.signals:
			cancelProbe()
			return false, "interrupt"
		case <-ctx.Done():
			cancelProbe()
			return false, "canceled"
		case <-tunnel.Done():
			cancelProbe()
			return false, "tunnel closed"
		case <-deadlineC:
			cancelProbe()
			return false, "" // positive-cap timeout — non-fatal
		case <-nudge.C:
			// Fire exactly once (the timer is one-shot; this arm is only reachable the
			// first time).
			printTunnelNudge(d.errw, tunnelURL)
		case <-heartbeat.C:
			switch {
			case localHopDown:
				fmt.Fprintf(d.errw, "  … tunnel up; still waiting for your local dev server on %s (%s)\n", localTarget, fmtMMSS(time.Since(start)))
			case showDNSPending():
				fmt.Fprintf(d.errw, "  … waiting for DNS to publish for %s (external-dns + Cloudflare, usually <1 min) (%s)\n", host, fmtMMSS(time.Since(start)))
			default:
				fmt.Fprintf(d.errw, "  … still waiting for %s (%s)\n", host, fmtMMSS(time.Since(start)))
			}
		case res := <-resultCh:
			cancelProbe()
			if tunnelProbeReady(d, res) {
				return true, ""
			}
			localHopDown, dnsPending = classifyNotReady(res)
			if localHopDown {
				localUnreachableNotice()
			}
			schedule.Reset(interval)
		case <-schedule.C:
			startProbe()
		}
	}
}

// ── TTY readiness wait (bubbletea + bubbles/spinner) ─────────────────────────

// probeResultMsg carries a completed combined readiness probe into the model.
type probeResultMsg struct{ res tunnelProbeResult }

// reprobeMsg fires after the poll interval to kick the next probe.
type reprobeMsg struct{}

// tunnelAbortMsg ends the wait with a teardown reason (interrupt / canceled /
// tunnel closed).
type tunnelAbortMsg struct{ reason string }

// tunnelDeadlineMsg fires when a POSITIVE readyTimeout cap elapses (non-fatal).
type tunnelDeadlineMsg struct{}

// tunnelNudgeMsg fires once after nudgeAfter.
type tunnelNudgeMsg struct{}

// tunnelWaitModel is the bubbletea model for the TTY readiness wait. It drives the
// SAME gate+local-hop probe machinery + state classification as waitTunnelQuiet,
// rendered as a live spinner. Pointer receiver so it retains the in-flight probe's
// cancel func across Update calls (to cancel it promptly on abort — snappy Ctrl-C).
type tunnelWaitModel struct {
	ctx       context.Context
	d         tunnelSessionDeps
	tunnel    devtunnel.Tunnel
	host      string
	tunnelURL string
	interval  time.Duration
	start     time.Time
	sp        spinner.Model

	localTarget string
	dnsGrace    time.Duration

	probeCancel      context.CancelFunc
	nudged           bool
	localHopDown     bool
	dnsPending       bool
	lastLocalNotice  time.Time
	localNoticeEvery time.Duration

	// outcome (read after the program exits).
	ready  bool
	reason string
}

// Init starts the spinner, the first probe, the one-shot nudge, and (when a
// positive cap is set) the deadline. The abort SOURCES (signals / ctx / tunnel) are
// NOT started here as fire-and-forget tea.Cmds — bubbletea never cancels a command
// goroutine on Quit, so a tea.Cmd parked reading the send-based, never-closed
// d.signals channel would survive the wait and STEAL the next SIGINT/SIGTERM from
// runTunnelSession's serving loop. Instead waitTunnelTTY owns those watchers on a
// WaitGroup and joins them before returning (see startTunnelWaitWatchers).
func (m *tunnelWaitModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.sp.Tick, m.nudgeCmd(), m.startProbe()}
	if m.d.readyTimeout > 0 {
		cmds = append(cmds, m.deadlineCmd())
	}
	return tea.Batch(cmds...)
}

// startProbe stores a fresh cancelable context for the in-flight probe and returns
// the command that runs the combined gate+local-hop probe.
func (m *tunnelWaitModel) startProbe() tea.Cmd {
	pctx, cancel := context.WithCancel(m.ctx)
	m.probeCancel = cancel
	d := m.d
	host := m.host
	return func() tea.Msg { return probeResultMsg{res: runTunnelReadinessProbe(pctx, d, host)} }
}

func (m *tunnelWaitModel) cancelProbe() {
	if m.probeCancel != nil {
		m.probeCancel()
		m.probeCancel = nil
	}
}

func (m *tunnelWaitModel) deadlineCmd() tea.Cmd {
	return tea.Tick(m.d.readyTimeout, func(time.Time) tea.Msg { return tunnelDeadlineMsg{} })
}

func (m *tunnelWaitModel) nudgeCmd() tea.Cmd {
	return tea.Tick(resolveNudgeAfter(m.d), func(time.Time) tea.Msg { return tunnelNudgeMsg{} })
}

// maybeLocalNotice returns a rate-limited tea.Printf command that renders the clear
// "local dev server unreachable" guidance ABOVE the spinner, or nil if the last
// notice was too recent.
func (m *tunnelWaitModel) maybeLocalNotice() tea.Cmd {
	now := time.Now()
	if !m.lastLocalNotice.IsZero() && now.Sub(m.lastLocalNotice) < m.localNoticeEvery {
		return nil
	}
	m.lastLocalNotice = now
	return tea.Printf("⚠ Tunnel is up, but your local dev server on %s is unreachable through the tunnel\n  — is it running, and is --local-host correct? (currently %q)",
		m.localTarget, localHostForDisplay(m.d.localHost))
}

func (m *tunnelWaitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.cancelProbe()
			m.ready, m.reason = false, "interrupt"
			return m, tea.Quit
		}
		return m, nil
	case tunnelAbortMsg:
		m.cancelProbe()
		m.ready, m.reason = false, msg.reason
		return m, tea.Quit
	case tunnelDeadlineMsg:
		// Positive-cap timeout — NON-fatal: (false, "").
		m.cancelProbe()
		m.ready, m.reason = false, ""
		return m, tea.Quit
	case tunnelNudgeMsg:
		if m.nudged {
			return m, nil
		}
		m.nudged = true
		// tea.Printf renders ABOVE the spinner without corrupting it.
		return m, tea.Printf("! Taking longer than usual. The tunnel IS live — you can try opening the URL now,\n  or keep waiting. (Ctrl-C to tear down.)\n\n      %s\n", m.tunnelURL)
	case probeResultMsg:
		m.cancelProbe()
		if tunnelProbeReady(m.d, msg.res) {
			m.ready, m.reason = true, ""
			return m, tea.Quit
		}
		var cmds []tea.Cmd
		m.localHopDown, m.dnsPending = classifyNotReady(msg.res)
		if m.localHopDown {
			if c := m.maybeLocalNotice(); c != nil {
				cmds = append(cmds, c)
			}
		}
		// Not ready — re-probe after the poll interval.
		cmds = append(cmds, tea.Tick(m.interval, func(time.Time) tea.Msg { return reprobeMsg{} }))
		return m, tea.Batch(cmds...)
	case reprobeMsg:
		return m, m.startProbe()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.sp, cmd = m.sp.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

func (m *tunnelWaitModel) View() string {
	elapsed := fmtMMSS(time.Since(m.start))
	switch {
	case m.localHopDown:
		return fmt.Sprintf("%s Tunnel up — waiting for your local dev server on %s… %s elapsed", m.sp.View(), m.localTarget, elapsed)
	case m.dnsPending && time.Since(m.start) >= m.dnsGrace:
		return fmt.Sprintf("%s Waiting for DNS to publish for %s (external-dns + Cloudflare, usually <1 min)… %s elapsed", m.sp.View(), m.host, elapsed)
	default:
		return fmt.Sprintf("%s Waiting for %s… %s elapsed", m.sp.View(), m.host, elapsed)
	}
}

// startTunnelWaitWatchers launches STOPPABLE goroutines that watch the abort
// sources (signals / ctx / tunnel) and feed the program the corresponding abort
// message via send (p.Send in production). It returns a stop func that closes the
// quit channel AND joins every watcher, GUARANTEEING none survives the wait — in
// particular, that no goroutine is left reading the send-based, single-receiver
// d.signals channel (which would otherwise steal the next signal from the caller's
// serving loop). watchCtx/watchTunnel are close-broadcast and would be benign leaks,
// but are joined too for tidiness + determinism.
//
// send may be called after the program has finished (a delivered abort racing the
// program's own Quit) — p.Send is a no-op once the program's context is done, so
// that is safe.
func startTunnelWaitWatchers(ctx context.Context, d tunnelSessionDeps, tunnel devtunnel.Tunnel, send func(tea.Msg)) (stop func()) {
	quit := make(chan struct{})
	var wg sync.WaitGroup

	watch := func(fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn()
		}()
	}

	// Signals: the ONLY send-based single-receiver source — MUST stop reading
	// d.signals when the wait ends so a later SIGINT/SIGTERM reaches the caller.
	if d.signals != nil {
		sig := d.signals
		watch(func() {
			select {
			case <-sig:
				send(tunnelAbortMsg{reason: "interrupt"})
			case <-quit:
			}
		})
	}
	// Ctx cancellation (close-broadcast).
	watch(func() {
		select {
		case <-ctx.Done():
			send(tunnelAbortMsg{reason: "canceled"})
		case <-quit:
		}
	})
	// Tunnel drop (close-broadcast).
	done := tunnel.Done()
	watch(func() {
		select {
		case <-done:
			send(tunnelAbortMsg{reason: "tunnel closed"})
		case <-quit:
		}
	})

	return func() {
		close(quit)
		wg.Wait()
	}
}

// waitTunnelTTY runs the bubbletea readiness spinner and returns the same
// (ready, reason) contract as waitTunnelQuiet. Only reached when d.errw is a real
// TTY, so it cannot run under the buffer-driven tests.
func waitTunnelTTY(ctx context.Context, d tunnelSessionDeps, tunnel devtunnel.Tunnel, host, tunnelURL string) (bool, string) {
	m := &tunnelWaitModel{
		ctx:              ctx,
		d:                d,
		tunnel:           tunnel,
		host:             host,
		tunnelURL:        tunnelURL,
		interval:         resolveReadyPollInterval(d),
		start:            time.Now(),
		sp:               ui.Spinner(),
		localTarget:      fmt.Sprintf("%s:%d", localHostForDisplay(d.localHost), d.port),
		dnsGrace:         resolveDNSPendingGrace(d),
		localNoticeEvery: resolveLocalHopNoticeInterval(d),
	}
	// Render to errw (status stream). WithoutSignalHandler so OUR reason strings
	// win — Ctrl-C is handled as KeyCtrlC, SIGTERM via the injected signals chan.
	p := tea.NewProgram(m, tea.WithOutput(d.errw), tea.WithoutSignalHandler())
	// Own the abort-source watchers so NONE survives this function (esp. the
	// d.signals reader) — join them before returning.
	stop := startTunnelWaitWatchers(ctx, d, tunnel, p.Send)
	_, err := p.Run()
	stop()
	if err != nil {
		// A bubbletea failure shouldn't strand the session — treat it as ready so the
		// caller prints the URL rather than tearing down (the tunnel is bound regardless).
		return true, ""
	}
	return m.ready, m.reason
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
func validateMintResponse(sess *appapi.DevTunnelSession, baseURL string) error {
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
