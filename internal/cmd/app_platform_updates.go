package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/civitai/cli/internal/appapi"
)

// STAFF PLATFORM UPDATES — the client flow of civitai/civitai#10594's
// "Owner-facing API". Civitai staff can push an update to someone else's app:
// one commit on its own branch (`platform-update/<id>`) of the app's SOURCE
// repository (civitai-apps-src/<slug>). `civitai app pull` and `civitai app
// submit` list the app's OPEN updates (pending, and conflicted ones nobody has
// merged yet) and merge each one, oldest first, into the checkout, then report
// the outcome to the server.
//
// 🔴 A CONFLICTED UPDATE IS RETRIED, NOT FORGOTTEN. The server keeps listing an
// update reported `conflicted` until it is merged or withdrawn, so every pull
// tries it again: it now merges cleanly → `resolve merged`; the owner merged it
// by hand (the sha is already in HEAD's history) → `resolve merged` with HEAD,
// merging nothing; it still conflicts → abort and re-print the hand-merge
// instructions, WITHOUT reporting `conflicted` again. And a submit still
// refuses over it.
//
// 🔴 WHAT IS MERGED IS THE RECORDED commitSha, NEVER THE BRANCH. The branch is
// ordinary git and can move after the push; only the sha is the change the
// server's record attests. So the flow is: fetch the branch BY URL, check the
// recorded commit arrived (fetch it by sha if not), and `git merge --no-ff
// <commitSha>`. A branch tip that is not commitSha has commits that are not part
// of the update: they are left out, and the owner is told.
//
// 🔴 WHETHER AN UPDATE IS SHOWN AS VERIFIED COMES ONLY FROM verifyCommit. The
// listing's own `by.verifiedStaff` is not read for it: verifyCommit is the one
// call the contract names as the source of that claim, and it also answers
// false for an update a moderator has since withdrawn. An unconfirmed update
// prints a could-not-verify note under the sender line.
//
// The source repository is fetched BY URL (`git fetch -- <url> <ref>`), never
// through a remote this code adds: `git remote add` writes the checkout's
// config, and a build-repo checkout (which `app pull` keeps on the build
// repository) has no remote for the source repository. The credential helper
// `app pull` installs is passed for that one command with `-c`, exactly as the
// clone passes it, so a checkout that never had it still authenticates and
// nothing is written to .git/config.

// platformUpdateAPI is the server half this flow talks to. *appapi.Client
// satisfies it; it is an interface so the flow does not care which client.
type platformUpdateAPI interface {
	ListPendingPlatformUpdates(ctx context.Context, slug string) ([]appapi.PlatformUpdate, error)
	VerifyPlatformUpdateCommit(ctx context.Context, slug, commitSha string) (bool, error)
	ResolvePlatformUpdate(ctx context.Context, id, outcome, mergeCommitSha string) error
}

// ErrUnmergedPlatformUpdate classifies every way the flow stops with an update
// left unmerged: a conflict, a dirty tree, a commit the branch no longer holds,
// a directory that is not a checkout. `app submit` wraps it into its refusal.
var ErrUnmergedPlatformUpdate = errors.New("unmerged platform update")

// allowUnmergedPlatformUpdateFlag is `app submit`'s override, named once so the
// refusal and the flag cannot spell it differently.
const allowUnmergedPlatformUpdateFlag = "--allow-unmerged-platform-update"

