// Command civitai is the unified Civitai CLI.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/cmd"
	"github.com/civitai/cli/pkg/civitai"
)

// Build metadata, injected at release time via goreleaser ldflags
// (see .goreleaser.yaml / .github/workflows/release.yml). They default to
// "dev"/"none"/"unknown" for `go install` / plain source builds.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Process exit codes. Scripts can branch on the FAILURE KIND without parsing
// stderr.
//
// What each code MEANS is deliberately not restated here. It has one source —
// cmd.ExitCodeDocs() in internal/cmd/exitcodes_doc.go — which renders both the
// root `--help` section and README.md's exit-code table. These comments used to
// restate it and drifted: they still described 4 as "HTTP 404" and 2 as "bad
// flags" after PR #233 taught `app status <unknown-slug>` to exit 4 on an HTTP
// 200 and a missing required flag to exit 2. A third copy of a contract is a
// third thing to forget, so the names carry the label and the docs carry the
// contract. TestExitCodeConstantsMatchDocs pins the two sets against each other.
const (
	exitOK          = 0
	exitGeneric     = 1
	exitUsage       = 2
	exitAuth        = 3
	exitNotFound    = 4
	exitNetwork     = 5
	exitRateLimited = 6
)

func main() {
	cmd.SetBuildInfo(version, commit, date)
	if err := cmd.NewRootCmd().Execute(); err != nil {
		// 🔴 THIS LINE IS THE ONLY SOURCE OF THE "Error: " PREFIX. cobra prints
		// nothing — the root sets `SilenceErrors: true` (internal/cmd/root.go) —
		// so every non-nil error from Execute() is rendered here and nowhere
		// else. An earlier attempt to stop `app doctor` reading as a tool
		// failure edited the SENTINEL's text instead, which was a fix aimed at
		// the wrong emitter: the prefix survived untouched, and the test written
		// to prove otherwise asserted on `err.Error()` — the string as it exists
		// BEFORE this line prepends anything, so it was structurally incapable
		// of observing the property it was named for.
		fmt.Fprintln(os.Stderr, errorLine(err))
		os.Exit(exitCode(err))
	}
}

// errorLine renders a command error for stderr.
//
// 🔴 A VERDICT IS NOT A FAILURE, AND THE PREFIX IS WHAT SAYS WHICH. `civitai app
// doctor` returns non-nil when it successfully diagnoses an incomplete listing:
// the command did exactly its job, the report is already on stdout, and the
// non-zero exit is the ANSWER rather than a malfunction. Prefixing that with
// "Error: " makes a CI scraper watching stderr for `Error:` flag a run that
// worked as designed — and, worse, tells a human the tool broke.
//
// So the one sentinel that means "a verdict about the subject" renders bare.
// Everything else keeps the prefix, because everything else really is a failure.
//
// It is a WHITELIST of one, deliberately: a predicate like "any error carrying a
// documented exit code" would silently un-prefix future failures. Adding a
// second verdict here should be a decision someone makes on purpose.
func errorLine(err error) string {
	if errors.Is(err, cmd.ErrListingBlocked) {
		return err.Error()
	}
	return "Error: " + err.Error()
}

// exitCode maps a command error to a differentiated process exit code so
// scripts can branch on the KIND of failure. It only inspects the error's
// classification (via errors.Is/errors.As against the sentinels the internal
// packages attach) — it never changes what is printed to the user. A nil error
// is success (0); any error it cannot classify falls back to the generic code
// (1). It never panics.
func exitCode(err error) int {
	switch {
	case err == nil:
		return exitOK

	// Usage errors are about the invocation, not the API — checked first. This
	// covers both client-side usage mistakes (bad flag / malformed invocation,
	// tagged by the root command's FlagErrorFunc or asUsageError) and a request
	// the server rejected as malformed (HTTP 400, civitai.ErrBadRequest — e.g. a bad
	// enum value caught by the API's own validation).
	case errors.Is(err, cmd.ErrUsage), errors.Is(err, civitai.ErrBadRequest):
		return exitUsage

	// Authentication / authorization: 401/403, the pre-flight "no token
	// configured" guards, a lost-scope credential, and OAuth device-login
	// failures.
	case errors.Is(err, civitai.ErrUnauthorized),
		errors.Is(err, appapi.ErrBuzzScope),
		isDeviceFlowErr(err):
		return exitAuth

	case errors.Is(err, civitai.ErrNotFound):
		return exitNotFound

	case errors.Is(err, civitai.ErrRateLimited):
		return exitRateLimited

	// Network/transport or service-availability failure: tagged 502/503/504 and
	// exhausted retries, plus raw transport errors (dial refused/reset, timeout,
	// deadline exceeded) that reach us unclassified.
	case errors.Is(err, civitai.ErrNetwork), isNetworkErr(err):
		return exitNetwork

	default:
		return exitGeneric
	}
}

