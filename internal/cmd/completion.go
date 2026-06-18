package cmd

import (
	"github.com/spf13/cobra"
)

// newCompletionCmd exposes Cobra's built-in shell-completion generator with a
// helpful, newcomer-friendly Long describing how to install it per shell.
func newCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate a shell-completion script",
		Long: `Generate a shell-completion script for the civitai CLI.

Load completions for your current session, or install them permanently:

Bash:
  # current shell
  source <(civitai completion bash)
  # permanent (Linux)
  civitai completion bash > /etc/bash_completion.d/civitai

Zsh:
  # ensure completion is enabled: echo "autoload -U compinit; compinit" >> ~/.zshrc
  civitai completion zsh > "${fpath[1]}/_civitai"

Fish:
  civitai completion fish > ~/.config/fish/completions/civitai.fish

PowerShell:
  civitai completion powershell | Out-String | Invoke-Expression`,
		Example: `  source <(civitai completion bash)
  civitai completion zsh > "${fpath[1]}/_civitai"`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletionV2(out, true)
			case "zsh":
				return cmd.Root().GenZshCompletion(out)
			case "fish":
				return cmd.Root().GenFishCompletion(out, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(out)
			}
			return nil
		},
	}
	return cmd
}
