package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

// TESTS FOR "HEAD IS ON NO REMOTE" WHEN THE ONLY REMOTE EVIDENCE IS A TAG.
//
// The symptom: civitai/app-submit-action@v1, on a tag push, printed "HEAD is on
// no remote" for a commit that IS the pushed tag's tip. actions/checkout on a tag
// fetches `refs/tags/<tag>` at depth 1 and detaches HEAD, so there is no
// refs/remotes/* at all and the guard's `for-each-ref --contains HEAD
// refs/remotes` came back empty. These fixtures rebuild that exact checkout
// against a real bare "remote" over file://, so ls-remote really runs.

const noRemoteWarning = "HEAD is on no remote"

// remoteTagCheckout publishes a clean app commit to a bare repo under tag
// v0.6.1 (annotated when annotated is true) — the tag ONLY, no branch — and
// returns a second repo that holds what actions/checkout leaves on a tag push:
// origin configured, the tag fetched at depth 1, HEAD detached at it.
func remoteTagCheckout(t *testing.T, annotated bool) *gitFixture {
	t.Helper()
	src := cleanAppFixture(t, "")
	if annotated {
		src.git("tag", "-a", "-m", "release", "v0.6.1")
	} else {
		src.git("tag", "v0.6.1")
	}

	bare := filepath.Join(t.TempDir(), "remote.git")
	src.git("init", "-q", "--bare", bare)
	bareURL := "file://" + filepath.ToSlash(bare)
	src.git("push", "-q", bareURL, "refs/tags/v0.6.1")

	ci := newGitFixture(t)
	ci.git("remote", "add", "origin", bareURL)
	ci.git("fetch", "-q", "--no-tags", "--depth=1", "origin", "+refs/tags/v0.6.1:refs/tags/v0.6.1")
	ci.git("checkout", "-q", "--detach", "v0.6.1")

	// PRECONDITION: this IS the state the old check misread. If a future git
	// starts creating remote-tracking refs here, the fixture no longer
	// reproduces the symptom and every "silent" assertion below would pass for
	// the wrong reason.
	if got := strings.TrimSpace(ci.git("for-each-ref", "--contains", "HEAD", "refs/remotes")); got != "" {
		t.Fatalf("fixture: a shallow tag checkout must have no remote-tracking ref containing HEAD, got %q", got)
	}
	if want, got := src.gitIn(src.root, "rev-parse", "HEAD"), ci.gitIn(ci.root, "rev-parse", "HEAD"); want != got {
		t.Fatalf("fixture: the checkout's HEAD %s is not the tagged commit %s", got, want)
	}
	return ci
}

// TestHeadOnRemoteTagTipIsSilent — the reported symptom, for both tag kinds. An
// annotated tag is the case where ls-remote's own `refs/tags/v0.6.1` line names
// the TAG OBJECT, and only the peeled `^{}` line names the commit HEAD is at.
func TestHeadOnRemoteTagTipIsSilent(t *testing.T) {
	for _, annotated := range []bool{false, true} {
		name := "lightweight"
		if annotated {
			name = "annotated"
		}
		t.Run(name, func(t *testing.T) {
			ci := remoteTagCheckout(t, annotated)
			warn, err := guardOn(t, ci.root, false)
			if err != nil {
				t.Fatalf("a clean tag checkout must proceed, got: %v", err)
			}
			if strings.Contains(warn, noRemoteWarning) {
				t.Errorf("HEAD is the tip of a tag that exists on origin; the warning is false. got:\n%s", warn)
			}
		})
	}
}

// TestHeadOnRemoteLocalOnlyCommitStillWarns — the negative control on the
// above: the SAME checkout, one local commit later, with origin still reachable.
// Without it, "silent at a tag tip" is satisfied by a check that answers true
// whenever a remote is configured.
func TestHeadOnRemoteLocalOnlyCommitStillWarns(t *testing.T) {
	ci := remoteTagCheckout(t, false)
	ci.write("notes.txt", "local only")
	ci.commit("local", "notes.txt")

	warn, err := guardOn(t, ci.root, false)
	if err != nil {
		t.Fatalf("an unpushed commit must warn, not block, got: %v", err)
	}
	if !strings.Contains(warn, noRemoteWarning) {
		t.Errorf("a commit made after the checkout exists only on this machine and must be announced, got:\n%s", warn)
	}
}

// TestHeadOnRemoteLsRemoteFailureKeepsTheWarning — a remote that cannot be
// reached is the pre-existing behaviour: warn, and never fail the submit. The
// commit is the same tag tip that TestHeadOnRemoteTagTipIsSilent proves silent,
// so the ONLY variable is ls-remote failing.
func TestHeadOnRemoteLsRemoteFailureKeepsTheWarning(t *testing.T) {
	ci := remoteTagCheckout(t, false)
	if warn, _ := guardOn(t, ci.root, false); strings.Contains(warn, noRemoteWarning) {
		t.Fatalf("control: with origin reachable this checkout must be silent, got:\n%s", warn)
	}
	missing := "file://" + filepath.ToSlash(filepath.Join(t.TempDir(), "gone.git"))
	ci.git("remote", "set-url", "origin", missing)

	warn, err := guardOn(t, ci.root, false)
	if err != nil {
		t.Fatalf("an unreachable remote must never fail the submit, got: %v", err)
	}
	if !strings.Contains(warn, noRemoteWarning) {
		t.Errorf("ls-remote failed, so nothing proved HEAD is on a remote; the existing warning must stay. got:\n%s", warn)
	}
}

// TestHeadOnRemoteGitHubSHA — in Actions, GITHUB_SHA naming HEAD is enough on
// its own, with no remote configured at all (so ls-remote cannot be what
// silences it). Both halves of the condition are load-bearing.
func TestHeadOnRemoteGitHubSHA(t *testing.T) {
	f := cleanAppFixture(t, "") // no remote, no remote-tracking ref
	head := f.gitIn(f.root, "rev-parse", "HEAD")
	other := strings.Repeat("0", len(head))

	rows := []struct {
		name, actions, sha string
		wantWarn           bool
	}{
		{"actions and GITHUB_SHA is HEAD", "true", head, false},
		{"actions but GITHUB_SHA is another commit", "true", other, true},
		{"GITHUB_SHA is HEAD outside actions", "", head, true},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Setenv("GITHUB_ACTIONS", row.actions)
			t.Setenv("GITHUB_SHA", row.sha)
			warn, err := guardOn(t, f.root, false)
			if err != nil {
				t.Fatalf("must never block, got: %v", err)
			}
			if got := strings.Contains(warn, noRemoteWarning); got != row.wantWarn {
				t.Errorf("warned=%v, want %v; warnings:\n%s", got, row.wantWarn, warn)
			}
		})
	}
}

// TestAppSubmitFromATagCheckoutIsSilentAndUploads — the symptom end to end,
// through the real command and the real git runner.
func TestAppSubmitFromATagCheckoutIsSilentAndUploads(t *testing.T) {
	ci := remoteTagCheckout(t, false)
	withStdinTTY(t, false)
	srv, submitted := dirtySubmitServer(t)
	withGuardEnv(t, srv)

	stdout, stderr, err := run(t, "app", "submit", ci.root, "--yes")
	if err != nil {
		t.Fatalf("a clean tag checkout must submit; got %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !*submitted {
		t.Error("the submit must reach the route")
	}
	if strings.Contains(stderr, noRemoteWarning) {
		t.Errorf("HEAD is a tag tip on origin; the warning is false. stderr:\n%s", stderr)
	}
}