var (
	platformUpdateShaRe    = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	platformUpdateIDRe     = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)
	platformUpdateBranchRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`)
)

// platformUpdatePathsCap bounds how many changed paths one notice lists.
const platformUpdatePathsCap = 20

// platformGit runs `git -C dir <args>` and returns its combined output. It is
// the flow's one git seam: fetch, merge and abort WRITE the checkout, so it is
// neither gitOutput (documented read-only) nor gitRunner (which streams to the
// terminal; this flow prints its own report instead of git's progress).
//
// It shares gitScrubbedEnv with both, for the reason gitRunner's comment gives:
// a relocating variable would point a merge at some other repository.
// GIT_TERMINAL_PROMPT=0 because an automatic merge must not stop to ask for a
// password — an unattended pull or submit gets a failed fetch instead.
//
// 🔴 A FETCH IS BOUNDED BY platformFetchTimeout. A source repository that accepts
// the connection and then never answers would otherwise hold `app pull` and
// `app submit` forever (git sets no read timeout of its own on smart HTTP). A
// fetch runs in its own session, as gitOutput's ls-remote does, so the timeout
// kills git's remote helper too rather than leaving it holding the pipe; a
// fetch that times out is an ordinary failed fetch (nothing merged, nothing
// reported).
// platformFetchTimeout bounds one platform-update fetch. It is not
// gitNetworkTimeout (5s, sized for one ls-remote round trip): a fetch moves
// objects, and the first one from a source repo forked from the build repo
// carries the app's whole history, so 5s would fail every pull and submit
// over a slow link.
var platformFetchTimeout = 60 * time.Second

var platformGit = func(dir string, args ...string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", errGitUnavailable
	}
	ctx := context.Background()
	network := platformGitSubcommand(args) == "fetch"
	if network {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, platformFetchTimeout)
		defer cancel()
	}
	c := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	c.Env = append(gitScrubbedEnv(os.Environ()), "GIT_TERMINAL_PROMPT=0")
	if network {
		c.Env = append(c.Env, "GIT_ASKPASS=", "SSH_ASKPASS_REQUIRE=never")
		c.WaitDelay = time.Second
		detachFromTerminal(c)
	}
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	err := c.Run()
	if network && ctx.Err() == context.DeadlineExceeded {
		return buf.String() + fmt.Sprintf("\nfatal: git fetch timed out after %s", platformFetchTimeout), err
	}
	return buf.String(), err
}

// platformGitSubcommand is args' git subcommand: the first argument after any
// leading `-c key=value` pairs (gitArgs puts the credential helper there).
func platformGitSubcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "-c" {
			i++
			continue
		}
		return args[i]
	}
	return ""
}

// platformGitExitCode is the exit status of a git run that failed, or -1 when
// git did not run to an exit (not found, killed).
func platformGitExitCode(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// platformUpdateRun is one pass of the flow over one checkout.
type platformUpdateRun struct {
	api       platformUpdateAPI
	out       io.Writer
	dir       string // the checkout (any directory inside it)
	slug      string
	sourceURL string // civitai-apps-src/<slug>, as the server names it
	helperCfg [][2]string
}

// listPendingPlatformUpdates is the first step on its own, so `app submit` can
// ask before it knows the source URL (it needs a second call for that).
func listPendingPlatformUpdates(ctx context.Context, api platformUpdateAPI, slug string) ([]appapi.PlatformUpdate, error) {
	if api == nil {
		return nil, nil
	}
	return api.ListPendingPlatformUpdates(ctx, slug)
}

// sourceURLFunc names the app's source repository. It is a func so `app
// submit`, which does not otherwise need the clone info, asks for it only once a
// merge is really going to be attempted.
type sourceURLFunc func(ctx context.Context) (string, error)

