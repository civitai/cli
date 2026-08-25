package appapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/civitai/cli/pkg/civitai"
)

// App Store Listing MEDIA client (civitai/cli #186). These are AUTHED tRPC calls
// against the `appListings.*` router — the CLI's SCOPED OAuth token reaches them
// because civitai/civitai #3472 annotated each with
// `requiredScope: AppBlocksSubmit` (a bit the `civitai login` token carries).
//
// Queries (GET) send `?input={"json":<input>}`; mutations (POST) send a JSON body
// `{"json":<input>}`. The success envelope is `{result:{data:{json:<result>}}}`.
// Mirrors the existing `blocks.*` plumbing in appblocks.go.

// ImageUploadPath mints a presigned PUT URL for a full-resolution asset
// (cover / screenshot). Bearer-authed; returns {id, uploadURL}. The `id` (a
// uuid) is the key persistAssetImage stores.
const ImageUploadPath = "/api/v1/image-upload"

// The listing-media routes. Each one carries its own listingOp — see the
// listingRoute doc for why the classification cannot live at the call site.
var (
	// Reads. Note getMyListingForEdit has a server-side shadow-revision side
	// effect (see GetMyListingForEdit); it is still a read from the caller's
	// point of view — the user asked to look, not to change.
	trpcGetMyListingForApp   = listingRoute{"/api/trpc/appListings.getMyListingForApp", listingOpRead}
	trpcGetMyListingForEdit  = listingRoute{"/api/trpc/appListings.getMyListingForEdit", listingOpRead}
	trpcGetAssetScanStatuses = listingRoute{"/api/trpc/appListings.getAssetScanStatuses", listingOpRead}
	// listMine is the read behind `civitai app doctor`: every listing the caller
	// owns OR holds an accepted editor seat on, each carrying `problems[]`.
	// Unlike getMyListingForEdit it has NO server-side side effect — it opens no
	// shadow revision — which is what makes `doctor` safe to run in a loop.
	trpcListMine = listingRoute{"/api/trpc/appListings.listMine", listingOpRead}

	// 🔴 INGEST, not change — and both are POSTs, which is exactly why the verb
	// is not the answer. Each creates an Image row and touches NO listing;
	// setIcon/setCover/addScreenshot is what attaches it. `imageUploadRoute` is
	// step 1 of the same user action `trpcPersistAssetImage` ends, so the two
	// must tell one story.
	imageUploadRoute           = listingRoute{ImageUploadPath, listingOpIngest}
	trpcIngestAssetFromDataURI = listingRoute{"/api/trpc/appListings.ingestAssetFromDataUri", listingOpIngest}
	trpcPersistAssetImage      = listingRoute{"/api/trpc/appListings.persistAssetImage", listingOpIngest}

	// Changes: these write the listing (or its shadow revision), so a rejection
	// may have PARTIALLY applied and must never be worded as "nothing changed".
	trpcSetIcon               = listingRoute{"/api/trpc/appListings.setIcon", listingOpChange}
	trpcSetCover              = listingRoute{"/api/trpc/appListings.setCover", listingOpChange}
	trpcAddScreenshot         = listingRoute{"/api/trpc/appListings.addScreenshot", listingOpChange}
	trpcRemoveScreenshot      = listingRoute{"/api/trpc/appListings.removeScreenshot", listingOpChange}
	trpcReorderScreenshots    = listingRoute{"/api/trpc/appListings.reorderScreenshots", listingOpChange}
	trpcBeginListingRevision  = listingRoute{"/api/trpc/appListings.beginListingRevision", listingOpChange}
	trpcSubmitListingRevision = listingRoute{"/api/trpc/appListings.submitListingRevision", listingOpChange}
)

