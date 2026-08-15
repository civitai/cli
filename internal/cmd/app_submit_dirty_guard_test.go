package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
)

// TESTS FOR THE DIRTY-WORK-TREE GUARD (issue #411).
//
// 🔴 THE FIXTURES ARE REAL GIT REPOSITORIES, in temp directories, and that is
// deliberate. A stub over gitOutputFunc can prove the DECISIONS but never that
// the arguments produce the answer the guard reads out of them — the porcelain
// -z record layout (where the original path of a rename goes), whether a
// pathspec really scopes a subdirectory, whether `--show-prefix` is the string
// the path arithmetic assumes. Every one of those is a place a confident wrong
// answer comes from, and only real git settles them. The stub is kept for the
// two states a real git cannot be asked to produce on demand: a missing binary
// and a failing status.
//
// 🔴 THE FIXTURES ARE ALSO HERMETIC. gitFixtureEnv points GIT_CONFIG_GLOBAL and
// GIT_CONFIG_SYSTEM at /dev/null, which is not tidiness: a developer's global
// `core.excludesFile` can hide an untracked file from `git status`, and that
// would make the POSITIVE CONTROL below pass while proving nothing.

// --- fixture plumbing ---

// requireGit skips when there is no git to build fixtures with. The guard's own
// no-git behaviour is covered by TestDirtyGuardTreatsAMissingGitLikeNoRepo,
// which needs no binary at all.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH — the real-repo fixtures cannot be built")
	}
}

// gitFixtureEnv isolates every git invocation in this test from the machine it
// runs on: no user config, no system config, a fixed identity.
func gitFixtureEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_AUTHOR_NAME", "cli-test")
	t.Setenv("GIT_AUTHOR_EMAIL", "cli-test@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "cli-test")
	t.Setenv("GIT_COMMITTER_EMAIL", "cli-test@example.invalid")
}

type gitFixture struct {
	t    *testing.T
	root string
}

// newGitFixture creates a real repository in a temp dir.
//
// 🔴 NEVER inside the CLI's own checkout: t.TempDir() only, so a fixture can
// never touch the work tree the developer is sitting in.
//
// The branch is named explicitly because `git init`'s default branch is a
// machine-level setting; a fixture must not inherit one.
func newGitFixture(t *testing.T) *gitFixture {
	t.Helper()
	requireGit(t)
	gitFixtureEnv(t)
	// EvalSymlinks: on macOS t.TempDir() lives under /var, a symlink to
	// /private/var, and git answers with the resolved path — which would break
	// the prefix arithmetic in a way that looks like a guard bug.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	f := &gitFixture{t: t, root: root}
	f.git("init", "-q", "-b", "fixture-main")
	return f
}

// git runs a git subcommand in the fixture and fails the test on error.
func (f *gitFixture) git(args ...string) string {
	f.t.Helper()
	c := exec.Command("git", append([]string{"-C", f.root}, args...)...)
	var out, errb bytes.Buffer
	c.Stdout = &out
	c.Stderr = &errb
	if err := c.Run(); err != nil {
		f.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, errb.String())
	}
	return out.String()
}

