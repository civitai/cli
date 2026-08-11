package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// wireTimeLayout is the timestamp format the analytics proc is called with:
// RFC3339 in UTC with millisecond precision, matching the shape the server
// itself echoes back in `range`.
const wireTimeLayout = "2006-01-02T15:04:05.000Z"

// dateOnlyLayout is the friendly `--from` / `--to` form. A bare date is
// interpreted as MIDNIGHT UTC, so the window a user asks for is reproducible
// regardless of where they run the CLI.
const dateOnlyLayout = "2006-01-02"

// appMetricsCredentialRoute names the ONE credential that works for this
// command, and it is deliberately NOT login.go's spendCredentialRoutes.
//
// 🔴 That constant names BOTH routes because generation accepts either. The
// analytics proc does not: it is full-scope, so an OAuth browser login is
// refused with 403 (see the Long text, and item 5 in AGENTS.md). Until issue
// #260 the no-token error here was the generic "run `civitai login`" — which is
// the ONE route this command cannot use, so following the CLI's own advice
// landed the author on a second refusal. `generate` and `workflows` already name
// their working routes; this mirrors that shape for the route that works here.
const appMetricsCredentialRoute = "a full-scope personal API key " +
	"(`civitai login --token <key>`, created at " + accountAPIKeysURL + ")"

// appMetricsDeps are the two network seams `app metrics` needs: slug →
// appBlockId resolution (the existing submissions route) and the analytics query
// itself. Bundled so the command core is exercisable without a live server.
type appMetricsDeps struct {
	listSubmissions func(ctx context.Context, blockID string) ([]appapi.Submission, error)
	getAnalytics    func(ctx context.Context, appBlockID, from, to string) (*appapi.AppAnalytics, json.RawMessage, error)
}

func newAppMetricsCmd() *cobra.Command {
	var fromFlag, toFlag string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "metrics <slug>",
		Short: "Show your App's install / run / Buzz / engagement analytics",
		Long: `Show the owner-only analytics for one of YOUR App Blocks: installs, runs and
the Buzz they spent, Buzz purchased through the app, and API engagement.

The slug is resolved to its appBlockId through your own submissions
(` + "`civitai app status`" + ` reads the same route), so analytics are only available
once a version of the app has been APPROVED — an app that was never approved has
no App Block to report on yet.

WINDOW: the server defaults to the last 30 days and clamps any request to 366
days, so a zero is only meaningful together with the period it covers. This
command therefore always prints the window the SERVER served (echoed from the
response), not the one you asked for. Pass --from / --to as a plain YYYY-MM-DD
date (midnight UTC) or a full RFC3339 timestamp to widen it.

CREDENTIAL: the analytics query is full-scope, so it needs ` + appMetricsCredentialRoute + `;
an OAuth browser login is refused with 403.

DATA CAVEAT: engagement counts only AUTHENTICATED, scope-gated API calls. An app
that ships no scoped API surface will show real installs and revenue with a flat
engagement section — that is expected, not a bug.`,
		Example: `  civitai app metrics my-block
  civitai app metrics my-block --from 2026-05-01 --to 2026-08-03
  civitai app metrics my-block --from 2026-05-01T00:00:00Z
  civitai app metrics my-block --json`,
		// cobra.ExactArgs(1) answers a bare `civitai app metrics` with "accepts 1
		// arg(s), received 0" — correct, and it names neither what the argument
		// IS nor where to find one. `app dev-token` / `app dev-tunnel` already
		// answer the same mistake by naming the slug and the command that lists
		// slugs; this matches them (civitai/cli#363). enforceUsageExitCodes tags
		// whatever this returns ErrUsage, so the exit code stays 2.
		Args: func(cmd *cobra.Command, args []string) error {
			switch {
			case len(args) == 0:
				return fmt.Errorf("an app slug is required — e.g. `civitai app metrics my-block` (list yours with `civitai app status`)")
			case len(args) > 1:
				return fmt.Errorf("accepts 1 app slug, received %d — `civitai app metrics <slug>` reports on one app at a time", len(args))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			from, to, err := parseMetricsWindow(fromFlag, toFlag)
			if err != nil {
				return err
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Token() == "" {
				// Name the route that WORKS, not the generic one: an OAuth
				// browser login is refused here with 403 (see
				// appMetricsCredentialRoute).
				return civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf(
					"no token configured — app analytics need %s; a browser login (`civitai login`) is full-scope-refused with 403 here. Or set CIVITAI_TOKEN to that key",
					appMetricsCredentialRoute))
			}

			client := appapi.NewWithSource(cfg.BaseURL(), auth.New(cfg), "")
			deps := appMetricsDeps{
				listSubmissions: client.ListSubmissions,
				getAnalytics:    client.GetMyAppAnalytics,
			}
			return runAppMetrics(cmd, deps, strings.TrimSpace(args[0]), from, to, jsonOut)
		},
	}
	cmd.Flags().StringVar(&fromFlag, "from", "", "window start: YYYY-MM-DD (midnight UTC) or RFC3339 (default: 30 days ago, server-side)")
	cmd.Flags().StringVar(&toFlag, "to", "", "window end: YYYY-MM-DD (midnight UTC) or RFC3339 (default: now, server-side)")
	// NOTE: no back-quotes in this usage string — cobra/pflag's UnquoteUsage
	// treats the first back-quoted span as the flag's VALUE NAME, so a stray
	// `notOwned: true` renders the boolean as "--json notOwned: true".
	cmd.Flags().BoolVar(&jsonOut, "json", false,
		"emit the raw analytics payload (scriptable). Unlike the human view, --json does NOT refuse a not-entitled read: "+
			"a notOwned:true payload is passed through with every counter zeroed and still exits 0, so a script MUST "+
			"branch on the notOwned field rather than trusting the counts")
	return cmd
}

