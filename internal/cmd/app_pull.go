package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// cloneInfoFetcher is the seam the pull command calls to get the tokened clone
// info. Defaulted to the real API client; tests swap it to avoid the network.
type cloneInfoFetcher func(ctx context.Context, app string) (*appapi.ForgejoCloneInfo, error)

// submissionLister is the seam pull uses to DISAMBIGUATE a not-found clone-info
// answer. It reads the caller's own submissions for a slug — the same route
// `civitai app status` reads — so a 404 that means "your app is still in review"
// can stop claiming the app does not exist.
type submissionLister func(ctx context.Context, blockID string) ([]appapi.Submission, error)

// gitRunner runs a git subcommand in `dir` (empty = current dir). It's a package
// var so tests can stub the exec without a real git/repo. Output is forwarded to
// the user's terminal so clone/pull progress is visible.
var gitRunner = func(dir string, args ...string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git is required for `civitai app pull` but was not found on PATH")
	}
	c := exec.Command("git", args...)
	if dir != "" {
		c.Dir = dir
	}
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// isGitRepo reports whether dir contains a git working tree (a `.git` entry).
// Used to decide clone (fresh dir) vs pull (existing checkout). A `.git` entry
// is either a directory (normal checkout) or a gitfile (linked worktree), so any
// stat hit counts.
func isGitRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func newAppPullCmd() *cobra.Command {
	var appFlag string

	cmd := &cobra.Command{
		Use:   "pull [dir]",
		Short: "Clone or sync your app's repository from Civitai",
		Long: `Clone (or, if [dir] is already a checkout, pull) the canonical repository
backing one of YOUR approved Apps. This is the read side of git authoring:
it fetches the current block.manifest.json + source so you can edit locally and
then submit (` + "`civitai app submit`" + `) or push.

Authentication uses your stored credential (` + "`civitai login`" + ` or a personal
API key). The command calls an owner-only endpoint that lazily provisions a
scoped, read-only Forgejo identity for you and returns a clone URL with a push/
pull token embedded.

⚠  SECURITY — TOKEN-IN-URL LEAKAGE: the clone URL embeds your access token as
HTTP-Basic credentials (https://<user>:<token>@...). On a fresh CLONE, git
stores the remote URL in .git/config, so the token lands on disk in the clone;
treat the checkout as sensitive: do NOT commit .git/config or share the
directory, and consider clearing the remote URL (or replacing it with the
credential-less HTTPS URL ` + "`git remote set-url origin <httpUrl>`" + `) after the
clone if you rely on a git credential helper. On a SYNC (pull into an existing
checkout) the URL is passed explicitly and is NOT persisted to .git/config, but
the token still transiently appears in the git child process's arguments, so it
is briefly visible to other processes via ` + "`ps`" + ` / /proc/<pid>/cmdline.

The repo only exists once your FIRST version has been submitted as a ZIP and
approved; before then the command tells you so instead of failing obscurely.`,
		Example: `  civitai app pull --app my-block               # clone into ./my-block
  civitai app pull ./my-block --app my-block    # clone/sync into ./my-block
  civitai app pull . --app my-block             # sync the current directory`,
		// The slug is a FLAG here and the positional is the DIRECTORY — the
		// inverse of every other `app` subcommand, so the natural first attempt
		// (`app pull <slug>`) is a mistake worth naming. Cobra's own count
		// validator says "accepts at most 1 arg(s)" and its required-flag check
		// says `required flag(s) "app" not set`; neither tells a user who typed
		// the slug positionally what the two slots ARE. enforceUsageExitCodes
		// runs this before cobra's required-flag validator (root.go), so these
		// messages win, and it tags them ErrUsage → exit 2, unchanged.
		//
		// The NO-ARGUMENT case is in here too: a bare `civitai app pull` is the
		// most likely first-contact invocation, and leaving it to cobra's
		// required-flag check answered it with `required flag(s) "app" not set`
		// — the one message that never says the positional is the directory.
		// The two-positional message is conditional for the same reason: with
		// --app already set, "the app goes in --app" is a non sequitur, and the
		// only thing wrong is the extra positional.
		Args: func(cmd *cobra.Command, args []string) error {
			hasApp := cmd.Flags().Changed("app")
			if len(args) > 1 {
				if hasApp {
					return fmt.Errorf("accepts at most 1 arg (the target DIRECTORY), received %d: `civitai app pull [dir] --app <slug>`", len(args))
				}
				return fmt.Errorf("accepts at most 1 arg (the target DIRECTORY), received %d — the app goes in --app: `civitai app pull [dir] --app <slug>`", len(args))
			}
			if !hasApp {
				return fmt.Errorf("--app is required, and the positional argument is the DIRECTORY, not the app: `civitai app pull [dir] --app <slug>` (find the slug with `civitai app status`)")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			app := strings.TrimSpace(appFlag)
			if app == "" {
				// A missing required flag is a usage error (exit 2).
				return asUsageError(fmt.Errorf("an app is required — pass --app <slug|appBlockId> (find the slug with `civitai app status`)"))
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Token() == "" {
				return civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf("no token configured — run `civitai login` (or set CIVITAI_TOKEN)"))
			}

			client := appapi.NewWithSource(cfg.BaseURL(), auth.New(cfg), "")
			return runAppPull(cmd, client.GetForgejoCloneInfo, client.ListSubmissions, app, args)
		},
	}
	cmd.Flags().StringVar(&appFlag, "app", "", "the app slug (repo name) or appBlockId to pull (required)")
	_ = cmd.MarkFlagRequired("app")
	return cmd
}

// explainMissingApp replaces the clone-info 404's "no such app for your account"
// when the CLI can prove the app DOES exist and simply has no approved version.
//
// 🔴 The server's 404 is UNCONDITIONAL for both causes. `getMyForgejoCloneInfo`
// answers 404 for "no such app" AND for "your app exists but nothing is approved
// yet", so `cloneInfoError` (internal/appapi/appblocks.go) cannot tell them
// apart, and runAppPull's own NotYetAvailable branch — which needs a 200 — is
// therefore UNREACHABLE for a never-approved app. That is why the command told
// users to run `civitai app status`, which then listed the app the error had
// just denied.
//
// The disambiguation is client-side and uses only the caller's own submissions:
// rows exist for the slug, but none carries an appBlockId (the server nulls it
// until a version is APPROVED — see resolveAppBlockID in app_metrics.go, which
// answers the identical precondition and whose wording this mirrors).
//
// Every uncertainty falls back to `orig`: a disambiguation that cannot be made
// must never REPLACE the server's answer. The not-found classification is
// re-applied verbatim, so the exit code (4) is unchanged — AGENTS.md item 7.
//
// 🔴 SO "no such app for your account" DOES NOT MEAN "no submission matches".
// It is the answer whenever the CLI cannot prove otherwise, which is FOUR
// states, three of them with a matching submission: the lookup itself failed, a
// version IS approved, or `--app` carried an appBlockId rather than the slug.
// The README section for this command must describe it that way — it once
// promised the narrow reading, which is false in every one of those states.
//
// The appBlockId case is inherent, not an oversight: ListSubmissions filters on
// `blockId`, which IS the slug (appapi.Submission.BlockID), so an appBlockId
// selector returns no rows and falls back. It cannot cost anyone the good
// message, because the server nulls `appBlockId` until a version is APPROVED —
// a never-approved app has no appBlockId for the user to type. `--app` still
// accepts one for the clone-info call itself, which resolves either spelling.
//
// The lookup is one extra request on an ALREADY-FAILING path, bounded by the
// appapi client's 30s timeout (appapi.defaultTimeout); appapi.ListSubmissionsCap
// documents why the row cap cannot hide the answer here — the listing is
// filtered to this one slug, far below the cap.
func explainMissingApp(ctx context.Context, list submissionLister, app string, orig error) error {
	if list == nil || !errors.Is(orig, civitai.ErrNotFound) {
		return orig
	}
	subs, err := list(ctx, app)
	if err != nil || len(subs) == 0 {
		// Lookup failed, or the slug really matches nothing of yours — the
		// server's own message is the correct one.
		return orig
	}
	for i := range subs {
		if subs[i].AppBlockID != nil && *subs[i].AppBlockID != "" {
			// Something IS approved, so "not approved yet" would be a new false
			// explanation replacing the old one.
			return orig
		}
	}
	// Newest first (ListSubmissions), so subs[0] is the state to report.
	latest := ""
	if state := strings.TrimSpace(strings.TrimSpace(subs[0].Version) + " " + strings.TrimSpace(subs[0].Status)); state != "" {
		latest = " (latest submission: " + state + ")"
	}
	return civitai.Tag(civitai.ErrNotFound, fmt.Errorf(
		"app %q has no approved version yet — `civitai app pull` clones the repo that only exists once a submitted version is approved%s; %s",
		app, latest, pullReviewAdvice(app, subs[0].Status)))
}

// pullReviewAdvice is the next step for an app with nothing approved. The state
// is in hand (`Submission.Status`), so a TERMINAL one must not be described as
// in progress: "check where it is in review" was printed for `rejected` and
// `withdrawn` rows, where there is nothing in review to check — the next step is
// a new submission.
//
// 🔴 TWO CALLERS, ONE RULE. `app metrics` (resolveAppBlockID, app_metrics.go)
// reaches the SAME precondition by a different route — submissions exist, none
// carries an appBlockId — and it open-coded the same sentence, so a rejected app
// was told to check a review by one command and told nothing is in review by the
// other, out of one binary. Anything state-dependent about "nothing is approved
// yet" belongs here, not at a call site.
//
// It returns a STRING and no error classification on purpose: the two callers
// publish different exit codes for this state (pull 4, metrics 1 — AGENTS.md
// item 7), so the shared piece must be incapable of moving either.
func pullReviewAdvice(app, status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "rejected":
		return fmt.Sprintf("that submission was REJECTED, so nothing is in review — read why with `civitai app status %s`, then submit a new version with `civitai app submit`", app)
	case "withdrawn":
		return fmt.Sprintf("that submission was WITHDRAWN, so nothing is in review — submit a new version with `civitai app submit` (the history is in `civitai app status %s`)", app)
	default:
		return fmt.Sprintf("check where it is in review with `civitai app status %s`", app)
	}
}