// write creates (or overwrites) a file at a repo-relative path.
func (f *gitFixture) write(rel, content string) {
	f.t.Helper()
	p := filepath.Join(f.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

// commit stages the named paths (EXPLICIT paths — never `add -A`, which is how a
// fixture quietly starts committing things a test meant to leave untracked) and
// commits them.
func (f *gitFixture) commit(msg string, paths ...string) {
	f.t.Helper()
	f.git(append([]string{"add", "--"}, paths...)...)
	f.git("commit", "-q", "-m", msg)
}

// publishHEAD fakes a pushed branch without a network or a second repo: a ref
// under refs/remotes is exactly what `for-each-ref --contains HEAD refs/remotes`
// looks for, and how it got there is not part of the question.
func (f *gitFixture) publishHEAD() {
	f.t.Helper()
	f.git("update-ref", "refs/remotes/origin/fixture-main", "HEAD")
}

// appDir returns the absolute path of a repo-relative directory.
func (f *gitFixture) appDir(rel string) string {
	if rel == "" || rel == "." {
		return f.root
	}
	return filepath.Join(f.root, filepath.FromSlash(rel))
}

const dirtySlug = "custom-generators"

// cleanAppFixture is the shared baseline: a repo whose app directory holds a
// committed manifest + source and nothing else.
func cleanAppFixture(t *testing.T, appRel string) *gitFixture {
	t.Helper()
	f := newGitFixture(t)
	dir := appRel
	if dir == "" {
		dir = "."
	}
	prefix := ""
	if dir != "." {
		prefix = dir + "/"
	}
	writeManifestVersion(t, mkdirAllT(t, f.appDir(appRel)), dirtySlug, "0.6.1")
	f.commit("app", prefix+"block.manifest.json", prefix+"index.html")
	return f
}

func mkdirAllT(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// guardOn runs the guard against a real repo directory and returns its warnings
// and its verdict.
func guardOn(t *testing.T, dir string, allowDirty bool) (string, error) {
	t.Helper()
	var warn bytes.Buffer
	err := checkWorkTreeClean(gitOutput, &warn, dir, dirtySlug, "0.6.1", allowDirty)
	return warn.String(), err
}

// --- POSITIVE CONTROL: a dirty tree MUST be refused ---

// TestDirtyGuardRefusesAnUncommittedBundleFile is the issue's own scenario: a
// source file that is in the bundle and in no commit.
func TestDirtyGuardRefusesAnUncommittedBundleFile(t *testing.T) {
	f := cleanAppFixture(t, "")
	f.write("src/App.tsx", "export default 1")

	warn, err := guardOn(t, f.root, false)
	if err == nil {
		t.Fatalf("an uncommitted src/App.tsx goes into the bundle and must be refused; warnings:\n%s", warn)
	}
	// The SENTINEL, not a substring — the exit-code contract is pinned against
	// this and a string match would survive a reworded message.
	if !errors.Is(err, ErrDirtyWorkTree) {
		t.Errorf("refusal must carry ErrDirtyWorkTree, got %#v", err)
	}
	msg := err.Error()
	for _, want := range []string{
		"refusing to submit custom-generators@0.6.1",
		"from a dirty git work tree",
		"src/App.tsx", // the modified path is NAMED
		"--allow-dirty",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal missing %q, got:\n%s", want, msg)
		}
	}
}

// TestDirtyGuardRefusesEveryKindOfDifference — staged, unstaged, deleted,
// renamed and untracked are all "the bundle differs from HEAD", and each must
// refuse and NAME the path.
//
// 🔴 The rename case is here because porcelain -z emits TWO paths for it. A
// parser that mistakes the second for a separate entry reports a file that no
// longer exists; one that fails to consume it reports a bare path with no status
// letters. Neither is visible in any other case.
func TestDirtyGuardRefusesEveryKindOfDifference(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(f *gitFixture)
		wantPath string
	}{
		{"unstaged modification", func(f *gitFixture) {
			f.write("index.html", "<html>changed</html>")
		}, "index.html"},
		{"staged but uncommitted", func(f *gitFixture) {
			f.write("src/New.tsx", "export default 2")
			f.git("add", "--", "src/New.tsx")
		}, "src/New.tsx"},
		{"untracked file", func(f *gitFixture) {
			f.write("notes.md", "scratch")
		}, "notes.md"},
		{"deleted tracked file", func(f *gitFixture) {
			if err := os.Remove(filepath.Join(f.root, "index.html")); err != nil {
				f.t.Fatal(err)
			}
		}, "index.html"},
		{"renamed file", func(f *gitFixture) {
			f.git("mv", "index.html", "entry.html")
		}, "entry.html"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := cleanAppFixture(t, "")
			tc.mutate(f)
			_, err := guardOn(t, f.root, false)
			if err == nil {
				t.Fatalf("%s must be refused — the bundle differs from HEAD", tc.name)
			}
			if !errors.Is(err, ErrDirtyWorkTree) {
				t.Errorf("%s: refusal must carry ErrDirtyWorkTree, got %#v", tc.name, err)
			}
			if !strings.Contains(err.Error(), tc.wantPath) {
				t.Errorf("%s: the refusal must NAME the path %q, got:\n%s", tc.name, tc.wantPath, err.Error())
			}
			// 🔴 A mis-parsed rename record does not leak a RECOGNISABLE path —
			// it leaks a mangled one. The record after `R  entry.html` is the
			// bare original path `index.html`, and a parser that reads it as a
			// fresh entry takes its first two bytes as status letters and the
			// rest as a path: `in ex.html`. Grepping for the original filename
			// therefore finds nothing and the defect survives. Count instead.
			if tc.name == "renamed file" {
				if !strings.Contains(err.Error(), "1 path(s)") {
					t.Errorf("a rename is ONE change; its -z record's second field is the original path, "+
						"not a second entry. got:\n%s", err.Error())
				}
			}
		})
	}
}

