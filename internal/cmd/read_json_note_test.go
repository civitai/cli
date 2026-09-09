package cmd

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"strings"
	"testing"
)

// crInAName is a raw carriage return inside a model NAME — the civitai/cli#525
// class, written as a Go escape rather than a literal control byte (ST1018).
// The two halves are distinct from every other fixture in this package so a
// mutant that hardcodes one cannot satisfy an assertion about the other.
const crBodyModelName = "Speckled\rGrebe"

// wantReadJSONNote is readJSONNote, typed out. It is a LITERAL expectation,
// derived from nothing, because a test that rebuilds the note from the same
// pieces the note is built from moves with it and asserts nothing.
const wantReadJSONNote = "--json writes the API response to stdout and nothing else — notes and errors go\n" +
	"to stderr, so `… --json | jq -e .` always parses. The document is the API's,\n" +
	"the bytes are not: it is re-indented on the way out, and a raw control byte the\n" +
	"API emits inside a string (which is not legal JSON) is rewritten as its escape\n" +
	"so the output still parses. Do not diff or hash it against the wire."

// TestReadJSONNoteDescribesTheRepair is the guard finding 1 asked for.
//
// 🔴 A CLAIM NOTHING ASSERTS ON IS UNPINNED BY CONSTRUCTION, AND THAT IS EXACTLY
// HOW THIS ONE WENT FALSE. readJSONNote's doc comment carried a MEASURED claim —
// that a body with a raw C0 byte inside a string exits 1 with empty stdout and
// never reaches emitJSON, so the repair "is not a behaviour of this group".
// civitai/cli#526 moved the repair ahead of the typed decode and every clause of
// that paragraph became false, with the whole suite green: the only test
// touching the constant was TestReadJSONNoteWordingMatchesEmitJSON's
// claimsByteIdentity check, which asks a different question.
//
// So this test does both halves in one place, deliberately:
//
//   - it RE-MEASURES the behaviour through the real command tree, so a future
//     change to where the repair happens goes red here; and
//   - it pins the constant's text VERBATIM against that measurement, so the
//     sentence cannot be reworded away from what was measured.
//
// A cosmetic reword fails this test on purpose. Pay it: when the note changes,
// re-run the measurement below and re-type wantReadJSONNote from what you saw.
func TestReadJSONNoteDescribesTheRepair(t *testing.T) {
	body := "{\"items\":[{\"id\":7714,\"name\":\"" + crBodyModelName + "\",\"type\":\"Checkpoint\"," +
		"\"creator\":{\"username\":\"lark\"},\"stats\":{}}],\"metadata\":{}}"

	// (a) The HUMAN path. The old comment measured exit 1 and empty stdout here.
	setupReadServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	})
	out, _, err := run(t, "models", "search")
	if err != nil {
		t.Fatalf("a body carrying a raw CR inside a name must now succeed, got: %v", err)
	}
	if !strings.Contains(out, "SpeckledGrebe") {
		t.Errorf("the repaired name should render (the CR removed by safeTerm):\n%s", out)
	}

	// (b) The --json path. The old comment said emitJSON is never entered.
	setupReadServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	})
	jsonOut, _, err := run(t, "models", "search", "--json")
	if err != nil {
		t.Fatalf("models search --json over the same body: %v", err)
	}
	if !json.Valid([]byte(jsonOut)) {
		t.Fatalf("--json stdout does not parse — the whole note rests on this:\n%q", jsonOut)
	}
	// The document keeps the control character; the ENCODING is what changed.
	var doc struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &doc); err != nil {
		t.Fatalf("decoding --json stdout: %v", err)
	}
	if len(doc.Items) != 1 || doc.Items[0].Name != crBodyModelName {
		t.Fatalf("--json changed the DOCUMENT, not just the encoding: %+v", doc)
	}
	if !strings.Contains(jsonOut, `\r`) {
		t.Errorf("the escape the note promises is absent from stdout:\n%s", jsonOut)
	}

	// (c) The note, pinned verbatim against what (a) and (b) just measured.
	if readJSONNote != wantReadJSONNote {
		t.Errorf("readJSONNote no longer matches the measured behaviour.\n"+
			"If you changed the WORDING: re-run this test's (a)/(b) measurements and\n"+
			"re-type wantReadJSONNote from what you saw — do not just paste the new note.\n"+
			"If you changed the BEHAVIOUR: (a)/(b) above are what the note claims; fix\n"+
			"the note and its doc comment before this line.\ngot:\n%s\nwant:\n%s",
			readJSONNote, wantReadJSONNote)
	}
}

// TestReadJSONNoteCommentNamesItsGuard keeps the doc comment and the guard
// attached to each other. It does NOT check that the comment is TRUE — no test
// can — but it makes the next person to rewrite the paragraph carry the pointer
// to the thing that re-measures it, which is what was missing when the previous
// paragraph went stale.
func TestReadJSONNoteCommentNamesItsGuard(t *testing.T) {
	const guard = "TestReadJSONNoteDescribesTheRepair"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "read_help.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot parse read_help.go: %v", err)
	}
	found := false
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST || gd.Doc == nil {
			continue
		}
		names := ""
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, n := range vs.Names {
				names += n.Name
			}
		}
		if !strings.Contains(names, "readJSONNote") {
			continue
		}
		found = true
		if !strings.Contains(gd.Doc.Text(), guard) {
			t.Errorf("readJSONNote's doc comment does not name %s, the test that\n"+
				"re-measures what it claims. The previous paragraph named no guard and\n"+
				"was false in every clause for a full release.", guard)
		}
	}
	// Positive control: a parser that matched nothing must fail, not pass with a
	// reassuring zero findings.
	if !found {
		t.Fatalf("CONTROL failure, not a finding: no documented const declaration " +
			"named readJSONNote was found in read_help.go")
	}
}
