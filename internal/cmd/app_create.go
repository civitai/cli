package cmd

import (
	"github.com/civitai/cli/internal/scaffold"
	"github.com/spf13/cobra"
)

// newAppCreateCmd is the friendly scaffolder. It shares `app init`'s RunE —
// same flags, same slug/display derivation, same self-validation and next-steps
// output — and, since web components became the default, the same default
// template too (scaffold.DefaultTemplate, `page-elements`). It used to default
// to `page-money` while init defaulted to `static`.
func newAppCreateCmd() *cobra.Command {
	var templateFlag string
	var fromSlug string
	var dirFlag string
	var nameFlag string
	var slugFlag string
	var noInput bool

	cmd := &cobra.Command{
		Use:   "create [name] [dir]",
		Short: "Create a ready-to-build App (web components by default)",
		Long: `Create a ready-to-build App.

This is the friendly happy path, the same scaffolder as "civitai app init". It
defaults to the page-elements template — a Vite + TypeScript full-page app
built with web components and no UI framework: @civitai/sdk for the host bridge
and <civitai-*> elements from @civitai/components (themed by @civitai/theme),
with a mock-host dev harness and a test. The scaffold is immediately runnable
(npm install && npm run dev:harness) and test-green. "civitai app validate"
passes once you have run "npm install" — until then it correctly reports the
package-lock.json the platform build installs from.

Prefer React? --template page-money is the React alternative, and the one with
a runnable money path: a txt2img sample AND a Comfy on Civitai (customComfy)
sample that runs a server-registered recipe (invite-only beta) — both share the
estimate -> consent -> submit -> poll driver, switched by an on-screen mode
toggle, and both work end-to-end in "npm run dev:harness".

customComfy has TWO arms. Besides the recipe arm above, an app may also ship
its own ComfyUI graph inline (mode: 'inline', page tokens only). src/comfy.ts
includes a complete, unit-tested buildInlineComfyBody for it: the graph, the
declared AIR resource manifest, and the maxBuzz ceiling (which is ALSO the step
timeout in seconds). See the generated README's "Comfy on Civitai samples" section.

Templates (override with --template):
  page-elements  [default] a vite + TS full-page app built with web components
                 and no UI framework (@civitai/sdk + @civitai/components)
  page-money     the React alternative: a vite + React + TS full-page (W10)
                 money-path app wired to the published App SDK (estimate ->
                 consent -> submit -> poll -> Buzz spend); includes a txt2img +
                 a Comfy on Civitai (customComfy) sample, recipe and inline-graph
                 body builders
  page-vite      a vite + React page app with no SDK (config-as-code build:
                 buildCommand + outputDir)
  static         a no-build page app (index.html + a tiny JS, no build step)

The display name can be free-form ("My Cool Block"); it is slugified for the
blockId. A slug-shaped name is used verbatim.

The blockId is your app's PERMANENT public identity — the hostname your app will
be served at once it is approved, and the argument every later command takes — so
derivation refuses rather than guesses when the name carries LETTERS a blockId
cannot hold ("Café Del Mar", "ÜberApp", any non-Latin name). Punctuation, symbols
and emoji still fold to a hyphen, as they always have ("Rocket 🚀 App" ->
rocket-app). Pass --slug <slug> to choose the blockId yourself; it bypasses
derivation entirely.

By default the project is created in ./<slug>. Override the output directory with
a positional [dir] or --dir <path>; override the display name independently with
--name (so name, slug, and directory can all differ).

Note (page-money): a DEFAULT ` + "`civitai login`" + ` (OAuth) grants submit but NOT
Buzz-spend. To run ` + "`dev:live`" + ` real generations, authenticate with a credential that carries
the AI Services scopes: ` + spendCredentialRoutes + `.`,
		Example: `  # A web-components app (the page-elements default) in ./my-block.
  civitai app create my-block

  # "My Cool Block" -> slug my-cool-block, dir ./my-cool-block.
  civitai app create "My Cool Block"

  # The React alternative, with the money path.
  civitai app create my-block --template page-money

  # A no-build static app.
  civitai app create my-block --template static

  # Custom output directory (slug stays my-block; created in ./apps/foo).
  civitai app create my-block --dir ./apps/foo

  # A name derivation cannot slugify: choose the blockId yourself.
  civitai app create "Café Del Mar" --slug cafe-del-mar`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAppScaffold(cmd, args, templateFlag, fromSlug, dirFlag, nameFlag, slugFlag, noInput)
		},
	}

	// Every flag matches init exactly, the default template included.
	cmd.Flags().StringVarP(&templateFlag, "template", "t", string(scaffold.DefaultTemplate), templateFlagUsage)
	cmd.Flags().StringVar(&fromSlug, "from", "", fromFlagUsage)
	cmd.Flags().StringVar(&dirFlag, "dir", "", "output directory (default ./<slug>)")
	cmd.Flags().StringVar(&nameFlag, "name", "", "display name (default derived from the name argument)")
	cmd.Flags().StringVar(&slugFlag, "slug", "", slugFlagUsage)
	cmd.Flags().BoolVarP(&noInput, "yes", "y", false, "non-interactive: never prompt (use flags/defaults; fail if a name is missing)")
	return cmd
}
