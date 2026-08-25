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
	"github.com/spf13/cobra"
)

// `civitai app listing set-text` — the WRITE half of the three text problems
// `civitai app doctor` reports (`empty-description`, `empty-tagline`,
// `empty-category`). Until civitai/civitai#4341 annotated
// `appListings.updateListing` with the Apps submit scope, the only way to fix
// them was the web listing editor.
//
// 🔴 ONE COMMAND WITH FLAGS, NOT THREE SUBCOMMANDS, and the split is not
// arbitrary. The three MEDIA commands are separate because each takes a
// positional FILE with its own byte cap, its own decode, its own attach proc and
// its own scan poll — genuinely three flows. These three fields are ONE proc
// taking ONE `patch` object, and the server's own schema refines that at least
// one key is present, i.e. it is modelled as a single partial update. Splitting
// it would send three requests where the server expects one, cost three of the
// 30/hour rate-limit budget for one logical edit, and leave two of the three
// writes to fail after the first succeeded — a partial edit with no transaction
// around it.
//
// 🔴 IT DOES NOT WIRE `updateRevisionDraft`, AND THAT IS A MEASURED DECISION
// RATHER THAN AN OMISSION. The plan this was written from said the command had
// to own a parent-vs-shadow branch: material changes stage onto a shadow, so
// text edits on an approved listing would need the shadow proc. That premise is
// FALSE for these three fields. `MATERIAL_PATCH_FIELDS` is
// `['externalUrl', 'name', 'contentRating', 'sourceRepoUrl']`
// (`<civitai>/src/server/services/blocks/offsite-listing.service.ts:732`), and
// `patchHasMaterialChange` (`:849-889`) is a DIFF that `continue`s on every
// `undefined` field — so a patch carrying only tagline/description/category
// matches nothing, returns false, and takes the in-place branch on every listing
// status. Both facts re-read at origin/main on 2026-08-24.
//
// A `updateRevisionDraft` call would therefore be unreachable code shaped like a
// safety mechanism, which is worse than none: the next reader sees the shadow
// case "handled" and stops looking. What the command does instead is DECODE the
// server's own `requiresReview`/`shadowId` and report them, so if that field set
// ever widens the CLI says what happened rather than printing a success line
// that has quietly become false.

// setTextFieldNames are the fields `--clear` accepts, in the order the help and
// every refusal print them.
var setTextFieldNames = []string{"tagline", "description", "category"}

// ErrOnsiteTextNotEditable is returned when `set-text` is pointed at an ON-SITE
// listing, whose tagline/description/category are manifest-governed. It carries
// the exit code and nothing else; the message names the remedy.
//
// A named sentinel rather than a bare error so the exit-code contract can be
// asserted with errors.Is rather than by matching prose — AGENTS item 7.
var ErrOnsiteTextNotEditable = errors.New("this app's listing text is manifest-governed")

