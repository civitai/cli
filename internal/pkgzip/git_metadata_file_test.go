package pkgzip

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TESTS FOR ISSUE #409: `.git` is a FILE in a linked worktree and in a
// submodule, and the exclusion used to be keyed on the entry's TYPE rather than
// its NAME.
//
// 🔴 THE DEFECT WAS NOT "we forgot .git" — `.git` was on excludedDirs from the
// start. It was that the name was consulted ONLY inside `if d.IsDir()`, so the
// rule held for the shape the author of it had in mind (a clone) and silently
// did not hold for the two shapes that are equally normal (a linked worktree,
// a submodule). In those, `.git` is a regular file holding `gitdir: <abspath>`;
// it passed the directory gate (not a directory) and the file gate
// (isExcludedFile only knew *.zip and .env*) and was packaged.
//
// What that cost: Forgejo rejects a bundle containing `.git` with
// "path contains a malformed path component [path: .git]" — a 400 that names
// neither the CLI, the bundle, nor the author's checkout, so `app submit` was a
// hard stop with no route to the cause. And the file's CONTENT is an absolute
// path on the author's machine, which the packager should not be assembling
// into an upload whatever the server does with it.
//
// The fixtures below build REAL worktrees and REAL submodules rather than
// hand-writing a `.git` file, because the claim under test is about what git
// actually produces. Each one asserts its own precondition (that `.git` really
// came out a regular file) — a fixture that silently produced a directory would
// exercise the path that always worked and pass for the wrong reason.

// gitEnv isolates every git invocation here from the machine's own config.
func gitEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_AUTHOR_NAME", "cli-test")
	t.Setenv("GIT_AUTHOR_EMAIL", "cli-test@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "cli-test")
	t.Setenv("GIT_COMMITTER_EMAIL", "cli-test@example.invalid")
}

// runGit runs a git subcommand in dir and fails the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out, errb bytes.Buffer
	c.Stdout = &out
	c.Stderr = &errb
	if err := c.Run(); err != nil {
		t.Fatalf("git -C %s %s: %v\n%s", dir, strings.Join(args, " "), err, errb.String())
	}
	return strings.TrimRight(out.String(), "\r\n")
}

// tempRepo creates a real git repository in a fresh temp dir — NEVER inside the
// CLI's own checkout — carrying a minimal packageable app.
func tempRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH — the real-repo fixtures cannot be built")
	}
	gitEnv(t)
	// EvalSymlinks: on macOS t.TempDir() lives under a symlink and git answers
	// with the resolved path, which makes the fixture's own paths disagree.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	runGit(t, root, "init", "-q", "-b", "fixture-main")
	writeFile(t, root, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, root, "package.json", "{}")
	writeFile(t, root, "src/App.tsx", "export default 1")
	// Negative control, carried by every fixture: names that merely START with
	// ".git" are bundle content and must survive whatever the fix does.
	writeFile(t, root, ".gitignore", "node_modules\n")
	writeFile(t, root, ".gitattributes", "* text=auto\n")
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-q", "-m", "fixture")
	return root
}

