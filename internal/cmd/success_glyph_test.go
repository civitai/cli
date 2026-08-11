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
//
// 🔴 AND IT MUST SEE EVERY SPELLING, BECAUSE FOR ONE REVISION IT SAW ONE OF
// THREE. The helpers exist as package functions (`ui.Warn(s)`) AND as methods on
// the styler — inline (`ui.For(w).Warn(s)`), via a local binding
// (`st := ui.For(w)`; `st.Warn(s)`), and via a `ui.Styler` PARAMETER
// (`func spendFilteredNotice(sty ui.Styler, …)`). The first version of this walk
// required the receiver to be the identifier `ui`, which matched the 26
// package-function sites and was structurally blind to the other 35 — more than
// half the package. Measured, not theorised: an audit injected
// `"⚠ --quantity %d is above…"` into the `ui.For(errw).Warn(...)` at
// generate.go:1050 — the exact doubled-glyph bug this guard exists for — and the
// guard reported a serene pass over "26 call sites" with the whole package
// green. So the claim above is scoped to what the walk can actually reach: see
// styledCallReceiver and stylerBindings for the shapes, and minStyledCalls plus
// the shape control for what fails if a rename silences a family again.
var styledGlyphs = map[string]string{
	"Success":  "✓",
	"Warn":     "⚠",
	"ErrorMsg": "✗",
}

// minStyledCalls is the POSITIVE CONTROL. An AST walk that stops matching —
// a renamed helper, a changed import alias, a walk rooted at the wrong
// directory, or a receiver shape it does not recognise — finds fewer call sites
// and validates fewer of them, which is indistinguishable from "every call site
// is clean". 61 existed across all four spellings when this bound was set; the
// receiver-blind version of this walk found 26 and the old bound was 20, so the
// control passed straight through the blindness it was supposed to catch. Keep
// this WELL above the 26 package-function sites — that is the number a
// regression to receiver-blindness lands on.
const minStyledCalls = 55

// stylerBindings collects the identifiers a file binds to a styler, so a
// `st.Warn(...)` call is recognised without accepting every `.Warn(` in the
// file. Requiring a visible binding is what keeps an unrelated
// `logger.Warn("⚠ …")` from being reported as this bug.
//
// THREE spellings produce such an identifier, and the third was found by
// counting: `st := ui.For(w)`, a `var st = ui.For(w)`, and — the one that had no
// counterpart in the first repair — a `ui.Styler` PARAMETER
// (`func spendFilteredNotice(sty ui.Styler, …)`, three call sites in
// app_dev_token.go). A walk that knows only the first two lands on 58 of 61.
func stylerBindings(file *ast.File) map[string]bool {
	bound := map[string]bool{}
	record := func(lhs, rhs []ast.Expr) {
		for i, r := range rhs {
			if i >= len(lhs) || !isUIForCall(r) {
				continue
			}
			if id, ok := lhs[i].(*ast.Ident); ok {
				bound[id.Name] = true
			}
		}
	}
	// Any name DECLARED as ui.Styler — parameter, result, receiver or var.
	recordTyped := func(fields *ast.FieldList) {
		if fields == nil {
			return
		}
		for _, f := range fields.List {
			if !isUIStylerType(f.Type) {
				continue
			}
			for _, nm := range f.Names {
				bound[nm.Name] = true
			}
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			record(s.Lhs, s.Rhs)
		case *ast.ValueSpec:
			lhs := make([]ast.Expr, 0, len(s.Names))
			for _, nm := range s.Names {
				lhs = append(lhs, nm)
			}
			record(lhs, s.Values)
			if isUIStylerType(s.Type) {
				for _, nm := range s.Names {
					bound[nm.Name] = true
				}
			}
		case *ast.FuncDecl:
			recordTyped(s.Recv)
			if s.Type != nil {
				recordTyped(s.Type.Params)
				recordTyped(s.Type.Results)
			}
		case *ast.FuncType:
			recordTyped(s.Params)
			recordTyped(s.Results)
		}
		return true
	})
	return bound
}

// isUIStylerType reports whether e names the `ui.Styler` type.
func isUIStylerType(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Styler" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "ui"
}

// isUIForCall reports whether e is a `ui.For(...)` call.
func isUIForCall(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "For" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "ui"
}

// styledCallReceiver classifies the receiver of a glyph-prefixing call and
// returns a human name for it, or "" if this is not one of the known spellings.
func styledCallReceiver(sel *ast.SelectorExpr, bound map[string]bool) string {
	switch x := sel.X.(type) {
	case *ast.Ident:
		if x.Name == "ui" {
			return "ui." + sel.Sel.Name // package function
		}
		if bound[x.Name] {
			return x.Name + "." + sel.Sel.Name // styler bound to a local
		}
	case *ast.CallExpr:
		if isUIForCall(x) {
			return "ui.For(…)." + sel.Sel.Name // styler used inline
		}
	}
	return ""
}

func TestStyledHelpersAreNotHandedTheirOwnGlyph(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read ./internal/cmd: %v", err)
	}

	calls := 0
	byShape := map[string]int{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		bound := stylerBindings(file)
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			glyph, ok := styledGlyphs[sel.Sel.Name]
			if !ok {
				return true
			}
			shape := styledCallReceiver(sel, bound)
			if shape == "" {
				return true
			}
			calls++
			byShape[shape]++
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
						t.Errorf("%s: %s(%s) — the argument spells %q, which %s already prefixes, so the line renders it twice",
							fset.Position(lit.Pos()), shape, lit.Value, glyph, shape)
					}
					return true
				})
			}
			return true
		})
	}

	// 🔴 SHAPE CONTROL. The count alone cannot tell "both spellings scanned"
	// from "one spelling scanned twice as often": the blind version of this walk
	// found 26 real call sites and reported a perfectly plausible number. Each
	// receiver shape must be observed at least once, or the walk has gone blind
	// to a whole family again.
	for _, shape := range []string{"ui.", "ui.For(…).", ""} {
		found := 0
		for k, n := range byShape {
			if shape == "" {
				// The `st := ui.For(w)` binding: neither of the two prefixes above.
				if !strings.HasPrefix(k, "ui.") {
					found += n
				}
				continue
			}
			if strings.HasPrefix(k, shape) && (shape != "ui." || !strings.HasPrefix(k, "ui.For")) {
				found += n
			}
		}
		if found == 0 {
			t.Errorf("CONTROL failure: the walk matched no call of the %q receiver shape. That family is now invisible "+
				"to this guard, which is exactly how the doubled-glyph bug survived it once already. Shapes seen: %v",
				shape, byShape)
		}
	}

	if calls < minStyledCalls {
		t.Fatalf("CONTROL failure: the walk found only %d styled-helper call sites, want >= %d. "+
			"A scan that finds nothing validates nothing and would otherwise report a serene pass. Shapes seen: %v",
			calls, minStyledCalls, byShape)
	}
	t.Logf("checked %d styled-helper call sites across %d receiver shapes: %v", calls, len(byShape), byShape)
}