// isDeviceFlowErr reports whether err is (or wraps) a terminal OAuth
// device-login failure — an authentication problem.
func isDeviceFlowErr(err error) bool {
	var d *appapi.DeviceFlowError
	return errors.As(err, &d)
}

// isNetworkErr reports whether err is (or wraps) a raw transport/connection
// failure that was not otherwise classified: a net.Error contributed by the net
// stack, a refused/reset connection, or a deadline/timeout.
//
// 🔴 A FILESYSTEM ERROR IS NOT A NETWORK ERROR, and the standard library makes
// that startlingly easy to get wrong. `syscall.Errno` carries both
// `Timeout() bool` and `Temporary() bool`, so it satisfies `net.Error` — while
// `*fs.PathError`, `*os.LinkError` and `*os.SyscallError` do NOT (measured on
// go1.25.12: none of the three declares `Temporary()`). A plain
// `errors.As(err, &netErr)` therefore unwraps straight PAST the filesystem
// wrapper and matches the bare Errno underneath it, so EVERY untagged
// os.ReadFile / os.Stat / os.MkdirAll failure in the CLI landed on exit 5 —
// the one code the README tells scripts to RETRY on. Measured before the fix:
// `app listing set-icon <mode-000 png>` and `login --token …` against an
// unwritable XDG_CONFIG_HOME both exited 5 with a `permission denied` message,
// so a CI retry loop span forever on a chmod problem. They now fall through to
// the generic code 1: the CLI has no filesystem category, and 1 is documented
// as "generic / unclassified", which is exactly what an opaque errno is. It is
// deliberately NOT 2 — exit 2 is a mistake about the INVOCATION, and the
// published contract already says in so many words that an unreadable-but-
// present file does not exit 2.
//
// 🔴 The walk itself lives in pkg/civitai (transport_error.go) and is reached
// through civitai.IsTransportError, because the SAME predicate was open-coded
// in this file and in that package's retry loop — and only this copy got fixed
// (issue #241 → PR #242), leaving the retry loop retrying filesystem failures
// until issue #244. One rule, one place. Do not re-inline it here.
//
// 🔴 The residual, stated rather than hidden — and it is WIDER than the "a bare
// syscall.ETIMEDOUT, say" this comment used to name. The rule, not a list:
// EVERY network errno except ECONNREFUSED and ECONNRESET (which keep explicit
// sentinels below) now falls to 1 when it reaches us with no net-stack wrapper,
// and equally when wrapped in an `*os.SyscallError` with no `*net.OpError`
// above it — an `*os.SyscallError` is not itself a net.Error, so the walk
// unwraps to the Errno and skips it. Measured on go1.25.12, `isNetworkErr` is
// false for all ten of bare ETIMEDOUT, EHOSTUNREACH, ENETUNREACH, EPIPE,
// ECONNABORTED, ENETDOWN, ENETRESET, EHOSTDOWN, EAGAIN and EWOULDBLOCK, and for
// each of them under `os.NewSyscallError`; each becomes exit 5 again the moment
// a `*net.OpError` sits above it, which is the only shape the net stack
// produces. Do not re-narrow this to a list of four — an enumerated set is what
// went stale here the first time.
//
// Nothing in this CLI produces those shapes — net/http surfaces a
// dial failure as *url.Error → *net.OpError → …, and both of those are real
// net.Errors — and the alternative is to keep reading an errno's
// Timeout()/Temporary() as evidence, which is what mis-sorted every filesystem
// failure in the first place. ECONNREFUSED/ECONNRESET keep their explicit
// bare-form checks above, because their Timeout() and Temporary() are BOTH
// false and the interface test never caught them anyway.
func isNetworkErr(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	return civitai.IsTransportError(err)
}
