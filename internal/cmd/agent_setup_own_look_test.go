package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/scaffold"
)

// THE "GIVE THE APP ITS OWN LOOK" SECTION (operator decision, 2026-11-24).
//
// Most submitted apps are built by an author's coding agent from the
// `page-elements` scaffold, and many ship Civitai's stock look because nothing
// the agent reads suggests otherwise. The managed block now RECOMMENDS a
// distinct look and shows the one safe pattern — a token override scoped to the
// app's root element, never `:root` (where it ties with the token sheet and
// silently loses). It is ADVISORY by decision: the section says so, and nothing
// in this CLI gates on it.
//
// The full how-to is the docs site's to own (AGENTS.md item 36: the CLI ships
// the address); the block carries one snippet, the both-themes check, and the
// link. See claudedocs/decisions/36-agents-block-per-project.md §"FOURTH NAMED
// EXCEPTION".

// wantOwnLookSection is the WHOLE section, whitespace-normalised.
//
// 🔴 PINNED AS A WHOLE STRING, NOT BY KEYWORDS — the artefact is prose, and a
// keyword guard ("theming") is walkable by rewording the section into one that
// drops "never `:root`" or "advisory". The URL is spelled out so the test also
// pins the spelling and the anchor.
const wantOwnLookSection = "### Give the app its own look (recommended) " +
	"Optional, never required for review. Rebrand by overriding the accent tokens on " +
	"the app's ROOT element (`[data-block-root]` in `page-elements`), never `:root`: " +
	"`[data-block-root] { --civitai-color-primary: #7c3aed; --civitai-color-primary-hover: #6d28d9; " +
	"--civitai-color-primary-fg: #ffffff; }`. " +
	"Check it in BOTH light and dark themes. How: " +
	"https://developer.civitai.com/apps/guide/theming.md#give-your-app-its-own-look"

// TestTheBlockRecommendsAnOwnLookInEveryShape pins the section for every project
// shape under BOTH SDK branches: it sits outside every `{{ if }}`, and a
// mis-nested edit that moved it inside one is what a single-shape check passes.
func TestTheBlockRecommendsAnOwnLookInEveryShape(t *testing.T) {
	shapes := allProjectShapesForTest()
	if len(shapes) < 3 {
		t.Fatalf("CONTROL failure, not a finding: %d shape(s) to render", len(shapes))
	}
	rendered := 0
	for _, sdk := range []bool{false, true} {
		for _, shape := range shapes {
			shape.ElementsSDK = sdk
			block, err := agentsManagedBlock(shape)
			if err != nil {
				t.Fatalf("rendering kind %q (sdk=%t): %v", shape.Kind, sdk, err)
			}
			rendered++
			got, ok := blockSectionOf(block, "Give the app its own look (recommended)")
			if !ok {
				t.Errorf("kind %q (sdk=%t): the managed block has no `### Give the app its own look "+
					"(recommended)` section", shape.Kind, sdk)
				continue
			}
			if norm := strings.Join(strings.Fields(got), " "); norm != wantOwnLookSection {
				t.Errorf("kind %q (sdk=%t): the own-look section is not what is pinned.\n--- want ---\n%s\n--- got ---\n%s",
					shape.Kind, sdk, wantOwnLookSection, norm)
			}
		}
	}
	if rendered < 6 {
		t.Fatalf("CONTROL failure, not a finding: rendered %d block(s), want >= 6", rendered)
	}
}

// TestOwnLookSnippetNamesOnlyRealTokensAndTheScaffoldRoot ties the section's
// snippet to the two things it claims: every token it names is one the
// page-elements scaffold's own stylesheet also names (and that stylesheet is
// held to the published token set by internal/scaffold's design-token guard),
// and `[data-block-root]` is the root selector that stylesheet styles.
func TestOwnLookSnippetNamesOnlyRealTokensAndTheScaffoldRoot(t *testing.T) {
	css := renderedPageElementsCSS(t)
	if !strings.Contains(css, "[data-block-root] {") {
		t.Fatal("the page-elements src/index.css no longer styles `[data-block-root]`, so the block's snippet " +
			"names a root the scaffold does not render — update both together")
	}
	for _, tok := range []string{"--civitai-color-primary:", "--civitai-color-primary-hover:", "--civitai-color-primary-fg:"} {
		if !strings.Contains(wantOwnLookSection, tok) {
			t.Fatalf("CONTROL failure, not a finding: the pinned section lacks %q", tok)
		}
		if !strings.Contains(css, tok) {
			t.Errorf("the block's snippet sets %s but the page-elements stylesheet's accent block does not — "+
				"the two are meant to show the same pattern", strings.TrimSuffix(tok, ":"))
		}
	}
}

// renderedPageElementsCSS scaffolds page-elements into a temp dir and returns
// its src/index.css as rendered.
func renderedPageElementsCSS(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "own-look-probe")
	if _, err := scaffold.Render(scaffold.PageElements, dir, scaffold.Data{Slug: "own-look-probe", Name: "Own Look Probe"}); err != nil {
		t.Fatalf("render page-elements: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "src", "index.css"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
