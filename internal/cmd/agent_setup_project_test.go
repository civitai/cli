package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/scaffold"
)

// The regression guards for the defect agent_setup_project.go records: the
// managed block naming npm commands in a project that has no package.json.
//
// 🔴 THESE RUN OVER THE REAL SCAFFOLDS, NOT OVER A HAND-BUILT FIXTURE. The
// defect was a disagreement between two shipped artefacts — `civitai app init`'s
// tree and the block `civitai agent-setup` writes into it — so a fixture that
// asserts what a static project "looks like" would have re-encoded the same
// assumption that produced the bug. Every case below scaffolds with
// scaffold.Render and then drives the real command, which is the check the blind
// dogfood ran by hand.

// declaredProjectKindRe pulls the `projectKind` constants out of
// agent_setup_project.go's own source. Go has no way to enumerate the members of
// a string-constant "enum" at runtime, so the alternative to reading the source
// is a literal that silently stops being the whole set — which is exactly what
// the comment on allProjectShapesForTest used to claim it was not.
var declaredProjectKindRe = regexp.MustCompile(`(?m)^\s*project\w+\s+projectKind\s*=\s*"([^"]+)"`)

// declaredProjectKinds returns every projectKind value declared in
// agent_setup_project.go, in source order.
//
// 🔴 THIS IS THE DERIVATION THE COMMENT BELOW USED TO ASSERT WITHOUT DOING.
// `allProjectShapesForTest` names its three kinds as a literal; nothing made a
// fourth constant show up there. Reading the source closes that, and
// TestAllProjectShapesCoversEveryDeclaredKind is where the two are compared.
func declaredProjectKinds(t *testing.T) []projectKind {
	t.Helper()
	path := filepath.Join(repoRootDir(t), "internal", "cmd", "agent_setup_project.go")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	matches := declaredProjectKindRe.FindAllStringSubmatch(string(src), -1)
	if len(matches) < 3 {
		t.Fatalf("CONTROL failure, not a finding: declaredProjectKindRe (%s) matched %d constant(s) in %s, "+
			"want >= 3. The regex has stopped reading the declarations, so every set comparison built on it "+
			"is a comparison against an empty set", declaredProjectKindRe, len(matches), path)
	}
	kinds := make([]projectKind, 0, len(matches))
	for _, m := range matches {
		kinds = append(kinds, projectKind(m[1]))
	}
	return kinds
}

// TestAllProjectShapesCoversEveryDeclaredKind makes the sentence on
// allProjectShapesForTest true by checking it: every `projectKind` constant
// declared in agent_setup_project.go is rendered by at least one shape, and no
// shape carries a kind that is not declared.
func TestAllProjectShapesCoversEveryDeclaredKind(t *testing.T) {
	declared := declaredProjectKinds(t)
	seen := map[projectKind]bool{}
	for _, shape := range allProjectShapesForTest() {
		seen[shape.Kind] = true
	}
	for _, k := range declared {
		if !seen[k] {
			t.Errorf("projectKind %q is declared in internal/cmd/agent_setup_project.go and rendered by NO "+
				"shape in allProjectShapesForTest. Every guard in this package that loops over the shapes is "+
				"silent about that branch of the template — add a shape for it.", k)
		}
	}
	declaredSet := map[projectKind]bool{}
	for _, k := range declared {
		declaredSet[k] = true
	}
	for k := range seen {
		if !declaredSet[k] {
			t.Errorf("allProjectShapesForTest renders kind %q, which is not declared as a projectKind constant "+
				"in internal/cmd/agent_setup_project.go — the constant was renamed or removed and the shapes "+
				"were left behind.", k)
		}
	}
}

