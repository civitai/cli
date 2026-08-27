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

It renders as one ` + "`Source`" + ` row on the /apps detail page — detail only,
never on a store grid card — and it is omitted entirely when unset.

The URL must be a repository ROOT on github.com, gitlab.com or codeberg.org:
https://<owner>/<repo>, with no deeper path. A trailing "/" or ".git", a query
string and a fragment are accepted and normalised away by the server. The
SERVER is the authority here and this command does not second-guess it, so an
unacceptable URL comes back as the server's own message rather than a local
guess that may be wrong in either direction.

Pass a URL to set it, or --clear to remove the link. Exactly one of the two.

ON-SITE apps are REFUSED (exit 1 — a verdict about the app, not a bad command):
their link comes from the ` + "`repository`" + ` key in block.manifest.json and the
platform re-syncs it from there at your next approved version.

🔴 THIS IS A MATERIAL CHANGE. On an APPROVED listing the server does NOT apply
it in place: it stages the edit on a revision and the listing re-enters
moderator review, because this is an outbound link on a public page. The
command reports which branch the server took, and names the command that sends
a staged revision for review. On a draft or pending listing it applies
directly.`,
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
			reportListingSourceRepoUpdated(cmd.OutOrStdout(), slug, patch, res)
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
}

func setSourceRepoPayload(slug string, ref *appapi.ListingRef, res *appapi.UpdateListingResult, p appapi.ListingSourceRepoPatch) setSourceRepoJSON {
	out := setSourceRepoJSON{Slug: slug, Action: "cleared"}
	if ref != nil {
		out.AppListingID = ref.AppListingID
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
func reportListingSourceRepoUpdated(out io.Writer, slug string, p appapi.ListingSourceRepoPatch, res *appapi.UpdateListingResult) {
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
		fmt.Fprintf(out, "  Send it for review: %s\n", st.Code("civitai app listing submit-revision"))
		return
	}

	fmt.Fprintln(out, st.Success(what))
}
