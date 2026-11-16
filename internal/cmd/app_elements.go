package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/civitai/cli/internal/ui"
	"github.com/spf13/cobra"
)

// newAppElementsCmd is `civitai app elements [tag]`: one `<civitai-*>` element's
// reference, read from the @civitai/components the project has INSTALLED.
//
// It exists because prose did not work. The page-elements AGENTS.md pointed
// coding agents at `custom-elements.json` and asked them to look up the one
// element they needed rather than read the file whole; evaluation runs still
// measured agents writing several ad-hoc `node -e` parsers per run to do that
// lookup. And in 0.10.0 and earlier the shared form-field attributes are not in
// the file at all; 0.10.1 flattens them onto each form control (see
// app_elements_cem.go, which also says how the older shape is reported). A
// command answers the question in one call.
//
// It lives under `app` because it is an authoring aid scoped to an App project,
// like `app validate`: it reads the project's own node_modules, and has no
// meaning outside one. It never touches the network.
func newAppElementsCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "elements [tag]",
		Short: "Look up a <civitai-*> element's attributes, events and slots",
		Long: `Print the reference for the <civitai-*> custom elements, read from the
@civitai/components package installed in this project.

With no tag, list every element with its one-line description. With a tag
("civitai-button" or "<civitai-button>"), print that element's attributes,
properties, events, slots and CSS parts — INCLUDING the members it inherits,
resolved from the package's custom-elements.json alone. A version whose
manifest does not describe an element's base class (0.10.0 and earlier omit the
shared form-field base, so label, name, value, … would be missing) prints the
reference with an "Incomplete" warning naming the class; upgrade
@civitai/components to fix it.

The package is found the way Node resolves an import: the nearest
node_modules/@civitai/components walking up from the current directory, so the
answer matches the version this project actually runs. Nothing is fetched from
the network; install the project's dependencies first.

An unknown tag exits 2 and names the closest matches. No installed package
exits 1.`,
		Example: `  civitai app elements                     # every element, one line each
  civitai app elements civitai-text-input  # one element, inherited attributes included
  civitai app elements '<civitai-button>'
  civitai app elements civitai-modal --json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			pkg, err := findElementsPackage(wd)
			if err != nil {
				if errors.Is(err, errElementsManifestNotFound) {
					return fmt.Errorf("no %s custom-elements.json found in node_modules from %s upward — "+
						"install this project's dependencies first (`npm install`), or add the package with "+
						"`npm install %s`", elementsPackageName, wd, elementsPackageName)
				}
				return err
			}
			out := cmd.OutOrStdout()
			if len(args) == 0 {
				return printElementsList(out, pkg, jsonOut)
			}
			e, err := pkg.lookupElement(args[0])
			if err != nil {
				return err
			}
			ref := pkg.resolveElement(e)
			if jsonOut {
				return writeJSON(out, map[string]any{"package": elementsPackageJSON(pkg), "element": ref})
			}
			printElementReference(out, elementsPackageHeader(pkg), terminalSafeReference(ref))
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the list or the element reference as JSON (scriptable)")
	return cmd
}

func elementsPackageJSON(p *elementsPackage) map[string]string {
	return map[string]string{"name": p.Name, "version": p.Version, "manifest": p.ManifestPath}
}

// normaliseTag accepts `civitai-button`, `<civitai-button>`, `</civitai-button>`
// and `<civitai-button/>`, in any case.
func normaliseTag(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimPrefix(s, "/")
	s = strings.TrimSuffix(s, ">")
	s = strings.TrimSuffix(s, "/")
	return strings.TrimSpace(s)
}

// lookupElement finds one element by tag. A bare `button` is accepted as
// `civitai-button` when that element exists.
func (p *elementsPackage) lookupElement(arg string) (elementDecl, error) {
	tag := normaliseTag(arg)
	all := p.elements()
	for _, try := range []string{tag, "civitai-" + tag} {
		for _, e := range all {
			if e.Decl.TagName == try {
				return e, nil
			}
		}
	}
	// The tag is what the user typed and is echoed as typed; the package name,
	// version and near-match tags are the PACKAGE's bytes, and main prints this
	// error raw, so those are gated (elementsPackageHeader, nearTags).
	msg := fmt.Sprintf("no element <%s> in %s", tag, elementsPackageHeader(p))
	if near := nearTags(tag, all); len(near) > 0 {
		msg += " — did you mean: " + strings.Join(near, ", ") + "?"
	}
	msg += " Run `civitai app elements` to list every element."
	return elementDecl{}, asUsageError(errors.New(msg))
}

// nearTags returns up to five tags close to the one asked for — those sharing a
// substring with it or within a small edit distance — nearest first.
func nearTags(tag string, all []elementDecl) []string {
	bare := strings.TrimPrefix(tag, "civitai-")
	type cand struct {
		tag  string
		dist int
	}
	var cands []cand
	for _, e := range all {
		t := e.Decl.TagName
		tb := strings.TrimPrefix(t, "civitai-")
		d := editDistance(bare, tb)
		related := bare != "" && (strings.Contains(tb, bare) || strings.Contains(bare, tb))
		if related || d <= 3 {
			cands = append(cands, cand{t, d})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].dist != cands[j].dist {
			return cands[i].dist < cands[j].dist
		}
		return cands[i].tag < cands[j].tag
	})
	var out []string
	for i := 0; i < len(cands) && i < 5; i++ {
		out = append(out, "<"+safeTermSingle(cands[i].tag)+">") // package bytes, into an error printed raw
	}
	return out
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

func printElementsList(w io.Writer, p *elementsPackage, jsonOut bool) error {
	all := p.elements()
	if jsonOut {
		type row struct {
			Tag         string `json:"tag"`
			Class       string `json:"class"`
			Description string `json:"description"`
		}
		rows := make([]row, 0, len(all))
		for _, e := range all {
			rows = append(rows, row{e.Decl.TagName, e.Decl.Name, firstSentence(firstNonEmpty(e.Decl.Summary, e.Decl.Description))})
		}
		return writeJSON(w, map[string]any{"package": elementsPackageJSON(p), "elements": rows})
	}
	type listRow struct{ tag, desc string }
	rows := make([]listRow, 0, len(all))
	width := 0
	for _, e := range all {
		// The package's text reaches the terminal here; gate it once, per cell.
		r := listRow{
			tag:  safeTermSingle(e.Decl.TagName),
			desc: safeTermSingle(firstSentence(firstNonEmpty(e.Decl.Summary, e.Decl.Description))),
		}
		width = max(width, len(r.tag))
		rows = append(rows, r)
	}
	fmt.Fprintf(w, "%s — %d elements\n", elementsPackageHeader(p), len(all))
	for _, r := range rows {
		fmt.Fprintln(w, strings.TrimRight(fmt.Sprintf("  %-*s  %s", width, r.tag, r.desc), " "))
	}
	fmt.Fprintln(w, "\nOne element's attributes, events and slots: civitai app elements <tag>")
	return nil
}

// firstSentence is a description's first sentence, on one line, capped — the
// list's one-line summary. Manifest descriptions are hard-wrapped prose, so
// the first LINE usually ends mid-sentence; the first sentence does not.
func firstSentence(s string) string {
	s = oneLine(s)
	if i := strings.Index(s, ". "); i >= 0 {
		s = s[:i+1]
	}
	return s
}

// oneLine joins a description onto one line and caps it. A member's
// description is kept whole up to the cap: its second sentence is often the
// one that matters ("Current value. The value ATTRIBUTE is the default").
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	const limit = 200
	if r := []rune(s); len(r) > limit {
		s = string(r[:limit-1]) + "…"
	}
	return s
}

// elementsPackageHeader is the `<name> <version>` line, from the package's own
// package.json and therefore gated like the rest of its text.
func elementsPackageHeader(p *elementsPackage) string {
	return safeTermSingle(p.Name) + " " + safeTermSingle(p.Version)
}

// terminalSafeReference returns a copy of r with every string that came out of
// the installed package passed through safeTermSingle — the ONE place the
// human renderer's text is gated. The package is third-party bytes from the
// npm registry: a description carrying an escape sequence must not drive the
// terminal of the agent or person reading it. `--json` does not come through
// here; the encoder escapes control characters itself and a script wants the
// manifest's text unaltered.
func terminalSafeReference(r elementReference) elementReference {
	out := r
	out.Tag = safeTermSingle(r.Tag)
	out.Class = safeTermSingle(r.Class)
	out.Description = safeTerm(r.Description)
	out.Extends = make([]string, len(r.Extends))
	for i := range r.Extends {
		out.Extends[i] = safeTermSingle(r.Extends[i])
	}
	out.Unresolved = make([]string, len(r.Unresolved))
	for i := range r.Unresolved {
		out.Unresolved[i] = safeTermSingle(r.Unresolved[i])
	}
	fields := func(in []elementField) []elementField {
		res := make([]elementField, len(in))
		for i, f := range in {
			res[i] = elementField{
				Name: safeTermSingle(f.Name), Type: safeTermSingle(f.Type), Default: safeTermSingle(f.Default),
				Description: safeTermSingle(f.Description), DeclaredBy: safeTermSingle(f.DeclaredBy),
			}
		}
		return res
	}
	named := func(in []elementNamed) []elementNamed {
		res := make([]elementNamed, len(in))
		for i, n := range in {
			res[i] = elementNamed{
				Name: safeTermSingle(n.Name), Type: safeTermSingle(n.Type), Default: safeTermSingle(n.Default),
				Description: safeTermSingle(n.Description), DeclaredBy: safeTermSingle(n.DeclaredBy),
			}
		}
		return res
	}
	out.Attributes, out.Properties = fields(r.Attributes), fields(r.Properties)
	out.Events, out.Slots = named(r.Events), named(r.Slots)
	out.CSSParts, out.CSSProperties = named(r.CSSParts), named(r.CSSProperties)
	return out
}

// printElementReference renders a reference that has ALREADY been through
// terminalSafeReference; it adds only this CLI's own labels.
func printElementReference(w io.Writer, header string, r elementReference) {
	chain := append([]string{r.Class}, r.Extends...)
	fmt.Fprintf(w, "%s  %s\n", ui.Bold("<"+r.Tag+">"), strings.Join(chain, " → "))
	fmt.Fprintln(w, header)
	if r.Description != "" {
		fmt.Fprintf(w, "\n%s\n", r.Description)
	}
	printFieldSection(w, "Attributes", r.Class, r.Attributes)
	printFieldSection(w, "Properties (no attribute)", r.Class, r.Properties)
	printNamedSection(w, "Events", r.Class, r.Events)
	printNamedSection(w, "Slots", r.Class, r.Slots)
	printNamedSection(w, "CSS parts", r.Class, r.CSSParts)
	printNamedSection(w, "CSS custom properties", r.Class, r.CSSProperties)
	if len(r.Unresolved) > 0 {
		fmt.Fprintf(w, "\n%s\n", ui.Warn("Incomplete: this package's custom-elements.json does not describe "+
			strings.Join(r.Unresolved, ", ")+", so members inherited from it are not listed above. "+
			"Fix: upgrade @civitai/components."))
	}
}

// fromNote marks a member declared by an ancestor rather than the element.
func fromNote(declaredBy, own string) string {
	if declaredBy == "" || declaredBy == own {
		return ""
	}
	return ui.Dim("  (" + declaredBy + ")")
}

func printFieldSection(w io.Writer, title, own string, fields []elementField) {
	if len(fields) == 0 {
		return
	}
	fmt.Fprintf(w, "\n%s\n", ui.Bold(title+":"))
	nameW, typeW := 0, 0
	for _, f := range fields {
		nameW = max(nameW, len(f.Name))
		typeW = max(typeW, len(f.Type))
	}
	for _, f := range fields {
		line := fmt.Sprintf("  %-*s  %-*s", nameW, f.Name, typeW, f.Type)
		if f.Default != "" {
			line += "  = " + f.Default
		}
		if f.Description != "" {
			line += "  — " + oneLine(f.Description)
		}
		fmt.Fprintln(w, strings.TrimRight(line, " ")+fromNote(f.DeclaredBy, own))
	}
}

func printNamedSection(w io.Writer, title, own string, items []elementNamed) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(w, "\n%s\n", ui.Bold(title+":"))
	for _, n := range items {
		line := "  " + n.Name
		if n.Name == "" && title == "Slots" {
			line = "  (default)"
		}
		if n.Default != "" {
			line += " = " + n.Default
		}
		if n.Description != "" {
			line += "  — " + oneLine(n.Description)
		}
		fmt.Fprintln(w, line+fromNote(n.DeclaredBy, own))
	}
}
