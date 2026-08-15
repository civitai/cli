package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

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
// 🔴 THE PICK ITSELF IS NOT HERE. `highestApprovedVersion` and everything it is
// built from (approvedPeak, appapi.SameSlug, isApprovedStatus, rowIsServing,
// comparableVersion) live in approved_version.go, because `civitai app status`'s
// drift warning — #412's other half, #413 — has to name the SAME version for the
// SAME rows or the two commands contradict each other. They briefly did: both
// PRs open-coded the predicate and the copies disagreed about pre-release
// ordering, slug normalisation and liveness. Read that file for why each
// sub-predicate is shaped the way it is; this file is only what `app submit`
// does with the answer.
//
// The one sub-predicate shared even more widely is the semver ORDERING:
// `semver.compare` in update_check.go is the single comparison this guard, the
// drift warning and the self-update check all call.
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
	// prevent); and every one of those rows then fails appapi.SameSlug, so slugRows is 0
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
