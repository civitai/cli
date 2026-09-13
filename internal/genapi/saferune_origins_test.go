package genapi

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

// civitai/cli#542 REACHING internal/genapi — WHAT hasPrintableContent IS GIVEN.
//
// #542's closing condition asks for a guard that fails when "any `saferune.*`
// call site outside `internal/cmd`" gains a caller whose argument is not
// server-supplied, and when that caller set changes at all, bidirectionally. It
// was closed by civitai/cli#557, which built exactly that for pkg/civitai's
// snippet(). This package's saferune call site — hasPrintableContent, at
// status.go — was never covered by anything: the module-root caller ledger
// records that internal/genapi imports saferune, and says in its own comment
// that it answers "who calls", never "with what".
//
// So this file is snippetArgs' sibling for the other half of the condition, and
// the module-root saferuneRefs ledger delegates internal/genapi's row to
// TestHasPrintableContentArgumentsAreServerBytes below. That delegation is
// resolved to a real declaration there, so renaming either test without moving
// the other is red rather than silent.
//
// # WHY THERE ARE TWO ASSERTIONS AND NOT ONE
//
// hasPrintableContent is a one-line wrapper, so its own argument origin is
// decided one level out, at dedupeReasons — and dedupeReasons' argument origin
// is decided one level out again, at the three methods that build the array.
// Pinning only the first hop would be a guard whose description ("the argument
// is server bytes") is wider than its body ("the argument is whatever
// dedupeReasons passes"), which is the single most common finding in this
// repository's audit history.
//
// 🔴 THE PROSE ONE LINE ABOVE dedupeReasons WAS ALREADY WRONG WHEN THIS WAS
// WRITTEN, WHICH IS THE ARGUMENT FOR THE SECOND ASSERTION IN ONE SENTENCE. It
// read "The four callers are the two step types' failureReasons and the two
// workflow-level FailureReasons." There is ONE step-type failureReasons
// (Step.failureReasons) and there are three callers, not four; no
// ListedStep.failureReasons has ever existed — ListedWorkflow.FailureReasons
// reads `steps[].errors` inline. A comment is a claim, and that one had been
// green through every suite since it was written because nothing asserted on
// it. TestDedupeReasonsCallersAreLedgered is what asserts on it now.

// genapiArgOrigin records where one call site's argument comes from.
type genapiArgOrigin struct {
	// server is whether these bytes came off the wire from the server.
	//
	// server: true is the invariant. A false entry is not permission — it is
	// the #393 regression written down, and the test says so by name.
	server bool
	// why is the provenance, specific enough to be checkable by reading the
	// named function.
	why string
}

// hasPrintableContentArgs is keyed "<enclosing function>:<argument expression>".
//
// 🔴 KEYED BY ENCLOSING FUNCTION, for the reason pkg/civitai's snippetArgs
// states and civitai/cli#582 then demonstrated live in internal/cmd: an
// argument NAME is not an identity. `t`, `s` and `raw` each appear in several
// functions in this package, and a name-keyed ledger lets one classification
// vouch for every site that happens to share the spelling.
var hasPrintableContentArgs = map[string]genapiArgOrigin{
	"dedupeReasons:t": {
		server: true,
		why: "t is strings.TrimSpace(e) over an element of dedupeReasons' own `raw []string` " +
			"parameter. Every caller of dedupeReasons is ledgered below — enforced, not asserted: " +
			"dedupeReasonsCallers is bidirectional and its walk owns package-level and func-literal " +
			"call sites too, which the first draft of this file missed. All three build raw from a " +
			"server `errors` array decoded straight off the generation payload; nothing in this " +
			"package hands it a flag value, an --input file or anything else the user typed",
	},
}

// dedupeReasonsCallers is the second hop: the set of functions that build the
// array hasPrintableContentArgs' single row delegates to.
//
// It is a LEDGER, not a count. The prose it replaces said "four" and named a
// method that does not exist, and a count alone would have been satisfied by
// any four functions at all.
var dedupeReasonsCallers = map[string]genapiArgOrigin{
	"(Step).failureReasons": {
		server: true,
		why: "s.Output.Errors — `steps[].output.errors` on the getWorkflow payload, decoded by " +
			"encoding/json from the response body",
	},
	"(*Workflow).FailureReasons": {
		server: true,
		why:    "every step's Output.Errors on the getWorkflow payload, concatenated in step order",
	},
	"(ListedWorkflow).FailureReasons": {
		server: true,
		why: "every step's Errors on the LIST payload — `steps[].errors`, a SIBLING of `output` " +
			"and a different wire path from the getWorkflow shape (civitai/cli#382)",
	},
}

