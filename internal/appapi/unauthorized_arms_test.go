package appapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The invariant: **nowhere in this package does a 401-conditioned branch build
// its own error message.** Every one returns unauthorizedError, so a user who
// hits a 401 reads the same two remedies whichever route they were on.
//
// # This guard has been wrong three times, each time in the same direction
//
// Draft 1 — file-granular `strings.Contains(src, "unauthorizedError(")`. One
// converted arm satisfied a file holding five. Round 0 found two live arms it
// had certified past.
//
// Draft 2 — an arm-granular REGEX, `^\s*case http\.StatusUnauthorized:\n…return…`.
// Round 1 found it blind to `case http.StatusUnauthorized, http.StatusForbidden:`
// — what `withdrawError` used, seventy lines from the code under change — and to
// a clause body that branches before returning. ⚠ Only the FIRST of those existed
// in the tree; the branching shape was proven by a mutant. An earlier version of
// this comment said both "already exist in this package", which is the overclaim
// this file's whole subject is about.
//
// Draft 3 — a go/parser walk over `*ast.CaseClause` only, whose comment claimed
// it "cannot have a pattern blind spot at all". Round 2 refuted that in one pass:
// it saw no `if status == http.StatusUnauthorized`, no `switch { case status ==
// … }`, and no bare `case 401:`. 🔴 **The `if` form is not hypothetical — it is at
// `appblocks.go:580`, in the same file, and the comment denying the blind spot
// was written above code that contained one.** Changing regex for AST removed
// the REGEX's blind spots and left the SHAPE assumption untouched; three
// surfaces then told the reader otherwise.
//
// # Why this draft is keyed differently, and not merely wider
//
// Widening draft 3 to `*ast.IfStmt` would have made `appblocks.go:580` — the
// transparent refresh-and-retry, which constructs no message at all — a false
// positive needing an exemption, and an exemption list is a to-do list that
// grows one line per omission.
//
// So this draft stops asking "what syntax encloses the 401?" and asks the
// question the invariant is actually about: **inside a region conditioned on
// StatusUnauthorized, is any error message CONSTRUCTED?** `fmt.Errorf` and
// `errors.New` are how you construct one here. That formulation:
//
//   - covers case clauses, case LISTS, `if`, `switch {case expr}` and bare `401`
//     alike, because the enclosing syntax stops being an input;
//   - passes the retry at `appblocks.go:580` for the right reason (it builds no
//     message) rather than by an allowlist;
//   - passes INDIRECT routing — an arm returning a wrapper that itself calls
//     unauthorizedError — which draft 3 flagged as a violation while telling the
//     reader to "give it its own case", advice that did not apply.
//
// It is still not omniscient, and this comment will not say that it is: a 401
// message built in a helper the walk never associates with a 401 condition is
// out of scope, as is one assembled from a variable. What it does cover is
// stated above; what it does not is stated here.

// unauthorized401Site is one region conditioned on http.StatusUnauthorized.
type unauthorized401Site struct {
	File   string
	Line   int
	Form   string   // "case" or "if"
	Builds []string // fmt.Errorf / errors.New calls found inside
	Routed bool     // reaches unauthorizedError
}

// unauthorized401Floor is the anti-vacuity floor, set at the ACTUAL site count
// rather than comfortably below it.
//
// 🔴 A SLACK FLOOR IS WHAT LET TWO ARMS VANISH UNNOTICED. Draft 2's control was
// `total < 5` against a real 7, so a mutant could de-convert two arms in silence
// — demonstrated. At the true count the control fires on the first site that
// stops being recognised. Moving it is a decision: say which site went and why,
// in the same commit.
const unauthorized401Floor = 9

// findUnauthorized401Sites parses every non-test .go file in this package and
// returns each region conditioned on StatusUnauthorized, with the error-message
// constructions found inside it.
func findUnauthorized401Sites(t *testing.T) []unauthorized401Site {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	fset := token.NewFileSet()

	// testsUnauthorized reports whether an expression tests StatusUnauthorized —
	// as a case value (`case http.StatusUnauthorized:`), as a comparison
	// (`status == http.StatusUnauthorized`), or as the bare code (`case 401:`).
	var testsUnauthorized func(ast.Expr) bool
	testsUnauthorized = func(e ast.Expr) bool {
		switch v := e.(type) {
		case *ast.SelectorExpr:
			id, ok := v.X.(*ast.Ident)
			return ok && id.Name == "http" && v.Sel.Name == "StatusUnauthorized"
		case *ast.BasicLit:
			return v.Kind == token.INT && v.Value == "401"
		case *ast.BinaryExpr:
			if v.Op != token.EQL && v.Op != token.NEQ {
				return false
			}
			return testsUnauthorized(v.X) || testsUnauthorized(v.Y)
		case *ast.ParenExpr:
			return testsUnauthorized(v.X)
		}
		return false
	}

	var sites []unauthorized401Site
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

		record := func(n ast.Node, form string) {
			var builds []string
			routed := false
			ast.Inspect(n, func(m ast.Node) bool {
				call, ok := m.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch fn := call.Fun.(type) {
				case *ast.Ident:
					if fn.Name == "unauthorizedError" {
						routed = true
					}
				case *ast.SelectorExpr:
					pkg, ok := fn.X.(*ast.Ident)
					if !ok {
						return true
					}
					if (pkg.Name == "fmt" && fn.Sel.Name == "Errorf") ||
						(pkg.Name == "errors" && fn.Sel.Name == "New") {
						p := fset.Position(call.Pos())
						txt := string(src[p.Offset:fset.Position(call.End()).Offset])
						builds = append(builds, strings.Join(strings.Fields(txt), " "))
					}
				}
				return true
			})
			sites = append(sites, unauthorized401Site{
				File: name, Line: fset.Position(n.Pos()).Line,
				Form: form, Builds: builds, Routed: routed,
			})
		}

		ast.Inspect(f, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.CaseClause:
				for _, e := range v.List {
					if testsUnauthorized(e) {
						record(v, "case")
						break
					}
				}
			case *ast.IfStmt:
				if v.Cond != nil && testsUnauthorized(v.Cond) {
					record(v, "if")
				}
			}
			return true
		})
	}

	if parsed < 5 {
		t.Fatalf("CONTROL failure: parsed only %d non-test .go files — the walk is reading the wrong tree", parsed)
	}
	if len(sites) < unauthorized401Floor {
		t.Fatalf("CONTROL failure: the AST walk found only %d region(s) conditioned on StatusUnauthorized, "+
			"want at least %d. It is not recognising them, so a clean verdict below means nothing. "+
			"If a site was legitimately removed, lower unauthorized401Floor deliberately and say which and why.",
			len(sites), unauthorized401Floor)
	}
	return sites
}

