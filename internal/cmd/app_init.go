package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/huh"
	"github.com/civitai/cli/internal/scaffold"
	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/internal/validate"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newAppInitCmd() *cobra.Command {
	var templateFlag string
	var fromSlug string
	var dirFlag string
	var nameFlag string
	var slugFlag string
	var noInput bool

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

The blockId is your app's PERMANENT public identity — the hostname your app will
be served at once it is approved, and the argument every later command takes — so
derivation refuses rather than guesses when the name carries LETTERS a blockId
cannot hold ("Café Del Mar", "ÜberApp", any non-Latin name). Punctuation, symbols
and emoji still fold to a hyphen, as they always have ("Rocket 🚀 App" ->
rocket-app). Pass --slug <slug> to choose the blockId yourself; it bypasses
derivation entirely.

By default the project is created in ./<slug>. Override the output directory with
a positional [dir] or --dir <path>; override the display name independently with
--name (so name, slug, and directory can all differ).`,
		Example: `  # A no-build static app in ./my-block.
  civitai app init my-block

  # A page-money app; "My Cool Block" -> slug my-cool-block, dir ./my-cool-block.
  civitai app init "My Cool Block" --template page-money

  # Custom output directory (slug stays my-block; created in ./apps/foo).
  civitai app init my-block --dir ./apps/foo

  # A name derivation cannot slugify: choose the blockId yourself.
  civitai app init "Café Del Mar" --slug cafe-del-mar

  # Name, slug, and dir all independent.
  civitai app init my-block ./apps/foo --name "My Block"`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAppScaffold(cmd, args, templateFlag, fromSlug, dirFlag, nameFlag, slugFlag, noInput)
		},
	}

	cmd.Flags().StringVarP(&templateFlag, "template", "t", string(scaffold.Static), "project template: static | page-vite | page-money")
	cmd.Flags().StringVar(&fromSlug, "from", "", fromFlagUsage)
	cmd.Flags().StringVar(&dirFlag, "dir", "", "output directory (default ./<slug>)")
	cmd.Flags().StringVar(&nameFlag, "name", "", "display name (default derived from the name argument)")
	cmd.Flags().StringVar(&slugFlag, "slug", "", slugFlagUsage)
	cmd.Flags().BoolVarP(&noInput, "yes", "y", false, "non-interactive: never prompt (use flags/defaults; fail if a name is missing)")
	return cmd
}

// slugFlagUsage / fromFlagUsage are shared by `app init` and `app create` so the
// two commands cannot drift on the same flag.
const (
	slugFlagUsage = "explicit blockId (bypasses derivation from the name; 3-40 chars, starts with a letter, lowercase a-z/0-9/hyphens)"
	// The flag stays discoverable — it is a real roadmap item and hiding it
	// would only move the surprise to a user who read about it elsewhere — but
	// the usage string says up front that it cannot work yet, so the failure is
	// predictable BEFORE the command is run rather than after.
	fromFlagUsage = "fork from an existing published app slug (NOT AVAILABLE YET — the CLI cannot fetch app source)"
)

// runAppScaffold is the shared scaffold body behind both `app init` and
// `app create`. The two commands differ only in the default template; all of
// the slug/display derivation, dir resolution, rendering, self-validation, and
// next-steps output lives here so there is a single code path.
func runAppScaffold(cmd *cobra.Command, args []string, templateFlag, fromSlug, dirFlag, nameFlag, slugFlag string, noInput bool) error {
	out := cmd.OutOrStdout()

	// TODO(server): expose a read endpoint that returns a published app's
	// canonical source tree by slug, then --from can scaffold from it. Until
	// then the flag has a 100% failure rate, so the message a user sees is a
	// plain actionable CLI error — the engineering context stays HERE. An
	// end-user error is not the place to ship an internal ticket.
	if fromSlug != "" {
		return fmt.Errorf("--from is not available yet: this CLI cannot fetch a published app's source. Scaffold a plain project with `%s <name>` and copy the upstream files in by hand", cmd.CommandPath())
	}

	// An explicit --slug bypasses derivation entirely, so it is checked against
	// the same server contract a derived slug is. A bad VALUE is a usage error
	// (exit 2), same class as a bad --template.
	if slugFlag != "" {
		if err := scaffold.ValidateSlug(slugFlag); err != nil {
			return asUsageError(err)
		}
	}

	name := ""
	if len(args) >= 1 {
		name = args[0]
	}

	// Interactive prompts (huh) ONLY when a name wasn't supplied AND we're on an
	// interactive terminal AND --yes wasn't passed. A supplied name, a piped/CI
	// stdin (not a TTY), or --yes all SKIP huh — so scripted invocations never
	// block on a prompt (huh requires a TTY). The prompt fills in the name +
	// template; everything downstream is identical to the flag-driven path.
	//
	// 🔴 --slug SUPPRESSES THE NAME FIELD, NOT THE PROMPT. An earlier version
	// added `&& slugFlag == ""` to this condition, reasoning that --slug
	// "supplies the one thing the prompt exists to collect". It does not: the
	// form collects a name AND a TEMPLATE, so `civitai app create --slug my-app`
	// on a TTY silently took page-money with no template choice offered — a
	// question the user was asked before and is no longer asked. The identity is
	// the only thing --slug settles, so it drops that one field and the template
	// select still runs; the display name then falls back to the slug's title
	// case. --yes remains the way to skip the prompt entirely.
	if name == "" && !noInput && stdinIsTTY() {
		inputs, err := scaffoldPromptFn(cmd, templateFlag, slugFlag == "")
		if err != nil {
			return err
		}
		name = strings.TrimSpace(inputs.name)
		if inputs.template != "" {
			templateFlag = inputs.template
		}
	}

	// The display name is written VERBATIM into block.manifest.json. JSON is
	// UTF-8 by definition, so an invalid-UTF-8 name produces a manifest no
	// conforming parser can read — and the platform is the one that finds out.
	// scaffold.Slugify refuses the same bytes for the blockId (see its (a)
	// residual); this is the other half, and it is needed because --slug
	// bypasses derivation entirely, so the name reaches the manifest unchecked.
	// A bad flag/arg VALUE is a usage error (exit 2), same class as --template.
	for _, v := range []struct{ what, val string }{{"the name argument", name}, {"--name", nameFlag}} {
		if v.val != "" && !utf8.ValidString(v.val) {
			return asUsageError(fmt.Errorf("%s is not valid UTF-8: %q — the display name is written into block.manifest.json verbatim, and JSON must be UTF-8", v.what, v.val))
		}
	}

	// A bad --template VALUE is a usage error (exit 2), exactly like a bad flag
	// NAME. scaffold cannot tag it itself — ErrUsage lives here, and internal/cmd
	// already imports internal/scaffold — so the tag is attached at the call site.
	// asUsageError preserves the message byte-for-byte.
	tmpl, err := scaffold.ParseTemplate(templateFlag)
	if err != nil {
		return asUsageError(err)
	}

	if name == "" && slugFlag == "" {
		return asUsageError(fmt.Errorf("provide a project name: %s <name>", cmd.CommandPath()))
	}

	// Positional [dir] (args[1]) and --dir both set the output directory;
	// they must not conflict.
	posDir := ""
	if len(args) == 2 {
		posDir = args[1]
	}
	if posDir != "" && dirFlag != "" && posDir != dirFlag {
		return asUsageError(fmt.Errorf("conflicting output directory: positional %q and --dir %q", posDir, dirFlag))
	}
	targetDir := dirFlag
	if targetDir == "" {
		targetDir = posDir
	}

	// Derive slug + display name. An explicit --slug wins outright; otherwise a
	// name that is already a valid slug is used verbatim, and anything else is
	// slugified (which REFUSES rather than dropping characters — see
	// scaffold.LossyChars, and the residuals in scaffold.Slugify's header:
	// derivation still folds symbols/emoji to a hyphen and still lowers the two
	// runes above ASCII that lower INTO ASCII).
	nameIsSlug := name != "" && scaffold.ValidateSlug(name) == nil
	var slug, display string
	switch {
	case slugFlag != "":
		slug = slugFlag
	case nameIsSlug:
		slug = name
	default:
		// An unusable <name> argument is a bad VALUE, same class as --template.
		slug, err = scaffold.Slugify(name)
		if err != nil {
			return asUsageError(err)
		}
	}

	// Display name: title-cased from whichever identifier we have when the user
	// typed one that reads as an id, verbatim when they typed prose.
	switch {
	case name == "":
		display = scaffold.TitleFromSlug(slug)
	case nameIsSlug:
		display = scaffold.TitleFromSlug(name)
	default:
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
	//
	// ManifestOnly, not Dir: the project-state checks describe the AUTHOR's
	// working tree, not the template. A scaffold has no lockfile until the
	// author's first `npm install` (next step 1), and reporting that as
	// "internal error: scaffolded manifest failed validation" would be a lie.
	// `civitai app validate` still flags it on the same directory.
	res, verr := validate.ManifestOnly(destDir)
	if verr != nil {
		return verr
	}
	if !res.OK() {
		return fmt.Errorf("internal error: scaffolded manifest failed validation:\n  %s", joinLines(validate.Messages(res.Errors)))
	}

	printScaffoldResult(out, display, slug, tmpl, destDir, abs, written)
	return nil
}

// installStepFmt is next-step 1 for the templates that install. It is shared by
// both npm branches so they cannot drift.
//
// 🔴 THE SECOND LINE IS THE FIX, NOT DECORATION. `civitai app validate` FAILS on
// a freshly-scaffolded page-money / page-vite project until `npm install` has
// written package-lock.json — the platform build installs strictly from the
// committed lockfile, so the check is right and its message is good. What was
// wrong was the PROMISE: `app create --help` said the scaffold "validates
// clean", and validating before installing ~103 MB of node_modules is the
// natural first instinct. Saying it here (the last screen before the author
// starts) and in the help text is what closes that minute-3 surprise. `static`
// ships no package.json, installs nothing, and does validate clean — which is
// why this line lives on the install branches only (issue #260).
const installStepFmt = "  1. cd %s && npm install    # writes package-lock.json — COMMIT it\n" +
	"     (`civitai app validate` fails until you do — it needs that lockfile)\n"

// printScaffoldResult prints a one-line summary (with a file count, not the
// full tree) followed by a numbered, template-tailored "Next steps" sequence.
func printScaffoldResult(out io.Writer, display, slug string, tmpl scaffold.Template, destDir, abs string, written []string) {
	// One scannable line: name, template, where, and how many files — no tree.
	fmt.Fprintln(out, ui.Success(fmt.Sprintf("Created App %q (%s)  ·  %s/  ·  %d files", display, tmpl, destDir, len(written))))

	// 🔴 ECHO THE blockId ALWAYS, not only when it was derived or only when --dir
	// was passed. It is the app's PERMANENT public identity — the hostname it is
	// served at, and the argument `app status` / `app metrics` / `app listing`
	// all take — and `slug` was a DEAD PARAMETER here: no line of this output
	// named it. With the default directory you can infer it from the directory
	// name; with --dir it is invisible unless you open block.manifest.json. So
	// the case where derivation is most likely to surprise you was the case
	// where it was least visible (issue #259).
	//
	// 🔴 THE URL IS A FUTURE TENSE, AND THAT IS NOT A STYLE CHOICE. The first
	// version printed the bare `https://<blockId>.civit.ai/` as the app's
	// "permanent public id" at scaffold time — a URL that is guaranteed to 404
	// at that exact moment, because the subdomain is only programmed on
	// approval + deploy (README: "Before approval, https://<blockId>.civit.ai/
	// 404s"). That is the same false-promise class as the "validates clean"
	// claim two lines up in this very output block, and `app status` already
	// gets it right ("Not live yet — … only serves after the app is approved and
	// deployed"). Keep the two surfaces saying the same thing.
	fmt.Fprintln(out, ui.Dim(fmt.Sprintf("  blockId: %s  —  your app's permanent public id (it cannot be renamed later)", slug)))
	fmt.Fprintln(out, ui.Dim(fmt.Sprintf("  Will be served at https://%s.civit.ai/ — only after the app is approved and deployed · `civitai app status %s`", slug, slug)))

	// page-money ships a runnable txt2img money path and a Comfy on Civitai
	// (customComfy) sample, with body builders for BOTH customComfy arms — the
	// server-registered recipe, and an inline graph the app ships itself. Surface
	// both honestly (each has its own access gate) and point at the README's
	// Comfy on Civitai section rather than the raw file count.
	if tmpl.NeedsHarness() {
		fmt.Fprintln(out, ui.Dim("  Includes a txt2img sample + a Comfy on Civitai (customComfy) sample (invite-only beta), with body builders for BOTH arms: a server-registered recipe and an inline ComfyUI graph your own app ships (app developers) — see the Comfy on Civitai section in README.md."))
	}

	fmt.Fprintln(out, "\n"+ui.Bold("Next steps:"))
	switch {
	case tmpl.NeedsHarness():
		// LEAD with the free, works-today path: `npm run dev:harness` mounts a
		// MOCK host (no Buzz, no beta access, no network) so a newcomer can build
		// and iterate immediately. The real-host surfaces — `civitai app submit`
		// (register) + the DEV TUNNEL (your LOCAL code rendered INSIDE the real
		// Civitai host, prod-fidelity) — are invite-only beta, so they're demoted
		// under a "When you have beta access" heading rather than led with (they'd
		// otherwise walk a first-time author straight into the gated wall).
		//
		// 🔴 SUBMIT IS *NOT* A PREREQUISITE FOR THE TUNNEL, AND THIS BANNER USED TO
		// SAY IT WAS — "(required before a dev tunnel)". It is not: the mint accepts
		// a brand-new slug with NO app row and grants scopes from the LOCAL manifest
		// (see the "UNSUBMITTED app (no submit needed)" path in app_dev_tunnel.go,
		// and the same statement in the generated README's live-mode section, which
		// this line contradicted). What the tunnel actually needs is an Apps-author
		// invite AND the dev-tunnel flag — an unenrolled account gets a 403 at mint
		// time saying exactly that (DevTunnelForbiddenError). Submission is
		// orthogonal. A dogfooding developer opened a working tunnel having never
		// submitted, which is what surfaced this.
		//
		// The two beta surfaces are still listed submit-first, but that is a
		// PRESENTATIONAL grouping (publish path, then preview path) — not a
		// dependency. Do not re-derive an ordering requirement from it.
		fmt.Fprintf(out, installStepFmt, destDir)
		fmt.Fprintln(out, "  2. npm run dev:harness     # mock host, no Buzz — works today")
		fmt.Fprintln(out, "  3. edit src/App.tsx and iterate")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "  When you have beta access (invite-only):")
		fmt.Fprintln(out, "     civitai app submit      # validate + register — when you're ready to publish")
		fmt.Fprintln(out, "     npm run dev:tunnel      # in another terminal: serve your app for the tunnel")
		fmt.Fprintln(out, "     civitai app dev-tunnel  # your LOCAL app INSIDE the real host, prod-fidelity — no submit needed")
	case tmpl == scaffold.PageVite:
		fmt.Fprintf(out, installStepFmt, destDir)
		fmt.Fprintln(out, "  2. npm run dev              # preview locally (your UI only — see below)")
		fmt.Fprintln(out, "  3. civitai app submit       # validate + submit for review")
	default:
		fmt.Fprintf(out, "  1. cd %s              # then open index.html or serve the directory\n", destDir)
		fmt.Fprintln(out, "  2. civitai app submit       # validate + submit for review")
	}

	// The no-host caveat, for the templates that don't ship a harness. A page
	// app only reaches the host's `ready` state by posting BLOCK_READY in
	// response to the host's BLOCK_INIT — and locally there is no host to send
	// one, so the handshake file the scaffold ships stays silent. Say so at the
	// moment the author is about to preview, or its silence reads as a bug and
	// the file looks deletable. (The NeedsHarness branch above already covers
	// this in its own words.)
	//
	// BOTH conjuncts are load-bearing even though they are coextensive across
	// today's three templates (page-money is the only harness template and the
	// only one with no emitter). Neither should be dropped as redundant:
	//   - `ReadyAckPath() != ""` stops the sentence printing with an EMPTY
	//     filename for a template that ships no emitter. Not hypothetical:
	//     forcing this condition true is exactly what produced
	//     "   performs the host handshake — keep it. …" during the audit.
	//   - `!NeedsHarness()` keeps the WORDING honest — "there's no host to send
	//     BLOCK_INIT" is false for a template whose `dev:harness` mounts one.
	// A future template shipping an emitter AND a harness needs both halves
	// reconsidered, not either conjunct deleted.
	if !tmpl.NeedsHarness() && tmpl.ReadyAckPath() != "" {
		fmt.Fprintln(out, "\n"+ui.Dim(fmt.Sprintf("  %s performs the host handshake — keep it. Previewing locally shows your", tmpl.ReadyAckPath())))
		fmt.Fprintln(out, ui.Dim("  UI only: there's no host to send BLOCK_INIT, so it stays quiet by design."))
		fmt.Fprintln(out, ui.Dim("  Without it the real host never reveals the app. See README.md."))
	}

	// The lockfile is load-bearing, and the failure it causes is remote and
	// opaque: the platform build installs STRICTLY from the committed lockfile
	// (no registry re-resolve fallback), so a scaffold that is never `npm
	// install`ed — or one installed with pnpm/yarn while buildCommand still says
	// npm — fails the build with nothing wrong locally. `civitai app validate`
	// catches both, but say it here too, at the moment the choice is made.
	if tmpl.NeedsInstall() {
		fmt.Fprintln(out, "\n"+ui.Dim("  Commit the lockfile. The platform build installs strictly from it (`npm ci`) and"))
		fmt.Fprintln(out, ui.Dim("  will not build without it. If you install with pnpm or yarn instead, set"))
		fmt.Fprintln(out, ui.Dim("  \"buildCommand\" to that package manager (and the \"outputDir\" the schema requires"))
		fmt.Fprintln(out, ui.Dim("  alongside it) and commit THAT lockfile — a mismatch fails the build."))
	}
}

