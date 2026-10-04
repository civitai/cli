package appapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/civitai/cli/pkg/civitai"
)

// The App-Blocks ROLLOUT probe — the ACCOUNT-level question `civitai app
// doctor` could not answer before civitai/civitai-app-starters#537.
//
// 🔴 THE PROBLEM IT SOLVES IS A WRONG-SUBJECT DIAGNOSIS, NOT A MISSING FEATURE.
// A developer's block storage calls answered `401 {"error":"Apps are not
// enabled"}`. They verified the app was approved and that the token carried both
// storage scopes, and still could not explain it — because the gate is NEITHER
// of those things. The host's `app-blocks-enabled` flag is off by default and
// resolves true only for specific cohorts, evaluated against a USER IDENTITY, so
// it is orthogonal to app approval and to token scopes. That is exactly why
// checking those led nowhere, and why the check has to ask the host about the
// ACCOUNT.
//
// 🔴 IT IS A DIFFERENT FLAG FROM THE ONE `app doctor` ALREADY DEPENDS ON, AND
// THAT IS THE REASON THE COMMAND WAS BLIND TO IT. `appListings.listMine` — the
// read the rest of this command is built on — is gated on `app-blocks-author`
// (`isAppBlocksAuthorEnabled`, which HAS a moderator floor). The flag this probe
// asks about is `app-blocks-enabled` (`isAppBlocksEnabled`, which has NO
// moderator floor). Two flags, two audiences, documented in-repo as rolling out
// to the same two segments today and "one segment edit away" from diverging
// (`civitai/civitai:src/server/notifications/app-listing.notifications.ts:65`).
// So a developer can read every listing they own, submit versions, get an app
// APPROVED — and still have every runtime host call from inside that app refused.
//
// ## The route, and why this one
//
// `GET /api/v1/blocks/submissions` (SubmissionsPath — the route `app status`
// already reads). Gate ladder at `civitai/civitai:src/pages/api/v1/blocks/
// submissions.ts`, origin/main, read 2026-10-04:
//
//	204/209  401  {"message":"Missing or malformed Bearer token"}
//	214      401  {"message":"Invalid API key"}            — ALSO a banned account
//	228      403  {"message":"App status requires a personal API key or an OAuth token with the Apps submit scope"}
//	244      403  {"message":"Apps are restricted to the Civitai team"}  — !isAppBlocksAuthorEnabled
//	251      503  {"message":"Apps are not enabled"}       — !isAppBlocksEnabled  ← THE ROLLOUT GATE
//	261      422  {"message":"Invalid query", …}
//	281/288  503  {"message":"Rate limiter unavailable; please retry"}
//	300      429  {"message":"Rate limit exceeded", …}
//	339/351  200  the submissions payload                  ← PAST the rollout gate
//
// 🔴 THE POSITIVE VERDICT IS THE ROUTE'S SUCCESS PATH, WHICH IS WHAT MAKES IT
// ROBUST. The obvious alternative is `POST /api/v1/blocks/dev-token` with a body
// its own zod schema must reject: there the 400 schema refusal sits AFTER the
// same flag gate, so a 400 would also mean "enrolled". It was REJECTED, and the
// reason is the direction it fails in — that reading depends on the host keeping
// the flag gate ahead of body validation, and the day it does not, a
// NON-enrolled account answers 400 and the probe reports ENROLLED. A 200 cannot
// break that way: every gate the host adds, in any order, refuses. (dev-token is
// also a money path, carries a pre-auth 503 of its own that collides with the
// flag 503, and would be probed with a deliberately malformed body.)
//
// 🔴 NO MESSAGE IS MATCHED, DELIBERATELY. A message is the least stable contract
// the host offers and these refusals are actively being reworded; every branch
// below keys on the STATUS, plus — for 503 only — the structural presence of the
// route's own JSON envelope. The server's text is CARRIED, so a human sees the
// string they searched for, and never DECIDED on.
//
// ## What this probe CANNOT separate — stated, not hidden
//
// There is no machine-readable code or field anywhere on this surface that
// separates a policy refusal from a credential refusal: all four bearer-authed
// `/api/v1/blocks/*` routes emit a bare `{"message": string}`. So the status code
// is the whole signal, and it leaves two residual ambiguities that this package
// reports rather than guesses at:
//
//   - **A 503 is the rollout gate OR a Redis incident.** Lines 281/288 answer 503
//     when the rate limiter is unavailable. Those sit AFTER the flag gate, so
//     that 503 can only reach an account that IS enrolled — i.e. the two causes
//     are mutually exclusive in meaning and indistinguishable on the wire. Only
//     the carried message tells them apart, so RolloutNotEnrolled's advice names
//     the alternative instead of pretending to certainty.
//   - **A 401 is a bad credential OR a BANNED account.** `getSessionFromBearerToken`
//     (`civitai/civitai:src/server/auth/bearer-token.ts:42`) returns null for a
//     banned user, so the route's own `user.bannedAt` 403 arm is unreachable and a
//     ban arrives as `401 Invalid API key`. RolloutNoCredential must not claim
//     "your token expired".
//
// ⚠ NOT MEASURED LIVE. Every status above is read off the host's source at
// origin/main, not from a probe against production — this machine holds no
// credential for an account on either side of the rollout. The ladder is quoted
// with line numbers so it can be re-read rather than trusted.

