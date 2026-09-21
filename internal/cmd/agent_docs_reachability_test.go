package cmd

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/scaffold"
)

// The three agent-facing-documentation defects measured on a blind dogfood run
// (one-line brief, 66 steps, not a line of code written), each guarded by the
// PROPERTY that was violated rather than by the prose that now fixes it.
//
//	defect 2 — AGENTS.md said "No Civitai App has been scaffolded in this
//	           directory" for 58 of the 66 steps, because `app create` never
//	           re-derived it.
//	defect 3 — the `### Docs` block sat at byte 3,104 of 3,460 (90% in) and was
//	           a bare list with no imperative; the run made exactly ONE HTTP
//	           request in 66 steps.
//	defect 4 — the commands table showed only `civitai app create <name>`, and
//	           the hidden default is the heaviest template (37 files, a ~40 KB
//	           README, a ~40 KB App.tsx): 53.5% of the failed run's carried
//	           tokens went on reading it before any work began.

// ---------------------------------------------------------------------------
// Defect 2 — `app create` leaves AGENTS.md describing the project it just made
// ---------------------------------------------------------------------------

// TestScaffoldingRefreshesAGENTSForTheProjectItJustMade is the regression guard.
//
// 🔴 IT IS A RELATIONSHIP, NOT A WORD CHECK. It does not grep the file for the
// absent-project sentence — that would pass the moment somebody reworded the
// branch. It requires the AGENTS.md on disk to be BYTE-IDENTICAL to the block
// `agent-setup` derives from that same directory, which is the actual claim:
// the file describes THIS project. The placeholder branch is then excluded by
// construction, and the test says so by rendering that branch separately and
// requiring it to differ (the positive control — if the two were equal, the
// identity assertion above would prove nothing).
//
// 🔴 WATCHED FAIL at origin/main (8603fc3), where `app create` wrote no
// AGENTS.md at all:
//
//	agent_docs_reachability_test.go: `civitai app create --template page-money`
//	left no AGENTS.md in the project it created …
func TestScaffoldingRefreshesAGENTSForTheProjectItJustMade(t *testing.T) {
	for _, tmpl := range scaffold.AllTemplates() {
		t.Run(string(tmpl), func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "proj")
			if out, errOut, err := run(t, "app", "create", "demo-app",
				"--dir", dir, "--template", string(tmpl), "--yes"); err != nil {
				t.Fatalf("app create --template %s: %v\n%s\n%s", tmpl, err, out, errOut)
			}

			raw, err := os.ReadFile(filepath.Join(dir, agentsFilename))
			if err != nil {
				t.Fatalf("`civitai app create --template %s` left no %s in the project it created (%v).\n"+
					"  The file this CLI designates as the source of truth for scaffolding, running and "+
					"validating a project is written by `agent-setup`, which `app create` never ran — so an "+
					"author whose agent read it got either nothing or a document about an empty directory.",
					tmpl, agentsFilename, err)
			}

			// The claim: what is on disk is what `agent-setup` would derive from
			// THIS directory, now that the project exists in it.
			want, err := agentsManagedBlock(detectProjectShape(dir))
			if err != nil {
				t.Fatalf("rendering the expected block: %v", err)
			}
			if got := string(raw); strings.TrimSpace(got) != strings.TrimSpace(want) {
				t.Errorf("the %s written by `app create --template %s` is not the block `agent-setup` derives "+
					"from that directory.\n--- on disk ---\n%s\n--- derived ---\n%s",
					agentsFilename, tmpl, got, want)
			}

			// POSITIVE CONTROL for the assertion above: the "nothing scaffolded
			// here" rendering must be a DIFFERENT document. If it were not, the
			// identity check would be satisfied by the very placeholder this
			// test exists to exclude.
			placeholder, err := agentsManagedBlock(projectShape{
				Kind: projectNone, Templates: scaffoldTemplateRows(),
			})
			if err != nil {
				t.Fatalf("rendering the placeholder block: %v", err)
			}
			if strings.TrimSpace(want) == strings.TrimSpace(placeholder) {
				t.Fatalf("CONTROL failure, not a finding: the block derived from a scaffolded %s project is "+
					"byte-identical to the `no project here` placeholder, so the identity assertion above "+
					"cannot tell the two apart", tmpl)
			}
		})
	}
}

