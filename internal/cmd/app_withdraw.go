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

func newAppWithdrawCmd() *cobra.Command {
	var idFlag string
	var assumeYes bool

	cmd := &cobra.Command{
		Use:   "withdraw [pubreq-id]",
		Short: "Withdraw your own pending App submission",
		Long: `Withdraw your own pending App submission so you can resubmit a new bundle
for the same slug.

Calls the token-authenticated, self-scoped withdraw route
(POST /api/v1/blocks/withdraw) with your stored credential — you can only ever
withdraw your OWN submissions. Both a personal API key and an OAuth login
(civitai login) work; the OAuth token must carry the Apps submit scope
(the same gate the submit route uses).

Only a submission still in the 'pending' review state can be withdrawn; an
already-approved/rejected (or already-withdrawn) request cannot. Withdrawing is
idempotent — withdrawing an already-withdrawn request still succeeds.

Pass the publish-request id as a positional argument or via --id (find it with
"civitai app status").

Withdraw is non-interactive (it never prompts); --yes/-y is accepted as a no-op
for symmetry with "civitai app submit" so the same scripted flag works on both.`,
		Example: `  civitai app withdraw pubreq_01H        # withdraw by publish-request id
  civitai app withdraw --id pubreq_01H   # same, via the flag`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Token() == "" {
				return civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf("no token configured — run `civitai login` (or set CIVITAI_TOKEN)"))
			}

			// Both refusals below are about the INVOCATION (a mutually exclusive
			// pair, and a missing required id), so both are usage errors —
			// exit 2, like the bad-flag-name errors Cobra already tags.
			if idFlag != "" && len(args) == 1 {
				return asUsageError(fmt.Errorf("pass the publish-request id as an argument OR via --id, not both"))
			}
			id := idFlag
			if id == "" && len(args) == 1 {
				id = args[0]
			}
			id = strings.TrimSpace(id)
			if id == "" {
				return asUsageError(fmt.Errorf("a publish-request id is required — pass it as an argument or with --id (find it via `civitai app status`)"))
			}

			client := appapi.NewWithSource(cfg.BaseURL(), auth.New(cfg), "")
			ctx := context.Background()

			if err := client.WithdrawRequest(ctx, id); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), ui.Success(fmt.Sprintf("Withdrew %s", id)))
			return nil
		},
	}
	cmd.Flags().StringVar(&idFlag, "id", "", "the publish-request id to withdraw (pubreq_...)")
	// Accepted for symmetry with `app submit`; withdraw is non-interactive so this
	// is a documented no-op (nothing to skip).
	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "accepted for symmetry with 'app submit' (withdraw is non-interactive; no-op)")
	return cmd
}