// mergePlatformUpdates merges `updates` (oldest first, as listed) into the
// checkout at dir and reports each outcome. It returns how many it merged — a
// new merge commit each; one already in the history does not count. It
// stops at the first update it cannot merge, with an error wrapping
// ErrUnmergedPlatformUpdate whose text says what to do; an update after that
// one is not attempted, because the list is ordered and a later update may build
// on the earlier one.
func mergePlatformUpdates(ctx context.Context, api platformUpdateAPI, out io.Writer, dir, slug string, source sourceURLFunc, updates []appapi.PlatformUpdate) (int, error) {
	if len(updates) == 0 {
		return 0, nil
	}
	n := len(updates)
	// The slug reaches git's config (the credential helper) and every line
	// below unsanitised, so it must have the shape an app slug has.
	if !helperAppRe.MatchString(slug) {
		return 0, fmt.Errorf("%w: %q is not an app slug platform updates can be merged for", ErrUnmergedPlatformUpdate, slug)
	}
	fmt.Fprintf(out, "Civitai staff pushed %d platform update(s) to %s.\n", n, slug)

	// Every update is checked for shape before anything is fetched: the sha,
	// branch and id reach git's argv and a suggested command line.
	for _, u := range updates {
		if !platformUpdateShaRe.MatchString(u.CommitSha) || !platformUpdateIDRe.MatchString(u.ID) || !platformUpdateBranchRe.MatchString(u.Branch) || strings.Contains(u.Branch, "..") {
			return 0, fmt.Errorf("the server returned a platform update for %s that cannot be merged safely (id %q, branch %q, commit %q); nothing was merged",
				slug, puText(u.ID), puText(u.Branch), puText(u.CommitSha))
		}
	}

	if _, err := platformGit(dir, "rev-parse", "--show-toplevel"); err != nil {
		return 0, fmt.Errorf("%w: %s is not a git checkout, so %d platform update(s) from Civitai staff cannot be merged into it — "+
			"get a checkout with `civitai app pull --app %s` and work from there", ErrUnmergedPlatformUpdate, dir, n, slug)
	}
	// 🔴 NEVER MERGE INTO UNCOMMITTED CHANGES TO THE APP. A merge that conflicts
	// with them is refused by git, but one that does not would leave the owner's
	// own unfinished work and the staff change tangled in one working tree, and
	// the abort path below could not separate them. Untracked files are not
	// counted: git refuses a merge that would overwrite one, and that refusal is
	// reported.
	//
	// SCOPED TO THE APP DIRECTORY, exactly as `app submit`'s own dirty guard
	// scopes (uncommittedAppPaths reuses its pathspec, prefix and packager
	// rules): an app inside a monorepo is not blocked by a sibling package's
	// unfinished work. A dirty file OUTSIDE the app that the merge would touch —
	// or a staged change anywhere, since git wants the index to match HEAD — is
	// still refused, by git itself, before it starts; mergeOne reports that
	// refusal as uncommitted changes rather than as git's own text.
	dirty, err := uncommittedAppPaths(dir)
	if err != nil {
		return 0, fmt.Errorf("%w: could not read the state of %s (%s); nothing was merged", ErrUnmergedPlatformUpdate, dir, puText(err.Error()))
	}
	if len(dirty) > 0 {
		for _, u := range updates {
			printPlatformUpdateSummary(out, u, false, false)
		}
		return 0, fmt.Errorf("%w: %s has uncommitted changes, so %d platform update(s) from Civitai staff were not merged into it — "+
			"commit or discard your changes, then run the command again", ErrUnmergedPlatformUpdate, dir, n)
	}
	sourceURL, err := source(ctx)
	if err != nil {
		return 0, fmt.Errorf("%w: could not look up the source repository of %s, where its platform updates live (%s); nothing was merged",
			ErrUnmergedPlatformUpdate, slug, puText(err.Error()))
	}
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		return 0, fmt.Errorf("%w: the server named no source repository for %s, where its platform updates live; nothing was merged",
			ErrUnmergedPlatformUpdate, slug)
	}

	r := &platformUpdateRun{api: api, out: out, dir: dir, slug: slug, sourceURL: sourceURL}
	if r.helperCfg, err = credentialHelperConfig(sourceURL, slug); err != nil {
		return 0, err
	}
	merged := 0
	for _, u := range updates {
		did, err := r.mergeOne(ctx, u)
		if did {
			merged++
		}
		if err != nil {
			return merged, err
		}
	}
	return merged, nil
}

// uncommittedAppPaths is the TRACKED half of `app submit`'s dirty-tree verdict
// for dir: the same `status -- .` pathspec, the same repo-root prefix and the
// same bundleDirtyPaths filter, with untracked files left out (see the caller).
// It reads through gitOutput, the read-only seam, because -z output must not
// have git's stderr mixed into it.
func uncommittedAppPaths(dir string) ([]gitStatusEntry, error) {
	pfx, err := gitOutput(dir, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, err
	}
	// Trim the line ending only: a prefix may begin with a space (see
	// checkWorkTreeClean).
	prefix := strings.TrimSuffix(strings.TrimSuffix(pfx, "\n"), "\r")
	raw, err := gitOutput(dir, "-c", "status.relativePaths=false", "status", "--porcelain", "-z", "--untracked-files=no", "--", ".")
	if err != nil {
		return nil, err
	}
	return bundleDirtyPaths(raw, prefix, dir), nil
}

// gitArgs prefixes args with the credential helper, as `-c` options.
func (r *platformUpdateRun) gitArgs(args ...string) []string {
	full := make([]string, 0, 2*len(r.helperCfg)+len(args))
	for _, kv := range r.helperCfg {
		full = append(full, "-c", kv[0]+"="+kv[1])
	}
	return append(full, args...)
}

func (r *platformUpdateRun) hasCommit(sha string) bool {
	_, err := platformGit(r.dir, "cat-file", "-e", sha+"^{commit}")
	return err == nil
}

