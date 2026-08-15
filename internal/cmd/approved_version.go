package cmd

import (
	"strings"

	"github.com/civitai/cli/internal/appapi"
)

// THE HIGHEST-APPROVED-VERSION PREDICATE (issue #412) — ONE COPY, TWO CALLERS.
//
// #412 has two halves and they shipped as two PRs:
//
//   - `civitai app submit` REFUSES a version that is not strictly above the
//     highest approved one (#414, app_submit_version_guard.go);
//   - `civitai app status` WARNS when the local manifest is BEHIND it
//     (#413, app_status.go).
//
// 🔴 THEY MUST NAME THE SAME NUMBER FOR THE SAME ROWS, OR THEY CONTRADICT EACH
// OTHER — and they briefly did. Both PRs open-coded this predicate against the
// same route, independently, and the two copies disagreed on three things: how a
// pre-release version orders (truncate-at-'-' vs not-orderable), whether the
// slug compare normalises, and whether the winning row's LIVE state is read at
// all. With an approved `0.6.0-rc.1` beside an approved `0.5.2` that produced a
// live contradiction — `app status` reporting the repo BEHIND `0.6.0-rc.1` while
// `app submit` happily accepted `0.5.3`. This file is the consolidation: both
// commands call this function, so a disagreement of that shape is no longer
// expressible. TestStatusAndSubmitNameTheSameVersion pins it.
//
// The read seam both callers use is `submissionLister` (app_pull.go) — the same
// func type `app pull` already uses for the same route.

// approvedPeak is the highest APPROVED version found for a slug, plus what had
// to be ignored to find it.
//
// A struct rather than the `(string, bool)` pair one of the two original copies
// returned: `app submit`'s refusal has to state whether the winning row is
// SERVING and which rows it could not order, and a caller that only wants the
// version reads `.version` / `.found`. Widening the pair would have meant a
// second, subtly-different picker for the fields the other caller needs.
type approvedPeak struct {
	// version is the raw version string of the winning row, reproduced verbatim
	// by callers so the author sees what the server actually holds.
	version string
	// parsed is that version, ordered.
	parsed semver
	// live records whether the SERVER considers the winning row to be serving.
	// `app submit`'s refusal only claims "and live" when this is true — an
	// approved row that has not finished deploying is approved, and saying more
	// than that would be a false claim printed at the moment someone is deciding
	// whether to trust it.
	//
	// 🔴 IT IS READ FROM liveUrl FIRST, AND deployState IS THE FALLBACK, NOT THE
	// OTHER WAY ROUND. `deployState == "live"` is NOT the server's definition of
	// serving: the shared predicate isApprovedAndServing
	// (civitai:src/shared/constants/app-block-deploy.constants.ts) also returns
	// TRUE for an approved row whose deployState is NULL when it carries a
	// deployUpdatedAt, has no reviewedAt, or was reviewed before the
	// deploy-state-tracking epoch — i.e. every LEGACY approval that predates
	// tracking. shapeRow (civitai:src/pages/api/v1/blocks/submissions.ts) emits
	// `liveUrl` from exactly that predicate, so a non-nil liveUrl IS the server
	// saying "this row is serving", and it is the only field on this route that
	// says it.
	//
	// Reading deployState alone made the guard print "approved but not live" —
	// and recommend --allow-downgrade as a retry — for a row that is serving at
	// a URL the same CLI prints under `Live at:`. That is a positive false claim
	// steering the author straight into the #412 accident this guard exists to
	// prevent. deployState is kept as a second source so a server that stopped
	// emitting liveUrl cannot silently turn the live claim off.
	live bool
	// found is false when no approved row was comparable at all.
	found bool
	// skipped lists approved versions that could NOT be ordered (see
	// comparableVersion). `app submit` reports them; `app status` stays silent
	// about them, because its whole contract is that a fact it could not
	// establish is not spoken. Either way they are never silently ranked.
	skipped []string
	// slugRows counts rows that matched the slug at all, BEFORE the status
	// filter. It exists to tell two very different situations apart, both of
	// which arrive as found=false: "this app has rows, none approved yet" (an
	// ordinary first submit, and silent by design) and "rows came back, none of
	// them for this app" (a blockId mismatch, which would make the guard a
	// no-op with no signal — see checkVersionNotRegression).
	slugRows int
}

