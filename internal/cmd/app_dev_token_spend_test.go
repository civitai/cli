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

	"github.com/civitai/cli/internal/ui"
)

// writeManifestWithScopes writes a block.manifest.json carrying scopes into a
// temp dir and chdirs there (dev-token reads the CWD).
func writeManifestWithScopes(t *testing.T, scopesJSON string) {
	t.Helper()
	dir := t.TempDir()
	body := `{"blockId":"my-block","version":"1.0.0","name":"My Block"`
	if scopesJSON != "" {
		body += `,"scopes":` + scopesJSON
	}
	body += "}"
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
}

// TestDevTokenRequestScopes is the pure-function contract behind --spend.
//
// The DEFAULT rows are the regression guard that matters most: they pin that
// omitting --spend leaves the request BYTE-IDENTICAL to today's, so a full-scope
// personal API key with no local manifest keeps getting budgeted spend from the
// server. Narrowing by default would silently take that away.
func TestDevTokenRequestScopes(t *testing.T) {
	cases := []struct {
		name      string
		manifest  []string
		spend     bool
		want      []string
		wantIsNil bool
	}{
		{
			name:     "default + no manifest → nothing sent (server decides, as today)",
			manifest: nil, spend: false, want: nil, wantIsNil: true,
		},
		{
			name:     "default + manifest → verbatim, NOT narrowed",
			manifest: []string{"user:read:self", "ai:write:budgeted", "apps:storage:read"},
			spend:    false,
			want:     []string{"user:read:self", "ai:write:budgeted", "apps:storage:read"},
		},
		{
			name:     "default + manifest WITHOUT spend → still verbatim (no scope invented)",
			manifest: []string{"user:read:self"}, spend: false,
			want: []string{"user:read:self"},
		},
		{
			name:     "--spend + manifest without it → unioned, manifest order preserved",
			manifest: []string{"user:read:self", "apps:storage:read"}, spend: true,
			want: []string{"user:read:self", "apps:storage:read", "ai:write:budgeted"},
		},
		{
			name:     "--spend + manifest that already declares it → unchanged, no duplicate",
			manifest: []string{"user:read:self", "ai:write:budgeted"}, spend: true,
			want: []string{"user:read:self", "ai:write:budgeted"},
		},
		{
			name:     "--spend + no manifest → the single explicit request",
			manifest: nil, spend: true,
			want: []string{"ai:write:budgeted"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Copy the input so an aliasing bug in the implementation is visible.
			in := append([]string(nil), tc.manifest...)
			if tc.manifest == nil {
				in = nil
			}
			got := devTokenRequestScopes(in, tc.spend)
			if tc.wantIsNil && got != nil {
				t.Fatalf("want nil (so the key is OMITTED from the body), got %#v", got)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("devTokenRequestScopes(%#v, spend=%v) = %#v, want %#v", tc.manifest, tc.spend, got, tc.want)
			}
			// The caller's slice must not have been mutated in place.
			if !reflect.DeepEqual(in, tc.manifest) && !(in == nil && tc.manifest == nil) {
				t.Errorf("input slice was mutated: %#v (was %#v)", in, tc.manifest)
			}
		})
	}
}

// TestSpendNarrowsToBudgetedOnly pins when the narrowing notice fires.
func TestSpendNarrowsToBudgetedOnly(t *testing.T) {
	if !spendNarrowsToBudgetedOnly(nil, true) {
		t.Error("--spend with no manifest scopes IS a narrowing to one scope")
	}
	if spendNarrowsToBudgetedOnly([]string{"user:read:self"}, true) {
		t.Error("--spend alongside manifest scopes is not a narrowing to one scope")
	}
	if spendNarrowsToBudgetedOnly(nil, false) {
		t.Error("without --spend nothing is narrowed")
	}
	notice := spendNarrowingNotice(ui.For(io.Discard))
	for _, want := range []string{"ai:write:budgeted", "ALONE", "block.manifest.json"} {
		if !strings.Contains(notice, want) {
			t.Errorf("narrowing notice missing %q; got:\n%s", want, notice)
		}
	}
}

// TestAppDevTokenSpendSendsBudgetedScope asserts the EXACT `scopes` array on the
// outgoing request body for the opted-in path.
func TestAppDevTokenSpendSendsBudgetedScope(t *testing.T) {
	writeManifestWithScopes(t, `["user:read:self","apps:storage:read"]`)

	var rec devTokenRec
	srv := devTokenServer(t, map[string]any{"token": "jwt-x"}, http.StatusOK, &rec)
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	if _, _, err := run(t, "app", "dev-token", "my-block", "--spend"); err != nil {
		t.Fatalf("app dev-token --spend: %v", err)
	}
	want := []string{"user:read:self", "apps:storage:read", "ai:write:budgeted"}
	if !reflect.DeepEqual(rec.scopes, want) {
		t.Errorf("--spend sent scopes = %#v, want %#v (raw body: %s)", rec.scopes, want, rec.rawBody)
	}
}

