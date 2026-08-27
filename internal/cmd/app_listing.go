package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/manifest"
	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// Per-kind SOURCE-FILE byte caps, mirroring the server's listing-asset caps:
//   - icon uses the inline data-URI path (server decoded cap 2 MiB).
//   - cover / screenshot use the mint+persist full-res path (4 MiB / 2 MiB).
//
// 🔴 FOR AN ICON THIS IS NOT THE BINDING CAP, AND CALLING IT "THE" CAP IS WHAT
// ISSUE #344 WAS. The listing schema separately caps the icon the SERVER made —
// the PNG it re-encodes after downscaling to at most 1024 px on the longer side —
// at 1 MiB, and that is a different measurement from the file the author passed.
// maxIconBytes is deliberately NOT lowered to match it: the two count different
// bytes, so a 1 MiB source gate would refuse valid images (a 1.5 MiB 512x512
// source re-encodes far under 1 MiB) while still not predicting the rejection it
// was lowered for. See AGENTS.md item 25 and attachRejectionAdvice below.
const (
	maxIconBytes       = 2 * 1024 * 1024
	maxCoverBytes      = 4 * 1024 * 1024
	maxScreenshotBytes = 2 * 1024 * 1024
)

// scanPollInterval / scanPollTimeout govern the post-ingest scan poll. Package
// vars so tests can shrink them; never zero the interval in prod (it would busy-loop).
var (
	scanPollInterval = 2 * time.Second
	scanPollTimeout  = 120 * time.Second
)

// listingImageFormats is the accepted source set, mirroring what
// appapi.DecodeImageInfo will actually decode (png/jpeg/webp — the server's
// LISTING_ASSET_ALLOWED_MIME). Stated once so the six help bodies below cannot
// disagree with each other.
const listingImageFormats = "png, jpeg or webp"

// liveRevisionHelp is what the three attach commands tell you about a LIVE
// listing. Stated once because it was stated three times and the three copies
// have to agree: the below-floor sentence was added after #400 shipped the
// behaviour, and three separate paragraphs are three chances to update two.
//
// It carries the floor exception because `--help` is the contract a reader has
// OFFLINE, and without it the help asserts a moderator re-review that a
// below-floor listing deliberately does not get. It is ONE CLAUSE because
// TestListingHelpStaysWithinTheBudget caps each Long at 1400 runes and this
// constant is paid by THREE bodies at once — measured on this tree,
// add-screenshot sits at ~1372 of 1400, so roughly 28 runes of headroom decide
// whether the next edit here reddens the suite. Lengthen the README instead;
// that budget exists to send prose there.
const liveRevisionHelp = `On a listing that is already LIVE this opens a REVISION for moderator re-review
instead of changing the live listing — pass --changelog to describe the change,
-y to skip the confirmation — but a live listing still below the publish floor
stages WITHOUT submitting, and exits 0. On a DRAFT listing it attaches directly.`

// listingSourceRule renders the one sentence describing what a source file for
// `kind` may be.
//
// 🔴 IT IS COMPUTED FROM THE CAP CONSTANTS AND MUST STAY THAT WAY — the number
// in the help has to be the number loadAndValidateImage enforces, and it is
// rendered through the SAME humanBytes the refusal message uses, so `--help`
// predicts the error text byte-for-byte ("larger than the 2.0 MiB max") instead
// of quoting a differently-rounded prose figure. A hand-typed "2 MiB" here is a
// second copy of a constant: it reads fine, it survives every test that only
// greps for a cap being mentioned, and it goes stale the day a cap moves.
// TestListingHelpQuotesTheEnforcedCaps pins the coupling in both directions.
func listingSourceRule(kind mediaKind) string {
	return fmt.Sprintf("%s, at most %s", listingImageFormats, humanBytes(int64(kindByteCap(kind))))
}

// newAppListingCmd is the `civitai app listing` group: attach the store-listing
// MEDIA (icon / cover / screenshots) an app needs to clear the publish floor,
// without the browser. Operates on the app in the current directory (or --slug).
func newAppListingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "listing",
		Short: "Manage your App store-listing media (icon, cover, screenshots)",
		Long: `Manage your App store listing's MEDIA — the icon, cover, and screenshots
a listing needs before it can publish.

A store listing must have an ICON and a COVER before it can go live; screenshots
are optional. These commands ingest a local image, attach it to your listing,
and then wait for the content scan — the same pipeline the web submit form
uses. The platform validates dimensions, aspect and format at the ATTACH step,
so a wrongly-shaped image is refused in seconds rather than after the scan.

For a listing that is already LIVE (approved), attaching media opens a REVISION
that goes back to moderator review (the live listing is untouched until the
revision is approved); pass --changelog to describe the change.

The app is resolved from block.manifest.json in the current directory (or pass
--slug). ` + "`civitai app submit`" + ` mints your listing as a DRAFT, so its media is
settable while pending review. It will
` + strings.Join(wrapRunes(withdrawListingCaveat+".", 78), "\n") + `
Do not hand-caption a set you may withdraw.

Source files are checked locally BEFORE any upload — ` + listingImageFormats + `, at most
` + humanBytes(maxIconBytes) + ` for an icon, ` + humanBytes(maxCoverBytes) + ` for a cover, ` + humanBytes(maxScreenshotBytes) + ` for a screenshot.
That reads the SOURCE FILE only: an icon is re-encoded server-side and the
platform caps the image IT made — it can pass here and be refused at attach.`,
		Example: `  civitai app listing status
  civitai app listing set-text --tagline "Batch upscaling, in your browser"
  civitai app listing set-icon ./assets/icon.png
  civitai app listing set-cover ./assets/cover.png
  civitai app listing add-screenshot ./shot.png --caption "Grid view"
  civitai app listing rm-screenshot alsc_01H...
  civitai app listing reorder alsc_02 alsc_01 alsc_03
  civitai app listing submit-revision --changelog "Refreshed the gallery"`,
	}
	cmd.AddCommand(newAppListingStatusCmd())
	cmd.AddCommand(newAppListingSetTextCmd())
	cmd.AddCommand(newAppListingSetSourceRepoCmd())
	cmd.AddCommand(newAppListingSetIconCmd())
	cmd.AddCommand(newAppListingSetCoverCmd())
	cmd.AddCommand(newAppListingAddScreenshotCmd())
	cmd.AddCommand(newAppListingRmScreenshotCmd())
	cmd.AddCommand(newAppListingReorderCmd())
	cmd.AddCommand(newAppListingSubmitRevisionCmd())
	return cmd
}

// listingCommon holds the shared --slug/--dir resolution flags.
type listingCommon struct {
	slug string
	dir  string
}

func (lc *listingCommon) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&lc.slug, "slug", "", "app slug (defaults to block.manifest.json's blockId)")
	cmd.Flags().StringVar(&lc.dir, "dir", ".", "app directory holding block.manifest.json (when --slug is not given)")
}

// newListingClient loads config + builds an authed App Blocks client, erroring
// clearly when no credential is configured.
func newListingClient() (*appapi.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if cfg.Token() == "" {
		return nil, civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf("no token configured — run `civitai login` (or set CIVITAI_TOKEN)"))
	}
	return appapi.NewWithSource(cfg.BaseURL(), auth.New(cfg), ""), nil
}

// listingSlugResolveFailure is the prefix resolveListingSlug puts on a manifest
// failure. It is a named constant so a test can assert POSITIVELY that a row
// actually REACHED this function, rather than merely that it did not fail at one
// specific earlier gate — see TestUngatedPathFlagsAreNotUsageErrors, and AGENTS
// item 25 for why a denylist-of-one premise let the defect regenerate twice.
const listingSlugResolveFailure = "could not resolve the app — run this from your app directory (with block.manifest.json) or pass --slug"

// resolveListingSlug resolves the app slug from --slug or the manifest.
func resolveListingSlug(lc listingCommon) (string, error) {
	if lc.slug != "" {
		return lc.slug, nil
	}
	m, err := manifest.Load(lc.dir)
	if err != nil {
		return "", fmt.Errorf(listingSlugResolveFailure+": %w", err)
	}
	if m.BlockID == "" {
		// The remedy is a flag, so this is a usage error (exit 2). The sibling
		// above is deliberately left alone: it wraps an opaque manifest.Load
		// cause (missing file, permissions, bad JSON), which is not a usage
		// mistake.
		return "", asUsageError(fmt.Errorf("block.manifest.json has no blockId — pass --slug"))
	}
	return m.BlockID, nil
}

