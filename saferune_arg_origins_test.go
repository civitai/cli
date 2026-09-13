package cli_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// civitai/cli#542's SECOND HALF — WHAT EVERY saferune.* REFERENCE IS GIVEN.
//
// #542's closing condition names two things: a guard that fails when
// `snippet()` "or any `saferune.*` call site outside `internal/cmd`" gains a
// caller whose argument is not server-supplied, AND one that fails when the
// caller set changes at all, bidirectionally. The issue was CLOSED by
// civitai/cli#557, which shipped pkg/civitai's snippetArgs ledger — and that
// ledger enumerates `snippet(` call sites in `pkg/civitai` only. It is the
// right guard for the clause it covers and it cannot reach the other one:
// measured on the tree this file was written against, `saferune.*` appears in
// non-test sources at THREE places, and snippetArgs sees exactly one of them.
//
//	internal/cmd/safeterm.go      saferune.Strip              (inside safeTerm)
//	internal/genapi/status.go     saferune.HasVisibleContent  (inside hasPrintableContent)
//	pkg/civitai/read.go           saferune.Strip              (inside snippet)
//
// A CLOSED ISSUE IS NOT A DISCHARGED CONDITION, which is the durable part: the
// second clause sat unpinned for a full arc behind a green suite and a closed
// issue, because the guard that closed it answered the first clause well.
//
// 🔴 WHY THE MODULE-ROOT LEDGER NEXT DOOR CANNOT ANSWER THIS EITHER.
// saferune_callers_ledger_test.go pins WHICH PACKAGES import internal/saferune,
// bidirectionally, and resolves each ledgered question to a real declaration.
// Its own comment says what it does not do: "It answers 'who calls', never
// 'with what'." A package already in that ledger can grow a second, unrelated
// `saferune.Strip(userTypedFlagValue)` call and the set of importers does not
// move. That is this file.
//
// # WHAT THIS ASSERTS
//
//  1. Every reference to the saferune package in the module's non-test sources
//     is ledgered, keyed by PACKAGE, ENCLOSING FUNCTION and the rendered
//     reference — so one note can never vouch for two sites.
//  2. Each row says where the bytes at that site come from, and a row that says
//     they are the user's own is a failure, not a classification: saferune's
//     package doc states the rule it binds every caller to — "the CLI does not
//     sanitise what the USER typed on the command line".
//  3. It fails BOTH WAYS. A site with no row is the set growing; a row with no
//     site is a stale note, and a stale note reads as coverage.
//  4. It fails when the set shrinks in the OTHER sense too — an unledgered
//     import alias or a bare (non-call) reference is caught structurally rather
//     than by spelling, so `sr "…/saferune"` or `f := saferune.Strip` cannot
//     walk around it.
//
// # THE ANSWER EVERY ROW ACTUALLY GIVES, AND WHY IT NEEDED ITS OWN KIND
//
// All three sites today are `originDelegated`, and that is the finding rather
// than a shortcut. Every one of them sits inside a ONE-LINE WRAPPER whose
// argument is the wrapper's own parameter, so nothing about the bytes' origin
// is decidable at the saferune call site itself — it is decided at the
// wrapper's call sites, one level out. A ledger that only asked "are these
// bytes server-supplied?" would have to answer "cannot tell here" three times
// out of three, which is how a guard ends up reading as coverage while
// providing none.
//
// So `originDelegated` carries a `pinnedBy`, and `pinnedBy` is RESOLVED TO A
// REAL Test DECLARATION before it is believed — the same move
// checkQuestionsResolve makes next door, for the same reason: a delegation to a
// guard that has been renamed or deleted is worse than no delegation, because
// it names a check nobody will look for.
type saferuneOriginKind int

const (
	// originUnset is the zero value and is never a valid row. It exists so a
	// row that forgot to say anything is red rather than defaulting to the
	// reassuring answer.
	originUnset saferuneOriginKind = iota
	// originServer means the bytes at THIS site are the server's own, decidable
	// by reading the enclosing function. No row uses it today — every current
	// site delegates — and it is kept because a future direct call is the
	// normal case, not an exotic one.
	originServer
	// originDelegated means the argument is the enclosing function's own
	// parameter, so the origin question belongs to that function's call sites
	// and is answered by the guard named in pinnedBy.
	originDelegated
	// originUserTyped is civitai/cli#393 reaching this site. It is
	// representable so that an author who believes it is acceptable has to
	// write it down and watch this test say why it is not.
	originUserTyped
)

type saferuneRefOrigin struct {
	kind saferuneOriginKind
	// pinnedBy is the Test function that answers the origin question for this
	// site. Required for originDelegated, and checked to exist.
	pinnedBy string
	// why is the provenance, specific enough to be checkable by reading the
	// named function — not "it's server data".
	why string
}

