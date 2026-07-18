package cmd

import (
	"os"
	"path/filepath"
	"strings"
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
