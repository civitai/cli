// Command dogfoodshards prints the CI partition of the repo-root test package:
// the filter `build-test` skips, each `dogfood-browser` shard's filter, and the
// name lists the aggregate job checks the shards' PASS lines against.
//
// Run from the repo root. Every output is derived from ./plan, which the root
// package's ledger test (dogfood_browser_shards_test.go) checks too — so CI and
// the ledger cannot disagree about who runs what.
//
//	go run ./tools/dogfoodshards -print skip               # build-test: -skip filter
//	go run ./tools/dogfoodshards -print run -shard 2       # shard 2: -run filter
//	go run ./tools/dogfoodshards -print list -shard 2      # shard 2's names
//	go run ./tools/dogfoodshards -print list -tier always  # build-test's names
//	go run ./tools/dogfoodshards -print list -tier browser # every shard's names
//	go run ./tools/dogfoodshards -print plan               # human-readable plan
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/civitai/cli/tools/dogfoodshards/plan"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "dogfoodshards:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("dogfoodshards", flag.ContinueOnError)
	print := fs.String("print", "plan", "what to print: skip | run | list | shards | plan")
	shard := fs.Int("shard", 0, "1-based shard index (run, list)")
	tier := fs.String("tier", "", "always | browser (list, instead of -shard)")
	dir := fs.String("dir", ".", "directory holding the root *_test.go files")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := plan.Build(*dir, plan.Shards)
	if err != nil {
		return err
	}
	shardNames := func() ([]string, error) {
		if *shard < 1 || *shard > len(p.Shards) {
			return nil, fmt.Errorf("-shard %d is outside 1..%d", *shard, len(p.Shards))
		}
		return p.Shards[*shard-1], nil
	}
	switch *print {
	case "skip":
		re, err := plan.Regex(p.Browser())
		if err != nil {
			return err
		}
		fmt.Println(re)
	case "run":
		names, err := shardNames()
		if err != nil {
			return err
		}
		re, err := plan.Regex(names)
		if err != nil {
			return err
		}
		fmt.Println(re)
	case "list":
		var names []string
		switch {
		case *tier == "always" && *shard == 0:
			names = p.Always
		case *tier == "browser" && *shard == 0:
			names = p.Browser()
		case *tier == "" && *shard != 0:
			if names, err = shardNames(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("-print list takes exactly one of -shard N or -tier always|browser")
		}
		fmt.Println(strings.Join(names, "\n"))
	case "shards":
		fmt.Println(len(p.Shards))
	case "plan":
		fmt.Printf("%d tests: %d always (build-test), %d browser in %d shards\n",
			len(p.All), len(p.Always), len(p.Browser()), len(p.Shards))
		for i, s := range p.Shards {
			fmt.Printf("shard %d: %2d tests, %5.1f s measured\n", i+1, len(s), p.Load[i])
		}
	default:
		return fmt.Errorf("unknown -print %q", *print)
	}
	return nil
}
