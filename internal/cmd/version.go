package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version, commit, and build date",
		Long: `Print detailed build information for this civitai binary.

The version, commit, and date are stamped in at release time. For a plain
"go install" or source build they read "dev" / "none" / "unknown".`,
		Example: `  civitai version`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "civitai %s\n", version)
			fmt.Fprintf(out, "  commit: %s\n", commit)
			fmt.Fprintf(out, "  built:  %s\n", date)
			fmt.Fprintf(out, "  go:     %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
			return nil
		},
	}
}
