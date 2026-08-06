package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/genapi"
	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// newWorkflowsCmd is the orchestrator-workflow command GROUP.
//
// It is a group and not a runnable command on purpose: root.go's
// enforceUsageExitCodes only installs the unknown-subcommand guard on parents
// that define no Run/RunE of their own, so keeping this non-runnable is what
// makes `civitai workflows frobnicate` a usage error (exit 2) instead of a
// silent exit 0. It is also why `generate` is deliberately NOT a parent —
// see root.go.
//
// The naming is deliberate too: these are orchestrator WORKFLOWS, not
// "generations".
func newWorkflowsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflows",
		Short: "List, inspect and cancel generation workflows",
		Long: `Work with the generation workflows your account has submitted.

A workflow is one submitted generation job. ` + "`civitai generate`" + ` prints a
workflow id; ` + "`list`" + ` and ` + "`get`" + ` are how you find one afterwards — which is what
makes ` + "`--no-wait`" + `, a --timeout expiry and a Ctrl-C recoverable rather than a
dead end.

` + "`list`" + ` and ` + "`get`" + ` are reads and SPEND NOTHING. 🔴 ` + "`cancel`" + ` stops a job but does
NOT refund it — a mid-run cancel bills the accrued cost, non-refundably.`,
		Example: `  civitai workflows list
  civitai workflows get 01JABCXYZ
  civitai workflows get 01JABCXYZ --json
  civitai workflows cancel 01JABCXYZ`,
	}
	cmd.AddCommand(newWorkflowsListCmd())
	cmd.AddCommand(newWorkflowsGetCmd())
	cmd.AddCommand(newWorkflowsCancelCmd())
	return cmd
}

// workflowsGetOpts is the parsed `workflows get` invocation.
type workflowsGetOpts struct {
	jsonOut bool
	baseURL string
}

// workflowsGetDeps is the single network seam, injected so the renderer and
// every error path are testable against httptest with no live server.
type workflowsGetDeps struct {
	getWorkflow getWorkflowFn
}

