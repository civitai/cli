package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewDefaultsSubmitPath(t *testing.T) {
	c := New("https://civitai.com/", "tok", "")
	if c.SubmitPath != DefaultSubmitPath {
		t.Errorf("default SubmitPath = %q, want %q", c.SubmitPath, DefaultSubmitPath)
	}
	if c.BaseURL != "https://civitai.com" {
		t.Errorf("BaseURL should be trimmed of trailing slash: %q", c.BaseURL)
	}
}

func TestSubmitVersionStatusErrors(t *testing.T) {
	cases := []struct {
		status int
		want   string
	}{
		{http.StatusUnauthorized, "unauthorized (401)"},
		{http.StatusForbidden, "forbidden (403)"},
		{http.StatusServiceUnavailable, "service unavailable (503)"},
		{http.StatusInternalServerError, "server returned 500"},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
		}))
		c := New(srv.URL, "tok", "")
		_, err := c.SubmitVersion(context.Background(), []byte("z"), "my-block", "0.1.0")
		srv.Close()
		if err == nil {
			t.Errorf("status %d: expected error", tc.status)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("status %d: error %q should contain %q", tc.status, err, tc.want)
		}
	}
}

func TestSubmitVersionGarbageResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()
	c := New(srv.URL, "tok", "")
	if _, err := c.SubmitVersion(context.Background(), []byte("z"), "my-block", "0.1.0"); err == nil {
		t.Fatal("expected error for non-JSON 200 response")
	}
}

func TestWhoAmIServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"bad token"}`))
	}))
	defer srv.Close()
	c := New(srv.URL, "tok", "")
	_, err := c.WhoAmI(context.Background())
	if err == nil || !strings.Contains(err.Error(), "bad token") {
		t.Errorf("WhoAmI error = %v, want server message", err)
	}
}

func TestWhoAmIGarbageResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<<garbage>>"))
	}))
	defer srv.Close()
	c := New(srv.URL, "tok", "")
	if _, err := c.WhoAmI(context.Background()); err == nil {
		t.Fatal("expected error for non-JSON identity response")
	}
}

func TestWhoAmISuccessReportsThroughCmd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"username":"alice","id":7}`))
	}))
	defer srv.Close()
	c := New(srv.URL, "tok", "")
	id, err := c.WhoAmI(context.Background())
	if err != nil {
		t.Fatalf("WhoAmI: %v", err)
	}
	if id.Username != "alice" || id.ID != 7 {
		t.Errorf("identity = %+v", id)
	}
}
