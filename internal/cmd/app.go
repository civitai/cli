package cmd

import "github.com/spf13/cobra"

// newAppCmd is the `civitai app` command group for App Blocks authoring.
func newAppCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "app",
		Short: "Author and ship Civitai App Blocks",
		Long: `Author and ship Civitai App Blocks.

An App Block is a sandboxed static web app served in an iframe. The platform
owns the build and the runtime; the only mandatory file is block.manifest.json.
The typical lifecycle is init -> validate -> submit.`,
		Example: `  civitai app init my-block --template page-vite
  civitai app validate ./my-block
  civitai app submit ./my-block`,
	}
	cmd.AddCommand(newAppInitCmd())
	cmd.AddCommand(newAppValidateCmd())
	cmd.AddCommand(newAppSubmitCmd())
	return cmd
}