// allProjectShapesForTest enumerates the shapes the template must render for.
// The kind list below is a literal — Go cannot enumerate a string-constant set —
// and TestAllProjectShapesCoversEveryDeclaredKind is what stops it drifting from
// the projectKind constants, so a fourth kind cannot be added without a rendering
// test for it.
func allProjectShapesForTest() []projectShape {
	kinds := []projectKind{projectNPM, projectNoBuild, projectNone}
	shapes := make([]projectShape, 0, len(kinds)+1)
	for _, k := range kinds {
		shapes = append(shapes, projectShape{Kind: k, Templates: scaffoldTemplateRows()})
	}
	// The npm kind has two renderings — with and without recognised scripts —
	// and the second one is the unreadable/unrecognised package.json path.
	shapes = append(shapes, projectShape{
		Kind:      projectNPM,
		Scripts:   knownDevScripts,
		Templates: scaffoldTemplateRows(),
	})
	return shapes
}

// scaffoldInto renders one shipped template into a fresh directory and returns
// it, so a case cannot accidentally assert against a directory somebody built by
// hand.
func scaffoldInto(t *testing.T, tmpl scaffold.Template) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "proj")
	if _, err := scaffold.Render(tmpl, dir, scaffold.Data{Slug: "demo-app", Name: "Demo App"}); err != nil {
		t.Fatalf("scaffold %s: %v", tmpl, err)
	}
	return dir
}

// managedBlockIn extracts what `agent-setup` wrote between its markers. Asserting
// against the whole file would also read whatever the author already had there.
func managedBlockIn(t *testing.T, dir string) string {
	t.Helper()
	raw := readFile(t, filepath.Join(dir, agentsFilename))
	begin := strings.Index(raw, agentsBeginMarker)
	end := strings.Index(raw, agentsEndMarker)
	if begin < 0 || end <= begin {
		t.Fatalf("no well-formed managed block in %s:\n%s", filepath.Join(dir, agentsFilename), raw)
	}
	return raw[begin : end+len(agentsEndMarker)]
}

// setupInto runs the real command into dir with a neutralised environment.
func setupInto(t *testing.T, dir string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "1")
	for _, s := range agentEnvSignals {
		t.Setenv(s.Var, "")
	}
	out, errOut, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude")
	if err != nil {
		t.Fatalf("agent-setup into %s: %v\n%s\n%s", dir, err, out, errOut)
	}
	return managedBlockIn(t, dir)
}

// npmCommandRe matches any npm/pnpm/yarn invocation the block might name.
var npmCommandRe = regexp.MustCompile(`\b(npm|pnpm|yarn)\b`)

// TestAStaticProjectIsNeverToldToRunNPM is THE regression guard.
//
// 🔴 IT WAS WATCHED FAIL ON origin/main, AND THE MEASUREMENT IS THE EXACT ONE.
// Run at a6a36f6 against a `--template static` scaffold, it reported
// `[npm npm npm]` — `npm run dev:harness`, `npm run dev:tunnel` and the `npm ci`
// in the lockfile gotcha — plus all three of the positive-half assertions below
// ("no `package.json`", "index.html", "serve the directory"), because the block
// had nothing to say to a project that ships no package.json at all (`app.js`,
// `assets/`, `block.manifest.json`, `civitai-host.js`, `.gitignore`,
// `index.html`, `README.md`, `style.css`).
//
// 🔴 THE SCAN IS DELIBERATELY BLUNT: it bans the WORD, not a code span. A block
// written for a project with no package manager has no reason to mention one
// even in prose, and a guard that only matched a backticked code span would pass
// a sentence telling the agent to run npm install without backticks.
//
// 🔴 AND IT ASSERTS THE POSITIVE HALF IN THE SAME RUN, so a fix that empties the
// section instead of branching it does not pass: the block must still tell a
// static author how to preview the app, in the words `civitai app init` uses.
func TestAStaticProjectIsNeverToldToRunNPM(t *testing.T) {
	dir := scaffoldInto(t, scaffold.Static)

	// PREMISE, checked rather than assumed: this really is a project with no
	// package.json. If the static template ever gains one, this test is measuring
	// something else and must say so instead of passing.
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
		t.Fatalf("PREMISE BROKEN: the static scaffold now ships a package.json, so this guard is not "+
			"exercising the no-build case (%s)", dir)
	}

	block := setupInto(t, dir)
	if hits := npmCommandRe.FindAllString(block, -1); len(hits) > 0 {
		t.Errorf("the managed block written into a static project names a package manager %v — that project "+
			"has no package.json, so every one of those commands fails. Block:\n%s", hits, block)
	}
	// The section has to say something true, not nothing.
	for _, want := range []string{"no `package.json`", "index.html", "serve the directory"} {
		if !strings.Contains(block, want) {
			t.Errorf("the static project's block never mentions %q — it dropped the npm rows without "+
				"replacing them:\n%s", want, block)
		}
	}
}