// highestApprovedVersion answers "what version of <slug> is approved, and would
// therefore be replaced on approval of a submit / is what a local repo can be
// BEHIND".
//
// 🔴 APPROVED, NOT THE NEWEST ROW — and the two genuinely differ (#390). The
// newest row by submittedAt can be a `pending` resubmission, or a `withdrawn`
// duplicate that outranks the approved row by timestamp; neither is code anyone
// is running. That is a DIFFERENT question from the one `ownedSubmission`
// (apps.go) answers — it deliberately takes the FIRST row of a newest-first list
// because it renders "the state of your latest submission" — so the two
// predicates are kept apart on purpose, and the fixtures are built so their
// answers differ.
//
// 🔴 AND IT IS ORDER-INDEPENDENT, unlike every other read of this route in this
// package. A maximum over a filtered set does not care which end of the list it
// starts from, so this pick cannot inherit the newest-first assumption the
// submissions-route ledger (newest_row_pick_test.go) exists to track. Pinned by
// a permuted-fixture case, not merely asserted here.
//
// It matches the slug HERE rather than trusting the ?blockId= narrowing, for the
// same reason ownedSubmission does: a server that ignored the filter would
// otherwise hand back a DIFFERENT app's rows and turn this into a confident
// refusal — or a fabricated drift warning — against a perfectly good version.
func highestApprovedVersion(subs []appapi.Submission, slug string) approvedPeak {
	var peak approvedPeak
	for i := range subs {
		s := subs[i]
		if !sameSlug(s.BlockID, slug) {
			continue
		}
		peak.slugRows++
		if !isApprovedStatus(s.Status) {
			continue
		}
		v, ok := comparableVersion(s.Version)
		if !ok {
			peak.skipped = append(peak.skipped, s.Version)
			continue
		}
		if peak.found && v.compare(peak.parsed) <= 0 {
			continue
		}
		peak.version, peak.parsed, peak.found = s.Version, v, true
		peak.live = rowIsServing(s)
	}
	return peak
}

// sameSlug compares a listing row's blockId to the manifest's slug.
//
// 🔴 IT NORMALISES, and that is a deliberate reversal of one copy's original
// exact match — but it is DEFENCE IN DEPTH against an undocumented server
// change, NOT a live hazard, and an earlier version of this comment claimed the
// stronger thing. The route as it stands today cannot hand back a mis-cased
// blockId: submissions.ts filters with `where.slug = blockId` (an exact Prisma
// match) and echoes `blockId: row.slug` from the same non-nullable column, so
// every row it returns matches the value asked for byte-for-byte. In the
// mis-casing scenario the server returns ZERO rows, not mis-cased ones — which
// is the announce branch's case, not this one.
//
// What the normalisation buys is the asymmetry: status and deployState were
// already compared case- and whitespace-insensitively while the slug was
// compared byte-for-byte, and the slug is the field whose mismatch is SILENT (a
// non-matching slug lands in the "no approved rows" branch, which proceeds
// without a word because that is what a genuine first submit looks like). So if
// that server contract ever changed, the whole #412 feature would switch off on
// exactly the app it exists to protect, with no output to notice. The check is
// two string compares; keeping it is cheap insurance, and claiming it closes a
// hazard that exists today is not true.
func sameSlug(rowBlockID, slug string) bool {
	return strings.EqualFold(strings.TrimSpace(rowBlockID), strings.TrimSpace(slug))
}

