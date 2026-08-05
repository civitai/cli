package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
)

// statusServer stands up an httptest server emulating GET
// /api/v1/blocks/submissions, returning the canned body and recording the
// request. handler may be nil for the default OK list.
func statusServer(t *testing.T, body any, status int, rec *struct {
	auth, path, id, blockID string
}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rec != nil {
			rec.auth = r.Header.Get("Authorization")
			rec.path = r.URL.Path
			rec.id = r.URL.Query().Get("id")
			rec.blockID = r.URL.Query().Get("blockId")
		}
		if status != 0 && status != http.StatusOK {
			w.WriteHeader(status)
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func TestAppStatusListRendersTable(t *testing.T) {
	var rec struct{ auth, path, id, blockID string }
	body := map[string]any{
		"submissions": []map[string]any{
			{"id": "pubreq_2", "blockId": "beta", "version": "0.2.0", "status": "approved",
				"deployState": "live", "submittedAt": "2026-06-21T09:00:00.000Z",
				"liveUrl": "https://beta.civit.ai/"},
			{"id": "pubreq_1", "blockId": "alpha", "version": "0.1.0", "status": "pending",
				"deployState": nil, "submittedAt": "2026-06-20T08:00:00.000Z", "liveUrl": nil},
		},
	}
	srv := statusServer(t, body, http.StatusOK, &rec)
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, _, err := run(t, "app", "status")
	if err != nil {
		t.Fatalf("app status: %v", err)
	}
	if rec.auth != "Bearer tok-1" {
		t.Errorf("auth header = %q", rec.auth)
	}
	if rec.path != "/api/v1/blocks/submissions" {
		t.Errorf("path = %q", rec.path)
	}
	if rec.id != "" || rec.blockID != "" {
		t.Errorf("full list should send no id/blockId, got id=%q blockId=%q", rec.id, rec.blockID)
	}
	for _, want := range []string{"BLOCK_ID", "VERSION", "STATUS", "DEPLOY", "SUBMITTED", "URL",
		"alpha", "beta", "approved", "pending", "live", "https://beta.civit.ai/", "2026-06-21"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q:\n%s", want, out)
		}
	}
}

func TestAppStatusEmptyListFriendly(t *testing.T) {
	srv := statusServer(t, map[string]any{"submissions": []any{}}, http.StatusOK, nil)
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, _, err := run(t, "app", "status")
	if err != nil {
		t.Fatalf("app status: %v", err)
	}
	if !strings.Contains(out, "No submissions yet") || !strings.Contains(out, "civitai app submit") {
		t.Errorf("empty list should be friendly: %s", out)
	}
}

func TestAppStatusDetailByBlockID(t *testing.T) {
	var rec struct{ auth, path, id, blockID string }
	body := map[string]any{
		"submissions": []map[string]any{
			{"id": "pubreq_9", "blockId": "rejected-app", "version": "0.3.0", "status": "rejected",
				"rejectionReason": "manifest scope ai:write:budgeted not allowed yet",
				"deployState":     nil, "submittedAt": "2026-06-22T12:00:00.000Z",
				"reviewedAt": "2026-06-22T15:30:00.000Z", "liveUrl": nil},
		},
	}
	srv := statusServer(t, body, http.StatusOK, &rec)
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, _, err := run(t, "app", "status", "rejected-app")
	if err != nil {
		t.Fatalf("app status <blockId>: %v", err)
	}
	if rec.blockID != "rejected-app" {
		t.Errorf("blockId query = %q, want rejected-app", rec.blockID)
	}
	for _, want := range []string{"rejected-app", "0.3.0", "rejected",
		"Rejection reason", "manifest scope ai:write:budgeted not allowed yet",
		"Not live yet", "rejected-app.civit.ai"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail output missing %q:\n%s", want, out)
		}
	}
}