func (r *platformUpdateRun) mergeOne(ctx context.Context, u appapi.PlatformUpdate) (bool, error) {
	sha := u.CommitSha
	// 1. Fetch the branch, by URL. A failure is not final: the recorded commit
	//    may still be reachable by sha (step 2).
	fetchHead := ""
	fetchOut, fetchErr := platformGit(r.dir, r.gitArgs("fetch", "--no-tags", "--", r.sourceURL, u.Branch)...)
	if fetchErr == nil {
		if h, err := platformGit(r.dir, "rev-parse", "--verify", "-q", "FETCH_HEAD"); err == nil {
			fetchHead = strings.TrimSpace(h)
		}
	}
	// 2. The recorded commit must be here; fetch it by sha if it is not.
	if !r.hasCommit(sha) {
		_, _ = platformGit(r.dir, r.gitArgs("fetch", "--no-tags", "--", r.sourceURL, sha)...)
	}
	if !r.hasCommit(sha) && fetchErr != nil {
		// 🔴 A FETCH THAT FAILED PROVES NOTHING ABOUT THE COMMIT. Network, auth
		// and a deleted branch all land here, and none of them is a conflict. So:
		// no report, and the next run tries again.
		return false, fmt.Errorf("%w: could not fetch platform update %s from %s (%s); nothing was merged or reported — run the command again",
			ErrUnmergedPlatformUpdate, u.ID, puText(r.sourceURL), gitErrLine(fetchOut))
	}
	verified := r.verify(ctx, sha)
	if !r.hasCommit(sha) {
		// The branch WAS fetched and the recorded commit is not on it, nor
		// reachable by sha: the update as recorded no longer exists there.
		printPlatformUpdateSummary(r.out, u, verified, true)
		told := "Civitai was told it did not merge"
		if !r.resolve(ctx, u, appapi.PlatformUpdateConflicted, "") {
			told = "Civitai already knows it did not merge"
		}
		fmt.Fprintf(r.out, "Its branch %s no longer holds the recorded commit %s, so nothing was merged; %s.\n",
			puText(u.Branch), sha, told)
		return false, fmt.Errorf("%w: platform update %s was not merged (its recorded commit %s is missing from the source repository)",
			ErrUnmergedPlatformUpdate, u.ID, sha)
	}

	// 🔴 A DETACHED HEAD IS NEVER REPORTED `merged`. The merge commit would be on
	// no branch: the next checkout leaves it behind, and the server would have
	// been told an update is in the app that is in no branch of it. So a
	// detached checkout still merges (a submit's bundle then carries the
	// update), says where the commit lives, and reports nothing; the next pull
	// or submit from a branch that contains it reports it.
	_, symErr := platformGit(r.dir, "symbolic-ref", "-q", "HEAD")
	detached := symErr != nil

	// Already in this history (merged by hand — the way out of a conflict — or
	// merged by an earlier run whose report did not reach the server): report
	// it merged with HEAD, merge nothing. HEAD is a commit that contains the
	// update, which is all mergeCommitSha has to be.
	if _, err := platformGit(r.dir, "merge-base", "--is-ancestor", sha, "HEAD"); err == nil {
		fmt.Fprintf(r.out, "Platform update %s (commit %s) is already in %s.\n", u.ID, sha, r.dir)
		printPlatformUpdateSummary(r.out, u, verified, true)
		if detached {
			fmt.Fprintf(r.out, "  HEAD is detached, so Civitai was not told it merged; run the command again from a branch that contains it.\n")
			return false, nil
		}
		head, herr := platformGit(r.dir, "rev-parse", "HEAD")
		if herr == nil {
			r.resolve(ctx, u, appapi.PlatformUpdateMerged, strings.TrimSpace(head))
		}
		return false, nil
	}

	// 🔴 NO COMMON ANCESTOR, NO MERGE. The update is a commit in the SOURCE
	// repository's history; a checkout of the build repository (which `app
	// pull` keeps syncing from it) or of any other unrelated history cannot
	// merge it — git refuses unrelated histories, and forcing it would graft the
	// whole source history in. That is not a conflict either, so nothing is
	// reported: the update stays pending, and the owner is told how to get it.
	// Exit 1 with no output is git's "none found"; any other failure (an unborn
	// HEAD, say) falls through to the merge, which reports for itself.
	if mbOut, err := platformGit(r.dir, "merge-base", "HEAD", sha); err != nil && platformGitExitCode(err) == 1 && strings.TrimSpace(mbOut) == "" {
		fmt.Fprintf(r.out, "Platform update %s (commit %s) is in the history of %s's source repository, which this checkout of %s does not share (git finds no common ancestor), so nothing was merged.\n",
			u.ID, sha, r.slug, r.dir)
		printPlatformUpdateSummary(r.out, u, verified, true)
		fmt.Fprintf(r.out, "  This is a checkout of the build repository, or of another history. Merge the update in a clone of the source repository (`civitai app pull <new dir> --app %s`), or apply it here by its commit:\n", r.slug)
		fmt.Fprintf(r.out, "  %s\n", r.cherryPickCommand(sha))
		return false, fmt.Errorf("%w: platform update %s was not merged: this checkout shares no history with %s's source repository, where the update lives (instructions above)",
			ErrUnmergedPlatformUpdate, u.ID, r.slug)
	}

	if u.Status == appapi.PlatformUpdateStatusConflicted {
		fmt.Fprintf(r.out, "Retrying platform update %s (commit %s), which did not merge cleanly last time.\n", u.ID, sha)
	}
	// 3+4. Merge the recorded sha and nothing else.
	mergeOut, mergeErr := platformGit(r.dir, "merge", "--no-ff", "--no-edit",
		"-m", "Merge Civitai platform update "+u.ID, sha)
	if mergeErr != nil {
		if _, err := platformGit(r.dir, "rev-parse", "-q", "--verify", "MERGE_HEAD"); err != nil {
			// git refused before it started (local changes or an untracked file
			// in the way, no committer identity, …): nothing to abort and nothing
			// to report as a conflict, because the update was never tried against
			// the tree.
			if files := overwrittenByMerge(mergeOut); len(files) > 0 {
				return false, fmt.Errorf("%w: platform update %s (commit %s) was not merged into %s: it would overwrite uncommitted changes in %s — commit or discard them, then run the command again",
					ErrUnmergedPlatformUpdate, u.ID, sha, r.dir, strings.Join(files, ", "))
			}
			return false, fmt.Errorf("%w: git could not merge platform update %s (commit %s) into %s: %s",
				ErrUnmergedPlatformUpdate, u.ID, sha, r.dir, puText(strings.TrimSpace(mergeOut)))
		}
		conflicted, _ := platformGit(r.dir, "diff", "--name-only", "--diff-filter=U")
		_, _ = platformGit(r.dir, "merge", "--abort")
		files := strings.Fields(conflicted)
		if len(files) == 0 {
			// 🔴 STOPPED WITH NO CONFLICTED FILE IS NOT A CONFLICT. The tree merged;
			// git then failed to write the commit (commit.gpgsign with a broken
			// gpg, a failing pre-merge-commit or commit-msg hook). Reporting it
			// `conflicted` would tell the owner — and the server — that the
			// update clashes with their work when it does not. Aborted above, and
			// nothing is reported: the next run tries again.
			return false, fmt.Errorf("%w: git could not complete the merge of platform update %s (commit %s) into %s (the merge was aborted): %s",
				ErrUnmergedPlatformUpdate, u.ID, sha, r.dir, gitErrLine(mergeOut))
		}
		r.resolve(ctx, u, appapi.PlatformUpdateConflicted, "") // not again for one already conflicted
		fmt.Fprintf(r.out, "Platform update %s (commit %s) conflicts with your changes in %s, so nothing was merged (the merge was aborted).\n", u.ID, sha, r.dir)
		printPlatformUpdateSummary(r.out, u, verified, true)
		for i := range files {
			files[i] = puText(files[i])
		}
		fmt.Fprintf(r.out, "  Conflicts in: %s\n", strings.Join(files, ", "))
		fmt.Fprintf(r.out, "To merge it by hand, merge the recorded commit %s (not the branch):\n", sha)
		fmt.Fprintf(r.out, "  %s\n", r.handMergeCommand(u.Branch, sha))
		fmt.Fprintln(r.out, "then resolve the conflicts and commit. The next `civitai app pull` reports it merged.")
		return false, fmt.Errorf("%w: platform update %s conflicts with your changes — merge %s by hand (instructions above)",
			ErrUnmergedPlatformUpdate, u.ID, sha)
	}

	head, err := platformGit(r.dir, "rev-parse", "HEAD")
	if err != nil {
		return true, fmt.Errorf("merged platform update %s but could not read the merge commit: %s", u.ID, gitErrLine(head))
	}
	mergeSha := strings.TrimSpace(head)
	if !detached {
		r.resolve(ctx, u, appapi.PlatformUpdateMerged, mergeSha)
	}
	fmt.Fprintf(r.out, "Merged platform update %s (commit %s) into %s.\n", u.ID, sha, r.dir)
	printPlatformUpdateSummary(r.out, u, verified, true)
	if fetchHead != "" && fetchHead != sha {
		fmt.Fprintf(r.out, "  Its branch %s has commits that are not part of the update (tip %s); they were left out. Only %s was merged.\n",
			puText(u.Branch), fetchHead, sha)
	}
	if detached {
		fmt.Fprintf(r.out, "  HEAD is detached, so the merge commit %s exists only in this checkout, on no branch: push it or merge it into your branch (e.g. `git -C %s branch <name> %s`). Civitai was not told it merged; the next pull or submit from a branch that contains it reports it.\n",
			mergeSha, shellQuote(r.dir), mergeSha)
	}
	return true, nil
}

