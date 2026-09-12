package cmd

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
)

// THE TABWRITER RENDERER LEDGER — civitai/cli#552.
//
// 🔴 THE HAZARD IS THE DELIMITER, NOT THE ESCAPE. safeTerm removes the control
// class but deliberately KEEPS `\n` and `\t` (internal/saferune: "Cc, minus \n
// and \t"), because legitimate multi-line server text exists. text/tabwriter
// uses `\t` as its COLUMN DELIMITER and starts a new row at every `\n`. So a
// server string carrying either one does not corrupt the table, it EXTENDS it:
//
//   - `\n` forges a row at column zero — a fake header, a fake result;
//   - `\t` injects a COLUMN, and tabwriter itself aligns the forgery, which is
//     the more convincing of the two (measured in #569 on `images search`).
//
// safeTermSingle is the gate for a value that lands in a cell. This file is the
// answer to "which renderers is it applied at, and which does nobody check".
//
// 🔴 THE PREDICATE IS NOT KEYED ON A HELPER'S NAME, AND THAT IS THE WHOLE
// LESSON OF #552. Its original ask was a ledger over "call sites that call
// indentContinuation" — satisfied by construction, green at base, and blind to
// every renderer that had never called it. A guard keyed to a function name can
// only find the places that already do the right thing.
//
// So the predicate here is STRUCTURAL and starts from the hazard:
//
//	a value written into a text/tabwriter cell may not reach that cell through
//	a bare safeTerm — safeTermSingle is the only sanitizer whose output cannot
//	carry the delimiter.
//
// It shrinks to zero when a renderer stops sanitising (the call disappears and
// the row's claimed gate no longer holds), and it grows when a renderer appears
// (a new tabwriter function with no row fails GREW). Neither direction can be
// satisfied by adding a call to a named helper.
//
// 🔴 WHAT THE STRUCTURAL HALF CANNOT SEE, stated rather than waved at:
//  1. Server text written into a cell with NO sanitizer at all. No AST can tell
//     a server string from a CLI-owned label, so that judgement is the LEDGER's
//     job: every renderer carries a gate classification and a reason, and the
//     unsanitised ones are counted by a ratchet that only goes down.
//  2. A tabwriter reached through a struct field, a function-typed variable or
//     an interface method — the taint walk follows locals, direct calls and
//     parameters, not values stored on a struct. The same gap on the VALUE side
//     (a cell fed from a struct field that was gated upstream) is what
//     gatePreSanitised exists to make someone write down.
//  3. A write that is not fmt.Fprint/Fprintf/Fprintln/io.WriteString —
//     `tw.Write([]byte(…))` would make the whole renderer invisible, GREW
//     included. No such call exists in this package today.
//  4. Whether the sanitised value is the RIGHT one. A structural check
//     type-checks past a wrong argument, which is why
//     TestTabwriterRenderersCannotBeForged drives the real renderers.
//  5. The SAME hazard one line outside a tabwriter: a `label: value` line
//     printed with plain Fprintf still puts a forged line at column zero if the
//     value carries `\n`. Several detail renderers (printModelDetail's header,
//     printCollectionDetail, printAppDetail) are in that state deliberately —
//     telling a single-line label from legitimately multi-line free text there
//     is a per-field judgement, not a structural one, and #569 already made it
//     for images.go. Stated as an open residual rather than half-converted.

// --- the analysis -----------------------------------------------------------

// twSink is one tabwriter write: `fmt.Fprintf(tw, …)` where tw is known to be a
// *tabwriter.Writer, either because it was assigned from tabwriter.NewWriter in
// this function or because a caller passed one into this parameter.
type twSink struct {
	fn  string // enclosing function key (safeTermFuncKey)
	pos string
}

// twViolation is a bare safeTerm reaching a tabwriter cell.
type twViolation struct {
	fn   string
	pos  string // position of the safeTerm call
	cell string // position of the write it reaches
	via  string // the helper chain, when it is not a direct argument
}

type tabwriterAnalysis struct {
	// renderers maps a function key to the number of tabwriter writes in it.
	renderers map[string]int
	// file maps a function key to the file declaring it.
	file map[string]string
	// sanitizers maps a function key to the set of sanitizer names that reach a
	// cell from it ("safeTerm", "safeTermSingle").
	sanitizers map[string]map[string]bool
	// calls maps a function key to the sanitizer names it calls ANYWHERE, cell or
	// not. It is what resolves a gatePreSanitised row's `upstream` to a fact.
	calls      map[string]map[string]bool
	violations []twViolation
	sinks      []twSink
	files      int
	funcs      int
}

