package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"gopkg.in/yaml.v3"
)

// The two literal masks, spelled here so the assertions do not read their
// expectation out of the implementation they test.
const (
	wantDefaultScope  = "100663297"
	wantGenerateScope = "100777985"
)

// loginScopeSrv stands up a full device-flow server that RECORDS the scope the
// CLI actually put on the device-init form, and counts device-init hits so a
// test can prove no network call happened. initStatus/initBody override the
// device-init response when initStatus != 0.
type loginScopeSrv struct {
	*httptest.Server
	initForm  url.Values
	initHits  int32
	echoScope string // what the device-token response echoes back as `scope`
}

func newLoginScopeSrv(t *testing.T, initStatus int, initBody any) *loginScopeSrv {
	t.Helper()
	s := &loginScopeSrv{echoScope: wantDefaultScope}
	var base string
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeLoginDiscovery(w, base)
		case "/api/auth/oauth/device":
			atomic.AddInt32(&s.initHits, 1)
			_ = r.ParseForm()
			s.initForm = r.PostForm
			if initStatus != 0 {
				w.WriteHeader(initStatus)
				_ = json.NewEncoder(w).Encode(initBody)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code":               "dev-secret-code",
				"user_code":                 "ABCD-1234",
				"verification_uri":          "https://civitai.com/oauth/device",
				"verification_uri_complete": "https://civitai.com/oauth/device?user_code=ABCD-1234",
				"expires_in":                900,
				"interval":                  1,
			})
		case "/api/auth/oauth/device-token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "access-123", "token_type": "Bearer",
				"expires_in": 3600, "refresh_token": "refresh-456", "scope": s.echoScope,
			})
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(s.Close)
	base = s.URL
	return s
}

// loginEnv points the CLI at srv with a fresh config dir and returns that dir.
func loginEnv(t *testing.T, srvURL string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_BASE_URL", srvURL)
	return dir
}

// TestLoginDefaultRequestsUnwidenedScope is the load-bearing regression guard:
// a plain `civitai login` must keep requesting EXACTLY 100663297. The whole
// point of a named opt-in is that the default does not silently gain Buzz-spend
// authority, and this asserts it on the WIRE, not on a constant.
func TestLoginDefaultRequestsUnwidenedScope(t *testing.T) {
	srv := newLoginScopeSrv(t, 0, nil)
	loginEnv(t, srv.URL)

	out, _, err := run(t, "login", "--no-browser")
	if err != nil {
		t.Fatalf("login: %v\n%s", err, out)
	}
	if got := srv.initForm.Get("scope"); got != wantDefaultScope {
		t.Errorf("default login sent scope=%q, want %q", got, wantDefaultScope)
	}
	// And it says nothing about spending — no scary warning on a plain login.
	if strings.Contains(out, "scope set") {
		t.Errorf("a default login must not announce a scope set; got:\n%s", out)
	}
}

// TestLoginScopesGenerateSendsWidenedScope: --scopes generate must put 100777985
// on the device-init form (assert the OUTGOING BODY, not the resolver).
func TestLoginScopesGenerateSendsWidenedScope(t *testing.T) {
	srv := newLoginScopeSrv(t, 0, nil)
	srv.echoScope = wantGenerateScope
	dir := loginEnv(t, srv.URL)

	out, _, err := run(t, "login", "--scopes", "generate", "--no-browser")
	if err != nil {
		t.Fatalf("login --scopes generate: %v\n%s", err, out)
	}
	if got := srv.initForm.Get("scope"); got != wantGenerateScope {
		t.Errorf("--scopes generate sent scope=%q, want %q", got, wantGenerateScope)
	}
	// The consequence is surfaced at the point of use, in the terminal.
	if !strings.Contains(out, "generate") || !strings.Contains(strings.ToUpper(out), "SPEND") {
		t.Errorf("login output must name the generate set AND that it grants Buzz spend; got:\n%s", out)
	}
	// The granted scope is persisted.
	raw, _ := os.ReadFile(filepath.Join(dir, "civitai", "config.yaml"))
	var onDisk map[string]any
	_ = yaml.Unmarshal(raw, &onDisk)
	if onDisk["scope"] != wantGenerateScope {
		t.Errorf("persisted scope = %v, want %v", onDisk["scope"], wantGenerateScope)
	}
}

