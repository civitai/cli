package cmd

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/civitai/cli/internal/scaffold"
	"github.com/civitai/cli/internal/validate"
	"github.com/spf13/cobra"
)

func newAppInitCmd() *cobra.Command {
	var templateFlag string
	var fromSlug string
	var dirFlag string
	var nameFlag string

	cmd := &cobra.Command{
		Use:   "init [name] [dir]",
		Short: "Scaffold a ready-to-build App project",
		Long: `Scaffold a correct, ready-to-build App project.

Templates:
  static      a no-build page app (index.html + a tiny JS, no build step)
  page-vite   a vite + React page app (config-as-code build: buildCommand + outputDir)
  page-money  a vite + React + TS full-page (W10) money-path app wired to the
              published App SDK (estimate -> consent -> submit -> poll -> Buzz spend)

The display name can be free-form ("My Cool Block"); it is slugified for the
blockId. A slug-shaped name is used verbatim.

By default the project is created in ./<slug>. Override the output directory with
a positional [dir] or --dir <path>; override the display name independently with
--name (so name, slug, and directory can all differ).`,
		Example: `  # A no-build static app in ./my-block.
  civitai app init my-block

  # A page-money app; "My Cool Block" -> slug my-cool-block, dir ./my-cool-block.
  civitai app init "My Cool Block" --template page-money

  # Custom output directory (slug stays my-block; created in ./apps/foo).
  civitai app init my-block --dir ./apps/foo

  # Name, slug, and dir all independent.
  civitai app init my-block ./apps/foo --name "My Block"`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAppScaffold(cmd, args, templateFlag, fromSlug, dirFlag, nameFlag)
		},
	}

	cmd.Flags().StringVarP(&templateFlag, "template", "t", string(scaffold.Static), "project template: static | page-vite | page-money")
	cmd.Flags().StringVar(&fromSlug, "from", "", "fork from an existing published app slug (not yet wired)")
	cmd.Flags().StringVar(&dirFlag, "dir", "", "output directory (default ./<slug>)")
	cmd.Flags().StringVar(&nameFlag, "name", "", "display name (default derived from the name argument)")
	return cmd
}

// runAppScaffold is the shared scaffold body behind both `app init` and
// `app create`. The two commands differ only in the default template; all of
// the slug/display derivation, dir resolution, rendering, self-validation, and
// next-steps output lives here so there is a single code path.
func runAppScaffold(cmd *cobra.Command, args []string, templateFlag, fromSlug, dirFlag, nameFlag string) error {
	out := cmd.OutOrStdout()

	if fromSlug != "" {
		return fmt.Errorf(`--from is not yet wired up.

Forking an existing published app requires fetching its source from the
server, which this CLI cannot do yet (no programmatic app-source endpoint).

TODO(server): expose a read endpoint that returns a published app's canonical
source tree by slug, then --from can scaffold from it. For now, run a plain
init and copy the upstream files in manually.`)
	}

	tmpl, err := scaffold.ParseTemplate(templateFlag)
	if err != nil {
		return err
	}

	name := ""
	if len(args) >= 1 {
		name = args[0]
	}
	if name == "" {
		return fmt.Errorf("provide a project name: %s <name>", cmd.CommandPath())
	}

	// Positional [dir] (args[1]) and --dir both set the output directory;
	// they must not conflict.
	posDir := ""
	if len(args) == 2 {
		posDir = args[1]
	}
	if posDir != "" && dirFlag != "" && posDir != dirFlag {
		return fmt.Errorf("conflicting output directory: positional %q and --dir %q", posDir, dirFlag)
	}
	targetDir := dirFlag
	if targetDir == "" {
		targetDir = posDir
	}

	// Derive slug + display name. If the name is already a valid slug
	// use it verbatim; otherwise slugify and title-case for display.
	var slug, display string
	if err := scaffold.ValidateSlug(name); err == nil {
		slug = name
		display = scaffold.TitleFromSlug(name)
	} else {
		slug, err = scaffold.Slugify(name)
		if err != nil {
			return err
		}
		display = name
	}

	// --name overrides the display name independently of the slug/dir,
	// so name, slug, and directory can all differ.
	if nameFlag != "" {
		display = nameFlag
	}

	// Output directory: --dir / positional [dir] if given, else ./<slug>
	// (back-compat: no flag -> current behaviour).
	destDir := slug
	if targetDir != "" {
		destDir = targetDir
	}
	abs, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}

	written, err := scaffold.Render(tmpl, destDir, scaffold.Data{
		Slug: slug,
		Name: display,
	})
	if err != nil {
		return err
	}

	// Sanity-check the scaffold we just produced is schema-valid. If it
	// isn't, that's a bug in our own templates — fail loudly.
	res, verr := validate.Dir(destDir)
	if verr != nil {
		return verr
	}
	if !res.OK() {
		return fmt.Errorf("internal error: scaffolded manifest failed validation:\n  %s", joinLines(res.Errors))
	}

	printScaffoldResult(out, display, slug, tmpl, destDir, abs, written)
	return nil
}

// printScaffoldResult prints a one-line summary (with a file count, not the
// full tree) followed by a numbered, template-tailored "Next steps" sequence.
func printScaffoldResult(out io.Writer, display, slug string, tmpl scaffold.Template, destDir, abs string, written []string) {
	// One scannable line: name, template, where, and how many files — no tree.
	fmt.Fprintf(out, "✓ Created App %q (%s)  ·  %s/  ·  %d files\n", display, tmpl, destDir, len(written))

	fmt.Fprintln(out, "\nNext steps:")
	switch {
	case tmpl.NeedsHarness():
		// SDK/page-money apps render blank under plain `dev` (no host) — the
		// dev loop needs the mock host via `dev:harness`. Keep the mock-vs-live
		// distinction as a clearly-separate tip so a placeholder "Generate"
		// result isn't mistaken for a broken real generation.
		fmt.Fprintf(out, "  1. cd %s && npm install\n", destDir)
		fmt.Fprintln(out, "  2. npm run dev:harness      # preview locally — MOCK host, no real Buzz")
		fmt.Fprintln(out, "  3. civitai app submit       # validate + submit for review")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "  Real generation (spends real Buzz)? npm run dev:live needs a full-scope personal API")
		fmt.Fprintln(out, "  key: create at https://civitai.com/user/account, then `civitai login --token <key>`.")
		fmt.Fprintln(out, "  An OAuth `civitai login` can submit/withdraw but cannot spend Buzz (check `civitai whoami`).")
	case tmpl == scaffold.PageVite:
		fmt.Fprintf(out, "  1. cd %s && npm install\n", destDir)
		fmt.Fprintln(out, "  2. npm run dev              # preview locally")
		fmt.Fprintln(out, "  3. civitai app submit       # validate + submit for review")
	default:
		fmt.Fprintf(out, "  1. cd %s              # then open index.html or serve the directory\n", destDir)
		fmt.Fprintln(out, "  2. civitai app submit       # validate + submit for review")
	}
}

func joinLines(lines []string) string {
	s := ""
	for i, l := range lines {
		if i > 0 {
			s += "\n  "
		}
		s += l
	}
	return s
}
