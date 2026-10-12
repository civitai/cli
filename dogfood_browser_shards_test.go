package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/civitai/cli/tools/dogfoodshards/plan"
)

// 🔴 THE LEDGER FOR THE CI SPLIT OF THIS PACKAGE: NO TEST LOST, NONE RUN TWICE.
//
// `build-test` runs this package with `-skip <every browser test>`, and each
// `dogfood-browser` shard runs it with `-run <its tests>`. Both filters come
// from tools/dogfoodshards/plan. A test that matches neither filter runs NOWHERE
// and every job stays green; a test both match runs twice and costs a shard for
// nothing. This test is the static half of that guarantee (the runtime half is
// ci.yml's `dogfood-browser` aggregate, which counts the PASS lines the shards
// actually produced against `go test -list`).
//
// It is in the ALWAYS tier on purpose: it must run on every PR, including the
// ones that do not run the browser shards.
func TestDogfoodBrowserShardsPartitionTheRootPackage(t *testing.T) {
	p, err := plan.Build(".", plan.Shards)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.All) < 100 || len(p.Always) == 0 || len(p.Shards) != plan.Shards {
		t.Fatalf("implausible plan: %d tests, %d always, %d shards — the parser is not reading this package",
			len(p.All), len(p.Always), len(p.Shards))
	}

	// Each exactly once.
	where := map[string][]string{}
	for _, n := range p.Always {
		where[n] = append(where[n], "build-test")
	}
	for i, s := range p.Shards {
		for _, n := range s {
			where[n] = append(where[n], fmt.Sprintf("shard %d", i+1))
		}
	}
	var bad []string
	for _, n := range p.All {
		if len(where[n]) != 1 {
			bad = append(bad, fmt.Sprintf("%s runs in %d places %v", n, len(where[n]), where[n]))
		}
		delete(where, n)
	}
	for n, w := range where {
		bad = append(bad, fmt.Sprintf("%s is scheduled in %v but is not a test in this package", n, w))
	}

	// 🔴 AND EACH FILTER, AS `go test` WILL APPLY IT, SELECTS EXACTLY ITS PART.
	// The partition above is a claim about name LISTS; CI hands `go test` a
	// REGEX built from them, and an escaping or anchoring bug there — an
	// unanchored `TestOracle` also matching `TestOracleX`, a filter that matches
	// nothing (which `go test` answers with exit 0 and `[no tests to run]`) —
	// leaves the lists right and the run wrong.
	selects := func(re string) []string {
		rx, err := regexp.Compile(re)
		if err != nil {
			t.Fatalf("filter %q does not compile: %v", re, err)
		}
		var out []string
		for _, n := range p.All {
			if rx.MatchString(n) {
				out = append(out, n)
			}
		}
		return out
	}
	for i, s := range p.Shards {
		re, err := plan.Regex(s)
		if err != nil {
			t.Fatalf("shard %d: %v", i+1, err)
		}
		if got := selects(re); strings.Join(got, ",") != strings.Join(s, ",") {
			bad = append(bad, fmt.Sprintf("shard %d's -run filter selects %d test(s), want exactly its %d: %v",
				i+1, len(got), len(s), got))
		}
	}
	skip, err := plan.Regex(p.Browser())
	if err != nil {
		t.Fatal(err)
	}
	skipped := map[string]bool{}
	for _, n := range selects(skip) {
		skipped[n] = true
	}
	var kept []string
	for _, n := range p.All {
		if !skipped[n] {
			kept = append(kept, n)
		}
	}
	if strings.Join(kept, ",") != strings.Join(p.Always, ",") {
		bad = append(bad, fmt.Sprintf("build-test's -skip filter leaves %d test(s), want exactly the %d always-tier ones",
			len(kept), len(p.Always)))
	}

	if len(bad) > 0 {
		sort.Strings(bad)
		t.Fatalf("the CI split of the root test package is not a partition:\n  %s", strings.Join(bad, "\n  "))
	}
}

// The shard COUNT is stated twice — plan.Shards and ci.yml's matrix — and the
// aggregate job only checks the shards that ran. A matrix that grew would run
// a shard the plan never filled (`-shard 5` refuses, so that one is loud); a
// matrix that SHRANK would silently drop a shard's tests from every PR. So the
// two are pinned against each other.
var shardMatrixRe = regexp.MustCompile(`(?m)^\s+shard:\s*\[([0-9,\s]*)\]\s*$`)

func TestDogfoodBrowserShardMatrixMatchesThePlan(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(workflowDir, "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	m := shardMatrixRe.FindAllSubmatch(raw, -1)
	if len(m) != 1 {
		t.Fatalf("ci.yml: found %d `shard: [ … ]` matrix line(s), want exactly 1 (the dogfood-browser-shard job). "+
			"A guard that matches nothing here passes vacuously.", len(m))
	}
	var got []int
	for _, f := range strings.FieldsFunc(string(m[0][1]), func(r rune) bool { return r == ',' || r == ' ' }) {
		n, err := strconv.Atoi(f)
		if err != nil {
			t.Fatalf("ci.yml shard matrix entry %q is not a number", f)
		}
		got = append(got, n)
	}
	var want []int
	for i := 1; i <= plan.Shards; i++ {
		want = append(want, i)
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("ci.yml's dogfood-browser-shard matrix is %v but tools/dogfoodshards/plan.Shards = %d (want %v). "+
			"Change both together: a matrix shorter than the plan drops a shard's tests from CI with every job green.",
			got, plan.Shards, want)
	}
}
