package cmd

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/scaffold"
)

// The managed block used to say, in the bullet that sends an agent to the hook
// reference: "every capability this platform has is a hook, and a capability you
// cannot find here probably exists under a name you have not guessed."
//
// That is false, and the falsehood has a measured cost. What a generation runs
// is chosen by the `kind` of the body submitted, and a kind is not a hook: the
// multi-step workflows guide says of itself "There is no new hook and no new
// message." A blind dogfood run told to build a multi-step app found the feature
// only by grepping the docs index for the word; an agent following the block
// searches the hook reference, finds nothing, and concludes the platform cannot
// do it.
//
// So the block now names the body kinds, each with the page that documents it.
// That is CONTENTS in a file nothing can recall — a named exception to AGENTS.md
// item 36's "the CLI ships the address" rule, argued and costed in
// claudedocs/decisions/36-agents-block-per-project.md.

// falseHookClaim is the sentence that was removed. It is asserted ABSENT as a
// literal, which is a guard on words and walkable by rewording — so it is the
// regression half only (red on the old template). The half that carries the
// claim is the whole-bullet pin below.
const falseHookClaim = "every capability this platform has is a hook"

// bodyKindRe reads the kinds the bullet names out of the bullet itself: a
// code-spanned identifier at the start of a nested list item, plus a second one
// listed after it on the same item.
var bodyKindRe = regexp.MustCompile("(?m)^  - `([A-Za-z]+)`(?:, `([A-Za-z]+)`)?")

// renderedBlocksByBranch renders the managed block for a REAL scaffold of each
// template family, so both `{{ if .ElementsSDK }}` branches are exercised. No
// shape in allProjectShapesForTest sets ElementsSDK, which is why the whole-
// section golden in agent_setup_docs_test.go says nothing about that branch.
func renderedBlocksByBranch(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for name, tmpl := range map[string]scaffold.Template{
		"react (page-money)":             scaffold.PageMoney,
		"web components (page-elements)": scaffold.PageElements,
	} {
		dir := filepath.Join(t.TempDir(), "proj")
		if _, err := scaffold.Render(tmpl, dir, scaffold.Data{Slug: "kinds-probe", Name: "Kinds Probe"}); err != nil {
			t.Fatalf("render %s: %v", tmpl, err)
		}
		block, err := agentsManagedBlock(detectProjectShape(dir))
		if err != nil {
			t.Fatalf("block for %s: %v", tmpl, err)
		}
		out[name] = block
	}
	empty, err := agentsManagedBlock(detectProjectShape(t.TempDir()))
	if err != nil {
		t.Fatalf("block for an empty directory: %v", err)
	}
	out["nothing scaffolded yet"] = empty
	return out
}

func TestTheBlockNamesEveryGenerationBodyKind(t *testing.T) {
	blocks := renderedBlocksByBranch(t)

	// CONTROL: the two SDK branches really are different renderings. If they are
	// not, "both branches" below is one branch counted twice.
	react, elements := blocks["react (page-money)"], blocks["web components (page-elements)"]
	if !strings.Contains(react, "hooks.md") || strings.Contains(elements, "hooks.md") {
		t.Fatal("CONTROL failure, not a finding: the page-money and page-elements blocks no longer differ on " +
			"hooks.md, so this test is not exercising both template branches")
	}

	for name, block := range blocks {
		t.Run(name, func(t *testing.T) {
			if strings.Contains(block, falseHookClaim) {
				t.Errorf("the block says %q again. It is false: a generation body `kind` (a multi-step workflow, "+
					"a ComfyUI graph) is a capability and is not a hook, so this sentence sends an agent to search "+
					"the hook reference for something that is not in it", falseHookClaim)
			}

			section, ok := docsSectionOf(block)
			if !ok {
				t.Fatal("no `### Docs` section")
			}
			// 🔴 THE WHOLE BULLET, BYTE FOR BYTE, EXACTLY ONCE, IN THE DOCS SECTION.
			// Prose is the artifact; a check for the five words passes on a bullet
			// reworded into something that no longer says where to read.
			if n := strings.Count(section, wantBodyKindsBullet); n != 1 {
				t.Fatalf("the `### Docs` section carries the generation-body-kinds bullet %d time(s), want 1.\n"+
					"--- want ---\n%s\n--- section ---\n%s", n, wantBodyKindsBullet, section)
			}
		})
	}

	// What the bullet NAMES, read out of it rather than restated: five kinds, the
	// members of the SDK's `WorkflowBody` union as published in the generation
	// reference the bullet links (verified there when this was written,
	// 2026-10-12). Nothing in this repository vendors that
	// union, so this is a pin on what the block says and not a mirror check. The
	// bullet's last item is what covers a kind added after this CLI shipped: it
	// sends the reader to the page that defines every kind.
	var got []string
	for _, m := range bodyKindRe.FindAllStringSubmatch(wantBodyKindsBullet, -1) {
		for _, k := range m[1:] {
			if k != "" {
				got = append(got, k)
			}
		}
	}
	sort.Strings(got)
	want := []string{"customComfy", "step", "textToImage", "training", "workflow"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("the bullet names kinds %v, want %v", got, want)
	}
}

// TestTheHookBulletNoLongerClaimsToBeThePlatform pins the React branch's
// replacement sentence, and that the Elements branch — a project that installs
// no hooks package — does not get it.
func TestTheHookBulletNoLongerClaimsToBeThePlatform(t *testing.T) {
	blocks := renderedBlocksByBranch(t)
	const want = "**Hooks are not every capability:** a generation body `kind`\n" +
		"  (below) is not a hook, and a multi-step workflow is a `kind`."
	if react := blocks["react (page-money)"]; strings.Count(react, want) != 1 {
		t.Errorf("a React project's block must say, once, that hooks are not every capability:\n%s", want)
	}
	if strings.Contains(blocks["web components (page-elements)"], "Hooks are not every capability") {
		t.Error("a page-elements project's block talks about hooks; that project installs no hooks package")
	}
}
