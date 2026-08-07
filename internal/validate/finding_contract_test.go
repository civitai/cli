package validate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// finding_contract_test.go is the guard set for issue #225: `--json` promised a
// `field` on every finding and delivered `null` for every ported semantic check,
// because the field was reverse-engineered from the message at the printer.
//
// THREE guards, and none subsumes the others.
//
//	(A) TestFindingsAreConstructedWithAField — STRUCTURAL. Parses the package and
//	    fails on a construction that cannot carry a field. It fires on the SOURCE,
//	    so it catches a new check nobody wrote a test for.
//	(B) TestEveryCheckEmitsAField — BEHAVIOURAL, with a REACHABILITY LEDGER. Drives
//	    a corpus through the real entry points and asserts every finding carries a
//	    field — and, via the findingSiteHook, that the corpus reached every
//	    finding-producing function the AST finds. Without the ledger a green here
//	    would be indistinguishable from a corpus that never ran the check.
//	(C) TestFindingFieldsUseOneNotation — the SECONDARY defect. Three notations
//	    used to coexist; this pins that exactly one survives.
//
// (A) cannot see a field computed to "" at runtime; (B) cannot see a check no
// fixture reaches that (A) already rejected statically; (C) says nothing about
// presence. Keep all three.

// packageGoFiles returns the non-test .go files of this package.
func packageGoFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	var out []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		out = append(out, n)
	}
	sort.Strings(out)
	if len(out) < 5 {
		// Positive control on the scanner itself: this package has semantic.go,
		// targets.go, warnings.go, lockfile.go, readyack.go, validate.go and
		// finding.go. A short list means we are scanning the wrong directory, and
		// every assertion below would pass vacuously.
		t.Fatalf("scanner found only %d package files (%v) — it is looking at the wrong tree", len(out), out)
	}
	return out
}

// findingSite is one `newFinding(...)` call in the package source.
type findingSite struct {
	file string
	fn   string // enclosing function, closures normalised to their parent
	line int
	// fieldIsEmptyLiteral is set when the first argument is a literal "" — the
	// one shape a static check can call out with certainty.
	fieldIsEmptyLiteral bool
}

// normaliseFuncName reduces a runtime function name to the bare identifier the
// AST scan produces. A closure (`schemaErrors`'s `walk`) is folded into its
// parent, because "which function constructs findings" is the granularity the
// ledger needs and the only one the two sources can agree on.
func normaliseFuncName(name string) string {
	// runtime names look like "github.com/civitai/cli/internal/validate.schemaErrors.func1".
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.Index(name, "."); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.Index(name, ".func"); i >= 0 {
		name = name[:i]
	}
	return name
}

// collectFindingSites parses the package and returns every newFinding call site,
// plus every bare `Finding{...}` composite literal it finds (which must be
// empty outside finding.go).
func collectFindingSites(t *testing.T) (sites []findingSite, compositeLits []string) {
	t.Helper()
	fset := token.NewFileSet()
	for _, name := range packageGoFiles(t) {
		f, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		// Track the enclosing top-level func for each node.
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			fnName := fd.Name.Name
			ast.Inspect(fd, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.CallExpr:
					id, ok := v.Fun.(*ast.Ident)
					if !ok || id.Name != "newFinding" {
						return true
					}
					pos := fset.Position(v.Lparen)
					site := findingSite{file: name, fn: fnName, line: pos.Line}
					if len(v.Args) == 0 {
						t.Errorf("%s:%d: newFinding called with no arguments", name, pos.Line)
						return true
					}
					if lit, ok := v.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING &&
						(lit.Value == `""` || lit.Value == "``") {
						site.fieldIsEmptyLiteral = true
					}
					sites = append(sites, site)
				case *ast.CompositeLit:
					id, ok := v.Type.(*ast.Ident)
					if !ok || id.Name != "Finding" {
						return true
					}
					if name != "finding.go" {
						compositeLits = append(compositeLits,
							fset.Position(v.Lbrace).String())
					}
				}
				return true
			})
		}
	}
	return sites, compositeLits
}

