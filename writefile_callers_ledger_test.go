package cli_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// writefile_callers_ledger_test.go pins WHO writes a file through the atomic
// write helpers, because the symlink-following property belongs to the HELPER
// and was being re-proved one caller at a time.
//
// 🔴 THE PROPERTY IS resolveWriteTarget's, AND IT WAS GUARDED PER CALLER.
// `writeFileAtomic` resolves the destination before renaming, so a write through
// `~/.codex/config.toml -> ~/dotfiles/codex.toml` lands on the dotfiles copy
// instead of replacing the link with a regular file. That defect was MEASURED
// (exit 0, link destroyed, dotfiles copy orphaned and silently stale), and the
// fix went into the helper — one place, covering every caller.
//
// What then happened is the thing this file replaces. `--fix-path` became a new
// caller and shipped a SECOND per-caller copy of the same proof: same fixture
// shape, same `Lstat`-mode assertion, nearly the same failure string as
// TestASymlinkedConfigIsFollowed. Two copies is not twice the coverage — the
// helper's behaviour is identical for both — and meanwhile `app_init.go`'s
// caller had no symlink coverage at all and nothing said so.
//
// 🔴 SO THE SHAPE OF THE GUARD IS WRONG WHEN IT IS PER CALLER: a copy per caller
// grows with the caller set and still cannot tell you a caller is MISSING. The
// set is the thing to assert. This ledger fails when the caller set GROWS (a new
// write path inherits a property nobody decided it should have, and nobody chose
// which coverage it needs) and when it SHRINKS (a caller stopped going through
// the helper, which is how the resolve step gets re-derived locally and wrong).
//
// 🔴 IT IS STRUCTURAL, SO IT IS NOT THE WHOLE GUARD. A ledger type-checks past a
// wrong argument: it sees that `writeProjectFile` is called, never that the path
// it was handed is the one the caller meant. The BEHAVIOURAL half is
// TestWriteProjectFileFollowsASymlinkRatherThanReplacingIt, on the helper
// itself, plus the pre-existing end-to-end TestASymlinkedConfigIsFollowed.
// Neither is optional, and this comment exists so a later reader does not read
// the ledger as covering behaviour.

// writeHelper is one of the two atomic write entry points, named so a rename is
// red HERE rather than silently emptying the scan.
type writeHelper struct {
	// name is the function identifier call sites spell.
	name string
	// declIn is the repo-relative file its declaration must be found in.
	declIn string
	// what the helper is for, for a reader of this ledger.
	why string
}

var writeHelpers = []writeHelper{
	{
		name:   "writeFileAtomic",
		declIn: "internal/cmd/agent_setup_mcp.go",
		why:    "temp file + rename, THROUGH resolveWriteTarget — the one place the symlink is followed",
	},
	{
		name:   "writeProjectFile",
		declIn: "internal/cmd/agent_setup_files.go",
		why:    "0644 instruction-file wrapper over writeFileAtomic (AGENTS.md, CLAUDE.md, shell startup files)",
	},
}

// writeCallSite is one non-test call of one helper, attributed to the function
// that makes it.
type writeCallSite struct {
	// file is the repo-relative file holding the call.
	file string
	// fn is the top-level function the call is made from (a call inside a
	// closure attributes to the enclosing declaration).
	fn string
	// callee is the helper being called.
	callee string
	// calls is how many times fn calls callee. Keyed separately from the
	// identity so ADDING a call inside an ALREADY-LEDGERED function is red too
	// — otherwise the one place a new write is most likely to be added is the
	// one place this ledger could not see it.
	calls int
	// symlinkGuard names what pins the symlink-following property for this
	// site, or states plainly that nothing does. 🔴 AN HONEST "none" IS THE
	// POINT: app_init's caller had no coverage and no record of having none,
	// which is indistinguishable from nobody having looked.
	symlinkGuard string
}

// writeCallSites is the ledger. Keep it sorted by file, then function.
var writeCallSites = []writeCallSite{
	{
		file:         "internal/cmd/agent_setup.go",
		fn:           "runAgentSetupWrite",
		callee:       "writeProjectFile",
		calls:        3,
		symlinkGuard: "AGENTS.md + CLAUDE.md: TestABrokenSymlinkIsRefusedByName covers the REFUSAL only, not the follow; the shell startup file (--fix-path) rides the helper-level behavioural test",
	},
	{
		file:         "internal/cmd/agent_setup_files.go",
		fn:           "writeProjectFile",
		callee:       "writeFileAtomic",
		calls:        1,
		symlinkGuard: "TestWriteProjectFileFollowsASymlinkRatherThanReplacingIt — the behavioural half, directly on this seam",
	},
	{
		file:         "internal/cmd/agent_setup_mcp.go",
		fn:           "writeMCPConfig",
		callee:       "writeFileAtomic",
		calls:        1,
		symlinkGuard: "TestASymlinkedConfigIsFollowed (end-to-end, ~/.codex/config.toml — the measured defect)",
	},
	{
		file:         "internal/cmd/app_init.go",
		fn:           "writeScaffoldAgentsMD",
		callee:       "writeProjectFile",
		calls:        1,
		symlinkGuard: "NONE. A scaffolded AGENTS.md in a fresh project directory is not a dotfile anyone symlinks, so this is a deliberate gap and not an oversight — but it is RECORDED, so a reader does not infer coverage from the ledger's existence.",
	},
}