// TestAppDevTokenDefaultDoesNotNarrow is the behaviour-change guard for the
// item-3 decision: WITHOUT --spend the body is exactly the manifest's scopes.
// Nothing is added and — critically — nothing is stripped.
func TestAppDevTokenDefaultDoesNotNarrow(t *testing.T) {
	writeManifestWithScopes(t, `["user:read:self","ai:write:budgeted"]`)

	var rec devTokenRec
	srv := devTokenServer(t, map[string]any{"token": "jwt-x"}, http.StatusOK, &rec)
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	if _, _, err := run(t, "app", "dev-token", "my-block"); err != nil {
		t.Fatalf("app dev-token: %v", err)
	}
	want := []string{"user:read:self", "ai:write:budgeted"}
	if !reflect.DeepEqual(rec.scopes, want) {
		t.Errorf("default sent scopes = %#v, want the manifest verbatim %#v", rec.scopes, want)
	}
}

// TestAppDevTokenDefaultNoManifestOmitsScopesKey is the FULL-KEY BLAST-RADIUS
// guard. A user with a full-scope personal API key and no local manifest gets
// budgeted spend today because the body carries NO `scopes` key and the server
// resolves spend from the bearer. If a future change starts narrowing by
// default, this goes red — it is the assertion that pins "we did not take spend
// away from the people it works for".
func TestAppDevTokenDefaultNoManifestOmitsScopesKey(t *testing.T) {
	t.Chdir(t.TempDir()) // no block.manifest.json

	var rec devTokenRec
	srv := devTokenServer(t, map[string]any{"token": "jwt-x"}, http.StatusOK, &rec)
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	if _, _, err := run(t, "app", "dev-token", "my-block"); err != nil {
		t.Fatalf("app dev-token: %v", err)
	}
	if rec.hasScopesKey {
		t.Errorf("default + no manifest must send NO scopes key (server decides); raw body: %s", rec.rawBody)
	}
	// Read the raw bytes too — a decoded struct cannot distinguish absent from empty.
	var generic map[string]any
	_ = json.Unmarshal(rec.rawBody, &generic)
	if _, ok := generic["scopes"]; ok {
		t.Errorf("raw body carries a scopes key: %s", rec.rawBody)
	}
}

// TestAppDevTokenSpendNoManifestNarrowsAndSaysSo: --spend with nothing to union
// into DOES narrow the request to one scope. That is a real side effect, so the
// command must both send it and warn.
func TestAppDevTokenSpendNoManifestNarrowsAndSaysSo(t *testing.T) {
	t.Chdir(t.TempDir())

	var rec devTokenRec
	srv := devTokenServer(t, map[string]any{"token": "jwt-x"}, http.StatusOK, &rec)
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	_, errOut, err := run(t, "app", "dev-token", "my-block", "--spend")
	if err != nil {
		t.Fatalf("app dev-token --spend: %v", err)
	}
	if want := []string{"ai:write:budgeted"}; !reflect.DeepEqual(rec.scopes, want) {
		t.Errorf("scopes = %#v, want %#v", rec.scopes, want)
	}
	if !strings.Contains(errOut, "narrows the token to ai:write:budgeted ALONE") {
		t.Errorf("expected the narrowing notice on stderr; got:\n%s", errOut)
	}
}

// TestAppDevTokenNoSpendFlagPrintsNoNarrowingNotice is the negative control on
// that notice: it must not appear on a normal mint, or it becomes noise nobody
// reads.
func TestAppDevTokenNoSpendFlagPrintsNoNarrowingNotice(t *testing.T) {
	t.Chdir(t.TempDir())

	var rec devTokenRec
	srv := devTokenServer(t, map[string]any{"token": "jwt-x"}, http.StatusOK, &rec)
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	_, errOut, err := run(t, "app", "dev-token", "my-block")
	if err != nil {
		t.Fatalf("app dev-token: %v", err)
	}
	if strings.Contains(errOut, "narrows the token") {
		t.Errorf("no narrowing notice should print without --spend; got:\n%s", errOut)
	}
}