// The positive controls. A scanner that has stopped matching — the function
// renamed, the walk reading the wrong directory, the parser handed the wrong
// mode — finds no call sites, reports nothing unledgered, and passes. A floor
// makes that state red by construction rather than by luck.
// 🔴 A FLOOR EQUAL TO THE LIVE COUNT MAKES THE SHRINK DIRECTION UNREACHABLE, AND
// THAT IS WHAT minDedupeReasonsCallSites = 3 DID. Removing one caller — precisely
// the "second table that will drift" hazard dedupeReasons exists to prevent —
// produced "CONTROL failure, not a finding: found 2 … want >= 3. The scan is
// broken", which t.Fatalf'd before the stale-row report that exists for exactly
// that case and would have named the orphaned row. Same class as the all-empty
// control next door: a control relabelling a finding as "not a finding".
//
// A floor's job is only "did the scan find ANYTHING" — the ledger's own count
// and stale arms carry the shrink detection, bidirectionally. So the floors are
// 1: enough to catch a scan wired to nothing, low enough that the arms below it
// stay reachable. The natural remedy a reader would otherwise apply ("the floor
// is stale, lower it to 2") is the one that quietly deletes its meaning.
const (
	minGenapiSourceFiles        = 5
	minHasPrintableContentCalls = 1
	minDedupeReasonsCallSites   = 1
	minGenapiParsedFuncsPerScan = 20
	genapiSaferuneWrapper       = "hasPrintableContent"
	// genapiFileLevel is the PREFIX for an owner with no enclosing function —
	// a package-level initialiser or a func literal at package scope. Such a
	// site CAN be ledgered; pretending it could not is how two of them once sat
	// unledgered behind a green suite.
	//
	// Every such owner is qualified by the variable it initialises, so the BARE
	// string is reachable only from a spec with no names, which an
	// *ast.ValueSpec cannot be. An earlier draft called this "a real key"; it is
	// the prefix of one.
	genapiFileLevel              = "<file-level>"
	genapiReasonDedupeEntryPoint = "dedupeReasons"
)

func TestHasPrintableContentArgumentsAreServerBytes(t *testing.T) {
	checkGenapiCallLedger(t, genapiSaferuneWrapper, hasPrintableContentArgs,
		minHasPrintableContentCalls, true,
		"hasPrintableContent hands its argument to saferune.HasVisibleContent, and "+
			"saferune's package doc binds every caller to one rule: the CLI does not apply the "+
			"class to what the USER typed. A new call site has to say where its bytes come from "+
			"before it can pass.")
}

func TestDedupeReasonsCallersAreLedgered(t *testing.T) {
	checkGenapiCallLedger(t, genapiReasonDedupeEntryPoint, dedupeReasonsCallers,
		minDedupeReasonsCallSites, false,
		"dedupeReasons is the one rule for turning a server `errors` array into the reasons a "+
			"renderer may show, and hasPrintableContentArgs' only row delegates to this set. A "+
			"caller building `raw` from anything other than a server field breaks that row without "+
			"touching it.")
}

