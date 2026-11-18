package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/pkg/civitai"
)

// THE LISTING-COMPLETENESS GATE on `civitai app submit` (civitai/cli#762).
//
// MEASURED CAUSE. A credentialed agent trial (`at3`, 2026-09-29) submitted an
// app at step 48, generated an icon and a cover at steps 54–55, attached them at
// 56–57, ran `civitai app doctor` at step 60 — the LAST command of the run, with
// no step left to act on it — read `empty-description` / `empty-tagline` /
// `no-screenshots` as "advisory, not blocking", and stopped. That is a rational
// read of the surfaces it had: the publish FLOOR is icon + cover, so once those
// two were attached every signal it could see said done. Nothing was missing
// from the CLI — `app doctor` already detects all five problems and routes each
// to its fixing command, `app listing set-text` already writes the text, and
// `civitai generate` can produce the images. What was missing was SEQUENCING:
// no moment in the flow made listing work a PRECONDITION of anything the agent
// was optimising for.
//
// 🔴 SO THE GATE IS ON `submit`, BECAUSE "submit succeeded" IS THE THING BEING
// OPTIMISED FOR. A warning printed after a successful submit is what
// printListingFloorHeadsUp already does, and the trial read it and moved on.
//
// 🔴 IT RE-USES `app doctor`'s DETECTION AND ADDS NOTHING OF ITS OWN. The
// problems are the SERVER's (`computeListingProblems`), reached through the same
// `appListings.listMine` read, and the remedy strings and the severity split come
// from doctorPayload — the one place either is computed. This file decides ONE
// new thing: which problems refuse a submit. A second copy of the detection is
// the predicate-at-N-sites shape this repo keeps finding wrong at N−1 of them.
//
// 🔴 WHAT THIS GATE STRUCTURALLY CANNOT CATCH, stated because a gate whose limit
// is unwritten gets quoted as wider than it is. A FIRST submit is what MINTS the
// store listing (`publish-request.service` mints the pre-approval draft), so
// before it there is no listing, no icon slot to fill, and nothing for this gate
// to read — `listMine` returns no row for the app and the gate passes. The
// measured trial's submit WAS a first submit. So this closes the loop from the
// SECOND submit onward: it makes an incomplete listing impossible to keep
// carrying forward, and it does not stop a one-shot submit-and-stop run. The
// scaffold half of the fix (a real `tagline` in block.manifest.json, which the
// draft is minted from) is what reaches the first submit.

// ErrListingIncomplete classifies the listing-completeness refusal. Like
// ErrVersionRegression it carries no user-facing text — it is ATTACHED
// (civitai.Tag) to the message-bearing error, so errors.Is reports the KIND
// while the printed message is unchanged.
//
// 🔴 EXIT 1, AND IT IS A CHOICE RATHER THAN A FALLTHROUGH. Three codes were
// candidates and two are wrong:
//
//   - 2 is documented as a mistake about the INVOCATION. Every flag, argument
//     and path is well-formed here; what is incomplete is the LISTING. Tagging
//     civitai.ErrBadRequest is the only route to 2, so this is deliberately left
//     untagged for the exit mapper's `default`.
//   - 0 is the code this whole defect class is made of. `civitai app submit`
//     ALREADY exits 0 when it did not submit — the no-token path writes the .zip
//     and prints `⚠ NOT SUBMITTED` (printManualNextSteps) — and a refusal that
//     joined it would be indistinguishable from that and from success, in a
//     command an agent is grading itself on. TestSubmitNoTokenPathStillExitsZeroAndSaysNotSubmitted
//     pins the two apart.
//
// So 1: the same code the version guard, the dirty-tree guard and the oversize
// refusal publish, for the same reason — a verdict about the PROJECT rather than
// about the call. TestSubmitListingGateProcessExitStatusEndToEnd (cmd/civitai) pins it at the
// process level, and TestSubmitListingIncompleteCarriesItsOwnSentinel pins the
// sentinel it is carried on.
//
// 🔴 AND IT IS NOT `ErrListingBlocked`. That sentinel is `cmd/civitai`'s
// one-entry whitelist for rendering an error WITHOUT the "Error: " prefix,
// because `app doctor`'s job IS to return a verdict. `app submit`'s job is to
// submit; a refusal there is a failure of the thing the caller asked for, and it
// keeps the prefix like every other submit refusal. Re-using the sentinel would
// have silently un-prefixed it.
var ErrListingIncomplete = errors.New("store listing is incomplete")