// resolveListing resolves a slug to the listing ref every `app listing`
// subcommand addresses: through the app's block SUBMISSION when it has one, and
// by the SLUG ALONE when it does not.
//
// 🔴 A submission with NO appBlockId is a FIRST-version app still pending review. It
// still has a store listing: `civitai app submit` mints one as a pre-approval DRAFT
// so its media is settable while pending. That draft has `appBlockId = NULL`, so the
// SLUG IS THE ONLY SELECTOR THAT CAN NAME IT — the appBlockId arm is skipped entirely
// when the CLI has none to send. That is why the slug goes on BOTH calls (redundant
// when the appBlockId resolves, the only handle when it is absent) and why
// short-circuiting to a "no listing" error here would strand the pending-media flow.
//
// 🔴 "ONLY BY SLUG" IS A CLAIM ABOUT WHICH SELECTOR THE CLI CAN SEND, NEVER ABOUT HOW
// WIDE THE SERVER'S SLUG ARM IS — civitai/cli#424, where this comment asserted the
// second while only ever having evidence for the first. Against the server it was
// written for, the arm was `{slug, kind:'onsite', appBlockId:null, status:'draft'}`:
// four clauses that admitted the pre-approval draft and nothing else, so every OTHER
// shape 404ed by slug — `gen-matrix` and `custom-generators` (onsite, approved, so
// carrying an appBlockId) failed clauses 2-3, `radio` and `comfy` failed clause 1.
// civitai/civitai#3989 then dropped it to `{slug, revisionOfId: null}`, which is what
// the paragraph below rests on; re-verified against civitai/civitai origin/main at
// `src/server/services/blocks/offsite-listing.service.ts:1746`. The pre-approval draft
// is now ONE MEMBER of a wide set rather than the only thing the arm admits, so do not
// re-derive a narrow server scope from this paragraph — read the server, it moves.
//
// 🟡 AND THE PENDING CASE IS ASSERTED FROM SERVER SOURCE, NOT MEASURED. No genuinely
// `pending` first-version onsite app was ever probed: the only two `appBlockId = nil`
// rows reachable on 2026-08-17 were WITHDRAWN, and the old clause keyed on the
// LISTING's status rather than the submission's, so they could not answer it either
// way. "The server supports it" is therefore unverified, not proven — do not upgrade
// it to a measurement, and do not downgrade it to a doubt either. Nothing turns on it
// today: the widened clause admits the row on the slug whatever its status, so this is
// archaeology. civitai/cli#424 carries the recipe if anyone wants to settle it.
//
// 🔴 THE SUBMISSION LOOKUP IS THE ONSITE PATH, NOT THE ONLY PATH — civitai/cli#422
// outcome 1, and this ordering is the whole of it. An OFFSITE app is a registered
// URL rather than a block bundle, so it has NO submission and the lookup above
// 404s; before this fallback existed, every `app listing` subcommand died there
// and told the author to run `civitai app submit`, a command that cannot succeed
// for such an app. The repair is the by-slug arm of `getMyListingForApp`, which
// `civitai/civitai#3989` rescoped from `{slug, kind:'onsite', appBlockId:null,
// status:'draft'}` to `{slug, revisionOfId:null}`. Measured live 2026-08-17
// against https://civitai.com: `radio`, `comfy`, `cosmetic-studio` and `vitrine`
// (4/4 offsite, all approved) each resolved BY SLUG to their `apl_` listing,
// `gen-matrix` (onsite, approved) still resolved, and `definitely-not-an-app-zzz`
// still 404ed. Every listing-keyed proc downstream — getMyListingForEdit, the
// asset attaches, the revision procs — gates on OWNERSHIP and never on kind, so
// one resolved ref is all any of these subcommands needed.
//
// 🔴 THE WIDENED SERVER CLAUSE `{slug, revisionOfId: null}` CARRIES NO USER
// PREDICATE, AND THE OWNERSHIP GATE IS THE *NEXT* STATEMENT — verified from
// source rather than assumed, because the refusal copy ("a listing this account
// does not own") and the "downstream gates on ownership" argument above both
// depend on it. `civitai/civitai → src/server/services/blocks/offsite-listing.service.ts`:
// the slug `findFirst` is at :1744-1749, and `resolveListingRole(listing.id,
// userId) === null → OffsiteRequestError('NOT_OWNED')` at :1772-1774; the router
// maps NOT_OWNED → FORBIDDEN (`app-listings.router.ts:637-654`, via
// `mapOffsiteError`). So the selector is session-scoped after resolution: a
// stranger's slug resolves a row and is then refused 403 — it never returns
// someone else's listing, and it never reads as "not found".
//
// 🔴 ONLY ErrNotFound FALLS THROUGH — FROM *EITHER* LOOKUP — and the refusal below
// NARROWS rather than disappears. A 403 from the invite-gated submissions route, a
// 5xx or a transport failure keep their own error and their own exit code; none of
// them is evidence that there is no submission, and spending a second request on
// one would be guessing. The by-slug arm is held to the same rule (see the gate
// below). What still reaches explainOffsiteMiss is the case where BOTH lookups
// answered NOT-FOUND: a server that predates #3989 (older or self-hosted), or an
// app with no listing row at all. A listing this account does not OWN is NOT in
// that set on a current server — it is the 403 above — and reaches the refusal
// only on a server predating #3989, whose narrower `where` clause makes it 404.
//
// 🔴 THE ONSITE HAPPY PATH IS UNCHANGED AND COSTS THE SAME TWO REQUESTS. The
// fallback is on the submission lookup's ERROR path only, which is the same
// argument app_offsite.go's probe rests on — pinned by
// TestSuccessfulCommandsMakeNoStoreProbe and TestOnsiteHappyPathMakesNoExtraCall.
//
// 🔴 AND THE FALLBACK'S OWN ERROR IS NOT THE SUBMISSION'S 404 — ONLY ITS 404
// FALLS THROUGH. The first cut wrote `if ref, slugErr := …; slugErr == nil`, which
// DISCARDED slugErr: a 403, a 500 or a transport failure from the by-slug arm then
// fell through carrying the SUBMISSION's not-found, so the command exited 4 under
// a message naming three causes — old server, no listing row, not your listing —
// none of which is "the listings route errored". The gate below is the same shape
// as the one above it: a not-found from either lookup means the resource is
// absent, and anything else is a failure that must keep its own text and its own
// exit code. Pinned by TestFallbackFailuresKeepTheirOwnError, which drives 403 /
// 500 / 503 through the fallback and asserts the classification, the absence of
// the OFFSITE claim, and that the kind probe never ran.
//
// 🟡 THE FALLBACK DELIBERATELY INHERITS THE CLIENT'S PER-REQUEST BUDGET (30 s,
// appapi's defaultTimeout) RATHER THAN TAKING A SHORT ONE OF ITS OWN, and that is
// the OPPOSITE call from offsiteProbeTimeout — for the reason that constant's own
// doc comment gives. offsiteProbeTimeout is short because the command has ALREADY
// failed by the time the probe runs and everything it can still do is improve
// wording. This call is not that: for an OFFSITE app it is the ANSWER, the only
// lookup that resolves the listing every subcommand then addresses. Capping it at
// a diagnostic's 5 s would fail the offsite path outright on a slow-but-working
// server, trading a real answer for latency. The COST, stated rather than hidden:
// the worst case of the "I have not submitted yet" error path is now three bounded
// requests — submissions (30 s) + this (30 s) + the kind probe (5 s) — where it
// was two. If that is ever measured to matter, the knob goes beside
// offsiteProbeTimeout, not here.
func resolveListing(ctx context.Context, client *appapi.Client, slug string) (*appapi.ListingRef, error) {
	sub, err := client.GetSubmission(ctx, "", slug)
	if err != nil {
		if errors.Is(err, civitai.ErrNotFound) {
			ref, slugErr := client.GetMyListingForApp(ctx, "", slug)
			if slugErr == nil {
				return ref, nil
			}
			if !errors.Is(slugErr, civitai.ErrNotFound) {
				return nil, slugErr
			}
		}
		// Both lookups answered NOT-FOUND (or the submission failure was one and
		// no fallback was warranted). Only the error path pays for the kind
		// probe; see app_offsite.go.
		return nil, explainOffsiteMiss(ctx, slug, err, offsiteListingRefusal)
	}
	appBlockID := ""
	if sub.AppBlockID != nil {
		appBlockID = *sub.AppBlockID
	}
	return client.GetMyListingForApp(ctx, appBlockID, slug)
}

// ----------------------------------------------------------------------------
// status
// ----------------------------------------------------------------------------

func newAppListingStatusCmd() *cobra.Command {
	var lc listingCommon
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show attached media and what's missing vs the publish floor",
		Long: `Show your store listing's attached media (icon, cover, screenshots) and what
is still required before it can publish (an icon and a cover are mandatory).

Your store listing exists as a DRAFT from the moment you run
` + "`civitai app submit`" + `, so this works while your app is still pending review.

Note: on a LIVE (approved) listing this opens an in-progress revision draft and
reports ITS media (idempotent — it reuses any existing draft, and nothing is
submitted for moderator review until you run a set-/add- command and confirm).

--json emits the same read as one object: parentId, shadowId (null when there
is no revision draft), status, hasPendingRevision, the attached media with
their image ids, and the publish-floor verdict. The parent and the shadow are
DIFFERENT listings and a change is addressed to one of them, so a script that
has to decide which needs both ids. The server's own editTargetId is NOT in
there: this CLI does not decode that field, and reporting an id it never read
would be a guess.

🔴 This is not a pure read — with or without --json — so do not poll it in a
loop: on a LIVE listing it opens the shadow revision described above, so a
script calling it repeatedly keeps a revision draft open on your listing. That
holds for an OFFSITE app too, and they are approved in practice.
civitai/cli#389 settled that a FAILED call writes nothing; a successful one
still does.`,
		Example: `  civitai app listing status
  civitai app listing status --slug my-app
  civitai app listing status --dir ./my-app
  civitai app listing status --json | jq -r .shadowId`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := newListingClient()
			if err != nil {
				return err
			}
			slug, err := resolveListingSlug(lc)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			ref, err := resolveListing(ctx, client, slug)
			if err != nil {
				return err
			}
			view, err := client.GetMyListingForEdit(ctx, ref.AppListingID)
			if err != nil {
				return err
			}
			// 🔴 The JSON path does NOT go through printListingStatus, and must
			// not: `internal/ui/CONVENTION.md` rule 1 is that machine-readable
			// output carries no styling at all, and that renderer emits
			// ui.Success/ui.Warn/ui.Code.
			if jsonOut {
				return writeJSON(cmd.OutOrStdout(), listingStatusPayload(slug, ref, view))
			}
			printListingStatus(cmd.OutOrStdout(), slug, ref, view)
			return nil
		},
	}
	lc.bind(cmd)
	cmd.Flags().BoolVar(&jsonOut, "json", false,
		"emit the listing as JSON (scriptable) — includes the parent and shadow listing ids; NOT a pure read (see the note in --help)")
	return cmd
}

