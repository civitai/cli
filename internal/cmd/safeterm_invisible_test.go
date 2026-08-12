package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode"

	"github.com/civitai/cli/internal/genapi"
)

// civitai/cli#393 — THE INVISIBLE AND BIDI CLASS, ASSERTED WHERE A USER WOULD
// SEE IT.
//
// internal/saferune's tests pin the CLASS against a full-Unicode sweep. These
// pin that the class actually reaches the SCREEN — that safeTerm still gates
// every human surface, that `--json` still does not go through it, and that a
// reason which renders as nothing no longer prints an empty parenthetical.
// "Verified in isolation" is the vacuous green this repo keeps rediscovering:
// saferune could be perfect and a surface could have stopped calling it.
//
// 🔴 THE PREDICATE BELOW IS DELIBERATELY NOT saferune.Stripped. A guard that
// asks the implementation what it strips is satisfied by an implementation that
// strips nothing. `unicode.Cf` is the standard library's own answer, and U+2800
// is the measured counterexample from #382, so this is an independently-built
// check that can disagree with the code under test.
func invisibleOrBidiRunes(s string) []string {
	var out []string
	for _, r := range s {
		if unicode.Is(unicode.Cf, r) || r == 0x2800 {
			out = append(out, fmt.Sprintf("U+%04X", r))
		}
	}
	return out
}

// The fixtures. Each names a different half of the hazard so a failure message
// says which one arrived:
//
//   - zwspReason is INVISIBLE: U+200B is not unicode.IsSpace, so strings.Fields
//     sees one token and the padding reads as column alignment it is not.
//   - rloReason REORDERS: U+202E makes the terminal display the rest of the
//     line right-to-left, so what the user reads is not what the bytes say.
const (
	zwspReason = "FIXTURE alpha\u200b\u200b\u200bbeta"
	rloReason  = "FIXTURE gamma\u202edelta\u202c"
	blankOnly  = "\u200b\u2800\u202e"
)

// wfWithReason builds a terminal-status workflow whose single step carries the
// reason as RAW UTF-8 bytes.
//
// 🔴 RAW, NOT `%q`, AND THAT IS THE DIFFERENCE BETWEEN A GUARD AND A DECORATION.
// The first version wrote the reason with `%q`, which spells every invisible
// rune as a JSON \uXXXX escape — seven ASCII bytes. `encoding/json` decodes
// them to the same runes, so every human-surface assertion passed either way…
// and the `--json` guard became VACUOUS: a mutant routing the raw payload
// through safeTerm found no invisible byte to remove and survived the whole
// battery. Only a payload whose bytes really are invisible can tell "not
// filtered" from "nothing there to filter".
//
// `encoding/json` is what does the quoting, and it is the right tool for
// exactly this reason: it escapes only what JSON cannot carry literally — the
// C0 controls, a quote, a backslash — and leaves every rune in the invisible
// class as RAW BYTES, which is the property the guard needs. `%q` would have
// escaped those too, which is the trap above.
func wfWithReason(reason string) string {
	q, err := json.Marshal(reason)
	if err != nil {
		panic(err)
	}
	return `{"id":"wf_123","status":"failed","createdAt":"2026-08-05T12:00:00Z","steps":[
	  {"$type":"imageGen","name":"$0","status":"failed","metadata":{},
	   "output":{"images":[{"id":"out_1","type":"image","available":false}],
	   "errors":[` + string(q) + `]}}]}`
}

// --- the gate itself ----------------------------------------------------------

