// Package cmd wires the cobra command tree for the civitai CLI.
package cmd

import (
	"github.com/spf13/cobra"
)

// version is overridden at build time via -ldflags.
var version = "dev"

// SetVersion lets main inject the build version.
func SetVersion(v string) {
	if v != "" {
		version = v
	}
}

// NewRootCmd builds the root command with all subcommands attached.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "civitai",
		Short: "Civitai CLI — author and ship App Blocks (and more)",
		Long: `civitai is the unified command-line interface for Civitai.

Its first feature group is App Blocks authoring:

  civitai app init my-block        scaffold a ready-to-build block project
  civitai app validate             validate block.manifest.json
  civitai app submit               package + submit for review

Authenticate once:

  civitai login                    store your API token
  civitai whoami                   verify your token`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}
	root.SetVersionTemplate("civitai {{.Version}}\n")

	root.AddCommand(newAppCmd())
	root.AddCommand(newLoginCmd())
	root.AddCommand(newWhoAmICmd())

	return root
}