// listingStatusJSON is the machine-readable shape of `app listing status`
// (civitai/cli#447). It is a PUBLISHED CONTRACT — README documents it — so a
// field is renamed or dropped only deliberately.
//
// 🔴 IT NAMES THE TWO ID SPACES, WHICH IS THE WHOLE REASON IT EXISTS. The human
// rendering prints neither `parentId` nor `shadowId`, and civitai/cli#430 was a
// bug that lived entirely in the gap between them: `reorder` addressed the
// PARENT while `status` printed the SHADOW revision's screenshot rows, so the
// server rejected ids the CLI had just displayed and diagnosing it took
// hand-written tRPC calls.
//
// 🔴 `editTargetId` IS DELIBERATELY ABSENT, and it is what the issue asked for.
// The server sends it on an approved listing, but `appapi.ListingRef` does not
// decode it (see the 🔴 note on that type for why — it has been observed once,
// and it would be a fourth way to name an edit target). This payload therefore
// reports the two ids the CLI actually read; inventing the third from
// `shadowId` would be this CLI asserting a value the server never handed it on
// this call. If that decode ever lands, adding the field here is the follow-on.
type listingStatusJSON struct {
	// Slug is the app this was resolved for — the value the human line prints
	// as "App:", from --slug or block.manifest.json, NOT from the server.
	Slug string `json:"slug"`
	// ParentID is the listing that owns the media, from getMyListingForEdit.
	ParentID string `json:"parentId"`
	// ShadowID is the in-flight revision draft, or null when there is none.
	// 🔴 Explicitly null rather than omitted: `.shadowId` must answer for every
	// listing, so a consumer never has to tell "no revision" from "old CLI".
	ShadowID *string `json:"shadowId"`
	// Status is the PARENT's lifecycle status (draft|pending|approved|…), from
	// the getMyListingForApp lookup — the same value the human line prints.
	Status string `json:"status"`
	// HasPendingRevision means SUBMITTED, not "a shadow exists" — see the
	// measurement in printListingStatus. It is the edit read's value.
	HasPendingRevision bool              `json:"hasPendingRevision"`
	Assets             listingAssetsJSON `json:"assets"`
	Floor              listingFloorJSON  `json:"floor"`
}

type listingAssetsJSON struct {
	Icon  listingAssetJSON `json:"icon"`
	Cover listingAssetJSON `json:"cover"`
	// Screenshots is never null — an empty gallery is [].
	Screenshots []listingScreenshotJSON `json:"screenshots"`
}

// listingAssetJSON is one required slot. `present` is the CLI's own predicate
// (appapi.ListingAsset.Present: a positive imageId), stated rather than left for
// a consumer to re-derive from the id.
type listingAssetJSON struct {
	Present bool `json:"present"`
	ImageID *int `json:"imageId"`
}

// listingScreenshotJSON is one gallery row. `id` is the `alsc_…` value
// `rm-screenshot` and `reorder` take, and on a live listing it belongs to the
// SHADOW named above — which is exactly the pairing #430 needed and could not
// see.
type listingScreenshotJSON struct {
	ID      string  `json:"id"`
	Order   int     `json:"order"`
	Caption *string `json:"caption"`
	ImageID *int    `json:"imageId"`
}

// listingFloorJSON is the publish-floor verdict. `missing` is never null — a met
// floor is [] — and it holds the same names, in the same order, that the human
// rendering prints.
type listingFloorJSON struct {
	Met     bool     `json:"met"`
	Missing []string `json:"missing"`
}

// listingStatusPayload builds the --json object from the two reads the command
// has already made. No styling, no writer: everything it returns is data.
func listingStatusPayload(slug string, ref *appapi.ListingRef, view *appapi.ListingEditView) listingStatusJSON {
	out := listingStatusJSON{
		Slug:               slug,
		ParentID:           view.ParentID,
		ShadowID:           view.ShadowID,
		Status:             ref.Status,
		HasPendingRevision: view.HasPendingRevision,
		Floor: listingFloorJSON{
			Met:     floorMet(view),
			Missing: floorMissing(view),
		},
	}
	out.Assets.Icon = listingAssetJSON{Present: view.Assets.Icon.Present(), ImageID: view.Assets.Icon.ImageID}
	out.Assets.Cover = listingAssetJSON{Present: view.Assets.Cover.Present(), ImageID: view.Assets.Cover.ImageID}
	out.Assets.Screenshots = make([]listingScreenshotJSON, 0, len(view.Assets.Screenshots))
	for _, s := range view.Assets.Screenshots {
		out.Assets.Screenshots = append(out.Assets.Screenshots, listingScreenshotJSON{
			ID: s.ID, Order: s.Order, Caption: s.Caption, ImageID: s.ImageID,
		})
	}
	return out
}

func printListingStatus(w io.Writer, slug string, ref *appapi.ListingRef, view *appapi.ListingEditView) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "App:\t%s\n", slug)
	fmt.Fprintf(tw, "Listing status:\t%s\n", ref.Status)
	fmt.Fprintf(tw, "Icon:\t%s\n", assetLabel(view.Assets.Icon.Present(), true))
	fmt.Fprintf(tw, "Cover:\t%s\n", assetLabel(view.Assets.Cover.Present(), true))
	fmt.Fprintf(tw, "Screenshots:\t%d\n", len(view.Assets.Screenshots))
	_ = tw.Flush()

	for _, s := range view.Assets.Screenshots {
		caption := ""
		if s.Caption != nil && *s.Caption != "" {
			caption = "  — " + *s.Caption
		}
		fmt.Fprintf(w, "  %s%s\n", s.ID, caption)
	}

	iconOK := view.Assets.Icon.Present()
	coverOK := view.Assets.Cover.Present()
	fmt.Fprintln(w)
	if floorMet(view) {
		fmt.Fprintln(w, ui.Success("Publish floor met — icon and cover are set."))
	} else {
		fmt.Fprintln(w, ui.Warn(fmt.Sprintf("Not publishable yet — missing %s.", strings.Join(floorMissing(view), " and "))))
		if !iconOK {
			fmt.Fprintf(w, "  Add one:  %s\n", ui.Code("civitai app listing set-icon <file>"))
		}
		if !coverOK {
			fmt.Fprintf(w, "  Add one:  %s\n", ui.Code("civitai app listing set-cover <file>"))
		}
	}
	// `hasPendingRevision` means SUBMITTED, not "a shadow exists" — measured
	// 2026-08-12 on an approved listing carrying an open shadow
	// (`shadowId: apl_01KZVG3M…`, media attached, never submitted): the server
	// still reported `hasPendingRevision: false`. That matters because #400's
	// below-floor path deliberately LEAVES such a shadow open, and if the flag
	// tracked existence instead this line would tell the author a moderator was
	// reviewing something that had not been sent. It does not.
	if view.HasPendingRevision {
		fmt.Fprintln(w, "\nA revision is currently under moderator review.")
	} else if ref.Status == "approved" {
		fmt.Fprintln(w, "\nThis listing is live — media changes open a revision for moderator re-review.")
	}
}

func assetLabel(present, required bool) string {
	if present {
		return "set ✓"
	}
	if required {
		return "MISSING (required)"
	}
	return "not set"
}

// ----------------------------------------------------------------------------
// set-icon / set-cover / add-screenshot (the shared attach flow)
// ----------------------------------------------------------------------------

// mediaKind is the asset being attached.
type mediaKind string

const (
	kindIcon       mediaKind = "icon"
	kindCover      mediaKind = "cover"
	kindScreenshot mediaKind = "screenshot"
)

func newAppListingSetIconCmd() *cobra.Command {
	var lc listingCommon
	var changelog string
	var assumeYes bool
	cmd := &cobra.Command{
		Use:   "set-icon <file>",
		Short: "Set the listing icon (" + listingSourceRule(kindIcon) + ")",
		Long: `Set your store listing's ICON — the small image shown beside your app's name.
An icon is MANDATORY: a listing cannot publish without one.

The source file is validated locally first (` + listingSourceRule(kindIcon) + `),
then ingested and attached, and the content scan is waited on afterwards.
Nothing is uploaded if the local check fails. The platform validates the
image's dimensions and aspect at the ATTACH step, so a wrongly-shaped image is
refused in seconds rather than after the scan.

An icon is also RE-ENCODED server-side to PNG (downscaled to at most 1024px on
the longer side) and the platform caps that re-encoded image — a different
measurement from the cap above, which is on your file. A detailed 1024x1024
icon can pass here and be refused there; the lever is smaller pixel dimensions.
See "Listing media requirements" in the README for the platform's bounds.

` + liveRevisionHelp + `

Run ` + "`civitai app listing status`" + ` to see what the publish floor still needs.`,
		Example: `  civitai app listing set-icon ./assets/icon.png
  civitai app listing set-icon ./icon.png --slug my-app
  civitai app listing set-icon ./icon.png --changelog "New brand mark" -y`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetMedia(cmd, kindIcon, args[0], "", lc, changelog, assumeYes)
		},
	}
	lc.bind(cmd)
	bindRevisionFlags(cmd, &changelog, &assumeYes)
	return cmd
}

func newAppListingSetCoverCmd() *cobra.Command {
	var lc listingCommon
	var changelog string
	var assumeYes bool
	cmd := &cobra.Command{
		Use:   "set-cover <file>",
		Short: "Set the listing cover (" + listingSourceRule(kindCover) + ")",
		Long: `Set your store listing's COVER — the wide image at the top of the listing
page. A cover is MANDATORY: a listing cannot publish without one.

The source file is validated locally first (` + listingSourceRule(kindCover) + `),
then ingested and attached, and the content scan is waited on afterwards.
Nothing is uploaded if the local check fails. The platform validates the
image's dimensions and aspect at the ATTACH step, so a wrongly-shaped image is
refused in seconds rather than after the scan.
See "Listing media requirements" in the README for the platform's bounds.

` + liveRevisionHelp + `

Run ` + "`civitai app listing status`" + ` to see what the publish floor still needs.`,
		Example: `  civitai app listing set-cover ./assets/cover.png
  civitai app listing set-cover ./cover.jpg --slug my-app
  civitai app listing set-cover ./cover.png --changelog "Updated hero" -y`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetMedia(cmd, kindCover, args[0], "", lc, changelog, assumeYes)
		},
	}
	lc.bind(cmd)
	bindRevisionFlags(cmd, &changelog, &assumeYes)
	return cmd
}

