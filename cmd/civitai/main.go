// Command civitai is the unified Civitai CLI.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/cmd"
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
// stderr. Keep this scheme in sync with the "Exit codes" section of README.md.
const (
	exitOK          = 0 // success
	exitGeneric     = 1 // generic / unknown / unclassified failure
	exitUsage       = 2 // bad flags, bad flag value, or a request rejected as malformed (HTTP 400)
	exitAuth        = 3 // authentication/authorization: login required, token invalid/expired, missing scope
	exitNotFound    = 4 // the requested resource does not exist (HTTP 404)
	exitNetwork     = 5 // network/transport failure or service unavailable (dial/timeout, HTTP 502/503/504)
	exitRateLimited = 6 // throttled by the API (HTTP 429)
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
	// the server rejected as malformed (HTTP 400, api.ErrBadRequest — e.g. a bad
	// enum value caught by the API's own validation).
	case errors.Is(err, cmd.ErrUsage), errors.Is(err, api.ErrBadRequest):
		return exitUsage

	// Authentication / authorization: 401/403, the pre-flight "no token
	// configured" guards, a lost-scope credential, and OAuth device-login
	// failures.
	case errors.Is(err, api.ErrUnauthorized),
		errors.Is(err, api.ErrBuzzScope),
		isDeviceFlowErr(err):
		return exitAuth

	case errors.Is(err, api.ErrNotFound):
		return exitNotFound

	case errors.Is(err, api.ErrRateLimited):
		return exitRateLimited

	// Network/transport or service-availability failure: tagged 502/503/504 and
	// exhausted retries, plus raw transport errors (dial refused/reset, timeout,
	// deadline exceeded) that reach us unclassified.
	case errors.Is(err, api.ErrNetwork), isNetworkErr(err):
		return exitNetwork

	default:
		return exitGeneric
	}
}

// isDeviceFlowErr reports whether err is (or wraps) a terminal OAuth
// device-login failure — an authentication problem.
func isDeviceFlowErr(err error) bool {
	var d *api.DeviceFlowError
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