// RolloutState is what the probe ESTABLISHED about the calling account. A string
// so it can be published in `--json` unchanged.
//
// 🔴 FOUR OF THE SEVEN MEAN "COULD NOT ESTABLISH", AND THEY ARE SEPARATE VALUES
// RATHER THAN ONE `unknown`. An absence has many causes, and conflating them is
// the exact defect this probe exists to close: reporting an unreachable host, a
// credential the host would not accept, or a refusal that fired BEFORE the
// rollout question as "you are not in the rollout" would regenerate the
// wrong-subject diagnosis one level out.
type RolloutState string

const (
	// RolloutEnrolled: the host answered 200, so this account is past the
	// `app-blocks-enabled` gate.
	RolloutEnrolled RolloutState = "enrolled"
	// RolloutNotEnrolled: HTTP 503 carrying the route's own JSON envelope — the
	// account was refused AT that gate. See the header for the one other cause
	// of a 503 here and why the advice names it.
	RolloutNotEnrolled RolloutState = "not-enrolled"
	// RolloutAuthorRefused: HTTP 403 — the host refused BEFORE it reached the
	// rollout question, so the rollout state is genuinely unknown behind it. Two
	// causes share this status: an OAuth token without the Apps-submit scope,
	// and an account outside the Apps-AUTHOR cohort — a DIFFERENT flag from the
	// one this probe asks about.
	RolloutAuthorRefused RolloutState = "author-refused"
	// RolloutNoCredential: HTTP 401, or a local token-source failure. The host
	// never accepted a credential, so it was never asked about the account.
	// Includes a BANNED account (see the header).
	RolloutNoCredential RolloutState = "no-credential"
	// RolloutRateLimited: HTTP 429. A real answer about the request and no
	// answer at all about the account.
	RolloutRateLimited RolloutState = "rate-limited"
	// RolloutUnreachable: no HTTP response arrived (dial, TLS, timeout).
	RolloutUnreachable RolloutState = "unreachable"
	// RolloutUnknown: a response arrived and no rollout verdict can be read out
	// of it — an unexpected status, or a 503 that did not come from the route
	// (an edge or CDN outage page).
	RolloutUnknown RolloutState = "unknown"
)

// Rollout is the probe's whole result. It is a VALUE, never an error — see
// CheckAppBlocksRollout for why that is load-bearing for the exit code.
type Rollout struct {
	State RolloutState
	// HTTPStatus is the status the host answered with, or 0 when no response
	// arrived. Published so a reader can re-derive the branch that was taken.
	HTTPStatus int
	// ServerMessage is the host's own `{"message": …}` text, verbatim and
	// possibly empty. CARRIED, never matched — it is the string a developer
	// searched for, and it is the only thing separating the two causes of a 503
	// named in the header.
	//
	// 🔴 IT IS SERVER-ORIGIN TEXT. Anything printing it on a human surface must
	// gate it (internal/cmd's safeTermBounded); `--json` emits it raw, which is
	// safe because spec-compliant JSON escapes the control class.
	ServerMessage string
	// TransportDetail is the transport or token-source failure, set only for
	// RolloutUnreachable and for a RolloutNoCredential raised locally.
	TransportDetail string
}

// Enrolled renders the verdict as a TRISTATE for machine consumers.
//
// 🔴 `nil` IS NOT `false`, AND THAT IS THE WHOLE POINT OF THE POINTER. A script
// asking "is this developer outside the rollout" must not be handed `false` for
// a run that never reached the host — which is how "an absence has many causes"
// turns into a wrong answer inside someone else's automation. `--json` emits
// this as `null`.
func (r Rollout) Enrolled() *bool {
	switch r.State {
	case RolloutEnrolled:
		yes := true
		return &yes
	case RolloutNotEnrolled:
		no := false
		return &no
	}
	return nil
}

// Established reports whether the probe actually answered the rollout question.
func (r Rollout) Established() bool { return r.Enrolled() != nil }

