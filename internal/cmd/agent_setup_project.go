package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/civitai/cli/internal/manifest"
	"github.com/civitai/cli/internal/scaffold"
)

// What `agent-setup` is allowed to say about running THIS project locally.
//
// 🔴 THE MANAGED BLOCK USED TO NAME npm SCRIPTS UNCONDITIONALLY, AND `civitai
// app init` DEFAULTS TO A TEMPLATE THAT HAS NO package.json. The block's command
// table said "Local dev, mock host, no Buzz spent | `npm run dev:harness`" and
// its gotchas said "Commit the lockfile. The platform builds with `npm ci`" in
// EVERY project it was written into. Measured on the three shipped templates:
// `dev:harness` and `dev:tunnel` exist only in page-money's package.json;
// page-vite defines `dev`, `build` and `preview` and neither of those two; and
// `static` — the DEFAULT — scaffolds app.js, assets/, block.manifest.json,
// civitai-host.js, .gitignore, index.html, README.md, style.css and no
// package.json at all. So the block was wrong for two templates of three, and
// wrong in the worst direction for the default one: it told an agent to run a
// package manager in a project that has nothing to run it on, while `civitai app
// init`'s own next-steps output for that same template says "then open
// index.html or serve the directory". A blind dogfood hit exactly that and had
// to work out which of two Civitai-authored documents to believe.
//
// 🔴 SO THE SECTION IS DERIVED, NOT ASSERTED. detectProjectShape READS the
// directory the block is being written into, and the npm branch names only
// scripts that are in that project's own package.json — a command it cannot read
// out of the file is a command it does not print. That is the property: the
// document cannot name a script the project does not define, because there is no
// code path that emits a script name from anywhere but the file.
//
// 🔴 AND THE "NO PROJECT HERE YET" BRANCH ENUMERATES THE TEMPLATES FROM
// `scaffold`, NOT FROM PROSE. `agent-setup` is a top-level command that may
// legitimately run in an empty directory, before any app exists (see the header
// of agent_setup.go), and there the honest answer is that it does not know. The
// sentence naming which templates are npm projects is built from
// scaffold.AllTemplates() and Template.NeedsInstall(), so a fourth template
// appears there without anybody remembering to edit a markdown file.
//
// AGENTS.md item 36 — evidence: claudedocs/decisions/36-agents-block-per-project.md
// carries the full measurement, the rejected alternatives and the enumerated
// residuals (what the knownDevScripts table does NOT close, and what was not
// measured about tunnelling a static project).

// projectKind is what the directory receiving AGENTS.md looks like. Three
// values, because the three have genuinely different answers and collapsing any
// two of them is how the defect above got written.
type projectKind string

const (
	// projectNPM: a package.json is present. Whatever else is here, npm commands
	// are meaningful and the lockfile rule applies.
	projectNPM projectKind = "npm"
	// projectNoBuild: a block.manifest.json and no package.json — the shape
	// `civitai app init --template static` produces. No npm command applies.
	projectNoBuild projectKind = "no-build"
	// projectNone: neither. `agent-setup` ran somewhere no app has been
	// scaffolded, which is a supported and documented way to run it.
	projectNone projectKind = "none"
)

// devScriptRow is one npm script the project defines, with what the scaffolds
// mean by that name.
type devScriptRow struct {
	Script string
	Task   string
}

// templateRow is one `civitai app init` template, for the "no project here"
// branch. Derived from the scaffold package so it cannot go stale.
type templateRow struct {
	Name string
	NPM  bool
}

// projectShape is the render input for the managed block.
type projectShape struct {
	Kind      projectKind
	Scripts   []devScriptRow
	Templates []templateRow
}

// knownDevScripts is the ORDER and the WORDING for the script names the shipped
// scaffolds use. A row is emitted only when the project's own package.json
// defines that exact key, so this table can never introduce a command — it can
// only describe one that is already there.
//
// 🔴 THE DESCRIPTIONS ARE THE SCAFFOLDS' MEANING, AND THE TEMPLATE SAYS SO. They
// are taken from the generated READMEs rather than invented here: page-money's
// README states `dev:harness` mounts the SDK mock host ("no real Buzz, no
// compute, no network") and `dev:live` mounts the live host against the real
// backend ("Spends REAL Buzz / real compute"); page-vite's states there is no
// host behind `npm run dev`. A project that redefines one of these names under
// its own meaning is covered by the sentence the template prints under the
// table, not by silently guessing.
//
// The safe one is first on purpose: an agent reading top-down reaches the
// no-Buzz command before the one that spends the user's money.
var knownDevScripts = []devScriptRow{
	{"dev:harness", "Local dev against a MOCK host — synthetic replies, no real Buzz, no compute, no network"},
	{"dev", "Plain local preview — there is no host behind it, so nothing sends BLOCK_INIT"},
	{"dev:live", "Local dev against the REAL Civitai backend with a dev token — spends REAL Buzz"},
	{"dev:tunnel", "Serve this app so `civitai app dev-tunnel` can reach it (run it in another terminal)"},
}

// scaffoldTemplateRows is the template enumeration, read from the scaffold
// package rather than restated.
func scaffoldTemplateRows() []templateRow {
	all := scaffold.AllTemplates()
	rows := make([]templateRow, 0, len(all))
	for _, t := range all {
		rows = append(rows, templateRow{Name: string(t), NPM: t.NeedsInstall()})
	}
	return rows
}

// detectProjectShape classifies dir. It is a pure read and never fails: a
// package.json this process cannot read or cannot parse is still a package.json,
// so the kind stays `npm` and the script list is simply empty — the template's
// npm branch has wording for that, and refusing the whole run because a file is
// malformed would be a much worse answer than a narrower section.
//
// 🔴 THE ORDER IS package.json FIRST. page-vite and page-money carry BOTH a
// package.json and a block.manifest.json, so testing the manifest first would
// classify every npm template as no-build.
func detectProjectShape(dir string) projectShape {
	shape := projectShape{Templates: scaffoldTemplateRows()}

	pkgPath := filepath.Join(dir, "package.json")
	if _, err := os.Stat(pkgPath); err == nil {
		shape.Kind = projectNPM
		shape.Scripts = devScriptsIn(pkgPath)
		return shape
	}
	if _, err := os.Stat(filepath.Join(dir, manifest.Filename)); err == nil {
		shape.Kind = projectNoBuild
		return shape
	}
	shape.Kind = projectNone
	return shape
}

// devScriptsIn returns the knownDevScripts the file at path defines, in
// knownDevScripts order. An unreadable or unparseable package.json yields none,
// which the template reports as "no dev script this CLI recognises" rather than
// as an error — see detectProjectShape.
func devScriptsIn(path string) []devScriptRow {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return nil
	}
	var rows []devScriptRow
	for _, candidate := range knownDevScripts {
		if _, ok := pkg.Scripts[candidate.Script]; ok {
			rows = append(rows, candidate)
		}
	}
	return rows
}
