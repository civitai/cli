package cli_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// neterr_ledger_test.go is the structural guard for issue #246's point 4, and
// it replaces a rule that could not enforce itself.
//
// THE RULE IT REPLACES. #246's suggested resolution ended "leave no bare
// `errors.As(err, &netErr)` in the tree without an adjacent comment explaining
// why that spelling is correct there". #248 closed the two sites the issue was
// about; this closes the point the issue could not close by writing it down.
// A convention decays the moment somebody adds a site without reading it —
// which is exactly how #244 survived #242, and how #246's pair survived #244:
// the spelling looked fine at every site, and nothing counted the sites. Four
// copies were found one at a time, over four issues.
//
// WHAT THIS DOES INSTEAD. It parses every non-test .go file in the module,
// finds every `errors.As(x, &v)` whose target `v` is typed as the `net.Error`
// INTERFACE, and asserts that set equals a ledger written out below with a
// one-line justification per entry. It fails when the set GROWS (a new site
// nobody justified) and when it SHRINKS (a ledgered site silently removed, so
// the ledger stops describing the tree). A guard that only catches growth is
// half a guard.
//
// 🔴 WHAT IT DOES NOT PROVE — read this before trusting a green run.
//
//   - 🔴 It does not prove any ledgered site is CORRECT — it proves the SET has
//     not moved. In particular it CANNOT see whether a site's
//     `civitai.IsTransportError` gate is still there, or still in the right
//     PLACE. #248 measured that placement is load-bearing at `isTimeoutErr`:
//     gating only the `errors.As` and leaving `os.IsTimeout` above it is a
//     half-fix, because the two lines are filesystem-broad over different
//     shapes. Deleting a gate outright, or sliding it one line down, leaves
//     THIS file byte-for-byte green. The behavioural guards are what catch
//     that: internal/appapi/submit_fs_not_timeout_test.go and
//     internal/cmd/app_dev_tunnel_probe_class_test.go. Do not read a green here
//     as "the sites are fine"; read it as "no site appeared or vanished".
//   - It does not see a site whose target's type it cannot resolve
//     SYNTACTICALLY. The module has no `golang.org/x/tools/go/packages`
//     dependency and adding one is an "ask first" in AGENTS.md, so there is no
//     full type resolution here. Specifically blind to: `nerr := someFunc()`
//     returning a net.Error (no type is spelled), a local type alias
//     (`type ne = net.Error`), a dot-imported `net`, and a target reached
//     through an intermediate variable (`p := &nerr; errors.As(err, p)`).
//     Every one of those is a shape nobody in this tree writes, and each would
//     be a deliberate evasion rather than an accident — which is the shape of
//     mistake this guard is not trying to catch.
//   - It is deliberately blind to COMMENTS, and that is load-bearing rather
//     than incidental: `pkg/civitai/transport_error.go`, `pkg/civitai/retry.go`
//     and `cmd/civitai/main.go` all QUOTE `errors.As(err, &netErr)` in prose to
//     explain why they do not use it. A text matcher counts those as sites; an
//     AST walk cannot see them at all, because a comment never becomes a
//     CallExpr.
//   - It is deliberately blind to _test.go FILES. `cmd/civitai/fs_not_network_test.go`
//     IMPLEMENTS the naive predicate on purpose, as the control that pins the
//     stdlib trap itself ("assert the naive spelling STILL matches this
//     fixture, or the row proves nothing"). Ledgering test files would either
//     force that control into the ledger or force it to be written obscurely.
//     The cost is real and stated: a new bare site in a _test.go file is
//     invisible here.
//
// 🔴 IT KEYS ON THE TYPE, NOT ON THE NAME. The two ledgered sites spell the
// variable `netErr` and `nerr`; a matcher keyed on either identifier would be a
// spelled guard that a third site named `ne` walks straight past. The scanner's
// own claim about which spellings it can SEE is validated separately by
// TestNetErrorAsScannerSeesEverySpelling — including the `gofmt -s` question
// that disarmed the analogous guard in #238, since `make fmt` rewrites the tree
// and CI enforces `gofmt -s -l .`.

// netErrorAsSite is one `errors.As(_, &<net.Error>)` call in the module source.
type netErrorAsSite struct {
	// File is slash-separated and relative to the module root.
	File string
	// Func is the enclosing top-level function or method, closures normalised to
	// their parent. It is "<file-level>" for a call outside any function body.
	Func string
}

