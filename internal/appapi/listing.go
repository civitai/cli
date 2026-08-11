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

// SubmitRevisionResult mirrors submitListingRevision's result.
type SubmitRevisionResult struct {
	PublishRequestID string `json:"publishRequestId"`
	ShadowID         string `json:"shadowId"`
	Slug             string `json:"slug"`
}

// trpcQuery issues a tRPC GET query and decodes result.data.json into out.
func (c *Client) trpcQuery(ctx context.Context, route listingRoute, input any, out any) error {
	inputJSON, err := json.Marshal(map[string]any{"json": input})
	if err != nil {
		return err
	}
	q := url.Values{}
	q.Set("input", string(inputJSON))
	reqURL := c.BaseURL + route.path + "?" + q.Encode()
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
// `civitai app submit`) has `appBlockId = NULL` and is resolvable ONLY by slug. That
// is the whole reason listing media is settable while pending — resolving by
// appBlockId alone can never see it.
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

// listingOp is WHAT THE ROUTE DOES — a lookup, a write to the listing, or the
// ingest of an image that is not attached to anything yet. A 400 is worded from
// it, so getting it wrong makes the CLI state something false about the user's
// data.
//
// 🔴 IT IS NOT THE HTTP VERB, and civitai/cli#374 is what that cost: keying it
// on the verb at the three call sites (trpcQuery → read, trpcMutation → change,
// MintImageUpload → upload) put both ingest routes on the wrong side.
// `ingestAssetFromDataUri` and `persistAssetImage` are POSTs that create an
// Image row and touch NO listing, yet reported "the server rejected this
// store-listing change … `civitai app listing status` shows the listing as it
// stands" — and in `IngestAssetFullRes` ONE user action told two stories, "no
// listing was changed" if the mint 400s and "fix the value" if the persist
// 400s, three lines later.
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
// declared, instead of being re-derived at each call site — a predicate
// open-coded at three sites was wrong at two of them, in the same direction.
//
// The compiler does not force you to fill in `op` (a keyed literal may omit
// it), so what enforces classification is the pair of guards in
// listing_op_test.go: the ledger fails on a declared route with no behavioural
// case, and the case's own hand-written `wantOp` fails on one classified wrong.
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
			return fmt.Errorf("the server rejected this store-listing lookup (400): %s — nothing was changed; check the app you named (list your apps with `civitai app status`)", msg)
		case listingOpIngest:
			return fmt.Errorf("the server rejected the image-upload request (400): %s — no listing was changed; check the image and retry", msg)
		case listingOpChange:
			return fmt.Errorf("the server rejected this store-listing change (400): %s — fix the value and retry; `civitai app listing status` shows the listing as it stands", msg)
		default:
			// An UNCLASSIFIED route. Every other arm asserts something about
			// the user's listing; none of them is safe to guess, so this one
			// says only what happened and names the call so a bug report can
			// find it. The proc name is a deliberate exception to the #363 rule
			// above, for the same reason the `default:` status arm keeps it: a
			// route nobody classified is a CLI bug, not bad user input.
			return fmt.Errorf("%s rejected the request (HTTP 400): %s", name, msg)
		}
	case http.StatusTooManyRequests:
		return fmt.Errorf("rate limited, try again shortly (429): %s", msg)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("apps unavailable (503): %s", msg)
	default:
		return fmt.Errorf("%s failed (HTTP %d): %s", name, status, msg)
	}
}
