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
			"parameter. Every caller of dedupeReasons is ledgered below, and all three build raw " +
			"from a server `errors` array decoded straight off the generation payload; nothing in " +
			"this package hands it a flag value, an --input file or anything else the user typed",
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
const (
	minGenapiSourceFiles         = 5
	minHasPrintableContentCalls  = 1
	minDedupeReasonsCallSites    = 3
	minGenapiParsedFuncsPerScan  = 20
	genapiSaferuneWrapper        = "hasPrintableContent"
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
		unclassified []string
		userTyped    []string
	)
	seen := map[string]bool{}

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

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			funcs++
			// The declaration itself is not a call site, and counting it would
			// hand the positive control a free hit.
			if fn.Name.Name == callee {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				ce, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				id, ok := ce.Fun.(*ast.Ident)
				if !ok || id.Name != callee || len(ce.Args) != 1 {
					return true
				}
				sites++
				key := genapiFuncKey(fn)
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
							genapiFuncKey(fn), key))
				case !origin.server:
					userTyped = append(userTyped,
						fmt.Sprintf("%s: %s(%s) in %s — %s",
							fset.Position(ce.Lparen), callee, genapiRenderExpr(ce.Args[0]),
							genapiFuncKey(fn), origin.why))
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

// genapiRenderExpr prints the expression shapes these arguments take. Anything
// unrecognised renders to a form that cannot collide with a ledgered key, so an
// unknown shape is reported as unclassified rather than treated as covered.
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
