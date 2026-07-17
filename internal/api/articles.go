package api

import (
	"context"
	"net/url"
)

// ArticleUser is the minimal author view the article endpoints embed. The list
// and detail payloads both nest the author under `user` (the models endpoints
// use `creator` instead); only id + username are rendered, the rest (image,
// cosmetics, profilePicture) is preserved for --json via the raw body.
type ArticleUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

// ArticleTag is a tag attached to an article (`tags[]` on both list + detail).
type ArticleTag struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ArticleStats is the per-article stats block the LIST endpoint embeds
// (`GET /api/v1/articles`). Note the detail endpoint uses different, AllTime-
// suffixed keys — see ArticleDetailStats.
type ArticleStats struct {
	FavoriteCount  int `json:"favoriteCount"`
	CollectedCount int `json:"collectedCount"`
	CommentCount   int `json:"commentCount"`
	LikeCount      int `json:"likeCount"`
	ViewCount      int `json:"viewCount"`
}

// ArticleListItem is the subset of a `GET /api/v1/articles` item the CLI renders
// in its compact list. The full item (coverImage, cosmetics, content flags) is
// preserved for --json via the result's Raw bytes.
type ArticleListItem struct {
	ID          int          `json:"id"`
	Title       string       `json:"title"`
	NSFWLevel   int          `json:"nsfwLevel"`
	PublishedAt string       `json:"publishedAt"`
	User        *ArticleUser `json:"user"`
	Tags        []ArticleTag `json:"tags"`
	Stats       ArticleStats `json:"stats"`
}

// ArticleDetailStats is the stats block `GET /api/v1/articles/{id}` embeds. The
// detail endpoint returns AllTime-suffixed counters (from the ArticleStat table)
// — distinct from the list endpoint's ArticleStats keys.
type ArticleDetailStats struct {
	ViewCountAllTime      int `json:"viewCountAllTime"`
	CommentCountAllTime   int `json:"commentCountAllTime"`
	LikeCountAllTime      int `json:"likeCountAllTime"`
	HeartCountAllTime     int `json:"heartCountAllTime"`
	FavoriteCountAllTime  int `json:"favoriteCountAllTime"`
	CollectedCountAllTime int `json:"collectedCountAllTime"`
}

// ArticleDetail is the subset of `GET /api/v1/articles/{id}` the CLI renders.
// The endpoint also returns the full article body (content/contentJson),
// attachments, and coverImage — all preserved for --json via the raw body.
type ArticleDetail struct {
	ID          int                 `json:"id"`
	Title       string              `json:"title"`
	NSFWLevel   int                 `json:"nsfwLevel"`
	PublishedAt string              `json:"publishedAt"`
	User        *ArticleUser        `json:"user"`
	Tags        []ArticleTag        `json:"tags"`
	Stats       *ArticleDetailStats `json:"stats"`
}

// ArticleSearchResult bundles the parsed items + pagination metadata with the
// raw response body (for --json passthrough).
type ArticleSearchResult struct {
	Items    []ArticleListItem `json:"items"`
	Metadata Metadata          `json:"metadata"`
	Raw      []byte            `json:"-"`
}

// SearchArticles calls GET /api/v1/articles with the given query params.
func (c *Client) SearchArticles(ctx context.Context, q url.Values) (*ArticleSearchResult, error) {
	var res ArticleSearchResult
	raw, err := c.getInto(ctx, "/api/v1/articles", q, &res)
	if err != nil {
		return nil, err
	}
	res.Raw = raw
	return &res, nil
}

// GetArticle calls GET /api/v1/articles/{id}. Returns the parsed detail and the
// raw body (for --json).
func (c *Client) GetArticle(ctx context.Context, id string) (*ArticleDetail, []byte, error) {
	var a ArticleDetail
	raw, err := c.getInto(ctx, "/api/v1/articles/"+url.PathEscape(id), nil, &a)
	if err != nil {
		return nil, nil, err
	}
	return &a, raw, nil
}