func (s netErrorAsSite) String() string { return s.File + ":" + s.Func }

// netErrorAsLedger is the ASSERTED SET. Every bare `errors.As(_, &<net.Error>)`
// in a non-test file must appear here, with a count, and every entry here must
// still exist. Adding an entry is how you justify a new site; the justification
// belongs on the entry, not in a comment three screens away, because a comment
// somewhere else is what #246 found does not survive.
var netErrorAsLedger = []struct {
	netErrorAsSite
	// Count is how many such calls the function holds. Pinned so a SECOND site
	// smuggled into an already-ledgered function is still caught.
	Count int
	// Why is the one-line justification. Keep it one line; the full argument
	// belongs in the function's doc comment, which is where a reader who is
	// about to change the code will actually be.
	Why string
}{
	{
		netErrorAsSite: netErrorAsSite{File: "internal/appapi/appblocks.go", Func: "isTimeoutErr"},
		Count:          1,
		Why: "GATED, NOT BARE (#246/#248): unreachable unless civitai.IsTransportError(err) already returned " +
			"true, and that gate sits ABOVE os.IsTimeout because the two lines are filesystem-broad over " +
			"different shapes. Guard: internal/appapi/submit_fs_not_timeout_test.go.",
	},
	{
		netErrorAsSite: netErrorAsSite{File: "internal/cmd/app_dev_tunnel.go", Func: "classifyProbeErr"},
		Count:          1,
		Why: "GATED, NOT BARE (#246/#248): unreachable unless civitai.IsTransportError(err) already returned " +
			"true; it only reads Timeout() off an error the walk accepted, which IsTransportError (a presence " +
			"answer) cannot give. Guard: internal/cmd/app_dev_tunnel_probe_class_test.go.",
	},
}

func TestNetErrorAsSitesMatchTheLedger(t *testing.T) {
	files := moduleGoFiles(t)
	// Positive control on the WALKER: this module has ~110 non-test .go files.
	// A short list means the walk is looking at the wrong tree and every
	// assertion below would pass vacuously — the "harness wired to nothing"
	// failure whose reassuring answer is a zero.
	if len(files) < 60 {
		t.Fatalf("walker found only %d non-test .go files — it is scanning the wrong tree, so a "+
			"clean result says nothing", len(files))
	}

	found := map[netErrorAsSite]int{}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		rel := filepath.ToSlash(f)
		for _, s := range scanNetErrorAs(t, rel, src) {
			found[s]++
		}
	}

	want := map[netErrorAsSite]int{}
	why := map[netErrorAsSite]string{}
	for _, e := range netErrorAsLedger {
		if e.Count < 1 {
			t.Fatalf("ledger entry %s has Count %d — an entry is a site that EXISTS", e, e.Count)
		}
		if strings.TrimSpace(e.Why) == "" {
			t.Fatalf("ledger entry %s carries no justification — that is the whole point of the entry", e)
		}
		want[e.netErrorAsSite] = e.Count
		why[e.netErrorAsSite] = e.Why
	}

	// GROWTH: a site the ledger does not name.
	var grown []string
	for s, n := range found {
		if w, ok := want[s]; !ok {
			grown = append(grown, fmt.Sprintf("%s (%d call(s))", s, n))
		} else if n != w {
			grown = append(grown, fmt.Sprintf("%s has %d call(s), ledger says %d", s, n, w))
		}
	}
	sort.Strings(grown)
	if len(grown) > 0 {
		t.Errorf("UNLEDGERED `errors.As(_, &<net.Error>)` site(s):\n  %s\n\n"+
			"`syscall.Errno` satisfies net.Error (it declares Timeout() and Temporary()) while "+
			"*fs.PathError / *os.LinkError / *os.SyscallError do not, so errors.As unwraps PAST the "+
			"filesystem wrapper and matches the bare errno — and Errno.Timeout() is TRUE for "+
			"ETIMEDOUT / EAGAIN / EWOULDBLOCK. See AGENTS.md item 24.\n"+
			"Either call `civitai.IsTransportError` instead, or add an entry to netErrorAsLedger in "+
			"this file with a one-line justification AND a test that pins the behaviour you are "+
			"claiming is correct.", strings.Join(grown, "\n  "))
	}

	// SHRINK: a ledgered site that no longer exists. The ledger is documentation
	// that other files' doc comments point at; a stale entry is a false map.
	var gone []string
	for s := range want {
		if _, ok := found[s]; !ok {
			gone = append(gone, fmt.Sprintf("%s — justified as: %s", s, why[s]))
		}
	}
	sort.Strings(gone)
	if len(gone) > 0 {
		t.Errorf("LEDGERED site(s) no longer present:\n  %s\n\n"+
			"If the site was legitimately removed or renamed, delete/update its netErrorAsLedger "+
			"entry in the same commit — and check whether the doc comment that referenced it "+
			"(and AGENTS.md item 24) still describes the tree.", strings.Join(gone, "\n  "))
	}
}

