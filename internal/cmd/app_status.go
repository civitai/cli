package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/manifest"
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

When a single submission is requested AND the current directory holds a
block.manifest.json for that same app, the local manifest version is compared
against your highest APPROVED version. If the repo is BEHIND, a warning is
printed on stderr — a repo behind its own live deployment is how an accidental
downgrade gets submitted. It is advisory only: the exit code never changes, and
nothing is said when the versions cannot be compared.

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
				// 🔴 …Rows, not GetSubmission: the `?blockId=` spelling of this
				// route answers with the app's whole narrowed listing, which is
				// byte-for-byte the request the drift check would otherwise
				// issue a second time. Take the rows here and the drift check
				// costs no request at all on the slug path. `rows` is nil on the
				// `--id` path (a single-row envelope, no listing), and there the
				// check does still have to ask — see driftLister.
				sub, rows, err := client.GetSubmissionRows(ctx, idFlag, blockID)
				if err != nil {
					return err
				}
				// 🔴 RENDER FIRST. The drift line is ADVISORY and may not even
				// print; the answer below is already in hand. Running the check
				// ahead of the render made every `app status <slug>` wait on a
				// second, optional round trip before showing an answer it had
				// already fetched — on a hung or throttled API that is an
				// arbitrary delay for a line the user may never see.
				//
				// The stderr/stdout split is untouched and is a separate
				// property: the warning goes to STDERR on both renderings, so
				// `--json` stdout stays a pure parseable payload. Ordering is
				// about WHEN, purity is about WHICH STREAM — moving the call
				// changes only the former.
				if jsonOut {
					if err := writeJSON(out, sub); err != nil {
						return err
					}
				} else {
					printSubmissionDetail(out, sub)
				}
				// The advisory call gets its own, shorter deadline: the client's
				// 30s budget is sized for the answer, and an optional extra is
				// not allowed to hold the process open for it.
				dctx, cancel := context.WithTimeout(ctx, driftLookupTimeout)
				defer cancel()
				warnLocalVersionDrift(dctx, driftLister(client, rows), cmd.ErrOrStderr(), sub)
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
			//
			// 🔴 AND THAT IS WHY THE NOUN IS "the server returned", NOT
			// "showing". The order above is right and the number is right;
			// what was wrong was the verb. With `--limit 5` against an at-cap
			// page the table below prints FIVE rows while this line said
			// "showing the newest 100 submissions" — two contradicting claims
			// on two streams of the same run. The count belongs to the API; the
			// sentence must attribute it there.
			if submissionsListTruncated(len(subs)) {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"note: the server returned the newest %d submissions — the API caps this listing and offers no way to page, "+
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

// ---------------------------------------------------------------------------
// Local-vs-published version drift (issue #412)
// ---------------------------------------------------------------------------
//
// A repo can fall BEHIND its own live deployment — five first-party apps were in
// that state when #412 was written — and nothing in the CLI said so. Submitting
// from such a repo is accepted and, on approval, replaces newer code with older
// code while the version number reads as an ordinary forward bump.
//
// This is the WARNING half of #412 (the refusal half lives in `app submit`). It
// is advisory by construction: it prints and returns, and every failure to
// establish the facts degrades to SILENCE. A false "your repo is behind" would
// be worse than saying nothing, because the remedy it implies — do not submit,
// go pull the released code — is expensive and wrong.

// driftManifestDir is where the drift check looks for the local manifest.
// `app status` takes no --dir/--path, so the check is scoped to exactly the
// situation #412 describes: run from inside your app directory. Outside one,
// manifest.Load fails and the check says nothing.
const driftManifestDir = "."

// driftLookupTimeout bounds the drift check's own request, when it makes one.
// The appapi client's budget (30s) is the budget for the ANSWER; this call is
// advisory and runs after the answer has already been printed, so it gets a
// tighter one — a hung listing must not keep the command alive for a line that
// may not print.
//
// 🔴 A `var`, not a `const`, and only so a test can SHRINK it. Setting it to 1ns
// already killed a dozen tests, but that only proves the value is WIRED — the
// removal direction (delete the WithTimeout, hand warnLocalVersionDrift the
// parent ctx) survived the whole suite, because nothing observed that the
// advisory request's deadline is SHORTER than the command's. Nothing else in the
// binary writes this, and a deadline is not observable from the server side of
// an httptest handler, so shrinking it in-process is the only way to tell a
// dedicated 10s budget from an inherited one. Pinned by
// TestAppStatusDriftLookupGetsADeadlineOfItsOwn.
var driftLookupTimeout = 10 * time.Second

// driftLister decides whether the drift check needs a request of its own.
//
// 🔴 THE SLUG PATH ALREADY HOLDS THE ROWS. `?blockId=` returns the app's whole
// narrowed listing and GetSubmissionRows hands it back, so re-fetching it would
// be the byte-identical GET (submissionsURL("", blockID)) a second time in one
// command — pure latency and rate-limit budget for data in memory.
//
// rows == nil means no listing was read (the `?id=` path answers with a
// single-row envelope), and there the real lister is the only way to see the
// app's other versions. nil is NOT "the listing was empty": an empty listing on
// the slug path is a not-found error and never reaches here.
func driftLister(client *appapi.Client, rows []appapi.Submission) submissionLister {
	if rows == nil {
		return client.ListSubmissions
	}
	return func(context.Context, string) ([]appapi.Submission, error) { return rows, nil }
}

// warnLocalVersionDrift writes the #412 drift warning to errOut when the local
// block.manifest.json is BEHIND the caller's highest APPROVED version of the
// same app. It writes nothing in every other case.
//
// 🔴 THE ORDER OF THE GATES IS LOAD-BEARING, AND THE MANIFEST IS FIRST. Reading
// the local manifest is free and offline; only once it exists, parses, names
// THIS app and carries a comparable version does the check spend a second HTTP
// round trip. So a caller who is not standing in the matching app directory —
// which is most callers of `app status <slug>` — pays nothing at all, and the
// command's request count is unchanged for them.
//
// Every branch below returns silently. That is the whole safety argument: no
// manifest, an unreadable or malformed one, a manifest for a DIFFERENT app, an
// unparseable version on either side, a failed or forbidden listing, or an app
// with nothing approved yet all mean "we could not establish drift", which is
// not the same thing as "there is none" — and only the first of those may be
// stated.
func warnLocalVersionDrift(ctx context.Context, lister submissionLister, errOut io.Writer, s *appapi.Submission) {
	if s == nil || s.BlockID == "" {
		return
	}
	m, err := manifest.Load(driftManifestDir)
	if err != nil || m == nil {
		return
	}
	// The manifest must be for THIS app. Without this, running `civitai app
	// status some-other-app` from inside an unrelated project would compare two
	// different apps' versions and report a fabricated regression.
	if m.BlockID == "" || m.BlockID != s.BlockID {
		return
	}
	if !isParseableVersion(m.Version) {
		return
	}
	subs, err := lister(ctx, s.BlockID)
	if err != nil {
		return
	}
	published, ok := highestApprovedVersion(subs, s.BlockID)
	if !ok {
		return
	}
	if msg := versionDriftWarning(errOut, s.BlockID, m.Version, published); msg != "" {
		fmt.Fprint(errOut, msg)
	}
}

// highestApprovedVersion returns the greatest APPROVED version among subs for
// blockID, by semver order.
//
// 🔴 APPROVED, NOT THE NEWEST ROW — and the two genuinely differ (#390). The
// newest row by submittedAt can be a `pending` resubmission, or a `withdrawn`
// duplicate that outranks the approved row by timestamp; neither is code anyone
// is running. Approval is what puts a version into the deploy pipeline, so the
// highest approved version is the thing a local repo can be BEHIND, and it is
// also the reference `app submit`'s monotonic guard refuses against — the two
// commands have to name the same number or they contradict each other.
//
// 🔴 AND IT IS ORDER-INDEPENDENT, unlike every other read of this route in this
// package. A maximum over a filtered set does not care which end of the list it
// starts from, so this pick cannot inherit the newest-first assumption the
// submissions-route ledger exists to track. Pinned by a permuted-fixture case,
// not merely asserted here.
//
// Rows for another blockId are skipped defensively (a server that ignored the
// ?blockId= narrowing would otherwise contribute someone else's versions), and
// so is any approved row whose version is not comparable semver — a value we
// cannot order must not become the number a warning quotes.
func highestApprovedVersion(subs []appapi.Submission, blockID string) (string, bool) {
	best := ""
	for i := range subs {
		s := &subs[i]
		if s.BlockID != blockID || !isApprovedStatus(s.Status) || !isParseableVersion(s.Version) {
			continue
		}
		if best == "" || compareVersions(s.Version, best) > 0 {
			best = s.Version
		}
	}
	return best, best != ""
}

// isApprovedStatus reports whether a submission row's status means APPROVED.
//
// 🔴 NORMALISED, NOT COMPARED RAW. `s.Status == "approved"` is a case- and
// space-sensitive test against a value this CLI does not own: the server picks
// the casing. A raw comparison does not fail loudly if the server ever answers
// "Approved" — it silently matches NOTHING, the highest-approved lookup finds no
// rows, and the drift warning goes permanently quiet. A check that can only ever
// fail by going inert has to be the tolerant one.
//
// 🔴 THE FOLDING IS NOT UNIVERSAL IN THIS BINARY — do not read this comment as a
// claim that it is (it did say so, wrongly, until #413's delta audit). Folded:
// appblocks.go's latestMatchingSubmission (`strings.ToLower(s.Status)`) and
// app_pull.go's pullReviewAdvice (`strings.ToLower(strings.TrimSpace(status))`).
// Raw, and reachable: app_status.go's own `s.Status == "rejected"` render check
// in printSubmissionDetail, and app_listing.go's `ref.Status == "approved"` at
// two sites — the latter reading appapi.ListingRef.Status, a DIFFERENT field
// with the same hazard rather than the same field. So in the very scenario this
// predicate defends against (the server answers "Approved"), this check keeps
// working while `app listing`'s live/not-live branch flips the wrong way. That
// inconsistency is real and deliberately OUT OF SCOPE here — #413 is the drift
// line, not a status-folding consolidation across the binary.
//
// The companion `app submit` monotonic guard (#412's other half) folds the same
// way; the two predicates are meant to be consolidated once both have landed,
// and they cannot be if they disagree about what "approved" is.
func isApprovedStatus(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "approved")
}

