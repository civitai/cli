package civitai

import (
	"context"
	"net/url"
)

// CollectionUser is the minimal owner view the collection endpoints embed. Both
// id and username can be null server-side, so id is a pointer and username is
// left as its zero value ("") when absent.
type CollectionUser struct {
	ID       *int   `json:"id"`
	Username string `json:"username"`
}

// CollectionTag is a tag attached to a collection (detail-only `tags[]`).
type CollectionTag struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// CollectionListItem is a `GET /api/v1/collections` item. The endpoint projects
// a fixed public shape (no cross-user fields); coverImageUrl is a ready-to-use
// edge URL (or null). itemCount is the count of ACCEPTED items.
type CollectionListItem struct {
	ID            int             `json:"id"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Type          string          `json:"type"`
	NSFWLevel     *int            `json:"nsfwLevel"`
	Read          string          `json:"read"`
	IsPublic      bool            `json:"isPublic"`
	ItemCount     int             `json:"itemCount"`
	CoverImageURL string          `json:"coverImageUrl"`
	User          *CollectionUser `json:"user"`
}

// CollectionDetail is the subset of `GET /api/v1/collections/{id}` the CLI
// renders. The detail shape drops itemCount but adds tags[].
type CollectionDetail struct {
	ID            int             `json:"id"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Type          string          `json:"type"`
	NSFWLevel     *int            `json:"nsfwLevel"`
	Read          string          `json:"read"`
	IsPublic      bool            `json:"isPublic"`
	CoverImageURL string          `json:"coverImageUrl"`
	User          *CollectionUser `json:"user"`
	Tags          []CollectionTag `json:"tags"`
}

// CollectionSearchResult bundles the parsed items + pagination metadata with the
// raw response body (for --json passthrough).
type CollectionSearchResult struct {
	Items    []CollectionListItem `json:"items"`
	Metadata Metadata             `json:"metadata"`
	Raw      []byte               `json:"-"`
}

// SearchCollections calls GET /api/v1/collections with the given query params.
func (c *Client) SearchCollections(ctx context.Context, q url.Values) (*CollectionSearchResult, error) {
	var res CollectionSearchResult
	raw, err := c.getInto(ctx, "/api/v1/collections", q, &res)
	if err != nil {
		return nil, err
	}
	res.Raw = raw
	return &res, nil
}

// GetCollection calls GET /api/v1/collections/{id}. Returns the parsed detail
// and the raw body (for --json).
func (c *Client) GetCollection(ctx context.Context, id string) (*CollectionDetail, []byte, error) {
	var col CollectionDetail
	raw, err := c.getInto(ctx, "/api/v1/collections/"+url.PathEscape(id), nil, &col)
	if err != nil {
		return nil, nil, err
	}
	return &col, raw, nil
}
