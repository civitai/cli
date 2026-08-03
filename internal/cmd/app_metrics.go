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

CREDENTIAL: the analytics query is full-scope, so it needs a personal API key
(` + "`civitai login --token <key>`" + `); an OAuth browser login is refused with 403.

DATA CAVEAT: engagement counts only AUTHENTICATED, scope-gated API calls. An app
that ships no scoped API surface will show real installs and revenue with a flat
engagement section — that is expected, not a bug.`,
		Example: `  civitai app metrics my-block
  civitai app metrics my-block --from 2026-05-01 --to 2026-08-03
  civitai app metrics my-block --from 2026-05-01T00:00:00Z
  civitai app metrics my-block --json`,
		Args: cobra.ExactArgs(1),
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
				return civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf("no token configured — run `civitai login` (or set CIVITAI_TOKEN)"))
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
	return "", fmt.Errorf("app %q has no approved App Block yet — analytics only exist once a submitted version is approved; "+
		"check where it is in review with `civitai app status %s`", slug, slug)
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
	fmt.Fprintf(tw, "  Total\t%d\n", a.Installs.Total)
	fmt.Fprintf(tw, "  Active\t%d\n", a.Installs.Active)
	_ = tw.Flush()

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
			"Engagement counts only authenticated, scope-gated API calls — an app with no scoped API surface reads flat here."))
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
