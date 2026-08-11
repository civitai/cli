package appapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
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
		t.Fatalf("want no-such-submission error, got %v", err)
	}
	// The KIND, not the text — the exit code is derived from this and from
	// nothing else (AGENTS.md item 7). A `?blockId=` miss answers 200 with an
	// empty list, so this never reaches submissionsError's TagStatus and has to
	// be tagged at the construction site; untagged it exits 1 where the docs
	// (and the 404-answering `?id=` spelling of the same lookup) promise 4.
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("empty-list miss must classify as not-found (exit 4), got %T: %v", err, err)
	}
	// Tag adds no visible text. The literal is written out rather than rebuilt
	// from the implementation, so an accidental edit to either side shows up.
	const want = "no such submission for app \"ghost\" — run `civitai app submit` first; " +
		"the submission and its draft store listing are created at submit time " +
		"(list what you have submitted with `civitai app status`)"
	if err.Error() != want {
		t.Errorf("classification must not change the message, got %q", err.Error())
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

// TestSubmissionNotFoundAdviceFollowsTheSelector: the 404 arm serves BOTH
// selectors, but only one of them realistically reaches it — a `?blockId=` miss
// answers 200-with-empty-list (resolved in GetSubmission), so a real 404 is the
// `?id=` spelling: a mistyped or stale publish-request id. Telling that user to
// "run `civitai app submit` first" is confidently wrong; they have submitted.
func TestSubmissionNotFoundAdviceFollowsTheSelector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Submission not found"})
	}))
	defer srv.Close()
	c := New(srv.URL, "tok", "")

	// Reachable: `civitai app status --id <id>` is exactly this call.
	_, err := c.GetSubmission(context.Background(), "pubreq_typo", "")
	if err == nil {
		t.Fatal("expected a 404 error")
	}
	msg := err.Error()
	if strings.Contains(msg, "civitai app submit") {
		t.Errorf("a bad publish-request id is not an unsubmitted app — do not send them to `app submit`; got: %s", msg)
	}
	if !strings.Contains(msg, "publish-request id") {
		t.Errorf("the advice must name what to check (the id); got: %s", msg)
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("want not-found classification (exit 4), got %T: %v", err, err)
	}

	// The SLUG selector keeps the submit advice — the same answer the empty-list
	// arm gives, so one question answered two ways still gives one answer. This
	// is the control that the fix keyed on the selector rather than deleting the
	// advice.
	if got := submissionNotFoundAdvice("", "my-block"); !strings.Contains(got, "civitai app submit") {
		t.Errorf("a slug 404 still means nothing was submitted for it; got: %s", got)
	}
	// Neither selector (the unfiltered listing): no claim about either.
	if got := submissionNotFoundAdvice("", ""); strings.Contains(got, "civitai app submit") || strings.Contains(got, "publish-request id") {
		t.Errorf("with no selector the CLI knows neither cause; got: %s", got)
	}
}

// TestSubmissionSelectorPrecedenceAgreesWithTheParse pins the id-first order in
// submissionSubject / submissionNotFoundAdvice against the precedence the CLIENT
// ITSELF applies. Swapping either branch was a SURVIVING MUTANT: the doc comment
// claimed "id wins when both are set" and nothing held the messages to it.
//
// Note the request carries BOTH params when both are given — the server resolves
// the ambiguity, and GetSubmission's parse order is where that resolution lands
// client-side: the single-submission (`?id=`) envelope is preferred over the
// list. So the messages must name the id too, or one lookup describes itself two
// contradictory ways. Reached by `civitai app status --id <id> <slug>`.
func TestSubmissionSelectorPrecedenceAgreesWithTheParse(t *testing.T) {
	const id, blockID = "pubreq_01H", "my-block"

	// (a) BEHAVIOURAL: with both selectors sent and the server answering both
	// shapes, the client takes the id-shaped one.
	byID := sampleSubmission("from-id", "approved")
	bySlug := sampleSubmission("from-slug", "rejected")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if q := r.URL.Query(); q.Get("id") != id || q.Get("blockId") != blockID {
			t.Errorf("both selectors should reach the server, got %q", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"submission":  byID,
			"submissions": []Submission{bySlug},
		})
	}))
	defer srv.Close()
	c := New(srv.URL, "tok", "")
	sub, err := c.GetSubmission(context.Background(), id, blockID)
	if err != nil {
		t.Fatalf("GetSubmission: %v", err)
	}
	if sub.BlockID != "from-id" {
		t.Fatalf("the client resolves both-selectors to the id reply; got blockId %q", sub.BlockID)
	}

	// (b) The messages must agree with (a).
	if got := submissionSubject(id, blockID); !strings.Contains(got, id) || strings.Contains(got, blockID) {
		t.Errorf("subject %q names the slug, but the lookup resolved by id", got)
	}
	if got := submissionNotFoundAdvice(id, blockID); strings.Contains(got, "civitai app submit") {
		t.Errorf("advice %q answers for the slug, but the lookup resolved by id", got)
	}
}

// TestListSubmissionsCapMirrorsServer pins the VENDORED value, not the predicate
// that reads it: ListSubmissionsCap mirrors MAX_ROWS in civitai/civitai
// src/pages/api/v1/blocks/submissions.ts, and every other test is written in
// terms of the constant, so it would happily follow the constant if someone
// edited it. The literal is the contract. If the server's page size genuinely
// changed, update BOTH this literal and the constant in the same change.
func TestListSubmissionsCapMirrorsServer(t *testing.T) {
	if ListSubmissionsCap != 100 {
		t.Errorf("ListSubmissionsCap = %d, want 100 (MAX_ROWS in submissions.ts) — "+
			"if the server page size really changed, update this literal too", ListSubmissionsCap)
	}
}