// isApprovedStatus reports whether a submission row's status means APPROVED.
//
// 🔴 NORMALISED, NOT COMPARED RAW. `s.Status == "approved"` is a case- and
// space-sensitive test against a value this CLI does not own: the server picks
// the casing. A raw comparison does not fail loudly if the server ever answers
// "Approved" — it silently matches NOTHING, the highest-approved lookup finds no
// rows, and BOTH halves of #412 go permanently quiet: the drift warning stops
// printing and the submit guard stops refusing. A check that can only ever fail
// by going inert has to be the tolerant one.
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
// inconsistency is real and deliberately OUT OF SCOPE: this is the #412
// predicate, not a status-folding consolidation across the binary.
func isApprovedStatus(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "approved")
}

// rowIsServing reports whether the SERVER says this row is serving.
//
// See approvedPeak.live for why liveUrl leads: it is the field the server
// computes from its own isApprovedAndServing predicate, and that predicate is
// TRUE for legacy approvals whose deployState is null. A guard that read only
// deployState called those rows "not live" — a false claim, and one that
// contradicted `civitai app status`, which renders `Live at:` from this same
// liveUrl.
//
// deployState is retained as a fallback rather than deleted: it is the field the
// route documents for the live state, and keeping it means a server that stops
// emitting liveUrl degrades to the old (understated) answer instead of claiming
// nothing is ever live.
func rowIsServing(s appapi.Submission) bool {
	if s.LiveURL != nil && strings.TrimSpace(*s.LiveURL) != "" {
		return true
	}
	return s.DeployState != nil && strings.EqualFold(strings.TrimSpace(*s.DeployState), "live")
}

// comparableVersion parses s ONLY when it can be ordered without losing
// information.
//
// 🔴 THIS IS STRICTER THAN parseSemver ON PURPOSE, AND IT IS THE POLICY BOTH
// #412 COMMANDS NOW SHARE. parseSemver truncates at the first '-' or '+', so it
// reports `0.5.0-beta.1` and `0.5.0` as the SAME version, and ranks
// `0.6.0-rc.1` ABOVE `0.5.2`. That is a fine simplification for the self-update
// check (it only decides whether to print a "newer release available" line) and
// a wrong answer here, in BOTH directions:
//
//   - real semver orders `0.5.0` ABOVE `0.5.0-beta.1`, so a truncating compare
//     would make `app submit` refuse a legitimate release-over-its-own-prerelease
//     submit and call it a regression;
//   - and it would make `app status` quote `0.6.0-rc.1` — a version that is
//     approved but whose ordering against `0.5.2` this CLI cannot honestly
//     establish — as "the highest APPROVED version" the repo is behind.
//
// Rather than vendor a full semver implementation for a case the manifest schema
// does not even encourage, a version carrying pre-release or build metadata is
// declared NOT COMPARABLE. `app submit` routes it to a warn-and-proceed branch
// that names what could not be ordered; `app status` stays silent, which is its
// contract for every fact it could not establish. Neither invents an order.
func comparableVersion(s string) (semver, bool) {
	// 🔴 THIS TRIM IS REDUNDANT, AND SAYING SO IS THE POINT. parseSemver does
	// its own TrimSpace + TrimPrefix(…, "v"), so removing this line changes the
	// answer for NO realistic input — measured over a 39-string corpus, 0
	// diverge; the only class that differs is a doubled prefix ("vv0.5.2"),
	// which this line accepts and parseSemver alone rejects. It is kept as a
	// local guarantee that `t` below is prefix-free before the "-+" scan, but
	// no test can pin it, and a reader must not mistake it for load-bearing:
	// the v-prefix contract is pinned end-to-end by
	// TestVersionGuardOrdersAVPrefixedApprovedVersion, which holds against
	// either spelling.
	t := strings.TrimPrefix(strings.TrimSpace(s), "v")
	if strings.ContainsAny(t, "-+") {
		return semver{}, false
	}
	return parseSemver(t)
}
