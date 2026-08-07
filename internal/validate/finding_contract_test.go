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
//
// 🔴 AND NONE OF THE THREE PINS THE FIELD'S VALUE — every one of them asks only
// "non-empty / right notation". That is a fourth question, and it lives in
// finding_fields_test.go, which exists because three mutants moving a field to a
// WRONG BUT PLAUSIBLE value survived all of the above with the suite green.
//
// (A)'s own scanner is validated by TestFindingFunnelScannerSeesEveryLiteralSpelling,
// because (A)'s green is a claim about the spellings it can SEE — and that claim
// was false for the one spelling `gofmt -s` produces.

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
	// fn is the enclosing function, closures normalised to their parent. It is
	// EMPTY for a construction outside any function body (a package-level `var`
	// initialiser), which Guard A rejects outright — see below.
	fn   string
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

// findingLitDepth reports how many levels of COMPOSITE-LITERAL NESTING separate
// a type expression from a `Finding` value:
//
//	Finding                            -> 0  (a literal of this type IS a Finding)
//	*Finding                           -> 0  (`&Finding{…}` / an elided `{…}`)
//	[]Finding / [3]Finding / map[k]Finding -> 1  (its ELEMENTS are Findings)
//	[][]Finding                        -> 2
//	anything else                      -> -1
//
// 🔴 THE DEPTH IS THE WHOLE POINT, AND IT IS WHY THE FIRST VERSION OF THIS GUARD
// WAS DISARMED BY `make fmt`. An element written with its type elided —
// `[]Finding{{Message: "x"}}` — is an `*ast.CompositeLit` whose `Type` is NIL, so
// a scanner that asks the literal for its own type finds none and skips it. That
// is not an exotic spelling: `gofmt -s` REWRITES the caught form
// `[]Finding{Finding{…}}` into exactly that uncaught one, and this repo runs
// `gofmt -s -w .` in `make fmt` and enforces `gofmt -s -l .` in CI. So the
// repo's own formatter converted every literal the guard could see into one it
// could not. The element type therefore has to come from the ENCLOSING literal,
// which is what this depth carries.
func findingLitDepth(t ast.Expr) int {
	switch e := t.(type) {
	case *ast.Ident:
		if e.Name == "Finding" {
			return 0
		}
	case *ast.StarExpr:
		return findingLitDepth(e.X)
	case *ast.ArrayType:
		if d := findingLitDepth(e.Elt); d >= 0 {
			return d + 1
		}
	case *ast.MapType:
		if d := findingLitDepth(e.Value); d >= 0 {
			return d + 1
		}
	}
	return -1
}

// findingScan accumulates one package's worth of scan results.
type findingScan struct {
	t     *testing.T
	fset  *token.FileSet
	file  string
	sites []findingSite
	lits  []string
	seen  map[string]bool
}

func (sc *findingScan) flagLit(pos token.Pos, what string) {
	if sc.file == "finding.go" {
		return // the one authority, where the literal is allowed to live
	}
	at := sc.fset.Position(pos).String()
	if sc.seen[at] {
		return
	}
	sc.seen[at] = true
	sc.lits = append(sc.lits, at+" ("+what+")")
}

// checkComposite flags a composite literal that constructs Finding(s). depth is
// the element depth of the CONTEXT the literal appears in (see findingLitDepth):
// 0 means this literal is itself a Finding, >0 means its elements are.
func (sc *findingScan) checkComposite(cl *ast.CompositeLit, depth int) {
	if depth < 0 {
		return
	}
	if depth == 0 {
		what := "Finding{…} composite literal"
		if cl.Type == nil {
			what = "elided-type Finding literal `{…}` inside a []Finding/map literal — the form `gofmt -s` produces"
		}
		sc.flagLit(cl.Lbrace, what)
		return
	}
	for _, el := range cl.Elts {
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			el = kv.Value
		}
		inner, ok := el.(*ast.CompositeLit)
		if !ok {
			continue // a newFinding(...) call, an identifier, anything else
		}
		if inner.Type != nil {
			continue // it names its own type; the walk visits it in its own right
		}
		sc.checkComposite(inner, depth-1)
	}
}