// analyzeTabwriterUse is the one implementation of the predicate. It takes
// parsed files so the calibration test can feed it source the tree does not
// contain — a control built only from shapes the tree already has proves
// nothing.
func analyzeTabwriterUse(fset *token.FileSet, files map[string]*ast.File) *tabwriterAnalysis {
	a := &tabwriterAnalysis{
		renderers:  map[string]int{},
		file:       map[string]string{},
		sanitizers: map[string]map[string]bool{},
		calls:      map[string]map[string]bool{},
		files:      len(files),
	}

	// decls: every function declaration, keyed the same way safeTermCoveredBy
	// keys its rows.
	look := declLookup{byKey: map[string]*ast.FuncDecl{}, byName: map[string]string{}}
	var order []string
	for _, name := range twSortedKeys(files) {
		for _, d := range files[name].Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			key := safeTermFuncKey(fd)
			look.byKey[key] = fd
			a.file[key] = name
			order = append(order, key)
			if fd.Recv == nil {
				look.byName[fd.Name.Name] = key
			}
		}
	}
	a.funcs = len(look.byKey)

	// sinks[key] is the set of identifier names in that function that hold a
	// *tabwriter.Writer.
	sinks := map[string]map[string]bool{}
	add := func(key, name string) bool {
		if key == "" || name == "" || name == "_" {
			return false
		}
		if sinks[key] == nil {
			sinks[key] = map[string]bool{}
		}
		if sinks[key][name] {
			return false
		}
		sinks[key][name] = true
		return true
	}

	// 1. Seed: `x := tabwriter.NewWriter(…)` / `x = tabwriter.NewWriter(…)`.
	for _, key := range order {
		fd := look.byKey[key]
		ast.Inspect(fd, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok || len(as.Lhs) != len(as.Rhs) {
				return true
			}
			for i, rhs := range as.Rhs {
				if !isTabwriterNew(rhs) {
					continue
				}
				if id, ok := as.Lhs[i].(*ast.Ident); ok {
					add(key, id.Name)
				}
			}
			return true
		})
	}

	// 2. Propagate to callees: `printCostMap(tw, …)` makes printCostMap's first
	//    parameter a tabwriter sink. Iterated to a fixpoint so a sink handed two
	//    hops down is still seen.
	params := func(fd *ast.FuncDecl) []string {
		var out []string
		if fd.Type.Params == nil {
			return out
		}
		for _, f := range fd.Type.Params.List {
			if len(f.Names) == 0 {
				out = append(out, "")
				continue
			}
			for _, nm := range f.Names {
				out = append(out, nm.Name)
			}
		}
		return out
	}
	for changed := true; changed; {
		changed = false
		for _, key := range order {
			fd := look.byKey[key]
			ast.Inspect(fd, func(n ast.Node) bool {
				ce, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				id, ok := ce.Fun.(*ast.Ident)
				if !ok {
					return true
				}
				calleeKey, known := look.byName[id.Name]
				if !known {
					return true
				}
				ps := params(look.byKey[calleeKey])
				for i, arg := range ce.Args {
					argID, ok := arg.(*ast.Ident)
					if !ok || !sinks[key][argID.Name] || i >= len(ps) {
						continue
					}
					if add(calleeKey, ps[i]) {
						changed = true
					}
				}
				return true
			})
		}
	}

	// 3. Record every sanitizer call by enclosing function, cell or not. This is
	//    what a gatePreSanitised row's `upstream` is resolved against.
	for _, key := range order {
		k := key
		ast.Inspect(look.byKey[k], func(n ast.Node) bool {
			ce, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			id, ok := ce.Fun.(*ast.Ident)
			if !ok || !scannedSanitizers[id.Name] || len(ce.Args) != 1 {
				return true
			}
			if a.calls[k] == nil {
				a.calls[k] = map[string]bool{}
			}
			a.calls[k][id.Name] = true
			return true
		})
	}

	// 4. Collect the writes and check every cell expression.
	for _, key := range order {
		fd := look.byKey[key]
		locals := localAssignments(fd)
		ast.Inspect(fd, func(n ast.Node) bool {
			ce, ok := n.(*ast.CallExpr)
			if !ok || !isWriteCall(ce) || len(ce.Args) == 0 {
				return true
			}
			dst, ok := ce.Args[0].(*ast.Ident)
			if !ok || !sinks[key][dst.Name] {
				return true
			}
			cellPos := fset.Position(ce.Lparen).String()
			a.renderers[key]++
			a.sinks = append(a.sinks, twSink{fn: key, pos: cellPos})
			for _, arg := range ce.Args[1:] {
				for _, hit := range sanitizerReach(fset, arg, key, locals, look, 0, map[string]bool{}) {
					if a.sanitizers[key] == nil {
						a.sanitizers[key] = map[string]bool{}
					}
					a.sanitizers[key][hit.name] = true
					if hit.name == "safeTerm" {
						a.violations = append(a.violations, twViolation{
							fn: key, pos: hit.pos, cell: cellPos, via: hit.via,
						})
					}
				}
			}
			return true
		})
	}
	sort.Slice(a.violations, func(i, j int) bool { return a.violations[i].pos < a.violations[j].pos })
	return a
}