// TestTheNPMScanCanSeeAnNPMCommand is the POSITIVE CONTROL for the scan above. A
// regex wired to nothing reports a clean static project and a clean page-money
// one identically, and only one of those is correct.
func TestTheNPMScanCanSeeAnNPMCommand(t *testing.T) {
	dir := scaffoldInto(t, scaffold.PageMoney)
	block := setupInto(t, dir)
	if hits := npmCommandRe.FindAllString(block, -1); len(hits) == 0 {
		t.Fatalf("the npm scan found nothing in a page-money project, which IS an npm project — the scan in "+
			"TestAStaticProjectIsNeverToldToRunNPM observes nothing. Block:\n%s", block)
	}
}

// TestTheBlockNeverNamesAScriptTheProjectDoesNotDefine is the RELATIONSHIP the
// fix actually establishes, asserted as a relationship rather than as a word
// list.
//
// 🔴 A LIST OF EXPECTED SCRIPT NAMES WOULD BE THE SAME MISTAKE ONE LEVEL UP: it
// would pass for a template that hardcodes the names this test happens to
// expect. So every `npm run X` the block prints is looked up in THAT project's
// own package.json, for every shipped template that has one. The guard therefore
// holds for a template nobody has written yet.
func TestTheBlockNeverNamesAScriptTheProjectDoesNotDefine(t *testing.T) {
	runScriptRe := regexp.MustCompile("`npm run ([a-z0-9:._-]+)`")
	checked := 0
	for _, tmpl := range scaffold.AllTemplates() {
		if !tmpl.NeedsInstall() {
			continue
		}
		t.Run(string(tmpl), func(t *testing.T) {
			dir := scaffoldInto(t, tmpl)
			block := setupInto(t, dir)

			var pkg struct {
				Scripts map[string]string `json:"scripts"`
			}
			if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "package.json"))), &pkg); err != nil {
				t.Fatalf("%s's package.json does not parse: %v", tmpl, err)
			}
			named := runScriptRe.FindAllStringSubmatch(block, -1)
			if len(named) == 0 {
				t.Fatalf("the block for %s names no `npm run` script at all, so this test asserts nothing "+
					"about it:\n%s", tmpl, block)
			}
			for _, m := range named {
				if _, ok := pkg.Scripts[m[1]]; !ok {
					t.Errorf("the block tells a %s project to run `npm run %s`, which is not in its "+
						"package.json (it defines %v)", tmpl, m[1], scriptNames(pkg.Scripts))
				}
				checked++
			}
		})
	}
	if checked == 0 {
		t.Fatal("no npm template was exercised — the loop found nothing to check")
	}
}

// TestPageViteIsNotToldAboutPageMoneysScripts is the discriminating case between
// the two npm templates. Branching only on "is there a package.json" would leave
// page-vite — which defines `dev`, `build` and `preview` and neither of the two
// the old block named — reading instructions written for page-money.
func TestPageViteIsNotToldAboutPageMoneysScripts(t *testing.T) {
	dir := scaffoldInto(t, scaffold.PageVite)
	block := setupInto(t, dir)
	if !strings.Contains(block, "`npm run dev`") {
		t.Errorf("the page-vite block never names `npm run dev`, which is the script it does define:\n%s", block)
	}
	for _, absent := range []string{"dev:harness", "dev:live", "dev:tunnel"} {
		if strings.Contains(block, absent) {
			t.Errorf("the page-vite block names %q, which only page-money's package.json defines:\n%s", absent, block)
		}
	}
}

