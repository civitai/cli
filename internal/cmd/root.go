// Package cmd wires the cobra command tree for the civitai CLI.
package cmd

import (
	"github.com/spf13/cobra"
)

// Build metadata, overridden at build time via -ldflags (see cmd/civitai/main.go
// and .goreleaser.yaml). They default to dev values for source builds.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// SetBuildInfo lets main inject the build version, commit, and date.
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
	root.AddCommand(newVersionCmd())
	root.AddCommand(newCompletionCmd())

	return root
}