// versionDriftWarning renders the warning for a local version that is BEHIND
// published, and the empty string for every other relation.
//
// THREE-WAY, and only one of the three speaks:
//
//   - BEHIND  — the #412 trap. Warn.
//   - AHEAD   — the normal state of a repo about to release. Silence; warning
//     here would train authors to ignore the line that matters.
//   - EQUAL   — the healthy steady state right after a release. Silence.
//     (`app submit` separately refuses an equal resubmit; that is a decision
//     about an action, not a description of a repo, and `status` describing a
//     just-released repo as a problem would be noise on every run.)
//
// 🔴 BOTH ARGUMENTS MUST BE PARSEABLE SEMVER, AND THAT IS ENFORCED HERE, not
// only at the call site. compareVersions treats an unparseable FIRST argument as
// OLDER (so `civitai version` surfaces any real release), which through this
// function reads as a confident "your repo is BEHIND" for a manifest carrying a
// typo — and for an EMPTY version string it renders `local block.manifest.json
// is  — BEHIND …`, with the hole where the version should be. warnLocalVersionDrift
// gates on isParseableVersion before ever calling this, so the guard below is a
// no-op on every path that exists today. It is here because THIS is the
// reusable half: the property "no false BEHIND" belonged to the caller, which
// meant the next caller — the `app submit` guard is the known one — inherited
// none of it. Feeding this function garbage now returns the empty string
// instead of an assertion nobody made.
//
// KNOWN LIMITATION, inherited from the shared comparator and left in place
// deliberately: parseSemver drops any -prerelease/+build suffix, so the ordering
// is over the numeric triple only. A local `0.5.2-rc1` therefore reads as EQUAL
// to a published `0.5.2` and stays silent. That is the safe direction — it can
// under-report drift, never invent it — and the message always quotes the RAW
// manifest string, never the reduced triple. Sharpening it means changing
// parseSemver, which `civitai version` also depends on; that is its own change.
func versionDriftWarning(w io.Writer, blockID, local, published string) string {
	if !isParseableVersion(local) || !isParseableVersion(published) {
		return ""
	}
	if compareVersions(local, published) >= 0 {
		return ""
	}
	st := ui.For(w)
	return st.Warn(fmt.Sprintf(
		"local %s is %s — BEHIND the highest APPROVED version of %s, which is %s.",
		manifest.Filename, local, blockID, published)) + "\n" +
		"  An approved version is what gets deployed, so submitting from this repo would replace newer code on approval.\n" +
		fmt.Sprintf("  Sync the released code (%s) or raise the local version above %s before %s.\n",
			st.Code(driftRemedyCommand(blockID)), published, st.Code("civitai app submit"))
}

