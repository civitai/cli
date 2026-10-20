package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/civitai/cli/internal/ui"
)

// newAppListingClearRedCmd removes the listing's civitai.red-only icon or cover
// override — the inverse of `set-icon|set-cover --red`.
//
// Like rm-screenshot it changes media without uploading anything and does NOT
// submit: on a live listing the CLI opens (or reuses) the revision with
// beginListingRevision and clears the override on the SHADOW, so the live
// listing keeps showing the override until a moderator approves the revision.
// The submit stays an explicit `submit-revision`, for the reason given on that
// command.
func newAppListingClearRedCmd() *cobra.Command {
	var lc listingCommon
	cmd := &cobra.Command{
		Use:   "clear-red <icon|cover>",
		Short: "Remove the civitai.red-only icon or cover override",
		Long: `Remove your listing's civitai.red-only ICON or COVER override (the image
` + "`set-icon --red` / `set-cover --red`" + ` attached). civitai.red then shows the
listing's own icon or cover, as civitai.com does.

On a listing that is already LIVE the removal lands in the open REVISION and
is NOT submitted: civitai.red keeps showing the override until a moderator
approves the revision. Run ` + "`civitai app listing submit-revision`" + ` when the
revision is what you want. On a DRAFT listing it applies to the listing itself.`,
		Example: `  civitai app listing clear-red icon
  civitai app listing clear-red cover --slug my-app`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{string(kindIcon), string(kindCover)},
		RunE: func(cmd *cobra.Command, args []string) error {
			slot := args[0]
			if slot != string(kindIcon) && slot != string(kindCover) {
				return asUsageError(fmt.Errorf("clear-red takes `icon` or `cover`, not %q", slot))
			}
			client, err := newListingClient()
			if err != nil {
				return err
			}
			ref, err := resolveListingRefFromFlags(cmd, client, lc)
			if err != nil {
				return err
			}
			if err := refuseUneditableListing(ref.Status); err != nil {
				return err
			}
			ctx := cmdCtx(cmd)
			live := ref.Status == "approved"
			// 🔴 ON A LIVE LISTING THE CLI OPENS THE REVISION ITSELF AND WRITES THE
			// SHADOW ID — the same path runSetMedia takes for `set-icon|set-cover
			// --red`. Sending the PARENT id and relying on the server to redirect is
			// true only for a non-moderator caller: the server's
			// resolveOwnerAssetEditTarget returns the listing UNCHANGED for a
			// moderator, so a parent-id clear was written straight onto the LIVE
			// listing (no review) while this command printed "staged on an open
			// revision". Addressing the shadow makes both callers land in the
			// revision, and the message below true for both.
			targetID := ref.AppListingID
			if live {
				shadowID, _, err := client.BeginListingRevision(ctx, ref.AppListingID)
				if err != nil {
					return err
				}
				targetID = shadowID
			}
			if _, err := client.SetRedMedia(ctx, targetID, slot, nil); err != nil {
				return err
			}
			reportRedOverrideCleared(cmd.OutOrStdout(), slot, live)
			return nil
		},
	}
	lc.bind(cmd)
	return cmd
}

// reportRedOverrideCleared prints the outcome of a clear. The LIVE arm must not
// say "cleared": the removal is staged on an unsubmitted revision, and
// civitai.red still shows the override (the same false-success trap
// reportScreenshotRemoved documents for rm-screenshot).
func reportRedOverrideCleared(out io.Writer, slot string, live bool) {
	subject := subjectHeading(mediaSubject(mediaKind(slot), true))
	if !live {
		fmt.Fprintln(out, ui.Success(subject+" override removed"))
		return
	}
	fmt.Fprintln(out, ui.Success(subject+" override removal staged on an open revision — not submitted for review yet."))
	fmt.Fprintln(out, "civitai.red still shows the override; the removal reaches it only after a")
	fmt.Fprintln(out, "moderator approves the revision.")
	fmt.Fprintf(out, "Publish it: %s — or stage more changes first if you are still curating.\n",
		ui.Code("civitai app listing submit-revision"))
}
