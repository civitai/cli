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
	"partPath":   "MIXED: `target` + \".part\", so it inherits target's two origins exactly — see targetPath",
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
	"typ":        "SERVER: the primary file's published `type`, defaulted to \"Other\" when blank (nonModelFileMarker)",
}

// sanitizerComposers are the functions IN safeterm.go that may call safeTerm on
// their own parameter. Such a call is one sanitizer DELEGATING to another; it is
// not a renderer handing safeTerm a value, which is the only thing #393 is about.
//
// 🔴 THIS IS A LEDGER, NOT A FILE EXEMPTION, AND THE DIFFERENCE IS THE WHOLE
// POINT. The obvious fix for the same problem is to allowlist the ARGUMENT NAME
// in bareIdentArgs — civitai/cli#554 proposed exactly that, with `"s"`. That is
// wrong in a way that is invisible: `s` is the most common local name in Go, so
// one entry blinds this harness in all ~67 files at once. Measured on that
// branch: `s := userTypedPrompt; … safeTerm(s)` injected into images.go SURVIVED,
// while the identical injection named `zzUnknownIdent` was KILLED.
//
// Keyed by enclosing function rather than by file so that adding a NEW function
// to safeterm.go does not silently inherit the pass — a new composer has to be
// named here, and TestSanitizerComposersAreLedgered fails if this set names a
// function that no longer exists.
var sanitizerComposers = map[string]string{
	"safeTermSingle": "collapses \\n to a space for single-line/tabwriter fields; delegates to safeTerm first",
	"safeTermErr":    "strips a WRAPPED CAUSE's message while leaving errors.Is/As reaching the original; delegates to safeTerm",
}

// sanitizerFile is the one file whose safeTerm calls are composition rather than
// rendering. Both conditions must hold — file AND ledgered function.
const sanitizerFile = "safeterm.go"

// minSafeTermCallsScanned is the POSITIVE CONTROL. A parser that has stopped
// finding calls — a moved package, a renamed helper, the wrong directory —
// scans nothing, finds no violation and reports a serene pass. There are 153
// calls today.
const minSafeTermCallsScanned = 100

// safeTermSite is one safeTerm(...) call, ATTRIBUTED TO THE FUNCTION THAT
// ENCLOSES IT.
type safeTermSite struct {
	// pos is file:line:col, for a failure message a reader can jump to.
	pos string
	// enclosing is the ledger key: the bare name for a top-level func, or
	// `(*T).name` for a method, so two methods of different types that share a
	// name stay distinguishable.
	enclosing string
	// arg is the rendered argument expression, and bareIdent says whether it was
	// spelled as a bare local.
	arg       string
	bareIdent bool
}

