package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/ui"
	"github.com/spf13/cobra"
)

// cloneInfoFetcher is the seam the pull command calls to get the tokened clone
// info. Defaulted to the real API client; tests swap it to avoid the network.
type cloneInfoFetcher func(ctx context.Context, app string) (*api.ForgejoCloneInfo, error)

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
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := strings.TrimSpace(appFlag)
			if app == "" {
				return fmt.Errorf("an app is required — pass --app <slug|appBlockId> (find the slug with `civitai app status`)")
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Token() == "" {
				return api.Tag(api.ErrUnauthorized, fmt.Errorf("no token configured — run `civitai login` (or set CIVITAI_TOKEN)"))
			}

			fetch := defaultCloneInfoFetcher(cfg)
			return runAppPull(cmd, fetch, app, args)
		},
	}
	cmd.Flags().StringVar(&appFlag, "app", "", "the app slug (repo name) or appBlockId to pull (required)")
	_ = cmd.MarkFlagRequired("app")
	return cmd
}

// defaultCloneInfoFetcher wires the real API client.
func defaultCloneInfoFetcher(cfg *config.Config) cloneInfoFetcher {
	client := api.NewWithSource(cfg.BaseURL(), auth.New(cfg), "")
	return client.GetForgejoCloneInfo
}

// runAppPull is the testable core: resolve the clone info, then clone (fresh
// dir) or pull (existing checkout). Pure of config/flag plumbing; git runs
// through gitRunner so tests can stub it.
func runAppPull(cmd *cobra.Command, fetch cloneInfoFetcher, app string, args []string) error {
	info, err := fetch(context.Background(), app)
	if err != nil {
		return err
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