func newAppListingSetTextCmd() *cobra.Command {
	var lc listingCommon
	var tagline, description, category string
	var clear []string
	var assumeYes, jsonOut bool

	cmd := &cobra.Command{
		Use:   "set-text",
		Short: "Set your listing's tagline, description or category",
		Long: `Set the store listing's TEXT fields — tagline, description and category.

These are the three problems ` + "`civitai app doctor`" + ` reports as
empty-tagline / empty-description / empty-category, and this is the command that
fixes them without the browser.

Pass any combination of --tagline, --description and --category; they are sent
as ONE patch, so a run either applies or does not. At least one is required.

CLEARING vs EMPTYING are different server states and both are reachable:
--tagline "" sets an EMPTY STRING; --clear tagline sets it to NULL. --clear
takes a comma-separated list (` + strings.Join(setTextFieldNames, ", ") + `)
and cannot be combined with the matching value flag.

Blanking a field needs --yes, so an unset shell variable cannot silently empty
a public field. Whitespace-only counts as blank (the server trims).

CATEGORY must be one of:
` + strings.Join(wrapRunes(strings.Join(appapi.MarketplaceCategories, ", "), 74), "\n") + `

ON-SITE apps are REFUSED (exit 1 — a verdict about the app, not a bad command):
their copy comes from block.manifest.json and the platform overwrites it at your
next approved version. Edit the manifest instead.

This applies IN PLACE on every listing status — these are not "material"
changes, so they never open a revision for re-review. The server rate-limits
these edits (roughly 30 an hour).`,
		Example: `  civitai app listing set-text --tagline "Batch upscaling, in your browser"
  civitai app listing set-text --category utility --slug my-app
  civitai app listing set-text --description "$(cat DESCRIPTION.md)"
  civitai app listing set-text --clear tagline,category`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			patch, err := buildListingTextPatch(cmd, tagline, description, category, clear, assumeYes)
			if err != nil {
				return err
			}
			client, err := newListingClient()
			if err != nil {
				return err
			}
			slug, err := resolveListingSlug(lc)
			if err != nil {
				return err
			}
			ctx := cmdCtx(cmd)
			// 🔴 THE KIND GATE COMES BEFORE THE WRITE, AND BEFORE THE RESOLVE.
			// An onsite listing's copy is manifest-governed; writing it here
			// would be reverted at the next approve. Refusing first also means
			// the refusal costs one read rather than a read plus a pointless
			// resolve.
			if err := refuseOnsiteTextEdit(ctx, client, slug); err != nil {
				return err
			}
			ref, err := resolveListing(ctx, client, slug)
			if err != nil {
				return err
			}
			res, err := client.UpdateListing(ctx, ref.AppListingID, patch)
			if err != nil {
				return err
			}
			if jsonOut {
				// 🔴 The JSON path does NOT go through the human renderer:
				// internal/ui/CONVENTION.md rule 1 is that machine-readable
				// output carries no styling. The advisory still goes to stderr,
				// so stdout stays a pure parseable payload.
				if err := writeJSON(cmd.OutOrStdout(), setTextPayload(slug, ref, res, patch)); err != nil {
					return err
				}
				warnOpenRevision(cmd.ErrOrStderr(), slug, ref)
				return nil
			}
			reportListingTextUpdated(cmd.OutOrStdout(), cmd.ErrOrStderr(), slug, patch, ref, res)
			return nil
		},
	}
	lc.bind(cmd)
	cmd.Flags().StringVar(&tagline, "tagline", "", "set the short tagline (max "+fmt.Sprint(appapi.MaxTaglineRunes)+" characters)")
	cmd.Flags().StringVar(&description, "description", "", "set the long description (max "+fmt.Sprint(appapi.MaxDescriptionRunes)+" characters)")
	cmd.Flags().StringVar(&category, "category", "", "set the marketplace category: "+strings.Join(appapi.MarketplaceCategories, ", "))
	// NOTE: no back-quotes in these usage strings — pflag's UnquoteUsage treats
	// the first back-quoted span as the flag's VALUE NAME.
	// 🔴 THE HELP DESCRIBES WHAT THE CODE DOES, NOT WHAT WOULD BE NICER. It
	// used to say "blanking a field that CURRENTLY HAS TEXT, which the CLI
	// otherwise refuses" — a check this command never performs: the guard runs
	// in buildListingTextPatch, BEFORE any request, so it has never read the
	// field's present value. A blank set is refused unconditionally, including
	// on an already-empty field, which is exactly the state `doctor` reports.
	// Claiming a conditional guard that does not exist is the
	// description-wider-than-implementation shape, in shipped help text.
	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false,
		"permit a blank value for --tagline/--description/--category; blanks are refused without it "+
			"(the check is on the VALUE you passed, not on the field's current contents)")
	cmd.Flags().BoolVar(&jsonOut, "json", false,
		"emit the result as JSON (scriptable) — what was sent, and the server's own branch")
	cmd.Flags().StringSliceVar(&clear, "clear", nil,
		"clear a field to null instead of setting it: "+strings.Join(setTextFieldNames, ", ")+" (comma-separated)")
	return cmd
}

