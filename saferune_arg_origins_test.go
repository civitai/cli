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

// civitai/cli#542's SECOND CLAUSE — WHO ASKS saferune, HOW MANY TIMES, AND WHO
// ANSWERS FOR THE BYTES.
//
// #542's closing condition names two things: a guard that fails when `snippet()`
// "or any `saferune.*` call site outside `internal/cmd`" gains a caller whose
// argument is not server-supplied, AND one that fails when the caller set
// changes at all, bidirectionally. The issue was CLOSED by civitai/cli#557,
// which shipped pkg/civitai's snippetArgs — the right guard for the first
// clause, and it enumerates `snippet(` call sites in `pkg/civitai` only. The
// words "or any `saferune.*` call site outside `internal/cmd`" were covered by
// nothing. A CLOSED ISSUE IS NOT A DISCHARGED CONDITION.
//
// The neighbour, saferune_callers_ledger_test.go, pins WHICH PACKAGES import
// internal/saferune, bidirectionally. Its own comment says what it does not do:
// "It answers 'who calls', never 'with what'." A package already in that ledger
// can grow a second call site without the importer set moving. That is this
// file.
//
// # THIS IS THE REDUCED FORM, AND THE REDUCTION IS THE POINT
//
// 🔴 THE FIRST VERSION KEYED EVERY REFERENCE BY PACKAGE, ENCLOSING DECLARATION
// AND RENDERED ARGUMENT, SO THAT NO TWO SITES COULD SHARE A ROW. Three audit
// rounds found that property broken three times — `vs.Names[0]` collapsing a
// multi-name spec; a refusal encoded in a key's spelling, which a row simply
// spelled; then a blank-identifier flag wired into one branch, which `func _()`
// and `func init()` walked straight past — plus two more classes on the same
// machinery: an unnameable argument rendering by AST TYPE so two expressions
// shared a key, and a control that masked the findings it guarded.
//
// Then the question nobody had asked: HAS THE THING THAT MACHINERY DEFENDS
// AGAINST EVER HAPPENED? Measured over the repository's whole history — EVERY
// PACKAGE HAS HAD EXACTLY ONE saferune CALL SITE, ALWAYS. The set has only ever
// grown by PACKAGE (two to three, when `snippet` arrived), and that direction
// was already covered bidirectionally before this file existed. Not one of the
// colliding shapes those rounds fixed has ever occurred here.
//
// So the identity machinery is gone — ~160 lines that found zero defects in this
// repository's code and nine in themselves. What replaces it is a COUNT. A row
// says how many references it covers; a package that gains a second one fails
// because 2 != 1, and the failure names every position. A count cannot collide
// with itself, cannot be out-spelled, and needs no owner key, no argument
// rendering and no uniqueness proof. It is strictly stronger in the direction
// #542 asks about and has no surface in the direction it does not.
//
// # WHAT THIS ASSERTS
//
//  1. The set of (package, saferune function) pairs referenced in the module's
//     non-test sources is exactly the ledger below — red when it grows and when
//     it shrinks.
//  2. Each pair's reference COUNT matches its row, so a second call site inside
//     a package that already imports saferune is red even though the pair is
//     unchanged. That is #542's clause 2 in one comparison.
//  3. Each row names who answers the origin question for those bytes, resolved
//     to a real Test declaration IN THE SITE'S OWN PACKAGE.
//  4. A bare, non-call reference is refused: `f := saferune.Strip` escapes the
//     enclosing-wrapper model every delegation below rests on.
//
// The import's LOCAL NAMES are resolved rather than assumed, so `import sr
// ".../saferune"` is caught, a file binding the path under two names is caught,
// and a dot import — which would make every reference unqualified and this scan
// blind — is a hard refusal.
type saferuneOriginKind int

const (
	// originUnset is the zero value and is never valid, so a row that forgot to
	// say anything is red rather than defaulting to the reassuring answer.
	originUnset saferuneOriginKind = iota
	// originDelegated means these references sit inside a wrapper whose own
	// parameter is the argument, so the origin question belongs to that
	// wrapper's call sites and is answered by the guard named in pinnedBy.
	originDelegated
	// originUserTyped is civitai/cli#393 reaching this site. Representable so
	// that an author who believes it acceptable must write it down and watch
	// this test say why it is not.
	originUserTyped
)

