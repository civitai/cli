package cmd

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/civitai/cli/internal/config"
	"github.com/spf13/cobra"
)

func newLoginCmd() *cobra.Command {
	var tokenFlag string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store your Civitai API token",
		Long: `Store a Civitai API token for authenticated commands (e.g. app submit).

Create a token at https://civitai.com/user/account (API Keys). The token is
saved to your config file (~/.config/civitai/config.yaml, owner-readable only)
and can also be supplied via the CIVITAI_TOKEN environment variable.

  civitai login --token <token>
  civitai login                 # prompts for the token`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			token := strings.TrimSpace(tokenFlag)
			if token == "" {
				fmt.Fprint(cmd.OutOrStdout(), "Civitai API token: ")
				r := bufio.NewReader(cmd.InOrStdin())
				line, _ := r.ReadString('\n')
				token = strings.TrimSpace(line)
			}
			if token == "" {
				return fmt.Errorf("no token provided")
			}
			if err := cfg.SetToken(token); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Token saved to %s\n", cfg.Path())
			fmt.Fprintln(cmd.OutOrStdout(), "Verify with: civitai whoami")
			return nil
		},
	}
	cmd.Flags().StringVar(&tokenFlag, "token", "", "API token (omit to be prompted)")
	return cmd
}