// checkGenapiCallLedger is the shared body of both assertions above: one rule,
// one place. byArg selects the key shape — the argument ledger keys on
// "<enclosing func>:<arg expr>" because one function may call the wrapper
// several times with different values, while the caller ledger keys on the
// enclosing function alone because what is being pinned is the SET of callers.
func checkGenapiCallLedger(t *testing.T, callee string, ledger map[string]genapiArgOrigin,
	minSites int, byArg bool, remedy string,
) {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot read the package directory: %v", err)
	}

	fset := token.NewFileSet()
	var (
		files        int
		funcs        int
		sites        int
		flat         int
		unclassified []string
		userTyped    []string
		unownable    []string
		valueRefs    []string
	)
	seen := map[string]bool{}

	// PASS 1 — parse every file and collect its declaration units, so an owner
	// collision that SPANS TWO FILES (two `func init()`, one in each) is visible.
	// A per-file check could not see it, and per-file is what the previous three
	// attempts at this property were.
	type parsedFile struct {
		file  *ast.File
		units []genapiOwner
	}
	var parsed []parsedFile
	ownerDecls := map[string]map[token.Pos]bool{}

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, perr := parser.ParseFile(fset, name, nil, 0)
		if perr != nil {
			t.Fatalf("CONTROL failure, not a finding: cannot parse %s: %v", name, perr)
		}
		files++
		units := genapiOwners(file, callee, &funcs)
		for _, u := range units {
			if ownerDecls[u.owner] == nil {
				ownerDecls[u.owner] = map[token.Pos]bool{}
			}
			ownerDecls[u.owner][u.pos] = true
		}
		parsed = append(parsed, parsedFile{file: file, units: units})
	}

	// PASS 2 — walk, now able to ask whether each site's owner is unique.
	for _, pf := range parsed {
		file := pf.file

		// Call positions first, so an identifier that IS the call's Fun is not
		// also counted as a value reference.
		calleeCalls := map[token.Pos]bool{}
		ast.Inspect(file, func(n ast.Node) bool {
			if ce := isGenapiCallTo(n, callee); ce != nil {
				calleeCalls[ce.Fun.Pos()] = true
			}
			return true
		})

		// THE FLAT WALK — one pass over the whole file node, counting every
		// matching call wherever it sits. Deliberately built differently from
		// the attributing walk below so the two can DISAGREE; see the totality
		// control after the loop.
		ast.Inspect(file, func(n ast.Node) bool {
			if fn, ok := n.(*ast.FuncDecl); ok && fn.Name.Name == callee {
				// The callee's own body, skipped here exactly as below, so the
				// two counts compare like with like.
				return false
			}
			if isGenapiCallTo(n, callee) != nil {
				flat++
			}
			// 🔴 A FUNCTION VALUE ESCAPES BOTH WALKS, AND THE TOTALITY CONTROL
			// CANNOT SEE IT because flat and sites move together. Measured:
			//   var visible = hasPrintableContent
			//   func KeepUserTyped(u string) bool { return visible(u) }
			// left this package and the module-root ledger both green. The
			// module-root guard refuses exactly this shape for saferune itself;
			// there was no analogue one level down, which is where the
			// delegation actually lands.
			if id, ok := n.(*ast.Ident); ok && id.Name == callee && !calleeCalls[id.Pos()] {
				valueRefs = append(valueRefs,
					fmt.Sprintf("%s: %s referenced as a value, not called", fset.Position(id.Pos()), callee))
			}
			return true
		})

		// THE ATTRIBUTING WALK. It covers every top-level declaration, not only
		// FuncDecls.
		//
		// 🔴 IT WALKED FuncDecls ONLY, AND THAT WAS A DEMONSTRATED SURVIVED
		// MUTANT AGAINST THE EXACT RELATIONSHIP THIS TEST PINS. Adding
		//
		//	var untrustedReasons = dedupeReasons([]string{"…"})
		//	var reasonsFor = func(raw []string) []string { return dedupeReasons(raw) }
		//
		// to this package left `go test ./...` FULLY GREEN, with this test still
		// logging "scanned 3 dedupeReasons(...) call site(s)" — two unledgered
		// callers of the function whose caller set it exists to hold. A call in a
		// package-level initialiser or in a func literal at package scope has no
		// enclosing FuncDecl, so the old walk could not see it AND the `sites`
		// floor could not either: both were computed from the same restricted
		// traversal, which is a positive control that shares the step it is
		// supposed to be checking.
		for _, decl := range pf.units {
			enclosing := decl.owner
			shared := len(ownerDecls[enclosing]) > 1
			ast.Inspect(decl.node, func(n ast.Node) bool {
				ce := isGenapiCallTo(n, callee)
				if ce == nil {
					return true
				}
				sites++
				if shared {
					// Checked BEFORE the ledger and never against it: a key
					// more than one declaration claims cannot carry a row that
					// means anything, and letting a row answer for it is the
					// collision this guard exists to stop.
					unownable = append(unownable,
						fmt.Sprintf("%s: %s(%s) — owner key %q is claimed by %d declarations",
							fset.Position(ce.Lparen), callee, genapiRenderExpr(ce.Args[0]),
							enclosing, len(ownerDecls[enclosing])))
					return true
				}
				key := enclosing
				if byArg {
					key += ":" + genapiRenderExpr(ce.Args[0])
				}
				seen[key] = true
				origin, known := ledger[key]
				switch {
				case !known:
					unclassified = append(unclassified,
						fmt.Sprintf("%s: %s(%s) in %s — key %q",
							fset.Position(ce.Lparen), callee, genapiRenderExpr(ce.Args[0]),
							enclosing, key))
				case !origin.server:
					userTyped = append(userTyped,
						fmt.Sprintf("%s: %s(%s) in %s — %s",
							fset.Position(ce.Lparen), callee, genapiRenderExpr(ce.Args[0]),
							enclosing, origin.why))
				}
				return true
			})
		}
	}

	if files < minGenapiSourceFiles {
		t.Fatalf("CONTROL failure, not a finding: only %d non-test source file(s) parsed in this package", files)
	}
	if funcs < minGenapiParsedFuncsPerScan {
		t.Fatalf("CONTROL failure, not a finding: only %d top-level func(s) parsed — the walk is "+
			"reading something, but not this package", funcs)
	}
	if sites < minSites {
		t.Fatalf("CONTROL failure, not a finding: found %d %s(...) call site(s), want >= %d. "+
			"The scan is broken, and a clean result from a broken scan means nothing.",
			sites, callee, minSites)
	}

	// 🔴 THE TOTALITY CONTROL — A SECOND WALK, BUILT DIFFERENTLY, OVER THE SAME
	// QUESTION. The floor above cannot see a call the attributing walk never
	// reached, because the floor counts what that walk found: a control that
	// shares the step it is checking is not a control. `flat` descends the whole
	// file node in one pass instead of iterating declarations, so a hole in the
	// per-declaration attribution surfaces as a DISAGREEMENT between two
	// traversals rather than as a serene number.
	//
	// It does not fire today and is not meant to. It exists so that a future
	// edit narrowing the attributing walk — precisely the defect the first draft
	// of this file shipped — is red at this line instead of quietly counting a
	// subset. internal/cmd/safeterm_userinput_test.go carries the same control
	// for the same reason; this is that shape, not a new idea.
	if flat != sites {
		t.Fatalf("CONTROL failure, not a finding: %d %s(...) call(s) exist in this package but the "+
			"attributing walk reached only %d. The difference is a call no declaration owns — and a "+
			"site with no owner can carry no ledger row, so every verdict below is about a subset "+
			"nobody declared. Widen the attributing walk before trusting either half of this test.",
			flat, callee, sites)
	}

	if len(valueRefs) > 0 {
		sort.Strings(valueRefs)
		t.Errorf("%d reference(s) to %s as a VALUE rather than a call:\n  %s\n\n"+
			"A function value has no call sites this ledger can enumerate, so every row below "+
			"stops meaning anything for whatever calls it. Call %s directly, or give the value a "+
			"named wrapper whose own callers a guard enumerates — and add that guard.",
			len(valueRefs), callee, strings.Join(valueRefs, "\n  "), callee)
	}

	if len(unownable) > 0 {
		sort.Strings(unownable)
		t.Errorf("%d %s(...) call site(s) sit under an owner key more than one declaration "+
			"claims:\n  %s\n\n"+
			"A row on a shared key vouches for every declaration that claims it, which is the "+
			"collision per-owner keying exists to prevent. Give the declarations distinct names.\n"+
			"This is a STATE check — how many declarations claim the key — not a list of special "+
			"names. Three earlier fixes enumerated names (`vs.Names[0]`, then `_` as a key "+
			"sentence, then `_` as a flag on one branch) and each was beaten by the next shape: a "+
			"multi-name spec, a row spelling the sentence, then `func _()` and `func init()`.",
			len(unownable), callee, strings.Join(unownable, "\n  "))
	}

	if len(unclassified) > 0 {
		sort.Strings(unclassified)
		t.Errorf("%d %s(...) call site(s) whose argument ORIGIN is not written down:\n  %s\n\n%s\n"+
			"Add a row keyed exactly as printed above.\nIf the bytes are NOT server-supplied, this "+
			"is civitai/cli#393 reaching internal/genapi: the CLI must not rewrite, or silently "+
			"drop, what the user typed.",
			len(unclassified), callee, strings.Join(unclassified, "\n  "), remedy)
	}

	if len(userTyped) > 0 {
		sort.Strings(userTyped)
		t.Errorf("%d %s(...) call site(s) are classified as NOT server bytes:\n  %s\n\n"+
			"hasPrintableContent decides whether a string is DROPPED. Applying that to input the "+
			"user typed makes the CLI silently discard the user's own text — civitai/cli#393. "+
			"Route those bytes around it, or change saferune's package-doc rule in the same "+
			"commit and justify it there.",
			len(userTyped), callee, strings.Join(userTyped, "\n  "))
	}

	// Shrink direction: a classification nobody uses any more is a stale note,
	// and a stale note reads as coverage.
	var stale []string
	for key := range ledger {
		if !seen[key] {
			stale = append(stale, key)
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("the ledger for %s classifies %d call site(s) that no longer exist:\n  %s\n\n"+
			"Delete the row, or fix the key if the enclosing function was renamed. A row that "+
			"still describes a removed call site reads as coverage of something that is not there.",
			callee, len(stale), strings.Join(stale, "\n  "))
	}

	t.Logf("scanned %d %s(...) call site(s) across %d file(s) and %d func(s); %d origins pinned",
		sites, callee, files, funcs, len(ledger))
}

