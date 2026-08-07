package cmd

import (
	"bufio"
	"context"
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

// Per-kind source-file byte caps, mirroring the server's listing-asset caps:
//   - icon uses the inline data-URI path (server decoded cap 2 MiB).
//   - cover / screenshot use the mint+persist full-res path (4 MiB / 2 MiB).
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
are optional. These commands ingest a local image, wait for the content scan, and
attach it to your listing — the same pipeline the web submit form uses.

For a listing that is already LIVE (approved), attaching media opens a REVISION
that goes back to moderator review (the live listing is untouched until the
revision is approved); pass --changelog to describe the change.

The app is resolved from block.manifest.json in the current directory (or pass
--slug). Your store listing is created as a DRAFT when you run ` + "`civitai app submit`" + `,
so you can set its media WHILE your app is pending review — the media you attach
carries forward when a moderator approves it. Set it early to clear the publish
floor before you go live.`,
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

// resolveListingSlug resolves the app slug from --slug or the manifest.
func resolveListingSlug(lc listingCommon) (string, error) {
	if lc.slug != "" {
		return lc.slug, nil
	}
	m, err := manifest.Load(lc.dir)
	if err != nil {
		return "", fmt.Errorf("could not resolve the app — run this from your app directory (with block.manifest.json) or pass --slug: %w", err)
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

Your store listing exists as a DRAFT from the moment you run ` + "`civitai app submit`" + `,
so this works while your app is still pending review.

Note: on a LIVE (approved) listing this opens an in-progress revision draft and
reports ITS media (idempotent — it reuses any existing draft, and nothing is
submitted for moderator review until you run a set-/add- command and confirm).`,
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
		Short: "Set the listing icon (a square-ish image)",
		Args:  cobra.ExactArgs(1),
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
		Short: "Set the listing cover (a landscape hero image)",
		Args:  cobra.ExactArgs(1),
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
		Args:  cobra.ExactArgs(1),
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

// runSetMedia is the shared resolve -> validate -> ingest -> scan -> attach flow
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
	fmt.Fprintf(out, "Uploading %s (%s)…\n", kind, humanBytes(int64(len(data))))
	var imageID int
	if kind == kindIcon {
		imageID, err = client.IngestAssetFromDataURI(ctx, data, info.MimeType)
	} else {
		imageID, err = client.IngestAssetFullRes(ctx, data, info)
	}
	if err != nil {
		return err
	}

	// 5. Poll the scan until Scanned (or Blocked / timeout).
	if err := pollScan(ctx, out, client, imageID); err != nil {
		return err
	}

	// 6. Attach — direct for draft/pending, via a shadow revision when live.
	targetID := ref.AppListingID
	var shadowID string
	if live {
		shadowID, _, err = client.BeginListingRevision(ctx, ref.AppListingID)
		if err != nil {
			return err
		}
		targetID = shadowID
	}
	if err := attachMedia(ctx, client, kind, targetID, imageID, caption); err != nil {
		return err
	}

	// 7. For a live listing, submit the revision for moderator re-review.
	if live {
		rev, err := client.SubmitListingRevision(ctx, shadowID, changelog)
		if err != nil {
			return err
		}
		// submitListingRevision is idempotent: a shadow that already had a pending
		// request returns THAT request rather than opening a second. The response
		// carries no fresh-vs-existing flag, so phrase it neutrally.
		fmt.Fprintln(out, ui.Success(fmt.Sprintf("%s staged on a revision — pending moderator review (%s).", capitalize(string(kind)), rev.PublishRequestID)))
		fmt.Fprintln(out, "Your live listing is unchanged until a moderator approves the revision.")
	} else {
		fmt.Fprintln(out, ui.Success(fmt.Sprintf("%s set ✓", capitalize(string(kind)))))
	}

	// 8. Print the resulting floor state (best-effort — the attach already succeeded).
	printFloorAfter(ctx, out, client, ref.AppListingID)
	return nil
}

func attachMedia(ctx context.Context, client *appapi.Client, kind mediaKind, listingID string, imageID int, caption string) error {
	switch kind {
	case kindIcon:
		_, err := client.SetIcon(ctx, listingID, imageID)
		return err
	case kindCover:
		_, err := client.SetCover(ctx, listingID, imageID)
		return err
	default:
		_, err := client.AddScreenshot(ctx, listingID, imageID, caption)
		return err
	}
}

// printFloorAfter reads the listing media and prints the remaining floor gap.
// Best-effort: a read failure after a successful attach is not fatal.
func printFloorAfter(ctx context.Context, out io.Writer, client *appapi.Client, appListingID string) {
	view, err := client.GetMyListingForEdit(ctx, appListingID)
	if err != nil {
		return
	}
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
			fmt.Fprintln(cmd.OutOrStdout(), ui.Success("Screenshot removed ✓"))
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
unknown set is rejected.`,
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
			fmt.Fprintln(cmd.OutOrStdout(), ui.Success(fmt.Sprintf("Reordered %d screenshots ✓", len(args))))
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
// 🔴 Be precise about what leaving them untagged costs today, because an earlier
// version of this comment was not, and its reasoning was the wrong way round. It
// said tagging would misreport an I/O failure "as a user mistake" — implying the
// status quo reports it as something better. It does not: untagged, a
// permission-denied read exits **5, the network code**. syscall.Errno has both
// Timeout() and Temporary(), so it satisfies net.Error, and cmd/civitai's
// isNetworkErr errors.As-matches it straight through the *fs.PathError.
// Measured: `civitai app listing set-icon <mode-000 png>` exits 5. So the real
// trade is "misreported as retryable transport" vs "misreported as a usage
// mistake", and the decision stands only because the fix belongs in isNetworkErr
// — where it corrects every filesystem error in the CLI at once — and not in a
// per-call-site tag here, which would leave the same defect everywhere else.
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
			return fmt.Errorf("the image was blocked by the content scan — it can't be attached; use a different image")
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
