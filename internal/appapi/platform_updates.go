package appapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/civitai/cli/pkg/civitai"
)

// Staff platform updates — the OWNER half (civitai/civitai#10594, "Owner-facing
// API"). Civitai staff can commit an update to someone else's app, as one commit
// on its own branch of the app's SOURCE repository; the owner's CLI lists the
// OPEN ones (pending, and conflicted ones not yet merged by hand), merges each
// recorded commit, and reports the outcome.
//
// 🔴 THIS FILE IS THE ONE PLACE THE WIRE CONTRACT LIVES: procedure names, field
// names, the outcome enum and the status mapping. It was written against the
// contract in that PR's body before the server merged, so a late contract change
// should be a change to this file only.
//
// tRPC, non-batched: a GET with ?input={"json":{…}} for the two queries, a POST
// with a {"json":{…}} body for resolve, success unwrapped from result.data.json.
const (
	PlatformUpdatesListPendingPath  = "/api/trpc/appPlatformUpdates.listPending"
	PlatformUpdatesVerifyCommitPath = "/api/trpc/appPlatformUpdates.verifyCommit"
	PlatformUpdatesResolvePath      = "/api/trpc/appPlatformUpdates.resolve"
)

// The two outcomes an owner's client reports.
const (
	PlatformUpdateMerged     = "merged"
	PlatformUpdateConflicted = "conflicted"
)

// The two statuses an OPEN update — one listPending returns — can carry.
const (
	PlatformUpdateStatusPending    = "pending"
	PlatformUpdateStatusConflicted = "conflicted"
)

// ErrPlatformUpdatesAppNotFound is a NOT_FOUND from a server that HAS the
// procedure: it does not know the app, or the caller may not edit it ("App not
// found" — the server answers both the same). It is NOT "no updates": for an app
// the caller believes it owns, the caller should say so.
var ErrPlatformUpdatesAppNotFound = errors.New("platform updates: the server does not know this app for this account")

// errPlatformUpdatesNoProcedure is a 404 from a server WITHOUT the procedure (an
// older server, or a route in front of it that does not know the path).
var errPlatformUpdatesNoProcedure = errors.New("platform updates: the server has no such procedure")

// trpcNoProcedureRe is tRPC's own message for a path its router does not have:
// v11 `No procedure found on path "<p>"`, v10 `No "query"-procedure on path "<p>"`.
var trpcNoProcedureRe = regexp.MustCompile(`^No (?:procedure found|"[a-z]+"-procedure) on path `)

