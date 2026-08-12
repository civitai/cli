package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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
// below-floor listing deliberately does not get. It is ONE SENTENCE because
// TestListingHelpStaysWithinTheBudget caps each Long at 1400 chars and
// add-screenshot had ~100 to spare — the full explanation belongs in the README,
// which is where that budget exists to send it.
const liveRevisionHelp = `On a listing that is already LIVE this opens a REVISION for moderator re-review
instead of changing the live listing — pass --changelog to describe the change,
-y to skip the confirmation. On a DRAFT listing it attaches directly. Below the
publish floor it stages WITHOUT submitting, and exits 0.`

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
  civitai app listing set-icon ./assets/icon.png
  civitai app listing set-cover ./assets/cover.png
  civitai app listing add-screenshot ./shot.png --caption "Grid view"
  civitai app listing rm-screenshot alsc_01H...
  civitai app listing reorder alsc_02 alsc_01 alsc_03`,
	}
	cmd.AddCommand(newAppListingStatusCmd())
	cmd.AddCommand(newAppListingSetIconCmd())
	cmd.AddCommand(newAppListingSetCoverCmd())
	cmd.AddCommand(newAppListingAddScreenshotCmd())
	cmd.AddCommand(newAppListingRmScreenshotCmd())
	cmd.AddCommand(newAppListingReorderCmd())
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

// resolveListing resolves slug -> appBlockId (via the submission row) -> listing
// ref.
//
// 🔴 A submission with NO appBlockId is a FIRST-version app still pending review. It
// still has a store listing: `civitai app submit` mints one as a pre-approval DRAFT
// so its media is settable while pending. That draft has `appBlockId = NULL` and is
// resolvable ONLY BY SLUG, so we must still call through — short-circuiting to a
// "no listing" error here would make the pending-media flow unreachable from the CLI
// even though the server supports it. The slug is passed on BOTH paths (harmless when
// the appBlockId resolves; load-bearing when it is absent).
func resolveListing(ctx context.Context, client *appapi.Client, slug string) (*appapi.ListingRef, error) {
	sub, err := client.GetSubmission(ctx, "", slug)
	if err != nil {
		return nil, err
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
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show attached media and what's missing vs the publish floor",
		Long: `Show your store listing's attached media (icon, cover, screenshots) and what
is still required before it can publish (an icon and a cover are mandatory).

Your store listing exists as a DRAFT from the moment you run
` + "`civitai app submit`" + `, so this works while your app is still pending review.