func newAppListingAddScreenshotCmd() *cobra.Command {
	var lc listingCommon
	var changelog string
	var assumeYes bool
	var caption string
	cmd := &cobra.Command{
		Use:   "add-screenshot <file>",
		Short: "Add a screenshot (up to 8) with an optional caption",
		Long: `Add a SCREENSHOT to your store listing's gallery. Screenshots are OPTIONAL:
they are not part of the publish floor.

The source file is validated locally first (` + listingSourceRule(kindScreenshot) + `),
then ingested and appended to the gallery, and the content scan is waited on
afterwards. Nothing is uploaded if the local check fails. The platform
validates dimensions, aspect and format at the ATTACH step, so a bad image is
refused in seconds rather than after the scan. --caption adds a one-line
caption.
See "Listing media requirements" in the README for the platform's bounds.

Each run appends one screenshot; there is no bulk add. Use
` + "`civitai app listing reorder`" + ` to change the order afterwards and
` + "`civitai app listing rm-screenshot`" + ` to drop one — both take the screenshot ids
that ` + "`civitai app listing status`" + ` prints.

The gallery has a ceiling (8 at the time of writing) and it is the SERVER's, not
this CLI's: nothing here counts the gallery before uploading, so hitting the
ceiling surfaces as a server refusal after the ingest rather than as a local
usage error.

` + liveRevisionHelp,
		Example: `  civitai app listing add-screenshot ./shot.png
  civitai app listing add-screenshot ./grid.png --caption "Grid view"
  civitai app listing add-screenshot ./shot.png --slug my-app`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetMedia(cmd, kindScreenshot, args[0], caption, lc, changelog, assumeYes)
		},
	}
	lc.bind(cmd)
	bindRevisionFlags(cmd, &changelog, &assumeYes)
	cmd.Flags().StringVar(&caption, "caption", "", "optional one-line caption for the screenshot")
	return cmd
}

func bindRevisionFlags(cmd *cobra.Command, changelog *string, assumeYes *bool) {
	cmd.Flags().StringVar(changelog, "changelog", "", "changelog for the moderator review (used only when the listing is already live)")
	cmd.Flags().BoolVarP(assumeYes, "yes", "y", false, "skip the live-listing revision confirmation")
}

// runSetMedia is the shared resolve -> validate -> ingest -> attach -> scan flow
// for set-icon / set-cover / add-screenshot, branching on the listing status for
// the live-listing shadow-revision path.
func runSetMedia(cmd *cobra.Command, kind mediaKind, file, caption string, lc listingCommon, changelog string, assumeYes bool) error {
	out := cmd.OutOrStdout()
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// 1. Local image validation BEFORE any network call.
	data, info, err := loadAndValidateImage(kind, file)
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

	// 2. Resolve the listing + branch on status.
	ref, err := resolveListing(ctx, client, slug)
	if err != nil {
		return err
	}
	switch ref.Status {
	case "rejected":
		return fmt.Errorf("this listing was rejected — resubmit the app before editing its media")
	case "removed":
		return fmt.Errorf("this listing was removed by a moderator and can no longer be edited")
	}
	live := ref.Status == "approved"

	// 3. For a live listing, confirm the revision + resolve a changelog first, so
	// we don't ingest/upload before the user has agreed to open a review cycle.
	if live {
		changelog, err = confirmLiveRevision(cmd, kind, changelog, assumeYes)
		if err != nil {
			return err
		}
	}

	// 4. Ingest bytes -> imageId (icon: inline data-URI; cover/screenshot: full-res).
	//
	// The PIXEL DIMENSIONS are printed beside the byte count (issue #295). They
	// cost nothing — loadAndValidateImage already decoded them from the header to
	// pick the MIME type, and threw them away on the icon path. They assert no
	// threshold and vendor no constant (AGENTS.md item 25 forbids that); they make
	// the ONE quantity every server-side listing-media bound is a function of
	// legible, so an author can compare it against the README's documented bounds
	// instead of reading a rejection that names a number their file does not have.
	fmt.Fprintf(out, "Uploading %s (%s, %d×%d)…\n", kind, humanBytes(int64(len(data))), info.Width, info.Height)
	var imageID int
	if kind == kindIcon {
		imageID, err = client.IngestAssetFromDataURI(ctx, data, info.MimeType)
	} else {
		imageID, err = client.IngestAssetFullRes(ctx, data, info)
	}
	if err != nil {
		return attachRejectionAdvice(err, kind, file, len(data), info)
	}

	// 5. Attach — direct for draft/pending, via a shadow revision when live.
	//
	// 🔴 ATTACH BEFORE THE SCAN POLL, AND THE ORDER IS THE FIX (issue #270). The
	// server validates GEOMETRY, ASPECT, MIME and BYTE SIZE at ATTACH, not at
	// ingest: `validateListingImage` runs inside `loadValidatedImage`
	// (civitai/civitai → src/server/services/blocks/app-listing-assets.service.ts)
	// BEFORE the ingestion-status gate, and all three attach procs pass
	// `allowPending: true`, so an image whose scan is still in flight is written
	// and flagged `scanPending` rather than refused. Polling first therefore made
	// an author with a 512x256 icon wait out the whole scan — up to
	// scanPollTimeout — before hearing `icon must be square-ish (aspect 2.00
	// outside 0.9–1.1)`. Asking first gets the server's own verdict in one
	// round-trip, and the CLI still vendors none of those bounds: it relays what
	// the server said. The scan is polled below, so nothing reports success while
	// a scan is pending or blocked.
	targetID := ref.AppListingID
	var shadowID string
	if live {
		shadowID, _, err = client.BeginListingRevision(ctx, ref.AppListingID)
		if err != nil {
			return err
		}
		targetID = shadowID
	}
	res, err := attachMedia(ctx, client, kind, targetID, imageID, caption)
	if err != nil {
		return attachRejectionAdvice(err, kind, file, len(data), info)
	}

	// 6. Poll the scan AFTER the attach, so success is still never reported while
	// the scan is pending or blocked.
	//
	// `scanPending` is the server's own answer and is what decides this — the
	// attach proc sets it ONLY on the still-scanning branch and omits the key
	// entirely once `ingestion == Scanned`, so absent and false both mean "the
	// server already saw a clean scan" and there is nothing to wait for.
	//
	// `status: "pending"` is the legacy `allowPending: false` shape: NOTHING was
	// written. Today's procs never return it, but if that ever changed, attaching
	// first would silently no-op and still print success — so fall back to the
	// pre-#270 order (wait out the scan, then attach) rather than lie.
	switch {
	case res.Status == attachStatusPending:
		if err := pollScan(ctx, out, client, imageID); err != nil {
			return scanFailure(out, err, kind, live, res)
		}
		if res, err = attachMedia(ctx, client, kind, targetID, imageID, caption); err != nil {
			return attachRejectionAdvice(err, kind, file, len(data), info)
		}
		if res.Status == attachStatusPending {
			return fmt.Errorf("the server did not attach the %s — it still reports the image as scanning; try again shortly", kind)
		}
	case res.ScanPending:
		if err := pollScan(ctx, out, client, imageID); err != nil {
			return scanFailure(out, err, kind, live, res)
		}
	}

	// 7. For a live listing, submit the revision for moderator re-review.
	if live {
		rev, err := client.SubmitListingRevision(ctx, shadowID, changelog)
		if err != nil {
			// The submit is the ONE step in this flow that can refuse for a
			// reason which is not a failure of the command that ran: a listing
			// below the publish floor cannot go to review YET, and clearing the
			// floor takes a second command. See reportStagedBelowFloor (#400).
			if reportStagedBelowFloor(ctx, out, client, ref.AppListingID, capitalize(string(kind)),
				func(v *appapi.ListingEditView) bool { return attachLanded(v, kind, imageID, res) }, err) {
				return nil
			}
			return err
		}
		// submitListingRevision is idempotent: a shadow that already had a pending
		// request returns THAT request rather than opening a second. The response
		// carries no fresh-vs-existing flag, so phrase it neutrally.
		fmt.Fprintln(out, ui.Success(fmt.Sprintf("%s staged on a revision — pending moderator review (%s).", capitalize(string(kind)), rev.PublishRequestID)))
		fmt.Fprintln(out, "Your live listing is unchanged until a moderator approves the revision.")
	} else {
		// No trailing ✓ here (nor on the two sibling lines below): ui.Success
		// already prefixes one, and spelling a second rendered `✓ Icon set ✓`.
		fmt.Fprintln(out, ui.Success(fmt.Sprintf("%s set", capitalize(string(kind)))))
	}

	// 8. Print the resulting floor state (best-effort — the attach already succeeded).
	printFloorAfter(ctx, out, client, ref.AppListingID)
	return nil
}

// attachStatusPending is the server's legacy `allowPending: false` attach result:
// the image was still scanning and NOTHING was written. The live listing-media
// procs all pass `allowPending: true` and never return it; see runSetMedia for
// why the CLI handles it anyway.
const attachStatusPending = "pending"

// attachMedia attaches the ingested image and returns the server's attach result
// (never nil on a nil error), which carries the `scanPending` flag the caller
// uses to decide whether the scan still has to be waited on.
func attachMedia(ctx context.Context, client *appapi.Client, kind mediaKind, listingID string, imageID int, caption string) (*appapi.AttachResult, error) {
	switch kind {
	case kindIcon:
		return client.SetIcon(ctx, listingID, imageID)
	case kindCover:
		return client.SetCover(ctx, listingID, imageID)
	default:
		return client.AddScreenshot(ctx, listingID, imageID, caption)
	}
}