// parseMetricsWindow converts the --from / --to flag values to the wire format.
// Both are optional; an unset bound stays empty so the server applies its own
// default. A malformed value or an inverted window is a USAGE error (exit 2),
// not a runtime failure — it is decided entirely client-side, before any call.
func parseMetricsWindow(fromFlag, toFlag string) (string, string, error) {
	from, fromT, err := parseMetricsTime("from", fromFlag)
	if err != nil {
		return "", "", err
	}
	to, toT, err := parseMetricsTime("to", toFlag)
	if err != nil {
		return "", "", err
	}
	if from != "" && to != "" && fromT.After(toT) {
		return "", "", asUsageError(fmt.Errorf(
			"--from %s is after --to %s — the window start must not be later than its end", fromFlag, toFlag))
	}
	return from, to, nil
}

// parseMetricsTime accepts a bare YYYY-MM-DD (midnight UTC) or a full RFC3339
// timestamp and returns the wire-format string plus the parsed instant. An empty
// value yields an empty string (bound omitted).
func parseMetricsTime(flag, value string) (string, time.Time, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", time.Time{}, nil
	}
	if t, err := time.Parse(dateOnlyLayout, v); err == nil {
		return t.UTC().Format(wireTimeLayout), t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC().Format(wireTimeLayout), t.UTC(), nil
	}
	return "", time.Time{}, asUsageError(fmt.Errorf(
		"invalid --%s %q — use a date (2026-05-01) or an RFC3339 timestamp (2026-05-01T00:00:00Z)", flag, v))
}

