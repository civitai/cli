package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/manifest"
	"github.com/spf13/cobra"
)

// withStdinTTY forces the package-level stdinIsTTY seam for the duration of a
// test, restoring it on cleanup. Confirmation behaviour hinges on this seam.
func withStdinTTY(t *testing.T, isTTY bool) {
	t.Helper()
	orig := stdinIsTTY
	stdinIsTTY = func() bool { return isTTY }
	t.Cleanup(func() { stdinIsTTY = orig })
}

func newConfirmCmd(stdin string) (*cobra.Command, *bytes.Buffer) {
	c := &cobra.Command{}
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetIn(strings.NewReader(stdin))
	return c, &out
}

// --- confirmSubmit unit tests (the safety logic in isolation) ---

// --yes always proceeds, regardless of TTY, without printing a prompt.
func TestConfirmSubmit_YesBypassesPrompt(t *testing.T) {
	withStdinTTY(t, false) // non-TTY, but --yes must still proceed
	c, out := newConfirmCmd("")
	m := &manifest.Manifest{BlockID: "demo", Version: "0.1.0"}
	if err := confirmSubmit(c, m, "https://civitai.com/", true); err != nil {
		t.Fatalf("confirmSubmit with --yes should proceed, got: %v", err)
	}
	if strings.Contains(out.String(), "Submit for review?") {
		t.Errorf("--yes must not print the confirmation prompt, got:\n%s", out.String())
	}
}

// Non-TTY without --yes REFUSES with a clear error (never hangs, never submits).
func TestConfirmSubmit_NonTTYRefusesWithoutYes(t *testing.T) {
	withStdinTTY(t, false)
	c, _ := newConfirmCmd("")
	m := &manifest.Manifest{BlockID: "demo", Version: "0.1.0"}
	err := confirmSubmit(c, m, "https://civitai.com/", false)
	if err == nil {
		t.Fatal("non-TTY without --yes must refuse")
	}
	if !strings.Contains(err.Error(), "refusing to submit without --yes") {
		t.Errorf("refusal error should be clear, got: %v", err)
	}
}

// Interactive TTY + explicit "y" proceeds, after showing what will happen.
func TestConfirmSubmit_TTYYesProceeds(t *testing.T) {
	withStdinTTY(t, true)
	c, out := newConfirmCmd("y\n")
	m := &manifest.Manifest{BlockID: "my-block", Version: "0.2.0"}
	if err := confirmSubmit(c, m, "https://civitai.com/", false); err != nil {
		t.Fatalf("TTY + 'y' should proceed, got: %v", err)
	}
	s := out.String()
	for _, want := range []string{"my-block@0.2.0", "moderator review", "Submit for review? [y/N]"} {
		if !strings.Contains(s, want) {
			t.Errorf("prompt output missing %q, got:\n%s", want, s)
		}
	}
}

// Interactive TTY + anything-but-yes (default / "n") cancels — no submission.
func TestConfirmSubmit_TTYDefaultCancels(t *testing.T) {
	for _, in := range []string{"\n", "n\n", "no\n", "maybe\n"} {
		withStdinTTY(t, true)
		c, _ := newConfirmCmd(in)
		m := &manifest.Manifest{BlockID: "demo", Version: "0.1.0"}
		err := confirmSubmit(c, m, "https://civitai.com/", false)
		if err == nil {
			t.Fatalf("input %q should cancel the submission", in)
		}
		if !strings.Contains(err.Error(), "cancelled") {
			t.Errorf("input %q: expected a cancellation error, got: %v", in, err)
		}
	}
}

// --- end-to-end command tests via the real submit path ---

// A bare `app submit` in a non-interactive shell (the accidental-footgun case)
// must NOT reach the submit endpoint even when a token is configured.
func TestAppSubmit_NonTTYRefusesWithoutYes_NoNetworkCall(t *testing.T) {
	withStdinTTY(t, false)
	tmp := t.TempDir()
	writeStaticManifest(t, tmp)

	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true // reaching here means a real publish request was fired
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfgdir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgdir)
	t.Setenv("CIVITAI_TOKEN", "tok-should-not-be-used")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	t.Setenv("CIVITAI_SUBMIT_PATH", "/api/blocks/submit-version")

	stdout, _, err := run(t, "app", "submit", tmp)
	if err == nil {
		t.Fatalf("bare submit in non-TTY must fail; stdout:\n%s", stdout)
	}
	if hit {
		t.Fatal("submit endpoint was hit — the gate did NOT prevent the submission")
	}
	if !strings.Contains(err.Error(), "refusing to submit without --yes") {
		t.Errorf("error should explain the refusal, got: %v", err)
	}
}

