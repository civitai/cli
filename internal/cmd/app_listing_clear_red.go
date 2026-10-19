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
// Like rm-screenshot it changes media without uploading anything, and it follows
// the same live-listing rule: the server writes the change into the open
// revision (minting it if needed) and does NOT submit it, so the live listing
// keeps showing the override until a moderator approves the revision. The
// submit stays an explicit `submit-revision`, for the reason given on that
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
			if _, err := client.SetRedMedia(cmdCtx(cmd), ref.AppListingID, slot, nil); err != nil {
				return err
			}
			reportRedOverrideCleared(cmd.OutOrStdout(), slot, ref.Status == "approved")
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
