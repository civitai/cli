package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/civitai/cli/pkg/civitai"
)

// THE REMOTE-COMMIT SYNC.
//
// `civitai app submit` packages whatever is on disk and uploads it as a BUNDLE.
// It does not push git, and it never read the canonical repo — so it had no way
// to notice that the repo had moved underneath it.
//
// 🔴 TWO WRITERS, ONE MANIFEST. The platform writes the app's manifest too:
// the web form (`ManifestEditForm`) calls `blocks.updateManifest`, which makes a
// BACKGROUND COMMIT to the app's Forgejo repo and re-enters moderator review.
// So an author who edits anything on the website and then submits from a clone
// taken before that edit ships a bundle built from the OLDER manifest, and the
// platform-side change is silently gone. It does not conflict, it does not warn:
// the bundle simply wins, because the bundle is the whole submission.
//
// That is not hypothetical — it is the mechanism behind the already-recorded
// note that a manifest value patched platform-side is "NOT durable: the author's
// next version submit overwrites it".
//
// This is the preventer: before a submit that can reach the server, fast-forward
// the packaged directory onto the canonical repo, so the bundle is built from a
// tree that CONTAINS the platform's commits rather than one that predates them.
//
// 🔴 IT MUST DEGRADE, NOT ENFORCE — the same rule the dirty-work-tree guard
// lives by, and for the same reason: `civitai app scaffold` produces a directory
// with no repo at all, and submitting one is completely ordinary. Every state
// this cannot establish an answer for PROCEEDS. The full matrix:
//
//	--no-pull                        → skip, silently (the escape hatch)
//	no repo, or no `git` on PATH     → skip, silently (the scaffold path)
//	clone info unavailable / errored → WARN, proceed (see the note on fetchErr)
//	app not yet approved             → skip, silently (there is no repo yet)
//	fetch failed (offline, auth)     → WARN, proceed
//	already up to date               → nothing, silently
//	local is AHEAD of the remote     → nothing (unpushed local work is legitimate)
//	local is BEHIND                  → FAST-FORWARD, and say what moved
//	histories have DIVERGED          → REFUSE, naming --no-pull
//
// The ONLY refusal is divergence, and it is a refusal rather than an automatic
// rebase on purpose: this command's job is to submit, not to rewrite an author's
// history. A refusal with an exact next step is recoverable; an unattended
// rebase of someone's repo, triggered by a command they ran to publish, is not.
//
// 🔴 WHY NO SEPARATE DIRTY PRE-CHECK, WHICH LOOKS LIKE AN OMISSION: `git merge
// --ff-only` ALREADY refuses when the fast-forward would overwrite uncommitted
// local changes ("Your local changes to the following files would be
// overwritten by merge"). Git enforces the safety property, so asking `git
// status` here would buy nothing and would BREAK something — the dirty-tree
// guard documents that there is exactly ONE git-invocation seam asking this repo
// these questions, because a second one is how the two answers start to
// disagree. A failed merge lands in the WARN-and-proceed arm, and the dirty
// guard then refuses the submit a few lines later with its own, better message.

// ErrDivergedFromRemote classifies the divergence refusal. Like ErrDirtyWorkTree
// it carries no user-facing text of its own — it is ATTACHED to the
// message-bearing error, so errors.Is reports the KIND while the printed message
// is unchanged.
//
// 🔴 EXIT CODE 1, matching ErrDirtyWorkTree and the version-regression refusal.
// Exit 2 is documented as a mistake about the INVOCATION, and every flag and
// path is well-formed when this fires: what is wrong is the PROJECT's history
// relative to its own canonical repo, which is the validation verdict code 1
// already publishes. Leaving it untagged for the exit mapper's `default` is what
// produces 1.
var ErrDivergedFromRemote = errors.New("local history has diverged from the app's canonical repository")

// gitWriteRunner runs a git subcommand that MUTATES the repository in dir, and
// returns its combined output rather than streaming it.
//
// 🔴 IT CAPTURES RATHER THAN STREAMS BECAUSE THE URL CARRIES A CREDENTIAL. The
// clone URL embeds the caller's Forgejo token as HTTP-Basic
// (https://<user>:<token>@…), and `git fetch` echoes the remote it contacted
// ("From https://…") on its progress output. `civitai app pull` streams and
// documents that exposure as the price of visible clone progress; a submit has
// no such progress to show, so there is no reason to print a credential during
// an ordinary publish. Output is surfaced only on FAILURE, through redactSecret.
//
// 🔴 IT SCRUBS THE ENVIRONMENT, and that is load-bearing on a WRITE path. Every
// variable in gitEnvOverrides BEATS `-C`, and git EXPORTS GIT_DIR to its hooks —
// so a `civitai app submit` invoked from a `pre-push` hook or under `git rebase
// -x` inherits a GIT_DIR pointing at the repo that invoked the hook. On the
// read-only dirty guard that misdirection produces a wrong answer; here it would
// FETCH AND MERGE INTO THE WRONG REPOSITORY. gitScrubbedEnv is shared with that
// guard deliberately: one rule, one place.
var gitWriteRunner = func(dir string, args ...string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", errGitUnavailable
	}
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	c.Env = gitScrubbedEnv(os.Environ())
	var out bytes.Buffer
	c.Stdout = &out
	c.Stderr = &out
	err := c.Run()
	return out.String(), err
}

