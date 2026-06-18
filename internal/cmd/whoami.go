package cmd

import (
	"context"
	"fmt"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/config"
	"github.com/spf13/cobra"
)

func newWhoAmICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Verify your stored API token",
		Long: `Verify the stored API token by calling the Civitai API and printing the
authenticated username. Reads the token from config or CIVITAI_TOKEN.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Token() == "" {
				return fmt.Errorf("no token configured — run `civitai login` (or set CIVITAI_TOKEN)")
			}
			client := api.New(cfg.BaseURL(), cfg.Token(), "")
			id, err := client.WhoAmI(context.Background())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Logged in as %s (id %d) at %s\n", id.Username, id.ID, cfg.BaseURL())
			return nil
		},
	}
	return cmd
}
