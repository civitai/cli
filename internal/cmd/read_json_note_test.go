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
	"sort"
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

// item38CommentedFiles are the files item 38 owns, REPO-RELATIVE. The citation
// check below runs over exactly these, and it is a LEDGER rather than a glob: a
// file added to the item must be added here to be covered, and a file removed
// from the item makes this list fail rather than silently shrink the check.
//
// 🔴 IT WAS FOUR FILES, ALL IN internal/cmd, AND THE TWO SETS DISAGREED IN BOTH
// DIRECTIONS. claudedocs/decisions/38's header declares item 38's files; this
// ledger covered four, of which TWO (read_help_test.go, read_r2_test.go) the
// header did not name at all, and NONE of pkg/civitai/read.go,
// pkg/civitai/read_repair_test.go, pkg/civitai/raw_doc_ledger_test.go,
// cmd/civitai/read_error_stderr_test.go or saferune_callers_ledger_test.go.
// Measured consequence: renaming
// TestDeepPagingCapClassifiesOnTheWireMessageNotTheStrippedOne left
// `go test ./...` fully green while pkg/civitai/read.go — item 38's PRIMARY code
// file — and the decision doc both cited a name nothing declared.
// TestItem38FileLedgerMatchesTheDecisionHeader now pins the two sets together,
// so this list cannot drift from the header again in either direction.
var item38CommentedFiles = []string{
	"cmd/civitai/main.go",
	"cmd/civitai/read_error_stderr_test.go",
	"internal/cmd/read_help.go",
	"internal/cmd/read_help_test.go",
	"internal/cmd/read_json_note_test.go",
	"internal/cmd/read_r2_test.go",
	"internal/saferune/saferune.go",
	"pkg/civitai/hashes.go",
	"pkg/civitai/raw_doc_ledger_test.go",
	"pkg/civitai/read.go",
	"pkg/civitai/read_repair_test.go",
	"pkg/civitai/snippet_args_ledger_test.go",
	"saferune_callers_ledger_test.go",
}