// TestAnEmptyDirectoryIsToldThatNothingIsScaffoldedHere covers the third shape.
// `agent-setup` is a top-level command that legitimately runs before any app
// exists, and there the honest answer is that it does not know — not a guess,
// and not page-money's commands.
func TestAnEmptyDirectoryIsToldThatNothingIsScaffoldedHere(t *testing.T) {
	dir := t.TempDir()
	block := setupInto(t, dir)
	if hits := npmCommandRe.FindAllString(block, -1); len(hits) > 0 {
		t.Errorf("the block written into a directory holding no app names a package manager %v:\n%s", hits, block)
	}
	if !strings.Contains(block, "No Civitai App has been scaffolded in this directory") {
		t.Errorf("the empty-directory block does not say the directory holds no app:\n%s", block)
	}
	// The template enumeration is DERIVED from scaffold.AllTemplates(), so every
	// shipped template must appear in it — including one added after this test.
	for _, tmpl := range scaffold.AllTemplates() {
		if !strings.Contains(block, "`"+string(tmpl)+"`") {
			t.Errorf("the empty-directory block does not name the %q template:\n%s", tmpl, block)
		}
	}
}

// TestDetectProjectShapeClassifiesEveryShippedTemplate pins the classifier
// itself against the trees `civitai app init` really produces, in BOTH
// directions: an npm template must not read as no-build, and the no-build one
// must not read as npm.
func TestDetectProjectShapeClassifiesEveryShippedTemplate(t *testing.T) {
	for _, tmpl := range scaffold.AllTemplates() {
		t.Run(string(tmpl), func(t *testing.T) {
			dir := scaffoldInto(t, tmpl)
			got := detectProjectShape(dir)
			want := projectNoBuild
			if tmpl.NeedsInstall() {
				want = projectNPM
			}
			if got.Kind != want {
				t.Fatalf("detectProjectShape(%s scaffold) = %q, want %q", tmpl, got.Kind, want)
			}
			if want == projectNPM && len(got.Scripts) == 0 {
				t.Errorf("%s is an npm template but no recognised dev script was read out of its "+
					"package.json — the block would tell its author there is none", tmpl)
			}
		})
	}
	t.Run("empty", func(t *testing.T) {
		if got := detectProjectShape(t.TempDir()); got.Kind != projectNone {
			t.Errorf("detectProjectShape(empty dir) = %q, want %q", got.Kind, projectNone)
		}
	})
	t.Run("manifest-and-package-json", func(t *testing.T) {
		// The order matters: page-vite and page-money carry BOTH files, so a
		// classifier testing the manifest first calls every npm template no-build.
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "block.manifest.json"), "{}\n")
		writeFile(t, filepath.Join(dir, "package.json"), `{"scripts":{"dev":"vite"}}`)
		if got := detectProjectShape(dir); got.Kind != projectNPM {
			t.Errorf("a directory with both files classified as %q, want %q", got.Kind, projectNPM)
		}
	})
	t.Run("unparseable-package-json", func(t *testing.T) {
		// Still an npm project; just one this command cannot read scripts out of.
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "package.json"), "{not json")
		got := detectProjectShape(dir)
		if got.Kind != projectNPM {
			t.Errorf("kind = %q, want %q", got.Kind, projectNPM)
		}
		if len(got.Scripts) != 0 {
			t.Errorf("scripts = %v, want none from a file that does not parse", got.Scripts)
		}
	})
}

// TestAgentsTemplateRendersForEveryShape is what makes the template.Must at
// package scope safe to read as "this cannot fail at runtime": a parse error is
// caught at init, and an EXECUTION error (a field that does not exist, a bad
// range) is caught here, for every shape the command can produce.
func TestAgentsTemplateRendersForEveryShape(t *testing.T) {
	for _, shape := range allProjectShapesForTest() {
		block, err := agentsManagedBlock(shape)
		if err != nil {
			t.Fatalf("rendering for kind %q: %v", shape.Kind, err)
		}
		if strings.Contains(block, "<no value>") || strings.Contains(block, "{{") {
			t.Errorf("the rendered block for kind %q still holds template syntax or an unresolved "+
				"field:\n%s", shape.Kind, block)
		}
	}
}

func scriptNames(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