// overwrittenByMerge returns the paths git lists under "Your local changes to
// the following files would be overwritten by merge:" (sanitised), or nil when
// git said something else.
func overwrittenByMerge(out string) []string {
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	var files []string
	in := false
	for _, l := range lines {
		if strings.Contains(l, "would be overwritten by merge:") {
			in = true
			continue
		}
		if !in {
			continue
		}
		if l == "" || (l[0] != ' ' && l[0] != '\t') {
			in = false
			continue
		}
		if f := strings.TrimSpace(l); f != "" {
			files = append(files, puText(f))
		}
	}
	return files
}

// gitCommandPrefix is `git -C <dir> -c <helper>…`, shell-quoted, for a command
// line printed for the owner to paste: the credential helper rides along with
// -c, so it authenticates in a checkout `civitai app pull` never configured.
func (r *platformUpdateRun) gitCommandPrefix() string {
	var b strings.Builder
	b.WriteString("git -C ")
	b.WriteString(shellQuote(r.dir))
	for _, kv := range r.helperCfg {
		b.WriteString(" -c ")
		b.WriteString(shellQuote(kv[0] + "=" + kv[1]))
	}
	return b.String()
}

// handMergeCommand is the printed hand-merge for a conflicted update: fetch the
// branch, then merge the recorded sha (never the branch). branch and sha passed
// the shape regexes in mergePlatformUpdates, so they need no quoting; the
// directory and the URL do.
func (r *platformUpdateRun) handMergeCommand(branch, sha string) string {
	return r.gitCommandPrefix() + " fetch " + shellQuote(puText(r.sourceURL)) + " " + branch +
		" && git -C " + shellQuote(r.dir) + " merge " + sha
}

