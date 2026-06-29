package cmd

import (
	"context"
	"fmt"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/spf13/cobra"
)

func newWhoAmICmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Verify your stored API token",
		Long: `Verify the stored API token by calling the Civitai API and printing the
authenticated username. Reads the token from config or CIVITAI_TOKEN.`,
		Example: `  civitai whoami
  civitai whoami --json   # raw JSON (scriptable)`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Token() == "" {
				return fmt.Errorf("no token configured — run `civitai login` (or set CIVITAI_TOKEN)")
			}
			client := api.NewWithSource(cfg.BaseURL(), auth.New(cfg), "")
			id, err := client.WhoAmI(context.Background())
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			if jsonOut {
				return writeJSON(out, map[string]any{
					"username": id.Username,
					"id":       id.ID,
					"base_url": cfg.BaseURL(),
					"capabilities": map[string]bool{
						"can_spend_buzz": id.CanSpendBuzz(),
						"can_read_buzz":  id.CanReadBuzz(),
					},
				})
			}

			fmt.Fprintf(out, "Logged in as %s (id %d) at %s\n", id.Username, id.ID, cfg.BaseURL())

			// Decode the token's scope bitmask so the money-path dead end (an
			// OAuth login can't spend) is visible BEFORE a dev:live run.
			fmt.Fprintln(out, "\nCapabilities:")
			fmt.Fprintf(out, "  Spend Buzz (AI Services): %s\n", yesNo(id.CanSpendBuzz()))
			fmt.Fprintf(out, "  Read Buzz balance:        %s\n", yesNo(id.CanReadBuzz()))
			if !id.CanSpendBuzz() {
				fmt.Fprintln(out, "\nThis credential can't spend Buzz — money-path `dev:live` generation needs a")
				fmt.Fprintln(out, "full-scope personal API key: create one at https://civitai.com/user/account,")
				fmt.Fprintln(out, "then `civitai login --token <key>`. (OAuth login can submit/withdraw but not spend.)")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit raw JSON (scriptable)")
	return cmd
}

// yesNo renders a capability bit as "yes"/"no".
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
