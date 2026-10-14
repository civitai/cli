package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/scaffold"
)

// The managed block used to tell a React project's agent "every capability this
// platform has is a hook". That is false: what a generation runs is chosen by
// the `kind` of the body submitted (a multi-step workflow is `kind: 'workflow'`),
// and a kind is not a hook, so an agent searching the hook reference for it
// finds nothing. A blind dogfood run hit exactly that.
//
// The sentence is replaced by ONE POINTER line (wantKindsPointer) to the page
// that lists the kinds — an address, not a list, per AGENTS.md item 36.

// falseHookClaim is asserted ABSENT. A guard on words is walkable by rewording,
// so this is only the regression half; the whole-line pin below and the
// whole-section golden in agent_setup_docs_test.go carry the claim.
const falseHookClaim = "every capability this platform has is a hook"

// blocksByBranch renders the block for a REAL scaffold of each SDK family so
// both `{{ if .ElementsSDK }}` branches are exercised — no shape in
// allProjectShapesForTest reaches the @civitai/sdk branch.
func blocksByBranch(t *testing.T) (react, elements string) {
	t.Helper()
	render := func(tmpl scaffold.Template) string {
		dir := filepath.Join(t.TempDir(), "proj")
		if _, err := scaffold.Render(tmpl, dir, scaffold.Data{Slug: "kinds-probe", Name: "Kinds Probe"}); err != nil {
			t.Fatalf("render %s: %v", tmpl, err)
		}
		block, err := agentsManagedBlock(detectProjectShape(dir))
		if err != nil {
			t.Fatalf("block for %s: %v", tmpl, err)
		}
		return block
	}
	return render(scaffold.PageMoney), render(scaffold.PageElements)
}

// TestTheElementsBlockPointsAtWorkflowsAndTheBodyKinds: the `@civitai/sdk`
// branch is the DEFAULT template's, and it must tell its reader that generation
// body kinds exist, that a multi-step workflow is one, and where both are
// documented. The pointer is pinned whole, exactly once, inside ### Docs.
func TestTheElementsBlockPointsAtWorkflowsAndTheBodyKinds(t *testing.T) {
	_, elements := blocksByBranch(t)

	section, ok := docsSectionOf(elements)
	if !ok {
		t.Fatal("page-elements block has no `### Docs` section")
	}
	got := normaliseMessage(section)
	if n := strings.Count(got, wantElementsGenerationPointer); n != 1 {
		t.Errorf("page-elements `### Docs` carries the generation pointer %d time(s), want 1.\n--- want ---\n%s\n--- section ---\n%s\n"+
			"Edit internal/cmd/templates/agents-app.md and wantElementsGenerationPointer together.",
			n, wantElementsGenerationPointer, got)
	}
}

// TestBothSDKBranchesSayMultiStepWorkflowsExist is the RELATIONSHIP guard: the
// two branches are edited separately, and the defect was one of them saying
// nothing. Both must name multi-step workflows and link the page that lists the
// body kinds.
//
// The React side is not rendered again here: wantKindsPointer is pinned into
// that branch by TestTheBlockPointsAtTheGenerationBodyKinds and by the
// whole-section golden, so holding the PIN to the claim holds the branch to it.
func TestBothSDKBranchesSayMultiStepWorkflowsExist(t *testing.T) {
	_, elements := blocksByBranch(t)
	section, ok := docsSectionOf(elements)
	if !ok {
		t.Fatal("page-elements block has no `### Docs` section")
	}
	for name, text := range map[string]string{
		"page-money (@civitai/blocks-react), as pinned by wantKindsPointer": wantKindsPointer,
		"page-elements (@civitai/sdk), as rendered":                         section,
	} {
		if !strings.Contains(strings.ToLower(normaliseMessage(text)), "multi-step workflow") {
			t.Errorf("%s: never mentions a multi-step workflow, so its reader concludes there is none", name)
		}
		if !strings.Contains(text, generationKindsURL) {
			t.Errorf("%s: does not link the generation body kinds (%s)", name, generationKindsURL)
		}
	}
}

func TestTheBlockPointsAtTheGenerationBodyKinds(t *testing.T) {
	react, elements := blocksByBranch(t)

	// CONTROL: the two renderings really are different branches.
	if !strings.Contains(react, "hooks.md") || strings.Contains(elements, "hooks.md") {
		t.Fatal("CONTROL failure, not a finding: the page-money and page-elements blocks no longer differ on " +
			"hooks.md, so this test is not exercising both template branches")
	}

	for name, block := range map[string]string{"react": react, "elements": elements} {
		if strings.Contains(block, falseHookClaim) {
			t.Errorf("%s block says %q again — false: a generation body `kind` is a capability and not a hook", name, falseHookClaim)
		}
	}

	// The React branch carries the pointer whole, exactly once, inside ### Docs.
	section, ok := docsSectionOf(react)
	if !ok {
		t.Fatal("React block has no `### Docs` section")
	}
	if n := strings.Count(section, wantKindsPointer); n != 1 {
		t.Errorf("React `### Docs` carries the generation-body-kinds pointer %d time(s), want 1.\n--- want ---\n%s\n--- section ---\n%s",
			n, wantKindsPointer, section)
	}
	// The Elements branch installs no hooks package, so a line about hooks is wrong there.
	if strings.Contains(elements, "Capabilities are hooks") {
		t.Error("the page-elements block talks about hooks; that project installs no hooks package")
	}
}
