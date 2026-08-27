// Package appapi is the CLI-internal client for the Civitai App Blocks
// developer surface: bundle submission, submission status / withdraw, the
// per-app dev-token and dev-tunnel machinery, the owner-only Forgejo clone
// info, and the OAuth device-authorization login flow. It builds on the public
// read/download SDK (github.com/civitai/cli/pkg/civitai) — reusing its
// TokenSource contract and error-kind classification — but is deliberately NOT
// part of that SDK's exported compatibility surface: these operations are
// internal to the `civitai` CLI and only the read/download client is a public
// contract.
package appapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/civitai/cli/pkg/civitai"
)

// Submitter submits a packaged bundle and returns the server's response. The
// slug + version identify the submission so that, if the upload's response is
// lost to a timeout, the submit path can poll for a landed submission and
// recover rather than reporting a false failure (see SubmitVersion).
type Submitter interface {
	SubmitVersion(ctx context.Context, zipBytes []byte, slug, version string, prov Provenance) (*SubmitResult, error)
}

// Provenance is what the CLI CLAIMS about the source the bundle was built from
// (issue #411, the stamp half). It is a claim and never a proof: the server
// stores what it is told and cannot check that these bytes were built from that
// commit, so nothing rendered from it may be worded as verified fact.
//
// Both halves are optional and independently unknown, which is why Dirty is a
// pointer:
//
//	Commit == "" , Dirty == nil   → unknown (no repo, no git, unborn HEAD)
//	Commit == sha, Dirty == false → the client asserted a CLEAN tree
//	Commit == sha, Dirty == true  → the client asserted a DIRTY tree (--allow-dirty)
//	Commit == sha, Dirty == nil   → the commit is known, dirtiness is not
//
// 🔴 nil AND false ARE DIFFERENT ANSWERS ON BOTH SIDES OF THE WIRE. `null` (or
// an absent key) means nobody said; `false` means a client looked and said
// clean. Collapsing them with `?? false` invents an assertion the CLI never
// made, and it is the same tri-state the READ path carries back on
// Submission.SourceDirty.
type Provenance struct {
	// Commit is the full 40-character lowercase hex sha of HEAD, or "" when it
	// is not known. Anything that is not sourceCommitRe is treated as unknown —
	// see sanitised.
	Commit string
	// Dirty is the client's assertion about its own work tree. nil ⇒ unknown.
	Dirty *bool
}

// sourceCommitRe is the server's own validator, mirrored (civitai/civitai#4061,
// POST /api/v1/blocks/submit-version): `sourceCommit` must be 40 lowercase hex
// characters.
//
// 🔴 IT IS MIRRORED HERE BECAUSE A MALFORMED VALUE IS A HARD 400 THAT FAILS THE
// WHOLE SUBMIT. The server rejects the request rather than dropping the field —
// deliberately, because silently dropping is the inert-feature shape — so the
// burden is on this side: a provenance stamp is a diagnostic nicety and an
// upload is the user's actual job, and the nicety must never cost the job. Every
// value that does not match is sent as ABSENT.
var sourceCommitRe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// sanitised returns the fields that may go on the wire, and it is the ONE gate
// they pass through — SubmitVersion and SubmitBodySize both call it, so the size
// the CLI reports and the body it sends cannot disagree about what was included.
//
// 🔴 AN UNUSABLE COMMIT DROPS THE DIRTY FLAG WITH IT. `sourceDirty` alone says
// "the tree I cannot name was dirty", which is not a fact anyone can act on, and
// the matrix in issue #411 spells every branch that cannot resolve HEAD as
// "send nothing" rather than "send half".
func (p Provenance) sanitised() (string, *bool) {
	if !sourceCommitRe.MatchString(p.Commit) {
		return "", nil
	}
	return p.Commit, p.Dirty
}

// Verifier verifies a token and returns the authenticated identity.
type Verifier interface {
	WhoAmI(ctx context.Context) (*Identity, error)
}

// StatusReader reads the caller's own App-Block submission review/deploy state.
type StatusReader interface {
	// ListSubmissions returns the caller's submissions, newest first. An empty
	// blockId lists all of them.
	ListSubmissions(ctx context.Context, blockID string) ([]Submission, error)
	// GetSubmission returns a single submission. Exactly one of id (a
	// pubreq_<ULID>) or blockID (an app slug) must be set.
	GetSubmission(ctx context.Context, id, blockID string) (*Submission, error)
}

// Withdrawer withdraws the caller's own pending App-Block publish request so a
// new bundle can be submitted for the same slug.
type Withdrawer interface {
	// WithdrawRequest withdraws the publish request with the given id. It is
	// idempotent: a 200 (incl. already-withdrawn) is success; a 409 means the
	// request is not in a withdrawable (pending) state.
	WithdrawRequest(ctx context.Context, publishRequestID string) error
}

// Submission mirrors the shaped row from GET /api/v1/blocks/submissions
// (civitai/civitai src/pages/api/v1/blocks/submissions.ts -> shapeRow). Field
// names + JSON casing track the server EXACTLY.
type Submission struct {
	ID              string  `json:"id"`
	BlockID         string  `json:"blockId"` // the app slug; builds <blockId>.civit.ai
	AppBlockID      *string `json:"appBlockId"`
	Version         string  `json:"version"`
	Status          string  `json:"status"` // pending | approved | rejected | withdrawn
	RejectionReason *string `json:"rejectionReason"`
	ApprovalNotes   *string `json:"approvalNotes"`
	DeployState     *string `json:"deployState"` // null | building | deploying | live | failed
	DeployDetail    *string `json:"deployDetail"`
	DeployUpdatedAt *string `json:"deployUpdatedAt"`
	SubmittedAt     string  `json:"submittedAt"`
	ReviewedAt      *string `json:"reviewedAt"`
	UpdatedAt       string  `json:"updatedAt"`
	CreatedAt       string  `json:"createdAt"`
	LiveURL         *string `json:"liveUrl"` // set once serving (approved+live)
	// SourceCommit / SourceDirty are the submitting CLIENT'S CLAIM about the
	// source the bundle was built from (issue #411; server side
	// civitai/civitai#4061). The server stores them unverified — it cannot check
	// that the bundle came from that commit — so every rendering says who said
	// it. Both are pointers because the tri-state is the whole point:
	//
	//	nil   → UNKNOWN: a row from before the feature, or a client that sent nothing
	//	false → the client asserted a CLEAN work tree
	//	true  → the client asserted a DIRTY work tree
	//
	// 🔴 nil AND false ARE DIFFERENT ANSWERS. Never `?? false`.
	SourceCommit *string `json:"sourceCommit"`
	SourceDirty  *bool   `json:"sourceDirty"`
}

// Subject identifies the credential behind a token as returned by
// GET /api/v1/me. Type == "oauth" means an OAuth device-login token (from
// `civitai login`); any other type (e.g. "apiKey"/"user") is a personal API
// key. Absent when auth is cookie/session (not applicable to the CLI).
type Subject struct {
	Type string `json:"type"`
	// ID is the credential's identifier. Its JSON shape is server-owned and
	// varies by credential kind — a numeric api-key id (e.g. 96633526) or a
	// string oauth subject — so it is kept as RawMessage to tolerate either
	// shape. whoami does not render it; only Type drives CredentialType/IsOAuth.
	ID json.RawMessage `json:"id,omitempty"`
}

// Identity is the authenticated-user view `whoami` reports. TokenScope and
// Subject are pointers because GET /api/v1/me omits them for some auth kinds
// (e.g. cookie/session), and a nil TokenScope must degrade to "scopes unknown"
// rather than decode as "no capabilities". The volatile, unrendered fields
// (BuzzLimit, Subject.ID) are json.RawMessage so a server-side type change to a
// peripheral field can never break the parse of the core identity whoami prints
// (see WhoAmI's core-identity fallback for the belt-and-suspenders guarantee).
type Identity struct {
	Username string `json:"username"`
	ID       int    `json:"id"`
	// TokenScope is the bearer token's scope bitmask. Decode it with the Scope*
	// bits below to learn what the credential can do (spend Buzz, read balance,
	// …). A personal full-scope key has every bit; an OAuth device-login token
	// typically has neither AIServicesWrite nor BuzzRead. nil ⇒ unknown (absent
	// from the response, e.g. cookie auth).
	TokenScope *int `json:"tokenScope,omitempty"`
	// BuzzLimit is the credential's raw per-window spend-cap payload as returned
	// by the server. Its shape is server-owned and has changed over time (a bare
	// number in older responses, an array of {type,limit,window,unit} windows in
	// current ones), so it is kept as RawMessage: whoami does not render it, and
	// it must never break the parse of the core identity. nil ⇒ absent/unknown.
	BuzzLimit json.RawMessage `json:"buzzLimit,omitempty"`
	// Subject identifies the credential (OAuth login vs personal API key). nil ⇒
	// cookie/session auth (not applicable to the CLI).
	Subject *Subject `json:"subject,omitempty"`

	// ---- account profile (#377 option b) --------------------------------------
	//
	// 🔴 THE POINTERS ARE THE SAME TRI-STATE RULE AS CanSubmitApps, NOT STYLE.
	// GET /api/v1/me omits every field below for some credentials, and a plain
	// `string`/`bool` would zero-fill the omission — publishing `"tier": ""` or
	// `"isMember": false` as if the server had SAID so. nil ⇒ the server did not
	// report it, and reaches `whoami --json` as `null`.
	//
	// 🔴 email AND emailVerified ARE DELIBERATELY NOT MODELLED, AND MUST NOT BE
	// ADDED. The live capture in api_test.go carries both, so this is an omission
	// with a reason, not an oversight: they are PII that `whoami` does not print
	// today, and `whoami --json` is a projection precisely so a script that
	// redirects its output never lands a user's email address in a log. Modelling
	// them here would put them one map entry away from being published — see
	// civitai/cli#377, which rejected "just pass the raw body through" for this.
	//
	// 🔴 "UNMODELLED" IS ONLY HALF THE GUARANTEE, AND THE OTHER HALF IS EASY TO
	// LOSE. A field the struct drops can still reach a terminal by any path that
	// handles the RAW bytes: WhoAmI's parse-failure branch echoed the whole body
	// until this was written, and Subscriptions below published anything the
	// server put in it for exactly as long as it was a json.RawMessage. Both are
	// closed. Before adding any code that touches the raw /me response, ask what
	// it does with bytes this struct deliberately never decodes.

	// Tier is the account's membership tier ("free", "silver", …). nil ⇒ absent.
	Tier *string `json:"tier,omitempty"`
	// Status is the account status ("active", …). nil ⇒ absent.
	Status *string `json:"status,omitempty"`
	// IsMember is the server's own answer to "is this a member account". It is
	// not idle trivia: AGENTS.md item 13
	// (claudedocs/decisions/13-generation-graph-not-validated.md) records that a
	// caller's usable — non-disabled, non-memberOnly — ecosystem set differs
	// between a free and a member account, so this is the one field that predicts
	// whether `civitai generate`'s defaults are even available. nil ⇒ absent.
	IsMember *bool `json:"isMember,omitempty"`
	// Subscriptions is the account's subscription list (the live capture carries
	// `["yellow"]`). nil ⇒ absent; a non-nil empty slice ⇒ reported and empty,
	// the same nil-is-not-empty distinction DecodeScopes documents.
	//
	// 🔴 IT IS TYPED, NOT json.RawMessage, AND THAT IS A PRIVACY DECISION THAT
	// OVERRODE A RESILIENCE ONE. RawMessage was tried first, reasoning that this
	// is the one COMPOSITE among the four profile fields and so the one that can
	// drift shape (buzzLimit did exactly that on this endpoint and hard-failed
	// whoami in production). It was wrong twice over, both measured:
	//
	//   - A RawMessage passes the server's bytes to `--json` VERBATIM, so a
	//     future object-shaped element carrying a billing email or a card
	//     fragment would be published with no code change at all. That is the
	//     very boundary the email/emailVerified omission above exists to hold —
	//     leaving one field an unbounded passthrough makes the argument false.
	//   - It did not even buy the resilience it was chosen for: a drift in Tier,
	//     Status OR IsMember drops WhoAmI into parseCoreIdentity and blanks all
	//     four regardless, so the raw field protected the group only against a
	//     drift in ITSELF.
	//
	// Typed, an unexpected shape degrades exactly like its siblings — every
	// profile field goes null, `whoami` still works, and nothing unmodelled ever
	// reaches stdout. TestWhoAmIProfileDriftDegradesTheWholeProfile pins that,
	// and TestWhoAmIJSONNeverPublishesUnmodelledSubscriptionContent pins that the
	// degradation is what stops the PII rather than luck.
	//
	// No `omitempty`: it omits nil AND empty alike, which would erase the
	// distinction the paragraph above promises on a direct json.Marshal of this
	// struct. (`whoami --json` builds its own map and was never affected.)
	// TestSubscriptionsTagKeepsNilAndEmptyDistinct pins it.
	Subscriptions []string `json:"subscriptions"`
}

