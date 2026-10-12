package cli_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
		{"an always-tier root test", "pull_request", []string{"agents_size_test.go"}, false, "0", "false", "classified irrelevant"},
		{"another workflow", "pull_request", []string{".github/workflows/release.yml"}, false, "0", "false", "classified irrelevant"},
		{"dogfood harness only", "pull_request", []string{"scripts/dogfood/briefs/_cdp.mjs"}, false, "0", "true", "browser harness"},
		{"a browser-tier test file", "pull_request", []string{"dogfood_oracle_post_test.go"}, false, "0", "true", "dogfood test file"},
		{"an always-tier dogfood test file (shares helpers)", "pull_request", []string{"dogfood_t1_floor_test.go"}, false, "0", "true", "dogfood test file"},
		{"the tier list", "pull_request", []string{"scripts/dogfood/browser-tier.sh"}, false, "0", "true", "browser harness"},
		{"an unrelated tool", "pull_request", []string{"tools/caskcheck/main.go"}, false, "0", "false", "classified irrelevant"},
		{"ci.yml", "pull_request", []string{".github/workflows/ci.yml"}, false, "0", "true", "defines the browser jobs"},
		{"go.mod", "pull_request", []string{"go.mod"}, false, "0", "true", "module graph"},
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

// 🔴 THE RULE THAT LETS AN ALWAYS-TIER ROOT TEST FILE SKIP THE BROWSER SUITE,
// CHECKED RATHER THAN ASSUMED. browser-suite-relevant.sh treats a root
// `*_test.go` that is not `dogfood_*_test.go` as irrelevant. That holds only
// while no browser-tier file USES a top-level symbol declared in such a file —
// otherwise editing that helper changes a browser test's behaviour on a PR
// that skipped the browser suite. So: every identifier a browser-tier file
// references and does not declare itself must be declared in a dogfood_*_test.go
// file (or not at the root at all — stdlib, imports, builtins).
func TestNoBrowserTestReachesAnUnwatchedRootFile(t *testing.T) {
	files, err := filepath.Glob("*_test.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no root *_test.go files (err %v)", err)
	}
	fset := token.NewFileSet()
	parsed := map[string]*ast.File{}
	declaredIn := map[string]string{}
	for _, f := range files {
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		parsed[f] = af
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
	var bad []string
	browserFiles := 0
	crossFile := 0
	browser := map[string]bool{}
	for _, f := range browserTier(t, "files") {
		browser[f] = true
	}
	for f, af := range parsed {
		if !browser[f] {
			continue
		}
		browserFiles++
		seen := map[string]bool{}
		ast.Inspect(af, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			// Obj == nil: not resolved within this file — a package-level
			// symbol from another file, an import, or a builtin.
			if !ok || id.Obj != nil || seen[id.Name] {
				return true
			}
			seen[id.Name] = true
			src, isRoot := declaredIn[id.Name]
			if !isRoot || src == f {
				return true
			}
			crossFile++
			if !strings.HasPrefix(src, "dogfood_") {
				bad = append(bad, f+" uses "+id.Name+", declared in "+src)
			}
			return true
		})
	}
	// Positive control: the browser tier is known to share helpers across
	// files (stubOracleEnv, runScript, oracleBrowser…). Zero cross-file uses
	// would mean this walk is not seeing them, and its clean verdict is empty.
	if browserFiles == 0 || crossFile == 0 {
		t.Fatalf("saw %d browser-tier file(s) and %d cross-file reference(s) — the walk is not reading the tier", browserFiles, crossFile)
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Fatalf("browser-tier tests depend on root files the on-change rule treats as irrelevant. Move the "+
			"helper into a dogfood_*_test.go file, or add its file to browser-suite-relevant.sh's trigger set:\n  %s",
			strings.Join(bad, "\n  "))
	}
}
