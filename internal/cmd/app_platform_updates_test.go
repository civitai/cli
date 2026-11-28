package cmd

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/civitai/cli/internal/appapi"
)

// Staff platform updates (civitai/civitai#10594's client flow), end to end:
// a REAL source repository served by `git http-backend` behind Basic auth (the
// gitServer of git_credential_git_test.go, so every fetch authenticates through
// the real `civitai git-credential` helper), and a fake of the three tRPC
// procedures that records what the CLI reported.

const (
	puID     = "apu_01JTESTPLATFORMUPDATE000001"
	puBranch = "platform-update/apu_01jtestplatformupdate000001"
	puReason = "Bump @civitai/sdk to 0.40 for the new auth flow"
	// puStaff is the sender the server's owner view names a verified staff
	// update with since civitai/civitai#10717 (its APP_PLATFORM_UPDATE_SENDER).
	puStaff = "CivitaiOfficial (App Assistant)"
)

type puResolve struct{ ID, Outcome, MergeSha string }

// puFake is the server half: listPending, verifyCommit, resolve. It keeps the
// server's state: resolve sets an update's status, and listPending lists only
// the OPEN ones (pending, conflicted), as the contract says.
type puFake struct {
	mu         sync.Mutex
	pending    []map[string]any
	listStatus int             // non-zero: listPending answers this status
	listBody   string          // with listStatus: the body (default: tRPC's own no-procedure envelope)
	verified   map[string]bool // commitSha → verifyCommit's `verified`
	resolves   []puResolve
	listCalls  int
	// submitBody records a submit-version body, for the submit tests.
	submitBody map[string]json.RawMessage
	// submitStatus, when non-zero, is the submit route's refusal status, with
	// submitResp as its body (the server's own platform-update check).
	submitStatus int
	submitResp   string
}

func (f *puFake) resolved() []puResolve {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]puResolve(nil), f.resolves...)
}

func trpcOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"data": map[string]any{"json": v}}})
}

func (f *puFake) handle(w http.ResponseWriter, r *http.Request) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	input := func() map[string]any {
		var env struct {
			JSON map[string]any `json:"json"`
		}
		if r.Method == http.MethodGet {
			_ = json.Unmarshal([]byte(r.URL.Query().Get("input")), &env)
		} else {
			_ = json.NewDecoder(r.Body).Decode(&env)
		}
		return env.JSON
	}
	switch r.URL.Path {
	case appapi.PlatformUpdatesListPendingPath:
		f.listCalls++
		if in := input(); in["slug"] != gitSlug {
			w.WriteHeader(http.StatusBadRequest)
			return true
		}
		if f.listStatus != 0 {
			w.WriteHeader(f.listStatus)
			if f.listBody != "" {
				_, _ = io.WriteString(w, f.listBody)
				return true
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"json": map[string]any{
				"message": `No procedure found on path "appPlatformUpdates.listPending"`, "code": -32004,
				"data": map[string]any{"code": "NOT_FOUND", "httpStatus": 404, "path": "appPlatformUpdates.listPending"}}}})
			return true
		}
		ups := []map[string]any{}
		for _, u := range f.pending {
			if st := u["status"]; st == "pending" || st == "conflicted" {
				ups = append(ups, u)
			}
		}
		trpcOK(w, map[string]any{"updates": ups})
		return true
	case appapi.PlatformUpdatesVerifyCommitPath:
		in := input()
		sha, _ := in["commitSha"].(string)
		trpcOK(w, map[string]any{"verified": f.verified[sha], "update": nil})
		return true
	case appapi.PlatformUpdatesResolvePath:
		in := input()
		id, _ := in["id"].(string)
		outcome, _ := in["outcome"].(string)
		ms, _ := in["mergeCommitSha"].(string)
		f.resolves = append(f.resolves, puResolve{id, outcome, ms})
		for _, u := range f.pending {
			if u["id"] == id {
				u["status"] = outcome
			}
		}
		trpcOK(w, map[string]any{"update": map[string]any{"id": id, "status": outcome}})
		return true
	// The routes `app submit` reads besides these.
	case appapi.SubmissionsPath:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"submissions": []any{}})
		return true
	case listMineTestPath:
		trpcOK(w, []any{})
		return true
	case appapi.SDKFloorPath:
		w.WriteHeader(http.StatusNotFound)
		return true
	case "/api/v1/blocks/submit-version":
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &f.submitBody)
		w.Header().Set("Content-Type", "application/json")
		if f.submitStatus != 0 {
			w.WriteHeader(f.submitStatus)
			_, _ = io.WriteString(w, f.submitResp)
			return true
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"publishRequestId": "pubreq_pu", "slug": gitSlug, "version": "0.6.1", "status": "pending"})
		return true
	}
	return false
}

