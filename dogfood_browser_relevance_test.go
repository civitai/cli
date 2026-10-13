package cli_test

import (
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
