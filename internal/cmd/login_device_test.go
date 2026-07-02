package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestLoginDeviceFlowHappyPath drives `civitai login` (no --token) end to end
// against an httptest server: init -> two authorization_pending polls -> 200
// success -> tokens persisted at 0600.
func TestLoginDeviceFlowHappyPath(t *testing.T) {
	var polls int32
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeLoginDiscovery(w, base)
		case "/api/auth/oauth/device":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code":               "dev-secret-code",
				"user_code":                 "ABCD-1234",
				"verification_uri":          "https://civitai.com/oauth/device",
				"verification_uri_complete": "https://civitai.com/oauth/device?user_code=ABCD-1234",
				"expires_in":                900,
				"interval":                  1,
			})
		case "/api/auth/oauth/device-token":
			n := atomic.AddInt32(&polls, 1)
			if n < 3 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "access-123",
				"token_type":    "Bearer",
				"expires_in":    3600,
				"refresh_token": "refresh-456",
				// Echo the login scope the CLI requests (UserRead|AppBlocksSubmit
				// = 33554433) so the persisted-scope assertion below reflects what
				// `civitai login` obtains for the dev:live read/estimate paths.
				"scope": "33554433",
			})
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()
	base = srv.URL

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	// --no-browser to avoid spawning anything; the device flow still runs.
	out, _, err := run(t, "login", "--no-browser")
	if err != nil {
		t.Fatalf("login device flow: %v\n%s", err, out)
	}

	// User-facing output shows the URL + code but never the device_code secret.
	if !strings.Contains(out, "ABCD-1234") {
		t.Errorf("login output should show the user_code: %s", out)
	}
	if strings.Contains(out, "dev-secret-code") {
		t.Errorf("login output must NOT leak the device_code: %s", out)
	}
	if !strings.Contains(out, "Logged in") {
		t.Errorf("login should confirm success: %s", out)
	}
	if polls != 3 {
		t.Errorf("expected 3 device-token polls, got %d", polls)
	}

	// Tokens persisted at 0600.
	cfgPath := filepath.Join(dir, "civitai", "config.yaml")
	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("config perms = %o, want 600", perm)
	}
	raw, _ := os.ReadFile(cfgPath)
	var onDisk map[string]any
	if err := yaml.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("config yaml: %v", err)
	}
	if onDisk["access_token"] != "access-123" || onDisk["refresh_token"] != "refresh-456" {
		t.Errorf("persisted tokens = %v", onDisk)
	}
	if onDisk["auth_kind"] != "oauth" {
		t.Errorf("auth_kind = %v, want oauth", onDisk["auth_kind"])
	}
	if onDisk["scope"] != "33554433" {
		t.Errorf("scope = %v", onDisk["scope"])
	}
}

// TestLoginDeviceFlowDenied surfaces a terminal access_denied error.
func TestLoginDeviceFlowDenied(t *testing.T) {
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeLoginDiscovery(w, base)
		case "/api/auth/oauth/device":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code": "dc", "user_code": "X", "verification_uri": "https://e/x",
				"expires_in": 900, "interval": 1,
			})
		case "/api/auth/oauth/device-token":
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "access_denied"})
		}
	}))
	defer srv.Close()
	base = srv.URL

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	_, _, err := run(t, "login", "--no-browser")
	if err == nil {
		t.Fatal("expected a terminal error on access_denied")
	}
	if !strings.Contains(err.Error(), "denied") {
		t.Errorf("error should mention denial: %v", err)
	}
	// Nothing should be persisted on failure.
	if _, statErr := os.Stat(filepath.Join(dir, "civitai", "config.yaml")); statErr == nil {
		t.Error("no config should be written on a failed login")
	}
}

// TestLoginTokenFlagStillWorks confirms the personal-key path is unchanged and
// does not invoke the device flow.
func TestLoginTokenFlagStillWorks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "")

	out, _, err := run(t, "login", "--token", "pk-123")
	if err != nil {
		t.Fatalf("login --token: %v", err)
	}
	if !strings.Contains(out, "Token saved") {
		t.Errorf("token login should confirm save: %s", out)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "civitai", "config.yaml"))
	var onDisk map[string]any
	_ = yaml.Unmarshal(raw, &onDisk)
	if onDisk["token"] != "pk-123" {
		t.Errorf("personal key not stored under token: %v", onDisk)
	}
	if onDisk["auth_kind"] != "token" {
		t.Errorf("auth_kind = %v, want token", onDisk["auth_kind"])
	}
}