func TestAppStatusDetailByIDFlagLive(t *testing.T) {
	var rec struct{ auth, path, id, blockID string }
	body := map[string]any{
		"submission": map[string]any{
			"id": "pubreq_live", "blockId": "shipped", "version": "1.0.0", "status": "approved",
			"deployState": "live", "submittedAt": "2026-06-10T00:00:00.000Z",
			"reviewedAt": "2026-06-11T00:00:00.000Z", "liveUrl": "https://shipped.civit.ai/",
		},
	}
	srv := statusServer(t, body, http.StatusOK, &rec)
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, _, err := run(t, "app", "status", "--id", "pubreq_live")
	if err != nil {
		t.Fatalf("app status --id: %v", err)
	}
	if rec.id != "pubreq_live" {
		t.Errorf("id query = %q, want pubreq_live", rec.id)
	}
	for _, want := range []string{"shipped", "approved", "live", "Live at: https://shipped.civit.ai/"} {
		if !strings.Contains(out, want) {
			t.Errorf("live detail missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Not live yet") {
		t.Errorf("a live submission should not say 'Not live yet':\n%s", out)
	}
}

func TestAppStatusJSONPassthrough(t *testing.T) {
	body := map[string]any{
		"submissions": []map[string]any{
			{"id": "pubreq_1", "blockId": "alpha", "version": "0.1.0", "status": "pending",
				"deployState": nil, "submittedAt": "2026-06-20T08:00:00.000Z", "liveUrl": nil},
		},
	}
	srv := statusServer(t, body, http.StatusOK, nil)
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, _, err := run(t, "app", "status", "--json")
	if err != nil {
		t.Fatalf("app status --json: %v", err)
	}
	// Must be valid JSON with the field names preserved (scriptable).
	var parsed struct {
		Submissions []map[string]any `json:"submissions"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\n%s", err, out)
	}
	if len(parsed.Submissions) != 1 || parsed.Submissions[0]["blockId"] != "alpha" {
		t.Errorf("unexpected --json payload: %s", out)
	}
}

// truncationNote is the distinguishing core of the cap caveat. Tests assert on
// this exact phrase so a generic word ("note:", "submissions") from some other
// message can't satisfy them.
const truncationNote = "the API caps this listing and offers no way to page"

// submissionRows builds n plausible listing rows, newest first.
func submissionRows(n int) []map[string]any {
	rows := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, map[string]any{
			"id": fmt.Sprintf("pubreq_%03d", n-i), "blockId": fmt.Sprintf("app-%03d", n-i),
			"version": "0.1.0", "status": "approved", "deployState": "live",
			"submittedAt": "2026-06-17T08:00:00.000Z", "liveUrl": nil,
		})
	}
	return rows
}

// TestSubmissionsListTruncatedBoundary pins the cap predicate at BOTH sides of
// the boundary plus the empty case. A `>` or `==` mutation flips one of these.
func TestSubmissionsListTruncatedBoundary(t *testing.T) {
	cases := []struct {
		n    int
		want bool
	}{
		{0, false},
		{1, false},
		{appapi.ListSubmissionsCap - 1, false},
		{appapi.ListSubmissionsCap, true},
		{appapi.ListSubmissionsCap + 1, true},
	}
	for _, tc := range cases {
		if got := submissionsListTruncated(tc.n); got != tc.want {
			t.Errorf("submissionsListTruncated(%d) = %v, want %v", tc.n, got, tc.want)
		}
	}
}

// TestAppStatusCappedListWarns: a full-length page is the only evidence rows were
// dropped, so the CLI must say the list may be incomplete — on stderr, leaving
// the stdout table clean.
func TestAppStatusCappedListWarns(t *testing.T) {
	rows := submissionRows(appapi.ListSubmissionsCap)
	srv := statusServer(t, map[string]any{"submissions": rows}, http.StatusOK, nil)
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, errOut, err := run(t, "app", "status")
	if err != nil {
		t.Fatalf("app status: %v", err)
	}
	if !strings.Contains(errOut, truncationNote) {
		t.Errorf("a %d-row (at-cap) listing must warn that it may be truncated; stderr:\n%s",
			appapi.ListSubmissionsCap, errOut)
	}
	if !strings.Contains(errOut, "civitai app status <blockId>") {
		t.Errorf("the caveat must name the remedy (a per-app lookup); stderr:\n%s", errOut)
	}
	// The note is advisory, not a failure, and must not pollute the table.
	if strings.Contains(out, truncationNote) {
		t.Errorf("the caveat belongs on stderr, not in the stdout table:\n%s", out)
	}
	if !strings.Contains(out, "BLOCK_ID") || !strings.Contains(out, "app-001") {
		t.Errorf("the table must still render in full:\n%s", out)
	}
}

// TestAppStatusShortListDoesNotWarn is the direction people forget: a listing
// that fits under the cap is COMPLETE, and claiming otherwise would train users
// to ignore the caveat. Covers one-under-the-cap and the empty list.
func TestAppStatusShortListDoesNotWarn(t *testing.T) {
	for _, n := range []int{0, 1, appapi.ListSubmissionsCap - 1} {
		srv := statusServer(t, map[string]any{"submissions": submissionRows(n)}, http.StatusOK, nil)
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		t.Setenv("CIVITAI_TOKEN", "tok")
		t.Setenv("CIVITAI_BASE_URL", srv.URL)
		out, errOut, err := run(t, "app", "status")
		srv.Close()
		if err != nil {
			t.Fatalf("%d rows: app status: %v", n, err)
		}
		if strings.Contains(errOut, truncationNote) {
			t.Errorf("%d rows is under the cap — must NOT warn; stderr:\n%s", n, errOut)
		}
		if strings.Contains(out, truncationNote) {
			t.Errorf("%d rows is under the cap — must NOT warn; stdout:\n%s", n, out)
		}
	}
}

// TestAppStatusCappedListJSONStaysPure: --json is the scripted path, so the
// caveat must not enter stdout — the payload has to stay parseable — while the
// human running the pipe still sees the warning on stderr.
func TestAppStatusCappedListJSONStaysPure(t *testing.T) {
	rows := submissionRows(appapi.ListSubmissionsCap)
	srv := statusServer(t, map[string]any{"submissions": rows}, http.StatusOK, nil)
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, errOut, err := run(t, "app", "status", "--json")
	if err != nil {
		t.Fatalf("app status --json: %v", err)
	}
	var parsed struct {
		Submissions []map[string]any `json:"submissions"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("--json stdout must stay parseable JSON: %v\n%s", err, out)
	}
	if len(parsed.Submissions) != appapi.ListSubmissionsCap {
		t.Errorf("--json payload = %d submissions, want %d", len(parsed.Submissions), appapi.ListSubmissionsCap)
	}
	if strings.Contains(out, "note:") {
		t.Errorf("--json stdout must carry no prose:\n%s", out)
	}
	if !strings.Contains(errOut, truncationNote) {
		t.Errorf("--json must still warn on stderr; stderr:\n%s", errOut)
	}
}

// TestAppStatusDetailNeverWarns pins the scope correction: the server applies
// `where.slug` BEFORE the row cap, so a per-app lookup is not affected and must
// not carry the caveat — even when the account is at the cap overall.
func TestAppStatusDetailNeverWarns(t *testing.T) {
	body := map[string]any{
		"submissions": []map[string]any{
			{"id": "pubreq_1", "blockId": "alpha", "version": "0.1.0", "status": "pending",
				"deployState": nil, "submittedAt": "2026-06-17T08:00:00.000Z", "liveUrl": nil},
		},
	}
	srv := statusServer(t, body, http.StatusOK, nil)
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, errOut, err := run(t, "app", "status", "alpha")
	if err != nil {
		t.Fatalf("app status alpha: %v", err)
	}
	if strings.Contains(errOut, truncationNote) || strings.Contains(out, truncationNote) {
		t.Errorf("a per-app lookup is narrowed server-side before the cap — no caveat.\nstdout:\n%s\nstderr:\n%s", out, errOut)
	}
}

func TestAppStatusMissingTokenErrors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "")
	_, _, err := run(t, "app", "status")
	if err == nil {
		t.Fatal("expected error with no token")
	}
	if !strings.Contains(err.Error(), "no token") || !strings.Contains(err.Error(), "civitai login") {
		t.Errorf("missing-token error should point to login: %v", err)
	}
}