// genapiFuncKey renders a function's identity including its receiver, so a
// method and a plain function of the same name cannot share one row.
func genapiFuncKey(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return "(" + genapiRenderExpr(fn.Recv.List[0].Type) + ")." + fn.Name.Name
}

// genapiRenderExpr prints the expression shapes these arguments take.
//
// 🔴 THIS COMMENT SAID AN UNRECOGNISED SHAPE "CANNOT COLLIDE WITH A LEDGERED
// KEY" AND THAT WAS FALSE — the same false sentence as the module-root guard's,
// left standing here when that one was corrected, because the correction was
// done by hand at one of the two sites. `<unrecognised %T>` is keyed on the AST
// TYPE, so two different expressions of one type render identically and one row
// covers both. `a + b` (*ast.BinaryExpr) is the common case and is handled now,
// along with parens and unary operators.
//
// The residual, stated rather than re-asserted away: a shape still unknown to
// this function renders by type and CAN be shared. It is bounded here rather
// than refused, because this renderer only feeds keys for
// hasPrintableContentArgs, whose single site takes a plain identifier — the
// module-root guard, whose keys carry arbitrary caller expressions, refuses an
// unnameable argument outright instead.
func genapiRenderExpr(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return genapiRenderExpr(v.X) + "." + v.Sel.Name
	case *ast.IndexExpr:
		return genapiRenderExpr(v.X) + "[" + genapiRenderExpr(v.Index) + "]"
	case *ast.StarExpr:
		return "*" + genapiRenderExpr(v.X)
	case *ast.ParenExpr:
		return "(" + genapiRenderExpr(v.X) + ")"
	case *ast.UnaryExpr:
		return v.Op.String() + genapiRenderExpr(v.X)
	case *ast.BinaryExpr:
		return genapiRenderExpr(v.X) + " " + v.Op.String() + " " + genapiRenderExpr(v.Y)
	case *ast.BasicLit:
		return v.Value
	case *ast.CallExpr:
		parts := make([]string, 0, len(v.Args))
		for _, a := range v.Args {
			parts = append(parts, genapiRenderExpr(a))
		}
		return genapiRenderExpr(v.Fun) + "(" + strings.Join(parts, ", ") + ")"
	case *ast.ArrayType:
		if v.Len == nil {
			return "[]" + genapiRenderExpr(v.Elt)
		}
		return "[" + genapiRenderExpr(v.Len) + "]" + genapiRenderExpr(v.Elt)
	default:
		return fmt.Sprintf("<unrecognised %T>", e)
	}
}