// scanFailure returns the scan error unchanged — it is the ONE diagnosis the user
// gets — after printing the state the attach-before-scan order leaves behind.
//
// The line is context, never a second verdict: attaching first means a blocked or
// never-settling image can already be written to the DRAFT when the scan verdict
// arrives, which was impossible when the poll ran first. For a live listing the
// revision is simply not submitted; for a screenshot the row exists and needs an
// explicit removal, so its id is handed over.
func scanFailure(out io.Writer, err error, kind mediaKind, live bool, res *appapi.AttachResult) error {
	switch {
	case live:
		fmt.Fprintln(out, "The revision was not submitted — your live listing is unchanged.")
	case kind == kindScreenshot && res != nil && res.ID != "":
		fmt.Fprintf(out, "The screenshot was added to your draft — remove it with %s\n",
			ui.Code("civitai app listing rm-screenshot "+res.ID))
	case res != nil && res.Status != attachStatusPending:
		fmt.Fprintf(out, "The %s is attached to your draft but cannot go live — re-run this command with a different image.\n", kind)
	}
	return err
}

// reportStagedBelowFloor decides whether a FAILED revision submit is actually
// progress, and prints that outcome if so. It reports whether it handled the
// error — true means the caller must return nil, because nothing failed.
//
// 🔴 THE DISCRIMINATOR IS THE LISTING'S STATE, NOT THE SERVER'S PROSE, and that
// is the whole design. `submitListingRevision` refuses a listing below the
// publish floor with a 400 whose message names what is missing, and issue #400
// is what that cost: `set-icon` on a live app that has no cover yet attached the
// icon, polled a clean scan, and then reported the submit's 400 as the command's
// error — so the ONE thing that could not have happened yet, by design, was
// reported as the command failing. #186's framing is that CLI-authored apps are
// born below the floor, so clearing it takes two commands and the first of them
// cannot be a failure for the second not having run. The natural scripted form
// is exactly the one that broke:
//
//	civitai app listing set-icon icon.png && civitai app listing set-cover cover.png
//
// Matching the 400's TEXT would have been the cheap fix and it is the wrong one:
// a message match goes silently false the day the server rewords, and it fails
// in the REASSURING direction — a genuine rejection would start exiting 0. So
// this asks three structural questions instead:
//
//  1. Was this a REFUSAL at all? See the gate below — only a 400 can be the
//     floor talking.
//  2. Did what this command staged actually land, BY IDENTITY? The caller
//     supplies that predicate, because the identity differs per path:
//     attachLanded matches an icon/cover by IMAGE ID and a screenshot by its
//     `alsc_…` row id; reorderLanded (civitai/cli#430) matches the revision's
//     gallery against the order that was asked for. `Present()` alone is not
//     this question, because a listing being re-branded already has an icon —
//     and "a revision exists" is not it either, for the same reason.
//  3. Is the floor still unmet — i.e. can the floor even BE the reason? This
//     one IS the `Present()` predicate `status` and the floor line use.
//
// All three must hold. Any one alone is an error the user still needs: a floor
// that is already met means the refusal was about something else, and an asset
// that did not land means the 400 is all the user has.
//
// 🔴 AND THE REFUSAL MUST BE A REFUSAL — only a 400 can be the floor talking. A
// 500, a 503, a 429 or a dropped connection says nothing about the listing, so
// swallowing one would report success during an outage: the below-floor state is
// by #186's premise the NORMAL state of every CLI-authored app's first media
// command, so "any submit error" would have made a platform failure invisible on
// the busiest path this command has. `errors.Is` against the status sentinel is
// structural — `civitai.TagStatus` attaches it from the HTTP status, never from
// the body — so this costs nothing the design's refusal to match server PROSE
// was protecting.
//
// The residual that remains, stated rather than hidden: a 400 refusing the
// submit for some OTHER reason while the floor is also unmet is reported as
// progress, and its reason is not shown. It is bounded rather than silent — the
// next set-/add- command reuses this same shadow and submits it again, so once
// the floor is met that reason surfaces as a normal error. Distinguishing it
// would need the spelled guard this design exists to avoid.
//
// A read failure returns false: the CLI cannot show that the attach landed, so
// it must not claim it did.
func reportStagedBelowFloor(ctx context.Context, out io.Writer, client *appapi.Client, appListingID, subject string, landed func(*appapi.ListingEditView) bool, submitErr error) bool {
	if !errors.Is(submitErr, civitai.ErrBadRequest) {
		return false // not a refusal, so the floor cannot be what happened
	}
	view, err := client.GetMyListingForEdit(ctx, appListingID)
	if err != nil {
		return false
	}
	if floorMet(view) {
		return false // the floor is met, so it cannot be why this was refused
	}
	if !landed(view) {
		return false
	}

	// No "pending moderator review" and no request id: there is no review. The
	// revision is open, holds this change, and is deliberately unsubmitted.
	fmt.Fprintln(out, ui.Success(fmt.Sprintf("%s staged on an open revision — not submitted for review yet.", subject)))
	fmt.Fprintln(out, "Your live listing is unchanged; nothing reaches a moderator until the publish floor is met.")
	printFloorLine(out, view)
	// Name the command that clears the floor, not a placeholder — at least one
	// of the two slots is empty here, or we would have returned false above. The
	// icon is named first when BOTH are empty; the floor line above has already
	// said that two things are missing, so this is the next step, not the whole
	// remaining list.
	next := "civitai app listing set-icon <file>"
	if view.Assets.Icon.Present() {
		next = "civitai app listing set-cover <file>"
	}
	fmt.Fprintf(out, "Next: %s — it reuses this revision and submits it once the floor is met.\n", ui.Code(next))
	return true
}

// attachLanded reports whether the asset THIS command attached is present in the
// listing view — BY IDENTITY, for every kind.
//
// 🔴 "THE SLOT IS NON-EMPTY" IS NOT "MY IMAGE IS THERE", and the difference is
// the whole value of this check. A live listing being re-branded already HAS an
// icon, so a presence-only test is satisfied by the image the author is
// replacing: the CLI would report the new icon staged while looking at the old
// one, which is the same false report — a claim about what the CLI cannot see —
// that this file exists to remove. Measured: with a presence-only test, an
// attach of image 501 whose re-read still showed 999 in the slot printed
// `✓ Icon staged` and exited 0.
//
// Either the id the ATTACH echoed (`iconId`/`coverId`) or the id that was
// INGESTED counts, because the server is free to answer with either and both
// name an image this command put there. A pre-existing image matches neither.
//
// 🔴 THE ONE ASSUMPTION THIS RESTS ON IS NOW MEASURED, and it holds. The worry
// was that the platform might store a re-encoded DERIVATIVE under a NEW image
// id — this repo documents that it re-encodes icons from decoded pixels
// (#295 / #344, see app_listing_reencode_test.go) — while echoing back the id
// it was GIVEN. The re-read would then report an id this CLI has never seen,
// the check would fail CLOSED, and the whole #400 fix would silently not apply.
//
// Measured 2026-08-12 against the live platform, one `set-icon` on an approved
// listing (`sensei`), reading the wire: `ingestAssetFromDataUri` minted image
// 139517683, `setIcon` echoed `iconId` 139517683, and `getMyListingForEdit`
// then reported 139517683 in the slot. ALL THREE AGREE — the Image row is
// created at INGEST and the slot references that same row; the re-encode is a
// URL TRANSFORM (`…/width=320/…` in the asset URL), not a second row. So the
// echo and the ingested id are the same value in practice, and accepting either
// is belt-and-braces rather than the load-bearing part.
//
// What is still NOT proven is that they must always agree — one listing, one
// kind, one day. Hence both candidates stay. If this ever regresses the failure
// is the safe one: the user gets the submit error they got before #400, never a
// false success. Widening to "the slot is non-empty" would trade that for the
// opposite failure — a listing being re-branded already HAS an icon, so
// presence alone would certify the image the author is REPLACING.
func attachLanded(view *appapi.ListingEditView, kind mediaKind, imageID int, res *appapi.AttachResult) bool {
	switch kind {
	case kindIcon:
		return slotHolds(view.Assets.Icon, imageID, derefID(resIconID(res)))
	case kindCover:
		return slotHolds(view.Assets.Cover, imageID, derefID(resCoverID(res)))
	default:
		if res == nil || res.ID == "" {
			return false
		}
		for _, s := range view.Assets.Screenshots {
			if s.ID == res.ID {
				return true
			}
		}
		return false
	}
}

// derefID reads an optional echoed id; 0 means the server said nothing, which
// is not the same as it having written a zero.
func derefID(echoed *int) int {
	if echoed == nil {
		return 0
	}
	return *echoed
}

func resIconID(res *appapi.AttachResult) *int {
	if res == nil {
		return nil
	}
	return res.IconID
}

func resCoverID(res *appapi.AttachResult) *int {
	if res == nil {
		return nil
	}
	return res.CoverID
}

// slotHolds reports whether a floor slot holds any of the given images. A zero
// id is "the server did not say" and can never match.
func slotHolds(slot appapi.ListingAsset, ids ...int) bool {
	if !slot.Present() {
		return false
	}
	for _, id := range ids {
		if id > 0 && *slot.ImageID == id {
			return true
		}
	}
	return false
}

// printFloorAfter reads the listing media and prints the remaining floor gap.
// Best-effort: a read failure after a successful attach is not fatal.
func printFloorAfter(ctx context.Context, out io.Writer, client *appapi.Client, appListingID string) {
	view, err := client.GetMyListingForEdit(ctx, appListingID)
	if err != nil {
		return
	}
	printFloorLine(out, view)
}