// myListingLister is the read seam for `appListings.listMine` —
// appapi.Client.ListMyListings satisfies it. A func type rather than an
// interface, matching submissionLister (app_pull.go), so a test fakes the read
// without a server.
type myListingLister func(ctx context.Context) ([]appapi.MyListing, error)

// submitGateBlocks reports whether ONE problem refuses a submit.
//
// 🔴 THE RULE IS "CAN THE AUTHOR FIX THIS, NOW, WITH A COMMAND THIS CLI SHIPS",
// NOT the server's publish severity. Those are different questions and reading
// one as the other is the whole defect: `empty-description` is `advisory`
// server-side (it is above the publish floor) and is precisely what the trial
// shipped without. Conversely a `blocking` problem whose remedy waits on a
// moderator must NOT refuse a submit — see the blocked-media arm.
//
// The table, and why each row is where it is:
//
//	missing-icon        BLOCK   `set-icon` works on every listing status and for
//	missing-cover       BLOCK   an editor seat and an offsite listing (doctorRemedy).
//	                            These two ARE the publish floor, so a listing
//	                            carrying them cannot go live at all.
//	empty-tagline       BLOCK   on BOTH recognised kinds, with different remedies:
//	                            offsite `set-text --tagline` (in place, immediate),
//	                            onsite block.manifest.json. The onsite arm is
//	                            SUPPRESSED by the caller when the manifest being
//	                            submitted already carries a tagline — without that
//	                            it deadlocks, see checkListingComplete.
//	empty-description   BLOCK   offsite only: `set-text --description`. On an ONSITE
//	                            listing this CLI has NO route — `set-text` refuses
//	                            onsite (refuseOnsiteEdit) and `description` is not a
//	                            field the vendored schema declares, so there is no
//	                            manifest key the CLI can name without inventing one.
//	                            A refusal whose remedy the credential cannot take is
//	                            worse than no refusal (doctorRemedy's own rule), so
//	                            onsite it WARNS.
//	empty-category      warn    ONSITE HAS NO AUTHOR ROUTE AT ALL: (3b-sync) sources
//	                            category from `AppBlock.category`, which only a
//	                            moderator's `setMarketplaceMeta` writes, and step
//	                            (3a) copies the manifest value only while that column
//	                            is still NULL. The server's own label leads with
//	                            "resubmit", not with the manifest.
//	no-screenshots      warn    Server-advisory and documented "recommended,
//	                            optional". `add-screenshot` APPENDS and this command
//	                            ships no removal path in the same breath
//	                            (`rm-screenshot` stages a revision), so forcing it
//	                            buys junk media nobody can take back.
//	blocked-media       warn    Server-BLOCKING, and still not ours to refuse: for a
//	                            blocked SCREENSHOT the remedy is `rm-screenshot` →
//	                            `submit-revision` → a MODERATOR approval, and
//	                            doctorRemedy says in so many words that `listMine`
//	                            keeps reporting the problem until that lands. Blocking
//	                            here would wedge every submit on an approval the
//	                            author cannot drive, and buys nothing: the platform
//	                            refuses go-live on a blocked asset regardless.
//	scanning-media      warn    Resolves itself; doctorRemedy prints no command for
//	                            it. A gate on it would make a submit a coin flip on
//	                            scan timing.
//	anything else       warn    An UNKNOWN code is a NEW server code. A gate that
//	                            starts refusing every submit the day the server adds
//	                            a ninth problem is a gate people disable with the
//	                            escape hatch and never re-enable — the same
//	                            reasoning doctorIsBlocking uses for an unknown
//	                            SEVERITY. It still PRINTS, with its code and label.
//
// `kind` is the listing's own, normalised by listingIsOnsite. An UNRECOGNISED or
// ABSENT kind is treated as NOT onsite for the media rows (their remedy does not
// depend on kind) and is treated as onsite for the two text rows — i.e. it does
// not block on text. The directions are not symmetric: a wrongly-refused submit
// is a hard stop on the primary workflow whose only way out is the escape hatch,
// while a wrongly-permitted one leaves a listing the author can still finish and
// that `app doctor` still reports.
func submitGateBlocks(code, kind string) bool {
	switch code {
	case problemMissingIcon, problemMissingCover:
		return true
	case problemEmptyTagline:
		// Both RECOGNISED kinds have a real remedy, and they are different
		// remedies: `set-text --tagline` off-site (in place, immediate), and
		// block.manifest.json on-site. doctorRemedy already prints the right one
		// per kind. The on-site arm is the one that would DEADLOCK if it were the
		// whole story, which is why checkListingComplete suppresses it when the
		// manifest being submitted already carries a tagline — see there.
		return listingIsOffsite(kind) || listingIsOnsite(kind)
	case problemEmptyDescription:
		// 🔴 EXPLICITLY `offsite`, NOT "not onsite". The two differ exactly on the
		// case that matters: an ABSENT or UNRECOGNISED kind. `!listingIsOnsite`
		// would BLOCK there — refusing a submit while unable to establish which
		// remedy is even true, which is the direction with no way out but the
		// escape hatch. refuseOnsiteEdit fails CLOSED on an unknown kind for the
		// mirror-image reason (a wrongly-permitted WRITE corrupts a public
		// listing); a wrongly-permitted SUBMIT leaves a listing the author can
		// still finish, so this one fails open. Same asymmetry, opposite default,
		// because the acts are not the same act.
		return listingIsOffsite(kind)
	default:
		return false
	}
}

