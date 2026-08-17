package cmd

import (
	"bufio"
	"context"
	"errors"
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
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// submitDiagnosisEntries is how many bundle entries the failure diagnosis names.
// Small on purpose: the list is printed under an error the author is already
// reading, and #423's cause (a directory of review screenshots) is visible in
// the first few rows because the offending files were the big ones. A longer
// list would bury the size line above it, which is the more important fact.
const submitDiagnosisEntries = 5

// printSubmitSizeDiagnosis writes what the CLI knows about the bundle whose
// upload just failed: the exact number of bytes it put on the wire, and the
// largest entries those bytes were made of.
//
// 🔴 IT IS AN ACCOUNT OF WHAT WAS SENT, NOT A DIAGNOSIS OF WHY IT WAS REFUSED,
// and the wording keeps that line. The CLI cannot see the server's limits: it
// does not know this failure is about size, and it must not imply it does —
// this same block prints under a 500 that has nothing to do with the bundle.
// What it can say is true of every one of those cases: here is what left this
// machine. See issue #423 for the failure that made the distinction matter, and
// pkgzip's cap comment for why the honest move is to report rather than refuse.
//
// It is on the FAILURE path only. On a success there is nothing to diagnose,
// and the size already appears on the `Packaged …` line for anyone who wants
// it — printing an entry table after every submit would train people to skip
// the block, which is precisely when it is worth reading.
func printSubmitSizeDiagnosis(w io.Writer, zipBytes []byte) {
	fmt.Fprintf(w, "\nWhat this CLI sent (it cannot tell whether that is why the submit failed):\n")
	fmt.Fprintf(w, "  %d bytes on the wire — a %d-byte zip, base64-encoded into a JSON body.\n",
		appapi.SubmitBodySize(len(zipBytes)), len(zipBytes))

	entries, err := pkgzip.LargestEntries(zipBytes, submitDiagnosisEntries)
	if err == nil && len(entries) > 0 {
		fmt.Fprintf(w, "  largest entries in the bundle (compressed / original):\n")
		for _, e := range entries {
			fmt.Fprintf(w, "    %10d / %-10d  %s\n", e.Compressed, e.Uncompressed, e.Name)
		}
	}

	for _, line := range wrapRunes("The size the server applies any request-body limit to is the first number, "+
		"not the zip. This CLI's own size caps are not the server's and are much higher, so clearing them is "+
		"not a prediction that a submit will be accepted (issue #423). If the bundle carries files the platform "+
		"build does not need, drop them and retry:", 78) {
		fmt.Fprintf(w, "  %s\n", line)
	}
	fmt.Fprintf(w, "    %s   # writes the exact .zip, so you can list it before retrying\n",
		ui.Code("civitai app submit --package-only"))
}

// skippedListCap is how many skipped paths the `Skipped …` line names before it
// truncates. Measured over six real Vite-shaped block projects the skipped set
// was 3–4 entries (`node_modules/`, `dist/`, usually `.git/` or `.venv/`, plus
// 0–1 pattern-matched), so this is a monorepo tail-guard and not the normal
// path: on a real project nothing is ever elided. It is deliberately generous
// for that reason — a cap that bites routinely would hide the one entry the
// author needed, which is the failure this whole line exists to fix.
const skippedListCap = 12

// renderSkippedLine renders pkgzip's skip decisions as the one line printed
// under `Packaged …`, or "" when nothing was skipped.
//
// 🔴 THIS IS THE ONE PLACE THE SKIP LIST IS RENDERED. pkgzip returns data and no
// String() — a second renderer is how the count and the list start disagreeing.
//
// 🔴 THE COUNT IS OF DECISION POINTS, NOT OF FILES, AND THE WORDING IS PICKED TO
// SAY SO. An excluded directory is ONE path: Build returns filepath.SkipDir, so
// what is under it is never enumerated and any file count would be invented.
// "Skipped 4 path(s)" is true of that; "skipped 4 files" would not be.
//
// The count stays accurate when the LIST is truncated — the prefix counts every
// decision and the tail says how many are not shown, so the two never disagree:
//
//	Skipped 4 path(s): public/environment.env (*.env), .git/, dist/, node_modules/
//	Skipped 15 path(s): a.env (*.env), … , src/z/ … and 3 more
//
// A directory carries a trailing "/" here and NOT in pkgzip.Skip.Path: the slash
// is a rendering decision (it is what tells an author a whole subtree went),
// and keeping it out of the data means a consumer comparing paths has nothing
// to strip.
func renderSkippedLine(skips []pkgzip.Skip) string {
	if len(skips) == 0 {
		return ""
	}
	shown := skips
	if len(shown) > skippedListCap {
		shown = shown[:skippedListCap]
	}
	names := make([]string, 0, len(shown))
	for _, s := range shown {
		name := s.Path
		if s.Dir {
			name += "/"
		}
		if s.Rule != "" {
			name += " (" + s.Rule + ")"
		}
		names = append(names, name)
	}
	line := fmt.Sprintf("Skipped %d path(s): %s", len(skips), strings.Join(names, ", "))
	if rest := len(skips) - len(shown); rest > 0 {
		line += fmt.Sprintf(" … and %d more", rest)
	}
	return line
}

// wrapCommaList renders names as a comma-separated list broken every perLine
// entries, indented to sit under a help-text heading. The 18 excluded directory
// names ran to ~130 characters on one line, which wraps badly in an 80-column
// terminal — and this list is the part of the help an author actually scans.
//
// Whitespace-only formatting: TestSubmitHelpIsTheReviewedCopy compares the help
// NORMALISED, so re-wrapping is free and rewording is not.
func wrapCommaList(names []string, perLine int) string {
	if perLine < 1 {
		// A zero would panic on i%perLine, and the caller wanting "no wrapping"
		// is the only sane reading of a non-positive value.
		perLine = len(names) + 1
	}
	var b strings.Builder
	for i, n := range names {
		if i > 0 {
			if i%perLine == 0 {
				b.WriteString(",\n  ")
			} else {
				b.WriteString(", ")
			}
		}
		b.WriteString(n)
	}
	return b.String()
}

func newAppSubmitCmd() *cobra.Command {
	var outFlag string
	var packageOnly bool
	var skipValidate bool
	var assumeYes bool
	var allowDowngrade bool
	var allowDirty bool

	cmd := &cobra.Command{
		Use:   "submit [dir]",
		Short: "Package and submit an App for review",
		Long: `Package the canonical App source tree and submit it for moderator
review.

The package is the SOURCE tree (manifest + src + build config) — NOT a
prebuilt dist. The platform rebuilds from source.

Excluded DIRECTORIES, by name, at any depth:
  ` + wrapCommaList(pkgzip.ExcludedNames(), 6) + `
...and by pattern: ` + pkgzip.DirectoryPatternSummary() + `

Excluded FILES, by base name:
  ` + wrapCommaList(pkgzip.ExcludedFilePatterns(), 6) + `
...but these three are KEPT and uploaded, anywhere every directory above them
is itself packaged:
  ` + strings.Join(pkgzip.KeptEnvFileNames(), ", ") + `
Nothing reads their contents, so put no token in any of them. A kept name does
NOT rescue its directory: .env.d/.env.production and
node_modules/pkg/.env.production are both dropped.

The two lists are separate rules, so the shape matters: a regular file named
build or dist IS packaged, and .git / .hg / .svn go either way (in a linked
worktree or a submodule, .git is a file).

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

Version guard:
  A submit that would really upload first checks the app's own submissions and
  REFUSES when the manifest version is not strictly above the highest APPROVED
  version — submitting an older (or the same) version replaces the newer live
  deployment on approval, which is what a repo that is behind what was last
  released produces naturally. Pass --allow-downgrade for a deliberate rollback.

Dirty-tree guard:
  When the packaged directory is inside a git work tree, a submit that would
  really upload REFUSES while files that go into the bundle are uncommitted —
  the bundle is built from what is on disk, so approving one deploys code that
  exists in no commit. Pass --allow-dirty to submit the tree as it is. This
  degrades: a directory with no git repo (every scaffolded app starts that way)
  submits exactly as before, and a clean tree whose HEAD is on no remote warns
  rather than refusing.

Defaults to the current directory.`,
		Example: `  civitai app submit                    # validate + package + confirm + submit
  civitai app submit --yes              # skip the confirmation prompt (scripts/CI)
  civitai app submit --package-only     # just write the .zip (safe preview, never submits)
  civitai app submit --allow-downgrade  # deliberate rollback below the approved version
  civitai app submit --allow-dirty      # submit uncommitted working-tree changes on purpose
  civitai app submit -o my-block.zip ./my-block`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			out := cmd.OutOrStdout()

			// 0. Classify the path the USER named. Same gate `app validate`
			// uses — one rule, one place (resolveProjectDir, project_dir.go):
			// a nonexistent path or a file exits 2, a real directory with no
			// manifest keeps its validation verdict and exit 1.
			//
			// It runs UNCONDITIONALLY, ahead of --skip-validate, because it is
			// not a validation check: `--skip-validate` waives our opinion of
			// the manifest, not the question of whether the directory the user
			// typed exists at all.
			if err := resolveProjectDir(dir); err != nil {
				return err
			}

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
						// printFinding, not a raw Fprintf: `printWarnings` below
						// wraps, so a bare one here printed a 400-char unwrapped
						// error and a wrapped warning in the SAME run — two
						// layouts on the highest-traffic path. It was the fourth
						// print site and the one that made "one place fixes
						// every long message" false.
						printFinding(errw, e.Message)
					}
					// Warnings are useful context on a failure too, and this is
					// the last moment before the app would have gone to review.
					printWarnings(errw, res)
					return fmt.Errorf("validation failed")
				}
				// Non-fatal advisories still have to be SEEN. `submit` is the
				// highest-traffic path and the last point before an app reaches
				// review, so printing them only in `app validate` means the
				// ready-ack advisory — which predicts a blank failure card in the
				// real host — reaches nobody who does not run validate by hand.
				// They do NOT block the submit; that stays the --strict contract.
				printWarnings(cmd.ErrOrStderr(), res)
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
			var client *appapi.Client
			if canUpload {
				client = appapi.NewWithSource(cfg.BaseURL(), auth.New(cfg), submitPath)

				// 2a. DIRTY-WORK-TREE GUARD (issue #411), BEFORE the
				// confirmation prompt.
				//
				// 🔴 EARLIER THAN THE VERSION GUARD, AND THE DIFFERENCE IS THE
				// NETWORK. checkVersionNotRegression sits after confirmSubmit
				// because moving it earlier would give a bare non-TTY `civitai
				// app submit` — the accidental-footgun invocation, which refuses
				// for lack of --yes — an API read it does not have today, and
				// TestAppSubmit_NonTTYRefusesWithoutYes_NoNetworkCall pins that.
				// This guard reads only the local filesystem through `git`, so
				// running it first acquires nothing: that test's invocation still
				// touches no network, and it keeps the CLI from asking "Submit
				// for review? [y/N]" about a submit it has already decided to
				// refuse. Pinned by TestDirtyGuardRefusesBeforePrompting.
				//
				// It runs only on the canUpload path, for the same reason the
				// version guard does: --package-only and the no-token fallback
				// never reach the server, so nothing there can publish an
				// untraceable bundle.
				if err := checkWorkTreeClean(gitOutput, cmd.ErrOrStderr(), dir, m.BlockID, m.Version, allowDirty); err != nil {
					return err
				}

				if err := confirmSubmit(cmd, m, cfg.BaseURL(), assumeYes); err != nil {
					return err
				}

				// 2b. MONOTONIC-VERSION GUARD (issue #412), between the
				// confirmation and the packaging.
				//
				// 🔴 THE ORDER IS DELIBERATE: the free LOCAL gate runs before
				// the one that costs a network read. Putting the guard first
				// would avoid prompting for a submit we are about to refuse —
				// but it would also make a bare `civitai app submit` in a
				// non-TTY shell (the accidental-footgun case, which refuses for
				// lack of --yes) reach the API first, and
				// TestAppSubmit_NonTTYRefusesWithoutYes_NoNetworkCall pins that
				// invocation as touching nothing. The cost of this order is
				// one confusing sequence on an interactive TTY — prompt, "y",
				// then the refusal — which is strictly better than a
				// confirmation gate that quietly acquired a network dependency.
				//
				// It runs only on the canUpload path: --package-only and the
				// no-token fallback never reach the server, so nothing there can
				// regress a live deployment. See app_submit_version_guard.go for
				// every branch of the guard itself.
				ctx := cmd.Context()
				if ctx == nil {
					ctx = context.Background()
				}
				if err := checkVersionNotRegression(ctx, client.ListSubmissions, cmd.ErrOrStderr(), m.BlockID, m.Version, allowDowngrade); err != nil {
					return err
				}
			}

			// 3. Package the canonical source tree.
			pkg, err := pkgzip.Build(dir)
			if err != nil {
				return err
			}
			// 🔴 THE THIRD NUMBER IS THE ONE A SIZE LIMIT APPLIES TO (#423).
			// The zip is base64-encoded into a JSON document before it is sent,
			// so the server never sees the compressed size — it sees ~4/3 of
			// it. The author who hit #423 read `8201270 bytes compressed` off
			// this line and had no way to reach the ~10.9 MB that was really
			// refused; no local measurement corresponded to the rejected
			// quantity at all. It is printed on every path including
			// --package-only, because --package-only is how you inspect a
			// bundle you cannot submit.
			fmt.Fprintf(out, "Packaged %d file(s) (%d bytes compressed, %d decompressed; %d bytes as the base64 JSON submit body)\n",
				len(pkg.Files), len(pkg.Zip), pkg.DecompressedBy, appapi.SubmitBodySize(len(pkg.Zip)))
			// 🔴 WHAT WAS LEFT OUT, ON EVERY PATH INCLUDING --package-only (#435).
			// Printed here, above the canUpload branch, for the same reason the
			// size line is: --package-only is how an author inspects a bundle,
			// and a drop they cannot see is a drop they meet at runtime in the
			// deployed app. Empty prints nothing — see renderSkippedLine.
			if line := renderSkippedLine(pkg.Skipped); line != "" {
				fmt.Fprintln(out, line)
			}

			// 3a. Programmatic submit if we have a token (OAuth or personal key).
			// The gate above already confirmed (or --yes bypassed) it.
			if canUpload {
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
	cmd.Flags().BoolVar(&allowDowngrade, "allow-downgrade", false, "submit even when the version is not above the highest approved one (deliberate rollback)")
	cmd.Flags().BoolVar(&allowDirty, "allow-dirty", false, "submit even when the packaged directory has uncommitted git changes")
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
		// 🔴 THE SERVER'S MESSAGE IS ALREADY VERBATIM, AND FOR #423 IT NAMES
		// NOTHING. appapi.serverError prints the response body as it arrived, so
		// there is no better message to extract from it: a bundle over the
		// request-body limit produced `400: Invalid JSON` — an error about the
		// PARSE, downstream of the real cause — and re-plumbing that decode
		// cannot recover information the response does not carry.
		//
		// What the CLI does hold, and the response does not, is what it SENT. So
		// add exactly that, labelled as the CLI's own account rather than
		// dressed up as a diagnosis: the size that went on the wire, and the
		// entries it was made of. Reaching those took `--package-only` plus
		// `unzip -l` when #423 was filed.
		//
		// Deliberately not printed for a credential refusal (401/403 — the
		// invite-only Apps beta is the most common submit failure and has
		// nothing to do with the bundle) or a 429, where an entry list is noise
		// on top of an error that already says what to do.
		if !errors.Is(err, civitai.ErrUnauthorized) && !errors.Is(err, civitai.ErrRateLimited) {
			printSubmitSizeDiagnosis(cmd.ErrOrStderr(), zipBytes)
		}
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
// This submit MINTED the store listing as a draft, so the media is settable NOW —
// no need to wait for approval.
//
// 🔴 THE "CARRIES FORWARD" LINE CARRIES ITS CAVEAT (#350). This was one of the
// three surfaces that promised the media survives, unqualified, and it is the
// worst of them: it is printed at the exact moment the author decides to spend
// an afternoon on artwork, and it was reprinted verbatim over an emptied listing
// after a withdraw→resubmit. See withdrawListingCaveat.
func printListingFloorHeadsUp(out io.Writer) {
	fmt.Fprintln(out, "\nStore listing: your listing needs an icon AND a cover before it can publish.")
	// Wrapped at 78 so the caveat does not arrive as one 130-column line that a
	// standard terminal breaks mid-clause; the rest of this block is already
	// under 80.
	for _, line := range wrapRunes("You can add them NOW, while the app is in review — they "+withdrawListingCaveat+":", 78) {
		fmt.Fprintln(out, line)
	}
	fmt.Fprintf(out, "  %s\n", ui.Code("civitai app listing set-icon <file>"))
	fmt.Fprintf(out, "  %s\n", ui.Code("civitai app listing set-cover <file>"))
	fmt.Fprintf(out, "  %s   # what's attached vs. required\n", ui.Code("civitai app listing status"))
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
// real `dev:live` Buzz spend needs the AI Services scopes, which a DEFAULT OAuth
// `civitai login` deliberately omits (issue #34). Since `login --scopes
// generate` shipped there are TWO routes, and naming only the personal key sent
// people to the web UI unnecessarily.
func printMoneyPathNote(out io.Writer, m *manifest.Manifest) {
	if !manifestNeedsSpend(m) {
		return
	}
	fmt.Fprintln(out, "\nNote: real `dev:live` Buzz spend needs the AI Services scopes. Two routes:")
	fmt.Fprintln(out, "  civitai login --scopes generate  # a browser login that opts into generation")
	fmt.Fprintln(out, "  civitai login --token <key>      # or a full-scope personal API key: https://civitai.com/user/account")
	fmt.Fprintln(out, "A DEFAULT `civitai login` can submit/withdraw but cannot spend Buzz — check with `civitai whoami`.")
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
