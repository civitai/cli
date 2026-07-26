package civitai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// appsListBody is a two-card GET /api/v1/apps page: one on-site card (with a
// recommend rollup) and one off-site card (no reviews yet → recommendPct null),
// plus a nextCursor. It exercises the full field-mapping + both kind shapes.
const appsListBody = `{
  "items": [
    {
      "id": "app_01onsite",
      "slug": "cool-onsite",
      "kind": "onsite",
      "name": "Cool Onsite",
      "tagline": "does cool things",
      "category": "productivity",
      "contentRating": "pg",
      "iconUrl": "https://x/icon.png",
      "coverUrl": "https://x/cover.png",
      "creator": {"id": 7, "username": "alice", "image": "https://x/a.png"},
      "recommend": {"recommendedCount": 9, "notRecommendedCount": 1, "recommendPct": 0.9},
      "reviewCount": 10,
      "kindData": {"kind": "onsite", "appBlockId": "blk_9", "hasPage": true}
    },
    {
      "id": "app_02offsite",
      "slug": "ext-offsite",
      "kind": "offsite",
      "name": "Ext Offsite",
      "tagline": null,
      "category": null,
      "contentRating": null,
      "iconUrl": null,
      "coverUrl": null,
      "creator": null,
      "recommend": {"recommendedCount": 0, "notRecommendedCount": 0, "recommendPct": null},
      "reviewCount": 0,
      "kindData": {"kind": "offsite", "subKind": "external-link", "externalUrl": "https://ext.example/app"}
    }
  ],
  "metadata": {"nextCursor": "cur-2", "nextPage": "https://civitai.com/api/v1/apps?cursor=cur-2"}
}`

func TestSearchAppsMapsFieldsParamsAndAuth(t *testing.T) {
	var gotQuery url.Values
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps" {
			t.Errorf("path = %q, want /api/v1/apps", r.URL.Path)
		}
		gotQuery = r.URL.Query()
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, appsListBody)
	}))
	defer srv.Close()

	c := New(srv.URL, "tok-apps")
	q := valuesOf(map[string]string{
		"kind":     "onsite",
		"category": "productivity",
		"sort":     "popular",
		"cursor":   "cur-1",
		"limit":    "25",
	})
	res, err := c.SearchApps(context.Background(), q)
	if err != nil {
		t.Fatalf("SearchApps: %v", err)
	}

	// Every filter param must be encoded onto the request URL.
	for k, want := range map[string]string{
		"kind": "onsite", "category": "productivity", "sort": "popular",
		"cursor": "cur-1", "limit": "25",
	} {
		if got := gotQuery.Get(k); got != want {
			t.Errorf("query %s = %q, want %q", k, got, want)
		}
	}
	// Bearer sent.
	if gotAuth != "Bearer tok-apps" {
		t.Errorf("auth = %q, want Bearer tok-apps", gotAuth)
	}

	if len(res.Items) != 2 {
		t.Fatalf("want 2 items, got %d", len(res.Items))
	}

	// On-site card field mapping.
	on := res.Items[0]
	if on.ID != "app_01onsite" || on.Slug != "cool-onsite" || on.Kind != "onsite" || on.Name != "Cool Onsite" {
		t.Errorf("onsite core fields wrong: %+v", on)
	}
	if on.Tagline != "does cool things" || on.Category != "productivity" || on.ContentRating != "pg" {
		t.Errorf("onsite string fields wrong: %+v", on)
	}
	if on.Creator == nil || on.Creator.ID != 7 || on.Creator.Username != "alice" {
		t.Errorf("onsite creator wrong: %+v", on.Creator)
	}
	if on.Recommend.RecommendedCount != 9 || on.Recommend.NotRecommendedCount != 1 {
		t.Errorf("onsite recommend counts wrong: %+v", on.Recommend)
	}
	if on.Recommend.RecommendPct == nil || *on.Recommend.RecommendPct != 0.9 {
		t.Errorf("onsite recommendPct = %v, want 0.9", on.Recommend.RecommendPct)
	}
	if on.ReviewCount != 10 {
		t.Errorf("onsite reviewCount = %d, want 10", on.ReviewCount)
	}
	if on.KindData.Kind != "onsite" || on.KindData.AppBlockID != "blk_9" || !on.KindData.HasPage {
		t.Errorf("onsite kindData wrong: %+v", on.KindData)
	}

	// Off-site card: nullable strings decode to "", null creator → nil, null
	// recommendPct → nil (distinct from 0).
	off := res.Items[1]
	if off.Kind != "offsite" || off.Tagline != "" || off.Category != "" || off.Creator != nil {
		t.Errorf("offsite nullable handling wrong: %+v", off)
	}
	if off.Recommend.RecommendPct != nil {
		t.Errorf("offsite recommendPct should be nil (no reviews), got %v", *off.Recommend.RecommendPct)
	}
	if off.KindData.Kind != "offsite" || off.KindData.SubKind != "external-link" ||
		off.KindData.ExternalURL != "https://ext.example/app" {
		t.Errorf("offsite kindData wrong: %+v", off.KindData)
	}

	// Cursor + raw preserved.
	if res.Metadata.CursorString() != "cur-2" {
		t.Errorf("nextCursor = %q, want cur-2", res.Metadata.CursorString())
	}
	if len(res.Raw) == 0 {
		t.Error("Raw body should be preserved for --json")
	}
}

