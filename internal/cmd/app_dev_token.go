package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/spf13/cobra"
)

func newAppDevTokenCmd() *cobra.Command {
	var envOut bool

	cmd := &cobra.Command{
		Use:   "dev-token <slug>",
		Short: "Mint a short-lived dev block token for `npm run dev:live`",
		Long: `Mint a short-lived dev block token so a scaffolded page-money app can run
"npm run dev:live" against the REAL Civitai backend.

Calls the moderator-gated mint route (POST /api/v1/blocks/dev-token) with your
stored credential and prints the token (a ~15-minute RS256 JWT). Paste it into
VITE_LIVE_BLOCK_TOKEN in .env.development.local, then restart "npm run dev:live".

The minted token's CAPABILITIES depend on the credential you mint with:
  - REAL generation (spends real Buzz) needs a FULL-SCOPE PERSONAL API KEY
    (create one at https://civitai.com/user/account — it carries AI Services).
    Confirm yours can spend with "civitai whoami".
  - An OAuth login ("civitai login") mints a READ/IDENTITY-ONLY dev token (no
    spend) — dev:live shows your viewer + catalog/storage, but estimate →
    submit → generation will NOT spend.

Pre-GA the mint route is moderator-only; a PENDING (un-approved) slug is
accepted. The token is short-lived — never commit it; re-mint when it expires.`,
		Example: `  civitai app dev-token my-block               # print the token to stdout
  civitai app dev-token my-block --env         # print VITE_LIVE_BLOCK_TOKEN=<token>
  civitai app dev-token my-block --env >> .env.development.local`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Token() == "" {
				return fmt.Errorf("no token configured — run `civitai login` (or set CIVITAI_TOKEN)")
			}

			var slug string
			if len(args) == 1 {
				slug = strings.TrimSpace(args[0])
			}
			if slug == "" {
				return fmt.Errorf("an app slug is required — e.g. `civitai app dev-token my-block` (find it with `civitai app status`)")
			}

			client := api.NewWithSource(cfg.BaseURL(), auth.New(cfg), "")
			token, err := client.MintDevToken(context.Background(), slug)
			if err != nil {
				return err
			}

			// Token to stdout (pipeable); the paste hint to stderr so it never
			// pollutes a `--env >> .env.development.local` redirect.
			out := cmd.OutOrStdout()
			if envOut {
				fmt.Fprintf(out, "VITE_LIVE_BLOCK_TOKEN=%s\n", token)
			} else {
				fmt.Fprintln(out, token)
			}
			fmt.Fprintln(cmd.ErrOrStderr(),
				"Paste into VITE_LIVE_BLOCK_TOKEN in .env.development.local, then restart `npm run dev:live`. "+
					"Short-lived (~15min); never commit it. Real spend needs a full-scope personal API key (`civitai whoami`).")
			return nil
		},
	}
	cmd.Flags().BoolVar(&envOut, "env", false, "print VITE_LIVE_BLOCK_TOKEN=<token> (paste-ready into .env.development.local)")
	return cmd
}