// TestReadOnlyTokenWarningPointsAtTheNewFixes: once an OAuth login CAN carry
// AIServicesWrite, the read-only warning must offer `login --scopes generate`
// rather than asserting OAuth can never spend; and the manifest-cause branch
// must offer --spend.
func TestReadOnlyTokenWarningPointsAtTheNewFixes(t *testing.T) {
	oauth := readOnlyTokenWarning(ui.For(io.Discard), false, "oauth", "my-block")
	if !strings.Contains(oauth, "civitai login --scopes generate") {
		t.Errorf("OAuth read-only warning must offer --scopes generate; got:\n%s", oauth)
	}
	if strings.Contains(oauth, "which can't spend Buzz.") {
		t.Errorf("warning must not assert OAuth can NEVER spend; got:\n%s", oauth)
	}

	canSpend := readOnlyTokenWarning(ui.For(io.Discard), true, "token", "my-block")
	if !strings.Contains(canSpend, "--spend") {
		t.Errorf("the credential-can-spend branch must offer --spend; got:\n%s", canSpend)
	}
}

// TestAppDevTokenSpendHelpNamesTheConsequence: --help must say --spend means
// REAL Buzz, and that omitting it changes nothing today.
func TestAppDevTokenSpendHelpNamesTheConsequence(t *testing.T) {
	out, _, err := run(t, "app", "dev-token", "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	for _, want := range []string{"--spend", "ai:write:budgeted", "REAL Buzz"} {
		if !strings.Contains(out, want) {
			t.Errorf("dev-token --help missing %q:\n%s", want, out)
		}
	}
}

// TestBuzzScopeHintNamesBothRoutes: BuzzRead ships inside the generate set, so
// the "you can't read your balance" hint must no longer claim a personal API key
// is the only fix.
func TestBuzzScopeHintNamesBothRoutes(t *testing.T) {
	for _, want := range []string{
		"civitai login --token <key>",
		"civitai login --scopes generate",
		"personal API key",
	} {
		if !strings.Contains(buzzScopeHint, want) {
			t.Errorf("buzz scope hint missing %q; got:\n%s", want, buzzScopeHint)
		}
	}
	// The stale absolute — "OAuth login tokens can't read balance or spend" — is
	// only true of a DEFAULT login now.
	if strings.Contains(buzzScopeHint, "OAuth login tokens (`civitai login`) can't read balance or spend Buzz.") {
		t.Errorf("stale unconditional OAuth claim still present:\n%s", buzzScopeHint)
	}
}

// whoamiSrv answers /api/v1/me with the given tokenScope.
func whoamiSrv(t *testing.T, tokenScope int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"username": "zach", "id": 1, "tokenScope": tokenScope})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestWhoAmISpendGuidanceForOAuth: an OAuth login that can't spend is now
// FIXABLE by re-logging in, so the guidance must lead with that instead of
// sending the user to the web UI (which was the only route before).
func TestWhoAmISpendGuidanceForOAuth(t *testing.T) {
	// UserRead|AppBlocksSubmit|AppBlocksDevTunnel = 100663297: a default login.
	srv := whoamiSrv(t, 100663297)
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	writeOAuthConfig(t, dir)

	out, _, err := run(t, "whoami")
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	if !strings.Contains(out, "Spend Buzz (AI Services): no") {
		t.Fatalf("precondition: this credential must read as non-spending:\n%s", out)
	}
	if !strings.Contains(out, "civitai login --scopes generate") {
		t.Errorf("OAuth guidance must offer --scopes generate:\n%s", out)
	}
	// The old text asserted a personal key was the ONLY way. It must be gone.
	if strings.Contains(out, "needs a\nfull-scope personal API key") {
		t.Errorf("stale 'only a personal key' guidance still present:\n%s", out)
	}
}

// TestWhoAmISpendGuidanceForPersonalKey: a personal key's scopes are fixed when
// it is minted, so that user must create a NEW key — lead with that.
func TestWhoAmISpendGuidanceForPersonalKey(t *testing.T) {
	srv := whoamiSrv(t, 1) // UserRead only
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "personal-key")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, _, err := run(t, "whoami")
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	if !strings.Contains(out, "can't spend Buzz") {
		t.Fatalf("precondition: the non-spend guidance must show:\n%s", out)
	}
	idxKey := strings.Index(out, "civitai login --token")
	idxScopes := strings.Index(out, "civitai login --scopes generate")
	if idxKey < 0 || idxScopes < 0 {
		t.Fatalf("both routes should be offered:\n%s", out)
	}
	if idxKey > idxScopes {
		t.Errorf("for a personal key the KEY route must be listed first:\n%s", out)
	}
}

// writeOAuthConfig writes a config.yaml that looks like a stored OAuth login so
// cfg.AuthKind() reports oauth.
func writeOAuthConfig(t *testing.T, xdgHome string) {
	t.Helper()
	dir := filepath.Join(xdgHome, "civitai")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := "access_token: at-1\nrefresh_token: rt-1\nauth_kind: oauth\nscope: \"100663297\"\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