// TestSearchAppsCursorPagination drives a real 2-page fetch: page 1 hands back a
// nextCursor; the client feeds it back via ?cursor= and gets page 2 with fresh
// items and no further cursor.
func TestSearchAppsCursorPagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("cursor") {
		case "":
			_, _ = io.WriteString(w, `{"items":[{"id":"a1","slug":"one","kind":"onsite","name":"One",
				"recommend":{"recommendedCount":0,"notRecommendedCount":0,"recommendPct":null},
				"reviewCount":0,"kindData":{"kind":"onsite"}}],"metadata":{"nextCursor":"page2"}}`)
		case "page2":
			_, _ = io.WriteString(w, `{"items":[{"id":"a2","slug":"two","kind":"onsite","name":"Two",
				"recommend":{"recommendedCount":0,"notRecommendedCount":0,"recommendPct":null},
				"reviewCount":0,"kindData":{"kind":"onsite"}}],"metadata":{}}`)
		default:
			t.Errorf("unexpected cursor %q", r.URL.Query().Get("cursor"))
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	page1, err := c.SearchApps(context.Background(), url.Values{})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1.Items) != 1 || page1.Items[0].Slug != "one" {
		t.Fatalf("page1 items wrong: %+v", page1.Items)
	}
	next := page1.Metadata.CursorString()
	if next != "page2" {
		t.Fatalf("page1 nextCursor = %q, want page2", next)
	}

	page2, err := c.SearchApps(context.Background(), valuesOf(map[string]string{"cursor": next}))
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2.Items) != 1 || page2.Items[0].Slug != "two" {
		t.Fatalf("page2 items wrong: %+v", page2.Items)
	}
	if page2.Metadata.CursorString() != "" {
		t.Errorf("page2 should have no further cursor, got %q", page2.Metadata.CursorString())
	}
}

func TestSearchAppsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"items":[],"metadata":{}}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	res, err := c.SearchApps(context.Background(), url.Values{})
	if err != nil {
		t.Fatalf("SearchApps: %v", err)
	}
	if len(res.Items) != 0 {
		t.Errorf("want empty items, got %+v", res.Items)
	}
	if res.Metadata.CursorString() != "" {
		t.Errorf("empty page should have no cursor, got %q", res.Metadata.CursorString())
	}
}

