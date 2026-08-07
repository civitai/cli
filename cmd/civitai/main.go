// Command civitai is the unified Civitai CLI.
package main

import (
	"context"
	"errors"
	"fmt"
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
// failure that was not otherwise classified: a net.Error, a refused/reset
// connection, or a deadline/timeout.
func isNetworkErr(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}
