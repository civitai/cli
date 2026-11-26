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

	"github.com/civitai/cli/internal/appapi"
)

// STAFF PLATFORM UPDATES — the client flow of civitai/civitai#10594's
// "Owner-facing API". Civitai staff can push an update to someone else's app:
// one commit on its own branch (`platform-update/<id>`) of the app's SOURCE
// repository (civitai-apps-src/<slug>). `civitai app pull` and `civitai app
// submit` list the app's pending updates and merge each one, oldest first, into
// the checkout, then report the outcome to the server.
//
// 🔴 WHAT IS MERGED IS THE RECORDED commitSha, NEVER THE BRANCH. The branch is
// ordinary git and can move after the push; only the sha is the change the
// server's record attests. So the flow is: fetch the branch BY URL, check the
// recorded commit arrived (fetch it by sha if not), and `git merge --no-ff
// <commitSha>`. A branch tip that is not commitSha has commits that are not part
// of the update: they are left out, and the owner is told.
//
// 🔴 THE "verified Civitai staff" BADGE COMES ONLY FROM verifyCommit. The
// listing's own `by.verifiedStaff` is not read for it: verifyCommit is the one
// call the contract names as the source of the badge, and it also answers false
// for an update a moderator has since withdrawn.
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
var platformGit = func(dir string, args ...string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", errGitUnavailable
	}
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	c.Env = append(gitScrubbedEnv(os.Environ()), "GIT_TERMINAL_PROMPT=0")
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	err := c.Run()
	return buf.String(), err
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
	// 🔴 NEVER MERGE INTO UNCOMMITTED CHANGES. A merge that conflicts with them
	// is refused by git, but one that does not would leave the owner's own
	// unfinished work and the staff change tangled in one working tree, and the
	// abort path below could not separate them. Untracked files are not counted:
	// git refuses a merge that would overwrite one, and that refusal is reported.
	status, err := platformGit(dir, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return 0, fmt.Errorf("%w: could not read the state of %s (%s); nothing was merged", ErrUnmergedPlatformUpdate, dir, gitErrLine(status))
	}
	if strings.TrimSpace(status) != "" {
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
		// and a deleted branch all land here, and reporting `conflicted` for any
		// of them would drop the update off the owner's pending list for good
		// (listPending returns pending updates only). So: no report, and the next
		// run tries again.
		return false, fmt.Errorf("%w: could not fetch platform update %s from %s (%s); nothing was merged or reported — run the command again",
			ErrUnmergedPlatformUpdate, u.ID, puText(r.sourceURL), gitErrLine(fetchOut))
	}
	verified := r.verify(ctx, sha)
	if !r.hasCommit(sha) {
		// The branch WAS fetched and the recorded commit is not on it, nor
		// reachable by sha: the update as recorded no longer exists there.
		printPlatformUpdateSummary(r.out, u, verified, true)
		r.resolve(ctx, u, appapi.PlatformUpdateConflicted, "")
		fmt.Fprintf(r.out, "Its branch %s no longer holds the recorded commit %s, so nothing was merged; Civitai was told it did not merge.\n",
			puText(u.Branch), sha)
		return false, fmt.Errorf("%w: platform update %s was not merged (its recorded commit %s is missing from the source repository)",
			ErrUnmergedPlatformUpdate, u.ID, sha)
	}

	// Already in this history (merged by hand, or merged by an earlier run whose
	// report did not reach the server): report it, merge nothing.
	if _, err := platformGit(r.dir, "merge-base", "--is-ancestor", sha, "HEAD"); err == nil {
		head, herr := platformGit(r.dir, "rev-parse", "HEAD")
		if herr == nil {
			r.resolve(ctx, u, appapi.PlatformUpdateMerged, strings.TrimSpace(head))
		}
		fmt.Fprintf(r.out, "Platform update %s (commit %s) is already in %s.\n", u.ID, sha, r.dir)
		printPlatformUpdateSummary(r.out, u, verified, true)
		return false, nil
	}

	// 3+4. Merge the recorded sha and nothing else.
	mergeOut, mergeErr := platformGit(r.dir, "merge", "--no-ff", "--no-edit",
		"-m", "Merge Civitai platform update "+u.ID, sha)
	if mergeErr != nil {
		if _, err := platformGit(r.dir, "rev-parse", "-q", "--verify", "MERGE_HEAD"); err != nil {
			// git refused before it started (an untracked file in the way, no
			// committer identity, …): nothing to abort and nothing to report as a
			// conflict, because the update was never tried against the tree.
			return false, fmt.Errorf("%w: git could not merge platform update %s (commit %s) into %s: %s",
				ErrUnmergedPlatformUpdate, u.ID, sha, r.dir, puText(strings.TrimSpace(mergeOut)))
		}
		conflicted, _ := platformGit(r.dir, "diff", "--name-only", "--diff-filter=U")
		_, _ = platformGit(r.dir, "merge", "--abort")
		r.resolve(ctx, u, appapi.PlatformUpdateConflicted, "")
		fmt.Fprintf(r.out, "Platform update %s (commit %s) conflicts with your changes in %s, so nothing was merged (the merge was aborted).\n", u.ID, sha, r.dir)
		printPlatformUpdateSummary(r.out, u, verified, true)
		if files := strings.Fields(conflicted); len(files) > 0 {
			for i := range files {
				files[i] = puText(files[i])
			}
			fmt.Fprintf(r.out, "  Conflicts in: %s\n", strings.Join(files, ", "))
		}
		fmt.Fprintf(r.out, "To merge it by hand, merge the recorded commit %s (not the branch):\n", sha)
		fmt.Fprintf(r.out, "  git -C %s fetch %s %s && git -C %s merge %s\n", r.dir, puText(r.sourceURL), u.Branch, r.dir, sha)
		fmt.Fprintln(r.out, "then resolve the conflicts and commit. The next `civitai app pull` reports it merged.")
		return false, fmt.Errorf("%w: platform update %s conflicts with your changes — merge %s by hand (instructions above)",
			ErrUnmergedPlatformUpdate, u.ID, sha)
	}

	head, err := platformGit(r.dir, "rev-parse", "HEAD")
	if err != nil {
		return true, fmt.Errorf("merged platform update %s but could not read the merge commit: %s", u.ID, gitErrLine(head))
	}
	mergeSha := strings.TrimSpace(head)
	r.resolve(ctx, u, appapi.PlatformUpdateMerged, mergeSha)
	fmt.Fprintf(r.out, "Merged platform update %s (commit %s) into %s.\n", u.ID, sha, r.dir)
	printPlatformUpdateSummary(r.out, u, verified, true)
	if fetchHead != "" && fetchHead != sha {
		fmt.Fprintf(r.out, "  Its branch %s has commits that are not part of the update (tip %s); they were left out. Only %s was merged.\n",
			puText(u.Branch), fetchHead, sha)
	}
	return true, nil
}