type saferuneRefOrigin struct {
	kind saferuneOriginKind
	// sites is how many references this row covers. It is the whole growth
	// check: a package gaining a second call site fails here, and no identity
	// scheme is needed to notice.
	sites int
	// pinnedBy is the Test that answers the origin question, resolved in the
	// site's OWN package.
	//
	// 🔴 IT WAS RESOLVED MODULE-WIDE AT FIRST, WHICH IS THE NAME-NOT-A-
	// RELATIONSHIP STATE civitai/cli#578 DELETED FROM THIS REPO SHORTLY BEFORE
	// THIS BRANCH WAS CUT — a stub of the right name in any package satisfied
	// it. Package scoping kills that. RESIDUAL, STATED: it does NOT catch a
	// GUTTED test in the right package, and no static scan can. `pinnedBy` is
	// not evidence the delegated guard is effective; it is evidence the guard
	// named lives where the hazard is.
	pinnedBy string
	// why is the provenance, checkable by reading the named function.
	why string
}

// saferuneRefs is the ledger, keyed "<package dir>:<saferune function>".
//
// Every entry was verified by READING the named function and following the
// value back, not by trusting this file's own prose.
var saferuneRefs = map[string]saferuneRefOrigin{
	"internal/cmd:Strip": {
		kind:     originDelegated,
		sites:    1,
		pinnedBy: "TestSafeTermIsNeverAppliedToUserTypedInput",
		why: "safeTerm's own parameter. This site cannot be classified server/user here and MUST " +
			"NOT BE: internal/cmd routes two deliberately non-server values through safeTerm — " +
			"`--input` file content and download's mixed-origin target path — both enumerated in " +
			"saferune's package doc as documented exceptions. The origin question is answered at " +
			"safeTerm's own call sites by the named guard, which is structural over every one of " +
			"them. No count of those call sites is quoted here: nothing asserts on one, so it " +
			"drifts — an earlier draft quoted a figure already wrong, and its replacement " +
			"disclaimed counts in a sentence containing two",
	},
	"internal/genapi:HasVisibleContent": {
		kind:     originDelegated,
		sites:    1,
		pinnedBy: "TestHasPrintableContentArgumentsAreServerBytes",
		why: "hasPrintableContent's own parameter. Its only call site is dedupeReasons, over a " +
			"trimmed element of the server's `errors` array — a relationship that was prose until " +
			"the named guard, and the prose one line above it was already wrong about its own " +
			"caller count",
	},
	"pkg/civitai:Strip": {
		kind:     originDelegated,
		sites:    1,
		pinnedBy: "TestSnippetArgumentsAreAllServerBytes",
		why: "snippet's own parameter. read.go claims over every present and future call site that " +
			"it is the server's own bytes; the named guard makes that claim checkable, keyed per " +
			"enclosing function",
	},
}

// The positive controls. A scan that has stopped matching finds no references,
// reports nothing unledgered, and passes serenely; a floor makes that state red
// by construction.
const (
	minSaferuneRefs              = 3
	minTestDeclsForPinResolution = 100
	minGoFilesWalked             = 100
)

// saferuneRef is one reference to the saferune package in a non-test source.
type saferuneRef struct {
	key  string
	pos  string
	pkg  string
	bare bool
}

