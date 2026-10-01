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
// layer.
//
// ⚠ THE OTHER END IS NOT FIXED YET — stated in the present tense on purpose.
// civitai/civitai-app-starters#499 fixes and gates those examples, and as of
// 2026-09-30 it is still **OPEN**: that repo's `main` still ships the `'SDXL'`
// @example, so the hosted reference an agent fetches still teaches the pinned
// pick. Do not read this comment as a record that the propagation path was
// closed upstream; re-check the PR's state before repeating the claim.
//
// This CLI is the OTHER end of the same reach. `civitai agent-setup` writes the
// managed AGENTS.md block into the author's project, and that block is what an
// agent reads on every turn — earlier than the hosted reference, and whether or
// not it ever fetches it. So the guidance is stated there too, in the Gotchas
// list, in two sentences.
//
// 🔴 THAT OVERRIDES AGENTS.md ITEM 36 ("the CLI ships the address, the docs repo
// ships the contents"), deliberately — operator decision, Zach, 2026-09-30, and
// the named exception is recorded in
// `claudedocs/decisions/36-agents-block-per-project.md` with its measured cost:
// +347 bytes on every rendered block (smallest shape 5,163 -> 5,510 B, +6.7%,
// ~85 tokens), paid on every turn in a file nothing can recall. Do not read this
// bullet as precedent for putting more contents here.
//
// 🔴 THE CORRECT SHAPE IS THE ONE THIS REPO'S OWN SCAFFOLD ALREADY USES.
// `internal/scaffold/templates/page-money/src/App.tsx.tmpl` passes
// `baseModelGroup: checkpoint.baseModel` at both picker call sites — derived
// from the checkpoint the viewer actually chose, never a literal.
//
// The prose is asserted against that template below as a LEDGER over the call
// sites: which ones exist, and what each passes. The precise claim — narrower
// than "they cannot drift apart", which is what the earlier wording here said
// while the code checked only one direction — is that the page-money scaffold
// cannot gain, lose or re-point a picker call site without this package going
// red. It says nothing about the SDK or the host, which live in other repos.

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

// wantPickerCallSites is the LEDGER: every host-picker call site the page-money
// scaffold ships, mapped to whether the guidance REQUIRES that site to carry a
// derived `baseModelGroup`.
//
// Both of these open a picker from a component that already holds a chosen
// checkpoint, which is the one exception the guidance names ("Pass it only when
// the app already holds a chosen checkpoint the pick must match"), so both must
// pass it and both must derive it from that checkpoint.
//
// 🔴 IT FAILS WHEN THE SET SHRINKS *OR* GROWS, and the shrink half is the one
// that was missing. A call site DELETED is how a narrower version of this guard
// stayed green on a scaffold that had stopped following its own advice: it only
// looked at `baseModelGroup:` occurrences that were PRESENT, so removing the
// checkpoint picker's argument removed the evidence along with the behaviour. A
// call site ADDED is the case that needs a decision rather than a default — a
// picker opened where NO checkpoint is held must pass nothing at all, and
// recording it here with `false` is that decision being written down.
var wantPickerCallSites = map[string]bool{
	"openCheckpointPicker": true,
	"openResourcePicker":   true,
}

// pickerCall is one host-picker invocation in the rendered app.
type pickerCall struct {
	name string // the identifier called, e.g. openCheckpointPicker
	args string // its argument object literal, braces included
	line int    // 1-based, so a failure points at the file
}

// declarationKeywords are the tokens that turn `…Picker({` into a DECLARATION
// with destructured parameters rather than a call with an object argument.
//
// 🔴 THIS IS NOT HYPOTHETICAL — the rendered page-money app contains
// `function AccountPicker({`, and without this filter the ledger below reported
// it as a third picker call site. A component taking destructured props is
// spelled identically to a call taking an object literal, so the preceding token
// is the only thing that separates them.
var declarationKeywords = map[string]bool{
	"function": true, "class": true, "const": true, "let": true,
	"var": true, "interface": true, "type": true,
}

