package civitai

import (
	"context"
	"net/url"
)

// The App Store discovery read surface — GET /api/v1/apps (list) and
// GET /api/v1/apps/{slug} (detail). These back `civitai app list` / `app view`.
//
// The endpoint serves the unified `/apps` marketplace over BOTH kinds of app
// (`onsite` AppBlocks and `offsite` external/connect listings) from the durable
// AppListing record. It is filter-based discovery only — kind/category/sort +
// keyset cursor paging — with NO free-text query or slot filter (the store
// service exposes neither; keyword search is a deferred follow-up). The bearer
// is optional server-side but the CLI requires login for a stable caller
// identity (per-user rate-limit + the auto-widening visibility scope).
//
// The Go structs below MIRROR the backend public DTOs field-for-field (json
// tags track `ListingCard` / `ListingDetail` in
// civitai:src/server/schema/blocks/app-listing-read.schema.ts EXACTLY, like
// HashMatch tracks the by-hash endpoint) — the DTOs are a public-field
// allowlist, so a backend field rename would silently null a column here and
// the field-mapping tests are the guard.

// ListingCreatorChip is the public creator chip (id/username/image only — no
// PII). It is `null` in JSON only when the owner row vanished, so the field is a
// pointer on the card/detail structs.
type ListingCreatorChip struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Image    string `json:"image"`
}

// ListingRecommend is the recommend rollup read from the AppListingMetric
// rollup. RecommendPct is a POINTER because it is `null` (not 0) when there are
// no reviews yet — so the CLI can render "no reviews" instead of a bogus "0%".
type ListingRecommend struct {
	RecommendedCount    int `json:"recommendedCount"`
	NotRecommendedCount int `json:"notRecommendedCount"`
	// RecommendPct is up/(up+down) in [0,1], or nil when there are no reviews.
	RecommendPct *float64 `json:"recommendPct"`
}

// AppCardKindData is the discriminated kind-specific card data. The backend
// emits ONLY the fields for the row's `kind` (onsite: appBlockId/hasPage;
// offsite: subKind/externalUrl); a flat struct captures both shapes with the
// absent side left zero — Kind is the discriminator to read.
type AppCardKindData struct {
	Kind string `json:"kind"`
	// onsite:
	AppBlockID string `json:"appBlockId"`
	HasPage    bool   `json:"hasPage"`
	// offsite:
	SubKind     string `json:"subKind"`
	ExternalURL string `json:"externalUrl"`
}

// AppCard is one store card over EITHER kind (the unified `/apps` grid item).
// Mirrors backend `ListingCard`. Nullable string fields (tagline/category/…)
// are plain strings — a JSON null decodes to "" and renders as a dash.
type AppCard struct {
	ID            string              `json:"id"`
	Slug          string              `json:"slug"`
	Kind          string              `json:"kind"`
	Name          string              `json:"name"`
	Tagline       string              `json:"tagline"`
	Category      string              `json:"category"`
	ContentRating string              `json:"contentRating"`
	IconURL       string              `json:"iconUrl"`
	CoverURL      string              `json:"coverUrl"`
	Creator       *ListingCreatorChip `json:"creator"`
	Recommend     ListingRecommend    `json:"recommend"`
	ReviewCount   int                 `json:"reviewCount"`
	KindData      AppCardKindData     `json:"kindData"`
}

// AppSearchResult bundles the parsed cards + pagination metadata with the raw
// response body (for --json passthrough). It uses the shared Metadata type —
// exactly like every other Search* result — so CursorString + printPageFooter
// work unchanged; the apps endpoint populates only nextCursor/nextPage on it
// (keyset cursor pagination, no page/offset).
type AppSearchResult struct {
	Items    []AppCard `json:"items"`
	Metadata Metadata  `json:"metadata"`
	Raw      []byte    `json:"-"`
}

// SearchApps calls GET /api/v1/apps with the given query params (kind, category,
// sort, cursor, limit — encoded from q). It returns a SINGLE page plus the next
// cursor in Metadata; the caller paginates by feeding Metadata.CursorString()
// back as `cursor` (it does NOT auto-page), mirroring SearchModels/SearchImages.
//
// This is filter-based discovery: the endpoint accepts no free-text `query` or
// `slot` param (the store service supports neither). TODO(app search): when the
// backend store service adds text search, a `civitai app search <query>` becomes
// a trivial addition — set q.Set("query", …) here and add the command; the
// transport/paging/auth path is unchanged.
func (c *Client) SearchApps(ctx context.Context, q url.Values) (*AppSearchResult, error) {
	var res AppSearchResult
	raw, err := c.getInto(ctx, "/api/v1/apps", q, &res)
	if err != nil {
		return nil, err
	}
	res.Raw = raw
	return &res, nil
}

// ListingScreenshot is one ordered gallery screenshot on the detail page.
type ListingScreenshot struct {
	URL     string `json:"url"`
	Caption string `json:"caption"`
}

// AppDetailKindData is the discriminated kind-specific action data on the detail
// page. Mirrors backend `ListingDetailKindData` — onsite adds `liveUrl` (the
// already-public standalone block origin), offsite adds `connectClientId` (the
// public OAuth client_id, NOT a secret). Kind is the discriminator.
type AppDetailKindData struct {
	Kind string `json:"kind"`
	// onsite:
	AppBlockID string `json:"appBlockId"`
	HasPage    bool   `json:"hasPage"`
	LiveURL    string `json:"liveUrl"`
	// offsite:
	SubKind         string `json:"subKind"`
	ExternalURL     string `json:"externalUrl"`
	ConnectClientID string `json:"connectClientId"`
}

// AppDetail is the full public detail for one approved listing (card fields +
// gallery + body). Mirrors backend `ListingDetail`. SerialID is the integer
// surrogate key carried for the comments discussion; the store `id` is a TEXT
// ULID.
type AppDetail struct {
	ID            string              `json:"id"`
	SerialID      int                 `json:"serialId"`
	Slug          string              `json:"slug"`
	Kind          string              `json:"kind"`
	Name          string              `json:"name"`
	Tagline       string              `json:"tagline"`
	Description   string              `json:"description"`
	Category      string              `json:"category"`
	ContentRating string              `json:"contentRating"`
	IconURL       string              `json:"iconUrl"`
	CoverURL      string              `json:"coverUrl"`
	Creator       *ListingCreatorChip `json:"creator"`
	Recommend     ListingRecommend    `json:"recommend"`
	ReviewCount   int                 `json:"reviewCount"`
	Screenshots   []ListingScreenshot `json:"screenshots"`
	KindData      AppDetailKindData   `json:"kindData"`
}

// GetApp calls GET /api/v1/apps/{slug}. Returns the parsed detail and the raw
// body (for --json), like GetModel/GetArticle. A missing / out-of-scope slug
// yields HTTP 404, which readError classifies as ErrNotFound (the entrypoint
// maps it to the not-found exit code) — the detail endpoint 404s natively, so
// no empty-search faking is needed.
func (c *Client) GetApp(ctx context.Context, slug string) (*AppDetail, []byte, error) {
	var d AppDetail
	raw, err := c.getInto(ctx, "/api/v1/apps/"+url.PathEscape(slug), nil, &d)
	if err != nil {
		return nil, nil, err
	}
	return &d, raw, nil
}