// runAppMetrics is the testable core: resolve the slug to an appBlockId, query
// the analytics, then either passthrough the raw payload or render it.
func runAppMetrics(cmd *cobra.Command, deps appMetricsDeps, slug, from, to string, jsonOut bool) error {
	ctx := context.Background()
	appBlockID, err := resolveAppBlockID(ctx, deps.listSubmissions, slug)
	if err != nil {
		return err
	}

	analytics, raw, err := deps.getAnalytics(ctx, appBlockID, from, to)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	if jsonOut {
		// Passthrough of the server's own payload (including notOwned), so a
		// script sees every field — even ones this CLI does not model yet.
		return writeRawJSON(out, raw)
	}
	if analytics.NotOwned {
		// The server answers 200-with-zeros for a non-owner / non-entitled caller.
		// Rendering that as a dashboard would present a permission failure as real
		// data, so refuse to render anything and say why.
		return fmt.Errorf("no analytics for %q — the server reports this app is not owned by the authenticated account, "+
			"or the account lacks Apps-author access. Confirm the account with `civitai whoami` and that the app is yours "+
			"with `civitai app status %s`", slug, slug)
	}
	printAppMetrics(out, slug, analytics)
	return nil
}

// resolveAppBlockID maps an app slug to its appBlockId via the caller's own
// submissions. `appBlockId` is null until a version is APPROVED, so a slug with
// only pending/rejected submissions resolves to nothing — which is a distinct,
// separately-actionable condition from "no such app".
//
// 🔴 THE "NOTHING APPROVED" NEXT STEP IS SHARED WITH `app pull`, NOT COPIED.
// explainMissingApp (app_pull.go) answers the IDENTICAL precondition — rows
// exist, none carries an appBlockId — and this function open-coded its own
// "check where it is in review", so the same user with the same REJECTED app got
// two contradictory answers from adjacent commands: `app pull` said nothing is
// in review, `app metrics` sent them to check a review that is not happening.
// One rule, one place: both call pullReviewAdvice.
//
// AGENTS.md item 7: only the ADVICE is shared. pullReviewAdvice returns a
// string and carries no classification, so this error stays UNTAGGED — the
// generic exit 1, a resource that exists but is not ready — while `app pull`'s
// stays civitai.ErrNotFound (exit 4). Sharing it cannot move either code.
func resolveAppBlockID(ctx context.Context, list func(context.Context, string) ([]appapi.Submission, error), slug string) (string, error) {
	if slug == "" {
		return "", asUsageError(fmt.Errorf("an app slug is required — list yours with `civitai app status`"))
	}
	subs, err := list(ctx, slug)
	if err != nil {
		return "", err
	}
	if len(subs) == 0 {
		return "", civitai.Tag(civitai.ErrNotFound, fmt.Errorf(
			"no submissions found for app %q — check the slug with `civitai app status`", slug))
	}
	// Newest first, so the first non-null appBlockId is the current block.
	for i := range subs {
		if subs[i].AppBlockID != nil && *subs[i].AppBlockID != "" {
			return *subs[i].AppBlockID, nil
		}
	}
	// Newest first, so subs[0] is the state whose next step to name.
	//
	// 🔴 subs[0], NEVER subs[len(subs)-1] — and it was unpinned here and in
	// explainMissingApp alike until #378: mutating BOTH to the last row left the
	// whole suite green (3653 RUN, 0 FAIL), because every fixture had one row,
	// where the two ends are the same row. This command prints no "latest
	// submission" parenthetical, so reading the wrong end is invisible — the
	// oldest row's advice simply reads as the truth. Pinned by
	// TestAppMetricsAdviceNamesTheNewestSubmission and the call-site ledger in
	// app_newest_submission_test.go.
	return "", fmt.Errorf("app %q has no approved App Block yet — analytics only exist once a submitted version is approved; %s",
		slug, pullReviewAdvice(slug, subs[0].Status))
}

// writeRawJSON pretty-prints the server payload verbatim (no re-marshalling, so
// no field is dropped or reordered by this CLI's structs).
func writeRawJSON(w io.Writer, raw json.RawMessage) error {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		// Not indentable (should not happen — it parsed upstream); emit as-is
		// rather than failing a scriptable path.
		_, err := fmt.Fprintf(w, "%s\n", raw)
		return err
	}
	_, err := fmt.Fprintf(w, "%s\n", buf.String())
	return err
}