// TestSearchApps401Refresh: a refreshable source is rejected once (401), the
// client refreshes and retries with the fresh token and succeeds — the same
// single-401-refresh contract as the other read methods.
func TestSearchApps401Refresh(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		seen = append(seen, auth)
		if auth != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"message":"expired"}`)
			return
		}
		_, _ = io.WriteString(w, `{"items":[],"metadata":{}}`)
	}))
	defer srv.Close()

	c := NewWithSource(srv.URL, &refreshOnceSource{badTok: "bad", goodTok: "good"})
	if _, err := c.SearchApps(context.Background(), url.Values{}); err != nil {
		t.Fatalf("SearchApps after refresh: %v", err)
	}
	if len(seen) != 2 || seen[0] != "Bearer bad" || seen[1] != "Bearer good" {
		t.Errorf("auth sequence = %v, want [bad good]", seen)
	}
}

func TestSearchAppsRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":"slow down"}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	c.RetryBackoffBase = zeroBackoff()
	c.Stderr = io.Discard
	_, err := c.SearchApps(context.Background(), url.Values{})
	if err == nil {
		t.Fatal("expected an error for a 429")
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("429 should classify as ErrRateLimited, got: %v", err)
	}
}

func TestSearchAppsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":"overloaded"}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	c.RetryBackoffBase = zeroBackoff()
	c.Stderr = io.Discard
	_, err := c.SearchApps(context.Background(), url.Values{})
	if err == nil {
		t.Fatal("expected an error for a persistent 503")
	}
	if !errors.Is(err, ErrNetwork) {
		t.Errorf("503 should classify as ErrNetwork, got: %v", err)
	}
}

func TestSearchAppsMalformedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items": "not an array"}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	_, err := c.SearchApps(context.Background(), url.Values{})
	if err == nil {
		t.Fatal("expected a parse error for a malformed body")
	}
	if !strings.Contains(err.Error(), "unexpected response") {
		t.Errorf("want an 'unexpected response' parse error, got: %v", err)
	}
}

func TestGetAppFound(t *testing.T) {
	const body = `{
	  "id": "app_detail1",
	  "serialId": 4242,
	  "slug": "cool-onsite",
	  "kind": "onsite",
	  "name": "Cool Onsite",
	  "tagline": "does cool things",
	  "description": "A longer description of the app.",
	  "category": "productivity",
	  "contentRating": "pg",
	  "iconUrl": "https://x/icon.png",
	  "coverUrl": "https://x/cover.png",
	  "creator": {"id": 7, "username": "alice", "image": "https://x/a.png"},
	  "recommend": {"recommendedCount": 9, "notRecommendedCount": 1, "recommendPct": 0.9},
	  "reviewCount": 10,
	  "screenshots": [{"url": "https://x/s1.png", "caption": "first"}, {"url": "https://x/s2.png", "caption": null}],
	  "kindData": {"kind": "onsite", "appBlockId": "blk_9", "hasPage": true, "liveUrl": "https://cool-onsite.civit.ai"}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/cool-onsite" {
			t.Errorf("path = %q, want /api/v1/apps/cool-onsite", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	d, raw, err := c.GetApp(context.Background(), "cool-onsite")
	if err != nil {
		t.Fatalf("GetApp: %v", err)
	}
	if d.ID != "app_detail1" || d.SerialID != 4242 || d.Slug != "cool-onsite" {
		t.Errorf("detail core fields wrong: %+v", d)
	}
	if d.Description != "A longer description of the app." {
		t.Errorf("description wrong: %q", d.Description)
	}
	if d.KindData.Kind != "onsite" || d.KindData.LiveURL != "https://cool-onsite.civit.ai" || !d.KindData.HasPage {
		t.Errorf("detail kindData wrong: %+v", d.KindData)
	}
	if len(d.Screenshots) != 2 || d.Screenshots[0].URL != "https://x/s1.png" || d.Screenshots[0].Caption != "first" {
		t.Errorf("screenshots wrong: %+v", d.Screenshots)
	}
	if d.Recommend.RecommendPct == nil || *d.Recommend.RecommendPct != 0.9 {
		t.Errorf("recommendPct = %v, want 0.9", d.Recommend.RecommendPct)
	}
	if len(raw) == 0 {
		t.Error("raw body should be returned for --json")
	}
}

func TestGetAppNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"App not found"}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	_, _, err := c.GetApp(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected an error for a 404")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("404 should classify as ErrNotFound, got: %v", err)
	}
}

func TestGetAppMalformedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `["not","an","object"]`)
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	_, _, err := c.GetApp(context.Background(), "slug")
	if err == nil {
		t.Fatal("expected a parse error for a malformed body")
	}
	if !strings.Contains(err.Error(), "unexpected response") {
		t.Errorf("want an 'unexpected response' parse error, got: %v", err)
	}
}

// TestReaderInterfaceHasApps is a compile-time assertion that *Client satisfies
// Reader (which now includes SearchApps + GetApp).
func TestReaderInterfaceHasApps(t *testing.T) {
	var _ Reader = (*Client)(nil)
}

// valuesOf builds a url.Values from a plain map (test convenience).
func valuesOf(m map[string]string) url.Values {
	v := url.Values{}
	for k, val := range m {
		v.Set(k, val)
	}
	return v
}
