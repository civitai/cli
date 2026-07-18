package api

import (
	"context"
	"encoding/json"
	"net/url"
)

// ImageStats is the reaction/comment stats block on an image item.
type ImageStats struct {
	LikeCount    int `json:"likeCount"`
	HeartCount   int `json:"heartCount"`
	LaughCount   int `json:"laughCount"`
	CryCount     int `json:"cryCount"`
	CommentCount int `json:"commentCount"`
}

// ImageMeta is the generation metadata attached to an image when it is requested
// with `withMeta=true` (and `flatMeta=true`, which forces the flat shape on every
// query route so this can be parsed uniformly). It is nil on the item when the
// API returns `meta: null` — either because meta was not requested or because the
// uploader chose to hide their generation data.
//
// Numeric fields use json.Number because the API values are attacker-influenced
// and can be large or float (seeds routinely exceed int32, e.g.
// 557589798350441; cfgScale is a float) — a tolerant type keeps a surprising
// value from failing the whole parse.
type ImageMeta struct {
	Prompt         string      `json:"prompt"`
	NegativePrompt string      `json:"negativePrompt"`
	Sampler        string      `json:"sampler"`
	CfgScale       json.Number `json:"cfgScale"`
	Steps          json.Number `json:"steps"`
	Seed           json.Number `json:"seed"`
	Model          string      `json:"Model"` // capitalized in the API payload
}

// ImageItem is the subset of a `GET /api/v1/images` item the CLI renders. The
// full item (tags, hash, and any meta fields beyond those below) is preserved
// for --json via Raw.
type ImageItem struct {
	ID        int        `json:"id"`
	URL       string     `json:"url"`
	Width     int        `json:"width"`
	Height    int        `json:"height"`
	NSFWLevel string     `json:"nsfwLevel"`
	Type      string     `json:"type"`
	PostID    *int       `json:"postId"`
	Username  string     `json:"username"`
	BaseModel string     `json:"baseModel"`
	Stats     ImageStats `json:"stats"`
	// Meta is nil unless the request was made with withMeta=true, and stays nil
	// when the API returns `meta: null` (uploader hid their generation data).
	Meta *ImageMeta `json:"meta"`
}

// ImageSearchResult bundles the parsed items + pagination metadata with the raw
// response body (for --json passthrough).
type ImageSearchResult struct {
	Items    []ImageItem `json:"items"`
	Metadata Metadata    `json:"metadata"`
	Raw      []byte      `json:"-"`
}

// SearchImages calls GET /api/v1/images with the given query params.
func (c *Client) SearchImages(ctx context.Context, q url.Values) (*ImageSearchResult, error) {
	var res ImageSearchResult
	raw, err := c.getInto(ctx, "/api/v1/images", q, &res)
	if err != nil {
		return nil, err
	}
	res.Raw = raw
	return &res, nil
}