// verify asks the server whether sha is a recorded update. Any failure is
// "not verified": the badge is only ever drawn on a positive answer.
func (r *platformUpdateRun) verify(ctx context.Context, sha string) bool {
	ok, err := r.api.VerifyPlatformUpdateCommit(ctx, r.slug, sha)
	return err == nil && ok
}

// resolve reports an outcome. A failure is printed and does not undo the merge:
// the merge is local and done, and the next run finds the commit already in the
// history and reports it again.
func (r *platformUpdateRun) resolve(ctx context.Context, u appapi.PlatformUpdate, outcome, mergeSha string) {
	if err := r.api.ResolvePlatformUpdate(ctx, u.ID, outcome, mergeSha); err != nil {
		fmt.Fprintf(r.out, "Could not report platform update %s as %s to Civitai: %s\n", u.ID, outcome, puText(err.Error()))
	}
}

// printPlatformUpdateSummary prints who made the update and why, what it
// changes, and — when it does — that it changes what the app may do.
//
// verifiedKnown false prints the attribution without any claim about staff
// status — used where verifyCommit has not been asked (the dirty-tree refusal,
// which stops before fetching anything).
func printPlatformUpdateSummary(out io.Writer, u appapi.PlatformUpdate, verified, verifiedKnown bool) {
	who := "An account with no username"
	if u.By.Username != nil && strings.TrimSpace(*u.By.Username) != "" {
		who = puText(strings.TrimSpace(*u.By.Username))
	}
	reason := puText(strings.TrimSpace(u.Reason))
	switch {
	case verifiedKnown && verified:
		fmt.Fprintf(out, "  %s (verified Civitai staff) updated your app: %s\n", who, reason)
	case verifiedKnown:
		fmt.Fprintf(out, "  %s updated your app: %s\n", who, reason)
		fmt.Fprintf(out, "  Could not verify that this update came from Civitai staff (the server did not confirm commit %s); review it with `git show %s`.\n",
			u.CommitSha, u.CommitSha)
	default:
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
// stderr, and the submit carries on. It is recorded nowhere server-side — the
// submit body has no field for it today.
//
// A server without the procedure, or with the flag off, has no updates: silent.
func submitPlatformUpdates(ctx context.Context, api platformUpdateAPI, cloneInfo cloneInfoFetcher, warn io.Writer, dir, slug string, allowUnmerged bool) (int, error) {
	updates, err := listPendingPlatformUpdates(ctx, api, slug)
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

// puText is the ONE place this flow sanitises text for the terminal: every
// server-supplied string it prints (reason, username, paths, branch, the source
// URL, server and git error text) goes through it, so a hostile field cannot
// carry an escape sequence or forge a line. It is the single-line sanitiser
// because every surface here is one line.
func puText(s string) string { return safeTermSingle(s) }