// TestFindingsAreConstructedWithAField is GUARD A — the structural one.
//
// It is the answer to "how does a NEWLY ADDED check fail to ship without a
// field?". It fires at the construction site, in the source, so it does not
// depend on anybody writing a fixture for the new check first.
//
// It enforces two properties:
//
//  1. `Finding{...}` is never written as a composite literal outside finding.go.
//     That is what makes newFinding the single funnel — a struct literal can
//     omit Field and compile, and no amount of arguing about conventions stops
//     the next one.
//  2. No newFinding call passes an empty string literal as the field.
//
// Residual, stated: a field computed to "" at runtime is invisible here. That is
// what GUARD B covers.
func TestFindingsAreConstructedWithAField(t *testing.T) {
	sites, compositeLits := collectFindingSites(t)

	if len(compositeLits) > 0 {
		t.Errorf("Finding{...} composite literal(s) outside finding.go at %v —\n"+
			"every finding must be built by newFinding(field, message) so that "+
			"\"does this check carry a field?\" is a question about one call. "+
			"See finding.go and issue #225.", compositeLits)
	}

	// Positive control on the parser: this package HAS finding sites. A zero here
	// would make every assertion below vacuous, and is exactly what a scanner
	// wired to nothing reports.
	const minSites = 20
	if len(sites) < minSites {
		t.Fatalf("found only %d newFinding call sites; expected at least %d — "+
			"the AST scan is not matching what it should", len(sites), minSites)
	}

	for _, s := range sites {
		if s.fieldIsEmptyLiteral {
			t.Errorf("%s:%d (%s): newFinding called with an EMPTY field — "+
				"every finding must name a manifest location, or one of the documented "+
				"sentinels FieldDocument / FieldProject. See finding.go.", s.file, s.line, s.fn)
		}
	}
}

