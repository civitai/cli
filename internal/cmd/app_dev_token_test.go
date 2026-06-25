package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// devTokenRec records the request the CLI sent to the dev-token endpoint.
type devTokenRec struct {
	auth, method, path, contentType, slug string
}

// devTokenServer stands up an httptest server emulating
// POST /api/v1/blocks/dev-token, returning the canned body+status and recording
// the request (incl. the decoded slug).
func devTokenServer(t *testing.T, body any, status int, rec *devTokenRec) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rec != nil {
			rec.auth = r.Header.Get("Authorization")
			rec.method = r.Method
			rec.path = r.URL.Path
			rec.contentType = r.Header.Get("Content-Type")
			raw, _ := io.ReadAll(r.Body)
			var parsed struct {
				Slug string `json:"slug"`
			}
			_ = json.Unmarshal(raw, &parsed)
			rec.slug = parsed.Slug
		}
		if status != 0 && status != http.StatusOK {
			w.WriteHeader(status)
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func TestAppDevTokenSuccess(t *testing.T) {
	var rec devTokenRec
	srv := devTokenServer(t, map[string]any{"token": "jwt-x", "expiresAt": "soon"}, http.StatusOK, &rec)
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, errOut, err := run(t, "app", "dev-token", "my-block")
	if err != nil {
		t.Fatalf("app dev-token: %v", err)
	}
	// Request shape.
	if rec.auth != "Bearer tok-1" {
		t.Errorf("auth header = %q", rec.auth)
	}
	if rec.method != http.MethodPost {
		t.Errorf("method = %q, want POST", rec.method)
	}
	if rec.path != "/api/v1/blocks/dev-token" {
		t.Errorf("path = %q", rec.path)
	}
	if rec.contentType != "application/json" {
		t.Errorf("content-type = %q, want application/json", rec.contentType)
	}
	if rec.slug != "my-block" {
		t.Errorf("slug body = %q, want my-block", rec.slug)
	}
	// The raw token goes to stdout, alone (pipeable).
	if strings.TrimSpace(out) != "jwt-x" {
		t.Errorf("stdout should be exactly the token, got %q", out)
	}
	// The paste hint goes to stderr, never stdout (so a redirect stays clean).
	if strings.Contains(out, "VITE_LIVE_BLOCK_TOKEN") || strings.Contains(out, "Paste") {
		t.Errorf("stdout should NOT carry the paste hint: %q", out)
	}
	if !strings.Contains(errOut, "VITE_LIVE_BLOCK_TOKEN") || !strings.Contains(errOut, "dev:live") {
		t.Errorf("stderr should carry the paste hint: %q", errOut)
	}
}

func TestAppDevTokenEnvFlag(t *testing.T) {
	srv := devTokenServer(t, map[string]any{"token": "jwt-x"}, http.StatusOK, nil)
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, _, err := run(t, "app", "dev-token", "my-block", "--env")
	if err != nil {
		t.Fatalf("app dev-token --env: %v", err)
	}
	if strings.TrimSpace(out) != "VITE_LIVE_BLOCK_TOKEN=jwt-x" {
		t.Errorf("--env stdout = %q, want VITE_LIVE_BLOCK_TOKEN=jwt-x", out)
	}
}

func TestAppDevTokenMissingSlugErrors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	_, _, err := run(t, "app", "dev-token")
	if err == nil {
		t.Fatal("expected error when no slug is given")
	}
	if !strings.Contains(err.Error(), "slug is required") {
		t.Errorf("missing-slug error should explain: %v", err)
	}
}

func TestAppDevTokenWhitespaceSlugErrors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	_, _, err := run(t, "app", "dev-token", "   ")
	if err == nil {
		t.Fatal("expected error for a whitespace-only slug")
	}
	if !strings.Contains(err.Error(), "slug is required") {
		t.Errorf("whitespace-slug error should explain: %v", err)
	}
}

func TestAppDevTokenMissingTokenErrors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "")
	_, _, err := run(t, "app", "dev-token", "my-block")
	if err == nil {
		t.Fatal("expected error with no token")
	}
	if !strings.Contains(err.Error(), "no token") || !strings.Contains(err.Error(), "civitai login") {
		t.Errorf("missing-token error should point to login: %v", err)
	}
}

func TestAppDevTokenErrorMapping(t *testing.T) {
	// The server returns {"message": ...} on every error status.
	cases := []struct {
		status int
		body   map[string]any
		want   string
	}{
		{http.StatusNotFound, map[string]any{"message": "no such app"}, "app not found (or not yours)"},
		{http.StatusForbidden, map[string]any{"message": "mods only"}, "not authorized"},
		{http.StatusUnauthorized, map[string]any{"message": "bad key"}, "not logged in"},
		{http.StatusTooManyRequests, map[string]any{"message": "slow down"}, "rate limited"},
		{http.StatusServiceUnavailable, map[string]any{"message": "flag off"}, "App Blocks unavailable (503)"},
	}
	for _, tc := range cases {
		srv := devTokenServer(t, tc.body, tc.status, nil)
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		t.Setenv("CIVITAI_TOKEN", "tok")
		t.Setenv("CIVITAI_BASE_URL", srv.URL)
		_, _, err := run(t, "app", "dev-token", "my-block")
		srv.Close()
		if err == nil {
			t.Fatalf("status %d: expected error", tc.status)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("status %d: error %q should contain %q", tc.status, err.Error(), tc.want)
		}
	}
}

// TestAppDevTokenForbiddenNamesPersonalKey asserts the 403 guidance steers the
// user to a full-scope personal API key + whoami (the key DX dead-end).
func TestAppDevTokenForbiddenNamesPersonalKey(t *testing.T) {
	srv := devTokenServer(t, map[string]any{"message": "insufficient scope"}, http.StatusForbidden, nil)
	defer srv.Close()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	_, _, err := run(t, "app", "dev-token", "my-block")
	if err == nil {
		t.Fatal("expected error for 403")
	}
	for _, want := range []string{"personal API key", "civitai whoami", "OAuth"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("403 error %q should mention %q", err.Error(), want)
		}
	}
}

func TestAppHelpListsDevToken(t *testing.T) {
	out, _, err := run(t, "app", "--help")
	if err != nil {
		t.Fatalf("app --help: %v", err)
	}
	if !strings.Contains(out, "dev-token") {
		t.Errorf("app help should list the dev-token subcommand:\n%s", out)
	}
}