// TestWriteHelperCallersAreLedgered is the set assertion. See the file header.
func TestWriteHelperCallersAreLedgered(t *testing.T) {
	files := moduleGoFiles(t, false)
	// POSITIVE CONTROL on the walk: a walk that reads nothing reports "no
	// unledgered callers", which is the reassuring zero this guard would
	// otherwise be indistinguishable from.
	if len(files) < 100 {
		t.Fatalf("CONTROL failure, not a finding: walked only %d non-test .go file(s) "+
			"in the module — the scan is not reading the tree", len(files))
	}

	fset := token.NewFileSet()
	wanted := map[string]bool{}
	for _, h := range writeHelpers {
		wanted[h.name] = true
	}

	// declFound proves each helper NAME still resolves to a declaration in the
	// file this ledger says holds it. Without it, `name` is a bare string needle:
	// rename the helper and the scan finds zero call sites, reports a clean
	// shrink-free set, and the whole ledger passes while guarding nothing.
	declFound := map[string]string{}

	type key struct{ file, fn, callee string }
	got := map[key]int{}

	for _, path := range files {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("CONTROL failure, not a finding: cannot parse %s: %v", path, err)
		}
		rel := filepath.ToSlash(path)
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			if wanted[fd.Name.Name] && fd.Recv == nil {
				declFound[fd.Name.Name] = rel
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				id, ok := call.Fun.(*ast.Ident)
				if !ok || !wanted[id.Name] {
					return true
				}
				// A helper's own recursive call would be noise; none exists
				// today, and attributing it to itself would be wrong anyway.
				if id.Name == fd.Name.Name {
					return true
				}
				got[key{rel, fd.Name.Name, id.Name}]++
				return true
			})
		}
	}

	// POSITIVE CONTROL on the MATCH, before any verdict: an identifier typo or a
	// parser reading no bodies yields an empty set, which a "grew?" comparison
	// would pass in silence.
	if len(got) == 0 {
		t.Fatalf("CONTROL failure, not a finding: no non-test file in the module calls any of %v. "+
			"These helpers have callers; the scan is broken.", helperNames())
	}

	for _, h := range writeHelpers {
		where, ok := declFound[h.name]
		if !ok {
			t.Fatalf("CONTROL failure, not a finding: no top-level func %s is declared anywhere in the module.\n"+
				"It was renamed or removed. Every call site below is found by that NAME, so this scan now "+
				"measures nothing — fix the ledger deliberately rather than deleting it.", h.name)
		}
		if where != h.declIn {
			t.Errorf("%s is declared in %s, ledgered as %s.\n"+
				"The helper moved. Update declIn — and check the move did not split the write path in two, "+
				"which is what %q exists to keep in one place.", h.name, where, h.declIn, h.why)
		}
	}

	render := func(k key, n int) string {
		return fmt.Sprintf("%s: %s -> %s x%d", k.file, k.fn, k.callee, n)
	}
	var gotLines []string
	for k, n := range got {
		gotLines = append(gotLines, render(k, n))
	}
	var wantLines []string
	for _, c := range writeCallSites {
		wantLines = append(wantLines, render(key{c.file, c.fn, c.callee}, c.calls))
	}
	sort.Strings(gotLines)
	sort.Strings(wantLines)

	if strings.Join(gotLines, "\n") != strings.Join(wantLines, "\n") {
		t.Fatalf("the atomic-write caller set does not match the ledger.\n\nFOUND IN THE TREE:\n  %s\n\nLEDGERED:\n  %s\n\n"+
			"GREW: a new write path now inherits resolveWriteTarget's symlink-following\n"+
			"      behaviour. That is a decision, not a bump — add an entry AND set its\n"+
			"      symlinkGuard, naming the test that covers it or stating plainly that\n"+
			"      nothing does. An unrecorded caller is one nobody chose coverage for.\n"+
			"SHRANK: a caller stopped going through the helper. Check it did not re-derive\n"+
			"      the resolve step locally — that is the defect measured on\n"+
			"      ~/.codex/config.toml, where a rename onto a symlink destroyed the link\n"+
			"      at exit 0 and orphaned the dotfiles copy.\n"+
			"COUNT CHANGED: a ledgered function gained or lost a call. The count is part of\n"+
			"      the identity on purpose: an already-listed function is the likeliest\n"+
			"      place for a new write, and a membership-only check cannot see it.",
			strings.Join(gotLines, "\n  "), strings.Join(wantLines, "\n  "))
	}

	// Every entry must say something about the symlink property — including
	// "NONE". A blank field is the shape that reads as coverage while providing
	// none, which is the whole defect this ledger replaces.
	for _, c := range writeCallSites {
		if strings.TrimSpace(c.symlinkGuard) == "" {
			t.Errorf("%s: %s -> %s has an empty symlinkGuard.\n"+
				"State the test that covers this site, or write NONE and why. A blank field "+
				"is indistinguishable from nobody having looked.", c.file, c.fn, c.callee)
		}
	}

	t.Logf("atomic-write ledger: %d call site(s) across %d helper(s) agree", len(got), len(writeHelpers))
}

func helperNames() []string {
	var out []string
	for _, h := range writeHelpers {
		out = append(out, h.name)
	}
	return out
}