// cherryPickCommand is the printed way to apply an update to a checkout that
// shares no history with the source repository: fetch the recorded commit by
// sha and cherry-pick it (one commit, so a cherry-pick is the whole change).
func (r *platformUpdateRun) cherryPickCommand(sha string) string {
	return r.gitCommandPrefix() + " fetch " + shellQuote(puText(r.sourceURL)) + " " + sha +
		" && git -C " + shellQuote(r.dir) + " cherry-pick " + sha
}

// verify asks the server whether sha is a recorded update. Any failure is
// "not verified": an update is only ever shown as verified on a positive answer.
func (r *platformUpdateRun) verify(ctx context.Context, sha string) bool {
	ok, err := r.api.VerifyPlatformUpdateCommit(ctx, r.slug, sha)
	return err == nil && ok
}

// resolve reports an outcome, and says whether it sent one. A failure is printed
// and does not undo the merge: the merge is local and done, and the next run
// finds the commit already in the history and reports it again.
//
// `conflicted` is never sent for an update the server already lists as
// conflicted: the server would no-op it, and the one place that decides this is
// here, so no call site can re-report it.
func (r *platformUpdateRun) resolve(ctx context.Context, u appapi.PlatformUpdate, outcome, mergeSha string) bool {
	if outcome == appapi.PlatformUpdateConflicted && u.Status == appapi.PlatformUpdateStatusConflicted {
		return false
	}
	if err := r.api.ResolvePlatformUpdate(ctx, u.ID, outcome, mergeSha); err != nil {
		fmt.Fprintf(r.out, "Could not report platform update %s as %s to Civitai: %s\n", u.ID, outcome, puText(err.Error()))
	}
	return true
}

