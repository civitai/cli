package cmd

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// crBodyModelName is a raw carriage return inside a model NAME — the
// civitai/cli#525 class, written as a Go escape rather than a literal control
// byte (ST1018).
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
// touching the constant was TestReadJSONNoteDoesNotClaimByteIdentity
// (read_help_test.go), which asks a different question — whether the note
// promises byte identity, not whether it describes the repair.
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

// item38CommentedFiles are the files item 38 owns. The citation check below
// runs over exactly these, and it is a LEDGER rather than a glob: a file added
// to the item must be added here to be covered, and a file removed from the
// item makes this list fail rather than silently shrink the check.
var item38CommentedFiles = []string{
	"read_help.go",
	"read_help_test.go",
	"read_json_note_test.go",
	"read_r2_test.go",
}

// TestItem38CommentsCiteTestsThatExist is round 3's finding 6.
//
// 🔴 THE COMMENT WRITTEN TO STOP COMMENTS GOING STALE CITED A TEST THAT HAS
// NEVER EXISTED. TestReadJSONNoteDescribesTheRepair's own doc comment above
// named a guard that appears nowhere in this repository and in no commit of its
// history; the real test is TestReadJSONNoteDoesNotClaimByteIdentity in
// read_help_test.go. The suite was green, the lint run was clean, and a round-2
// audit of this very branch read the sentence without being able to check it.
//
// The dead identifier is spelled out ONCE, as a string literal in the third
// control below and deliberately not in any comment: this test reads comments,
// so writing it in prose here would make the test flag itself.
//
// 🔴 TestReadJSONNoteCommentNamesItsGuard PINS ONLY THE FORWARD DIRECTION —
// that readJSONNote's doc comment CONTAINS the guard's name. A comment in the
// same file naming a test that does not exist is structurally invisible to it,
// which is what happened. This is the reverse direction, and it is the
// deterministic fix: a name is either resolvable or it is not, so nobody has to
// notice.
//
// Resolution is REPO-WIDE, not package-local: comments here legitimately cite
// tests in pkg/civitai (read_r2_test.go names
// TestEscapeJSONStringControlCharsLeavesStructuralWhitespace, which lives
// there), and a package-local resolver would report those as dangling.
//
// 🔴 WHAT IT DOES NOT COVER. The scan is scoped to the files above, not to the
// tree: measured across all of internal/cmd, 31 comment-cited names do not
// resolve — a mix of cross-package citations, hypothetical examples
// (`TestFoo`), and real rot. Widening this to the package is a separate change
// with 31 findings to triage, and pretending otherwise by globbing would make
// the guard permanently red, which is worse than no guard.
func TestItem38CommentsCiteTestsThatExist(t *testing.T) {
	defined := repoTestFuncNames(t)
	// POSITIVE CONTROL on the resolver: a walk that collected nothing declares
	// every citation dangling, and a walk that collected garbage declares none.
	if len(defined) < 1000 {
		t.Fatalf("CONTROL failure, not a finding: found only %d Test functions in the "+
			"module — the resolver is not reading the tree", len(defined))
	}
	if !defined["TestReadJSONNoteDescribesTheRepair"] {
		t.Fatalf("CONTROL failure, not a finding: the resolver cannot see a test " +
			"defined in this very file")
	}
	// NEGATIVE CONTROL: the resolver must NOT resolve the dead name this test
	// exists because of. If it does, the resolver is matching something other
	// than a test-function declaration and every verdict below is worthless.
	const deadCitation = "TestReadJSONNoteWordingMatchesEmitJSON"
	if defined[deadCitation] {
		t.Fatalf("CONTROL failure, not a finding: the resolver claims to have found "+
			"%s, the name this test exists because NOTHING defines. Either it has "+
			"since been written — in which case delete this control — or the "+
			"resolver is matching something that is not a test function declaration.",
			deadCitation)
	}

	fset := token.NewFileSet()
	cited := 0
	for _, name := range item38CommentedFiles {
		f, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("CONTROL failure, not a finding: cannot parse %s: %v", name, err)
		}
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				for _, ident := range testIdentRe.FindAllString(c.Text, -1) {
					cited++
					if defined[ident] {
						continue
					}
					t.Errorf("%s:%d cites %s, which no test function in this module "+
						"declares.\nA comment naming a guard is a POINTER; an unresolvable "+
						"one sends the next reader looking for a test that was never "+
						"written, and reads as coverage while providing none.",
						name, fset.Position(c.Pos()).Line, ident)
				}
			}
		}
	}
	// POSITIVE CONTROL on the SCAN: zero citations found is indistinguishable
	// from zero dangling citations.
	if cited < 5 {
		t.Fatalf("CONTROL failure, not a finding: the scan found only %d cited test "+
			"names across %v — it is not reading the comments", cited, item38CommentedFiles)
	}
}

// testIdentRe matches a Go test-function identifier as written in prose. The
// 4-character tail keeps it off the bare word "Test".
var testIdentRe = regexp.MustCompile(`\bTest[A-Za-z0-9_]{4,}\b`)

// testFuncRe matches a test function DECLARATION at the start of a line.
var testFuncRe = regexp.MustCompile(`(?m)^func (Test[A-Za-z0-9_]*)\s*\(`)

// repoTestFuncNames returns every Test function declared anywhere in the
// module, so a comment may cite a guard that lives in another package.
func repoTestFuncNames(t *testing.T) map[string]bool {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot resolve the module root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("CONTROL failure, not a finding: %s is not the module root "+
			"(no go.mod): %v", root, err)
	}
	out := map[string]bool{}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			n := d.Name()
			if path != root && (strings.HasPrefix(n, ".") || n == "node_modules" ||
				n == "dist" || n == "bin" || n == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for _, m := range testFuncRe.FindAllStringSubmatch(string(src), -1) {
			out[m[1]] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: walking the module: %v", err)
	}
	return out
}