// floorMet is THE publish-floor predicate: a listing can go to review once it
// has both an icon and a cover. It was open-coded at four sites; ALL FOUR now
// ask it here — printListingStatus, reportStagedBelowFloor, printFloorLine and
// #436's reportSubmitRefusedBelowFloor. #437 folded in three and left
// reportStagedBelowFloor's copy inline only because civitai/cli#434 was
// rewriting the lines around it at the time; #440 folded in the fourth once
// that merged.
//
// One rule, one place — and here that is load-bearing rather than tidy, because
// two of the four callers answer this ONE question with OPPOSITE outcomes on
// purpose (AGENTS.md item 30): an unmet floor is PROGRESS to
// reportStagedBelowFloor (the attach/reorder path — the staged change is what
// the user asked for, exit 0) and a real FAILURE to
// reportSubmitRefusedBelowFloor (submitting IS the job, exit non-zero). That
// asymmetry is exactly the configuration in which a later edit to "the floor
// check" changes only one site and nobody hears it. Sharing the predicate is
// what makes any future disagreement audible: change the floor here and both
// answers move together, or state the divergence at the call site as a
// deliberate one.
func floorMet(view *appapi.ListingEditView) bool {
	return view.Assets.Icon.Present() && view.Assets.Cover.Present()
}

// floorMissing names WHAT the floor is still missing, in the order the human
// rendering has always listed them (icon before cover). It is empty — never nil
// — when the floor is met, because it is marshalled straight into `--json` and
// `.floor.missing` must be an array on every listing.
//
// One rule, one place, for the same reason floorMet is: the human line and the
// `--json` verdict answer the same question and must not be able to disagree
// about which asset is absent.
func floorMissing(view *appapi.ListingEditView) []string {
	missing := []string{}
	if !view.Assets.Icon.Present() {
		missing = append(missing, "icon")
	}
	if !view.Assets.Cover.Present() {
		missing = append(missing, "cover")
	}
	return missing
}

// printFloorLine prints the remaining floor gap for an ALREADY-READ view, so a
// caller that has one does not fetch it twice.
func printFloorLine(out io.Writer, view *appapi.ListingEditView) {
	iconOK := view.Assets.Icon.Present()
	coverOK := view.Assets.Cover.Present()
	switch {
	case floorMet(view):
		fmt.Fprintln(out, "Publish floor met — icon and cover are set.")
	case !iconOK && !coverOK:
		fmt.Fprintln(out, "Still required before publishing: icon and cover.")
	case !iconOK:
		fmt.Fprintln(out, "Still required before publishing: icon.")
	case !coverOK:
		fmt.Fprintln(out, "Still required before publishing: cover.")
	}
}

// confirmLiveRevision resolves the changelog + gates the live-listing revision.
// A provided --changelog proceeds. Otherwise: a non-TTY refuses (needs an
// explicit --changelog); a TTY warns about the moderator-review cycle and, on a
// "yes", proceeds with a default changelog. --yes skips the prompt.
func confirmLiveRevision(cmd *cobra.Command, kind mediaKind, changelog string, assumeYes bool) (string, error) {
	out := cmd.OutOrStdout()
	defaultChangelog := fmt.Sprintf("Update listing %s via civitai CLI", kind)

	if changelog != "" {
		fmt.Fprintln(out, ui.Warn("This listing is live — your change opens a revision for moderator re-review."))
		return changelog, nil
	}
	if assumeYes {
		return defaultChangelog, nil
	}
	if !stdinIsTTY() {
		return "", fmt.Errorf("this listing is live — attaching media opens a moderator-reviewed revision. Pass --changelog \"...\" (or --yes) to proceed in a non-interactive shell")
	}
	fmt.Fprintln(out, ui.Warn("This listing is live — your change opens a revision for moderator re-review."))
	fmt.Fprintln(out, "The current live listing stays unchanged until the revision is approved.")
	fmt.Fprint(out, "Open a revision and submit it for review? [y/N]: ")
	r := bufio.NewReader(cmd.InOrStdin())
	line, _ := r.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return defaultChangelog, nil
	default:
		return "", fmt.Errorf("cancelled")
	}
}

// ----------------------------------------------------------------------------
// rm-screenshot / reorder
// ----------------------------------------------------------------------------

func newAppListingRmScreenshotCmd() *cobra.Command {
	var lc listingCommon
	cmd := &cobra.Command{
		Use:   "rm-screenshot <screenshotId>",
		Short: "Remove a screenshot by its id (see `app listing status`)",
		Long: `Remove a screenshot from your listing by its screenshot id (the id shown by
` + "`civitai app listing status`" + `, e.g. alsc_...).

On a listing that is already LIVE the removal lands in the open REVISION — the
ids ` + "`status`" + ` prints are that revision's — and it is NOT submitted, so your
public gallery keeps the screenshot until a moderator approves the revision.
Curating a gallery is usually several removals, so the submit stays yours to
make: run ` + "`civitai app listing submit-revision`" + ` when the revision is what you
want. On a DRAFT listing the removal applies to the listing itself.`,
		Example: `  civitai app listing rm-screenshot alsc_01H8XYZ
  civitai app listing rm-screenshot alsc_01H8XYZ --slug my-app`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newListingClient()
			if err != nil {
				return err
			}
			// The ref is resolved to validate the listing exists (clear error) AND
			// to learn its lifecycle status — civitai/cli#436: the status was
			// resolved here and DISCARDED, so the live arm below could not exist.
			ref, err := resolveListingRefFromFlags(cmd, client, lc)
			if err != nil {
				return err
			}
			ctx := cmdCtx(cmd)
			if err := client.RemoveScreenshot(ctx, args[0]); err != nil {
				return err
			}
			reportScreenshotRemoved(cmd.OutOrStdout(), ref.Status == "approved")
			return nil
		},
	}
	lc.bind(cmd)
	return cmd
}

// reportScreenshotRemoved prints the outcome of a removal, and the LIVE arm is
// the whole of civitai/cli#436.
//
// 🔴 `✓ Screenshot removed` IS FALSE OF A LIVE LISTING, and it exited 0 saying
// it. `removeScreenshot` sends only a `screenshotId`, so the server resolves the
// owning listing from the row itself — and on an approved listing the `apls_…`
// ids `app listing status` prints are the SHADOW revision's rows (`status`
// renders getMyListingForEdit, which idempotently opens that shadow). So the
// removal really happens, in the revision, and the public gallery keeps serving
// the screenshot. Measured live in #436 against `model-benchmarking`:
// `✓ Screenshot removed`, exit 0, `app listing status` then reporting 3
// screenshots while `GET /api/v1/apps/model-benchmarking` still returned 4.
//
// The removal is deliberately NOT submitted here — see
// newAppListingSubmitRevisionCmd for why the review cycle stays an explicit act
// — so what this owes the user is the fact that nothing is published yet and the
// name of the command that publishes it. A bare success line that distinguishes
// nothing is exactly what the issue reports.
func reportScreenshotRemoved(out io.Writer, live bool) {
	if !live {
		fmt.Fprintln(out, ui.Success("Screenshot removed"))
		return
	}
	fmt.Fprintln(out, ui.Success("Screenshot removal staged on an open revision — not submitted for review yet."))
	fmt.Fprintln(out, "Your live listing still shows this screenshot; the removal reaches the")
	fmt.Fprintln(out, "public gallery only after a moderator approves the revision.")
	// 🔴 THE ALTERNATIVE TO PUBLISHING NOW IS STAGING MORE, NOT REMOVING MORE
	// (civitai/cli#443). This sentence used to end "remove the rest first if you
	// are still curating" — an instruction to DELETE MORE grafted onto the
	// instruction to publish, read by someone who had just asked how to publish.
	// It is also wrong about what curating involves: an `add-screenshot` or a
	// `reorder` stages into this same revision, so removal is one kind of
	// pending change, not the kind. The whole line is pinned by
	// rmLivePublishLine — naming the command inside a sentence is not the same
	// claim as the sentence meaning what it should.
	fmt.Fprintf(out, "Publish it: %s — or stage more changes first if you are still curating.\n",
		ui.Code("civitai app listing submit-revision"))
}

// newAppListingSubmitRevisionCmd is the standalone submit — civitai/cli#436's
// root cause, and the repair path for anyone who already hit it.
//
// 🔴 BEFORE THIS COMMAND, `submitListingRevision` HAD EXACTLY ONE CALL SITE:
// step 7 of the attach flow. A user who staged a change any other way — a
// removal, and after civitai/cli#434 nothing else — had staged something NO
// command in this CLI could publish; #436's reporter got it to review by hand,
// with a raw tRPC call. Three paths now need to submit, so the submit is its own
// verb rather than a side effect of whichever command happened to run last.
//
// 🔴 IT DOES NOT AUTO-SUBMIT FROM `rm-screenshot`, AND THAT IS THE DECISION.
// Curation is N removals, not one atomic operation, so auto-submitting on the
// first would open a moderator review cycle in the middle of an edit. Whether a
// shadow stays EDITABLE after submission is NOT measured — this repo has no way
// to measure it — so auto-submit would be built on an assumption nobody has
// checked, while fixing the false CLAIM is what actually closes #436.
//
// The `created` flag is the one structural guard available. `beginListingRevision`
// is idempotent: it REUSES an open shadow (created=false) and only mints one when
// none exists (created=true). So created=true means the user staged nothing —
// submitting would send an empty revision to a moderator — and this refuses
// instead. The residual, stated rather than hidden: a shadow opened by an
// earlier `app listing status` read (which opens one as a side effect) and never
// written to reports created=false, so an empty revision CAN still be submitted
// that way. Narrowing that would need a shadow-vs-live diff this CLI cannot
// compute; the server is free to refuse it, and this claims nothing it cannot
// see.
func newAppListingSubmitRevisionCmd() *cobra.Command {
	var lc listingCommon
	var changelog string
	cmd := &cobra.Command{
		Use:   "submit-revision",
		Short: "Submit your live listing's open revision for moderator review",
		Long: `Submit the REVISION your live listing already has open — the one that
` + "`civitai app listing status`" + ` reports and that ` + "`rm-screenshot`" + ` writes into — for
moderator re-review. Pass --changelog to describe the change.

Only an approved (LIVE) listing has a revision: a draft or pending listing is
edited directly, so there is nothing to submit and this refuses. It refuses too
when there is no open revision to submit, rather than sending a moderator an
empty one.

The revision is submitted as it stands, so stage everything you want in one
review cycle first. Submitting is idempotent — a revision already awaiting
review returns that same request rather than opening a second.`,
		Example: `  civitai app listing submit-revision
  civitai app listing submit-revision --changelog "Dropped the outdated grid shot"
  civitai app listing submit-revision --slug my-app`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSubmitRevision(cmd, lc, changelog)
		},
	}
	lc.bind(cmd)
	cmd.Flags().StringVar(&changelog, "changelog", "", "changelog for the moderator review")
	return cmd
}

