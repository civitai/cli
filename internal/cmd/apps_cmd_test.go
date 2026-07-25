package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// setupAuthedAppsServer points the CLI at a local server via CIVITAI_BASE_URL
// AND configures a token (the App-store commands are login-gated client-side,
// so a hermetic anonymous setup like setupReadServer would trip the login guard
// before the request). Returns nothing; the handler asserts what it needs.
func setupAuthedAppsServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	setupReadServer(t, handler)
	// setupReadServer clears the token for anonymous reads; re-set it so the
	// login gate passes and the bearer is sent.
	t.Setenv("CIVITAI_TOKEN", "tok-apps")
}

const appsListCmdBody = `{
  "items": [
    {"id":"a1","slug":"cool-onsite","kind":"onsite","name":"Cool Onsite","tagline":"neat",
     "category":"productivity","contentRating":"pg",
     "creator":{"id":7,"username":"alice","image":""},
     "recommend":{"recommendedCount":9,"notRecommendedCount":1,"recommendPct":0.9},
     "reviewCount":10,"kindData":{"kind":"onsite","appBlockId":"blk_9","hasPage":true}},
    {"id":"a2","slug":"ext-app","kind":"offsite","name":"Ext App","tagline":null,"category":null,
     "contentRating":null,"creator":null,
     "recommend":{"recommendedCount":0,"notRecommendedCount":0,"recommendPct":null},
     "reviewCount":0,"kindData":{"kind":"offsite","subKind":"external-link","externalUrl":"https://ext/x"}}
  ],
  "metadata": {"nextCursor":"cur-2"}
}`

func TestAppListHumanTable(t *testing.T) {
	setupAuthedAppsServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok-apps" {
			t.Errorf("bearer not sent: %q", r.Header.Get("Authorization"))
		}
		// Filter params must be threaded onto the request.
		for k, want := range map[string]string{"kind": "onsite", "sort": "popular", "limit": "10"} {
			if got := r.URL.Query().Get(k); got != want {
				t.Errorf("query %s = %q, want %q", k, got, want)
			}
		}
		_, _ = w.Write([]byte(appsListCmdBody))
	})

	out, _, err := run(t, "app", "list", "--kind", "onsite", "--sort", "popular", "--limit", "10")
	if err != nil {
		t.Fatalf("app list: %v", err)
	}
	// Table headers + rows.
	for _, want := range []string{"NAME", "SLUG", "KIND", "CATEGORY", "AUTHOR", "RATING", "REVIEWS",
		"Cool Onsite", "cool-onsite", "onsite", "alice", "90%", "Ext App", "ext-app", "offsite"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q:\n%s", want, out)
		}
	}
	// The next-cursor footer + paste-ready hint.
	if !strings.Contains(out, "next cursor: cur-2") {
		t.Errorf("footer should show next cursor: %s", out)
	}
	if !strings.Contains(out, "civitai app list --cursor 'cur-2'") {
		t.Errorf("footer should show the next-page hint: %s", out)
	}
}

func TestAppListJSON(t *testing.T) {
	setupAuthedAppsServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(appsListCmdBody))
	})

	out, _, err := run(t, "app", "list", "--json")
	if err != nil {
		t.Fatalf("app list --json: %v", err)
	}
	// --json must emit valid, parseable JSON (the raw body round-tripped).
	var parsed struct {
		Items    []map[string]any `json:"items"`
		Metadata map[string]any   `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\n%s", err, out)
	}
	if len(parsed.Items) != 2 {
		t.Errorf("--json should carry both items, got %d", len(parsed.Items))
	}
	// The human table must NOT be present under --json.
	if strings.Contains(out, "NAME\t") || strings.Contains(out, "next cursor:") {
		t.Errorf("--json output should be pure JSON, got: %s", out)
	}
}

func TestAppListEmptyHint(t *testing.T) {
	setupAuthedAppsServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[],"metadata":{}}`))
	})
	out, _, err := run(t, "app", "list")
	if err != nil {
		t.Fatalf("app list (empty): %v", err)
	}
	if !strings.Contains(out, "No apps found") {
		t.Errorf("empty list should print a hint, got: %s", out)
	}
}

// The login gate: with no token configured, `app list` must fail with a clear
// "run civitai login" error (ErrUnauthorized), never reaching the network.
func TestAppListLoginGated(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "")
	_, _, err := run(t, "app", "list")
	if err == nil {
		t.Fatal("expected a login-required error with no token")
	}
	if !errors.Is(err, civitai.ErrUnauthorized) {
		t.Errorf("no-token error should classify as ErrUnauthorized, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "civitai login") {
		t.Errorf("error should point at `civitai login`, got: %v", err)
	}
}

func TestAppListExplicitZeroLimitRejected(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "tok-apps")
	_, _, err := run(t, "app", "list", "--limit", "0")
	if err == nil {
		t.Fatal("expected an explicit --limit 0 to be rejected")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("bad --limit should be a usage error, got: %v", err)
	}
}

func TestAppViewDetail(t *testing.T) {
	const body = `{
	  "id":"d1","serialId":42,"slug":"cool-onsite","kind":"onsite","name":"Cool Onsite",
	  "tagline":"neat","description":"A longer description.","category":"productivity","contentRating":"pg",
	  "creator":{"id":7,"username":"alice","image":""},
	  "recommend":{"recommendedCount":9,"notRecommendedCount":1,"recommendPct":0.9},"reviewCount":10,
	  "screenshots":[{"url":"https://x/s1.png","caption":"first"}],
	  "kindData":{"kind":"onsite","appBlockId":"blk_9","hasPage":true,"liveUrl":"https://cool-onsite.civit.ai"}
	}`
	setupAuthedAppsServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/cool-onsite" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(body))
	})

	out, _, err := run(t, "app", "view", "cool-onsite")
	if err != nil {
		t.Fatalf("app view: %v", err)
	}
	for _, want := range []string{"Cool Onsite", "cool-onsite", "productivity", "alice",
		"90%", "https://cool-onsite.civit.ai", "A longer description."} {
		if !strings.Contains(out, want) {
			t.Errorf("detail output missing %q:\n%s", want, out)
		}
	}
}

// A 404 from the detail endpoint must render as a clean not-found message
// (classified ErrNotFound), not a stack trace.
func TestAppViewNotFound(t *testing.T) {
	setupAuthedAppsServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"App not found"}`))
	})

	_, _, err := run(t, "app", "view", "ghost")
	if err == nil {
		t.Fatal("expected a not-found error")
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("404 should classify as ErrNotFound, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should read as not-found, got: %v", err)
	}
}

func TestAppViewLoginGated(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "")
	_, _, err := run(t, "app", "view", "any-slug")
	if err == nil {
		t.Fatal("expected a login-required error with no token")
	}
	if !errors.Is(err, civitai.ErrUnauthorized) {
		t.Errorf("no-token error should classify as ErrUnauthorized, got: %v", err)
	}
}

func TestAppHelpListsDiscoveryCommands(t *testing.T) {
	out, _, err := run(t, "app", "--help")
	if err != nil {
		t.Fatalf("app --help: %v", err)
	}
	for _, want := range []string{"list", "view"} {
		if !strings.Contains(out, want) {
			t.Errorf("app help missing discovery subcommand %q:\n%s", want, out)
		}
	}
}
