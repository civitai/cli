package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"text/tabwriter"

	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// appsLimitMax is the server-enforced page-size cap for GET /api/v1/apps (1-50,
// default 20). Kept in sync with the endpoint's zod bound.
const appsLimitMax = 50

// newAppListCmd is `civitai app list` — discover published Apps in the store.
//
// This is FILTER-based discovery (kind/category/sort + keyset cursor), NOT
// keyword search: the backend store service exposes no free-text query, so there
// is deliberately no `app search` command yet (it would duplicate `list`). When
// text search lands, `app search <query>` is a trivial addition over SearchApps.
func newAppListCmd() *cobra.Command {
	var kind, category, sort, cursor string
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Discover published Apps in the store (GET /api/v1/apps)",
		Long: `List published Apps from the Civitai store via GET /api/v1/apps.

This is filter-based discovery — filter by --kind / --category, order by --sort,
and page with --cursor. There is no free-text search yet (the store service
doesn't support it), so there is no ` + "`app search`" + ` command.

Login is required (` + "`civitai login`" + `): the endpoint keys the visible catalog
off your identity, so an anonymous call would see nothing. Pagination is keyset
cursor-based (no --page); the next cursor is printed after the results — pass it
back via --cursor.

The store is rate-limited per caller; a tight scripted loop may see 429s (the CLI
backs off and retries automatically).

NOTE: the store is gated by a launch flag — until it opens publicly you will only
see apps if your account is a moderator or app-dev-tester; a normal login may get
an empty list. This command is useless until the backing API is deployed.`,
		Example: `  civitai app list
  civitai app list --kind onsite --sort popular --limit 10
  civitai app list --category productivity --json
  civitai app list --cursor '<next-cursor-from-a-previous-page>'`,
		Args: cobra.NoArgs,
		PreRunE: func(c *cobra.Command, args []string) error {
			// An EXPLICIT non-positive --limit is a usage mistake (an unset --limit
			// is 0 and means "server default"); reject only the explicit case.
			if f := c.Flags().Lookup("limit"); f != nil && f.Changed && limit < 1 {
				return asUsageError(fmt.Errorf(
					"--limit must be a positive integer (got %d); omit --limit to use the server default", limit))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkLimit(limit, appsLimitMax); err != nil {
				return err
			}
			jsonOut, _ := cmd.Flags().GetBool("json")

			q := url.Values{}
			addIfSet(q, "kind", kind)
			addIfSet(q, "category", category)
			addIfSet(q, "sort", sort)
			addIfSet(q, "cursor", cursor)
			addIfPositive(q, "limit", limit)

			client, err := newLoginGatedReader()
			if err != nil {
				return err
			}
			res, err := client.SearchApps(context.Background(), q)
			if err != nil {
				return err
			}

			if jsonOut {
				return emitJSON(cmd, res.Raw)
			}
			printAppList(cmd, res.Items)
			printPageFooter(cmd, "civitai app list", res.Metadata)
			return nil
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "filter by kind (all, onsite, offsite)")
	cmd.Flags().StringVar(&category, "category", "", "filter by marketplace category")
	cmd.Flags().StringVar(&sort, "sort", "", "sort order (top-rated, popular, newest, name)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor from a previous response")
	cmd.Flags().IntVar(&limit, "limit", 0, fmt.Sprintf("results per page (1-%d)", appsLimitMax))
	cmd.Flags().Bool("json", false, "print the raw API JSON response (for scripting)")
	return cmd
}

// newAppViewCmd is `civitai app view <slug>` — one App's public detail.
func newAppViewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "view <slug>",
		Short: "Show one App's detail (GET /api/v1/apps/{slug})",
		Long: `Show the public detail for one published App by slug via
GET /api/v1/apps/{slug} — its description, category, rating, gallery, and the
kind-specific action target (an on-site app's live URL, or an off-site app's
external / connect target).

Login is required (` + "`civitai login`" + `). A missing or out-of-scope slug returns a
clean "not found" message. This command is useless until the backing API is
deployed.`,
		Example: `  civitai app view my-cool-app
  civitai app view my-cool-app --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Flags().GetBool("json")

			client, err := newLoginGatedReader()
			if err != nil {
				return err
			}
			d, raw, err := client.GetApp(context.Background(), args[0])
			if err != nil {
				return err
			}
			if jsonOut {
				return emitJSON(cmd, raw)
			}
			printAppDetail(cmd, d)
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "print the raw API JSON response (for scripting)")
	return cmd
}

// newLoginGatedReader builds a civitai.Client for the App-store read endpoints,
// REQUIRING a stored login token client-side. The endpoint is optional-bearer
// server-side, but the CLI requires login for a stable caller identity (per-user
// rate-limit + the identity-scoped visible catalog) — mirroring the token guard
// on `civitai app status`. The token (OAuth device-login or a personal key) is
// sent as a bearer and refreshed transparently on a 401.
func newLoginGatedReader() (*civitai.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if cfg.Token() == "" {
		return nil, civitai.Tag(civitai.ErrUnauthorized,
			fmt.Errorf("no token configured — run `civitai login` (or set CIVITAI_TOKEN) to browse the App store"))
	}
	return civitai.NewWithSource(cfg.BaseURL(), auth.New(cfg)), nil
}

// appRating renders a recommend rollup as a compact percentage, or a dash when
// there are no reviews yet (recommendPct is null, not 0).
func appRating(r civitai.ListingRecommend) string {
	if r.RecommendPct == nil {
		return "-"
	}
	return fmt.Sprintf("%d%%", int(*r.RecommendPct*100+0.5))
}

// appCardAuthor renders a card's creator username, or a dash when the creator
// chip is absent (a vanished owner row).
func appCardAuthor(c *civitai.AppCard) string {
	if c.Creator != nil && c.Creator.Username != "" {
		return safeTerm(c.Creator.Username)
	}
	return "-"
}

func printAppList(cmd *cobra.Command, items []civitai.AppCard) {
	out := cmd.OutOrStdout()
	if len(items) == 0 {
		fmt.Fprintln(out, "No apps found — the store may not be publicly launched yet, or no app matches these filters.")
		fmt.Fprintln(out, "If you are a moderator or app-dev-tester, make sure you are logged in (`civitai login`).")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tSLUG\tKIND\tCATEGORY\tAUTHOR\tRATING\tREVIEWS")
	for i := range items {
		c := &items[i]
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%d\n",
			safeTerm(c.Name),
			dashIfEmpty(safeTerm(c.Slug)),
			dashIfEmpty(safeTerm(c.Kind)),
			dashIfEmpty(safeTerm(c.Category)),
			appCardAuthor(c),
			appRating(c.Recommend),
			c.ReviewCount,
		)
	}
	_ = tw.Flush()
}

func printAppDetail(cmd *cobra.Command, d *civitai.AppDetail) {
	out := cmd.OutOrStdout()
	author := "-"
	if d.Creator != nil && d.Creator.Username != "" {
		author = safeTerm(d.Creator.Username)
	}
	fmt.Fprintf(out, "%s (%s)\n", safeTerm(d.Name), safeTerm(d.Slug))
	if d.Tagline != "" {
		fmt.Fprintf(out, "  %s\n", safeTerm(truncate(d.Tagline, 200)))
	}
	fmt.Fprintf(out, "  kind:     %s\n", dashIfEmpty(safeTerm(d.Kind)))
	fmt.Fprintf(out, "  category: %s\n", dashIfEmpty(safeTerm(d.Category)))
	fmt.Fprintf(out, "  rating:   %s (%d review(s))\n", appRating(d.Recommend), d.ReviewCount)
	if d.ContentRating != "" {
		fmt.Fprintf(out, "  rating:   content %s\n", safeTerm(d.ContentRating))
	}
	fmt.Fprintf(out, "  author:   %s\n", author)

	// Kind-specific action data.
	switch d.KindData.Kind {
	case "onsite":
		if d.KindData.LiveURL != "" {
			fmt.Fprintf(out, "  live:     %s\n", safeTerm(d.KindData.LiveURL))
		}
		fmt.Fprintf(out, "  page:     %t\n", d.KindData.HasPage)
	case "offsite":
		if d.KindData.SubKind != "" {
			fmt.Fprintf(out, "  offsite:  %s\n", safeTerm(d.KindData.SubKind))
		}
		if d.KindData.ExternalURL != "" {
			fmt.Fprintf(out, "  url:      %s\n", safeTerm(d.KindData.ExternalURL))
		}
		if d.KindData.ConnectClientID != "" {
			fmt.Fprintf(out, "  clientId: %s\n", safeTerm(d.KindData.ConnectClientID))
		}
	}

	if len(d.Screenshots) > 0 {
		fmt.Fprintf(out, "  gallery:  %d screenshot(s)\n", len(d.Screenshots))
	}
	if d.Description != "" {
		fmt.Fprintf(out, "\n%s\n", safeTerm(truncate(strings.TrimSpace(d.Description), 600)))
	}
}
