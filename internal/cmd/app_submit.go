package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/manifest"
	"github.com/civitai/cli/internal/pkgzip"
	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/internal/validate"
	"github.com/spf13/cobra"
)

func newAppSubmitCmd() *cobra.Command {
	var outFlag string
	var packageOnly bool
	var skipValidate bool
	var assumeYes bool

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

Submitting creates a real "pending moderator review" request (undone only with
` + "`civitai app withdraw`" + `), so it is NOT fired blindly: before uploading you are
shown the app@version and asked to confirm. Pass --yes/-y to skip the prompt
(for scripts/CI). In a non-interactive shell (no TTY) submit REFUSES unless
--yes is given, rather than hang or submit silently. --package-only is the safe
preview — it never submits.

Defaults to the current directory.`,
		Example: `  civitai app submit                 # validate + package + confirm + submit
  civitai app submit --yes           # skip the confirmation prompt (scripts/CI)
  civitai app submit --package-only  # just write the .zip (safe preview, never submits)
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
					errw := cmd.ErrOrStderr()
					fmt.Fprintln(errw, ui.For(errw).ErrorMsg(fmt.Sprintf("validation failed (%d error(s)) — fix before submitting, or pass --skip-validate:", len(res.Errors))))
					for _, e := range res.Errors {
						fmt.Fprintf(errw, "  - %s\n", e)
					}
					return fmt.Errorf("validation failed")
				}
			}

			m, err := manifest.Load(dir)
			if err != nil {
				return err
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}

			// The submit route defaults to the v1 token route; env overrides it.
			submitPath := os.Getenv("CIVITAI_SUBMIT_PATH")

			// 2. Confirmation gate BEFORE packaging. A programmatic submit fires a
			// REAL moderator-review request immediately, so gate it first: a
			// non-TTY shell without --yes must REFUSE before doing any packaging
			// work (no wasted "Packaged …" line that momentarily reads as if it's
			// proceeding). --yes proceeds silently; an interactive TTY prompts
			// here, then packages + submits below. The --package-only / no-token
			// fallback path never submits, so it skips the gate and just packages.
			canUpload := !packageOnly && cfg.Token() != ""
			if canUpload {
				if err := confirmSubmit(cmd, m, cfg.BaseURL(), assumeYes); err != nil {
					return err
				}
			}

			// 3. Package the canonical source tree.
			pkg, err := pkgzip.Build(dir)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Packaged %d file(s) (%d bytes compressed, %d decompressed)\n",
				len(pkg.Files), len(pkg.Zip), pkg.DecompressedBy)

			// 3a. Programmatic submit if we have a token (OAuth or personal key).
			// The gate above already confirmed (or --yes bypassed) it.
			if canUpload {
				client := appapi.NewWithSource(cfg.BaseURL(), auth.New(cfg), submitPath)
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
			fmt.Fprintln(out, ui.Success(fmt.Sprintf("Wrote canonical bundle: %s", abs)))

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
	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "skip the confirmation prompt and submit (for scripts/CI)")
	return cmd
}

// confirmSubmit gates the actual moderator-review submission behind an explicit
// confirmation, so a bare `civitai app submit` never fires an outward-facing,
// hard-to-reverse publish request by accident.
//
//   - --yes/-y  → proceed without prompting.
//   - non-TTY stdin (pipe/CI) without --yes → REFUSE (clear error, non-zero
//     exit) rather than hang waiting on input or submit silently.
//   - interactive TTY → print what will happen, prompt "Submit for review?
//     [y/N]", and proceed only on an explicit yes.
func confirmSubmit(cmd *cobra.Command, m *manifest.Manifest, baseURL string, assumeYes bool) error {
	if assumeYes {
		return nil
	}
	if !stdinIsTTY() {
		return fmt.Errorf("refusing to submit without --yes in a non-interactive shell (submitting creates a real moderator-review request; pass --yes to confirm, or --package-only to just write the .zip)")
	}

	out := cmd.OutOrStdout()
	base := strings.TrimRight(baseURL, "/")
	fmt.Fprintf(out, "About to submit %s@%s for moderator review at %s.\n", m.BlockID, m.Version, base)
	fmt.Fprintln(out, "This creates a real pending moderator-review request — reversible only via `civitai app withdraw`.")
	fmt.Fprintln(out, "(Use --package-only to just write the .zip without submitting.)")
	fmt.Fprint(out, "Submit for review? [y/N]: ")

	r := bufio.NewReader(cmd.InOrStdin())
	line, _ := r.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	default:
		return fmt.Errorf("submission cancelled")
	}
}

