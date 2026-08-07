package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// buzzScopeHint is the actionable message shown when the stored credential
// cannot read the Buzz balance (a 403 from buzz.getBuzzAccount, typical of a
// DEFAULT OAuth login token, whose scope set omits BuzzRead). It names BOTH
// working paths: a full-scope personal API key, or a browser login that opted
// into the generate set (which carries BuzzRead alongside AI Services).
const buzzScopeHint = "This credential can't read your Buzz balance (needs the BuzzRead scope).\n" +
	"Either create a full-scope personal API key at https://civitai.com/user/account, then run:\n" +
	"  civitai login --token <key>\n" +
	"or re-login opting into the generate scope set (which also grants Buzz spend):\n" +
	"  civitai login --scopes generate\n" +
	"A DEFAULT OAuth login (`civitai login`, no --scopes) can read neither balance nor spend."

func newBuzzCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "buzz",
		Short: "Show your spendable Buzz balance",
		Long: `Show your spendable Buzz balance (blue / green / yellow, plus a total) using
your stored credential.

Reads buzz.getBuzzAccount with the same credential as ` + "`whoami`" + ` / ` + "`app status`" + `.
A full-scope personal API key can read your balance, as can a browser login that
opted into the generate scope set (` + "`civitai login --scopes generate`" + `). A
DEFAULT OAuth login (` + "`civitai login`" + `) can read neither balance nor spend Buzz —
in that case this prints both ways to fix it.`,
		Example: `  civitai buzz
  civitai buzz --json   # raw JSON (scriptable)`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Token() == "" {
				return civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf("no token configured — run `civitai login` (or set CIVITAI_TOKEN)"))
			}

			client := appapi.NewWithSource(cfg.BaseURL(), auth.New(cfg), "")
			acct, err := client.GetBuzzAccount(context.Background())
			if err != nil {
				if errors.Is(err, appapi.ErrBuzzScope) {
					// Actionable guidance + a non-zero exit (a 403 is a real,
					// fixable failure, not "balance is zero").
					errw := cmd.ErrOrStderr()
					fmt.Fprintln(errw, ui.For(errw).Warn(buzzScopeHint))
					// Re-tag so exitCode maps the lost-scope case to the auth exit
					// code (3), not the generic fallback — the message is unchanged.
					return civitai.Tag(appapi.ErrBuzzScope, fmt.Errorf("credential can't read Buzz balance"))
				}
				return err
			}

			out := cmd.OutOrStdout()

			if jsonOut {
				return writeJSON(out, map[string]any{
					"blue":   acct.Blue,
					"green":  acct.Green,
					"yellow": acct.Yellow,
					"total":  acct.Total(),
				})
			}

			fmt.Fprintln(out, ui.Bold("Buzz balance:"))
			fmt.Fprintf(out, "  Blue:   %d\n", acct.Blue)
			fmt.Fprintf(out, "  Green:  %d\n", acct.Green)
			fmt.Fprintf(out, "  Yellow: %d\n", acct.Yellow)
			fmt.Fprintf(out, "  Total:  %d\n", acct.Total())
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit raw JSON (scriptable)")
	return cmd
}