// TestEveryCheckEmitsAField is GUARD B — behavioural, with a reachability
// ledger.
//
// 🔴 THE LEDGER IS THE POINT. "Every finding I observed carried a field" is a
// claim about the corpus, not about the checks: a corpus that never trips the
// sandbox rule reports a serene pass for a sandbox rule that emits nothing at
// all. So the test ALSO derives, from the AST, the set of functions that
// construct findings, and requires the corpus to have reached every one of
// them. A check added without a fixture fails here by name.
//
// The reached set is observed through findingSiteHook, which is nil in every
// production run.
func TestEveryCheckEmitsAField(t *testing.T) {
	sites, _ := collectFindingSites(t)

	want := map[string]string{} // funcName -> "file:line" of one of its sites
	for _, s := range sites {
		if _, ok := want[s.fn]; !ok {
			want[s.fn] = s.file + ":" + itoaTest(s.line)
		}
	}

	reached := map[string]bool{}
	findingSiteHook = func(fn string) { reached[normaliseFuncName(fn)] = true }
	defer func() { findingSiteHook = nil }()

	var allFindings []Finding
	var corpusNames []string
	for _, fx := range fieldCoverageCorpus() {
		dir := writeFixture(t, fx.files)
		res, err := Dir(dir)
		if err != nil {
			t.Fatalf("%s: Dir: %v", fx.name, err)
		}
		got := append(append([]Finding{}, res.Errors...), res.Warnings...)
		if len(got) == 0 {
			t.Errorf("fixture %q produced NO findings — it is not exercising what it "+
				"claims to, so it contributes nothing to this test", fx.name)
		}
		allFindings = append(allFindings, got...)
		corpusNames = append(corpusNames, fx.name)
	}

	// --- Positive control -------------------------------------------------
	// A reassuring "no finding had an empty field" is indistinguishable from a
	// harness that observed nothing. Prove the harness CAN see fields, and that
	// the number moves: assert a floor on the findings observed AND that a known
	// dotted field and both sentinels actually turned up.
	const minFindings = 25
	if len(allFindings) < minFindings {
		t.Fatalf("corpus produced only %d findings across %d fixtures; expected at least %d — "+
			"the harness is not exercising the checks", len(allFindings), len(corpusNames), minFindings)
	}
	seenField := map[string]bool{}
	for _, f := range allFindings {
		seenField[f.Field] = true
	}
	for _, control := range []string{"iframe.sandbox", "blockId", FieldDocument, FieldProject} {
		if !seenField[control] {
			t.Errorf("positive control: the corpus never produced a finding with field %q, "+
				"so a pass here says nothing about that shape of field", control)
		}
	}

	// --- The actual assertion --------------------------------------------
	for _, f := range allFindings {
		if f.Field == "" {
			t.Errorf("finding with an EMPTY field (issue #225 is back):\n  %s", f.Message)
		}
	}

	// --- The reachability ledger -----------------------------------------
	var missing []string
	for fn, at := range want {
		if !reached[fn] {
			missing = append(missing, fn+" ("+at+")")
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("these finding-producing functions were never reached by the corpus, so this "+
			"test proves nothing about them:\n  %s\n"+
			"Add a fixture to fieldCoverageCorpus() that trips each one.",
			strings.Join(missing, "\n  "))
	}
	// The mirror direction: a function reached but absent from the AST set would
	// mean the scanner missed a construction site.
	for fn := range reached {
		if _, ok := want[fn]; !ok {
			t.Errorf("the corpus reached finding-producer %q, which the AST scan did not find — "+
				"the scanner is missing construction sites", fn)
		}
	}
}

// TestFindingFieldsUseOneNotation pins the SECONDARY defect of issue #225: the
// three notations (`(root)`, JSON Pointer `/blockId`, dotted `iframe.sandbox`)
// are unified on DOTTED.
//
// It asserts the negative — no field is a JSON Pointer — because that is the
// notation that was actually there and would come back if schemaErrors were
// reverted. Asserting only that dotted fields exist would stay green with both
// notations present.
func TestFindingFieldsUseOneNotation(t *testing.T) {
	findingSiteHook = nil
	var all []Finding
	for _, fx := range fieldCoverageCorpus() {
		dir := writeFixture(t, fx.files)
		res, err := Dir(dir)
		if err != nil {
			t.Fatalf("%s: Dir: %v", fx.name, err)
		}
		all = append(all, res.Errors...)
		all = append(all, res.Warnings...)
	}
	if len(all) == 0 {
		t.Fatal("no findings to check the notation of")
	}
	sentinels := map[string]bool{FieldDocument: true, FieldProject: true}
	var sawDotted, sawIndexed bool
	for _, f := range all {
		if sentinels[f.Field] {
			continue
		}
		if strings.Contains(f.Field, "/") {
			t.Errorf("field %q uses JSON Pointer notation; findings are unified on dotted "+
				"paths (issue #225). Message: %s", f.Field, f.Message)
		}
		if strings.HasPrefix(f.Field, "(") {
			t.Errorf("field %q looks like a sentinel but is not one of the two documented "+
				"values (%q, %q)", f.Field, FieldDocument, FieldProject)
		}
		if strings.Contains(f.Field, ".") {
			sawDotted = true
		}
		if strings.Contains(f.Field, "[") {
			sawIndexed = true
		}
	}
	// Positive control: the corpus really does contain both notation shapes, so
	// the negative assertions above had something to be wrong about.
	if !sawDotted {
		t.Error("positive control: no nested (dotted) field in the corpus at all")
	}
	if !sawIndexed {
		t.Error("positive control: no array-indexed field in the corpus at all")
	}
}

// TestFieldPathRendersDottedPaths pins the schema-location renderer directly,
// including the two shapes the JSON-Pointer version got wrong for a reader:
// the root, and an array element.
func TestFieldPathRendersDottedPaths(t *testing.T) {
	cases := []struct {
		tokens []string
		want   string
	}{
		{nil, FieldDocument},
		{[]string{}, FieldDocument},
		{[]string{"blockId"}, "blockId"},
		{[]string{"iframe", "sandbox"}, "iframe.sandbox"},
		{[]string{"scopes", "1"}, "scopes[1]"},
		{[]string{"targets", "0", "slotId"}, "targets[0].slotId"},
		{[]string{"a", "10", "b", "2"}, "a[10].b[2]"},
		// A member name that merely LOOKS numeric-adjacent is not an index.
		{[]string{"v2"}, "v2"},
		{[]string{"page", "1x"}, "page.1x"},
	}
	for _, tc := range cases {
		if got := fieldPath(tc.tokens); got != tc.want {
			t.Errorf("fieldPath(%v) = %q, want %q", tc.tokens, got, tc.want)
		}
	}
}

// TestDedupeFindingsKeysOnTheFieldMessagePAIR pins the semantics the string
// dedupe could not have: the value being deduped grew a second axis, and
// collapsing on either one alone loses real findings.
//
// It is not a restatement of the implementation — each case is a shape the
// checks genuinely produce (scopeJustifications emits one message per key; an
// iframe is routinely wrong in several ways at once).
func TestDedupeFindingsKeysOnTheFieldMessagePAIR(t *testing.T) {
	sameMsgDifferentFields := []Finding{
		{Field: "scopeJustifications.a", Message: "unknown justification key"},
		{Field: "scopeJustifications.b", Message: "unknown justification key"},
	}
	if got := dedupeFindings(sameMsgDifferentFields); len(got) != 2 {
		t.Errorf("two findings with the same message and DIFFERENT fields must both survive, got %d: %v", len(got), got)
	}

	sameFieldDifferentMsgs := []Finding{
		{Field: "iframe.sandbox", Message: "token not allowed"},
		{Field: "iframe.sandbox", Message: "sandbox escape combination"},
	}
	if got := dedupeFindings(sameFieldDifferentMsgs); len(got) != 2 {
		t.Errorf("two findings on the same field with DIFFERENT messages must both survive, got %d: %v", len(got), got)
	}

	exactDupes := []Finding{
		{Field: "iframe.sandbox", Message: "token not allowed"},
		{Field: "iframe.sandbox", Message: "token not allowed"},
		{Field: "iframe.sandbox", Message: "token not allowed"},
	}
	got := dedupeFindings(exactDupes)
	if len(got) != 1 {
		t.Errorf("identical findings must collapse to one, got %d: %v", len(got), got)
	}
	if len(got) == 1 && got[0] != exactDupes[0] {
		t.Errorf("dedupe kept the wrong value: %v", got[0])
	}
}

// TestMessagesPreservesOrderAndText pins the text seam: the human-readable
// renderers print Message and nothing else, so Messages must be a pure
// projection. If this ever needed to compose Field into the line, the byte-for-
// byte human output promise would be the thing that broke.
func TestMessagesPreservesOrderAndText(t *testing.T) {
	in := []Finding{
		{Field: "b", Message: "second"},
		{Field: "a", Message: "first"},
	}
	got := Messages(in)
	if len(got) != 2 || got[0] != "second" || got[1] != "first" {
		t.Errorf("Messages must project Message in order, got %v", got)
	}
	if len(Messages(nil)) != 0 {
		t.Error("Messages(nil) must be empty")
	}
}

func itoaTest(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// The corpus.
// ---------------------------------------------------------------------------

type fieldFixture struct {
	name  string
	files map[string]string
}

func writeFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// goodBase is a manifest that validates clean, used as the starting point for
// fixtures that need exactly one thing wrong.
const goodBase = `{
  "blockId": "cov-block",
  "name": "Coverage Block",
  "version": "1.0.0",
  "description": "a manifest used by the field-coverage corpus",
  "author": "tester",
  "contentRating": "g",
  "renderMode": "iframe",
  "iframe": {"sandbox": "allow-scripts", "minHeight": 400, "resizable": false},
  "entry": "index.html"
}`

// fieldCoverageCorpus returns one fixture per finding-producing site. It is the
// input to the reachability ledger — if a check is added and no fixture here
// trips it, TestEveryCheckEmitsAField names it.
func fieldCoverageCorpus() []fieldFixture {
	return []fieldFixture{
		{
			name:  "no manifest at all (validateDir, FieldDocument)",
			files: map[string]string{"README.md": "nothing here"},
		},
		{
			name:  "unparseable manifest (validateDir, FieldDocument)",
			files: map[string]string{"block.manifest.json": `{"blockId": `},
		},
		{
			name: "schema violations at the root and inside an array (schemaErrors)",
			files: map[string]string{"block.manifest.json": `{
			  "blockId": "Bad_Id",
			  "name": "x",
			  "version": "1.0.0",
			  "scopes": ["user:read:self", "totally-not-a-scope"],
			  "iframe": {"sandbox": "allow-scripts", "minHeight": 400, "resizable": false}
			}`},
		},
		{
			name: "server-owned fields set (serverOwnedFieldChecks)",
			files: map[string]string{"block.manifest.json": `{
			  "blockId": "cov-block",
			  "name": "Coverage Block",
			  "version": "1.0.0",
			  "description": "server-owned fields",
			  "author": "tester",
			  "contentRating": "g",
			  "trustTier": "verified",
			  "iframe": {"src": "https://example.com/x.html", "sandbox": "allow-scripts", "minHeight": 400, "resizable": false}
			}`},
		},
		{
			name: "buildCommand not on the allowlist, and no outputDir (buildCoherence)",
			files: map[string]string{"block.manifest.json": `{
			  "blockId": "cov-block",
			  "name": "Coverage Block",
			  "version": "1.0.0",
			  "description": "bad build command",
			  "author": "tester",
			  "contentRating": "g",
			  "buildCommand": "rm -rf /",
			  "iframe": {"sandbox": "allow-scripts", "minHeight": 400, "resizable": false}
			}`},
		},
		{
			name: "outputDir escapes the project root, with no buildCommand (buildCoherence)",
			files: map[string]string{"block.manifest.json": `{
			  "blockId": "cov-block",
			  "name": "Coverage Block",
			  "version": "1.0.0",
			  "description": "stray outputDir",
			  "author": "tester",
			  "contentRating": "g",
			  "outputDir": "../escape",
			  "iframe": {"sandbox": "allow-scripts", "minHeight": 400, "resizable": false}
			}`},
		},
		{
			name: "outputDir is absolute (buildCoherence)",
			files: map[string]string{"block.manifest.json": `{
			  "blockId": "cov-block",
			  "name": "Coverage Block",
			  "version": "1.0.0",
			  "description": "absolute outputDir",
			  "author": "tester",
			  "contentRating": "g",
			  "buildCommand": "npm run build",
			  "outputDir": "/abs",
			  "iframe": {"sandbox": "allow-scripts", "minHeight": 400, "resizable": false}
			}`},
		},
		{
			name: "buildCommand set with no outputDir (buildCoherence)",
			files: map[string]string{"block.manifest.json": `{
			  "blockId": "cov-block",
			  "name": "Coverage Block",
			  "version": "1.0.0",
			  "description": "build with no output",
			  "author": "tester",
			  "contentRating": "g",
			  "buildCommand": "npm run build",
			  "iframe": {"sandbox": "allow-scripts", "minHeight": 400, "resizable": false}
			}`},
		},
		{
			name: "buildCommand over the length cap (buildCoherence)",
			files: map[string]string{"block.manifest.json": `{
			  "blockId": "cov-block",
			  "name": "Coverage Block",
			  "version": "1.0.0",
			  "description": "long build command",
			  "author": "tester",
			  "contentRating": "g",
			  "outputDir": "dist",
			  "buildCommand": "npm run ` + strings.Repeat("x", 140) + `",
			  "iframe": {"sandbox": "allow-scripts", "minHeight": 400, "resizable": false}
			}`},
		},
		{
			name: "renderMode tier gate, and no iframe block (semanticChecks)",
			files: map[string]string{"block.manifest.json": `{
			  "blockId": "cov-block",
			  "name": "Coverage Block",
			  "version": "1.0.0",
			  "description": "inline render mode",
			  "author": "tester",
			  "contentRating": "g",
			  "renderMode": "inline"
			}`},
		},
		{
			name: "page without an iframe block (semanticChecks)",
			files: map[string]string{"block.manifest.json": `{
			  "blockId": "cov-block",
			  "name": "Coverage Block",
			  "version": "1.0.0",
			  "description": "page without iframe",
			  "author": "tester",
			  "contentRating": "g",
			  "page": {"path": "/", "title": "App"}
			}`},
		},
		{
			name: "iframe missing minHeight and resizable (iframeRequiredFields)",
			files: map[string]string{"block.manifest.json": `{
			  "blockId": "cov-block",
			  "name": "Coverage Block",
			  "version": "1.0.0",
			  "description": "bare iframe",
			  "author": "tester",
			  "contentRating": "g",
			  "iframe": {"sandbox": "allow-scripts"}
			}`},
		},
		{
			name: "iframe minHeight above the ceiling (iframeRequiredFields)",
			files: map[string]string{"block.manifest.json": `{
			  "blockId": "cov-block",
			  "name": "Coverage Block",
			  "version": "1.0.0",
			  "description": "over-tall iframe",
			  "author": "tester",
			  "contentRating": "g",
			  "iframe": {"sandbox": "allow-scripts", "minHeight": 99999, "resizable": false}
			}`},
		},
		{
			name: "sandbox escape combination and a disallowed token (sandboxChecks)",
			files: map[string]string{"block.manifest.json": `{
			  "blockId": "cov-block",
			  "name": "Coverage Block",
			  "version": "1.0.0",
			  "description": "bad sandbox",
			  "author": "tester",
			  "contentRating": "g",
			  "iframe": {"sandbox": "allow-same-origin allow-scripts allow-popups", "minHeight": 400, "resizable": false}
			}`},
		},
		{
			name: "sandbox is whitespace only (sandboxChecks)",
			files: map[string]string{"block.manifest.json": `{
			  "blockId": "cov-block",
			  "name": "Coverage Block",
			  "version": "1.0.0",
			  "description": "empty sandbox",
			  "author": "tester",
			  "contentRating": "g",
			  "iframe": {"sandbox": "   ", "minHeight": 400, "resizable": false}
			}`},
		},
		{
			name: "justification for a scope that is not declared (scopeJustificationChecks)",
			files: map[string]string{"block.manifest.json": `{
			  "blockId": "cov-block",
			  "name": "Coverage Block",
			  "version": "1.0.0",
			  "description": "stray justification",
			  "author": "tester",
			  "contentRating": "g",
			  "scopes": ["user:read:self"],
			  "scopeJustifications": {"models:read:self": "not declared"},
			  "iframe": {"sandbox": "allow-scripts", "minHeight": 400, "resizable": false}
			}`},
		},
		{
			name: "sensitive scope with no justification (sensitiveScopeJustificationChecks)",
			files: map[string]string{"block.manifest.json": `{
			  "blockId": "cov-block",
			  "name": "Coverage Block",
			  "version": "1.0.0",
			  "description": "unjustified sensitive scope",
			  "author": "tester",
			  "contentRating": "g",
			  "scopes": ["buzz:read:self"],
			  "iframe": {"sandbox": "allow-scripts", "minHeight": 400, "resizable": false}
			}`},
		},
		{
			name: "unknown slot and the page slot in targets (targetChecks)",
			files: map[string]string{"block.manifest.json": `{
			  "blockId": "cov-block",
			  "name": "Coverage Block",
			  "version": "1.0.0",
			  "description": "bad targets",
			  "author": "tester",
			  "contentRating": "g",
			  "targets": [
			    {"slotId": "model.not_a_real_slot"},
			    {"slotId": "app.page"}
			  ],
			  "iframe": {"sandbox": "allow-scripts", "minHeight": 400, "resizable": false}
			}`},
		},
		{
			name: "budgeted scope on a page with no per-gen budget (warningChecks)",
			files: map[string]string{"block.manifest.json": `{
			  "blockId": "cov-block",
			  "name": "Coverage Block",
			  "version": "1.0.0",
			  "description": "budgeted page, no budget",
			  "author": "tester",
			  "contentRating": "g",
			  "scopes": ["ai:write:budgeted"],
			  "scopeJustifications": {"ai:write:budgeted": "the app generates images for the viewer"},
			  "page": {"path": "/", "title": "App"},
			  "iframe": {"sandbox": "allow-scripts", "minHeight": 400, "resizable": false}
			}`},
		},
		{
			name: "budgeted scope with no page at all (warningChecks)",
			files: map[string]string{"block.manifest.json": `{
			  "blockId": "cov-block",
			  "name": "Coverage Block",
			  "version": "1.0.0",
			  "description": "budgeted, no page",
			  "author": "tester",
			  "contentRating": "g",
			  "scopes": ["ai:write:budgeted"],
			  "scopeJustifications": {"ai:write:budgeted": "the app generates images for the viewer"},
			  "iframe": {"sandbox": "allow-scripts", "minHeight": 400, "resizable": false}
			}`},
		},
		{
			name: "inert budget: a per-gen budget with no budgeted scope (warningChecks)",
			files: map[string]string{"block.manifest.json": `{
			  "blockId": "cov-block",
			  "name": "Coverage Block",
			  "version": "1.0.0",
			  "description": "inert budget",
			  "author": "tester",
			  "contentRating": "g",
			  "page": {"path": "/", "title": "App", "buzzBudgetPerGen": 40},
			  "iframe": {"sandbox": "allow-scripts", "minHeight": 400, "resizable": false}
			}`},
		},
		{
			name: "package.json with no lockfile (lockfileChecks, hard error)",
			files: map[string]string{
				"block.manifest.json": goodBase,
				"package.json":        `{"name":"cov-block","private":true}`,
			},
		},
		{
			name: "two lockfiles committed (lockfileChecks, advisory)",
			files: map[string]string{
				"block.manifest.json": goodBase,
				"package.json":        `{"name":"cov-block","private":true}`,
				"package-lock.json":   "{}\n",
				"pnpm-lock.yaml":      "lockfileVersion: '9.0'\n",
			},
		},
		{
			name: "a page app whose source never posts the ready ack (readyAckChecks)",
			files: map[string]string{
				"block.manifest.json": `{
				  "blockId": "cov-block",
				  "name": "Coverage Block",
				  "version": "1.0.0",
				  "description": "page app with no ack",
				  "author": "tester",
				  "contentRating": "g",
				  "page": {"path": "/", "title": "App"},
				  "iframe": {"sandbox": "allow-scripts", "minHeight": 400, "resizable": false},
				  "entry": "index.html"
				}`,
				"index.html": `<!doctype html><script src="./app.js"></script>`,
				"app.js":     `document.title = 'hello';`,
			},
		},
	}
}
