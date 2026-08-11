package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"text/tabwriter"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// appsLimitMax is the server-enforced page-size cap for GET /api/v1/apps (1-50,
// default 20). Kept in sync with the endpoint's zod bound.
const appsLimitMax = 50

// appKinds / appSorts are the FIXED enums the store's zod schema accepts for
// --kind and --sort. They rarely change, so the CLI validates them client-side
// for a fast, offline error (mirroring --limit's local check) instead of a
// round-trip. --category is deliberately NOT hardcoded here: the marketplace
// category set grows as the backend adds categories, so a client-side allowlist
// would drift — an invalid --category is left to the server's 400, which the SDK
// now surfaces with the offending field + allowed values (see badRequestDetail).
var (
	appKinds = []string{"all", "onsite", "offsite"}
	appSorts = []string{"top-rated", "popular", "newest", "name"}
	// appCategoriesHint documents the current marketplace category enum for the
	// --category flag help. It mirrors the backend MARKETPLACE_CATEGORIES /
	// listAppListingsSchema enum; it is NOT enforced client-side (see above), so a
	// backend-added category still works even before this list is updated.
	appCategoriesHint = "generation, games, utility, discovery, moderation, analytics, other"
)

// validateEnumFlag rejects an explicitly-set flag value that is not in the
// allowed set, with a clear allowed-values message (a usage error, no HTTP
// call). An empty value ("", the unset default) passes — it means "server
// default", exactly like an unset --limit.
func validateEnumFlag(name, value string, allowed []string) error {
	if value == "" {
		return nil
	}
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return asUsageError(fmt.Errorf(
		"invalid --%s %q; must be one of: %s", name, value, strings.Join(allowed, ", ")))
}

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
an empty list.`,
		Example: `  civitai app list
  civitai app list --kind onsite --sort popular --limit 10
  civitai app list --category generation --json
  civitai app list --cursor '<next-cursor-from-a-previous-page>'`,
		Args: cobra.NoArgs,
		PreRunE: func(c *cobra.Command, args []string) error {
			// An EXPLICIT non-positive --limit is a usage mistake (an unset --limit
			// is 0 and means "server default"); reject only the explicit case.
			if f := c.Flags().Lookup("limit"); f != nil && f.Changed && limit < 1 {
				return asUsageError(fmt.Errorf(
					"--limit must be a positive integer (got %d); omit --limit to use the server default", limit))
			}
			// Fast, offline validation of the FIXED enums (--category is left to the
			// server's 400 — its allowed set drifts as the backend adds categories).
			if err := validateEnumFlag("kind", kind, appKinds); err != nil {
				return err
			}
			if err := validateEnumFlag("sort", sort, appSorts); err != nil {
				return err
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
	cmd.Flags().StringVar(&kind, "kind", "", "filter by kind ("+strings.Join(appKinds, ", ")+")")
	cmd.Flags().StringVar(&category, "category", "", "filter by marketplace category ("+appCategoriesHint+")")
	cmd.Flags().StringVar(&sort, "sort", "", "sort order ("+strings.Join(appSorts, ", ")+")")
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
clean "not found" message.

This reads the PUBLIC STORE CATALOG, which is NOT the same thing as your
deployment: an app can be approved, deployed and serving at <slug>.civit.ai and
still not be in the store. When a 404 lands on a slug you own, the error says so
and points at ` + "`civitai app listing status`" + ` / ` + "`civitai app status`" + `.`,
		Example: `  civitai app view my-cool-app
  civitai app view my-cool-app --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Flags().GetBool("json")
			slug := args[0]
			ctx := context.Background()

			client, err := newLoginGatedReader()
			if err != nil {
				return err
			}
			d, raw, err := client.GetApp(ctx, slug)
			if err != nil {
				return explainAppViewNotFound(ctx, slug, err)
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

// explainAppViewNotFound upgrades the bare 404 from GET /api/v1/apps/{slug} to
// an actionable error when the caller turns out to OWN that slug (issue #223).
//
// The 404 is TRUTHFUL — `app view` and `app status` read genuinely DIFFERENT
// resources. `app view` reads the PUBLIC STORE CATALOG (GET /api/v1/apps/{slug},
// an approved+published AppListing), while `app status` reads the AUTHOR
// SUBMISSION PIPELINE (GET /api/v1/blocks/submissions), which is what carries
// deployState and the live URL. An app can therefore be approved, deployed and
// serving while the store cannot show it, and the store detail route correctly
// 404s. So the fix is the MESSAGE, not the lookup: nothing about the request
// changes, only what we tell the author a 404 means.
//
// 🔴 The ownership probe runs ONLY on the 404 path, and that is deliberate. The
// happy path must not pay a second round trip for advice it will never print,
// and the submissions route is invite-gated (a caller who is not an Apps author
// gets a 403 there) — so probing unconditionally would add both latency and a
// brand-new failure mode to a command that already succeeded.
func explainAppViewNotFound(ctx context.Context, slug string, err error) error {
	if !errors.Is(err, civitai.ErrNotFound) {
		return err
	}
	sub := ownedSubmission(ctx, slug)
	if sub == nil {
		// 🔴 Ownership was NOT ESTABLISHED — either the caller does not own this
		// slug, or the probe itself could not answer (network, auth, no token,
		// not an Apps author). Both collapse to the plain 404 on purpose:
		// reading nothing is not finding nothing, and asserting "your store
		// listing is not published" when we could not actually check would be a
		// false diagnosis at a correct project — strictly worse than the terse
		// 404 it replaced.
		return err
	}
	// %w keeps the classification (and therefore the not-found exit code)
	// attached; the advice is appended, never substituted.
	return fmt.Errorf("%w\n\n%s", err, appViewOwnedAdvice(slug, sub))
}

// ownedSubmission answers "does the caller own <slug>?" against the author
// submission route, returning the matching submission row or nil.
//
// nil means "not established" and deliberately does NOT distinguish "not yours"
// from "could not ask" — explainAppViewNotFound must treat those identically,
// so collapsing them here keeps the one decision in one place.
func ownedSubmission(ctx context.Context, slug string) *appapi.Submission {
	cfg, err := config.Load()
	if err != nil || cfg.Token() == "" {
		return nil
	}
	subs, err := appapi.NewWithSource(cfg.BaseURL(), auth.New(cfg), "").ListSubmissions(ctx, slug)
	if err != nil {
		return nil
	}
	// Match the slug HERE rather than trusting the ?blockId= narrowing: a
	// submission's blockId IS the slug, and a server that ignored the filter
	// would otherwise hand back someone else's newest row and turn this into a
	// confident false claim of ownership.
	//
	// 🔴 THE FIRST MATCH, NEVER THE LAST. The list is newest-first and
	// appViewOwnedAdvice prints THIS row's Status and DeployState VERBATIM, so an
	// older row's state reads as plain fact about the newest submission: an author
	// whose newest version is pending over an older approved/live one would be
	// told the wrong deploy state of their own app, with nothing on screen to mark
	// it wrong. Unpinned until #390 — the fixture was a single row, where both
	// ends ARE the same row, so reversing this loop left the whole suite green
	// (3786 RUN, 0 FAIL). Pinned now by
	// TestAppViewOwnedAdviceNamesTheNewestSubmission plus the reader ledger in
	// newest_row_pick_test.go. That the SERVER orders the list newest-first is an
	// unverified dependency on the route's contract, stated in the ledger rather
	// than closed — nothing here can check it.
	for i := range subs {
		if subs[i].BlockID == slug {
			return &subs[i]
		}
	}
	return nil
}

// appViewOwnedAdvice is the actionable half of the #223 message. It reports only
// what the two endpoints actually said — the store 404ed, and the submissions
// route holds a row for this slug with this status/deployState — and names the
// next command for each of the two resources. It deliberately does NOT claim a
// single cause: the CLI cannot see whether the listing is unpublished or the
// store's launch flag is simply hiding it, and re-deriving publication rules
// locally is exactly the vendoring this repo refuses.
func appViewOwnedAdvice(slug string, s *appapi.Submission) string {
	return fmt.Sprintf(
		"`%s` IS one of your own apps (submission status: %s, deploy: %s), so this 404 is not evidence that your deploy is broken.\n"+
			"`civitai app view` reads the PUBLIC store catalog (GET /api/v1/apps/{slug}) — a different resource from your submission, "+
			"which is what carries the deploy state and the live URL. The store lists an app only once its store LISTING is published "+
			"(a listing needs an icon and a cover), and the catalog itself is still gated by a launch flag: until it opens publicly you "+
			"only see apps if your account is a moderator or app-dev-tester.\n"+
			"Next: `civitai app listing status --slug %s` for the listing, `civitai app status %s` for the deploy state and live URL",
		slug, dashIfEmpty(safeTerm(s.Status)), deployLabel(s.DeployState), slug, slug)
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
		fmt.Fprintln(out, "No apps visible for your account — until the store opens publicly you only see apps if your account is a moderator or app-dev-tester, or nothing matches your filters.")
		fmt.Fprintln(out, "Make sure you are logged in (`civitai login`), or relax --kind / --category / --sort.")
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
		fmt.Fprintf(out, "  content:  %s\n", safeTerm(d.ContentRating))
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