// printPlatformUpdateSummary prints who made the update and why, what it
// changes, and — when it does — that it changes what the app may do.
//
// verifiedKnown false prints the attribution without any claim about staff
// status — used where verifyCommit has not been asked (the dirty-tree refusal,
// which stops before fetching anything).
//
// The sender is the server's `by.username`, printed as sent and never
// suffixed: since civitai/civitai#10717 the owner's record names a verified
// staff update "CivitaiOfficial (App Assistant)" — the official account itself
// — and names nobody (null) otherwise, so a " (verified Civitai staff)" suffix
// would only double the claim. Whether the line may say "verified" at all is
// still decided by verifyCommit alone: a null sender is "Verified Civitai
// staff" only when verifyCommit confirmed the commit, else "An unnamed sender".
func printPlatformUpdateSummary(out io.Writer, u appapi.PlatformUpdate, verified, verifiedKnown bool) {
	name := ""
	if u.By.Username != nil {
		name = puText(strings.TrimSpace(*u.By.Username))
	}
	who := name
	if who == "" {
		who = "An unnamed sender"
		if verifiedKnown && verified {
			who = "Verified Civitai staff"
		}
	}
	reason := puText(strings.TrimSpace(u.Reason))
	switch {
	case verifiedKnown && verified:
		fmt.Fprintf(out, "  %s updated your app: %s\n", who, reason)
	case verifiedKnown:
		fmt.Fprintf(out, "  %s updated your app: %s\n", who, reason)
		fmt.Fprintf(out, "  Could not verify that this update came from Civitai staff (the server did not confirm commit %s); review it with `git show %s`.\n",
			u.CommitSha, u.CommitSha)
	default:
		if name == "" {
			who = "an unnamed sender"
		}
		fmt.Fprintf(out, "  Platform update %s from %s: %s\n", u.ID, who, reason)
	}
	if len(u.Paths) > 0 {
		parts := make([]string, 0, platformUpdatePathsCap)
		for i, p := range u.Paths {
			if i == platformUpdatePathsCap {
				parts = append(parts, fmt.Sprintf("and %d more", len(u.Paths)-platformUpdatePathsCap))
				break
			}
			parts = append(parts, puText(p.Operation)+" "+puText(p.Path))
		}
		fmt.Fprintf(out, "  Files: %s\n", strings.Join(parts, ", "))
	}
	if u.TouchesManifest {
		fmt.Fprintf(out, "  It changes block.manifest.json or a storage hook, which decide what your app is allowed to do — review it with `git show %s` before you submit.\n",
			u.CommitSha)
	}
}

// gitErrLine is the last non-empty line of git's output — where git puts its
// `fatal:` — sanitised (a fetch's output can carry the server's `remote:` text).
func gitErrLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	return puText(strings.TrimSpace(s))
}