// stdinIsTTY reports whether stdin is an interactive terminal. A package var so
// tests can force it (huh needs a real TTY, so the non-interactive path must be
// exercisable without one).
var stdinIsTTY = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// scaffoldInputs are the values the interactive prompt collects.
type scaffoldInputs struct {
	name     string
	template string
}

// scaffoldPromptFn is the interactive collector. A package var so tests can stub
// it (the real one runs a huh form, which needs a TTY).
var scaffoldPromptFn = runScaffoldForm

// runScaffoldForm runs the huh form that collects a missing app name + template.
// defaultTemplate pre-selects the template (the command's default). The form
// renders to stderr (status stream) and reads the command's stdin.
//
// askName is false when --slug already settled the identity: the TEMPLATE select
// still runs, because that is a second, independent question the prompt exists
// to ask and --slug says nothing about it.
func runScaffoldForm(cmd *cobra.Command, defaultTemplate string, askName bool) (scaffoldInputs, error) {
	in := scaffoldInputs{template: defaultTemplate}
	var fields []huh.Field
	if askName {
		fields = append(fields, huh.NewInput().
			Title("App name").
			Description(`Free-form ("My Cool Block") — slugified for the blockId.`).
			Value(&in.name).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("a name is required")
				}
				return nil
			}))
	}
	fields = append(fields, huh.NewSelect[string]().
		Title("Template").
		Options(
			huh.NewOption("static — no-build page app (index.html + a tiny JS)", string(scaffold.Static)),
			huh.NewOption("page-vite — Vite + React page app", string(scaffold.PageVite)),
			huh.NewOption("page-money — Vite + React + TS SDK money-path app", string(scaffold.PageMoney)),
		).
		Value(&in.template))
	form := huh.NewForm(
		huh.NewGroup(fields...),
	).WithInput(cmd.InOrStdin()).WithOutput(cmd.ErrOrStderr())
	if err := form.Run(); err != nil {
		return in, err
	}
	in.name = strings.TrimSpace(in.name)
	return in, nil
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