// refuseOnsiteTextEdit blocks a text write on an ON-SITE listing and names the
// remedy that actually works.
//
// 🔴 WITHOUT THIS THE FEATURE LOOP CLOSES WRONGLY. `civitai app doctor` reports
// `empty-tagline` / `empty-description` / `empty-category` for onsite apps too —
// `computeListingProblems` is kind-blind — and this command's own help says it
// fixes them. But an onsite listing's copy has NO author surface other than
// `block.manifest.json`: the `(3b-sync)` re-sync
// (`<civitai>/src/server/services/blocks/publish-request.service.ts:2742-2800`)
// overwrites all four columns from the manifest at THREE points, not one: at
// DRAFT MINT (`:1307`), at the FIRST approve (`:2682`), and at the
// `(3b-sync)` subsequent-version approve (`:2792`, scoped `kind: 'onsite'`). An
// earlier version of this note cited only the third and so UNDERSTATED the
// hazard — the refusal is more justified than the argument once given for it. So the write appears to succeed,
// `doctor` goes quiet, and the next approve silently reverts it. `--category` is
// worse still: that sync writes `AppBlock.category`, which is null unless a
// moderator curated one, so a category set here is CLEARED rather than restored.
//
// Nothing server-side stops this: `updateListing` selects `kind` and never
// branches on it, and `loadOwnedEditableListing` does not either. The gate has
// to be here.
//
// 🔴 THE KIND COMES FROM `listMine`, NOT FROM WHICH LOOKUP `resolveListing`
// HAPPENED TO TAKE. `getMyListingForApp` does not return `kind` (its select is
// `{id, status, contentRating, revisionOfId}`), and inferring onsite-ness from
// "the submissions route answered" would be a guess about a derived surface —
// the exact shape this repo keeps finding wrong. `listMine` carries `kind`
// authoritatively, is already scope-annotated, and is a pure read.
//
// 🔴 IT FAILS CLOSED. A slug that `listMine` does not list, or a kind this CLI
// does not recognise, REFUSES rather than proceeding. The directions are not
// symmetric: a wrongly-refused offsite edit is recoverable in the browser and
// says so, while a wrongly-permitted onsite edit corrupts a public listing in a
// way whose reversal nobody observes.
func refuseOnsiteTextEdit(ctx context.Context, client *appapi.Client, slug string) error {
	rows, err := client.ListMyListings(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if !appapi.SameSlug(slug, r.Slug) {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(r.Kind))
		if kind == appapi.ListingKindOnsite {
			// 🔴 EXIT 1, NOT 2, AND IT IS TAGGED SO THAT IS DELIBERATE RATHER
			// THAN A FALLTHROUGH. Every flag, value and slug in the invocation is
			// well-formed — a `2` would tell a script the CALL was malformed and
			// send someone re-reading their command line. What is wrong is the
			// SUBJECT: this app's copy is not editable through this route. That
			// is the same shape as an invalid manifest or a version regression,
			// which the exit-code contract already publishes under `1` as a
			// verdict about the project. Left untagged for the API classifier —
			// tagging it civitai.ErrBadRequest is the only route to `2` — and
			// pinned by TestSetTextOnsiteRefusalIsAVerdictNotAUsageError, because
			// nothing else in the suite would notice the move.
			return fmt.Errorf(
				"%w: %q is an ON-SITE app, and its tagline, description and category come from "+
					"block.manifest.json — editing them here would be overwritten by the manifest at your "+
					"next approved version. Edit `name` / `tagline` / `description` in block.manifest.json and run "+
					"`civitai app submit`. (Category is set by a moderator on an on-site app.)",
				ErrOnsiteTextNotEditable, r.Slug)
		}
		if kind == "" {
			// 🔴 TAGGED, LIKE ITS SIBLING. This was a bare error pinned only by a
			// prose assertion, so stripping the classification — or tagging it an
			// API kind — would move its exit code with every character of the
			// message intact and nothing failing. That is the gap S34 closed for
			// the ON-SITE arm; the same arm one branch down had it open.
			return fmt.Errorf(
				"%w: could not establish whether %q is an on-site or off-site app, and the two have different "+
					"owners for this text — refusing rather than risk an edit the platform reverts. "+
					"Update the CLI, or edit the listing in the browser", ErrOnsiteTextNotEditable, r.Slug)
		}
		return nil
	}
	// 🔴 A CAPPED PAGE MAKES THIS ANSWER UNCERTAIN, AND FAIL-CLOSED IS NOT AN
	// EXCUSE TO STATE IT AS FACT. `listMine` clamps to appapi.ListMineCap with no
	// cursor and no total, so for a caller whose accessible set exceeds the cap a
	// listing OUTSIDE the newest page is indistinguishable here from one that
	// does not exist — and this refusal hard-blocks a write `updateListing` would
	// have accepted, while telling the author their own app does not exist.
	//
	// Refusing remains right: the kind is what makes the write safe, and an
	// unestablished kind must not proceed. What changes is the CLAIM. The same
	// reasoning and the same constant are used by `civitai app doctor` in the
	// sibling PR, which is where ListMineCap's failure mode is documented at
	// length; this is the second consumer of that route, and it inherited the
	// hazard without inheriting the note.
	if len(rows) >= appapi.ListMineCap {
		return civitai.Tag(civitai.ErrNotFound, fmt.Errorf(
			"%q is not among the %d listings the server returned — but it CAPS this read at %d and offers "+
				"no way to page, so an app you own beyond that cap looks identical to one that does not "+
				"exist. This answer is not conclusive; open the listing in the browser if you are sure you "+
				"own it", slug, len(rows), appapi.ListMineCap))
	}
	return civitai.Tag(civitai.ErrNotFound, fmt.Errorf(
		"no listing of yours is called %q — `civitai app doctor` lists every app you can work on", slug))
}

