package cmd

import (
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
	for _, react := range []string{"blocks-react", "hooks.md", "useSharedStorage", "every capability this platform has is a hook"} {
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

	money := render(scaffold.PageMoney)
	for _, want := range []string{"blocks-react", "hooks.md"} {
		if !strings.Contains(money, want) {
			t.Errorf("a page-money project's AGENTS.md lost %q — it still installs @civitai/blocks-react", want)
		}
	}
}
