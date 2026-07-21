package civitai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSearchArticlesBuildsRequestAndParses(t *testing.T) {
	const body = `{
	  "items": [
	    {"id": 7, "title": "My Workflow", "nsfwLevel": 1, "publishedAt": "2026-07-01T12:00:00.000Z",
	     "user": {"id": 3, "username": "bob"},
	     "tags": [{"id": 5, "name": "comfyui"}, {"id": 8, "name": "workflow"}],
	     "stats": {"favoriteCount": 4, "collectedCount": 2, "commentCount": 1, "likeCount": 9, "viewCount": 100}}
	  ],
	  "metadata": {"nextCursor": "1719835200|7", "nextPage": "https://x/y?cursor=1719835200%7C7"}
	}`
	srv, gotPath, gotQuery, gotAuth := newTestServer(t, body)

	c := New(srv.URL, "tok-1")
	q := url.Values{}
	q.Set("query", "workflow")
	q.Set("limit", "5")
	q.Set("tags", "5,8")
	q.Set("sort", "Most Reactions")
	res, err := c.SearchArticles(context.Background(), q)
	if err != nil {
		t.Fatalf("SearchArticles: %v", err)
	}
	if *gotPath != "/api/v1/articles" {
		t.Errorf("path = %q, want /api/v1/articles", *gotPath)
	}
	if gotQuery.Get("query") != "workflow" || gotQuery.Get("limit") != "5" ||
		gotQuery.Get("tags") != "5,8" || gotQuery.Get("sort") != "Most Reactions" {
		t.Errorf("query params not passed through: %v", *gotQuery)
	}
	if *gotAuth != "Bearer tok-1" {
		t.Errorf("auth = %q, want Bearer tok-1", *gotAuth)
	}
	if len(res.Items) != 1 || res.Items[0].ID != 7 || res.Items[0].User.Username != "bob" {
		t.Errorf("parsed items wrong: %+v", res.Items)
	}
	if res.Items[0].Stats.LikeCount != 9 || res.Items[0].Stats.CommentCount != 1 {
		t.Errorf("stats not parsed: %+v", res.Items[0].Stats)
	}
	if len(res.Items[0].Tags) != 2 || res.Items[0].Tags[0].Name != "comfyui" {
		t.Errorf("tags not parsed: %+v", res.Items[0].Tags)
	}
	// Article cursor is the opaque "<v>|<id>" string — must round-trip verbatim.
	if got := res.Metadata.CursorString(); got != "1719835200|7" {
		t.Errorf("CursorString = %q, want 1719835200|7", got)
	}
	if len(res.Raw) == 0 {
		t.Error("Raw body should be preserved for --json")
	}
}

func TestSearchArticlesAnonymousSendsNoAuthHeader(t *testing.T) {
	srv, _, _, gotAuth := newTestServer(t, `{"items":[],"metadata":{}}`)
	c := New(srv.URL, "") // empty token => anonymous
	if _, err := c.SearchArticles(context.Background(), url.Values{}); err != nil {
		t.Fatalf("SearchArticles: %v", err)
	}
	if *gotAuth != "" {
		t.Errorf("anonymous request should send no Authorization header, got %q", *gotAuth)
	}
}

func TestGetArticleParsesDetail(t *testing.T) {
	// Detail stats use the AllTime-suffixed keys (distinct from the list shape).
	const body = `{"id": 42, "title": "Deep Dive", "nsfwLevel": 1,
	  "publishedAt": "2026-06-15T09:30:00.000Z",
	  "user": {"id": 11, "username": "alice"},
	  "tags": [{"id": 1, "name": "tutorial"}],
	  "stats": {"viewCountAllTime": 500, "commentCountAllTime": 12, "likeCountAllTime": 40,
	            "heartCountAllTime": 5, "favoriteCountAllTime": 7, "collectedCountAllTime": 3}}`
	srv, gotPath, _, _ := newTestServer(t, body)

	c := New(srv.URL, "")
	a, raw, err := c.GetArticle(context.Background(), "42")
	if err != nil {
		t.Fatalf("GetArticle: %v", err)
	}
	if *gotPath != "/api/v1/articles/42" {
		t.Errorf("path = %q, want /api/v1/articles/42", *gotPath)
	}
	if a.ID != 42 || a.Title != "Deep Dive" || a.User.Username != "alice" {
		t.Errorf("parsed detail wrong: %+v", a)
	}
	if a.Stats == nil || a.Stats.ViewCountAllTime != 500 || a.Stats.FavoriteCountAllTime != 7 {
		t.Errorf("detail stats not parsed: %+v", a.Stats)
	}
	if len(a.Tags) != 1 || a.Tags[0].Name != "tutorial" {
		t.Errorf("tags not parsed: %v", a.Tags)
	}
	if len(raw) == 0 {
		t.Error("raw body should be returned")
	}
}