// scanSafeTermCallSites parses this package's own non-test sources and returns
// every one-argument safeTerm(...) call with its enclosing function.
//
// 🔴 IT WALKS file.Decls AND RECURSES INTO EACH *ast.FuncDecl, AND THAT ONE
// STRUCTURAL DIFFERENCE IS WHAT #399 TURNED ON. The original walk here was a
// flat `ast.Inspect(file, …)`: it saw every call and knew where NONE of them
// lived, so the only thing it could count was a total. Deleting a safeTerm call
// moved that total from ~151 to ~150 — nowhere near the floor below — and the
// suite stayed green, which is exactly the defect #399 reports (20 of 25 sampled
// sites survived deletion). Attribution is what lets a ledger be keyed by
// SOMETHING THAT SHRINKS TO ZERO when a function's last call is removed.
//
// Two controls run here rather than in either caller, so both get them:
//
//  1. the file and call counts, against the floors above;
//  2. 🔴 TOTALITY — a flat count of the same calls must equal the attributed
//     count. A call in a package-level var initialiser, or in a func literal
//     assigned outside any FuncDecl, is invisible to a Decls walk; without this
//     it would silently be attributed to nothing and be covered by no row,
//     which reads exactly like "no such site exists".
func scanSafeTermCallSites(t *testing.T) []safeTermSite {
	t.Helper()
	// parser.ParseFile over an explicit file list rather than parser.ParseDir,
	// which Go 1.25 deprecated. The list is the package's own non-test sources;
	// a read that returns none of them trips the control below.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot read the package directory: %v", err)
	}
	fset := token.NewFileSet()
	files, flat := 0, 0
	var sites []safeTermSite
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
			if isSafeTermCall(n) != nil {
				flat++
			}
			return true
		})
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			key := safeTermFuncKey(fd)
			ast.Inspect(fd, func(n ast.Node) bool {
				ce := isSafeTermCall(n)
				if ce == nil {
					return true
				}
				_, bare := ce.Args[0].(*ast.Ident)
				sites = append(sites, safeTermSite{
					pos:       fset.Position(ce.Lparen).String(),
					enclosing: key,
					arg:       renderExpr(ce.Args[0]),
					bareIdent: bare,
				})
				return true
			})
		}
	}
	if files < 20 {
		t.Fatalf("CONTROL failure, not a finding: only %d source file(s) parsed in this package", files)
	}
	if len(sites) < minSafeTermCallsScanned {
		t.Fatalf("CONTROL failure, not a finding: only %d safeTerm call(s) found, want >= %d. The scan is "+
			"broken, and a clean result from it means nothing.", len(sites), minSafeTermCallsScanned)
	}
	if flat != len(sites) {
		t.Fatalf("CONTROL failure, not a finding: %d safeTerm call(s) exist but only %d sit inside a func "+
			"declaration. The missing one is in a package-level initialiser or a func literal outside any "+
			"FuncDecl, so no enclosing function owns it and no ledger row can cover it. Widen this walk "+
			"before trusting either test that reads it.", flat, len(sites))
	}
	return sites
}

// isSafeTermCall returns the call when n is a one-argument safeTerm(...), and
// nil otherwise. ONE spelling of the match, used by both halves of the totality
// control above — two copies of it could disagree and the disagreement would
// read as a finding.
func isSafeTermCall(n ast.Node) *ast.CallExpr {
	ce, ok := n.(*ast.CallExpr)
	if !ok {
		return nil
	}
	id, ok := ce.Fun.(*ast.Ident)
	if !ok || !scannedSanitizers[id.Name] || len(ce.Args) != 1 {
		return nil
	}
	return ce
}

// scannedSanitizers are the sanitizer entry points this harness treats as
// equivalent for #393 purposes: each one hands its argument to safeTerm, so
// applying EITHER to user-typed input is the same defect.
//
// 🔴 MATCHING ONLY "safeTerm" SILENTLY DROPS COVERAGE THE MOMENT A WRAPPER IS
// INTRODUCED, AND THAT IS NOT HYPOTHETICAL. civitai/cli#554 converts 16 call
// sites in images.go from safeTerm to safeTermSingle. Merging that branch with a
// matcher keyed to the single name "safeTerm" takes the scanned total from 152 to
// 137 — those 16 sites leave this harness's view entirely — and the only thing
// that says so is bareIdentArgs' shrank-direction check tripping on a now-unused
// entry ("h"), which reads as a stale note rather than as lost coverage.
//
// ⚠ CORRECTION, measured after the above was written: "the only thing" is wrong.
// TestSafeTermCallSitesAreCoveredByANamedTest ALSO reddens, and with an accurate
// message ("CALL REMOVED: that is the #399 defect happening"). The narrowing is
// still worth preventing here — this is the guard that should name it — but the
// tree is not as blind to it as this paragraph claimed.
//
// So: a new wrapper around safeTerm belongs HERE, in the same commit that adds it.
var scannedSanitizers = map[string]bool{
	"safeTerm":       true,
	"safeTermSingle": true,
}

