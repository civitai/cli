package scaffold

// Guards for the `manifest.bootSkeleton` opt-in.
//
// THE WHOLE HAZARD IS THAT THE TWO HALVES SEPARATE. `bootSkeleton: true` makes
// the Civitai run host stand down its loading veil, so the block's own entry
// document becomes the loading state. The manifest key and the markup inside the
// mount container are therefore ONE change, and shipping either half alone is a
// regression the other half's absence makes invisible:
//
//   - key without markup  -> a BLANK iframe for the entire load, worse than not
//     opting in, and nothing about the app looks broken to the author;
//   - markup without key  -> harmless but pointless (the veil still covers it).
//
// So these tests do not just assert "the templates contain the right strings".
// TestEveryScaffoldTemplatePassesTheBootSkeletonGate runs the real platform gate
// (BootSkeletonGateOK) over every template's rendered output, which is what
// makes a FUTURE template that declares the key without painting anything fail
// the build.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// renderTemplate renders one template into a fresh temp dir and returns it.
func renderTemplate(t *testing.T, tmpl Template) string {
	t.Helper()
	dest := filepath.Join(t.TempDir(), string(tmpl))
	if _, err := Render(tmpl, dest, Data{Slug: "boot-block", Name: "Boot Block"}); err != nil {
		t.Fatalf("render %s: %v", tmpl, err)
	}
	return dest
}

// readRendered reads one file out of a rendered template tree.
func readRendered(t *testing.T, dest, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return b
}

// reactTemplates are the templates that mount a React root and therefore ship
// skeleton MARKUP. `static` deliberately does not — see
// TestStaticTemplateDeclaresBootSkeletonWithoutSkeletonMarkup.
var reactTemplates = []Template{PageVite, PageMoney}

// TestEveryScaffoldTemplateDeclaresBootSkeleton pins the manifest half for all
// three templates, by PARSING the manifest rather than grepping it — a grep for
// `"bootSkeleton": true` passes on the string appearing in a comment or in prose
// and says nothing about the value the platform will read.
func TestEveryScaffoldTemplateDeclaresBootSkeleton(t *testing.T) {
	examined := 0
	for _, tmpl := range AllTemplates() {
		examined++
		t.Run(string(tmpl), func(t *testing.T) {
			dest := renderTemplate(t, tmpl)
			var m map[string]any
			if err := json.Unmarshal(readRendered(t, dest, "block.manifest.json"), &m); err != nil {
				t.Fatalf("manifest is not valid JSON: %v", err)
			}
			v, present := m["bootSkeleton"]
			if !present {
				t.Fatalf("template %q renders a manifest with no `bootSkeleton` key.\n"+
					"  Every scaffold template opts in: a static app's real content is already on\n"+
					"  screen at first paint, and the two React templates paint a skeleton inside\n"+
					"  #root. Without the key the host keeps its veil and that work is invisible.", tmpl)
			}
			b, ok := v.(bool)
			if !ok || !b {
				t.Fatalf("template %q renders `bootSkeleton: %#v`, want the boolean true", tmpl, v)
			}
		})
	}
	// COUNT POSITIVE CONTROL. The reassuring answer here is "every subtest
	// passed", which is also what an empty AllTemplates() produces.
	if examined < len(AllTemplates()) || examined < 3 {
		t.Fatalf("examined only %d template(s), expected at least 3 — the enumeration stopped OBSERVING", examined)
	}
}

