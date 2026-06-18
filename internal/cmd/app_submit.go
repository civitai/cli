package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/manifest"
	"github.com/civitai/cli/internal/pkgzip"
	"github.com/civitai/cli/internal/validate"
	"github.com/spf13/cobra"
)

func newAppSubmitCmd() *cobra.Command {
	var outFlag string
	var packageOnly bool
	var skipValidate bool

	cmd := &cobra.Command{
		Use:   "submit [dir]",
		Short: "Package and submit an App Block for review",
		Long: `Package the canonical App Block source tree and submit it for moderator
review.

The package is the SOURCE tree (manifest + src + build config) — NOT a
prebuilt dist. The platform rebuilds from source. These are excluded:
  ` + pkgzip.JoinExcluded() + `

Submission path:
  The live server upload route (POST /api/blocks/submit-version) is SESSION-
  COOKIE + moderator authenticated today — it does NOT accept an API token, so
  a fully programmatic token submit needs a companion server endpoint (see the
  repo README / PR). When CIVITAI_SUBMIT_PATH points at such a token-accepting
  endpoint, this command uploads directly. Otherwise it writes the canonical
  .zip and prints the exact manual next steps.

  --package-only always just writes the .zip and stops.

Defaults to the current directory.`,
		Example: `  civitai app submit                 # validate + package + submit (or print next steps)
  civitai app submit --package-only  # just write the .zip
  civitai app submit -o my-block.zip ./my-block`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			out := cmd.OutOrStdout()

			// 1. Validate first — never submit a known-bad manifest.
			if !skipValidate {
				res, err := validate.Dir(dir)
				if err != nil {
					return err
				}
				if !res.OK() {
					fmt.Fprintf(cmd.ErrOrStderr(), "validation failed (%d error(s)) — fix before submitting, or pass --skip-validate:\n", len(res.Errors))
					for _, e := range res.Errors {
						fmt.Fprintf(cmd.ErrOrStderr(), "  - %s\n", e)
					}
					return fmt.Errorf("validation failed")
				}
			}

			m, err := manifest.Load(dir)
			if err != nil {
				return err
			}

			// 2. Package the canonical source tree.
			pkg, err := pkgzip.Build(dir)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Packaged %d file(s) (%d bytes compressed, %d decompressed)\n",
				len(pkg.Files), len(pkg.Zip), pkg.DecompressedBy)

			cfg, err := config.Load()
			if err != nil {
				return err
			}

			// Resolve the (optional) token-accepting submit endpoint.
			submitPath := os.Getenv("CIVITAI_SUBMIT_PATH")

			// 3a. Programmatic submit if we have both a token and a configured
			// token-accepting endpoint.
			canUpload := !packageOnly && cfg.Token() != "" && submitPath != ""
			if canUpload {
				client := api.New(cfg.BaseURL(), cfg.Token(), submitPath)
				return doUpload(cmd, client, pkg.Zip, m)
			}

			// 3b. Fallback: write the canonical .zip + print next steps.
			zipPath := outFlag
			if zipPath == "" {
				zipPath = fmt.Sprintf("%s-%s.zip", m.BlockID, m.Version)
			}
			if err := os.WriteFile(zipPath, pkg.Zip, 0o644); err != nil {
				return err
			}
			abs, _ := filepath.Abs(zipPath)
			fmt.Fprintf(out, "Wrote canonical bundle: %s\n", abs)

			if packageOnly {
				return nil
			}

			printManualNextSteps(cmd, cfg, m, abs)
			return nil
		},
	}
	cmd.Flags().StringVarP(&outFlag, "out", "o", "", "output .zip path (default: <blockId>-<version>.zip)")
	cmd.Flags().BoolVar(&packageOnly, "package-only", false, "only write the .zip; do not attempt submission")
	cmd.Flags().BoolVar(&skipValidate, "skip-validate", false, "skip manifest validation before packaging")
	return cmd
}

func doUpload(cmd *cobra.Command, client api.Submitter, zipBytes []byte, m *manifest.Manifest) error {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Submitting %s@%s ...\n", m.BlockID, m.Version)
	r, err := client.SubmitVersion(context.Background(), zipBytes)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Submitted. Publish request %s (%s@%s) is now %s — pending moderator review.\n",
		r.PublishRequestID, r.Slug, r.Version, r.Status)
	return nil
}

func printManualNextSteps(cmd *cobra.Command, cfg *config.Config, m *manifest.Manifest, zipPath string) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "\nSubmission is not yet automated for API tokens. Submit the bundle one of these ways:")
	fmt.Fprintf(out, "\n  1) Web (works today): upload %s at\n", filepath.Base(zipPath))
	fmt.Fprintf(out, "     %s/apps/submit\n", cfg.BaseURL())
	fmt.Fprintln(out, "     (requires a moderator account while App Blocks is mod-gated).")
	fmt.Fprintln(out, "\n  2) Git push (for updates after your first version is approved):")
	fmt.Fprintln(out, "     after approval, `blocks.getMyAppRepo` provisions a Forgejo repo —")
	fmt.Fprintln(out, "     clone it and `git push` to park a pending review.")
	fmt.Fprintln(out, "\n  To enable `civitai app submit` to upload directly, the server needs a")
	fmt.Fprintln(out, "  token-authenticated submit endpoint; set CIVITAI_SUBMIT_PATH to it once it exists.")
}