func runSubmitRevision(cmd *cobra.Command, lc listingCommon, changelog string) error {
	out := cmd.OutOrStdout()
	client, err := newListingClient()
	if err != nil {
		return err
	}
	ref, err := resolveListingRefFromFlags(cmd, client, lc)
	if err != nil {
		return err
	}
	ctx := cmdCtx(cmd)

	switch ref.Status {
	case "rejected":
		return fmt.Errorf("this listing was rejected — resubmit the app before editing its media")
	case "removed":
		return fmt.Errorf("this listing was removed by a moderator and can no longer be edited")
	case "approved":
	default:
		// A draft/pending listing IS the edit target: every listing command
		// writes it directly, so there is no revision and nothing was staged
		// behind a review gate.
		return fmt.Errorf("this listing is not live (status %s) — only an approved listing has revisions, and your changes already apply to it; check them with `civitai app listing status`", ref.Status)
	}

	shadowID, created, err := client.BeginListingRevision(ctx, ref.AppListingID)
	if err != nil {
		return err
	}
	if created {
		return fmt.Errorf("there is no open revision to submit — stage a change first (for example `civitai app listing rm-screenshot <id>`), then re-run this command")
	}
	if changelog == "" {
		changelog = "Update listing via civitai CLI"
	}

	rev, err := client.SubmitListingRevision(ctx, shadowID, changelog)
	if err != nil {
		// 🔴 THIS IS NOT reportStagedBelowFloor, AND THE DIFFERENCE IS THE EXIT
		// CODE. That helper (#400) exists because a below-floor refusal is not a
		// failure of a command whose job was to ATTACH — the image landed, so it
		// exits 0. Here the submit IS the job the user asked for: exiting 0
		// having submitted nothing would be civitai/cli#436's own false-success
		// class, reproduced in the command that exists to fix it. So the error
		// stands and the exit code with it; what is added is the diagnosis,
		// printed as CONTEXT the way scanFailure prints it — never folded into
		// the error text, which listingError's change arm owns alone.
		reportSubmitRefusedBelowFloor(ctx, out, client, ref.AppListingID, err)
		return err
	}
	fmt.Fprintln(out, ui.Success(fmt.Sprintf("Revision submitted — pending moderator review (%s).", rev.PublishRequestID)))
	fmt.Fprintln(out, "Your live listing is unchanged until a moderator approves the revision.")
	return nil
}

// reportSubmitRefusedBelowFloor prints WHY a revision submit was refused when
// the listing's own state says the publish floor is the reason.
//
// It asks the same structural questions reportStagedBelowFloor does — was it a
// REFUSAL (only a 400 can be the floor talking; a 500/503 says nothing about the
// listing), and is the floor really unmet — and deliberately not the third one:
// there is no asset this command attached, because submitting is all it did.
// It prints nothing when it cannot answer, so a read failure or an unrelated
// 400 leaves the server's own sentence as the whole story.
func reportSubmitRefusedBelowFloor(ctx context.Context, out io.Writer, client *appapi.Client, appListingID string, submitErr error) {
	if !errors.Is(submitErr, civitai.ErrBadRequest) {
		return
	}
	view, err := client.GetMyListingForEdit(ctx, appListingID)
	if err != nil || floorMet(view) {
		return
	}
	fmt.Fprintln(out, "Nothing was submitted — this listing is below the publish floor.")
	printFloorLine(out, view)
	next := "civitai app listing set-icon <file>"
	if view.Assets.Icon.Present() {
		next = "civitai app listing set-cover <file>"
	}
	fmt.Fprintf(out, "Next: %s — then re-run this command.\n", ui.Code(next))
}

func newAppListingReorderCmd() *cobra.Command {
	var lc listingCommon
	var changelog string
	cmd := &cobra.Command{
		Use:   "reorder <screenshotId...>",
		Short: "Reorder screenshots (pass ALL current screenshot ids in the new order)",
		Long: `Reorder your listing's screenshots. Pass EXACTLY the current set of screenshot
ids (from ` + "`civitai app listing status`" + `) in the desired order — a partial or
unknown set is rejected.

Ordering is positional: the first id becomes the first screenshot in the
gallery. There is no "move one" form — read the current order out of
` + "`civitai app listing status`" + ` and pass the whole list back.

On a listing that is already LIVE this opens a REVISION and submits it for
moderator re-review instead of reordering the live gallery — pass --changelog
to describe the change — but a live listing still below the publish floor
stages WITHOUT submitting, and exits 0. A DRAFT listing is reordered directly.`,
		Example: `  civitai app listing reorder alsc_02 alsc_01 alsc_03
  civitai app listing reorder alsc_02 alsc_01 --slug my-app`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReorder(cmd, args, lc, changelog)
		},
	}
	lc.bind(cmd)
	cmd.Flags().StringVar(&changelog, "changelog", "", "changelog for the moderator review (used only when the listing is already live)")
	return cmd
}

// runReorder rewrites the screenshot order, branching on the listing status for
// the live-listing shadow-revision path — the same shape runSetMedia uses.
//
// 🔴 THE ID SPACE IS THE WHOLE BUG (civitai/cli#430), AND IT IS INVISIBLE FROM
// THE CALL SITE. `reorderScreenshots` takes a `listingId` AND the screenshot
// ids, and the server checks the two for set-equality. `app listing status`
// renders getMyListingForEdit, which for an APPROVED parent resolves media
// through an idempotently-opened SHADOW revision — so the `apls_…` ids a user
// can SEE belong to the shadow, while this command addressed the mutation to the
// PARENT. Every reorder of a live listing was therefore refused with
// `orderedIds must be exactly the listing's current screenshot ids`, naming the
// ids the CLI itself had just printed, and the remedy that error suggests
// (`app listing status`) printed those same ids again. Measured live in #430:
// same ids, same order, seconds apart — parent id 400, shadow id 200.
//
// The sibling commands that WORK are the ones with no second id space to get
// wrong: `rm-screenshot` sends only a `screenshotId`, so the server resolves the
// owning listing from the row itself, and set-icon/set-cover/add-screenshot
// already branch on `live` exactly as this now does.
//
// 🔴 AND THE REVISION IS SUBMITTED, not merely opened. `submitListingRevision`
// has no standalone subcommand: a change staged in a shadow that nothing submits
// is a change the public gallery NEVER sees, with a ✓ in front of it — which is
// the trap `rm-screenshot` is still in (#430's follow-up comment measured it:
// `✓ Screenshot removed`, exit 0, and `GET /api/v1/apps/<slug>` still serving
// the screenshot). Opening a revision without submitting it would have traded
// this bug's loud 400 for a silent false success.
//
// TWO CHEAPER-LOOKING ROUTES TO THE SHADOW ID ARE REJECTED, both of which read
// as obvious once you are here. (a) Decoding `editTargetId` off the
// getMyListingForApp reply — #430's own suggested fix; see the ListingRef doc
// comment for why not, and note it would save a round-trip rather than enable
// anything. (b) Reusing `ListingEditView.ShadowID`, which IS already decoded:
// that would build this command on `getMyListingForEdit`, a route classified
// `listingOpRead` while its own doc records that it OPENS a revision —
// civitai/cli#389. The classification is now MEASURED correct on the arm #389
// doubted (every refusal that proc can raise precedes the INSERT, so its 400
// wording tells the truth — see the `listingOp` doc in
// `internal/appapi/listing.go`), and the reason to keep this command off it
// never rested on that doubt: the revision that route opens is a SIDE EFFECT of
// a look-up, so staging a reorder on it would mint the shadow through a route
// that tells a rejected caller nothing was changed. This command opens it
// through `beginListingRevision` instead, which is classified `listingOpChange`
// because opening it is what the caller asked for.
//
// There is deliberately NO interactive confirmation here, unlike runSetMedia's
// step 3. That gate refuses in a non-TTY without --changelog/--yes, and adding
// it would mean the exact command #430 reports as broken still does not work
// after the fix — it would fail differently. The user named the ids and the
// order; what they get instead is the warning line below, before anything is
// written, and an outcome line that never claims the live gallery moved.
func runReorder(cmd *cobra.Command, orderedIDs []string, lc listingCommon, changelog string) error {
	out := cmd.OutOrStdout()
	client, err := newListingClient()
	if err != nil {
		return err
	}
	ref, err := resolveListingRefFromFlags(cmd, client, lc)
	if err != nil {
		return err
	}
	ctx := cmdCtx(cmd)
	live := ref.Status == "approved"

	// Draft/pending listings have no shadow, so the parent IS the edit target and
	// opening a revision would be both wrong and an extra round-trip.
	targetID := ref.AppListingID
	var shadowID string
	if live {
		fmt.Fprintln(out, ui.Warn("This listing is live — reordering opens a revision for moderator re-review."))
		if changelog == "" {
			changelog = "Reorder listing screenshots via civitai CLI"
		}
		shadowID, _, err = client.BeginListingRevision(ctx, ref.AppListingID)
		if err != nil {
			return err
		}
		// 🔴 THIS ONE ASSIGNMENT IS THE FIX. `shadowID` stays separate from
		// `targetID` — as it does in runSetMedia — so that reverting the retarget
		// leaves the submit addressed at the revision and changes exactly one
		// thing: which listing the reorder is validated against.
		targetID = shadowID
	}
	if err := client.ReorderScreenshots(ctx, targetID, orderedIDs); err != nil {
		return err
	}
	if !live {
		fmt.Fprintln(out, ui.Success(fmt.Sprintf("Reordered %d screenshots", len(orderedIDs))))
		return nil
	}

	rev, err := client.SubmitListingRevision(ctx, shadowID, changelog)
	if err != nil {
		// Same refusal-that-is-not-a-failure as the attach path (#400): a listing
		// below the publish floor cannot go to review yet, and the reorder did
		// land in the revision.
		if reportStagedBelowFloor(ctx, out, client, ref.AppListingID, "New screenshot order",
			func(v *appapi.ListingEditView) bool { return reorderLanded(v, orderedIDs) }, err) {
			return nil
		}
		return err
	}
	fmt.Fprintln(out, ui.Success(fmt.Sprintf("New screenshot order staged on a revision — pending moderator review (%s).", rev.PublishRequestID)))
	fmt.Fprintln(out, "Your live listing is unchanged until a moderator approves the revision.")
	return nil
}

