package civitai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGetForgejoCloneInfoSendsSlugInput pins the tRPC input encoding: the app is
// sent under ?input={"json":{"slug":"<app>"}} and the success envelope
// {result:{data:{json:{...}}}} decodes into ForgejoCloneInfo.
func TestGetForgejoCloneInfoSendsSlugInput(t *testing.T) {
	var gotPath, gotInput, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotInput = r.URL.Query().Get("input")
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{"data": map[string]any{"json": map[string]any{
				"notYetAvailable": false,
				"slug":            "my-block",
				"forgejoUsername": "dev-7",
				"token":           "tok-sha1",
				"httpUrl":         "https://forgejo.example.com/apps/my-block.git",
				"cloneUrl":        "https://dev-7:tok-sha1@forgejo.example.com/apps/my-block.git",
			}}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok123", "")
	info, err := c.GetForgejoCloneInfo(context.Background(), "my-block")
	if err != nil {
		t.Fatalf("GetForgejoCloneInfo: %v", err)
	}
	if gotPath != CloneInfoPath {
		t.Errorf("path = %q, want %q", gotPath, CloneInfoPath)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth header = %q, want Bearer tok123", gotAuth)
	}
	// The slug travels under {"json":{"slug":...}}.
	if !strings.Contains(gotInput, `"slug":"my-block"`) || !strings.Contains(gotInput, `"json"`) {
		t.Errorf("tRPC input should carry {json:{slug}}: %q", gotInput)
	}
	if info.Slug != "my-block" || info.ForgejoUsername != "dev-7" {
		t.Errorf("info = %+v", info)
	}
	if !strings.HasPrefix(info.CloneURL, "https://dev-7:tok-sha1@") {
		t.Errorf("clone URL should embed the basic-auth credential: %q", info.CloneURL)
	}
}

// TestGetForgejoCloneInfoNotYetAvailable: the server returns notYetAvailable=true
// (no credential minted) — the parse must surface the flag + message so the pull
// command can print the friendly "approve your first version" guidance.
func TestGetForgejoCloneInfoNotYetAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{"data": map[string]any{"json": map[string]any{
				"notYetAvailable": true,
				"slug":            "my-block",
				"message":         "approve your first version first",
			}}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	info, err := c.GetForgejoCloneInfo(context.Background(), "my-block")
	if err != nil {
		t.Fatalf("GetForgejoCloneInfo: %v", err)
	}
	if !info.NotYetAvailable || info.Message != "approve your first version first" {
		t.Errorf("info = %+v, want NotYetAvailable with message", info)
	}
	if info.CloneURL != "" {
		t.Errorf("no clone URL should be minted yet: %q", info.CloneURL)
	}
}

func TestGetForgejoCloneInfoRequiresToken(t *testing.T) {
	c := New("https://example.com", "", "")
	if _, err := c.GetForgejoCloneInfo(context.Background(), "my-block"); err == nil {
		t.Fatal("expected error with no token")
	}
}

// TestGetForgejoCloneInfoGarbageResponse: a 200 whose body is not the expected
// tRPC envelope (or a null .json) must be a clear error, not a nil-deref.
func TestGetForgejoCloneInfoGarbageResponse(t *testing.T) {
	for _, body := range []string{
		`not json at all`,
		`{"result":{"data":{"json":null}}}`,
		`{"result":{}}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		c := New(srv.URL, "tok", "")
		_, err := c.GetForgejoCloneInfo(context.Background(), "my-block")
		srv.Close()
		if err == nil {
			t.Errorf("expected error for garbage body %q", body)
		}
	}
}

// TestCloneInfoErrorMapping pins the status->message mapping AND the tRPC-message
// extraction ({error:{json:{message}}}), with a raw-body fallback when the body
// isn't a tRPC error envelope.
func TestCloneInfoErrorMapping(t *testing.T) {
	trpcBody := func(msg string) []byte {
		b, _ := json.Marshal(map[string]any{
			"error": map[string]any{"json": map[string]any{"message": msg, "code": -32600}},
		})
		return b
	}
	cases := []struct {
		name       string
		status     int
		body       []byte
		wantSubstr string // must appear (the actionable prefix)
		wantMsg    string // extracted/echoed server message that must appear
	}{
		{"401 tRPC message", http.StatusUnauthorized, trpcBody("token expired"), "not authenticated", "token expired"},
		{"403 tRPC message", http.StatusForbidden, trpcBody("not the owner"), "not permitted", "not the owner"},
		{"404 tRPC message", http.StatusNotFound, trpcBody("App not found"), "no such app", "App not found"},
		{"500 default", http.StatusInternalServerError, trpcBody("kaboom"), "getMyForgejoCloneInfo failed (HTTP 500)", "kaboom"},
		{"non-tRPC raw fallback", http.StatusForbidden, []byte("plain text denial"), "not permitted", "plain text denial"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := cloneInfoError(tc.status, tc.body)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error %q should contain %q", err, tc.wantSubstr)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error %q should echo the server message %q", err, tc.wantMsg)
			}
		})
	}
}

// TestGetForgejoCloneInfoMapsStatusError confirms a non-200 flows through
// cloneInfoError end to end (403 -> "not permitted").
func TestGetForgejoCloneInfoMapsStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"json": map[string]any{"message": "Apps are not enabled"}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	_, err := c.GetForgejoCloneInfo(context.Background(), "my-block")
	if err == nil {
		t.Fatal("expected error on 403")
	}
	if !strings.Contains(err.Error(), "not permitted") || !strings.Contains(err.Error(), "Apps are not enabled") {
		t.Errorf("403 should map to not-permitted + server message: %v", err)
	}
}
