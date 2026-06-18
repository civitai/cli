package cmd

import (
	"fmt"
	"io"

	"github.com/civitai/cli/internal/validate"
	"github.com/spf13/cobra"
)

func newAppValidateCmd() *cobra.Command {
	var strict bool
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

It also emits non-fatal WARNINGS for money-path footguns the schema can't catch
as hard errors (e.g. a budgeted page with no page.buzzBudgetPerGen). Warnings do
NOT fail validation (exit 0) unless --strict is passed.

Defaults to the current directory.`,
		Example: `  civitai app validate            # the current directory
  civitai app validate ./my-block
  civitai app validate --strict   # treat warnings as failures`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			out := cmd.OutOrStdout()
			errw := cmd.ErrOrStderr()

			res, err := validate.Dir(dir)
			if err != nil {
				return err
			}

			// Hard errors always fail.
			if !res.OK() {
				fmt.Fprintf(errw, "%d validation error(s) in %s:\n", len(res.Errors), dir)
				for _, e := range res.Errors {
					fmt.Fprintf(errw, "  - %s\n", e)
				}
				// Surface warnings too — they're useful context even on a failure.
				printWarnings(errw, res)
				return fmt.Errorf("validation failed")
			}

			// Warnings: print to stderr, fail only under --strict.
			if res.HasWarnings() {
				printWarnings(errw, res)
				if strict {
					return fmt.Errorf("validation failed: %d warning(s) with --strict", len(res.Warnings))
				}
				fmt.Fprintf(out, "OK (with %d warning(s)) — %s is valid\n", len(res.Warnings), dir)
				return nil
			}

			fmt.Fprintf(out, "OK — %s is valid\n", dir)
			return nil
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "treat warnings as failures (non-zero exit)")
	return cmd
}

func printWarnings(w io.Writer, res validate.Result) {
	if !res.HasWarnings() {
		return
	}
	fmt.Fprintf(w, "%d warning(s):\n", len(res.Warnings))
	for _, warn := range res.Warnings {
		fmt.Fprintf(w, "  ! %s\n", warn)
	}
}