// printAppMetrics renders the human dashboard. The window comes from the
// RESPONSE range so a zero is never ambiguous about the period it covers.
func printAppMetrics(w io.Writer, slug string, a *appapi.AppAnalytics) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "App:\t%s\n", slug)
	fmt.Fprintf(tw, "Window:\t%s → %s\n", utcStamp(a.Range.From), utcStamp(a.Range.To))
	fmt.Fprintf(tw, "Granularity:\t%s\n", dashIfEmpty(a.Range.Granularity))
	_ = tw.Flush()

	fmt.Fprintf(w, "\n%s\n", ui.For(w).Bold("Installs"))
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if a.Installs.NotApplicable {
		// 🔴 Not a zero, and not an outage. The server sets this only when NO
		// owned app has an install slot — a page app is stateless by design, so
		// a subscription row cannot exist for it. Printing `0` here reads as
		// "nobody installed my app" when the truth is "installs do not exist
		// for this app type", which is the same fabricated-zero class the
		// notOwned and views.unavailable branches already guard.
		//
		// A TRUTHFUL zero (an installable app nobody has installed yet) does
		// NOT set the flag and must keep printing 0 — the server owns that
		// distinction; do not re-derive it here from the counters.
		fmt.Fprintf(tw, "  Total\t%s\n", "n/a")
		fmt.Fprintf(tw, "  Active\t%s\n", "n/a")
		_ = tw.Flush()
		fmt.Fprintf(w, "\n%s\n", ui.For(w).Dim(
			"This app cannot be installed — it has no install slot, so installs do not apply. That is different from zero installs."))
	} else {
		fmt.Fprintf(tw, "  Total\t%d\n", a.Installs.Total)
		fmt.Fprintf(tw, "  Active\t%d\n", a.Installs.Active)
		_ = tw.Flush()
	}

	fmt.Fprintf(w, "\n%s\n", ui.For(w).Bold("Runs"))
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  Count\t%d\n", a.Runs.Count)
	fmt.Fprintf(tw, "  Buzz spent\t%d\n", a.Runs.BuzzSpent)
	_ = tw.Flush()

	fmt.Fprintf(w, "\n%s\n", ui.For(w).Bold("Buzz purchased"))
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  Purchases\t%d\n", a.BuzzPurchased.Count)
	fmt.Fprintf(tw, "  Buzz\t%d\n", a.BuzzPurchased.BuzzAmount)
	fmt.Fprintf(tw, "  Gross\t%s\n", usdFromCents(a.BuzzPurchased.GrossCents))
	_ = tw.Flush()

	// VIEWS sits before Engagement on purpose: it is the broader number (every
	// load, including signed-out ones), so an author reads the wide measure
	// first and the authenticated-only subset second — which is also the order
	// the coverage caveat below then makes sense in.
	fmt.Fprintf(w, "\n%s\n", ui.For(w).Bold("App loads"))
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	// 🔴 Never print 0 in either unknown case. The impression store is the one
	// section not derived from Postgres, so it can be unreadable while every
	// counter above is genuinely measured — and a printed 0 is
	// indistinguishable from "nobody looked", the fabricated-zero defect this
	// flag exists to prevent. `a.Views == nil` is the SECOND way to not know:
	// a server predating the impressions reader omits the section entirely,
	// and a value type would have silently turned that into a measured zero.
	if a.Views == nil || a.Views.Unavailable {
		fmt.Fprintf(tw, "  Impressions\t%s\n", "unavailable")
		fmt.Fprintf(tw, "  Unique viewers\t%s\n", "unavailable")
		_ = tw.Flush()
		if a.Views == nil {
			fmt.Fprintf(w, "\n%s\n", ui.For(w).Dim(
				"This server did not report app loads — this is NOT a report of zero loads. Upgrade the server, or ignore this section."))
		} else {
			fmt.Fprintf(w, "\n%s\n", ui.For(w).Dim(
				"The impression store could not be read — this is NOT a report of zero loads. Every other metric above is unaffected."))
		}
	} else {
		fmt.Fprintf(tw, "  Impressions\t%d\n", a.Views.Count)
		fmt.Fprintf(tw, "  Unique viewers\t%d\n", a.Views.UniqueViewers)
		fmt.Fprintf(tw, "  Signed-out loads\t%d\n", a.Views.AnonCount)
		_ = tw.Flush()
	}

	fmt.Fprintf(w, "\n%s\n", ui.For(w).Bold("Engagement"))
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  API calls\t%d\n", a.Engagement.APICalls)
	fmt.Fprintf(tw, "  Active users\t%d\n", a.Engagement.ActiveUsers)
	fmt.Fprintf(tw, "  Error rate\t%s\n", pctFromRatio(a.Engagement.ErrorRate))
	_ = tw.Flush()

	if len(a.Engagement.TopScopes) > 0 {
		fmt.Fprintln(w, "\n  Top scopes:")
		tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, s := range a.Engagement.TopScopes {
			fmt.Fprintf(tw, "    %s\t%d\n", safeTerm(s.Scope), s.Count)
		}
		_ = tw.Flush()
	}
	if len(a.Engagement.TopEndpoints) > 0 {
		fmt.Fprintln(w, "\n  Top endpoints:")
		tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, e := range a.Engagement.TopEndpoints {
			fmt.Fprintf(tw, "    %s\t%d\n", safeTerm(e.Endpoint), e.Count)
		}
		_ = tw.Flush()
	}
	if a.Engagement.APICalls == 0 {
		fmt.Fprintf(w, "\n%s\n", ui.For(w).Dim(
			"Engagement counts only authenticated, scope-gated API calls — an app with no scoped API surface reads flat here. App loads is measured on every load, so it still counts those visitors."))
	}
}