// Token-scope bits, mirrored from @civitai/auth token-scope (civitai/civitai
// src/shared/constants/token-scope.constants.ts). These are STABLE/frozen bit
// positions in the tokenScope bitmask GET /api/v1/me returns.
const (
	ScopeUserRead           = 1 << 0
	ScopeUserWrite          = 1 << 1
	ScopeModelsRead         = 1 << 2
	ScopeModelsWrite        = 1 << 3
	ScopeModelsDelete       = 1 << 4
	ScopeMediaRead          = 1 << 5
	ScopeMediaWrite         = 1 << 6
	ScopeMediaDelete        = 1 << 7
	ScopeArticlesRead       = 1 << 8
	ScopeArticlesWrite      = 1 << 9
	ScopeArticlesDelete     = 1 << 10
	ScopeBountiesRead       = 1 << 11
	ScopeBountiesWrite      = 1 << 12
	ScopeBountiesDelete     = 1 << 13
	ScopeAIServicesRead     = 1 << 14
	ScopeAIServicesWrite    = 1 << 15 // spend Buzz on AI services (generation)
	ScopeBuzzRead           = 1 << 16 // read the user's Buzz balance
	ScopeCollectionsRead    = 1 << 17
	ScopeCollectionsWrite   = 1 << 18
	ScopeSocialWrite        = 1 << 19
	ScopeSocialTip          = 1 << 20
	ScopeNotificationsRead  = 1 << 21
	ScopeNotificationsWrite = 1 << 22
	ScopeVaultRead          = 1 << 23
	ScopeVaultWrite         = 1 << 24
	ScopeAppBlocksSubmit    = 1 << 25
	// ScopeFull is the OR of bits 0..24 — every scope a personal key carries. It
	// EXCLUDES AppBlocksSubmit (1<<25), matching the upstream Full constant
	// (1<<25)-1.
	ScopeFull = (1 << 25) - 1
)

// scopeBit maps a single scope bit to its (upstream-const-style) name for the
// `whoami --scopes` decode. Ordered low → high bit.
type scopeBit struct {
	bit  int
	name string
}

var scopeBits = []scopeBit{
	{ScopeUserRead, "UserRead"}, {ScopeUserWrite, "UserWrite"},
	{ScopeModelsRead, "ModelsRead"}, {ScopeModelsWrite, "ModelsWrite"}, {ScopeModelsDelete, "ModelsDelete"},
	{ScopeMediaRead, "MediaRead"}, {ScopeMediaWrite, "MediaWrite"}, {ScopeMediaDelete, "MediaDelete"},
	{ScopeArticlesRead, "ArticlesRead"}, {ScopeArticlesWrite, "ArticlesWrite"}, {ScopeArticlesDelete, "ArticlesDelete"},
	{ScopeBountiesRead, "BountiesRead"}, {ScopeBountiesWrite, "BountiesWrite"}, {ScopeBountiesDelete, "BountiesDelete"},
	{ScopeAIServicesRead, "AIServicesRead"}, {ScopeAIServicesWrite, "AIServicesWrite"},
	{ScopeBuzzRead, "BuzzRead"},
	{ScopeCollectionsRead, "CollectionsRead"}, {ScopeCollectionsWrite, "CollectionsWrite"},
	{ScopeSocialWrite, "SocialWrite"}, {ScopeSocialTip, "SocialTip"},
	{ScopeNotificationsRead, "NotificationsRead"}, {ScopeNotificationsWrite, "NotificationsWrite"},
	{ScopeVaultRead, "VaultRead"}, {ScopeVaultWrite, "VaultWrite"},
	{ScopeAppBlocksSubmit, "AppBlocksSubmit"},
}

// ScopeKnown reports whether the identity carries a decodable scope bitmask.
// When false, capability queries are unknowable and the caller should say so
// rather than reporting "no".
func (id *Identity) ScopeKnown() bool { return id.TokenScope != nil }

// hasScope reports whether a KNOWN scope mask includes bit. A nil (unknown)
// mask is false.
func (id *Identity) hasScope(bit int) bool {
	return id.TokenScope != nil && *id.TokenScope&bit != 0
}

// CanSpendBuzz reports whether the identity's token carries the AI-Services
// (Buzz-spend) scope. An unknown scope is treated as false.
func (id *Identity) CanSpendBuzz() bool { return id.hasScope(ScopeAIServicesWrite) }

// CanReadBuzz reports whether the identity's token can read the Buzz balance.
// An unknown scope is treated as false.
func (id *Identity) CanReadBuzz() bool { return id.hasScope(ScopeBuzzRead) }

// CanSubmitApps reports whether the credential can clear `civitai app submit`'s
// SCOPE gate. The backend scope-gates submit ONLY on OAuth tokens: an OAuth
// device-login token must carry the opt-in AppBlocksSubmit bit (bit 25, excluded
// from ScopeFull), whereas a personal API key is NOT scope-gated for submit at
// all — submit-version runs the AppBlocksSubmit check only when
// subject.type == "oauth", so any personal key clears it. (The remaining
// author-cohort / not-banned gates are server-side and not visible here, so a
// "yes" means the credential's SCOPE permits submit, not that the account is in
// the author cohort.)
//
// 🔴 THE RESULT IS TRI-STATE, AND THE POINTER IS WHY. There are THREE answers,
// not two — yes, no, and *we cannot tell* — and this is the same discriminator
// shape as AGENTS.md item 9's `views.unavailable`
// (claudedocs/decisions/09-views-unavailable-discriminator.md): read that item
// for the rationale rather than re-deriving it here. A plain `bool` collapses
// "cannot tell" into "no", which is a false negative stated as fact — measured
// on `{"username":…,"id":…,"subject":{"type":"oauth","id":"a"}}` (an OAuth
// credential whose `tokenScope` the server omitted), where the bool version
// emitted `"canSubmitApps": false` while the truth was unknowable. The pointer
// return also makes the third state impossible for a caller to ignore: it
// cannot be handed to a yes/no renderer without an explicit nil branch, and it
// marshals straight to JSON `null`.
//
// The three states, exactly:
//
//   - non-nil true/false — an OAuth credential with a KNOWN mask (the bit
//     decides), or any personal API key (never scope-gated, so always true).
//   - nil — an OAuth credential whose scope mask is absent (the bit is the
//     whole answer and we do not have it), or a credential with no `subject`
//     at all (CredentialType() == "unknown": we cannot even tell whether the
//     OAuth gate applies).
//
// Callers must branch on nil and say "unknown"; they must never print "no".
func (id *Identity) CanSubmitApps() *bool {
	switch {
	case id.IsOAuth():
		if !id.ScopeKnown() {
			return nil // the bit is the whole answer, and it is not reported.
		}
		return boolPtr(id.hasScope(ScopeAppBlocksSubmit))
	case id.CredentialType() == "personal API key":
		// Not scope-gated for submit at all, so this holds with or without a mask.
		return boolPtr(true)
	default:
		// No subject: we cannot tell which gate applies, let alone its answer.
		return nil
	}
}

func boolPtr(b bool) *bool { return &b }

// DecodeScopes returns the names of every set scope bit (low → high).
//
// 🔴 nil AND EMPTY ARE DIFFERENT ANSWERS, so the field is self-describing when
// it reaches a `--json` surface: a nil (unknown) mask returns nil, while a mask
// that is KNOWN and zero returns a non-nil empty slice. Collapsing the two made
// `whoami --json` emit `"scopes": null` in two unrelated states — scope
// unreported, and a real key with no bits set — which no consumer could tell
// apart. Go callers see len 0 either way; only the JSON encoding differs
// (`null` vs `[]`).
func (id *Identity) DecodeScopes() []string {
	if id.TokenScope == nil {
		return nil
	}
	out := []string{}
	for _, s := range scopeBits {
		if *id.TokenScope&s.bit != 0 {
			out = append(out, s.name)
		}
	}
	return out
}

// IsOAuth reports whether the credential is an OAuth device-login token
// (subject.type == "oauth"). A nil/absent subject is not OAuth.
func (id *Identity) IsOAuth() bool { return id.Subject != nil && id.Subject.Type == "oauth" }

// CredentialType is a human label for the credential behind the token:
// "OAuth login", "personal API key", or "unknown" when the subject is absent.
func (id *Identity) CredentialType() string {
	if id.Subject == nil || id.Subject.Type == "" {
		return "unknown"
	}
	if id.Subject.Type == "oauth" {
		return "OAuth login"
	}
	return "personal API key"
}

// BuzzAccount is the spendable Buzz balance from buzz.getBuzzAccount.
type BuzzAccount struct {
	Blue   int64 `json:"blue"`
	Green  int64 `json:"green"`
	Yellow int64 `json:"yellow"`
}

// Total is the sum of the blue, green, and yellow balances.
func (a *BuzzAccount) Total() int64 { return a.Blue + a.Green + a.Yellow }

// BuzzReader reads the caller's spendable Buzz balance.
type BuzzReader interface {
	// GetBuzzAccount returns the caller's Buzz balance. A credential lacking the
	// Buzz-read scope yields ErrBuzzScope (the server answers 403).
	GetBuzzAccount(ctx context.Context) (*BuzzAccount, error)
}

// ErrBuzzScope is returned by GetBuzzAccount when the stored credential lacks
// the Buzz-read scope (the server answers 403 FORBIDDEN). The command layer maps
// this to actionable, personal-key guidance.
var ErrBuzzScope = fmt.Errorf("credential lacks the Buzz-read scope")

// BuzzAccountPath is the tRPC route that returns the spendable Buzz balance.
const BuzzAccountPath = "/api/trpc/buzz.getBuzzAccount"

// SubmitResult is the publish-request result the server returns.
type SubmitResult struct {
	PublishRequestID string `json:"publishRequestId"`
	Slug             string `json:"slug"`
	Version          string `json:"version"`
	Status           string `json:"status"`
}