// TestNetErrorAsScannerSeesEverySpelling validates the SCANNER, not the tree.
// TestNetErrorAsSitesMatchTheLedger's green is a claim about the spellings this
// scanner can see, and in #238 the analogous claim was false for exactly one
// spelling — the one `gofmt -s` produces, which `make fmt` writes and CI
// enforces. So each row here is a negative or positive control fed to the same
// function the ledger test uses.
func TestNetErrorAsScannerSeesEverySpelling(t *testing.T) {
	const preamble = "package p\n\nimport (\n\t\"errors\"\n\t\"net\"\n)\n\nvar err error\n\n"

	cases := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "the canonical spelling",
			src:  preamble + "func f() bool {\n\tvar netErr net.Error\n\treturn errors.As(err, &netErr)\n}\n",
			want: 1,
		},
		{
			name: "SAME SHAPE, DIFFERENT NAME — the spelled-guard control",
			src:  preamble + "func f() bool {\n\tvar wibble net.Error\n\treturn errors.As(err, &wibble)\n}\n",
			want: 1,
		},
		{
			name: "single-line var decl inside an if",
			src:  preamble + "func f() bool {\n\tvar ne net.Error\n\tif errors.As(err, &ne) && ne.Timeout() {\n\t\treturn true\n\t}\n\treturn false\n}\n",
			want: 1,
		},
		{
			name: "grouped var decl",
			src:  preamble + "func f() bool {\n\tvar (\n\t\tx int\n\t\tne net.Error\n\t)\n\t_ = x\n\treturn errors.As(err, &ne)\n}\n",
			want: 1,
		},
		{
			name: "aliased net import",
			src:  "package p\n\nimport (\n\t\"errors\"\n\tgonet \"net\"\n)\n\nvar err error\n\nfunc f() bool {\n\tvar ne gonet.Error\n\treturn errors.As(err, &ne)\n}\n",
			want: 1,
		},
		{
			name: "aliased errors import",
			src:  "package p\n\nimport (\n\tgoerrors \"errors\"\n\t\"net\"\n)\n\nvar err error\n\nfunc f() bool {\n\tvar ne net.Error\n\treturn goerrors.As(err, &ne)\n}\n",
			want: 1,
		},
		{
			name: "target is a function parameter",
			src:  preamble + "func f(target net.Error) bool {\n\treturn errors.As(err, &target)\n}\n",
			want: 1,
		},
		{
			name: "target is new(net.Error) — no named variable at all",
			src:  preamble + "func f() bool {\n\treturn errors.As(err, new(net.Error))\n}\n",
			want: 1,
		},
		{
			name: "package-level target, call inside a closure",
			src:  preamble + "var ne net.Error\n\nfunc f() func() bool {\n\treturn func() bool { return errors.As(err, &ne) }\n}\n",
			want: 1,
		},
		{
			name: "two sites in one function are counted as two",
			src:  preamble + "func f() bool {\n\tvar a net.Error\n\tvar b net.Error\n\treturn errors.As(err, &a) || errors.As(err, &b)\n}\n",
			want: 2,
		},
		// NEGATIVE CONTROLS.
		{
			name: "NEGATIVE *net.DNSError is a concrete type, not the interface",
			src:  preamble + "func f() bool {\n\tvar d *net.DNSError\n\treturn errors.As(err, &d)\n}\n",
			want: 0,
		},
		{
			name: "NEGATIVE *net.OpError is a concrete type, not the interface",
			src:  preamble + "func f() bool {\n\tvar o *net.OpError\n\treturn errors.As(err, &o)\n}\n",
			want: 0,
		},
		{
			name: "NEGATIVE errors.Is is a different function",
			src:  preamble + "func f() bool {\n\tvar ne net.Error\n\t_ = ne\n\treturn errors.Is(err, nil)\n}\n",
			want: 0,
		},
		{
			name: "NEGATIVE the pattern quoted in a doc comment (how transport_error.go explains itself)",
			src:  preamble + "// It is close to, but deliberately not, `errors.As(err, &netErr)`.\n//\n//\tvar netErr net.Error\n//\treturn errors.As(err, &netErr)\nfunc f() bool {\n\treturn false\n}\n",
			want: 0,
		},
		{
			name: "NEGATIVE a same-named local of a DIFFERENT type in another file",
			src:  preamble + "type myErr struct{}\n\nfunc f() bool {\n\tvar ne *myErr\n\treturn errors.As(err, &ne)\n}\n",
			want: 0,
		},
		{
			name: "NEGATIVE net.Error used as a type but never an errors.As target",
			src:  preamble + "func f(ne net.Error) bool {\n\treturn ne.Timeout()\n}\n",
			want: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scanNetErrorAs(t, "synthetic.go", []byte(tc.src))
			if len(got) != tc.want {
				t.Fatalf("scanner found %d site(s), want %d\n--- source ---\n%s", len(got), tc.want, tc.src)
			}
		})
	}
}

