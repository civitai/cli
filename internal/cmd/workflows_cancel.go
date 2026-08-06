package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
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
	// assumeYes is --yes/-y: skip the confirmation prompt (for scripts/CI).
	assumeYes bool
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
` + "`civitai generate`" + ` needs.

CONFIRMATION: cancelling is IRREVERSIBLE and destroys a job you have already paid
for, so an interactive run asks first. Pass ` + "`--yes`" + ` to skip the prompt in a
script; a non-interactive shell without ` + "`--yes`" + ` REFUSES rather than cancelling
silently.`,
		Example: `  civitai workflows cancel 01JABCXYZ
  civitai workflows cancel 01JABCXYZ --yes
  civitai workflows cancel 01JABCXYZ --json --yes`,
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
	cmd.Flags().BoolVarP(&o.assumeYes, "yes", "y", false, "skip the confirmation prompt and cancel (for scripts/CI)")
	return cmd
}

// confirmCancel gates the cancel mutation, mirroring confirmSubmit and
// confirmGenerate.
//
// Every other destructive path in this feature gates, and this one has the
// strongest case of the three: `app submit` gates a REVERSIBLE action, and
// `generate` gates a spend the user has not made yet, whereas a cancel is
// irreversible AND throws away a job that has ALREADY been paid for. A bare
// `civitai workflows cancel <id>` on the wrong id used to destroy that job with
// no prompt at all.
//
//   - --yes/-y              → proceed without prompting.
//   - non-TTY without --yes → REFUSE (never hang, never destroy silently).
//   - interactive TTY       → say what is lost, prompt, proceed only on "y".
//
// Everything it prints goes to STDERR so a `--json` stdout stays machine-clean.
//
// 🔴 The prompt DEFAULTS TO NO: the switch enumerates the accepting answers and
// everything else — including a bare Enter — cancels. Rewriting it to enumerate
// the refusing answers instead would turn `[y/N]` into `[Y/n]` and destroy a
// paid-for job on a stray keystroke.
func confirmCancel(cmd *cobra.Command, workflowID string, assumeYes bool) error {
	if assumeYes {
		return nil
	}
	if !stdinIsTTY() {
		return fmt.Errorf("refusing to cancel without --yes in a non-interactive shell — cancelling %s is irreversible and does NOT refund the Buzz already spent on it. "+
			"Pass --yes to confirm, or `civitai workflows get %s` to see what it has produced first",
			safeTerm(workflowID), safeTerm(workflowID))
	}

	errw := cmd.ErrOrStderr()
	st := ui.For(errw)
	fmt.Fprintf(errw, "About to cancel workflow %s.\n", safeTerm(workflowID))
	fmt.Fprintln(errw, st.Warn("This does NOT refund anything — a mid-run cancel bills the accrued cost, non-refundably."))
	fmt.Fprintln(errw, st.Dim("You are throwing away a job you have already paid for. It cannot be un-cancelled."))
	fmt.Fprint(errw, "Cancel it? [y/N]: ")

	r := bufio.NewReader(cmd.InOrStdin())
	line, _ := r.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	default:
		return errors.New("cancel aborted")
	}
}

// runWorkflowsCancel is the testable core.
func runWorkflowsCancel(cmd *cobra.Command, deps workflowsCancelDeps, o workflowsCancelOpts, workflowID string) error {
	id := strings.TrimSpace(workflowID)
	if id == "" {
		return asUsageError(fmt.Errorf("a workflow id is required: civitai workflows cancel <workflow-id>"))
	}
	// 🔴 The gate runs BEFORE the mutation seam is touched, so a refusal cannot
	// have cancelled anything.
	if err := confirmCancel(cmd, id, o.assumeYes); err != nil {
		return err
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