// pickerCallsIn finds every `…Picker({ … })` CALL in a rendered App.tsx and
// returns its argument object, BRACE-MATCHED.
//
// A line-oriented scan cannot answer the question this guard asks — "does THIS
// call site pass the field" — because a call spans lines and the field sits on
// one of them. Brace matching is what ties an argument to its call.
func pickerCallsIn(app string) []pickerCall {
	const marker = "Picker({"
	isIdent := func(c byte) bool {
		return c == '_' || c == '$' ||
			(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
	}
	var out []pickerCall
	for i := 0; ; {
		j := strings.Index(app[i:], marker)
		if j < 0 {
			return out
		}
		at := i + j
		nameEnd := at + len("Picker")
		nameStart := at
		for nameStart > 0 && isIdent(app[nameStart-1]) {
			nameStart--
		}
		// Skip a declaration: back over the whitespace, then read the word before.
		before := strings.TrimRight(app[:nameStart], " \t")
		wordStart := len(before)
		for wordStart > 0 && isIdent(before[wordStart-1]) {
			wordStart--
		}
		if declarationKeywords[before[wordStart:]] {
			i = nameEnd
			continue
		}
		brace := nameEnd + 1 // the '(' is at nameEnd, the '{' right after it
		depth, end := 0, -1
		for k := brace; k < len(app); k++ {
			switch app[k] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					end = k + 1
				}
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			// Unbalanced: report what there is rather than silently dropping a
			// call site, which would make the ledger below read as a shrink.
			end = len(app)
		}
		out = append(out, pickerCall{
			name: app[nameStart:nameEnd],
			args: app[brace:end],
			line: strings.Count(app[:nameStart], "\n") + 1,
		})
		i = brace
	}
}

