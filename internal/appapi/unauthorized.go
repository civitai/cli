package appapi

import "fmt"

// The ONE 401 message every `appapi` route returns, and the remedy it names.
//
// # Why this is a function and not five string literals
//
// It was five. Four spelled the remedy `run `civitai login` (or set
// CIVITAI_TOKEN)` and the fifth — submissionsError — spelled it `run `civitai
// login“, silently dropping the environment-variable route for the one command
// group most likely to be run from CI. Nobody decided that; it is what an
// open-coded predicate does at N sites, wrong at N−1 of them in the same
// direction. Consolidating is what made the disagreement audible.
//
// # What it says that it did not before
//
// A 401 that reaches a user has ALREADY survived an automatic refresh — the
// token source refreshes transparently on a 401 (see `pkg/civitai/api.go`), so
// this message existing means the refresh token is dead too, and "log in again"
// can fail a second time for the same reason. The README's Troubleshooting table
// carried that diagnosis and the personal-key escape hatch; the binary did not,
// and a table is not reachable from a CI log. The wording of the key route is
// taken verbatim from the one surface that already had it right —
// `analytics.go`'s 403 arm — so the CLI names one way to do this, not two.
//
// 🔴 TestUnauthorizedCallersAreLedgered pins the caller set in BOTH directions:
// it fails when a new 401 arm is written without this helper, and when a
// ledgered one stops calling it. A helper nothing is required to use regenerates
// exactly the drift it was written to end.
const unauthorizedRemedy = "run `civitai login` (or set CIVITAI_TOKEN). " +
	"An OAuth token refreshes itself on a 401, so reaching this message means the refresh failed too — " +
	"if logging in again does not clear it, create a personal API key at " +
	"https://civitai.com/user/account and run `civitai login --token <key>`"

// unauthorizedError builds the 401 error for an appapi route.
//
// serverMsg is the server's own words, or "" where the caller has decided they
// are misleading and dropped them — `devTunnelError` does exactly that, and says
// why at its call site. Both shapes are deliberate; the remedy is identical
// either way, which is the whole point of the helper.
func unauthorizedError(serverMsg string) error {
	if serverMsg == "" {
		return fmt.Errorf("not logged in (401) — %s", unauthorizedRemedy)
	}
	return fmt.Errorf("not logged in (401): %s — %s", serverMsg, unauthorizedRemedy)
}
