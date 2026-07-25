package cmd

import "github.com/spf13/cobra"

// newAppCmd is the `civitai app` command group for Apps authoring.
func newAppCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "app",
		Short: "Author and ship Civitai Apps",
		Long: `Author and ship Civitai Apps.

An App is a sandboxed static web app served in an iframe. The platform
owns the build and the runtime; the only mandatory file is block.manifest.json.
The typical lifecycle is create -> validate -> submit.

"civitai app create" is the friendly, batteries-included scaffolder (defaults to
the rich page-money SDK template); "civitai app init" is the same scaffolder
with a no-build static default (back-compat alias).`,
		Example: `  civitai app list
  civitai app view my-block
  civitai app create my-block
  civitai app validate ./my-block
  civitai app submit ./my-block
  civitai app status
  civitai app withdraw pubreq_01H
  civitai app dev-token my-block`,
	}
	cmd.AddCommand(newAppListCmd())
	cmd.AddCommand(newAppViewCmd())
	cmd.AddCommand(newAppCreateCmd())
	cmd.AddCommand(newAppInitCmd())
	cmd.AddCommand(newAppValidateCmd())
	cmd.AddCommand(newAppSubmitCmd())
	cmd.AddCommand(newAppStatusCmd())
	cmd.AddCommand(newAppWithdrawCmd())
	cmd.AddCommand(newAppDevTokenCmd())
	cmd.AddCommand(newAppDevTunnelCmd())
	cmd.AddCommand(newAppPullCmd())
	return cmd
}
