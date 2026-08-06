package cmd

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/civitai/cli/internal/genapi"
	"github.com/civitai/cli/internal/ui"
)

// substitutionPhase selects the tense of the warning: an estimate has not been
// charged yet, a submit or a read-back has.
type substitutionPhase int

const (
	// substitutionPhaseEstimate is the pre-spend surface (whatIf): --dry-run,
	// the TTY confirmation and --yes.
	substitutionPhaseEstimate substitutionPhase = iota
	// substitutionPhaseCharged is any surface where the money is already gone:
	// the submit reply and `workflows get`.
	substitutionPhaseCharged
)

// versionResolver is the model-version lookup seam. It matches
// genapi.Client.ResolveModelVersion, and may be nil: a surface with no resolver
// still warns, with ids instead of names.
type versionResolver func(ctx context.Context, id int) (*genapi.ResolvedVersion, error)

// substitutionReasonGloss explains the server's reason tokens in one clause.
//
// 🔴 IT IS ADVISORY AND FAILS SOFT, and that shape is what keeps it from being
// the vendored table AGENTS.md item 13 forbids. An unknown token — a reason the
// platform adds after this build — renders VERBATIM with no gloss and the
// warning fires exactly as loudly. A map lookup here can never decide whether
// the user is told; it only decides whether a sentence is appended. If you ever
// make the warning conditional on a hit in this map, you have reintroduced the
// silence this feature exists to end.
//
// Mirrors the doc comments on `MODEL_SUBSTITUTION_REASONS`
// (civitai/civitai -> src/shared/data-graph/generation/model-substitution.ts).
var substitutionReasonGloss = map[string]string{
	"wrong-workflow": "that version exists in this ecosystem but belongs to a different workflow",
	"unrecognized":   "that version is in none of this ecosystem's lists — a community checkpoint, or one retired since",
	"gated":          "that version is offered here, but a gate rule hides it from your account",
}

// printModelSubstitutions is the ONE renderer for a silent model substitution
// the server has now told us about (civitai#3665, PRs #3692 / #3673).
//
// 🔴 IT WARNS AND NEVER REFUSES — a deliberate decision, argued rather than
// defaulted. Four reasons:
//
//   - Substitution on a `modelLocked` ecosystem is legitimate graceful
//     degradation. It is what keeps an app pinned to a since-retired version
//     generating instead of hard-failing, and the server documents it as such.
//     Refusing would break flows that work today, for a class of request the
//     platform considers correct.
//   - The platform has DELIBERATELY NOT MADE the rejection decision. Its own
//     module note says recording the swap "changes no behaviour" and that
//     "deciding whether any case should REJECT is a later phase, gated on what
//     the counter measures". A CLI that refused first would be vendoring a
//     policy the server has explicitly deferred — item 13's anti-mirror rule,
//     applied to a policy rather than to a table.
//   - A refusal would be UNENFORCEABLE ANYWAY. Absence of the field means "no
//     substitution" OR "a server predating it"; the two are indistinguishable on
//     the wire, so a gate would bind only against servers new enough to confess.
//   - Two of the four surfaces are POST-SPEND (the submit reply, `workflows
//     get`) where refusing buys nothing at all. On the pre-spend surfaces the
//     user still has --dry-run and the confirmation prompt, both of which now
//     carry this, and a script has the raw field under --json.
//
// The same fail-soft shape as serverQuantityClamp (AGENTS.md item 13b): say the
// expensive thing loudly, then let the request proceed.
//
// Everything goes to STDERR, unconditionally — including under --json, where the
// machine payload owns stdout and already carries the field. A warning about a
// wrong charge must not be the one thing a scripted surface drops.
func printModelSubstitutions(ctx context.Context, errw io.Writer, subs []genapi.ModelSubstitution, resolve versionResolver, phase substitutionPhase) {
	if len(subs) == 0 {
		// The overwhelmingly common case, and it must stay SILENT — a "no
		// substitutions" line on every run would train users past the real one.
		// It is also not a guarantee: a server predating civitai#3665 sends
		// nothing here either.
		return
	}
	st := ui.For(errw)
	lead := "The server SUBSTITUTED a different checkpoint — the estimate below prices the model it would actually run, not the one you asked for."
	if phase == substitutionPhaseCharged {
		lead = "The server SUBSTITUTED a different checkpoint — you were charged for a generation that ran a model you did not ask for."
	}
	fmt.Fprintln(errw, st.Warn(lead))

	// One cache per call: a repeated id costs one lookup, and a surface with no
	// resolver costs none.
	names := map[int]string{}
	name := func(id int) string {
		if s, ok := names[id]; ok {
			return s
		}
		s := resolveSubstitutedVersion(ctx, resolve, id)
		names[id] = s
		return s
	}

	for _, s := range subs {
		fmt.Fprintf(errw, "  Requested:  %s\n", name(s.Requested))
		fmt.Fprintf(errw, "  Ran:        %s\n", name(s.Applied))
		reason := safeTerm(s.Reason)
		if gloss, ok := substitutionReasonGloss[s.Reason]; ok {
			reason += " — " + gloss
		}
		fmt.Fprintf(errw, "  Reason:     %s\n", reason)
	}

	tail := "This is not a server error: the request succeeds and is billed at the SUBSTITUTE's price. Pass a version this ecosystem and workflow offer, or accept the substitute."
	if phase == substitutionPhaseCharged {
		tail = "This is not a server error and there is no refund. Check the version against the ecosystem and workflow you named before the next run."
	}
	fmt.Fprintln(errw, st.Dim(tail))
}

// resolveSubstitutedVersion renders one version id as a name when it can.
//
// 🔴 A FAILED LOOKUP MUST NEVER SUPPRESS THE WARNING. The name is a convenience;
// the substitution is the fact. So the id is always rendered, the failure is
// stated inline rather than swallowed, and a nil resolver is simply a surface
// that has no lookup wired — not a reason to say nothing.
func resolveSubstitutedVersion(ctx context.Context, resolve versionResolver, id int) string {
	if resolve == nil {
		return "id " + strconv.Itoa(id)
	}
	rv, err := resolve(ctx, id)
	if err != nil || rv == nil {
		return fmt.Sprintf("id %d (could not look up the name)", id)
	}
	return describeVersion(rv, nil)
}