// --- THE PORCELAIN PARSER, on literal records ---

// TestBundleDirtyPathsParsesTheZRecordLayout drives bundleDirtyPaths with
// hand-written `-z` output rather than a repository.
//
// 🔴 IT EXISTS BECAUSE THE RENAME CASE IS INVISIBLE FROM ABOVE. A repository
// fixture can only assert what the refusal SAYS, and a parser that fails to
// consume a rename's second field does not emit a recognisable extra path — it
// emits `in ex.html`, two bytes of the original filename read as status letters.
// Nothing a message-substring test looks for matches that. Here the entry list
// itself is the assertion.
//
// It also pins the two filters that a repository fixture cannot separate,
// because `git status -- .` never reports an out-of-subtree path in the first
// place: see TestBundleDirtyPathsDropsPathsOutsideThePackagedSubtree.
func TestBundleDirtyPathsParsesTheZRecordLayout(t *testing.T) {
	// Exactly the shape git emits: NUL-terminated records, the rename's ORIGINAL
	// path following the new one as its own field.
	raw := strings.Join([]string{
		"R  entry.html\x00",
		"index.html\x00",
		" M src/App.tsx\x00",
		"?? notes.md\x00",
		"?? dist/assets/app.js\x00", // excluded by the packager
		"?? prev.zip\x00",           // excluded by the packager
	}, "")

	got := bundleDirtyPaths(raw, "")
	var paths []string
	for _, e := range got {
		paths = append(paths, e.code+"|"+e.path)
	}
	want := []string{"R |entry.html", " M|src/App.tsx", "??|notes.md"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Errorf("bundleDirtyPaths =\n  %v\nwant\n  %v\n"+
			"A rename is ONE entry (its second field is the original path, not a new record); "+
			"dist/ and *.zip are not bundle content.", paths, want)
	}
}

// TestBundleDirtyPathsDropsPathsOutsideThePackagedSubtree pins the prefix
// filter on its own.
//
// 🔴 AND IT RECORDS A KNOWN REDUNDANCY. The `-- .` pathspec on the status call
// and this prefix test select the SAME set — git's `.` pathspec means "paths
// under the cwd", and `prefix` is that cwd spelled repo-root-relative — so
// removing the pathspec is an EQUIVALENT mutation as far as the verdict goes,
// and a mutation battery reports it as a survivor. Measured: with the pathspec
// removed, all 2321 tests still pass. The pathspec is kept for COST (git stops
// walking the rest of a monorepo), the prefix test for CORRECTNESS, and this
// test is what keeps the correctness half from being deleted as "already
// covered by the pathspec" — at which point the redundancy would be gone and
// nothing would notice.
func TestBundleDirtyPathsDropsPathsOutsideThePackagedSubtree(t *testing.T) {
	raw := strings.Join([]string{
		" M packages/my-block/src/App.tsx\x00",
		" M packages/other/index.ts\x00",
		"?? tools/script.sh\x00",
	}, "")
	got := bundleDirtyPaths(raw, "packages/my-block/")
	if len(got) != 1 || got[0].path != "src/App.tsx" {
		t.Errorf("bundleDirtyPaths(prefix=packages/my-block/) = %v, want exactly [src/App.tsx] — "+
			"a sibling package is not in this bundle, and the surviving path must be relative to the packaged dir", got)
	}
}

// --- WHICH DIRECTORY: the tree being PACKAGED, not the cwd ---

