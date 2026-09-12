package cmd

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// indentcontinuation_ledger_test.go pins the set of call sites that route
// multi-line server text through indentContinuation, and asserts that each one
// sanitises its text and indents with real whitespace (civitai/cli#545, #552).
//
// 🔴 IT IS NOT THE CLOSING CONDITION FOR #552, AND IT USED TO SAY IT WAS. That
// claim was written when this was the only ledger in the package, and #552's own
// body disowns it: its ask was a ledger over "call sites that call
// indentContinuation", which is satisfiable BY CONSTRUCTION and blind to every
// renderer that has never called the helper — the issue records that #554 should
// not be read as closing it even once merged. The closing condition is
// tabwriterRenderers in tabwriter_ledger_test.go, whose predicate is structural
// ("a value written into a tabwriter cell may not reach it through a bare
// safeTerm") rather than keyed on a helper's name, plus the behavioural table in
// tabwriter_forgery_test.go that drives the real renderers.
//
// 🔴 SO WHAT THIS FILE ACTUALLY PINS, stated at the scope it measures: for the
// eight places that DO call indentContinuation, the argument passed through
// safeTerm or wrapServerText, the pad is whitespace (RESOLVED, not
// name-allowlisted — see checkIndentPadArg), and the set neither grows nor
// shrinks unnoticed. It says nothing about whether the set is the RIGHT one.
//
// 🔴 WHAT THIS LEDGER CANNOT SEE, stated rather than waved at:
// 1. A call routed through a local function-typed variable or alias;
// 2. An un-ledgered renderer that prints multi-line server text using raw
//    fmt.Fprintf without ever invoking indentContinuation or safeTermSingle.
//    This is (2) restated as the general case, and it is precisely why this file
//    cannot close #552: printSubmissionDetail sat in exactly that state — no
//    call, therefore no row, therefore no signal — while printing a raw ESC.
// The behavioural subtest beside this guards the images.go surfaces against (2);
// TestGatedRenderersDoNotForgeOutsideTheirTable guards the app-path ones.
//
// 🔴 THE SET GROWS OR SHRINKS:
// Adding a call site without adding it to the ledger fails this test (GREW).
// Removing a call site without updating the ledger fails this test (SHRANK).

type indentCallSite struct {
	file string
	fn   string
	why  string
}

var pinnedIndentCallSites = []indentCallSite{
	{
		file: "app_status.go",
		fn:   "printSubmissionDetail",
		why: "`app status --id`'s rejection reason — reviewer free text, multi-line by design, printed " +
			"one line under the submission table this function flushes. It was UNGATED until #552's " +
			"follow-up: a reason of \"\\x1b[1A\\x1b[2KOVERWRITTEN\" put a raw ESC on stdout and overwrote " +
			"the row above it",
	},
	{
		file: "app_status.go",
		fn:   "printSubmissionDetail",
		why:  "`app status --id`'s approval notes — the same field class, the same gap, the same fix",
	},
	{
		file: "generate.go",
		fn:   "serverReasonSuffix",
		why:  "terminal orchestrator failure reason (multi-line unbounded free text)",
	},
	{
		file: "generate_output.go",
		fn:   "reportExcludedOutputs",
		why:  "per-output exclusion reason",
	},
	{
		file: "images.go",
		fn:   "printImageMetaBlock",
		why:  "image generation prompt (multi-line user free text)",
	},
	{
		file: "images.go",
		fn:   "printImageMetaBlock",
		why:  "image generation negative prompt (multi-line user free text)",
	},
	{
		file: "workflows.go",
		fn:   "printWorkflow",
		why:  "workflow run failure reason in detail view",
	},
	{
		file: "workflows_list.go",
		fn:   "printWorkflowList",
		why:  "workflow run failure reason beneath table row",
	},
}

// minIndentCallSitesExpected is the POSITIVE CONTROL — it answers "did the
// scanner find anything at all?", NOT "is the ledger complete".
//
// 🔴 IT MUST STAY STRICTLY BELOW len(pinnedIndentCallSites), AND IT WAS EQUAL TO
// IT. At 6-against-6 a single deleted call site — precisely the regression this
// ledger exists to catch — tripped this Fatalf instead of the SHRANK branch
// below, so the only message a reader saw was "CONTROL failure", which is this
// file's own idiom for "the harness is broken, not your code". That is how a
// floor gets lowered instead of a regression investigated. Measured: deleting
// the real call at workflows_list.go reported `CONTROL failure: found only 5`
// and never printed the SHRANK text.
//
// 3 is deliberately loose: a scanner that has stopped working returns 0 or 1,
// and anything at or above this is the set comparison's business, not ours.
const minIndentCallSitesExpected = 3