// TestNo401SiteBuildsItsOwnMessage is the seam guard.
func TestNo401SiteBuildsItsOwnMessage(t *testing.T) {
	var bad []string
	for _, s := range findUnauthorized401Sites(t) {
		if len(s.Builds) == 0 {
			continue // constructs no message — nothing to diverge
		}
		detail := s.File + ":" + strconv.Itoa(s.Line) + "  (" + s.Form + ")"
		if s.Routed {
			// Both in one region: the site calls the helper AND builds a message.
			// Still a violation — a reader gets whichever branch they hit.
			detail += "  [also calls unauthorizedError — mixed, not exempt]"
		}
		for _, b := range s.Builds {
			if len(b) > 160 {
				b = b[:160] + "…"
			}
			detail += "\n        " + b
		}
		bad = append(bad, detail)
	}
	if len(bad) == 0 {
		return
	}
	sort.Strings(bad)
	t.Errorf("%d region(s) conditioned on StatusUnauthorized build their own error message:\n    %s\n\n"+
		"Every 401 in this package returns unauthorizedError, so a user reads the same two remedies "+
		"whichever route they were on. Before that helper existed FOUR spellings had drifted apart — one "+
		"dropped the CIVITAI_TOKEN route for the command group most likely to run in CI, one said \"check "+
		"your token\", and one said \"check your API key / Apps invite\"; none of those names a command "+
		"anyone can run.\n\n"+
		"Return unauthorizedError(serverMsg), passing \"\" if the server's words are misleading here. "+
		"If a 401 at this site genuinely is not the ordinary one, say why AT THE SITE — this guard reads "+
		"message CONSTRUCTION, so a documented exception still has to be argued in the source.",
		len(bad), strings.Join(bad, "\n    "))
}

// TestUnauthorized401WalkSeesEveryForm is the positive control for the walk's
// SHAPE coverage, and it exists because the previous three drafts each passed
// while blind to a form the package contained.
//
// 🔴 IT PINS THE FORMS, NOT A COUNT. A count floor catches a walk that stopped
// seeing things in bulk; it cannot catch a walk that never saw one particular
// spelling — which is exactly how drafts 2 and 3 stayed green. So this asserts
// that both real forms present in the tree are found, by name and form.
//
// ⚠ MEASURED, AND THE HONEST VERSION IS NARROWER THAN THAT SOUNDS. In the mutant
// that disables the `if` matcher, the FLOOR fires first (9 sites → 8) and this
// test's own assertion never runs. Its distinct value is the case the floor
// cannot see: a form lost while a site is ADDED, leaving the count whole. Stated
// rather than left to a reader who would otherwise credit it with the kill.
func TestUnauthorized401WalkSeesEveryForm(t *testing.T) {
	sites := findUnauthorized401Sites(t)

	forms := map[string]int{}
	for _, s := range sites {
		forms[s.Form]++
	}
	for _, want := range []string{"case", "if"} {
		if forms[want] == 0 {
			t.Errorf("the walk found no `%s` form conditioned on StatusUnauthorized. This package "+
				"contains both (the retry at appblocks.go is an `if`), so a zero here means the "+
				"matcher stopped recognising that shape — which is how every previous draft of this "+
				"guard went blind. Found: %v", want, forms)
		}
	}

	// The retry site specifically: it is the one `if` form, it builds no message,
	// and it must pass WITHOUT an exemption. If it ever starts building one, the
	// guard above should catch it — this asserts it is in scope to be caught.
	found := false
	for _, s := range sites {
		if s.Form == "if" && s.File == "appblocks.go" {
			found = true
		}
	}
	if !found {
		t.Errorf("the transparent refresh-and-retry `if status == http.StatusUnauthorized` in "+
			"appblocks.go is not among the %d site(s) the walk found. It is the reason this guard is "+
			"keyed on message CONSTRUCTION rather than on enclosing syntax; if the walk cannot see it, "+
			"the `if` form is unguarded everywhere.", len(sites))
	}
}
