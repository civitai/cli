package cmd

import (
	"fmt"
	"io"
	"strings"

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

It also emits non-fatal WARNINGS for money-path footguns the schema can't catch
as hard errors (e.g. a budgeted page with no page.buzzBudgetPerGen). Warnings do
NOT fail validation (exit 0) unless --strict is passed.

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

// issues converts the flat validation messages into structured objects with a
// machine-readable field (when one can be derived) plus the full message. Schema
// errors are formatted "<json/path>: <reason>"; we surface the leading path as
// the field. Messages with no derivable path carry field "" (omitted).
func issues(msgs []string) []map[string]string {
	out := make([]map[string]string, 0, len(msgs))
	for _, m := range msgs {
		item := map[string]string{"message": m}
		if field, ok := deriveField(m); ok {
			item["field"] = field
		}
		out = append(out, item)
	}
	return out
}

// deriveField pulls the JSON-path field out of a schema-style "<path>: <reason>"
// message. It only treats the prefix as a field when it looks like a path
// (starts with "/" or is the literal "(root)") so prose messages aren't split.
func deriveField(msg string) (string, bool) {
	i := strings.Index(msg, ": ")
	if i <= 0 {
		return "", false
	}
	prefix := msg[:i]
	if prefix == "(root)" || strings.HasPrefix(prefix, "/") {
		return prefix, true
	}
	return "", false
}

func printWarnings(w io.Writer, res validate.Result) {
	if !res.HasWarnings() {
		return
	}
	fmt.Fprintln(w, ui.For(w).Warn(fmt.Sprintf("%d warning(s):", len(res.Warnings))))
	for _, warn := range res.Warnings {
		fmt.Fprintf(w, "  - %s\n", warn)
	}
}
