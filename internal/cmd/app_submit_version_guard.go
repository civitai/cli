package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/pkg/civitai"
)

// THE MONOTONIC-VERSION GUARD (issue #412).
//
// `civitai app submit` used to accept ANY version string, including one BELOW
// what is already approved and serving. Submitting `0.4.1` while `0.5.2` is live
// was accepted without comment, and on approval replaced the newer deployment
// with older code — while the version number read as an ordinary forward bump,
// so nothing downstream looked wrong. Five first-party apps were in that state
// when the issue was filed, so this is a live trap, not a hypothetical.
//
// The check is entirely CLIENT-SIDE: GET /api/v1/blocks/submissions already
// returns every row's version and status, so the highest approved version is
// computable here and no server change is involved.
//
// 🔴 HIGHEST APPROVED, NOT NEWEST ROW — AND THAT IS A DIFFERENT QUESTION FROM
// THE ONE `ownedSubmission` (apps.go) ANSWERS. That function deliberately takes
// the FIRST row of a newest-first list, because what it renders is "what is the
// state of your latest submission" — an older approved row would misreport a
// newer pending one. Here the question is "what version would this submit
// REPLACE if approved", and only an approved row can be replaced: a later
// pending/withdrawn/rejected row for a HIGHER version outranks the approved one
// by timestamp while being nothing that is serving. The two predicates agree on
// a single-row fixture and disagree on every fixture that matters, which is
// exactly how #390 shipped green — so they are kept apart on purpose rather than
// unified, and the fixtures in app_submit_version_guard_test.go are built so
// the two answers differ.
//
// The one genuinely shared sub-predicate is the semver ORDERING, and that is
// shared rather than re-open-coded: `semver.compare` in update_check.go is the
// single comparison both this guard and the self-update check call.

// ErrVersionRegression classifies the monotonic-version refusal. It carries no
// user-facing text of its own — it is ATTACHED (civitai.Tag) to the
// message-bearing error, so errors.Is reports the KIND while the printed
// message is unchanged.
//
// 🔴 EXIT CODE 1, NOT 2, AND THAT IS DELIBERATE. It is intentionally NOT tagged
// with civitai.ErrBadRequest (the only route to exit 2 from a command error):
// exit 2 is documented as a mistake about the INVOCATION, and every flag,
// argument and path here is well-formed. What is wrong is the PROJECT — the
// manifest's version relative to what is published — which is the same shape as
// an invalid manifest, and exitCodeDocs already publishes a validation verdict
// under code 1. Leaving it untagged for the exit mapper's `default` is what
// produces 1; TestVersionRegressionExitsGeneric (cmd/civitai) pins it so the
// code cannot drift silently.
var ErrVersionRegression = errors.New("submitted version is not greater than the highest approved version")

// The read seam is `submissionLister` (app_pull.go) — the same func type
// `app pull` already uses for the same route, rather than a second interface
// with an identical signature.

// approvedPeak is the highest APPROVED version found for a slug, plus what had
// to be ignored to find it.
type approvedPeak struct {
	// version is the raw version string of the winning row, reproduced verbatim
	// in the refusal so the author sees what the server actually holds.
	version string
	// parsed is that version, ordered.
	parsed semver
	// live records whether the SERVER considers the winning row to be serving.
	// The refusal only claims "and live" when this is true — an approved row that
	// has not finished deploying is approved, and saying more than that would be
	// a false claim printed at the moment someone is deciding whether to trust
	// it.
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
	// comparableVersion). They are reported, never silently dropped.
	skipped []string
	// slugRows counts rows that matched the slug at all, BEFORE the status
	// filter. It exists to tell two very different situations apart, both of
	// which arrive as found=false: "this app has rows, none approved yet" (an
	// ordinary first submit, and silent by design) and "rows came back, none of
	// them for this app" (a blockId mismatch, which would make the guard a
	// no-op with no signal — see checkVersionNotRegression).
	slugRows int
}

