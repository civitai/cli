package scaffold

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The page-elements scaffold carries a READY-TO-UNCOMMENT accent block in
// src/index.css and a note in index.html that the boot skeleton's colours should
// follow a rebrand. Both are ADVISORY by operator decision (2026-11-24): the
// scaffold still ships the stock look, so the override must be inside a comment,
// and the skeleton's colours are unchanged.

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

func TestPageElementsBootSkeletonNotesItShouldFollowARebrand(t *testing.T) {
	dir := renderPageElements(t)
	doc := readOwnLookFile(t, dir, "index.html")
	i := strings.Index(doc, "REBRANDING?")
	if i < 0 {
		t.Fatalf("index.html has no rebranding note for the boot skeleton:\n%s", doc)
	}
	// The note sits BEFORE the inline <style>, as an HTML comment, so the boot
	// skeleton's own stylesheet — which TestBootSkeletonPaintsDarkByDefault parses
	// — is untouched.
	if s := strings.Index(doc, "<style>"); s < 0 || i > s {
		t.Errorf("the rebranding note must precede the inline <style> (note at %d, <style> at %d)", i, s)
	}
	note := doc[i:]
	note = strings.Join(strings.Fields(note[:strings.Index(note, "-->")]), " ")
	for _, want := range []string{"src/index.css", "make these follow it", "html[data-theme='light']"} {
		if !strings.Contains(note, want) {
			t.Errorf("the rebranding note lacks %q: %s", want, note)
		}
	}
	// Behaviour unchanged: the four stock skeleton colours are still there.
	for _, hex := range []string{"#1a1b1e", "#ffffff", "#2c2e33", "#e9ecef"} {
		if !strings.Contains(doc, hex) {
			t.Errorf("index.html lost the stock skeleton colour %s — the note must not change the skeleton", hex)
		}
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