// TestIndentContinuationCallSitesAreLedgered combines the structural AST ledger
// over indentContinuation's call sites with a behavioural seam test over
// images.go, so `go test ./internal/cmd -run Ledger` covers both in one
// invocation.
//
// 🔴 IT IS NOT THE CLOSING CONDITION FOR #552 — that is tabwriterRenderers, and
// the file header says why this one cannot be. Read the claim there before
// citing this test as coverage of anything wider than its own eight call sites.
func TestIndentContinuationCallSitesAreLedgered(t *testing.T) {
	t.Run("StructuralLedger", func(t *testing.T) {
		entries, err := os.ReadDir(".")
		if err != nil {
			t.Fatalf("CONTROL failure: cannot read package directory: %v", err)
		}

		fset := token.NewFileSet()
		filesParsed := 0
		var actualCalls []indentCallSite

		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			f, perr := parser.ParseFile(fset, name, nil, 0)
			if perr != nil {
				t.Fatalf("CONTROL failure: cannot parse %s: %v", name, perr)
			}
			filesParsed++

			var currentFn string
			ast.Inspect(f, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.FuncDecl:
					currentFn = x.Name.Name
				case *ast.CallExpr:
					id, ok := x.Fun.(*ast.Ident)
					if !ok || id.Name != "indentContinuation" {
						return true
					}
					pos := fset.Position(x.Lparen)
					if len(x.Args) != 2 {
						t.Errorf("%s: indentContinuation must have exactly 2 arguments, got %d", pos, len(x.Args))
						return true
					}

					// 1. Assert arg[0] passes through safeTerm or wrapServerText
					if !containsSanitizerCall(x.Args[0]) {
						t.Errorf("%s: indentContinuation arg[0] must pass through safeTerm or wrapServerText", pos)
					}

					// 2. Assert arg[1] (pad) is a valid whitespace literal or known indent constant
					checkIndentPadArg(t, pos, x.Args[1])

					actualCalls = append(actualCalls, indentCallSite{
						file: name,
						fn:   currentFn,
					})
				}
				return true
			})
		}

		if filesParsed < 20 {
			t.Fatalf("CONTROL failure: parsed only %d files in internal/cmd", filesParsed)
		}

		if len(actualCalls) < minIndentCallSitesExpected {
			t.Fatalf("CONTROL failure: found only %d indentContinuation call(s), want >= %d",
				len(actualCalls), minIndentCallSitesExpected)
		}

		// Bidirectional set comparison
		gotKey := func(s indentCallSite) string { return s.file + ":" + s.fn }
		var gotKeys, wantKeys []string
		for _, c := range actualCalls {
			gotKeys = append(gotKeys, gotKey(c))
		}
		for _, c := range pinnedIndentCallSites {
			wantKeys = append(wantKeys, gotKey(c))
		}
		sort.Strings(gotKeys)
		sort.Strings(wantKeys)

		if strings.Join(gotKeys, ",") != strings.Join(wantKeys, ",") {
			t.Fatalf("indentContinuation call sites in internal/cmd do not match the ledger.\n"+
				"got (%d):\n  %s\nwant (%d):\n  %s\n\n"+
				"GREW: a new call site now uses indentContinuation — add it to pinnedIndentCallSites with why.\n"+
				"SHRANK: a call site was deleted or renamed — update the ledger deliberately.",
				len(gotKeys), strings.Join(gotKeys, "\n  "),
				len(wantKeys), strings.Join(wantKeys, "\n  "))
		}
	})

	t.Run("BehaviouralSeam", func(t *testing.T) {
		// Test against the 8 forgeable fields identified in civitai/cli#552:
		// 7 inline/metadata fields in printImageMetaBlock + 1 table row field in printImageList.
		probe := "probe\n[FORGED_ROW]"

		// 1. Verify printImageMetaBlock (all inline/metadata fields, including resources weight & hash)
		metaJSON := fmt.Sprintf(`{
			"prompt": %q,
			"negativePrompt": %q,
			"Model": %q,
			"sampler": %q,
			"cfgScale": %q,
			"steps": %q,
			"seed": %q,
			"resources": [
				{"type": %q, "name": %q, "weight": %q, "hash": %q}
			]
		}`, probe, probe, probe, probe, probe, probe, probe, probe, probe, probe, probe)

		im := civitai.ImageItem{
			ID:        123456,
			URL:       probe,
			NSFWLevel: probe,
			Width:     512,
			Height:    512,
			Meta:      []byte(metaJSON),
		}
		_ = im.Username.UnmarshalJSON([]byte(fmt.Sprintf("%q", probe)))

		var buf bytes.Buffer
		printImageMetaBlock(&buf, im)
		rendered := buf.String()

		// Prompt & NegativePrompt: multi-line must survive, but with continuation indented (never col 0)
		if !strings.Contains(rendered, "  prompt: probe\n          [FORGED_ROW]") {
			t.Errorf("prompt was not indented with 10 spaces: %q", rendered)
		}
		if !strings.Contains(rendered, "  negative: probe\n            [FORGED_ROW]") {
			t.Errorf("negative prompt was not indented with 12 spaces: %q", rendered)
		}

		// All other fields in printImageMetaBlock must have \n replaced with space
		inlineChecks := []struct {
			name     string
			needle   string
			antiNeed string
		}{
			{"model", "model: probe [FORGED_ROW]", "model: probe\n"},
			{"sampler", "sampler: probe [FORGED_ROW]", "sampler: probe\n"},
			{"cfgScale", "cfg: probe [FORGED_ROW]", "cfg: probe\n"},
			{"steps", "steps: probe [FORGED_ROW]", "steps: probe\n"},
			{"seed", "seed: probe [FORGED_ROW]", "seed: probe\n"},
			{"url", "url: probe [FORGED_ROW]", "url: probe\n"},
			{"header username", "by probe [FORGED_ROW]", "by probe\n"},
			{"header nsfw", "[probe [FORGED_ROW]]", "[probe\n"},
			{"resource type/name", "[probe [FORGED_ROW]] probe [FORGED_ROW]", "[probe\n"},
			{"resource weight", "weight probe [FORGED_ROW]", "weight probe\n"},
			{"resource hash", "hash probe [FORGED_ROW]", "hash probe\n"},
		}
		for _, c := range inlineChecks {
			if !strings.Contains(rendered, c.needle) {
				t.Errorf("%s was not sanitized to a single line; want %q", c.name, c.needle)
			}
			if strings.Contains(rendered, c.antiNeed) {
				t.Errorf("%s leaked a newline: %q", c.name, rendered)
			}
		}

		// Assert [FORGED_ROW] never occupies column zero anywhere in the block
		for _, line := range strings.Split(rendered, "\n") {
			if strings.HasPrefix(line, "[FORGED_ROW]") {
				t.Errorf("found forged text occupying column zero: %q", line)
			}
		}

		// 2. Verify plain printImageList table row (1 field: username, plus BaseModel, NSFWLevel, URL)
		var tableBuf bytes.Buffer
		fakeCmd := &cobra.Command{}
		fakeCmd.SetOut(&tableBuf)
		printImageList(fakeCmd, []civitai.ImageItem{im})
		tableRendered := tableBuf.String()

		tableLines := strings.Split(strings.TrimRight(tableRendered, "\n"), "\n")
		// Header + exactly 1 data row = 2 lines
		if len(tableLines) != 2 {
			t.Errorf("table rendered %d lines (want 2); tabwriter was split into fake rows:\n%s",
				len(tableLines), tableRendered)
		}
		for _, line := range tableLines {
			if strings.HasPrefix(strings.TrimSpace(line), "[FORGED_ROW]") {
				t.Errorf("table row forged by username newline: %q", line)
			}
		}
	})
}

