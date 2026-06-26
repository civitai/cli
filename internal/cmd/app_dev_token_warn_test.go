package cmd

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
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
