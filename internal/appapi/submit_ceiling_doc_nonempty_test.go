package appapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExportedSubmitCeilingDeclsCarryANonEmptyDoc owns exactly ONE half of the
// #585 defect, and the half staticcheck cannot see.
//
// 🔴 THE SPLIT, BECAUSE THE WHOLE POINT IS NOT TO RE-IMPLEMENT A LINTER. #585
// shipped because a missing blank line glued two doc blocks together, and godoc
// gives the whole thing to whichever declaration comes first in source order.
// That leaves TWO wrong outputs at once, and they are separate predicates:
//
//   - THE DOC THAT MOVED lands on a declaration it does not name, so its first
//     word is another identifier. ⟵ ST1022's predicate (ST1020 for methods,
//     ST1021 for types), enabled in .golangci.yml. NOT this test's business.
//   - THE DECLARATION IT LEFT is now BARE — `go doc appapi.MaxSubmitBodyBytes`
//     printed the const line and nothing else. ⟵ THIS TEST, and nothing else in
//     the repo. ST1020-ST1022 fire only on a doc comment that EXISTS and starts
//     with the wrong identifier; an ABSENT doc is not a finding for any of them.
//     Measured on this tree: with all three enabled, deleting this constant's doc
//     block entirely gives golangci-lint rc=0, 0 issues.
//
// So the linter replaced the name-leading assertion of the 139-line AST test
// this file's three-symbol check is carved out of, and replaced nothing else.
// Keep this one small: three named declarations, one predicate, no name checking.
func TestExportedSubmitCeilingDeclsCarryANonEmptyDoc(t *testing.T) {
	// want is the published submit-ceiling surface: the number three docs quote
	// literally, the sentinel callers match on, and the sizing function both are
	// stated in terms of.
	want := []string{"MaxSubmitBodyBytes", "ErrBundleTooLarge", "SubmitBodySize"}

	// Walked file by file rather than with parser.ParseDir (deprecated in Go
	// 1.25) or go/packages (a dependency this repo does not carry for tests).
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: readdir: %v", err)
	}
	fset := token.NewFileSet()
	docs := map[string]string{}
	parsed := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Clean(name), nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("CONTROL failure, not a finding: parse %s: %v", name, err)
		}
		parsed++
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil {
					docs[d.Name.Name] = d.Doc.Text()
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					// A spec's own doc wins; an ungrouped decl's doc belongs to
					// its single spec.
					text := vs.Doc.Text()
					if text == "" && len(d.Specs) == 1 {
						text = d.Doc.Text()
					}
					for _, n := range vs.Names {
						docs[n.Name] = text
					}
				}
			}
		}
	}

	// POSITIVE CONTROL: the walk must actually have found these declarations. A
	// walk that matched nothing satisfies the loop below by iterating an empty
	// map — the reassuring zero, and the only way this test can pass while
	// measuring nothing.
	if parsed == 0 {
		t.Fatal("CONTROL failure, not a finding: no non-test .go files were parsed, so this test is " +
			"measuring nothing")
	}
	for _, n := range want {
		if _, ok := docs[n]; !ok {
			t.Fatalf("CONTROL failure, not a finding: %s was not found among the declarations of the "+
				"%d parsed file(s), so this test is measuring nothing. Found %d named declarations.",
				n, parsed, len(docs))
		}
	}

	for _, n := range want {
		if strings.TrimSpace(docs[n]) == "" {
			t.Errorf("%s has no doc comment, so `go doc appapi.%s` prints the declaration and nothing "+
				"else. That is how #585 shipped, and no linter in this repo can see it: ST1020-ST1022 "+
				"check that a doc comment BEGINS with its own identifier and say nothing about one "+
				"being absent.\nThe usual cause is a MISSING BLANK LINE — the block above this "+
				"declaration ran into the block above that, and godoc handed the whole thing to the "+
				"earlier one. Look for the declaration now wearing two docs; `make lint` will name it.",
				n, n)
		}
	}
}
