package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

func newAppStatusCmd() *cobra.Command {
	var idFlag string
	var jsonOut bool
	var limit int

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

--limit N shows only the newest N of them. It is a DISPLAY limit, not a page
size: this route accepts no limit and no cursor (that is what the cap note is
about), so the CLI always fetches the same page and prints fewer rows of it.
--limit therefore cannot reach submissions the API did not return.

Note: a submission's <blockId>.civit.ai surface only serves AFTER it is approved
and deployed (deployState 'live').`,
		Example: `  civitai app status                 # list all your submissions
  civitai app status --limit 5       # just the newest five
  civitai app status my-block        # detail for the my-block app
  civitai app status --id pubreq_01H # detail by publish-request id
  civitai app status --json          # raw JSON (scriptable)`,
		Args: cobra.MaximumNArgs(1),
		PreRunE: func(c *cobra.Command, args []string) error {
			f := c.Flags().Lookup("limit")
			if f == nil || !f.Changed {
				return nil
			}
			// An unset --limit means "everything", so only an EXPLICIT
			// non-positive value is a mistake — the same rule `app list`
			// applies to its own (server-side) --limit.
			if limit < 1 {
				return asUsageError(fmt.Errorf(
					"--limit must be a positive integer (got %d); omit --limit to list every submission the API returned", limit))
			}
			// 🔴 Refused rather than ignored on the detail view. A single
			// submission cannot be limited, and a flag that is silently
			// accepted and does nothing is precisely the papercut this flag
			// was added to fix.
			if idFlag != "" || len(args) == 1 {
				return asUsageError(fmt.Errorf(
					"--limit applies to the submission LISTING, but a single submission was requested — drop --limit, or drop the blockId/--id to list"))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Token() == "" {
				return civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf("no token configured — run `civitai login` (or set CIVITAI_TOKEN)"))
			}

			var blockID string
			if len(args) == 1 {
				blockID = args[0]
			}

			client := appapi.NewWithSource(cfg.BaseURL(), auth.New(cfg), "")
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
			// The route caps the unfiltered listing and returns no cursor and no
			// total, so a full-length page is the only hint that older rows were
			// dropped. Say so instead of presenting a truncated list as the whole
			// truth. STDERR (like the other API-limitation notes in this CLI) so
			// --json stdout stays pure; exit stays 0.
			// 🔴 Computed from what the SERVER returned, before --limit trims
			// anything. The caveat is a statement about the API's cap, not
			// about how many rows the CLI chose to print — deriving it from
			// the trimmed slice would let `--limit 5` silently claim a capped
			// listing was complete.
			if submissionsListTruncated(len(subs)) {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"note: showing the newest %d submissions — the API caps this listing and offers no way to page, "+
						"so older submissions may exist but are not listed. "+
						"Look up a specific app with `civitai app status <blockId>`.\n", len(subs))
			}
			// Display-side only: the route takes no limit parameter, so the
			// trim happens here and NOTHING about it goes on the wire.
			shown := subs
			if limit > 0 && limit < len(shown) {
				shown = shown[:limit]
			}
			if jsonOut {
				return writeJSON(out, map[string]any{"submissions": shown})
			}
			if len(shown) == 0 {
				fmt.Fprintf(out, "No submissions yet — run %s to create one.\n", ui.Code("civitai app submit"))
				return nil
			}
			printSubmissionTable(out, shown)
			return nil
		},
	}
	cmd.Flags().StringVar(&idFlag, "id", "", "look up a single submission by publish-request id (pubreq_...)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit raw JSON (scriptable)")
	cmd.Flags().IntVar(&limit, "limit", 0, "show only the newest N submissions (display-side; the API pages nothing)")
	return cmd
}

// submissionsListTruncated reports whether an UNFILTERED submissions listing of
// n rows may have been cut off by the server's row cap
// (appapi.ListSubmissionsCap — see its doc comment for why this can only ever be
// an inference). The comparison is `>=`, not `==`: at or beyond the cap we know
// about, this CLI cannot claim the listing is complete. A caller holding exactly
// the cap gets the caveat while nothing is actually missing — accepted, because
// the alternative failure (silently reporting a truncated list as complete) is
// the bug this exists to fix, and the wording says "may exist", not "exist".
//
// Only the unfiltered list goes through here: a slug/id lookup is narrowed
// server-side before the cap applies.
func submissionsListTruncated(n int) bool { return n >= appapi.ListSubmissionsCap }

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printSubmissionTable(w io.Writer, subs []appapi.Submission) {
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

func printSubmissionDetail(w io.Writer, s *appapi.Submission) {
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
