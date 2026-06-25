package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWithdrawRequestSendsBearerAndBody(t *testing.T) {
	var gotAuth, gotMethod, gotPath, gotID, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			PublishRequestID string `json:"publishRequestId"`
		}
		_ = json.Unmarshal(raw, &body)
		gotID = body.PublishRequestID
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok123", "")
	if err := c.WithdrawRequest(context.Background(), "pubreq_01H"); err != nil {
		t.Fatalf("WithdrawRequest: %v", err)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth header = %q, want Bearer tok123", gotAuth)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != WithdrawPath {
		t.Errorf("path = %q, want %q", gotPath, WithdrawPath)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type = %q, want application/json", gotCT)
	}
	if gotID != "pubreq_01H" {
		t.Errorf("publishRequestId = %q, want pubreq_01H", gotID)
	}
}

func TestWithdrawRequestIdempotentOK(t *testing.T) {
	// An already-withdrawn request still returns 200 — no error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()
	c := New(srv.URL, "tok", "")
	if err := c.WithdrawRequest(context.Background(), "pubreq_done"); err != nil {
		t.Fatalf("idempotent withdraw should succeed: %v", err)
	}
}

func TestWithdrawRequestErrorMapping(t *testing.T) {
	cases := []struct {
		status int
		body   map[string]string
		want   string
	}{
		{http.StatusNotFound, map[string]string{"error": "not found"}, "not found (or not yours)"},
		{http.StatusConflict, map[string]string{"error": "request is already approved"}, "request is already approved"},
		{http.StatusUnauthorized, map[string]string{"error": "bad key"}, "not authorized"},
		{http.StatusForbidden, map[string]string{"error": "no mod"}, "not authorized"},
		{http.StatusTooManyRequests, map[string]string{"error": "slow"}, "rate limited"},
		{http.StatusInternalServerError, map[string]string{"error": "boom"}, "server returned 500"},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_ = json.NewEncoder(w).Encode(tc.body)
		}))
		c := New(srv.URL, "tok", "")
		err := c.WithdrawRequest(context.Background(), "pubreq_01H")
		srv.Close()
		if err == nil {
			t.Fatalf("status %d: expected error", tc.status)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("status %d: error %q should contain %q", tc.status, err.Error(), tc.want)
		}
	}
}
