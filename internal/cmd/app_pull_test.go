package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cloneInfoServer stands up an httptest server emulating the
// GET /api/trpc/blocks.getMyForgejoCloneInfo query, returning the canned tRPC
// envelope + recording the decoded `input` so URL/input encoding is asserted.
func cloneInfoServer(t *testing.T, jsonResult any, status int, gotInput *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotInput != nil {
			*gotInput = r.URL.Query().Get("input")
		}
		if status != 0 && status != http.StatusOK {
			w.WriteHeader(status)
			// tRPC error envelope.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"json": map[string]any{"message": "boom", "code": status}},
			})
			return
		}
		// tRPC success envelope: { result: { data: { json: <result> } } }.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{"data": map[string]any{"json": jsonResult}},
		})
	}))
}

// stubGit replaces gitRunner for the duration of the test, recording calls.
func stubGit(t *testing.T, fn func(dir string, args ...string) error) {
	t.Helper()
	orig := gitRunner
	gitRunner = fn
	t.Cleanup(func() { gitRunner = orig })
}

const okCloneURL = "https://dev-7:tok-sha1@forgejo.civitai.com/civitai-apps/my-block.git"
const okHTTPURL = "https://forgejo.civitai.com/civitai-apps/my-block.git"

func okCloneInfo() map[string]any {
	return map[string]any{
		"notYetAvailable": false,
		"slug":            "my-block",
		"forgejoUsername": "dev-7",
		"token":           "tok-sha1",
		"httpUrl":         okHTTPURL,
		"cloneUrl":        okCloneURL,
	}
}

func TestAppPullClonesFreshDir(t *testing.T) {
	var gotInput string
	srv := cloneInfoServer(t, okCloneInfo(), http.StatusOK, &gotInput)
	defer srv.Close()

	var gitCalls [][]string
	stubGit(t, func(dir string, args ...string) error {
		gitCalls = append(gitCalls, args)
		return nil
	})

	tmp := t.TempDir()
	chdir(t, tmp)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, errOut, err := run(t, "app", "pull", "--app", "my-block")
	if err != nil {
		t.Fatalf("app pull: %v\n%s", err, errOut)
	}

	// The tRPC input carries the slug under {"json":{"slug":...}}.
	if !strings.Contains(gotInput, `"slug":"my-block"`) {
		t.Errorf("tRPC input should carry the slug: %q", gotInput)
	}
	// A fresh dir → `git clone -- <cloneUrl> <slug>`. The literal `--` separator
	// is mandatory: it stops git from parsing a dash-leading URL/dir as a flag
	// (argument-injection hardening).
	if len(gitCalls) != 1 {
		t.Fatalf("expected exactly one git call, got %d: %v", len(gitCalls), gitCalls)
	}
	wantClone := []string{"clone", "--", okCloneURL, "my-block"}
	if len(gitCalls[0]) != len(wantClone) {
		t.Fatalf("clone args = %v, want %v", gitCalls[0], wantClone)
	}
	for i, a := range wantClone {
		if gitCalls[0][i] != a {
			t.Fatalf("clone args = %v, want %v", gitCalls[0], wantClone)
		}
	}
	if !strings.Contains(out, "Cloning my-block") {
		t.Errorf("output should announce the clone: %s", out)
	}
	// The token-leakage caveat MUST be surfaced (token now on disk).
	if !strings.Contains(errOut, "embeds your access token") {
		t.Errorf("pull must warn about token-in-URL leakage: %s", errOut)
	}
	// The CLONE path persists the URL to .git/config, so the warning MUST claim
	// config-persistence and offer the set-url remedy.
	if !strings.Contains(errOut, ".git/config") {
		t.Errorf("clone warning must mention .git/config persistence: %s", errOut)
	}
	if !strings.Contains(errOut, "remote set-url origin "+okHTTPURL) {
		t.Errorf("clone warning must offer the set-url remedy: %s", errOut)
	}
}

