package appapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/civitai/cli/pkg/civitai"
)

// The owner-side app-feedback procedures (civitai/civitai
// src/server/routers/app-feedback.router.ts). Like blocks.getMyAppAnalytics
// (AGENTS.md item 5) they exist only as tRPC — there is no /api/v1 route — so
// queries are non-batched GETs with the input in ?input={"json":{…}} and
// mutations are POSTs with a {"json":{…}} body; success unwraps
// result.data.json.
//
// Every one of them takes an `appListingId`, never a slug. The command layer
// resolves the slug through the listing resolution `civitai app listing`
// already uses; nothing here re-derives it.
const (
	AppFeedbackListPath      = "/api/trpc/appFeedback.listForListing"
	AppFeedbackCountNewPath  = "/api/trpc/appFeedback.countNewForMyListings"
	AppFeedbackSetStatusPath = "/api/trpc/appFeedback.setOwnerStatus"
	AppFeedbackFlagPath      = "/api/trpc/appFeedback.flagAbusive"
)

// FeedbackPageMax is the server's ceiling on one listForListing page
// (`limit: z.number().int().min(1).max(100)` in
// src/server/schema/app-feedback.schema.ts). Its default, when `limit` is
// absent, is 50.
const FeedbackPageMax = 100

// The owner statuses the server stores. "new" is NOT one of them: a row nobody
// has triaged has a NULL ownerStatus, and "new" is only the list FILTER's
// spelling of that NULL. That is why FeedbackStatusNew is accepted by
// ListAppFeedback and refused by SetAppFeedbackStatus's schema server-side —
// there is no way to move a row back to new.
const (
	FeedbackStatusNew          = "new"
	FeedbackStatusAcknowledged = "acknowledged"
	FeedbackStatusResolved     = "resolved"
	FeedbackStatusWontFix      = "wont_fix"
)

// FeedbackReporter is who wrote a feedback row. The server sends it on every
// row; whether it is SHOWN is the command layer's decision (hidden unless
// --with-reporter).
type FeedbackReporter struct {
	ID int64 `json:"id"`
	// Username is a FlexString for the reason AGENTS.md item 37 records: an
	// all-digit username can arrive as a bare JSON number. Null decodes to "".
	Username civitai.FlexString `json:"username"`
}

// Feedback is one row of appFeedback.listForListing — the server's
// OwnerFeedbackDto (src/server/services/blocks/app-feedback.service.ts).
//
// 🔴 Message IS UNTRUSTED THIRD-PARTY TEXT. It is written by an arbitrary
// signed-in site user, not by the app's owner and not by the platform, and its
// reader is frequently the owner's coding agent. Nothing in this package
// interprets it; the command layer sanitises it for a terminal and labels it as
// untrusted in --json. Do not add a consumer that treats it as anything but
// data.
//
// The nullable fields are pointers because null is a real answer the server
// gives, and it is not the empty string: AppBlockVersion/AppBlockSha are null
// for feedback about an OFFSITE app (no block), Surface is null when the stored
// context is not one of the server's closed set, and OwnerStatus is null for a
// row that is still new.
type Feedback struct {
	ID              int64            `json:"id"`
	Message         string           `json:"message"`
	CreatedAt       string           `json:"createdAt"`
	Reporter        FeedbackReporter `json:"reporter"`
	AppBlockVersion *string          `json:"appBlockVersion"`
	AppBlockSha     *string          `json:"appBlockSha"`
	Surface         *string          `json:"surface"`
	OwnerStatus     *string          `json:"ownerStatus"`
	OwnerStatusAt   *string          `json:"ownerStatusAt"`
	OwnerFlaggedAt  *string          `json:"ownerFlaggedAt"`
}

// Status is the row's owner status in the FILTER's vocabulary: a NULL
// ownerStatus reads "new", so one word names the same state on the way in
// (--status new) and on the way out.
func (f Feedback) Status() string {
	if f.OwnerStatus == nil || *f.OwnerStatus == "" {
		return FeedbackStatusNew
	}
	return *f.OwnerStatus
}