// sameSlug compares a listing row's blockId to the manifest's slug.
//
// 🔴 IT NORMALISES, and that is a deliberate reversal of the original exact
// match — but it is DEFENCE IN DEPTH against an undocumented server change, NOT
// a live hazard, and an earlier version of this comment claimed the stronger
// thing. The route as it stands today cannot hand back a mis-cased blockId:
// submissions.ts filters with `where.slug = blockId` (an exact Prisma match) and
// echoes `blockId: row.slug` from the same non-nullable column, so every row it
// returns matches the value asked for byte-for-byte. In the mis-casing scenario
// the server returns ZERO rows, not mis-cased ones — which is the announce
// branch's case, not this one.
//
// What the normalisation buys is the asymmetry: status and deployState were
// already compared case- and whitespace-insensitively while the slug was
// compared byte-for-byte, and the slug is the field whose mismatch is SILENT (a
// non-matching slug lands in the "no approved rows" branch, which proceeds
// without a word because that is what a genuine first submit looks like). So if
// that server contract ever changed, the whole #412 guard would switch off on
// exactly the app it exists to protect, with no output to notice. The check is
// two string compares; keeping it is cheap insurance, and claiming it closes a
// hazard that exists today is not true.
func sameSlug(rowBlockID, slug string) bool {
	return strings.EqualFold(strings.TrimSpace(rowBlockID), strings.TrimSpace(slug))
}

