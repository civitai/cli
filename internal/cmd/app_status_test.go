package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