// FeedbackPage is one page of the owner inbox, newest first.
type FeedbackPage struct {
	Items []Feedback `json:"items"`
	// NextCursor is the id of the LAST row of this page when the server holds
	// more, and absent/null when it does not (keyset pagination over
	// `ORDER BY createdAt DESC, id DESC`). A pointer because 0 is not "no more":
	// only absence is.
	NextCursor *int64 `json:"nextCursor"`
}

// FeedbackListInput is the listForListing input.
//
// 🔴 EVERY OPTIONAL FIELD IS ABSENT WHEN UNSET, NEVER A ZERO VALUE (AGENTS.md
// item 14). `ownerStatus: ""` is not a member of the server's enum and is
// rejected; `cursor: 0` fails `.positive()`; `limit: 0` fails `.min(1)`. The
// "all statuses" request is spelled by OMITTING ownerStatus — there is no "all"
// member server-side — so the empty string here means exactly that and must
// never reach the wire as a key.
type FeedbackListInput struct {
	AppListingID string `json:"appListingId"`
	Cursor       int64  `json:"cursor,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	OwnerStatus  string `json:"ownerStatus,omitempty"`
}

// feedbackSetStatusInput is the setOwnerStatus input.
//
// 🔴 ExpectedOwnerStatus HAS NO omitempty, AND THAT IS THE POINT. The schema is
// `feedbackOwnerStatusSchema.nullable()` — required, with JSON null meaning
// "I last saw this row as new". It is the optimistic-concurrency token: the
// server's UPDATE is scoped `ownerStatus: expectedOwnerStatus`, so a row that
// moved since the caller read it matches nothing and the write is refused with
// CONFLICT. Dropping the key would be a schema rejection; sending a guess would
// defeat the check. A nil pointer marshals to null, which is the "new" case.
type feedbackSetStatusInput struct {
	ID                  int64   `json:"id"`
	AppListingID        string  `json:"appListingId"`
	OwnerStatus         string  `json:"ownerStatus"`
	ExpectedOwnerStatus *string `json:"expectedOwnerStatus"`
}

type feedbackFlagInput struct {
	ID           int64  `json:"id"`
	AppListingID string `json:"appListingId"`
}

// feedbackOp is what the caller asked for. Like listingOp it exists so that one
// status mapper serving several procedures only claims what is true of the one
// that was called — a 409 means something different for a status write than for
// a flag, and a read cannot have changed anything.
type feedbackOp int

const (
	feedbackOpRead feedbackOp = iota + 1
	feedbackOpSetStatus
	feedbackOpFlag
)

// FeedbackConflictError is the server's CONFLICT (HTTP 409) on a feedback
// write. Typed so the command layer can errors.As it and say which app and
// which command to re-run — this package knows the listing id, not the slug.
//
// 🔴 IT DOES NOT MEAN ONLY "THE STATUS MOVED". Both writes are an `updateMany`
// whose WHERE is the owner-visibility predicate plus the precondition, and the
// server throws the SAME error whenever that matches zero rows
// (app-feedback.service.ts, `if (count === 0) throw … CONFLICT`). So it is also
// what an id that is not this app's, a row a moderator has since hidden, and a
// reporter who has since been banned look like — and for a flag, a row that is
// already flagged. The wording built from this must not narrow it to one cause.
type FeedbackConflictError struct {
	// ServerMsg is the server's own text.
	ServerMsg string
	// Flag is true when the refused write was flagAbusive rather than
	// setOwnerStatus.
	Flag bool
}

func (e *FeedbackConflictError) Error() string {
	if e.Flag {
		return fmt.Sprintf("the feedback was not flagged (409): %s", e.ServerMsg)
	}
	return fmt.Sprintf("the feedback status was not changed (409): %s", e.ServerMsg)
}

// ListAppFeedback reads one page of an app's owner inbox.
func (c *Client) ListAppFeedback(ctx context.Context, in FeedbackListInput) (*FeedbackPage, error) {
	var out FeedbackPage
	if err := c.feedbackQuery(ctx, AppFeedbackListPath, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CountNewAppFeedback returns `{appListingId: count}` of rows still new to the
// developer, across every listing the caller owns or holds an accepted editor
// seat on.
//
// 🔴 A LISTING WITH NO NEW FEEDBACK IS ABSENT FROM THE MAP, NOT PRESENT WITH 0.
// The server builds it from a `GROUP BY`, and returns `{}` outright for a
// caller with no listings. So "absent" is a measured zero for a listing the
// caller can read, and callers must look the listing up rather than range the
// map to learn which apps exist.
func (c *Client) CountNewAppFeedback(ctx context.Context) (map[string]int64, error) {
	out := map[string]int64{}
	if err := c.feedbackQuery(ctx, AppFeedbackCountNewPath, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SetAppFeedbackStatus moves one feedback row to status, on the condition that
// its status is still expected (nil = still new). See feedbackSetStatusInput.
//
// 🔴 `resolved` AND `wont_fix` NOTIFY THE USER WHO WROTE THE FEEDBACK
// (`isReporterNotifiedOwnerStatus` in
// src/server/notifications/app-feedback.notifications.ts); `acknowledged` does
// not. The notification is sent best-effort after the UPDATE commits, so a
// success here says the status changed, not that anything was delivered.
func (c *Client) SetAppFeedbackStatus(ctx context.Context, appListingID string, id int64, status string, expected *string) error {
	return c.feedbackMutation(ctx, AppFeedbackSetStatusPath, feedbackSetStatusInput{
		ID: id, AppListingID: appListingID, OwnerStatus: status, ExpectedOwnerStatus: expected,
	}, feedbackOpSetStatus)
}

// FlagAppFeedback marks one feedback row as abusive for moderator review.
func (c *Client) FlagAppFeedback(ctx context.Context, appListingID string, id int64) error {
	return c.feedbackMutation(ctx, AppFeedbackFlagPath, feedbackFlagInput{ID: id, AppListingID: appListingID}, feedbackOpFlag)
}

// feedbackQuery issues a non-batched tRPC GET. A nil input sends NO ?input=
// parameter, for the reason trpcQuery (listing.go) gives: countNewForMyListings
// declares no `.input()`, and an explicit null is a value the server never has
// to accept.
func (c *Client) feedbackQuery(ctx context.Context, path string, input any, out any) error {
	reqURL := c.BaseURL + path
	if input != nil {
		inputJSON, err := json.Marshal(map[string]any{"json": input})
		if err != nil {
			return err
		}
		q := url.Values{}
		q.Set("input", string(inputJSON))
		reqURL += "?" + q.Encode()
	}
	build := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return feedbackError(status, raw, feedbackOpRead)
	}
	return decodeFeedbackData(raw, out, trpcName(path))
}

// feedbackMutation issues a tRPC POST. The result body is not decoded: both
// writes echo back the id (and status) the caller sent, which carries nothing a
// 200 has not already said.
func (c *Client) feedbackMutation(ctx context.Context, path string, input any, op feedbackOp) error {
	body, err := json.Marshal(map[string]any{"json": input})
	if err != nil {
		return err
	}
	build := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
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
		return feedbackError(status, raw, op)
	}
	return nil
}

// decodeFeedbackData pulls result.data.json out of a tRPC success envelope.
//
// 🔴 A `null` PAYLOAD IS MALFORMED, NOT EMPTY — the same rule GetMyAppAnalytics
// applies. `null` unmarshals cleanly into a zero FeedbackPage, which would
// render as "no feedback": a fabricated empty inbox out of a response that said
// nothing. A real empty inbox is `{"items":[]}` and a real empty count is `{}`.
//
// 🔴 THE BODY IS NOT ECHOED INTO THE ERROR, unlike the sibling decoders. This
// response is the one that carries third-party message text, and an error
// string goes to the terminal through no sanitiser this package owns. The byte
// count is what a bug report needs; the bytes are what an attacker wrote.
func decodeFeedbackData(raw []byte, out any, proc string) error {
	var env struct {
		Result struct {
			Data struct {
				JSON json.RawMessage `json:"json"`
			} `json:"data"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil ||
		len(env.Result.Data.JSON) == 0 ||
		string(bytes.TrimSpace(env.Result.Data.JSON)) == "null" {
		return fmt.Errorf("unexpected %s response: %d bytes that are not a tRPC result envelope", proc, len(raw))
	}
	if err := json.Unmarshal(env.Result.Data.JSON, out); err != nil {
		return fmt.Errorf("unexpected %s payload: %d bytes that do not decode as the expected shape", proc, len(raw))
	}
	return nil
}