// saferuneRefs is the ledger. The key is
// "<package dir>:<enclosing func>:<rendered reference>".
//
// 🔴 THE ENCLOSING FUNCTION IS IN THE KEY ON PURPOSE. `s` is the commonest
// parameter name in this module and `raw` appears in three different functions
// in pkg/civitai alone; a ledger keyed on the argument spelling alone lets one
// classification vouch for every site that happens to use that name. That
// failure has already been paid for twice here — civitai/cli#557 recorded it
// while building snippetArgs, and civitai/cli#582 fixed the live instance of it
// in internal/cmd's bare-identifier ledger.
//
// Every entry below was verified by READING the named function and following
// the value back, not by trusting this file's own prose.
var saferuneRefs = map[string]saferuneRefOrigin{
	"internal/cmd:safeTerm:saferune.Strip(s)": {
		kind:     originDelegated,
		pinnedBy: "TestSafeTermIsNeverAppliedToUserTypedInput",
		why: "safeTerm's own parameter. This site cannot be classified server/user here and " +
			"MUST NOT BE: internal/cmd routes two deliberately non-server values through safeTerm " +
			"— `--input` file content and download's mixed-origin target path — both enumerated in " +
			"saferune's package doc as documented exceptions. The origin question is therefore " +
			"answered across safeTerm's ~150 call sites by the named guard, which is structural " +
			"over every one of them",
	},
	"internal/genapi:hasPrintableContent:saferune.HasVisibleContent(s)": {
		kind:     originDelegated,
		pinnedBy: "TestHasPrintableContentArgumentsAreServerBytes",
		why: "hasPrintableContent's own parameter. Its only call site is dedupeReasons, over a " +
			"trimmed element of the server's `errors` array; that relationship was prose until " +
			"the named guard, and the prose one line above it was already wrong about its own " +
			"caller count",
	},
	"pkg/civitai:snippet:saferune.Strip(string(raw))": {
		kind:     originDelegated,
		pinnedBy: "TestSnippetArgumentsAreAllServerBytes",
		why: "snippet's own parameter. read.go claims over every present and future call site that " +
			"it is the server's own bytes; the named guard is what makes that claim checkable, " +
			"keyed per enclosing function",
	},
}

// minSaferuneRefs is the POSITIVE CONTROL on the scan. A walk that has stopped
// matching — the import path retyped, the alias resolution broken, the walker
// reading the wrong directory — finds no references, reports no unledgered
// sites, and passes serenely. A floor makes that state red by construction.
//
// It is the count measured when this file was written. Raising it is a claim
// that the class is shared more widely; lowering it needs the same review a
// SHRANK failure gets.
const minSaferuneRefs = 3

// minTestDeclsForPinResolution is the positive control on the pinnedBy
// resolver. An index built from the wrong tree is empty, and an empty index
// reports every ledgered guard as missing — a red for the wrong reason, which
// is the shape that gets a real guard deleted.
const minTestDeclsForPinResolution = 100

// saferuneRef is one reference to the saferune package in a non-test source.
type saferuneRef struct {
	key  string
	pos  string
	bare bool
}

