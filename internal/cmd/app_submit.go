package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/auth"
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
		Short: "Package and submit an App for review",
		Long: `Package the canonical App source tree and submit it for moderator
review.

The package is the SOURCE tree (manifest + src + build config) — NOT a
prebuilt dist. The platform rebuilds from source. These are excluded:
  ` + pkgzip.JoinExcluded() + `

Submission path:
  By default this uploads the bundle directly using your stored token to the
  token-authenticated submit route (POST /api/v1/blocks/submit-version). OAuth
  device-login tokens (` + "`civitai login`" + `) and personal API keys both work;
  OAuth tokens refresh automatically. Set CIVITAI_SUBMIT_PATH to override the
  route. With no token configured (and no --package-only), it writes the
  canonical .zip and prints the manual next steps.

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

			// The submit route defaults to the v1 token route; env overrides it.
			submitPath := os.Getenv("CIVITAI_SUBMIT_PATH")

			// 3a. Programmatic submit if we have a token (OAuth or personal key).
			canUpload := !packageOnly && cfg.Token() != ""
			if canUpload {
				client := api.NewWithSource(cfg.BaseURL(), auth.New(cfg), submitPath)
				return doUpload(cmd, client, pkg.Zip, m, cfg.BaseURL())
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

func doUpload(cmd *cobra.Command, client api.Submitter, zipBytes []byte, m *manifest.Manifest, baseURL string) error {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Submitting %s@%s …\n", m.BlockID, m.Version)
	r, err := client.SubmitVersion(context.Background(), zipBytes, m.BlockID, m.Version)
	if err != nil {
		return err
	}
	base := strings.TrimRight(baseURL, "/")
	fmt.Fprintf(out, "✓ Submitted — %s is pending moderator review.\n", r.PublishRequestID)
	fmt.Fprintf(out, "\nWhat's next: a moderator reviews it; on approval it builds + deploys to https://%s.civit.ai (usually a few minutes).\n", r.Slug)
	fmt.Fprintln(out, "\nTrack it:")
	fmt.Fprintln(out, "  civitai app status                          # review + deploy status")
	fmt.Fprintf(out, "  %s/apps/my-submissions               # your submissions\n", base)
	return nil
}

func printManualNextSteps(cmd *cobra.Command, cfg *config.Config, m *manifest.Manifest, zipPath string) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "\nNo token configured, so the bundle was written but not uploaded.")
	fmt.Fprintln(out, "\n  1) Authenticate, then re-run to upload directly:")
	fmt.Fprintln(out, "     civitai login          # browser device login")
	fmt.Fprintln(out, "     civitai app submit")
	fmt.Fprintf(out, "\n  2) Or upload %s via the web UI:\n", filepath.Base(zipPath))
	fmt.Fprintf(out, "     %s/apps/submit\n", cfg.BaseURL())
	fmt.Fprintln(out, "     (requires an invite while Apps is in invite-only beta).")
}
