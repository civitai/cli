package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/ui"
	"github.com/spf13/cobra"
)

// `civitai app listing set-source-repo` — the OFF-SITE half of the public
// source-repository link (civitai/civitai#4314). It writes `sourceRepoUrl`, the
// value behind the one `Source` row on an app's `/apps` detail page.
//
// 🔴 A SEPARATE COMMAND FROM `set-text`, AND THE REASON IS MEASURED, NOT
// STYLISTIC. `set-text`'s own header argues at length that one command with
// flags beats several, because its three fields are ONE proc taking ONE patch.
// This field uses that same proc and that same patch object, so by that argument
// it belongs there. It does not, because it is MATERIAL and they are not.
//
// `MATERIAL_PATCH_FIELDS` is `['externalUrl', 'name', 'contentRating',
// 'sourceRepoUrl']`, and on an APPROVED listing the server's `approved` branch
// does this once ANY material field differs:
//
//	// MATERIAL change → stage on a shadow. The parent stays LIVE untouched; the
//	// FULL patch (material + trivial) is written to the shadow.
//
// (`<civitai>/src/server/services/blocks/offsite-listing.service.ts:1318-1339`,
// quoted from its own comment, read at origin/main 2026-08-27.) So a single
// patch carrying `--tagline` AND a source-repo change would stage BOTH — the
// tagline edit would stop applying in place and start waiting on a moderator.
// Adding a flag to `set-text` would therefore change what its EXISTING flags do,
// depending on whether an unrelated flag happened to be passed. That is the
// silent, action-at-a-distance shape this repo keeps getting bitten by.
//
// The separation is enforced by the TYPE SYSTEM rather than by discipline:
// `appapi.ListingPatch` is a closed interface and `ListingTextPatch` has no
// source-repo field to set. See its doc comment.
//
// 🔴 IT DOES NOT PRE-VALIDATE THE URL. See `appapi.ListingSourceRepoPatch.wire`:
// this CLI's only mirror of the server's repo-URL rule is the coarse `pattern`
// in the vendored manifest schema, which is measurably wrong in BOTH directions,
// and a second copy on this path would be a second thing to be wrong where the
// server answers authoritatively anyway.

