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
	money := render(scaffold.PageMoney)
	for _, react := range []string{"blocks-react", "hooks.md", "useSharedStorage", "Capabilities are hooks AND generation body kinds"} {
		if strings.Contains(elements, react) {
			t.Errorf("a page-elements project's AGENTS.md mentions %q — that project does not install @civitai/blocks-react", react)
		}
	}
	for _, want := range []string{
		"@civitai/sdk", "initialize()", "app.onChange", "app.site", "app.requestGrants",
		"@civitai/components",
	} {
		if !strings.Contains(elements, want) {
			t.Errorf("a page-elements project's AGENTS.md should point at %q", want)
		}
	}

	// The element reference is a COMMAND, not a file. Evaluation runs measured
	// agents pointed at custom-elements.json writing `node -e` parsers for it
	// (16 in one run), and still writing 4–14 per run after the block asked
	// them to look up one element rather than read the file whole — and the
	// file omits every form control's inherited attributes besides. So the
	// block names `civitai app elements`, the name must resolve against THIS
	// CLI's command tree (a pointer at a command that does not exist is worse
	// than none), and the block must no longer name the JSON file at all: any
	// mention of it is an invitation to parse it. Asserted on the command and
	// the file NAME, not on the surrounding prose.
	const elementsCmd = "civitai app elements"
	if !strings.Contains(elements, "`"+elementsCmd+" <tag>`") {
		t.Errorf("a page-elements project's AGENTS.md should name `%s <tag>` as the element reference", elementsCmd)
	}
	if err := resolveCommandRef(NewRootCmd(), elementsCmd); err != nil {
		t.Errorf("the block names %q, which this CLI does not have: %v", elementsCmd, err)
	}
	if strings.Contains(elements, "custom-elements.json") {
		t.Errorf("a page-elements project's AGENTS.md names custom-elements.json — an agent reads that as the " +
			"thing to parse; `civitai app elements` is the reference")
	}
	if strings.Contains(money, elementsCmd) {
		t.Errorf("a page-money project's AGENTS.md names %q — that project does not install @civitai/components", elementsCmd)
	}

	// The readable pointer, and the MARKUP.md trap. @civitai/components also
	// ships MARKUP.md, which LOOKS like the readable answer but documents the
	// `data-civitai-ui` contract for hand-written HTML — a separate consumption
	// path the `<civitai-*>` elements this template uses do not follow (its own
	// intro says so). So the block adds the package README's "Elements" section
	// as the readable pointer, and may name MARKUP.md only inside the warning
	// that it is NOT the elements' reference — pinned as a relationship: strip
	// the warning and no mention of MARKUP.md may remain. README.md and MARKUP.md
	// were in the `files` of @civitai/components 0.9.0 and 0.9.3 (`npm pack
	// --dry-run`) and are in an installed 0.10.0, the template's current floor.
	// Mirrors civitai/civitai-app-starters#648's starter AGENTS.md.
	const markupWarning = "🔴 `node_modules/@civitai/components/MARKUP.md` is NOT the elements' reference: " +
		"it is the `data-civitai-ui` contract for HAND-WRITTEN HTML " +
		"(`<div data-civitai-ui=\"text-input\"><label>…<input>`), which the `<civitai-*>` elements do not use. " +
		"Here, write `<civitai-text-input>`."
	norm := normaliseProseForTest(elements)
	for _, want := range []string{
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

	for _, want := range []string{"blocks-react", "hooks.md"} {
		if !strings.Contains(money, want) {
			t.Errorf("a page-money project's AGENTS.md lost %q — it still installs @civitai/blocks-react", want)
		}
	}
}

// scratchLine is the AGENTS.md line about scratch/, normalised, as EVERY project
// with a root scratch/ gets it. It claims only what holds in every such project:
// `app submit` leaves the root scratch/ out (pkgzip's rootExcludedDirs). Whether
// the directory is git-ignored or outside the typecheck is that project's own
// config's business, so the block does not say it.
const scratchLine = "**Put probes and debug files in `scratch/`, never in `src/` or `test/`.** " +
	"`civitai app submit` leaves this project-root `scratch/` out of the package."

// scratchReadmePointer follows scratchLine only where `scratch/README.md`
// exists — page-elements ships one stating what its scaffold makes true.
const scratchReadmePointer = " `scratch/README.md` says what else holds for it in this project."

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

	if got := blockFor(render(scaffold.PageElements)); !strings.Contains(got, scratchLine+scratchReadmePointer) {
		t.Errorf("a page-elements project's AGENTS.md does not carry the scratch/ line and README pointer:\nwant: %s", scratchLine+scratchReadmePointer)
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
	got := blockFor(money)
	if !strings.Contains(got, scratchLine) {
		t.Errorf("a page-money project WITH a scratch/ directory did not get the scratch/ line")
	}
	// 🔴 A BARE scratch/ PROVES ONLY THE DIRECTORY. page-money's .gitignore has
	// no scratch rule, so a block telling the agent its probes are git-ignored
	// would get them committed. Neither that claim nor the README pointer (no
	// README here) may appear.
	for _, banned := range []string{"git-ignored", "scratch/README.md"} {
		if strings.Contains(got, banned) {
			t.Errorf("a page-money project with a bare scratch/ got %q in its AGENTS.md — "+
				"nothing in that project establishes it", banned)
		}
	}
	// And the README, not the template, is what turns the pointer on.
	if err := os.WriteFile(filepath.Join(money, "scratch", "README.md"), []byte("# notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := blockFor(money); !strings.Contains(got, scratchLine+scratchReadmePointer) {
		t.Errorf("a scratch/README.md did not add the pointer to the block")
	}

	// A `Scratch/` is NOT the excluded directory: the packager matches by exact
	// case and ships it, so the block must not say it is left out. ⚠ On a
	// case-SENSITIVE filesystem (Linux CI) os.Stat("scratch") misses `Scratch/`
	// too, so this row is an INVARIANT GUARD there; it bites only where the FS
	// folds case (macOS), which is the case hasDirNamedExactly exists for.
	other := render(scaffold.PageMoney)
	if err := os.Mkdir(filepath.Join(other, "Scratch"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := blockFor(other); strings.Contains(got, "scratch/") {
		t.Errorf("a `Scratch/` directory made the block claim the project-root scratch/ is left out of the package")
	}
}
