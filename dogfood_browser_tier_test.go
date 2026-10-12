package cli_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const browserTierScript = "scripts/dogfood/browser-tier.sh"

// browserTier runs scripts/dogfood/browser-tier.sh — the one place the CI split
// of this package is written down — and returns one of its three outputs.
func browserTier(t *testing.T, mode string) []string {
	t.Helper()
	out, err := exec.Command(dogfoodTool(t, "bash"), browserTierScript, mode).Output()
	if err != nil {
		t.Fatalf("%s %s: %v", browserTierScript, mode, err)
	}
	return strings.Fields(string(out))
}

// rootTests parses this package's *_test.go files and returns every top-level
// test, by file.
func rootTests(t *testing.T) map[string][]string {
	t.Helper()
	files, err := filepath.Glob("*_test.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no root *_test.go files (err %v) — nothing to partition", err)
	}
	fset := token.NewFileSet()
	byFile := map[string][]string{}
	for _, f := range files {
		af, err := parser.ParseFile(fset, f, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range af.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && isTopLevelTest(fd.Name.Name) {
				byFile[f] = append(byFile[f], fd.Name.Name)
			}
		}
	}
	return byFile
}

// What `go test` collects: TestXxx where Xxx does not start lower-case.
func isTopLevelTest(name string) bool {
	rest, ok := strings.CutPrefix(name, "Test")
	return ok && (rest == "" || rest[0] < 'a' || rest[0] > 'z')
}

// 🔴 THE LEDGER FOR THE CI SPLIT OF THIS PACKAGE: NO TEST LOST, NONE RUN TWICE.
//
// `build-test` runs this package with `-skip <regex>` and `dogfood-browser`
// runs it with `-run <regex>`, the SAME regex, from browser-tier.sh. So the
// split is a partition if and only if that regex selects exactly the tests it
// is meant to: this is the static half of that (each CI job checks its own half
// against `go test -list` of the real binary and the PASS lines it produced).
//
// It is in the ALWAYS tier on purpose — it must run on every PR, including the
// ones that do not run the browser job.
func TestBrowserTierIsAPartitionOfTheRootPackage(t *testing.T) {
	byFile := rootTests(t)
	var all []string
	for _, tests := range byFile {
		all = append(all, tests...)
	}
	sort.Strings(all)

	files := browserTier(t, "files")
	names := browserTier(t, "names")
	regex := browserTier(t, "regex")
	if len(files) == 0 || len(names) == 0 || len(regex) != 1 {
		t.Fatalf("browser-tier.sh printed %d file(s), %d name(s), %d regex line(s) — nothing to check",
			len(files), len(names), len(regex))
	}

	// The script greps names out of source; the parser is the authority.
	var want []string
	var bad []string
	for _, f := range files {
		if len(byFile[f]) == 0 {
			bad = append(bad, f+" is in the browser tier but declares no test (or does not exist)")
		}
		want = append(want, byFile[f]...)
	}
	sort.Strings(want)
	if strings.Join(names, "\n") != strings.Join(want, "\n") {
		bad = append(bad, "browser-tier.sh names ≠ the tests the parser finds in its files:\n    script: "+
			strings.Join(names, " ")+"\n    parser: "+strings.Join(want, " "))
	}

	// 🔴 AND THE FILTER, AS `go test` WILL APPLY IT, SELECTS EXACTLY THOSE.
	// The list being right says nothing about the regex built from it: an
	// unanchored alternation also matches every name that CONTAINS one of them,
	// and a filter that matches nothing is answered by `go test` with exit 0.
	rx, err := regexp.Compile(regex[0])
	if err != nil {
		t.Fatalf("the filter %q does not compile: %v", regex[0], err)
	}
	var selected, left []string
	for _, n := range all {
		if rx.MatchString(n) {
			selected = append(selected, n)
		} else {
			left = append(left, n)
		}
	}
	if strings.Join(selected, "\n") != strings.Join(want, "\n") {
		bad = append(bad, "the filter selects "+strings.Join(selected, " ")+" — want exactly the browser tier")
	}
	if len(selected) == 0 || len(left) == 0 || len(selected)+len(left) != len(all) {
		bad = append(bad, "the filter leaves one tier empty")
	}
	// Anchoring, checked with names the real set cannot supply: today no test
	// name is a substring of another, so an unanchored filter would pass the
	// comparison above.
	if len(want) == 0 {
		t.Fatalf("the browser tier is empty:\n  %s", strings.Join(bad, "\n  "))
	}
	first := want[0]
	for _, probe := range []string{first + "X", "X" + first} {
		if rx.MatchString(probe) {
			bad = append(bad, "the filter is not anchored: it matches "+probe)
		}
	}
	if len(bad) > 0 {
		t.Fatalf("the CI split of the root test package is not a partition:\n  %s", strings.Join(bad, "\n  "))
	}
}
