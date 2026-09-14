package appapi

import (
	"go/doc"
	"go/parser"
	"go/token"
	"io/fs"
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
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: parse: %v", err)
	}
	astPkg, ok := pkgs["appapi"]
	if !ok {
		t.Fatalf("CONTROL failure, not a finding: package appapi not found in the parsed dir (got %v) — "+
			"every assertion below would iterate nothing", parsedPkgNames(pkgs))
	}
	d := doc.New(astPkg, "github.com/civitai/cli/internal/appapi", doc.AllDecls)

	// name -> doc text, for the declaration kinds this file cares about.
	docs := map[string]string{}
	for _, c := range d.Consts {
		for _, n := range c.Names {
			docs[n] = c.Doc
		}
	}
	for _, v := range d.Vars {
		for _, n := range v.Names {
			docs[n] = v.Doc
		}
	}
	for _, f := range d.Funcs {
		docs[f.Name] = f.Doc
	}

	want := []string{"MaxSubmitBodyBytes", "ErrBundleTooLarge", "SubmitBodySize"}

	// POSITIVE CONTROL: the extractor must actually be finding these
	// declarations. A parse that silently matched nothing would satisfy every
	// loop below by iterating an empty map — the reassuring zero.
	for _, n := range want {
		if _, ok := docs[n]; !ok {
			t.Fatalf("CONTROL failure, not a finding: %s was not found among the package's parsed "+
				"declarations, so this test is measuring nothing. Parsed %d consts, %d vars, %d funcs.",
				n, len(d.Consts), len(d.Vars), len(d.Funcs))
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

func parsedPkgNames[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
