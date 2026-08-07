package cmd

import (
	"fmt"
	"io"

	"github.com/civitai/cli/internal/genapi"
	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/pkg/civitai"
)

// Reporting silent checkpoint substitutions (civitai/civitai#3665).
//
// The server accepts a checkpoint version id that is not valid for the
// ecosystem, substitutes that ecosystem's default, runs the job and bills for
// what ran — at HTTP 200, with a reply that used to be byte-indistinguishable
// from success. It now records each swap. This file is where that record becomes
// something a human sees.
//
// 🔴 EVERYTHING HERE GOES TO STDERR. `--json` is a raw passthrough of the server
// payload on stdout, so the record already reaches a script verbatim without any
// help from this file; writing a warning to stdout would corrupt that stream for
// the exact callers most likely to be automating a spend. Warning on stderr in
// EVERY mode — including `--json` — is the house shape (see the --quantity clamp
// and the balance-read failure, both of which warn before the JSON branch).

// substitutionInspectCmd is the command the "unrecognized" advice tells the user
// to run to check a version id.
//
// 🔴 IT IS A CONSTANT SO A TEST CAN PROVE THE COMMAND EXISTS. Advice naming a
// command that does not exist is worse than no advice — it sends a user who has
// just been mis-billed to a dead end. The first draft of this line said
// `civitai models versions <id>`, which is NOT a command in this CLI (the real
// one lives under a different parent); running the built binary is what caught
// it, and TestSubstitutionAdvice_NamesARealCommand is what stops it coming back.
const substitutionInspectCmd = "civitai model-versions get"

// substitutionPhase selects the lead sentence, which is the part that differs:
// whether the money has already moved.
type substitutionPhase int

const (
	// substitutionAtEstimate is the whatIf/dry-run path. Nothing has been spent
	// and the user can still walk away — the one place this warning can prevent
	// the charge rather than explain it.
	substitutionAtEstimate substitutionPhase = iota
	// substitutionAfterSubmit is the generateFromGraph reply. The charge has
	// already happened.
	substitutionAfterSubmit
	// substitutionOnRead is a later read of a workflow (`civitai workflows get`).
	// The job may be long finished.
	substitutionOnRead
)

// substitutionAdvice returns the actionable half for one reason.
//
// 🔴 IT SUPPLEMENTS THE SERVER'S TOKEN, IT DOES NOT REPLACE IT. AGENTS.md item 8
// requires server-owned tokens to be printed RAW rather than humanised, so
// reportModelSubstitutions prints `reason: <token>` verbatim and hangs this line
// underneath. Never fold the advice in as a friendlier label for the token —
// a caller grepping stderr for the server's own vocabulary must still find it.
//
// An UNRECOGNISED reason gets generic advice rather than being dropped. A newer
// server may add a fourth reason, and the ids are the load-bearing part of the
// record: discarding an entry because its label is unfamiliar would restore the
// silence this feature exists to remove.
func substitutionAdvice(reason string) string {
	switch reason {
	case genapi.SubstitutionWrongWorkflow:
		return "that version is real for this model family but belongs to a DIFFERENT workflow " +
			"(for example an edit-only version sent to text-to-image). There is usually a specific version " +
			"you meant: pick one offered for the workflow you are running, or change --ecosystem to match the version"
	case genapi.SubstitutionUnrecognized:
		return "the server does not offer that version in this model family at all — it may be a community " +
			"checkpoint that was never generatable, or a version retired since this command was written. " +
			"Check it with `" + substitutionInspectCmd + " <id>` and pin a version that is still offered"
	case genapi.SubstitutionGated:
		return "that version IS offered for this workflow, but a gate rule hides it from your account — " +
			"this is an entitlement problem, not a mistake in your command. It may need a membership, " +
			"an early-access window, or an accepted licence on the model page"
	default:
		// Reached only against a server newer than this CLI.
		return "this CLI does not recognise that reason code — the substitution is real and the ids above are " +
			"authoritative, but the explanation may be incomplete. Upgrade with `civitai upgrade`"
	}
}

// reportModelSubstitutions writes the substitution warning block, or nothing.
//
// 🔴 AN EMPTY RECORD PRINTS ABSOLUTELY NOTHING, and that silence is deliberate.
// The server OMITS the key when it substituted nothing, so "empty" means EITHER
// "nothing was substituted" OR "this server predates the field" — the two are
// the same bytes. Printing a reassuring "no substitutions" would be a positive
// claim this CLI cannot support, and would be WRONG against every server older
// than civitai/civitai#3692. Say nothing instead of saying something false.
func reportModelSubstitutions(errw io.Writer, subs []genapi.ModelSubstitution, phase substitutionPhase) {
	if len(subs) == 0 {
		return
	}
	fmt.Fprintln(errw, ui.For(errw).Warn(substitutionLead(phase)))

	for _, s := range subs {
		// The ids are ints, so they cannot carry terminal escapes; the reason is a
		// server-supplied STRING and goes through safeTerm like every other one.
		//
		// 🔴 The VERB is phase-dependent. On the estimate nothing has run yet, so
		// "ran version N" under a lead that says "nothing has been charged yet" is
		// self-contradictory on the one path where the user is deciding whether to
		// spend.
		fmt.Fprintf(errw, "    requested version %d -> %s version %d  (reason: %s)\n",
			s.Requested, substitutionVerb(phase), s.Applied, safeTerm(s.Reason))
		fmt.Fprintf(errw, "      %s\n", substitutionAdvice(s.Reason))
	}
}