// DefaultSubmitPath is the token-authenticated submit-version route.
const DefaultSubmitPath = "/api/v1/blocks/submit-version"

// submitTimeout governs the submit-version upload specifically. A submit
// uploads a multi-file base64 ZIP and then waits for server-side processing
// (publish-request creation), which can take well over the fast-call timeout.
// A short timeout here produced a FALSE failure ("context deadline exceeded")
// on submits that had actually succeeded server-side, so the user's retry hit
// "you already have a pending submission". Scope this longer window to submit
// only — do NOT lengthen the fast calls.
const submitTimeout = 120 * time.Second

// SubmissionsPath is the token-authenticated, self-scoped submission-status
// route (GET; civitai/civitai src/pages/api/v1/blocks/submissions.ts).
const SubmissionsPath = "/api/v1/blocks/submissions"

// ListSubmissionsCap mirrors MAX_ROWS in that route: the UNFILTERED listing is
// `take: MAX_ROWS` (submissions.ts) with NO cursor, NO offset and NO total count
// in the response — its zod query schema accepts ONLY `id` and `blockId`, so
// there is nothing to page with and nothing to compare a length against.
// A full-length page is therefore the ONLY evidence available that rows were
// dropped, and it is INFERENCE, not a signal: a caller holding exactly this many
// submissions is indistinguishable from one holding more. Callers must present it
// as "may be incomplete", never as a complete answer and never as a hard fact.
//
// A `?blockId=<slug>` (or `?id=`) lookup is NOT affected: the server puts
// `where.slug` into the query BEFORE `take`, so a single app would need more
// than this many submissions of its own to be truncated. Don't add a caveat
// there.
//
// KNOWN LIMITATION — this is a vendored mirror, and it can drift SILENTLY in
// two directions with different severities. Stating them precisely, because a
// vague "keep in lockstep" comment is exactly what lets both happen unnoticed:
//
//	Server RAISES MAX_ROWS (say to 250) — callers holding between 100 and 250
//	rows get the caveat when nothing is missing. The user-visible text stays
//	TRUE: it quotes the OBSERVED row count, never this constant, and says older
//	submissions "may" exist (pinned by TestAppStatusCaveatQuotesObservedCount).
//	So the failure is a false warning, not a false statement.
//
//	Server LOWERS MAX_ROWS (say to 50) — the response carries 50 rows, the
//	`n >= 100` predicate is false, and NO caveat fires: the silent-truncation
//	bug this constant exists to prevent comes straight back, and no offline
//	test can notice. This is the serious direction.
//
// Neither direction is detectable from a single response — it carries no total
// and no cursor, the same gap that makes the truncation itself invisible.
//
// 🔴 NOTHING CHECKS THIS AUTOMATICALLY. TestSubmissionsCapDriftAgainstLiveAPI
// (cap_drift_test.go) CAN detect both directions, but it is opt-in — gated on
// CIVITAI_CHECK_SUBMISSIONS_CAP=1 plus a credential — and NO CI job sets either.
// It runs only when a maintainer runs it. Do not read its existence as
// protection; until someone runs it, this constant rests on having read
// submissions.ts, not on a measurement.
//
// That was a DECISION, not an oversight: observing the cap needs a full-scope
// civitai personal API key on an account holding more than the cap, and this
// repo is PUBLIC. Putting a production credential in its CI to guard one
// integer is a worse trade than the drift it prevents. (`pins-vs-published` can
// be wired precisely because npm needs no credential.) A credential-free job
// would be worse than none — it would always skip, always report green, and
// look like a guard.
//
// The durable fix removes the question instead of guarding it: a `hasMore` flag
// (or a cursor) on the listing response deletes this constant, the inference,
// and that whole test. Ask for it before investing further here.
const ListSubmissionsCap = 100

// WithdrawPath is the token-authenticated, self-scoped withdraw route
// (POST {"publishRequestId": ...}; civitai/civitai
// src/pages/api/v1/blocks/withdraw.ts). 200 on success (incl. an idempotent
// already-withdrawn), 404 not-found-or-not-yours, 409 not in a withdrawable
// (pending) state.
const WithdrawPath = "/api/v1/blocks/withdraw"

// DevTokenPath is the invite-gated route that mints a short-lived dev block
// token for `npm run dev:live` (POST {"slug": ..., "scopes"?: [...],
// "requestBudgetedSpend"?: bool}; civitai/civitai
// src/pages/api/v1/blocks/dev-token.ts). 200 { token, ... } on success; a
// PENDING (un-approved) slug is accepted, and a slug with NO app row yet mints
// from the request-body `scopes` (the dev's LOCAL manifest scopes, clamped
// server-side) — so `create → dev-token → dev:live` works with no submit step.
// Error bodies are {message}: 404 slug registered to a different account /
// genuinely not found, 403 not-invited/insufficient-scope, 429 rate-limited,
// 503 flag-off.
//
// 🔴 SPEND TAKES **TWO** PREDICATES, NOT ONE (civitai/civitai#3703 step 1, live
// on main as of ed1427d9fe). The minted token keeps `ai:write:budgeted` only
// when BOTH hold — either alone STRIPS it, silently, and the mint still
// succeeds:
//
//   - ENTITLEMENT (`spendEntitled`) — the BEARER carries AIServicesWrite. A
//     full-scope personal API key, or an OAuth credential from
//     `civitai login --scopes generate`, does; a DEFAULT `civitai login` does
//     not, and mints read-only however the request is phrased.
//   - INTENT (`spendRequested`) — THIS request asked for budgeted spend, i.e.
//     the body's `requestBudgetedSpend`. The server currently resolves an ABSENT
//     field as `?? true` (the deliberately non-breaking step-1 default), so
//     omitting the key is not neutral — it reads as "yes".
//
// This is why the doc above used to describe the bearer's bit as the whole
// story, and no longer can: a spend-entitled bearer is now necessary and not
// sufficient. See devTokenBody.RequestBudgetedSpend for what the CLI sends.
const DevTokenPath = "/api/v1/blocks/dev-token"

func (c *Client) authedDo(ctx context.Context, build func() (*http.Request, error)) (int, []byte, error) {
	return c.authedDoWith(ctx, c.HTTP, build)
}

// authedDoWith is authedDo against a specific *http.Client, so a slow call
// (the submit upload) can use a longer-timeout client without affecting the
// fast, interactive calls that share c.HTTP.
func (c *Client) authedDoWith(ctx context.Context, httpClient *http.Client, build func() (*http.Request, error)) (int, []byte, error) {
	token, err := c.Tokens.Token(ctx)
	if err != nil {
		return 0, nil, err
	}
	status, raw, err := c.doOnceWith(httpClient, build, token)
	if err != nil {
		return 0, nil, err
	}
	if status == http.StatusUnauthorized {
		// Try a single refresh + retry. A non-refreshable source returns
		// ErrNoRefresh and we keep the original 401.
		newTok, rerr := c.Tokens.Refresh(ctx)
		if rerr == nil && newTok != "" {
			return c.doOnceWith(httpClient, build, newTok)
		}
	}
	return status, raw, nil
}

func (c *Client) doOnceWith(httpClient *http.Client, build func() (*http.Request, error), token string) (int, []byte, error) {
	req, err := build()
	if err != nil {
		return 0, nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := c.readBody(resp.Body)
	if err != nil {
		return resp.StatusCode, raw, err
	}
	return resp.StatusCode, raw, nil
}

// submitBody mirrors submitVersionSchema: a base64-encoded ZIP, plus the
// optional provenance the client claims for it (issue #411).
//
// 🔴 BOTH PROVENANCE FIELDS ARE `omitempty`, AND FOR SourceDirty THAT IS A
// DECISION ABOUT THE TRI-STATE, NOT A TIDINESS ONE. It is a *bool: omitempty
// drops a NIL pointer and keeps a pointer to false, so "unknown" leaves the key
// out (the documented wire spelling of UNKNOWN — the server also accepts null)
// while "the client looked and the tree was clean" is sent as an explicit
// `false`. A value bool would make those two indistinguishable and would assert
// CLEAN for every scaffolded app with no repo at all.
type submitBody struct {
	BundleBase64 string `json:"bundleBase64"`
	SourceCommit string `json:"sourceCommit,omitempty"`
	SourceDirty  *bool  `json:"sourceDirty,omitempty"`
}

// submitBodyEnvelope is the literal JSON document json.Marshal produces for an
// EMPTY submitBody — i.e. everything the marshaller adds around the payload,
// with NO provenance. Keep it beside the struct: adding a field to submitBody
// can change this, and the two must move together.
// TestSubmitBodySizeMatchesRealMarshal pins the arithmetic against json.Marshal
// itself rather than against this constant, and
// TestSubmitBodyEnvelopeIsTheEmptyMarshal pins the constant itself.
const submitBodyEnvelope = `{"bundleBase64":""}`

// submitEnvelopeLen returns the length of everything the marshaller puts around
// the base64 payload for THIS submit — the constant above when no provenance is
// carried, and the real marshalled envelope when some is.
//
// 🔴 IT IS MEASURED, NOT ADDED UP. The obvious alternative — keep the constant
// and add `len(",\"sourceCommit\":\"\"")+40` and so on — is a second model of
// what encoding/json does, and the whole reason SubmitBodySize exists is that
// the number it prints is compared against a real limit. Marshalling a struct
// whose payload field is empty costs nothing and cannot drift from the struct
// above, so a field added to submitBody is accounted for here without anyone
// remembering to update an arithmetic term.
func submitEnvelopeLen(prov Provenance) int {
	commit, dirty := prov.sanitised()
	if commit == "" && dirty == nil {
		return len(submitBodyEnvelope)
	}
	env, err := json.Marshal(submitBody{SourceCommit: commit, SourceDirty: dirty})
	if err != nil {
		// Unreachable for these field types; degrade to the no-provenance
		// envelope rather than reporting a nonsense size.
		return len(submitBodyEnvelope)
	}
	return len(env)
}

// SubmitBodySize returns the exact size, in bytes, of the HTTP request body
// SubmitVersion sends for a zip of zipLen bytes carrying provenance prov.
//
// 🔴 IT TAKES THE PROVENANCE BECAUSE THE BODY DOES. This number is PRINTED TO
// USERS — on the `Packaged …` line and again under a failed submit — as "what
// this CLI sent", and #411's stamp makes a submit that carries provenance ~70
// bytes larger than one that does not. A signature that could not see the
// provenance would have kept reporting the smaller number, which is a small
// error in the quantity and a total one in the claim: the point of the line is
// that it is EXACT (see below), so it may not be an estimate the moment a
// feature lands. Pass a zero Provenance for a path that sends none
// (--package-only, the no-token fallback) and the number is unchanged.
//
// 🔴 THE ZIP IS NOT WHAT GOES ON THE WIRE, AND THE DIFFERENCE IS THE WHOLE OF
// ISSUE #423. SubmitVersion base64-encodes the archive into a JSON document, so
// the bytes the server receives — and the bytes any request-body limit is
// applied to — are ~4/3 of the compressed size. An author reading
// `8201270 bytes compressed` off `app submit` had no way to see the ~10.9 MB
// that was actually sent, so nothing they could measure locally corresponded to
// the quantity that was refused.
//
// It is EXACT, not an estimate: base64's alphabet (A–Z a–z 0–9 + / =) contains
// no character encoding/json escapes, so the payload is copied through verbatim
// and the envelope is a constant. Do not substitute a 1.37 multiplier for it —
// the point of printing the number is that the author can compare it with a
// limit, and a rounded number cannot be compared with anything.
func SubmitBodySize(zipLen int, prov Provenance) int {
	return base64.StdEncoding.EncodedLen(zipLen) + submitEnvelopeLen(prov)
}

// submitClient returns an *http.Client for the submit upload: it mirrors the
// shared client's Transport but uses the longer submitTimeout, so the slow
// upload + server-side processing doesn't trip the short fast-call timeout
// (which caused false "context deadline exceeded" failures on submits that had
// already succeeded). If no shared client is set, a zero Client is used.
func (c *Client) submitClient() *http.Client {
	base := c.HTTP
	if base == nil {
		base = &http.Client{}
	}
	timeout := submitTimeout
	if c.SubmitTimeout > 0 {
		timeout = c.SubmitTimeout
	}
	return &http.Client{
		Timeout:       timeout,
		Transport:     base.Transport,
		CheckRedirect: base.CheckRedirect,
		Jar:           base.Jar,
	}
}

// SubmitVersion uploads the bundle to the token-authenticated submit route,
// refreshing the OAuth access token transparently if needed.
//
// The upload can complete server-side while its HTTP response is slow or never
// arrives within the timeout — observed in the wild as a false "context
// deadline exceeded" failure on a submit that had actually landed, leaving the
// user to retry into "you already have a pending submission". So when (and only
// when) the POST fails with a timeout / deadline-exceeded / no-response error
// (as opposed to a clean HTTP error status), this polls
// GET /api/v1/blocks/submissions for a submission matching slug+version and, if
// one is now present, reports it as a success — surfacing the pubreq id. If no
// matching submission is found, it returns a clear error telling the user to
// check `civitai app status` before resubmitting.
func (c *Client) SubmitVersion(ctx context.Context, zipBytes []byte, slug, version string, prov Provenance) (*SubmitResult, error) {
	// 🔴 sanitised(), NOT the caller's word. This is the last line before the
	// wire, so it is where the server's `^[0-9a-f]{40}$` is enforced: a value
	// that would not survive it is sent as ABSENT, because the server answers a
	// malformed sourceCommit with a hard 400 that fails the upload. A provenance
	// stamp must never be able to turn a working submit into a failed one.
	commit, dirty := prov.sanitised()
	body, err := json.Marshal(submitBody{
		BundleBase64: base64.StdEncoding.EncodeToString(zipBytes),
		SourceCommit: commit,
		SourceDirty:  dirty,
	})
	if err != nil {
		return nil, err
	}
	build := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+c.SubmitPath, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}
	status, raw, err := c.authedDoWith(ctx, c.submitClient(), build)
	if err != nil {
		if isTimeoutErr(err) {
			return c.recoverTimedOutSubmit(ctx, slug, version, err)
		}
		return nil, err
	}
	if status != http.StatusOK {
		return nil, serverError(status, raw)
	}
	var out SubmitResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("unexpected response (status %d): %s", status, string(raw))
	}
	return &out, nil
}

