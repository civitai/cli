package api

import (
	"context"
	"net/url"
)

// Creator is the minimal creator view the model endpoints embed.
type Creator struct {
	Username string `json:"username"`
}

// ModelStats is the AllTime stats block the model list/detail endpoints embed.
type ModelStats struct {
	DownloadCount int `json:"downloadCount"`
	ThumbsUpCount int `json:"thumbsUpCount"`
	CommentCount  int `json:"commentCount"`
}

// ModelListItem is the subset of a `GET /api/v1/models` item the CLI renders in
// its compact list. The full item (versions, files, images, license flags) is
// preserved for --json via the result's Raw bytes.
type ModelListItem struct {
	ID      int        `json:"id"`
	Name    string     `json:"name"`
	Type    string     `json:"type"`
	NSFW    bool       `json:"nsfw"`
	Creator *Creator   `json:"creator"`
	Stats   ModelStats `json:"stats"`
}

// ModelVersionSummary is the per-version summary shown under a model detail.
// Files is carried so the detail renderer can flag a version whose primary file
// is not model weights (the on-site-gen / training-data trap) and so `download
// --model` can resolve + inspect the model's default version. The full
// GET /api/v1/models/{id} response embeds each version's files[].
type ModelVersionSummary struct {
	ID        int                `json:"id"`
	Name      string             `json:"name"`
	BaseModel string             `json:"baseModel"`
	Files     []ModelVersionFile `json:"files"`
}

// ModelDetail is the subset of `GET /api/v1/models/{id}` the CLI renders.
type ModelDetail struct {
	ID            int                   `json:"id"`
	Name          string                `json:"name"`
	Type          string                `json:"type"`
	NSFW          bool                  `json:"nsfw"`
	Creator       *Creator              `json:"creator"`
	Tags          []string              `json:"tags"`
	Stats         ModelStats            `json:"stats"`
	ModelVersions []ModelVersionSummary `json:"modelVersions"`
}

// ModelSearchResult bundles the parsed items + pagination metadata with the raw
// response body (for --json passthrough).
type ModelSearchResult struct {
	Items    []ModelListItem `json:"items"`
	Metadata Metadata        `json:"metadata"`
	Raw      []byte          `json:"-"`
}

// SearchModels calls GET /api/v1/models with the given query params.
func (c *Client) SearchModels(ctx context.Context, q url.Values) (*ModelSearchResult, error) {
	var res ModelSearchResult
	raw, err := c.getInto(ctx, "/api/v1/models", q, &res)
	if err != nil {
		return nil, err
	}
	res.Raw = raw
	return &res, nil
}

// GetModel calls GET /api/v1/models/{id}. Returns the parsed detail and the raw
// body (for --json).
func (c *Client) GetModel(ctx context.Context, id string) (*ModelDetail, []byte, error) {
	var m ModelDetail
	raw, err := c.getInto(ctx, "/api/v1/models/"+url.PathEscape(id), nil, &m)
	if err != nil {
		return nil, nil, err
	}
	return &m, raw, nil
}