// TestLoginScopesEqualsFormSendsWidenedScope: the `--scopes=generate` spelling
// is equivalent. Worth its own case because the space form and the equals form
// take different pflag paths, and the equals form is the one unaffected by
// SetInterspersed(false).
func TestLoginScopesEqualsFormSendsWidenedScope(t *testing.T) {
	srv := newLoginScopeSrv(t, 0, nil)
	loginEnv(t, srv.URL)

	if _, _, err := run(t, "login", "--scopes=generate", "--no-browser"); err != nil {
		t.Fatalf("login --scopes=generate: %v", err)
	}
	if got := srv.initForm.Get("scope"); got != wantGenerateScope {
		t.Errorf("--scopes=generate sent scope=%q, want %q", got, wantGenerateScope)
	}
}

// TestLoginUnknownScopeSetFailsBeforeAnyNetworkCall: a typo must error with the
// valid names AND must not have started a device flow (which would leave the
// user staring at a code that will never be approved).
func TestLoginUnknownScopeSetFailsBeforeAnyNetworkCall(t *testing.T) {
	srv := newLoginScopeSrv(t, 0, nil)
	loginEnv(t, srv.URL)

	_, _, err := run(t, "login", "--scopes", "generte", "--no-browser")
	if err == nil {
		t.Fatal("an unknown scope set must fail")
	}
	if !strings.Contains(err.Error(), "generte") || !strings.Contains(err.Error(), "generate") {
		t.Errorf("error must echo the bad name and list the valid sets; got %q", err)
	}
	if n := atomic.LoadInt32(&srv.initHits); n != 0 {
		t.Errorf("device init was called %d times — validation must happen BEFORE the network", n)
	}
}

// TestLoginInvalidScopeFromServerIsActionable is the ORDERING TRAP, end to end:
// `--scopes generate` against a server whose civitai-cli allowedScopes has not
// been widened yet answers invalid_scope, and the user must be told that plain
// `civitai login` still works.
func TestLoginInvalidScopeFromServerIsActionable(t *testing.T) {
	srv := newLoginScopeSrv(t, http.StatusBadRequest, map[string]string{"error": "invalid_scope"})
	dir := loginEnv(t, srv.URL)

	_, _, err := run(t, "login", "--scopes", "generate", "--no-browser")
	if err == nil {
		t.Fatal("invalid_scope must surface as an error")
	}
	msg := err.Error()
	for _, want := range []string{"invalid_scope", "civitai login", "--scopes generate"} {
		if !strings.Contains(msg, want) {
			t.Errorf("invalid_scope message missing %q; got:\n%s", want, msg)
		}
	}
	// Nothing was stored — a failed login must not half-write a credential.
	if _, statErr := os.Stat(filepath.Join(dir, "civitai", "config.yaml")); statErr == nil {
		raw, _ := os.ReadFile(filepath.Join(dir, "civitai", "config.yaml"))
		if strings.Contains(string(raw), "access_token") {
			t.Errorf("a rejected login must not persist tokens; config:\n%s", raw)
		}
	}
}