// isGenapiCallTo returns the call when n is a one-argument call to callee, and
// nil otherwise.
//
// ONE spelling of the match, used by BOTH walks above. Two copies could
// disagree, and a disagreement between them would surface as the totality
// control firing — i.e. as a finding about the code under audit rather than
// about the instrument, which is the most expensive kind of wrong answer.
func isGenapiCallTo(n ast.Node, callee string) *ast.CallExpr {
	ce, ok := n.(*ast.CallExpr)
	if !ok {
		return nil
	}
	id, ok := ce.Fun.(*ast.Ident)
	if !ok || id.Name != callee || len(ce.Args) != 1 {
		return nil
	}
	return ce
}

// genapiOwner pairs a syntax node with the key that OWNS any call inside it.
type genapiOwner struct {
	node  ast.Node
	owner string
	// pos is where the declaration starts. Two units with the same owner key
	// are a COLLISION, and this is how they are told apart when reporting it.
	pos token.Pos
}

// genapiOwners splits a file into the units a ledger row can name, so that
// every call site has exactly one owner and no two sites share one.
//
// 🔴 A PACKAGE-LEVEL OWNER IS THE DECLARED NAME, NOT THE BARE SENTINEL, AND THE
// FIRST FIX FOR THE FuncDecl-ONLY WALK GOT THIS WRONG. Attributing every
// file-level call to the literal string "<file-level>" made the two callers in
// the reproducing mutant collapse onto ONE key — so a single row would have
// vouched for both, which is the "one note vouches for two sites" defect the
// per-function keying exists to prevent, reintroduced one level down while
// fixing something else. Each ValueSpec is therefore its own owner, named after
// the variable it initialises.
//
// funcs is incremented per FuncDecl seen, for the caller's positive control.
func genapiOwners(file *ast.File, callee string, funcs *int) []genapiOwner {
	var out []genapiOwner
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			*funcs++
			// The callee's own declaration is not a call site of itself.
			// Skipped in BOTH walks, so the totality control compares like
			// with like.
			if d.Name.Name == callee {
				continue
			}
			out = append(out, genapiOwner{node: d, owner: genapiFuncKey(d), pos: d.Pos()})
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || len(vs.Names) == 0 {
					// A type or import spec cannot hold a call to callee, but
					// it is still walked under the bare sentinel rather than
					// dropped: a unit this function declines to name is a unit
					// the totality control will report as missing, which is the
					// loud failure and the intended one.
					out = append(out, genapiOwner{node: spec, owner: genapiFileLevel, pos: spec.Pos()})
					continue
				}
				// 🔴 ONE OWNER PER VALUE, PAIRED BY INDEX — vs.Names[0] WAS
				// WRONG IN TWO WAYS AND AN AUDIT DEMONSTRATED BOTH.
				// `var a, b = dedupeReasons(x), dedupeReasons(y)` is ONE
				// ValueSpec with two names: keying on Names[0] gave both call
				// sites the key `<file-level>:a`, so one row covered both AND
				// the second site was reported under the wrong variable's name.
				// Names[i] pairs with Values[i] whenever the counts match,
				// which is the only shape that can hold two calls.
				if len(vs.Names) == len(vs.Values) {
					for i, name := range vs.Names {
						out = append(out, genapiOwner{
							node:  vs.Values[i],
							owner: genapiValueOwner(name.Name),
							pos:   vs.Values[i].Pos(),
						})
					}
					continue
				}
				// Counts differ (`var a, b = f()` — one multi-return call).
				// One owner, named for the first variable: the single call
				// genuinely does initialise all of them.
				out = append(out, genapiOwner{
					node:  vs,
					owner: genapiValueOwner(vs.Names[0].Name),
					pos:   vs.Pos(),
				})
			}
		default:
			out = append(out, genapiOwner{node: decl, owner: genapiFileLevel, pos: decl.Pos()})
		}
	}
	return out
}

// genapiValueOwner renders the owner key for a package-level variable.
func genapiValueOwner(name string) string { return genapiFileLevel + ":" + name }

// 🔴 THREE ATTEMPTS AT "NO TWO SITES SHARE A ROW" FAILED BECAUSE ALL THREE
// ENUMERATED NAMES. The first keyed on vs.Names[0], so a multi-name spec
// collided. The second made `_` a key-sentence a row was not supposed to spell,
// and a row spelling it walked past. The third made `_` a FLAG — but wired it
// into the ValueSpec branch only, so `func _() { … }` (legal Go, and legal more
// than once per package) collided exactly as before, and so did two
// `func init()`, which no blocklist of blank identifiers would ever have
// covered.
//
// A blocklist of names cannot be completed by thinking harder; the next shape
// is always one identifier away. So the question is no longer "is this name
// special" but the STATE the guard actually cares about: DOES MORE THAN ONE
// DECLARATION CLAIM THIS OWNER KEY? That is checked below by counting distinct
// declaration positions per key — `_`, `init`, a repeated var name and any
// future shape are all the same answer, and none of them has to be foreseen.