func puUpdate(sha string, touchesManifest bool, paths ...map[string]any) map[string]any {
	if paths == nil {
		paths = []map[string]any{{"path": "UPDATE.md", "operation": "create"}}
	}
	return map[string]any{
		"id": puID, "branch": puBranch, "commitSha": sha, "baseCommitSha": strings.Repeat("0", 40),
		"reason": puReason, "paths": paths, "touchesManifest": touchesManifest, "status": "pending",
		"by":        map[string]any{"username": puStaff, "verifiedStaff": true},
		"createdAt": "2026-11-26T10:00:00.000Z",
	}
}

// puEnv is one owner checkout of the app's source repository, pulled through
// the CLI, plus the fake.
type puEnv struct {
	g    *gitServer
	fake *puFake
	cwd  string
	dir  string // the checkout, absolute
}

func (e *puEnv) srcBare() string { return filepath.Join(e.g.root, "civitai-apps-src", gitSlug+".git") }

// newPUEnv serves a source repository, seeds its main with seed (path →
// content, committed on top of SOURCE.md), and clones it with `civitai app
// pull` while no update is pending.
func newPUEnv(t *testing.T, seed map[string]string) *puEnv {
	t.Helper()
	g := newGitServerOpts(t, true)
	e := &puEnv{g: g, fake: &puFake{verified: map[string]bool{}}}
	g.extra = e.fake.handle
	if len(seed) > 0 {
		e.staffCommit(t, "main", "main", "seed", seed)
	}
	e.cwd = t.TempDir()
	chdir(t, e.cwd)
	if _, errOut, err := run(t, "app", "pull", "--app", gitSlug); err != nil {
		t.Fatalf("initial app pull: %v\n%s", err, errOut)
	}
	e.dir = filepath.Join(e.cwd, gitSlug)
	return e
}

// staffCommit commits files on branch (cut from base) of the source repository,
// directly on disk, and returns the commit's sha.
func (e *puEnv) staffCommit(t *testing.T, base, branch, msg string, files map[string]string) string {
	t.Helper()
	work := filepath.Join(t.TempDir(), "staff")
	mustGit(t, "", "clone", "-q", e.srcBare(), work)
	mustGit(t, work, "checkout", "-q", "-B", branch, "origin/"+base)
	for p, c := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(work, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(work, p), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
		mustGit(t, work, "add", "--", p)
	}
	mustGit(t, work, "commit", "-q", "-m", msg)
	mustGit(t, work, "push", "-q", "origin", "HEAD:refs/heads/"+branch)
	return strings.TrimSpace(mustGitOut(t, work, "rev-parse", "HEAD"))
}

func (e *puEnv) ownerCommit(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(e.dir, path), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, e.dir, "add", "--", path)
	mustGit(t, e.dir, "commit", "-q", "-m", "owner: "+path)
	return e.head(t)
}

func (e *puEnv) head(t *testing.T) string {
	t.Helper()
	return strings.TrimSpace(mustGitOut(t, e.dir, "rev-parse", "HEAD"))
}

func (e *puEnv) pull(t *testing.T) (string, string, error) {
	t.Helper()
	return run(t, "app", "pull", "--app", gitSlug)
}

// 🔴 The core path: the recorded commit is merged with a merge commit, the
// owner's own work is kept, `resolve merged` carries that merge commit, and the
// badge is printed because verifyCommit said so.
func TestPlatformUpdatePullMergesTheRecordedCommit(t *testing.T) {
	e := newPUEnv(t, nil)
	sha := e.staffCommit(t, "main", puBranch, "Platform update", map[string]string{"UPDATE.md": "from staff\n"})
	ownerHead := e.ownerCommit(t, "OWNER.md", "mine\n")
	e.fake.pending = []map[string]any{puUpdate(sha, false)}
	e.fake.verified[sha] = true

	out, errOut, err := e.pull(t)
	if err != nil {
		t.Fatalf("app pull: %v\n%s\n%s", err, out, errOut)
	}
	merge := e.head(t)
	want := "Civitai staff pushed 1 platform update(s) to my-block.\n" +
		"Merged platform update " + puID + " (commit " + sha + ") into my-block.\n" +
		"  CivitaiOfficial (App Assistant) updated your app: " + puReason + "\n" +
		"  Files: create UPDATE.md\n"
	if !strings.Contains(out, want) {
		t.Fatalf("stdout does not carry the merge notice.\nwant:\n%s\ngot:\n%s", want, out)
	}
	if got := e.fake.resolved(); len(got) != 1 || got[0] != (puResolve{puID, "merged", merge}) {
		t.Fatalf("resolve calls = %+v, want one merged with the merge commit %s", got, merge)
	}
	// A real merge commit: first parent the owner's work, second the recorded sha.
	parents := strings.Fields(mustGitOut(t, e.dir, "rev-list", "--parents", "-n", "1", "HEAD"))
	if len(parents) != 3 || parents[1] != ownerHead || parents[2] != sha {
		t.Fatalf("HEAD parents = %v, want [%s %s %s]", parents, merge, ownerHead, sha)
	}
	if _, err := os.Stat(filepath.Join(e.dir, "OWNER.md")); err != nil {
		t.Fatalf("the owner's own commit was lost: %v", err)
	}
	// Nothing was written to the checkout's remotes.
	if remotes := strings.TrimSpace(mustGitOut(t, e.dir, "remote")); remotes != "origin" {
		t.Fatalf("remotes = %q, want only origin (the source repository is fetched by URL)", remotes)
	}
}

