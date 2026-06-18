package cmd

import (
	"fmt"

	"github.com/civitai/cli/internal/validate"
	"github.com/spf13/cobra"
)

func newAppValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [dir]",
		Short: "Validate block.manifest.json against the App Block schema",
		Long: `Validate an App Block project.

This is a best-effort LOCAL pre-check that mirrors the platform's approve-time
validator (BlockManifestValidator). It catches most rejections before you
submit, but the SERVER remains the source of truth.

Checks block.manifest.json against the vendored JSON Schema (syntactic shape),
plus the ported semantic rules and structural checks:
  - the manifest is present at the project root
  - buildCommand and outputDir are coherent (outputDir set when buildCommand is);
    outputDir must be a safe relative path (no leading "/", no ".." traversal)
  - server-owned fields (iframe.src, trustTier) are REJECTED if set
  - sandbox tokens are limited to the unverified-tier allowlist
    (allow-scripts, allow-forms); allow-same-origin+allow-scripts is rejected
  - a "page" manifest must declare an iframe block; renderMode=iframe needs one too
  - iframe.minHeight and iframe.resizable are required when an iframe is present
  - renderMode inline/hybrid is rejected (requires a verified tier the platform
    only assigns post-submit)
  - targets[].slotId must be a known registered slot

Defaults to the current directory.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			out := cmd.OutOrStdout()

			res, err := validate.Dir(dir)
			if err != nil {
				return err
			}
			if res.OK() {
				fmt.Fprintf(out, "OK — %s is valid\n", dir)
				return nil
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "%d validation error(s) in %s:\n", len(res.Errors), dir)
			for _, e := range res.Errors {
				fmt.Fprintf(cmd.ErrOrStderr(), "  - %s\n", e)
			}
			return fmt.Errorf("validation failed")
		},
	}
	return cmd
}
