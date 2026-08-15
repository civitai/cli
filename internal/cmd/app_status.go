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
	//
	// 🔴 THIS IS THE SHARED PREDICATE (appapi.SameSlug) ANSWERING A DIFFERENT
	// QUESTION FROM THE OTHER THREE CALLERS, and the difference is worth stating
	// because it is what decided the call. The other three ask "does this row
	// match the slug the CALLER NAMED", request against server row. This one
	// asks "is the project directory I am standing in the same app as this row"
	// — LOCAL FILE against server row. It is the same underlying question ("are
	// these two spellings the same slug?"), so it gets the same predicate.
	//
	// 🔴 AND IT IS THE CALLER WITH THE STRONGEST ARGUMENT FOR NORMALISING, not
	// the weakest. At the other three the unnormalised value could only come
	// from the server, which by its own route contract cannot emit one (see
	// slug.go). Here it comes from block.manifest.json via manifest.Load, which
	// is a bare json.Unmarshal with NO schema validation — so a hand-edited
	// `"blockId": " Custom-Generators "` reaches this line today, with no server
	// change required. Still not a demonstrated live break: a manifest spelled
	// that way is rejected by `civitai app submit`'s own validation, so an
	// author holding one has not successfully submitted under it.
	//
	// The failure it removes is the silent one. An exact compare returns here
	// and the drift warning never prints, which is indistinguishable from
	// "nothing is approved yet" — the #412 hazard exactly. The false-positive
	// direction is closed by the schema: valid slugs are lowercase and unpadded,
	// so a fold-and-trim match cannot join two DIFFERENT apps, only a
	// mis-spelling to the app it mis-spells.
	//
	// The hand-written `m.BlockID == ""` clause this replaces is now
	// SameSlug's own guarantee (it rejects an empty side, including
	// empty-against-empty, and rejects all-whitespace too, which the old clause
	// did not). Note also that what gets PRINTED below is s.BlockID — the
	// server's spelling — so a mis-spelled manifest can never put its own
	// spelling in the warning.
	if !appapi.SameSlug(m.BlockID, s.BlockID) {
		return
	}
	// 🔴 comparableVersion, NOT isParseableVersion — the SAME strictness the
	// `app submit` guard applies, and the reason the two commands can no longer
	// disagree. isParseableVersion truncates at the first '-'/'+' (see
	// approved_version.go), so a local `0.6.0-rc.1` would be ordered here as a
	// bare `0.6.0` while the submit guard declares it unorderable and says so.
	// One of those two answers has to go, and the truncating one is the one that
	// invents an order.
	if _, ok := comparableVersion(m.Version); !ok {
		return
	}
	subs, err := lister(ctx, s.BlockID)
	if err != nil {
		return
	}
	// 🔴 THE SHARED PICK (approved_version.go). `peak.skipped` is deliberately
	// NOT reported here, unlike in `app submit`: this line is advisory and its
	// whole contract is that a fact it could not establish is not spoken, so an
	// approved version it could not order is a silence, not a caveat. What
	// matters is that both commands SKIP the same rows — the two agree on the
	// number even where only one of them talks about the remainder.
	peak := highestApprovedVersion(subs, s.BlockID)
	// 🔴 OUTPUT-REDUNDANT, MEASURED, AND KEPT — with the reason stated instead of
	// implied. Deleting this return SURVIVES the whole suite: peak.version is ""
	// when found is false, and versionDriftWarning's own guard rejects "" (via
	// comparableVersion -> parseSemver, which fails on the empty string), so the
	// silence happens either way. It is an EQUIVALENT mutant, not a coverage hole
	// — the mirror image of the guard inside versionDriftWarning, which is itself
	// output-redundant against THIS line and is killable only at the direct-call
	// seam. Neither is dead; each covers the other's caller, and only one of them
	// is inherited by a future caller of versionDriftWarning.
	if !peak.found {
		return
	}
	if msg := versionDriftWarning(errOut, s.BlockID, m.Version, peak.version); msg != "" {
		fmt.Fprint(errOut, msg)
	}
}

// The highest-approved pick and the "is this row approved" predicate used to
// live HERE, in a second copy written independently of `app submit`'s. Both are
// now in approved_version.go, shared by both halves of #412 — see the header
// there for why two copies of one predicate is the bug and not the tidiness nit
// it looks like.

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
// 🔴 BOTH ARGUMENTS MUST BE ORDERABLE, AND THAT IS ENFORCED HERE, not only at
// the call site. compareVersions treats an unparseable FIRST argument as OLDER
// (so `civitai version` surfaces any real release), which through this function
// reads as a confident "your repo is BEHIND" for a manifest carrying a typo —
// and for an EMPTY version string it renders `local block.manifest.json is  —
// BEHIND …`, with the hole where the version should be. warnLocalVersionDrift
// gates before ever calling this, so the guard below is OUTPUT-REDUNDANT on
// every path that exists today. It is not dead: it is here because THIS is the
// reusable half, and the property "no false BEHIND" belonged to the caller,
// which meant the next caller inherited none of it. Kept, and pinned directly by
// TestVersionDriftWarningRefusesUnparseableVersionsItself — a guard nothing can
// reach through the command has to be reachable through the seam, or it is a
// comment pretending to be code.
//
// 🔴 THE COMPARISON IS comparableVersion + semver.compare, NOT
// isParseableVersion + compareVersions, AND THAT CHANGED THE BEHAVIOUR — the
// note that used to sit here described the old policy and is now wrong, so it is
// replaced rather than left standing. parseSemver (and therefore
// compareVersions) truncates at the first '-'/'+', which reduced a
// pre-release to its numeric triple and ORDERED it:
//
//   - it read a local `0.5.2-rc1` as EQUAL to a published `0.5.2` (silent — the
//     safe direction, and the only part of the old behaviour that survives, now
//     for the different reason that it is not orderable at all);
//   - it read a local `0.4.0-3-gabc123` as `0.4.0` and WARNED that the repo was
//     behind — an ordering it had no basis for;
//   - and on the published side it would have quoted an approved `0.6.0-rc.1` as
//     "the highest APPROVED version", ranking it above an approved `0.5.2`,
//     which is the reverse of real semver.
//
// That last one is not hypothetical: `app submit`'s guard declares such a
// version unorderable and refuses to rank it, so the two commands named
// DIFFERENT versions for the same rows — exactly the contradiction the doc
// comment on highestApprovedVersion warns about. Both now use comparableVersion.
//
// USER-VISIBLE CONSEQUENCE: where the highest approved version, or the local
// one, carries a pre-release/build suffix, `app status` is now SILENT rather
// than warning. That is a loss of one true-ish warning (the git-describe case
// above) in exchange for never stating an order this CLI cannot justify, and it
// is the direction the whole feature is built in — every fact it cannot
// establish is a silence. `app submit` is louder in the same situation because
// it is about to ACT: it names what it could not order and proceeds.
func versionDriftWarning(w io.Writer, blockID, local, published string) string {
	lv, lok := comparableVersion(local)
	pv, pok := comparableVersion(published)
	if !lok || !pok {
		return ""
	}
	if lv.compare(pv) >= 0 {
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
