package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/civitai/cli/internal/scaffold"
	"github.com/civitai/cli/internal/validate"
	"github.com/spf13/cobra"
)

func newAppInitCmd() *cobra.Command {
	var templateFlag string
	var fromSlug string

	cmd := &cobra.Command{
		Use:   "init [name]",
		Short: "Scaffold a ready-to-build App Block project",
		Long: `Scaffold a correct, ready-to-build App Block project.

Templates:
  static     a no-build page block (index.html + a tiny JS, no build step)
  page-vite  a vite + React page block (config-as-code build: buildCommand + outputDir)

Examples:
  civitai app init my-block
  civitai app init "My Cool Block" --template page-vite
  civitai app init forked --from some-published-slug`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if fromSlug != "" {
				return fmt.Errorf(`--from is not yet wired up.

Forking an existing published block requires fetching its source from the
server, which this CLI cannot do yet (no programmatic block-source endpoint).

TODO(server): expose a read endpoint that returns a published block's canonical
source tree by slug, then --from can scaffold from it. For now, run a plain
init and copy the upstream files in manually.`)
			}

			tmpl, err := scaffold.ParseTemplate(templateFlag)
			if err != nil {
				return err
			}

			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			if name == "" {
				return fmt.Errorf("provide a project name: civitai app init <name>")
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

			destDir := slug
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

			fmt.Fprintf(out, "Created App Block %q (slug: %s, template: %s)\n", display, slug, tmpl)
			fmt.Fprintf(out, "  %s\n", abs)
			for _, w := range written {
				// `written` paths are relative to destDir's parent; show them
				// relative to the project dir for a clean tree.
				rel, err := filepath.Rel(destDir, w)
				if err != nil {
					rel = w
				}
				fmt.Fprintf(out, "    %s\n", rel)
			}
			fmt.Fprintln(out, "\nNext steps:")
			if tmpl == scaffold.PageVite {
				fmt.Fprintf(out, "  cd %s && npm install && npm run dev\n", slug)
			} else {
				fmt.Fprintf(out, "  cd %s   # open index.html or serve the directory\n", slug)
			}
			fmt.Fprintln(out, "  civitai app validate")
			fmt.Fprintln(out, "  civitai app submit")
			return nil
		},
	}

	cmd.Flags().StringVarP(&templateFlag, "template", "t", string(scaffold.Static), "project template: static | page-vite")
	cmd.Flags().StringVar(&fromSlug, "from", "", "fork from an existing published block slug (not yet wired)")
	return cmd
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