// declLookup is the declaration index: every function by ledger key, and the
// name->key map for resolving a direct call to a package-level helper.
type declLookup struct {
	byKey  map[string]*ast.FuncDecl
	byName map[string]string
}

type sanitizerHit struct {
	name string
	pos  string
	via  string
}

// sanitizerReach reports every sanitizer call reachable from a cell expression:
// directly, through a local variable assigned in the same function, or through a
// package-level helper the expression calls.
//
// The helper hop is what makes the predicate structural rather than syntactic:
// `appCardAuthor(c)` puts a server username in a cell without naming a sanitizer
// at the call site at all.
func sanitizerReach(fset *token.FileSet, e ast.Expr, fn string, locals map[string][]ast.Expr, d declLookup, depth int, seen map[string]bool) []sanitizerHit {
	if depth > 4 || e == nil {
		return nil
	}
	var out []sanitizerHit
	ast.Inspect(e, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			id, ok := x.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			if scannedSanitizers[id.Name] && len(x.Args) == 1 {
				out = append(out, sanitizerHit{name: id.Name, pos: fset.Position(x.Lparen).String()})
				return true
			}
			calleeKey, known := d.byName[id.Name]
			if !known || seen[calleeKey] {
				return true
			}
			seen[calleeKey] = true
			fd := d.byKey[calleeKey]
			if fd == nil {
				return true
			}
			for _, hit := range sanitizerReach(fset, bodyExpr(fd), calleeKey, localAssignments(fd), d, depth+1, seen) {
				hit.via = id.Name + "()"
				out = append(out, hit)
			}
		case *ast.Ident:
			rhs, ok := locals[x.Name]
			if !ok || seen[fn+"::"+x.Name] {
				return true
			}
			seen[fn+"::"+x.Name] = true
			for _, r := range rhs {
				out = append(out, sanitizerReach(fset, r, fn, locals, d, depth+1, seen)...)
			}
		}
		return true
	})
	return out
}

// bodyExpr wraps a function body so ast.Inspect can walk it as an expression
// tree. A function literal is the cheapest node that carries a *ast.BlockStmt.
func bodyExpr(fd *ast.FuncDecl) ast.Expr {
	return &ast.FuncLit{Type: fd.Type, Body: fd.Body}
}

// localAssignments maps every identifier assigned in fd to the expressions
// assigned to it, so a cell argument spelled as a bare local can be resolved to
// what it was built from (`creator := safeTerm(...)`, `row := fmt.Sprintf(...)`).
func localAssignments(fd *ast.FuncDecl) map[string][]ast.Expr {
	out := map[string][]ast.Expr{}
	ast.Inspect(fd, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			if len(x.Lhs) != len(x.Rhs) {
				return true
			}
			for i, lhs := range x.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name != "_" {
					out[id.Name] = append(out[id.Name], x.Rhs[i])
				}
			}
		case *ast.ValueSpec:
			for i, nm := range x.Names {
				if i < len(x.Values) && nm.Name != "_" {
					out[nm.Name] = append(out[nm.Name], x.Values[i])
				}
			}
		}
		return true
	})
	return out
}

func isTabwriterNew(e ast.Expr) bool {
	ce, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := ce.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "NewWriter" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "tabwriter"
}

// isWriteCall reports whether ce writes to the io.Writer in its first argument.
// fmt.Fprint/Fprintf/Fprintln are the three spellings this package uses;
// io.WriteString is accepted too, so a renderer cannot leave this guard's view
// by switching to it. A METHOD write (`tw.Write(…)`) is not seen — residual 3 in
// this file's header.
func isWriteCall(ce *ast.CallExpr) bool {
	sel, ok := ce.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	switch pkg.Name + "." + sel.Sel.Name {
	case "fmt.Fprint", "fmt.Fprintf", "fmt.Fprintln", "io.WriteString":
		return true
	}
	return false
}