// TestDirtyGuardInspectsThePackagedDirNotTheCwd is decision 1 stated as a test.
// `app submit [dir]` takes a positional directory, so the two can be different
// repositories — and a guard that answers about the wrong one is worse than no
// guard, because it is confidently wrong in BOTH directions.
func TestDirtyGuardInspectsThePackagedDirNotTheCwd(t *testing.T) {
	// Arm A: cwd's repo is FILTHY, the packaged repo is clean → must proceed.
	t.Run("dirty cwd, clean target", func(t *testing.T) {
		target := cleanAppFixture(t, "")
		other := newGitFixture(t)
		other.write("readme.md", "x")
		other.commit("init", "readme.md")
		other.write("readme.md", "changed")
		other.write("junk.tsx", "untracked")
		t.Chdir(other.root)

		warn, err := guardOn(t, target.root, false)
		if err != nil {
			t.Fatalf("the PACKAGED tree is clean; the cwd's repo is irrelevant. got: %v\nwarnings:\n%s", err, warn)
		}
	})

	// Arm B: the exact inverse, so a guard that simply always says "clean"
	// cannot pass both arms.
	t.Run("clean cwd, dirty target", func(t *testing.T) {
		target := cleanAppFixture(t, "")
		target.write("src/App.tsx", "uncommitted")
		clean := newGitFixture(t)
		clean.write("readme.md", "x")
		clean.commit("init", "readme.md")
		t.Chdir(clean.root)

		_, err := guardOn(t, target.root, false)
		if err == nil {
			t.Fatal("the PACKAGED tree is dirty and must be refused, however clean the cwd's repo is")
		}
		if !strings.Contains(err.Error(), "src/App.tsx") {
			t.Errorf("the refusal must name the packaged tree's path, got:\n%s", err.Error())
		}
	})
}

// TestDirtyGuardScopesToThePackagedSubtree — a monorepo. The app lives in
// packages/my-block; a sibling package's uncommitted work is not in this bundle
// and must not block this submit, while the app's own is.
//
// It also pins the PATH SPELLING: the refusal names paths relative to the
// packaged directory (the paths that are in the zip), not repo-root-relative
// ones.
func TestDirtyGuardScopesToThePackagedSubtree(t *testing.T) {
	f := cleanAppFixture(t, "packages/my-block")
	dir := f.appDir("packages/my-block")

	// A sibling package is filthy — not our bundle.
	f.write("packages/other/index.ts", "uncommitted elsewhere")
	f.write("tools/script.sh", "also elsewhere")

	warn, err := guardOn(t, dir, false)
	if err != nil {
		t.Fatalf("a SIBLING package's uncommitted work is not in this bundle and must not block the submit; got: %v", err)
	}
	if strings.Contains(warn, "packages/other") {
		t.Errorf("the sibling package must not even be mentioned, got warnings:\n%s", warn)
	}

	// Now dirty the packaged subtree itself.
	f.write("packages/my-block/src/App.tsx", "uncommitted here")
	_, err = guardOn(t, dir, false)
	if err == nil {
		t.Fatal("the PACKAGED subtree is dirty and must be refused")
	}
	msg := err.Error()
	if !strings.Contains(msg, "src/App.tsx") {
		t.Errorf("the path should be named relative to the packaged dir (that is its path in the zip), got:\n%s", msg)
	}
	if strings.Contains(msg, "packages/my-block/src/App.tsx") {
		t.Errorf("the path is repo-root-relative — the --show-prefix arithmetic did not run:\n%s", msg)
	}
	if strings.Contains(msg, "packages/other") {
		t.Errorf("the refusal names a sibling package that is not in this bundle:\n%s", msg)
	}
}

// --- WHAT COUNTS AS DIRTY: the packager's own exclusions ---

// TestDirtyGuardIgnoresWhatTheBundleDoesNotCarry is decision 2's boundary. None
// of these reaches the server, so none of them is a difference between the
// bundle and HEAD — and firing on them is how a guard teaches everyone to pass
// --allow-dirty by reflex.
func TestDirtyGuardIgnoresWhatTheBundleDoesNotCarry(t *testing.T) {
	f := cleanAppFixture(t, "")
	// The exact residue a normal working session leaves.
	f.write("node_modules/react/index.js", "vendored")
	f.write("dist/assets/app.js", "built")
	f.write("coverage/lcov.info", "measured")
	f.write("custom-generators-0.6.0.zip", "a previous --package-only run")
	f.write(".env.local", "VITE_LIVE_BLOCK_TOKEN=secret")
	f.write(".envrc", "use flake")

	warn, err := guardOn(t, f.root, false)
	if err != nil {
		t.Fatalf("none of these reaches the bundle, so none is a difference between the bundle and HEAD; got: %v", err)
	}
	// A GITIGNORED file is invisible to --porcelain for the same reason, and
	// that has to hold too.
	f.write(".gitignore", "secrets.txt\n")
	f.commit("ignore", ".gitignore")
	f.write("secrets.txt", "nope")
	if _, err := guardOn(t, f.root, false); err != nil {
		t.Fatalf("a gitignored file is not tracked content and must not block a submit; got: %v", err)
	}
	_ = warn
}

