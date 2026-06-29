package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/spf13/cobra"
)

// buzzScopeHint is the actionable message shown when the stored credential
// cannot read the Buzz balance (a 403 from buzz.getBuzzAccount, typical of an
// OAuth login token). It points at the only working path: a full-scope personal
// API key.
const buzzScopeHint = "This credential can't read your Buzz balance (needs a full-scope personal API key).\n" +
	"Create one at https://civitai.com/user/account, then run:\n" +
	"  civitai login --token <key>\n" +
	"OAuth login tokens (`civitai login`) can't read balance or spend Buzz."

func newBuzzCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "buzz",
		Short: "Show your spendable Buzz balance",
		Long: `Show your spendable Buzz balance (blue / green / yellow) using your stored
credential. Yellow Buzz is the currency real dev:live generation spends.

Reads buzz.getBuzzAccount with the same credential as ` + "`whoami`" + ` / ` + "`app status`" + `.
A full-scope personal API key can read your balance; an OAuth login token
(` + "`civitai login`" + `) cannot read balance or spend Buzz — in that case this prints
how to switch to a personal key.`,
		Example: `  civitai buzz
  civitai buzz --json   # raw JSON (scriptable)`,
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
			acct, err := client.GetBuzzAccount(context.Background())
			if err != nil {
				if errors.Is(err, api.ErrBuzzScope) {
					// Actionable guidance + a non-zero exit (a 403 is a real,
					// fixable failure, not "balance is zero").
					fmt.Fprintln(cmd.ErrOrStderr(), buzzScopeHint)
					return fmt.Errorf("credential can't read Buzz balance")
				}
				return err
			}

			out := cmd.OutOrStdout()

			if jsonOut {
				return writeJSON(out, map[string]any{
					"yellow": acct.Yellow,
					"blue":   acct.Blue,
					"green":  acct.Green,
				})
			}

			fmt.Fprintln(out, "Spendable Buzz:")
			fmt.Fprintf(out, "  Yellow: %d  (the generation-spend currency)\n", acct.Yellow)
			fmt.Fprintf(out, "  Blue:   %d\n", acct.Blue)
			fmt.Fprintf(out, "  Green:  %d\n", acct.Green)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit raw JSON (scriptable)")
	return cmd
}