// safeTermFuncKey is the ledger key for a function declaration.
func safeTermFuncKey(fd *ast.FuncDecl) string {
	if fd.Recv != nil && len(fd.Recv.List) > 0 {
		return "(" + renderExpr(fd.Recv.List[0].Type) + ")." + fd.Name.Name
	}
	return fd.Name.Name
}

func TestSafeTermIsNeverAppliedToUserTypedInput(t *testing.T) {
	sites := scanSafeTermCallSites(t)
	var bad, unclassified []string
	seenBare := map[string]bool{}
	for _, s := range sites {
		if why, forbidden := userTypedArgs[s.arg]; forbidden {
			bad = append(bad, fmt.Sprintf("%s: safeTerm(%s) — %s", s.pos, s.arg, why))
		}
		// A sanitizer in safeterm.go delegating to safeTerm is composition, not
		// rendering — see sanitizerComposers. BOTH conditions must hold, and the
		// userTypedArgs check above deliberately still applies: composing is no
		// licence to sanitise the user's own bytes.
		if s.bareIdent && strings.HasPrefix(s.pos, sanitizerFile+":") {
			if _, composer := sanitizerComposers[s.enclosing]; composer {
				continue
			}
		}
		if s.bareIdent {
			if _, known := bareIdentArgs[s.arg]; !known {
				unclassified = append(unclassified, fmt.Sprintf("%s: safeTerm(%s)", s.pos, s.arg))
			}
			seenBare[s.arg] = true
		}
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
	t.Logf("scanned %d safeTerm call site(s); %d forbidden shapes and %d bare-identifier origins are pinned",
		len(sites), len(userTypedArgs), len(bareIdentArgs))
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

// TestSanitizerComposersAreLedgered pins sanitizerComposers in BOTH directions.
//
// 🔴 A ONE-SIDED CHECK HERE WOULD BE WORSE THAN NONE. If the set only had to be
// a SUPERSET, a composer could be deleted and the stale entry would keep reading
// as coverage; if only a SUBSET, a new function in safeterm.go could quietly
// inherit the pass that sanitizerComposers exists to withhold. So: every name
// here must exist in safeterm.go AND actually call safeTerm, and every safeTerm
// call in safeterm.go must be enclosed by a name that is here.
func TestSanitizerComposersAreLedgered(t *testing.T) {
	sites := scanSafeTermCallSites(t)

	inFile := map[string]bool{}
	for _, s := range sites {
		if strings.HasPrefix(s.pos, sanitizerFile+":") {
			inFile[s.enclosing] = true
		}
	}

	// GREW: a safeTerm call in safeterm.go whose enclosing function is unledgered.
	for fn := range inFile {
		if _, ok := sanitizerComposers[fn]; !ok {
			t.Errorf("UNLEDGERED COMPOSER: %s in %s calls safeTerm but is not in sanitizerComposers.\n"+
				"A new function in %s does NOT silently inherit the composition pass. Either add it with a "+
				"reason, or — if it RENDERS rather than composes — it belongs outside %s.",
				fn, sanitizerFile, sanitizerFile, sanitizerFile)
		}
	}

	// SHRANK: a ledgered composer that no longer calls safeTerm at all.
	for fn, why := range sanitizerComposers {
		if !inFile[fn] {
			t.Errorf("STALE COMPOSER: sanitizerComposers names %q (%s) but no safeTerm call in %s is enclosed by it.\n"+
				"It was renamed, deleted, or no longer delegates. Remove the entry — a stale one reads as coverage "+
				"while providing none.", fn, why, sanitizerFile)
		}
	}

	// POSITIVE CONTROL: a zero here is indistinguishable from a scanner that
	// stopped finding anything, so require the set to be non-empty and matched.
	if len(sanitizerComposers) == 0 {
		t.Fatal("CONTROL failure, not a finding: sanitizerComposers is empty, so both loops above are vacuous")
	}
	if len(inFile) == 0 {
		t.Fatalf("CONTROL failure, not a finding: no safeTerm call in %s was attributed to any function; "+
			"the scanner or the file moved", sanitizerFile)
	}
}