Note: on a LIVE (approved) listing this opens an in-progress revision draft and
reports ITS media (idempotent — it reuses any existing draft, and nothing is
submitted for moderator review until you run a set-/add- command and confirm).`,
		Example: `  civitai app listing status
  civitai app listing status --slug my-app
  civitai app listing status --dir ./my-app`,
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
			printListingStatus(cmd.OutOrStdout(), slug, ref, view)
			return nil
		},
	}
	lc.bind(cmd)
	return cmd
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
	if iconOK && coverOK {
		fmt.Fprintln(w, ui.Success("Publish floor met — icon and cover are set."))
	} else {
		missing := []string{}
		if !iconOK {
			missing = append(missing, "icon")
		}
		if !coverOK {
			missing = append(missing, "cover")
		}
		fmt.Fprintln(w, ui.Warn(fmt.Sprintf("Not publishable yet — missing %s.", strings.Join(missing, " and "))))
		if !iconOK {
			fmt.Fprintf(w, "  Add one:  %s\n", ui.Code("civitai app listing set-icon <file>"))
		}
		if !coverOK {
			fmt.Fprintf(w, "  Add one:  %s\n", ui.Code("civitai app listing set-cover <file>"))
		}
	}
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

` + liveRevisionHelp + ``,
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
			if reportStagedBelowFloor(ctx, out, client, ref.AppListingID, kind, imageID, res, err) {
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
// this re-reads the listing and asks two structural questions, on the same
// `Present()` predicate `status` and the floor line already use:
//
//  1. Did the asset this command attached actually land?
//  2. Is the floor still unmet — i.e. can the floor even BE the reason?
//
// Both must hold. Either alone is an error the user still needs: a floor that is
// already met means the refusal was about something else, and an asset that did
// not land means the 400 is all the user has.
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
func reportStagedBelowFloor(ctx context.Context, out io.Writer, client *appapi.Client, appListingID string, kind mediaKind, imageID int, res *appapi.AttachResult, submitErr error) bool {
	if !errors.Is(submitErr, civitai.ErrBadRequest) {
		return false // not a refusal, so the floor cannot be what happened
	}
	view, err := client.GetMyListingForEdit(ctx, appListingID)
	if err != nil {
		return false
	}
	if view.Assets.Icon.Present() && view.Assets.Cover.Present() {
		return false // the floor is met, so it cannot be why this was refused
	}
	if !attachLanded(view, kind, imageID, res) {
		return false
	}

	// No "pending moderator review" and no request id: there is no review. The
	// revision is open, holds this asset, and is deliberately unsubmitted.
	fmt.Fprintln(out, ui.Success(fmt.Sprintf("%s staged on an open revision — not submitted for review yet.", capitalize(string(kind)))))
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
// The id compared against is the one the ATTACH echoed (`iconId`/`coverId`),
// falling back to the ingested image id when the server omitted it. The echo is
// preferred deliberately: it is the server's own statement of what it wrote, so
// it stays correct if the platform ever stores a re-encoded derivative under a
// new id, where the ingested id would not match and this would fail CLOSED —
// the user would simply get the submit error, which is what they get today.
func attachLanded(view *appapi.ListingEditView, kind mediaKind, imageID int, res *appapi.AttachResult) bool {
	switch kind {
	case kindIcon:
		return slotHolds(view.Assets.Icon, attachedID(imageID, resIconID(res)))
	case kindCover:
		return slotHolds(view.Assets.Cover, attachedID(imageID, resCoverID(res)))
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

// attachedID picks the id to prove landed: what the attach echoed, else what was
// ingested. A zero echo means the server said nothing, not that it wrote a zero.
func attachedID(imageID int, echoed *int) int {
	if echoed != nil && *echoed > 0 {
		return *echoed
	}
	return imageID
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

// slotHolds reports whether a floor slot holds exactly the given image.
func slotHolds(slot appapi.ListingAsset, imageID int) bool {
	return slot.Present() && imageID > 0 && *slot.ImageID == imageID
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

// printFloorLine prints the remaining floor gap for an ALREADY-READ view, so a
// caller that has one does not fetch it twice.
func printFloorLine(out io.Writer, view *appapi.ListingEditView) {
	iconOK := view.Assets.Icon.Present()
	coverOK := view.Assets.Cover.Present()
	switch {
	case iconOK && coverOK:
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
` + "`civitai app listing status`" + `, e.g. alsc_...). Note: for a LIVE listing,
direct screenshot edits are only possible while a revision is open.`,
		Example: `  civitai app listing rm-screenshot alsc_01H8XYZ
  civitai app listing rm-screenshot alsc_01H8XYZ --slug my-app`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newListingClient()
			if err != nil {
				return err
			}
			// Validate the listing exists (clear error), then remove.
			if _, err := resolveListingRefFromFlags(cmd, client, lc); err != nil {
				return err
			}
			ctx := cmdCtx(cmd)
			if err := client.RemoveScreenshot(ctx, args[0]); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), ui.Success("Screenshot removed"))
			return nil
		},
	}
	lc.bind(cmd)
	return cmd
}

func newAppListingReorderCmd() *cobra.Command {
	var lc listingCommon
	cmd := &cobra.Command{
		Use:   "reorder <screenshotId...>",
		Short: "Reorder screenshots (pass ALL current screenshot ids in the new order)",
		Long: `Reorder your listing's screenshots. Pass EXACTLY the current set of screenshot
ids (from ` + "`civitai app listing status`" + `) in the desired order — a partial or
unknown set is rejected.

Ordering is positional: the first id becomes the first screenshot in the
gallery. There is no "move one" form — read the current order out of
` + "`civitai app listing status`" + ` and pass the whole list back.`,
		Example: `  civitai app listing reorder alsc_02 alsc_01 alsc_03
  civitai app listing reorder alsc_02 alsc_01 --slug my-app`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newListingClient()
			if err != nil {
				return err
			}
			ref, err := resolveListingRefFromFlags(cmd, client, lc)
			if err != nil {
				return err
			}
			ctx := cmdCtx(cmd)
			if err := client.ReorderScreenshots(ctx, ref.AppListingID, args); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), ui.Success(fmt.Sprintf("Reordered %d screenshots", len(args))))
			return nil
		},
	}
	lc.bind(cmd)
	return cmd
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