// findingWalker carries the enclosing function name down the AST. It is a VALUE
// so a nested func gets its own name without mutating the parent's.
type findingWalker struct {
	sc *findingScan
	fn string // "" == not inside any function body
}

func (w findingWalker) Visit(n ast.Node) ast.Visitor {
	switch v := n.(type) {
	case *ast.FuncDecl:
		// A method is named by its own identifier; the ledger normalises the
		// runtime name to the same thing.
		return findingWalker{sc: w.sc, fn: v.Name.Name}
	case *ast.CompositeLit:
		if v.Type != nil {
			w.sc.checkComposite(v, findingLitDepth(v.Type))
		}
	case *ast.CallExpr:
		id, ok := v.Fun.(*ast.Ident)
		if !ok || id.Name != "newFinding" {
			break
		}
		pos := w.sc.fset.Position(v.Lparen)
		site := findingSite{file: w.sc.file, fn: w.fn, line: pos.Line}
		if len(v.Args) == 0 {
			w.sc.t.Errorf("%s:%d: newFinding called with no arguments", w.sc.file, pos.Line)
			break
		}
		if lit, ok := v.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING &&
			(lit.Value == `""` || lit.Value == "``") {
			site.fieldIsEmptyLiteral = true
		}
		w.sc.sites = append(w.sc.sites, site)
	}
	return w
}

// scanFindingSource parses one file — from disk when src is nil, otherwise from
// src — and returns its newFinding sites plus every Finding-constructing
// composite literal in it.
//
// It is factored out of collectFindingSites so the scanner itself can be driven
// by a CONTROL CORPUS of known-good and known-bad spellings
// (TestFindingFunnelScannerSeesEveryLiteralSpelling). A funnel guard that cannot
// be shown to go red on the spelling it exists to catch is a claim about the
// guard, not about the package.
func scanFindingSource(t *testing.T, fset *token.FileSet, name string, src any) ([]findingSite, []string) {
	t.Helper()
	f, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	sc := &findingScan{t: t, fset: fset, file: name, seen: map[string]bool{}}
	ast.Walk(findingWalker{sc: sc}, f)
	return sc.sites, sc.lits
}

// collectFindingSites parses the package and returns every newFinding call site,
// plus every composite literal that constructs a Finding (which must be none
// outside finding.go).
//
// 🔴 IT WALKS THE WHOLE FILE, NOT ONLY `*ast.FuncDecl`s. An earlier version
// iterated `f.Decls` and skipped everything that was not a function, so a
// package-level `var x = newFinding("", …)` was invisible to BOTH guards: Guard
// A never saw the empty field, and Guard B never listed it in the reachability
// ledger. Such a site is now collected with an EMPTY `fn` and rejected by name.
func collectFindingSites(t *testing.T) (sites []findingSite, compositeLits []string) {
	t.Helper()
	fset := token.NewFileSet()
	for _, name := range packageGoFiles(t) {
		fileSites, fileLits := scanFindingSource(t, fset, name, nil)
		sites = append(sites, fileSites...)
		compositeLits = append(compositeLits, fileLits...)
	}
	return sites, compositeLits
}