// loginDeviceServer stands up an httptest server for the device flow: discovery,
// device-init (with a code-prefilled complete URL), and an immediate 200 token.
func loginDeviceServer(t *testing.T) (*httptest.Server, *string) {
	t.Helper()
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeLoginDiscovery(w, base)
		case "/api/auth/oauth/device":
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
				"expires_in": 3600, "refresh_token": "refresh-456", "scope": "33554433",
			})
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	return srv, &base
}

// TestLoginDeviceOpensBrowserAndPrintsSingleFallback: with browser-open enabled
// and SUCCEEDING, the primary action is the auto-open and the output carries a
// SINGLE "or open ..." fallback (bare URL + code) — NOT the code-prefilled
// complete URL again, and never the device_code.
func TestLoginDeviceOpensBrowserAndPrintsSingleFallback(t *testing.T) {
	srv, base := loginDeviceServer(t)
	defer srv.Close()
	*base = srv.URL

	var openedURL string
	prev := browserOpener
	browserOpener = func(u string) error { openedURL = u; return nil }
	defer func() { browserOpener = prev }()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, _, err := run(t, "login") // browser-open ENABLED
	if err != nil {
		t.Fatalf("login: %v\n%s", err, out)
	}

	// The browser was auto-opened on the code-prefilled complete URL.
	if openedURL != "https://civitai.com/oauth/device?user_code=ABCD-1234" {
		t.Errorf("browser should open the complete URL, opened %q", openedURL)
	}
	if !strings.Contains(out, "Opened your browser to approve.") {
		t.Errorf("expected the opened-browser primary line:\n%s", out)
	}
	// Single "or open ..." fallback: the BARE url + the code shown separately.
	if !strings.Contains(out, "Or open https://civitai.com/oauth/device and enter code ABCD-1234") {
		t.Errorf("expected the single bare-URL fallback line:\n%s", out)
	}
	// The complete (code-prefilled) URL must NOT be printed again in the happy path.
	if strings.Contains(out, "?user_code=") {
		t.Errorf("happy path must not reprint the code-prefilled complete URL:\n%s", out)
	}
	// The old both-URLs manual block must be gone.
	if strings.Contains(out, "URL:  ") {
		t.Errorf("happy path must not print the manual URL:/Code: block:\n%s", out)
	}
	if strings.Contains(out, "dev-secret-code") {
		t.Errorf("device_code (secret) must never be printed:\n%s", out)
	}
	if !strings.Contains(out, "Logged in") {
		t.Errorf("login should confirm success:\n%s", out)
	}
}

// TestLoginDeviceBrowserOpenFailureFallsBack: when browser-open FAILS, the flow
// falls back to the full manual instructions (the complete URL to open) and does
// NOT claim it opened a browser. The device_code is never printed.
func TestLoginDeviceBrowserOpenFailureFallsBack(t *testing.T) {
	srv, base := loginDeviceServer(t)
	defer srv.Close()
	*base = srv.URL

	prev := browserOpener
	browserOpener = func(string) error { return errStub }
	defer func() { browserOpener = prev }()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, _, err := run(t, "login") // browser-open ENABLED but fails
	if err != nil {
		t.Fatalf("login: %v\n%s", err, out)
	}
	if strings.Contains(out, "Opened your browser") {
		t.Errorf("must not claim it opened a browser when open failed:\n%s", out)
	}
	if !strings.Contains(out, "your code is pre-filled") || !strings.Contains(out, "?user_code=ABCD-1234") {
		t.Errorf("expected the manual complete-URL instructions on open failure:\n%s", out)
	}
	if strings.Contains(out, "dev-secret-code") {
		t.Errorf("device_code (secret) must never be printed:\n%s", out)
	}
}

// errStub is a fixed non-nil error for simulating browser-open failure.
var errStub = stubErr("open failed")

type stubErr string

func (e stubErr) Error() string { return string(e) }

// writeLoginDiscovery serves an OpenID discovery doc whose endpoints point back
// at the test server `base`, mirroring how the real auth host advertises its
// OAuth endpoints. The CLI resolves these before the device-init POST.
func writeLoginDiscovery(w http.ResponseWriter, base string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"issuer":                        base,
		"device_authorization_endpoint": base + "/api/auth/oauth/device",
		"token_endpoint":                base + "/api/auth/oauth/token",
	})
}