// verifyCommit false → the same merge, no badge, and the not-verified note.
func TestPlatformUpdateUnverifiedCommitHasNoBadge(t *testing.T) {
	e := newPUEnv(t, nil)
	sha := e.staffCommit(t, "main", puBranch, "Platform update", map[string]string{"UPDATE.md": "from staff\n"})
	e.fake.pending = []map[string]any{puUpdate(sha, false)}
	// e.fake.verified[sha] stays false.

	out, errOut, err := e.pull(t)
	if err != nil {
		t.Fatalf("app pull: %v\n%s", err, errOut)
	}
	want := "Merged platform update " + puID + " (commit " + sha + ") into my-block.\n" +
		"  CivitaiOfficial (App Assistant) updated your app: " + puReason + "\n" +
		"  Could not verify that this update came from Civitai staff (the server did not confirm commit " + sha + "); review it with `git show " + sha + "`.\n"
	if !strings.Contains(out, want) {
		t.Fatalf("want:\n%s\ngot:\n%s", want, out)
	}
}

// 🔴 A branch with a commit on top of the recorded one: only commitSha is
// merged, and the owner is told the rest was left out.
func TestPlatformUpdateLeavesOutCommitsOnTopOfTheBranch(t *testing.T) {
	e := newPUEnv(t, nil)
	sha := e.staffCommit(t, "main", puBranch, "Platform update", map[string]string{"UPDATE.md": "from staff\n"})
	extra := e.staffCommit(t, puBranch, puBranch, "not part of it", map[string]string{"EXTRA.md": "sneaky\n"})
	e.fake.pending = []map[string]any{puUpdate(sha, false)}
	e.fake.verified[sha] = true

	out, errOut, err := e.pull(t)
	if err != nil {
		t.Fatalf("app pull: %v\n%s", err, errOut)
	}
	want := "  Its branch " + puBranch + " has commits that are not part of the update (tip " + extra + "); they were left out. Only " + sha + " was merged.\n"
	if !strings.Contains(out, want) {
		t.Fatalf("want:\n%s\ngot:\n%s", want, out)
	}
	if _, err := os.Stat(filepath.Join(e.dir, "EXTRA.md")); !os.IsNotExist(err) {
		t.Fatalf("the commit on top of the update was merged (EXTRA.md: %v)", err)
	}
	if _, err := os.Stat(filepath.Join(e.dir, "UPDATE.md")); err != nil {
		t.Fatalf("the update itself was not merged: %v", err)
	}
}

// 🔴 A conflict: the merge is aborted (no MERGE_HEAD, HEAD unchanged), the
// server is told `conflicted`, the command fails, and the hand-merge
// instructions name the recorded sha.
func TestPlatformUpdateConflictIsAbortedAndReported(t *testing.T) {
	e := newPUEnv(t, map[string]string{"shared.txt": "base\n"})
	sha := e.staffCommit(t, "main", puBranch, "Platform update", map[string]string{"shared.txt": "staff\n"})
	ownerHead := e.ownerCommit(t, "shared.txt", "owner\n")
	e.fake.pending = []map[string]any{puUpdate(sha, false, map[string]any{"path": "shared.txt", "operation": "update"})}
	e.fake.verified[sha] = true

	out, _, err := e.pull(t)
	if !errors.Is(err, ErrUnmergedPlatformUpdate) {
		t.Fatalf("want ErrUnmergedPlatformUpdate, got %v\n%s", err, out)
	}
	want := "Platform update " + puID + " (commit " + sha + ") conflicts with your changes in my-block, so nothing was merged (the merge was aborted).\n" +
		"  CivitaiOfficial (App Assistant) updated your app: " + puReason + "\n" +
		"  Files: update shared.txt\n" +
		"  Conflicts in: shared.txt\n" +
		"To merge it by hand, merge the recorded commit " + sha + " (not the branch):\n" +
		"  " + puHandMerge(e, "my-block", sha) + "\n" +
		"then resolve the conflicts and commit. The next `civitai app pull` reports it merged.\n"
	if !strings.Contains(out, want) {
		t.Fatalf("want:\n%s\ngot:\n%s", want, out)
	}
	if got := e.fake.resolved(); len(got) != 1 || got[0] != (puResolve{puID, "conflicted", ""}) {
		t.Fatalf("resolve calls = %+v, want one conflicted", got)
	}
	if head := e.head(t); head != ownerHead {
		t.Fatalf("HEAD moved to %s; the aborted merge must leave %s", head, ownerHead)
	}
	if _, err := os.Stat(filepath.Join(e.dir, ".git", "MERGE_HEAD")); !os.IsNotExist(err) {
		t.Fatalf("MERGE_HEAD exists: the merge was not aborted (%v)", err)
	}
	if st := strings.TrimSpace(mustGitOut(t, e.dir, "status", "--porcelain")); st != "" {
		t.Fatalf("the checkout is left dirty after the abort:\n%s", st)
	}
}