// TestDirtyGuardStillSeesWhatTheBundleDOESCarry is the negative control on the
// filter above: the same shape of file, but one the packager KEEPS. Without this
// the exclusion test is satisfied by a guard that excludes everything.
func TestDirtyGuardStillSeesWhatTheBundleDOESCarry(t *testing.T) {
	for _, rel := range []string{
		".env.example",     // dotenv, but allow-listed BY NAME and uploaded
		"docs/dist.md",     // "dist" as part of a FILE name, not a directory
		"src/App.tsx.orig", // editor residue the packager does not exclude — it ships
		"assets/logo.svg",  // ordinary content
	} {
		t.Run(rel, func(t *testing.T) {
			f := cleanAppFixture(t, "")
			f.write(rel, "content")
			_, err := guardOn(t, f.root, false)
			if err == nil {
				t.Fatalf("%s IS packaged into the bundle, so an uncommitted one must be refused", rel)
			}
			if !strings.Contains(err.Error(), rel) {
				t.Errorf("the refusal must name %s, got:\n%s", rel, err.Error())
			}
		})
	}
}

// --- NEGATIVE CONTROLS: every case that must still SUCCEED ---

// 🔴 TestDirtyGuardNoRepoAtAllProceedsSilently IS THE ONE THAT BREAKS THE
// SCAFFOLD PATH IF IT REGRESSES. `civitai app scaffold` produces a plain
// directory with no repository, and submitting one is completely ordinary — the
// issue says a hard refusal there "should not be considered". This is that
// sentence as a test, and it must proceed AND stay silent: a warning on every
// scaffolded submit is noise on the happiest path there is.
func TestDirtyGuardNoRepoAtAllProceedsSilently(t *testing.T) {
	requireGit(t)
	gitFixtureEnv(t)
	dir := t.TempDir()
	writeManifestVersion(t, dir, dirtySlug, "0.6.1")
	// An untracked-looking mess, which is exactly what a scaffold leaves.
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("scratch"), 0o600); err != nil {
		t.Fatal(err)
	}

	warn, err := guardOn(t, dir, false)
	if err != nil {
		t.Fatalf("a directory with NO git repo must submit exactly as before — this is the scaffold path. got: %v", err)
	}
	if warn != "" {
		t.Errorf("no repo must be SILENT; a warning on every scaffolded submit is noise. got:\n%s", warn)
	}
}

// TestDirtyGuardCleanRepoProceeds — the ordinary release: everything committed
// and pushed. No refusal, and no warning either.
func TestDirtyGuardCleanRepoProceeds(t *testing.T) {
	f := cleanAppFixture(t, "")
	f.publishHEAD()

	warn, err := guardOn(t, f.root, false)
	if err != nil {
		t.Fatalf("a clean, pushed tree is the happy path and must proceed; got: %v", err)
	}
	if warn != "" {
		t.Errorf("a clean, pushed tree must be silent, got:\n%s", warn)
	}
}

// TestDirtyGuardAllowDirtySubmitsAnyway — the escape hatch, on the same tree the
// positive control refuses. It short-circuits before any subprocess: a release
// script that has already decided should not pay for an answer it discards.
func TestDirtyGuardAllowDirtySubmitsAnyway(t *testing.T) {
	f := cleanAppFixture(t, "")
	f.write("src/App.tsx", "uncommitted")

	// Prove the same tree really IS refused without the flag, or this test is a
	// claim about a clean repo.
	if _, err := guardOn(t, f.root, false); err == nil {
		t.Fatal("control: this tree must be refused WITHOUT --allow-dirty, or the flag proves nothing")
	}

	calls := 0
	counting := func(dir string, args ...string) (string, error) {
		calls++
		return gitOutput(dir, args...)
	}
	var warn bytes.Buffer
	if err := checkWorkTreeClean(counting, &warn, f.root, dirtySlug, "0.6.1", true); err != nil {
		t.Fatalf("--allow-dirty must permit a dirty tree, got: %v", err)
	}
	if calls != 0 {
		t.Errorf("--allow-dirty made %d git call(s); it must short-circuit before any subprocess", calls)
	}
	if warn.String() != "" {
		t.Errorf("--allow-dirty must be silent, got:\n%s", warn.String())
	}
}