// isTimeoutErr reports whether err is a REQUEST timeout — the POST went out and
// no response came back inside the deadline — as opposed to a clean HTTP error
// status or any other failure. It matches context.DeadlineExceeded, an os
// timeout, and any net.Error whose Timeout() is true (which is what
// http.Client.Timeout surfaces while awaiting response headers).
//
// 🔴 A FILESYSTEM ERROR IS NEVER A REQUEST TIMEOUT, BECAUSE NOTHING WAS SENT.
// That is what civitai.IsTransportError gates, and the gate must stay ABOVE
// os.IsTimeout — see the two halves below. This is the third instance of the
// trap AGENTS.md item 24 documents (issues #241, #244, #246): syscall.Errno
// declares Timeout() and Temporary(), so it IS a net.Error, and Timeout() is
// TRUE for ETIMEDOUT, EAGAIN and EWOULDBLOCK.
//
// Reachable, not theoretical. internal/cmd/app_submit.go wires auth.New(cfg)
// into SubmitVersion → authedDoWith → auth.Source.Token(ctx) → refreshLocked →
// cfg.SetOAuthTokens → config.save(), a real filesystem write, and
// internal/auth/source.go returns `persist refreshed tokens: %w` when it fails.
// A config dir on NFS-soft / sshfs / CIFS fails with exactly those errnos. Route
// that into recoverTimedOutSubmit and the CLI polls /submissions three times for
// a submission that never existed — zero bytes ever left the machine — and then
// tells the author "submit timed out and the upload may not have completed …
// check whether it landed", about an upload that was never attempted.
//
// The predicate is filesystem-broad through TWO COMPLEMENTARY LINES, and fixing
// only the errors.As spelling is a HALF-FIX. Measured on go1.25.12:
//
//   - os.IsTimeout unwraps *fs.PathError / *os.LinkError / *os.SyscallError at
//     the TOP LEVEL only, so os.IsTimeout(&fs.PathError{Err: ETIMEDOUT}) is
//     TRUE — the direct shape a filesystem call site returns.
//   - errors.As walks through a fmt.Errorf wrapper, so os.IsTimeout of
//     `persist refreshed tokens: %w` around that same PathError is FALSE while
//     errors.As matches the Errno underneath and reports Timeout() TRUE — the
//     exact shape the submit path above produces.
//
// EACCES / ENOENT / EIO / ENOSPC are false in both halves and were never
// mis-sorted. Guard: internal/appapi/submit_fs_not_timeout_test.go.
func isTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// A filesystem error is never a request timeout — see the doc comment. This
	// must gate BOTH halves below, not only the errors.As one.
	if !civitai.IsTransportError(err) {
		return false
	}
	if os.IsTimeout(err) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}

// submitPollAttempts / submitPollDelay bound the post-timeout recovery poll:
// the submission may take a moment to become visible after the upload lands, so
// poll a few times with a short delay rather than just once.
const (
	submitPollAttempts = 3
	submitPollDelay    = 2 * time.Second
)

// recoverTimedOutSubmit is called only after a submit POST timed out. It polls
// the submissions list for a row matching slug+version; if found it reports a
// success (the submit landed), otherwise a clear error citing the original
// timeout and pointing at `civitai app status`.
func (c *Client) recoverTimedOutSubmit(ctx context.Context, slug, version string, cause error) (*SubmitResult, error) {
	delay := submitPollDelay
	if c.SubmitPollDelay != nil {
		delay = *c.SubmitPollDelay
	}
	for attempt := 0; attempt < submitPollAttempts; attempt++ {
		if attempt > 0 && delay > 0 {
			t := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				t.Stop()
				return nil, timedOutSubmitError(slug, cause)
			case <-t.C:
			}
		}
		subs, err := c.ListSubmissions(ctx, slug)
		if err != nil {
			// The poll itself failed; keep trying within the bound.
			continue
		}
		if sub := latestMatchingSubmission(subs, slug, version); sub != nil {
			return &SubmitResult{
				PublishRequestID: sub.ID,
				Slug:             sub.BlockID,
				Version:          sub.Version,
				Status:           sub.Status,
			}, nil
		}
	}
	return nil, timedOutSubmitError(slug, cause)
}

// latestMatchingSubmission returns the first submission matching slug+version in
// a non-terminal (pending/submitted) state, falling back to any slug+version
// match. Submissions are returned newest-first, so the first match is latest.
//
// 🔴 TWO TIERS, AND THE TIER DECIDES BEFORE RECENCY DOES: a non-terminal match
// wins outright even when a NEWER terminal row matched, and recency only orders
// rows within a tier. It is not a plain "newest row" rule, so do not describe it
// as one.
//
// The row picked here becomes the PublishRequestID `app submit` prints and the
// user hands to `civitai app withdraw`, so the wrong end aims a withdrawal at
// the wrong submission. Unpinned until #390 — every fixture was a single row,
// where the first and last match are the same row, so reversing this loop left
// the whole suite green (3786 RUN, 0 FAIL). Pinned now by
// TestRecoverTimedOutSubmitReadsTheNewestMatchingRow (both tiers) and the reader
// ledger in internal/cmd/newest_row_pick_test.go, which spans this package for
// exactly this site. That the SERVER orders the list newest-first is an
// unverified dependency on the route's contract: ListSubmissions does not sort
// and no caller inspects the sequence, so a change on that side would leave every
// one of those guards green while this answer went wrong. Not checked is not the
// same as not checkable — every row carries SubmittedAt, so verifying (or
// imposing) the order is possible and simply unwritten.
func latestMatchingSubmission(subs []Submission, slug, version string) *Submission {
	var anyMatch *Submission
	for i := range subs {
		s := &subs[i]
		// 🔴 THE SLUG COMPARE IS SHARED (SameSlug, slug.go); THE VERSION COMPARE
		// IS DELIBERATELY STILL EXACT. They are not the same question. The slug
		// is the app's identity and has one canonical spelling per the manifest
		// schema, so folding case and padding can only rejoin a mis-spelling to
		// the app it names. A version is an ORDERED value whose spelling this
		// function does not own: "0.2.0", " 0.2.0 " and "v0.2.0" are the same
		// release, but deciding that is comparableVersion's job in
		// internal/cmd/approved_version.go, which encodes a policy (a
		// pre-release is NOT ORDERABLE) that a fold-and-trim compare would
		// quietly contradict. Widening it here would also widen what a timed-out
		// submit may report as "the row that landed", which is the id a user
		// hands to `civitai app withdraw`. Out of scope for the slug
		// consolidation, and named so it does not read as an oversight.
		if !SameSlug(s.BlockID, slug) || s.Version != version {
			continue
		}
		switch strings.ToLower(s.Status) {
		case "pending", "submitted":
			return s
		}
		if anyMatch == nil {
			anyMatch = s
		}
	}
	return anyMatch
}

// timedOutSubmitError builds the actionable error returned when a submit timed
// out and no matching submission could be confirmed afterwards.
func timedOutSubmitError(slug string, cause error) error {
	return fmt.Errorf("submit timed out and the upload may not have completed (%v) — "+
		"run `civitai app status %s` to check whether it landed before resubmitting", cause, slug)
}

// WhoAmI verifies the token against /api/v1/me, refreshing the OAuth access
// token transparently if needed.
func (c *Client) WhoAmI(ctx context.Context) (*Identity, error) {
	tok, err := c.Tokens.Token(ctx)
	if err != nil {
		return nil, err
	}
	if tok == "" {
		return nil, civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf("no token configured — run `civitai login` first"))
	}
	build := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/v1/me", nil)
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, serverError(status, raw)
	}
	var id Identity
	if err := json.Unmarshal(raw, &id); err == nil {
		return &id, nil
	}
	// The full parse failed — almost always because a peripheral, server-owned
	// field (buzzLimit, subject.id, …) drifted its JSON type. Don't hard-fail the
	// whole command over a field whoami never renders: fall back to the core
	// identity (the exact fields whoami prints) so a valid /me body still works.
	if core, cerr := parseCoreIdentity(raw); cerr == nil {
		return core, nil
	}
	// 🔴 THE BODY IS NOT ECHOED. It used to be, and that made this error the one
	// path by which a user's email address DID reach a terminal — /api/v1/me
	// carries `email`/`emailVerified`, this branch printed the whole response,
	// and `main` writes it to stderr where a shell redirect or a CI log keeps it.
	// The struct's argument that email is unpublishable because it is unmodelled
	// was false while this line existed, so the fix belongs here rather than in
	// the comment. Diagnosability is preserved by describeMeBody, which reports
	// the SHAPE — the keys present and the offending type — and never a value.
	//
	// The advice names the only action that can help. Re-authenticating cannot:
	// the token was ACCEPTED (this is a 200) and the body's SHAPE is what
	// defeated both parses, so a fresh token produces the identical failure.
	// TestUnparseableMeErrorAdvisesUpgradeNotLogin pins that — an error string
	// with no assertion on it is one careless restore away from reverting, which
	// is how this exact line was lost once already.
	return nil, fmt.Errorf("unexpected /api/v1/me response (%s) — "+
		"the server sent a shape this CLI cannot read; try `civitai upgrade`, "+
		"and report it at https://github.com/civitai/cli/issues", describeMeBody(raw))
}

