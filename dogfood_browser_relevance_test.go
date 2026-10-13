package cli_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The table for scripts/dogfood/browser-suite-relevant.sh — the ONE place that
// decides whether a PR may skip the `dogfood-browser` job. One row per class
// of change, and every row that could skip is matched by one that must not.
func TestBrowserSuiteRelevanceDecision(t *testing.T) {
	bash := dogfoodTool(t, "bash")
	script := filepath.Join(oracleDir, "browser-suite-relevant.sh")
	for _, tc := range []struct {
		name    string
		event   string
		paths   []string
		noList  bool
		diffRC  string
		wantRun string
		reason  string
	}{
		{"docs only", "pull_request", []string{"README.md", "claudedocs/handoff.md", "AGENTS.md"}, false, "0", "false", "classified irrelevant"},
		{"internal/cmd only", "pull_request", []string{"internal/cmd/app_doctor.go", "internal/cmd/app_doctor_test.go"}, false, "0", "false", "classified irrelevant"},
		{"product packages and templates", "pull_request", []string{"internal/scaffold/templates/page-money/src/App.tsx.tmpl", "internal/validate/rules.go", "cmd/civitai/main.go", "pkg/civitai/client.go"}, false, "0", "false", "classified irrelevant"},
		{"a root test file that is not a dogfood one (same package)", "pull_request", []string{"agents_size_test.go"}, false, "0", "true", "root test file"},
		{"a test file that is NOT at the root", "pull_request", []string{"internal/cmd/root_test.go"}, false, "0", "false", "classified irrelevant"},
		{"another workflow", "pull_request", []string{".github/workflows/release.yml"}, false, "0", "false", "classified irrelevant"},
		{"dogfood harness only", "pull_request", []string{"scripts/dogfood/briefs/_cdp.mjs"}, false, "0", "true", "browser harness"},
		{"a browser-tier test file", "pull_request", []string{"dogfood_oracle_post_test.go"}, false, "0", "true", "root test file"},
		{"an always-tier dogfood test file (shares helpers)", "pull_request", []string{"dogfood_t1_floor_test.go"}, false, "0", "true", "root test file"},
		{"the tier list", "pull_request", []string{"scripts/dogfood/browser-tier.sh"}, false, "0", "true", "browser harness"},
		{"an unrelated tool", "pull_request", []string{"tools/caskcheck/main.go"}, false, "0", "false", "classified irrelevant"},
		{"ci.yml", "pull_request", []string{".github/workflows/ci.yml"}, false, "0", "true", "defines the browser jobs"},
		{"go.mod", "pull_request", []string{"go.mod"}, false, "0", "true", "module graph"},
		{"go.sum", "pull_request", []string{"go.sum"}, false, "0", "true", "module graph"},
		{"docs plus one dogfood file", "pull_request", []string{"README.md", "scripts/dogfood/oracle.sh"}, false, "0", "true", "browser harness"},
		{"an unknown path fails open", "pull_request", []string{"README.md", "newdir/thing.txt"}, false, "0", "true", "not classified"},
		{"an unknown root file fails open", "pull_request", []string{"Dockerfile"}, false, "0", "true", "not classified"},
		{"empty diff fails open", "pull_request", nil, false, "0", "true", "empty"},
		{"diff error fails open", "pull_request", []string{"README.md"}, false, "128", "true", "diff failed"},
		{"unreadable list fails open", "pull_request", nil, true, "0", "true", "unreadable"},
		{"push always runs", "push", []string{"README.md"}, false, "0", "true", "always runs"},
		{"schedule always runs", "schedule", nil, false, "0", "true", "always runs"},
		{"workflow_dispatch always runs", "workflow_dispatch", []string{"README.md"}, false, "0", "true", "always runs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			list := filepath.Join(dir, "paths.txt")
			if !tc.noList {
				body := strings.Join(tc.paths, "\n")
				if body != "" {
					body += "\n"
				}
				if err := os.WriteFile(list, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			out := filepath.Join(dir, "github_output")
			cmd := exec.Command(bash, script, tc.event, list, tc.diffRC)
			cmd.Env = append(os.Environ(), "GITHUB_OUTPUT="+out)
			stdout, err := cmd.Output()
			if err != nil {
				t.Fatalf("the decision script failed (%v) — the workflow would fail open, but the script must not:\n%s", err, stdout)
			}
			if !strings.Contains(string(stdout), "run="+tc.wantRun+"\n") {
				t.Fatalf("got\n%s\nwant run=%s", stdout, tc.wantRun)
			}
			if !strings.Contains(string(stdout), tc.reason) {
				t.Fatalf("reason does not say %q:\n%s", tc.reason, stdout)
			}
			// The value the workflow reads is $GITHUB_OUTPUT, not stdout.
			ghout, err := os.ReadFile(out)
			if err != nil || !strings.Contains(string(ghout), "run="+tc.wantRun+"\n") {
				t.Fatalf("$GITHUB_OUTPUT = %q (err %v), want run=%s", ghout, err, tc.wantRun)
			}
		})
	}

	// Usage error is the ONE non-zero exit; the workflow then runs the suite.
	if err := exec.Command(bash, script).Run(); err == nil {
		t.Fatal("the script accepted no arguments")
	}
}

// 🔴 THE PREMISE BEHIND `internal/**`, `pkg/**` AND `cmd/**` BEING IRRELEVANT,
// AS A GUARD. The dogfood tests drive scripts/dogfood with a STUB docker and a
// STUB `civitai`; they never link this repo's product code. The moment one of
// them imports this module — an internal package, the public SDK, the root
// package, by any import form — a product change can alter what a browser test
// does on a PR whose diff the classifier let skip the browser suite.
//
// A blank or dot import counts: the import PATH is what is checked, never the
// name it is bound to.
func TestNoDogfoodTestImportsTheModule(t *testing.T) {
	const module = "github.com/civitai/cli"
	files, err := filepath.Glob("dogfood_*_test.go")
	if err != nil || len(files) < 10 {
		t.Fatalf("found %d dogfood_*_test.go file(s) (err %v) — this guard is not reading the suite", len(files), err)
	}
	fset := token.NewFileSet()
	imports := 0
	var bad []string
	for _, f := range files {
		af, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, im := range af.Imports {
			imports++
			p, err := strconv.Unquote(im.Path.Value)
			if err != nil {
				t.Fatalf("%s: import %s: %v", f, im.Path.Value, err)
			}
			if p == module || strings.HasPrefix(p, module+"/") {
				bad = append(bad, f+" imports "+p)
			}
		}
	}
	// Positive control: these files import plenty of stdlib; zero imports seen
	// would mean the parser read nothing and the clean verdict is empty.
	if imports < len(files) {
		t.Fatalf("saw only %d import(s) across %d files — the walk is not reading them", imports, len(files))
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Fatalf("a dogfood test imports this module, so product code can now change a browser test's "+
			"behaviour on a PR that skips the browser suite. Either drop the import, or make the imported "+
			"path a trigger in scripts/dogfood/browser-suite-relevant.sh:\n  %s", strings.Join(bad, "\n  "))
	}
}

// 🔴 AND THE TWO WAYS PRODUCT CODE REACHES A BROWSER TEST WITHOUT AN IMPORT OF
// ITS OWN. The guard above reads each dogfood file's import block; these are
// the routes it cannot see.
//
//  1. An INTERNAL test file. A browser-tier file declaring `package cli` (not
//     `cli_test`) reads the root package's symbols — `SchemaJSON`, say — by
//     bare name, with no import to find.
//  2. A HELPER in a root test file that DOES import this module (examples_test.go
//     today). The browser-tier file imports nothing; it calls a function
//     declared next to an import of `internal/validate`.
//
// So: every browser-tier file must be `package cli_test`, and no browser-tier
// file may reference a top-level identifier (func, var, const, type) declared
// in a root test file that imports this module.
//
// ⚠ ONE HOP. A browser-tier file calling a helper in another dogfood file that
// in turn calls into examples_test.go is not caught here; any edit to either
// root test file runs the suite anyway (browser-suite-relevant.sh), so the hop
// only matters for a change made in product code ALONE.
func TestNoProductCodeReachesTheBrowserTier(t *testing.T) {
	const module = "github.com/civitai/cli"
	out, err := exec.Command(dogfoodTool(t, "bash"), "scripts/dogfood/browser-tier.sh", "files").Output()
	if err != nil {
		t.Fatalf("browser-tier.sh files: %v", err)
	}
	tier := map[string]bool{}
	for _, f := range strings.Fields(string(out)) {
		tier[f] = true
	}
	files, err := filepath.Glob("*_test.go")
	if err != nil || len(tier) == 0 || len(files) == 0 {
		t.Fatalf("tier=%d files=%d err=%v — nothing to check", len(tier), len(files), err)
	}
	fset := token.NewFileSet()
	parsed := map[string]*ast.File{}
	declaredIn := map[string]string{} // identifier -> product-importing file
	var importers []string
	for _, f := range files {
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		parsed[f] = af
		imports := false
		for _, im := range af.Imports {
			if p, _ := strconv.Unquote(im.Path.Value); p == module || strings.HasPrefix(p, module+"/") {
				imports = true
			}
		}
		if !imports {
			continue
		}
		importers = append(importers, f)
		for _, d := range af.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil {
					declaredIn[d.Name.Name] = f
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.ValueSpec:
						for _, n := range s.Names {
							declaredIn[n.Name] = f
						}
					case *ast.TypeSpec:
						declaredIn[s.Name.Name] = f
					}
				}
			}
		}
	}
	// Positive control: examples_test.go imports internal/validate today. An
	// empty importer set would make the reference check below vacuous.
	if len(importers) == 0 || len(declaredIn) == 0 {
		t.Fatalf("found no root test file importing %s — the reference check is wired to nothing", module)
	}
	var bad []string
	refs := 0
	for f := range tier {
		af, ok := parsed[f]
		if !ok {
			bad = append(bad, f+" is in browser-tier.sh but not a root *_test.go")
			continue
		}
		if af.Name.Name != "cli_test" {
			bad = append(bad, f+" is `package "+af.Name.Name+"`, not `package cli_test` — it reads the root package's symbols with no import")
		}
		seen := map[string]bool{}
		ast.Inspect(af, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			// Obj == nil: not declared in this file — another file's
			// package-level symbol, an import name, or a builtin.
			if !ok || id.Obj != nil || id.Name == "_" || seen[id.Name] {
				return true
			}
			seen[id.Name] = true
			refs++
			if src, hit := declaredIn[id.Name]; hit {
				bad = append(bad, f+" uses "+id.Name+", declared in "+src+", which imports "+module)
			}
			return true
		})
	}
	if refs == 0 {
		t.Fatal("saw no cross-file references in the browser tier — the walk is not reading it")
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Fatalf("product code can reach a browser-tier test without the classifier seeing it (importers: %s):\n  %s",
			strings.Join(importers, ", "), strings.Join(bad, "\n  "))
	}
}