// ListingRef is the result of getMyListingForApp — the entry read that resolves
// an app's backing AppListing + its lifecycle status.
//
// 🔴 IT DELIBERATELY DOES NOT DECODE `editTargetId`, and civitai/cli#430's own
// suggested fix was to add it — so this is the field a reader arrives here
// intending to write. The server does send it on an approved listing, naming the
// shadow that edits must target, but it has been OBSERVED EXACTLY ONCE (in that
// issue's manual tRPC read) and adding it would give this CLI a FOURTH way to
// name an edit target beside `AppListingID`, `ListingEditView.ShadowID` and
// `BeginListingRevision`'s return. The commands that need the shadow call
// `BeginListingRevision` — idempotent, already the attach path's mechanism, and
// unambiguous about what it returns. Decoding `editTargetId` is a reasonable
// LATER optimisation (one fewer round-trip); it is not a prerequisite for
// anything, and it should not arrive as a side effect of some other change.
//
// 🔴 RE-ASKED AND RE-REFUSED BY civitai/cli#447, the `app listing status --json`
// payload — the surface with the strongest claim on it, since the issue asked
// for the field BY NAME. That payload emits `parentId` and `shadowId`, which
// this CLI really reads, and says in its own doc comment and in the README that
// `editTargetId` is absent because reporting an id the server did not hand it
// on that call would be a guess. So the refusal now has a user-visible
// consequence: adding the decode means adding the field there too.
type ListingRef struct {
	AppListingID       string `json:"appListingId"`
	Status             string `json:"status"` // draft|pending|approved|rejected|removed
	ContentRating      string `json:"contentRating"`
	HasPendingRevision bool   `json:"hasPendingRevision"`
}

// ListingAsset mirrors ListingEditAsset ({imageId, url}); a nil/zero imageId +
// empty url means the slot is unset.
type ListingAsset struct {
	ImageID *int    `json:"imageId"`
	URL     *string `json:"url"`
}

// Present reports whether the asset slot is populated.
func (a ListingAsset) Present() bool { return a.ImageID != nil && *a.ImageID > 0 }

// ListingScreenshot mirrors ListingEditScreenshot.
type ListingScreenshot struct {
	ID      string  `json:"id"`
	ImageID *int    `json:"imageId"`
	URL     *string `json:"url"`
	Caption *string `json:"caption"`
	Order   int     `json:"order"`
}

// ListingEditView mirrors getMyListingForEdit's result (the subset the CLI reads
// to render `status` + the trailing floor line). For an APPROVED parent the
// server resolves the media from the in-flight shadow revision (an idempotent
// begin), so the assets reflect the pending revision, not the live listing.
//
// 🔴 THAT SHADOW IS SEEDED FROM THE LIVE LISTING, NOT EMPTY — measured
// 2026-08-12 against an approved listing whose shadow this very read had just
// opened: it reported the live icon and cover ids, not two empty slots. The CLI
// depends on it. `reportStagedBelowFloor` (#400) decides whether the publish
// floor is met by reading THIS view, so an empty-seeded shadow would make every
// live listing look below-floor and would route a genuine rejection down the
// "this is only the floor" path. The same read also reported
// `hasPendingRevision: false` while that shadow existed, so the flag means
// SUBMITTED, not "a shadow exists".
type ListingEditView struct {
	ParentID           string  `json:"parentId"`
	Slug               string  `json:"slug"`
	Status             string  `json:"status"`
	HasPendingRevision bool    `json:"hasPendingRevision"`
	ShadowID           *string `json:"shadowId"`
	Assets             struct {
		Icon        ListingAsset        `json:"icon"`
		Cover       ListingAsset        `json:"cover"`
		Screenshots []ListingScreenshot `json:"screenshots"`
	} `json:"assets"`
}

// AttachResult is the (loosely-parsed) union result of setIcon/setCover/
// addScreenshot.
//
// 🔴 `ScanPending` is LOAD-BEARING, not diagnostic. Since issue #270 the CLI
// attaches BEFORE polling the scan (the server validates geometry/aspect/MIME/
// bytes at attach), so this flag is what tells it whether a poll is still owed.
// The server sets `scanPending: true` only on the still-scanning branch of
// `loadValidatedImage` and OMITS the key once `ingestion == Scanned` — so absent
// and false mean the same thing here, "the server already saw a clean scan", and
// a plain bool is the honest shape.
//
// `Status == "pending"` is the legacy `allowPending: false` variant and means
// NOTHING was written; the live listing-media procs never return it.
type AttachResult struct {
	Status      string `json:"status"`
	IconID      *int   `json:"iconId,omitempty"`
	CoverID     *int   `json:"coverId,omitempty"`
	ID          string `json:"id,omitempty"` // screenshot id
	Order       *int   `json:"order,omitempty"`
	ScanPending bool   `json:"scanPending,omitempty"`
}

// ScanStatus is a per-image scan state (getAssetScanStatuses).
type ScanStatus struct {
	ImageID int    `json:"imageId"`
	Status  string `json:"status"` // "scanned" | "blocked" | "pending"
}