// TestLoginScopesFlagParsingMatrix walks the documented SetInterspersed(false)
// scheme with --scopes added, including the stray-positional shapes the --token
// comment block warns about. The contract each case pins:
//
//   - --scopes never creates a positional (it takes a value), so it can never be
//     confused with the `--token <value>` positional-recovery form.
//   - anything that halts flag parsing before --scopes is REJECTED, not silently
//     run with the default scope.
//   - --scopes with --token is rejected outright (a personal key's scopes are
//     fixed at creation).
func TestLoginScopesFlagParsingMatrix(t *testing.T) {
	cases := []struct {
		name string
		args []string
		// wantErr: the invocation must fail.
		wantErr bool
		// wantErrHas: substring the error must contain (when wantErr).
		wantErrHas string
		// wantScope: the scope that must reach the wire (when !wantErr).
		wantScope string
		// wantStoredToken: the personal key that must be stored (empty = none).
		wantStoredToken string
	}{
		{
			name: "no flags → default scope", args: []string{"login", "--no-browser"},
			wantScope: wantDefaultScope,
		},
		{
			name: "--scopes generate → widened", args: []string{"login", "--scopes", "generate", "--no-browser"},
			wantScope: wantGenerateScope,
		},
		{
			name: "--scopes before --no-browser and after", args: []string{"login", "--no-browser", "--scopes", "generate"},
			wantScope: wantGenerateScope,
		},
		{
			name: "empty --scopes value is the default, not an error", args: []string{"login", "--scopes", "", "--no-browser"},
			wantScope: wantDefaultScope,
		},
		{
			name: "unknown set", args: []string{"login", "--scopes", "bogus", "--no-browser"},
			wantErr: true, wantErrHas: "valid scope sets",
		},
		{
			// --scopes FIRST, then the --token space form: --scopes is consumed as
			// a flag, `k` is the single allowed positional, recovered as the token.
			name:    "--scopes then --token <value> → rejected as a combination",
			args:    []string{"login", "--scopes", "generate", "--token", "k"},
			wantErr: true, wantErrHas: "cannot be combined with --token",
		},
		{
			name:    "--scopes with --token=<value> → rejected as a combination",
			args:    []string{"login", "--scopes", "generate", "--token=k"},
			wantErr: true, wantErrHas: "cannot be combined with --token",
		},
		{
			// THE FOOTGUN CASE. `k` is a positional, which halts flag parsing under
			// SetInterspersed(false) — so `--scopes generate` arrive as extra
			// positionals, --scopes is never Changed, and the Args guard rejects the
			// lot. It must NOT quietly store `k` while dropping the scope request.
			name:    "--token <value> then --scopes → rejected, nothing stored",
			args:    []string{"login", "--token", "k", "--scopes", "generate"},
			wantErr: true,
		},
		{
			// A stray positional BEFORE --scopes halts parsing the same way.
			name:    "stray positional then --scopes → rejected",
			args:    []string{"login", "stray", "--scopes", "generate"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newLoginScopeSrv(t, 0, nil)
			dir := loginEnv(t, srv.URL)

			_, _, err := run(t, tc.args...)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("args %v: want an error, got none (scope sent = %q)", tc.args, srv.initForm.Get("scope"))
				}
				if tc.wantErrHas != "" && !strings.Contains(err.Error(), tc.wantErrHas) {
					t.Errorf("args %v: error %q missing %q", tc.args, err, tc.wantErrHas)
				}
				// A rejected invocation must never have stored a credential.
				raw, _ := os.ReadFile(filepath.Join(dir, "civitai", "config.yaml"))
				if strings.Contains(string(raw), "token") {
					t.Errorf("args %v: rejected invocation stored a credential:\n%s", tc.args, raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("args %v: %v", tc.args, err)
			}
			if got := srv.initForm.Get("scope"); got != tc.wantScope {
				t.Errorf("args %v: sent scope=%q, want %q", tc.args, got, tc.wantScope)
			}
		})
	}
}

// TestLoginHelpDocumentsScopeSets: `civitai login --help` must let a user learn
// (a) that the set exists, and (b) that it grants Buzz-SPEND authority. Help
// text IS the surface where this decision is made.
func TestLoginHelpDocumentsScopeSets(t *testing.T) {
	out, _, err := run(t, "login", "--help")
	if err != nil {
		t.Fatalf("login --help: %v", err)
	}
	for _, want := range []string{"--scopes", "generate", "SPEND"} {
		if !strings.Contains(out, want) {
			t.Errorf("login --help missing %q:\n%s", want, out)
		}
	}
	// The default's guarantee must be stated too, or a reader can't tell that
	// omitting the flag is the safe choice.
	if !strings.Contains(out, "NOT Buzz-spend") {
		t.Errorf("login --help should state that the DEFAULT grants no Buzz-spend:\n%s", out)
	}
}
