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
	// live records whether the winning row's deployState says it is serving. The
	// refusal only claims "and live" when this is true — an approved row that has
	// not finished deploying is approved, and saying more than that would be a
	// false claim printed at the moment someone is deciding whether to trust it.
	live bool
	// found is false when no approved row was comparable at all.
	found bool
	// skipped lists approved versions that could NOT be ordered (see
	// comparableVersion). They are reported, never silently dropped.
	skipped []string
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
		if s.BlockID != slug || !strings.EqualFold(strings.TrimSpace(s.Status), "approved") {
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
		peak.live = s.DeployState != nil && strings.EqualFold(strings.TrimSpace(*s.DeployState), "live")
	}
	return peak
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

// versionRegressionError builds the refusal. The wording is the issue's, with
// two adaptations: the middle line differs for the EQUAL case (nothing is being
// replaced by something older there — the version is simply the one already
// live), and "and live" is claimed only when the approved row's deployState
// really says so.
func versionRegressionError(slug, version string, peak approvedPeak, equal bool) error {
	state := "approved"
	if peak.live {
		state = "approved and live"
	}
	middle := "Submitting an older version replaces the newer deployment on approval."
	if equal {
		middle = "Resubmitting the version that is already live is almost always an accident."
	}
	return civitai.Tag(ErrVersionRegression, fmt.Errorf(
		"refusing to submit %s@%s — %s is already %s.\n%s\nYour repo may be behind what was last released. Pass --allow-downgrade if this is deliberate",
		slug, version, peak.version, state, middle))
}
