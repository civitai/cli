package api

import (
	"context"
	"net/url"
)

// ModelVersionFile is the subset of a model-version file the CLI lists.
type ModelVersionFile struct {
	Name   string  `json:"name"`
	Type   string  `json:"type"`
	SizeKB float64 `json:"sizeKB"`
}

// ModelVersionModel is the embedded parent-model summary on a version detail.
type ModelVersionModel struct {
	Name string `json:"name"`
	Type string `json:"type"`
	NSFW bool   `json:"nsfw"`
}

// ModelVersionStats is the version-level stats block.
type ModelVersionStats struct {
	DownloadCount int `json:"downloadCount"`
	ThumbsUpCount int `json:"thumbsUpCount"`
}

// ModelVersionDetail is the subset of `GET /api/v1/model-versions/{id}` (and
// `/by-hash/{hash}`) the CLI renders. The full body (images, all files with
// hashes) is preserved for --json via the raw bytes the getter returns.
type ModelVersionDetail struct {
	ID           int                `json:"id"`
	ModelID      int                `json:"modelId"`
	Name         string             `json:"name"`
	BaseModel    string             `json:"baseModel"`
	AIR          string             `json:"air"`
	DownloadURL  string             `json:"downloadUrl"`
	TrainedWords []string           `json:"trainedWords"`
	Model        *ModelVersionModel `json:"model"`
	Files        []ModelVersionFile `json:"files"`
	Stats        ModelVersionStats  `json:"stats"`
}

// GetModelVersion calls GET /api/v1/model-versions/{id}.
func (c *Client) GetModelVersion(ctx context.Context, id string) (*ModelVersionDetail, []byte, error) {
	var v ModelVersionDetail
	raw, err := c.getInto(ctx, "/api/v1/model-versions/"+url.PathEscape(id), nil, &v)
	if err != nil {
		return nil, nil, err
	}
	return &v, raw, nil
}

// GetModelVersionByHash calls GET /api/v1/model-versions/by-hash/{hash}. The
// server upper-cases the hash server-side; any file-hash type (AutoV2, SHA256,
// …) is accepted.
func (c *Client) GetModelVersionByHash(ctx context.Context, hash string) (*ModelVersionDetail, []byte, error) {
	var v ModelVersionDetail
	raw, err := c.getInto(ctx, "/api/v1/model-versions/by-hash/"+url.PathEscape(hash), nil, &v)
	if err != nil {
		return nil, nil, err
	}
	return &v, raw, nil
}
