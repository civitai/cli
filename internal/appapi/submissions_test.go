package appapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func sampleSubmission(slug, status string) Submission {
	live := "https://" + slug + ".civit.ai/"
	building := "building"
	return Submission{
		ID:          "pubreq_01H",
		BlockID:     slug,
		Version:     "0.1.0",
		Status:      status,
		DeployState: &building,
		SubmittedAt: "2026-06-20T10:00:00.000Z",
		UpdatedAt:   "2026-06-20T10:00:00.000Z",
		CreatedAt:   "2026-06-20T10:00:00.000Z",
		LiveURL:     &live,
	}
}

func TestListSubmissionsSendsBearerAndParses(t *testing.T) {
	var gotAuth, gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"submissions": []Submission{
				sampleSubmission("alpha", "pending"),
				sampleSubmission("beta", "approved"),
			},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok123", "")
	subs, err := c.ListSubmissions(context.Background(), "")
	if err != nil {
		t.Fatalf("ListSubmissions: %v", err)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth header = %q, want Bearer tok123", gotAuth)
	}
	if gotPath != SubmissionsPath {
		t.Errorf("path = %q, want %q", gotPath, SubmissionsPath)
	}
	if gotQuery != "" {
		t.Errorf("query = %q, want empty for a full list", gotQuery)
	}
	if len(subs) != 2 || subs[0].BlockID != "alpha" || subs[1].Status != "approved" {
		t.Errorf("unexpected submissions: %+v", subs)
	}
}

func TestListSubmissionsNarrowsByBlockID(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("blockId")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"submissions": []Submission{sampleSubmission("alpha", "pending")},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	if _, err := c.ListSubmissions(context.Background(), "alpha"); err != nil {
		t.Fatalf("ListSubmissions: %v", err)
	}
	if gotQuery != "alpha" {
		t.Errorf("blockId query = %q, want alpha", gotQuery)
	}
}

func TestGetSubmissionByIDParsesSingle(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("id")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"submission": sampleSubmission("alpha", "rejected"),
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	sub, err := c.GetSubmission(context.Background(), "pubreq_01H", "")
	if err != nil {
		t.Fatalf("GetSubmission: %v", err)
	}
	if gotQuery != "pubreq_01H" {
		t.Errorf("id query = %q, want pubreq_01H", gotQuery)
	}
	if sub.Status != "rejected" || sub.BlockID != "alpha" {
		t.Errorf("unexpected submission: %+v", sub)
	}
}

func TestGetSubmissionByBlockIDFallsBackToList(t *testing.T) {
	// A `?blockId=` lookup returns {submissions:[...]} — GetSubmission should take
	// the first element.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"submissions": []Submission{sampleSubmission("alpha", "approved")},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	sub, err := c.GetSubmission(context.Background(), "", "alpha")
	if err != nil {
		t.Fatalf("GetSubmission: %v", err)
	}
	if sub.Status != "approved" {
		t.Errorf("unexpected submission: %+v", sub)
	}
}

func TestGetSubmissionEmptyListIsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"submissions": []Submission{}})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	_, err := c.GetSubmission(context.Background(), "", "ghost")
	if err == nil || !strings.Contains(err.Error(), "no such submission") {
		t.Errorf("want no-such-submission error, got %v", err)
	}
}

func TestSubmissionsErrorMapping(t *testing.T) {
	cases := []struct {
		status int
		body   map[string]string
		want   string
	}{
		{http.StatusUnauthorized, map[string]string{"message": "Invalid API key"}, "civitai login"},
		{http.StatusForbidden, map[string]string{"message": "restricted to the civitai team"}, "apps access required — invite-only beta"},
		{http.StatusNotFound, map[string]string{"message": "Submission not found"}, "no such submission"},
		{http.StatusTooManyRequests, map[string]string{"message": "Rate limit exceeded"}, "rate limited"},
		{http.StatusServiceUnavailable, map[string]string{"message": "Apps are not enabled"}, "not enabled"},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_ = json.NewEncoder(w).Encode(tc.body)
		}))
		c := New(srv.URL, "tok", "")
		_, err := c.ListSubmissions(context.Background(), "")
		if err == nil {
			srv.Close()
			t.Fatalf("status %d: expected error", tc.status)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("status %d: error %q should contain %q", tc.status, err.Error(), tc.want)
		}
		srv.Close()
	}
}
