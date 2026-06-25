package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuzzCommandPrintsBalance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-buzz" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"result":{"data":{"json":{"blue":5,"green":7,"yellow":4242}}}}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "tok-buzz")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, _, err := run(t, "buzz")
	if err != nil {
		t.Fatalf("buzz: %v", err)
	}
	if !strings.Contains(out, "4242") {
		t.Errorf("buzz output should show the yellow balance: %s", out)
	}
	if !strings.Contains(out, "Yellow") || !strings.Contains(out, "generation-spend") {
		t.Errorf("buzz output should label yellow as the generation-spend currency: %s", out)
	}
}

func TestBuzzCommand403ShowsPersonalKeyGuidance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"json":{"message":"Your API key does not have the required scope for this action"}}}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "oauth-tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	_, errOut, err := run(t, "buzz")
	if err == nil {
		t.Fatal("a 403 should be a non-zero exit (actionable failure, not zero balance)")
	}
	if !strings.Contains(errOut, "personal API key") || !strings.Contains(errOut, "civitai login --token") {
		t.Errorf("403 should print the personal-key guidance: %s", errOut)
	}
	if !strings.Contains(errOut, "OAuth login") {
		t.Errorf("403 guidance should explain OAuth can't read/spend: %s", errOut)
	}
}

func TestBuzzCommandWithoutToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "")
	_, _, err := run(t, "buzz")
	if err == nil {
		t.Fatal("expected error when no token configured")
	}
	if !strings.Contains(err.Error(), "no token") {
		t.Errorf("error should mention missing token: %v", err)
	}
}

// TestWhoAmICapabilitiesSpendYes drives a full-scope key: both capabilities yes,
// no hint.
func TestWhoAmICapabilitiesSpendYes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// UserRead | AIServicesWrite | BuzzRead = 1 | 32768 | 65536 = 98305.
		_, _ = w.Write([]byte(`{"username":"zach","id":1,"tokenScope":98305}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "full-key")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, _, err := run(t, "whoami")
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	if !strings.Contains(out, "Capabilities:") {
		t.Errorf("whoami should print a Capabilities section: %s", out)
	}
	if !strings.Contains(out, "Spend Buzz (AI Services): yes") {
		t.Errorf("full-scope key should show Spend Buzz: yes: %s", out)
	}
	if !strings.Contains(out, "Read Buzz balance:        yes") {
		t.Errorf("full-scope key should show Read Buzz balance: yes: %s", out)
	}
	if strings.Contains(out, "can't spend Buzz") {
		t.Errorf("no spend-hint should print for a full-scope key: %s", out)
	}
}

// TestWhoAmICapabilitiesSpendNo drives an OAuth login token (UserRead +
// AppBlocksSubmit, no spend/read): both no, hint shown.
func TestWhoAmICapabilitiesSpendNo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// UserRead(1) | AppBlocksSubmit(1<<25=33554432) = 33554433. No spend/read.
		_, _ = w.Write([]byte(`{"username":"zach","id":1,"tokenScope":33554433}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "oauth-tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, _, err := run(t, "whoami")
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	if !strings.Contains(out, "Spend Buzz (AI Services): no") {
		t.Errorf("OAuth token should show Spend Buzz: no: %s", out)
	}
	if !strings.Contains(out, "Read Buzz balance:        no") {
		t.Errorf("OAuth token should show Read Buzz balance: no: %s", out)
	}
	if !strings.Contains(out, "personal API key") || !strings.Contains(out, "civitai login --token") {
		t.Errorf("Spend Buzz: no should print the personal-key hint: %s", out)
	}
}

// TestWhoAmICapabilitiesReadOnly drives a BuzzRead-but-no-spend token: read yes,
// spend no.
func TestWhoAmICapabilitiesReadOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// UserRead | BuzzRead = 1 | 65536 = 65537. No AIServicesWrite.
		_, _ = w.Write([]byte(`{"username":"zach","id":1,"tokenScope":65537}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "read-key")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, _, err := run(t, "whoami")
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	if !strings.Contains(out, "Spend Buzz (AI Services): no") {
		t.Errorf("read-only token should show Spend Buzz: no: %s", out)
	}
	if !strings.Contains(out, "Read Buzz balance:        yes") {
		t.Errorf("read-only token should show Read Buzz balance: yes: %s", out)
	}
}