// TestPickerCallExtractorCanFail is the negative control for pickerCallsIn: an
// extractor that returned the same thing for every input, or that stopped at the
// first nested `}`, would make the ledger below vacuous.
func TestPickerCallExtractorCanFail(t *testing.T) {
	const doc = "const x = 1;\n" +
		"await openCheckpointPicker({\n  baseModelGroup: checkpoint.baseModel,\n});\n" +
		"await openResourcePicker({\n  resourceType: 'LORA',\n  opts: { nested: true },\n});\n" +
		"function AccountPicker({ value }: Props) {\n  return null;\n}\n"
	got := pickerCallsIn(doc)
	if len(got) != 2 {
		t.Fatalf("extracted %d call(s) from the control document, want 2: %#v\n\n"+
			"3 means the `function AccountPicker({` declaration was counted as a call — a component "+
			"taking destructured props is spelled exactly like a call taking an object literal, and "+
			"the real page-money scaffold ships one.", len(got), got)
	}
	if got[0].name != "openCheckpointPicker" || got[1].name != "openResourcePicker" {
		t.Errorf("names are %q/%q — the walk back over the identifier is wrong", got[0].name, got[1].name)
	}
	if got[0].line != 2 || got[1].line != 5 {
		t.Errorf("lines are %d/%d, want 2/5", got[0].line, got[1].line)
	}
	if strings.Contains(got[0].args, "openResourcePicker") {
		t.Errorf("the first call's args ran into the second call: %q", got[0].args)
	}
	// The nested object is the case a first-`}` scan gets wrong: it would cut
	// the second call's args at `{ nested: true }` and lose the closing lines.
	if !strings.Contains(got[1].args, "nested: true") || !strings.HasSuffix(got[1].args, "}") {
		t.Errorf("the second call's args are not brace-matched through the nested object: %q", got[1].args)
	}
	if len(pickerCallsIn("no pickers here at all\n")) != 0 {
		t.Error("the extractor invented a call site in a document that has none")
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
// 🔴 THE DESCRIPTION USED TO BE WIDER THAN THE IMPLEMENTATION, which is the
// failure this comment now exists to not repeat. The guard said the advice and
// the code "cannot drift apart" while inspecting only the `baseModelGroup:`
// occurrences that were PRESENT — so DELETING the checkpoint picker's
// `baseModelGroup: checkpoint.baseModel` (App.tsx.tmpl, the `openCheckpointPicker`
// call) left it GREEN: the evidence went out with the behaviour, and the
// surviving premise check is satisfied by the OTHER call site. Measured, not
// reasoned. It now asserts a LEDGER — the set of call sites, and the field at
// each — so a deletion fails as a shrink and an addition fails as an unrecorded
// site.
//
// 🔴 WHAT IT STILL DOES NOT ESTABLISH, stated so it is not assumed: it reads the
// SCAFFOLD, not the SDK and not the host. That `baseModelGroup` is a filter, that
// the host hides what it excludes, and that omitting it is an unconstrained pick
// are facts about `useResourcePicker` and `PageBlockHost.tsx` in other repos,
// which nothing in this package can see. This guard says the scaffold agrees with
// the prose, never that either is right about the platform.
//
// 🔴 LABELLED HONESTLY: THIS ONE IS AN INVARIANT GUARD, NOT A REGRESSION GUARD.
// It is GREEN at the merge-base, because the scaffold was already correct —
// the bug was never here. Do not count it as coverage of the defect this change
// fixes; the regression half is the test above. Its job is forward-looking: it
// fails the first time the scaffold and the advice disagree. Watched fail FOUR
// ways against `App.tsx.tmpl`, each producing this test's own message and no
// other test's:
//
//	deleting line 477 (`baseModelGroup: checkpoint.baseModel`) entirely
//	  -> "openCheckpointPicker … passes NO baseModelGroup"
//	  🔴 this is the mutation the narrower version SURVIVED, and a full
//	     `go test ./...` under it — with this test skipped — is otherwise GREEN,
//	     so nothing else in the repo sees it either
//	a literal `'SDXL'` at that call site -> "passes baseModelGroup a hardcoded ecosystem"
//	deriving it from `someOtherState.family` -> "derives … from something other than"
//	adding an unledgered `openEmbeddingPicker({…})` call -> "LEDGER GREW"
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

	// --- The ledger: which picker call sites exist. ---
	calls := pickerCallsIn(app)
	if len(calls) == 0 {
		t.Fatalf("CONTROL failure, not a finding: pickerCallsIn found no `…Picker({` in the rendered "+
			"page-money app, so every assertion below is about an empty set (%s)", dir)
	}
	seen := map[string]int{}
	for _, c := range calls {
		seen[c.name]++
	}
	for name := range wantPickerCallSites {
		if seen[name] == 0 {
			t.Errorf("LEDGER SHRANK: the page-money scaffold no longer calls %s, which "+
				"wantPickerCallSites says it does.\n\nA call site that disappears takes its "+
				"`baseModelGroup` with it, and a guard that only inspects the arguments that are "+
				"PRESENT goes green on exactly that edit. If the removal is intentional, drop the "+
				"entry from wantPickerCallSites in the same change.", name)
		}
	}
	for name := range seen {
		if _, ok := wantPickerCallSites[name]; !ok {
			t.Errorf("LEDGER GREW: the page-money scaffold calls %s, which is not in "+
				"wantPickerCallSites.\n\nAdd it, with `true` if that call site holds a chosen "+
				"checkpoint the pick must match (so it passes a derived `baseModelGroup`) or `false` "+
				"if it does not (so it passes none at all, which is what the managed AGENTS.md block "+
				"tells the author is the default).", name)
		}
	}

	// --- The field at each ledgered call site. ---
	for _, c := range calls {
		mustCarry, known := wantPickerCallSites[c.name]
		if !known {
			continue // already reported as a ledger growth
		}
		has := strings.Contains(c.args, "baseModelGroup")
		switch {
		case mustCarry && !has:
			t.Errorf("src/App.tsx:%d — %s opens a picker from a component that holds a chosen "+
				"checkpoint, and passes NO baseModelGroup:\n%s\n\nThe managed AGENTS.md block names "+
				"this as the exception: pass it when the app already holds a checkpoint the pick must "+
				"match. A call site that drops it is the scaffold disagreeing with the advice this "+
				"CLI writes into the same author's project.", c.line, c.name, c.args)
		case !mustCarry && has:
			t.Errorf("src/App.tsx:%d — %s passes a baseModelGroup, but wantPickerCallSites records "+
				"that this call site holds no chosen checkpoint:\n%s\n\nThe block's default is an "+
				"UNCONSTRAINED pick; a filter here hides every resource outside one family from the "+
				"viewer.", c.line, c.name, c.args)
		}
	}

	// --- And every occurrence anywhere must be DERIVED, not a literal. ---
	// This half is deliberately whole-file rather than per-call: it also covers a
	// `baseModelGroup` spelled somewhere the call-site scan does not model.
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
