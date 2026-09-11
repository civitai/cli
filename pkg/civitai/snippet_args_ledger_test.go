package civitai

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

// civitai/cli#542 — THE "EVERY ARGUMENT IS SERVER BYTES" INVARIANT, PINNED.
//
// read.go states, in prose, over every present AND FUTURE call site:
//
//	🔴 EVERY ARGUMENT THIS FUNCTION IS EVER GIVEN IS THE SERVER'S OWN BYTES
//
// Until this file, nothing enforced it. That matters because snippet() is the
// one place in this package that decides what an ERROR STRING may carry, and
// cmd/civitai/main.go prints `Error: <err>` straight to stderr with no renderer
// in front of it. A future call site handing it user-typed bytes is the #393
// regression — the CLI rewriting the user's own input back at them — and it
// would land silently.
//
// 🔴 WHY THE EXISTING GUARDS CANNOT SEE THIS, WHICH IS THE WHOLE REASON THE
// GAP EXISTED. Two ledgers already touch this area and neither asks this
// question:
//
//   - internal/cmd/safeterm_userinput_test.go pins argument ORIGINS, which is
//     the right question — but it parses only internal/cmd's own sources and
//     matches only the spelling `safeTerm(<one arg>)`. pkg/civitai is outside
//     its scan and snippet is a different spelling, so it is structurally blind
//     here. That blindness is what #542 records.
//   - saferune_callers_ledger_test.go (module root) pins WHICH packages ask
//     saferune's question, bidirectionally, and resolves each name to a real
//     declaration. It answers "who calls", never "with what".
//
// So: same class, third surface, and this file is the piece that was missing.
// It is deliberately NOT merged into either of the above — one parses a
// different package, the other asks a different question, and folding them
// would make each answer less legible than it is now.
//
// WHAT THIS ASSERTS
//  1. Every snippet(...) call site in this package's non-test sources has its
//     argument's ORIGIN written down, keyed by enclosing function so the same
//     spelling in two places cannot share one note.
//  2. It fails BOTH WAYS — an unclassified site (the set grew) and a classified
//     entry with no matching site (the set shrank, so the note is stale and the
//     next reader would trust it).
//  3. A positive control on the number of call sites, because a scanner that
//     has stopped matching finds no violations and reports a serene pass.

// snippetArgOrigin records, for one snippet() call site, where its bytes come
// from. The key is "<enclosing function>:<argument expression>".
//
// 🔴 KEYED BY ENCLOSING FUNCTION ON PURPOSE, AND THIS IS A DELIBERATE
// STRENGTHENING OVER THE internal/cmd LEDGER IT OTHERWISE MIRRORS. That one is
// keyed by bare name alone, and its own comment concedes the cost: "the same
// name holds a server-returned id at other call sites". Here `raw` appears in
// three different functions across three files. A name-keyed ledger would let
// one classification vouch for all three, so a future `raw` that is NOT server
// bytes would inherit a note written about a different variable — the exact
// shape of a guard that reads as coverage while providing none.
//
// server: true is the invariant. An entry may only be marked server:false if
// the prose in read.go is changed in the same commit to stop claiming
// otherwise — and at that point this test fails loudly and says so, which is
// the review that change deserves.
type snippetArgOrigin struct {
	// server is whether these bytes came off the wire from the server.
	server bool
	// why is the provenance, specific enough to be checkable by reading the
	// named function — not "it's server data".
	why string
}

// Each entry below was verified by READING the named function and following the
// value back to its source, not by trusting the issue that filed this or the
// prose in read.go — which is the claim under test and therefore cannot be the
// evidence for it.
var snippetArgs = map[string]snippetArgOrigin{
	"jsonKind:raw": {
		server: true,
		why: "the raw JSON value handed to FlexString.UnmarshalJSON — bytes encoding/json " +
			"took straight off the response body. jsonKind only ever sees a value UnmarshalJSON " +
			"already declined, so it is a server value by construction",
	},
	"decodeBody:raw": {
		server: true,
		why: "the HTTP response body, passed through getInto/postInto. NOTE the sibling argument " +
			"`path` in the SAME Errorf is deliberately NOT snippet()ed: it is caller-supplied and " +
			"partly user-typed, and #393 says the CLI must not rewrite the user's own bytes",
	},
	"readError:[]byte(msg)": {
		server: true,
		why: "the error text unmarshalled out of the non-2xx body — wrapped.Error (string or raw) " +
			"or wrapped.Message. Note classifyMsg is taken BEFORE this line on purpose: the " +
			"classifier reads what arrived, only the display reads the stripped copy",
	},
	"badRequestDetail:[]byte(d)": {
		server: true,
		why: "a zod issue rendered by firstZodIssue/flattenedZodDetail, both of which parse only " +
			"`raw` — the 400 response body. Three call sites share this key; all three take d from " +
			"one of those two helpers",
	},
	"badRequestDetail:[]byte(s)": {
		server: true,
		why: "the `error` field of the 400 body when it is a plain JSON string, unmarshalled from " +
			"wrapped.Error",
	},
	"badRequestDetail:[]byte(wrapped.Message)": {
		server: true,
		why:    "the `message` field of the 400 body",
	},
	"retryExhaustedError:raw": {
		server: true,
		why:    "the response body from the final failed attempt, handed down by the retry loop",
	},
}