// TestScaffoldReportsAnAgentsWriteItCouldNotDo covers the branch the happy path
// cannot reach: `app create` refuses a non-empty directory, so the only way the
// AGENTS.md write fails is a destination the process cannot use. The scaffold
// must still be a success, and the failure must still be NAMED.
//
// 🔴 A SILENT SKIP HERE IS THE WORST OUTCOME. An author whose AGENTS.md could
// not be written and who was never told has an agent reading a stale document
// with nothing to indicate it.
func TestScaffoldReportsAnAgentsWriteItCouldNotDo(t *testing.T) {
	dir := t.TempDir()
	// AGENTS.md as a DIRECTORY: readable as a path, unusable as a file.
	if err := os.Mkdir(filepath.Join(dir, agentsFilename), 0o755); err != nil {
		t.Fatal(err)
	}
	note := writeScaffoldAgentsMD(dir)
	if len(note) == 0 {
		t.Fatal("writeScaffoldAgentsMD reported nothing for a destination it cannot write — the failure " +
			"is silent, which is the one outcome this branch exists to prevent")
	}
	joined := strings.Join(note, " ")
	if !strings.Contains(joined, agentsFilename) {
		t.Errorf("the failure note does not name the file it could not write: %q", joined)
	}
	if !strings.Contains(joined, "civitai agent-setup") {
		t.Errorf("the failure note names no way forward — this repo's rule is that an error names the "+
			"next command to run: %q", joined)
	}

	// The other arm: a writable directory reports a SUCCESS note, so the two are
	// distinguishable. Without this the test above passes on a function that
	// returns the same text unconditionally.
	ok := writeScaffoldAgentsMD(t.TempDir())
	if strings.Join(ok, " ") == joined {
		t.Error("the success note and the failure note are the same text — the caller cannot tell the " +
			"two outcomes apart, and neither can a reader")
	}
}

// ---------------------------------------------------------------------------
// Defect 3 — the docs are reachable, and they are an instruction
// ---------------------------------------------------------------------------

// blockSectionOrder returns the byte offset of each `### ` heading in a rendered
// block, keyed by the heading's first word after `### `.
func blockSectionOrder(block string) map[string]int {
	out := map[string]int{}
	for i := 0; i < len(block); {
		j := strings.Index(block[i:], "\n### ")
		if j < 0 {
			break
		}
		start := i + j + 1
		rest := block[start+len("### "):]
		if k := strings.IndexAny(rest, " \n"); k >= 0 {
			rest = rest[:k]
		}
		if _, seen := out[rest]; !seen {
			out[rest] = start
		}
		i = start + 1
	}
	return out
}

// TestTheDocsSectionComesBeforeEverythingItCompetesWith is the defect-3 guard.
//
// 🔴 POSITION IS THE DEFECT, SO POSITION IS THE ASSERTION. The old block put
// `### Docs` LAST — byte 3,104 of 3,460, 90% in, below the commands table, a
// stale local-dev section and five gotchas — and the agent that was given it
// made exactly ONE HTTP request in 66 steps while spending 28.2% of its budget
// reverse-engineering the SDK from `.d.ts` files. A link nobody reaches is not
// documentation.
//
// It is per-SHAPE because the template branches three ways between Docs and
// Gotchas, and a mis-nested `{{ if }}` can move a heading in one branch only.
func TestTheDocsSectionComesBeforeEverythingItCompetesWith(t *testing.T) {
	for _, shape := range allProjectShapesForTest() {
		block, err := agentsManagedBlock(shape)
		if err != nil {
			t.Fatalf("rendering for kind %q: %v", shape.Kind, err)
		}
		order := blockSectionOrder(block)

		// Control: the extractor must have found the sections at all.
		for _, want := range []string{"Docs", "Commands", "Gotchas"} {
			if _, ok := order[want]; !ok {
				t.Fatalf("CONTROL failure, not a finding: the rendered block for kind %q has no `### %s` "+
					"heading (found %v) — the ordering assertions below would compare absent offsets",
					shape.Kind, want, sortedKeys(headingSet(order)))
			}
		}
		for _, after := range []string{"Commands", "Gotchas", "Local"} {
			pos, ok := order[after]
			if !ok {
				continue
			}
			if order["Docs"] > pos {
				t.Errorf("kind %q: `### Docs` is at byte %d, AFTER `### %s` at byte %d. The measured "+
					"failure is that a reader never gets there: the agent this was written for made one "+
					"HTTP request in 66 steps with the docs block 90%% of the way down the file.",
					shape.Kind, order["Docs"], after, pos)
			}
		}

		// And it has to read as an instruction, not as a bibliography. The
		// discriminating fact is a fetchable, machine-readable target: the
		// plain-text `.md` spelling of the hook reference, which is the ONE
		// document the measured run needed and never opened.
		section, ok := docsSectionOf(block)
		if !ok {
			t.Fatalf("kind %q: no `### Docs` section to read", shape.Kind)
		}
		if !strings.Contains(section, "reference/hooks.md") {
			t.Errorf("kind %q: the Docs section names no plain-text hook reference. That is the document "+
				"the measured run needed (`useCreatePostFromApp`) and the one it never fetched; a section "+
				"that does not name it hands the reader a list of landing pages.\n%s", shape.Kind, section)
		}
	}
}

