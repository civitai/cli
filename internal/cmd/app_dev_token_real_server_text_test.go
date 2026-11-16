package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// devTokenNoLiveDeploymentMessage is the owned-but-not-deployed 404 message the
// civitai server's POST /api/v1/blocks/dev-token returns, copied from the
// handler's template literal with the blockId substituted. It is 179 runes plus
// the blockId, so its length depends on the slug.
func devTokenNoLiveDeploymentMessage(blockID string) string {
	return "block '" + blockID + "' has no live deployment — dev:live requires an " +
		"approved + deployed version (deployState: live). Until your first " +
		"version is live, validate locally with dev:harness (mock)."
}

// devTokenOAuthScopeMessage is the server's 403 for an OAuth token that lacks
// the Apps submit scope, copied from the same handler.
const devTokenOAuthScopeMessage = "dev-token needs a personal API key (full scope, create at " +
	"civitai.com/user/account) OR an OAuth login whose token carries the " +
	"Apps submit scope; real Buzz spend additionally needs AI Services scope"

// TestAppDevTokenRealServerTextsAreShownAndNeverRename feeds `app dev-token` the
// server's REAL error messages and pins two things for each: the message reaches
// the developer whole, and block.manifest.json is not rewritten.
//
// 🔴 THE RENAME IS PERMANENT, SO A MISCLASSIFIED 404 COSTS THE APP ITS NAME. Only
// the bare "App not found" 404 (a slug owned by another account) may rename; the
// owned-but-not-deployed 404 is told apart by its "no live deployment" text. When
// that text was read AFTER the display filter, and the filter refused messages
// over 200 runes, a blockId of 22+ characters pushed the real message over the
// limit, the placeholder that replaced it lacked the phrase, and the CLI renamed
// an app the developer owns. The 30-character slug row is that case; the 8-char
// row is the same message under the old limit, so the pair measures both sides
// of it.
func TestAppDevTokenRealServerTextsAreShownAndNeverRename(t *testing.T) {
	const longSlug = "my-really-long-dev-block-name1" // 30 characters
	cases := []struct {
		name    string
		slug    string
		status  int
		message string
	}{
		{"404 no live deployment, 30-char slug", longSlug, http.StatusNotFound, devTokenNoLiveDeploymentMessage(longSlug)},
		{"404 no live deployment, 8-char slug", "my-block", http.StatusNotFound, devTokenNoLiveDeploymentMessage("my-block")},
		{"403 OAuth token without the submit scope", longSlug, http.StatusForbidden, devTokenOAuthScopeMessage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.slug) != 8 && len(tc.slug) != 30 {
				t.Fatalf("PREMISE BROKEN: slug %q is %d chars", tc.slug, len(tc.slug))
			}
			mints := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/dev-token") {
					mints++
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(map[string]any{"message": tc.message})
			}))
			defer srv.Close()

			prev := devTokenSuffixGen
			devTokenSuffixGen = func() string {
				t.Error("the suffix generator ran: this error must never rename the app")
				return "zzzzz"
			}
			defer func() { devTokenSuffixGen = prev }()

			dir := t.TempDir()
			writeDevTokenManifest(t, dir, `{"blockId":"`+tc.slug+`","version":"0.1.0","name":"My Block"}`)
			chdir(t, dir)
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("CIVITAI_TOKEN", "tok")
			t.Setenv("CIVITAI_BASE_URL", srv.URL)

			_, _, err := run(t, "app", "dev-token", tc.slug)
			if err == nil {
				t.Fatal("expected the server's error")
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Errorf("the server's message did not reach the developer whole:\n  want it to contain %q\n  got %v", tc.message, err)
			}
			if got := readBlockID(t, dir); got != tc.slug {
				t.Errorf("block.manifest.json blockId was rewritten to %q; this error must never rename", got)
			}
			if mints != 1 {
				t.Errorf("want exactly 1 mint call (no rename retry), got %d", mints)
			}
		})
	}
}