// buildListingTextPatch turns the flags into a patch, refusing locally
// everything that can be refused locally.
//
// 🔴 IT READS `Flags().Changed`, NOT THE VALUE. A `--tagline ""` is a REAL edit
// (the empty string is a legal, distinct state), so deciding "was this flag
// given" from whether the string is empty would silently drop exactly the edit
// the tri-state exists to carry.
func buildListingTextPatch(cmd *cobra.Command, tagline, description, category string, clear []string, assumeYes bool) (appapi.ListingTextPatch, error) {
	var p appapi.ListingTextPatch
	f := cmd.Flags()

	// The --clear list first, so a contradiction is reported against a set that
	// is already known to be well-formed.
	seen := map[string]bool{}
	for _, raw := range clear {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		if !contains(setTextFieldNames, name) {
			return p, asUsageError(fmt.Errorf(
				"--clear does not know the field %q — it takes %s",
				raw, strings.Join(setTextFieldNames, ", ")))
		}
		seen[name] = true
	}
	for _, name := range setTextFieldNames {
		if seen[name] && f.Changed(name) {
			// 🔴 REFUSED, NOT SILENTLY RESOLVED. "Set it to X" and "clear it" are
			// contradictory instructions about one column, and picking a winner
			// would apply an edit the user did not ask for — to a field on a
			// public store page.
			return p, asUsageError(fmt.Errorf(
				"--%s and `--clear %s` contradict each other — one sets the field, the other nulls it; pass only one",
				name, name))
		}
	}
	p.ClearTagline, p.ClearDescription, p.ClearCategory = seen["tagline"], seen["description"], seen["category"]

	if f.Changed("tagline") {
		if n := len([]rune(tagline)); n > appapi.MaxTaglineRunes {
			return p, asUsageError(fmt.Errorf(
				"--tagline is %d characters; the server accepts at most %d", n, appapi.MaxTaglineRunes))
		}
		p.Tagline = &tagline
	}
	if f.Changed("description") {
		if n := len([]rune(description)); n > appapi.MaxDescriptionRunes {
			return p, asUsageError(fmt.Errorf(
				"--description is %d characters; the server accepts at most %d", n, appapi.MaxDescriptionRunes))
		}
		p.Description = &description
	}
	if f.Changed("category") {
		if err := validateCategory(category); err != nil {
			return p, err
		}
		p.Category = &category
	}

	if p.Empty() {
		return p, asUsageError(fmt.Errorf(
			"nothing to set — pass at least one of --tagline, --description, --category, or --clear <field>"))
	}
	// 🔴 A BLANK SET IS GUARDED, BECAUSE THE SHELL MAKES IT AN ACCIDENT RATHER
	// THAN A CHOICE. `--tagline "$T"` with `T` unset expands to `--tagline ""`,
	// and this command's own documented example
	// `--description "$(cat DESCRIPTION.md)"` does the same whenever `cat`
	// fails — silently blanking a PUBLIC field at exit 0. The siblings all gate
	// a destructive act (`app submit` needs `--yes`, `app withdraw` confirms);
	// this had nothing.
	//
	// 🔴 WHITESPACE-ONLY COUNTS AS BLANK, and that is not pedantry: the server's
	// own `isEmpty` trims, so `" "` leaves `civitai app doctor` still reporting
	// `empty-tagline`. Treating it as a real value would let the command report
	// success for an edit that fixes nothing.
	//
	// It guards only the SET path. `--clear` is already an explicit request to
	// empty the field, so requiring a second confirmation for it would be noise.
	if !assumeYes {
		for _, f := range []struct {
			name string
			val  *string
		}{{"tagline", p.Tagline}, {"description", p.Description}, {"category", p.Category}} {
			if f.val != nil && strings.TrimSpace(*f.val) == "" {
				return appapi.ListingTextPatch{}, asUsageError(fmt.Errorf(
					"--%s is blank, which would empty a public field. If that is deliberate, pass --yes "+
						"(or `--clear %s` to null it instead). If it is not, an unset shell variable or a failed "+
						"command substitution is the usual cause", f.name, f.name))
			}
		}
	}
	return p, nil
}