// touchesManifest → the "what your app is allowed to do" note.
func TestPlatformUpdateThatTouchesTheManifestSaysSo(t *testing.T) {
	e := newPUEnv(t, nil)
	sha := e.staffCommit(t, "main", puBranch, "Platform update", map[string]string{"block.manifest.json": "{\"blockId\":\"my-block\",\"scopes\":[\"x\"]}\n"})
	e.fake.pending = []map[string]any{puUpdate(sha, true, map[string]any{"path": "block.manifest.json", "operation": "create"})}
	e.fake.verified[sha] = true

	out, errOut, err := e.pull(t)
	if err != nil {
		t.Fatalf("app pull: %v\n%s", err, errOut)
	}
	want := "  Files: create block.manifest.json\n" +
		"  It changes block.manifest.json or a storage hook, which decide what your app is allowed to do — review it with `git show " + sha + "` before you submit.\n"
	if !strings.Contains(out, want) {
		t.Fatalf("want:\n%s\ngot:\n%s", want, out)
	}
}

// The control for the note: an update that does not touch the manifest never
// prints it.
func TestPlatformUpdateNoManifestNoteWhenItDoesNotTouchTheManifest(t *testing.T) {
	e := newPUEnv(t, nil)
	sha := e.staffCommit(t, "main", puBranch, "Platform update", map[string]string{"UPDATE.md": "x\n"})
	e.fake.pending = []map[string]any{puUpdate(sha, false)}
	out, _, err := e.pull(t)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "allowed to do") {
		t.Fatalf("the manifest note was printed for an update that does not touch it:\n%s", out)
	}
}

// No updates, an older server (404) and the flag off ({updates: []}) are all
// silent: nothing printed, nothing resolved. The positive control is the last
// row: the same harness with one pending update does print.
func TestPlatformUpdateNoneIsSilent(t *testing.T) {
	cases := []struct {
		name       string
		listStatus int
		pending    bool
		wantPrint  bool
	}{
		{"no pending updates (also the flag-off answer: {updates: []})", 0, false, false},
		{"older server: no such procedure (404)", http.StatusNotFound, false, false},
		{"CONTROL: one pending update", 0, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newPUEnv(t, nil)
			if tc.pending {
				sha := e.staffCommit(t, "main", puBranch, "Platform update", map[string]string{"UPDATE.md": "x\n"})
				e.fake.pending = []map[string]any{puUpdate(sha, false)}
			}
			e.fake.listStatus = tc.listStatus
			before := e.fake.listCalls
			out, errOut, err := e.pull(t)
			if err != nil {
				t.Fatalf("app pull: %v\n%s", err, errOut)
			}
			if e.fake.listCalls != before+1 {
				t.Fatalf("listPending calls = %d, want %d: the silent rows must still ASK", e.fake.listCalls, before+1)
			}
			printed := strings.Contains(strings.ToLower(out+errOut), "platform update")
			if printed != tc.wantPrint {
				t.Fatalf("printed a platform-update line = %v, want %v\nstdout:\n%s\nstderr:\n%s", printed, tc.wantPrint, out, errOut)
			}
			if !tc.pending && len(e.fake.resolved()) != 0 {
				t.Fatalf("resolve was called with nothing pending: %+v", e.fake.resolved())
			}
		})
	}
}

