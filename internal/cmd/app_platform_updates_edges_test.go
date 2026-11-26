package cmd

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/civitai/cli/internal/appapi"
)

// Edges of the staff platform-update flow (app_platform_updates.go): a merge
// git cannot commit, a checkout with no shared history, a detached HEAD, a
// stalled fetch, the dirty check's scope, the printed command lines, and the
// server's own platform_update_missing refusal of a submit.

// puStub is the server half without a server, for calls of the flow itself.
type puStub struct {
	mu       sync.Mutex
	updates  []appapi.PlatformUpdate
	verified bool
	resolves []puResolve
}

func (s *puStub) ListPendingPlatformUpdates(context.Context, string) ([]appapi.PlatformUpdate, error) {
	return s.updates, nil
}

func (s *puStub) VerifyPlatformUpdateCommit(context.Context, string, string) (bool, error) {
	return s.verified, nil
}

func (s *puStub) ResolvePlatformUpdate(_ context.Context, id, outcome, mergeSha string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resolves = append(s.resolves, puResolve{id, outcome, mergeSha})
	return nil
}

func (s *puStub) resolved() []puResolve {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]puResolve(nil), s.resolves...)
}

func puAPIUpdate(sha string, paths ...string) appapi.PlatformUpdate {
	u := appapi.PlatformUpdate{ID: puID, Branch: puBranch, CommitSha: sha, Reason: puReason, Status: appapi.PlatformUpdateStatusPending}
	for _, p := range paths {
		u.Paths = append(u.Paths, appapi.PlatformUpdatePath{Path: p, Operation: "update"})
	}
	return u
}

// puGitPrefix is the expected `git -C <dir> -c <helper>…` of a printed command
// in the e2e environment (the helper is this test binary, already quoted by
// helperExecutable). The literal spelling is pinned separately, by
// TestPlatformUpdatePrintedCommandsAreShellQuoted.
func puGitPrefix(e *puEnv, dir string) string {
	ctx := e.g.srv.URL
	return "git -C " + shellQuote(dir) +
		" -c " + shellQuote("credential."+ctx+".helper=") +
		" -c " + shellQuote("credential."+ctx+".helper=!"+helperExecutable()+" git-credential --app "+gitSlug) +
		" -c " + shellQuote("credential."+ctx+".useHttpPath=true")
}

func puHandMerge(e *puEnv, dir, sha string) string {
	return puGitPrefix(e, dir) + " fetch " + shellQuote(e.g.sourceURL()) + " " + puBranch + " && git -C " + shellQuote(dir) + " merge " + sha
}

func puCherryPick(e *puEnv, dir, sha string) string {
	return puGitPrefix(e, dir) + " fetch " + shellQuote(e.g.sourceURL()) + " " + sha + " && git -C " + shellQuote(dir) + " cherry-pick " + sha
}

// printedCommand returns the one output line that starts with "  git -C ".
func printedCommand(t *testing.T, out string) string {
	t.Helper()
	var found []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "  git -C ") {
			found = append(found, strings.TrimPrefix(l, "  "))
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one printed git command, got %d:\n%s", len(found), out)
	}
	return found[0]
}

func puRunShell(t *testing.T, dir, line string) (string, error) {
	t.Helper()
	c := exec.Command("sh", "-c", line)
	c.Dir = dir
	b, err := c.CombinedOutput()
	return string(b), err
}

func hasMergeHead(t *testing.T, dir string) bool {
	t.Helper()
	return exec.Command("git", "-C", dir, "rev-parse", "-q", "--verify", "MERGE_HEAD").Run() == nil
}