// parseCoreIdentity unmarshals only the CORE identity (id, username, tokenScope,
// and subject.type) from a /api/v1/me body, ignoring every peripheral field. It
// is the resilience fallback for WhoAmI: even if a future server change breaks
// the strict Identity parse, the core identity still surfaces. It errors only
// when the body is not a JSON object or a core field itself has an incompatible
// type.
//
// 🔴 THE PROFILE FIELDS (Tier/Status/IsMember/Subscriptions) ARE NOT PARSED HERE
// ON PURPOSE, EVEN THOUGH `--json` NOW RENDERS THEM. This function's whole value
// is that it cannot fail on a peripheral type drift; re-listing those fields
// would hand the drift a second chance to kill the command. When this fallback
// fires they degrade to `null` — "the CLI does not have it", which is exactly
// the state the tri-state pointers exist to express — rather than to a
// fabricated zero value.
func parseCoreIdentity(raw []byte) (*Identity, error) {
	var core struct {
		Username   string `json:"username"`
		ID         int    `json:"id"`
		TokenScope *int   `json:"tokenScope"`
		Subject    *struct {
			Type string `json:"type"`
		} `json:"subject"`
	}
	if err := json.Unmarshal(raw, &core); err != nil {
		return nil, err
	}
	id := &Identity{Username: core.Username, ID: core.ID, TokenScope: core.TokenScope}
	if core.Subject != nil {
		id.Subject = &Subject{Type: core.Subject.Type}
	}
	return id, nil
}

// meKeyAllowlist is every /api/v1/me key describeMeBody may NAME. It is an
// allowlist, not a denylist, on purpose: a denylist of known-sensitive keys goes
// stale the moment the server adds one, and the failure is silent and in the
// leaking direction. A key the CLI has never heard of is counted, never printed.
var meKeyAllowlist = map[string]bool{
	"id": true, "username": true, "tier": true, "status": true,
	"isMember": true, "subscriptions": true, "tokenScope": true,
	"buzzLimit": true, "subject": true,
}

// describeMeBody renders the SHAPE of an unparseable /api/v1/me body for an
// error message: its top-level keys (allowlisted ones by name, the rest only
// counted) and each named key's JSON type.
//
// 🔴 IT MUST NEVER RETURN A VALUE FROM THE BODY, ONLY KEYS AND TYPES. The body
// carries `email`/`emailVerified`, and this string is printed to stderr where a
// redirect or a CI log keeps it. Types are what actually diagnose the failure
// this is reached from — a peripheral field that drifted its JSON type — so the
// omission costs nothing diagnostically. TestDescribeMeBodyNeverLeaksValues
// pins it against a body whose every value is a distinct PII-shaped marker.
func describeMeBody(raw []byte) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return fmt.Sprintf("not a JSON object, %d bytes", len(raw))
	}
	named := make([]string, 0, len(obj))
	unknown := 0
	for k, v := range obj {
		if !meKeyAllowlist[k] {
			unknown++
			continue
		}
		named = append(named, k+": "+jsonKindOf(v))
	}
	sort.Strings(named)
	desc := "keys " + strings.Join(named, ", ")
	if len(named) == 0 {
		desc = "no recognised keys"
	}
	if unknown > 0 {
		desc += fmt.Sprintf(" (+%d unrecognised)", unknown)
	}
	return desc
}

// jsonKindOf names a raw JSON value's type without revealing the value.
func jsonKindOf(v json.RawMessage) string {
	t := bytes.TrimSpace(v)
	if len(t) == 0 {
		return "empty"
	}
	switch t[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "bool"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

// GetBuzzAccount reads the caller's spendable Buzz balance via the
// buzz.getBuzzAccount tRPC route, refreshing the OAuth access token if needed.
// On 200 it returns the {blue,green,yellow} balance; on a 403 (the credential
// lacks the Buzz-read scope) it returns ErrBuzzScope so the command layer can
// print the personal-key guidance. The tRPC success envelope is
// {"result":{"data":{"json":{...}}}}.
func (c *Client) GetBuzzAccount(ctx context.Context) (*BuzzAccount, error) {
	tok, err := c.Tokens.Token(ctx)
	if err != nil {
		return nil, err
	}
	if tok == "" {
		return nil, civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf("no token configured — run `civitai login` first"))
	}
	build := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+BuzzAccountPath, nil)
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return nil, err
	}
	if status == http.StatusForbidden {
		return nil, ErrBuzzScope
	}
	if status != http.StatusOK {
		return nil, serverError(status, raw)
	}
	var env struct {
		Result struct {
			Data struct {
				JSON *BuzzAccount `json:"json"`
			} `json:"data"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || env.Result.Data.JSON == nil {
		return nil, fmt.Errorf("unexpected buzz.getBuzzAccount response: %s", string(raw))
	}
	return env.Result.Data.JSON, nil
}

// submissionsURL builds the GET URL with optional id / blockId query params.
func (c *Client) submissionsURL(id, blockID string) string {
	u := c.BaseURL + SubmissionsPath
	q := url.Values{}
	if id != "" {
		q.Set("id", id)
	}
	if blockID != "" {
		q.Set("blockId", blockID)
	}
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

// ListSubmissions returns the caller's own submissions (newest first). An empty
// blockID lists all; a non-empty blockID narrows to that app's submissions.
func (c *Client) ListSubmissions(ctx context.Context, blockID string) ([]Submission, error) {
	build := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, c.submissionsURL("", blockID), nil)
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, submissionsError(status, raw, "", blockID)
	}
	var out struct {
		Submissions []Submission `json:"submissions"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("unexpected /api/v1/blocks/submissions response: %s", string(raw))
	}
	return out.Submissions, nil
}

// GetSubmission returns a single submission. Exactly one of id (a pubreq id) or
// blockID (an app slug) should be set; id takes precedence if both are given.
//
// It is the narrow spelling of GetSubmissionRows: identical request, identical
// row pick, and the rest of the narrowed listing discarded.
func (c *Client) GetSubmission(ctx context.Context, id, blockID string) (*Submission, error) {
	s, _, err := c.getSubmissionRows(ctx, id, blockID)
	return s, err
}

// GetSubmissionRows is GetSubmission plus the rows it read to answer.
//
// 🔴 IT EXISTS SO A CALLER DOES NOT HAVE TO ASK TWICE. The `?blockId=` spelling
// of this route answers with the app's WHOLE narrowed listing and GetSubmission
// throws all of it away but Submissions[0] — so `app status <slug>`'s drift
// check used to re-issue the byte-identical GET (submissionsURL("", blockID))
// just to see the rows this call already held. Same URL, same auth, same page;
// only the latency and the rate-limit budget were extra.
//
// The second return is the row set BEHIND the answer, and it is nil — not empty
// — whenever there is no such set to hand back. That distinction is the whole
// contract: the `?id=` spelling answers with a single-row envelope and no
// listing at all, so a caller that needs every row for the app must still fetch
// it itself. A nil return therefore means "I did not read a listing", never
// "the listing was empty"; the empty listing is a real, distinct answer (a slug
// with no submissions) and it is reported as a not-found error above, not as
// nil rows.
//
// Rows are returned as read (newest first, per the route) and are NOT filtered
// or reordered here — highestApprovedVersion and friends do their own filtering
// and must see exactly what the server sent.
func (c *Client) GetSubmissionRows(ctx context.Context, id, blockID string) (*Submission, []Submission, error) {
	return c.getSubmissionRows(ctx, id, blockID)
}

// getSubmissionRows is the single implementation both spellings above delegate
// to. It is unexported ON PURPOSE: newest_row_pick_test.go's ledger derives
// "every EXPORTED func that reaches submissionsURL" and follows unexported
// chains, so keeping the body private makes the derivation report BOTH exported
// boundaries — GetSubmission and GetSubmissionRows — instead of silently
// dropping whichever one became a wrapper. A wrapper is still an accessor to
// every caller that uses it.
func (c *Client) getSubmissionRows(ctx context.Context, id, blockID string) (*Submission, []Submission, error) {
	build := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, c.submissionsURL(id, blockID), nil)
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return nil, nil, err
	}
	if status != http.StatusOK {
		return nil, nil, submissionsError(status, raw, id, blockID)
	}
	// An `?id=` lookup returns {submission: {...}}; an `?blockId=` lookup returns
	// {submissions: [...]} (narrowed list) — handle both so either selector works.
	var single struct {
		Submission *Submission `json:"submission"`
	}
	if err := json.Unmarshal(raw, &single); err == nil && single.Submission != nil {
		// No listing was read on this path — nil rows, deliberately (see the doc
		// comment): a caller needing every row for the app must fetch it itself.
		return single.Submission, nil, nil
	}
	// 🔴 THE `?blockId=` FORM IS A NARROWED LIST, NOT A SINGLE ROW, and the row
	// taken below is `Submissions[0]` — NEVER `Submissions[len-1]`. Every
	// submission ever made for that slug comes back, newest first. That one row
	// is printed verbatim by `app status <slug>` (version, status, deploy state)
	// and is where resolveListing takes the appBlockId deciding WHICH listing
	// every `app listing` subcommand reads and MUTATES. Unpinned until the
	// adversarial audit of #390's first fix: every fixture here was one row,
	// where both ends ARE the same row, so reversing the pick left the whole
	// suite green (3812 RUN, 0 FAIL). Pinned now by
	// TestGetSubmissionByBlockIDTakesTheNewestRow, the two per-surface cases in
	// internal/cmd, and the reader ledger in newest_row_pick_test.go. Whether the
	// SERVER really orders it newest-first is not checked client-side today —
	// every row carries SubmittedAt, so such a check is possible, just unwritten.
	var list struct {
		Submissions []Submission `json:"submissions"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, nil, fmt.Errorf("unexpected /api/v1/blocks/submissions response: %s", string(raw))
	}
	if len(list.Submissions) == 0 {
		// A `?blockId=` lookup for an unknown slug answers 200 with an EMPTY list
		// rather than a 404, so this is resolved client-side and never reaches
		// submissionsError's TagStatus. Tag it here or the exit code silently
		// differs from the `?id=` spelling of the same question (which does 404
		// and maps to exit 4). Tag adds no text of its own.
		//
		// This is the FIRST wall a user hits after `app create`: `app listing
		// status` and `app status <slug>` both land here before anything has been
		// submitted, and a bare "no such submission" named no next command in a
		// tool whose house style always does (civitai/cli#363). The submission —
		// and with it the draft store listing the listing commands need — is
		// created by `app submit`, so that is the step to name.
		return nil, nil, civitai.Tag(civitai.ErrNotFound, fmt.Errorf(
			"no such submission for %s — run `civitai app submit` first; the submission and its draft store listing are created at submit time (list what you have submitted with `civitai app status`)",
			submissionSubject(id, blockID)))
	}
	return &list.Submissions[0], list.Submissions, nil
}

