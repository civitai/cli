package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestSafeTermErrCallersAreLedgered pins WHERE safeTermErr may be called.
//
// 🔴 WHY A LEDGER AND NOT A DOC COMMENT. safeTermErr collapses \n and \t, not
// just the invisible class. That is only safe because every caller is on
// download.go's SINGLE-LINE error path — and for a while the function's own doc
// comment said both "use it at ANY site where %w carries server-derived bytes"
// AND "every caller is on download.go's single-line error path", in the same
// paragraph. Both cannot be the contract, and nothing in the suite decided it.
//
// The failure the wide reading produces is silent: a caller elsewhere wrapping a
// legitimately MULTI-LINE server string — an orchestrator failure reason, the one
// case indentContinuation exists to format — gets flattened to one line, and no
// test notices because no test knows the caller set.
//
// 🔴 THIS FAILS BOTH WAYS ON PURPOSE. A ledger that only fails when the set GROWS
// lets a caller be deleted silently, which is how a guard quietly stops covering
// anything; one that only fails when it SHRINKS lets a new unreviewed caller in.
// Adding a caller is not forbidden — it requires deciding, in this file, that the
// new site is single-line, and widening safeTermErr's doc comment to match.
func TestSafeTermErrCallersAreLedgered(t *testing.T) {
	// file -> enclosing function, for every permitted safeTermErr call site.
	// Every entry is on download.go's single-line error path.
	ledger := map[string][]string{
		"download.go": {
			"downloadOne",
			"downloadOne",
			"writePart",
			"writePart",
			"writePart",
		},
	}

	fset := token.NewFileSet()
	pkg, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse internal/cmd: %v", err)
	}

	found := map[string][]string{}
	total := 0
	for _, p := range pkg {
		for path, f := range p.Files {
			base := filepath.Base(path)
			var fn string
			ast.Inspect(f, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.FuncDecl:
					fn = v.Name.Name
				case *ast.CallExpr:
					id, ok := v.Fun.(*ast.Ident)
					if ok && id.Name == "safeTermErr" {
						found[base] = append(found[base], fn)
						total++
					}
				}
				return true
			})
		}
	}

	// POSITIVE CONTROL. A scan that matched nothing would satisfy every
	// "unledgered set is empty" check below and report a cheerful pass — the
	// reassuring zero that is indistinguishable from a harness wired to nothing.
	if total == 0 {
		t.Fatal("CONTROL failure, not a finding: the AST scan found ZERO safeTermErr calls in internal/cmd. " +
			"The function is called at least five times, so this is a broken scanner, not a clean tree. " +
			"Every assertion below is vacuous until this passes.")
	}

	norm := func(m map[string][]string) map[string][]string {
		out := map[string][]string{}
		for k, v := range m {
			c := append([]string(nil), v...)
			sort.Strings(c)
			out[k] = c
		}
		return out
	}
	want, got := norm(ledger), norm(found)

	for file, fns := range got {
		if _, ok := want[file]; !ok {
			t.Errorf("safeTermErr is called in %s, which is NOT on the ledger (functions: %v).\n"+
				"safeTermErr COLLAPSES \\n and \\t, so it is only safe where the message is a single line by "+
				"construction. If %s's call sites really are single-line, add them here AND widen safeTermErr's "+
				"doc comment, which currently scopes itself to download.go. If any of them can carry a "+
				"legitimately multi-line server string — an orchestrator failure reason — use safeTerm and "+
				"indentContinuation instead.", file, fns, file)
		}
	}
	for file, fns := range want {
		if _, ok := got[file]; !ok {
			t.Errorf("the ledger names %s as holding safeTermErr calls (%v) and the tree has NONE. "+
				"If they were deliberately removed, delete the row; a ledger that outlives its subject "+
				"reads as coverage while providing none.", file, fns)
			continue
		}
		if strings.Join(got[file], ",") != strings.Join(fns, ",") {
			t.Errorf("safeTermErr's call sites in %s have MOVED.\n  ledger: %v\n  tree:   %v\n"+
				"Both directions matter: a new site needs the single-line decision made for it, and a "+
				"vanished one means the guard no longer covers what this ledger claims.", file, fns, got[file])
		}
	}
}
