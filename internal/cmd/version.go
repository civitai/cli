package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

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
			fmt.Fprintf(out, "civitai %s\n", version)
			fmt.Fprintf(out, "  commit: %s\n", commit)
			fmt.Fprintf(out, "  built:  %s\n", date)
			fmt.Fprintf(out, "  go:     %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)

			// --no-update-check is now a persistent flag inherited from root.
			noUpdateCheck, _ := cmd.Flags().GetBool("no-update-check")
			if !updateCheckDisabled(noUpdateCheck) {
				printUpdateNotice(out, version)
			}
			return nil
		},
	}
	return cmd
}