// submitPlatformUpdates is `app submit`'s step for staff platform updates. It
// returns how many it merged (the caller re-reads the tree when that is not 0).
//
// 🔴 SUBMIT REFUSES WHILE AN UPDATE IS UNMERGED. The bundle is built from the
// directory on disk, so a submit from a checkout that lacks a pending update —
// or from a directory that is not a checkout at all — would publish a version
// that silently drops it. Every way the update is left unmerged therefore
// refuses: a conflict, a dirty tree, no checkout, a missing commit, and a
// listing that could not be read (it might have held one).
//
// allowUnmerged (--allow-unmerged-platform-update) is the way past that. It
// merges nothing and refuses nothing: it lists what is being left out, on
// stderr, and the submit carries on. The submit body then carries
// `allowUnmergedPlatformUpdate: true` (Provenance.AllowUnmergedPlatformUpdate),
// which the server records for the moderator reviewing the submission.
//
// 🔴 THIS CHECK IS THE CLIENT HALF; THE SERVER IS THE BACKSTOP. The server
// checks the uploaded bundle itself for every staff update not yet shipped in
// an approved version and refuses the submit (`platform_update_missing`) unless
// the override was sent — so an older CLI, a hand-built bundle or a checkout
// this flow could not merge into is caught there too. doUpload explains that
// refusal (platformUpdateMissingRefusal).
//
// A server without the procedure, or with the flag off, has no updates: silent.
// So is the server's "App not found" (appapi.ErrPlatformUpdatesAppNotFound)
// HERE: a first submit of a brand-new slug lands there, the app has no updates
// to merge, and the submit itself is what tells an owner the app is not theirs.
func submitPlatformUpdates(ctx context.Context, api platformUpdateAPI, cloneInfo cloneInfoFetcher, warn io.Writer, dir, slug string, allowUnmerged bool) (int, error) {
	updates, err := listPendingPlatformUpdates(ctx, api, slug)
	if errors.Is(err, appapi.ErrPlatformUpdatesAppNotFound) {
		return 0, nil
	}
	if allowUnmerged {
		if err != nil {
			fmt.Fprintf(warn, "The platform-update check failed (%s); submitting anyway (%s).\n", puText(err.Error()), allowUnmergedPlatformUpdateFlag)
			return 0, nil
		}
		if len(updates) > 0 {
			fmt.Fprintf(warn, "Submitting WITHOUT merging %d platform update(s) from Civitai staff (%s):\n", len(updates), allowUnmergedPlatformUpdateFlag)
			for _, u := range updates {
				printPlatformUpdateSummary(warn, u, false, false)
			}
		}
		return 0, nil
	}
	if err != nil {
		// FAILS OPEN, like the listing gate and the SDK-floor read beside it: a
		// read that fails says nothing about whether an update is pending, and
		// refusing every submit while the procedure is broken would block the
		// platform's whole submit path on one read. The warning names the risk.
		warnf(warn, "the platform-update check failed (%s) — submitting without it; a pending update would not be in this bundle.",
			puText(err.Error()))
		return 0, nil
	}
	if len(updates) == 0 {
		return 0, nil
	}
	source := func(ctx context.Context) (string, error) {
		info, err := cloneInfo(ctx, slug)
		if err != nil {
			return "", err
		}
		return info.SourceHTTPURL, nil
	}
	merged, err := mergePlatformUpdates(ctx, api, warn, dir, slug, source, updates)
	if err != nil {
		return merged, fmt.Errorf("%w\nsubmit refused: a bundle built from this directory would drop the platform update; "+
			"merge it first, or pass %s to submit without it", err, allowUnmergedPlatformUpdateFlag)
	}
	if merged > 0 {
		fmt.Fprintf(warn, "Merged %d platform update(s) into this checkout; the bundle below includes them. Push the merge so your own repository keeps it.\n", merged)
	}
	return merged, nil
}

// platformUpdateMissingRefusal explains the server's platform_update_missing
// refusal of a submit: it lists the updates the server named (when it named
// any in a shape this client reads) on warn, and returns err wrapped with the
// way out. The server's own message is always in the returned text, so a
// refusal with an unknown details shape still says what the server said.
func platformUpdateMissingRefusal(warn io.Writer, missing *appapi.PlatformUpdateMissingError, err error) error {
	if len(missing.Updates) > 0 {
		fmt.Fprintf(warn, "Civitai refused this submit: the bundle does not contain %d platform update(s) from Civitai staff that no approved version has shipped yet:\n", len(missing.Updates))
		for _, u := range missing.Updates {
			id := puText(strings.TrimSpace(u.ID))
			if id == "" {
				id = "(no id)"
			}
			line := "  " + id
			if by := puText(strings.TrimSpace(u.By)); by != "" {
				line += " from " + by
			}
			if reason := puText(strings.TrimSpace(u.Reason)); reason != "" {
				line += ": " + reason
			}
			fmt.Fprintln(warn, line)
			if len(u.Paths) > 0 {
				paths := u.Paths
				more := 0
				if len(paths) > platformUpdatePathsCap {
					paths, more = paths[:platformUpdatePathsCap], len(paths)-platformUpdatePathsCap
				}
				parts := make([]string, 0, len(paths)+1)
				for _, p := range paths {
					parts = append(parts, puText(p))
				}
				if more > 0 {
					parts = append(parts, fmt.Sprintf("and %d more", more))
				}
				fmt.Fprintf(warn, "    Files: %s\n", strings.Join(parts, ", "))
			}
		}
	}
	return fmt.Errorf("%w\nsubmit refused by the server: the bundle is missing a staff platform update. Run `civitai app pull` in a checkout of the app's source repository to merge it, "+
		"then submit again — or pass %s to submit without it (the override is recorded for the moderator reviewing the submission)",
		err, allowUnmergedPlatformUpdateFlag)
}

// puText is the ONE place this flow sanitises text for the terminal: every
// server-supplied string it prints (reason, username, paths, branch, the source
// URL, server and git error text) goes through it, so a hostile field cannot
// carry an escape sequence or forge a line. It is the single-line sanitiser
// because every surface here is one line.
func puText(s string) string { return safeTermSingle(s) }