// The printed commands, LITERALLY: the directory (with a space and a quote)
// and the URL single-quoted, and the credential helper carried with -c so the
// fetch authenticates in a checkout `civitai app pull` never configured.
func TestPlatformUpdatePrintedCommandsAreShellQuoted(t *testing.T) {
	orig := helperExecutable
	helperExecutable = func() string { return "civitai" }
	t.Cleanup(func() { helperExecutable = orig })
	const url = "https://git.civitai.com/civitai-apps-src/my-block.git"
	cfg, err := credentialHelperConfig(url, "my-block")
	if err != nil {
		t.Fatal(err)
	}
	r := &platformUpdateRun{dir: "/home/dev/my apps/it's-mine", slug: "my-block", sourceURL: url, helperCfg: cfg}
	const sha = "0123456789abcdef0123456789abcdef01234567"
	prefix := `git -C '/home/dev/my apps/it'\''s-mine'` +
		` -c 'credential.https://git.civitai.com.helper='` +
		` -c 'credential.https://git.civitai.com.helper=!civitai git-credential --app my-block'` +
		` -c 'credential.https://git.civitai.com.useHttpPath=true'`
	if got, want := r.handMergeCommand("platform-update/apu_x", sha), prefix+
		` fetch 'https://git.civitai.com/civitai-apps-src/my-block.git' platform-update/apu_x`+
		` && git -C '/home/dev/my apps/it'\''s-mine' merge `+sha; got != want {
		t.Fatalf("hand-merge command =\n%s\nwant\n%s", got, want)
	}
	if got, want := r.cherryPickCommand(sha), prefix+
		` fetch 'https://git.civitai.com/civitai-apps-src/my-block.git' `+sha+
		` && git -C '/home/dev/my apps/it'\''s-mine' cherry-pick `+sha; got != want {
		t.Fatalf("cherry-pick command =\n%s\nwant\n%s", got, want)
	}
}

