// Command civitai is the unified Civitai CLI.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
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
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(exitCode(err))
	}
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
// go1.25: none of the three declares `Temporary()`). A plain
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
// The residual, stated rather than hidden: a network errno that reaches us
// with NO net-stack wrapper at all now falls to 1 as well (a bare
// syscall.ETIMEDOUT, say). Nothing in this CLI produces that shape —
// net/http surfaces a dial failure as *url.Error → *net.OpError → …, and both
// of those are real net.Errors — and the alternative is to keep reading an
// errno's Timeout()/Temporary() as evidence, which is what mis-sorted every
// filesystem failure in the first place. ECONNREFUSED/ECONNRESET keep their
// explicit bare-form checks above, because their Timeout() and Temporary() are
// BOTH false and the interface test never caught them anyway.
func isNetworkErr(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	return hasTransportError(err)
}

// hasTransportError walks err's tree — both the `Unwrap() error` and
// `Unwrap() []error` shapes, so it sees everything errors.As would — looking
// for a net.Error that the NET STACK contributed.
//
// Two rules, and neither subsumes the other:
//
//   - A bare `syscall.Errno` is not evidence. It satisfies net.Error by
//     accident of having Timeout()/Temporary(); the walk SKIPS it and keeps
//     going rather than stopping, so a genuine net.Error elsewhere in a
//     multi-error tree is still found.
//   - An error ABOUT A PATH is a filesystem error, full stop. `*fs.PathError`
//     and `*os.LinkError` terminate the walk, so nothing beneath one can be
//     read as transport evidence. Today that is belt-and-braces (they bottom
//     out in an Errno the first rule already rejects); it is here because a
//     future Go release adding `Temporary()` to either type would otherwise
//     re-open this exact hole silently, matching the wrapper itself instead of
//     the Errno. No net API returns either type.
//
// `*os.SyscallError` is deliberately NOT a terminator: the net stack nests one
// inside *net.OpError on every dial failure, and the OpError — a real
// net.Error — is found before the walk ever reaches it.
func hasTransportError(err error) bool {
	for err != nil {
		switch err.(type) {
		case *fs.PathError, *os.LinkError:
			return false
		}
		if ne, ok := err.(net.Error); ok {
			if _, isErrno := ne.(syscall.Errno); !isErrno {
				return true
			}
		}
		switch x := err.(type) {
		case interface{ Unwrap() error }:
			err = x.Unwrap()
		case interface{ Unwrap() []error }:
			for _, e := range x.Unwrap() {
				if hasTransportError(e) {
					return true
				}
			}
			return false
		default:
			return false
		}
	}
	return false
}