// TestBlockSectionOrderExtractorCanFail is the negative control for the
// offsets: a reader that returns the same map for every input would make the
// ordering assertions vacuous.
func TestBlockSectionOrderExtractorCanFail(t *testing.T) {
	doc := "## Top\n\n### Bravo\n\ntext\n\n### Alpha\n\nmore\n"
	got := blockSectionOrder(doc)
	if len(got) != 2 {
		t.Fatalf("extracted %d section(s) from the control document, want 2: %v", len(got), got)
	}
	if got["Bravo"] >= got["Alpha"] {
		t.Errorf("the extractor reports Bravo at %d and Alpha at %d — it is not reading document order",
			got["Bravo"], got["Alpha"])
	}
	if len(blockSectionOrder("a document with no headings at all\n")) != 0 {
		t.Error("the extractor invented sections in a document that has none")
	}
}

// ---------------------------------------------------------------------------
// Defect 4 — the template choice is in the table, and it is enumerated
// ---------------------------------------------------------------------------

// TestTheBlockNamesEveryScaffoldTemplateAndTheFlagThatPicksOne is the defect-4
// guard, and it is DERIVED: the wanted set is `scaffold.AllTemplates()`, so a
// fourth template fails here until the block documents it. A literal list would
// re-encode exactly the staleness this is meant to catch.
//
// 🔴 EVERY SHAPE, NOT JUST THE EMPTY ONE. The template already enumerated the
// templates — but only inside the `no project scaffolded here` branch, which an
// author who HAS a project never renders. The reader who needs to know that
// `create` defaults to the heaviest template is the one who is about to create
// their SECOND app from a directory that already holds their first.
//
// 🔴 WATCHED FAIL at origin/main (8603fc3):
//
//	kind "npm": the managed block never names `--template`, and names 0 of the
//	3 scaffold templates …
func TestTheBlockNamesEveryScaffoldTemplateAndTheFlagThatPicksOne(t *testing.T) {
	want := scaffold.AllTemplates()
	if len(want) < 2 {
		t.Fatalf("CONTROL failure, not a finding: scaffold.AllTemplates() returned %d template(s) — "+
			"there is nothing for the block to enumerate", len(want))
	}
	for _, shape := range allProjectShapesForTest() {
		block, err := agentsManagedBlock(shape)
		if err != nil {
			t.Fatalf("rendering for kind %q: %v", shape.Kind, err)
		}
		var missing []string
		for _, tmpl := range want {
			if !strings.Contains(block, string(tmpl)) {
				missing = append(missing, string(tmpl))
			}
		}
		sort.Strings(missing)
		if !strings.Contains(block, "--template") || len(missing) > 0 {
			t.Errorf("kind %q: the managed block %s, and names %d of the %d scaffold templates "+
				"(missing %v).\n  `civitai app create` defaults to `%s` — 37 files, a ~40 KB README and a "+
				"~40 KB App.tsx — and the table used to show only the bare `civitai app create <name>`. "+
				"Reading that scaffold cost 53.5%% of a failed run's carried tokens before any work began.",
				shape.Kind,
				map[bool]string{true: "names `--template`", false: "never names `--template`"}[strings.Contains(block, "--template")],
				len(want)-len(missing), len(want), missing, scaffold.PageMoney)
		}
	}
}

// TestTheDocumentedDefaultTemplateIsTheRealOne ties the sentence to the flag.
//
// The block tells a reader which template `create` picks when they do not
// choose. That is a claim about a default value in app_create.go, and a default
// that changes without the sentence changing turns the fix into a new defect —
// the reader budgets for the wrong scaffold either way.
func TestTheDocumentedDefaultTemplateIsTheRealOne(t *testing.T) {
	root := NewRootCmd()
	c, _, err := root.Find([]string{"app", "create"})
	if err != nil {
		t.Fatalf("app create: %v", err)
	}
	flag := c.Flags().Lookup("template")
	if flag == nil {
		t.Fatal("`app create` has no --template flag — the block documents one that does not exist")
	}
	actual := flag.DefValue
	if actual == "" {
		t.Fatal("CONTROL failure, not a finding: --template reports an empty default, so the assertion " +
			"below would be about nothing")
	}
	for _, shape := range allProjectShapesForTest() {
		block, err := agentsManagedBlock(shape)
		if err != nil {
			t.Fatalf("rendering for kind %q: %v", shape.Kind, err)
		}
		bullets := templateBulletsIn(block)
		named := map[string]bool{}
		var marked []string
		for _, b := range bullets {
			named[b.name] = true
			if strings.Contains(strings.ToLower(b.text), "default") {
				marked = append(marked, b.name)
			}
		}
		if len(named) != len(scaffold.AllTemplates()) {
			t.Fatalf("CONTROL failure, not a finding: kind %q — parsed bullets for %d of the %d scaffold "+
				"templates (%v). The bullet extractor is not reading the list, so the assertion below "+
				"observes nothing", shape.Kind, len(named), len(scaffold.AllTemplates()), sortedKeys(named))
		}
		sort.Strings(marked)
		if len(marked) != 1 || marked[0] != actual {
			t.Errorf("kind %q: `civitai app create --template` defaults to %q, but the block's template "+
				"list marks %v as the default. A reader budgets their context from this sentence; when the "+
				"flag and the sentence disagree, the sentence is the one that gets believed.",
				shape.Kind, actual, marked)
		}
	}
}