// 🔴 A checkout with uncommitted changes is never merged into: the command
// fails, HEAD does not move, nothing is resolved.
func TestPlatformUpdateRefusesADirtyTree(t *testing.T) {
	e := newPUEnv(t, map[string]string{"shared.txt": "base\n"})
	sha := e.staffCommit(t, "main", puBranch, "Platform update", map[string]string{"UPDATE.md": "x\n"})
	e.fake.pending = []map[string]any{puUpdate(sha, false)}
	e.fake.verified[sha] = true
	headBefore := e.head(t)
	if err := os.WriteFile(filepath.Join(e.dir, "shared.txt"), []byte("work in progress\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _, err := e.pull(t)
	if !errors.Is(err, ErrUnmergedPlatformUpdate) {
		t.Fatalf("want ErrUnmergedPlatformUpdate, got %v\n%s", err, out)
	}
	wantErr := "unmerged platform update: my-block has uncommitted changes, so 1 platform update(s) from Civitai staff were not merged into it — commit or discard your changes, then run the command again"
	if err.Error() != wantErr {
		t.Fatalf("error =\n%s\nwant\n%s", err, wantErr)
	}
	if head := e.head(t); head != headBefore {
		t.Fatalf("HEAD moved (%s → %s) on a dirty tree", headBefore, head)
	}
	if _, err := os.Stat(filepath.Join(e.dir, "UPDATE.md")); !os.IsNotExist(err) {
		t.Fatalf("the update reached a dirty tree: %v", err)
	}
	if got := e.fake.resolved(); len(got) != 0 {
		t.Fatalf("resolve called on a refusal: %+v", got)
	}
	if b, _ := os.ReadFile(filepath.Join(e.dir, "shared.txt")); string(b) != "work in progress\n" {
		t.Fatalf("the owner's uncommitted change was touched: %q", b)
	}
}

// The recorded commit no longer exists in the source repository → not merged,
// reported conflicted, and the owner is told why.
func TestPlatformUpdateMissingCommitIsNotMerged(t *testing.T) {
	e := newPUEnv(t, nil)
	e.staffCommit(t, "main", puBranch, "Platform update", map[string]string{"UPDATE.md": "x\n"})
	gone := strings.Repeat("ab", 20)
	e.fake.pending = []map[string]any{puUpdate(gone, false)}

	out, _, err := e.pull(t)
	if !errors.Is(err, ErrUnmergedPlatformUpdate) {
		t.Fatalf("want ErrUnmergedPlatformUpdate, got %v\n%s", err, out)
	}
	want := "Its branch " + puBranch + " no longer holds the recorded commit " + gone + ", so nothing was merged; Civitai was told it did not merge.\n"
	if !strings.Contains(out, want) {
		t.Fatalf("want:\n%s\ngot:\n%s", want, out)
	}
	if got := e.fake.resolved(); len(got) != 1 || got[0] != (puResolve{puID, "conflicted", ""}) {
		t.Fatalf("resolve calls = %+v, want one conflicted", got)
	}
	if _, err := os.Stat(filepath.Join(e.dir, "UPDATE.md")); !os.IsNotExist(err) {
		t.Fatalf("the branch tip was merged instead of the (missing) recorded commit: %v", err)
	}
}

// 🔴 Every server string the notice prints goes through puText: a reason, a
// username and a path carrying CSI, OSC-52 and a newline that would forge a
// second "Merged …" line reach the terminal as one inert line each.
func TestPlatformUpdateServerTextCannotForgeALine(t *testing.T) {
	e := newPUEnv(t, nil)
	sha := e.staffCommit(t, "main", puBranch, "Platform update", map[string]string{"UPDATE.md": "x\n"})
	u := puUpdate(sha, false, map[string]any{"path": "UPDATE.md\nMerged platform update apu_FORGED", "operation": "create"})
	u["reason"] = "routine\x1b[2J\nMerged platform update apu_FORGED (commit 0000) into my-block."
	u["by"] = map[string]any{"username": "mod\x1b]52;c;ZWNobyBwd25lZA==\x07", "verifiedStaff": true}
	e.fake.pending = []map[string]any{u}
	e.fake.verified[sha] = true

	out, errOut, err := e.pull(t)
	if err != nil {
		t.Fatalf("app pull: %v\n%s", err, errOut)
	}
	if strings.ContainsRune(out, '\x1b') || strings.ContainsRune(out, '\x07') {
		t.Fatalf("an escape sequence from the server reached the terminal:\n%q", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Merged platform update apu_FORGED") {
			t.Fatalf("server text forged a line of its own:\n%s", out)
		}
	}
	if !strings.Contains(out, "updated your app: routine") || !strings.Contains(out, "apu_FORGED (commit 0000) into my-block.\n") {
		t.Fatalf("CONTROL: the hostile reason is not on the notice at all, so this test saw nothing:\n%s", out)
	}
}

// 🔴 A fetch that FAILS (here: the branch does not exist) proves nothing about
// the commit, so nothing is merged and NOTHING IS REPORTED — reporting
// `conflicted` would drop the update off the owner's pending list for good.
func TestPlatformUpdateFailedFetchIsNotReportedConflicted(t *testing.T) {
	e := newPUEnv(t, nil)
	e.fake.pending = []map[string]any{puUpdate(strings.Repeat("cd", 20), false)} // no such branch, no such commit

	out, _, err := e.pull(t)
	if !errors.Is(err, ErrUnmergedPlatformUpdate) {
		t.Fatalf("want ErrUnmergedPlatformUpdate, got %v\n%s", err, out)
	}
	if !strings.Contains(err.Error(), "could not fetch platform update "+puID+" from "+e.g.sourceURL()+" (") ||
		!strings.HasSuffix(err.Error(), "); nothing was merged or reported — run the command again") {
		t.Fatalf("error does not name the failed fetch:\n%s", err)
	}
	if got := e.fake.resolved(); len(got) != 0 {
		t.Fatalf("a failed fetch was reported to the server: %+v", got)
	}
}

// --- app submit ---

// puSubmitDir is an app directory that is NOT a git checkout (a ZIP-style
// workflow, or a copy of a stale tree) for the slug the fake serves.
func puSubmitDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeManifestVersion(t, dir, gitSlug, "0.6.1")
	return dir
}

// 🔴 Submit refuses while an update is unmerged and names the override; the
// submit route is never reached.
func TestSubmitRefusesWithAnUnmergedPlatformUpdate(t *testing.T) {
	e := newPUEnv(t, nil)
	sha := e.staffCommit(t, "main", puBranch, "Platform update", map[string]string{"UPDATE.md": "x\n"})
	e.fake.pending = []map[string]any{puUpdate(sha, false)}
	dir := puSubmitDir(t)

	_, errOut, err := run(t, "app", "submit", dir, "--yes")
	if !errors.Is(err, ErrUnmergedPlatformUpdate) {
		t.Fatalf("want ErrUnmergedPlatformUpdate, got %v\n%s", err, errOut)
	}
	if !strings.Contains(err.Error(), "pass --allow-unmerged-platform-update to submit without it") {
		t.Fatalf("the refusal must name the override flag:\n%s", err)
	}
	if !strings.Contains(err.Error(), "is not a git checkout") {
		t.Fatalf("the refusal must say why it could not merge:\n%s", err)
	}
	if e.fake.submitBody != nil {
		t.Fatal("the submit route was hit despite the refusal")
	}
}

// The override proceeds: the submit reaches the server, nothing is merged or
// resolved, and the update being left out is named on stderr.
func TestSubmitOverrideSubmitsWithoutTheUpdate(t *testing.T) {
	e := newPUEnv(t, nil)
	sha := e.staffCommit(t, "main", puBranch, "Platform update", map[string]string{"UPDATE.md": "x\n"})
	e.fake.pending = []map[string]any{puUpdate(sha, false)}
	dir := puSubmitDir(t)

	_, errOut, err := run(t, "app", "submit", dir, "--yes", "--allow-unmerged-platform-update")
	if err != nil {
		t.Fatalf("submit with the override: %v\n%s", err, errOut)
	}
	if e.fake.submitBody == nil {
		t.Fatal("the override did not reach the submit route")
	}
	// The override rides the body, for the server's own check and the reviewer.
	if got := string(e.fake.submitBody["allowUnmergedPlatformUpdate"]); got != "true" {
		t.Fatalf("allowUnmergedPlatformUpdate = %q, want true", got)
	}
	want := "Submitting WITHOUT merging 1 platform update(s) from Civitai staff (--allow-unmerged-platform-update):\n" +
		"  Platform update " + puID + " from CivitaiOfficial (App Assistant): " + puReason + "\n" +
		"  Files: create UPDATE.md\n"
	if !strings.Contains(errOut, want) {
		t.Fatalf("want on stderr:\n%s\ngot:\n%s", want, errOut)
	}
	if got := e.fake.resolved(); len(got) != 0 {
		t.Fatalf("the override resolved something: %+v", got)
	}
}

// Submit from a real checkout merges the update first, so the BUNDLE carries
// it, and the provenance names the merge commit rather than the pre-merge HEAD.
func TestSubmitMergesAPendingUpdateIntoTheBundle(t *testing.T) {
	e := newPUEnv(t, map[string]string{"block.manifest.json": manifestText(t, gitSlug, "0.6.1"), "index.html": "<html></html>"})
	sha := e.staffCommit(t, "main", puBranch, "Platform update", map[string]string{"UPDATE.md": "x\n"})
	e.fake.pending = []map[string]any{puUpdate(sha, false)}
	e.fake.verified[sha] = true
	before := e.head(t)

	_, errOut, err := run(t, "app", "submit", e.dir, "--yes")
	if err != nil {
		t.Fatalf("submit: %v\n%s", err, errOut)
	}
	merge := e.head(t)
	if merge == before {
		t.Fatal("submit did not merge the pending update")
	}
	if got := e.fake.resolved(); len(got) != 1 || got[0] != (puResolve{puID, "merged", merge}) {
		t.Fatalf("resolve calls = %+v, want merged with %s", got, merge)
	}
	if e.fake.submitBody == nil {
		t.Fatal("the submit route was not reached")
	}
	if raw, ok := e.fake.submitBody["allowUnmergedPlatformUpdate"]; ok {
		t.Fatalf("allowUnmergedPlatformUpdate sent without the flag: %s", raw)
	}
	if files := strings.Join(bundleFiles(t, e.fake.submitBody), ","); !strings.Contains(","+files+",", ",UPDATE.md,") {
		t.Fatalf("the bundle does not carry the merged update; files: %s", files)
	}
	if got := string(e.fake.submitBody["sourceCommit"]); got != `"`+merge+`"` {
		t.Fatalf("sourceCommit = %s, want the merge commit %s (the pre-merge HEAD was %s)", got, merge, before)
	}
	if !strings.Contains(errOut, "\n  CivitaiOfficial (App Assistant) updated your app: "+puReason+"\n") {
		t.Fatalf("the attribution was not printed:\n%s", errOut)
	}
}

func manifestText(t *testing.T, blockID, version string) string {
	t.Helper()
	dir := t.TempDir()
	writeManifestVersion(t, dir, blockID, version)
	b, err := os.ReadFile(filepath.Join(dir, "block.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// bundleFiles lists the file names in a submit body's base64 ZIP.
func bundleFiles(t *testing.T, body map[string]json.RawMessage) []string {
	t.Helper()
	var b64 string
	if err := json.Unmarshal(body["bundleBase64"], &b64); err != nil {
		t.Fatalf("bundleBase64: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	return names
}

// --- a conflicted update stays open (civitai/civitai#10594: the owner list is
// pending AND conflicted) ---

// puConflict sets up an update that conflicts with the owner's own commit and
// runs the first pull, which reports it conflicted. It returns the update's sha
// and the owner's HEAD.
func puConflict(t *testing.T, seed map[string]string) (*puEnv, string, string) {
	t.Helper()
	if seed == nil {
		seed = map[string]string{}
	}
	seed["shared.txt"] = "base\n"
	e := newPUEnv(t, seed)
	sha := e.staffCommit(t, "main", puBranch, "Platform update", map[string]string{"shared.txt": "staff\n"})
	ownerHead := e.ownerCommit(t, "shared.txt", "owner\n")
	e.fake.pending = []map[string]any{puUpdate(sha, false, map[string]any{"path": "shared.txt", "operation": "update"})}
	e.fake.verified[sha] = true
	if _, _, err := e.pull(t); !errors.Is(err, ErrUnmergedPlatformUpdate) {
		t.Fatalf("first pull: want ErrUnmergedPlatformUpdate, got %v", err)
	}
	if got := e.fake.resolved(); len(got) != 1 || got[0] != (puResolve{puID, "conflicted", ""}) {
		t.Fatalf("first pull: resolve calls = %+v, want one conflicted", got)
	}
	return e, sha, ownerHead
}

// 🔴 The gap this closes: the update did NOT vanish after the one conflicted
// report. A second pull retries it, still conflicts, re-prints the hand-merge
// instructions naming the recorded sha, fails — and does not report
// `conflicted` a second time.
func TestPlatformUpdateConflictedIsRetriedAndReprinted(t *testing.T) {
	e, sha, ownerHead := puConflict(t, nil)

	out, _, err := e.pull(t)
	if !errors.Is(err, ErrUnmergedPlatformUpdate) {
		t.Fatalf("second pull: want ErrUnmergedPlatformUpdate, got %v\n%s", err, out)
	}
	want := "Retrying platform update " + puID + " (commit " + sha + "), which did not merge cleanly last time.\n" +
		"Platform update " + puID + " (commit " + sha + ") conflicts with your changes in my-block, so nothing was merged (the merge was aborted).\n" +
		"  CivitaiOfficial (App Assistant) updated your app: " + puReason + "\n" +
		"  Files: update shared.txt\n" +
		"  Conflicts in: shared.txt\n" +
		"To merge it by hand, merge the recorded commit " + sha + " (not the branch):\n" +
		"  " + puHandMerge(e, "my-block", sha) + "\n"
	if !strings.Contains(out, want) {
		t.Fatalf("second pull did not re-print the instructions.\nwant:\n%s\ngot:\n%s", want, out)
	}
	if got := e.fake.resolved(); len(got) != 1 {
		t.Fatalf("resolve calls = %+v, want still only the first conflicted (no re-report)", got)
	}
	if head := e.head(t); head != ownerHead {
		t.Fatalf("HEAD moved to %s; the aborted retry must leave %s", head, ownerHead)
	}
}

// The owner merges the recorded commit by hand and commits: the next pull sees
// the sha already in HEAD's history and reports `merged` with HEAD, merging
// nothing again.
func TestPlatformUpdateConflictedMergedByHandIsReportedMerged(t *testing.T) {
	e, sha, _ := puConflict(t, nil)
	mustGit(t, e.dir, "fetch", "-q", e.srcBare(), puBranch)
	if out, err := exec.Command("git", "-C", e.dir, "merge", "--no-edit", sha).CombinedOutput(); err == nil {
		t.Fatalf("CONTROL: the hand merge was expected to conflict, it did not:\n%s", out)
	}
	if err := os.WriteFile(filepath.Join(e.dir, "shared.txt"), []byte("owner + staff\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, e.dir, "add", "--", "shared.txt")
	mustGit(t, e.dir, "commit", "-q", "--no-edit")
	handMerge := e.head(t)

	out, errOut, err := e.pull(t)
	if err != nil {
		t.Fatalf("pull after the hand merge: %v\n%s\n%s", err, out, errOut)
	}
	if !strings.Contains(out, "Platform update "+puID+" (commit "+sha+") is already in my-block.\n") {
		t.Fatalf("pull did not report the hand merge:\n%s", out)
	}
	if strings.Contains(out, "Retrying") || strings.Contains(out, "conflicts with") {
		t.Fatalf("pull retried a merge that was already in the history:\n%s", out)
	}
	if head := e.head(t); head != handMerge {
		t.Fatalf("HEAD moved %s → %s: nothing should be merged again", handMerge, head)
	}
	want := []puResolve{{puID, "conflicted", ""}, {puID, "merged", handMerge}}
	if got := e.fake.resolved(); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("resolve calls = %+v, want %+v", got, want)
	}
	// And it is off the open list: a third pull is silent.
	out, _, err = e.pull(t)
	if err != nil || strings.Contains(out, "platform update") {
		t.Fatalf("third pull: err %v, out:\n%s", err, out)
	}
}

// The owner backs out their conflicting change: the retry now merges cleanly
// and is reported `merged` with the new merge commit (conflicted → merged).
func TestPlatformUpdateConflictedThatNowMergesCleanlyIsMerged(t *testing.T) {
	e, sha, _ := puConflict(t, nil)
	e.ownerCommit(t, "shared.txt", "base\n")

	out, errOut, err := e.pull(t)
	if err != nil {
		t.Fatalf("retry pull: %v\n%s\n%s", err, out, errOut)
	}
	merge := e.head(t)
	if !strings.Contains(out, "Merged platform update "+puID+" (commit "+sha+") into my-block.\n") {
		t.Fatalf("the retry did not merge:\n%s", out)
	}
	if got := e.fake.resolved(); len(got) != 2 || got[1] != (puResolve{puID, "merged", merge}) {
		t.Fatalf("resolve calls = %+v, want conflicted then merged with %s", got, merge)
	}
	if b, _ := os.ReadFile(filepath.Join(e.dir, "shared.txt")); string(b) != "staff\n" {
		t.Fatalf("shared.txt = %q, want the staff change", b)
	}
}

// 🔴 Submit still refuses over a conflicted update, naming the override, and
// never reaches the submit route or re-reports the conflict.
func TestSubmitRefusesOverAConflictedUpdate(t *testing.T) {
	e, sha, _ := puConflict(t, map[string]string{"block.manifest.json": manifestText(t, gitSlug, "0.6.1"), "index.html": "<html></html>"})

	_, errOut, err := run(t, "app", "submit", e.dir, "--yes")
	if !errors.Is(err, ErrUnmergedPlatformUpdate) {
		t.Fatalf("want ErrUnmergedPlatformUpdate, got %v\n%s", err, errOut)
	}
	if !strings.Contains(err.Error(), "pass --allow-unmerged-platform-update to submit without it") ||
		!strings.Contains(err.Error(), "merge "+sha+" by hand") {
		t.Fatalf("the refusal must name the sha and the override:\n%s", err)
	}
	if e.fake.submitBody != nil {
		t.Fatal("the submit route was hit despite a conflicted update")
	}
	if got := e.fake.resolved(); len(got) != 1 {
		t.Fatalf("resolve calls = %+v, want only the pull's conflicted", got)
	}
}

// The two 404s. An older server's (tRPC's own no-procedure envelope, or a 404
// with no envelope at all) is silent; the server's "App not found" for the app
// pull just resolved prints ONE warning line. Submit stays silent on "App not
// found" (a brand-new slug's first submit lands there) and submits.
func TestPlatformUpdate404s(t *testing.T) {
	appNotFound := `{"error":{"json":{"message":"App not found","code":-32004,"data":{"code":"NOT_FOUND","httpStatus":404,"path":"appPlatformUpdates.listPending"}}}}`
	const warnLine = "Civitai answered \"App not found\" when asked for my-block's platform updates, so none were checked"
	cases := []struct {
		name     string
		body     string
		wantWarn bool
	}{
		{"older server: tRPC's no-procedure envelope", "", false},
		{"older server: a 404 with no tRPC envelope", "<html>Not Found</html>", false},
		{"older server: tRPC v10's message", `{"error":{"json":{"message":"No \"query\"-procedure on path \"appPlatformUpdates.listPending\"","code":-32004}}}`, false},
		{"App not found", appNotFound, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newPUEnv(t, nil)
			e.fake.listStatus, e.fake.listBody = http.StatusNotFound, tc.body
			out, errOut, err := e.pull(t)
			if err != nil {
				t.Fatalf("pull: %v\n%s", err, errOut)
			}
			if n := strings.Count(out+errOut, warnLine); (n == 1) != tc.wantWarn || n > 1 {
				t.Fatalf("warning lines = %d, want warn=%v\nstdout:\n%s\nstderr:\n%s", n, tc.wantWarn, out, errOut)
			}
			if !tc.wantWarn && strings.Contains(strings.ToLower(out+errOut), "platform update") {
				t.Fatalf("an older server's 404 printed something:\n%s\n%s", out, errOut)
			}
		})
	}
	t.Run("submit: App not found is silent", func(t *testing.T) {
		e := newPUEnv(t, nil)
		e.fake.listStatus, e.fake.listBody = http.StatusNotFound, appNotFound
		_, errOut, err := run(t, "app", "submit", puSubmitDir(t), "--yes")
		if err != nil {
			t.Fatalf("submit: %v\n%s", err, errOut)
		}
		if e.fake.submitBody == nil {
			t.Fatal("the submit route was not reached")
		}
		if strings.Contains(strings.ToLower(errOut), "platform") {
			t.Fatalf("submit printed a platform-update line for App not found:\n%s", errOut)
		}
	})
}
