package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// devTokenRec records the request the CLI sent to the dev-token endpoint.
type devTokenRec struct {
	auth, method, path, contentType, slug string
	scopes                                []string
	hasScopesKey                          bool
	// rawBody is the request body EXACTLY as it went on the wire. Assertions
	// about which keys the CLI sends (or omits) have to read this, not a decoded
	// struct — a struct field is indistinguishable between "absent" and "zero".
	rawBody []byte
}

// devTokenServer stands up an httptest server emulating
// POST /api/v1/blocks/dev-token, returning the canned body+status and recording
// the request (incl. the decoded slug). It also answers GET /api/v1/me with a
// minimal non-spendable identity — the dev-token command calls WhoAmI in the
// read-only warning path, and that call must neither error nor pollute `rec`.
func devTokenServer(t *testing.T, body any, status int, rec *devTokenRec) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/me") {
			_ = json.NewEncoder(w).Encode(map[string]any{"username": "u", "id": 1, "tokenScope": 0})
			return
		}
		if rec != nil {
			rec.auth = r.Header.Get("Authorization")
			rec.method = r.Method
			rec.path = r.URL.Path
			rec.contentType = r.Header.Get("Content-Type")
			raw, _ := io.ReadAll(r.Body)
			rec.rawBody = raw
			var parsed struct {
				Slug   string   `json:"slug"`
				Scopes []string `json:"scopes"`
			}
			_ = json.Unmarshal(raw, &parsed)
			rec.slug = parsed.Slug
			rec.scopes = parsed.Scopes
			var generic map[string]any
			_ = json.Unmarshal(raw, &generic)
			_, rec.hasScopesKey = generic["scopes"]
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
		{http.StatusNotFound, map[string]any{"message": "no such app"}, "registered to a different account"},
		{http.StatusForbidden, map[string]any{"message": "mods only"}, "not authorized"},
		{http.StatusUnauthorized, map[string]any{"message": "bad key"}, "not logged in"},
		{http.StatusTooManyRequests, map[string]any{"message": "slow down"}, "rate limited"},
		{http.StatusServiceUnavailable, map[string]any{"message": "flag off"}, "apps unavailable (503)"},
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

// TestAppDevTokenForbiddenNamesBothSpendRoutes asserts the 403 guidance steers
// the user to a spend-capable credential + whoami (the key DX dead-end).
//
// It requires BOTH routes to be named. Asserting only "personal API key" is what
// let the message keep saying a personal key was the ONLY way long after
// `civitai login --scopes generate` shipped: the assertion was still true, and
// the guidance was still wrong.
func TestAppDevTokenForbiddenNamesBothSpendRoutes(t *testing.T) {
	srv := devTokenServer(t, map[string]any{"message": "insufficient scope"}, http.StatusForbidden, nil)
	defer srv.Close()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	_, _, err := run(t, "app", "dev-token", "my-block")
	if err == nil {
		t.Fatal("expected error for 403")
	}
	for _, want := range []string{"personal API key", "civitai login --scopes generate", "civitai whoami", "OAuth"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("403 error %q should mention %q", err.Error(), want)
		}
	}
}

// writeDevTokenManifest writes a block.manifest.json with the given raw body
// into dir.
func writeDevTokenManifest(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestAppDevTokenSendsManifestScopes: a block.manifest.json with `scopes` in the
// current working directory is read and its scopes are POSTed in the body — this
// is the no-row local-manifest mint path.
func TestAppDevTokenSendsManifestScopes(t *testing.T) {
	var rec devTokenRec
	srv := devTokenServer(t, map[string]any{"token": "jwt-x"}, http.StatusOK, &rec)
	defer srv.Close()

	dir := t.TempDir()
	writeDevTokenManifest(t, dir, `{
  "blockId": "my-block",
  "version": "0.1.0",
  "name": "My Block",
  "scopes": ["ai:write:budgeted", "identity:read"]
}`)
	chdir(t, dir)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	if _, _, err := run(t, "app", "dev-token", "my-block"); err != nil {
		t.Fatalf("app dev-token: %v", err)
	}
	if rec.slug != "my-block" {
		t.Errorf("slug = %q, want my-block", rec.slug)
	}
	want := []string{"ai:write:budgeted", "identity:read"}
	if !reflect.DeepEqual(rec.scopes, want) {
		t.Errorf("scopes = %v, want %v", rec.scopes, want)
	}
}

// TestAppDevTokenNoManifestSendsNoScopes: with no manifest in cwd the body has
// no `scopes` key (just the slug) — the slug still identifies a registered app.
func TestAppDevTokenNoManifestSendsNoScopes(t *testing.T) {
	var rec devTokenRec
	srv := devTokenServer(t, map[string]any{"token": "jwt-x"}, http.StatusOK, &rec)
	defer srv.Close()

	chdir(t, t.TempDir()) // empty dir, no manifest

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	if _, _, err := run(t, "app", "dev-token", "my-block"); err != nil {
		t.Fatalf("app dev-token: %v", err)
	}
	if rec.slug != "my-block" {
		t.Errorf("slug = %q, want my-block", rec.slug)
	}
	if rec.hasScopesKey {
		t.Errorf("body should carry NO scopes key without a manifest, got %v", rec.scopes)
	}
}

// TestAppDevTokenMalformedManifestDegrades: a malformed manifest must not crash
// or fail the command — it degrades to sending no scopes (still mints for a
// registered app / read-only token).
func TestAppDevTokenMalformedManifestDegrades(t *testing.T) {
	var rec devTokenRec
	srv := devTokenServer(t, map[string]any{"token": "jwt-x"}, http.StatusOK, &rec)
	defer srv.Close()

	dir := t.TempDir()
	writeDevTokenManifest(t, dir, `{ this is not valid json `)
	chdir(t, dir)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	if _, _, err := run(t, "app", "dev-token", "my-block"); err != nil {
		t.Fatalf("malformed manifest should degrade, not fail: %v", err)
	}
	if rec.hasScopesKey {
		t.Errorf("malformed manifest should send NO scopes, got %v", rec.scopes)
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