// listingIsOffsite reports whether a listing's TEXT is author-supplied — i.e.
// whether `civitai app listing set-text` can write it at all (refuseOnsiteEdit
// refuses every other kind).
//
// It normalises, like listingIsOnsite and doctorGates, because `kind` is a server
// string and a spelling change must not silently re-route a gate. It is
// deliberately NOT `!listingIsOnsite` — see submitGateBlocks.
func listingIsOffsite(kind string) bool {
	return strings.EqualFold(strings.TrimSpace(kind), "offsite")
}

// listingGateProblem is one finding the gate acted on, carrying the SERVER's own
// code and label plus doctorRemedy's fix string.
type listingGateProblem struct {
	code  string
	label string
	fix   string
	block bool
}

// checkListingComplete refuses a submit whose store listing is incomplete.
//
// 🔴 EVERY NON-REFUSAL BRANCH IS DELIBERATE, and the two that fail OPEN say so
// out loud rather than passing silently:
//
//   - --allow-incomplete-listing: returns BEFORE the network call, like
//     --allow-downgrade. A human submitting early on purpose should not pay a
//     round trip to have the answer discarded.
//   - listMine failed: WARN and proceed. This is an accident preventer, not an
//     authorization check; hard-failing would let one API blip block every submit
//     in the fleet, which is strictly worse than the defect it prevents.
//   - no listing for this app: proceed SILENTLY. That is a FIRST submit — the
//     submit itself mints the listing — and it is the normal case, so a warning
//     there would be noise on the happy path. See the file header for what that
//     means the gate cannot catch.
//   - an ONSITE listing whose only blocking finding is `empty-tagline` while the
//     manifest being submitted CARRIES one: warn and proceed. This is the
//     deadlock suppression, and without it the gate is unusable — see below.
//
// 🔴 THE SUPPRESSION IS NOT A SOFTENING, IT IS WHAT MAKES THE GATE CORRECT. An
// ONSITE listing's tagline is manifest-governed and is re-derived ONLY at approve
// time (`buildListingScalarSync`, scoped `kind: 'onsite'`). So an author who
// fixes `empty-tagline` the one way that works — adding it to
// block.manifest.json — cannot make the LISTING show it without an approved
// submit. Refusing that submit refuses the only act that fixes the problem: a
// permanent deadlock escapable only by the flag, i.e. a worse defect than the one
// being closed. Asking the local manifest is not a new validation rule and not a
// server mirror — the question is "is the fix already in the bundle I am about to
// send", which only the bundle can answer.
//
// `manifestTagline` is the manifest's own `tagline`, "" when absent. It is used
// for nothing else.
func checkListingComplete(ctx context.Context, list myListingLister, out io.Writer, slug, manifestTagline, baseURL string, allowIncomplete bool) error {
	if allowIncomplete {
		return nil
	}
	// 🔴 NO SLUG, NO GATE. manifest.Load does not require blockId and
	// --skip-validate waives the schema, so `slug` can arrive empty — the same
	// branch and the same reason as checkVersionNotRegression's. appapi.SameSlug
	// would match nothing, so every run would read as a first submit; saying so
	// is cheaper and truthful.
	if strings.TrimSpace(slug) == "" {
		warnf(out, "this manifest declares no blockId, so the listing-completeness gate has no listing to check — submitting without it.")
		return nil
	}
	rows, err := list(ctx)
	if err != nil {
		warnf(out, "could not check %s's store listing (%v) — submitting without the listing-completeness gate.", slug, err)
		return nil
	}
	var row *appapi.MyListing
	for i := range rows {
		// appapi.SameSlug, not `==`: the same shared predicate runAppDoctor and
		// refuseOnsiteEdit select with.
		if appapi.SameSlug(slug, rows[i].Slug) {
			row = &rows[i]
			break
		}
	}
	if row == nil {
		// 🔴 A CAPPED PAGE MAKES THIS SILENCE A CLAIM, so it is only silent when
		// the page cannot have hidden anything. `listMine` clamps to ListMineCap
		// with no cursor and no total, so past the cap an app the caller really
		// owns is indistinguishable here from one with no listing yet — and this
		// gate reads that as "first submit" and passes. Announcing it keeps the
		// fail-open from being silent, exactly like the read-failed branch.
		if doctorPageTruncated(len(rows)) {
			warnf(out, "the server caps the listing read at %d and offers no way to page, so %s's listing may "+
				"exist and was NOT checked — submitting without the listing-completeness gate.",
				appapi.ListMineCap, slug)
		}
		return nil
	}

	// 🔴 THE FINDINGS AND THEIR FIX STRINGS COME FROM doctorPayload, WHICH IS
	// `app doctor`'S OWN. That is the point of the gate re-using it: the remedy
	// the refusal prints and the remedy `app doctor` prints for the same code on
	// the same listing are the same sentence, because they are one call.
	payload := doctorPayload([]appapi.MyListing{*row}, baseURL)
	if len(payload.Apps) != 1 {
		// Unreachable for a one-row input; a guard rather than an index panic.
		return nil
	}
	app := payload.Apps[0]

	found := make([]listingGateProblem, 0, len(app.Blocking)+len(app.Advisory))
	for _, group := range [][]doctorProblemJSON{app.Blocking, app.Advisory} {
		for _, p := range group {
			found = append(found, listingGateProblem{
				code:  p.Code,
				label: p.Label,
				fix:   p.Fix,
				block: submitGateBlocks(p.Code, row.Kind),
			})
		}
	}
	if len(found) == 0 {
		return nil
	}

	// The deadlock suppression. Scoped to the ONE code it is about, so an onsite
	// listing missing an icon is still refused while its tagline is being fixed.
	if listingIsOnsite(row.Kind) && strings.TrimSpace(manifestTagline) != "" {
		for i := range found {
			if found[i].code == problemEmptyTagline && found[i].block {
				found[i].block = false
				found[i].fix = "already fixed in block.manifest.json — the store copy is re-derived from the " +
					"manifest when this version is approved"
			}
		}
	}

	var blocking, advisory []listingGateProblem
	for _, p := range found {
		if p.block {
			blocking = append(blocking, p)
			continue
		}
		advisory = append(advisory, p)
	}

	if len(blocking) == 0 {
		// 🔴 THE ADVISORIES ARE STILL PRINTED, ON THE PATH THAT PROCEEDS. Hiding
		// them is how `no-screenshots` and an onsite `empty-description` become
		// invisible, which is half of what the trial was reading when it called
		// them optional and stopped. They do not change the exit code.
		printListingGateAdvisories(out, slug, advisory)
		return nil
	}
	return listingIncompleteError(slug, blocking, advisory)
}

