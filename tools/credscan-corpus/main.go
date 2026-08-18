// Command credscan-corpus re-measures the warning's firing rate over real
// project directories using the SHIPPED path (pkgzip.Build then credscan.Scan
// over Result.Files). One-shot measurement aid; not part of the build.
package main

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/civitai/cli/internal/credscan"
	"github.com/civitai/cli/internal/pkgzip"
)

func main() {
	var scanned, fired, errored, files, truncated int
	var firedDirs []string
	var slowest time.Duration
	var slowestDir string
	labels := map[string]int{}
	for _, dir := range os.Args[1:] {
		res, err := pkgzip.Build(dir)
		if err != nil {
			errored++
			fmt.Fprintf(os.Stderr, "SKIP %s: %v\n", dir, err)
			continue
		}
		scanned++
		files += len(res.Files)
		start := time.Now()
		rep := credscan.Scan(dir, res.Files)
		if d := time.Since(start); d > slowest {
			slowest, slowestDir = d, dir
		}
		if rep.Truncated {
			truncated++
			fmt.Fprintf(os.Stderr, "TRUNC %s: %d of %d files\n", dir, rep.FilesScanned, rep.FilesTotal)
		}
		if len(rep.Findings) == 0 {
			continue
		}
		fired++
		firedDirs = append(firedDirs, dir)
		for _, f := range rep.Findings {
			labels[f.Label]++
			fmt.Fprintf(os.Stderr, "FIRE %s :: %s:%d (%s)\n", dir, f.Path, f.Line, f.Label)
		}
	}
	sort.Strings(firedDirs)
	denom := scanned
	if denom == 0 {
		denom = 1
	}
	fmt.Printf("projects scanned: %d (build errors: %d)\npackaged files:   %d\nprojects fired:   %d (%.2f%%)\ntruncated scans:  %d\nslowest project:  %v (%s)\n",
		scanned, errored, files, fired, 100*float64(fired)/float64(denom), truncated, slowest.Round(time.Millisecond), slowestDir)
	for _, d := range firedDirs {
		fmt.Printf("  fired: %s\n", d)
	}
	for l, n := range labels {
		fmt.Printf("  label %q: %d line(s)\n", l, n)
	}
}
