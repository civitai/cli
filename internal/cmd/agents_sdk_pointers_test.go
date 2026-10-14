package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/scaffold"
)

// TestAgentsBlockPointsAtTheSDKTheProjectInstalled: the managed AGENTS.md block
// tells an agent where the API reference is. A page-elements project installs
// `@civitai/sdk` and `@civitai/components`, not `@civitai/blocks-react`, so a
// block sending it to the React hook reference and to blocks-react's `.d.ts`
// sends it to a package that is not in node_modules. page-money still installs
// blocks-react, so its block must still say so — the control that this test can
// tell the two apart, rather than passing on a block that names neither.
func TestAgentsBlockPointsAtTheSDKTheProjectInstalled(t *testing.T) {
	render := func(tmpl scaffold.Template) string {
		t.Helper()
		dir := filepath.Join(t.TempDir(), "proj")
		if _, err := scaffold.Render(tmpl, dir, scaffold.Data{Slug: "agents-probe", Name: "Agents Probe"}); err != nil {
			t.Fatalf("render %s: %v", tmpl, err)
		}
		block, err := agentsManagedBlock(detectProjectShape(dir))
		if err != nil {
			t.Fatalf("render block for %s: %v", tmpl, err)
		}
		return block
	}

	elements := render(scaffold.PageElements)
	for _, react := range []string{"blocks-react", "hooks.md", "useSharedStorage", "Capabilities are hooks AND generation body kinds"} {
		if strings.Contains(elements, react) {
			t.Errorf("a page-elements project's AGENTS.md mentions %q — that project does not install @civitai/blocks-react", react)
		}
	}
	for _, want := range []string{
		"@civitai/sdk", "initialize()", "app.onChange", "app.site", "app.requestGrants",
		"@civitai/components", "custom-elements.json",
	} {
		if !strings.Contains(elements, want) {
			t.Errorf("a page-elements project's AGENTS.md should point at %q", want)
		}
	}

	// The readable pointer, and the MARKUP.md trap. An eval run measured agents
	// pointed only at custom-elements.json spending whole turns writing `node -e`
	// parsers for it (16 in one run). @civitai/components also ships MARKUP.md,
	// which LOOKS like the readable answer but documents the `data-civitai-ui`
	// contract for hand-written HTML — a separate consumption path the
	// `<civitai-*>` elements this template uses do not follow (its own intro says
	// so). So the block adds the package README's "Elements" section as the
	// readable pointer, and may name MARKUP.md only inside the warning that it is
	// NOT the elements' reference — pinned as a relationship: strip the warning
	// and no mention of MARKUP.md may remain. README.md, custom-elements.json and
	// MARKUP.md are all in the `files` of @civitai/components 0.9.0 and 0.9.3 (the
	// template's `^0.9.0` floor and the current release; `npm pack --dry-run`).
	// Mirrors civitai/civitai-app-starters#648's starter AGENTS.md.
	const markupWarning = "🔴 `node_modules/@civitai/components/MARKUP.md` is NOT the elements' reference: " +
		"it is the `data-civitai-ui` contract for HAND-WRITTEN HTML " +
		"(`<div data-civitai-ui=\"text-input\"><label>…<input>`), which the `<civitai-*>` elements do not use. " +
		"Here, write `<civitai-text-input>`."
	norm := normaliseProseForTest(elements)
	for _, want := range []string{
		"node_modules/@civitai/components/custom-elements.json",
		"`node_modules/@civitai/components/README.md`**, section \"Elements\"",
		markupWarning,
	} {
		if !strings.Contains(norm, want) {
			t.Errorf("a page-elements project's AGENTS.md lacks (normalised): %s", want)
		}
	}
	if rest := strings.Replace(norm, markupWarning, "", 1); strings.Contains(rest, "MARKUP") {
		t.Errorf("a page-elements project's AGENTS.md names MARKUP.md outside the warning that it is not the "+
			"elements' reference — an agent would read it as the place to look:\n%s", rest)
	}

	money := render(scaffold.PageMoney)
	for _, want := range []string{"blocks-react", "hooks.md"} {
		if !strings.Contains(money, want) {
			t.Errorf("a page-money project's AGENTS.md lost %q — it still installs @civitai/blocks-react", want)
		}
	}
}

// scratchLine is the one AGENTS.md line about scratch/, normalised.
const scratchLine = "**Put probes and debug files in `scratch/`, never in `src/` or `test/`.** It is " +
	"git-ignored and outside both the typecheck and the test run, so a throwaway file there cannot " +
	"fail `npm run typecheck` or `npm test`."

// TestAgentsBlockNamesScratchOnlyWhereItExists: page-elements scaffolds a
// git-ignored scratch/ outside its tsconfig and vitest includes, and an eval run
// measured probes written into test/ becoming the only tsc errors and a test
// file that failed to load. The block tells the agent where probes go — but
// only in a project that HAS scratch/ (item 36: rendered from the project), so
// page-money, which ships none, must not name it until one is created.
func TestAgentsBlockNamesScratchOnlyWhereItExists(t *testing.T) {
	blockFor := func(dir string) string {
		t.Helper()
		block, err := agentsManagedBlock(detectProjectShape(dir))
		if err != nil {
			t.Fatal(err)
		}
		return normaliseProseForTest(block)
	}
	render := func(tmpl scaffold.Template) string {
		t.Helper()
		dir := filepath.Join(t.TempDir(), "proj")
		if _, err := scaffold.Render(tmpl, dir, scaffold.Data{Slug: "scratch-probe", Name: "Scratch Probe"}); err != nil {
			t.Fatalf("render %s: %v", tmpl, err)
		}
		return dir
	}

	if got := blockFor(render(scaffold.PageElements)); !strings.Contains(got, scratchLine) {
		t.Errorf("a page-elements project's AGENTS.md does not carry the scratch/ line:\nwant: %s", scratchLine)
	}

	money := render(scaffold.PageMoney)
	if got := blockFor(money); strings.Contains(got, "scratch/") {
		t.Errorf("a page-money project's AGENTS.md names scratch/, which that project does not have")
	}
	// A FILE named scratch is not the directory.
	if err := os.WriteFile(filepath.Join(money, "scratch"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := blockFor(money); strings.Contains(got, "scratch/") {
		t.Errorf("a regular FILE named scratch made the block name a scratch/ directory")
	}
	// Positive control: the directory, not the template, is what turns it on.
	if err := os.Remove(filepath.Join(money, "scratch")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(money, "scratch"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := blockFor(money); !strings.Contains(got, scratchLine) {
		t.Errorf("a page-money project WITH a scratch/ directory did not get the scratch/ line")
	}
}
