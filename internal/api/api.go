// Package api is the (thin) HTTP client for the Civitai Apps surface.
//
// Auth/submit contract (civitai/civitai PR #2644):
//
//   - The token-authenticated bundle-upload route is
//     POST /api/v1/blocks/submit-version (Bearer access token). This is the
//     default the CLI targets; CIVITAI_SUBMIT_PATH overrides it.
//   - The legacy /api/blocks/submit-version route is session-cookie + moderator
//     authenticated and not used by the CLI.
//
// Auth is supplied via a TokenSource so OAuth (device-flow) access tokens are
// refreshed transparently before a request and once on a 401; a personal API
// key is a static source with no refresh. Keeping the network call behind the
// Submitter interface makes the whole thing testable without a live server.
package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// TokenSource yields the Bearer token to send. Refresh, if supported, refreshes
// the token (e.g. after a 401) and returns the new one; a static personal-key
// source returns ErrNoRefresh.
type TokenSource interface {
	// Token returns the current bearer token, refreshing first if it is known
	// to be expired. An empty token means "unauthenticated".
	Token(ctx context.Context) (string, error)
	// Refresh forces a refresh (used on a 401) and returns the new token.
	// Sources that cannot refresh return ("", ErrNoRefresh).
	Refresh(ctx context.Context) (string, error)
}

// ErrNoRefresh is returned by a non-refreshable (personal-key) TokenSource.
var ErrNoRefresh = fmt.Errorf("token source cannot refresh")

// StaticToken is a TokenSource for a fixed token (personal API key). It never
// refreshes.
type StaticToken string

// Token returns the static token.
func (s StaticToken) Token(context.Context) (string, error) { return string(s), nil }

// Refresh always fails: a personal key has no refresh path.
func (s StaticToken) Refresh(context.Context) (string, error) { return "", ErrNoRefresh }

// Submitter submits a packaged bundle and returns the server's response. The
// slug + version identify the submission so that, if the upload's response is
// lost to a timeout, the submit path can poll for a landed submission and
// recover rather than reporting a false failure (see SubmitVersion).
type Submitter interface {
	SubmitVersion(ctx context.Context, zipBytes []byte, slug, version string) (*SubmitResult, error)
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
}

