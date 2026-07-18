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
)

// TestLoginTokenNoValuePrintsMintDeeplink covers the feature: `civitai login
// --token` with NO value must NOT error ("flag needs an argument"), NOT attempt
// any network login, and instead print the mint URL + how to re-run, exiting 0.
func TestLoginTokenNoValuePrintsMintDeeplink(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "")

	// A stub browser opener that fails the test if the device flow is reached.
	prev := browserOpener
	browserOpener = func(string) error {
		t.Fatal("login --token (no value) must not start the device/browser flow")
		return nil
	}
	defer func() { browserOpener = prev }()

	out, _, err := run(t, "login", "--token")
	if err != nil {
		t.Fatalf("login --token (no value) should exit cleanly, got: %v\n%s", err, out)
	}
	if !strings.Contains(out, accountAPIKeysURL) {
		t.Errorf("output should include the mint URL %q:\n%s", accountAPIKeysURL, out)
	}
	if !strings.Contains(out, "civitai login --token <key>") {
		t.Errorf("output should show how to re-run with a key:\n%s", out)
	}
	// Nothing should have been persisted — no login was attempted.
	if _, statErr := os.Stat(filepath.Join(dir, "civitai", "config.yaml")); statErr == nil {
		t.Error("no config should be written when only guidance is printed")
	}
}

// TestLoginTokenEmptyValuePrintsMintDeeplink covers the explicit-empty form
// (`login --token=`), which must behave like the no-value form.
func TestLoginTokenEmptyValuePrintsMintDeeplink(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "")

	out, _, err := run(t, "login", "--token=")
	if err != nil {
		t.Fatalf("login --token= should exit cleanly, got: %v\n%s", err, out)
	}
	if !strings.Contains(out, accountAPIKeysURL) {
		t.Errorf("output should include the mint URL %q:\n%s", accountAPIKeysURL, out)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "civitai", "config.yaml")); statErr == nil {
		t.Error("no config should be written for an empty --token value")
	}
}

// TestLoginTokenWithValueStillStores pins that `--token <value>` is UNCHANGED:
// it stores the personal key and does not print the mint guidance.
func TestLoginTokenWithValueStillStores(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "")

	out, _, err := run(t, "login", "--token", "abc123")
	if err != nil {
		t.Fatalf("login --token abc123: %v", err)
	}
	if !strings.Contains(out, "Token saved") {
		t.Errorf("token login should confirm save:\n%s", out)
	}
	// The mint guidance must NOT appear when a real value was provided.
	if strings.Contains(out, "create one at") {
		t.Errorf("mint guidance should not print when a token value is given:\n%s", out)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "civitai", "config.yaml"))
	if !strings.Contains(string(raw), "abc123") {
		t.Errorf("personal key should be persisted: %s", raw)
	}
}