// item38DecisionDoc is item 38's evidence file. It is scanned for citations too
// — it cites more guards by name than any single source file does, and a
// dangling name there sends exactly the same reader looking for exactly the same
// missing test. It is NOT in item38CommentedFiles because that list is compared,
// element for element, against the file set this document's own header declares
// — and a document cannot be a member of the list it declares.
const item38DecisionDoc = "claudedocs/decisions/38-read-body-repair-and-snippet.md"

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
// 🔴 WHAT IT DOES NOT COVER, AND THE NUMBER THAT SAYS SO WAS MEASURED WITH THE
// WRONG INSTRUMENT. Round 3 wrote "measured across all of internal/cmd, 31
// comment-cited names do not resolve", listing cross-package citations among
// them — but repoTestFuncNames is repo-WIDE precisely so cross-package citations
// resolve, so that 31 came from a package-local resolver this guard does not
// ship. Re-measured with the instruments below (repoTestFuncNames + testIdentRe)
// over all 471 .go files in the module, at this commit: 2103 citations, 23
// unresolved sites, 20 distinct names. Those are a measurement of this tree, not
// an invariant — the citation total moves with any comment edit.
//
// All 20 were triaged. Each is cited by FILE AND LINE rather than by name: this
// scan reads comments — and, since round 4, the evidence file too — so spelling
// an unresolvable identifier in either would make the guard flag itself. Go and
// look at the lines.
//
// SIXTEEN names at nineteen sites are CORRECT PROSE that this regex cannot tell
// from rot, in five shapes:
//
//   - a name WRAPPED across a comment line-break, or elided with "…", leaving
//     the regex a truncated identifier (internal/scaffold/bootskeleton.go:15,
//     internal/validate/pattern.go:63, internal/cmd/agent_setup_round4_test.go:22,
//     internal/cmd/workflows_list_failure_reason_test.go:217);
//   - a GLOB naming a family (internal/cmd/app_pull_not_approved_test.go:261);
//   - a SUBTEST path, Parent_Sub, which no func declares
//     (internal/genapi/generate_test.go:661);
//   - a fixture identifier quoted from the test's own source
//     (internal/cmd/app_newest_submission_test.go:425, two names);
//   - a deliberate citation of a test that was DELETED, RENAMED or REPLACED —
//     this repo's own convention for recording what a test used to assert
//     (internal/cmd/cmd_test.go:446, internal/cmd/app_listing_test.go:471 and
//     :858, internal/scaffold/slug_test.go:21, internal/cmd/app_status_drift_test.go:522,
//     internal/cmd/agent_setup_round2_test.go:31, agents_evidence_test.go:394,
//     agents_trigger_test.go:78, internal/cmd/app_metrics_test.go:864,
//     internal/cmd/newest_row_pick_test.go:323, internal/appapi/submissions_test.go:114).
//
// FOUR names at four sites are GENUINE ROT — a doc comment or a "pinned by"
// pointer naming something nothing declares. They are listed with their real
// targets as a residual in
// claudedocs/decisions/38-read-body-repair-and-snippet.md, and are NOT fixed
// here: each needs its own function read before its comment can be rewritten,
// and a name-only fix leaves a truer-looking comment that is still wrong.
//
// So the scope limit stays for now, on the corrected number: globbing this check
// today reports 23 sites of which 19 are correct prose. Widening it needs a
// suppression convention for those five shapes first — a separate change, with
// the four rot findings above as its motivation.
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

	root := moduleRoot(t)
	fset := token.NewFileSet()
	goCited := 0
	for _, name := range item38CommentedFiles {
		f, err := parser.ParseFile(fset, filepath.Join(root, name), nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("CONTROL failure, not a finding: cannot parse %s: %v", name, err)
		}
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				for _, ident := range testIdentRe.FindAllString(c.Text, -1) {
					goCited++
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
	// The evidence file is Markdown, so it is scanned as TEXT rather than parsed.
	// Same regex, same resolver, same verdict.
	doc, err := os.ReadFile(filepath.Join(root, item38DecisionDoc))
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot read %s: %v", item38DecisionDoc, err)
	}
	docCited := 0
	for i, line := range strings.Split(string(doc), "\n") {
		for _, ident := range testIdentRe.FindAllString(line, -1) {
			docCited++
			if defined[ident] {
				continue
			}
			t.Errorf("%s:%d cites %s, which no test function in this module declares.\n"+
				"The evidence file names more guards than any source file does; an "+
				"unresolvable one reads as coverage while providing none.",
				item38DecisionDoc, i+1, ident)
		}
	}
	// POSITIVE CONTROLS on the SCAN — one PER ARM, because zero citations found is
	// indistinguishable from zero dangling citations and the two arms read
	// different things with different code.
	//
	// 🔴 THEY WERE ONE COMBINED FLOOR, AND IT COULD NOT SEE THE .go ARM GO TO ZERO.
	// Both arms incremented a single counter floored at 20 while the Markdown arm
	// alone contributes 28, so the .go arm's zero was structurally unobservable:
	// dropping parser.ParseComments from the ParseFile call above left this test
	// green while it read no comments at all from the twelve ledgered files.
	// Measured at this commit: goCited 82, docCited 28. The floors below are
	// lower bounds well under those, not the measurements themselves — the counts
	// move with any comment edit, and a floor pinned to today's exact value would
	// fail on the next one.
	if docCited < 5 {
		t.Fatalf("CONTROL failure, not a finding: the scan found only %d cited test "+
			"names in %s — the Markdown arm is not reading the file", docCited, item38DecisionDoc)
	}
	if goCited < 40 {
		t.Fatalf("CONTROL failure, not a finding: the scan found only %d cited test "+
			"names across %v — the .go arm is not reading the comments",
			goCited, item38CommentedFiles)
	}
}

// item38HeaderPathRe matches a backticked repo-relative .go path in the decision
// file's header paragraph.
var item38HeaderPathRe = regexp.MustCompile("`([A-Za-z0-9_./-]+\\.go)`")