// platformUpdates404 classifies a 404 body.
//
// 🔴 BOTH 404s CARRY A tRPC ERROR ENVELOPE WITH code NOT_FOUND. An older server
// is a tRPC server too: a procedure its router lacks is answered by tRPC itself,
// `{error:{json:{message:"No procedure found on path \"…\"", data:{code:"NOT_FOUND"}}}}`,
// exactly the shape of the server's own "App not found". So the code cannot
// tell them apart; tRPC's fixed message can. "No such procedure" is therefore
// recognised POSITIVELY — that message, or a 404 with no tRPC envelope at all
// (something in front of the server answered) — and every other 404 is
// ErrPlatformUpdatesAppNotFound. The failure direction is deliberate: if tRPC
// ever rewords its message, an older server produces a warning, never a silent
// "App not found".
func platformUpdates404(raw []byte) error {
	// The superjson-wrapped envelope ({error:{json:{…}}}) the server writes, or a
	// bare one ({error:{message}}) from a tRPC without a transformer.
	type shape struct {
		Message *string `json:"message"`
		Code    any     `json:"code"`
	}
	var env struct {
		Error *struct {
			shape
			JSON *shape `json:"json"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &env) != nil || env.Error == nil {
		return errPlatformUpdatesNoProcedure
	}
	inner := env.Error.JSON
	if inner == nil {
		inner = &env.Error.shape
	}
	if inner.Message == nil && inner.Code == nil {
		return errPlatformUpdatesNoProcedure
	}
	msg := ""
	if inner.Message != nil {
		msg = strings.TrimSpace(*inner.Message)
	}
	if trpcNoProcedureRe.MatchString(msg) {
		return errPlatformUpdatesNoProcedure
	}
	if msg == "" {
		return ErrPlatformUpdatesAppNotFound
	}
	return fmt.Errorf("%w (%s)", ErrPlatformUpdatesAppNotFound, msg)
}

// PlatformUpdatePath is one entry of PlatformUpdate.Paths: a file the update's
// commit creates, updates or deletes.
type PlatformUpdatePath struct {
	Path      string `json:"path"`
	Operation string `json:"operation"`
}

// PlatformUpdateBy names who pushed the update. Username is null on the wire for
// an account that no longer has one. VerifiedStaff is the server's read-time
// answer; the badge is NOT drawn from it — only from verifyCommit's `verified`.
type PlatformUpdateBy struct {
	Username      *string `json:"username"`
	VerifiedStaff bool    `json:"verifiedStaff"`
}

// PlatformUpdate mirrors the contract's PlatformUpdate.
//
// 🔴 CommitSha IS THE UPDATE. A client merges that sha and nothing else — never
// the branch, FETCH_HEAD or `<remote>/<branch>`: the branch is ordinary git and
// can move after the push; only the recorded sha is what the record attests.
type PlatformUpdate struct {
	ID              string               `json:"id"`
	Branch          string               `json:"branch"`
	CommitSha       string               `json:"commitSha"`
	BaseCommitSha   string               `json:"baseCommitSha"`
	Reason          string               `json:"reason"`
	Paths           []PlatformUpdatePath `json:"paths"`
	TouchesManifest bool                 `json:"touchesManifest"`
	Status          string               `json:"status"`
	By              PlatformUpdateBy     `json:"by"`
	CreatedAt       string               `json:"createdAt"`
}

// ListPendingPlatformUpdates returns the app's OPEN platform updates — status
// `pending` AND `conflicted` (one a client could not merge, which stays listed
// until it is merged or withdrawn) — oldest first, the order to merge them in.
// `slug` is the app's slug. The procedure keeps its older name, listPending.
//
// Two 404s, two answers (platformUpdates404):
//   - a server WITHOUT the procedure (older): no updates — empty list, nil error;
//   - a server that answers NOT_FOUND itself ("App not found"): an error wrapping
//     ErrPlatformUpdatesAppNotFound, so the caller decides whether that is
//     expected (a first submit of a brand-new slug) or worth a warning (a pull of
//     an app the CLI just resolved).
//
// A flag that is off answers `{updates: []}` in a 200, which needs no special case.
func (c *Client) ListPendingPlatformUpdates(ctx context.Context, slug string) ([]PlatformUpdate, error) {
	var out struct {
		Updates []PlatformUpdate `json:"updates"`
	}
	_, err := c.platformUpdatesQuery(ctx, PlatformUpdatesListPendingPath, map[string]string{"slug": slug}, &out)
	if errors.Is(err, errPlatformUpdatesNoProcedure) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return out.Updates, nil
}

// VerifyPlatformUpdateCommit asks whether commitSha is a recorded platform
// update of the app — the ONLY source of the "verified Civitai staff" badge.
// Any 404 (an older server, or an app the server does not know) is `false`,
// never an error: the badge is only drawn on a positive answer.
func (c *Client) VerifyPlatformUpdateCommit(ctx context.Context, slug, commitSha string) (bool, error) {
	var out struct {
		Verified bool `json:"verified"`
	}
	status, err := c.platformUpdatesQuery(ctx, PlatformUpdatesVerifyCommitPath,
		map[string]string{"slug": slug, "commitSha": commitSha}, &out)
	if status == http.StatusNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return out.Verified, nil
}

// ResolvePlatformUpdate reports the owner-side outcome of one update.
// mergeCommitSha is sent only for PlatformUpdateMerged (the server requires it
// there and has no use for it otherwise).
func (c *Client) ResolvePlatformUpdate(ctx context.Context, id, outcome, mergeCommitSha string) error {
	in := map[string]string{"id": id, "outcome": outcome}
	if outcome == PlatformUpdateMerged {
		in["mergeCommitSha"] = mergeCommitSha
	}
	body, err := json.Marshal(map[string]any{"json": in})
	if err != nil {
		return err
	}
	build := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+PlatformUpdatesResolvePath, bytes.NewReader(body))
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
		return platformUpdatesError(PlatformUpdatesResolvePath, status, raw)
	}
	return nil
}

// platformUpdatesQuery issues one of the two GET queries. It returns the HTTP
// status alongside the error so the callers can branch on a 404.
func (c *Client) platformUpdatesQuery(ctx context.Context, path string, input any, out any) (int, error) {
	inputJSON, err := json.Marshal(map[string]any{"json": input})
	if err != nil {
		return 0, err
	}
	q := url.Values{}
	q.Set("input", string(inputJSON))
	reqURL := c.BaseURL + path + "?" + q.Encode()
	build := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return status, err
	}
	if status == http.StatusNotFound {
		return status, platformUpdates404(raw)
	}
	if status != http.StatusOK {
		return status, platformUpdatesError(path, status, raw)
	}
	return status, decodeFeedbackData(raw, out, trpcName(path))
}

// platformUpdatesError maps a non-200 to an error carrying the server's message.
// The body is a tRPC error envelope ({error:{json:{message}}}); it carries no
// credential, but its message is server text, so the command layer sanitises it
// before printing.
func platformUpdatesError(path string, status int, raw []byte) error {
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
	if status == http.StatusUnauthorized {
		return civitai.TagStatus(status, unauthorizedError(msg))
	}
	return civitai.TagStatus(status, fmt.Errorf("%s failed (HTTP %d): %s", trpcName(path), status, msg))
}

// PlatformUpdateMissingCode is the submit route's error code for a bundle that
// lacks a staff platform update not yet shipped in an approved version, sent
// without `allowUnmergedPlatformUpdate` (the server half of the submit check).
const PlatformUpdateMissingCode = "platform_update_missing"

// ErrPlatformUpdateMissing classifies the server's platform_update_missing
// refusal of a submit.
var ErrPlatformUpdateMissing = errors.New("the server refused the submit: the bundle is missing a staff platform update")

// MissingPlatformUpdate is one entry of the refusal's `details.updates`. Every
// field is optional and read leniently (decodeMissingPlatformUpdate): the
// client prints what arrived and nothing else.
type MissingPlatformUpdate struct {
	ID     string
	Reason string
	By     string   // the staff username, "" when absent
	Paths  []string // changed paths, "" entries dropped
}

// decodeMissingPlatformUpdate reads one entry. `by` may be {username} or a bare
// string; `paths` may be strings or {path, operation} objects. A field in any
// other shape is left empty rather than dropping the entry.
func decodeMissingPlatformUpdate(raw json.RawMessage) (MissingPlatformUpdate, bool) {
	var in struct {
		ID     string          `json:"id"`
		Reason string          `json:"reason"`
		By     json.RawMessage `json:"by"`
		Paths  json.RawMessage `json:"paths"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return MissingPlatformUpdate{}, false
	}
	u := MissingPlatformUpdate{ID: in.ID, Reason: in.Reason}
	var byObj struct {
		Username *string `json:"username"`
	}
	var byStr string
	if json.Unmarshal(in.By, &byObj) == nil && byObj.Username != nil {
		u.By = *byObj.Username
	} else if json.Unmarshal(in.By, &byStr) == nil {
		u.By = byStr
	}
	var strs []string
	var objs []PlatformUpdatePath
	if json.Unmarshal(in.Paths, &strs) == nil {
		for _, p := range strs {
			if p != "" {
				u.Paths = append(u.Paths, p)
			}
		}
	} else if json.Unmarshal(in.Paths, &objs) == nil {
		for _, p := range objs {
			if p.Path != "" {
				u.Paths = append(u.Paths, p.Path)
			}
		}
	}
	return u, true
}

// PlatformUpdateMissingError is the server's platform_update_missing refusal.
// Error() is the same `server returned <status>: <message>` serverError writes,
// so the text and the status tag are what any other refusal would carry;
// Updates is what the server listed, nil when it listed nothing this client can
// read.
type PlatformUpdateMissingError struct {
	Status  int
	Message string
	Updates []MissingPlatformUpdate
}

func (e *PlatformUpdateMissingError) Error() string {
	return fmt.Sprintf("server returned %d: %s", e.Status, e.Message)
}

// Is makes errors.Is(err, ErrPlatformUpdateMissing) true.
func (e *PlatformUpdateMissingError) Is(target error) bool { return target == ErrPlatformUpdateMissing }

// submitRefusal is serverError for the submit route, plus the one refusal the
// CLI explains rather than relays: platform_update_missing. The code is read at
// the top level of the body (`{code, message, details}`).
//
// 🔴 TOLERANT OF THE SHAPE. Only the code decides; `details.updates` is decoded
// on its own, and a details this client cannot read (absent, another shape, a
// future one) leaves Updates nil — the refusal is still recognised and the
// server's message still printed. Any other code, or none, is serverError's.
func submitRefusal(status int, raw []byte) error {
	var head struct {
		Code    string          `json:"code"`
		Details json.RawMessage `json:"details"`
	}
	if json.Unmarshal(raw, &head) != nil || head.Code != PlatformUpdateMissingCode {
		return serverError(status, raw)
	}
	e := &PlatformUpdateMissingError{Status: status, Message: serverMessage(raw)}
	var details struct {
		Updates []json.RawMessage `json:"updates"`
	}
	if len(head.Details) > 0 && json.Unmarshal(head.Details, &details) == nil {
		for _, r := range details.Updates {
			if u, ok := decodeMissingPlatformUpdate(r); ok {
				e.Updates = append(e.Updates, u)
			}
		}
	}
	return civitai.TagStatus(status, e)
}