// TestReactTemplatesPaintABootSkeletonInsideTheMountContainer pins the markup
// half for the two React templates, and pins it POSITIONALLY: the marker must be
// a DESCENDANT of #root, because a skeleton painted as a sibling of the mount
// container is never replaced by the app's render and stays on screen forever.
//
// This is the non-vacuous companion to the gate test below — the gate passes
// when a document has no container at all, this one fails.
func TestReactTemplatesPaintABootSkeletonInsideTheMountContainer(t *testing.T) {
	examined := 0
	for _, tmpl := range reactTemplates {
		examined++
		t.Run(string(tmpl), func(t *testing.T) {
			dest := renderTemplate(t, tmpl)
			doc, err := html.Parse(strings.NewReader(string(readRendered(t, dest, "index.html"))))
			if err != nil {
				t.Fatalf("parse index.html: %v", err)
			}

			containers := findBootSkeletonContainers(doc)
			if len(containers) == 0 {
				t.Fatalf("template %q renders an index.html with no #root / #app / [data-app-root] "+
					"mount container — the gate would pass VACUOUSLY on it", tmpl)
			}

			markers := findAllWithAttr(doc, BootSkeletonMarkerAttr)
			if len(markers) != 1 {
				t.Fatalf("template %q renders %d [%s] element(s), want exactly 1 "+
					"(the marker goes on the OUTERMOST element of the boot content)",
					tmpl, len(markers), BootSkeletonMarkerAttr)
			}
			m := markers[0]

			insideSome := false
			for _, c := range containers {
				if isDescendantOf(m, c) {
					insideSome = true
					break
				}
			}
			if !insideSome {
				t.Fatalf("template %q paints [%s] OUTSIDE the mount container — React's render "+
					"clears the container's children, so a skeleton outside it is never removed",
					tmpl, BootSkeletonMarkerAttr)
			}

			// The skeleton is decorative; the host already publishes aria-busy
			// on the iframe, so a second announcement in here talks over it.
			ariaHidden := ""
			for _, a := range m.Attr {
				if a.Key == "aria-hidden" {
					ariaHidden = a.Val
				}
			}
			if ariaHidden != "true" {
				t.Errorf("template %q: [%s] carries aria-hidden=%q, want \"true\"",
					tmpl, BootSkeletonMarkerAttr, ariaHidden)
			}

			// And it must actually paint SHAPES, not be an empty marker div —
			// an empty [data-boot-skeleton] satisfies the gate's non-emptiness
			// rule (it is a non-inert element) while painting nothing.
			if m.FirstChild == nil || !bootSkeletonContainerPaints(m) {
				t.Errorf("template %q: [%s] has no painted content of its own",
					tmpl, BootSkeletonMarkerAttr)
			}
		})
	}
	if examined != len(reactTemplates) || examined < 2 {
		t.Fatalf("examined only %d template(s), expected %d", examined, len(reactTemplates))
	}
}

// TestStaticTemplateDeclaresBootSkeletonWithoutSkeletonMarkup is an INVARIANT
// GUARD, not regression coverage: it pins a decision rather than a bug that
// happened. The `static` template opts in (its real content paints at first
// paint, so standing down the veil is pure win) and deliberately ships NO
// skeleton. A skeleton there would need explicit JS removal — there is no
// framework render to clear the container — and would buy a skeleton→content
// flash for nothing.
func TestStaticTemplateDeclaresBootSkeletonWithoutSkeletonMarkup(t *testing.T) {
	dest := renderTemplate(t, Static)
	raw := readRendered(t, dest, "index.html")

	doc, err := html.Parse(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("parse index.html: %v", err)
	}
	if got := len(findAllWithAttr(doc, BootSkeletonMarkerAttr)); got != 0 {
		t.Fatalf("the static template ships %d [%s] element(s); it must ship none — "+
			"nothing removes a skeleton in a no-build app", got, BootSkeletonMarkerAttr)
	}

	// The manifest key is only honest if the container really does paint on its
	// own. This is the fact that makes "no skeleton" correct rather than lazy.
	containers := findBootSkeletonContainers(doc)
	if len(containers) == 0 {
		t.Fatal("the static template renders no mount container — the claim that its own content " +
			"satisfies the gate would be untestable")
	}
	for _, c := range containers {
		if !bootSkeletonContainerPaints(c) {
			t.Fatalf("the static template's %s paints nothing, so `bootSkeleton: true` would leave "+
				"the viewer with a blank iframe", bootSkeletonContainerLabel(c))
		}
	}
}

