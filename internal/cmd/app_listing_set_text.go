package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/ui"
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

func newAppListingSetTextCmd() *cobra.Command {
	var lc listingCommon
	var tagline, description, category string
	var clear []string

	cmd := &cobra.Command{
		Use:   "set-text",
		Short: "Set your listing's tagline, description or category",
		Long: `Set the store listing's TEXT fields — tagline, description and category.

These are the three problems ` + "`civitai app doctor`" + ` reports as
empty-tagline / empty-description / empty-category, and this is the command that
fixes them without the browser.

Pass any combination of --tagline, --description and --category; they are sent
as ONE patch, so a run either applies or does not. At least one is required.

CLEARING vs EMPTYING are different states on the server and both are reachable:
  --tagline ""          sets an EMPTY STRING (legal for tagline and description)
  --clear tagline       sets it to NULL
--clear takes a comma-separated list (` + strings.Join(setTextFieldNames, ", ") + `) and
cannot be combined with the matching value flag. Category accepts null but not
the empty string, so ` + "`--category \"\"`" + ` is refused here rather than by the server.

CATEGORY must be one of:
` + strings.Join(wrapRunes(strings.Join(appapi.MarketplaceCategories, ", "), 74), "\n") + `

This applies IN PLACE, on every listing status — these three fields are not
"material" changes, so unlike editing a name or an external URL they never open
a revision for moderator re-review. What the server reports is printed rather
than assumed.

The server rate-limits this to 30 edits per hour.`,
		Example: `  civitai app listing set-text --tagline "Batch upscaling, in your browser"
  civitai app listing set-text --category utility --slug my-app
  civitai app listing set-text --description "$(cat DESCRIPTION.md)"
  civitai app listing set-text --clear tagline,category`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			patch, err := buildListingTextPatch(cmd, tagline, description, category, clear)
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
			ref, err := resolveListing(ctx, client, slug)
			if err != nil {
				return err
			}
			res, err := client.UpdateListing(ctx, ref.AppListingID, patch)
			if err != nil {
				return err
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
	cmd.Flags().StringSliceVar(&clear, "clear", nil,
		"clear a field to null instead of setting it: "+strings.Join(setTextFieldNames, ", ")+" (comma-separated)")
	return cmd
}

// buildListingTextPatch turns the flags into a patch, refusing locally
// everything that can be refused locally.
//
// 🔴 IT READS `Flags().Changed`, NOT THE VALUE. A `--tagline ""` is a REAL edit
// (the empty string is a legal, distinct state), so deciding "was this flag
// given" from whether the string is empty would silently drop exactly the edit
// the tri-state exists to carry.
func buildListingTextPatch(cmd *cobra.Command, tagline, description, category string, clear []string) (appapi.ListingTextPatch, error) {
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
// 🔴 THE RESIDUAL, STATED RATHER THAN HIDDEN. `hasPendingRevision` means
// SUBMITTED, not "a shadow exists" (measured 2026-08-12 and recorded on
// appapi.ListingEditView). An open-but-unsubmitted shadow is therefore invisible
// here, and no side-effect-free read exposes one. This warning covers the case
// it can see and does not claim to cover the other.
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
		return
	}

	if ref != nil && ref.HasPendingRevision {
		se := ui.For(errOut)
		fmt.Fprintln(errOut, se.Warn("A revision of this listing is already under moderator review."))
		fmt.Fprintln(errOut, "  This edit went to the LIVE listing, not to that revision. If this app is")
		fmt.Fprintln(errOut, "  OFF-SITE, approving that revision copies its own tagline/description/category")
		fmt.Fprintln(errOut, "  back over the listing and would undo this edit; an ON-SITE app is unaffected,")
		fmt.Fprintln(errOut, "  because approving its revision copies only the icon and cover.")
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
