package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/pkg/civitai"
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
  civitai whoami --json     # a stable, curated identity object (scriptable)`,
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
			id, err := client.WhoAmI(context.Background())
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			// 🔴 ONE PREDICATE, ONE PLACE. Both surfaces below read this single
			// tri-state answer; neither re-derives "can this credential submit?"
			// from the subject type or the mask. nil means UNKNOWABLE — see
			// appapi.Identity.CanSubmitApps, which states the three states and
			// cites AGENTS item 9 for the discriminator rationale.
			canSubmit := id.CanSubmitApps()

			if jsonOut {
				payload := map[string]any{
					"username":       id.Username,
					"id":             id.ID,
					"base_url":       cfg.BaseURL(),
					"credentialType": id.CredentialType(),
					"scopesKnown":    id.ScopeKnown(),
					"canReadBalance": id.CanReadBuzz(),
					"canSpend":       id.CanSpendBuzz(),
					// true / false / null — null means unknowable, never "no".
					"canSubmitApps": canSubmit,
					"scopes":        id.DecodeScopes(),
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
			// 🔴 THE SUBMIT ROW IS NOT GATED ON ScopeKnown(). A personal API key
			// is not scope-gated for submit at all, so its answer is KNOWN even
			// when the server reports no mask — the old early return withheld a
			// fact the command was holding, while `--json` published it. The row
			// prints in every branch; only its VALUE degrades, and it degrades to
			// "unknown", never to "no".
			submitRow := func() {
				fmt.Fprintf(out, "  Submit Apps:              %s\n", yesNoUnknown(canSubmit))
			}
			if !id.ScopeKnown() {
				submitRow()
				// Scoped to BUZZ deliberately: the submit row above may well be
				// known, so "capabilities unknown" would have been false.
				fmt.Fprintln(out, "  "+ui.Dim("(token scope not reported by the server — Buzz capabilities unknown)"))
				return nil
			}
			fmt.Fprintf(out, "  Read Buzz balance:        %s\n", yesNo(id.CanReadBuzz()))
			fmt.Fprintf(out, "  Spend Buzz (AI Services): %s\n", yesNo(id.CanSpendBuzz()))
			submitRow()

			if showScopes {
				scopes := id.DecodeScopes()
				list := strings.Join(scopes, ", ")
				if len(scopes) == 0 {
					// A known mask with no bits set. An empty trailing list read
					// as truncated output; say what it means.
					list = ui.Dim("(none granted)")
				}
				fmt.Fprintf(out, "\nScopes (%d): %s\n", len(scopes), list)
			}

			// The #34 dead end, made visible before dev:live: a credential without
			// the AI-Services scope can't spend. There are now TWO fixes — an OAuth
			// login can opt into generation at login time — so name the one that
			// matches the credential in hand rather than always pointing at the web
			// UI. (An OAuth login is re-runnable; a personal key's scopes are fixed
			// when it is minted, so that user must create a new key.)
			if !id.CanSpendBuzz() {
				fmt.Fprintln(out, "\n"+ui.Warn("This credential can't spend Buzz — `civitai generate` and money-path `dev:live`"))
				fmt.Fprintln(out, "generation both need the AI Services scope. To get it:")
				if cfg.AuthKind() == config.AuthKindOAuth {
					fmt.Fprintln(out, "  civitai login --scopes generate  # re-login, additively granting generation + Buzz spend")
					fmt.Fprintln(out, "  civitai login --token <key>      # or a full-scope personal API key: https://civitai.com/user/account")
				} else {
					fmt.Fprintln(out, "  civitai login --token <key>      # a FULL-SCOPE personal API key: create one at https://civitai.com/user/account")
					fmt.Fprintln(out, "  civitai login --scopes generate  # or a browser login that opts into generation")
				}
			}
			return nil
		},
	}
	// 🔴 NOT "raw JSON" (#377). The payload is a hand-built projection of ten
	// keys; the server demonstrably sends six more — `tier`, `status`,
	// `isMember`, `subscriptions`, `email`, `emailVerified` — that never appear
	// (see the real production capture in internal/appapi/api_test.go). Making
	// it actually raw is NOT the fix: `email`/`emailVerified` are PII this
	// command does not print today, so passing the body through would be a
	// privacy regression dressed as a bug fix. The words are what was wrong.
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit a stable, curated identity object (scriptable)")
	cmd.Flags().BoolVar(&showScopes, "scopes", false, "also print the full decoded scope list")
	return cmd
}

// yesNo renders a KNOWN capability bit as "yes"/"no".
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// yesNoUnknown renders a TRI-STATE capability as "yes"/"no"/"unknown". nil is
// the third state (see appapi.Identity.CanSubmitApps) and must never render as
// "no" — that is the false-negative-stated-as-fact this exists to prevent.
func yesNoUnknown(b *bool) string {
	if b == nil {
		return ui.Dim("unknown")
	}
	return yesNo(*b)
}
