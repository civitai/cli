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
backing one of YOUR approved App Blocks. This is the read side of git authoring:
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
				return fmt.Errorf("no token configured — run `civitai login` (or set CIVITAI_TOKEN)")
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

	// Target dir: explicit [dir] arg, else ./<slug>.
	dir := info.Slug
	if len(args) == 1 && strings.TrimSpace(args[0]) != "" {
		dir = strings.TrimSpace(args[0])
	}

	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	if isGitRepo(dir) {
		// Existing checkout → pull. We don't trust the stored remote (it may be
		// stale or credential-less); fetch + reset to the tokened URL's HEAD branch
		// would be heavier than needed, so pass the tokened URL explicitly. Use
		// --ff-only so a diverged/dirty checkout fails cleanly with git's own
		// message instead of silently creating a merge commit.
		fmt.Fprintf(out, "Syncing existing checkout in %s …\n", dir)
		if err := gitRunner("", "-C", dir, "pull", "--ff-only", info.CloneURL); err != nil {
			return fmt.Errorf("git pull failed: %w", err)
		}
		fmt.Fprintf(out, "Synced %s.\n", info.Slug)
		// On the pull path the tokened URL is passed explicitly on the command line
		// and is NOT written to .git/config — so there's nothing to scrub from disk.
		// The only exposure is the transient one: the token appeared in the git
		// child process's arguments while the pull ran.
		fmt.Fprintln(errOut,
			"\n⚠  The pull URL embedded your access token in the git command line — it was "+
				"briefly visible to other local processes (via `ps` / /proc/<pid>/cmdline) "+
				"while syncing. It was NOT persisted to .git/config.")
		return nil
	}

	// Fresh dir → clone. git refuses to clone into a non-empty existing dir, so a
	// pre-existing-but-not-a-repo dir surfaces git's own clear error.
	fmt.Fprintf(out, "Cloning %s into %s …\n", info.Slug, dir)
	if err := gitRunner("", "clone", info.CloneURL, dir); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}
	fmt.Fprintf(out, "Cloned %s into %s.\n", info.Slug, dir)
	// On the clone path git persists the tokened remote URL to .git/config, so the
	// token now lives on disk. Surface the config-persistence caveat + the remedy.
	fmt.Fprintln(errOut,
		"\n⚠  The clone URL embeds your access token. git stored it in .git/config — "+
			"do not commit or share it. To drop it: `git -C "+dir+" remote set-url origin "+info.HTTPURL+"`.")
	return nil
}
