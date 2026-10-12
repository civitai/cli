package plan

import (
	"regexp"
	"testing"
)

// The root ledger checks every filter against the REAL test names, and today
// no name is a substring of another — so an unanchored filter passes it
// (measured: a mutant dropping `^…$` survived that ledger). These names are
// chosen so it cannot: a prefix, a suffix, and a regex metacharacter.
func TestRegexIsAnchoredAndQuoted(t *testing.T) {
	re, err := Regex([]string{"TestA", "Test.B"})
	if err != nil {
		t.Fatal(err)
	}
	rx := regexp.MustCompile(re)
	for name, want := range map[string]bool{
		"TestA":  true,
		"Test.B": true,
		"TestAB": false, // unanchored end
		"XTestA": false, // unanchored start
		"TestxB": false, // unquoted `.`
	} {
		if got := rx.MatchString(name); got != want {
			t.Errorf("%s matches %q = %v, want %v", re, name, got, want)
		}
	}
	if _, err := Regex(nil); err == nil {
		t.Error("Regex(nil) built a filter; an empty alternation matches nothing (or everything)")
	}
}

// Same inputs, same plan — and longest-first onto the least-loaded shard.
func TestBuildIsDeterministicAndBalanced(t *testing.T) {
	a, err := Build("../../..", Shards)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		b, err := Build("../../..", Shards)
		if err != nil {
			t.Fatal(err)
		}
		for s := range a.Shards {
			if len(a.Shards[s]) != len(b.Shards[s]) {
				t.Fatalf("run %d: shard %d differs", i, s+1)
			}
			for j := range a.Shards[s] {
				if a.Shards[s][j] != b.Shards[s][j] {
					t.Fatalf("run %d: shard %d differs at %d", i, s+1, j)
				}
			}
		}
	}
	var longest, min, max float64
	for _, d := range Durations {
		if d > longest {
			longest = d
		}
	}
	min, max = a.Load[0], a.Load[0]
	for _, l := range a.Load {
		if l < min {
			min = l
		}
		if l > max {
			max = l
		}
	}
	// LPT's guarantee: no shard exceeds the lightest by more than the longest job.
	if max-min > longest {
		t.Fatalf("shard loads %v spread %.1f s > longest test %.1f s", a.Load, max-min, longest)
	}
}
