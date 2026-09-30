package cmd

import (
	"strings"
	"testing"

	"github.com/civitai/cli/internal/scaffold"
)

// THE HARDCODED PICKER ECOSYSTEM, PINNED IN THE FILE THE AGENT READS FIRST.
//
// 🔴 THE DEFECT WAS NEVER IN THE SDK. `useResourcePicker`'s `baseModelGroup` is
// OPTIONAL, only reaches the wire when non-null, and its own JSDoc already said
// "Omit for an unconstrained pick of the type". There was no default to change.
// What shipped wrong was the TEACHING surface, and it had a propagation path
// that made one line expensive:
//
//	civitai-app-starters useResourcePicker.ts @example
//	  -> generated into developer.civitai.com apps/reference/hooks.md
//	    -> fetched by an AI coding agent building an App Block
//	      -> copied VERBATIM into the app
//
// That @example read `open({ resourceType: 'LORA', baseModelGroup: 'SDXL' })`.
// Weaker models copy an example literally, so apps shipped with the picker
// pinned to one ecosystem. `baseModelGroup` is a FILTER — the host HIDES every
// resource outside the family it is given — so every LoRA a viewer owned
// outside SDXL was invisible to them. That does not present as a wrong filter;
// it presents as "the picker is empty" or "the app is broken", which is the
// expensive kind of defect because it sends the author looking at the wrong
// layer. The starters repo fixed the examples and gated them.
//
// This CLI is the OTHER end of the same reach. `civitai agent-setup` writes the
// managed AGENTS.md block into the author's project, and that block is what an
// agent reads on every turn — earlier than the hosted reference, and whether or
// not it ever fetches it. So the guidance is stated there too, in the Gotchas
// list, in two sentences.
//
// 🔴 THE CORRECT SHAPE IS THE ONE THIS REPO'S OWN SCAFFOLD ALREADY USES.
// `internal/scaffold/templates/page-money/src/App.tsx.tmpl` passes
// `baseModelGroup: checkpoint.baseModel` at both picker call sites — derived
// from the checkpoint the viewer actually chose, never a literal. The prose is
// asserted against that template below, so the advice and the code an author is
// handed cannot drift apart.

// wantPickerEcosystemGuidance is the guidance, WHOLE and normalised.
//
// 🔴 PINNED AS A WHOLE STRING, NOT BY KEYWORDS. A guard on a WORD is walkable by
// REWORDING: `strings.Contains(block, "baseModelGroup")` is satisfied by any
// number of restatements, including one that tells the reader to pass a literal.
// The claim here is prose, and the whole of it is the claim — that the default
// is to pass nothing, that the reason is the hiding, and that the exception is
// a checkpoint the app already holds.
//
// A cosmetic reword now fails this test. That is the price of a machine-readable
// claim: re-read the sentences against the SDK before updating the literal, and
// never weaken this back to a substring check.
const wantPickerEcosystemGuidance = "**Open the resource picker with NO base-model ecosystem by default.** " +
	"`baseModelGroup` is a filter, so a hardcoded one hides every resource outside that family and the " +
	"picker looks empty to the viewer. Pass it only when the app already holds a chosen checkpoint the " +
	"pick must match, and derive it from that checkpoint's own `baseModel`."