func doUpload(cmd *cobra.Command, client appapi.Submitter, zipBytes []byte, m *manifest.Manifest, baseURL string) error {
	out := cmd.OutOrStdout()
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Spin (on a TTY) while the bundle uploads — a real network wait. On a non-TTY
	// (pipe/CI/tests) WithSpinner prints one plain "Submitting …" line and runs the
	// upload inline, so scripted/captured output stays deterministic.
	var r *appapi.SubmitResult
	err := ui.WithSpinner(ctx, out, fmt.Sprintf("Submitting %s@%s", m.BlockID, m.Version),
		func(ctx context.Context) error {
			var e error
			r, e = client.SubmitVersion(ctx, zipBytes, m.BlockID, m.Version)
			return e
		})
	if err != nil {
		return err
	}
	base := strings.TrimRight(baseURL, "/")
	fmt.Fprintln(out, ui.Success(fmt.Sprintf("Submitted — %s is pending moderator review.", r.PublishRequestID)))
	fmt.Fprintf(out, "\nWhat's next: a moderator reviews it; on approval it builds + deploys to %s (usually a few minutes).\n", ui.URL(fmt.Sprintf("https://%s.civit.ai", r.Slug)))
	fmt.Fprintln(out, "\nTrack it:")
	fmt.Fprintf(out, "  %s                          # review + deploy status\n", ui.Code("civitai app status"))
	fmt.Fprintf(out, "  %s/apps/my-submissions               # your submissions\n", base)
	printListingFloorHeadsUp(out)
	printMoneyPathNote(out, m)
	return nil
}

// printListingFloorHeadsUp is a non-fatal reminder (issue #186 point 4) that a
// store listing won't publish without an icon + cover, with the exact commands
// to add them. Kept static (no server call) so it works even pre-first-listing.
func printListingFloorHeadsUp(out io.Writer) {
	fmt.Fprintln(out, "\nStore listing: once your app is APPROVED, its store listing needs an icon AND a cover before it can publish.")
	fmt.Fprintln(out, "After approval, add them with:")
	fmt.Fprintf(out, "  %s\n", ui.Code("civitai app listing set-icon <file>"))
	fmt.Fprintf(out, "  %s\n", ui.Code("civitai app listing set-cover <file>"))
	fmt.Fprintf(out, "  %s   # what's attached vs. required (after approval)\n", ui.Code("civitai app listing status"))
}

// manifestNeedsSpend reports whether the manifest declares a Buzz-spend scope
// (an `ai:write*` scope) — i.e. the money path where the OAuth spend dead end
// is relevant. Keeps the note scoped to money apps only.
func manifestNeedsSpend(m *manifest.Manifest) bool {
	for _, s := range m.Scopes {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(s)), "ai:write") {
			return true
		}
	}
	return false
}

// printMoneyPathNote prints a concise reminder — only for money-path apps — that
// real `dev:live` Buzz spend needs a full-scope personal API key, since an OAuth
// `civitai login` can submit/withdraw but cannot spend Buzz (issue #34).
func printMoneyPathNote(out io.Writer, m *manifest.Manifest) {
	if !manifestNeedsSpend(m) {
		return
	}
	fmt.Fprintln(out, "\nNote: real `dev:live` Buzz spend needs a full-scope personal API key")
	fmt.Fprintln(out, "(create at https://civitai.com/user/account, then `civitai login --token <key>`).")
	fmt.Fprintln(out, "An OAuth `civitai login` can submit/withdraw but cannot spend Buzz — check with `civitai whoami`.")
}

func printManualNextSteps(cmd *cobra.Command, cfg *config.Config, m *manifest.Manifest, zipPath string) {
	out := cmd.OutOrStdout()
	base := strings.TrimRight(cfg.BaseURL(), "/")
	// Lead with an unmistakable NOT-submitted banner: no token means the bundle
	// was written locally but never reached the server, so no review started.
	fmt.Fprintln(out)
	fmt.Fprintln(out, ui.Warn("NOT SUBMITTED — no token configured, so the bundle was written locally but never uploaded to Civitai."))
	fmt.Fprintln(out, "No moderator review has started. To actually submit it, do ONE of:")
	fmt.Fprintln(out, "\n  1) Authenticate, then re-run to upload directly:")
	fmt.Fprintln(out, "     civitai login          # browser device login")
	fmt.Fprintln(out, "     civitai app submit")
	fmt.Fprintf(out, "     %s/apps/my-submissions   # your submissions appear here once uploaded\n", base)
	fmt.Fprintf(out, "\n  2) Or upload %s via the web UI:\n", filepath.Base(zipPath))
	fmt.Fprintf(out, "     %s/apps/submit\n", base)
	fmt.Fprintln(out, "     (requires an invite while Apps is in invite-only beta).")
	printMoneyPathNote(out, m)
}