func TestAppStatusErrorMapping(t *testing.T) {
	cases := []struct {
		status int
		msg    string
		want   string
	}{
		{http.StatusUnauthorized, "Invalid API key", "civitai login"},
		{http.StatusForbidden, "restricted to the civitai team", "invite-only beta"},
		{http.StatusNotFound, "Submission not found", "no such submission"},
		{http.StatusTooManyRequests, "Rate limit exceeded", "rate limited"},
		{http.StatusServiceUnavailable, "Apps are not enabled", "not enabled"},
	}
	for _, tc := range cases {
		srv := statusServer(t, map[string]string{"message": tc.msg}, tc.status, nil)
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		t.Setenv("CIVITAI_TOKEN", "tok")
		t.Setenv("CIVITAI_BASE_URL", srv.URL)
		_, _, err := run(t, "app", "status")
		srv.Close()
		if err == nil {
			t.Fatalf("status %d: expected error", tc.status)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("status %d: error %q should contain %q", tc.status, err.Error(), tc.want)
		}
	}
}

func TestAppHelpListsStatus(t *testing.T) {
	out, _, err := run(t, "app", "--help")
	if err != nil {
		t.Fatalf("app --help: %v", err)
	}
	if !strings.Contains(out, "status") {
		t.Errorf("app help should list the status subcommand:\n%s", out)
	}
}