func TestSaferuneReferenceArgumentsAreLedgered(t *testing.T) {
	files := moduleGoFiles(t, false)
	if len(files) < minGoFilesWalked {
		t.Fatalf("CONTROL failure, not a finding: walked only %d non-test .go files in the "+
			"module — the scan is not reading the tree", len(files))
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
		t.Fatalf("CONTROL failure, not a finding: found %d saferune reference(s) in the module's "+
			"non-test sources, want >= %d. The scan is broken, and a clean result from a broken "+
			"scan means nothing.", len(refs), minSaferuneRefs)
	}

	testDecls := moduleTestDeclsByPackage(t)
	total := 0
	for _, names := range testDecls {
		total += len(names)
	}
	if total < minTestDeclsForPinResolution {
		t.Fatalf("CONTROL failure, not a finding: indexed only %d Test declaration(s) across the "+
			"module, want >= %d — the pinnedBy resolver is not reading the tree, and every "+
			"delegation below would be reported missing for that reason",
			total, minTestDeclsForPinResolution)
	}

	// Group by key, keeping every position so a count mismatch can name them.
	seen := map[string][]saferuneRef{}
	for _, r := range refs {
		seen[r.key] = append(seen[r.key], r)
	}

	// 🔴 ALL-EMPTY IS AN INSTRUMENT FAILURE; SOME-EMPTY IS A FINDING, AND THE
	// FIRST VERSION OF THIS CONTROL CONFLATED THEM. It Fatal'd whenever ANY
	// delegated package resolved empty — a strict subset of the dangling-pin
	// check below — so a real finding was relabelled "not a finding" and every
	// later arm went unreached. Measured. If EVERY delegated package resolves
	// empty the index was built from the wrong tree and no verdict means
	// anything; if only some are, the index works and those are genuine.
	delegated := map[string]bool{}
	for key, group := range seen {
		if origin, known := saferuneRefs[key]; known && origin.kind == originDelegated {
			delegated[group[0].pkg] = true
		}
	}
	empty := 0
	for pkg := range delegated {
		if len(testDecls[pkg]) == 0 {
			empty++
		}
	}
	if len(delegated) > 0 && empty == len(delegated) {
		t.Fatalf("CONTROL failure, not a finding: every one of the %d package(s) a row delegates "+
			"into resolved ZERO Test declarations. One empty package is a dangling delegation and "+
			"is a finding; ALL of them empty is the index being built from the wrong tree.",
			len(delegated))
	}

	var grew, shrank, miscounted, userTyped, unpinned, bare []string

	for key, group := range seen {
		origin, known := saferuneRefs[key]
		if !known {
			grew = append(grew, fmt.Sprintf("%s — %d reference(s), first at %s",
				key, len(group), group[0].pos))
			continue
		}
		if len(group) != origin.sites {
			var where []string
			for _, r := range group {
				where = append(where, r.pos)
			}
			sort.Strings(where)
			miscounted = append(miscounted, fmt.Sprintf(
				"%s — ledgered as %d reference(s), found %d:\n      %s",
				key, origin.sites, len(group), strings.Join(where, "\n      ")))
		}
		for _, r := range group {
			if r.bare {
				bare = append(bare, fmt.Sprintf("%s: %s", r.pos, key))
			}
		}
		switch origin.kind {
		case originUserTyped:
			userTyped = append(userTyped, fmt.Sprintf("%s — %s", key, origin.why))
		case originUnset:
			unpinned = append(unpinned, fmt.Sprintf(
				"%s — the row says nothing: kind is the zero value", key))
		case originDelegated:
			pkg := group[0].pkg
			switch {
			case origin.pinnedBy == "":
				unpinned = append(unpinned, fmt.Sprintf("%s — originDelegated with an empty "+
					"pinnedBy: the row defers the question and names nobody to answer it", key))
			case !testDecls[pkg][origin.pinnedBy]:
				unpinned = append(unpinned, fmt.Sprintf("%s — pinnedBy names %s, which no "+
					"_test.go file in %s declares. RENAMED, DELETED, or IN THE WRONG PACKAGE: a "+
					"delegation is only meaningful if the guard it names can see this site",
					key, origin.pinnedBy, pkg))
			}
		}
	}

	for key := range saferuneRefs {
		if len(seen[key]) == 0 {
			shrank = append(shrank, key)
		}
	}

	report := func(items []string, headline, remedy string) {
		if len(items) == 0 {
			return
		}
		sort.Strings(items)
		t.Errorf("%s\n  %s\n\n%s", headline, strings.Join(items, "\n  "), remedy)
	}

	report(grew, fmt.Sprintf("%d (package, saferune function) pair(s) with no row:", len(grew)),
		"GREW. saferune's package doc binds every caller to one rule — the CLI does not sanitise "+
			"what the USER typed on the command line — so a new reference must say where its "+
			"bytes come from before it can pass. Add a row keyed exactly as printed above, with "+
			"its reference count and the Test that answers the origin question at the enclosing "+
			"wrapper's own call sites.")

	report(miscounted, fmt.Sprintf("%d ledgered pair(s) whose reference COUNT moved:", len(miscounted)),
		"This is civitai/cli#542's clause 2: a package that already asks saferune's question has "+
			"gained or lost a call site, and the pair alone cannot see it. Read the new site, "+
			"decide whether its bytes are the server's, and move the count in the same commit.\n"+
			"Measured over this repository's whole history, every package has had exactly ONE "+
			"call site — so a count moving is a real change to the class, not routine drift.")

	report(userTyped, fmt.Sprintf("%d pair(s) classified as the user's own bytes:", len(userTyped)),
		"civitai/cli#393: the strip is unconditional, so applying it to input the user typed makes "+
			"the CLI misreport what the user actually supplied. Route those bytes around saferune, "+
			"or change the package doc's rule in the same commit and justify it there.")

	report(unpinned, fmt.Sprintf("%d pair(s) delegate the origin question to nothing:", len(unpinned)),
		"originDelegated is the honest answer for a one-line wrapper, and it is honest only while "+
			"the guard it names exists where the hazard is.")

	report(bare, fmt.Sprintf("%d BARE (non-call) reference(s):", len(bare)),
		"A value-form reference — `f := saferune.Strip` — escapes the enclosing-wrapper model "+
			"every delegation rests on: the guard named by pinnedBy answers for the wrapper's call "+
			"sites, and a function value has none it can see. Call it directly, or wrap it in a "+
			"named function whose callers a guard can enumerate.")

	report(shrank, fmt.Sprintf("%d row(s) with no matching reference:", len(shrank)),
		"SHRANK. Delete the row, or fix the key if the package or the saferune function was "+
			"renamed. A row describing a reference that is not there reads as coverage of "+
			"something that does not exist — and if it went because the caller re-derived the "+
			"rune class locally, that is the two-tables-that-disagree defect civitai/cli#393 "+
			"exists to prevent.")

	t.Logf("ledgered %d saferune reference(s) over %d pair(s), across %d non-test file(s)",
		len(refs), len(seen), len(files))
}

