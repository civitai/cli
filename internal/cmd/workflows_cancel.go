package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/genapi"
	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// workflowsCancelOpts is the parsed `workflows cancel` invocation.
type workflowsCancelOpts struct {
	jsonOut bool
	baseURL string
}

// workflowsCancelDeps is the single network seam.
//
// 🔴 cancelWorkflow is a MUTATION. A test must never point this at a real
// server; every test in this repo drives it through httptest.
type workflowsCancelDeps struct {
	cancelWorkflow func(ctx context.Context, workflowID string) (json.RawMessage, error)
}

func newWorkflowsCancelCmd() *cobra.Command {
	var o workflowsCancelOpts
	cmd := &cobra.Command{
		Use:   "cancel <workflow-id>",
		Short: "Cancel a running generation workflow (DOES NOT REFUND)",
		Long: `Cancel a generation workflow that is still running.

🔴 CANCELLING DOES NOT GET YOUR BUZZ BACK. A mid-run cancel BILLS THE ACCRUED
COST, orchestrator-side and non-refundably. There is no cancel-for-refund
anywhere on this platform: by the time a workflow is running, the money has
moved. Cancel a job because you no longer want its OUTPUT — never as a way to
save money, and never as a way to undo a submit you regret.

That is also why ` + "`civitai generate --timeout`" + ` and Ctrl-C do not cancel anything:
stopping the wait costs nothing, while stopping the job would cost the same as
letting it finish and would throw away the result you already paid for.

Cancelling an already-finished workflow is harmless — the outputs of a succeeded
workflow are not deleted by it (use the website to delete results).

This needs the same personal API key with the AI Services scopes that
` + "`civitai generate`" + ` needs.`,
		Example: `  civitai workflows cancel 01JABCXYZ
  civitai workflows cancel 01JABCXYZ --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Token() == "" {
				return civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf(
					"no token configured — cancelling a workflow needs a personal API key: run `civitai login --token <key>` (or set CIVITAI_TOKEN)"))
			}
			o.baseURL = cfg.BaseURL()
			gen := genapi.NewWithSource(cfg.BaseURL(), auth.New(cfg))
			return runWorkflowsCancel(cmd, workflowsCancelDeps{cancelWorkflow: gen.CancelWorkflow}, o, args[0])
		},
	}
	cmd.Flags().BoolVar(&o.jsonOut, "json", false, "emit the raw server reply on stdout (scriptable)")
	return cmd
}

// runWorkflowsCancel is the testable core.
func runWorkflowsCancel(cmd *cobra.Command, deps workflowsCancelDeps, o workflowsCancelOpts, workflowID string) error {
	id := strings.TrimSpace(workflowID)
	if id == "" {
		return asUsageError(fmt.Errorf("a workflow id is required: civitai workflows cancel <workflow-id>"))
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	raw, err := deps.cancelWorkflow(ctx, id)
	if err != nil {
		return classifyGenerateError(err)
	}
	if o.jsonOut {
		if len(raw) == 0 {
			// The procedure returns nothing on success, so there is genuinely no
			// payload. Emit a valid document a script can parse rather than an
			// empty stdout, which `jq` would treat as a hard error.
			raw = json.RawMessage(`{}`)
		}
		return writeRawJSON(cmd.OutOrStdout(), raw)
	}
	out, errw := cmd.OutOrStdout(), cmd.ErrOrStderr()
	fmt.Fprintf(out, "Cancelled workflow %s\n", safeTerm(id))
	// 🔴 Repeated at the point of use, not just in --help: the one place a user
	// is guaranteed to read is the line printed after the thing happened.
	fmt.Fprintln(errw, ui.For(errw).Warn(
		"This did NOT refund anything — a mid-run cancel bills the accrued cost, non-refundably."))
	fmt.Fprintln(errw, ui.For(errw).Dim(fmt.Sprintf(
		"Check what it produced before stopping with `civitai workflows get %s`.", safeTerm(id))))
	return nil
}
