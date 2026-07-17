package api

import (
	"context"
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

// ImageItem is the subset of a `GET /api/v1/images` item the CLI renders. The
// full item (meta, tags, hash, baseModel) is preserved for --json via Raw.
type ImageItem struct {
	ID        int        `json:"id"`
	URL       string     `json:"url"`
	Width     int        `json:"width"`
	Height    int        `json:"height"`
	NSFWLevel string     `json:"nsfwLevel"`
	Type      string     `json:"type"`
	PostID    *int       `json:"postId"`
	Username  string     `json:"username"`
	Stats     ImageStats `json:"stats"`
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
