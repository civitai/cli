// Package cmd wires the cobra command tree for the civitai CLI.
package cmd

import (
	"runtime/debug"

	"github.com/spf13/cobra"
)

// Build metadata, overridden at build time via -ldflags (see cmd/civitai/main.go
// and .goreleaser.yaml). They default to dev values for source builds.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// readBuildInfo is a seam so tests can stub runtime/debug.ReadBuildInfo.
var readBuildInfo = debug.ReadBuildInfo

// SetBuildInfo lets main inject the build version, commit, and date. Goreleaser
// release binaries pass real ldflag values, which are authoritative. For a plain
// `go install github.com/civitai/cli/cmd/civitai@latest` (or any source build),
// the ldflags are absent and the injected values are still the "dev" defaults —
// in that case we fall back to runtime/debug.ReadBuildInfo so `civitai version`
// reports the module version (e.g. v0.1.1 or a pseudo-version) and the embedded
// VCS revision/time instead of dev/none/unknown.
func SetBuildInfo(v, c, d string) {
	if v != "" {
		version = v
	}
	if c != "" {
		commit = c
	}
	if d != "" {
		date = d
	}
	applyBuildInfoFallback()
}

// applyBuildInfoFallback fills version/commit/date from the embedded Go build
// info when (and only when) the corresponding value is still its dev default —
// i.e. no goreleaser ldflag overrode it. Each field falls back independently.
func applyBuildInfoFallback() {
	info, ok := readBuildInfo()
	if !ok || info == nil {
		return
	}
	if isDevDefault(version) {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			version = v
		}
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if commit == "none" && s.Value != "" {
				commit = s.Value
			}
		case "vcs.time":
			if date == "unknown" && s.Value != "" {
				date = s.Value
			}
		}
	}
}

// isDevDefault reports whether v is one of the placeholder values that mean
// "no real version was stamped in" (so a build-info fallback should apply).
func isDevDefault(v string) bool {
	return v == "" || v == "dev"
}

// NewRootCmd builds the root command with all subcommands attached.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "civitai",
		Short: "Civitai CLI — author and ship App Blocks",
		Long: `civitai is the command-line interface for Civitai (https://civitai.com).

Its first feature group is App Blocks authoring — App Blocks are small,
sandboxed web apps that run inside Civitai surfaces. The CLI scaffolds a
correct project, validates it against the platform contract, and packages it
for submission, so you don't have to hand-format a ZIP.

Get started:

  civitai login                    store your API token
  civitai app init my-app          scaffold a ready-to-build App Block
  civitai app validate             check the manifest before you submit
  civitai app submit               package + submit for review`,
		Example: `  # First time: authenticate, then scaffold and submit an app.
  civitai login
  civitai app init my-first-app --template page-vite
  cd my-first-app
  civitai app validate
  civitai app submit`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}
	root.SetVersionTemplate("civitai {{.Version}}\n")

	root.AddCommand(newAppCmd())
	root.AddCommand(newLoginCmd())
	root.AddCommand(newWhoAmICmd())
	root.AddCommand(newBuzzCmd())
	root.AddCommand(newVersionCmd())
	root.AddCommand(newCompletionCmd())

	return root
}
