package scaffold

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The page-elements scaffold carries a READY-TO-UNCOMMENT accent block in
// src/index.css. It is ADVISORY by operator decision (2026-11-24): the scaffold
// still ships the stock look, so the override must be inside a comment.

// cssCommentRe matches one CSS block comment (non-greedy, across lines).
var cssCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)

// ownLookAccentRule is the rule the accent block offers, whitespace-normalised.
// The selector is the root src/block.ts renders (`data-block-root` on the view's
// outer element and on the error wrapper).
const ownLookAccentRule = "[data-block-root] { --civitai-color-primary: #7c3aed; " +
	"--civitai-color-primary-hover: #6d28d9; --civitai-color-primary-fg: #ffffff; }"

const ownLookGuideURL = "https://developer.civitai.com/apps/guide/theming.md#give-your-app-its-own-look"

func TestPageElementsOffersACommentedAccentBlockOnTheAppRoot(t *testing.T) {
	dir := renderPageElements(t)
	css := readOwnLookFile(t, dir, "src/index.css")

	var accent string
	for _, c := range cssCommentRe.FindAllString(css, -1) {
		if strings.Contains(c, "Make it yours") {
			accent = c
		}
	}
	if accent == "" {
		t.Fatalf("src/index.css has no `Make it yours` comment block:\n%s", css)
	}
	norm := strings.Join(strings.Fields(accent), " ")
	for _, want := range []string{ownLookAccentRule, "NEVER to :root", "BOTH the light and the dark theme", ownLookGuideURL,
		"recommended, not required"} {
		if !strings.Contains(norm, want) {
			t.Errorf("the accent comment lacks %q:\n%s", want, accent)
		}
	}

	// ADVISORY: with every comment removed, the stylesheet sets none of the
	// primary tokens — the scaffold still ships the stock look until the author
	// opts in.
	live := cssCommentRe.ReplaceAllString(css, "")
	if strings.Contains(live, "--civitai-color-primary") {
		t.Errorf("src/index.css sets a primary token OUTSIDE a comment — the accent block must ship commented out:\n%s", live)
	}
	// Positive control for the strip: the live stylesheet still styles the root.
	if !strings.Contains(live, "[data-block-root] {") {
		t.Fatalf("CONTROL failure, not a finding: the comment strip removed the live `[data-block-root]` rule too")
	}

	// The selector is the one the block's markup carries.
	block := readOwnLookFile(t, dir, "src/block.ts")
	if !strings.Contains(block, "data-block-root>") || !strings.Contains(block, "dataset.blockRoot = ''") {
		t.Errorf("src/block.ts no longer marks its root with data-block-root, so the accent block's selector " +
			"matches nothing — update both together")
	}
}

func readOwnLookFile(t *testing.T, dir, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
