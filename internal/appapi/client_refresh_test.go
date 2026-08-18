package appapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// stubSource is a refreshable TokenSource: it serves "stale" until Refresh is
// called once, after which it serves "fresh".
type stubSource struct {
	current      atomic.Value // string
	refreshes    int32
	refreshErr   error
	refreshedTok string
}

func newStubSource(initial, refreshed string) *stubSource {
	s := &stubSource{refreshedTok: refreshed}
	s.current.Store(initial)
	return s
}

func (s *stubSource) Token(context.Context) (string, error) { return s.current.Load().(string), nil }

func (s *stubSource) Refresh(context.Context) (string, error) {
	atomic.AddInt32(&s.refreshes, 1)
	if s.refreshErr != nil {
		return "", s.refreshErr
	}
	s.current.Store(s.refreshedTok)
	return s.refreshedTok, nil
}

func TestClient401TriggersRefreshAndRetry(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		seen = append(seen, auth)
		if auth != "Bearer fresh" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"expired"}`))
			return
		}
		_, _ = w.Write([]byte(`{"username":"zach","id":1}`))
	}))
	defer srv.Close()

	src := newStubSource("stale", "fresh")
	c := NewWithSource(srv.URL, src, "")
	id, err := c.WhoAmI(context.Background())
	if err != nil {
		t.Fatalf("WhoAmI after refresh: %v", err)
	}
	if id.Username != "zach" {
		t.Errorf("identity = %+v", id)
	}
	if src.refreshes != 1 {
		t.Errorf("expected exactly one refresh, got %d", src.refreshes)
	}
	if len(seen) != 2 || seen[0] != "Bearer stale" || seen[1] != "Bearer fresh" {
		t.Errorf("auth sequence = %v, want [stale fresh]", seen)
	}
}

func TestClientStaticTokenDoesNotRetryOn401(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"nope"}`))
	}))
	defer srv.Close()

	// StaticToken (personal key) cannot refresh: a 401 must not retry.
	c := New(srv.URL, "personal-key", "")
	if _, err := c.WhoAmI(context.Background()); err == nil {
		t.Fatal("expected a 401 error")
	}
	if hits != 1 {
		t.Errorf("static token should hit the server once (no retry), got %d", hits)
	}
}

func TestSubmitVersionUsesV1RouteAndRefreshes(t *testing.T) {
	var path, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"publishRequestId":"pr_1","slug":"x","version":"0.1.0","status":"pending"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	if _, err := c.SubmitVersion(context.Background(), []byte("ZIP"), "my-block", "0.1.0", Provenance{}); err != nil {
		t.Fatalf("SubmitVersion: %v", err)
	}
	if path != DefaultSubmitPath {
		t.Errorf("submit path = %q, want %q", path, DefaultSubmitPath)
	}
	if auth != "Bearer tok" {
		t.Errorf("auth = %q", auth)
	}
}
