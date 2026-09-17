package appapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
)

// unauthorized401Arm is one `case http.StatusUnauthorized…:` clause found by the
// AST walk below.
type unauthorized401Arm struct {
	File     string
	Line     int
	Statuses []string // every status the case lists, in source order
	Returns  []string // the source text of each `return` in the clause body
	Routed   bool     // every return in the body reaches unauthorizedError
}

// findUnauthorizedArms parses every non-test .go file in this package and
// returns each 401 case clause, whether it returns unauthorizedError, and the
// statuses the clause lists.
//
// 🔴 THIS REPLACES A REGEX, AND THE REGEX WAS WRONG TWICE IN ONE PR.
//
// Draft 1 was file-granular: `strings.Contains(src, "unauthorizedError(")`, which
// one converted arm satisfied for a file holding five. Round 0 found two live
// arms it had certified past.
//
// Draft 2 was arm-granular but matched `^\s*case http\.StatusUnauthorized:\n…return…`.
// Round 1 found it blind to two shapes that already exist in this package:
//
//   - a case LIST — `case http.StatusUnauthorized, http.StatusForbidden:` is what
//     `withdrawError` used, seventy lines from the code under change, and it was
//     the EIGHTH arm after the count had already been corrected from five to seven;
//   - a clause body that BRANCHES before returning — an `if` ahead of the final
//     `return` makes the regex's single-return assumption miss the whole arm.
//
// Both were proven by surviving mutants. The lesson this repo already records is
// "prefer DELETING a parse to teaching it a better pattern", and two failed
// hardenings of the same regex is the evidence for it: `go/parser` makes both
// blind spots unrepresentable rather than merely handled, because it reads the
// same grammar the compiler does.
func findUnauthorizedArms(t *testing.T) []unauthorized401Arm {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	fset := token.NewFileSet()

	var arms []unauthorized401Arm
	parsed := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		parsed++

		ast.Inspect(f, func(n ast.Node) bool {
			cc, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			var statuses []string
			hit := false
			for _, e := range cc.List {
				sel, ok := e.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok {
					continue
				}
				statuses = append(statuses, pkg.Name+"."+sel.Sel.Name)
				if pkg.Name == "http" && sel.Sel.Name == "StatusUnauthorized" {
					hit = true
				}
			}
			if !hit {
				return true
			}

			// Every RETURN anywhere in the clause body — including inside an `if`,
			// a loop or a nested switch — must reach unauthorizedError. Collecting
			// them all is what makes a branching body representable; the regex
			// could only ever see one.
			var returns []string
			routed := true
			ast.Inspect(cc, func(m ast.Node) bool {
				ret, ok := m.(*ast.ReturnStmt)
				if !ok {
					return true
				}
				text := string(src[fset.Position(ret.Pos()).Offset:fset.Position(ret.End()).Offset])
				returns = append(returns, strings.Join(strings.Fields(text), " "))
				calls := false
				ast.Inspect(ret, func(k ast.Node) bool {
					call, ok := k.(*ast.CallExpr)
					if !ok {
						return true
					}
					if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "unauthorizedError" {
						calls = true
					}
					return true
				})
				if !calls {
					routed = false
				}
				return true
			})
			arms = append(arms, unauthorized401Arm{
				File:     name,
				Line:     fset.Position(cc.Pos()).Line,
				Statuses: statuses,
				Returns:  returns,
				Routed:   routed && len(returns) > 0,
			})
			return true
		})
	}

	// Positive control on the WALK, not on the verdict. A parser that read no
	// files, or an AST matcher that recognised no case clause, reports zero
	// unrouted arms — indistinguishable from full conversion.
	if parsed < 5 {
		t.Fatalf("CONTROL failure: parsed only %d non-test .go files — the walk is reading the wrong tree", parsed)
	}
	if len(arms) < unauthorizedArmFloor {
		t.Fatalf("CONTROL failure: the AST walk found only %d `case http.StatusUnauthorized…:` clause(s), "+
			"want at least %d. It is not recognising case clauses, so a clean verdict below means nothing. "+
			"If arms were legitimately removed, lower the floor deliberately and say why.",
			len(arms), unauthorizedArmFloor)
	}
	return arms
}

// unauthorizedArmFloor is the anti-vacuity floor for the walk, set at the ACTUAL
// arm count rather than comfortably below it.
//
// 🔴 A SLACK FLOOR IS WHAT LET TWO ARMS VANISH UNNOTICED. The previous control
// was `total < 5` against a real count of 7, so a mutant could de-convert two
// arms and stay silent — demonstrated. At the true count the control fires on the
// first arm that stops being recognised, which is the only value that makes it a
// control rather than a formality. Moving it is a decision: say which arm went
// and why, in the same commit.
const unauthorizedArmFloor = 8

// TestEvery401ArmRoutesThroughTheHelper is the seam guard: every 401 answered
// anywhere in this package returns the same message, because a user hitting one
// has the same problem and the same two remedies whichever route they were on.
func TestEvery401ArmRoutesThroughTheHelper(t *testing.T) {
	var unrouted []string
	for _, a := range findUnauthorizedArms(t) {
		if a.Routed {
			continue
		}
		detail := a.File + ":" + itoa(a.Line) + "  case " + strings.Join(a.Statuses, ", ") + ":"
		if len(a.Returns) == 0 {
			detail += "\n        (no return statement in the clause body)"
		}
		for _, r := range a.Returns {
			detail += "\n        " + r
		}
		unrouted = append(unrouted, detail)
	}
	if len(unrouted) == 0 {
		return
	}
	sort.Strings(unrouted)
	t.Errorf("%d `case http.StatusUnauthorized…:` clause(s) do not return unauthorizedError:\n    %s\n\n"+
		"An arm with its own literal is free to disagree with the others, and before this helper "+
		"existed three of them already did — one dropped the CIVITAI_TOKEN route for the command "+
		"group most likely to run in CI, one said \"check your token\", and one said \"check your "+
		"API key / Apps invite\". None of those is a command anyone can run.\n\n"+
		"If an arm must differ, give it its OWN case (not a shared `case A, B:`) and say at the site "+
		"why a 401 there is not the ordinary one.",
		len(unrouted), strings.Join(unrouted, "\n    "))
}

// itoa avoids pulling strconv in for one call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