// Subject identifies the credential behind a token as returned by
// GET /api/v1/me. Type == "oauth" means an OAuth device-login token (from
// `civitai login`); any other type (e.g. "apiKey"/"user") is a personal API
// key. Absent when auth is cookie/session (not applicable to the CLI).
type Subject struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// Identity is the authenticated-user view `whoami` reports. TokenScope,
// BuzzLimit, and Subject are pointers because GET /api/v1/me omits them for some
// auth kinds (e.g. cookie/session), and a nil TokenScope must degrade to
// "scopes unknown" rather than decode as "no capabilities".
type Identity struct {
	Username string `json:"username"`
	ID       int    `json:"id"`
	// TokenScope is the bearer token's scope bitmask. Decode it with the Scope*
	// bits below to learn what the credential can do (spend Buzz, read balance,
	// …). A personal full-scope key has every bit; an OAuth device-login token
	// typically has neither AIServicesWrite nor BuzzRead. nil ⇒ unknown (absent
	// from the response, e.g. cookie auth).
	TokenScope *int `json:"tokenScope,omitempty"`
	// BuzzLimit is the credential's per-window spend cap, when the server reports
	// one. nil ⇒ absent/unknown.
	BuzzLimit *int64 `json:"buzzLimit,omitempty"`
	// Subject identifies the credential (OAuth login vs personal API key). nil ⇒
	// cookie/session auth (not applicable to the CLI).
	Subject *Subject `json:"subject,omitempty"`
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

// DecodeScopes returns the names of every set scope bit (low → high). A nil
// (unknown) mask returns nil.
func (id *Identity) DecodeScopes() []string {
	if id.TokenScope == nil {
		return nil
	}
	var out []string
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

// defaultTimeout governs the fast, interactive calls (whoami, submissions).
// Kept short so a hung connection surfaces quickly.
const defaultTimeout = 30 * time.Second

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

// WithdrawPath is the token-authenticated, self-scoped withdraw route
// (POST {"publishRequestId": ...}; civitai/civitai
// src/pages/api/v1/blocks/withdraw.ts). 200 on success (incl. an idempotent
// already-withdrawn), 404 not-found-or-not-yours, 409 not in a withdrawable
// (pending) state.
const WithdrawPath = "/api/v1/blocks/withdraw"

// DevTokenPath is the invite-gated route that mints a short-lived dev block
// token for `npm run dev:live` (POST {"slug": ..., "scopes"?: [...]};
// civitai/civitai src/pages/api/v1/blocks/dev-token.ts). 200 { token, ... } on
// success; a PENDING (un-approved) slug is accepted, and a slug with NO app row
// yet mints from the request-body `scopes` (the dev's LOCAL manifest scopes,
// clamped server-side) — so `create → dev-token → dev:live` works with no
// submit step. Error bodies are {message}: 404 slug registered to a different
// account / genuinely not found, 403 not-invited/insufficient-scope, 429
// rate-limited, 503 flag-off. The minted token's CAPABILITIES depend on the
// bearer: a full-scope personal API key mints a spend-capable token; an OAuth
// (`civitai login`) credential mints a read-only one.
const DevTokenPath = "/api/v1/blocks/dev-token"

// Client is the default HTTP implementation.
type Client struct {
	BaseURL    string
	Tokens     TokenSource
	SubmitPath string // route for submit-version; CIVITAI_SUBMIT_PATH overrides
	HTTP       *http.Client
	// SubmitTimeout overrides the submit-upload timeout when non-zero; it
	// defaults to submitTimeout. Used by tests to exercise the timeout-recovery
	// path without a real slow upload.
	SubmitTimeout time.Duration
	// SubmitPollDelay overrides the inter-attempt delay of the post-timeout
	// recovery poll when set (>= 0 with the zero value meaning "use the
	// default"); tests set it to 0 to avoid sleeping.
	SubmitPollDelay *time.Duration
}

// New builds a Client with sane defaults from a static token (personal API key
// or a one-shot access token). For refreshable OAuth credentials use
// NewWithSource.
func New(baseURL, token, submitPath string) *Client {
	return NewWithSource(baseURL, StaticToken(token), submitPath)
}

// NewWithSource builds a Client backed by a TokenSource (which may refresh).
func NewWithSource(baseURL string, src TokenSource, submitPath string) *Client {
	if submitPath == "" {
		submitPath = DefaultSubmitPath
	}
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Tokens:     src,
		SubmitPath: submitPath,
		HTTP:       &http.Client{Timeout: defaultTimeout},
	}
}

// authedDo runs build() (which builds a fresh *http.Request each call, since a
// retried request needs a fresh body) with a Bearer token from the source,
// refreshing once on a 401 and retrying. The returned response body is fully
// read into raw and the response is closed.
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
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, raw, nil
}

