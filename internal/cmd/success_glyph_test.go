package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// ui.Success / ui.Warn / ui.ErrorMsg each PREFIX their own glyph
// (internal/ui/ui.go). A format string handed to one of them that spells the
// same glyph again renders it twice — `✓ Icon set ✓`, which is what three call
// sites in app_listing.go shipped.
//
// 🔴 This is a SOURCE-level guard on purpose, and it is the complement of
// TestAppListingSuccessLinesCarryOneCheckGlyph, not a substitute for it. That
// test proves one rendered line is right; this one proves the mistake is not
// present at any call site in the package, including the two the behavioural
// test does not drive (`Screenshot removed`, `Reordered N screenshots`) and any
// call site added later.
var styledGlyphs = map[string]string{
	"Success":  "✓",
	"Warn":     "⚠",
	"ErrorMsg": "✗",
}

// minStyledCalls is the POSITIVE CONTROL. An AST walk that stops matching —
// a renamed helper, a changed import alias, a walk rooted at the wrong
// directory — finds zero call sites and validates zero of them, which is
// indistinguishable from "every call site is clean". 30+ existed when this
// landed.
const minStyledCalls = 20

func TestStyledHelpersAreNotHandedTheirOwnGlyph(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read ./internal/cmd: %v", err)
	}

	calls := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		{
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkgIdent, ok := sel.X.(*ast.Ident)
				if !ok || pkgIdent.Name != "ui" {
					return true
				}
				glyph, ok := styledGlyphs[sel.Sel.Name]
				if !ok {
					return true
				}
				calls++
				// Every string literal ANYWHERE under the call — including
				// inside a nested fmt.Sprintf — is part of what gets rendered.
				for _, arg := range call.Args {
					ast.Inspect(arg, func(m ast.Node) bool {
						lit, ok := m.(*ast.BasicLit)
						if !ok || lit.Kind != token.STRING {
							return true
						}
						s, err := strconv.Unquote(lit.Value)
						if err != nil {
							s = lit.Value
						}
						if strings.Contains(s, glyph) {
							t.Errorf("%s: ui.%s(%s) — the argument spells %q, which ui.%s already prefixes, so the line renders it twice",
								fset.Position(lit.Pos()), sel.Sel.Name, lit.Value, glyph, sel.Sel.Name)
						}
						return true
					})
				}
				return true
			})
		}
	}

	if calls < minStyledCalls {
		t.Fatalf("CONTROL failure: the walk found only %d ui.Success/Warn/ErrorMsg call sites, want >= %d. "+
			"A scan that finds nothing validates nothing and would otherwise report a serene pass.",
			calls, minStyledCalls)
	}
	t.Logf("checked %d styled-helper call sites", calls)
}