// CheckAppBlocksRollout asks the host whether the calling account is inside the
// `app-blocks-enabled` rollout.
//
// 🔴 IT RETURNS NO ERROR, AND THAT IS A STATEMENT ABOUT THE EXIT CODE RATHER
// THAN A STYLE CHOICE. Every outcome here is an OBSERVATION, not a failure of
// the command that asked — including the 503, which `civitai.TagStatus` would
// classify as ErrNetwork and `cmd/civitai`'s mapper would publish as exit 5. A
// signature returning `error` invites a caller to propagate it, and propagating
// this 503 would turn a healthy `app doctor` run into exit 5 for every developer
// outside the rollout: a release script red forever, for a state the developer
// cannot change. There is deliberately nothing for a caller to propagate.
//
// 🔴 IT SENDS NO QUERY AND READS NO ROWS. The route narrows by `?id=`/`?blockId=`
// and this probe passes neither: the verdict is the STATUS, and a selector could
// only add a 422/404 arm that answers the same question less clearly. The 200
// body is discarded unread.
func (c *Client) CheckAppBlocksRollout(ctx context.Context) Rollout {
	build := func() (*http.Request, error) {
		// 🔴 THROUGH THE GATEWAY, NOT `c.BaseURL+SubmissionsPath`. The first
		// version of this function built the URL itself and
		// TestSubmissionsRouteHasOneGateway caught it: deriving the route's
		// accessor set as "the funcs that reach submissionsURL" is only sound
		// while that is the ONE way to the route, so a func assembling the URL
		// by hand is an accessor no derivation can see. Both empty arguments are
		// the point — see the no-selector note above.
		return http.NewRequestWithContext(ctx, http.MethodGet, c.submissionsURL("", ""), nil)
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		// 🔴 authedDo's error is TWO different things and they must not share a
		// branch. A transport failure means the host never answered; a
		// token-source failure (internal/auth's "no refresh token stored", which
		// arrives tagged ErrUnauthorized) means this machine could not produce a
		// credential and nothing left it. Under one arm the second reads as "the
		// host is down", which is both wrong and unactionable.
		if errors.Is(err, civitai.ErrUnauthorized) {
			return Rollout{State: RolloutNoCredential, TransportDetail: err.Error()}
		}
		return Rollout{State: RolloutUnreachable, TransportDetail: err.Error()}
	}
	msg, fromRoute := rolloutEnvelope(raw)
	switch status {
	case http.StatusOK:
		return Rollout{State: RolloutEnrolled, HTTPStatus: status, ServerMessage: msg}
	case http.StatusServiceUnavailable:
		// 🔴 THE ENVELOPE CHECK IS WHAT KEEPS A HOST OUTAGE OUT OF THIS ARM. A
		// 503 from an edge proxy or CDN is the likeliest way "the host is having
		// an incident" would get reported as "you are not in the rollout", and
		// it is separable without reading any text: the route answers
		// `{"message": …}`, an outage page answers HTML.
		if !fromRoute {
			return Rollout{State: RolloutUnknown, HTTPStatus: status, ServerMessage: msg}
		}
		return Rollout{State: RolloutNotEnrolled, HTTPStatus: status, ServerMessage: msg}
	case http.StatusUnauthorized:
		return Rollout{State: RolloutNoCredential, HTTPStatus: status, ServerMessage: msg}
	case http.StatusForbidden:
		return Rollout{State: RolloutAuthorRefused, HTTPStatus: status, ServerMessage: msg}
	case http.StatusTooManyRequests:
		return Rollout{State: RolloutRateLimited, HTTPStatus: status, ServerMessage: msg}
	default:
		return Rollout{State: RolloutUnknown, HTTPStatus: status, ServerMessage: msg}
	}
}

// rolloutEnvelope parses the route's refusal envelope ONCE and answers both
// questions this file asks of a body: what did the host say, and did the host
// actually say it. `fromRoute` is true only for a JSON object carrying a
// non-empty `message` — the key every refusal on this route uses — so a body
// this cannot parse is NOT from the route. That is the conservative direction:
// it downgrades a 503 to RolloutUnknown rather than asserting a rollout state on
// the strength of a status code any intermediary could have produced.
//
// 🔴 IT IS NOT serverMessage, AND THE DIFFERENCE IS A REAL DEFECT THIS CLOSES.
// serverMessage falls back to the trimmed RAW BODY when the key is absent, which
// is right for an error mapper — every caller there already holds a refusal —
// and wrong here, because this probe reads SUCCESS and OUTAGE bodies too.
// Measured against the fake before the split: the 200 arm reported `The host
// said: {"submissions":[]}`, the route's own payload rendered as if it were a
// sentence from the host. The worse case is the same mechanism on an edge 503,
// where the fallback would have put a whole HTML outage page on the user's
// terminal as the host's message.
//
// 🔴 ONE FUNCTION, NOT TWO, BECAUSE IT IS ONE RULE. The first version had a
// separate `rolloutRouteAnswered` doing its own `json.Unmarshal` of the same
// bytes for the same key: two spellings of "is this the route's envelope",
// already disagreeing about whether `error` counted.
func rolloutEnvelope(raw []byte) (msg string, fromRoute bool) {
	var env struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &env) != nil {
		return "", false
	}
	msg = strings.TrimSpace(env.Message)
	return msg, msg != ""
}