// Severity values carried by ListingProblem.Severity. They are the SERVER's
// vocabulary (`ListingProblemSeverity` in
// civitai:src/server/services/blocks/listing-problems.ts), reproduced here as
// constants so the CLI compares against one spelling rather than a literal at
// each site.
//
// 🔴 THE CLI DOES NOT RE-DERIVE SEVERITY FROM THE CODE, and that is deliberate.
// `computeListingProblems` owns which codes are blocking; a second table here
// would be the two-copies-of-one-predicate shape that this repo keeps finding
// wrong at N-1 sites. So an UNKNOWN severity string is neither promoted to
// blocking nor silently dropped — see doctorIsBlocking in
// internal/cmd/app_doctor.go for what the command does with one.
const (
	SeverityBlocking = "blocking"
	SeverityAdvisory = "advisory"
)

// ListingProblem is one row of a listing's completeness advisory, exactly as
// `computeListingProblems` emits it: a stable `code`, a human `label` the SERVER
// writes, and a `severity`.
//
// 🔴 `Label` IS THE SERVER'S SENTENCE AND THE CLI DOES NOT REWRITE IT. For
// `blocked-media` and `scanning-media` the label is the only place the affected
// asset KIND appears — the code itself is kind-less ("Replace the blocked icon
// before it can publish"). A CLI-side label table would therefore lose the one
// fact those two codes carry.
type ListingProblem struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Severity string `json:"severity"`
}

// MyListing is one row of appListings.listMine — a listing the caller OWNS or
// holds an ACCEPTED editor seat on.
//
// 🔴 IT IS NOT A SUBMISSIONS ROW. `GET /api/v1/blocks/submissions` (what `app
// status` reads) is scoped to what the caller SUBMITTED, so a listing acquired
// by ownership transfer is invisible there to its new owner and a collaborator
// who submitted nothing sees none at all. This read is scoped by ownership ∪
// accepted seats, which is why `app doctor` uses it and not the submissions
// route.
//
// Only the fields `app doctor` renders are decoded. The server also sends
// `kind`, `capabilities`, `iconUrl`, `coverUrl` and `updatedAt`; adding one here
// is a decision to render it, not a formality.
type MyListing struct {
	AppListingID string `json:"appListingId"`
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	// Status is the listing lifecycle: draft|pending|approved|rejected|removed.
	Status string `json:"status"`
	// Role is "owner" or "editor" — an accepted collaborator seat.
	Role string `json:"role"`
	// Kind is "onsite" or "offsite", and it decides WHO OWNS THE TEXT.
	//
	// 🔴 IT IS DECODED BECAUSE THE REMEDY DEPENDS ON IT, not for completeness.
	// An ONSITE listing's name/tagline/description/category are MANIFEST-governed
	// and have no author surface other than `block.manifest.json`: on every
	// subsequent-version moderator approve, the `(3b-sync)` re-sync in
	// `<civitai>/src/server/services/blocks/publish-request.service.ts:2742-2800`
	// overwrites all four from `buildListingScalarSync`, scoped `kind: 'onsite'`.
	// So telling an onsite author to edit those fields anywhere but the manifest
	// is advice whose effect the platform reverts. Verified at origin/main,
	// 2026-08-24.
	Kind string `json:"kind"`
	// AppBlockID is null for an OFF-SITE listing, and for an on-site app whose
	// first version has not been approved yet. Legitimately absent, not missing.
	AppBlockID *string `json:"appBlockId"`
	// Problems is never null on a current server — an all-complete listing sends
	// `[]`. A nil slice here therefore reads the same as an empty one and no
	// caller has to tell them apart.
	Problems []ListingProblem `json:"problems"`
}

