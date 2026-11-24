package cmd

import (
	"strings"
	"testing"
)

// THE THEMING POINTER (operator decision, 2026-11-24: "Advisory only — point to
// the theming guide and say a distinct look is recommended").
//
// Most submitted apps are built by an author's coding agent from a scaffold, and
// many ship Civitai's stock look because nothing the agent reads suggests
// otherwise. The managed block's `### Docs` list now carries ONE advisory line
// pointing at the theming guide's own-look section. It is an ADDRESS plus one
// sentence, not a recipe: the how-to is the docs site's to own (AGENTS.md item
// 36), and the `page-elements` scaffold carries its own commented accent block
// in src/index.css. A section with a token snippet was considered and dropped in
// review: it rendered for every project kind, including `page-vite` and `static`,
// which use no `--civitai-color-*` tokens.

// wantThemingPointer is the whole line, whitespace-normalised. Pinned as a
// whole string so a reword that drops "never required" (the advisory claim) or
// the fragment goes red.
const wantThemingPointer = "- Theming — a distinct look (your own accent colour) is recommended, never " +
	"required: https://developer.civitai.com/apps/guide/theming.md#give-your-app-its-own-look"

// TestTheDocsSectionPointsAtTheOwnLookGuideInEveryShape pins the pointer in the
// `### Docs` section for every project shape under BOTH SDK branches: the line
// sits in the shared tail, outside every `{{ if }}`, and a mis-nested edit that
// moved it inside one is what a single-shape check passes.
func TestTheDocsSectionPointsAtTheOwnLookGuideInEveryShape(t *testing.T) {
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
			docs, ok := docsSectionOf(block)
			if !ok {
				t.Errorf("kind %q (sdk=%t): the managed block has no `### Docs` section", shape.Kind, sdk)
				continue
			}
			if norm := strings.Join(strings.Fields(docs), " "); !strings.Contains(norm, wantThemingPointer) {
				t.Errorf("kind %q (sdk=%t): the `### Docs` section lacks the theming pointer.\n--- want ---\n%s\n--- got ---\n%s",
					shape.Kind, sdk, wantThemingPointer, docs)
			}
		}
	}
	if rendered < 6 {
		t.Fatalf("CONTROL failure, not a finding: rendered %d block(s), want >= 6", rendered)
	}
}