// containsSanitizerCall traverses an AST expression to confirm that safeTerm
// or wrapServerText is present in the call chain.
func containsSanitizerCall(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		ce, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if id, ok := ce.Fun.(*ast.Ident); ok {
			if id.Name == "safeTerm" || id.Name == "wrapServerText" {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// checkIndentPadArg verifies that pad is whitespace: either a string literal
// containing only spaces/tabs, or a package-level string constant WHOSE VALUE IS
// RESOLVED and checked by the same rule. It does not trust an identifier's name.
func checkIndentPadArg(t *testing.T, pos token.Position, padExpr ast.Expr) {
	t.Helper()
	switch p := padExpr.(type) {
	case *ast.BasicLit:
		if p.Kind != token.STRING {
			t.Errorf("%s: indentContinuation pad must be a string literal, got token %v", pos, p.Kind)
			return
		}
		unquoted, err := strconv.Unquote(p.Value)
		if err != nil {
			t.Errorf("%s: invalid string literal %s: %v", pos, p.Value, err)
			return
		}
		if len(unquoted) == 0 {
			t.Errorf("%s: indentContinuation pad cannot be empty", pos)
			return
		}
		for _, r := range unquoted {
			if r != ' ' && r != '\t' {
				t.Errorf("%s: indentContinuation pad literal %q contains non-whitespace rune %q", pos, unquoted, r)
				return
			}
		}
	case *ast.Ident:
		// 🔴 RESOLVE THE CONSTANT'S VALUE — DO NOT ALLOWLIST ITS NAME.
		//
		// This branch used to check only that the identifier was one of a set of
		// approved spellings, while the docstring claimed it verified a
		// "whitespace-indent constant". That is a guard on a WORD, walkable by
		// rewriting what the word means. Measured: setting
		// `listReasonIndent = "XX>>"` left this test fully GREEN, both subtests,
		// while the pad it vets was no longer whitespace at all.
		//
		// So the name is now only how we FIND the declaration; the assertion is
		// on its value, using the same whitespace rule as the literal branch.
		val, ok := resolvePackageStringConst(p.Name)
		if !ok {
			t.Errorf("%s: indentContinuation pad uses identifier %q, which is not a package-level string constant "+
				"in internal/cmd; must be a string literal or such a constant", pos, p.Name)
			return
		}
		if len(val) == 0 {
			t.Errorf("%s: indentContinuation pad constant %s is empty", pos, p.Name)
			return
		}
		for _, r := range val {
			if r != ' ' && r != '\t' {
				t.Errorf("%s: indentContinuation pad constant %s = %q contains non-whitespace rune %q; "+
					"a pad that is not whitespace lets a continuation line impersonate real output",
					pos, p.Name, val, r)
				return
			}
		}
	default:
		t.Errorf("%s: indentContinuation pad must be a string literal or vetted constant, got %T", pos, padExpr)
	}
}

// resolvePackageStringConst finds a package-level `const name = "..."` in this
// package's own non-test sources and returns its unquoted value. It exists so
// checkIndentPadArg can assert on what a pad constant IS rather than on what it
// is called.
func resolvePackageStringConst(name string) (string, bool) {
	entries, err := os.ReadDir(".")
	if err != nil {
		return "", false
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		fn := e.Name()
		if e.IsDir() || !strings.HasSuffix(fn, ".go") || strings.HasSuffix(fn, "_test.go") {
			continue
		}
		file, perr := parser.ParseFile(fset, fn, nil, 0)
		if perr != nil {
			continue
		}
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, id := range vs.Names {
					if id.Name != name || i >= len(vs.Values) {
						continue
					}
					lit, ok := vs.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						return "", false
					}
					unq, uerr := strconv.Unquote(lit.Value)
					if uerr != nil {
						return "", false
					}
					return unq, true
				}
			}
		}
	}
	return "", false
}