// TestFindingsAreConstructedWithAField is GUARD A — the structural one.
//
// It is the answer to "how does a NEWLY ADDED check fail to ship without a
// field?". It fires at the construction site, in the source, so it does not
// depend on anybody writing a fixture for the new check first.
//
// It enforces three properties:
//
//  1. No composite literal outside finding.go CONSTRUCTS a Finding — in ANY
//     spelling. That is what makes newFinding the single funnel: a struct
//     literal can omit Field and compile, and no amount of arguing about
//     conventions stops the next one.
//
//     🔴 "IN ANY SPELLING" IS LOAD-BEARING, AND SAYING LESS THAN THAT IS HOW
//     THIS GUARD WAS ONCE DISARMED BY `make fmt`. The first version matched
//     only a literal that names its own type (`Finding{…}`), and `gofmt -s`
//     rewrites `[]Finding{Finding{…}}` into the ELIDED `[]Finding{{…}}`, whose
//     `Type` is nil — so the repo's own formatter, run by `make fmt` and
//     enforced by CI's `gofmt -s -l .`, converted the one caught form into an
//     uncaught one. Measured: a new check returning `[]Finding{{Message: …}}`
//     with no Field, wired into the real pipeline and with NO corpus fixture,
//     passed the entire suite. Do not narrow this back to a form; see
//     findingLitDepth, and TestFindingFunnelScannerSeesEveryLiteralSpelling for
//     the spellings both directions are pinned on.
//
//  2. No newFinding call passes an empty string literal as the field.
//
//  3. Every construction sits INSIDE a function, so Guard B's reachability
//     ledger can attribute it. A package-level `var x = newFinding(…)` runs at
//     init, before any test can install the hook, and was invisible to both
//     guards.
//
// Residual, stated: a field computed to "" at runtime is invisible here. That is
// what GUARD B covers. So is a Finding built without a literal at all (`var f
// Finding; f.Message = …`), which no spelling-based funnel can see.
func TestFindingsAreConstructedWithAField(t *testing.T) {
	sites, compositeLits := collectFindingSites(t)

	if len(compositeLits) > 0 {
		t.Errorf("Finding-constructing composite literal(s) outside finding.go at %v —\n"+
			"every finding must be built by newFinding(field, message) so that "+
			"\"does this check carry a field?\" is a question about one call. "+
			"See finding.go and issue #225.", compositeLits)
	}

	for _, s := range sites {
		if s.fn == "" {
			t.Errorf("%s:%d: newFinding called OUTSIDE any function body (a package-level "+
				"initialiser) — construct findings inside the check that reports them, or "+
				"Guard B's reachability ledger cannot attribute them and the site is "+
				"unobserved by every test in this file.", s.file, s.line)
		}
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

// TestFindingFunnelScannerSeesEveryLiteralSpelling VALIDATES THE INSTRUMENT
// Guard A reads its verdict from.
//
// 🔴 Guard A's green is a claim about the SPELLINGS ITS SCANNER CAN SEE, and
// that claim was FALSE for the one spelling `gofmt -s` produces. So the scanner
// is driven here against a corpus of synthetic sources with a known answer:
// every literal form that constructs a Finding must be flagged, and the forms
// that do not construct one must NOT be (a guard that flagged everything would
// be equally useless and equally green on the real tree).
//
// The two rows that matter most are the gofmt PAIR: `[]Finding{Finding{…}}` is
// what a person writes, `[]Finding{{…}}` is what `make fmt` leaves behind, and
// the second is the form that shipped a fieldless finding past the whole suite.
// If you ever change findingLitDepth or checkComposite, this table is the thing
// that says whether you narrowed the guard.
func TestFindingFunnelScannerSeesEveryLiteralSpelling(t *testing.T) {
	const preamble = "package validate\n\ntype Finding struct{ Field, Message string }\n\nfunc newFinding(a, b string) Finding { panic(\"stub\") }\n"

	cases := []struct {
		name     string
		body     string
		wantLits int
		// wantSites is the number of newFinding calls, and wantFnEmpty the
		// number of them attributed to no function (a package-level init).
		wantSites   int
		wantFnEmpty int
	}{
		{
			name:     "elided element in a []Finding literal (what `gofmt -s` produces)",
			body:     "func c() []Finding { return []Finding{{Message: \"x\"}} }",
			wantLits: 1,
		},
		{
			name:     "explicitly typed element in a []Finding literal (what a person writes)",
			body:     "func c() []Finding { return []Finding{Finding{Message: \"x\"}} }",
			wantLits: 1,
		},
		{
			name:     "bare Finding literal",
			body:     "func c() Finding { return Finding{Field: \"a\"} }",
			wantLits: 1,
		},
		{
			name:     "address-of a Finding literal",
			body:     "func c() *Finding { return &Finding{} }",
			wantLits: 1,
		},
		{
			name:     "elided element in a []*Finding literal",
			body:     "func c() []*Finding { return []*Finding{{Message: \"x\"}} }",
			wantLits: 1,
		},
		{
			name:     "elided value in a map[string]Finding literal",
			body:     "func c() map[string]Finding { return map[string]Finding{\"k\": {Message: \"x\"}} }",
			wantLits: 1,
		},
		{
			name:     "two elided elements are two literals",
			body:     "func c() []Finding { return []Finding{{Message: \"a\"}, {Message: \"b\"}} }",
			wantLits: 2,
		},
		{
			name:     "elided element nested two deep",
			body:     "func c() [][]Finding { return [][]Finding{{{Message: \"x\"}}} }",
			wantLits: 1,
		},
		{
			name:     "a var declaration is not a hiding place",
			body:     "func c() { var f = []Finding{{Message: \"x\"}}; _ = f }",
			wantLits: 1,
		},
		// --- the NEGATIVE side: these must NOT be flagged ------------------
		{
			name:      "the sanctioned funnel",
			body:      "func c() []Finding { return []Finding{newFinding(\"a\", \"b\")} }",
			wantSites: 1,
		},
		{
			name: "an unrelated composite literal",
			body: "func c() []string { return []string{\"x\"} }",
		},
		{
			name:      "a struct that merely CONTAINS findings",
			body:      "type R struct{ E []Finding }\n\nfunc c() R { return R{E: []Finding{newFinding(\"a\", \"b\")}} }",
			wantSites: 1,
		},
		// --- attribution ---------------------------------------------------
		{
			name:      "a closure is attributed to its enclosing function",
			body:      "func outer() []Finding { f := func() Finding { return newFinding(\"a\", \"b\") }; return []Finding{f()} }",
			wantSites: 1,
		},
		{
			name:      "a method is attributed to the method name",
			body:      "type T struct{}\n\nfunc (T) m() Finding { return newFinding(\"a\", \"b\") }",
			wantSites: 1,
		},
		{
			name:        "a package-level initialiser is attributed to NO function",
			body:        "var pkgLevel = newFinding(\"\", \"x\")",
			wantSites:   1,
			wantFnEmpty: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sites, lits := scanFindingSource(t, token.NewFileSet(), "subject.go", preamble+"\n"+tc.body+"\n")
			if len(lits) != tc.wantLits {
				t.Errorf("flagged %d Finding literal(s), want %d: %v", len(lits), tc.wantLits, lits)
			}
			if len(sites) != tc.wantSites {
				t.Errorf("found %d newFinding site(s), want %d: %v", len(sites), tc.wantSites, sites)
			}
			var empty int
			for _, s := range sites {
				if s.fn == "" {
					empty++
				}
			}
			if empty != tc.wantFnEmpty {
				t.Errorf("%d site(s) attributed to no function, want %d: %v", empty, tc.wantFnEmpty, sites)
			}
		})
	}

	// The file the funnel LIVES in is exempt, and that exemption must be
	// file-scoped rather than global — otherwise the guard is off everywhere.
	if _, lits := scanFindingSource(t, token.NewFileSet(), "finding.go",
		preamble+"\nfunc c() []Finding { return []Finding{{Message: \"x\"}} }\n"); len(lits) != 0 {
		t.Errorf("finding.go must be exempt from the funnel rule, got %v", lits)
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
		if s.fn == "" {
			// A package-level construction. It has no enclosing function to
			// reach, so it cannot enter the ledger; Guard A rejects it by name
			// and line instead. Listing it here would only add a second,
			// misleading "add a fixture" failure for the same defect.
			continue
		}
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