// twSortedKeys is sortedKeys (readme_troubleshooting_test.go) generalised over
// the value type, so the same deterministic ordering applies to the maps here.
func twSortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// parsePackageFiles parses this package's own non-test sources.
func parsePackageFiles(t *testing.T) (*token.FileSet, map[string]*ast.File) {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot read the package directory: %v", err)
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, name, nil, 0)
		if perr != nil {
			t.Fatalf("CONTROL failure, not a finding: cannot parse %s: %v", name, perr)
		}
		files[name] = f
	}
	return fset, files
}

// --- the ledger -------------------------------------------------------------

// The gate a renderer's cells are under. These are STATES, not spellings: each
// one is checked against what the analysis found, so a row cannot claim a gate
// the code does not have (nor hide one it does).
const (
	// gateSingle: at least one cell value is routed through safeTermSingle, and
	// none through a bare safeTerm.
	gateSingle = "safeTermSingle"
	// gateNoServerText: no cell in this renderer carries server-supplied text, so
	// there is nothing to gate. Asserted as "no sanitizer call reaches a cell" —
	// a renderer that starts sanitising stops matching this row.
	gateNoServerText = "no-server-text"
	// gatePreSanitised: a cell DOES carry server text, but it was gated upstream
	// and reaches the cell through a struct field the scan does not follow.
	//
	// 🔴 IT IS A BINDING, NOT AN EXCUSE. The row must name the upstream function
	// in `upstream`, and that name is RESOLVED: it has to exist in this package
	// and to call safeTermSingle. Otherwise this state would be the hole every
	// other row could be moved into — "it is handled somewhere else" asserted in
	// prose, which is how a guard on a word gets walked.
	gatePreSanitised = "pre-sanitised-upstream"
	// gateUnsanitised: server text reaches a cell with no gate at all. An honest
	// row for a known hole, counted by maxUnsanitisedTabwriterRenderers, which
	// only goes down. It is currently EMPTY, and that is the point of keeping the
	// value: the next renderer added without a gate has somewhere truthful to go
	// other than a lie.
	gateUnsanitised = "none"
)

type tabwriterRenderer struct {
	file string
	gate string
	// upstream is required by — and only by — gatePreSanitised: the function
	// that applied the gate before the value reached this renderer. It is
	// resolved against the package, not read as a label.
	upstream string
	// why says what server text lands in a cell here — or, for a
	// gateNoServerText row, why nothing does. Every row must say something.
	why string
}