// runAppPull is the testable core: resolve the clone info, then clone (fresh
// dir) or pull (existing checkout). Pure of config/flag plumbing; git runs
// through gitRunner so tests can stub it.
func runAppPull(cmd *cobra.Command, fetch cloneInfoFetcher, list submissionLister, app string, args []string) error {
	ctx := context.Background()
	info, err := fetch(ctx, app)
	if err != nil {
		return explainMissingApp(ctx, list, app, err)
	}
	if info.NotYetAvailable {
		msg := info.Message
		if msg == "" {
			msg = "git access is not available for this app yet — submit and get your first version approved first."
		}
		return fmt.Errorf("%s", msg)
	}
	if info.CloneURL == "" {
		return fmt.Errorf("server returned no clone URL")
	}

	// Target dir: explicit [dir] arg, else ./<slug>. The slug is server-controlled,
	// so a default dir derived from it must NOT be allowed to escape the current
	// directory: reject a slug-derived dir that contains a path separator or `..`
	// (a malicious server could otherwise target an arbitrary/absolute path). An
	// explicit user-supplied [dir] is the user's own choice, so it's left as-is.
	dir := info.Slug
	if len(args) == 1 && strings.TrimSpace(args[0]) != "" {
		dir = strings.TrimSpace(args[0])
	} else {
		if dir != filepath.Base(dir) || dir == "." || dir == ".." || strings.ContainsRune(dir, filepath.Separator) || strings.Contains(dir, "..") {
			return fmt.Errorf("server returned an unsafe slug %q for the target directory — pass an explicit [dir] instead", info.Slug)
		}
	}

	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	if isGitRepo(dir) {
		// Existing checkout → sync. We don't trust the stored remote (it may be
		// stale or credential-less), so pass the tokened URL explicitly. Done as an
		// explicit fetch+merge rather than `git pull <url>`: the `--` before the URL
		// neutralizes a dash-leading URL (argument-injection hardening — a bare
		// positional URL beginning with `-` would otherwise be parsed by git as an
		// option, e.g. `--upload-pack=<cmd>`, and execute attacker-chosen commands).
		// `merge --ff-only FETCH_HEAD` keeps the fast-forward-only semantics: it
		// blocks DIVERGED history and aborts on a CONFLICTING dirty tree, and never
		// creates a merge commit. It merges FETCH_HEAD into whatever branch is
		// currently checked out (not a server-named branch) — same as the prior
		// no-refspec pull, just made injection-safe.
		fmt.Fprintf(out, "Syncing existing checkout in %s …\n", dir)
		if err := gitRunner("", "-C", dir, "fetch", "--", info.CloneURL, "HEAD"); err != nil {
			return fmt.Errorf("git fetch failed: %w", err)
		}
		if err := gitRunner("", "-C", dir, "merge", "--ff-only", "FETCH_HEAD"); err != nil {
			return fmt.Errorf("git merge --ff-only failed: %w", err)
		}
		fmt.Fprintln(out, ui.Success(fmt.Sprintf("Synced %s.", info.Slug)))
		// On the sync path the tokened URL is passed explicitly to `git fetch` and is
		// NOT written to .git/config — so there's nothing to scrub from disk. The only
		// exposure is the transient one: the token appeared in the git fetch child
		// process's arguments while the fetch ran.
		fmt.Fprintf(errOut, "\n%s\n", ui.For(errOut).Warn(
			"The fetch URL embedded your access token in the git command line — it was "+
				"briefly visible to other local processes (via `ps` / /proc/<pid>/cmdline) "+
				"while syncing. It was NOT persisted to .git/config."))
		return nil
	}

	// Fresh dir → clone. git refuses to clone into a non-empty existing dir, so a
	// pre-existing-but-not-a-repo dir surfaces git's own clear error.
	fmt.Fprintf(out, "Cloning %s into %s …\n", info.Slug, dir)
	if err := gitRunner("", "clone", "--", info.CloneURL, dir); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}
	fmt.Fprintln(out, ui.Success(fmt.Sprintf("Cloned %s into %s.", info.Slug, dir)))
	// On the clone path git persists the tokened remote URL to .git/config, so the
	// token now lives on disk. Surface the config-persistence caveat + the remedy.
	fmt.Fprintf(errOut, "\n%s\n", ui.For(errOut).Warn(
		"The clone URL embeds your access token. git stored it in .git/config — "+
			"do not commit or share it. To drop it: `git -C "+dir+" remote set-url origin "+info.HTTPURL+"`."))
	return nil
}