// 🔴 The printed hand-merge command WORKS as printed, from a directory whose
// path has a space, in a checkout with no credential helper (a plain clone).
// The control: the same command without the -c options cannot even fetch.
func TestPlatformUpdateHandMergeCommandRunsInACheckoutWithoutTheHelper(t *testing.T) {
	e := newPUEnv(t, map[string]string{"shared.txt": "base\n"})
	sha := e.staffCommit(t, "main", puBranch, "Platform update", map[string]string{"shared.txt": "staff\n"})
	dir := filepath.Join(t.TempDir(), "my apps", gitSlug)
	mustGit(t, "", "clone", "-q", e.srcBare(), dir)
	if err := os.WriteFile(filepath.Join(dir, "shared.txt"), []byte("owner\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "commit", "-q", "-am", "owner")

	stub := &puStub{verified: true}
	var out bytes.Buffer
	_, err := mergePlatformUpdates(context.Background(), stub, &out, dir, gitSlug,
		func(context.Context) (string, error) { return e.g.sourceURL(), nil }, []appapi.PlatformUpdate{puAPIUpdate(sha, "shared.txt")})
	if !errors.Is(err, ErrUnmergedPlatformUpdate) {
		t.Fatalf("want a conflict, got %v\n%s", err, out.String())
	}
	line := printedCommand(t, out.String())
	if want := puHandMerge(e, dir, sha); line != want {
		t.Fatalf("printed =\n%s\nwant\n%s", line, want)
	}

	bare := "git -C " + shellQuote(dir) + " fetch " + shellQuote(e.g.sourceURL()) + " " + puBranch + " && git -C " + shellQuote(dir) + " merge " + sha
	if o, err := puRunShell(t, t.TempDir(), bare); err == nil || hasMergeHead(t, dir) {
		t.Fatalf("CONTROL: without the helper the fetch was expected to fail (err %v, MERGE_HEAD %v):\n%s", err, hasMergeHead(t, dir), o)
	}
	o, _ := puRunShell(t, t.TempDir(), line) // the merge itself conflicts: non-zero is expected
	if !hasMergeHead(t, dir) {
		t.Fatalf("the printed command did not get as far as the merge:\n%s", o)
	}
}

// 🔴 Y1: a merge that git stops with MERGE_HEAD set but NO conflicted file (a
// failing commit signer here) is not a conflict: aborted, reported to nobody,
// no hand-merge instructions. Control: the same checkout without the broken
// signer merges and reports.
func TestPlatformUpdateMergeGitCannotCommitIsNotAConflict(t *testing.T) {
	e := newPUEnv(t, nil)
	sha := e.staffCommit(t, "main", puBranch, "Platform update", map[string]string{"UPDATE.md": "from staff\n"})
	e.fake.pending = []map[string]any{puUpdate(sha, false)}
	e.fake.verified[sha] = true
	before := e.head(t)
	mustGit(t, e.dir, "config", "commit.gpgsign", "true")
	mustGit(t, e.dir, "config", "gpg.program", "false")

	out, _, err := e.pull(t)
	if !errors.Is(err, ErrUnmergedPlatformUpdate) {
		t.Fatalf("want ErrUnmergedPlatformUpdate, got %v\n%s", err, out)
	}
	if want := "git could not complete the merge of platform update " + puID + " (commit " + sha + ") into my-block (the merge was aborted): "; !strings.Contains(err.Error(), want) {
		t.Fatalf("error does not say git could not complete the merge:\n%s", err)
	}
	if got := e.fake.resolved(); len(got) != 0 {
		t.Fatalf("resolve called for a merge git could not commit: %+v", got)
	}
	if strings.Contains(out, "conflicts with") || strings.Contains(out, "To merge it by hand") {
		t.Fatalf("reported as a conflict:\n%s", out)
	}
	if head := e.head(t); head != before || hasMergeHead(t, e.dir) {
		t.Fatalf("HEAD %s (want %s), MERGE_HEAD %v: the merge was not aborted", head, before, hasMergeHead(t, e.dir))
	}
	if st := strings.TrimSpace(mustGitOut(t, e.dir, "status", "--porcelain")); st != "" {
		t.Fatalf("left dirty:\n%s", st)
	}

	mustGit(t, e.dir, "config", "commit.gpgsign", "false")
	if out, errOut, err := e.pull(t); err != nil {
		t.Fatalf("CONTROL: without the broken signer the merge must succeed: %v\n%s\n%s", err, out, errOut)
	}
	if got := e.fake.resolved(); len(got) != 1 || got[0].Outcome != "merged" {
		t.Fatalf("CONTROL: resolve calls = %+v, want one merged", got)
	}
}

// newPUBuildCheckout is a checkout of the app's BUILD repository: pulled while
// the app had no source repository, then the source repository appears. With
// fork, the source repository is a fork of the build repository (how a newly
// seeded source repository starts: shared history); without, it is the fixture's
// own unrelated history.
func newPUBuildCheckout(t *testing.T, fork bool) *puEnv {
	t.Helper()
	g := newGitServerOpts(t, true)
	e := &puEnv{g: g, fake: &puFake{verified: map[string]bool{}}}
	g.extra = e.fake.handle
	if fork {
		if err := os.RemoveAll(e.srcBare()); err != nil {
			t.Fatal(err)
		}
		mustGit(t, "", "clone", "-q", "--bare", filepath.Join(g.root, "civitai-apps", gitSlug+".git"), e.srcBare())
		mustGit(t, e.srcBare(), "config", "http.receivepack", "true")
	}
	g.withSource.Store(false)
	e.cwd = t.TempDir()
	chdir(t, e.cwd)
	if _, errOut, err := run(t, "app", "pull", "--app", gitSlug); err != nil {
		t.Fatalf("initial app pull: %v\n%s", err, errOut)
	}
	g.withSource.Store(true)
	e.dir = filepath.Join(e.cwd, gitSlug)
	if got := strings.TrimSpace(mustGitOut(t, e.dir, "remote", "get-url", "origin")); got != g.repoURL() {
		t.Fatalf("CONTROL: origin = %s, want the build repository %s", got, g.repoURL())
	}
	return e
}

// 🔴 Y2: a build-repo checkout that shares no history with the source
// repository is not merged into and nothing is reported; the message says why
// and prints a cherry-pick by sha — which works as printed.
func TestPlatformUpdateUnrelatedHistoryIsNotMerged(t *testing.T) {
	e := newPUBuildCheckout(t, false)
	sha := e.staffCommit(t, "main", puBranch, "Platform update", map[string]string{"UPDATE.md": "from staff\n"})
	e.fake.pending = []map[string]any{puUpdate(sha, false)}
	e.fake.verified[sha] = true
	before := e.head(t)

	out, errOut, err := e.pull(t)
	if !errors.Is(err, ErrUnmergedPlatformUpdate) {
		t.Fatalf("want ErrUnmergedPlatformUpdate, got %v\n%s\n%s", err, out, errOut)
	}
	if !strings.Contains(errOut, "this checkout tracks the build repository") {
		t.Fatalf("CONTROL: not the build-checkout path:\n%s", errOut)
	}
	wantErr := "unmerged platform update: platform update " + puID + " was not merged: this checkout shares no history with my-block's source repository, where the update lives (instructions above)"
	if err.Error() != wantErr {
		t.Fatalf("error =\n%s\nwant\n%s", err, wantErr)
	}
	want := "Platform update " + puID + " (commit " + sha + ") is in the history of my-block's source repository, which this checkout of my-block does not share (git finds no common ancestor), so nothing was merged.\n"
	if !strings.Contains(out, want) {
		t.Fatalf("want:\n%s\ngot:\n%s", want, out)
	}
	line := printedCommand(t, out)
	if wantLine := puCherryPick(e, "my-block", sha); line != wantLine {
		t.Fatalf("printed =\n%s\nwant\n%s", line, wantLine)
	}
	if got := e.fake.resolved(); len(got) != 0 {
		t.Fatalf("resolve called for an unrelated history: %+v", got)
	}
	if head := e.head(t); head != before || hasMergeHead(t, e.dir) {
		t.Fatalf("HEAD moved or a merge was left (HEAD %s, want %s)", head, before)
	}

	if o, err := puRunShell(t, e.cwd, line); err != nil {
		t.Fatalf("the printed cherry-pick failed: %v\n%s", err, o)
	}
	if b, err := os.ReadFile(filepath.Join(e.dir, "UPDATE.md")); err != nil || string(b) != "from staff\n" {
		t.Fatalf("the cherry-pick did not apply the update: %q %v", b, err)
	}
}

// Y2's other half: a build checkout whose source repository is a FORK of the
// build repository (shared history) merges normally and reports it.
func TestPlatformUpdateBuildCheckoutWithSharedHistoryMerges(t *testing.T) {
	e := newPUBuildCheckout(t, true)
	sha := e.staffCommit(t, "main", puBranch, "Platform update", map[string]string{"UPDATE.md": "from staff\n"})
	e.fake.pending = []map[string]any{puUpdate(sha, false)}
	e.fake.verified[sha] = true

	out, errOut, err := e.pull(t)
	if err != nil {
		t.Fatalf("pull: %v\n%s\n%s", err, out, errOut)
	}
	if !strings.Contains(errOut, "this checkout tracks the build repository") {
		t.Fatalf("CONTROL: not the build-checkout path:\n%s", errOut)
	}
	merge := e.head(t)
	if !strings.Contains(out, "Merged platform update "+puID+" (commit "+sha+") into my-block.\n") {
		t.Fatalf("not merged:\n%s", out)
	}
	if got := e.fake.resolved(); len(got) != 1 || got[0] != (puResolve{puID, "merged", merge}) {
		t.Fatalf("resolve calls = %+v, want merged with %s", got, merge)
	}
}

// Y2 on submit: unrelated history is unmerged, so the submit refuses naming
// the override.
func TestSubmitRefusesFromACheckoutWithUnrelatedHistory(t *testing.T) {
	e := newPUBuildCheckout(t, false)
	sha := e.staffCommit(t, "main", puBranch, "Platform update", map[string]string{"UPDATE.md": "x\n"})
	stub := &puStub{updates: []appapi.PlatformUpdate{puAPIUpdate(sha)}}
	cloneInfo := func(context.Context, string) (*appapi.ForgejoCloneInfo, error) {
		return &appapi.ForgejoCloneInfo{SourceHTTPURL: e.g.sourceURL()}, nil
	}
	var warn bytes.Buffer
	_, err := submitPlatformUpdates(context.Background(), stub, cloneInfo, &warn, e.dir, gitSlug, false)
	if !errors.Is(err, ErrUnmergedPlatformUpdate) {
		t.Fatalf("want ErrUnmergedPlatformUpdate, got %v", err)
	}
	if !strings.Contains(err.Error(), "shares no history with my-block's source repository") ||
		!strings.Contains(err.Error(), "pass --allow-unmerged-platform-update to submit without it") {
		t.Fatalf("the refusal must say why and name the override:\n%s", err)
	}
	if len(stub.resolved()) != 0 {
		t.Fatalf("resolve called: %+v", stub.resolved())
	}
}

// 🔴 Y3: from a detached HEAD the update is merged locally (so a bundle would
// carry it) but never reported merged — the merge commit is on no branch. A
// later run from a branch that contains it reports it.
func TestPlatformUpdateDetachedHeadMergesButIsNotReported(t *testing.T) {
	e := newPUEnv(t, nil)
	sha := e.staffCommit(t, "main", puBranch, "Platform update", map[string]string{"UPDATE.md": "from staff\n"})
	e.fake.pending = []map[string]any{puUpdate(sha, false)}
	e.fake.verified[sha] = true
	mustGit(t, e.dir, "checkout", "-q", "--detach")
	before := e.head(t)

	out, errOut, err := e.pull(t)
	if err != nil {
		t.Fatalf("pull: %v\n%s\n%s", err, out, errOut)
	}
	merge := e.head(t)
	if parents := strings.Fields(mustGitOut(t, e.dir, "rev-list", "--parents", "-n", "1", "HEAD")); merge == before || len(parents) != 3 || parents[2] != sha {
		t.Fatalf("not merged locally: HEAD parents %v", parents)
	}
	want := "  HEAD is detached, so the merge commit " + merge + " exists only in this checkout, on no branch: push it or merge it into your branch (e.g. `git -C 'my-block' branch <name> " + merge + "`). " +
		"Civitai was not told it merged; the next pull or submit from a branch that contains it reports it.\n"
	if !strings.Contains(out, want) {
		t.Fatalf("want:\n%s\ngot:\n%s", want, out)
	}
	if got := e.fake.resolved(); len(got) != 0 {
		t.Fatalf("reported merged from a detached HEAD: %+v", got)
	}

	// Still detached: already in the history, still not reported.
	out, _, err = e.pull(t)
	if err != nil || !strings.Contains(out, "  HEAD is detached, so Civitai was not told it merged; run the command again from a branch that contains it.\n") {
		t.Fatalf("second detached pull: %v\n%s", err, out)
	}
	if got := e.fake.resolved(); len(got) != 0 {
		t.Fatalf("reported merged from a detached HEAD: %+v", got)
	}

	// Control: on a branch that contains it, it is reported.
	mustGit(t, e.dir, "checkout", "-q", "-B", "main", merge)
	if _, errOut, err := e.pull(t); err != nil {
		t.Fatalf("pull on a branch: %v\n%s", err, errOut)
	}
	if got := e.fake.resolved(); len(got) != 1 || got[0] != (puResolve{puID, "merged", merge}) {
		t.Fatalf("CONTROL: resolve calls = %+v, want merged with %s", got, merge)
	}
}

// G4: a fetch is bounded by gitNetworkTimeout. The server accepts the
// connection and never answers.
func TestPlatformGitFetchIsBoundedByTheNetworkTimeout(t *testing.T) {
	requireGit(t)
	gitFixtureEnv(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		ln.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			c.Close()
		}
	})
	orig := gitNetworkTimeout
	gitNetworkTimeout = 300 * time.Millisecond
	t.Cleanup(func() { gitNetworkTimeout = orig })
	repo := t.TempDir()
	mustGit(t, "", "init", "-q", repo)

	type result struct {
		out string
		err error
	}
	done := make(chan result, 1)
	start := time.Now()
	go func() {
		out, err := platformGit(repo, "-c", "credential.helper=", "fetch", "--no-tags", "--", "http://"+ln.Addr().String()+"/x.git", "main")
		done <- result{out, err}
	}()
	select {
	case r := <-done:
		if r.err == nil {
			t.Fatalf("a fetch from a server that never answers succeeded:\n%s", r.out)
		}
		if !strings.Contains(r.out, "fatal: git fetch timed out after 300ms") {
			t.Fatalf("output does not name the timeout:\n%s", r.out)
		}
		if el := time.Since(start); el > 5*time.Second {
			t.Fatalf("took %s against a 300ms timeout", el)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("platformGit fetch is not bounded: still running after 10s against a 300ms gitNetworkTimeout")
	}
}

// G6: the dirty check is scoped to the app directory, as submit's is. An app
// inside a larger repository (apps/my-block) merges past a sibling's
// uncommitted work; a dirty file the merge would touch still blocks, with a
// clean message; a dirty file in the app itself blocks as before.
func TestPlatformUpdateDirtyCheckIsScopedToTheAppDirectory(t *testing.T) {
	seed := map[string]string{"apps/my-block/app.txt": "a\n", "other/y.txt": "y\n"}
	setup := func(t *testing.T, files map[string]string) (*puEnv, string, string) {
		e := newPUEnv(t, seed)
		sha := e.staffCommit(t, "main", puBranch, "Platform update", files)
		return e, sha, filepath.Join(e.dir, "apps", "my-block")
	}
	merge := func(e *puEnv, appDir, sha string, stub *puStub) (int, string, error) {
		var out bytes.Buffer
		n, err := mergePlatformUpdates(context.Background(), stub, &out, appDir, gitSlug,
			func(context.Context) (string, error) { return e.g.sourceURL(), nil }, []appapi.PlatformUpdate{puAPIUpdate(sha)})
		return n, out.String(), err
	}
	write := func(t *testing.T, path, c string) {
		if err := os.WriteFile(path, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	read := func(path string) string { b, _ := os.ReadFile(path); return string(b) }

	t.Run("a sibling's uncommitted work does not block", func(t *testing.T) {
		e, sha, appDir := setup(t, map[string]string{"apps/my-block/UPDATE.md": "staff\n"})
		write(t, filepath.Join(e.dir, "other", "y.txt"), "wip\n")
		stub := &puStub{}
		n, out, err := merge(e, appDir, sha, stub)
		if err != nil || n != 1 {
			t.Fatalf("merged %d, err %v\n%s", n, err, out)
		}
		if got := read(filepath.Join(e.dir, "other", "y.txt")); got != "wip\n" {
			t.Fatalf("the sibling's work was touched: %q", got)
		}
		if got := stub.resolved(); len(got) != 1 || got[0].Outcome != "merged" {
			t.Fatalf("resolve calls = %+v", got)
		}
	})
	t.Run("a dirty file the merge would touch blocks, cleanly", func(t *testing.T) {
		e, sha, appDir := setup(t, map[string]string{"other/y.txt": "staff\n"})
		write(t, filepath.Join(e.dir, "other", "y.txt"), "wip\n")
		stub := &puStub{}
		_, out, err := merge(e, appDir, sha, stub)
		want := "unmerged platform update: platform update " + puID + " (commit " + sha + ") was not merged into " + appDir +
			": it would overwrite uncommitted changes in other/y.txt — commit or discard them, then run the command again"
		if err == nil || err.Error() != want {
			t.Fatalf("error =\n%v\nwant\n%s\n%s", err, want, out)
		}
		if got := read(filepath.Join(e.dir, "other", "y.txt")); got != "wip\n" || hasMergeHead(t, e.dir) {
			t.Fatalf("y.txt %q, MERGE_HEAD %v", got, hasMergeHead(t, e.dir))
		}
		if len(stub.resolved()) != 0 {
			t.Fatalf("resolve called: %+v", stub.resolved())
		}
	})
	t.Run("a dirty file in the app blocks (invariant)", func(t *testing.T) {
		e, sha, appDir := setup(t, map[string]string{"apps/my-block/UPDATE.md": "staff\n"})
		write(t, filepath.Join(appDir, "app.txt"), "wip\n")
		stub := &puStub{}
		_, out, err := merge(e, appDir, sha, stub)
		want := "unmerged platform update: " + appDir + " has uncommitted changes, so 1 platform update(s) from Civitai staff were not merged into it — commit or discard your changes, then run the command again"
		if err == nil || err.Error() != want {
			t.Fatalf("error =\n%v\nwant\n%s\n%s", err, want, out)
		}
	})
}

// The server's own check: a submit the server refuses with
// platform_update_missing is explained — the listed updates on stderr, the
// server's message and the override in the error — and an unknown details
// shape still prints the server's message.
func TestSubmitExplainsTheServersPlatformUpdateMissingRefusal(t *testing.T) {
	const tail = "\nsubmit refused by the server: the bundle is missing a staff platform update. Run `civitai app pull` in a checkout of the app's source repository to merge it, " +
		"then submit again — or pass --allow-unmerged-platform-update to submit without it (the override is recorded for the moderator reviewing the submission)"
	t.Run("with details", func(t *testing.T) {
		e := newPUEnv(t, nil)
		e.fake.submitStatus = http.StatusBadRequest
		e.fake.submitResp = `{"code":"platform_update_missing","message":"This bundle is missing 1 platform update.",` +
			`"details":{"updates":[{"id":"apu_1","reason":"SDK bump","by":{"username":"mod-jane"},"paths":["package.json"]}]}}`
		_, errOut, err := run(t, "app", "submit", puSubmitDir(t), "--yes")
		if !errors.Is(err, appapi.ErrPlatformUpdateMissing) {
			t.Fatalf("want ErrPlatformUpdateMissing, got %v\n%s", err, errOut)
		}
		if want := "server returned 400: This bundle is missing 1 platform update." + tail; err.Error() != want {
			t.Fatalf("error =\n%s\nwant\n%s", err, want)
		}
		want := "Civitai refused this submit: the bundle does not contain 1 platform update(s) from Civitai staff that no approved version has shipped yet:\n" +
			"  apu_1 from mod-jane: SDK bump\n    Files: package.json\n"
		if !strings.Contains(errOut, want) {
			t.Fatalf("want on stderr:\n%s\ngot:\n%s", want, errOut)
		}
		if strings.Contains(errOut, "civitai feedback send") {
			t.Fatalf("the platform-friction hint was printed for a refusal the caller can fix:\n%s", errOut)
		}
	})
	t.Run("unknown details shape", func(t *testing.T) {
		e := newPUEnv(t, nil)
		e.fake.submitStatus = http.StatusConflict
		e.fake.submitResp = `{"code":"platform_update_missing","message":"Missing a platform update.","details":"v2"}`
		_, errOut, err := run(t, "app", "submit", puSubmitDir(t), "--yes")
		if want := "server returned 409: Missing a platform update." + tail; err == nil || err.Error() != want {
			t.Fatalf("error =\n%v\nwant\n%s", err, want)
		}
		if strings.Contains(errOut, "Civitai refused this submit") {
			t.Fatalf("listed updates the server did not send:\n%s", errOut)
		}
	})
}
