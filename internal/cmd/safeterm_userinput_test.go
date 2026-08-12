package cmd

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// civitai/cli#393 — THE CLI DOES NOT SANITISE WHAT THE USER TYPED, AND THE
// APPROVAL SCREEN IS WHY.
//
// The first cut of #393 applied safeTerm to `o.prompt`. Measured live through
// the real interactive `confirmGenerate`: a typed Persian prompt `می\u200cروم روی
// اسب` — held apart by a ZWNJ — rendered as `میروم روی اسب`, joined, while the
// graph on the wire carried the original. So the last screen before an
// irreversible spend stopped showing what would be sent, on the CLI's only
// money path. `printGenerateQuote`'s own comment already said that screen
// "DOES echo the real prompt and must keep doing so".
//
// Two guards, because neither alone is enough:
//
//  1. TestSafeTermIsNeverAppliedToUserTypedInput is STRUCTURAL — it reads this
//     package's own source and fails if any call site routes a user-typed
//     option field through safeTerm again. It cannot see whether the value is
//     actually printed.
//  2. TestSameBytesTypedAndReceived_AreEchoedAndStripped is BEHAVIOURAL — it
//     drives the real approval screen and the real server-text renderer with
//     the SAME bytes and asserts the two opposite outcomes. It cannot see a new
//     call site that is never exercised.

// userTypedArgs are the argument expressions that unambiguously hold what the
// user typed on the command line. Bare identifiers (`id`, `workflowID`) are
// deliberately NOT listed: the same name holds a server-returned id at other
// call sites, so a name-based rule there would report the wrong answer with
// confidence. Those sites are covered by review and by the behavioural test.
var userTypedArgs = map[string]string{
	"o.prompt":          "the typed prompt — the screen must show what will be SENT",
	"o.negativePrompt":  "the typed negative prompt, same reason",
	"o.aspectRatio":     "a typed flag value",
	"o.ecosystem":       "a typed flag value",
	"o.images[i]":       "the path the user typed, printed so they can match it to the blob",
	"built.inputPath":   "the path the user typed to --input",
	"o.inputPath":       "the same value before it is resolved",
	"o.checkpoint":      "a typed flag value",
	"o.negativePrompts": "reserved: any future typed field",
}

// 🔴 bareIdentArgs IS THE OTHER HALF OF THE GUARD, AND IT EXISTS BECAUSE THE
// FIRST HALF WAS BLIND TO A NAMING CONVENTION.
//
// userTypedArgs above pins argument SHAPES (`o.prompt`, `built.inputPath`). A
// delta audit found that `safeTerm(target)` in `download.go` carries the user's
// own `--out` path — `targetPath` returns `o.out` verbatim — and no assertion
// could see it, because the argument is spelled as a bare local. A guard that
// only understands one naming convention is the reason that survived.
//
// So every bare-identifier argument must be classified HERE. The ledger does
// not decide whether sanitising is right; it makes the decision impossible to
// skip. A new bare identifier fails this test until someone writes down where
// its bytes come from, which is the thinking that was missing for `target`.
//
// It fails in both directions: an unclassified name (the set grew) and a
// classified name that no longer appears (the set shrank, so the note is stale
// and the next reader would trust it).
var bareIdentArgs = map[string]string{
	"workflowID": "SERVER: the id from the submit reply / poll, not the one the user typed",
	"target":     "MIXED: --out verbatim, else filepath.Base(SERVER file name). Sanitised for the server half — see targetPath",
	"w":          "SERVER-derived: a download warning, or an image-metadata weight",
	"status":     "SERVER: a workflow status string",
	"r":          "SERVER: an orchestrator failure reason",
	"note":       "SERVER-derived: a routing note built from the server's file name",
	"k":          "SERVER cost-map key, and (generate_input.go) a key out of the user's own --input file",
	"workflow":   "the workflow value out of the user's --input FILE — the documented file-content exception",
	"t":          "SERVER: a trained word",
	"sha":        "SERVER: a published hash",
	"reason":     "SERVER: an orchestrator failure reason",
	"name":       "SERVER: a published file name",
	"h":          "SERVER: a hash out of image metadata",
	"baseModel":  "SERVER: a base-model label",
}

