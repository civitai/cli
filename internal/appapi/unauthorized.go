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
// A 401 that reaches a user on an OAuth login has already been through the
// refresh-and-retry at `appblocks.go:580`, whichever way that went: the refresh
// errored (no refresh token, or the grant failed) and the original 401 stands, or
// it succeeded and the retry 401'd anyway. Both mean "log in again" may not clear
// it. 🔴 THE MESSAGE MUST NOT NAME WHICH — two earlier wordings did, and each was
// false about the other case: "the refresh failed too" is false when it
// succeeded, and "it has already failed twice" is false when no retry ever ran.
// A personal key never refreshes at all (`auth/source.go` returns ErrNoRefresh),
// which is why the clause is conditioned on OAuth. The README's Troubleshooting table
// carried that diagnosis and the personal-key escape hatch; no 401 in THIS
// package did, and a table is not reachable from a CI log.
//
// ⚠ NARROWER THAN AN EARLIER DRAFT OF THIS COMMENT CLAIMED. It said the key
// route was carried by "no user-facing surface" and that this wording is
// "verbatim from the one surface that already had it right". Both are false:
// `genapi/errors.go` carries both routes on its own 401, `analytics.go`'s 403
// carries the key route in a different spelling, and roughly ten surfaces name
// it in several. This is therefore ANOTHER copy, not a unification — and
// `internal/cmd/login.go`'s `spendCredentialRoutes` already states the
// convention that governs it: other packages cannot import that constant, so
// their wording is corrected in place and must be KEPT IN STEP by hand. Nothing
// pins the two against each other; that is a known, accepted cost here, not an
// oversight to discover later.
//
// 🔴 TestNo401SiteBuildsItsOwnMessage (unauthorized_arms_test.go) is what makes
// this mandatory rather than advisory: it fails when any region conditioned on
// StatusUnauthorized constructs its own message. A helper nothing is REQUIRED to
// use regenerates exactly the drift it was written to end.
//
// ⚠ It is not a per-file ledger and there is no longer one. An earlier draft of
// this comment named `TestUnauthorizedCallersAreLedgered`, which the same commit
// that wrote this sentence DELETED — so the source pointed at a guard that did
// not exist. The replacement pins message construction plus a site-count floor;
// it does not pin which files answer a 401, so moving every arm into a new file
// keeps it silent.
const unauthorizedRemedy = "run `civitai login` (or set CIVITAI_TOKEN). " +
	"On an OAuth login the CLI has already tried to refresh and retry, so logging in again may not clear " +
	"it — create a personal API key at " +
	"https://civitai.com/user/account and run `civitai login --token <key>`"

// unauthorizedError builds the 401 error for an appapi route.
//
// serverMsg is the server's own words, or "" where the caller drops them. TWO
// callers pass "", and they are not equally well justified: `devTunnelError`
// drops the origin-gate string and says why at its call site; `listingError`
// computes `msg` and discards it on the 401 arm with no comment at all, which
// predates this helper and is recorded here rather than silently tidied — the
// drop may well be right, but nothing says so. Both shapes are supported; the
// remedy is identical either way, which is the point of the helper.
func unauthorizedError(serverMsg string) error {
	if serverMsg == "" {
		return fmt.Errorf("not logged in (401) — %s", unauthorizedRemedy)
	}
	return fmt.Errorf("not logged in (401): %s — %s", serverMsg, unauthorizedRemedy)
}