// 🔴 THIS SET IS THE CLOSING CONDITION OF civitai/cli#552: every function that
// writes into a text/tabwriter, with what its cells carry. It fails when the set
// GROWS (a new renderer nobody classified) and when it SHRINKS (a renderer
// deleted, renamed, or one that stopped using a tabwriter while this row went on
// reading as a classification).
//
// The rows were measured by the scan below, not read off the source: run
// `go test -run TestTabwriterRenderersAreLedgered -v` and every row is logged
// with the sanitizers actually reaching its cells.
var tabwriterRenderers = map[string]tabwriterRenderer{
	// --- read path ----------------------------------------------------------
	"printModelList": {"models.go", gateSingle, "",
		"`models search` rows: uploader model name, type and creator username"},
	"printModelDetail": {"models.go", gateSingle, "",
		"`models get`'s version table: version name, base model, and the " +
			"[Archive]/[Training Data] marker built from files[].type"},
	"printModelVersionDetail": {"model_versions.go", gateSingle, "",
		"`model-versions get`'s file table: published file name and type"},
	"printCreatorList": {"creators.go", gateSingle, "",
		"`creators search` rows: username and link. The measured #552 forgery — a username of " +
			"\"alice\\t99\\thttps://evil.example/steal\" rendered a fully aligned attacker-controlled row"},
	"printTagList": {"tags.go", gateSingle, "",
		"`tags search` rows: tag name and link"},
	"printCollectionList": {"collections.go", gateSingle, "",
		"`collections search` rows: name, type and owner username"},
	"printArticleList": {"articles.go", gateSingle, "",
		"`articles search` rows: title, author username and published date (shortDate passes a " +
			"bad timestamp through verbatim)"},
	"printAppList": {"apps.go", gateSingle, "",
		"`app list` rows: name, slug, kind, category and the creator chip (appCardAuthor)"},
	"printImageList": {"images.go", gateSingle, "",
		"`images search` rows: uploader, base model, rating and URL — the surface #569 fixed"},
	"newUsersGetCmd": {"users.go", gateSingle, "",
		"the `other matches` table after an inexact `users get`: each candidate username"},

	// --- app path -----------------------------------------------------------
	"printAppMetrics": {"app_metrics.go", gateSingle, "",
		"`app metrics`: the window timestamps (utcStamp falls back to the RAW string), the " +
			"granularity, and the top-scope / top-endpoint tokens, which are kept raw on purpose " +
			"(AGENTS.md item 8) so the gate is the only thing between an uploader-shaped token and the table"},
	"printSubmissionTable": {"app_status.go", gateSingle, "",
		"`app status` rows: block id, version, status, deploy state, claimed source commit, date and " +
			"live URL. It had NO gate at all until #552, which also made it invisible to safeTermCoveredBy"},
	"printSubmissionDetail": {"app_status.go", gateSingle, "",
		"`app status --id` rows: the same fields plus the publish-request id and deploy detail. The " +
			"free-text rejection reason / approval notes below the table are NOT in a cell and are not " +
			"gated here — see the residuals in this file's header"},
	"printListingStatus": {"app_listing.go", gateSingle, "",
		"`app listing status`: the listing status. `App:` is the slug the USER typed and is echoed " +
			"exactly (civitai/cli#393)"},

	// --- generate path ------------------------------------------------------
	"printWorkflow": {"workflows.go", gateSingle, "",
		"`workflows get`'s header block: workflow id, status and timestamps"},
	"printWorkflowList": {"workflows_list.go", gateSingle, "",
		"`workflows list` rows: id, status and date. A newline here also mis-splices the reason " +
			"lines spliced into the flushed block"},
	"reportWorkflowSettlement": {"workflow_settlement.go", gateSingle, "",
		"the settlement table's transaction TYPE, printed next to a Buzz amount"},
	"printSubmitResult": {"generate.go", gateSingle, "",
		"the post-spend receipt: workflow id and status — the only handle to a job already paid for"},
	"printCostMap": {"generate.go", gateSingle, "",
		"the PRE-SPEND cost table's server-named factor/fixed/tip keys. It takes the tabwriter as a " +
			"PARAMETER from printGenerateQuote, which is why the scan propagates the sink across calls"},
	"printGenerateQuote": {"generate.go", gatePreSanitised, "describeVersion",
		"most of the quote screen's cells are CLI-owned labels, user-typed flag values echoed exactly " +
			"(civitai/cli#393) and numbers. Its two SERVER-derived cells — Checkpoint and LoRA — are " +
			"gated by describeVersion and reach the cell through a resolvedGraph FIELD, which the scan " +
			"deliberately does not follow. That is why the row names the upstream function: the name is " +
			"resolved, so deleting describeVersion's gate fails HERE too"},
}

const (
	// minTabwriterRenderersFound is the POSITIVE CONTROL on the scan: did it find
	// renderers at all? It is NOT a completeness check — that is the set
	// comparison's job.
	//
	// 🔴 IT IS STRICTLY BELOW len(tabwriterRenderers), AND THE REASON IS A
	// MEASURED DEFECT IN THIS REPO. indentcontinuation_ledger_test.go shipped
	// with its floor EQUAL to its pinned set (6 against 6) and checked BEFORE the
	// set comparison, so deleting a real call site printed "CONTROL failure" —
	// this file's idiom for "the harness is broken, not your code" — and the
	// SHRANK message never ran. A floor at or near the set size converts the
	// finding into a message that invites lowering the floor. 8 against 20 is
	// loose on purpose: a scan that has stopped working returns 0 or 1.
	minTabwriterRenderersFound = 8

	// minTabwriterWrites and minTabwriterFiles are the same control one level
	// down: the writes are what the violation check reads, and a walk over the
	// wrong directory parses nothing and reports a serene zero.
	minTabwriterWrites = 30
	minTabwriterFiles  = 20

	// maxUnsanitisedTabwriterRenderers is the RATCHET, asserted as an EQUALITY
	// for the reason maxUncoveredSafeTermFuncs is: a ceiling silently acquires
	// headroom, and the next commit spends it by adding an ungated renderer with
	// an honest row and a green suite. It is ZERO today because #552 closed every
	// one; a commit that has to raise it is a commit shipping a new forgery
	// surface, and it should have to say so.
	maxUnsanitisedTabwriterRenderers = 0
)

