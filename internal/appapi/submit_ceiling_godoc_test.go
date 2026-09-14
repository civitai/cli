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

// TestExportedSubmitCeilingDeclsCarryTheirOwnDoc pins the ONE thing that broke
// silently in #585 and that no other check in this repo can see.
//
// 🔴 A MISSING BLANK LINE REASSIGNS A DOC BLOCK, AND EVERYTHING STILL COMPILES.
// MaxSubmitBodyBytes's doc ran into ErrBundleTooLarge's with no blank line
// between them, so godoc handed the whole thing — the proxy-matcher evidence,
// the truncation chain, the two upstream issue numbers — to whichever
// declaration came first in source order. Measured at the commit that shipped:
//
//	go doc appapi.MaxSubmitBodyBytes  ->  the bare const line, nothing else
//	go doc appapi.ErrBundleTooLarge   ->  "MaxSubmitBodyBytes is the largest…"
//
// The constant that three published surfaces quote literally shipped with no
// godoc at all, and the sentinel shipped wearing the constant's evidence. Gofmt
// does not mind. `go vet` does not mind. staticcheck's ST1020-ST1022 would have
// caught the empty half and .golangci.yml disables them.
//
// 🔴 THE ASSERTION IS "THE DOC NAMES ITS OWN DECLARATION", NOT "THE DOC IS
// NON-EMPTY". Non-emptiness is the weaker half and cannot see the failure that
// actually happened: ErrBundleTooLarge's doc was long, detailed and about
// something else entirely. Go's own convention — a doc comment begins with the
// identifier it documents — is what makes the misattribution mechanically
// visible, so that is what is checked.
func TestExportedSubmitCeilingDeclsCarryTheirOwnDoc(t *testing.T) {
	// Walked file by file rather than with parser.ParseDir (deprecated in Go
	// 1.25) or go/packages (a dependency this repo does not carry for tests).
	// Only the doc comment attached to each declaration is needed, and that is
	// on the AST directly.
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

	want := []string{"MaxSubmitBodyBytes", "ErrBundleTooLarge", "SubmitBodySize"}

	// POSITIVE CONTROL: the extractor must actually be finding these
	// declarations. A walk that silently matched nothing would satisfy every
	// loop below by iterating an empty map — the reassuring zero.
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
		text := strings.TrimSpace(docs[n])
		if text == "" {
			t.Errorf("%s has no doc comment. `go doc appapi.%s` prints the declaration and nothing else.\n"+
				"The usual cause is a MISSING BLANK LINE: the block above it runs into the block above "+
				"that, and godoc gives the whole thing to the earlier declaration.", n, n)
			continue
		}
		if !strings.HasPrefix(text, n) {
			// Name the thief when we can, because the fix is a blank line and
			// the reader needs to know where.
			var thief string
			for _, other := range want {
				if other != n && strings.HasPrefix(text, other) {
					thief = other
					break
				}
			}
			msg := "%s's doc comment does not begin with %q — it begins %q.\n" +
				"Go's convention is that a doc comment opens with the identifier it documents, and " +
				"breaking it here means godoc is serving one declaration's evidence as another's."
			if thief != "" {
				msg += "\nThis doc belongs to " + thief + ": the two blocks are glued together by a missing " +
					"blank line, so both `go doc` outputs are wrong at once. Separate them."
			}
			t.Errorf(msg, n, n, firstLine(text))
		}
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 72 {
		s = s[:72] + "…"
	}
	return s
}
