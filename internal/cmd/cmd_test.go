package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// run executes the root command with args, capturing stdout+stderr.
func run(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	root := NewRootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errb.String(), err
}

func TestRootHelpListsCommands(t *testing.T) {
	out, _, err := run(t, "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	for _, want := range []string{"app", "login", "whoami", "version", "completion"} {
		if !strings.Contains(out, want) {
			t.Errorf("root help missing command %q\n%s", want, out)
		}
	}
}

func TestVersionCommand(t *testing.T) {
	out, _, err := run(t, "version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	for _, want := range []string{"civitai", "commit:", "built:", "go:"} {
		if !strings.Contains(out, want) {
			t.Errorf("version output missing %q: %s", want, out)
		}
	}
}

func TestSetBuildInfo(t *testing.T) {
	origV, origC, origD := version, commit, date
	t.Cleanup(func() { version, commit, date = origV, origC, origD })

	SetBuildInfo("1.2.3", "abc123", "2026-01-01")
	if version != "1.2.3" || commit != "abc123" || date != "2026-01-01" {
		t.Fatalf("SetBuildInfo did not apply: %s %s %s", version, commit, date)
	}
	// Empty values must not clobber existing.
	SetBuildInfo("", "", "")
	if version != "1.2.3" || commit != "abc123" || date != "2026-01-01" {
		t.Errorf("empty SetBuildInfo clobbered values: %s %s %s", version, commit, date)
	}
}

func TestCompletionGeneratesForEachShell(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		out, _, err := run(t, "completion", shell)
		if err != nil {
			t.Fatalf("completion %s: %v", shell, err)
		}
		if len(out) == 0 {
			t.Errorf("completion %s produced no output", shell)
		}
	}
}

func TestCompletionRejectsUnknownShell(t *testing.T) {
	if _, _, err := run(t, "completion", "tcsh"); err == nil {
		t.Fatal("expected error for unknown shell")
	}
}

func TestAppHelp(t *testing.T) {
	out, _, err := run(t, "app", "--help")
	if err != nil {
		t.Fatalf("app --help: %v", err)
	}
	for _, want := range []string{"init", "validate", "submit"} {
		if !strings.Contains(out, want) {
			t.Errorf("app help missing subcommand %q", want)
		}
	}
}

func TestAppInitScaffoldsAndValidates(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)

	out, _, err := run(t, "app", "init", "my-block")
	if err != nil {
		t.Fatalf("app init: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Created App Block") {
		t.Errorf("unexpected init output: %s", out)
	}
	// Manifest must exist and validate clean.
	if _, err := os.Stat(filepath.Join(tmp, "my-block", "block.manifest.json")); err != nil {
		t.Fatalf("scaffolded manifest missing: %v", err)
	}
	if _, _, err := run(t, "app", "validate", filepath.Join(tmp, "my-block")); err != nil {
		t.Errorf("scaffolded project should validate: %v", err)
	}
}

func TestAppInitPageViteTemplate(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	out, _, err := run(t, "app", "init", "vite-app", "--template", "page-vite")
	if err != nil {
		t.Fatalf("app init page-vite: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(tmp, "vite-app", "package.json")); err != nil {
		t.Errorf("page-vite should scaffold package.json: %v", err)
	}
	if !strings.Contains(out, "npm install") {
		t.Errorf("page-vite next steps should mention npm install: %s", out)
	}
}

func TestAppInitRequiresName(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	if _, _, err := run(t, "app", "init"); err == nil {
		t.Fatal("expected error when no name is given")
	}
}

func TestAppInitFromIsNotWired(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	_, _, err := run(t, "app", "init", "forked", "--from", "some-slug")
	if err == nil {
		t.Fatal("expected --from to be reported as not wired")
	}
	if !strings.Contains(err.Error(), "not yet wired") {
		t.Errorf("error should say not yet wired: %v", err)
	}
}

func TestAppInitUnknownTemplate(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	if _, _, err := run(t, "app", "init", "x", "--template", "nope"); err == nil {
		t.Fatal("expected error for unknown template")
	}
}

func TestAppInitSlugifiesDisplayName(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	out, _, err := run(t, "app", "init", "My Cool Block")
	if err != nil {
		t.Fatalf("app init: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(tmp, "my-cool-block", "block.manifest.json")); err != nil {
		t.Errorf("expected slug dir my-cool-block: %v", err)
	}
}

func TestAppValidateReportsErrors(t *testing.T) {
	tmp := t.TempDir()
	// A manifest missing required fields.
	if err := os.WriteFile(filepath.Join(tmp, "block.manifest.json"), []byte(`{"blockId":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, errOut, err := run(t, "app", "validate", tmp)
	if err == nil {
		t.Fatal("expected validation to fail for a bad manifest")
	}
	if !strings.Contains(errOut, "validation error") {
		t.Errorf("stderr should list validation errors: %s", errOut)
	}
}

func TestAppValidateMissingManifest(t *testing.T) {
	tmp := t.TempDir()
	_, _, err := run(t, "app", "validate", tmp)
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
}

func TestWhoAmIWithoutToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "")
	_, _, err := run(t, "whoami")
	if err == nil {
		t.Fatal("expected error when no token configured")
	}
	if !strings.Contains(err.Error(), "no token") {
		t.Errorf("error should mention missing token: %v", err)
	}
}

func TestWhoAmISuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"username":"bob","id":99}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, _, err := run(t, "whoami")
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	if !strings.Contains(out, "bob") || !strings.Contains(out, "99") {
		t.Errorf("whoami output should report the user: %s", out)
	}
}

func TestLoginStoresToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "")
	out, _, err := run(t, "login", "--token", "abc123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !strings.Contains(out, "Token saved") {
		t.Errorf("login should confirm save: %s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "civitai", "config.yaml")); err != nil {
		t.Errorf("config file should be written: %v", err)
	}
}

func TestLoginEmptyTokenViaStdinFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "")
	root := NewRootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetIn(strings.NewReader("\n")) // empty line at the prompt
	root.SetArgs([]string{"login"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestJoinLines(t *testing.T) {
	if got := joinLines([]string{"a", "b"}); got != "a\n  b" {
		t.Errorf("joinLines = %q", got)
	}
	if got := joinLines(nil); got != "" {
		t.Errorf("joinLines(nil) = %q", got)
	}
}

// chdir changes into dir for the duration of the test.
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}