func TestSaferuneReferenceArgumentsAreLedgered(t *testing.T) {
	files := moduleGoFiles(t, false)
	// POSITIVE CONTROL on the walk, before any verdict is read off it.
	if len(files) < 100 {
		t.Fatalf("CONTROL failure, not a finding: walked only %d non-test .go files "+
			"in the module — the scan is not reading the tree", len(files))
	}

	var refs []saferuneRef
	for _, path := range files {
		found, err := saferuneRefsInFile(t, path)
		if err != nil {
			t.Fatalf("CONTROL failure, not a finding: %v", err)
		}
		refs = append(refs, found...)
	}

	if len(refs) < minSaferuneRefs {
		t.Fatalf("CONTROL failure, not a finding: found %d saferune reference(s) in the "+
			"module's non-test sources, want >= %d. The scan is broken, and a clean "+
			"result from a broken scan means nothing.", len(refs), minSaferuneRefs)
	}

	testDecls := moduleTestDecls(t)
	if len(testDecls) < minTestDeclsForPinResolution {
		t.Fatalf("CONTROL failure, not a finding: indexed only %d Test declarations in the "+
			"module, want >= %d — the pinnedBy resolver is not reading the tree, and every "+
			"delegation below would be reported missing for that reason",
			len(testDecls), minTestDeclsForPinResolution)
	}

	seen := map[string]bool{}
	var unledgered, userTyped, unpinned []string
	for _, r := range refs {
		seen[r.key] = true
		origin, known := saferuneRefs[r.key]
		if !known {
			shape := "call"
			if r.bare {
				shape = "bare reference (not a call — the argument question cannot even be asked)"
			}
			unledgered = append(unledgered,
				fmt.Sprintf("%s: %s — key %q, %s", r.pos, r.key, r.key, shape))
			continue
		}
		switch origin.kind {
		case originUserTyped:
			userTyped = append(userTyped, fmt.Sprintf("%s: %s — %s", r.pos, r.key, origin.why))
		case originUnset:
			unpinned = append(unpinned, fmt.Sprintf("%s: %s — the row says nothing: "+
				"kind is the zero value", r.pos, r.key))
		case originDelegated:
			switch {
			case origin.pinnedBy == "":
				unpinned = append(unpinned, fmt.Sprintf("%s: %s — originDelegated with an "+
					"empty pinnedBy: the row defers the question and names nobody to answer it",
					r.pos, r.key))
			case !testDecls[origin.pinnedBy]:
				unpinned = append(unpinned, fmt.Sprintf("%s: %s — pinnedBy names %s, which no "+
					"_test.go file in this module declares. RENAMED or DELETED: a delegation to "+
					"a guard that is not there names a check nobody will go and read",
					r.pos, r.key, origin.pinnedBy))
			}
		case originServer:
			// Decidable here; nothing further to resolve.
		}
	}

	if len(unledgered) > 0 {
		sort.Strings(unledgered)
		t.Errorf("%d saferune reference(s) with no row in saferuneRefs:\n  %s\n\n"+
			"GREW. saferune's package doc binds every caller to one rule — the CLI does not "+
			"sanitise what the USER typed on the command line — and a new reference has to say "+
			"where its bytes come from before it can pass. Add a row keyed exactly as printed "+
			"above.\nIf the argument is the enclosing function's own parameter, the honest row is "+
			"originDelegated plus the Test that pins THAT function's call sites; if there is no "+
			"such Test, writing one is the work this guard is asking for.",
			len(unledgered), strings.Join(unledgered, "\n  "))
	}

	if len(userTyped) > 0 {
		sort.Strings(userTyped)
		t.Errorf("%d saferune reference(s) are classified as the user's own bytes:\n  %s\n\n"+
			"This is civitai/cli#393: the strip is unconditional, so applying it to input the "+
			"user typed makes the CLI misreport what the user actually supplied. Route those "+
			"bytes around saferune, or change the package doc's rule in the same commit and "+
			"justify it there.",
			len(userTyped), strings.Join(userTyped, "\n  "))
	}

	if len(unpinned) > 0 {
		sort.Strings(unpinned)
		t.Errorf("%d saferune reference(s) delegate the origin question to nothing:\n  %s\n\n"+
			"originDelegated is the honest answer for a one-line wrapper, and it is only honest "+
			"while the guard it names exists.",
			len(unpinned), strings.Join(unpinned, "\n  "))
	}

	// Shrink direction.
	var stale []string
	for key := range saferuneRefs {
		if !seen[key] {
			stale = append(stale, key)
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("saferuneRefs holds %d row(s) with no matching reference:\n  %s\n\n"+
			"SHRANK. Delete the row, or fix the key if the package, the enclosing function or "+
			"the argument expression was renamed. A row describing a site that is not there "+
			"reads as coverage of something that does not exist — and if the reference was "+
			"removed because the caller re-derived the rune class locally, that is the "+
			"two-tables-that-disagree defect civitai/cli#393 exists to prevent.",
			len(stale), strings.Join(stale, "\n  "))
	}

	t.Logf("ledgered %d saferune reference(s) across %d non-test file(s)", len(refs), len(files))
}

// saferuneRefsInFile returns every reference to the saferune package in one
// non-test source file.
//
// 🔴 THE IMPORT'S LOCAL NAME IS RESOLVED, NEVER ASSUMED TO BE "saferune".
// A guard that greps for the spelling `saferune.` is walkable by a one-word
// edit — `import sr "…/internal/saferune"` — which is the "can it pass while
// the hazard exists in a different shape?" question this repo keeps answering
// the hard way. The import PATH is the identity; the local name is whatever
// that file chose.
func saferuneRefsInFile(t *testing.T, path string) ([]saferuneRef, error) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", path, err)
	}

	local := ""
	for _, imp := range f.Imports {
		if imp.Path == nil || strings.Trim(imp.Path.Value, `"`) != saferuneImportPath {
			continue
		}
		switch {
		case imp.Name == nil:
			local = "saferune"
		case imp.Name.Name == ".":
			// A dot import makes every reference unqualified, so no selector
			// scan can see it. That is a decision about the class, not an
			// import style.
			t.Fatalf("%s dot-imports %s. Every reference then loses the package "+
				"qualifier and this ledger goes structurally blind. Import it normally.",
				path, saferuneImportPath)
		case imp.Name.Name == "_":
			// A blank import cannot produce a reference; nothing to ledger.
			continue
		default:
			local = imp.Name.Name
		}
	}
	if local == "" {
		return nil, nil
	}

	pkgDir := filepath.ToSlash(filepath.Dir(path))
	var out []saferuneRef
	for _, decl := range f.Decls {
		enclosing := "<file-level>"
		var node ast.Node = decl
		if fd, ok := decl.(*ast.FuncDecl); ok {
			enclosing = saferuneFuncKey(fd)
			node = fd
		}

		// Two passes over the same declaration: the calls first, so a selector
		// that is a call's Fun is not double-counted as a bare reference. A
		// bare reference is reported SEPARATELY rather than ignored — it
		// escapes the argument question entirely, which is exactly the shape a
		// ledger keyed on call arguments would miss.
		calls := map[token.Pos]*ast.CallExpr{}
		ast.Inspect(node, func(n ast.Node) bool {
			ce, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := ce.Fun.(*ast.SelectorExpr); ok && isIdent(sel.X, local) {
				calls[sel.Pos()] = ce
			}
			return true
		})
		ast.Inspect(node, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || !isIdent(sel.X, local) {
				return true
			}
			rendered := local + "." + sel.Sel.Name
			bare := true
			if ce, isCall := calls[sel.Pos()]; isCall {
				bare = false
				rendered += "(" + saferuneJoinArgs(ce.Args) + ")"
			}
			// The key always spells the package as `saferune`, whatever the
			// file called it: the ledger is about the class, and an alias must
			// not be able to mint a second identity for one site.
			key := pkgDir + ":" + enclosing + ":" +
				"saferune" + strings.TrimPrefix(rendered, local)
			out = append(out, saferuneRef{
				key:  key,
				pos:  fset.Position(sel.Pos()).String(),
				bare: bare,
			})
			return true
		})
	}
	return out, nil
}

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