// requireGitIsAFile is the fixture's own precondition. Without it a fixture that
// produced a `.git` DIRECTORY would exercise the branch that never broke, and
// the test would pass while saying nothing about #409.
func requireGitIsAFile(t *testing.T, dir string) {
	t.Helper()
	info, err := os.Lstat(filepath.Join(dir, ".git"))
	if err != nil {
		t.Fatalf("fixture has no .git at %s: %v — nothing to exclude, so this test is vacuous", dir, err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("fixture's .git is %v, not a regular file — this fixture exercises the DIRECTORY "+
			"branch, which never had the bug; #409 is unobserved by it", info.Mode())
	}
	b, err := os.ReadFile(filepath.Join(dir, ".git"))
	if err != nil {
		t.Fatalf("read fixture .git: %v", err)
	}
	if !strings.HasPrefix(string(b), "gitdir:") {
		t.Fatalf("fixture's .git does not hold a gitdir: pointer (%q) — not the shape #409 is about", string(b))
	}
}

// TestBuildExcludesGitFileInLinkedWorktree is the issue's own reproduction:
// package from a linked worktree, where `.git` is a file.
func TestBuildExcludesGitFileInLinkedWorktree(t *testing.T) {
	root := tempRepo(t)
	wt := filepath.Join(filepath.Dir(root), "linked-worktree")
	runGit(t, root, "worktree", "add", "-q", "--detach", wt, "HEAD")
	t.Cleanup(func() { _ = exec.Command("git", "-C", root, "worktree", "remove", "--force", wt).Run() })

	requireGitIsAFile(t, wt)

	res, err := Build(wt)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := namesInZip(t, res.Zip)
	sort.Strings(got)

	for _, f := range got {
		if f == ".git" || strings.HasPrefix(f, ".git/") {
			t.Errorf("the bundle carries %q from a linked worktree (#409). Forgejo rejects the push with "+
				"\"path contains a malformed path component [path: .git]\", and the file's body is an "+
				"absolute path on the author's machine. Full bundle: %v", f, got)
		}
	}

	// The bundle must still be the app. Asserted as an exact set so a fix that
	// over-matches (dropping .gitignore / .gitattributes) is a FAILURE here, not
	// a silently smaller upload.
	want := []string{".gitattributes", ".gitignore", "block.manifest.json", "package.json", "src/App.tsx"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("worktree bundle = %v, want %v", got, want)
	}

	// The count comparison the issue used as its tell: same commit, same files.
	clone := res.Files
	cloneRes, err := Build(root)
	if err != nil {
		t.Fatalf("Build(clone): %v", err)
	}
	if len(clone) != len(cloneRes.Files) {
		t.Errorf("worktree packaged %d file(s), clone packaged %d — the same commit must package identically "+
			"whatever the shape of the checkout (worktree=%v clone=%v)",
			len(clone), len(cloneRes.Files), clone, cloneRes.Files)
	}
}

// TestBuildExcludesGitFileInSubmodule covers the other shape, and it puts the
// `.git` file BELOW the root — the exclusion has to hold at any depth, not only
// at the directory being packaged.
func TestBuildExcludesGitFileInSubmodule(t *testing.T) {
	parent := tempRepo(t)

	child, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	runGit(t, child, "init", "-q", "-b", "fixture-main")
	writeFile(t, child, "lib.ts", "export const x = 1")
	runGit(t, child, "add", "-A")
	runGit(t, child, "commit", "-q", "-m", "child")

	// git >= 2.38 refuses file:// submodules unless told otherwise.
	runGit(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", "-q", child, "sub")
	runGit(t, parent, "commit", "-q", "-m", "add submodule")

	requireGitIsAFile(t, filepath.Join(parent, "sub"))

	res, err := Build(parent)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := namesInZip(t, res.Zip)
	sort.Strings(got)

	for _, f := range got {
		if f == "sub/.git" || strings.Contains(f, "/.git/") || strings.HasSuffix(f, "/.git") {
			t.Errorf("the bundle carries %q from a submodule (#409): a `.git` file below the root. "+
				"Full bundle: %v", f, got)
		}
	}

	// NEGATIVE CONTROLS, and the reason this fix must be an EXACT-NAME match
	// rather than a prefix: `.gitmodules` is what tells the platform the
	// submodule exists, and `.gitignore` / `.gitattributes` are ordinary project
	// files. Dropping any of them would be a silent content loss.
	want := []string{
		".gitattributes", ".gitignore", ".gitmodules",
		"block.manifest.json", "package.json", "src/App.tsx", "sub/lib.ts",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("submodule bundle = %v, want %v", got, want)
	}
}

// TestBuildExcludesVCSMetadataFilesAtAnyDepth is the same claim without a git
// binary: the exclusion is by NAME at every depth, for a file exactly as for a
// directory, and it stops at the exact name.
func TestBuildExcludesVCSMetadataFilesAtAnyDepth(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "src/App.tsx", "export default 1")

	// VCS metadata as a FILE — the #409 shape — at the root and nested.
	writeFile(t, dir, ".git", "gitdir: /home/someone/secret-layout/.git/worktrees/wt\n")
	writeFile(t, dir, "sub/.git", "gitdir: ../.git/modules/sub\n")
	writeFile(t, dir, "a/b/c/.git", "gitdir: /elsewhere\n")
	writeFile(t, dir, ".hg", "x")
	writeFile(t, dir, "vendor/.svn", "x")
	// VCS metadata as a DIRECTORY — the shape that always worked. Kept so a fix
	// cannot pass by moving the rule from one branch to the other.
	writeFile(t, dir, "nested/.git/HEAD", "ref: refs/heads/main")

	// NEGATIVE CONTROLS: real project files whose names merely begin with
	// ".git". A prefix match instead of an exact one silently deletes these.
	kept := []string{".gitignore", ".gitattributes", ".gitmodules", "src/.gitkeep", ".github/workflows/ci.yml"}
	for _, k := range kept {
		writeFile(t, dir, k, "x")
	}

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := namesInZip(t, res.Zip)
	sort.Strings(got)

	for _, f := range got {
		base := f
		if i := strings.LastIndex(f, "/"); i >= 0 {
			base = f[i+1:]
		}
		switch base {
		case ".git", ".hg", ".svn":
			t.Errorf("VCS metadata %q was packaged — the exclusion is keyed on the name, at any depth, "+
				"for a file as for a directory. Full bundle: %v", f, got)
		}
	}

	want := []string{
		".gitattributes", ".github/workflows/ci.yml", ".gitignore", ".gitmodules",
		"block.manifest.json", "src/.gitkeep", "src/App.tsx",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("bundle = %v, want %v (a difference here is either leaked VCS metadata or a "+
			"legitimately-named project file silently dropped)", got, want)
	}
}

// TestVCSNamesExcludedForEitherType pins the predicates the DIRTY-TREE GUARD
// calls (#411, internal/cmd/app_submit_dirty_guard.go). They are the same rule
// as Build's, reproduced for a caller that holds a path instead of a walk — so
// a change to Build that does not move these leaves the guard disagreeing with
// the bundle, which is a false refusal in one direction and a fail-open in the
// other.
func TestVCSNamesExcludedForEitherType(t *testing.T) {
	for _, n := range []string{".git", ".hg", ".svn"} {
		if !IsExcludedFile(n) {
			t.Errorf("IsExcludedFile(%q) = false — a worktree/submodule %s is a regular FILE, and the "+
				"packager would upload it", n, n)
		}
		if !IsExcludedPath(n) {
			t.Errorf("IsExcludedPath(%q) = false", n)
		}
		if !IsExcludedPath("sub/" + n) {
			t.Errorf("IsExcludedPath(%q) = false — the rule must hold at any depth", "sub/"+n)
		}
		if !IsExcludedEntry(n, 0) {
			t.Errorf("IsExcludedEntry(%q, regular) = false", n)
		}
		if !IsExcludedEntry(n, fs.ModeDir) {
			t.Errorf("IsExcludedEntry(%q, dir) = false — the directory case must not regress", n)
		}
	}
	// NEGATIVE CONTROLS on both predicates.
	for _, n := range []string{".gitignore", ".gitattributes", ".gitmodules", ".gitkeep", "git", "gitdir", ".githooks"} {
		if IsExcludedFile(n) {
			t.Errorf("IsExcludedFile(%q) = true — an exact-name rule must not swallow it", n)
		}
		if IsExcludedPath(n) {
			t.Errorf("IsExcludedPath(%q) = true", n)
		}
		if IsExcludedEntry("src/"+n, 0) {
			t.Errorf("IsExcludedEntry(%q, regular) = true", "src/"+n)
		}
	}
	// A DIRECTORY named .github is bundle content too (the prefix mutant's other
	// victim).
	if IsExcludedEntry(".github", fs.ModeDir) {
		t.Error("IsExcludedEntry(\".github\", dir) = true — .github/ is project content")
	}
}

// TestVCSMetadataNamesAreASubsetOfExcludedDirs is the LEDGER between the two
// maps. vcsMetadataNames names the entries whose exclusion is type-independent;
// every one of them must also be in excludedDirs, or the directory case quietly
// depends on which map a future edit touched.
func TestVCSMetadataNamesAreASubsetOfExcludedDirs(t *testing.T) {
	if len(vcsMetadataNames) == 0 {
		t.Fatal("CONTROL failure: vcsMetadataNames is empty, so this ledger checks nothing")
	}
	for n := range vcsMetadataNames {
		if _, ok := excludedDirs[n]; !ok {
			t.Errorf("%q is in vcsMetadataNames but not excludedDirs — a DIRECTORY by that name would "+
				"still be walked into, so the two halves of the rule disagree", n)
		}
		if !isExcludedFile(n) {
			t.Errorf("🔴 SEAM: %q is in vcsMetadataNames but isExcludedFile(%q) = false. The map is the "+
				"declaration; isExcludedFile is what ships.", n, n)
		}
	}
	// And the reverse must NOT hold: excludedDirs is deliberately wider, and
	// promoting all of it to a file rule would drop bundle content (a shell
	// script named `build`, a data file named `out`).
	if _, promoted := vcsMetadataNames["dist"]; promoted {
		t.Error("`dist` must not be type-independent — a plain FILE named dist is bundle content")
	}
	if isExcludedFile("build") || isExcludedFile("dist") || isExcludedFile("out") || isExcludedFile("node_modules") {
		t.Error("a regular file named after a build/dependency DIRECTORY is bundle content; only VCS " +
			"metadata names are excluded regardless of type")
	}
}
