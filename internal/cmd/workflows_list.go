package cmd

import (
	"context"
	"encoding/json"
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

// workflowsListOpts is the parsed `workflows list` invocation.
type workflowsListOpts struct {
	limit    int
	limitSet bool
	cursor   string
	tags     []string
	jsonOut  bool
	baseURL  string
}

// workflowsListDeps is the single network seam.
type workflowsListDeps struct {
	queryWorkflows func(ctx context.Context, opts genapi.ListOptions) (*genapi.WorkflowPage, json.RawMessage, error)
}

func newWorkflowsListCmd() *cobra.Command {
	var o workflowsListOpts
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the generation workflows you have submitted",
		Long: `List your own generation workflows, newest first.

This is the feed behind the website's generator queue: one entry per submitted
job, with its status, when it was created, what it cost and how many outputs it
produced.

PAGING is by cursor, not page number. --limit sets the page size; when more
results exist the command prints the next cursor, which you pass back as
--cursor to fetch the following page. Deep pages are not cached server-side, so
walk them at a civil pace.

Each row reports outputs as "<deliverable>/<total>". They differ when an output
was blocked by moderation, never landed, or was hidden on the website — a
workflow you were charged for can legitimately have fewer usable results than it
produced, and collapsing the two numbers would hide that. Use
` + "`civitai workflows get <id>`" + ` for the per-output reasons and the URLs.

Reading SPENDS NOTHING. It needs the same AI Services scopes that
` + "`civitai generate`" + ` needs: ` + spendCredentialRoutes + `.`,
		Example: `  civitai workflows list
  civitai workflows list --limit 5
  civitai workflows list --limit 50 --cursor <next-cursor>
  civitai workflows list --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			o.limitSet = cmd.Flags().Changed("limit")
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Token() == "" {
				return civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf(
					"no token configured — listing workflows needs a credential with the AI Services scopes: "+
						spendCredentialRoutes+". Or set CIVITAI_TOKEN"))
			}
			o.baseURL = cfg.BaseURL()
			gen := genapi.NewWithSource(cfg.BaseURL(), auth.New(cfg))
			return runWorkflowsList(cmd, workflowsListDeps{queryWorkflows: gen.QueryWorkflows}, o)
		},
	}
	cmd.Flags().IntVar(&o.limit, "limit", 0, "how many workflows to fetch in this page (server default when unset)")
	cmd.Flags().StringVar(&o.cursor, "cursor", "", "opaque cursor from a previous page's next-cursor line")
	cmd.Flags().StringArrayVar(&o.tags, "tag", nil, "only list workflows carrying this orchestrator tag. Repeatable")
	cmd.Flags().BoolVar(&o.jsonOut, "json", false, "emit the raw server payload on stdout (scriptable)")
	return cmd
}

// runWorkflowsList is the testable core.
func runWorkflowsList(cmd *cobra.Command, deps workflowsListDeps, o workflowsListOpts) error {
	if o.limitSet && o.limit < 1 {
		return asUsageError(fmt.Errorf("--limit must be at least 1, got %d", o.limit))
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	page, raw, err := deps.queryWorkflows(ctx, genapi.ListOptions{
		Take:   o.limit,
		Cursor: o.cursor,
		Tags:   o.tags,
	})
	if err != nil {
		return classifyGenerateError(err)
	}
	if o.jsonOut {
		// Raw passthrough: a script sees every server field, including the ones
		// this CLI does not model, and reads nextCursor itself.
		return writeRawJSON(cmd.OutOrStdout(), raw)
	}
	printWorkflowList(cmd.OutOrStdout(), cmd.ErrOrStderr(), page, o)
	return nil
}

// printWorkflowList renders the human table. Every server string goes through
// safeTerm — ids, statuses and tags are all server-origin text.
func printWorkflowList(out, errw io.Writer, page *genapi.WorkflowPage, o workflowsListOpts) {
	if page == nil || len(page.Items) == 0 {
		// An empty feed is a legitimate answer, not an error — but say WHICH
		// question came back empty, so a mis-typed --tag or an exhausted cursor
		// does not read as "you have never generated anything".
		switch {
		case o.cursor != "":
			fmt.Fprintln(out, "No more workflows on this page.")
		case len(o.tags) > 0:
			fmt.Fprintf(out, "No workflows tagged %s.\n", strings.Join(o.tags, ", "))
		default:
			fmt.Fprintln(out, "No workflows yet.")
			fmt.Fprintln(errw, ui.For(errw).Dim("Submit one with `civitai generate \"a cat wearing sunglasses\"`."))
		}
		return
	}

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "WORKFLOW ID\tSTATUS\tCREATED\tCOST\tOUTPUTS")
	for _, w := range page.Items {
		total, deliverable := w.OutputCounts()
		cost := "-"
		if w.Cost != nil {
			cost = buzzAmount(w.Cost.Total)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d/%d\n",
			safeTerm(dashIfEmpty(w.ID)),
			safeTerm(dashIfEmpty(w.Status)),
			safeTerm(dashIfEmpty(w.CreatedAt)),
			cost,
			deliverable, total)
	}
	_ = tw.Flush()

	st := ui.For(errw)
	if page.NextCursor != nil && *page.NextCursor != "" {
		// The cursor goes to STDOUT: it is data a pipeline consumes, and this
		// repo's convention keeps machine-usable values off stderr.
		fmt.Fprintf(out, "\nNext cursor: %s\n", safeTerm(*page.NextCursor))
		fmt.Fprintln(errw, st.Dim("More results — re-run with --cursor <next cursor> to continue."))
	}
	fmt.Fprintln(errw, st.Dim(
		"OUTPUTS is deliverable/total; they differ when an output was blocked, never landed, or you hid it. `civitai workflows get <id>` shows the reason and the URLs."))
}
