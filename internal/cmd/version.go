package cmd

import (
	"fmt"
	"runtime"

	"github.com/civitai/cli/internal/ui"
	"github.com/spf13/cobra"
)

// unresolvedMarker is the honest stand-in printed when commit/date couldn't be
// resolved by any source (ldflags, vcs.*, pseudo-version, or GitHub). It names
// the most likely build shape so the value reads as honestly-unavailable rather
// than the misleading raw "none"/"unknown": a clean tag means a `go install`
// build (proxy cache, no VCS, offline); anything else is a bare source build.
func unresolvedMarker(version string) string {
	if isCleanReleaseTag(version) {
		return "unavailable (go install)"
	}
	return "unavailable (source build)"
}

// displayCommit renders the commit for `civitai version`, substituting the
// honest marker for the still-unresolved default instead of the misleading
// raw "none".
func displayCommit(commit, version string) string {
	if commit == "none" {
		return unresolvedMarker(version)
	}
	return commit
}

// displayDate renders the build date, substituting the honest marker for the
// still-unresolved default instead of the misleading raw "unknown".
func displayDate(date, version string) string {
	if date == "unknown" {
		return unresolvedMarker(version)
	}
	return date
}

func newVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version, commit, and build date",
		Long: `Print detailed build information for this civitai binary.

The version, commit, and date are stamped in at release time. For a plain
"go install" or source build they fall back to the embedded Go build info
(module version + VCS revision/time).

After printing build info, this command makes a single unauthenticated call to
the GitHub releases API to tell you if a newer release is available. The check
is best-effort (short timeout, fails silently offline) and never sends your API
token. Skip it with --no-update-check or by setting CIVITAI_NO_UPDATE_CHECK.`,
		Example: `  civitai version
  civitai version --no-update-check`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			// --no-update-check is now a persistent flag inherited from root.
			noUpdateCheck, _ := cmd.Flags().GetBool("no-update-check")
			disabled := updateCheckDisabled(noUpdateCheck)

			// A go-install build (tagged module@vX) carries no commit/date. If
			// we're online and not opted out, resolve them from GitHub for the
			// resolved tag. Fail-silent: leaves the defaults on any error.
			rCommit, rDate := enrichBuildInfoFromGitHub(commit, date, version, disabled)

			fmt.Fprintf(out, "civitai %s\n", version)
			fmt.Fprintf(out, "  commit: %s\n", displayCommit(rCommit, version))
			fmt.Fprintf(out, "  built:  %s\n", displayDate(rDate, version))
			fmt.Fprintf(out, "  go:     %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)

			// Honest hint when metadata is genuinely unavailable (e.g. an offline
			// tagged go-install): release/Homebrew builds (or being online)
			// carry the full commit + date.
			if rCommit == "none" || rDate == "unknown" {
				fmt.Fprintln(out, ui.Dim("  note:   build metadata is unavailable for this build;"))
				fmt.Fprintln(out, ui.Dim("          release/Homebrew builds (or running a tagged install online) carry the full commit + date."))
			}

			if !disabled {
				printUpdateNotice(out, version)
			}
			return nil
		},
	}
	return cmd
}