// submitBody mirrors submitVersionSchema: a base64-encoded ZIP.
type submitBody struct {
	BundleBase64 string `json:"bundleBase64"`
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
func (c *Client) SubmitVersion(ctx context.Context, zipBytes []byte, slug, version string) (*SubmitResult, error) {
	body, err := json.Marshal(submitBody{
		BundleBase64: base64.StdEncoding.EncodeToString(zipBytes),
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

// isTimeoutErr reports whether err is a request timeout / deadline-exceeded /
// no-response condition (as opposed to a clean HTTP error response). It matches
// context.DeadlineExceeded, os timeouts, and any net.Error whose Timeout() is
// true (which is what http.Client.Timeout surfaces when awaiting headers).
func isTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
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
func latestMatchingSubmission(subs []Submission, slug, version string) *Submission {
	var anyMatch *Submission
	for i := range subs {
		s := &subs[i]
		if s.BlockID != slug || s.Version != version {
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
		return nil, fmt.Errorf("no token configured — run `civitai login` first")
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
	if err := json.Unmarshal(raw, &id); err != nil {
		return nil, fmt.Errorf("unexpected /api/v1/me response: %s", string(raw))
	}
	return &id, nil
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
		return nil, fmt.Errorf("no token configured — run `civitai login` first")
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
		return nil, submissionsError(status, raw)
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
func (c *Client) GetSubmission(ctx context.Context, id, blockID string) (*Submission, error) {
	build := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, c.submissionsURL(id, blockID), nil)
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, submissionsError(status, raw)
	}
	// An `?id=` lookup returns {submission: {...}}; an `?blockId=` lookup returns
	// {submissions: [...]} (narrowed list) — handle both so either selector works.
	var single struct {
		Submission *Submission `json:"submission"`
	}
	if err := json.Unmarshal(raw, &single); err == nil && single.Submission != nil {
		return single.Submission, nil
	}
	var list struct {
		Submissions []Submission `json:"submissions"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("unexpected /api/v1/blocks/submissions response: %s", string(raw))
	}
	if len(list.Submissions) == 0 {
		return nil, fmt.Errorf("no such submission")
	}
	return &list.Submissions[0], nil
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
		return nil, fmt.Errorf("no token configured — run `civitai login` first")
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
func cloneInfoError(status int, raw []byte) error {
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

// DevTokenMinter mints a short-lived dev block token for `npm run dev:live`.
type DevTokenMinter interface {
	// MintDevToken mints a dev block token for the given app slug and returns
	// the JWT. scopes carries the caller's LOCAL block.manifest.json scopes for
	// the server's no-row mint path (clamped server-side); pass nil/empty when
	// no manifest is available. A non-2xx is mapped by devTokenError.
	MintDevToken(ctx context.Context, slug string, scopes []string) (string, error)
}

// devTokenBody is the POST /api/v1/blocks/dev-token request body. Scopes is the
// caller's LOCAL manifest scopes; it is omitted (not sent) when empty so a
// registered app's server-side scopes still govern.
type devTokenBody struct {
	Slug   string   `json:"slug"`
	Scopes []string `json:"scopes,omitempty"`
}

// MintDevToken mints a short-lived dev block token for the given app slug,
// returning the JWT from the response's .token field. scopes carries the
// caller's local manifest scopes for the server's no-row (no app registered
// yet) mint path; they are clamped server-side and omitted from the body when
// empty/nil (registered-app and read-only paths are unaffected). The OAuth
// access token is refreshed transparently on a 401. A non-2xx is mapped by
// devTokenError.
func (c *Client) MintDevToken(ctx context.Context, slug string, scopes []string) (string, error) {
	body, err := json.Marshal(devTokenBody{Slug: slug, Scopes: scopes})
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

// devTokenError maps a non-2xx dev-token response to a clear, actionable CLI
// error. The route returns {"message": ...} on every error status: 404
// not-found-or-not-yours, 403 not-invited-or-insufficient-scope, 429
// rate-limited, 503 flag-off. The 403 message is the key DX case — a spend
// token needs a full-scope personal API key (an OAuth login mints read-only).
func devTokenError(status int, raw []byte) error {
	msg := serverMessage(raw)
	switch status {
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
		return fmt.Errorf("not authorized (403): %s — minting needs an invite (invite-only beta) AND a full-scope personal API key; an OAuth `civitai login` token can't mint a spend token (check with `civitai whoami`)", msg)
	case http.StatusTooManyRequests:
		return fmt.Errorf("rate limited, try again shortly (429): %s", msg)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("Apps unavailable (503): %s", msg)
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

// devTunnelError maps a non-200 dev-tunnel tRPC response to an actionable CLI
// error. tRPC error bodies are {error:{json:{message,code,...}}}; the HTTP
// status carries the mapped code (403 flag-off/not-author, 404 not-your-app).
func devTunnelError(status int, raw []byte) error {
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
func withdrawError(status int, raw []byte) error {
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
		return fmt.Errorf("Apps unavailable (503): %s", msg)
	default:
		return fmt.Errorf("server returned %d: %s", status, msg)
	}
}

// submissionsError maps a non-2xx submissions response to a clear, actionable
// error, with Apps-specific guidance for 403/404/429/503.
func submissionsError(status int, raw []byte) error {
	msg := serverMessage(raw)
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("not logged in (401): %s — run `civitai login`", msg)
	case http.StatusForbidden:
		return fmt.Errorf("Apps access required — invite-only beta (403): %s", msg)
	case http.StatusNotFound:
		return fmt.Errorf("no such submission (404): %s", msg)
	case http.StatusTooManyRequests:
		return fmt.Errorf("rate limited (429): %s — wait a moment and retry", msg)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("Apps is not enabled (503): %s", msg)
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
func serverError(status int, raw []byte) error {
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