// The non-TTY refusal must fire BEFORE any packaging work: no "Packaged …"
// line is printed (it momentarily reads as if the submit is proceeding) and no
// .zip is left on disk. Guards the gate-before-package ordering.
func TestAppSubmit_NonTTYRefuseHappensBeforePackaging(t *testing.T) {
	withStdinTTY(t, false)
	tmp := t.TempDir()
	writeStaticManifest(t, tmp)

	cfgdir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgdir)
	t.Setenv("CIVITAI_TOKEN", "tok-should-not-be-used")
	t.Setenv("CIVITAI_BASE_URL", "https://civitai.com")

	stdout, stderr, err := run(t, "app", "submit", tmp)
	if err == nil {
		t.Fatalf("bare submit in non-TTY must fail; stdout:\n%s", stdout)
	}
	if strings.Contains(stdout, "Packaged") {
		t.Errorf("refusal must happen BEFORE packaging — stdout must not contain \"Packaged\", got:\n%s", stdout)
	}
	if !strings.Contains(err.Error(), "refusing to submit without --yes") {
		t.Errorf("error should explain the refusal, got: %v", err)
	}
	// No bundle should have been written anywhere under the app dir.
	entries, _ := os.ReadDir(tmp)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".zip") {
			t.Errorf("refusal wrote a .zip (%s) — packaging must not have run", e.Name())
		}
	}
	_ = stderr
}

// With --yes, the same non-interactive submit proceeds and DOES reach the
// (mocked) endpoint — the bypass works and no real endpoint is contacted.
func TestAppSubmit_YesProceedsToMockedEndpoint(t *testing.T) {
	withStdinTTY(t, false)
	tmp := t.TempDir()
	writeStaticManifest(t, tmp)

	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"publishRequestId":"pubreq_mock","slug":"demo-block","version":"0.1.0","status":"pending"}`))
	}))
	defer srv.Close()

	cfgdir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgdir)
	t.Setenv("CIVITAI_TOKEN", "tok-yes")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	t.Setenv("CIVITAI_SUBMIT_PATH", "/api/blocks/submit-version")

	stdout, _, err := run(t, "app", "submit", tmp, "--yes")
	if err != nil {
		t.Fatalf("submit --yes should proceed: %v\n%s", err, stdout)
	}
	if !hit {
		t.Fatal("submit --yes should have reached the (mocked) endpoint")
	}
	if !strings.Contains(stdout, "pubreq_mock") {
		t.Errorf("output should report the publish request id, got:\n%s", stdout)
	}
}

// --package-only never submits even with a token + non-TTY + no --yes: it stays
// the safe preview and the gate leaves it untouched.
func TestAppSubmit_PackageOnlyBypassesGate(t *testing.T) {
	withStdinTTY(t, false)
	tmp := t.TempDir()
	writeStaticManifest(t, tmp)

	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfgdir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgdir)
	t.Setenv("CIVITAI_TOKEN", "tok-present")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	t.Setenv("CIVITAI_SUBMIT_PATH", "/api/blocks/submit-version")

	out := tmp + "/bundle.zip"
	stdout, _, err := run(t, "app", "submit", tmp, "--package-only", "--out", out)
	if err != nil {
		t.Fatalf("submit --package-only: %v\n%s", err, stdout)
	}
	if hit {
		t.Fatal("--package-only must not contact the submit endpoint")
	}
	if !strings.Contains(stdout, "Wrote canonical bundle") {
		t.Errorf("--package-only should write the bundle, got:\n%s", stdout)
	}
}

// The help text advertises the confirmation + --yes bypass so the safety is
// discoverable.
func TestAppSubmit_HelpMentionsConfirmationAndYes(t *testing.T) {
	stdout, _, err := run(t, "app", "submit", "--help")
	if err != nil {
		t.Fatalf("submit --help: %v", err)
	}
	for _, want := range []string{"--yes", "confirm", "non-interactive"} {
		if !strings.Contains(strings.ToLower(stdout), strings.ToLower(want)) {
			t.Errorf("submit --help should mention %q, got:\n%s", want, stdout)
		}
	}
}
