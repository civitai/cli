package cmd

import (
	"net/http"
	"strings"
	"testing"
)

func TestArticlesCollectionsRegistered(t *testing.T) {
	out, _, err := run(t, "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	for _, want := range []string{"articles", "collections"} {
		if !strings.Contains(out, want) {
			t.Errorf("root help missing read command %q\n%s", want, out)
		}
	}
}

func TestArticlesSearchHumanOutput(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/articles" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.URL.Query().Get("query") != "comfyui" {
			t.Errorf("query not passed: %v", r.URL.Query())
		}
		if r.URL.Query().Get("tags") != "5,8" {
			t.Errorf("tags not passed: %v", r.URL.Query())
		}
		_, _ = w.Write([]byte(`{"items":[{"id":7,"title":"My Workflow","nsfwLevel":1,
		  "publishedAt":"2026-07-01T12:00:00.000Z","user":{"id":3,"username":"bob"},
		  "tags":[{"id":5,"name":"comfyui"}],
		  "stats":{"favoriteCount":4,"commentCount":1,"likeCount":9,"viewCount":100}}],
		  "metadata":{"nextCursor":"1719835200|7"}}`))
	})
	out, _, err := run(t, "articles", "search", "--query", "comfyui", "--tags", "5,8", "--limit", "1")
	if err != nil {
		t.Fatalf("articles search: %v", err)
	}
	if !strings.Contains(out, "My Workflow") || !strings.Contains(out, "bob") {
		t.Errorf("human output missing fields: %s", out)
	}
	if !strings.Contains(out, "next cursor: 1719835200|7") {
		t.Errorf("footer should show next cursor: %s", out)
	}
}

func TestArticlesSearchJSONPassthrough(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[],"metadata":{}}`))
	})
	out, _, err := run(t, "articles", "search", "--json")
	if err != nil {
		t.Fatalf("articles search --json: %v", err)
	}
	if !strings.Contains(out, `"items"`) || !strings.Contains(out, `"metadata"`) {
		t.Errorf("--json should print raw API JSON: %s", out)
	}
}

func TestArticlesSearchLimitValidation(t *testing.T) {
	_, _, err := run(t, "articles", "search", "--limit", "101")
	if err == nil {
		t.Fatal("expected error for --limit > 100")
	}
	if !strings.Contains(err.Error(), "1 and 100") {
		t.Errorf("error should name the max: %v", err)
	}
}

func TestArticlesGetRejectsNonNumericId(t *testing.T) {
	if _, _, err := run(t, "articles", "get", "abc"); err == nil {
		t.Fatal("expected error for non-numeric article id")
	}
}

func TestArticlesGetSurfacesServerError(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"No article with id 999"}`))
	})
	_, _, err := run(t, "articles", "get", "999")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "No article with id 999") {
		t.Errorf("error should surface API body: %v", err)
	}
}

func TestArticlesGetHumanOutput(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/articles/42" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":42,"title":"Deep Dive","nsfwLevel":1,
		  "publishedAt":"2026-06-15T09:30:00.000Z","user":{"id":11,"username":"alice"},
		  "tags":[{"id":1,"name":"tutorial"}],
		  "stats":{"viewCountAllTime":500,"likeCountAllTime":40,"favoriteCountAllTime":7,
		           "commentCountAllTime":12,"collectedCountAllTime":3}}`))
	})
	out, _, err := run(t, "articles", "get", "42")
	if err != nil {
		t.Fatalf("articles get: %v", err)
	}
	if !strings.Contains(out, "Deep Dive (id 42)") || !strings.Contains(out, "alice") {
		t.Errorf("detail output missing fields: %s", out)
	}
	if !strings.Contains(out, "views: 500") {
		t.Errorf("detail output missing AllTime stats: %s", out)
	}
}

func TestCollectionsSearchHumanOutput(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/collections" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.URL.Query().Get("query") != "anime" {
			t.Errorf("query not passed: %v", r.URL.Query())
		}
		_, _ = w.Write([]byte(`{"items":[{"id":3,"name":"Anime Picks","type":"Image",
		  "read":"Public","isPublic":true,"itemCount":25,
		  "user":{"id":9,"username":"carol"}}],
		  "metadata":{"nextCursor":3}}`))
	})
	out, _, err := run(t, "collections", "search", "--query", "anime", "--limit", "1")
	if err != nil {
		t.Fatalf("collections search: %v", err)
	}
	if !strings.Contains(out, "Anime Picks") || !strings.Contains(out, "carol") {
		t.Errorf("human output missing fields: %s", out)
	}
	// Numeric cursor must still render in the footer.
	if !strings.Contains(out, "next cursor: 3") {
		t.Errorf("footer should show numeric next cursor: %s", out)
	}
}

func TestCollectionsSearchJSONPassthrough(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[],"metadata":{}}`))
	})
	out, _, err := run(t, "collections", "search", "--json")
	if err != nil {
		t.Fatalf("collections search --json: %v", err)
	}
	if !strings.Contains(out, `"items"`) {
		t.Errorf("--json should print raw API JSON: %s", out)
	}
}

func TestCollectionsSearchLimitValidation(t *testing.T) {
	_, _, err := run(t, "collections", "search", "--limit", "101")
	if err == nil {
		t.Fatal("expected error for --limit > 100")
	}
	if !strings.Contains(err.Error(), "1 and 100") {
		t.Errorf("error should name the max: %v", err)
	}
}

func TestCollectionsGetRejectsNonNumericId(t *testing.T) {
	if _, _, err := run(t, "collections", "get", "abc"); err == nil {
		t.Fatal("expected error for non-numeric collection id")
	}
}

func TestCollectionsGetHumanOutput(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/collections/88" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":88,"name":"Favorites","description":"my faves",
		  "type":"Model","read":"Public","isPublic":true,
		  "user":{"id":9,"username":"dave"},"tags":[{"id":2,"name":"anime"}]}`))
	})
	out, _, err := run(t, "collections", "get", "88")
	if err != nil {
		t.Fatalf("collections get: %v", err)
	}
	if !strings.Contains(out, "Favorites (id 88)") || !strings.Contains(out, "dave") {
		t.Errorf("detail output missing fields: %s", out)
	}
	if !strings.Contains(out, "anime") {
		t.Errorf("detail output missing tags: %s", out)
	}
}