// minSafeTermCallsScanned is the POSITIVE CONTROL. A parser that has stopped
// finding calls — a moved package, a renamed helper, the wrong directory —
// scans nothing, finds no violation and reports a serene pass. There are 150
// calls today.
const minSafeTermCallsScanned = 100

func TestSafeTermIsNeverAppliedToUserTypedInput(t *testing.T) {
	// parser.ParseFile over an explicit file list rather than parser.ParseDir,
	// which Go 1.25 deprecated. The list is the package's own non-test sources;
	// a read that returns none of them trips the control below.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot read the package directory: %v", err)
	}
	fset := token.NewFileSet()
	scanned, files := 0, 0
	var bad, unclassified []string
	seenBare := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, perr := parser.ParseFile(fset, name, nil, 0)
		if perr != nil {
			t.Fatalf("CONTROL failure, not a finding: cannot parse %s: %v", name, perr)
		}
		files++
		ast.Inspect(file, func(n ast.Node) bool {
			ce, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			id, ok := ce.Fun.(*ast.Ident)
			if !ok || id.Name != "safeTerm" || len(ce.Args) != 1 {
				return true
			}
			scanned++
			arg := renderExpr(ce.Args[0])
			if why, forbidden := userTypedArgs[arg]; forbidden {
				bad = append(bad, fmt.Sprintf("%s: safeTerm(%s) — %s", fset.Position(ce.Lparen), arg, why))
			}
			if _, bare := ce.Args[0].(*ast.Ident); bare {
				if _, known := bareIdentArgs[arg]; !known {
					unclassified = append(unclassified, fmt.Sprintf("%s: safeTerm(%s)", fset.Position(ce.Lparen), arg))
				}
				seenBare[arg] = true
			}
			return true
		})
	}
	if files < 20 {
		t.Fatalf("CONTROL failure, not a finding: only %d source file(s) parsed in this package", files)
	}

	if scanned < minSafeTermCallsScanned {
		t.Fatalf("CONTROL failure, not a finding: only %d safeTerm call(s) found, want >= %d. The scan is "+
			"broken, and a clean result from it means nothing.", scanned, minSafeTermCallsScanned)
	}
	if len(bad) > 0 {
		t.Errorf("%d call site(s) sanitise input the USER typed:\n  %s\n\n"+
			"safeTerm is for SERVER-supplied text. Rewriting the user's own bytes makes the pre-spend approval "+
			"screen stop describing the job it is approving — civitai/cli#393. See internal/saferune's package doc.",
			len(bad), strings.Join(bad, "\n  "))
	}
	if len(unclassified) > 0 {
		t.Errorf("%d safeTerm call site(s) pass a bare local whose ORIGIN is not written down:\n  %s\n\n"+
			"Add it to bareIdentArgs saying where its bytes come from. A bare name hides the answer — "+
			"`safeTerm(target)` carries the user's own --out path and no guard could see it until this "+
			"ledger existed.", len(unclassified), strings.Join(unclassified, "\n  "))
	}
	for name := range bareIdentArgs {
		if !seenBare[name] {
			t.Errorf("bareIdentArgs classifies %q, which is no longer passed to safeTerm anywhere. A stale "+
				"note reads as coverage; delete it or fix the name.", name)
		}
	}
	t.Logf("scanned %d safeTerm call site(s) across %d file(s); %d forbidden shapes and %d bare-identifier "+
		"origins are pinned", scanned, files, len(userTypedArgs), len(bareIdentArgs))
}

// renderExpr prints the small set of expression shapes safeTerm arguments take.
// Anything else renders as a form that cannot collide with a pinned name, so an
// unrecognised shape is never silently treated as safe-by-default.
func renderExpr(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return renderExpr(v.X) + "." + v.Sel.Name
	case *ast.IndexExpr:
		return renderExpr(v.X) + "[" + renderExpr(v.Index) + "]"
	case *ast.StarExpr:
		return "*" + renderExpr(v.X)
	case *ast.CallExpr:
		var args []string
		for _, a := range v.Args {
			args = append(args, renderExpr(a))
		}
		return renderExpr(v.Fun) + "(" + strings.Join(args, ", ") + ")"
	default:
		return "<expr>"
	}
}