// --- WARN, DO NOT BLOCK: clean but unpushed ---

// TestDirtyGuardWarnsWhenHEADIsOnNoRemote asserts BOTH halves — the warning is
// present AND the submit proceeds. A guard that refused here would block every
// author who commits before pushing, which is legitimate.
func TestDirtyGuardWarnsWhenHEADIsOnNoRemote(t *testing.T) {
	f := cleanAppFixture(t, "") // committed, never published

	warn, err := guardOn(t, f.root, false)
	if err != nil {
		t.Fatalf("an unpushed HEAD must NOT block — submitting before pushing is legitimate. got: %v", err)
	}
	if !strings.Contains(warn, "HEAD is on no remote") {
		t.Errorf("an unpushed HEAD must be announced, got warnings:\n%s", warn)
	}
	if !strings.Contains(warn, "only on this machine") {
		t.Errorf("the warning should say what is actually at stake (traceability), got:\n%s", warn)
	}
}

// TestDirtyGuardIsSilentOncePushed is the negative control on that warning: the
// same repo, one ref later. Without it, "warns when unpushed" is satisfied by a
// guard that warns always.
func TestDirtyGuardIsSilentOncePushed(t *testing.T) {
	f := cleanAppFixture(t, "")
	if warn, _ := guardOn(t, f.root, false); !strings.Contains(warn, "HEAD is on no remote") {
		t.Fatalf("control: the unpushed warning must fire before publishing, got:\n%s", warn)
	}
	f.publishHEAD()
	warn, err := guardOn(t, f.root, false)
	if err != nil {
		t.Fatalf("a pushed clean tree must proceed, got: %v", err)
	}
	if strings.Contains(warn, "HEAD is on no remote") {
		t.Errorf("HEAD is now on a remote ref; the warning must stop, got:\n%s", warn)
	}
}

// --- DEGRADATION: the two states real git cannot be asked to produce ---

// TestDirtyGuardTreatsAMissingGitLikeNoRepo. The brief is explicit: no `git` on
// PATH must degrade like "not a repo", not crash and not refuse.
func TestDirtyGuardTreatsAMissingGitLikeNoRepo(t *testing.T) {
	noGit := func(dir string, args ...string) (string, error) { return "", errGitUnavailable }
	var warn bytes.Buffer
	if err := checkWorkTreeClean(noGit, &warn, t.TempDir(), dirtySlug, "0.6.1", false); err != nil {
		t.Fatalf("no git binary must degrade like no repo, got: %v", err)
	}
	if warn.String() != "" {
		t.Errorf("no git binary must be silent, like no repo, got:\n%s", warn.String())
	}
}

// TestDirtyGuardWarnsAndProceedsWhenStatusFails — the tree IS a repo, but the
// status read failed (a corrupt index, a lock held by another process). This is
// an accident preventer, not an authorization check: blocking here would take
// every submit down with one broken checkout. It must not be SILENT either — a
// silent fail-open is how a guard becomes decoration.
func TestDirtyGuardWarnsAndProceedsWhenStatusFails(t *testing.T) {
	boom := errors.New("fatal: index file corrupt")
	stub := func(dir string, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "rev-parse" {
			return "true\n\n", nil
		}
		return "", boom
	}
	var warn bytes.Buffer
	if err := checkWorkTreeClean(stub, &warn, "/some/dir", dirtySlug, "0.6.1", false); err != nil {
		t.Fatalf("a failed status read must not block the submit, got: %v", err)
	}
	s := warn.String()
	if !strings.Contains(s, "could not check") {
		t.Errorf("the fail-open must be announced, got:\n%s", s)
	}
	if !strings.Contains(s, "index file corrupt") {
		t.Errorf("the warning should carry the underlying error so the cause is visible, got:\n%s", s)
	}
}

// TestDirtyGuardProceedsOnABareRepo — `--is-inside-work-tree` answers `false`
// where there is no working tree to be dirty.
func TestDirtyGuardProceedsOnABareRepo(t *testing.T) {
	stub := func(dir string, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "rev-parse" {
			return "false\n", nil
		}
		return "", errors.New("nothing else should be asked")
	}
	var warn bytes.Buffer
	if err := checkWorkTreeClean(stub, &warn, "/some/dir", dirtySlug, "0.6.1", false); err != nil {
		t.Fatalf("no working tree means nothing can be dirty, got: %v", err)
	}
	if warn.String() != "" {
		t.Errorf("must be silent, got:\n%s", warn.String())
	}
}