// highestApprovedVersion answers "what version of <slug> is approved and would
// therefore be replaced on approval of this submit".
//
// It matches the slug HERE rather than trusting the ?blockId= narrowing, for the
// same reason ownedSubmission does: a server that ignored the filter would
// otherwise hand back a DIFFERENT app's rows and turn this guard into a
// confident refusal of a perfectly good version.
func highestApprovedVersion(subs []appapi.Submission, slug string) approvedPeak {
	var peak approvedPeak
	for i := range subs {
		s := subs[i]
		if !sameSlug(s.BlockID, slug) {
			continue
		}
		peak.slugRows++
		if !strings.EqualFold(strings.TrimSpace(s.Status), "approved") {
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
// 🔴 THIS IS STRICTER THAN parseSemver ON PURPOSE. parseSemver truncates at the
// first '-' or '+', so it reports `0.5.0-beta.1` and `0.5.0` as the SAME
// version. That is a fine simplification for the self-update check (it only
// decides whether to print a "newer release available" line), and a wrong answer
// here: real semver orders `0.5.0` ABOVE `0.5.0-beta.1`, so a truncating compare
// would refuse a legitimate release-over-its-own-prerelease submit and call it a
// regression. Rather than vendor a full semver implementation for a case the
// manifest schema does not even encourage, a version carrying pre-release or
// build metadata is declared NOT COMPARABLE and routed to the warn-and-proceed
// branch, where the author is told the guard could not order it.
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

// checkVersionNotRegression refuses a submit whose version is not strictly
// greater than the highest approved version of the same app.
//
// 🔴 EVERY NON-REFUSAL BRANCH IS DELIBERATE, AND NONE OF THEM IS SILENT EXCEPT
// THE TWO THAT CARRY NO INFORMATION:
//
//   - --allow-downgrade: returns immediately, BEFORE the network call. A
//     deliberate rollback is legitimate, and paying a round trip to compute an
//     answer we would discard is pure latency on the path a release script takes.
//   - ListSubmissions failed: WARN and proceed. This guard is an accident
//     preventer, not an authorization check, and hard-failing here would mean a
//     transient API blip blocks every submit in the fleet — a strictly worse
//     failure than the one it prevents. The warning is what keeps it from being
//     a silent fail-open.
//   - no approved rows: proceed SILENTLY. A first submit is the normal case and
//     there is nothing to compare against; a warning there would be noise on the
//     happy path.
//   - the local version, or every approved version, cannot be ordered: WARN and
//     proceed, naming what could not be ordered. Refusing would block any app on
//     a pre-release tag; proceeding quietly would be a fail-open.
func checkVersionNotRegression(ctx context.Context, lister submissionLister, warn io.Writer, slug, version string, allowDowngrade bool) error {
	if allowDowngrade {
		return nil
	}
	// 🔴 NO SLUG, NO GUARD — AND SAY THE TRUE THING ABOUT IT. manifest.Load does
	// not require blockId and `--skip-validate` waives the schema, so `slug` can
	// legitimately arrive EMPTY here. Two consequences, both bad, and both were
	// live before this branch existed: submissionsURL omits an empty blockId, so
	// the read is UNNARROWED and returns every app's rows (which the route caps,
	// exactly the failure TestVersionGuardNarrowsTheListingToThisApp exists to
	// prevent); and every one of those rows then fails sameSlug, so slugRows is 0
	// and the mismatch announcement below fires with the app's name spliced in as
	// an empty string — "none of them for  — … If  is already published, this is
	// a blockId mismatch". That misdiagnoses a manifest with NO blockId as a
	// blockId MISMATCH. Returning here is both the cheaper and the truthful
	// answer: there is no app to compare against.
	if strings.TrimSpace(slug) == "" {
		warnf(warn, "this manifest declares no blockId, so the version guard has no app to compare against — submitting without it.")
		return nil
	}
	subs, err := lister(ctx, slug)
	if err != nil {
		warnf(warn, "could not check the highest approved version of %s (%v) — submitting without the version guard.", slug, err)
		return nil
	}
	peak := highestApprovedVersion(subs, slug)
	if len(peak.skipped) > 0 {
		warnf(warn, "ignoring %d approved version(s) of %s that carry pre-release/build metadata and cannot be ordered: %s.",
			len(peak.skipped), slug, strings.Join(peak.skipped, ", "))
	}
	if !peak.found {
		// 🔴 THE ONE SILENT BRANCH, MADE CONDITIONAL. "No approved rows" is
		// silent because a first submit is the normal case — but it is also
		// what a slug that failed to match looks like, and that reading is a
		// dead guard rather than a happy path. The two are distinguishable:
		// a first submit has no rows FOR THIS APP, while a mismatch has rows
		// that are for something else. Announce only the second.
		//
		// SCOPE, honestly: against the route as it is documented today this is
		// UNREACHABLE, because `?blockId=` narrows server-side (`where.slug =
		// blockId`) — so a listing that came back non-empty came back narrowed,
		// and every row in it is for this app. It is defence in depth against a
		// server that stopped narrowing or started returning a different
		// identifier, not a live hazard, and the empty-slug case that DID reach
		// it (an unnarrowed read from a manifest with no blockId) is returned
		// above rather than misdiagnosed here as a mismatch.
		if len(subs) > 0 && peak.slugRows == 0 {
			warnf(warn, "the submissions listing returned %d row(s), none of them for %s — the version guard "+
				"has nothing to compare against. If %s is already published, this is a blockId mismatch rather "+
				"than a first submit.", len(subs), slug, slug)
		}
		return nil
	}
	local, ok := comparableVersion(version)
	if !ok {
		warnf(warn, "cannot order %s@%s against the approved %s (pre-release/build metadata) — submitting without the version guard.",
			slug, version, peak.version)
		return nil
	}
	if local.compare(peak.parsed) > 0 {
		return nil
	}
	return versionRegressionError(slug, version, peak, local.compare(peak.parsed) == 0)
}

// warnf prints one ui-styled warning line to w.
func warnf(w io.Writer, format string, a ...any) {
	if w == nil {
		return
	}
	fmt.Fprintln(w, ui.For(w).Warn(fmt.Sprintf(format, a...)))
}

// versionRegressionError builds the refusal.
//
// 🔴 EVERY LINE IS CONDITIONAL ON WHAT IS ACTUALLY KNOWN — peak.live and
// equal-vs-lower — because this message is read at the moment someone is
// deciding whether the tool is right about their repo, and it used to
// contradict itself inside three lines. The first line already dropped "and
// live" for a non-live row, while the second hard-coded "the version that is
// already live" and the third said "your repo may be behind what was last
// released": against an approved row whose deploy FAILED, at the same local
// version, line 1 said not-live, line 2 said live, and line 3 said behind when
// the repo is exactly at it. That combination is not a corner case — it is the
// approved-but-deploy-failed RETRY path, the one case in this whole guard where
// resubmitting the same version is a plausible deliberate act rather than an
// accident, and calling it an accident is precisely the wrong steer.
//
// It still REFUSES there: #412's contract is that the escape hatch is explicit,
// and --allow-downgrade is the documented way to say "yes, again, on purpose".
// What changes is that the text names the retry instead of denying it.
//
// 🔴 FOUR ARMS, NOT THREE, AND THE FOURTH IS WHY. The lower-version case used to
// be one arm that said "replaces the newer deployment on approval" whatever
// peak.live held — the same false-claim class the equal arm had just been fixed
// for, left standing one branch over. Against an approved row that is NOT
// serving there is no deployment of that version to replace, so the sentence
// asserts something the guard has just established is not the case. Each arm now
// states only what its own combination of (equal|lower) × (live|not) knows.
//
// The two claims deliberately NOT made anywhere here: that nothing at all is
// deployed for this app (a LOWER approved version can be serving while the peak
// is not — this route does not say, and the guard does not look), and that the
// bundle being resubmitted is unchanged (#411: the tree may be dirty; the CLI
// packages what is on disk, so "unchanged" was never ours to assert).
func versionRegressionError(slug, version string, peak approvedPeak, equal bool) error {
	state := "approved"
	if peak.live {
		state = "approved and live"
	}
	var middle, tail string
	switch {
	case equal && peak.live:
		middle = "Resubmitting the version that is already live is almost always an accident."
		tail = "Bump the version in your manifest, or pass --allow-downgrade if this is deliberate"
	case equal:
		// Approved, NOT serving: a deploy that has not landed. Do not call this
		// an accident, and do not tell the author their repo is behind — it is
		// at exactly the version the server holds. "has not landed" rather than
		// "never landed": this arm also covers `building` and `deploying`, where
		// the deploy is IN PROGRESS and may land in a moment.
		middle = "That version is approved but not live, so this may be a deliberate resubmit of a deploy that has not landed."
		// NOT "resubmit it unchanged" — see the note above.
		tail = "Bump the version in your manifest, or pass --allow-downgrade to submit this version again"
	case peak.live:
		middle = "Submitting an older version replaces the newer live deployment on approval."
		tail = "Your repo may be behind what was last released. Pass --allow-downgrade if this is deliberate"
	default:
		// Lower, and the peak is NOT serving. Nothing of that version is
		// deployed, so approval does not replace a deployment of it — saying it
		// does would be the equal arm's old bug in the branch next door.
		//
		// 🔴 AND NOT "supersedes it as the highest approved version" EITHER,
		// which is what the first draft of this arm said and is FALSE by this
		// guard's own definition: highestApprovedVersion takes the MAXIMUM, not
		// the newest, so approving 0.4.1 under an approved 0.5.2 leaves 0.5.2
		// the highest approved. What approval really does is deploy the code of
		// the version being approved, which is older than the peak — and that
		// is a statement about code, so that is what it says.
		middle = "That version is approved but not live, so approving an older one deploys code older than the highest approved version."
		tail = "Your repo may be behind the highest approved version. Pass --allow-downgrade if this is deliberate"
	}
	return civitai.Tag(ErrVersionRegression, fmt.Errorf(
		"refusing to submit %s@%s — %s is already %s.\n%s\n%s",
		slug, version, peak.version, state, middle, tail))
}