// feedbackError maps a non-200 from an app-feedback procedure to an actionable
// error. tRPC error bodies are {error:{json:{message,code,…}}}.
//
// The arms, and what the server lets a caller observe on each (read at
// civitai/civitai origin/main, src/server/services/blocks/app-feedback.service.ts
// and src/server/services/oauth/enforce-token-scope.ts):
//
//   - 403 with the token-scope message — the credential's SCOPE does not cover
//     the procedure. A procedure with no `requiredScope` annotation needs a
//     full-scope credential, so a scoped `civitai login` token is refused here
//     until the server annotates these procedures with the Apps submit scope.
//   - any other 403 — `resolveInboxListingId`: the caller is neither owner nor
//     accepted editor of the listing. 🔴 The server returns the SAME error for a
//     listing that does not exist, on purpose ("not an existence oracle"), so
//     this arm must never claim to know which.
//   - 409 — a write whose precondition matched no row. See
//     FeedbackConflictError.
//   - 404 — tRPC's own answer for a procedure the server does not have, i.e. a
//     deployment that predates app feedback. None of the four procedures throws
//     NOT_FOUND itself.
//
// 🔴 THERE IS NO "FEEDBACK IS DISABLED" ARM, BECAUSE THE OWNER SIDE CANNOT SEE
// THAT STATE. The feature flag gates only who may WRITE feedback
// (`resolveAppFeedbackTarget` → `isFeedbackAreaEnabled`); none of the owner
// procedures consults it. An owner whose users are not offered the form simply
// reads an empty inbox, indistinguishable on the wire from an app nobody has
// written to. The command layer says exactly that rather than naming a cause.
func feedbackError(status int, raw []byte, op feedbackOp) (err error) {
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
	msg = strings.TrimSpace(msg)
	switch status {
	case http.StatusUnauthorized:
		return unauthorizedError(msg)
	case http.StatusForbidden:
		if isInsufficientScopeMsg(msg) {
			return fmt.Errorf("this credential's scope does not cover app feedback (403): %s — "+
				"a full-scope personal API key works: create one at https://civitai.com/user/account and run "+
				"`civitai login --token <key>`. A `civitai login` token works only on a server that accepts the Apps submit "+
				"scope for app feedback; re-running `civitai login` helps only if your token predates that scope "+
				"(`civitai whoami` shows which credential is active)", msg)
		}
		return fmt.Errorf("no access to this app's feedback (403): %s — only the app's owner and its accepted "+
			"collaborators can read or change it, and the server answers the same way for an app that is not yours and "+
			"for one that does not exist. `civitai app doctor` lists the apps this account can work on; "+
			"`civitai whoami` shows which account that is", msg)
	case http.StatusConflict:
		return &FeedbackConflictError{ServerMsg: msg, Flag: op == feedbackOpFlag}
	case http.StatusNotFound:
		return fmt.Errorf("this server does not offer app feedback (404): %s — the feedback API is missing from it, "+
			"which is what an older or self-hosted Civitai answers (CIVITAI_BASE_URL selects the server)", msg)
	case http.StatusBadRequest:
		return fmt.Errorf("the server rejected this app-feedback request (400): %s — "+
			"see `civitai app feedback --help` for the accepted values", msg)
	case http.StatusTooManyRequests:
		return fmt.Errorf("rate limited, try again shortly (429): %s", msg)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("app feedback unavailable (503): %s", msg)
	default:
		return fmt.Errorf("server returned %d: %s", status, msg)
	}
}