// redactSecret removes a credential from text that is about to be shown to a
// human or written to a log.
//
// 🔴 IT REDACTS THE TOKEN, NOT THE WHOLE URL, and it must be given the token
// even when the caller "knows" the message looks clean: git's error text is not
// a contract, and the failure modes that print the remote are exactly the ones
// nobody rehearses (a 403, a proxy error, a redirect). An empty token is a
// no-op rather than a match-everything, which is the bug this function would
// otherwise have: strings.ReplaceAll(s, "", "***") interleaves "***" between
// every character.
func redactSecret(s, token string) string {
	if token == "" {
		return s
	}
	return strings.ReplaceAll(s, token, "***")
}

// syncOutcome is what the sync did, for the caller's benefit. Only Advanced
// matters to control flow — it means the working tree CHANGED, so anything the
// caller already read off disk (the manifest, a validation result) is stale.
type syncOutcome struct {
	// Advanced is true iff the working tree was fast-forwarded. The caller MUST
	// re-read the manifest when this is set.
	Advanced bool
	// Behind is how many commits were pulled in. Reported, never acted on.
	Behind int
}

// syncRemoteCommits fast-forwards dir onto the app's canonical Forgejo repo.
//
// 🔴 WHICH DIRECTORY: `dir`, THE TREE BEING PACKAGED — never the process cwd,
// for the reason the dirty guard spells out at length. `app submit [dir]` takes
// an optional positional, so the tree being submitted and the cwd are routinely
// different places, and this function MUTATES what it is pointed at. Every git
// call goes through a seam whose contract is `git -C dir`.
//
// It returns an error ONLY for divergence. Everything else that goes wrong is a
// warning, because this is an accident preventer and not an authorization check:
// an author offline on a train must still be able to submit.
func syncRemoteCommits(
	ctx context.Context,
	fetchInfo cloneInfoFetcher,
	git gitOutputFunc,
	write func(dir string, args ...string) (string, error),
	warn io.Writer,
	dir, app string,
) (syncOutcome, error) {
	var zero syncOutcome
	if git == nil || write == nil || fetchInfo == nil {
		return zero, nil
	}

	// Is this a work tree at all? A non-zero exit is the no-repo case and the
	// no-git-binary case, and both mean the same thing operationally: there is no
	// history here to sync. Proceed silently — this is the scaffold path.
	out, err := git(dir, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(out) != "true" {
		return zero, nil
	}

	// A repo with no commits has nothing to fast-forward and no HEAD to compare.
	local := headCommit(git, dir)
	if local == "" {
		return zero, nil
	}

	info, err := fetchInfo(ctx, app)
	if err != nil {
		// 🔴 WARN, NEVER REFUSE. This call is owner-only and App-Blocks-flag-gated,
		// and it 404s for BOTH "no such app" and "your app exists but nothing is
		// approved yet" (the ambiguity `app pull` documents). None of those is a
		// reason to block a submit that is otherwise fine — and a collaborator
		// submitting someone else's app legitimately cannot read this endpoint.
		warnf(warn, "could not reach the canonical repository for %s (%v) — submitting without the remote-commit sync, "+
			"so anything changed on the website since your last pull will be overwritten by this bundle.", app, err)
		return zero, nil
	}
	if info == nil || info.NotYetAvailable || strings.TrimSpace(info.CloneURL) == "" {
		// The repo is created when the FIRST version is approved. Before then
		// there is genuinely nothing to sync, and saying so on every first submit
		// would be noise on the happy path.
		return zero, nil
	}

	// Fetch into FETCH_HEAD without touching .git/config. The URL is passed
	// EXPLICITLY rather than configured as a remote, which is what keeps the
	// embedded token off disk — the same choice `app pull` makes on its sync
	// path, and the reason that path is safer than a fresh clone.
	//
	// `HEAD` rather than a named branch: the server's default branch is whatever
	// the repo has checked out, and naming one here would be a guess that breaks
	// the day it changes.
	if fo, ferr := write(dir, "fetch", "--quiet", "--", info.CloneURL, "HEAD"); ferr != nil {
		warnf(warn, "could not fetch the canonical repository for %s (%s) — submitting without the remote-commit sync, "+
			"so anything changed on the website since your last pull will be overwritten by this bundle.",
			app, strings.TrimSpace(redactSecret(fo, info.Token)))
		return zero, nil
	}

	// 🔴 CLASSIFY WITH rev-list, NOT `merge-base --is-ancestor`. That command's
	// ANSWER IS ITS EXIT CODE, so "not an ancestor" and "the ref did not resolve"
	// are the same observation to a caller that only sees err != nil — and an
	// unresolvable ref returns a bare false, which reads as a confident "no".
	// `rev-list --left-right --count A...B` prints two numbers on stdout and
	// fails loudly if either ref is bad, so the answer and the error are
	// different channels.
	counts, err := git(dir, "rev-list", "--left-right", "--count", local+"...FETCH_HEAD")
	if err != nil {
		warnf(warn, "could not compare %s against the canonical repository (%v) — submitting without the remote-commit sync.", dir, err)
		return zero, nil
	}
	ahead, behind, ok := parseAheadBehind(counts)
	if !ok {
		warnf(warn, "could not read the commit comparison for %s (%q) — submitting without the remote-commit sync.", dir, strings.TrimSpace(counts))
		return zero, nil
	}

	switch {
	case behind == 0:
		// Up to date, or ahead with unpushed local work. Both are fine and
		// neither is this function's business — the dirty guard already warns
		// about a HEAD that exists on no remote.
		return zero, nil
	case ahead > 0:
		// Diverged: the author committed locally AND the repo moved. A
		// fast-forward is impossible and a merge or rebase would rewrite history
		// nobody asked this command to touch.
		return zero, divergedError(app, ahead, behind)
	}

	// Behind only: fast-forward. This is the case the whole guard exists for —
	// the website changed the manifest and the author has not pulled since.
	if mo, merr := write(dir, "merge", "--ff-only", "FETCH_HEAD"); merr != nil {
		// Almost always "your local changes would be overwritten": the tree is
		// dirty, git refused, and the dirty-tree guard is about to refuse the
		// submit anyway with a message that names the paths. Warn and let it.
		warnf(warn, "could not fast-forward %s onto the canonical repository (%s) — submitting without the remote-commit sync.",
			dir, strings.TrimSpace(redactSecret(mo, info.Token)))
		return zero, nil
	}

	warnf(warn, "pulled %s from the canonical repository before packaging — the website's changes are now in this bundle. "+
		"Re-read %s if you keep it open in an editor.", pluralCommits(behind), dir)
	return syncOutcome{Advanced: true, Behind: behind}, nil
}

// parseAheadBehind reads `rev-list --left-right --count` output: two
// whitespace-separated integers, LEFT (ahead) then RIGHT (behind).
//
// It returns ok=false rather than guessing on anything it does not recognise.
// A silent 0/0 here would be indistinguishable from "up to date", which is the
// reassuring answer — exactly the direction a parse failure must never fall.
func parseAheadBehind(s string) (ahead, behind int, ok bool) {
	f := strings.Fields(s)
	if len(f) != 2 {
		return 0, 0, false
	}
	a, err := strconv.Atoi(f[0])
	if err != nil {
		return 0, 0, false
	}
	b, err := strconv.Atoi(f[1])
	if err != nil {
		return 0, 0, false
	}
	return a, b, true
}

// pluralCommits renders a commit count for a human.
func pluralCommits(n int) string {
	if n == 1 {
		return "1 commit"
	}
	return fmt.Sprintf("%d commits", n)
}

// divergedError builds the one refusal this guard issues.
//
// It states the CONSEQUENCE rather than the rule, and names both ways out. The
// reader is someone who has local commits and a repo that moved, so "reconcile
// them" is the actual next step and `--no-pull` is the override for the author
// who knows their bundle is the one that should win.
func divergedError(app string, ahead, behind int) error {
	var b strings.Builder
	fmt.Fprintf(&b, "refusing to submit %s — your local history and the app's canonical repository have diverged: "+
		"%s here that the repository does not have, and %s there that you do not.\n",
		app, pluralCommits(ahead), pluralCommits(behind))
	b.WriteString("The website can commit to that repository too (editing the manifest in the web form does exactly that), " +
		"so submitting now would package a tree that is missing those changes and silently overwrite them.\n")
	b.WriteString("Reconcile them first — `civitai app pull . --app " + app + "` after committing or stashing, or rebase onto the fetched head — " +
		"or pass --no-pull to submit this tree exactly as it is")
	return civitai.Tag(ErrDivergedFromRemote, errors.New(b.String()))
}