// saferuneFuncKey renders a function's identity including its receiver, so a
// method and a function of the same name cannot share one row.
func saferuneFuncKey(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return fd.Name.Name
	}
	return "(" + saferuneRenderExpr(fd.Recv.List[0].Type) + ")." + fd.Name.Name
}

// saferuneRenderExpr prints the expression shapes these arguments take.
// Anything unrecognised renders to a form that cannot collide with a ledgered
// key, so an unknown shape is reported as unledgered rather than silently
// treated as covered.
func saferuneRenderExpr(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return saferuneRenderExpr(v.X) + "." + v.Sel.Name
	case *ast.IndexExpr:
		return saferuneRenderExpr(v.X) + "[" + saferuneRenderExpr(v.Index) + "]"
	case *ast.StarExpr:
		return "*" + saferuneRenderExpr(v.X)
	case *ast.CallExpr:
		return saferuneRenderExpr(v.Fun) + "(" + saferuneJoinArgs(v.Args) + ")"
	case *ast.ArrayType:
		if v.Len == nil {
			return "[]" + saferuneRenderExpr(v.Elt)
		}
		return "[" + saferuneRenderExpr(v.Len) + "]" + saferuneRenderExpr(v.Elt)
	default:
		return fmt.Sprintf("<unrecognised %T>", e)
	}
}

func saferuneJoinArgs(args []ast.Expr) string {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		parts = append(parts, saferuneRenderExpr(a))
	}
	return strings.Join(parts, ", ")
}

// moduleGoFiles walks the module for .go files, sorted, relative to the module
// root. `tests` selects _test.go files instead of excluding them.
//
// 🔴 IT IS THE ONLY COPY, AND CONSOLIDATING IT IS WHAT FOUND THE SECOND ONE.
// This package already held the same walk twice under two names —
// neterr_ledger_test.go's moduleGoFiles and saferune_callers_ledger_test.go's
// goSourceFiles, byte-identical in body — and adding a third is what made them
// visible. The skip list is a predicate, and a predicate open-coded N times is
// N predicates that will disagree. `.claude/worktrees/` is the live reason it
// matters here: it holds whole checkouts of this repo, so a walker that stops
// skipping dot-directories reports every agent worktree's copy of every file,
// and the verdict becomes a fact about how many worktrees are lying around.
func moduleGoFiles(t *testing.T, tests bool) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != "." && (strings.HasPrefix(name, ".") || name == "testdata" ||
				name == "node_modules" || name == "dist" || name == "bin") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") != tests {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk module: %v", err)
	}
	sort.Strings(out)
	return out
}

// moduleTestDecls indexes every `func TestXxx(t *testing.T)` declared anywhere
// in the module, so a pinnedBy can be resolved rather than trusted.
func moduleTestDecls(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	fset := token.NewFileSet()
	for _, path := range moduleGoFiles(t, true) {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("CONTROL failure, not a finding: cannot parse %s: %v", path, err)
		}
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || !strings.HasPrefix(fd.Name.Name, "Test") {
				continue
			}
			out[fd.Name.Name] = true
		}
	}
	return out
}
