package cmd

import (
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/civitai/cli/internal/ui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// updateCheckHiddenCmd is the name of the hidden subcommand that performs the
// background GitHub refresh and writes the cache. The post-run hook spawns a
// detached copy of ourselves running this command.
const updateCheckHiddenCmd = "__update-check"

// noticeExcludedCommands are commands for which the cached update notice (and
// the background refresh spawn) are suppressed:
//   - version:        does its OWN synchronous check; double-notifying is noise.
//   - upgrade:        the whole point of running it; a notice is redundant.
//   - __update-check: the refresh worker itself; must stay silent.
//   - completion:     emits shell-eval'd script to stdout — keep it pristine.
//   - help:           help output shouldn't carry a network-derived footer.
var noticeExcludedCommands = map[string]bool{
	"version":            true,
	"upgrade":            true,
	updateCheckHiddenCmd: true,
	"completion":         true,
	"help":               true,
}

// ciEnvVars are environment variables whose presence indicates a CI/automation
// context where an interactive update notice is unwanted.
var ciEnvVars = []string{
	"CI",
	"CONTINUOUS_INTEGRATION",
	"GITHUB_ACTIONS",
	"GITLAB_CI",
	"BUILDKITE",
	"CIRCLECI",
	"TF_BUILD", // Azure Pipelines
}

// stderrIsTerminal is a seam so tests can force the TTY check. By default it
// checks whether os.Stderr is a real terminal.
var stderrIsTerminal = func() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// requestRefresh is a seam: in production it spawns the detached refresh
// process; tests override it to record that a refresh was requested without
// actually forking. It must never block.
var requestRefresh = spawnDetachedRefresh

// runningInCI reports whether any known CI env var is set.
func runningInCI() bool {
	for _, k := range ciEnvVars {
		if os.Getenv(k) != "" {
			return true
		}
	}
	return false
}

// invokedCommandName returns the name of the leaf command cobra is about to run
// (or just ran), used for the exclusion check. For the completion machinery the
// command name starts with "__complete"; we treat any such name as excluded.
func noticeSuppressedForCommand(name string) bool {
	if name == "" {
		return true
	}
	if len(name) >= 2 && name[0] == '_' && name[1] == '_' {
		// __complete, __completeNoDesc, __update-check, etc. — all internal.
		return true
	}
	return noticeExcludedCommands[name]
}

// updateNoticeGated reports whether the notice+refresh should be entirely
// suppressed for this invocation. It does NOT touch the network or disk.
//
// Suppressed when ANY of:
//   - the user opted out (--no-update-check / CIVITAI_NO_UPDATE_CHECK), OR
//   - stderr is not a TTY, OR
//   - we're in CI, OR
//   - the invoked command is excluded (version/upgrade/completion/help/internal).
func updateNoticeGated(cmdName string, noFlag bool) bool {
	if updateCheckDisabled(noFlag) {
		return true
	}
	if noticeSuppressedForCommand(cmdName) {
		return true
	}
	if runningInCI() {
		return true
	}
	if !stderrIsTerminal() {
		return true
	}
	return false
}

// maybeNotifyUpdate is the PersistentPostRun body. It is best-effort and MUST
// NOT block: it reads the cache, prints the (possibly stale) cached notice if
// warranted, kicks off a detached background refresh when the cache is stale,
// and persists the last-notified timestamp. Any error is swallowed.
//
// errW is the command's stderr writer; cmdName is the leaf command's name;
// noFlag is the resolved --no-update-check value.
func maybeNotifyUpdate(errW io.Writer, cmdName string, noFlag bool) {
	if updateNoticeGated(cmdName, noFlag) {
		return
	}

	path, err := updateCachePath()
	if err != nil {
		return
	}
	cache := readUpdateCache(path)
	now := time.Now()
	d := decideNotice(cache, version, now)

	if d.refresh {
		// Fire-and-forget; never wait. A failure here is irrelevant — next run
		// just tries again.
		_ = requestRefresh()
	}

	if d.show {
		// stderr ONLY — never stdout, so pipes/scripts are unaffected.
		printDim(errW, d.notice)
		// Record that we notified so we don't repeat within updateNotifyTTL.
		cache.LastNotified = now
		_ = writeUpdateCache(path, cache)
	}
}

// printDim writes s as a dim line to w, framed by a leading newline so it
// visually separates from the command's own output. Color is resolved against w
// itself (the command's stderr) via the shared ui layer — so ANSI is emitted
// only when w is a real terminal, and NO_COLOR/--no-color suppress it. We
// already gate on stderr being a TTY before calling, but routing through ui
// keeps the escape handling in one place.
func printDim(w io.Writer, s string) {
	_, _ = io.WriteString(w, "\n"+ui.For(w).Dim(s)+"\n")
}

// spawnDetachedRefresh launches `civitai __update-check` as a detached
// background process that fetches the latest release and rewrites the cache,
// then returns immediately. The current process never waits on it.
func spawnDetachedRefresh() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, updateCheckHiddenCmd)
	// Propagate the test/override URL into the child so it checks the same
	// endpoint. (In production this env var is unset and the child uses the
	// default GitHub URL.)
	cmd.Env = os.Environ()
	return detachAndStart(cmd)
}

// newUpdateCheckCmd builds the hidden background-refresh subcommand. It produces
// NO output, calls fetchLatestRelease, and writes the cache. It always exits 0
// (best-effort) so a detached failure never surfaces anywhere.
func newUpdateCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:    updateCheckHiddenCmd,
		Hidden: true,
		Args:   cobra.NoArgs,
		// No RunE error is ever returned — keep it utterly silent.
		Run: func(cmd *cobra.Command, args []string) {
			refreshUpdateCache()
		},
	}
}

// refreshUpdateCache does the actual GitHub fetch + cache write. Exported via a
// var seam (updateCheckURL) so the hidden command resolves the same endpoint
// the rest of the code uses. Best-effort: errors are swallowed.
func refreshUpdateCache() {
	ctx, cancel := contextWithUpdateTimeout()
	defer cancel()

	latest, err := fetchLatestRelease(ctx, latestReleaseURL)
	if err != nil || latest == "" || !isParseableVersion(latest) {
		// Even on failure, stamp last_check so we don't hammer GitHub every
		// single command while offline — back off for the full TTL.
		path, perr := updateCachePath()
		if perr == nil {
			c := readUpdateCache(path)
			c.LastCheck = time.Now()
			_ = writeUpdateCache(path, c)
		}
		return
	}

	path, err := updateCachePath()
	if err != nil {
		return
	}
	c := readUpdateCache(path)
	c.LastCheck = time.Now()
	c.LatestVersion = latest
	_ = writeUpdateCache(path, c)
}