// 🔴 THE SAME BYTES, TWO ORIGINS, TWO OPPOSITE OUTCOMES — WHICH IS THE WHOLE
// RULE IN ONE ASSERTION. Typed by the user, the string is echoed exactly.
// Received from the server, the same string loses the class. A test that only
// checked one direction is satisfiable by a build that sanitises everything
// (the bug) or nothing (the other bug).
//
// The corpus spans the class rather than listing the three runes from the
// issue: a C0 escape, the zero-width and bidi Cf runes, the blank-but-graphic
// residue, the `Mn` rune the category cut missed, a non-emoji variation
// selector, and — as the control on the class itself — the emoji presentation
// selector and a must-be-drawn mark, which survive BOTH paths.
func TestSameBytesTypedAndReceived_AreEchoedAndStripped(t *testing.T) {
	for _, probe := range []struct {
		name    string
		rune_   string
		survive bool // true when the rune is not in the class at all
	}{
		// One rune per probe, so "stripped to the surrounding words" is the same
		// assertion for every row.
		{"C0 escape", "\x1b", false},
		{"zero width space", "\u200b", false},
		{"right-to-left override", "\u202e", false},
		{"braille pattern blank", "\u2800", false},
		{"combining grapheme joiner", "͏", false},
		{"zero width non-joiner", "\u200c", false},
		{"text presentation selector", "︎", false},
		{"emoji presentation selector", "️", true},
		{"arabic end of ayah", "\u06dd", true},
		{"ordinary CJK", "日", true},
	} {
		t.Run(probe.name, func(t *testing.T) {
			typed := "FIXTURE alpha" + probe.rune_ + "beta"

			// --- typed by the user: echoed byte-for-byte ---------------------
			withStdinTTY(t, true)
			c, _, errb := genCmd("n\n")
			o := generateOpts{prompt: typed}
			if err := confirmGenerate(c, o, &resolvedGraph{}, 100, 1000, true); err == nil {
				t.Fatal("CONTROL failure, not a finding: answering 'n' did not refuse, so this is not the " +
					"interactive approval screen")
			}
			screen := errb.String()
			if !strings.Contains(screen, "Generate? [y/N]") {
				t.Fatalf("CONTROL failure, not a finding: the approval screen did not render:\n%q", screen)
			}
			if !strings.Contains(screen, typed) {
				t.Errorf("the approval screen did not echo the typed prompt byte-for-byte.\n want: %q\n"+
					" got:  %q\n\nThis screen is the last thing shown before an irreversible spend, so it has "+
					"to show what will actually be sent (civitai/cli#393).", typed, screen)
			}

			// --- received from the server: the class is removed ---------------
			c2, out, _ := genCmd("")
			if err := runWorkflowsGet(c2, wfGetDeps(wfWithReason(typed), nil),
				workflowsGetOpts{baseURL: "https://civitai.com"}, "wf_123"); err != nil {
				t.Fatalf("workflows get: %v", err)
			}
			rendered := out.String()
			if !strings.Contains(rendered, "FIXTURE alpha") {
				t.Fatalf("CONTROL failure, not a finding: the reason never rendered:\n%q", rendered)
			}
			switch {
			case probe.survive:
				if !strings.Contains(rendered, typed) {
					t.Errorf("the server reason lost %s, which is NOT in the class — it must survive both "+
						"paths:\n %q", probe.name, rendered)
				}
			default:
				if strings.Contains(rendered, typed) {
					t.Errorf("the server reason kept %s. Typed by the user it is echoed; sent by the server it "+
						"must be removed — that asymmetry is the rule:\n %q", probe.name, rendered)
				}
				if !strings.Contains(rendered, "FIXTURE alphabeta") {
					t.Errorf("the server reason was not stripped to the surrounding words:\n %q", rendered)
				}
			}
		})
	}
}
