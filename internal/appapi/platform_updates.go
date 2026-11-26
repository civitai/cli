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

// Staff platform updates — the OWNER half (civitai/civitai#10594, "Owner-facing
// API"). Civitai staff can commit an update to someone else's app, as one commit
// on its own branch of the app's SOURCE repository; the owner's CLI lists the
// pending ones, merges each recorded commit, and reports the outcome.
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

// ListPendingPlatformUpdates returns the app's PENDING platform updates, oldest
// first (the order to merge them in). `slug` is the app's slug.
//
// 🔴 A 404 IS "NO UPDATES", AND IT IS RETURNED AS AN EMPTY LIST WITH A NIL
// ERROR. tRPC answers 404 both for a procedure the server does not have (an
// older server) and for an app the caller cannot reach ("App not found" — a
// first submit of a brand-new slug lands there). The contract says to treat the
// first as no updates; the second has nothing to merge either. A flag that is
// off answers `{updates: []}` in a 200, which needs no special case.
func (c *Client) ListPendingPlatformUpdates(ctx context.Context, slug string) ([]PlatformUpdate, error) {
	var out struct {
		Updates []PlatformUpdate `json:"updates"`
	}
	status, err := c.platformUpdatesQuery(ctx, PlatformUpdatesListPendingPath, map[string]string{"slug": slug}, &out)
	if status == http.StatusNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return out.Updates, nil
}

// VerifyPlatformUpdateCommit asks whether commitSha is a recorded platform
// update of the app — the ONLY source of the "verified Civitai staff" badge.
// A 404 (older server) is `false`, never an error.
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