// validateCategory refuses a value the server's `z.enum` would refuse, and NAMES
// THE ALLOWED SET, which the server's own 400 does not.
//
// 🔴 THE EMPTY STRING IS REFUSED HERE AND IT IS NOT SYMMETRIC WITH THE OTHER
// TWO. `tagline` and `description` carry no `.min()`, so `""` is a legal value
// for them; `category` is a `z.enum` and `""` is not a member. Sending it would
// spend a round trip to be told so in a message that does not list the members.
// The remedy names `--clear category`, because "I want no category" is the thing
// the user was probably reaching for.
func validateCategory(v string) error {
	if v == "" {
		return asUsageError(fmt.Errorf(
			"--category cannot be empty — it is one of %s. To remove the category entirely, use `--clear category`",
			strings.Join(appapi.MarketplaceCategories, ", ")))
	}
	if contains(appapi.MarketplaceCategories, v) {
		return nil
	}
	// A case-only miss is the likeliest typo and gets its own sentence: the
	// server compares exactly, so "Utility" is a real refusal, not a nit.
	for _, known := range appapi.MarketplaceCategories {
		if strings.EqualFold(known, v) {
			return asUsageError(fmt.Errorf(
				"--category %q is not accepted — categories are lower-case; did you mean %q? (allowed: %s)",
				v, known, strings.Join(appapi.MarketplaceCategories, ", ")))
		}
	}
	return asUsageError(fmt.Errorf(
		"--category %q is not one of the marketplace categories: %s",
		v, strings.Join(appapi.MarketplaceCategories, ", ")))
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

// reportListingTextUpdated prints what changed, and — where it applies — the one
// caveat this command cannot remove.
//
// 🔴 THE WARNING IS SCOPED TO A LISTING WITH A REVISION UNDER REVIEW, and it is
// a REAL hazard rather than a hedge. A shadow revision is seeded from the parent
// when it opens; on an OFF-SITE listing, `applyApprovedRevision` copies the
// shadow's FULL scalar set back onto the parent on approval — `name`, `tagline`,
// `description`, `category`, `contentRating`
// (`<civitai>/src/server/services/blocks/offsite-listing.service.ts:2854-2870`).
// So a text edit written now can be overwritten later by text the shadow
// captured before it.
//
// 🔴 AN ON-SITE LISTING IS STRUCTURALLY IMMUNE, and the asymmetry is why the
// warning names it instead of asserting the hazard for everyone. The `onsite`
// branch of that same function (`:2837-2852`) writes ONLY `iconId` and
// `coverId`, because an on-site listing's scalars mirror the manifest. Verified
// by reading both branches at origin/main on 2026-08-24.
//
// 🔴 WHY THIS IS A WARNING AND NOT A REROUTE. The obvious "fix" is to detect the
// shadow and write THAT instead. It is refused twice over. First, a write that
// silently targets a different row than the user named is the surprise class
// this CLI keeps getting bitten by. Second, and decisively, the only read that
// exposes an open shadow — `getMyListingForEdit` — OPENS ONE as a side effect on
// an approved listing, so a command that consulted it to avoid the hazard would
// CREATE the hazard on every run against a listing that had none.
//
// 🔴 IT GATES ON `ShadowID != nil || HasPendingRevision`, AND THE FIRST HALF IS
// THE ONE THAT MATTERS. An earlier version gated on `HasPendingRevision` alone
// and argued, as its decisive premise, that "no side-effect-free read exposes an
// open shadow". THAT WAS FALSE, and it made the warning unable to fire in the
// one state it exists for.
//
// `hasPendingRevision` is a `appListingPublishRequest.findFirst({status:
// 'pending'})` — it means SUBMITTED. But `getMyListingForApp`, the read this
// command ALREADY makes, also returns `shadowId`, resolved by a `findFirst` with
// no create (offsite-listing.service.ts:1975-1993, under a comment that says
// "WITHOUT creating one"). So an OPEN, UNSUBMITTED shadow is visible, for free,
// on a request already in flight.
//
// That gap was the normal path, not an edge case: `rm-screenshot` leaves its
// revision unsubmitted BY DOCUMENTED DESIGN, and `set-icon`/`set-cover`/
// `add-screenshot` mint the shadow lazily too. Stage any media change, then
// `set-text`, then submit and get approved, and the shadow's stale copy lands
// back on the parent — silently undoing the text edit, with nothing having
// warned.
//
// 🔴 WHAT SURVIVES OF THE OLD ARGUMENT, AND WHAT DOES NOT. The reroute refusal
// still stands and for the SECOND of its two reasons only: `getMyListingForEdit`
// really does OPEN a shadow as a side effect, so consulting IT would create the
// hazard. The first reason — that nothing else could see a shadow — is retracted.
func reportListingTextUpdated(out, errOut io.Writer, slug string, p appapi.ListingTextPatch, ref *appapi.ListingRef, res *appapi.UpdateListingResult) {
	st := ui.For(out)
	fmt.Fprintln(out, st.Success(fmt.Sprintf("Updated %s: %s", slug, strings.Join(changedFields(p), ", "))))

	// 🔴 REPORTED FROM THE SERVER'S ANSWER, NEVER FROM THIS CLI'S BELIEF. A
	// text-only patch is not a material change, so this branch is unreachable
	// today — that is a claim about MATERIAL_PATCH_FIELDS, which lives in the
	// other repo and can move. If it does, the user is told a revision was
	// opened instead of being handed a success line that has become false.
	if res != nil && res.RequiresReview {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, st.Warn("The server staged this on a REVISION rather than applying it — it needs moderator review."))
		if res.ShadowID != nil && *res.ShadowID != "" {
			fmt.Fprintf(out, "  Revision: %s\n", *res.ShadowID)
		}
		fmt.Fprintf(out, "  Send it for review: %s\n", st.Code("civitai app listing submit-revision"))
		// 🔴 NO EARLY RETURN. This branch used to `return` here, skipping the
		// shared advisory below — while `--json` called it unconditionally. So
		// with `requiresReview:true` AND an open shadow the HUMAN got no
		// overwrite warning while `--json` reported `openRevision:true`: the two
		// renderings disagreeing about whether the hazard applies, which is
		// exactly the property warnOpenRevision's own doc comment claims they
		// cannot. A comment asserting a property the control flow denies is
		// worse than no comment.
		//
		// Unreachable today — a text-only patch is never material — but this
		// branch is deliberately kept live in case MATERIAL_PATCH_FIELDS moves,
		// and if it fires the HUMAN path is the one that would lose the warning.
	}

	warnOpenRevision(errOut, slug, ref)
}