// TestTabwriterRenderersAreLedgered is the structural half of civitai/cli#552.
//
// Five assertions, each with its own message so a failure names the thing that
// went wrong:
//
//	BARE SAFETERM — a cell value reaches a tabwriter through safeTerm, which
//	                keeps the `\n` that forges a row and the `\t` that injects
//	                a column.
//	GREW          — a function writes into a tabwriter and no row classifies it.
//	SHRANK        — a row names a function that no longer writes into one.
//	MISLABELLED   — a row's gate disagrees with what the scan found.
//	RATCHET       — the ungated count is not exactly maxUnsanitisedTabwriterRenderers.
func TestTabwriterRenderersAreLedgered(t *testing.T) {
	fset, files := parsePackageFiles(t)
	a := analyzeTabwriterUse(fset, files)

	// --- controls (strictly below the sets they guard) -----------------------
	if a.files < minTabwriterFiles {
		t.Fatalf("CONTROL failure, not a finding: parsed only %d non-test file(s) in this package, want >= %d. "+
			"The scan is reading the wrong tree and every verdict below would be about nothing.",
			a.files, minTabwriterFiles)
	}
	if len(a.renderers) < minTabwriterRenderersFound || len(a.sinks) < minTabwriterWrites {
		t.Fatalf("CONTROL failure, not a finding: found %d tabwriter renderer(s) and %d cell write(s), "+
			"want >= %d and >= %d. A scan that finds nothing reports 'no violations' just as loudly as a "+
			"clean tree does.", len(a.renderers), len(a.sinks), minTabwriterRenderersFound, minTabwriterWrites)
	}

	// --- BARE SAFETERM -------------------------------------------------------
	if len(a.violations) > 0 {
		var lines []string
		for _, v := range a.violations {
			via := ""
			if v.via != "" {
				via = " via " + v.via
			}
			lines = append(lines, fmt.Sprintf("%s: safeTerm(...)%s reaches the tabwriter cell written at %s (in %s)",
				v.pos, via, v.cell, v.fn))
		}
		t.Errorf("%d bare safeTerm call(s) reach a tabwriter cell:\n  %s\n\n"+
			"Use safeTermSingle. safeTerm deliberately KEEPS `\\n` and `\\t` (internal/saferune), and "+
			"text/tabwriter treats the first as a row break and the second as its COLUMN DELIMITER — so "+
			"server text in a cell forges a row at column zero, or an ALIGNED extra column that tabwriter "+
			"itself lays out. If the value is genuinely multi-line free text, it does not belong in a cell: "+
			"render it outside the tabwriter with indentContinuation, as printWorkflow does with the failure "+
			"reason (civitai/cli#552).", len(a.violations), strings.Join(lines, "\n  "))
	}

	// --- GREW ----------------------------------------------------------------
	var unledgered []string
	for _, fn := range twSortedKeys(a.renderers) {
		if _, ok := tabwriterRenderers[fn]; !ok {
			unledgered = append(unledgered, fmt.Sprintf("%s (%s) — %d cell write(s)", fn, a.file[fn], a.renderers[fn]))
		}
	}
	if len(unledgered) > 0 {
		t.Errorf("%d function(s) write into a tabwriter with no row in tabwriterRenderers:\n  %s\n\n"+
			"Classify it. gateSingle when its cells carry server text routed through safeTermSingle; "+
			"gateNoServerText when nothing in a cell comes from the server (say why); gateUnsanitised when "+
			"server text reaches a cell with no gate — that is an honest row, and it counts against "+
			"maxUnsanitisedTabwriterRenderers (%d), which does not go up.",
			len(unledgered), strings.Join(unledgered, "\n  "), maxUnsanitisedTabwriterRenderers)
	}

	// --- SHRANK --------------------------------------------------------------
	var stale []string
	for _, fn := range twSortedKeys(tabwriterRenderers) {
		if a.renderers[fn] == 0 {
			stale = append(stale, fmt.Sprintf("%s (recorded in %s as %s)", fn, tabwriterRenderers[fn].file, tabwriterRenderers[fn].gate))
		}
	}
	if len(stale) > 0 {
		t.Errorf("%d ledger row(s) name a function that no longer writes into a tabwriter:\n  %s\n\n"+
			"RENAMED or MOVED: move the row with it, in the same commit. DELETED: delete the row. "+
			"STOPPED USING A TABWRITER: check what replaced it is not line-structured too — this row would "+
			"otherwise go on reading as a classification of a surface nobody has looked at.",
			len(stale), strings.Join(stale, "\n  "))
	}

	// --- MISLABELLED + RATCHET ----------------------------------------------
	ungated := 0
	for _, fn := range twSortedKeys(tabwriterRenderers) {
		row := tabwriterRenderers[fn]
		if row.why == "" {
			t.Errorf("tabwriterRenderers[%q] has an empty `why`. A row with no reason is a row nobody thought "+
				"about; say what server text lands in a cell, or why none does.", fn)
		}
		if a.file[fn] != "" && row.file != a.file[fn] {
			t.Errorf("tabwriterRenderers[%q] says %s, but the declaration is in %s.", fn, row.file, a.file[fn])
		}
		sanitised := a.sanitizers[fn][gateSingle]
		switch row.gate {
		case gateSingle:
			if !sanitised {
				t.Errorf("MISLABELLED: tabwriterRenderers[%q] claims %s, but no safeTermSingle call reaches any "+
					"of its cells. Either the gate was deleted — which is the #552 defect happening, and the "+
					"row would have gone on reading as coverage — or the value now arrives pre-sanitised "+
					"through a field the scan cannot follow, in which case say so in `why` and change the "+
					"gate.", fn, gateSingle)
			}
		case gateNoServerText:
			if sanitised {
				t.Errorf("MISLABELLED: tabwriterRenderers[%q] claims %s, but a sanitizer call DOES reach one of "+
					"its cells. Somebody found server text there; the row says there is none.", fn, gateNoServerText)
			}
		case gatePreSanitised:
			if sanitised {
				t.Errorf("MISLABELLED: tabwriterRenderers[%q] claims %s, but a sanitizer DOES run at one of its "+
					"own cells. Use %s — the row is describing a gate somewhere else while one is right here.",
					fn, gatePreSanitised, gateSingle)
			}
			// 🔴 RESOLVE THE UPSTREAM, DO NOT READ IT. A row asserting "this was
			// handled elsewhere" is the one state that could absorb every other
			// row, so the name it gives has to be a function that exists AND that
			// still applies the gate. Deleting describeVersion's safeTermSingle
			// therefore fails HERE as well as in the behavioural test.
			switch {
			case row.upstream == "":
				t.Errorf("tabwriterRenderers[%q] claims %s but names no upstream function. The claim is only "+
					"checkable if it names one.", fn, gatePreSanitised)
			case a.file[row.upstream] == "":
				t.Errorf("tabwriterRenderers[%q] names upstream %q, which is not a function in this package. A "+
					"row pointing at a gate that does not exist reads as coverage and stops anyone looking.",
					fn, row.upstream)
			case !a.calls[row.upstream][gateSingle]:
				t.Errorf("tabwriterRenderers[%q] says its cells are pre-sanitised by %s, but %s does not call "+
					"safeTermSingle anywhere. Either the gate was deleted upstream — in which case this "+
					"renderer's cells are now ungated and nothing else would have said so — or the value comes "+
					"from somewhere else and the row is wrong.", fn, row.upstream, row.upstream)
			}
		case gateUnsanitised:
			ungated++
			if sanitised {
				t.Errorf("MISLABELLED: tabwriterRenderers[%q] claims %s, but safeTermSingle reaches a cell — the "+
					"hole was closed and the row was not. Move it to %s and lower "+
					"maxUnsanitisedTabwriterRenderers in the same commit.", fn, gateUnsanitised, gateSingle)
			}
		default:
			t.Errorf("tabwriterRenderers[%q] has an unknown gate %q.", fn, row.gate)
		}
	}
	if ungated != maxUnsanitisedTabwriterRenderers {
		t.Errorf("RATCHET: %d renderer(s) are ledgered %s and maxUnsanitisedTabwriterRenderers is %d.\n\n"+
			"Above it, a new ungated forgery surface landed. Below it, there is unbanked headroom a LATER "+
			"commit can spend in silence — lower the constant to %d in this commit.",
			ungated, gateUnsanitised, maxUnsanitisedTabwriterRenderers, ungated)
	}

	for _, k := range twSortedKeys(a.renderers) {
		t.Logf("%-26s %-24s cells=%2d sanitizers=%v", k, a.file[k], a.renderers[k], twSortedKeys(a.sanitizers[k]))
	}
	t.Logf("scanned %d file(s), %d func(s): %d tabwriter renderer(s), %d cell write(s), %d bare-safeTerm violation(s)",
		a.files, a.funcs, len(a.renderers), len(a.sinks), len(a.violations))
}

