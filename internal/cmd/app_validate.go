package cmd

import (
	"fmt"
	"io"

	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/internal/validate"
	"github.com/spf13/cobra"
)

func newAppValidateCmd() *cobra.Command {
	var strict bool
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "validate [dir]",
		Short: "Validate block.manifest.json against the App schema",
		Long: `Validate an App project.

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
  - the committed LOCKFILE matches the package manager the platform build
    derives from buildCommand (its first word): pnpm -> pnpm-lock.yaml,
    yarn -> yarn.lock, and npm/vite/npx/unset -> package-lock.json. The
    platform installs strictly from the lockfile, so a mismatch or a missing
    lockfile is a guaranteed build failure. Only applies when package.json
    exists — a static app never installs.

It also emits non-fatal WARNINGS the schema can't catch as hard errors:
  - money-path footguns (e.g. a budgeted page with no page.buzzBudgetPerGen)
  - a "page" app whose source never posts BLOCK_READY. The host will not reveal
    a page app until it acks BLOCK_INIT, so such an app renders fine locally and
    is replaced by a failure card in the real host — the shape of anything
    scaffolded before that was fixed. Advisory ONLY: it infers runtime behaviour
    from static text. A project depending on @civitai/* is never flagged (the
    SDK transport acks internally), and it reads source only — never outputDir,
    node_modules, markdown, or comments.
Warnings do NOT fail validation (exit 0) unless --strict is passed.

Defaults to the current directory.`,
		Example: `  civitai app validate            # the current directory
  civitai app validate ./my-block
  civitai app validate --strict   # treat warnings as failures
  civitai app validate --json     # raw JSON result (scriptable)`,
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

			// JSON mode: emit the full structured result to stdout (so an agent
			// can parse failures), then preserve the same non-zero exit the text
			// path uses (hard errors always fail; warnings fail only --strict).
			if jsonOut {
				ok := res.OK() && !(strict && res.HasWarnings())
				payload := map[string]any{
					"ok":       ok,
					"dir":      dir,
					"errors":   issues(res.Errors),
					"warnings": issues(res.Warnings),
				}
				if err := writeJSON(out, payload); err != nil {
					return err
				}
				if !res.OK() {
					return fmt.Errorf("validation failed")
				}
				if strict && res.HasWarnings() {
					return fmt.Errorf("validation failed: %d warning(s) with --strict", len(res.Warnings))
				}
				return nil
			}

			// Hard errors always fail. Style the header against stderr (per-stream
			// color) so a colored stderr while stdout is piped still works.
			if !res.OK() {
				fmt.Fprintln(errw, ui.For(errw).ErrorMsg(fmt.Sprintf("%d validation error(s) in %s:", len(res.Errors), dir)))
				for _, e := range res.Errors {
					printFinding(errw, e.Message)
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
				fmt.Fprintln(out, ui.Success(fmt.Sprintf("%s is valid (with %d warning(s))", dir, len(res.Warnings))))
				return nil
			}

			fmt.Fprintln(out, ui.Success(fmt.Sprintf("%s is valid", dir)))
			return nil
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "treat warnings as failures (non-zero exit)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the validation result as JSON (scriptable)")
	return cmd
}

// issues renders findings as the `--json` objects README promises: `field` plus
// `message`, on EVERY finding.
//
// 🔴 The field is READ OFF the finding, never re-derived from the message. It
// used to be recovered by string-parsing a schema-style "<path>: <reason>"
// prefix, which worked for schema errors and produced `field: null` for every
// ported semantic check — the prose ones, i.e. the findings a local pre-check
// exists for (issue #225). Do not reintroduce a parser here: a new check that
// emits prose would silently go back to null, with nothing failing.
//
// The empty-field fallback is defence in depth for the README's "every finding
// carries a field" promise, not a working path — `TestFindingsAreConstructedWithAField`
// rejects an empty field at the construction site, and
// `TestEveryCheckEmitsAField` drives the checks and asserts it end to end. If it
// ever fires, the finding is at least grouped somewhere honest instead of null.
func issues(fs []validate.Finding) []map[string]string {
	out := make([]map[string]string, 0, len(fs))
	for _, f := range fs {
		field := f.Field
		if field == "" {
			field = validate.FieldDocument
		}
		out = append(out, map[string]string{"field": field, "message": f.Message})
	}
	return out
}

func printWarnings(w io.Writer, res validate.Result) {
	if !res.HasWarnings() {
		return
	}
	fmt.Fprintln(w, ui.For(w).Warn(fmt.Sprintf("%d warning(s):", len(res.Warnings))))
	for _, warn := range res.Warnings {
		printFinding(w, warn.Message)
	}
}
