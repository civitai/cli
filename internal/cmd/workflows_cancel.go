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
		Short: "Cancel a running generation workflow (you are billed for what it delivered)",
		Long: `Cancel a generation workflow that is still running.

🔴 CANCELLING IS NOT A CLEAN REFUND — AND IT IS NOT A TOTAL LOSS EITHER. Buzz is
charged UP FRONT, when the orchestrator schedules the run. Cancelling stops the
steps that have not finished; the orchestrator then RE-PRICES the workflow
against the work it actually did, and settles the difference against what it
took. What the run already delivered is billed: a step that had already finished
keeps its full cost, and a job a worker has already started and cannot interrupt
runs to completion and is billed for.

So cancel a job because you no longer want its OUTPUT. How much of the charge
that leaves you paying depends on how far the run had got, which you do not
control and cannot see from here — the settlement is server-side, it happens once
the workflow reaches its final state rather than when this command returns, and
` + buzzLedgerUnknownNote + `.

That is also why ` + "`civitai generate --timeout`" + ` and Ctrl-C do not cancel anything:
they stop the WAIT, not the job. The run keeps going server-side and its outputs
stay yours — pick them up later with ` + "`civitai workflows get <workflow-id>`" + `.

Cancelling an already-finished workflow is harmless — it is a no-op server-side,
and the outputs of a succeeded workflow are not deleted by it (use the website to
delete results).

This needs the same AI Services scopes that ` + "`civitai generate`" + ` needs:
` + spendCredentialRoutes + `.

CONFIRMATION: cancelling is IRREVERSIBLE — it throws away whatever the run had
not produced yet — so an interactive run asks first. Pass ` + "`--yes`" + ` to skip the
prompt in a script; a non-interactive shell without ` + "`--yes`" + ` REFUSES rather than
cancelling silently.`,
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
					"no token configured — cancelling a workflow needs a credential with the AI Services scopes: "+
						spendCredentialRoutes+". Or set CIVITAI_TOKEN"))
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
// irreversible AND throws away work on a job that has ALREADY been charged for.
// A bare `civitai workflows cancel <id>` on the wrong id used to destroy that job
// with no prompt at all.
//
// 🔴 "ALREADY BEEN PAID FOR" IS RETRACTED (#307) and must not come back: the
// orchestrator re-prices a cancelled workflow to the work it actually did, so
// what a cancel destroys is the OUTPUT, not necessarily the money. See
// runWorkflowsCancel's comment for the traced mechanism.
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
		return fmt.Errorf("refusing to cancel without --yes in a non-interactive shell — cancelling %s is irreversible: it throws away whatever the run has not produced yet, and you are still billed for what it did produce. "+
			"Pass --yes to confirm, or `civitai workflows get %s` to see what it has produced first",
			safeTerm(workflowID), safeTerm(workflowID))
	}

	errw := cmd.ErrOrStderr()
	st := ui.For(errw)
	fmt.Fprintf(errw, "About to cancel workflow %s.\n", safeTerm(workflowID))
	fmt.Fprintln(errw, st.Warn("You are billed for what this run has already delivered; the server re-prices the rest."))
	fmt.Fprintln(errw, st.Dim("What the ledger ends up doing is decided server-side — "+buzzLedgerUnknownNote+"."))
	fmt.Fprintln(errw, st.Dim("Outputs it has not produced yet are lost. It cannot be un-cancelled."))
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
//
// 🔴 WHAT A CANCEL DOES TO THE CHARGE — TRACED, NOT ASSUMED (#307). Until the
// orchestrator source was read this file asserted "a mid-run cancel BILLS THE
// ACCRUED COST … stopping it does not call that back". That is FALSE, and it is
// false in the direction that costs the user: it tells them the money is gone
// when part of it is not. The chain, in `civitai/civitai-orchestration`:
//
//   - `orchestrator.cancelWorkflow` (this file's seam) resolves to
//     `updateWorkflow({status:'canceled'})`, which the orchestrator's
//     v2 Consumers `WorkflowsController.UpdateAsync` routes to
//     `IWorkflowGrain.CancelAsync`.
//   - `WorkflowManager.CancelAsync` cancels only NON-FINAL steps; a step that
//     already finished is skipped and keeps its full cost.
//   - each cancelled job raises `JobEventType.Canceled`, and
//     `WorkflowStepManager` recomputes the step cost on every final non-success
//     job event. `CalculateCostAsync` then subtracts, per failed/cancelled job,
//     `job.Cost * undeliveredFraction`, where the fraction is
//     (expected blobs - delivered blobs) / expected blobs. A job that delivered
//     nothing subtracts in full; a job with no known blobs also subtracts in
//     full. Fixed costs, tips and licence fees are prorated the same way.
//   - when the workflow reaches a final status, `WorkflowGrain`'s workflow-event
//     observer calls `EnsureCorrectBuzzChargedAsync(true)`, which refunds
//     `charged - recomputed total` through `BuzzClient.RefundBuzzAsync`.
//
// So the honest statement is RE-PRICING, not "no refund" and not "you get it
// back": delivered work is billed, undelivered work comes off, and the
// difference settles server-side AFTER this call returns. Two conditions the
// copy must not flatten away — a post-billing step handler (CustomComfy)
// re-prices from measured runtime instead of blobs, and a job a worker has
// already claimed that is not claim-cancellable runs to completion and bills.
// The CLI still cannot observe the ledger, which is why buzzLedgerUnknownNote
// stays on every one of these surfaces.
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
		"You are billed for what it already delivered; the server re-prices the rest once the workflow settles."))
	fmt.Fprintln(errw, ui.For(errw).Dim("What the ledger ends up doing is decided server-side — "+buzzLedgerUnknownNote+"."))
	fmt.Fprintln(errw, ui.For(errw).Dim(fmt.Sprintf(
		"Check what it produced before stopping with `civitai workflows get %s`.", safeTerm(id))))
	return nil
}
