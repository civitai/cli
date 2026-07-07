package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/ui"
	"github.com/spf13/cobra"
)

func newWhoAmICmd() *cobra.Command {
	var jsonOut bool
	var showScopes bool

	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Verify your stored API token and its capabilities",
		Long: `Verify the stored API token by calling the Civitai API and printing the
authenticated user PLUS a short capability summary: the credential type (OAuth
login vs personal API key), whether it can read your Buzz balance, and whether
it can spend Buzz. The money-path dead end — an OAuth ` + "`civitai login`" + ` token
can submit/withdraw but cannot spend Buzz — is surfaced here, before dev:live.

Reads the token from config or CIVITAI_TOKEN.`,
		Example: `  civitai whoami
  civitai whoami --scopes   # also list every granted scope
  civitai whoami --json     # raw JSON (scriptable)`,
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
				payload := map[string]any{
					"username":       id.Username,
					"id":             id.ID,
					"base_url":       cfg.BaseURL(),
					"credentialType": id.CredentialType(),
					"scopesKnown":    id.ScopeKnown(),
					"canReadBalance": id.CanReadBuzz(),
					"canSpend":       id.CanSpendBuzz(),
					"scopes":         id.DecodeScopes(),
					// Back-compat: the nested capabilities object earlier releases emit.
					"capabilities": map[string]bool{
						"can_spend_buzz": id.CanSpendBuzz(),
						"can_read_buzz":  id.CanReadBuzz(),
					},
				}
				return writeJSON(out, payload)
			}

			// Preserve this first line verbatim (scripts + tests depend on it).
			fmt.Fprintf(out, "Logged in as %s (id %d) at %s\n", ui.Bold(id.Username), id.ID, cfg.BaseURL())

			// A short capability summary so the money-path dead end (an OAuth
			// login can't spend) is visible BEFORE a dev:live run.
			fmt.Fprintln(out, "\n"+ui.Bold("Capabilities:"))
			fmt.Fprintf(out, "  Credential type:          %s\n", id.CredentialType())
			if !id.ScopeKnown() {
				fmt.Fprintln(out, "  "+ui.Dim("(token scope not reported by the server — capabilities unknown)"))
				return nil
			}
			fmt.Fprintf(out, "  Read Buzz balance:        %s\n", yesNo(id.CanReadBuzz()))
			fmt.Fprintf(out, "  Spend Buzz (AI Services): %s\n", yesNo(id.CanSpendBuzz()))

			if showScopes {
				fmt.Fprintf(out, "\nScopes (%d): %s\n", len(id.DecodeScopes()), strings.Join(id.DecodeScopes(), ", "))
			}

			// The #34 dead end, made visible before dev:live: an OAuth login (or
			// any credential without the AI-Services scope) can't spend.
			if !id.CanSpendBuzz() {
				fmt.Fprintln(out, "\n"+ui.Warn("This credential can't spend Buzz — money-path `dev:live` generation needs a"))
				fmt.Fprintln(out, "full-scope personal API key: create one at https://civitai.com/user/account,")
				fmt.Fprintln(out, "then `civitai login --token <key>`. (OAuth login can submit/withdraw but not spend.)")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit raw JSON (scriptable)")
	cmd.Flags().BoolVar(&showScopes, "scopes", false, "also print the full decoded scope list")
	return cmd
}

// yesNo renders a capability bit as "yes"/"no".
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
