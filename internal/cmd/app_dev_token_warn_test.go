package cmd

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/ui"
)

// makeJWT builds a syntactically-valid JWT whose payload carries the given
// scopes. The signature segment is a dummy — tokenCanSpend decodes the payload
// WITHOUT verifying the signature (the server verifies on every real call).
func makeJWT(t *testing.T, scopes []string) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload, err := json.Marshal(map[string]any{"scopes": scopes, "sub": "user:1"})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	return header + "." + body + ".sig"
}

func TestTokenCanSpend(t *testing.T) {
	cases := []struct {
		name string
		jwt  string
		want bool
	}{
		{"budgeted", makeJWT(t, []string{"user:read:self", "ai:write:budgeted"}), true},
		{"read-only", makeJWT(t, []string{"user:read:self"}), false},
		{"empty-scopes", makeJWT(t, []string{}), false},
		{"not-a-jwt", "garbage", false},
		{"two-segments", "a.b", false},
		{"bad-base64-payload", "a.!!!.c", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tokenCanSpend(tc.jwt); got != tc.want {
				t.Errorf("tokenCanSpend(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// A read-only (OAuth-minted) token must trigger the loud "can't spend" warning
// on stderr — the early catch for the silent dev:live dead-end.
func TestAppDevTokenReadOnlyWarns(t *testing.T) {
	jwt := makeJWT(t, []string{"user:read:self"})
	srv := devTokenServer(t, map[string]any{"token": jwt}, http.StatusOK, nil)
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, errOut, err := run(t, "app", "dev-token", "my-block")
	if err != nil {
		t.Fatalf("app dev-token: %v", err)
	}
	// The token still goes to stdout cleanly (warning must not pollute it).
	if strings.TrimSpace(out) != jwt {
		t.Errorf("stdout should be exactly the token, got %q", out)
	}
	for _, want := range []string{"READ-ONLY", "ai:write:budgeted", "civitai login --token", "will NOT spend"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr missing %q; got:\n%s", want, errOut)
		}
	}
}

// A spendable (personal-key-minted) token must NOT trigger the warning.
func TestAppDevTokenSpendableNoWarn(t *testing.T) {
	jwt := makeJWT(t, []string{"user:read:self", "ai:write:budgeted"})
	srv := devTokenServer(t, map[string]any{"token": jwt}, http.StatusOK, nil)
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	_, errOut, err := run(t, "app", "dev-token", "my-block")
	if err != nil {
		t.Fatalf("app dev-token: %v", err)
	}
	if strings.Contains(errOut, "READ-ONLY") {
		t.Errorf("spendable token should NOT warn; stderr:\n%s", errOut)
	}
	// The normal paste hint still prints.
	if !strings.Contains(errOut, "VITE_LIVE_BLOCK_TOKEN") {
		t.Errorf("paste hint missing; stderr:\n%s", errOut)
	}
}

// The warning must name the ACTUAL cause, not presume OAuth.
func TestReadOnlyTokenWarning(t *testing.T) {
	cases := []struct {
		name      string
		canSpend  bool
		authKind  string
		wantHas   []string
		wantNotHa []string
	}{
		{
			name:      "credential can spend → blame the manifest",
			canSpend:  true,
			authKind:  config.AuthKindToken,
			wantHas:   []string{"credential CAN spend", "block.manifest.json", "re-mint"},
			wantNotHa: []string{"OAuth", "lacks the AI Services", "civitai login --token"},
		},
		{
			name:      "oauth login → blame OAuth",
			canSpend:  false,
			authKind:  config.AuthKindOAuth,
			wantHas:   []string{"OAuth login", "civitai login --token", "personal API key"},
			wantNotHa: []string{"credential CAN spend", "lacks the AI Services"},
		},
		{
			name:      "personal key without AI Services → blame the key scope",
			canSpend:  false,
			authKind:  config.AuthKindToken,
			wantHas:   []string{"personal API key lacks the AI Services", "civitai login --token"},
			wantNotHa: []string{"OAuth login", "credential CAN spend"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := readOnlyTokenWarning(ui.For(io.Discard), tc.canSpend, tc.authKind, "my-block")
			if !strings.Contains(got, "READ-ONLY") {
				t.Errorf("missing READ-ONLY header; got:\n%s", got)
			}
			for _, w := range tc.wantHas {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q; got:\n%s", w, got)
				}
			}
			for _, w := range tc.wantNotHa {
				if strings.Contains(got, w) {
					t.Errorf("should NOT contain %q; got:\n%s", w, got)
				}
			}
		})
	}
}

// End-to-end: when whoami reports the credential CAN spend but the minted token
// is still read-only, the cause is the manifest — the warning must say so and
// NOT send the dev chasing a credential fix.
func TestAppDevTokenReadOnlyManifestCause(t *testing.T) {
	readOnly := makeJWT(t, []string{"user:read:self"})
	// Path-aware server: /api/v1/me reports a spend-capable credential; the
	// dev-token route returns a read-only token (manifest didn't ask).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/me") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"username": "m", "id": 7, "tokenScope": api.ScopeAIServicesWrite | api.ScopeUserRead,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"token": readOnly})
	}))
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	_, errOut, err := run(t, "app", "dev-token", "my-block")
	if err != nil {
		t.Fatalf("app dev-token: %v", err)
	}
	if !strings.Contains(errOut, "credential CAN spend") || !strings.Contains(errOut, "block.manifest.json") {
		t.Errorf("expected a manifest-cause warning; got:\n%s", errOut)
	}
	if strings.Contains(errOut, "lacks the AI Services") || strings.Contains(errOut, "OAuth login") {
		t.Errorf("must NOT blame the credential when it can spend; got:\n%s", errOut)
	}
}