// TestPageMoneyReallyIsTheBiggestTemplate pins the one QUANTITATIVE claim the
// block's template paragraph makes.
//
// 🔴 THE CLAIM IS AN ORDERING, SO THE GUARD IS AN ORDERING. The paragraph tells
// a reader that the default is "by a wide margin the largest of the three", and
// that is the fact they act on when they decide what to scaffold. A guard on the
// exact file count or byte size would be a different, more brittle claim — and
// the first edit to any scaffold would make it red for a reason that has nothing
// to do with the advice being wrong. (This very PR moved the count from 37 to 38
// and the README from ~40 KB to ~46 KB.)
func TestPageMoneyReallyIsTheBiggestTemplate(t *testing.T) {
	sizes := map[scaffold.Template]int{}
	for _, tmpl := range scaffold.AllTemplates() {
		dir := filepath.Join(t.TempDir(), "proj")
		if _, err := scaffold.Render(tmpl, dir, scaffold.Data{Slug: "size-probe", Name: "Size Probe"}); err != nil {
			t.Fatalf("scaffold %s: %v", tmpl, err)
		}
		total := 0
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			info, ierr := d.Info()
			if ierr != nil {
				return ierr
			}
			total += int(info.Size())
			return nil
		})
		if err != nil {
			t.Fatalf("measuring %s: %v", tmpl, err)
		}
		sizes[tmpl] = total
	}
	if sizes[scaffold.PageMoney] == 0 {
		t.Fatal("CONTROL failure, not a finding: the page-money scaffold measured 0 bytes — the walk is " +
			"reading nothing and the comparison below is between zeros")
	}
	for tmpl, n := range sizes {
		if tmpl == scaffold.PageMoney {
			continue
		}
		if n >= sizes[scaffold.PageMoney] {
			t.Errorf("the managed block tells a reader that `page-money` is by a wide margin the largest "+
				"template, and %s renders %d bytes against page-money's %d. Either the advice is now wrong "+
				"or the templates have changed shape — a reader budgets their context from that sentence.",
				tmpl, n, sizes[scaffold.PageMoney])
		}
	}
	t.Logf("rendered bytes per template: %v", sizes)
}

// templateBullet is one `- ` + "`<template>`" + ` — …` list entry.
type templateBullet struct{ name, text string }

// templateBulletsIn parses every template bullet in a rendered block, IN ORDER
// and WITHOUT deduplicating, so a test can ask WHICH bullet carries a claim
// instead of grepping the whole document for a word.
//
// 🔴 A SLICE, NOT A MAP, BECAUSE ONE BLOCK CAN CARRY TWO LISTS. The
// `no project scaffolded here` branch renders its own per-template bullets, so a
// map keyed by name silently keeps whichever came last and a claim made in the
// first list disappears.
func templateBulletsIn(block string) []templateBullet {
	known := map[string]bool{}
	for _, t := range scaffold.AllTemplates() {
		known[string(t)] = true
	}
	var out []templateBullet
	const open = "\n- `"
	for i := 0; ; {
		j := strings.Index(block[i:], open)
		if j < 0 {
			return out
		}
		start := i + j + len(open)
		end := strings.Index(block[start:], "`")
		if end < 0 {
			return out
		}
		name := block[start : start+end]
		rest := block[start+end:]
		if k := strings.Index(rest, open); k >= 0 {
			rest = rest[:k]
		}
		if known[name] {
			out = append(out, templateBullet{name: name, text: rest})
		}
		i = start + end
	}
}

// headingSet adapts an offset map for sortedKeys.
func headingSet(m map[string]int) map[string]bool {
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}