// TestTabwriterScannerSeesShapesNotInTree is the CALIBRATION, and every case is
// a shape the tree does not contain — a control built from the shape you already
// handle proves nothing (the pattern is lifted from
// floor_predicate_ledger_test.go).
//
// It is both controls in one table: the flagged cases prove the scan CAN go red
// (a zero from it is not a scan wired to nothing), and the clean cases prove it
// is not simply red at everything, which would be the same uselessness inverted.
func TestTabwriterScannerSeesShapesNotInTree(t *testing.T) {
	const head = "package cmd\n\nimport (\n\t\"fmt\"\n\t\"io\"\n\t\"text/tabwriter\"\n)\n\n"
	for _, tc := range []struct {
		name           string
		src            string
		wantViolations int
		wantRenderers  []string
	}{
		{
			name: "bare safeTerm straight into a cell",
			src: head + `func zzRender(w io.Writer, s string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s\t%s\n", safeTerm(s), "x")
	_ = tw.Flush()
}`,
			wantViolations: 1,
			wantRenderers:  []string{"zzRender"},
		},
		{
			name: "through a local variable",
			src: head + `func zzRender(w io.Writer, s string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	cell := safeTerm(s)
	fmt.Fprintln(tw, cell)
	_ = tw.Flush()
}`,
			wantViolations: 1,
			wantRenderers:  []string{"zzRender"},
		},
		{
			name: "through a helper that sanitises internally",
			src: head + `func zzLabel(s string) string { return "[" + safeTerm(s) + "]" }

func zzRender(w io.Writer, s string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s\t1\n", zzLabel(s))
	_ = tw.Flush()
}`,
			wantViolations: 1,
			wantRenderers:  []string{"zzRender"},
		},
		{
			name: "the tabwriter handed to another function",
			src: head + `func zzRows(w io.Writer, s string) {
	fmt.Fprintf(w, "  %s\t1\n", safeTerm(s))
}

func zzRender(w io.Writer, s string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	zzRows(tw, s)
	_ = tw.Flush()
}`,
			wantViolations: 1,
			wantRenderers:  []string{"zzRows"},
		},
		{
			name: "reassigned to a second tabwriter in the same function",
			src: head + `func zzRender(w io.Writer, s string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "A\tB")
	_ = tw.Flush()
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s\t1\n", safeTerm(s))
	_ = tw.Flush()
}`,
			wantViolations: 1,
			wantRenderers:  []string{"zzRender"},
		},
		{
			name: "sanitised correctly — a renderer, no violation",
			src: head + `func zzRender(w io.Writer, s string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s\t1\n", safeTermSingle(s))
	_ = tw.Flush()
}`,
			wantViolations: 0,
			wantRenderers:  []string{"zzRender"},
		},
		{
			name: "safeTerm outside any tabwriter — not this guard's business",
			src: head + `func zzRender(w io.Writer, s string) {
	fmt.Fprintf(w, "reason: %s\n", safeTerm(s))
}`,
			wantViolations: 0,
			wantRenderers:  nil,
		},
		{
			// io.WriteString is a second spelling of the same write, so a renderer
			// cannot leave this guard's view by switching to it.
			name: "written with io.WriteString instead of fmt.Fprintf",
			src: head + `func zzRender(w io.Writer, s string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = io.WriteString(tw, safeTerm(s))
	_ = tw.Flush()
}`,
			wantViolations: 1,
			wantRenderers:  []string{"zzRender"},
		},
		{
			name: "a tabwriter whose cells are numbers and literals",
			src: head + `func zzRender(w io.Writer, n int) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Total\t%d\n", n)
	_ = tw.Flush()
}`,
			wantViolations: 0,
			wantRenderers:  []string{"zzRender"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "zz_fixture.go", tc.src, 0)
			if err != nil {
				t.Fatalf("CONTROL failure, not a finding: the fixture does not parse: %v", err)
			}
			a := analyzeTabwriterUse(fset, map[string]*ast.File{"zz_fixture.go": f})
			if len(a.violations) != tc.wantViolations {
				t.Errorf("scanner reported %d violation(s), want %d — it cannot see this shape:\n%s\n%+v",
					len(a.violations), tc.wantViolations, tc.src, a.violations)
			}
			got := twSortedKeys(a.renderers)
			if strings.Join(got, ",") != strings.Join(tc.wantRenderers, ",") {
				t.Errorf("scanner attributed the tabwriter to %v, want %v:\n%s", got, tc.wantRenderers, tc.src)
			}
		})
	}
}
