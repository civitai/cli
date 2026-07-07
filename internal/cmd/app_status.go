package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/ui"
	"github.com/spf13/cobra"
)

func newAppStatusCmd() *cobra.Command {
	var idFlag string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "status [blockId]",
		Short: "Check the review/deploy status of your App submissions",
		Long: `Check the review and deploy status of your own App submissions.

Calls the token-authenticated, self-scoped status route
(GET /api/v1/blocks/submissions) with your stored credential — you only ever see
your OWN submissions. Both a personal API key and an OAuth login (civitai login)
work; the OAuth token must carry the Apps submit scope (the same gate the
submit route uses).

With no argument it lists all your submissions (newest first). Pass a blockId
(app slug) or --id <pubreq_id> to see a single submission in detail, including the
rejection reason (if rejected) and the live URL (if approved + deployed).

Note: a submission's <blockId>.civit.ai surface only serves AFTER it is approved
and deployed (deployState 'live').`,
		Example: `  civitai app status                 # list all your submissions
  civitai app status my-block        # detail for the my-block app
  civitai app status --id pubreq_01H # detail by publish-request id
  civitai app status --json          # raw JSON (scriptable)`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Token() == "" {
				return fmt.Errorf("no token configured — run `civitai login` (or set CIVITAI_TOKEN)")
			}

			var blockID string
			if len(args) == 1 {
				blockID = args[0]
			}

			client := api.NewWithSource(cfg.BaseURL(), auth.New(cfg), "")
			ctx := context.Background()
			out := cmd.OutOrStdout()

			// Detail view when a blockId arg OR --id is given.
			if idFlag != "" || blockID != "" {
				sub, err := client.GetSubmission(ctx, idFlag, blockID)
				if err != nil {
					return err
				}
				if jsonOut {
					return writeJSON(out, sub)
				}
				printSubmissionDetail(out, sub)
				return nil
			}

			subs, err := client.ListSubmissions(ctx, "")
			if err != nil {
				return err
			}
			if jsonOut {
				return writeJSON(out, map[string]any{"submissions": subs})
			}
			if len(subs) == 0 {
				fmt.Fprintf(out, "No submissions yet — run %s to create one.\n", ui.Code("civitai app submit"))
				return nil
			}
			printSubmissionTable(out, subs)
			return nil
		},
	}
	cmd.Flags().StringVar(&idFlag, "id", "", "look up a single submission by publish-request id (pubreq_...)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit raw JSON (scriptable)")
	return cmd
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printSubmissionTable(w io.Writer, subs []api.Submission) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "BLOCK_ID\tVERSION\tSTATUS\tDEPLOY\tSUBMITTED\tURL")
	for _, s := range subs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			s.BlockID,
			s.Version,
			s.Status,
			deployLabel(s.DeployState),
			shortDate(s.SubmittedAt),
			strOr(s.LiveURL, "-"),
		)
	}
	_ = tw.Flush()
}

func printSubmissionDetail(w io.Writer, s *api.Submission) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Block ID:\t%s\n", s.BlockID)
	fmt.Fprintf(tw, "Version:\t%s\n", s.Version)
	fmt.Fprintf(tw, "Publish request:\t%s\n", s.ID)
	fmt.Fprintf(tw, "Status:\t%s\n", s.Status)
	fmt.Fprintf(tw, "Deploy state:\t%s\n", deployLabel(s.DeployState))
	if s.DeployDetail != nil && *s.DeployDetail != "" {
		fmt.Fprintf(tw, "Deploy detail:\t%s\n", *s.DeployDetail)
	}
	fmt.Fprintf(tw, "Submitted:\t%s\n", fullDate(s.SubmittedAt))
	if s.ReviewedAt != nil && *s.ReviewedAt != "" {
		fmt.Fprintf(tw, "Reviewed:\t%s\n", fullDate(*s.ReviewedAt))
	}
	if s.DeployUpdatedAt != nil && *s.DeployUpdatedAt != "" {
		fmt.Fprintf(tw, "Deploy updated:\t%s\n", fullDate(*s.DeployUpdatedAt))
	}
	_ = tw.Flush()

	if s.Status == "rejected" && s.RejectionReason != nil && *s.RejectionReason != "" {
		fmt.Fprintf(w, "\nRejection reason:\n  %s\n", *s.RejectionReason)
	}
	if s.ApprovalNotes != nil && *s.ApprovalNotes != "" {
		fmt.Fprintf(w, "\nApproval notes:\n  %s\n", *s.ApprovalNotes)
	}
	if s.LiveURL != nil && *s.LiveURL != "" {
		fmt.Fprintf(w, "\nLive at: %s\n", ui.URL(*s.LiveURL))
	} else {
		fmt.Fprintf(w, "\nNot live yet — %s.civit.ai only serves after the app is approved and deployed (deployState 'live').\n", s.BlockID)
	}
}

// deployLabel renders a possibly-null deployState; null means "no Phase-2 deploy
// pipeline ran" (legacy/approved rows assume live).
func deployLabel(state *string) string {
	if state == nil || *state == "" {
		return "-"
	}
	return *state
}

func strOr(p *string, fallback string) string {
	if p == nil || *p == "" {
		return fallback
	}
	return *p
}

// shortDate renders an RFC3339 timestamp as YYYY-MM-DD; the raw string on a parse
// failure so we never hide data.
func shortDate(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Format("2006-01-02")
}

// fullDate renders an RFC3339 timestamp in local time, minute precision.
func fullDate(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Local().Format("2006-01-02 15:04 MST")
}