// setTextJSON is the `--json` payload. PUBLISHED CONTRACT.
//
// 🔴 IT EXISTS BECAUSE THE LOOP THIS COMMAND CLOSES IS MACHINE-READABLE AT BOTH
// OTHER ENDS AND WAS HUMAN-ONLY HERE. `civitai app doctor --json` gives a
// machine-readable diagnosis; without this, a script could read the problem and
// apply the fix but had no machine-readable confirmation of what it changed —
// and `app listing status --json`, the obvious substitute, cannot serve, because
// it OPENS a shadow revision as a side effect.
type setTextJSON struct {
	Slug         string `json:"slug"`
	AppListingID string `json:"appListingId"`
	// Fields is what was SENT, per key: "set", "empty" or "cleared". Never null.
	Fields map[string]string `json:"fields"`
	// RequiresReview / ShadowID are the SERVER's own branch, passed through.
	RequiresReview bool    `json:"requiresReview"`
	ShadowID       *string `json:"shadowId"`
	// OpenRevision is true when a revision draft exists (submitted or not) whose
	// approval would overwrite this edit — the machine-readable form of the
	// stderr advisory.
	OpenRevision bool `json:"openRevision"`
}

func setTextPayload(slug string, ref *appapi.ListingRef, res *appapi.UpdateListingResult, p appapi.ListingTextPatch) setTextJSON {
	out := setTextJSON{Slug: slug, Fields: map[string]string{}}
	if ref != nil {
		out.AppListingID = ref.AppListingID
		out.ShadowID = ref.ShadowID
		out.OpenRevision = ref.ShadowID != nil || ref.HasPendingRevision
	}
	if res != nil {
		out.RequiresReview = res.RequiresReview
	}
	set := func(k string, v *string, cleared bool) {
		switch {
		case cleared:
			out.Fields[k] = "cleared"
		case v == nil:
			return
		case strings.TrimSpace(*v) == "":
			out.Fields[k] = "empty"
		default:
			out.Fields[k] = "set"
		}
	}
	set("tagline", p.Tagline, p.ClearTagline)
	set("description", p.Description, p.ClearDescription)
	set("category", p.Category, p.ClearCategory)
	return out
}