// TestItem38FileLedgerMatchesTheDecisionHeader is round 4's finding 4.
//
// 🔴 TWO DECLARATIONS OF ONE FILE SET, DISAGREEING IN BOTH DIRECTIONS, NEITHER
// ASSERTED. claudedocs/decisions/38's header paragraph names item 38's Code and
// Guards; item38CommentedFiles names the files the citation check reads. The
// ledger covered four files, two of which the header did not list, and missed
// item 38's primary code file — so a comment in pkg/civitai/read.go could, and
// did, cite a test by a name a rename would break, invisibly.
//
// This is the bidirectional assertion. Adding a file to the item means adding it
// in BOTH places or this fails by name.
func TestItem38FileLedgerMatchesTheDecisionHeader(t *testing.T) {
	root := moduleRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, item38DecisionDoc))
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot read %s: %v", item38DecisionDoc, err)
	}
	// The header is the paragraph opening "**Item 38.**", up to the first blank
	// line after it. Scoping to that paragraph is deliberate: the rest of the
	// document names plenty of files it does not OWN.
	lines := strings.Split(string(raw), "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "**Item 38.**") {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("CONTROL failure, not a finding: %s has no line opening "+
			"\"**Item 38.**\" — this test cannot find the declaration it guards",
			item38DecisionDoc)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			end = i
			break
		}
	}
	header := strings.Join(lines[start:end], "\n")
	// POSITIVE CONTROL on the extraction: a one-line header means the paragraph
	// scanner is wrong and every path below would be reported missing.
	if end-start < 3 {
		t.Fatalf("CONTROL failure, not a finding: the Item 38 header extracted to %d "+
			"lines:\n%s", end-start, header)
	}

	declared := map[string]bool{}
	for _, m := range item38HeaderPathRe.FindAllStringSubmatch(header, -1) {
		declared[m[1]] = true
	}
	// POSITIVE CONTROL on the MATCH: an empty set passes a "does the ledger
	// contain everything declared?" check silently.
	if len(declared) == 0 {
		t.Fatalf("CONTROL failure, not a finding: no backticked .go path in the Item 38 "+
			"header:\n%s", header)
	}

	ledgered := map[string]bool{}
	for _, f := range item38CommentedFiles {
		ledgered[f] = true
	}
	var missingFromLedger, missingFromHeader []string
	for p := range declared {
		if !ledgered[p] {
			missingFromLedger = append(missingFromLedger, p)
		}
	}
	for p := range ledgered {
		if !declared[p] {
			missingFromHeader = append(missingFromHeader, p)
		}
	}
	sort.Strings(missingFromLedger)
	sort.Strings(missingFromHeader)
	if len(missingFromLedger) > 0 {
		t.Errorf("%s declares %v as item 38's files, but item38CommentedFiles does not "+
			"cover them — their comments are not citation-checked, which is how a\n"+
			"rename left a dangling reference in pkg/civitai/read.go with the suite green.",
			item38DecisionDoc, missingFromLedger)
	}
	if len(missingFromHeader) > 0 {
		t.Errorf("item38CommentedFiles covers %v, which the Item 38 header in %s does not "+
			"declare.\nEither the header is missing a file the item owns, or the ledger "+
			"has grown past the item — say which, in the header.",
			missingFromHeader, item38DecisionDoc)
	}

	// Every ledgered file must actually exist: a typo silently narrows the scan.
	for _, f := range item38CommentedFiles {
		if _, err := os.Stat(filepath.Join(root, f)); err != nil {
			t.Errorf("item38CommentedFiles names %s, which does not exist: %v", f, err)
		}
	}
}

// moduleRoot resolves the module root from this package's directory and proves
// it by finding go.mod there. Every path the item-38 guards read is relative to
// it, so a wrong root is a broken instrument, not a finding.
func moduleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot resolve the module root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("CONTROL failure, not a finding: %s is not the module root "+
			"(no go.mod): %v", root, err)
	}
	return root
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
	root := moduleRoot(t)
	out := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
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