// --- THE MESSAGE ---

// TestDirtyRefusalCapsThePathListAndSaysHowManyItHid. A tree with 40
// uncommitted files must not emit 40 lines into a terminal the reader then has
// to scroll past to find the fix — but the count must still be honest.
func TestDirtyRefusalCapsThePathListAndSaysHowManyItHid(t *testing.T) {
	var dirty []gitStatusEntry
	for i := 0; i < 25; i++ {
		dirty = append(dirty, gitStatusEntry{code: "??", path: fmt.Sprintf("src/f%02d.tsx", i)})
	}
	err := dirtyWorkTreeError(dirtySlug, "0.6.1", dirty)
	msg := err.Error()
	if !strings.Contains(msg, "25 path(s)") {
		t.Errorf("the TOTAL must be stated, got:\n%s", msg)
	}
	if !strings.Contains(msg, "… and 15 more") {
		t.Errorf("the hidden remainder must be counted, got:\n%s", msg)
	}
	if strings.Contains(msg, "src/f24.tsx") {
		t.Errorf("the list must be capped at %d, got:\n%s", dirtyPathListCap, msg)
	}
	if !strings.Contains(msg, "src/f00.tsx") {
		t.Errorf("the first paths must still be shown, got:\n%s", msg)
	}
}

// TestDirtyRefusalWithNoBlockIdStillReadsAsASentence — `manifest.Load` does not
// require blockId and `--skip-validate` waives the schema, so the slug can
// legitimately arrive EMPTY here. That is not a reason to skip the guard the way
// the VERSION guard must (it has no app to compare against; this one has a work
// tree either way), but it is a reason not to splice an empty string into the
// first line: "refusing to submit @0.6.1" reads as a bug in the CLI.
func TestDirtyRefusalWithNoBlockIdStillReadsAsASentence(t *testing.T) {
	err := dirtyWorkTreeError("", "0.6.1", []gitStatusEntry{{code: "??", path: "src/App.tsx"}})
	msg := err.Error()
	if strings.Contains(msg, "submit @") || strings.Contains(msg, "submit  ") {
		t.Errorf("an empty blockId was spliced in verbatim:\n%s", msg)
	}
	if !strings.Contains(msg, "refusing to submit this app from a dirty git work tree") {
		t.Errorf("with no blockId the refusal should still name what it is refusing, got:\n%s", msg)
	}
	if !strings.Contains(msg, "src/App.tsx") || !strings.Contains(msg, "--allow-dirty") {
		t.Errorf("the path and the escape hatch must survive the no-slug branch, got:\n%s", msg)
	}
	// NEGATIVE CONTROL: with a slug present the version IS attached, so the
	// branch above is a branch and not the only behaviour.
	withSlug := dirtyWorkTreeError(dirtySlug, "0.6.1", []gitStatusEntry{{code: "??", path: "x"}}).Error()
	if !strings.Contains(withSlug, "custom-generators@0.6.1") {
		t.Errorf("with a slug the refusal must name app@version, got:\n%s", withSlug)
	}
}

// --- END TO END: the guard is actually WIRED into `civitai app submit` ---
//
// 🔴 A unit test on checkWorkTreeClean proves the predicate, never that the
// command REACHES it.

func dirtySubmitServer(t *testing.T) (*httptest.Server, *bool) {
	t.Helper()
	submitted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, appapi.SubmissionsPath) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"submissions": []appapi.Submission{}})
			return
		}
		submitted = true
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"publishRequestId": "pubreq_new", "slug": dirtySlug, "version": "0.6.1", "status": "pending",
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &submitted
}

