package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// THE REAL `gitRunner`, NOT THE STUB.
//
// 🔴 EVERY OTHER TEST IN THIS PACKAGE REPLACES `gitRunner` WITH A FAKE, WHICH IS
// EXACTLY WHY THIS GAP EXISTED. Stubbing it out is right for testing `app pull`'s
// branching — nobody wants a clone over the network in a unit test — but it means
// the real closure's own behaviour (which binary it runs, in which directory,
// with which environment) had no coverage at all. A property nothing exercises is
// a property nothing protects.
//
// So this test deliberately calls the package var itself, with a git subcommand
// chosen to be a WRITE that is cheap, offline, and observable in the filesystem
// afterwards: `git config --local` writes one key into the repository's own
// config. Which repository it lands in is precisely the question.

// TestGitRunnerScrubsTheRelocatingEnvironment.
//
// The hazard: git's relocating variables BEAT `-C` and `c.Dir`, and git exports
// GIT_DIR to its hooks — so `civitai app pull` run from a `pre-push` hook (or
// under `git rebase -x`) inherits a GIT_DIR pointing at the hook's repository.
// Every call through this runner is a write, so an unscrubbed runner would
// clone/fetch/merge into that repository instead of the one the user named.
//
// The mutation this kills is the one-line removal of
// `c.Env = gitScrubbedEnv(os.Environ())`.
func TestGitRunnerScrubsTheRelocatingEnvironment(t *testing.T) {
	requireGit(t)
	gitFixtureEnv(t)

	target := t.TempDir()
	decoy := t.TempDir()
	mustGit(t, target, "init", "-q", "-b", "fixture-main")
	mustGit(t, decoy, "init", "-q", "-b", "decoy-main")

	// Aim the environment at the decoy, the way a git hook would. Both are set
	// because git wants a work tree to go with a relocated GIT_DIR.
	t.Setenv("GIT_DIR", filepath.Join(decoy, ".git"))
	t.Setenv("GIT_WORK_TREE", decoy)

	// A write, through the REAL runner, explicitly aimed at `target` with -C —
	// the same argument shape the sync call sites use (app_pull.go passes -C in
	// args with dir == "").
	if err := gitRunner("", "-C", target, "config", "--local", "probe.scrubbed", "yes"); err != nil {
		t.Fatalf("gitRunner: %v", err)
	}

	if got := configValue(t, target, "probe.scrubbed"); got != "yes" {
		t.Errorf("the write did not land in the repository named by -C (%s): probe.scrubbed = %q", target, got)
	}
	if got := configValue(t, decoy, "probe.scrubbed"); got != "" {
		t.Errorf("gitRunner obeyed GIT_DIR instead of -C and wrote into the DECOY repository (%s): probe.scrubbed = %q.\n"+
			"Every call through this runner is a write (clone/fetch/merge), so this is a merge into the wrong repository, "+
			"not merely a wrong answer.", decoy, got)
	}
}

// TestGitRunnerLeavesOrdinaryConfigAlone is the other half, and it is what stops
// the scrub being "fixed" later by neutralising the whole environment.
//
// gitScrubbedEnv drops ONLY the relocating variables. The user's git config and
// everything else must survive, because those are their own answers about their
// own repository — a runner that reset them would change behaviour nobody asked
// it to change. `GIT_AUTHOR_NAME` is not in the relocating set, so it must still
// reach git.
//
// 🔴 IT IS AN INVARIANT GUARD, NOT A REGRESSION TEST FOR THIS CHANGE — it stays
// GREEN with the payload line deleted, so do not count it as coverage of the
// scrub itself. The mutant it uniquely kills is `c.Env = []string{}` written AT
// THIS RUNNER, bypassing the helper: the helper's own test still passes (helper
// untouched), and the scrub test above still passes (an empty env contains no
// GIT_DIR to obey), so this is the only thing standing between that edit and
// main. Narrow, but a plausible future "hardening".
//
// ⚠️ AND IT DOES NOT FAIL WHERE ITS NAME SUGGESTS. Both realistic over-scrub
// mutants red it at the `gitRunner commit` line — git exits 128 with "unable to
// auto-detect email address" once the identity variables are gone — not at the
// `%an` comparison below. The test works; the `%an` assertion is not what does
// the killing.
func TestGitRunnerLeavesOrdinaryConfigAlone(t *testing.T) {
	requireGit(t)
	gitFixtureEnv(t)

	repo := t.TempDir()
	mustGit(t, repo, "init", "-q", "-b", "fixture-main")

	t.Setenv("GIT_AUTHOR_NAME", "Scrub Probe")
	t.Setenv("GIT_AUTHOR_EMAIL", "probe@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "Scrub Probe")
	t.Setenv("GIT_COMMITTER_EMAIL", "probe@example.invalid")

	if err := gitRunner("", "-C", repo, "commit", "-q", "--allow-empty", "-m", "probe"); err != nil {
		t.Fatalf("gitRunner commit: %v", err)
	}

	out := mustGitOut(t, repo, "log", "-1", "--format=%an")
	if strings.TrimSpace(out) != "Scrub Probe" {
		t.Errorf("a non-relocating environment variable was dropped: author = %q, want %q.\n"+
			"gitScrubbedEnv must remove ONLY the variables that relocate the repository.",
			strings.TrimSpace(out), "Scrub Probe")
	}
}

// mustGit runs a git subcommand directly (NOT through gitRunner) and fails the
// test on error. Fixtures must not be built with the thing under test.
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// mustGitOut is mustGit with stdout, for the one assertion that reads git's
// answer rather than the filesystem.
func mustGitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return string(out)
}

// configValue reads one local config key, returning "" when it is unset.
//
// 🔴 IT MUST NOT READ THROUGH A RELOCATABLE `git config`. Measured: with GIT_DIR
// aimed at the decoy, `git -C <target> config --local --get probe.scrubbed`
// returns the DECOY's value — so a `-C`-based reader would have read the same
// repository for both assertions and made the scrub test silently vacuous.
//
// ⚠️ It is NOT the only immune reader, and an earlier version of this comment
// claimed it was. `git config -f <path> --get <key>` is equally immune
// (measured: correct value under the same poisoned env, rc 1 + empty on an unset
// key). Reading the file is kept because it needs no subprocess and cannot be
// affected by any future env variable at all; the one-exec form is a fine
// alternative, not a worse one.
//
// The parse is deliberately minimal and only has to hold for a fresh `git init`
// fixture: it is section-BLIND (any section with the same leaf key wins) and
// does not handle `[section "sub"]`, multi-values or quoted values. The key is
// required to be dotted, because a dotless one would otherwise index out of
// range rather than fail with something a reader can act on.
func configValue(t *testing.T, repo, key string) string {
	t.Helper()
	_, leaf, dotted := strings.Cut(key, ".")
	if !dotted {
		t.Fatalf("configValue needs a dotted key like `section.name`, got %q", key)
	}
	b, err := os.ReadFile(filepath.Join(repo, ".git", "config"))
	if err != nil {
		t.Fatalf("reading %s config: %v", repo, err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.TrimSpace(k) == leaf {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