// submissionSubject names WHICH lookup came back empty, using the selector the
// caller actually passed (GetSubmission takes either a pubreq id or a slug, and
// id wins when both are set — mirror that order here or the error names a
// selector the request did not use). The id-first order is REACHABLE and pinned
// by TestSubmissionSelectorPrecedenceMirrorsTheRequest: `civitai app status --id
// <id> <slug>` passes both, and submissionsURL sends only the id.
func submissionSubject(id, blockID string) string {
	if s := strings.TrimSpace(id); s != "" {
		return fmt.Sprintf("submission id %q", s)
	}
	if s := strings.TrimSpace(blockID); s != "" {
		return fmt.Sprintf("app %q", s)
	}
	return "this app"
}

// submissionNotFoundAdvice is the next step for a 404 from the submissions
// route, keyed on the SELECTOR — see submissionsError's 404 arm for why the two
// answers must differ. Same precedence as submissionSubject (id wins).
func submissionNotFoundAdvice(id, blockID string) string {
	if strings.TrimSpace(id) != "" {
		return "check the publish-request id: `civitai app status <blockId>` shows the id for one app, and `civitai app status --json` lists them all"
	}
	if strings.TrimSpace(blockID) != "" {
		return "run `civitai app submit` first; the submission and its draft store listing are created at submit time (list what you have submitted with `civitai app status`)"
	}
	return "list what you have submitted with `civitai app status`"
}

// withdrawBody is the POST /api/v1/blocks/withdraw request body.
type withdrawBody struct {
	PublishRequestID string `json:"publishRequestId"`
}

// WithdrawRequest withdraws the caller's own pending publish request. A 200 is
// success (the server is idempotent: already-withdrawn also returns 200). A
// non-2xx is mapped to an actionable error by withdrawError.
func (c *Client) WithdrawRequest(ctx context.Context, publishRequestID string) error {
	body, err := json.Marshal(withdrawBody{PublishRequestID: publishRequestID})
	if err != nil {
		return err
	}
	build := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+WithdrawPath, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return withdrawError(status, raw)
	}
	return nil
}

// ErrSlugRegisteredToOtherAccount is wrapped by MintDevToken's error when the
// dev-token route 404s with the bare "App not found" — the server's anti-shadow
// guard: the requested slug is an APPROVED app owned by a DIFFERENT account, so
// the no-row local-manifest mint path is refused. It is the ONLY rename-retriable
// 404 (the caller can pick a new, free slug and retry). Other 404s (e.g. an
// owned-but-not-yet-deployed app, which carries a "no live deployment" message)
// are NOT retriable and do NOT wrap this sentinel. Callers branch with
// errors.Is(err, ErrSlugRegisteredToOtherAccount) rather than matching strings.
var ErrSlugRegisteredToOtherAccount = errors.New("slug is registered to a different account")

// CloneInfoPath is the tRPC query that returns the caller's per-user Forgejo
// clone info for one of THEIR apps (owner-only, App-Blocks-flag-gated). Backs
// `civitai app pull`. The token is embedded in CloneURL (HTTP-Basic) — caller
// must treat it as a secret (see the leakage caveat in `civitai app pull`).
const CloneInfoPath = "/api/trpc/blocks.getMyForgejoCloneInfo"

// ForgejoCloneInfo mirrors the getMyForgejoCloneInfo result. When the app's
// first version has not yet been ZIP-approved the server returns
// NotYetAvailable=true (no credential is minted) with a Message explaining why.
type ForgejoCloneInfo struct {
	NotYetAvailable bool   `json:"notYetAvailable"`
	Slug            string `json:"slug"`
	Message         string `json:"message"`
	ForgejoUsername string `json:"forgejoUsername"`
	Token           string `json:"token"`
	HTTPURL         string `json:"httpUrl"`
	CloneURL        string `json:"cloneUrl"`
}

// GetForgejoCloneInfo calls the owner-only getMyForgejoCloneInfo tRPC query for
// the given app (a slug — the repo name — or an appBlockId). It lazily
// provisions the caller's scoped Forgejo identity server-side and returns the
// tokened clone URL the `pull` command hands to git.
func (c *Client) GetForgejoCloneInfo(ctx context.Context, app string) (*ForgejoCloneInfo, error) {
	tok, err := c.Tokens.Token(ctx)
	if err != nil {
		return nil, err
	}
	if tok == "" {
		return nil, civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf("no token configured — run `civitai login` first"))
	}

	// tRPC query input: ?input={"json":{"slug":"<app>"}}. The server accepts the
	// human-friendly slug OR an appBlockId; the slug is what a developer knows, so
	// we always send `slug` (an appBlockId is also a valid blockId lookup miss →
	// the server falls through to NOT_FOUND, which the caller reports cleanly).
	inputJSON, err := json.Marshal(map[string]any{"json": map[string]string{"slug": app}})
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("input", string(inputJSON))
	reqURL := c.BaseURL + CloneInfoPath + "?" + q.Encode()

	build := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, cloneInfoError(status, raw)
	}
	var env struct {
		Result struct {
			Data struct {
				JSON *ForgejoCloneInfo `json:"json"`
			} `json:"data"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || env.Result.Data.JSON == nil {
		return nil, fmt.Errorf("unexpected getMyForgejoCloneInfo response: %s", string(raw))
	}
	return env.Result.Data.JSON, nil
}

// cloneInfoError maps a non-200 from the clone-info tRPC query to an actionable
// message. tRPC error bodies are {error:{json:{message,code,...}}}.
func cloneInfoError(status int, raw []byte) (err error) {
	defer func() { err = civitai.TagStatus(status, err) }()
	var env struct {
		Error struct {
			JSON struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"json"`
		} `json:"error"`
	}
	msg := strings.TrimSpace(string(raw))
	if json.Unmarshal(raw, &env) == nil && env.Error.JSON.Message != "" {
		msg = env.Error.JSON.Message
	}
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("not authenticated — run `civitai login` (or set CIVITAI_TOKEN): %s", msg)
	case http.StatusForbidden:
		return fmt.Errorf("not permitted (are you the app owner, and is Apps enabled for your account?): %s", msg)
	case http.StatusNotFound:
		return fmt.Errorf("no such app for your account — check the slug with `civitai app status`: %s", msg)
	default:
		return fmt.Errorf("getMyForgejoCloneInfo failed (HTTP %d): %s", status, msg)
	}
}

// Vendored dev-token Buzz-budget bounds. Both mirror
// civitai/civitai src/server/services/blocks/dev-scoped-mint.service.ts:
//
//	export const DEV_BUZZ_BUDGET_CAP = 250;
//	export const DEV_BUZZ_BUDGET_DEFAULT = 50;
//
// which that file's resolveDevBuzzBudget composes as
//
//	Math.min(requestedBudget ?? manifestDefaultBudget ?? DEFAULT, CAP)
//
// gated on the minted token retaining `ai:write:budgeted` (without the spend
// scope the claim is dropped entirely and no budget is issued, however large the
// request).
//
// The two bounds fail DIFFERENTLY server-side, which is why the CLI checks both
// itself rather than deferring:
//   - Below 1 the route's zod schema (`z.number().int().positive()`) answers 400.
//   - ABOVE the cap there is no schema bound at all — the request succeeds and
//     `Math.min` silently clamps. A developer who asks for 400 gets a 250-budget
//     token and no indication the number moved, which is the more expensive
//     failure of the two and cannot be detected after the fact from the CLI.
//
// Neither constant is observable from a minted token (the JWT's budget claim
// reflects the RESOLVED value, so a clamped 250 and a requested 250 are
// byte-identical), so there is no sound live drift probe to write here — unlike
// ListSubmissionsCap above, whose truncation asymmetry makes one possible. These
// are pinned by TestDevBuzzBudgetBoundsMirrorServer and by the comment above;
// re-read the server file when touching them.
const (
	// DevBuzzBudgetCap is the largest per-generation Buzz budget the dev-token
	// route will issue. A larger request is clamped, not refused.
	DevBuzzBudgetCap = 250
	// DevBuzzBudgetMin is the smallest budget the route's schema accepts.
	DevBuzzBudgetMin = 1
	// DevBuzzBudgetDefault is what the server resolves to when NEITHER the
	// request nor the app's stored manifest names a budget.
	DevBuzzBudgetDefault = 50
)

// DevTokenMinter mints a short-lived dev block token for `npm run dev:live`.
type DevTokenMinter interface {
	// MintDevToken mints a dev block token for the given app slug and returns
	// the JWT. scopes carries the caller's LOCAL block.manifest.json scopes for
	// the server's no-row mint path (clamped server-side); pass nil/empty when
	// no manifest is available. buzzBudget is the optional per-generation Buzz
	// budget — nil means "not requested", leaving the server's own resolution
	// intact. requestBudgetedSpend states this mint's SPEND INTENT and is always
	// sent (see devTokenBody). A non-2xx is mapped by devTokenError.
	MintDevToken(ctx context.Context, slug string, scopes []string, buzzBudget *int, requestBudgetedSpend bool) (string, error)
}

// devTokenBody is the POST /api/v1/blocks/dev-token request body. Scopes is the
// caller's LOCAL manifest scopes; it is omitted (not sent) when empty so a
// registered app's server-side scopes still govern.
//
// BuzzBudget is a POINTER, and that is load-bearing rather than stylistic: the
// server resolves an ABSENT `buzzBudget` differently from a present one, so a
// plain `int` would make the CLI send `0` (or a CLI-chosen default) on every
// mint and permanently shadow the server's own resolution. Only nil is omitted
// by `omitempty` on a pointer, so nil is the one encoding that reproduces
// today's wire shape exactly.
//
// RequestBudgetedSpend is the CLI's SPEND INTENT for this mint
// (civitai/civitai#3703 step 1). It is a plain bool with NO `omitempty`, and
// both halves of that are deliberate — this is the one field on this struct
// whose correct encoding is the OPPOSITE of BuzzBudget's:
//
//	🔴 `omitempty` WOULD INVERT THE MEANING OF `false`. The server resolves the
//	field as `spendRequested = requestBudgetedSpend ?? true`, so ABSENT means
//	"yes, infer spend from the bearer". With `omitempty` on a bool, `false`
//	serializes to nothing — the CLI would say "no" and the server would read
//	"yes". A default mint would keep requesting budgeted spend implicitly,
//	which is the exact thing this field exists to stop.
//
//	🔴 A `*bool` WOULD ADD A STATE THE CLI MUST NEVER SEND. Tri-state is only
//	worth its cost when the caller can genuinely be undecided; `--spend` is a
//	bool flag with no third position, so the CLI always knows its intent. The
//	only thing nil would buy is a way to silently reintroduce inference — and
//	because a forgotten field in a Go struct literal is the ZERO value, a plain
//	bool makes the forgetful case `false` (state "no spend", the conservative
//	answer) rather than nil (state nothing, and get spend inferred). The safe
//	value is the zero value; that is the whole argument.
//
//	Contrast BuzzBudget, a pointer for the mirror-image reason: there the
//	server's absent-resolution (manifest default, then 50) is a value the CLI
//	cannot name, so "say nothing" is a real, correct request. Here it is not.
//
// Compatibility: the route's zod schema is a non-strict `z.object`, so a
// deployment predating step 1 STRIPS the unknown key and behaves exactly as it
// does today. Sending it is safe against both.
//
// Claim discipline: this makes the CLI STATE ITS SPEND INTENT EXPLICITLY. It is
// not a claim that spend can no longer be inferred anywhere — the CLI still
// sends no `scopes` narrowing when there is no local manifest, and nothing here
// changes what other clients (SDK live host, dev tunnel, mod review) send.
type devTokenBody struct {
	Slug                 string   `json:"slug"`
	Scopes               []string `json:"scopes,omitempty"`
	BuzzBudget           *int     `json:"buzzBudget,omitempty"`
	RequestBudgetedSpend bool     `json:"requestBudgetedSpend"`
}