// TestAppSubmitRefusesADirtyTreeAndNeverUploads — the reachability half of the
// positive control.
func TestAppSubmitRefusesADirtyTreeAndNeverUploads(t *testing.T) {
	f := cleanAppFixture(t, "")
	f.write("src/App.tsx", "uncommitted")
	withStdinTTY(t, false)
	srv, submitted := dirtySubmitServer(t)
	withGuardEnv(t, srv)

	stdout, _, err := run(t, "app", "submit", f.root, "--yes")
	if err == nil {
		t.Fatalf("`app submit` must refuse a dirty tree; stdout:\n%s", stdout)
	}
	if !errors.Is(err, ErrDirtyWorkTree) {
		t.Errorf("the command error must carry ErrDirtyWorkTree (that is what pins exit code 1), got %#v", err)
	}
	if *submitted {
		t.Error("the submit route was hit — the guard must refuse BEFORE uploading anything")
	}
	if strings.Contains(stdout, "Packaged ") {
		t.Errorf("the guard must run before packaging, got stdout:\n%s", stdout)
	}
}

// TestAppSubmitAllowDirtyUploadsAnyway — the reachability half of the escape
// hatch: the same refused invocation plus --allow-dirty reaches the route.
func TestAppSubmitAllowDirtyUploadsAnyway(t *testing.T) {
	f := cleanAppFixture(t, "")
	f.write("src/App.tsx", "uncommitted")
	withStdinTTY(t, false)
	srv, submitted := dirtySubmitServer(t)
	withGuardEnv(t, srv)

	stdout, stderr, err := run(t, "app", "submit", f.root, "--yes", "--allow-dirty")
	if err != nil {
		t.Fatalf("--allow-dirty must submit; got %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !*submitted {
		t.Error("--allow-dirty must reach the submit route")
	}
}

// 🔴 TestAppSubmitFromANonRepoStillUploads is the scaffold path END TO END, and
// it is the unmissable one: if this goes red, `civitai app submit` has stopped
// working for every app that was scaffolded and never `git init`ed.
func TestAppSubmitFromANonRepoStillUploads(t *testing.T) {
	requireGit(t)
	gitFixtureEnv(t)
	dir := t.TempDir()
	writeManifestVersion(t, dir, dirtySlug, "0.6.1")
	withStdinTTY(t, false)
	srv, submitted := dirtySubmitServer(t)
	withGuardEnv(t, srv)

	stdout, stderr, err := run(t, "app", "submit", dir, "--yes")
	if err != nil {
		t.Fatalf("a scaffolded app with NO git repo must still submit; got %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !*submitted {
		t.Error("the no-repo path must reach the submit route")
	}
}

// TestAppSubmitCleanUnpushedWarnsAndStillUploads — the warn-not-block row,
// end to end, asserting BOTH halves.
func TestAppSubmitCleanUnpushedWarnsAndStillUploads(t *testing.T) {
	f := cleanAppFixture(t, "") // clean, never published
	withStdinTTY(t, false)
	srv, submitted := dirtySubmitServer(t)
	withGuardEnv(t, srv)

	stdout, stderr, err := run(t, "app", "submit", f.root, "--yes")
	if err != nil {
		t.Fatalf("an unpushed HEAD must not block; got %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !*submitted {
		t.Error("the submit must still reach the route — this is a warning, not a refusal")
	}
	if !strings.Contains(stderr, "HEAD is on no remote") {
		t.Errorf("the unpushed warning must reach the user, got stderr:\n%s", stderr)
	}
}

// TestDirtyGuardRefusesBeforePrompting is decision 4 stated as a test: the guard
// is purely local, so there is no reason to ask "Submit for review? [y/N]" about
// a submit already decided against. On an interactive TTY with no --yes, a dirty
// tree must refuse WITHOUT the prompt ever being printed.
func TestDirtyGuardRefusesBeforePrompting(t *testing.T) {
	f := cleanAppFixture(t, "")
	f.write("src/App.tsx", "uncommitted")
	withStdinTTY(t, true) // an interactive shell: confirmSubmit WOULD prompt
	srv, submitted := dirtySubmitServer(t)
	withGuardEnv(t, srv)

	stdout, _, err := run(t, "app", "submit", f.root)
	if err == nil {
		t.Fatalf("a dirty tree must be refused; stdout:\n%s", stdout)
	}
	if !errors.Is(err, ErrDirtyWorkTree) {
		t.Errorf("the refusal must be the DIRTY one, not the confirmation's — got %#v", err)
	}
	for _, prompt := range []string{"About to submit", "Submit for review?"} {
		if strings.Contains(stdout, prompt) {
			t.Errorf("the guard runs before the confirmation, so %q must not be printed:\n%s", prompt, stdout)
		}
	}
	if *submitted {
		t.Error("nothing may reach the submit route")
	}
}
