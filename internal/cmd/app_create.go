package cmd

import (
	"github.com/civitai/cli/internal/scaffold"
	"github.com/spf13/cobra"
)

// newAppCreateCmd is the batteries-included, friendly scaffolder. It is a thin
// superset of `app init` — same flags, same slug/display derivation, same
// self-validation and next-steps output — that defaults --template to the rich
// `page-money` SDK template (vs init's no-build `static` default).
func newAppCreateCmd() *cobra.Command {
	var templateFlag string
	var fromSlug string
	var dirFlag string
	var nameFlag string
	var noInput bool

	cmd := &cobra.Command{
		Use:   "create [name] [dir]",
		Short: "Create a ready-to-build App (batteries-included, SDK money-path)",
		Long: `Create a ready-to-build App, batteries included.

This is the friendly happy path: a thin superset of "civitai app init" that
defaults to the rich page-money template — a Vite + React + TypeScript full-page
app wired to the published App SDK (estimate -> consent -> submit -> poll ->
Buzz spend), with a mock-host dev harness and a unit test. The scaffold is
immediately runnable (npm install && npm run dev:harness), test-green, and
validates clean.

The default scaffold ships a runnable txt2img money path AND a Comfy on Civitai
(customComfy) sample that runs a server-registered recipe (invite-only beta) —
both share the estimate -> consent -> submit -> poll driver, switched by an
on-screen mode toggle, and both work end-to-end in "npm run dev:harness".

customComfy has TWO arms. Besides the recipe arm above, an app may also ship
its own ComfyUI graph inline (mode: 'inline', app developers only). src/comfy.ts
includes a complete, unit-tested buildInlineComfyBody for it: the graph, the
declared AIR resource manifest, and the maxBuzz ceiling (which is ALSO the step
timeout in seconds). See the generated README's "Comfy on Civitai samples" section.

Templates (override with --template):
  static      a no-build page app (index.html + a tiny JS, no build step)
  page-vite   a vite + React page app (config-as-code build: buildCommand + outputDir)
  page-money  a vite + React + TS full-page (W10) money-path app wired to the
              published App SDK (estimate -> consent -> submit -> poll -> Buzz
              spend); includes a txt2img + a Comfy on Civitai (customComfy) sample,
              recipe and inline-graph body builders [default for create]

The display name can be free-form ("My Cool Block"); it is slugified for the
blockId. A slug-shaped name is used verbatim.

By default the project is created in ./<slug>. Override the output directory with
a positional [dir] or --dir <path>; override the display name independently with
--name (so name, slug, and directory can all differ).

Note: a DEFAULT ` + "`civitai login`" + ` (OAuth) grants submit but NOT Buzz-spend. To
run ` + "`dev:live`" + ` real generations, authenticate with a credential that carries
the AI Services scopes: ` + spendCredentialRoutes + `.`,
		Example: `  # A page-money app in ./my-block (the batteries-included default).
  civitai app create my-block

  # "My Cool Block" -> slug my-cool-block, dir ./my-cool-block.
  civitai app create "My Cool Block"

  # Same as init: a no-build static app.
  civitai app create my-block --template static

  # Custom output directory (slug stays my-block; created in ./apps/foo).
  civitai app create my-block --dir ./apps/foo`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAppScaffold(cmd, args, templateFlag, fromSlug, dirFlag, nameFlag, noInput)
		},
	}

	// create defaults to the batteries-included page-money template; every
	// other flag matches init exactly.
	cmd.Flags().StringVarP(&templateFlag, "template", "t", string(scaffold.PageMoney), "project template: static | page-vite | page-money")
	cmd.Flags().StringVar(&fromSlug, "from", "", "fork from an existing published app slug (not yet wired)")
	cmd.Flags().StringVar(&dirFlag, "dir", "", "output directory (default ./<slug>)")
	cmd.Flags().StringVar(&nameFlag, "name", "", "display name (default derived from the name argument)")
	cmd.Flags().BoolVarP(&noInput, "yes", "y", false, "non-interactive: never prompt (use flags/defaults; fail if a name is missing)")
	return cmd
}