// MintDevToken mints a short-lived dev block token for the given app slug,
// returning the JWT from the response's .token field. scopes carries the
// caller's local manifest scopes for the server's no-row (no app registered
// yet) mint path; they are clamped server-side and omitted from the body when
// empty/nil (registered-app and read-only paths are unaffected).
//
// buzzBudget is the optional per-generation Buzz budget the token should carry.
// Pass nil to request nothing — the key is then absent from the body and the
// server resolves the budget itself (see the DevBuzzBudget* constants). Callers
// are expected to have range-checked a non-nil value; this method sends what it
// is given so that a bound the server changes still reaches it.
//
// requestBudgetedSpend states whether THIS mint asks for `ai:write:budgeted`.
// Unlike scopes and buzzBudget it is ALWAYS serialized (see devTokenBody), so
// there is no "say nothing" option and callers must pass their real intent —
// for the CLI that is the `--spend` flag, the same value that drives the scope
// narrowing. The two must never disagree: a body asking for spend while the
// scopes list strips it (or the reverse) is a bug in the caller, not something
// this method reconciles.
//
// The OAuth access token is refreshed transparently on a 401. A non-2xx is
// mapped by devTokenError.
func (c *Client) MintDevToken(ctx context.Context, slug string, scopes []string, buzzBudget *int, requestBudgetedSpend bool) (string, error) {
	body, err := json.Marshal(devTokenBody{
		Slug:                 slug,
		Scopes:               scopes,
		BuzzBudget:           buzzBudget,
		RequestBudgetedSpend: requestBudgetedSpend,
	})
	if err != nil {
		return "", err
	}
	build := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+DevTokenPath, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", devTokenError(status, raw)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("unexpected /api/v1/blocks/dev-token response: %s", string(raw))
	}
	if out.Token == "" {
		return "", fmt.Errorf("dev-token response had no token: %s", string(raw))
	}
	return out.Token, nil
}

// devTokenValidationDetail renders the per-field detail the dev-token route
// attaches to a 400. The route answers a schema violation with
//
//	{"message":"Invalid request body","details":{"formErrors":[],"fieldErrors":{"buzzBudget":["Too small: expected number to be >0"]}}}
//
// i.e. zod's flatten(). The top-level message is the same generic sentence for
// EVERY malformed field, so without the field detail a rejected `--budget` is
// indistinguishable from a rejected slug — the caller learns that something was
// wrong and nothing about what. Field names are sorted so the text is stable.
//
// Returns "" when the body carries no recognizable detail, so the caller falls
// back to the plain message rather than printing an empty parenthetical.
func devTokenValidationDetail(raw []byte) string {
	var env struct {
		Details struct {
			FormErrors  []string            `json:"formErrors"`
			FieldErrors map[string][]string `json:"fieldErrors"`
		} `json:"details"`
	}
	if json.Unmarshal(raw, &env) != nil {
		return ""
	}
	fields := make([]string, 0, len(env.Details.FieldErrors))
	for f := range env.Details.FieldErrors {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	parts := make([]string, 0, len(fields)+len(env.Details.FormErrors))
	for _, f := range fields {
		if msgs := env.Details.FieldErrors[f]; len(msgs) > 0 {
			parts = append(parts, f+": "+strings.Join(msgs, "; "))
		}
	}
	parts = append(parts, env.Details.FormErrors...)
	return strings.Join(parts, " | ")
}

// devTokenError maps a non-2xx dev-token response to a clear, actionable CLI
// error. The route returns {"message": ...} on every error status: 400
// schema-rejected (with per-field details), 404 not-found-or-not-yours, 403
// not-invited-or-insufficient-scope, 429 rate-limited, 503 flag-off. The 403
// message is the key DX case — a spend token needs a bearer carrying
// AIServicesWrite, i.e. a full-scope personal API key OR an OAuth login that
// opted in via `civitai login --scopes generate`. A DEFAULT `civitai login`
// mints read-only.
//
// That entitlement is NECESSARY, not sufficient: since #3703 step 1 the mint
// also needs the request to ASK (`requestBudgetedSpend`), and a request that
// does not ask is not an error — the scope is silently stripped and the mint
// returns 200. So a read-only token is NOT diagnosable from this function; it is
// caught after the fact by decoding the minted JWT (see the CLI's tokenCanSpend
// / readOnlyTokenWarning).
func devTokenError(status int, raw []byte) (err error) {
	defer func() { err = civitai.TagStatus(status, err) }()
	msg := serverMessage(raw)
	switch status {
	case http.StatusBadRequest:
		// Report the server's OWN verdict rather than re-deriving one: the CLI
		// range-checks --budget before sending, so reaching here means the
		// server's rules and the CLI's understanding of them have diverged, and
		// that divergence is exactly what has to reach the developer intact.
		if detail := devTokenValidationDetail(raw); detail != "" {
			return fmt.Errorf("the server rejected the request (400): %s — %s", msg, detail)
		}
		return fmt.Errorf("the server rejected the request (400): %s", msg)
	case http.StatusNotFound:
		// Two 404 shapes: the anti-shadow guard returns a bare "App not found"
		// when the slug is an approved app owned by another account (the no-row
		// mint is refused) — that one is rename-retriable, so wrap the sentinel.
		// The owned-but-not-yet-deployed 404 carries a "no live deployment"
		// message and is NOT retriable (you own the slug; renaming is wrong).
		if strings.Contains(strings.ToLower(msg), "no live deployment") {
			return fmt.Errorf("app not found (404): %s", msg)
		}
		return fmt.Errorf("app not found (404): %s — check the slug. (dev-token mints from your local block.manifest.json; a 404 means the slug is registered to a different account.): %w", msg, ErrSlugRegisteredToOtherAccount)
	case http.StatusUnauthorized:
		return fmt.Errorf("not logged in (401): %s — run `civitai login` (or set CIVITAI_TOKEN)", msg)
	case http.StatusForbidden:
		return fmt.Errorf("not authorized (403): %s — minting needs an invite (invite-only beta) AND a credential carrying the AI Services scopes: `civitai login --scopes generate` or a full-scope personal API key. A DEFAULT OAuth `civitai login` token can't mint a spend token (check with `civitai whoami`)", msg)
	case http.StatusTooManyRequests:
		return fmt.Errorf("rate limited, try again shortly (429): %s", msg)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("apps unavailable (503): %s", msg)
	default:
		return fmt.Errorf("server returned %d: %s", status, msg)
	}
}

// ── APP DEV TUNNEL (P2 CLI ↔ P1 server contract) ─────────────────────────────
//
// The `civitai app dev-tunnel` command drives three tRPC procedures on the
// `blocks` router (civitai/civitai src/server/routers/blocks.router.ts):
//
//   - blocks.startDevTunnel  (mutation) input  { blockId, sshPublicKey, declaredScopes? }
//                                      result { sessionId, host, url, expiresAt, spendCapBuzz }
//   - blocks.stopDevTunnel   (mutation) input  { sessionId? , blockId? } (one required)
//                                      result { ok, stopped }
//
// These are non-batched tRPC HTTP calls: the request body is `{"json": <input>}`
// and the success envelope is `{"result":{"data":{"json":<result>}}}` (superjson),
// exactly matching GetBuzzAccount above. All three procedures are gated
// server-side behind `appDeveloperProcedure` + the dark `app-blocks-dev-tunnel`
// kill-switch, so until P1 merges + P3 flips the flag every call answers
// FORBIDDEN — that is expected (the command is inert end-to-end pre-P3).
//
// The CLI sends its EPHEMERAL SSH PUBLIC key: the server keys the tunnel
// credential by sha256(normalized pubkey) and the CLI's `ssh -R` bind presents
// the matching private key (which never leaves memory). See
// dev-tunnel-session.ts (normalizeSshPublicKey / fingerprintSshPublicKey).

// StartDevTunnelPath / StopDevTunnelPath are the non-batched tRPC routes.
const (
	StartDevTunnelPath = "/api/trpc/blocks.startDevTunnel"
	StopDevTunnelPath  = "/api/trpc/blocks.stopDevTunnel"
)

// DevTunnelSession mirrors blocks.startDevTunnel's result (the server's
// StartDevTunnelResult in dev-tunnel.service.ts). Field names + JSON casing
// track the server EXACTLY.
type DevTunnelSession struct {
	SessionID string `json:"sessionId"`
	// Host is the assigned unguessable `dev-<16hex>.<APPS_DOMAIN>` the reverse
	// tunnel binds to; the CLI passes it to `ssh -R` as the remote bind host.
	Host string `json:"host"`
	// URL is the `/apps/dev/<blockId>` page the developer opens in their browser.
	URL string `json:"url"`
	// ExpiresAt is the hard-TTL expiry (unix seconds) after which the server
	// reaper reclaims the route even if the CLI never calls stopDevTunnel.
	ExpiresAt int64 `json:"expiresAt"`
	// SpendCapBuzz is the per-session cumulative Buzz ceiling (backstop).
	SpendCapBuzz int64 `json:"spendCapBuzz"`
	// SSHHostPublicKey is the sish endpoint's OpenSSH host public-key line
	// (`ssh-ed25519 AAAA...`) — a NON-SECRET value the CLI PINS as the SSH
	// HostKeyCallback so the `ssh -R` bind can't be MITM'd (an on-path attacker
	// impersonating sish would reach the dev's localhost + tamper tunneled
	// traffic). The mint returns it; the CLI fails closed if it is absent
	// (never falls back to InsecureIgnoreHostKey).
	SSHHostPublicKey string `json:"sshHostPublicKey"`
}

// DevTunnelController mints + revokes a dev-tunnel session. Behind an interface
// so the command layer is testable without a live server.
type DevTunnelController interface {
	// StartDevTunnel mints a tunnel credential + host for blockId, binding it to
	// the caller's ephemeral SSH public key. declaredScopes carries the LOCAL
	// manifest's `scopes` so the server can grant them to an UNSUBMITTED app's
	// tunnel token (empty = read-only). Returns the assigned host + the /apps/dev
	// URL the developer opens.
	StartDevTunnel(ctx context.Context, blockID, sshPublicKey string, declaredScopes []string) (*DevTunnelSession, error)
	// StopDevTunnel revokes the caller's tunnel by sessionId (preferred) or, when
	// sessionId is empty, by blockId. Returns whether a session was torn down.
	StopDevTunnel(ctx context.Context, sessionID, blockID string) (bool, error)
}

// startDevTunnelInput mirrors the blocks.startDevTunnel zod input. DeclaredScopes
// is `omitempty` so an empty/absent local manifest sends nothing — matching the
// server's `.optional()` and keeping the request identical to the pre-scopes
// shape (an old server ignores the extra field).
type startDevTunnelInput struct {
	BlockID        string   `json:"blockId"`
	SSHPublicKey   string   `json:"sshPublicKey"`
	DeclaredScopes []string `json:"declaredScopes,omitempty"`
}

// stopDevTunnelInput mirrors the blocks.stopDevTunnel zod input (one of the two
// is set). `omitempty` so exactly the provided selector is sent.
type stopDevTunnelInput struct {
	SessionID string `json:"sessionId,omitempty"`
	BlockID   string `json:"blockId,omitempty"`
}

// StartDevTunnel POSTs blocks.startDevTunnel and returns the minted session. The
// OAuth access token is refreshed transparently on a 401.
func (c *Client) StartDevTunnel(ctx context.Context, blockID, sshPublicKey string, declaredScopes []string) (*DevTunnelSession, error) {
	body, err := json.Marshal(map[string]any{"json": startDevTunnelInput{BlockID: blockID, SSHPublicKey: sshPublicKey, DeclaredScopes: declaredScopes}})
	if err != nil {
		return nil, err
	}
	build := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+StartDevTunnelPath, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, devTunnelError(status, raw)
	}
	var env struct {
		Result struct {
			Data struct {
				JSON *DevTunnelSession `json:"json"`
			} `json:"data"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || env.Result.Data.JSON == nil {
		return nil, fmt.Errorf("unexpected blocks.startDevTunnel response: %s", string(raw))
	}
	if env.Result.Data.JSON.Host == "" || env.Result.Data.JSON.SessionID == "" {
		return nil, fmt.Errorf("blocks.startDevTunnel response missing host/sessionId: %s", string(raw))
	}
	return env.Result.Data.JSON, nil
}

// StopDevTunnel POSTs blocks.stopDevTunnel. A non-empty sessionID selects by
// session (preferred); otherwise blockID selects the caller's active tunnel for
// that app. Returns whether the server tore a session down.
func (c *Client) StopDevTunnel(ctx context.Context, sessionID, blockID string) (bool, error) {
	if sessionID == "" && blockID == "" {
		return false, fmt.Errorf("stopDevTunnel needs a sessionId or blockId")
	}
	body, err := json.Marshal(map[string]any{"json": stopDevTunnelInput{SessionID: sessionID, BlockID: blockID}})
	if err != nil {
		return false, err
	}
	build := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+StopDevTunnelPath, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return false, err
	}
	if status != http.StatusOK {
		return false, devTunnelError(status, raw)
	}
	var env struct {
		Result struct {
			Data struct {
				JSON struct {
					OK      bool `json:"ok"`
					Stopped bool `json:"stopped"`
				} `json:"json"`
			} `json:"data"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return false, fmt.Errorf("unexpected blocks.stopDevTunnel response: %s", string(raw))
	}
	return env.Result.Data.JSON.Stopped, nil
}

// DevTunnelForbiddenError is returned when the dev-tunnel mint is refused with 403.
// Typed so the command layer can errors.As it and give the RIGHT fix, which differs
// by cause:
//   - InsufficientScope: the CLI's credential lacks Full scope (the token-scope
//     gate runs before the author/flag gates) → fix is a full-scope personal API
//     key, NOT a different account.
//   - otherwise: the account lacks the Apps-author invite + dev-tunnel flag → fix
//     is signing in as an enrolled account.
type DevTunnelForbiddenError struct {
	ServerMsg         string
	InsufficientScope bool
}

func (e *DevTunnelForbiddenError) Error() string {
	if e.InsufficientScope {
		return fmt.Sprintf("dev tunnels need a full-scope credential (403): %s", e.ServerMsg)
	}
	return fmt.Sprintf("dev tunnels are not available for your account (403): %s — needs an Apps-author invite AND the dev-tunnel flag (dark until GA)", e.ServerMsg)
}

// isInsufficientScopeMsg detects the server's token-SCOPE refusal. It is the only
// wire signal that distinguishes it from the author/flag 403s — both are TRPCError
// FORBIDDEN with no distinct code — so we classify on the stable core of the server
// message "Your API key does not have the required scope for this action".
func isInsufficientScopeMsg(msg string) bool {
	return strings.Contains(strings.ToLower(msg), "required scope")
}

// isModeratorTakedownMsg detects the server's moderator-TAKEDOWN refusal, for
// exactly the reason isInsufficientScopeMsg exists: the wire carries no distinct
// code for it either — both are a TRPCError FORBIDDEN — so the server message is
// the only signal separating it from the author/cohort 403s. It is therefore
// classified on the stable CORE of that message rather than on a whole sentence.
//
// Two spellings ship upstream and the core is what they share, which is why the
// match is neither exact nor case-sensitive (`civitai/civitai@origin/main`):
//
//   - the edit refusal, lower-case — "this listing has been removed by a
//     moderator and can no longer be edited"
//     (src/server/services/blocks/offsite-listing.service.ts:1242 on the
//     updateListing write path, :1888 on the prefill read path);
//   - the republish refusal, capitalised — "This listing was removed by a
//     moderator and cannot be restored by its owner."
//     (src/server/services/blocks/offsite-moderation.service.ts:1638).
//
// 🔴 It means a moderator DELIST and never an owner-initiated unpublish. The
// same helper that produces the first string returns `null` when the listing was
// owner-unpublished instead (offsite-listing.service.ts:2320), so the string
// cannot be reached for a listing its owner took down. That case is a different
// refusal entirely — MATERIAL_CHANGE_BLOCKED, answered as HTTP 400, classified
// here as a bad request (exit 2) — and must not be folded into this one.
func isModeratorTakedownMsg(msg string) bool {
	return strings.Contains(strings.ToLower(msg), "removed by a moderator")
}

// devTunnelError maps a non-200 dev-tunnel tRPC response to an actionable CLI
// error. tRPC error bodies are {error:{json:{message,code,...}}}; the HTTP
// status carries the mapped code (403 flag-off/not-author, 404 not-your-app).
func devTunnelError(status int, raw []byte) (err error) {
	defer func() { err = civitai.TagStatus(status, err) }()
	var env struct {
		Error struct {
			JSON struct {
				Message string `json:"message"`
			} `json:"json"`
		} `json:"error"`
	}
	msg := serverMessage(raw)
	if json.Unmarshal(raw, &env) == nil && env.Error.JSON.Message != "" {
		msg = env.Error.JSON.Message
	}
	switch status {
	case http.StatusUnauthorized:
		// A 401 here is always a missing/expired/invalid credential; the server
		// message on this path is the origin-gate string ("Please use the public
		// API instead"), which is misleading to a CLI user. Drop it — the only
		// action is `civitai login`.
		return fmt.Errorf("not logged in (401) — run `civitai login` (or set CIVITAI_TOKEN)")
	case http.StatusForbidden:
		return &DevTunnelForbiddenError{ServerMsg: msg, InsufficientScope: isInsufficientScopeMsg(msg)}
	case http.StatusNotFound:
		// With the ephemeral pre-submit resolver deployed (civitai #2983/#2984), a
		// brand-new UNCLAIMED app owned by the caller now tunnels WITHOUT submitting
		// (run `civitai app dev-tunnel` from the app dir). The server returns
		// NOT_FOUND now only when the slug is registered/claimed by a DIFFERENT
		// account (anti-shadow refusal) or the blockId isn't a valid canonical slug
		// (the #2984 SLUG_REGEX guard maps a malformed slug → null → NOT_FOUND). A
		// caller lacking cohort access gets a 403 (DevTunnelForbiddenError), not this
		// 404 — so don't mention the invite/cohort here.
		return fmt.Errorf("can't tunnel this app (404): %s — that slug is registered to a different account, or isn't a valid app slug. A new app of your own now tunnels without submitting (run this from its dir); list your apps with `civitai app status`", msg)
	case http.StatusTooManyRequests:
		return fmt.Errorf("rate limited, try again shortly (429): %s", msg)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("dev tunnels unavailable (503): %s", msg)
	default:
		return fmt.Errorf("server returned %d: %s", status, msg)
	}
}

// withdrawError maps a non-2xx withdraw response to a clear, actionable CLI
// error. The withdraw route returns {"message": ...} on every error status:
// 404 not-found-or-not-yours, 409 not-in-a-withdrawable-(pending)-state (the
// server's message carries the reason), 401/403 auth, 429 rate-limited, 503
// flag-off/rate-limiter-incident.
func withdrawError(status int, raw []byte) (err error) {
	defer func() { err = civitai.TagStatus(status, err) }()
	msg := serverMessage(raw)
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("not authorized (check your API key / Apps invite) (%d): %s", status, msg)
	case http.StatusNotFound:
		return fmt.Errorf("publish request not found (or not yours) (404): %s", msg)
	case http.StatusConflict:
		// The request is not in a withdrawable (pending) state; the server's
		// message is already a complete sentence, so surface it verbatim.
		return fmt.Errorf("%s (409)", msg)
	case http.StatusTooManyRequests:
		return fmt.Errorf("rate limited, try again shortly (429): %s", msg)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("apps unavailable (503): %s", msg)
	default:
		return fmt.Errorf("server returned %d: %s", status, msg)
	}
}