// TestLoginArgMatrix pins the FULL `login`/`--token` argument-handling contract
// (one subtest per spec row), closing the audit footgun where a stray positional
// before a bare `--token` (`login foo --token`) was stored as the token,
// overwriting a good credential with garbage.
//
// The fix is structural: `--token` keeps NoOptDefVal (so bare `--token` prints
// the mint deeplink and a following flag isn't swallowed as the value) and the
// login command parses NON-interspersed, so a positional only binds as the token
// value when it FOLLOWS `--token`. A positional before it halts flag parsing,
// leaving `--token` unchanged so the NoArgs guard rejects the stray arg.
//
// Each row asserts: exit/err, whether a config was written (and its contents +
// 0600 perms for the store rows), whether the OAuth device flow made a network
// call, that the browser opener is never reached on a non-device row, and that
// no token/stray value leaks to stdout/stderr or (for strays) into a config.
func TestLoginArgMatrix(t *testing.T) {
	tests := []struct {
		name string
		args []string

		wantErr      bool   // command should exit non-zero
		wantConfig   bool   // a config file should exist afterward
		wantStored   string // config must contain this token AND it must NOT be echoed to stdout
		wantDeeplink bool   // stdout should print the mint deeplink + re-run guidance
		wantDevice   bool   // the OAuth device flow must make a network call

		// stray must never appear in stdout/stderr and must never be persisted to a
		// config file (the garbage-credential footgun + secret-hygiene guard).
		stray string
	}{
		// Row 1: `login` (bare) → device/browser flow (here the device-init fails
		// fast so the flow errors deterministically without polling).
		{name: "bare login runs device flow", args: []string{"login", "--no-browser"},
			wantErr: true, wantDevice: true},
		// Row 2: `login --token <key>` (space) → store.
		{name: "space form stores the key", args: []string{"login", "--token", "key-space"},
			wantConfig: true, wantStored: "key-space"},
		// Row 3: `login --token=<key>` (equals) → store.
		{name: "equals form stores the key", args: []string{"login", "--token=key-equals"},
			wantConfig: true, wantStored: "key-equals"},
		// Row 4: `login --token` (bare flag) → mint deeplink, no config, no network.
		{name: "bare --token prints deeplink", args: []string{"login", "--token"},
			wantDeeplink: true},
		// Row 5: `login --token=` (empty equals) → mint deeplink.
		{name: "empty equals prints deeplink", args: []string{"login", "--token="},
			wantDeeplink: true},
		// Row 6: `login foo` → stray positional rejected.
		{name: "stray positional rejected", args: []string{"login", "foo"},
			wantErr: true, stray: "foo"},
		// Row 7 (FOOTGUN): `login foo --token` → MUST NOT store foo as the token.
		// Chosen outcome: a clear error (the stray positional halts parsing, so
		// --token is never set and NoArgs rejects the stray) — nothing is stored.
		{name: "footgun stray positional then bare --token errors", args: []string{"login", "foo", "--token"},
			wantErr: true, stray: "foo"},
		// Row 8: `login foo --token bar` → stray positional, store nothing.
		{name: "stray positional then --token value errors", args: []string{"login", "foo", "--token", "bar"},
			wantErr: true, stray: "foo"},
		// Row 9: `login --token bar foo` → stray positional after the value, error.
		{name: "extra positional after value errors", args: []string{"login", "--token", "bar", "foo"},
			wantErr: true, stray: "bar"},
		// Row 10: `login --token a b` → two positionals, error.
		{name: "two positionals error", args: []string{"login", "--token", "a", "b"},
			wantErr: true, stray: "a"},
		// Row 11: `login --token --someUnknownFlag` → unknown flag, NOT swallowed
		// as the token value.
		{name: "unknown flag not swallowed as value", args: []string{"login", "--token", "--someUnknownFlag"},
			wantErr: true, stray: "--someUnknownFlag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", dir)
			t.Setenv("CIVITAI_TOKEN", "")

			// An OAuth server that records every hit and fails device-init fast, so
			// the ONLY thing that reaches it is the actual device flow (row 1) — and
			// even then without polling. Any hit on a token/deeplink/error row means
			// the device flow was wrongly entered.
			var hits int32
			var base string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&hits, 1)
				if r.URL.Path == "/.well-known/openid-configuration" {
					writeLoginDiscovery(w, base)
					return
				}
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_client"})
			}))
			defer srv.Close()
			base = srv.URL
			t.Setenv("CIVITAI_BASE_URL", srv.URL)

			// The browser opener must never be reached for a non-device row; on the
			// device row device-init fails before any open, so it's never reached
			// either. Any call is a bug.
			prev := browserOpener
			browserOpener = func(string) error {
				t.Error("browser opener must not be called")
				return nil
			}
			defer func() { browserOpener = prev }()

			out, errOut, err := run(t, tt.args...)

			if tt.wantErr && err == nil {
				t.Fatalf("expected a non-zero exit\nstdout:%s\nstderr:%s", out, errOut)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v\nstdout:%s\nstderr:%s", err, out, errOut)
			}

			cfgPath := filepath.Join(dir, "civitai", "config.yaml")
			fi, statErr := os.Stat(cfgPath)
			cfgExists := statErr == nil
			if tt.wantConfig && !cfgExists {
				t.Errorf("expected a config file at %s", cfgPath)
			}
			if !tt.wantConfig && cfgExists {
				t.Errorf("no config should be written for %v", tt.args)
			}

			if tt.wantStored != "" {
				raw, _ := os.ReadFile(cfgPath)
				if !strings.Contains(string(raw), tt.wantStored) {
					t.Errorf("config should persist the key %q: %s", tt.wantStored, raw)
				}
				// The credential file must be owner-only.
				if cfgExists && fi.Mode().Perm() != 0o600 {
					t.Errorf("config perms = %v, want 0600", fi.Mode().Perm())
				}
				// A stored credential must never be echoed back to the terminal.
				if strings.Contains(out, tt.wantStored) || strings.Contains(errOut, tt.wantStored) {
					t.Errorf("stored key %q must not be printed:\nstdout:%s\nstderr:%s", tt.wantStored, out, errOut)
				}
			}

			if tt.wantDeeplink {
				if !strings.Contains(out, accountAPIKeysURL) {
					t.Errorf("expected the mint deeplink %q:\n%s", accountAPIKeysURL, out)
				}
				if !strings.Contains(out, "civitai login --token <key>") {
					t.Errorf("expected re-run guidance:\n%s", out)
				}
			}

			gotDevice := atomic.LoadInt32(&hits) > 0
			if gotDevice != tt.wantDevice {
				t.Errorf("device flow attempted = %v, want %v (hits=%d)", gotDevice, tt.wantDevice, atomic.LoadInt32(&hits))
			}

			if tt.stray != "" {
				if strings.Contains(out, tt.stray) || strings.Contains(errOut, tt.stray) {
					t.Errorf("stray value %q leaked to output:\nstdout:%s\nstderr:%s", tt.stray, out, errOut)
				}
				if cfgExists {
					raw, _ := os.ReadFile(cfgPath)
					if strings.Contains(string(raw), tt.stray) {
						t.Errorf("stray value %q must never be persisted: %s", tt.stray, raw)
					}
				}
			}
		})
	}
}
