package cmd

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
)

// TestSafeTermErrCallersAreLedgered pins WHERE safeTermErr may be called.
//
// 🔴 WHY A LEDGER AND NOT A DOC COMMENT. safeTermErr collapses \n and \t, not
// just the invisible class. That is only safe because every caller is on
// a SINGLE-LINE error path — and for a while the function's own doc
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
// attributedIn reports how many call sites the walk attributed to a function in
// the given file. Split out so the totality check reads as one comparison.
func attributedIn(found map[string][]string, file string) int { return len(found[file]) }

func TestSafeTermErrCallersAreLedgered(t *testing.T) {
	// file -> enclosing function, for every permitted safeTermErr call site.
	//
	// Every entry is on a SINGLE-LINE error path. That used to read "on
	// download.go's single-line error path", and civitai/cli#574 widened it:
	// generate's blob download path is the same shape one command over — the
	// same writePart, the same presigned-blob transfer, the same one-line
	// errors — and gating its `%s` operands while leaving the `%w` causes raw
	// would have reproduced, on the money-spending path, exactly the half-fix
	// #577 shipped and #590 had to repair.
	ledger := map[string][]string{
		"download.go": {
			"downloadOne",
			"downloadOne",
			"writePart",
			"writePart",
			"writePart",
		},
		"generate_output.go": {
			"downloadBlobTo", // download %s: %w
			"downloadBlobTo", // install %s: %w
		},
		// civitai/cli#612 widened it again, and the single-line decision for
		// these THREE was made deliberately rather than inherited:
		//
		//   - buildGenerateGraph's two sites wrap pkg/civitai's readError output
		//     ("not found (404): <snippet>"). snippet KEEPS \n, and the result is
		//     interpolated into `--checkpoint %d: %w` / `--lora %s: %w`, which
		//     main.go prints as one `Error: …` line. There is no indentation
		//     baseline and no block structure to preserve, so a newline there is
		//     forgery and never layout.
		//   - classifyGenerateError's FALL-THROUGH is the single chokepoint every
		//     generate/workflows error with a *genapi.APIError passes through, and
		//     the messages on the other side are genapi's one-line
		//     `<what> (<status>): <server message>` sentences. The orchestrator's
		//     multi-line FAILURE REASON — the one case indentContinuation exists
		//     for — does NOT come through here: it is rendered by
		//     serverReasonSuffix and printWorkflow, which stay on safeTerm +
		//     indentContinuation.
		//
		// 🔴 ONE entry for classifyGenerateError, not two, and the missing one is
		// deliberate: its !errors.As early return is MEASURED to carry raw ANSI
		// (genapi interpolates the unparsed HTTP body into `unexpected %s
		// response: %s`) and is still left ungated, because the same raw body is
		// interpolated at SEVEN genapi sites of which only some come back through
		// this function — so the fix belongs there, not here — and because that
		// return is a pass-through pinned by identity. Read the comment at that
		// return before "completing" this row.
		"generate.go": {
			"buildGenerateGraph",    // --checkpoint %d: %w
			"buildGenerateGraph",    // --lora %s: %w
			"classifyGenerateError", // the fall-through
		},
	}

	// os.ReadDir + ParseFile, matching indentcontinuation_ledger_test.go and
	// floor_predicate_ledger_test.go. NOT parser.ParseDir: it is deprecated as of
	// Go 1.25 and staticcheck's SA1019 fails the `lint` job on it — which `make
	// ci` does not run, so this cost a red CI check to discover.
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read internal/cmd: %v", err)
	}

	found := map[string][]string{}
	total, scanned := 0, 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if perr != nil {
			t.Fatalf("parse %s: %v", name, perr)
		}
		scanned++

		// 🔴 WALK Decls AND ATTRIBUTE TO THE DECL, not a running "most recent
		// FuncDecl" set by a flat ast.Inspect. The flat form computes "the last
		// FuncDecl seen in this file", which is the enclosing function only by
		// coincidence: a call in a package-level `var x = safeTermErr(...)` is
		// attributed to whichever function happens to sit above it, so the
		// multiset can stay identical while a call leaves the single-line path
		// entirely. safeterm_userinput_test.go's scanner already documents this
		// class under "🔴 TOTALITY"; this is the same shape.
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			key := fd.Name.Name
			// Keep the receiver: a method `writePart` on some type is not the
			// function `writePart`, and dropping it makes them indistinguishable.
			if fd.Recv != nil && len(fd.Recv.List) > 0 {
				var buf strings.Builder
				if err := printer.Fprint(&buf, fset, fd.Recv.List[0].Type); err == nil {
					key = "(" + buf.String() + ")." + fd.Name.Name
				}
			}
			ast.Inspect(fd, func(n ast.Node) bool {
				if ce, ok := n.(*ast.CallExpr); ok {
					if id, ok := ce.Fun.(*ast.Ident); ok && id.Name == "safeTermErr" {
						found[name] = append(found[name], key)
						total++
					}
				}
				return true
			})
		}

		// 🔴 TOTALITY. Count the SAME calls flatly over the whole file and require
		// the two numbers to agree. Without this, a call that no FuncDecl encloses
		// — a package-level var initialiser, a func literal assigned outside any
		// declaration — is attributed to NOTHING, covered by no row, and reads
		// exactly like "no such site exists". That is the failure this whole
		// ledger exists to prevent, one level up.
		flat := 0
		ast.Inspect(f, func(n ast.Node) bool {
			if ce, ok := n.(*ast.CallExpr); ok {
				if id, ok := ce.Fun.(*ast.Ident); ok && id.Name == "safeTermErr" {
					flat++
				}
			}
			return true
		})
		if flat != attributedIn(found, name) {
			t.Errorf("%s holds %d safeTermErr call(s) but only %d are inside a function declaration. "+
				"The rest sit in a package-level initialiser or a func literal outside any decl, so no ledger "+
				"row can cover them and their absence reads exactly like \"no such site exists\". Move them "+
				"into a function, or widen this scanner deliberately.", name, flat, attributedIn(found, name))
		}
	}

	if scanned == 0 {
		t.Fatal("CONTROL failure, not a finding: no non-test .go files were parsed in internal/cmd")
	}

	// POSITIVE CONTROL. A scan that matched nothing would satisfy every
	// "unledgered set is empty" check below and report a cheerful pass — the
	// reassuring zero that is indistinguishable from a harness wired to nothing.
	if total == 0 {
		t.Fatal("the AST scan found ZERO safeTermErr calls in internal/cmd, so every assertion below is " +
			"vacuous. TWO different causes, and they need opposite responses: either this scanner broke " +
			"(most likely — check the identifier it matches), or the last safeTermErr call was legitimately " +
			"removed, in which case delete this test AND the scope paragraph in safeterm.go that points at " +
			"it. Do not assume the first: an earlier version of this message asserted the function 'is called " +
			"at least five times', which would be a false statement about the tree in the second case.")
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
				"doc comment, which scopes itself to a single-line error path — widen the LEDGER first, "+
				"then the sentence, in that order. If any of them can carry a "+
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