// minSnippetCalls is the POSITIVE CONTROL. A walk that has stopped matching —
// snippet renamed, the package moved, the file list read from the wrong
// directory — scans nothing, finds nothing unclassified, and reports success.
// A floor makes that state red by construction rather than by luck.
const minSnippetCalls = 6

func TestSnippetArgumentsAreAllServerBytes(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot read the package directory: %v", err)
	}

	fset := token.NewFileSet()
	var (
		files        int
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

		// Walk declarations rather than the whole file, so every call site can
		// be attributed to the function that contains it.
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				ce, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				id, ok := ce.Fun.(*ast.Ident)
				if !ok || id.Name != "snippet" || len(ce.Args) != 1 {
					return true
				}
				sites++
				key := fn.Name.Name + ":" + snippetRenderExpr(ce.Args[0])
				seen[key] = true
				origin, known := snippetArgs[key]
				switch {
				case !known:
					unclassified = append(unclassified,
						fmt.Sprintf("%s: snippet(%s) in %s — key %q",
							fset.Position(ce.Lparen), snippetRenderExpr(ce.Args[0]), fn.Name.Name, key))
				case !origin.server:
					userTyped = append(userTyped,
						fmt.Sprintf("%s: snippet(%s) in %s — %s",
							fset.Position(ce.Lparen), snippetRenderExpr(ce.Args[0]), fn.Name.Name, origin.why))
				}
				return true
			})
		}
	}

	if files < 5 {
		t.Fatalf("CONTROL failure, not a finding: only %d non-test source file(s) parsed in this package", files)
	}
	if sites < minSnippetCalls {
		t.Fatalf("CONTROL failure, not a finding: found only %d snippet() call site(s), want >= %d. "+
			"The walk is broken, and a clean result from it means nothing.", sites, minSnippetCalls)
	}

	if len(unclassified) > 0 {
		sort.Strings(unclassified)
		t.Errorf("%d snippet() call site(s) whose argument ORIGIN is not written down:\n  %s\n\n"+
			"read.go claims EVERY argument this function is ever given is the server's own bytes. "+
			"That is a claim over future call sites too, so a new one has to say where its bytes come "+
			"from before it can pass. Add an entry to snippetArgs keyed exactly as printed above.\n"+
			"If the bytes are NOT server-supplied, this is civitai/cli#393 reaching pkg/civitai: the "+
			"CLI must not rewrite what the user typed back at them.",
			len(unclassified), strings.Join(unclassified, "\n  "))
	}

	if len(userTyped) > 0 {
		sort.Strings(userTyped)
		t.Errorf("%d snippet() call site(s) are classified as NOT server bytes:\n  %s\n\n"+
			"snippet() strips runes. Doing that to input the user typed makes the CLI misreport what "+
			"the user actually supplied — civitai/cli#393. Either route those bytes around snippet(), "+
			"or change read.go's invariant prose in the same commit and justify it.",
			len(userTyped), strings.Join(userTyped, "\n  "))
	}

	// Shrink direction: a classification nobody uses any more is a stale note,
	// and a stale note reads as coverage.
	var stale []string
	for key := range snippetArgs {
		if !seen[key] {
			stale = append(stale, key)
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("snippetArgs classifies %d call site(s) that no longer exist:\n  %s\n\n"+
			"Delete the entry, or fix the key if the enclosing function was renamed. A ledger that "+
			"still describes a removed call site reads as coverage of something that is not there.",
			len(stale), strings.Join(stale, "\n  "))
	}

	t.Logf("scanned %d snippet() call site(s) across %d file(s); %d origins pinned",
		sites, files, len(snippetArgs))
}

// snippetRenderExpr prints the expression shapes snippet() arguments take.
// Anything unrecognised renders to a form that cannot collide with a pinned
// key, so an unknown shape is reported as unclassified rather than being
// silently treated as safe.
func snippetRenderExpr(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return snippetRenderExpr(v.X) + "." + v.Sel.Name
	case *ast.IndexExpr:
		return snippetRenderExpr(v.X) + "[" + snippetRenderExpr(v.Index) + "]"
	case *ast.StarExpr:
		return "*" + snippetRenderExpr(v.X)
	case *ast.CallExpr:
		// The common shape here is a conversion: []byte(x), string(x).
		return snippetRenderExpr(v.Fun) + "(" + joinArgs(v.Args) + ")"
	case *ast.ArrayType:
		if v.Len == nil {
			return "[]" + snippetRenderExpr(v.Elt)
		}
		return "[" + snippetRenderExpr(v.Len) + "]" + snippetRenderExpr(v.Elt)
	default:
		return fmt.Sprintf("<unrecognised %T>", e)
	}
}

func joinArgs(args []ast.Expr) string {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		parts = append(parts, snippetRenderExpr(a))
	}
	return strings.Join(parts, ", ")
}