// substitutionLead is the phase's lead sentence — the part that says whether the
// money has already moved.
//
// 🔴 IT IS A FUNCTION OF THE PHASE CONSTANT SO A TEST CAN ASSERT THE STATE
// RATHER THAN SPELL A WORD. An earlier test checked `Contains(out, "charged")`
// to prove the estimate call site passed substitutionAtEstimate. That is a
// SPELLED guard, and it was satisfied by an unrelated pre-existing line ("…
// nothing was submitted and nothing was charged") printed by the quote renderer
// — so mutating the call site to substitutionAfterSubmit left the whole suite
// green while `--dry-run` told the user "this generation HAS BEEN CHARGED".
// Deriving the expected text from the constant makes a wrong argument at a call
// site fail, because the assertion moves with the phase instead of matching a
// word another line can also spell.
//
// A phase with no case returns "", which
// TestSubstitutionLead_AllPhasesNonEmptyAndPairwiseDistinct rejects —
// an empty lead would make every Contains() check pass vacuously.
func substitutionLead(phase substitutionPhase) string {
	switch phase {
	case substitutionAtEstimate:
		// 🔴 NO POSITIONAL PROMISE — an earlier wording said "the estimate BELOW".
		// This lead is printed on all three pre-spend surfaces, and on one of them
		// nothing follows it: `--yes` skips the prompt, and `confirmGenerate`'s
		// --yes arm prints the image disclosure and returns, never the cost. So the
		// old wording pointed at an estimate that is never rendered — on the one
		// surface that is MANDATORY non-interactively and has nobody watching.
		// Describing the estimate that PRODUCED this warning is true everywhere;
		// describing where it lands on screen is not. Pinned by
		// TestConfirmGenerate_YesPathPrintsNoCost, which goes red if `--yes` ever
		// starts printing the cost — at which point a positional wording would be
		// honest again and this comment is the record of why it wasn't.
		return "The server will NOT use the checkpoint you asked for. It has substituted a different model, " +
			"and this estimate prices the SUBSTITUTE. Nothing has been submitted or charged yet."
	case substitutionAfterSubmit:
		return "The server did NOT use the checkpoint you asked for. It substituted a different model and " +
			"this generation HAS BEEN CHARGED for the model that actually ran."
	case substitutionOnRead:
		return "The server did not use the checkpoint that was requested for this workflow. It substituted a " +
			"different model, and it was CHARGED for the model that actually ran."
	}
	return ""
}

// substitutionVerb is the tense of the id line, which follows the same
// already-spent/not-yet-spent split as the lead.
func substitutionVerb(phase substitutionPhase) string {
	if phase == substitutionAtEstimate {
		return "will run"
	}
	return "ran"
}

// substitutionFlagHint suggests --fail-on-substitution, and ONLY when the caller
// has not already passed it.
//
// 🔴 It lives here rather than inside reportModelSubstitutions because the
// renderer has no business knowing about flags — and because telling someone who
// just used --fail-on-substitution to "pass --fail-on-substitution" (which the
// first version did, verified by running the binary) reads as the CLI not having
// understood the command.
func substitutionFlagHint(errw io.Writer, o generateOpts, subs []genapi.ModelSubstitution) {
	if len(subs) == 0 || o.failOnSubstitution {
		return
	}
	fmt.Fprintln(errw, ui.For(errw).Dim(
		"To refuse a run like this instead of being told about it, pass --fail-on-substitution."))
}

// substitutionRefusal implements --fail-on-substitution. It returns nil unless
// the flag is set AND the server reported at least one substitution.
//
// 🔴 IT IS EVALUATED ON THE ESTIMATE, WHICH IS WHY IT CAN REFUSE FOR FREE. The
// whatIf runs the same validation as the submit against the same graph, spends
// nothing and persists nothing, so a refusal here costs the caller exactly
// nothing — the same bargain --max-cost offers.
//
// 🔴 KNOWN AND DELIBERATE LIMIT: it does NOT re-fire on the submit reply. If a
// substitution somehow appeared only there, the money is already gone, and
// aborting at that point would strand outputs the caller has paid for and still
// needs to collect. That case is always REPORTED (see the substitutionAfterSubmit
// call in runGenerate) and is always present in --json; it just does not change
// the exit code. Refusing after the charge would trade a real, collectable result
// for a non-zero exit, which is the wrong way round.
func substitutionRefusal(o generateOpts, subs []genapi.ModelSubstitution) error {
	if !o.failOnSubstitution || len(subs) == 0 {
		return nil
	}
	// 🔴 subs[0], and the count is len-1 because the FIRST one is the one named.
	// Both were unpinned until an audit mutated them: `subs[len(subs)-1]` and an
	// off-by-one `(and %d more)` each survived the whole suite.
	first := subs[0]
	detail := ""
	if rest := len(subs) - 1; rest > 0 {
		detail = fmt.Sprintf(" (and %d more)", rest)
	}
	// Tagged so a script pins the kind with errors.Is rather than matching this
	// text, which is free to change.
	//
	// 🔴 OPERAND ORDER IS LOAD-BEARING AND WAS UNPINNED: APPLIED first ("would
	// run"), REQUESTED second ("you asked for"). Swapping them survived the whole
	// suite and made the money-refusal line state the substitution BACKWARDS,
	// which is worse than not printing it — it names the model the caller wanted
	// as the one that would run.
	return civitai.Tag(ErrModelSubstituted, fmt.Errorf(
		"refusing to submit: the server reports it would run version %d instead of the version %d you asked for%s (reason: %s) — "+
			"nothing was submitted and nothing was charged. Pin a version this workflow offers, or drop --fail-on-substitution to run with the substitute",
		first.Applied, first.Requested, detail, safeTerm(first.Reason)))
}