// TestBootSkeletonGateOK table-tests the gate itself against hand-built
// documents, including the three shapes that MUST fail. Without these the gate
// could be a function that returns nil unconditionally and every other test in
// this file would still pass.
func TestBootSkeletonGateOK(t *testing.T) {
	const declared = `{"blockId":"x","bootSkeleton":true}`
	const notDeclared = `{"blockId":"x"}`

	cases := []struct {
		name     string
		manifest string
		doc      string
		wantErr  error
	}{
		{
			name:     "passes: skeleton inside #root",
			manifest: declared,
			doc: `<!doctype html><html><body><div id="root">` +
				`<div data-boot-skeleton aria-hidden="true"><div></div></div>` +
				`</div><script src="/m.js"></script></body></html>`,
			wantErr: nil,
		},
		{
			name:     "passes: static app, real content and no marker",
			manifest: declared,
			doc:      `<!doctype html><html><body><main id="app"><h1>Hi</h1></main></body></html>`,
			wantErr:  nil,
		},
		{
			name:     "fails (a): bootSkeleton true over an EMPTY #root",
			manifest: declared,
			doc:      `<!doctype html><html><body><div id="root"></div><script src="/m.js"></script></body></html>`,
			wantErr:  ErrBootSkeletonEmptyContainer,
		},
		{
			name:     "fails (b): marker OUTSIDE the mount container",
			manifest: declared,
			doc: `<!doctype html><html><body>` +
				`<div data-boot-skeleton aria-hidden="true"><div></div></div>` +
				`<div id="root"><span>content</span></div>` +
				`</body></html>`,
			wantErr: ErrBootSkeletonMarkerOutsideContainer,
		},
		{
			name:     "fails (c1): #root holds only a <script>",
			manifest: declared,
			doc:      `<!doctype html><html><body><div id="root"><script>var a = 1;</script></div></body></html>`,
			wantErr:  ErrBootSkeletonEmptyContainer,
		},
		{
			name:     "fails (c2): #root holds only whitespace",
			manifest: declared,
			doc:      "<!doctype html><html><body><div id=\"root\">\n   \n\t</div></body></html>",
			wantErr:  ErrBootSkeletonEmptyContainer,
		},
		{
			name:     "fails (c3): #root holds only a comment",
			manifest: declared,
			doc:      `<!doctype html><html><body><div id="root"><!-- mounted at runtime --></div></body></html>`,
			wantErr:  ErrBootSkeletonEmptyContainer,
		},
		{
			name:     "fails: one of TWO containers is empty",
			manifest: declared,
			doc: `<!doctype html><html><body><div id="root"><span>x</span></div>` +
				`<div data-app-root></div></body></html>`,
			wantErr: ErrBootSkeletonEmptyContainer,
		},
		{
			name:     "passes: gate does not apply when bootSkeleton is absent",
			manifest: notDeclared,
			doc:      `<!doctype html><html><body><div id="root"></div></body></html>`,
			wantErr:  nil,
		},
		{
			name:     "passes: no recognisable container, the gate does not guess",
			manifest: declared,
			doc:      `<!doctype html><html><body><div id="mount"></div></body></html>`,
			wantErr:  nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := BootSkeletonGateOK([]byte(tc.manifest), []byte(tc.doc))
			switch {
			case tc.wantErr == nil && err != nil:
				t.Fatalf("want pass, got: %v", err)
			case tc.wantErr != nil && err == nil:
				t.Fatalf("want %v, got a PASS — the gate did not fire on a document it must reject", tc.wantErr)
			case tc.wantErr != nil && !errors.Is(err, tc.wantErr):
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
		})
	}

	// COUNT POSITIVE CONTROL: an empty table is a serene green.
	failing := 0
	for _, tc := range cases {
		if tc.wantErr != nil {
			failing++
		}
	}
	if len(cases) < 10 || failing < 6 {
		t.Fatalf("table shrank to %d case(s) / %d failing — this test's whole value is the failing arms",
			len(cases), failing)
	}
}

// TestEveryScaffoldTemplatePassesTheBootSkeletonGate is the guard that makes the
// two halves inseparable GOING FORWARD. It runs the real gate over every
// template's real rendered entry document, so a future template that declares
// `bootSkeleton: true` without painting anything inside its mount container
// fails the build here rather than shipping to an author.
//
// It reads the SOURCE index.html rather than a built bundle. Stated rather than
// hidden: the platform gate operates on the BUILT artifact, and vite copies
// index.html through while rewriting only the script src, so the container and
// the skeleton survive. The template-page-vite / template-page-money CI jobs
// build the scaffolded apps for real; this offline guard is about the templates.
func TestEveryScaffoldTemplatePassesTheBootSkeletonGate(t *testing.T) {
	examined := 0
	for _, tmpl := range AllTemplates() {
		examined++
		t.Run(string(tmpl), func(t *testing.T) {
			dest := renderTemplate(t, tmpl)
			manifestJSON := readRendered(t, dest, "block.manifest.json")
			doc := readRendered(t, dest, "index.html")
			if err := BootSkeletonGateOK(manifestJSON, doc); err != nil {
				t.Fatalf("template %q FAILS the platform's bootSkeleton gate:\n  %v", tmpl, err)
			}
		})
	}
	if examined < len(AllTemplates()) || examined < 3 {
		t.Fatalf("examined only %d template(s), expected at least 3 — the enumeration stopped OBSERVING", examined)
	}
}