// ListMyListings reads every listing the caller owns or holds an accepted editor
// seat on, each with its completeness `problems[]`.
//
// The proc takes NO input (see trpcQuery's nil-input note). It is a pure read:
// unlike GetMyListingForEdit it opens no shadow revision, so it is safe to poll.
//
// 🔴 An empty result is a REAL answer — "you can work on no listings" — and is
// returned as an empty slice with a nil error. It is not a 404: the server
// answers `[]` for a caller with the author flag and nothing to show.
func (c *Client) ListMyListings(ctx context.Context) ([]MyListing, error) {
	var out []MyListing
	if err := c.trpcQuery(ctx, trpcListMine, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SubmitRevisionResult mirrors submitListingRevision's result.
type SubmitRevisionResult struct {
	PublishRequestID string `json:"publishRequestId"`
	ShadowID         string `json:"shadowId"`
	Slug             string `json:"slug"`
}

// trpcQuery issues a tRPC GET query and decodes result.data.json into out.
//
// 🔴 A nil `input` sends NO `?input=` parameter at all, and that is not the same
// wire request as `?input={"json":null}`. A tRPC procedure declared with no
// `.input()` schema (appListings.listMine is the one such route this client
// speaks) parses whatever arrives; sending an explicit null is a value the
// server never has to accept, and the CLI has no reason to make it. Omitting the
// key is what the tRPC client itself does for an input-less query.
func (c *Client) trpcQuery(ctx context.Context, route listingRoute, input any, out any) error {
	reqURL := c.BaseURL + route.path
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
		return listingError(status, raw, route)
	}
	return decodeTRPCData(raw, out, route)
}

// trpcMutation issues a tRPC POST mutation and decodes result.data.json into out
// (out may be nil to ignore the result).
func (c *Client) trpcMutation(ctx context.Context, route listingRoute, input any, out any) error {
	body, err := json.Marshal(map[string]any{"json": input})
	if err != nil {
		return err
	}
	build := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+route.path, bytes.NewReader(body))
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
		return listingError(status, raw, route)
	}
	if out == nil {
		return nil
	}
	return decodeTRPCData(raw, out, route)
}