func TestSafeTerm_StripsTheInvisibleAndBidiClass(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"zero width space", "a\u200bb", "ab"},
		{"zero width joiner", "a\u200db", "ab"},
		{"word joiner", "a\u2060b", "ab"},
		{"soft hyphen", "a\u00adb", "ab"},
		{"byte order mark", "a\ufeffb", "ab"},
		{"right-to-left override", "a\u202eb", "ab"},
		{"left-to-right isolate", "a\u2066b\u2069", "ab"},
		{"braille pattern blank", "a\u2800b", "ab"},
		{"hangul filler", "a\u3164b", "ab"},
		{"still keeps C0 behaviour", "a\x1b[2Kb", "a[2Kb"},
		{"still keeps newline and tab", "a\tb\nc", "a\tb\nc"},
		{"leaves right-to-left SCRIPT alone", "שלום", "שלום"},
		{"leaves CJK and emoji alone", "日本語 🚀", "日本語 🚀"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := safeTerm(tc.in); got != tc.want {
				t.Errorf("safeTerm(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// --- every surface that prints server free text -------------------------------

// 🔴 FOUR SURFACES, BECAUSE A GUARD ON ONE OF THEM IS A GUARD ON ONE OF THEM.
// #367's first round mutation-tested a single site and two others survived
// dropping safeTerm entirely. Each case here drives a different renderer, and
// each carries a positive control so a green cannot come from nothing being
// printed.
func TestServerTextSurfaces_EmitNoInvisibleOrBidiRune(t *testing.T) {
	for _, reason := range []string{zwspReason, rloReason} {
		t.Run("workflows get reason block", func(t *testing.T) {
			c, out, _ := genCmd("")
			if err := runWorkflowsGet(c, wfGetDeps(wfWithReason(reason), nil),
				workflowsGetOpts{baseURL: "https://civitai.com"}, "wf_123"); err != nil {
				t.Fatalf("workflows get: %v", err)
			}
			assertNoInvisibleRunes(t, "the `workflows get` reason block", out.String())
		})

		t.Run("excluded-output line", func(t *testing.T) {
			var b bytes.Buffer
			reportExcludedOutputs(&b, []genapi.Output{
				{Blob: genapi.Blob{ID: "out_1"}, StepErrors: []string{reason}},
			}, false, nil)
			assertNoInvisibleRunes(t, "the excluded-output line", b.String())
		})

		t.Run("workflows list reason lines", func(t *testing.T) {
			stdout, _ := renderList(t, listPage(listRow("wf_bad", "failed",
				`["`+reason+`"]`)))
			assertNoInvisibleRunes(t, "`workflows list` stdout", stdout)
		})

		t.Run("the generate dead-end error", func(t *testing.T) {
			msg := runToTerminal(t, wfWithReason(reason)).Error()
			assertNoInvisibleRunes(t, "the `generate` error", msg)
		})
	}
}

// assertNoInvisibleRunes is the paired predicate: the class is gone AND the
// words arrived, so a green cannot come from the whole string being dropped.
func assertNoInvisibleRunes(t *testing.T, surface, got string) {
	t.Helper()
	if bad := invisibleOrBidiRunes(got); len(bad) > 0 {
		t.Errorf("%s put %s on the terminal. An invisible rune is a separator no wrapper splits on; a bidi "+
			"control changes the order the line is DISPLAYED in — what the user reads is then not what the "+
			"bytes say (civitai/cli#393):\n%q", surface, strings.Join(bad, ", "), got)
	}
	for _, want := range []string{"FIXTURE"} {
		if !strings.Contains(got, want) {
			t.Errorf("CONTROL failure, not a finding: %s never rendered the reason, so the check above is "+
				"vacuous: %q", surface, got)
		}
	}
}

// --- `--json` is a raw passthrough and must NOT be filtered --------------------

// 🔴 THE PAIR, NOT THE ABSENCE. Asserting only that the human path strips the
// runes says nothing about `--json`; asserting only that `--json` keeps them
// says nothing about the human path. Both branches run over ONE payload, so the
// test cannot be satisfied by a build in which neither branch does anything.
//
// internal/ui/CONVENTION.md rule 1: machine-readable output emits zero styling
// and is not sanitised — control characters are already \uXXXX-escaped by
// spec-compliant JSON, and filtering would corrupt bytes a script is parsing.
//
// 🔴 THE ASSERTION IS ON THE DECODED VALUE, NOT ON THE BYTES, AND THAT IS THE
// ONLY HONEST SPELLING. A JSON \u200b escape and a raw U+200B are the SAME JSON
// string, so `strings.ContainsRune` over the emitted bytes answers a question
// about which spelling the SERVER chose, not about what the CLI passed on — the
// first version of this test failed for exactly that reason. What a script sees
// is the decoded value, so that is what is checked, and the whole payload is
// compared field-by-field below.
func TestWorkflowsGetJSON_KeepsWhatTheHumanPathStrips(t *testing.T) {
	payload := wfWithReason(zwspReason + rloReason)

	c, jsonOut, _ := genCmd("")
	if err := runWorkflowsGet(c, wfGetDeps(payload, nil), workflowsGetOpts{jsonOut: true}, "wf_123"); err != nil {
		t.Fatalf("workflows get --json: %v", err)
	}
	got := jsonOut.String()
	if !strings.Contains(got, "FIXTURE alpha") {
		t.Fatalf("CONTROL failure, not a finding: `--json` emitted no payload at all:\n%s", got)
	}
	// `workflows get --json` is a raw passthrough of the server's PAYLOAD — it
	// re-indents, so the claim is semantic and not byte-for-byte: every field,
	// including ones the CLI does not model, decodes to exactly what arrived.
	var gotDoc, wantDoc any
	if err := json.Unmarshal([]byte(got), &gotDoc); err != nil {
		t.Fatalf("CONTROL failure, not a finding: `--json` emitted invalid JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(payload), &wantDoc); err != nil {
		t.Fatalf("fixture is not valid JSON: %v", err)
	}
	if !reflect.DeepEqual(gotDoc, wantDoc) {
		t.Errorf("`workflows get --json` altered the server payload:\n got: %s\nwant: %s", got, payload)
	}
	assertJSONReasonKeepsTheClass(t, "workflows get --json", got)

	// The other half of the pair, same payload through the human renderer.
	c2, human, _ := genCmd("")
	if err := runWorkflowsGet(c2, wfGetDeps(payload, nil),
		workflowsGetOpts{baseURL: "https://civitai.com"}, "wf_123"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	assertNoInvisibleRunes(t, "the `workflows get` reason block", human.String())
}

// The same pair for `workflows list`, whose `--json` path is a different
// function reading a different wire shape (and which re-marshals, so only the
// decoded value is a contract there).
func TestWorkflowsListJSON_KeepsWhatTheHumanPathStrips(t *testing.T) {
	payload := listPage(listRow("wf_bad", "failed", `["`+zwspReason+`"]`))

	c, out, _ := genCmd("")
	if err := runWorkflowsList(c, wfListDeps(payload, nil, nil, nil), workflowsListOpts{jsonOut: true}); err != nil {
		t.Fatalf("workflows list --json: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "FIXTURE alpha") {
		t.Fatalf("CONTROL failure, not a finding: `--json` emitted no payload at all:\n%s", got)
	}
	assertJSONReasonKeepsTheClass(t, "workflows list --json", got)

	human, _ := renderList(t, payload)
	assertNoInvisibleRunes(t, "`workflows list` stdout", human)
}

// assertJSONReasonKeepsTheClass decodes the emitted JSON and asserts every
// reason string still carries the runes the human path removes. It walks the
// decoded tree rather than naming a path, so it works for both wire shapes.
func assertJSONReasonKeepsTheClass(t *testing.T, surface, emitted string) {
	t.Helper()
	var doc any
	if err := json.Unmarshal([]byte(emitted), &doc); err != nil {
		t.Fatalf("CONTROL failure, not a finding: %s emitted invalid JSON (%v):\n%s", surface, err, emitted)
	}
	var found []string
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case string:
			if strings.Contains(t, "FIXTURE") {
				found = append(found, t)
			}
		case []any:
			for _, e := range t {
				walk(e)
			}
		case map[string]any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(doc)
	if len(found) == 0 {
		t.Fatalf("CONTROL failure, not a finding: %s carried no reason string at all, so the checks below are "+
			"vacuous:\n%s", surface, emitted)
	}
	for _, s := range found {
		if !strings.ContainsRune(s, 0x200b) {
			t.Errorf("%s: the decoded reason lost U+200B. `--json` is a raw passthrough — a script must see "+
				"what the server sent, and sanitising it would corrupt machine-readable output "+
				"(internal/ui/CONVENTION.md rule 1): %q", surface, s)
		}
	}
}

// --- the empty parenthetical (the internal/genapi half) -----------------------

// 🔴 END-TO-END FOR THE OTHER HALF OF #393. `hasPrintableContent` used
// unicode.IsControl, which is Cc-only, so a reason made entirely of invisible
// runes counted as CONTENT: the heading printed with nothing under it, and on
// the excluded-output line it displaced the categorical fallback with
// `(the server reported: )`.
//
// Both directions are asserted. The positive control is a reason that differs
// from the fixture by ONE visible rune — without it, "no heading" is equally
// satisfied by a renderer that has stopped printing reasons at all.
func TestWorkflowsGet_AReasonThatRendersAsNothingIsNotAReason(t *testing.T) {
	c, out, _ := genCmd("")
	if err := runWorkflowsGet(c, wfGetDeps(wfWithReason(blankOnly), nil),
		workflowsGetOpts{baseURL: "https://civitai.com"}, "wf_123"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "wf_123") {
		t.Fatalf("CONTROL failure, not a finding: the workflow did not render at all:\n%s", got)
	}
	if strings.Contains(got, "The server reported:") {
		t.Errorf("a reason made only of invisible runes printed a heading with nothing under it:\n%q", got)
	}

	// POSITIVE CONTROL: one visible rune more, and the heading is back.
	c2, out2, _ := genCmd("")
	if err := runWorkflowsGet(c2, wfGetDeps(wfWithReason(blankOnly+"x"), nil),
		workflowsGetOpts{baseURL: "https://civitai.com"}, "wf_123"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	if !strings.Contains(out2.String(), "The server reported:") {
		t.Errorf("CONTROL failure, not a finding: a reason carrying a visible rune printed no heading either, "+
			"so the absence above is not about emptiness:\n%q", out2.String())
	}
}

// The same rule on the excluded-output line, where the empty parenthetical
// DISPLACED a sentence that said more.
//
// 🔴 IT IS DRIVEN THROUGH THE PAYLOAD, NOT BY HAND-SETTING StepErrors. The drop
// happens in genapi.dedupeReasons, so an Output built directly in a test with
// `StepErrors: []string{blankOnly}` is a state the wire cannot produce — and a
// direct call therefore tests the renderer against an impossible input and
// fails for a reason nobody has. This goes through the command, which is where
// the seam between the two packages actually is.
func TestWorkflowsGet_AnInvisibleReasonFallsBackToTheCategoricalSentence(t *testing.T) {
	c, _, errb := genCmd("")
	if err := runWorkflowsGet(c, wfGetDeps(wfWithReason(blankOnly), nil),
		workflowsGetOpts{baseURL: "https://civitai.com"}, "wf_123"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	got := errb.String()
	if !strings.Contains(got, "out_1") {
		t.Fatalf("CONTROL failure, not a finding: the excluded output did not render:\n%s", got)
	}
	if strings.Contains(got, "reported: )") {
		t.Errorf("an all-invisible reason produced an empty parenthetical, which says strictly less than the "+
			"categorical sentence it displaced:\n%q", got)
	}
	if !strings.Contains(got, "the job finished without producing a usable file") {
		t.Errorf("the categorical fallback did not take over from the empty reason:\n%q", got)
	}

	// POSITIVE CONTROL: the same fixture plus one visible rune still reports the
	// server's account, so the fallback above is about emptiness and not about
	// reasons having stopped reaching this line.
	c2, _, errb2 := genCmd("")
	if err := runWorkflowsGet(c2, wfGetDeps(wfWithReason(blankOnly+"FIXTURE x"), nil),
		workflowsGetOpts{baseURL: "https://civitai.com"}, "wf_123"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	if !strings.Contains(errb2.String(), "FIXTURE x") {
		t.Errorf("CONTROL failure, not a finding: a reason with a visible rune did not reach the "+
			"excluded-output line either:\n%q", errb2.String())
	}
}