func TestAppPullSyncsExistingCheckout(t *testing.T) {
	srv := cloneInfoServer(t, okCloneInfo(), http.StatusOK, nil)
	defer srv.Close()

	var gitCalls [][]string
	stubGit(t, func(dir string, args ...string) error {
		gitCalls = append(gitCalls, args)
		return nil
	})

	tmp := t.TempDir()
	chdir(t, tmp)
	// Make ./my-block already a git checkout (a .git dir).
	if err := os.MkdirAll(filepath.Join(tmp, "my-block", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, errOut, err := run(t, "app", "pull", "--app", "my-block")
	if err != nil {
		t.Fatalf("app pull (sync): %v", err)
	}
	// An existing checkout → `git -C my-block pull --ff-only <cloneUrl>`.
	if len(gitCalls) != 1 {
		t.Fatalf("expected one git call, got %v", gitCalls)
	}
	want := []string{"-C", "my-block", "pull", "--ff-only", okCloneURL}
	for i, a := range want {
		if i >= len(gitCalls[0]) || gitCalls[0][i] != a {
			t.Fatalf("pull args = %v, want %v", gitCalls[0], want)
		}
	}
	if len(gitCalls[0]) != len(want) {
		t.Fatalf("pull args = %v, want exactly %v", gitCalls[0], want)
	}
	if !strings.Contains(out, "Syncing existing checkout") {
		t.Errorf("output should announce the sync: %s", out)
	}
	// The SYNC path passes the URL explicitly; git does NOT persist it to
	// .git/config, so the warning must NOT claim config-persistence (that would
	// send the user chasing a non-existent on-disk token + a useless set-url).
	if strings.Contains(errOut, "stored it in .git/config") {
		t.Errorf("sync warning must NOT claim the token was stored in .git/config: %s", errOut)
	}
	if strings.Contains(errOut, "remote set-url") {
		t.Errorf("sync warning must NOT offer the clone-only set-url remedy: %s", errOut)
	}
	// And it should affirmatively reassure that nothing was persisted to config.
	if !strings.Contains(errOut, "NOT persisted to .git/config") {
		t.Errorf("sync warning should clarify the token was NOT persisted to config: %s", errOut)
	}
	// It should still surface the transient process-args exposure accurately.
	if !strings.Contains(errOut, "access token") {
		t.Errorf("sync should still warn about the transient token exposure: %s", errOut)
	}
}

func TestAppPullExplicitDir(t *testing.T) {
	srv := cloneInfoServer(t, okCloneInfo(), http.StatusOK, nil)
	defer srv.Close()
	var gitCalls [][]string
	stubGit(t, func(dir string, args ...string) error {
		gitCalls = append(gitCalls, args)
		return nil
	})

	tmp := t.TempDir()
	chdir(t, tmp)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	if _, _, err := run(t, "app", "pull", "custom-dir", "--app", "my-block"); err != nil {
		t.Fatalf("app pull custom-dir: %v", err)
	}
	// `clone -- <url> <dir>` → the explicit dir is the 4th argv element.
	if len(gitCalls[0]) != 4 || gitCalls[0][3] != "custom-dir" {
		t.Errorf("explicit dir should be the clone target after `--`: %v", gitCalls[0])
	}
}

// TestAppPullCloneSeparatesPositionals pins the argument-injection hardening: a
// dash-leading target dir (e.g. `-bad`) MUST be passed as a positional AFTER the
// literal `--` separator, so git treats it as the clone destination rather than
// parsing it as a flag (e.g. `--bare`, `-c core.hooksPath=…`, `--template=…`).
func TestAppPullCloneSeparatesPositionals(t *testing.T) {
	srv := cloneInfoServer(t, okCloneInfo(), http.StatusOK, nil)
	defer srv.Close()

	var gitCalls [][]string
	stubGit(t, func(dir string, args ...string) error {
		gitCalls = append(gitCalls, args)
		return nil
	})

	tmp := t.TempDir()
	chdir(t, tmp)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	// A dash-leading target dir — the canonical argument-injection probe. The CLI
	// `--` lets cobra accept `-bad` as a positional arg (rather than a CLI flag);
	// the code under test must in turn pass it to git after git's own `--`.
	if _, _, err := run(t, "app", "pull", "--app", "my-block", "--", "-bad"); err != nil {
		t.Fatalf("app pull -- -bad: %v", err)
	}
	if len(gitCalls) != 1 {
		t.Fatalf("expected one git call, got %v", gitCalls)
	}
	argv := gitCalls[0]
	// `--` must appear immediately before the URL+dir positionals.
	want := []string{"clone", "--", okCloneURL, "-bad"}
	if len(argv) != len(want) {
		t.Fatalf("clone argv = %v, want %v", argv, want)
	}
	for i, a := range want {
		if argv[i] != a {
			t.Fatalf("clone argv = %v, want %v", argv, want)
		}
	}
	// Belt-and-suspenders: the dash-leading dir must come AFTER the `--`, never
	// before it (where git would interpret it as a flag).
	dashSep, dirIdx := -1, -1
	for i, a := range argv {
		if a == "--" {
			dashSep = i
		}
		if a == "-bad" {
			dirIdx = i
		}
	}
	if dashSep == -1 {
		t.Fatalf("clone argv must contain a `--` separator: %v", argv)
	}
	if dirIdx <= dashSep {
		t.Fatalf("dash-leading dir must come after `--` (got sep=%d dir=%d): %v", dashSep, dirIdx, argv)
	}
}

func TestAppPullNotYetAvailable(t *testing.T) {
	srv := cloneInfoServer(t, map[string]any{
		"notYetAvailable": true,
		"slug":            "my-block",
		"message":         "approve your first version first",
	}, http.StatusOK, nil)
	defer srv.Close()

	called := false
	stubGit(t, func(dir string, args ...string) error { called = true; return nil })

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	_, _, err := run(t, "app", "pull", "--app", "my-block")
	if err == nil {
		t.Fatal("expected error when git access is not yet available")
	}
	if !strings.Contains(err.Error(), "approve your first version") {
		t.Errorf("error should carry the server message: %v", err)
	}
	if called {
		t.Error("git must NOT run when the repo is not yet available")
	}
}

func TestAppPullMissingAppErrors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	_, _, err := run(t, "app", "pull")
	if err == nil {
		t.Fatal("expected error when --app is missing")
	}
}

func TestAppPullMissingTokenErrors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "")
	_, _, err := run(t, "app", "pull", "--app", "my-block")
	if err == nil {
		t.Fatal("expected error with no token")
	}
	if !strings.Contains(err.Error(), "no token") {
		t.Errorf("missing-token error should explain: %v", err)
	}
}

func TestAppPullServerErrorMapped(t *testing.T) {
	srv := cloneInfoServer(t, nil, http.StatusForbidden, nil)
	defer srv.Close()
	stubGit(t, func(dir string, args ...string) error {
		t.Error("git must not run on a server error")
		return nil
	})
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	_, _, err := run(t, "app", "pull", "--app", "my-block")
	if err == nil {
		t.Fatal("expected error on 403")
	}
	if !strings.Contains(err.Error(), "not permitted") {
		t.Errorf("403 should map to a not-permitted message: %v", err)
	}
}

func TestAppHelpListsPull(t *testing.T) {
	out, _, err := run(t, "app", "--help")
	if err != nil {
		t.Fatalf("app --help: %v", err)
	}
	if !strings.Contains(out, "pull") {
		t.Errorf("app help should list the pull subcommand:\n%s", out)
	}
}