// decodeTRPCData pulls result.data.json out of a tRPC success envelope.
func decodeTRPCData(raw []byte, out any, route listingRoute) error {
	var env struct {
		Result struct {
			Data struct {
				JSON json.RawMessage `json:"json"`
			} `json:"data"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || len(env.Result.Data.JSON) == 0 {
		return fmt.Errorf("unexpected %s response: %s", route.name(), string(raw))
	}
	if err := json.Unmarshal(env.Result.Data.JSON, out); err != nil {
		return fmt.Errorf("unexpected %s payload: %s", route.name(), string(raw))
	}
	return nil
}

// GetMyListingForApp resolves the caller's own listing to an AppListing id +
// lifecycle status, by its backing appBlockId and/or its slug. At least one must
// be non-empty (the server enforces the same rule) — pass BOTH when known.
//
// 🔴 The SLUG path is what reaches a PRE-APPROVAL DRAFT. A first-version app has no
// backing AppBlock while it is pending review, so its draft listing (minted at
// `civitai app submit`) has `appBlockId = NULL` and there is no appBlockId to send:
// resolving by appBlockId alone can never see it. That is the whole reason listing
// media is settable while pending — and it is a fact about which SELECTOR this client
// holds, not about how wide the server's slug arm is. The scope of that arm has
// already moved once (civitai/civitai#3989) and is stated in ONE place, next to the
// caller that depends on it: see `resolveListing` in `internal/cmd/app_listing.go`,
// and civitai/cli#424 for the over-general claim this replaced.
//
// NOT_FOUND (404) means no listing row exists for the app at all.
func (c *Client) GetMyListingForApp(ctx context.Context, appBlockID, slug string) (*ListingRef, error) {
	in := map[string]string{}
	if appBlockID != "" {
		in["appBlockId"] = appBlockID
	}
	if slug != "" {
		in["slug"] = slug
	}
	if len(in) == 0 {
		return nil, fmt.Errorf("getMyListingForApp needs an appBlockId or a slug")
	}
	var out ListingRef
	if err := c.trpcQuery(ctx, trpcGetMyListingForApp, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetMyListingForEdit reads the effective listing media (icon/cover/screenshots)
// for the given AppListing id. 🔴 Side effect: for an APPROVED parent the server
// idempotently opens a shadow revision and returns ITS assets.
func (c *Client) GetMyListingForEdit(ctx context.Context, listingID string) (*ListingEditView, error) {
	var out ListingEditView
	if err := c.trpcQuery(ctx, trpcGetMyListingForEdit, map[string]string{"listingId": listingID}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetAssetScanStatuses polls the scan state of the given image ids.
func (c *Client) GetAssetScanStatuses(ctx context.Context, imageIDs []int) ([]ScanStatus, error) {
	var out struct {
		Statuses []ScanStatus `json:"statuses"`
	}
	if err := c.trpcQuery(ctx, trpcGetAssetScanStatuses, map[string]any{"imageIds": imageIDs}, &out); err != nil {
		return nil, err
	}
	return out.Statuses, nil
}

// IngestAssetFromDataURI ingests inline icon bytes as a `data:image/...;base64,…`
// URI (the icon-only lean path) and returns the scannable Image id. The server
// caps the decoded size at ~2 MiB and rasterizes to PNG. `kind` is fixed to
// "icon" — the proc's schema only accepts icons on this path.
func (c *Client) IngestAssetFromDataURI(ctx context.Context, data []byte, mimeType string) (int, error) {
	dataURI := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
	// The proc returns `{ imageId: number }` (listing-meta.service.ts) — an OBJECT,
	// not a bare number.
	var out struct {
		ImageID int `json:"imageId"`
	}
	if err := c.trpcMutation(ctx, trpcIngestAssetFromDataURI, map[string]any{"dataUri": dataURI, "kind": "icon"}, &out); err != nil {
		return 0, err
	}
	return out.ImageID, nil
}

// MintImageUpload mints a presigned PUT URL for a full-resolution asset upload.
func (c *Client) MintImageUpload(ctx context.Context) (id, uploadURL string, err error) {
	build := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+imageUploadRoute.path, nil)
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return "", "", err
	}
	if status != http.StatusOK {
		return "", "", listingError(status, raw, imageUploadRoute)
	}
	var out struct {
		ID        string `json:"id"`
		UploadURL string `json:"uploadURL"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.ID == "" || out.UploadURL == "" {
		return "", "", fmt.Errorf("unexpected image-upload response: %s", string(raw))
	}
	return out.ID, out.UploadURL, nil
}

// putPresigned uploads bytes to a presigned PUT URL. The URL is signed for a bare
// PUT (host only — Content-Type is NOT signed), so, mirroring the web client's
// `xhr.open('PUT'); xhr.send(file)`, no Content-Type / Authorization header is
// set (they would either be ignored or break the signature).
func (c *Client) putPresigned(ctx context.Context, uploadURL string, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("uploading image bytes: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := c.readBody(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("image upload PUT failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

// IngestAssetFullRes mints an upload URL, PUTs the raw bytes, then persists the
// Image row via persistAssetImage — the full-resolution path for cover /
// screenshot (the data-URI path is icon-only). Returns the scannable Image id.
func (c *Client) IngestAssetFullRes(ctx context.Context, data []byte, info ImageInfo) (int, error) {
	id, uploadURL, err := c.MintImageUpload(ctx)
	if err != nil {
		return 0, err
	}
	if err := c.putPresigned(ctx, uploadURL, data); err != nil {
		return 0, err
	}
	in := map[string]any{
		"url":       id,
		"width":     info.Width,
		"height":    info.Height,
		"mimeType":  info.MimeType,
		"sizeBytes": len(data),
	}
	// persistListingAssetImage returns `{ imageId: number }` (offsite-listing.service.ts)
	// — an OBJECT, not a bare number.
	var out struct {
		ImageID int `json:"imageId"`
	}
	if err := c.trpcMutation(ctx, trpcPersistAssetImage, in, &out); err != nil {
		return 0, err
	}
	return out.ImageID, nil
}

// SetIcon attaches an ingested (icon) image to the listing.
func (c *Client) SetIcon(ctx context.Context, listingID string, imageID int) (*AttachResult, error) {
	var out AttachResult
	if err := c.trpcMutation(ctx, trpcSetIcon, map[string]any{"listingId": listingID, "imageId": imageID}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SetCover attaches an ingested (cover) image to the listing.
func (c *Client) SetCover(ctx context.Context, listingID string, imageID int) (*AttachResult, error) {
	var out AttachResult
	if err := c.trpcMutation(ctx, trpcSetCover, map[string]any{"listingId": listingID, "imageId": imageID}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AddScreenshot appends an ingested screenshot (with an optional caption).
func (c *Client) AddScreenshot(ctx context.Context, listingID string, imageID int, caption string) (*AttachResult, error) {
	in := map[string]any{"listingId": listingID, "imageId": imageID}
	if caption != "" {
		in["caption"] = caption
	}
	var out AttachResult
	if err := c.trpcMutation(ctx, trpcAddScreenshot, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RemoveScreenshot removes a screenshot by its id.
func (c *Client) RemoveScreenshot(ctx context.Context, screenshotID string) error {
	return c.trpcMutation(ctx, trpcRemoveScreenshot, map[string]any{"screenshotId": screenshotID}, nil)
}

// ReorderScreenshots writes the listing's screenshots into the given order
// (orderedIds MUST be exactly the current set).
func (c *Client) ReorderScreenshots(ctx context.Context, listingID string, orderedIDs []string) error {
	return c.trpcMutation(ctx, trpcReorderScreenshots, map[string]any{"listingId": listingID, "orderedIds": orderedIDs}, nil)
}

// BeginListingRevision opens (or reuses) a shadow-draft revision of an approved
// listing, returning the shadow id to attach media against.
func (c *Client) BeginListingRevision(ctx context.Context, listingID string) (shadowID string, created bool, err error) {
	var out struct {
		ShadowID string `json:"shadowId"`
		Created  bool   `json:"created"`
	}
	if err := c.trpcMutation(ctx, trpcBeginListingRevision, map[string]string{"listingId": listingID}, &out); err != nil {
		return "", false, err
	}
	return out.ShadowID, out.Created, nil
}

// SubmitListingRevision submits a prepared shadow revision for moderator review.
func (c *Client) SubmitListingRevision(ctx context.Context, shadowID, changelog string) (*SubmitRevisionResult, error) {
	in := map[string]any{"shadowId": shadowID}
	if changelog != "" {
		in["changelog"] = changelog
	}
	var out SubmitRevisionResult
	if err := c.trpcMutation(ctx, trpcSubmitListingRevision, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// trpcName renders a proc path as its bare proc name for error messages.
func trpcName(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// listingOp is WHAT THE CALLER ASKED THIS ROUTE FOR — a lookup, a write to the
// listing, or the ingest of an image that is attached to nothing yet. A 400 is
// worded from it, so getting it wrong makes the CLI state something false about
// the user's data.
//
// 🔴 The rule is the CALLER'S REQUEST, deliberately not "every write the server
// performs", and `getMyListingForEdit` is the entry that makes the difference
// visible: it is classified `read` because the user asked to look, while the
// server documents a side effect — for an APPROVED parent it idempotently opens
// a shadow revision (see GetMyListingForEdit). Classifying it `change` would
// tell someone whose `listingId` was rejected to go fix a value they never sent.
//
// 🔴 THE 400 ARM IS MEASURED, AND `listingOpRead`'s WORDING SURVIVES IT.
// civitai/cli#389 doubted exactly one thing: this CLI cannot see the database,
// so "nothing was changed" on a 400 was a claim about the REQUEST. The question
// it named was whether the server opens the revision BEFORE or AFTER validating
// the input. It is AFTER: the open is the LAST thing the resolver does before
// reading, and every refusal it can raise precedes it. Read in
// `civitai/civitai` at 3ff050f2568504aa0c4c302f53818baaa35fbe11,
// `src/server/services/blocks/offsite-listing.service.ts`, in source order:
//
//	:1494  loadOwnedEditableListing  → NOT_FOUND / NOT_OWNED
//	:1496  listing.revisionOfId != null → INVALID_REVISION
//	:1502  the status switch → removed FORBIDDEN, rejected MUST_RESUBMIT,
//	       anything not draft/pending/approved INVALID_REVISION
//	:1540  if (listing.status === 'approved') { beginListingRevision(...) }
//
// Every refusal the resolver can raise sits ABOVE :1540, so each one returns
// before the INSERT is attempted. The proc's zod input (`listingId` 1..64, in
// `src/server/schema/blocks/offsite-listing.schema.ts:247`) and the
// `appDeveloperProcedure` middleware both reject before the resolver is entered
// at all. `mapOffsiteError` (`src/server/routers/app-listings.router.ts:252`)
// sends NOT_FOUND→404, NOT_OWNED/FORBIDDEN→403, and every other typed code —
// INVALID_REVISION, MUST_RESUBMIT — to 400; an untyped failure becomes a 500
// with a generic message. AFTER the shadow-open only 404 (loadListingEditView's
// NOT_FOUND) and 500 are reachable. The single 400 that can fire once the
// INSERT has been ATTEMPTED is `beginListingRevision`'s `if (!winner)`
// INVALID_REVISION (:1143), and it fires only when no shadow row exists for the
// parent — i.e. precisely when nothing was written.
//
// So a 400 from this route means nothing was changed, and this classification
// is correct rather than merely convenient.
//
// 🔴 THAT SETTLES THE ERROR ARM ONLY — THE WRITE ON SUCCESS STANDS, AND GOT
// BIGGER. A SUCCESSFUL call against an APPROVED parent still mints a shadow
// revision. Since civitai/cli#453 (#422 outcome 1) `app listing` also reaches
// OFFSITE apps by slug, and every offsite app measured on civitai.com is
// approved — so minting a shadow is now the NORMAL outcome there, not an edge
// case. `app listing status` is still not a
// pure read and still must not be polled; the warnings in its `Long` and in
// README.md say so and stay. Nothing here licenses re-labelling this route, and
// nothing here makes it safe to point a live-app probe at it.
//
// 🔴 IT IS NOT THE HTTP VERB, and civitai/cli#374 is what that cost: keying it
// on the verb at the three call sites (trpcQuery → read, trpcMutation → change,
// MintImageUpload → upload) put both ingest routes on the wrong side.
// `ingestAssetFromDataUri` and `persistAssetImage` are POSTs that create an
// Image row and touch NO listing, yet reported "the server rejected this
// store-listing change … `civitai app listing status` shows the listing as it
// stands" — and in `IngestAssetFullRes` ONE user action told two stories, "no
// listing was changed" if the mint 400s and "fix the value" if the persist
// 400s, three lines later. (Both quotes are the wording OF THAT DAY; the change
// arm no longer says "fix the value" at all — see civitai/cli#391 at the arm
// itself. They are kept verbatim because they are what the bug printed.)
//
// The mirror is the dangerous direction: setIcon / setCover / addScreenshot may
// have PARTIALLY APPLIED when they 400, so wording one as a lookup asserts
// "nothing was changed" about a change that might have happened.
type listingOp int

const (
	// listingOpUnclassified is the ZERO value, and it is deliberately NOT
	// `read`: `listingRoute{path: "…"}` with the op left off is a keyed literal
	// Go accepts happily, and if the zero meant "read" that route would ship
	// silently telling users "nothing was changed" — #374's dangerous direction,
	// regenerated. Its 400 arm claims nothing instead.
	listingOpUnclassified listingOp = iota
	// listingOpRead looks something up and writes nothing.
	listingOpRead
	// listingOpChange writes the listing (or its shadow revision).
	listingOpChange
	// listingOpIngest creates or uploads an Image row that is attached to
	// nothing yet — the presigned mint, the data-URI ingest, the persist.
	listingOpIngest
)

// listingRoute is a listing-media route: its path AND what it does, in one
// value. They travel together so the op is stated ONCE, where the route is
// declared, instead of being re-derived at each call site. Open-coded, the
// predicate was wrong at ONE of its three sites (`trpcMutation`) — but that one
// site answered for two routes, and `MintImageUpload` was right, which is
// exactly why nobody spotted it: two of the three sites read correctly.
//
// The compiler does not force you to fill in `op` (a keyed literal may omit
// it), so what enforces classification is three guards that must AGREE:
// listing_op_test.go's ledger (no declared route without a behavioural case),
// its per-case hand-written `wantOp`, and procname_leak_test.go's spelled-out
// wording table — the last deliberately in another file, because editing the
// first two together is a two-line re-label that used to go green.
type listingRoute struct {
	path string
	op   listingOp
}

// name renders the route as its bare proc name for error messages.
func (r listingRoute) name() string { return trpcName(r.path) }

// listingError maps a non-200 listing-media response to an actionable CLI error.
// tRPC error bodies are {error:{json:{message,code}}}; the HTTP status carries
// the mapped code (403 scope/cohort, 404 no listing, 400 attach/scan rejection).
func listingError(status int, raw []byte, route listingRoute) (err error) {
	defer func() { err = civitai.TagStatus(status, err) }()
	var env struct {
		Error struct {
			JSON struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"json"`
		} `json:"error"`
	}
	msg := serverMessage(raw)
	if json.Unmarshal(raw, &env) == nil && env.Error.JSON.Message != "" {
		msg = env.Error.JSON.Message
	}
	name := route.name()
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("not logged in (401) — run `civitai login` (or set CIVITAI_TOKEN)")
	case http.StatusForbidden:
		if isInsufficientScopeMsg(msg) {
			return fmt.Errorf("listing media needs the Apps submit scope (403): %s — re-run `civitai login` (an old token may predate the scope), or use a full-scope personal API key", msg)
		}
		return fmt.Errorf("not permitted for your account (403): %s — managing store listings needs Apps-author access (invite-only beta)", msg)
	case http.StatusNotFound:
		return fmt.Errorf("no store listing found for this app (404): %s — a store listing is created when you run `civitai app submit` and is settable while the app is pending review; submit the app first, then these commands will work", msg)
	case http.StatusBadRequest:
		// 🔴 NOT `name` here. A 400 is the user's input being refused, and
		// leading with the tRPC method ("appListings.setIcon rejected the
		// request") reads as "the CLI is calling something that does not
		// exist" — the tool looks broken instead of the input (civitai/cli#363).
		// The default: arm below keeps the method name, which is where a
		// genuinely unexpected status makes it worth reporting.
		//
		// The SUBJECT comes from the ROUTE's op, not from the status and not
		// from the HTTP verb: naming the operation keeps the error identifiable
		// without the proc name, and a read or an image ingest must not be
		// reported as a change that did not happen.
		switch route.op {
		case listingOpRead:
			// 🔴 IT NO LONGER SAYS "check the app you named", and adding the
			// FOURTH read (appListings.listMine, the `app doctor` read) is why.
			// That route takes NO INPUT AT ALL — no slug, no listing id, no
			// image id — so the old remedy presumed a value its caller had never
			// supplied. That is civitai/cli#391's wrong-subject class exactly,
			// regenerated on the read arm instead of the change arm: one arm
			// answering for N routes may only claim what is true of all N.
			//
			// The replacement keeps the diagnostic value without the
			// presumption, because the command it names PRINTS the valid app
			// names — so a caller who did mis-name an app still gets the list
			// they needed. It also corrects the pointer: `civitai app status`
			// reads the SUBMISSIONS route, which is scoped to what the caller
			// submitted, so it cannot list a listing acquired by ownership
			// transfer or held on a collaborator seat. `app doctor` reads
			// listMine, which is scoped by ownership ∪ accepted seats.
			return fmt.Errorf("the server rejected this store-listing lookup (400): %s — nothing was changed; `civitai app doctor` lists every app you can work on", msg)
		case listingOpIngest:
			return fmt.Errorf("the server rejected the image-upload request (400): %s — no listing was changed; check the image and retry", msg)
		case listingOpChange:
			// 🔴 IT DOES NOT SAY "fix the value and retry", and civitai/cli#391
			// is why. That remedy presumes the request carried something the
			// author supplied, and this ONE arm answers for all seven change
			// routes. Re-derived per route against the call sites: five do carry
			// such a value (setIcon/setCover from the file argument,
			// addScreenshot also from --caption, removeScreenshot from the
			// screenshot id typed as argv, reorderScreenshots from the id list
			// typed as argv). `beginListingRevision` sends ONLY `listingId` —
			// minted by the CLI from a lookup that already returned 200 (the
			// live branch of runSetMedia), never typed — and
			// `submitListingRevision`'s `changelog` is CLI-minted whenever the
			// author passed --yes rather than --changelog. So the remedy was
			// false for one route and unproven for a second: the wrong-subject
			// class #374 exists to remove.
			//
			// What replaces it is true of EVERY change route, and is the whole
			// reason this arm is not the read or the ingest arm: the write may
			// have landed IN PART before the refusal. That was previously only
			// implied, by the absence of "nothing was changed". The next command
			// is unchanged — `app listing status` is still the only way to see
			// what is attached now.
			//
			// 🔴 And this arm is ALL that three of the seven get, which is why
			// it may not promise more. Concrete follow-on advice comes from
			// `attachRejectionAdvice`, reached from exactly three flows —
			// set-icon, set-cover, add-screenshot (app_listing.go's ingest and
			// attach steps) — and it names the FILE: bytes, pixel dimensions,
			// MIME. It never names a `--caption`. `rm-screenshot` and `reorder`
			// return this error bare, as do both revision steps. Measured
			// through the real command tree: set-icon renders 7 lines,
			// rm-screenshot and reorder render 1. So "the routes with a value
			// get advice elsewhere" is true of three routes, not of the five
			// that carry an author-controlled field — do not restate it as the
			// latter, which an earlier draft of this comment and of README's
			// Troubleshooting row both did.
			return fmt.Errorf("the server rejected this store-listing change (400): %s — the change may have partially applied; `civitai app listing status` shows the listing as it stands", msg)
		default:
			// An UNCLASSIFIED route. Every other arm asserts something about
			// the user's listing; none of them is safe to guess, so this one
			// says only what happened, names the call, and asks for the report
			// that is the actual next step — a route nobody classified is a CLI
			// bug, not bad user input, which is also why the proc name is a
			// deliberate exception to the #363 rule above.
			return fmt.Errorf("%s rejected the request (HTTP 400): %s — this is a CLI bug (the route is unclassified); please report it at https://github.com/civitai/cli/issues", name, msg)
		}
	case http.StatusTooManyRequests:
		return fmt.Errorf("rate limited, try again shortly (429): %s", msg)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("apps unavailable (503): %s", msg)
	default:
		return fmt.Errorf("%s failed (HTTP %d): %s", name, status, msg)
	}
}