func newAppListingSetSourceRepoCmd() *cobra.Command {
	var lc listingCommon
	var clear, jsonOut bool

	cmd := &cobra.Command{
		Use:   "set-source-repo [url]",
		Short: "Set (or clear) the public source-repository link on your listing",
		Long: `Publish a link to your app's PUBLIC SOURCE on its store detail page.

It renders as one ` + "`Source`" + ` row on the /apps DETAIL page — never on a
grid card — and is omitted entirely when unset.

Pass a repository ROOT url to set it, or --clear to remove it. Exactly one of
the two. The SERVER validates the url and this command does not second-guess
it, so a rejection comes back in the server's own words.

ON-SITE apps are REFUSED (exit 1 — a verdict about the app, not a bad
command): their link comes from the ` + "`repository`" + ` key in
block.manifest.json, which the platform re-syncs at every approved version.

🔴 THIS IS A MATERIAL CHANGE, unlike set-text. On an APPROVED listing the
server stages it on a REVISION instead of applying it, so the live page is
unchanged until a moderator approves that revision. This command reports which
branch the server took — it never guesses. On a draft or pending listing it
applies directly.

See the guide for the accepted hosts, what counts as a "change", and the
states that are refused outright.`,

		Example: `  civitai app listing set-source-repo https://github.com/me/my-app
  civitai app listing set-source-repo https://gitlab.com/me/my-app --slug my-app
  civitai app listing set-source-repo --clear
  civitai app listing set-source-repo https://github.com/me/my-app --json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			patch, err := buildListingSourceRepoPatch(args, clear)
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
			// 🔴 THE KIND GATE COMES BEFORE THE WRITE, AND BEFORE THE RESOLVE —
			// same ordering, and the same shared gate, as `set-text`. An onsite
			// listing's link is manifest-governed, so writing it here would be
			// re-synced away at the next approve.
			if err := refuseOnsiteEdit(ctx, client, slug, onsiteSubjectSourceRepo); err != nil {
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
				// The JSON path does NOT go through the human renderer:
				// internal/ui/CONVENTION.md rule 1 — machine-readable output
				// carries no styling.
				return writeJSON(cmd.OutOrStdout(), setSourceRepoPayload(slug, ref, res, patch))
			}
			reportListingSourceRepoUpdated(cmd.OutOrStdout(), slug, patch, ref, res)
			return nil
		},
	}
	lc.bind(cmd)
	cmd.Flags().BoolVar(&clear, "clear", false,
		"remove the source-repository link (sends an explicit null) instead of setting one")
	cmd.Flags().BoolVar(&jsonOut, "json", false,
		"emit the result as JSON (scriptable) — what was sent, and the server's own branch")
	return cmd
}

// buildListingSourceRepoPatch turns the positional URL and --clear into a patch.
//
// 🔴 NEITHER-NOR AND BOTH-AT-ONCE ARE BOTH USAGE ERRORS, refused before any
// request. "Set it to X" and "remove it" are contradictory instructions about
// one column on a public page, and picking a winner would apply an edit nobody
// asked for. The server would reject the empty patch too (its schema refines
// that at least one key is present), but as a 400 that costs a round trip and
// one of the ~30/hour rate-limit budget to say what is knowable locally.
func buildListingSourceRepoPatch(args []string, clear bool) (appapi.ListingSourceRepoPatch, error) {
	var p appapi.ListingSourceRepoPatch
	switch {
	case len(args) == 1 && clear:
		return p, asUsageError(fmt.Errorf(
			"pass a URL or --clear, not both — one sets the source-repository link, the other removes it"))
	case len(args) == 1:
		url := strings.TrimSpace(args[0])
		// 🔴 THE ONLY LOCAL CHECK ON THE VALUE, and it is about EMPTINESS rather
		// than shape. `updateListingPatchSchema` bounds this field
		// `z.string().min(1)`, so "" is not a legal value and there is no
		// "set empty" state to protect (unlike tagline/description, where the
		// empty string is a distinct, storable state). The usual cause is an
		// unset shell variable, so the message says so — and it names --clear,
		// which is what someone reaching for "no link" actually wants.
		if url == "" {
			return p, asUsageError(fmt.Errorf(
				"the source-repository URL is blank — an unset shell variable is the usual cause. " +
					"To REMOVE the link instead, pass --clear"))
		}
		p.URL = &url
	case clear:
		p.Clear = true
	default:
		return p, asUsageError(fmt.Errorf(
			"nothing to do — pass a repository URL to set the link, or --clear to remove it"))
	}
	return p, nil
}

// setSourceRepoJSON is the `--json` payload. PUBLISHED CONTRACT.
//
// 🔴 `requiresReview` / `shadowId` ARE THE POINT OF THIS PAYLOAD, not decoration.
// Unlike `set-text`, where they are structurally always false/nil today, this
// command REALLY DOES stage a revision on an approved listing — so a script that
// treated a 0 exit as "the link is live" would be wrong exactly half the time.
// They are the server's own answer, passed through, never this CLI's belief
// about which branch it took.
type setSourceRepoJSON struct {
	Slug         string `json:"slug"`
	AppListingID string `json:"appListingId"`
	// SourceRepoURL is what was SENT: the URL, or null when the link was cleared.
	SourceRepoURL *string `json:"sourceRepoUrl"`
	// Action is "set" or "cleared" — the discriminator, so a consumer never has
	// to infer intent from a null that also means "cleared".
	Action string `json:"action"`
	// RequiresReview / ShadowID are the SERVER's own branch, passed through.
	RequiresReview bool    `json:"requiresReview"`
	ShadowID       *string `json:"shadowId"`
	// OpenRevision reports whether a revision draft ALREADY existed before this
	// edit — read from the listing BEFORE the write, never from the result.
	//
	// 🔴 `shadowId` ALONE CANNOT ANSWER THIS, WHICH IS THE WHOLE REASON THE KEY
	// EXISTS. `beginListingRevision` is idempotent: it reuses an open shadow
	// rather than minting a second one, so a populated `shadowId` means "this
	// edit is on a revision" and says nothing about whether that revision was
	// already carrying somebody else's staged work. The two cases need different
	// handling and are indistinguishable without this key — see the renderer.
	//
	// The sibling `setTextJSON` publishes an `openRevision` too, and it must
	// stay spelled the same: two commands on one proc that disagree about the
	// name of the same hazard is a contract seam.
	OpenRevision bool `json:"openRevision"`
}

func setSourceRepoPayload(slug string, ref *appapi.ListingRef, res *appapi.UpdateListingResult, p appapi.ListingSourceRepoPatch) setSourceRepoJSON {
	out := setSourceRepoJSON{Slug: slug, Action: "cleared"}
	if ref != nil {
		out.AppListingID = ref.AppListingID
		out.OpenRevision = hadOpenRevision(ref)
	}
	if p.URL != nil {
		out.SourceRepoURL = p.URL
		out.Action = "set"
	}
	if res != nil {
		out.RequiresReview = res.RequiresReview
		out.ShadowID = res.ShadowID
	}
	return out
}

// hadOpenRevision reports whether the listing was ALREADY carrying a revision
// draft when this command read it — submitted (`hasPendingRevision`) or merely
// open (`shadowId`).
//
// 🔴 IT IS READ BEFORE THE WRITE, AND THAT ORDERING IS THE MEASUREMENT. After
// the write every staged edit has a `shadowId`, so asking afterwards cannot tell
// "the server opened a revision for MY change" from "my change joined one that
// already existed". `resolveListing` runs before `UpdateListing`, so this is
// free and it is the only moment the distinction is observable.
func hadOpenRevision(ref *appapi.ListingRef) bool {
	return ref != nil && (ref.ShadowID != nil || ref.HasPendingRevision)
}

// reportListingSourceRepoUpdated prints what changed and, when the server staged
// it, what is still needed to make it public.
//
// 🔴 A STAGED CHANGE IS NOT A PUBLISHED ONE, AND SAYING "Updated" ALONE WOULD BE
// FALSE. On an approved listing this edit lands on a shadow revision that nobody
// can see until a moderator approves it, so the success line has to distinguish
// the two outcomes rather than report the request having been accepted. AGENTS
// item 30 is the same rule for `rm-screenshot`'s deliberately-staged change.
//
// It is still exit 0: staging is what was asked for and it succeeded. What the
// user needs is the NEXT command, which is `submit-revision`.
//
// 🔴 "SEND IT FOR REVIEW" IS NOT UNCONDITIONALLY SAFE ADVICE, AND THAT IS WHY
// `ref` IS A PARAMETER. `beginListingRevision` is IDEMPOTENT — it reuses an
// already-open shadow rather than minting a second one
// (`<civitai>/src/server/services/blocks/offsite-listing.service.ts:1370-1380`).
// So when a revision was ALREADY open — an `rm-screenshot` deliberately left
// staged (AGENTS item 30), or one minted lazily by `set-icon` / `set-cover` /
// `add-screenshot` / `listing status --json` — this edit lands on THAT shadow,
// beside work the author did not mean to publish now. `applyApprovedRevision`
// then copies the shadow's WHOLE scalar set and its screenshots onto the parent
// on approval (`:3018+`), so following a bare "send it for review" publishes all
// of it and can revert a live tagline the shadow captured earlier.
//
// The distinction is only observable BEFORE the write (see hadOpenRevision), so
// it is measured there and reported here. Telling the author which of the two
// they are in costs one sentence; getting it wrong costs a public page.
func reportListingSourceRepoUpdated(out io.Writer, slug string, p appapi.ListingSourceRepoPatch, ref *appapi.ListingRef, res *appapi.UpdateListingResult) {
	st := ui.For(out)
	what := "Cleared the source-repository link on " + slug
	if p.URL != nil {
		what = fmt.Sprintf("Set the source-repository link on %s: %s", slug, *p.URL)
	}

	if res != nil && res.RequiresReview {
		fmt.Fprintln(out, st.Success(what+" — STAGED for review"))
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, st.Warn("This is a material change, so the server staged it on a REVISION rather than applying it."))
		fmt.Fprintln(out, "  The live listing is unchanged until a moderator approves the revision.")
		if res.ShadowID != nil && *res.ShadowID != "" {
			fmt.Fprintf(out, "  Revision: %s\n", *res.ShadowID)
		}
		if hadOpenRevision(ref) {
			// The pre-existing-revision case. Deliberately NOT phrased as "run
			// this next": the right next step depends on what else is staged,
			// which this command cannot see and must not guess at.
			fmt.Fprintln(out, "")
			if ref.HasPendingRevision {
				fmt.Fprintln(out, st.Warn("That revision was ALREADY under moderator review before this edit."))
			} else {
				fmt.Fprintln(out, st.Warn("That revision ALREADY existed and may carry other staged changes."))
			}
			fmt.Fprintln(out, "  Approving it publishes EVERYTHING staged on it, not just this link, and")
			fmt.Fprintln(out, "  copies its text back over the live listing.")
			fmt.Fprintf(out, "  Check what is on it first: %s\n", st.Code("civitai app listing status --slug "+slug))
			fmt.Fprintf(out, "  Then, if that is all meant to go public: %s\n", st.Code("civitai app listing submit-revision"))
			return
		}
		fmt.Fprintf(out, "  Send it for review: %s\n", st.Code("civitai app listing submit-revision"))
		return
	}

	fmt.Fprintln(out, st.Success(what))
}
