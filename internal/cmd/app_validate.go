package cmd

import (
	"fmt"

	"github.com/civitai/cli/internal/validate"
	"github.com/spf13/cobra"
)

func newAppValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [dir]",
		Short: "Validate block.manifest.json against the App Block schema",
		Long: `Validate an App Block project.

Checks block.manifest.json against the vendored JSON Schema (the shared
contract derived from the platform validator), plus structural checks:
  - the manifest is present at the project root
  - buildCommand and outputDir are coherent (outputDir set when buildCommand is)

Server-owned fields (iframe.src, trustTier) are REJECTED if set — the platform
assigns them; your manifest must not.

Defaults to the current directory.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			out := cmd.OutOrStdout()

			res, err := validate.Dir(dir)
			if err != nil {
				return err
			}
			if res.OK() {
				fmt.Fprintf(out, "OK — %s is valid\n", dir)
				return nil
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "%d validation error(s) in %s:\n", len(res.Errors), dir)
			for _, e := range res.Errors {
				fmt.Fprintf(cmd.ErrOrStderr(), "  - %s\n", e)
			}
			return fmt.Errorf("validation failed")
		},
	}
	return cmd
}