// reorderLanded reports whether the revision really holds the order this command
// asked for — the reorder path's answer to attachLanded's identity question.
//
// "A revision exists" is not that question, and neither is "it has three
// screenshots": a shadow whose gallery is untouched satisfies both while nothing
// this command asked for is in it. The comparison is by ROW ID and POSITION,
// against the list the user typed, read back out of the same view the floor
// check uses. Sorted by the server's own `order` field rather than trusting the
// array's arrangement, because the ordering is the very property under test.
func reorderLanded(view *appapi.ListingEditView, want []string) bool {
	if len(view.Assets.Screenshots) != len(want) {
		return false
	}
	shots := append([]appapi.ListingScreenshot(nil), view.Assets.Screenshots...)
	sort.SliceStable(shots, func(i, j int) bool { return shots[i].Order < shots[j].Order })
	for i, s := range shots {
		if s.ID != want[i] {
			return false
		}
	}
	return true
}

func resolveListingRefFromFlags(cmd *cobra.Command, client *appapi.Client, lc listingCommon) (*appapi.ListingRef, error) {
	slug, err := resolveListingSlug(lc)
	if err != nil {
		return nil, err
	}
	return resolveListing(cmdCtx(cmd), client, slug)
}

func cmdCtx(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

// ----------------------------------------------------------------------------
// local validation + scan poll helpers
// ----------------------------------------------------------------------------

// loadAndValidateImage reads the file, validates its type is png/jpeg/webp and
// its size is within the per-kind cap, and returns the bytes + header info — all
// BEFORE any network call.
//
// Every refusal about the VALUE the user passed is asUsageError-tagged (exit 2),
// mirroring resolveLocalImage in generate_image.go, which does the same job for
// `generate --image`. The set of tagged refusals is published — it is rendered
// into the README/`--help` exit-code contract from imageUsageRefusals in
// exitcodes_doc.go, and TestImageUsageRefusalLedger requires the two to agree.
//
// The raw os.Stat/os.ReadFile failures are deliberately NOT tagged: they wrap an
// opaque filesystem cause (permissions, I/O), which is not a mistake about the
// invocation and must not be reported as one.
//
// 🔴 Be precise about what leaving them untagged costs, because two earlier
// versions of this comment were not. The first said tagging would misreport an
// I/O failure "as a user mistake", implying the status quo reported it as
// something better; it did not — untagged, a permission-denied read exited **5,
// the network code**, because syscall.Errno has both Timeout() and Temporary()
// and so satisfies net.Error, which cmd/civitai's isNetworkErr errors.As-matched
// straight through the *fs.PathError. Measured then:
// `civitai app listing set-icon <mode-000 png>` exited 5.
// That is fixed (issue #241) where the second version of this comment said it
// belonged — in isNetworkErr, which corrects every filesystem error in the CLI
// at once rather than one call site. An unreadable image now exits **1**, the
// generic code, and the published contract says so. Leaving these untagged is
// still the right call: exit 2 means the user got the INVOCATION wrong, and a
// chmod problem is not that.
func loadAndValidateImage(kind mediaKind, file string) ([]byte, appapi.ImageInfo, error) {
	fi, err := os.Stat(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, appapi.ImageInfo{}, asUsageError(fmt.Errorf("no such file: %s", file))
		}
		return nil, appapi.ImageInfo{}, err
	}
	if fi.IsDir() {
		return nil, appapi.ImageInfo{}, asUsageError(fmt.Errorf("%s is a directory, not an image file", file))
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, appapi.ImageInfo{}, err
	}
	if len(data) == 0 {
		return nil, appapi.ImageInfo{}, asUsageError(fmt.Errorf("%s is empty", file))
	}
	limit := kindByteCap(kind)
	if len(data) > limit {
		return nil, appapi.ImageInfo{}, asUsageError(fmt.Errorf("%s is %s — larger than the %s max for a %s; provide a smaller image", file, humanBytes(int64(len(data))), humanBytes(int64(limit)), kind))
	}
	info, err := appapi.DecodeImageInfo(data)
	if err != nil {
		return nil, appapi.ImageInfo{}, asUsageError(fmt.Errorf("%s: %w", file, err))
	}
	return data, info, nil
}

// attachRejectionAdvice annotates a server BAD_REQUEST from a listing-media
// ingest or attach with what the CLI measured about the file it sent. It is a
// pass-through for every other error, and for nil.
//
// 🔴 IT VENDORS NO BOUND (AGENTS.md item 25). It adds no threshold, no dimension
// check and no constant the server owns: every NUMBER it prints was measured
// from the author's own file, and the server's sentence is still relayed
// verbatim ahead of it. The only claim it makes about the platform is the
// MECHANISM — that an icon is re-encoded before it is measured — which is what
// makes the server's number legible rather than a second bound to keep in sync.
//
// Why it exists (issue #344). `appListings.setIcon` refused a 38,201-byte
// 1024x1024 JPEG with "icon is 1202233 bytes (max 1048576)": a byte count
// matching nothing the author can see — not the file (37.3 KiB), not the cap the
// CLI enforces (2 MiB), not even the cap quoted in the message (1 MiB) as
// applied to anything they hold. There was no path from that number to "shrink
// the image", so the fix is to name the units, not to guess the formula.
//
// What the 2026-08-10 credentialed dogfood run established, and what it did NOT.
// Two rejections at the SAME <=1024px re-encode ceiling reported 1,202,233 and
// 2,201,537 bytes — 1.15 and 2.10 bytes per pixel. A RAW decoded buffer is a
// CONSTANT 3 or 4 bytes/px, so a content-dependent ratio rules raw decoding out:
// the quantity is a compressed re-encode, matching item 25's reading of
// listing-meta.service.ts. That is the EFFECT, measured. The encoder's exact
// settings are NOT knowable from this repo, and a 512x512 source (79.2 KiB) that
// passed is one point, not a bound — which is exactly why this names the LEVER
// (pixel dimensions) instead of predicting the number, and why no local gate was
// added. #286 recorded this reachability as unmeasured; it is now measured.
func attachRejectionAdvice(err error, kind mediaKind, file string, srcBytes int, info appapi.ImageInfo) error {
	if err == nil || !errors.Is(err, civitai.ErrBadRequest) {
		return err
	}
	// Every value below comes from the source file, so this line is true whichever
	// bound the server applied — geometry, aspect, MIME or size.
	sent := fmt.Sprintf("what was sent: %s — %s on disk, %d×%d px, %s",
		file, humanBytes(int64(srcBytes)), info.Width, info.Height, info.MimeType)
	if kind != kindIcon {
		// Cover and screenshot ride the full-res path, where the CLI sends
		// sizeBytes: len(data) — the server measures the same bytes the CLI does, so
		// the re-encode paragraph would be false here.
		return fmt.Errorf("%w\n  %s\n  see \"Listing media requirements\" in the README for the platform's bounds", err, sent)
	}
	return fmt.Errorf("%w\n  %s\n%s", err, sent, strings.Join([]string{
		"  an icon is re-encoded server-side to PNG (downscaled to at most 1024px on",
		"  the longer side) and the platform validates THAT image, so a byte count",
		"  above is the size of the server's PNG and not of your file",
		"  the lever is smaller PIXEL dimensions, not heavier compression — see",
		"  \"Listing media requirements\" in the README for the platform's bounds",
	}, "\n"))
}

func kindByteCap(kind mediaKind) int {
	switch kind {
	case kindIcon:
		return maxIconBytes
	case kindCover:
		return maxCoverBytes
	default:
		return maxScreenshotBytes
	}
}

// pollScan polls the image scan until Scanned, erroring on Blocked or timeout.
//
// It runs AFTER the attach (see runSetMedia), so its messages are deliberately
// position-neutral: they say what is wrong with the IMAGE, not whether it reached
// the listing. The caller owns the "what that leaves behind" line.
func pollScan(ctx context.Context, out io.Writer, client *appapi.Client, imageID int) error {
	deadline := time.Now().Add(scanPollTimeout)
	start := time.Now()
	for {
		statuses, err := client.GetAssetScanStatuses(ctx, []int{imageID})
		if err != nil {
			return err
		}
		st := "pending"
		for _, s := range statuses {
			if s.ImageID == imageID {
				st = s.Status
			}
		}
		switch st {
		case "scanned":
			return nil
		case "blocked":
			return fmt.Errorf("the image was blocked by the content scan — a blocked image can never go live; use a different image")
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for the image scan (still %s) — check `civitai app listing status` shortly", scanPollTimeout, st)
		}
		fmt.Fprintf(out, "  scanning image… (%ds)\n", int(time.Since(start).Seconds()))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(scanPollInterval):
		}
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