var (
	// A `<style>` element's text content, non-greedy.
	styleBlockRe = regexp.MustCompile(`(?s)<style[^>]*>(.*?)</style>`)
	// Any `prefers-color-scheme: dark` media query, however spaced.
	prefersDarkRe = regexp.MustCompile(`@media[^{]*prefers-color-scheme\s*:\s*dark`)
	// Any `prefers-color-scheme: light` media query, however spaced.
	prefersLightRe = regexp.MustCompile(`@media[^{]*prefers-color-scheme\s*:\s*light`)
	// `<meta name="color-scheme" content="...">`, attribute order-insensitive
	// enough for the templates we control.
	metaColorSchemeRe = regexp.MustCompile(`<meta\s+name="color-scheme"\s+content="([^"]*)"`)
	// An `html { … }` rule and its body. The leading delimiter class includes
	// `{` so the rule is also found when it is NESTED inside an @media block —
	// which is exactly where the light override lives.
	htmlRuleRe = regexp.MustCompile(`(?s)(^|[};{])\s*html\s*\{([^}]*)\}`)
	// A hex colour.
	hexColourRe = regexp.MustCompile(`#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})\b`)
)

// TestBootSkeletonPaintsDarkByDefault pins the theme rule STRUCTURALLY.
//
// The rule is not "the CSS mentions a dark colour somewhere" — that is a word a
// reword walks past. It is a relationship between where values live:
//
//  1. the BASE (unconditioned) rules carry the dark values, specifically an
//     `html { background: <dark> }`;
//  2. there is NO `@media (prefers-color-scheme: dark)` block anywhere in the
//     inline style. That block is the actual defect this guards: it looks
//     correct and inverts the default, because a UA reporting `no-preference`
//     — or one without the query at all — matches neither branch and falls
//     through to the base rules;
//  3. light IS present, and only inside `@media (prefers-color-scheme: light)`;
//  4. `<meta name="color-scheme">` reads `dark light`, dark FIRST.
//
// Point 4 is asserted for ALL THREE templates. The static one has no inline
// style, so points 1–3 apply to the two React templates.
func TestBootSkeletonPaintsDarkByDefault(t *testing.T) {
	// A colour is "dark" here if every channel is below this. Both palettes in
	// the templates sit far from the boundary in both directions, so this
	// threshold is not load-bearing to a few units either way.
	const darkChannelMax = 0x60

	t.Run("meta color-scheme is dark-first in every template", func(t *testing.T) {
		examined := 0
		for _, tmpl := range AllTemplates() {
			examined++
			dest := renderTemplate(t, tmpl)
			doc := string(readRendered(t, dest, "index.html"))
			m := metaColorSchemeRe.FindStringSubmatch(doc)
			if m == nil {
				t.Errorf("template %q renders no <meta name=\"color-scheme\">; the UA canvas is the "+
					"FIRST thing an iframe paints and without this it defaults light", tmpl)
				continue
			}
			fields := strings.Fields(m[1])
			if len(fields) == 0 || fields[0] != "dark" {
				t.Errorf("template %q: <meta name=\"color-scheme\" content=%q> — the FIRST token must "+
					"be `dark`; `light dark` defaults the canvas to LIGHT", tmpl, m[1])
			}
		}
		if examined < len(AllTemplates()) || examined < 3 {
			t.Fatalf("examined only %d template(s), expected at least 3", examined)
		}
	})

	examined := 0
	for _, tmpl := range reactTemplates {
		examined++
		t.Run(string(tmpl), func(t *testing.T) {
			dest := renderTemplate(t, tmpl)
			doc := string(readRendered(t, dest, "index.html"))

			blocks := styleBlockRe.FindAllStringSubmatch(doc, -1)
			if len(blocks) != 1 {
				t.Fatalf("template %q has %d inline <style> block(s) in index.html, want exactly 1 — "+
					"the skeleton must be styled inline or it waits on a second round-trip",
					tmpl, len(blocks))
			}
			css := blocks[0][1]
			if !strings.Contains(css, BootSkeletonMarkerAttr) {
				t.Fatalf("template %q: the inline <style> never mentions [%s], so it is not the "+
					"skeleton's stylesheet", tmpl, BootSkeletonMarkerAttr)
			}

			// (2) — the inverting block must not exist.
			if loc := prefersDarkRe.FindString(css); loc != "" {
				t.Errorf("template %q: the inline style contains %q.\n"+
					"  DARK MUST BE THE BASE, not a media query. A UA reporting `no-preference`, or one\n"+
					"  without the query, matches neither branch and falls through to the base rules —\n"+
					"  so putting the dark values behind this block inverts the default.", tmpl, loc)
			}
			// (3) — light must be present, and behind the light query.
			if !prefersLightRe.MatchString(css) {
				t.Errorf("template %q: no `@media (prefers-color-scheme: light)` block — light is the "+
					"only theme that belongs behind a query", tmpl)
			}

			// (1) — the base `html { … }` rule carries a dark background. "Base"
			// = the first html rule, which must sit before any @media block.
			firstMedia := strings.Index(css, "@media")
			hm := htmlRuleRe.FindStringSubmatchIndex(css)
			if hm == nil {
				t.Fatalf("template %q: the inline style has no `html { … }` rule. That rule is the "+
					"STRONG guarantee — unlike `color-scheme` it does not depend on UA support", tmpl)
			}
			if firstMedia >= 0 && hm[0] > firstMedia {
				t.Fatalf("template %q: the first `html { … }` rule sits inside/after an @media block; "+
					"the base rules must carry the dark values", tmpl)
			}
			body := css[hm[4]:hm[5]]
			if !strings.Contains(body, "background") {
				t.Fatalf("template %q: the base `html { … }` rule sets no background: %q", tmpl, body)
			}
			hex := hexColourRe.FindString(body)
			if hex == "" {
				t.Fatalf("template %q: the base `html` background is not a literal colour (%q). It must "+
					"not be a custom property — nothing has defined one this early", tmpl, body)
			}
			if !isDarkHex(t, hex, darkChannelMax) {
				t.Errorf("template %q: the BASE `html` background is %s, which is not dark. The base "+
					"rules are what a `no-preference` UA gets.", tmpl, hex)
			}

			// And the LIGHT override must genuinely be lighter, or the "light
			// lives behind the query" structure would be decorative.
			lightIdx := prefersLightRe.FindStringIndex(css)
			lightCSS := css[lightIdx[0]:]
			lm := htmlRuleRe.FindStringSubmatch(lightCSS)
			if lm == nil {
				t.Fatalf("template %q: the light media block has no `html { … }` override, so the base "+
					"dark background survives in light mode", tmpl)
			}
			lightHex := hexColourRe.FindString(lm[2])
			if lightHex == "" || isDarkHex(t, lightHex, darkChannelMax) {
				t.Errorf("template %q: the light override sets `html` background to %q, which is not "+
					"lighter than the dark base", tmpl, lightHex)
			}
		})
	}
	if examined != len(reactTemplates) || examined < 2 {
		t.Fatalf("examined only %d template(s), expected %d", examined, len(reactTemplates))
	}
}

// isDarkHex reports whether every channel of a #rgb / #rrggbb colour is below
// max.
func isDarkHex(t *testing.T, hex string, max int) bool {
	t.Helper()
	h := strings.TrimPrefix(hex, "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 {
		t.Fatalf("unparseable colour %q", hex)
	}
	for i := 0; i < 6; i += 2 {
		var v int
		for _, c := range h[i : i+2] {
			v <<= 4
			switch {
			case c >= '0' && c <= '9':
				v |= int(c - '0')
			case c >= 'a' && c <= 'f':
				v |= int(c-'a') + 10
			case c >= 'A' && c <= 'F':
				v |= int(c-'A') + 10
			default:
				t.Fatalf("unparseable colour %q", hex)
			}
		}
		if v >= max {
			return false
		}
	}
	return true
}
