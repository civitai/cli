package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"strings"
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
// query route so this can be parsed uniformly).
//
// The numeric-ish fields (cfgScale/steps/seed) are json.RawMessage rather than a
// concrete number: the values are generator-supplied and freeform — a seed can
// exceed int32 (e.g. 557589798350441), cfgScale can be a float, and a field can
// occasionally arrive as an empty string, a non-numeric string, a bool, or an
// array. Capturing the raw bytes means one surprising field can never fail the
// decode; render them with CfgScaleString/StepsString/SeedString.
type ImageMeta struct {
	Prompt         string          `json:"prompt"`
	NegativePrompt string          `json:"negativePrompt"`
	Sampler        string          `json:"sampler"`
	CfgScale       json.RawMessage `json:"cfgScale"`
	Steps          json.RawMessage `json:"steps"`
	Seed           json.RawMessage `json:"seed"`
	Model          string          `json:"Model"` // capitalized in the API payload
}

// CfgScaleString renders the cfgScale for display (empty when absent).
func (m ImageMeta) CfgScaleString() string { return numString(m.CfgScale) }

// StepsString renders the steps for display (empty when absent).
func (m ImageMeta) StepsString() string { return numString(m.Steps) }

// SeedString renders the seed for display (empty when absent).
func (m ImageMeta) SeedString() string { return numString(m.Seed) }

// numString renders a numeric-ish raw JSON value for display: an absent/null
// value → "", a JSON string ("123" / "unknown") is unwrapped, and any other JSON
// literal (number, bool, array) is printed verbatim. It never fails and never
// emits control bytes from a number literal, but callers still route the result
// through safeTerm defensively.
func numString(r json.RawMessage) string {
	s := strings.TrimSpace(string(r))
	if s == "" || s == "null" {
		return ""
	}
	if s[0] == '"' { // a JSON string — unwrap it
		var unq string
		if err := json.Unmarshal(r, &unq); err == nil {
			return unq
		}
	}
	return s
}

// MetaState classifies an image's meta field for rendering.
type MetaState int

const (
	// MetaAbsent: meta was not requested, or the API returned meta:null (the
	// uploader hid their generation data).
	MetaAbsent MetaState = iota
	// MetaOK: meta parsed into an ImageMeta object.
	MetaOK
	// MetaUnparseable: meta was present but not the expected object shape (e.g.
	// an array, or a scalar) — degrade gracefully rather than fail the page.
	MetaUnparseable
)

// ImageItem is the subset of a `GET /api/v1/images` item the CLI renders. The
// full item (tags, hash, and any meta fields beyond those in ImageMeta) is
// preserved for --json via Raw.
//
// Meta is kept as json.RawMessage — not a decoded *ImageMeta — so a malformed
// meta on any single image can never fail the whole-page decode (which would
// also break `--meta --json`, an advertised raw passthrough). It is decoded
// best-effort at render time via ParseMeta.
type ImageItem struct {
	ID        int             `json:"id"`
	URL       string          `json:"url"`
	Width     int             `json:"width"`
	Height    int             `json:"height"`
	NSFWLevel string          `json:"nsfwLevel"`
	Type      string          `json:"type"`
	PostID    *int            `json:"postId"`
	Username  string          `json:"username"`
	BaseModel string          `json:"baseModel"`
	Stats     ImageStats      `json:"stats"`
	Meta      json.RawMessage `json:"meta"`
}

// ParseMeta best-effort decodes the raw meta. It never returns an error: an
// absent or null meta → MetaAbsent, a present-but-unexpected shape →
// MetaUnparseable, so one odd image can't fail the page or the --json passthrough.
func (im ImageItem) ParseMeta() (ImageMeta, MetaState) {
	raw := bytes.TrimSpace(im.Meta)
	if len(raw) == 0 || string(raw) == "null" {
		return ImageMeta{}, MetaAbsent
	}
	var m ImageMeta
	if err := json.Unmarshal(raw, &m); err != nil {
		return ImageMeta{}, MetaUnparseable
	}
	return m, MetaOK
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
