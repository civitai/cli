package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGetBuzzAccountGarbageResponse: a 200 that isn't the tRPC success envelope
// (or a null .json) must error clearly rather than nil-deref the balance.
func TestGetBuzzAccountGarbageResponse(t *testing.T) {
	for _, body := range []string{
		`not json`,
		`{"result":{"data":{"json":null}}}`,
		`{}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		c := New(srv.URL, "tok", "")
		_, err := c.GetBuzzAccount(context.Background())
		srv.Close()
		if err == nil {
			t.Errorf("expected error for garbage buzz body %q", body)
		}
		if errors.Is(err, ErrBuzzScope) {
			t.Errorf("a garbage 200 must not be classified as ErrBuzzScope: %v", err)
		}
	}
}

// TestGetBuzzAccountServerError: a non-200, non-403 status (e.g. 500) maps
// through serverError — NOT ErrBuzzScope (only a 403 means missing scope).
func TestGetBuzzAccountServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"json":{"message":"boom"}}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	_, err := c.GetBuzzAccount(context.Background())
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if errors.Is(err, ErrBuzzScope) {
		t.Errorf("a 500 must not be classified as ErrBuzzScope: %v", err)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("500 error should surface the status: %v", err)
	}
}