// TestSafeTermSingleNeutralisesTheTabColumnVector is the regression guard for the
// forgery that a \n-only replacement left open.
//
// 🔴 WHY A TAB AND NOT JUST A NEWLINE: text/tabwriter uses the tab as its COLUMN
// DELIMITER, and saferune deliberately keeps \t. So before this fix, server text
// containing tabs did not break the table — it EXTENDED it, producing an aligned
// row the user cannot distinguish from real output. Measured on `images search`
// with a username of "alice\tSDXL\t9x9\tNone\t0\t0\thttps://evil.example/steal".
func TestSafeTermSingleNeutralisesTheTabColumnVector(t *testing.T) {
	const forged = "alice\tSDXL\t9x9\tNone\t0\t0\thttps://evil.example/steal"
	got := safeTermSingle(forged)

	if strings.ContainsRune(got, '\t') {
		t.Errorf("safeTermSingle left a TAB in %q.\n"+
			"A tab is tabwriter's column delimiter, so server text can inject columns and forge an "+
			"ALIGNED row. It must be replaced, not preserved.", got)
	}
	if strings.ContainsRune(got, '\n') {
		t.Errorf("safeTermSingle left a NEWLINE in %q", got)
	}
	// The text must survive, not be dropped — a guard that ate the value would
	// also pass the two checks above.
	for _, want := range []string{"alice", "SDXL", "evil.example"} {
		if !strings.Contains(got, want) {
			t.Errorf("safeTermSingle dropped %q from the value; got %q", want, got)
		}
	}
	if want := "alice SDXL 9x9 None 0 0 https://evil.example/steal"; got != want {
		t.Errorf("safeTermSingle(%q)\n got: %q\nwant: %q", forged, got, want)
	}
}

// (TestTabForgeryIsNeutralisedInTheRealImagesTable lived here. It drove
// printImageList with the #569 username payload and asserted a 2-line,
// square-column render in which the payload survived as inert text. It was
// deleted as STRICTLY SUBSUMED: TestTabwriterRenderersCannotBeForged's
// `images search` case asserts everything it did against a payload carrying a
// tab AND a newline, plus the per-field naming this one could not do — and it
// does so for eighteen other renderers besides. The historically measured
// payload was not lost with it: it is the `images search`, the #569 payload case
// in that table, kept as a second fixture precisely because it is the shape an
// attacker sent rather than one a test author invented.
//
// TestSafeTermSingleNeutralisesTheTabColumnVector above is NOT subsumed — it
// pins the helper directly, which is the claim that survives a renderer being
// rewritten.)