func newWorkflowsGetCmd() *cobra.Command {
	var o workflowsGetOpts
	cmd := &cobra.Command{
		Use:   "get <workflow-id>",
		Short: "Show one generation workflow and its outputs",
		Long: `Show one generation workflow: its status, its steps, and its outputs.

Use it to re-attach to a job you did not wait for — after ` + "`--no-wait`" + `, after a
--timeout expiry, or after Ctrl-C. The workflow id is printed by
` + "`civitai generate`" + ` in all three cases.

OUTPUT URLS ARE PRESIGNED AND EXPIRE. The links this prints are short-lived;
fetch them promptly or re-run this command for fresh ones.

Outputs that are blocked by moderation, not available, or hidden are listed with
the reason rather than omitted — a finished workflow can legitimately contain
fewer usable results than it was charged for, and silently dropping them would
make that invisible.

Reading a workflow SPENDS NOTHING. This needs the same personal API key with the
AI Services scopes that ` + "`civitai generate`" + ` needs.`,
		Example: `  civitai workflows get 01JABCXYZ
  civitai workflows get 01JABCXYZ --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Token() == "" {
				return civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf(
					"no token configured — reading a workflow needs a personal API key: run `civitai login --token <key>` (or set CIVITAI_TOKEN)"))
			}
			o.baseURL = cfg.BaseURL()
			gen := genapi.NewWithSource(cfg.BaseURL(), auth.New(cfg))
			return runWorkflowsGet(cmd, workflowsGetDeps{getWorkflow: gen.GetWorkflow}, o, args[0])
		},
	}
	cmd.Flags().BoolVar(&o.jsonOut, "json", false, "emit the raw server payload on stdout (scriptable)")
	return cmd
}

// runWorkflowsGet is the testable core.
func runWorkflowsGet(cmd *cobra.Command, deps workflowsGetDeps, o workflowsGetOpts, workflowID string) error {
	id := strings.TrimSpace(workflowID)
	if id == "" {
		return asUsageError(fmt.Errorf("a workflow id is required: civitai workflows get <workflow-id>"))
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	wf, raw, err := deps.getWorkflow(ctx, id)
	if err != nil {
		return classifyGenerateError(err)
	}
	if o.jsonOut {
		// Raw passthrough: a script sees every server field, including ones the
		// CLI does not model. It must branch on the payload itself.
		return writeRawJSON(cmd.OutOrStdout(), raw)
	}
	printWorkflow(cmd.OutOrStdout(), cmd.ErrOrStderr(), wf)
	return nil
}

// printWorkflow renders one workflow for humans. Every server string goes
// through safeTerm — workflow ids, statuses and moderation reasons are all
// server-origin text.
func printWorkflow(out, errw io.Writer, wf *genapi.Workflow) {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Workflow ID:\t%s\n", safeTerm(wf.ID))
	fmt.Fprintf(tw, "Status:\t%s\n", safeTerm(dashIfEmpty(wf.Status)))
	fmt.Fprintf(tw, "Created:\t%s\n", safeTerm(dashIfEmpty(wf.CreatedAt)))
	if wf.StartedAt != nil {
		fmt.Fprintf(tw, "Started:\t%s\n", safeTerm(dashIfEmpty(*wf.StartedAt)))
	}
	if wf.CompletedAt != nil {
		fmt.Fprintf(tw, "Completed:\t%s\n", safeTerm(dashIfEmpty(*wf.CompletedAt)))
	}
	_ = tw.Flush()

	// 🔴 THIS IS THE LAST CHANCE TO LEARN IT. `civitai generate --no-wait` prints
	// a workflow id and tells the user to collect the results here, so for that
	// documented flow the submit reply is long gone and this persisted record is
	// the ONLY surviving evidence that a different checkpoint was billed.
	reportModelSubstitutions(errw, wf.Substitutions(), substitutionOnRead)

	kept, excluded := genapi.PartitionOutputs(wf)
	fmt.Fprintf(out, "\n%s\n", ui.For(out).Bold(fmt.Sprintf("Outputs (%d deliverable, %d excluded)", len(kept), len(excluded))))
	if len(kept) == 0 && len(excluded) == 0 {
		if genapi.IsTerminalStatus(wf.Status) {
			fmt.Fprintln(out, "  (none)")
		} else {
			fmt.Fprintln(out, "  (none yet — the workflow has not finished)")
		}
	}
	for i, o := range kept {
		fmt.Fprintf(out, "\n  [%d] %s\n", i+1, safeTerm(dashIfEmpty(o.ID)))
		if o.Width != nil && o.Height != nil {
			fmt.Fprintf(out, "      size:    %dx%d\n", *o.Width, *o.Height)
		}
		if o.Seed != nil {
			fmt.Fprintf(out, "      seed:    %d\n", *o.Seed)
		}
		if o.NSFWLevel != nil {
			fmt.Fprintf(out, "      rating:  %s\n", safeTerm(*o.NSFWLevel))
		}
		if o.URLExpiresAt != nil {
			fmt.Fprintf(out, "      expires: %s\n", safeTerm(*o.URLExpiresAt))
		}
		if o.URL != nil {
			fmt.Fprintf(out, "      url:     %s\n", safeTerm(*o.URL))
		}
	}
	reportExcludedOutputs(errw, excluded)

	if !genapi.IsTerminalStatus(wf.Status) {
		fmt.Fprintln(errw, ui.For(errw).Dim(
			"This workflow has not finished. Re-run this command to check again — it is a read and spends nothing."))
	} else if len(kept) > 0 {
		fmt.Fprintln(errw, ui.For(errw).Dim(
			"Output URLs are presigned and expire — fetch them promptly, or re-run this command for fresh links."))
	}
}