// TestTheManagedBlockTeachesTheUnconstrainedResourcePick is the regression guard.
//
// 🔴 IT IS RED AT THE MERGE-BASE, and for a behavioural reason rather than a
// missing symbol: it uses no new production identifier, so the same file run at
// the base commit compiles and fails on the block's CONTENT — the Gotchas list
// there has five bullets and none of them mentions the picker filter.
//
// 🔴 AND IT RUNS ON EVERY TEMPLATE, because the Gotchas section is the
// UNconditional half of the block. The local-dev section branches on what is in
// the directory (item 36); the gotchas do not, and a mis-nested `{{ if }}` that
// moved this bullet into one branch would be exactly the kind of change that
// passes a single-template check.
func TestTheManagedBlockTeachesTheUnconstrainedResourcePick(t *testing.T) {
	for _, tmpl := range []scaffold.Template{scaffold.Static, scaffold.PageVite, scaffold.PageMoney} {
		t.Run(string(tmpl), func(t *testing.T) {
			dir := scaffoldInto(t, tmpl)
			block := normaliseMessage(setupInto(t, dir))

			// INSTRUMENT CONTROL: a block that was never rendered, or one whose
			// markers did not parse, would make the assertion below vacuous in the
			// direction that looks like a pass only if the wanted text were empty —
			// but an empty `want` would match everything, so check both ends.
			if len(block) == 0 {
				t.Fatalf("the managed block for %s is empty — setupInto is wired to nothing", tmpl)
			}
			if wantPickerEcosystemGuidance == "" {
				t.Fatal("wantPickerEcosystemGuidance is empty, so this guard asserts nothing")
			}

			if !strings.Contains(block, wantPickerEcosystemGuidance) {
				t.Errorf("the managed block for a %s project does not carry the picker-ecosystem "+
					"guidance, WHOLE.\n\nThis is the line that stops an agent hardcoding an ecosystem "+
					"into `open()` and hiding every resource outside it from the viewer. It is pinned "+
					"as a complete normalised sentence pair, not by keywords, because a reworded "+
					"version is still a changed claim.\n\nwant: %s\n\nblock:\n%s",
					tmpl, wantPickerEcosystemGuidance, block)
			}
		})
	}
}

// TestThePickerGuidanceMatchesWhatTheScaffoldActuallyDoes is the RELATIONSHIP
// guard, and it is the half that fails when the scaffold moves.
//
// 🔴 A SUBSTRING LEDGER ON ITS OWN PINS THIS FILE'S MEMORY OF THE SCAFFOLD, NOT
// THE SCAFFOLD. The block tells the author to derive the ecosystem from the
// checkpoint the app already holds; `page-money` is the template that ships a
// picker, and it does exactly that at both call sites. If a future edit changed
// the template to pass a literal, the advice above would still be present, still
// whole, and now contradicted by the code this CLI hands the same author.
//
// 🔴 LABELLED HONESTLY: THIS ONE IS AN INVARIANT GUARD, NOT A REGRESSION GUARD.
// It is GREEN at the merge-base, because the scaffold was already correct —
// the bug was never here. Do not count it as coverage of the defect this change
// fixes; the regression half is the test above. Its job is forward-looking: it
// fails the first time the scaffold and the advice disagree. Watched fail by
// mutating `App.tsx.tmpl`'s first call site to a literal `'SDXL'`, which
// produced this test's own "passes baseModelGroup a hardcoded ecosystem"
// message and no other test's.
func TestThePickerGuidanceMatchesWhatTheScaffoldActuallyDoes(t *testing.T) {
	dir := scaffoldInto(t, scaffold.PageMoney)
	app := readFile(t, dir+"/src/App.tsx")

	// PREMISE, read rather than assumed: this template really does open a picker.
	// If page-money ever stops doing so, this guard is measuring nothing and must
	// say so instead of passing.
	if !strings.Contains(app, "baseModelGroup") {
		t.Fatalf("PREMISE BROKEN: the page-money scaffold no longer passes baseModelGroup anywhere, "+
			"so there is no call site for the AGENTS.md guidance to agree with (%s)", dir)
	}

	// Every `baseModelGroup:` in the rendered app must be DERIVED. The character
	// after the colon is what decides: a quote opens a literal, anything else is
	// an expression.
	for i, line := range strings.Split(app, "\n") {
		idx := strings.Index(line, "baseModelGroup")
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(line[idx+len("baseModelGroup"):])
		if !strings.HasPrefix(rest, ":") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(rest, ":"))
		if strings.HasPrefix(value, "'") || strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "`") {
			t.Errorf("src/App.tsx:%d passes baseModelGroup a hardcoded ecosystem: %s\n\n"+
				"The managed AGENTS.md block this CLI writes tells the author to derive it from the "+
				"checkpoint the app already holds. A scaffold that hardcodes one contradicts that "+
				"advice and ships an app whose picker hides every other family from the viewer.",
				i+1, strings.TrimSpace(line))
		}
		if !strings.Contains(value, "checkpoint.baseModel") {
			t.Errorf("src/App.tsx:%d derives baseModelGroup from something other than the chosen "+
				"checkpoint (%s). The guidance names `checkpoint.baseModel`; if the template moved "+
				"to another source, update the guidance in the same change.", i+1, value)
		}
	}
}