// TestNetErrorAsScannerSurvivesGofmt is the #238 lesson made into a test. That
// guard was disarmed because `gofmt -s` REWROTE the literal spelling it keyed
// on, and CI then enforced the rewritten form. So: take every positive row's
// source, run it through the same printer `gofmt` uses, and require the scanner
// to still see it.
func TestNetErrorAsScannerSurvivesGofmt(t *testing.T) {
	srcs := []string{
		"package p\n\nimport (\n\"errors\"\n\"net\"\n)\n\nvar err error\n\nfunc f() bool {\nvar netErr net.Error\nreturn errors.As( err , &netErr )\n}\n",
		"package p\n\nimport (\n\"errors\"\n\"net\"\n)\n\nvar err error\n\nfunc f() bool {\nvar ne net.Error\nif errors.As(err,&ne)&&ne.Timeout(){return true}\nreturn false\n}\n",
		"package p\n\nimport (\n\"errors\"\n\"net\"\n)\n\nvar err error\n\nfunc f() bool { return errors.As(err, new(net.Error)) }\n",
	}
	for i, src := range srcs {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			before := scanNetErrorAs(t, "synthetic.go", []byte(src))
			if len(before) != 1 {
				t.Fatalf("pre-format: scanner found %d site(s), want 1 — the row is not a positive control", len(before))
			}
			formatted := gofmtSimplify(t, []byte(src))
			after := scanNetErrorAs(t, "synthetic.go", formatted)
			if len(after) != 1 {
				t.Fatalf("POST-`gofmt -s`: scanner found %d site(s), want 1. The repo's own formatter "+
					"disarmed the guard — this is exactly what happened in #238.\n--- formatted ---\n%s",
					len(after), formatted)
			}
		})
	}
}

// gofmtSimplify runs the module's real `gofmt -s` over src. It shells out to the
// same binary `make fmt` and CI use rather than reimplementing the simplifier,
// because a reimplementation would be a fourth thing to keep in lockstep and the
// whole point of the check is "what does the tool actually do".
func gofmtSimplify(t *testing.T, src []byte) []byte {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "x.go")
	if err := os.WriteFile(p, src, 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	bin, err := exec.LookPath("gofmt")
	if err != nil {
		// `go test` always runs with a toolchain, so gofmt should be next to it;
		// fall back to `go fmt`'s binary location before giving up.
		if goroot := os.Getenv("GOROOT"); goroot != "" {
			bin = filepath.Join(goroot, "bin", "gofmt")
		} else {
			t.Fatalf("gofmt not on PATH (%v) — this control cannot run, and a SKIP here would "+
				"read as a pass; install the toolchain or fix PATH", err)
		}
	}
	out, err := exec.Command(bin, "-s", p).Output()
	if err != nil {
		t.Fatalf("gofmt -s %s: %v", p, err)
	}
	if len(out) == 0 {
		t.Fatal("gofmt -s produced empty output — the control is measuring nothing")
	}
	return out
}

// moduleGoFiles returns every non-test .go file in the module, relative to the
// module root, sorted. Hidden dirs, testdata and the scaffold's template trees
// are excluded; nothing else is.
func moduleGoFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != "." && (strings.HasPrefix(name, ".") || name == "testdata" || name == "node_modules" || name == "dist" || name == "bin") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk module: %v", err)
	}
	sort.Strings(out)
	return out
}