// utcStamp renders an RFC3339 timestamp in UTC (minute precision), falling back
// to the raw string on a parse failure so server data is never hidden.
func utcStamp(ts string) string {
	if strings.TrimSpace(ts) == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

// pctFromRatio renders the engagement error rate as a percentage. The unit is
// settled server-side — civitai/civitai
// src/server/services/blocks/app-analytics.service.ts computes
// `errorRate = apiCalls > 0 ? errorCount / apiCalls : 0` (line 276), i.e. a 0–1
// ratio — so printing it verbatim shows an unlabelled `0.02040816326530612`
// where the reader wants `2.0%`. One decimal place keeps a real rate legible
// without implying precision the sample size can't support. Only the human view
// is rescaled: `--json` still passes the server's raw ratio through untouched.
//
// SMALL-BUT-NONZERO: one decimal place alone collapses every ratio below 0.0005
// to `0.0%`, which is byte-identical to a genuine zero — so a healthy app with
// 5,000 calls and 2 errors (0.0004) would read as having no errors at all. That
// is exactly the quietly-wrong number this command exists to avoid, so anything
// that is nonzero yet rounds away is rendered `<0.1%` instead. Only an exact
// zero prints `0.0%`.
//
// INPUT PRECONDITION [0,1]: `errorCount` counts a SUBSET of the rows `apiCalls`
// counts — the same `block_scope_invocations` filter plus `statusCode >= 400`
// (service lines 241-256) — and the divide is guarded by `apiCalls > 0`. The
// ratio is therefore structurally in [0,1]; negative, >1, NaN and Inf inputs are
// not producible by this server, so this function does not defend against them
// (a negative would fall through to the plain `-x.y%` form).
func pctFromRatio(r float64) string {
	pct := strconv.FormatFloat(r*100, 'f', 1, 64)
	// Decided on the RENDERED value, not on a hard-coded 0.0005 threshold, so
	// the two can never drift apart at the float64 boundary.
	if r > 0 && pct == "0.0" {
		return "<0.1%"
	}
	return pct + "%"
}

// usdFromCents renders integer USD cents as dollars.
func usdFromCents(cents int64) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	return fmt.Sprintf("%s$%d.%02d", sign, cents/100, cents%100)
}
