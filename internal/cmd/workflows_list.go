package cmd

import (
	"bytes"
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

// listReasonIndent is the pad EVERY line of a server-supplied reason carries
// under its table row — the first line and, via indentContinuation, each one
// after it. It is one constant because the two have to agree: a first line
// padded four columns and a continuation padded two would still be a block that
// starts where nothing else on the screen starts, but only the invariant
// "no reason line begins at column zero" makes row forgery impossible, and that
// is what the guards assert.
const listReasonIndent = "    "

// listReasonWrapWidth is the text budget one reason line gets, so that indent +
// text lands on findingWrapWidth — the same total the CLI's other long-prose
// surface wraps to. The CLI's prose surfaces should not disagree about how wide
// a terminal is.
const listReasonWrapWidth = findingWrapWidth - len(listReasonIndent)

// listReasonLegend explains the indented lines, and is printed only when at
// least one was. It says where the text came from and stops there: it does not
// say the workflow failed (the CLI has not established that the orchestrator
// records `errors` only on failure) and it says nothing about the charge
// (AGENTS.md item 28).
const listReasonLegend = "An indented line under a row is what the server recorded for that workflow, printed as it arrived."

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

WHERE THE SERVER RECORDED AN ACCOUNT of what happened to a workflow, it is
printed on indented lines under that workflow's row, in the server's own words
and in full. Not every workflow has one; nothing is printed for those.

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

	// 🔴 THE TABLE IS ALIGNED IN A BUFFER AND EMITTED AFTERWARDS, BECAUSE THE
	// REASON LINES CANNOT GO THROUGH THE TABWRITER. A tabwriter aligns a column
	// block, and a line carrying no tab ENDS that block: interleaving reason
	// lines directly would re-align every group of rows independently, so the
	// columns would jump around the failures. Writing every row into one block
	// first, then splicing the reason lines into the flushed text, keeps all rows
	// in a single alignment block — the same "render it outside the tabwriter"
	// answer printWorkflow reached, adapted to a body that has to interleave.
	var aligned bytes.Buffer
	tw := tabwriter.NewWriter(&aligned, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "WORKFLOW ID\tSTATUS\tCREATED\tCOST\tOUTPUTS")
	// rowLines[i] is how many OUTPUT lines item i's row occupies. It is almost
	// always 1; it is not 1 when a server-origin cell contains a newline, which
	// safeTerm deliberately preserves. tabwriter never adds or removes line
	// breaks — it only pads cells — so counting them on the way IN is what keeps
	// the splice below attached to the right row. Pairing by index instead would
	// hand one workflow's reason to another's row the moment an id contained a
	// newline, which is a worse failure than the ragged column that id already
	// produces today.
	rowLines := make([]int, len(page.Items))
	for i, w := range page.Items {
		total, deliverable := w.OutputCounts()
		cost := "-"
		if w.Cost != nil {
			cost = buzzAmount(w.Cost.Total)
		}
		row := fmt.Sprintf("%s\t%s\t%s\t%s\t%d/%d",
			safeTerm(dashIfEmpty(w.ID)),
			safeTerm(dashIfEmpty(w.Status)),
			safeTerm(dashIfEmpty(w.CreatedAt)),
			cost,
			deliverable, total)
		rowLines[i] = 1 + strings.Count(row, "\n")
		fmt.Fprintln(tw, row)
	}
	_ = tw.Flush()

	lines := strings.Split(strings.TrimSuffix(aligned.String(), "\n"), "\n")
	next := 0
	emit := func(n int) {
		for ; n > 0 && next < len(lines); n-- {
			fmt.Fprintln(out, lines[next])
			next++
		}
	}
	emit(1) // the header row
	reported := false
	for i, w := range page.Items {
		emit(rowLines[i])
		// 🔴 #382: THE ANSWER TO "WHY DID IT FAIL", WHICH THIS VIEW ALSO DISCARDED.
		// The listing endpoint hands the reason back on the SAME response that
		// renders `failed … 0/1`, at `steps[].errors` — a different path from the
		// one #367 fixed on `workflows get`, which is why that PR ruled this
		// surface out of scope. `ListedStep` had no field for it, so it was
		// dropped at unmarshal.
		//
		// 🔴 EVERY LINE IS INDENTED, AND THAT IS A SECURITY PROPERTY, NOT LAYOUT.
		// This is external-provider free text printed directly beneath a table of
		// rows, and safeTerm keeps `\n` on purpose, so a reason containing one
		// would otherwise put attacker-chosen text at column zero — where it is
		// indistinguishable from a row the CLI wrote, complete with a fake id,
		// status and cost. indentContinuation pads every line after the first;
		// the Fprintf pads the first; wrapServerText breaks a line long enough
		// that the TERMINAL would wrap it, which is the same vector with no
		// newline in it.
		//
		// 🔴 THE SCOPE OF THAT CLAIM, BECAUSE AN ABSOLUTE VERSION OF IT SHIPPED
		// AND WAS FALSE. It read "no continuation line can occupy column zero,
		// and every real row does, so a reason cannot impersonate one". What
		// actually holds is narrower: no line the CLI EMITS starts at column
		// zero, and none exceeds the CLI's fixed wrap budget. The CLI never
		// measures the terminal (`x/term.GetSize` is nowhere in this repo), so
		// in a terminal narrower than that budget the terminal soft-wraps and
		// the spill lands at column zero regardless — as it does for a line that
		// is within budget in RUNES but over it in display CELLS. Both residuals
		// are stated at wrapServerText and in the README; neither is closable
		// here.
		//
		// A tab in a reason is harmless for the same reason the block sits here:
		// it is written straight to `out`, never to the tabwriter, so it cannot
		// re-align the table above.
		//
		// It is NOT gated on status, for the reason printWorkflow gives at
		// length: a reason the server has already recorded is a RECORD, and
		// nothing has established that the orchestrator populates `errors` only
		// on failure. It is gated on there BEING one — a `failed` workflow
		// carrying none is a measured branch, and an empty label under it would
		// be noise.
		for _, r := range w.FailureReasons() {
			reported = true
			fmt.Fprintf(out, "%s%s\n", listReasonIndent,
				indentContinuation(wrapServerText(safeTerm(r), listReasonWrapWidth), listReasonIndent))
		}
	}
	// Nothing should remain, but emitting the tail rather than dropping it means
	// a miscount would show as a misplaced row and never as a vanished workflow.
	emit(len(lines) - next)

	st := ui.For(errw)
	if reported {
		fmt.Fprintln(errw, st.Dim(listReasonLegend))
	}
	if page.NextCursor != nil && *page.NextCursor != "" {
		// The cursor goes to STDOUT: it is data a pipeline consumes, and this
		// repo's convention keeps machine-usable values off stderr.
		fmt.Fprintf(out, "\nNext cursor: %s\n", safeTerm(*page.NextCursor))
		fmt.Fprintln(errw, st.Dim("More results — re-run with --cursor <next cursor> to continue."))
	}
	fmt.Fprintln(errw, st.Dim(
		"OUTPUTS is deliverable/total; they differ when an output was blocked, never landed, or you hid it. `civitai workflows get <id>` shows the reason and the URLs."))
}
