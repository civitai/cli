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

// stubHelperExe pins how the installed helper names the binary, so argv
// assertions do not depend on whether `civitai` is on the test machine's PATH.
func stubHelperExe(t *testing.T) {
	t.Helper()
	orig := helperExecutable
	helperExecutable = func() string { return "civitai" }
	t.Cleanup(func() { helperExecutable = orig })
}

// stubGitOutput replaces the read-only git runner (used for the remote scan).
func stubGitOutput(t *testing.T, fn gitOutputFunc) {
	t.Helper()
	orig := gitOutput
	gitOutput = fn
	t.Cleanup(func() { gitOutput = orig })
}

const okCredCtx = "credential.https://forgejo.civitai.com"

// wantHelperCfg is the exact helper configuration `app pull` must install for
// okCloneInfo, as `-c key=value` clone arguments.
var wantCloneConfig = []string{
	"-c", okCredCtx + ".helper=",
	"-c", okCredCtx + ".helper=!civitai git-credential --app my-block",
	"-c", okCredCtx + ".useHttpPath=true",
}

func equalArgv(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// assertNoToken fails if the token appears in ANY git argv: a token on a
// command line is visible to every local process, and a clone URL with one is
// persisted into .git/config.
func assertNoToken(t *testing.T, calls [][]string) {
	t.Helper()
	for _, c := range calls {
		for _, a := range c {
			if strings.Contains(a, "tok-sha1") {
				t.Fatalf("git argv carries the token: %v", c)
			}
		}
	}
}

// 🔴 Regression (red on the pre-change CLI, which cloned okCloneURL — the token
// URL — and so persisted the token into .git/config).
func TestAppPullClonesTokenlessURLWithHelper(t *testing.T) {
	var gotInput string
	srv := cloneInfoServer(t, okCloneInfo(), http.StatusOK, &gotInput)
	defer srv.Close()
	stubHelperExe(t)

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
	if !strings.Contains(gotInput, `"slug":"my-block"`) {
		t.Errorf("tRPC input should carry the slug: %q", gotInput)
	}
	// One `git clone -c … -- <token-less url> <slug>`. The literal `--` separator
	// is mandatory: it stops git from parsing a dash-leading URL/dir as a flag.
	if len(gitCalls) != 1 {
		t.Fatalf("expected exactly one git call, got %d: %v", len(gitCalls), gitCalls)
	}
	want := append(append([]string{"clone"}, wantCloneConfig...), "--", okHTTPURL, "my-block")
	if !equalArgv(gitCalls[0], want) {
		t.Fatalf("clone args =\n  %q\nwant\n  %q", gitCalls[0], want)
	}
	assertNoToken(t, gitCalls)
	if !strings.Contains(out, "Cloning my-block") {
		t.Errorf("output should announce the clone: %s", out)
	}
	if !strings.Contains(out, "no token is stored there") {
		t.Errorf("output should say no token is stored: %s", out)
	}
	if strings.Contains(errOut, "embeds your access token") {
		t.Errorf("the token-in-URL warning is obsolete and must not print: %s", errOut)
	}
}

// 🔴 Regression (red pre-change: the sync fetched okCloneURL, putting the token
// on git's command line, and installed no helper).
func TestAppPullSyncsExistingCheckoutWithoutToken(t *testing.T) {
	srv := cloneInfoServer(t, okCloneInfo(), http.StatusOK, nil)
	defer srv.Close()
	stubHelperExe(t)
	// No remotes carry a credential.
	stubGitOutput(t, func(dir string, args ...string) (string, error) { return "", nil })

	var gitCalls [][]string
	stubGit(t, func(dir string, args ...string) error {
		gitCalls = append(gitCalls, args)
		return nil
	})

	tmp := t.TempDir()
	chdir(t, tmp)
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
	want := [][]string{
		{"-C", "my-block", "config", "--local", "--unset-all", okCredCtx + ".helper"},
		{"-C", "my-block", "config", "--local", "--add", okCredCtx + ".helper", ""},
		{"-C", "my-block", "config", "--local", "--add", okCredCtx + ".helper", "!civitai git-credential --app my-block"},
		{"-C", "my-block", "config", "--local", "--unset-all", okCredCtx + ".useHttpPath"},
		{"-C", "my-block", "config", "--local", "--add", okCredCtx + ".useHttpPath", "true"},
		// `--` immediately before the URL: a dash-leading URL stays a positional.
		{"-C", "my-block", "fetch", "--", okHTTPURL, "HEAD"},
		{"-C", "my-block", "merge", "--ff-only", "FETCH_HEAD"},
	}
	if len(gitCalls) != len(want) {
		t.Fatalf("git calls =\n  %q\nwant\n  %q", gitCalls, want)
	}
	for i := range want {
		if !equalArgv(gitCalls[i], want[i]) {
			t.Fatalf("git call %d = %q, want %q", i, gitCalls[i], want[i])
		}
	}
	assertNoToken(t, gitCalls)
	if !strings.Contains(out, "Syncing existing checkout") {
		t.Errorf("output should announce the sync: %s", out)
	}
	if strings.Contains(errOut, "access token") {
		t.Errorf("nothing was migrated, so no token warning may print: %s", errOut)
	}
}

func TestAppPullExplicitDir(t *testing.T) {
	srv := cloneInfoServer(t, okCloneInfo(), http.StatusOK, nil)
	defer srv.Close()
	stubHelperExe(t)
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
	argv := gitCalls[0]
	if len(argv) < 2 || argv[len(argv)-1] != "custom-dir" || argv[len(argv)-3] != "--" {
		t.Errorf("explicit dir should be the clone target after `--`: %v", argv)
	}
}

// TestAppPullCloneSeparatesPositionals pins the argument-injection hardening: a
// dash-leading target dir (e.g. `-bad`) MUST be passed as a positional AFTER the
// literal `--` separator, so git treats it as the clone destination rather than
// parsing it as a flag (e.g. `--bare`, `-c core.hooksPath=…`, `--template=…`).
func TestAppPullCloneSeparatesPositionals(t *testing.T) {
	srv := cloneInfoServer(t, okCloneInfo(), http.StatusOK, nil)
	defer srv.Close()
	stubHelperExe(t)

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

	if _, _, err := run(t, "app", "pull", "--app", "my-block", "--", "-bad"); err != nil {
		t.Fatalf("app pull -- -bad: %v", err)
	}
	if len(gitCalls) != 1 {
		t.Fatalf("expected one git call, got %v", gitCalls)
	}
	want := append(append([]string{"clone"}, wantCloneConfig...), "--", okHTTPURL, "-bad")
	if !equalArgv(gitCalls[0], want) {
		t.Fatalf("clone argv = %q, want %q", gitCalls[0], want)
	}
}

// TestAppPullRefusesUnusableRepoURL: the repository URL is server-controlled and
// lands in git's argv and in .git/config, so one that is not an http(s) URL — the
// dash-leading `--upload-pack=` probe — or that carries a credential is refused
// before ANY git command runs.
func TestAppPullRefusesUnusableRepoURL(t *testing.T) {
	for _, bad := range []string{"--upload-pack=/tmp/pwn.sh", "ext::sh -c touch% /tmp/pwned", "https://u:p@forgejo.civitai.com/civitai-apps/my-block.git"} {
		t.Run(bad, func(t *testing.T) {
			info := okCloneInfo()
			info["httpUrl"] = bad
			srv := cloneInfoServer(t, info, http.StatusOK, nil)
			defer srv.Close()
			called := false
			stubGit(t, func(dir string, args ...string) error { called = true; return nil })

			tmp := t.TempDir()
			chdir(t, tmp)
			if err := os.MkdirAll(filepath.Join(tmp, "my-block", ".git"), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("CIVITAI_TOKEN", "tok-1")
			t.Setenv("CIVITAI_BASE_URL", srv.URL)

			if _, _, err := run(t, "app", "pull", "--app", "my-block"); err == nil {
				t.Fatalf("expected a refusal for repository URL %q", bad)
			}
			if called {
				t.Fatalf("git must not run for repository URL %q", bad)
			}
		})
	}
}

// TestAppPullRefusesShellUnsafeSlug: the slug is written into a `!` helper that
// git runs through sh. A slug carrying shell syntax is refused before git runs,
// even when an explicit [dir] makes the directory check irrelevant.
func TestAppPullRefusesShellUnsafeSlug(t *testing.T) {
	for _, slug := range []string{"a;touch /tmp/x", "a$(id)", "a b", "a'b", "-x"} {
		t.Run(slug, func(t *testing.T) {
			info := okCloneInfo()
			info["slug"] = slug
			srv := cloneInfoServer(t, info, http.StatusOK, nil)
			defer srv.Close()
			called := false
			stubGit(t, func(dir string, args ...string) error { called = true; return nil })

			tmp := t.TempDir()
			chdir(t, tmp)
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("CIVITAI_TOKEN", "tok-1")
			t.Setenv("CIVITAI_BASE_URL", srv.URL)

			_, _, err := run(t, "app", "pull", "safe-dir", "--app", "my-block")
			if err == nil || !strings.Contains(err.Error(), "cannot be written into a git credential helper") {
				t.Fatalf("expected the helper-slug refusal, got %v", err)
			}
			if called {
				t.Fatalf("git must not run for slug %q", slug)
			}
		})
	}
}

// TestAppPullRejectsUnsafeSlugDir pins F3: a server-controlled `slug` that, when
// used as the default target dir, would escape the current directory (path
// separator, `..`, or absolute path) MUST be rejected before any git command
// runs. An explicit user-supplied [dir] is unaffected.
func TestAppPullRejectsUnsafeSlugDir(t *testing.T) {
	for _, slug := range []string{"../evil", "/abs/path", "sub/dir", "..", "."} {
		t.Run(slug, func(t *testing.T) {
			info := okCloneInfo()
			info["slug"] = slug
			srv := cloneInfoServer(t, info, http.StatusOK, nil)
			defer srv.Close()

			called := false
			stubGit(t, func(dir string, args ...string) error { called = true; return nil })

			tmp := t.TempDir()
			chdir(t, tmp)
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("CIVITAI_TOKEN", "tok-1")
			t.Setenv("CIVITAI_BASE_URL", srv.URL)

			_, _, err := run(t, "app", "pull", "--app", "my-block")
			if err == nil {
				t.Fatalf("expected rejection of unsafe slug-derived dir %q", slug)
			}
			if !strings.Contains(err.Error(), "unsafe slug") {
				t.Errorf("error should explain the unsafe slug: %v", err)
			}
			if called {
				t.Errorf("git must NOT run for an unsafe slug-derived dir %q", slug)
			}
		})
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

// 🔴 Regression (red on the pre-change CLI, whose help called the identity
// "read-only" and documented a token in .git/config). The help is the published
// contract for what the token can do and where it lives, so the security
// paragraphs are pinned WHOLE (whitespace-normalised): a reword that drops the
// write scope or the reset line fails here instead of passing a keyword check.
func TestAppPullHelpStatesTheHelperAndTheWriteScope(t *testing.T) {
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	long := norm(newAppPullCmd().Long)
	for _, want := range []string{
		"Authentication uses your stored credential (`civitai login` or a personal API key). " +
			"The command calls an owner-only endpoint that lazily provisions your platform git identity, " +
			"then clones the TOKEN-LESS repository URL. That identity's token is a WRITE token " +
			"(scope write:repository): it can push to every app repository you have write access to, " +
			"not only this one, which is why it is kept out of the checkout. " +
			"(A push never deploys: it is queued for review.)",
		"NO TOKEN ON DISK: the clone's .git/config gets a repo-local git credential helper, scoped to " +
			"the platform git host, instead of a token in the remote URL: " +
			"[credential \"https://<git host>\"] helper = helper = !civitai git-credential --app <slug> useHttpPath = true " +
			"git runs `civitai git-credential` on each fetch/push; it asks the platform for your git " +
			"credential with your stored Civitai login and hands it to git only for this app's repository. " +
			"The empty `helper =` line stops any helper from your global git config (e.g. `store`) from " +
			"saving that credential to disk.",
		"OLDER CLONES: a checkout made by an earlier CLI stored the token in its remote URL. Running " +
			"`civitai app pull` in it removes the token from every remote that points at this app's " +
			"repository, installs the helper, and says so; a remote that carries a credential but points " +
			"elsewhere is left alone and the command prints the `git remote set-url` that would clean it. " +
			"The token itself is not rotated by this.",
	} {
		if !strings.Contains(long, want) {
			t.Errorf("app pull --help is missing the paragraph:\n  %s\n\ngot:\n  %s", want, long)
		}
	}
	for _, gone := range []string{"read-only", "TOKEN-IN-URL", "token lands on disk"} {
		if strings.Contains(long, gone) {
			t.Errorf("app pull --help still says %q", gone)
		}
	}
}

// Regression (audit round 1): migrating an old clone must not put the token on
// a git command line either — the rewrite matches the tokened value with a
// pattern, never by quoting it.
func TestAppPullMigrationKeepsTheTokenOffArgv(t *testing.T) {
	srv := cloneInfoServer(t, okCloneInfo(), http.StatusOK, nil)
	defer srv.Close()
	stubHelperExe(t)
	stubGitOutput(t, func(dir string, args ...string) (string, error) {
		return "remote.origin.url " + okCloneURL + "\n", nil
	})
	var gitCalls [][]string
	stubGit(t, func(dir string, args ...string) error {
		gitCalls = append(gitCalls, args)
		return nil
	})
	tmp := t.TempDir()
	chdir(t, tmp)
	if err := os.MkdirAll(filepath.Join(tmp, "my-block", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	_, errOut, err := run(t, "app", "pull", "--app", "my-block")
	if err != nil {
		t.Fatalf("app pull: %v", err)
	}
	want := []string{"-C", "my-block", "config", "--local", "--replace-all", "remote.origin.url", okHTTPURL,
		`^https://[^/@]+@forgejo\.civitai\.com/civitai-apps/my-block\.git$`}
	if !equalArgv(gitCalls[0], want) {
		t.Fatalf("migration call = %q, want %q", gitCalls[0], want)
	}
	assertNoToken(t, gitCalls)
	if !strings.Contains(errOut, `Removed the access token from remote "origin"`) {
		t.Fatalf("the migration must be reported: %s", errOut)
	}
}
