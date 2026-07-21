package civitai

import (
	"context"
	"net/url"
)

// FileHashes is the per-file hash set the API returns under `files[].hashes`.
// The keys are UPPER-cased algorithm names on the wire (AutoV1, AutoV2, SHA256,
// CRC32, BLAKE3); SHA256 is the one `download` verifies against.
type FileHashes struct {
	AutoV1 string `json:"AutoV1,omitempty"`
	AutoV2 string `json:"AutoV2,omitempty"`
	SHA256 string `json:"SHA256,omitempty"`
	CRC32  string `json:"CRC32,omitempty"`
	BLAKE3 string `json:"BLAKE3,omitempty"`
}

// ModelVersionFile is the subset of a model-version `files[]` entry the CLI
// lists and downloads. ID/Primary/DownloadURL/Hashes are carried (beyond the
// display-only Name/Type/SizeKB) because `download` selects + fetches + verifies
// individual files. Field names track the live API EXACTLY (verified against
// GET /api/v1/model-versions/{id}: id, name, type, sizeKB, primary, downloadUrl,
// hashes{SHA256,…}).
type ModelVersionFile struct {
	ID          int        `json:"id"`
	Name        string     `json:"name"`
	Type        string     `json:"type"`
	SizeKB      float64    `json:"sizeKB"`
	Primary     bool       `json:"primary"`
	DownloadURL string     `json:"downloadUrl"`
	Hashes      FileHashes `json:"hashes"`
}

// ModelFileType is the `type` value of the main model-weights file. A file whose
// type is anything else (e.g. "Archive", "Training Data", "Config", "VAE") is not
// weights but a different deliverable — the renderers tag it with an
// informational type marker; `download` fetches any type regardless.
const ModelFileType = "Model"

// IsModelWeights reports whether the file is the main model-weights file
// (type == "Model"), as opposed to an archive / training data / config / a
// component file. Used only for the informational non-weights marker.
func (f *ModelVersionFile) IsModelWeights() bool { return f.Type == ModelFileType }

// PrimaryFile returns the primary file of a version's file list (the one flagged
// `primary: true`), falling back to the first file when none is flagged. Returns
// nil for an empty list. This is the file `download` fetches by default and the
// one the list/detail renderers inspect for the non-weights marker.
func PrimaryFile(files []ModelVersionFile) *ModelVersionFile {
	if len(files) == 0 {
		return nil
	}
	for i := range files {
		if files[i].Primary {
			return &files[i]
		}
	}
	return &files[0]
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
