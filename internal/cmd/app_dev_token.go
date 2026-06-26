package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/manifest"
	"github.com/spf13/cobra"
)

// budgetedScope is the spend scope a dev token must carry for `npm run dev:live`
// to actually generate (spend Buzz). The server strips it from an OAuth-minted
// token (the civitai-cli OAuth client lacks AI Services), so an OAuth login
// yields a read-only token whose Generate button silently dead-ends in the live
// harness. We decode the minted JWT to detect that case and warn loudly.
const budgetedScope = "ai:write:budgeted"

// tokenCanSpend reports whether the minted dev-token JWT carries the budgeted
// spend scope. It decodes the JWT payload segment WITHOUT verifying the
// signature (that's the server's job on every API call) — we only need the
// claims to give an early, actionable warning. Any malformed input returns
// false conservatively (the worst case is an extra warning, never a missed
// spend). Mirrors the SDK live host's decodeBlockTokenPayload.
func tokenCanSpend(jwt string) bool {
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var claims struct {
		Scopes []string `json:"scopes"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return false
	}
	for _, s := range claims.Scopes {
		if s == budgetedScope {
			return true
		}
	}
	return false
}

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

Pre-GA the mint route is moderator-only. You do NOT need to submit the app
first — for a brand-new slug with no app row yet, the token is minted from the
scopes in your local block.manifest.json (clamped server-side), so
"create → dev-token → dev:live" works directly. The token is short-lived —
never commit it; re-mint when it expires.`,
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

			// Read the LOCAL manifest scopes (current working directory — the
			// user runs this from the scaffolded project dir) so the server can
			// mint a token for a slug with no app row yet (no submit needed).
			// Degrade gracefully: a missing/malformed manifest sends no scopes
			// (read-only token), and a registered app's server-side scopes still
			// govern when none are sent.
			scopes := manifest.LoadScopes(".")

			client := api.NewWithSource(cfg.BaseURL(), auth.New(cfg), "")
			token, err := client.MintDevToken(context.Background(), slug, scopes)
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
			errOut := cmd.ErrOrStderr()
			fmt.Fprintln(errOut,
				"Paste into VITE_LIVE_BLOCK_TOKEN in .env.development.local, then restart `npm run dev:live`. "+
					"Short-lived (~15min); never commit it.")

			// Catch the #1 dev:live dead-end EARLY: a token minted from an OAuth
			// login carries no spend scope, so `dev:live` Generate silently does
			// nothing (the live host can't grant consent for a scope the token
			// lacks). Warn at the source — the mint — not after a wasted click.
			if !tokenCanSpend(token) {
				fmt.Fprintln(errOut,
					"\n⚠  This token is READ-ONLY — it has no `ai:write:budgeted` scope, so "+
						"`npm run dev:live` will NOT spend Buzz or generate (Generate dead-ends silently).\n"+
						"   You're authenticated with an OAuth login. Real generation needs a full-scope personal API key:\n"+
						"     civitai login --token <key>      # create one at https://civitai.com/user/account\n"+
						"     civitai app dev-token "+slug+" --env >> .env.development.local   # re-mint, then restart dev:live")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&envOut, "env", false, "print VITE_LIVE_BLOCK_TOKEN=<token> (paste-ready into .env.development.local)")
	return cmd
}