// warnOpenRevision is the overwrite advisory, shared by both renderings so they
// cannot disagree about whether it applies.
func warnOpenRevision(errOut io.Writer, slug string, ref *appapi.ListingRef) {
	if ref != nil && (ref.ShadowID != nil || ref.HasPendingRevision) {
		se := ui.For(errOut)
		// The two states are DIFFERENT and the reader can act on them
		// differently — one is still theirs to edit, the other is with a
		// moderator — so they are not collapsed into one sentence.
		if ref.HasPendingRevision {
			fmt.Fprintln(errOut, se.Warn("A revision of this listing is already under moderator review."))
		} else {
			fmt.Fprintln(errOut, se.Warn("This listing has an OPEN revision draft that has not been submitted yet."))
		}
		if ref.ShadowID != nil && *ref.ShadowID != "" {
			fmt.Fprintf(errOut, "  Revision: %s\n", *ref.ShadowID)
		}
		fmt.Fprintln(errOut, "  This edit went to the LIVE listing, not to that revision. Approving the")
		fmt.Fprintln(errOut, "  revision copies ITS tagline/description/category back over the listing and")
		fmt.Fprintln(errOut, "  would undo this edit.")
		fmt.Fprintf(errOut, "  Check afterwards with %s.\n", se.Code("civitai app doctor "+slug))
	}
}

// changedFields names what the run actually sent, in the fixed field order, so
// the success line is a report rather than a restatement of the flags.
func changedFields(p appapi.ListingTextPatch) []string {
	var out []string
	if p.Tagline != nil {
		out = append(out, describeSet("tagline", *p.Tagline))
	}
	if p.ClearTagline {
		out = append(out, "tagline cleared")
	}
	if p.Description != nil {
		out = append(out, describeSet("description", *p.Description))
	}
	if p.ClearDescription {
		out = append(out, "description cleared")
	}
	if p.Category != nil {
		out = append(out, "category set to "+*p.Category)
	}
	if p.ClearCategory {
		out = append(out, "category cleared")
	}
	return out
}

// describeSet distinguishes "set to an empty string" from "set", because the
// two are different server states and a bare "tagline set" would hide which one
// the user just chose.
func describeSet(field, v string) string {
	if v == "" {
		return field + " set to an empty string (use `--clear " + field + "` to null it instead)"
	}
	return field + " set"
}
