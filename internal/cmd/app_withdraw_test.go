package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// withdrawRec records the request the CLI sent to the withdraw endpoint.
type withdrawRec struct {
	auth, method, path, publishRequestID string
}

// withdrawServer stands up an httptest server emulating
// POST /api/v1/blocks/withdraw, returning the canned body+status and recording
// the request (incl. the decoded publishRequestId).
func withdrawServer(t *testing.T, body any, status int, rec *withdrawRec) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rec != nil {
			rec.auth = r.Header.Get("Authorization")
			rec.method = r.Method
			rec.path = r.URL.Path
			raw, _ := io.ReadAll(r.Body)
			var parsed struct {
				PublishRequestID string `json:"publishRequestId"`
			}
			_ = json.Unmarshal(raw, &parsed)
			rec.publishRequestID = parsed.PublishRequestID
		}
		if status != 0 && status != http.StatusOK {
			w.WriteHeader(status)
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func TestAppWithdrawSuccess(t *testing.T) {
	var rec withdrawRec
	srv := withdrawServer(t, map[string]any{"ok": true}, http.StatusOK, &rec)
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, _, err := run(t, "app", "withdraw", "pubreq_01H")
	if err != nil {
		t.Fatalf("app withdraw: %v", err)
	}
	if rec.auth != "Bearer tok-1" {
		t.Errorf("auth header = %q", rec.auth)
	}
	if rec.method != http.MethodPost {
		t.Errorf("method = %q, want POST", rec.method)
	}
	if rec.path != "/api/v1/blocks/withdraw" {
		t.Errorf("path = %q", rec.path)
	}
	if rec.publishRequestID != "pubreq_01H" {
		t.Errorf("publishRequestId body = %q, want pubreq_01H", rec.publishRequestID)
	}
	if !strings.Contains(out, "Withdrew") || !strings.Contains(out, "pubreq_01H") {
		t.Errorf("success output missing the withdrew line: %q", out)
	}
}

func TestAppWithdrawByIDFlag(t *testing.T) {
	var rec withdrawRec
	srv := withdrawServer(t, map[string]any{"ok": true}, http.StatusOK, &rec)
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	if _, _, err := run(t, "app", "withdraw", "--id", "pubreq_flag"); err != nil {
		t.Fatalf("app withdraw --id: %v", err)
	}
	if rec.publishRequestID != "pubreq_flag" {
		t.Errorf("publishRequestId body = %q, want pubreq_flag", rec.publishRequestID)
	}
}

func TestAppWithdrawMissingIDErrors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	_, _, err := run(t, "app", "withdraw")
	if err == nil {
		t.Fatal("expected error when no publish-request id is given")
	}
	if !strings.Contains(err.Error(), "publish-request id is required") {
		t.Errorf("missing-id error should explain: %v", err)
	}
}

func TestAppWithdrawMissingTokenErrors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "")
	_, _, err := run(t, "app", "withdraw", "pubreq_01H")
	if err == nil {
		t.Fatal("expected error with no token")
	}
	if !strings.Contains(err.Error(), "no token") || !strings.Contains(err.Error(), "civitai login") {
		t.Errorf("missing-token error should point to login: %v", err)
	}
}

func TestAppWithdrawErrorMapping(t *testing.T) {
	cases := []struct {
		status int
		body   map[string]any
		want   string
	}{
		{http.StatusNotFound, map[string]any{"error": "not found"}, "not found (or not yours)"},
		{http.StatusConflict, map[string]any{"error": "request is already approved"}, "request is already approved"},
		{http.StatusUnauthorized, map[string]any{"error": "invalid key"}, "not authorized"},
		{http.StatusForbidden, map[string]any{"error": "no access"}, "not authorized"},
		{http.StatusTooManyRequests, map[string]any{"error": "slow down"}, "rate limited"},
	}
	for _, tc := range cases {
		srv := withdrawServer(t, tc.body, tc.status, nil)
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		t.Setenv("CIVITAI_TOKEN", "tok")
		t.Setenv("CIVITAI_BASE_URL", srv.URL)
		_, _, err := run(t, "app", "withdraw", "pubreq_01H")
		srv.Close()
		if err == nil {
			t.Fatalf("status %d: expected error", tc.status)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("status %d: error %q should contain %q", tc.status, err.Error(), tc.want)
		}
	}
}

func TestAppHelpListsWithdraw(t *testing.T) {
	out, _, err := run(t, "app", "--help")
	if err != nil {
		t.Fatalf("app --help: %v", err)
	}
	if !strings.Contains(out, "withdraw") {
		t.Errorf("app help should list the withdraw subcommand:\n%s", out)
	}
}