// printListingGateAdvisories names what is still incomplete on a submit the gate
// PERMITTED, or prints nothing when there is nothing to say.
func printListingGateAdvisories(out io.Writer, slug string, advisory []listingGateProblem) {
	if len(advisory) == 0 || out == nil {
		return
	}
	st := ui.For(out)
	fmt.Fprintln(out, st.Warn(fmt.Sprintf(
		"%s's store listing is still incomplete in %d way(s) this submit does not refuse:", slug, len(advisory))))
	for _, p := range sortedGateProblems(advisory) {
		fmt.Fprintf(out, "  %s  %s\n", p.code, p.label)
		fmt.Fprintf(out, "    Fix: %s\n", st.Code(p.fix))
	}
	fmt.Fprintf(out, "  Re-check with %s.\n", st.Code("civitai app doctor "+slug))
}

// listingIncompleteError builds the refusal.
//
// 🔴 IT NAMES EVERY PROBLEM AND EVERY FIX, INCLUDING THE ONES IT DID NOT REFUSE
// FOR. An author who is about to do listing work should do all of it in one pass;
// printing only the blocking half trains a second refused submit. The two groups
// are LABELLED, because "you must fix this" and "you should also fix this" are
// different instructions and collapsing them is what made the advisories look
// optional in the first place.
func listingIncompleteError(slug string, blocking, advisory []listingGateProblem) error {
	var b strings.Builder
	fmt.Fprintf(&b, "refusing to submit %s — its store listing is incomplete in %d way(s) that must be fixed first:",
		slug, len(blocking))
	for _, p := range sortedGateProblems(blocking) {
		fmt.Fprintf(&b, "\n  %s  %s\n    Fix: %s", p.code, p.label, p.fix)
	}
	if len(advisory) > 0 {
		b.WriteString("\n\nAlso incomplete, but not refused here — worth doing in the same pass:")
		for _, p := range sortedGateProblems(advisory) {
			fmt.Fprintf(&b, "\n  %s  %s\n    Fix: %s", p.code, p.label, p.fix)
		}
	}
	// 🔴 THE ESCAPE HATCH IS NAMED, AND `--yes` IS NAMED AS *NOT* BEING ONE.
	// `--yes` is the flag a script (and an agent) already passes, so leaving it
	// unmentioned invites exactly one wasted retry through the flag that cannot
	// work. See newAppSubmitCmd's Long for the same statement in the help.
	fmt.Fprintf(&b, "\n\nRun %s for the full diagnosis. If you mean to submit before the listing is "+
		"finished, pass --allow-incomplete-listing; --yes does not waive this.",
		"`civitai app doctor "+slug+"`")
	return civitai.Tag(ErrListingIncomplete, errors.New(b.String()))
}

// sortedGateProblems orders findings by code, so a refusal's text is stable
// whatever order the server sent the problems in. A message a test pins, and a
// message a human diffs across two runs, both need that.
func sortedGateProblems(in []listingGateProblem) []listingGateProblem {
	out := append([]listingGateProblem(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].code < out[j].code })
	return out
}