// submissionsError maps a non-2xx submissions response to a clear, actionable
// error, with Apps-specific guidance for 403/404/429/503. id and blockID are the
// selectors the failing request carried (both empty for the unfiltered listing);
// only the 404 arm reads them — see the comment there.
func submissionsError(status int, raw []byte, id, blockID string) (err error) {
	defer func() { err = civitai.TagStatus(status, err) }()
	msg := serverMessage(raw)
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("not logged in (401): %s — run `civitai login`", msg)
	case http.StatusForbidden:
		return fmt.Errorf("apps access required — invite-only beta (403): %s", msg)
	case http.StatusNotFound:
		// 🔴 THE TWO SELECTORS DO NOT MEAN THE SAME THING HERE, and only one of
		// them can realistically reach this arm. A `?blockId=` miss answers 200
		// with an EMPTY LIST, resolved client-side in GetSubmission above — so a
		// real 404 is almost always the `?id=` spelling, i.e. a mistyped or stale
		// publish-request id. "Run `civitai app submit` first" is confidently
		// wrong for that user: they have submitted, they just named the wrong
		// request. Key the advice on the selector the request actually used, the
		// same way submissionSubject does for the empty-list arm
		// (civitai/cli#363).
		return fmt.Errorf("no such submission (404): %s — %s", msg, submissionNotFoundAdvice(id, blockID))
	case http.StatusTooManyRequests:
		return fmt.Errorf("rate limited (429): %s — wait a moment and retry", msg)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("apps is not enabled (503): %s", msg)
	default:
		return fmt.Errorf("server returned %d: %s", status, msg)
	}
}

// serverMessage extracts the {"message"|"error": ...} field, falling back to the
// trimmed raw body.
func serverMessage(raw []byte) string {
	msg := strings.TrimSpace(string(raw))
	var wrapped struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(raw, &wrapped) == nil {
		if wrapped.Message != "" {
			return wrapped.Message
		}
		if wrapped.Error != "" {
			return wrapped.Error
		}
	}
	return msg
}

// serverError turns a non-2xx response into a clear, actionable error.
func serverError(status int, raw []byte) (err error) {
	defer func() { err = civitai.TagStatus(status, err) }()
	msg := serverMessage(raw)
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("unauthorized (401): %s — check your token with `civitai login`", msg)
	case http.StatusForbidden:
		return fmt.Errorf("forbidden (403): %s — your account may lack Apps access", msg)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("service unavailable (503): %s — Apps may not be enabled", msg)
	default:
		return fmt.Errorf("server returned %d: %s", status, msg)
	}
}