// scanNetErrorAs parses src and returns one site per `errors.As(_, target)` call
// whose target is typed as the `net.Error` INTERFACE.
//
// Resolution is syntactic and scoped to the file, which over-approximates: a
// name declared `net.Error` anywhere in a file makes every `errors.As(_, &name)`
// in that file a site, even if the second one is a different variable that
// happens to share the name. Over-approximation is the safe direction — it can
// only produce a spurious FAILURE, which is a stop-and-read, never a spurious
// pass. The under-approximations are listed in this file's header comment.
func scanNetErrorAs(t *testing.T, rel string, src []byte) []netErrorAsSite {
	t.Helper()
	fset := token.NewFileSet()
	// Comments are not parsed: a quoted `errors.As(err, &netErr)` in prose must
	// never become a site, and three files in this repo quote it deliberately.
	file, err := parser.ParseFile(fset, rel, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", rel, err)
	}

	netPkg := importedAs(file, "net")
	errorsPkg := importedAs(file, "errors")
	if netPkg == "" || errorsPkg == "" {
		// A file that imports neither cannot hold the pattern in the shapes this
		// scanner understands.
		return nil
	}

	// Phase 1: every identifier in this file declared with the type `net.Error`
	// (var specs at any level, function parameters, results, struct fields).
	targets := map[string]bool{}
	isNetError := func(e ast.Expr) bool {
		sel, ok := e.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Error" {
			return false
		}
		id, ok := sel.X.(*ast.Ident)
		return ok && id.Name == netPkg
	}
	addNames := func(names []*ast.Ident, typ ast.Expr) {
		if typ == nil || !isNetError(typ) {
			return
		}
		for _, n := range names {
			targets[n.Name] = true
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ValueSpec:
			addNames(x.Names, x.Type)
		case *ast.Field:
			addNames(x.Names, x.Type)
		}
		return true
	})

	// Phase 2: the calls. Walk top-level declarations so the enclosing function
	// name is known; closures normalise to their parent FuncDecl.
	var sites []netErrorAsSite
	collect := func(node ast.Node, fnName string) {
		ast.Inspect(node, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "As" {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != errorsPkg {
				return true
			}
			if !isNetErrorTarget(call.Args[1], targets, isNetError) {
				return true
			}
			sites = append(sites, netErrorAsSite{File: rel, Func: fnName})
			return true
		})
	}
	for _, d := range file.Decls {
		switch x := d.(type) {
		case *ast.FuncDecl:
			collect(x, funcLabel(x))
		default:
			collect(x, "<file-level>")
		}
	}
	return sites
}

// isNetErrorTarget reports whether an `errors.As` second argument is a net.Error
// target: `&someNetErrorVar` or `new(net.Error)`.
func isNetErrorTarget(arg ast.Expr, targets map[string]bool, isNetError func(ast.Expr) bool) bool {
	switch x := arg.(type) {
	case *ast.UnaryExpr:
		if x.Op != token.AND {
			return false
		}
		id, ok := x.X.(*ast.Ident)
		return ok && targets[id.Name]
	case *ast.CallExpr:
		id, ok := x.Fun.(*ast.Ident)
		if !ok || id.Name != "new" || len(x.Args) != 1 {
			return false
		}
		return isNetError(x.Args[0])
	}
	return false
}

// funcLabel names a FuncDecl the way the ledger spells it: `Name` for a
// function, `Recv.Name` for a method.
func funcLabel(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return fd.Name.Name
	}
	recv := fd.Recv.List[0].Type
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}
	if id, ok := recv.(*ast.Ident); ok {
		return id.Name + "." + fd.Name.Name
	}
	return fd.Name.Name
}

// importedAs returns the local name a file imports importPath under ("" if it
// does not import it). This is what keeps the scanner off a spelling: an aliased
// `gonet "net"` is still net.
func importedAs(file *ast.File, importPath string) string {
	for _, imp := range file.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || p != importPath {
			continue
		}
		if imp.Name != nil {
			if imp.Name.Name == "_" {
				continue
			}
			return imp.Name.Name
		}
		return importBase(importPath)
	}
	return ""
}

// importBase is filepath.Base for an import path, which is always slash-separated.
func importBase(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}