// driftRemedyCommand is the pull invocation that syncs THE DIRECTORY THE
// WARNING WAS PRINTED IN.
//
// 🔴 THE `.` IS THE WHOLE POINT AND IT IS NOT OPTIONAL. This warning can only
// appear when cwd IS the app checkout (driftManifestDir is "."), so the remedy
// is always "sync here". `civitai app pull --app <slug>` with no [dir] does
// something else entirely: app_pull defaults the target to ./<slug>, so run
// verbatim from where this line printed it CLONES A SECOND COPY OF THE APP
// NESTED INSIDE THE REPO and leaves the actual checkout exactly as far behind as
// it was. The user follows the advice, sees a success line, and submits the
// downgrade anyway — the precise outcome #412 exists to prevent. (The nested
// copy is not inert either: it is app source inside the packaged directory, so
// the next `app submit` uploads the app twice.)
//
// Pinned by TestDriftRemedyCommandIsRunnableFromTheWarningsOwnDirectory, which
// PARSES this string with the real command tree rather than spelling it out —
// a literal assertion agrees with any string, including one no CLI would accept.
func driftRemedyCommand(blockID string) string {
	return "civitai app pull . --app " + blockID
}

// fullDate renders an RFC3339 timestamp in local time, minute precision.
func fullDate(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Local().Format("2006-01-02 15:04 MST")
}
