package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Real-git coverage for the credential helper: git itself runs `civitai
// git-credential` (this test binary, see TestMain) against a smart-HTTP git
// server (`git http-backend` behind Basic auth) that also answers the
// clone-info query. Nothing here is stubbed between git and the helper.

// testBinaryIsCLIEnv makes this test binary behave as the CLI (TestMain).
const testBinaryIsCLIEnv = "CIVITAI_CLI_TEST_BINARY_IS_CLI"

// runAsCLI is the CLI's main, minus the release plumbing: run args, print the
// error like the real binary does, and return the exit status.
func runAsCLI(args []string) int {
	root := NewRootCmd()
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	return 0
}

const (
	gitUser  = "dev-7"
	gitToken = "tok-sha1-real-git"
	gitSlug  = "my-block"
)

type gitServer struct {
	srv       *httptest.Server
	root      string
	authedOK  atomic.Int32 // git requests that carried the right Basic credential
	challenge atomic.Int32 // git requests answered 401 (no/wrong credential)
	cloneInfo atomic.Int32 // clone-info queries (the helper's, and app pull's own)
}

func (g *gitServer) repoURL() string { return g.srv.URL + "/civitai-apps/" + gitSlug + ".git" }

// newGitServer serves a bare repo holding one commit at /civitai-apps/my-block.git
// and the clone-info query at the CLI's tRPC path.
func newGitServer(t *testing.T) *gitServer {
	t.Helper()
	requireGit(t)
	gitFixtureEnv(t)
	t.Setenv("GIT_TERMINAL_PROMPT", "0")

	execPath, err := exec.Command("git", "--exec-path").Output()
	if err != nil {
		t.Fatalf("git --exec-path: %v", err)
	}
	backend := filepath.Join(strings.TrimSpace(string(execPath)), "git-http-backend")
	if _, err := os.Stat(backend); err != nil {
		// Not a skip: this file is the only real-git proof of the helper.
		t.Fatalf("git-http-backend not found at %s: %v", backend, err)
	}

	g := &gitServer{root: t.TempDir()}
	work := filepath.Join(t.TempDir(), "work")
	mustGit(t, "", "init", "-q", "-b", "main", work)
	if err := os.WriteFile(filepath.Join(work, "block.manifest.json"), []byte(`{"blockId":"my-block"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, work, "add", "block.manifest.json")
	mustGit(t, work, "commit", "-q", "-m", "first")
	bare := filepath.Join(g.root, "civitai-apps", gitSlug+".git")
	mustGit(t, "", "clone", "-q", "--bare", work, bare)

	cgiHandler := &cgi.Handler{
		Path: backend,
		Env:  []string{"GIT_PROJECT_ROOT=" + g.root, "GIT_HTTP_EXPORT_ALL=1", "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_SYSTEM=" + os.DevNull},
	}
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/trpc/blocks.getMyForgejoCloneInfo" {
			g.cloneInfo.Add(1)
			if r.Header.Get("Authorization") != "Bearer civitai-login-token" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"json": map[string]any{"message": "expired", "code": 401}}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"data": map[string]any{"json": map[string]any{
				"notYetAvailable": false,
				"slug":            gitSlug,
				"forgejoUsername": gitUser,
				"token":           gitToken,
				"httpUrl":         g.repoURL(),
				"cloneUrl":        strings.Replace(g.repoURL(), "http://", "http://"+gitUser+":"+gitToken+"@", 1),
			}}}})
			return
		}
		u, p, ok := r.BasicAuth()
		if !ok || u != gitUser || p != gitToken {
			g.challenge.Add(1)
			w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		g.authedOK.Add(1)
		cgiHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(g.srv.Close)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "civitai-login-token")
	t.Setenv("CIVITAI_BASE_URL", g.srv.URL)
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "1")
	t.Setenv(testBinaryIsCLIEnv, "1")

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	orig := helperExecutable
	helperExecutable = func() string { return shellQuote(self) }
	t.Cleanup(func() { helperExecutable = orig })
	return g
}

// useRecordingGlobalHelper installs a GLOBAL git config whose credential helper
// is `store` writing to a file — the configuration that would put the token on
// disk if the repo-local reset entry were missing. Returns that file's path.
func useRecordingGlobalHelper(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	store := filepath.Join(dir, "global-store")
	cfg := filepath.Join(dir, "gitconfig")
	if err := os.WriteFile(cfg, []byte("[credential]\n\thelper = store --file="+store+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	return store
}

func gitOut(t *testing.T, dir string, stdin string, args ...string) (string, string, error) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	done := make(chan error, 1)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { done <- c.Wait() }()
	select {
	case err := <-done:
		return out.String(), errb.String(), err
	case <-time.After(30 * time.Second):
		_ = c.Process.Kill()
		t.Fatalf("git %v did not finish in 30s (a prompt or a hang)", args)
		return "", "", nil
	}
}

func readFileString(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// 🔴 Regression for the clone path (red on the pre-change CLI: it cloned the
// tokened URL, so .git/config held the token and no helper existed).
func TestAppPullRealGitCloneStoresNoToken(t *testing.T) {
	g := newGitServer(t)
	leak := useRecordingGlobalHelper(t)
	cwd := t.TempDir()
	chdir(t, cwd)

	out, errOut, err := run(t, "app", "pull", "--app", gitSlug)
	if err != nil {
		t.Fatalf("app pull: %v\n%s", err, errOut)
	}
	if _, err := os.Stat(filepath.Join(cwd, gitSlug, "block.manifest.json")); err != nil {
		t.Fatalf("the clone did not check out the repository: %v", err)
	}
	cfgText := readFileString(t, filepath.Join(cwd, gitSlug, ".git", "config"))
	if strings.Contains(cfgText, gitToken) {
		t.Fatalf(".git/config holds the token:\n%s", cfgText)
	}
	// (The positive control for this byte search is in the migration test: the
	// same search finds the token in an old clone's .git/config.)
	if strings.Contains(out+errOut, gitToken) {
		t.Fatalf("app pull printed the token:\n%s\n%s", out, errOut)
	}
	origin, _, _ := gitOut(t, filepath.Join(cwd, gitSlug), "", "config", "--get", "remote.origin.url")
	if strings.TrimSpace(origin) != g.repoURL() {
		t.Fatalf("remote.origin.url = %q, want the token-less %q", strings.TrimSpace(origin), g.repoURL())
	}
	helpers, _, _ := gitOut(t, filepath.Join(cwd, gitSlug), "", "config", "--local", "--get-all", "credential."+g.srv.URL+".helper")
	wantHelpers := "\n!" + helperExecutable() + " git-credential --app " + gitSlug + "\n"
	if helpers != wantHelpers {
		t.Fatalf("credential helpers = %q, want %q (empty reset FIRST, then ours)", helpers, wantHelpers)
	}
	// The clone authenticated through the helper: git was challenged, then
	// presented the credential the helper handed it.
	if g.challenge.Load() == 0 || g.authedOK.Load() == 0 {
		t.Fatalf("expected a 401 challenge then authed requests; challenge=%d authed=%d", g.challenge.Load(), g.authedOK.Load())
	}
	// 🔴 The global `store` helper never saw the token (E11). Without the empty
	// reset entry git would have called it on success and written the file.
	if _, err := os.Stat(leak); !os.IsNotExist(err) {
		t.Fatalf("the global credential helper stored the token (%s exists: %v)", leak, err)
	}

	// A later plain `git fetch` in the checkout keeps working through the helper.
	before := g.authedOK.Load()
	if _, stderr, err := gitOut(t, filepath.Join(cwd, gitSlug), "", "fetch", "origin"); err != nil {
		t.Fatalf("git fetch in the clone: %v\n%s", err, stderr)
	}
	if g.authedOK.Load() == before {
		t.Fatal("git fetch did not reach the server authenticated")
	}
	if _, err := os.Stat(leak); !os.IsNotExist(err) {
		t.Fatalf("the global credential helper stored the token after fetch")
	}
}

// Mutation partner of the test above: the SAME clone with the reset entry
// removed from the config `app pull` writes DOES leak into the global store.
// This proves the leak assertion above can go red, i.e. that it measures the
// reset entry and not an environment where `store` never runs.
func TestGlobalStoreLeaksWithoutTheResetEntry(t *testing.T) {
	g := newGitServer(t)
	leak := useRecordingGlobalHelper(t)
	dst := filepath.Join(t.TempDir(), "clone")
	key := "credential." + g.srv.URL
	_, stderr, err := gitOut(t, "", "", "clone", "-q",
		"-c", key+".helper=!"+helperExecutable()+" git-credential --app "+gitSlug,
		"-c", key+".useHttpPath=true",
		"--", g.repoURL(), dst)
	if err != nil {
		t.Fatalf("clone: %v\n%s", err, stderr)
	}
	if !strings.Contains(readFileString(t, leak), gitToken) {
		t.Fatalf("control failed: without the reset the global store should hold the token")
	}
}

// 🔴 Regression for the migration of an old clone (red pre-change: a sync
// passed the tokened URL to `git fetch` and left .git/config untouched).
func TestAppPullRealGitMigratesOldTokenClone(t *testing.T) {
	g := newGitServer(t)
	leak := useRecordingGlobalHelper(t)
	cwd := t.TempDir()
	chdir(t, cwd)
	checkout := filepath.Join(cwd, gitSlug)

	// What an older CLI left behind: the token in origin's URL, no helper.
	tokened := strings.Replace(g.repoURL(), "http://", "http://"+gitUser+":"+gitToken+"@", 1)
	if _, stderr, err := gitOut(t, "", "", "clone", "-q", "--", tokened, checkout); err != nil {
		t.Fatalf("seed clone: %v\n%s", err, stderr)
	}
	// A second remote with a credential that is NOT this app's repository.
	other := "https://someone:secret@example.invalid/elsewhere.git"
	mustGit(t, checkout, "remote", "add", "other", other)
	if !strings.Contains(readFileString(t, filepath.Join(checkout, ".git", "config")), gitToken) {
		t.Fatal("control: the seeded clone must hold the token in .git/config")
	}
	// The seed clone itself ran with the token in its URL; the global store
	// recorded it then. Reset that so only `app pull` is measured.
	_ = os.Remove(leak)

	_, errOut, err := run(t, "app", "pull", "--app", gitSlug)
	if err != nil {
		t.Fatalf("app pull (sync): %v\n%s", err, errOut)
	}
	cfgText := readFileString(t, filepath.Join(checkout, ".git", "config"))
	if strings.Contains(cfgText, gitToken) {
		t.Fatalf("the token is still in .git/config after migration:\n%s", cfgText)
	}
	origin, _, _ := gitOut(t, checkout, "", "config", "--get", "remote.origin.url")
	if strings.TrimSpace(origin) != g.repoURL() {
		t.Fatalf("origin = %q, want %q", strings.TrimSpace(origin), g.repoURL())
	}
	// The unrelated remote is untouched, and the user is told how to clean it.
	otherURL, _, _ := gitOut(t, checkout, "", "config", "--get", "remote.other.url")
	if strings.TrimSpace(otherURL) != other {
		t.Fatalf("remote other was rewritten: %q", otherURL)
	}
	if !strings.Contains(errOut, `Removed the access token from remote "origin"`) {
		t.Fatalf("the migration must say what it did: %s", errOut)
	}
	if !strings.Contains(errOut, `Remote "other"`) || !strings.Contains(errOut, "remote set-url other https://example.invalid/elsewhere.git") {
		t.Fatalf("the leftover credential remote must be reported with its fix: %s", errOut)
	}
	if strings.Contains(errOut, "secret") || strings.Contains(errOut, gitToken) {
		t.Fatalf("a credential was printed: %s", errOut)
	}
	if _, err := os.Stat(leak); !os.IsNotExist(err) {
		t.Fatalf("the global credential helper stored the token during the migrated sync")
	}

	// Idempotent: a second pull rewrites nothing and leaves config identical.
	_, errOut2, err := run(t, "app", "pull", "--app", gitSlug)
	if err != nil {
		t.Fatalf("second app pull: %v\n%s", err, errOut2)
	}
	if strings.Contains(errOut2, "Removed the access token") {
		t.Fatalf("second pull claims a migration: %s", errOut2)
	}
	if after := readFileString(t, filepath.Join(checkout, ".git", "config")); after != cfgText {
		t.Fatalf("second pull changed .git/config:\nbefore:\n%s\nafter:\n%s", cfgText, after)
	}
}

// The helper protocol exercised by git itself (`git credential fill`) in a
// checkout `app pull` configured: the matching repository is answered, a
// different repository on the same host is refused (git quits rather than
// prompting), and an expired civitai login is refused with a `civitai login`
// hint on stderr.
func TestGitCredentialFillThroughRealGit(t *testing.T) {
	g := newGitServer(t)
	cwd := t.TempDir()
	chdir(t, cwd)
	if _, errOut, err := run(t, "app", "pull", "--app", gitSlug); err != nil {
		t.Fatalf("app pull: %v\n%s", err, errOut)
	}
	checkout := filepath.Join(cwd, gitSlug)
	host := strings.TrimPrefix(g.srv.URL, "http://")

	out, stderr, err := gitOut(t, checkout, "protocol=http\nhost="+host+"\npath=civitai-apps/"+gitSlug+".git\n\n", "credential", "fill")
	if err != nil {
		t.Fatalf("credential fill: %v\n%s", err, stderr)
	}
	if !strings.Contains(out, "username="+gitUser+"\n") || !strings.Contains(out, "password="+gitToken+"\n") {
		t.Fatalf("fill output = %q", out)
	}

	// Wrong slug: another repository on the same host.
	out, stderr, err = gitOut(t, checkout, "protocol=http\nhost="+host+"\npath=civitai-apps/other-block.git\n\n", "credential", "fill")
	if err == nil || strings.Contains(out, gitToken) {
		t.Fatalf("fill for another repository must fail without the token; out=%q err=%v", out, err)
	}
	if !strings.Contains(stderr, "not answering") || !strings.Contains(stderr, "told us to quit") {
		t.Fatalf("stderr should carry the helper's refusal and git's quit: %s", stderr)
	}

	// Expired civitai login: the clone-info query 401s.
	t.Setenv("CIVITAI_TOKEN", "expired")
	out, stderr, err = gitOut(t, checkout, "protocol=http\nhost="+host+"\npath=civitai-apps/"+gitSlug+".git\n\n", "credential", "fill")
	if err == nil || strings.Contains(out, gitToken) {
		t.Fatalf("an expired login must not yield a credential; out=%q err=%v", out, err)
	}
	if !strings.Contains(stderr, "civitai login") {
		t.Fatalf("stderr should tell the user to log in again: %s", stderr)
	}

	// No login at all.
	t.Setenv("CIVITAI_TOKEN", "")
	_, stderr, err = gitOut(t, checkout, "protocol=http\nhost="+host+"\npath=civitai-apps/"+gitSlug+".git\n\n", "credential", "fill")
	if err == nil || !strings.Contains(stderr, "no token configured") {
		t.Fatalf("no login: err=%v stderr=%s", err, stderr)
	}
}