// saferuneRefsInFile returns every reference to the saferune package in one
// non-test source file.
//
// 🔴 THE IMPORT'S LOCAL NAMES ARE RESOLVED, NEVER ASSUMED TO BE "saferune", AND
// THEY ARE A SET. A guard that greps for the spelling `saferune.` is walkable by
// a one-word edit (`import sr "…/internal/saferune"`), and an earlier draft that
// kept only the LAST matching import was walkable by a file binding the path
// under two names — measured: a planted `sr.Strip(userFlagValue)` was never seen
// at all, and neither gofmt nor golangci-lint rejects the duplicate import. The
// import PATH is the identity; the local names are whatever that file chose.
func saferuneRefsInFile(t *testing.T, path string) ([]saferuneRef, error) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", path, err)
	}

	locals := map[string]bool{}
	for _, imp := range f.Imports {
		if imp.Path == nil || strings.Trim(imp.Path.Value, `"`) != saferuneImportPath {
			continue
		}
		switch {
		case imp.Name == nil:
			locals["saferune"] = true
		case imp.Name.Name == ".":
			t.Fatalf("%s dot-imports %s. Every reference then loses the package qualifier and "+
				"this ledger goes structurally blind. Import it normally.", path, saferuneImportPath)
		case imp.Name.Name == "_":
			continue // a blank import cannot produce a reference
		default:
			locals[imp.Name.Name] = true
		}
	}
	if len(locals) == 0 {
		return nil, nil
	}

	pkgDir := filepath.ToSlash(filepath.Dir(path))

	// Two passes: the calls first, so a selector that is a call's Fun is not
	// also counted as a bare reference. A bare reference is reported rather
	// than ignored — it escapes the wrapper model the delegations rest on.
	calls := map[token.Pos]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		ce, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := ce.Fun.(*ast.SelectorExpr); ok && isSaferuneQualifier(sel.X, locals) {
			calls[sel.Pos()] = true
		}
		return true
	})

	var out []saferuneRef
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || !isSaferuneQualifier(sel.X, locals) {
			return true
		}
		// The key spells the package by its DIRECTORY and the function by its
		// own name, whatever the file called the import: the ledger is about the
		// class, and an alias must not mint a second identity for one pair.
		out = append(out, saferuneRef{
			key:  pkgDir + ":" + sel.Sel.Name,
			pos:  fset.Position(sel.Pos()).String(),
			pkg:  pkgDir,
			bare: !calls[sel.Pos()],
		})
		return true
	})
	return out, nil
}

// isSaferuneQualifier reports whether e is one of the local names this file
// bound to internal/saferune.
func isSaferuneQualifier(e ast.Expr, locals map[string]bool) bool {
	id, ok := e.(*ast.Ident)
	return ok && locals[id.Name]
}

// moduleGoFiles walks the module for .go files, sorted, relative to the module
// root. `tests` selects _test.go files instead of excluding them.
//
// 🔴 IT IS THE ONLY COPY, AND CONSOLIDATING IT IS WHAT FOUND THE SECOND ONE.
// This package already held the same walk twice under two names —
// neterr_ledger_test.go's moduleGoFiles and saferune_callers_ledger_test.go's
// goSourceFiles, byte-identical in body — and adding a third is what made them
// visible. The skip list is a predicate, and a predicate open-coded N times is N
// predicates that will disagree. `.claude/worktrees/` is the live reason it
// matters: it holds whole checkouts of this repo, so a walker that stops
// skipping dot-directories reports every agent worktree's copy of every file.
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

// moduleTestDeclsByPackage indexes every `func TestXxx(...)` in the module,
// KEYED BY THE PACKAGE DIRECTORY IT IS DECLARED IN, so a pinnedBy resolves
// against the package whose site it claims to cover rather than the module as a
// whole. A package's `_test` variant shares the directory key on purpose: it
// sees the same sites and is the same guard from this ledger's point of view.
func moduleTestDeclsByPackage(t *testing.T) map[string]map[string]bool {
	t.Helper()
	out := map[string]map[string]bool{}
	fset := token.NewFileSet()
	for _, path := range moduleGoFiles(t, true) {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("CONTROL failure, not a finding: cannot parse %s: %v", path, err)
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || !strings.HasPrefix(fd.Name.Name, "Test") {
				continue
			}
			if out[dir] == nil {
				out[dir] = map[string]bool{}
			}
			out[dir][fd.Name.Name] = true
		}
	}
	return out
}