func TestSearchCollectionsBuildsRequestAndParses(t *testing.T) {
	// Collections nextCursor is NUMERIC — CursorString must still render it.
	const body = `{
	  "items": [
	    {"id": 3, "name": "Favorites", "description": "my faves", "type": "Model",
	     "nsfwLevel": 1, "read": "Public", "isPublic": true, "itemCount": 25,
	     "coverImageUrl": "https://img/cover.jpg", "user": {"id": 9, "username": "carol"}}
	  ],
	  "metadata": {"nextCursor": 3, "nextPage": "https://x/y?cursor=3"}
	}`
	srv, gotPath, gotQuery, _ := newTestServer(t, body)

	c := New(srv.URL, "")
	q := url.Values{}
	q.Set("query", "fav")
	q.Set("limit", "3")
	q.Set("sort", "Newest")
	res, err := c.SearchCollections(context.Background(), q)
	if err != nil {
		t.Fatalf("SearchCollections: %v", err)
	}
	if *gotPath != "/api/v1/collections" {
		t.Errorf("path = %q, want /api/v1/collections", *gotPath)
	}
	if gotQuery.Get("query") != "fav" || gotQuery.Get("limit") != "3" || gotQuery.Get("sort") != "Newest" {
		t.Errorf("query params not passed through: %v", *gotQuery)
	}
	if len(res.Items) != 1 || res.Items[0].ID != 3 || res.Items[0].Name != "Favorites" {
		t.Errorf("parsed items wrong: %+v", res.Items)
	}
	if res.Items[0].ItemCount != 25 || !res.Items[0].IsPublic || res.Items[0].User.Username != "carol" {
		t.Errorf("collection fields not parsed: %+v", res.Items[0])
	}
	if got := res.Metadata.CursorString(); got != "3" {
		t.Errorf("numeric cursor CursorString = %q, want 3", got)
	}
	if len(res.Raw) == 0 {
		t.Error("Raw body should be preserved for --json")
	}
}

func TestGetCollectionParsesDetail(t *testing.T) {
	// Detail drops itemCount but adds tags[]; user.id may be null.
	const body = `{"id": 88, "name": "Anime Picks", "description": null, "type": "Image",
	  "nsfwLevel": null, "read": "Public", "isPublic": true,
	  "coverImageUrl": null, "user": {"id": null, "username": "dave"},
	  "tags": [{"id": 2, "name": "anime"}, {"id": 4, "name": "style"}]}`
	srv, gotPath, _, _ := newTestServer(t, body)

	c := New(srv.URL, "")
	col, raw, err := c.GetCollection(context.Background(), "88")
	if err != nil {
		t.Fatalf("GetCollection: %v", err)
	}
	if *gotPath != "/api/v1/collections/88" {
		t.Errorf("path = %q, want /api/v1/collections/88", *gotPath)
	}
	if col.ID != 88 || col.Name != "Anime Picks" || !col.IsPublic {
		t.Errorf("parsed detail wrong: %+v", col)
	}
	if col.Description != "" || col.NSFWLevel != nil {
		t.Errorf("null fields should decode to zero/nil: %+v", col)
	}
	if col.User == nil || col.User.ID != nil || col.User.Username != "dave" {
		t.Errorf("null user id should be nil: %+v", col.User)
	}
	if len(col.Tags) != 2 || col.Tags[1].Name != "style" {
		t.Errorf("tags not parsed: %v", col.Tags)
	}
	if len(raw) == 0 {
		t.Error("raw body should be returned")
	}
}

func TestArticlesCollectionsSurfaceReadErrors(t *testing.T) {
	// 404 on both detail endpoints surfaces the API error body via readError.
	for _, tc := range []struct {
		name string
		call func(c *Client) error
	}{
		{"article", func(c *Client) error { _, _, err := c.GetArticle(context.Background(), "999"); return err }},
		{"collection", func(c *Client) error { _, _, err := c.GetCollection(context.Background(), "999"); return err }},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"No ` + tc.name + ` with id 999"}`))
		}))
		c := New(srv.URL, "")
		err := tc.call(c)
		srv.Close()
		if err == nil {
			t.Errorf("%s: expected error", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), "not found (404)") ||
			!strings.Contains(err.Error(), "No "+tc.name+" with id 999") {
			t.Errorf("%s: error should surface 404 body: %v", tc.name, err)
		}
	}
}
