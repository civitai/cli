package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// EVERY CLAUSE OF THE PRE-SPLIT CONTRACT IS STILL PUBLISHED — ASSERTED, NOT
// ASSURED.
//
// The summary/detail split (see the header in exitcodes_doc.go) moved ~4 kB of
// prose out of two markdown table cells and into per-code subsections. The
// content was correct: an audit verified every clause by running the binary,
// and the code-2 paragraph in particular has been published WRONG TWICE — once
// over-narrow ("every local path a FLAG names", which excluded the two commands
// taking the path positionally) and once over-broad ("every local path the CLI
// is HANDED", which had a live counterexample inside its own scope). Its
// precision is what those two rounds bought.
//
// So "I moved it all, nothing was lost" is exactly the claim a reviewer cannot
// check by eye across 4 kB of prose, and exactly the claim a diff makes look
// checked. This file checks it mechanically: every sentence of the pre-split
// published text must still appear, verbatim, in the code it belonged to.
//
// 🔴 IT IS DELIBERATELY GIT-FREE. CI checks out at depth 1, so a guard that
// read the base commit would be green locally (where the object exists) and
// broken in CI (where it does not) — and a guard that SKIPS in CI protects
// nothing. The pre-split text is therefore a testdata fixture, and the git
// cross-check below is the belt to that braces: it proves the fixture is
// genuinely the base commit's text when the object is reachable, and skips
// LOUDLY when it is not, while the fixture-based guard above keeps holding the
// property either way.

// preSplitBaseCommit is the commit whose README exit-code table the fixture was
// taken from — this branch's base. It is only used by the cross-check, which
// skips when the object is unreachable (a shallow clone).
const preSplitBaseCommit = "932c5e9"

// preSplitFixture is the path, relative to this package, of the verbatim
// pre-split table.
const preSplitFixture = "testdata/exitcodes_base_table.md"

// sentenceAbbrevs are the token endings a naive splitter mistakes for a
// sentence end. A false split is harmless to the assertion — every sub-slice of
// a preserved passage is still a substring of it — but it makes the failure
// message and the sentence count meaningless, so they are handled.
var sentenceAbbrevs = []string{"e.g.", "i.e.", "etc.", "cf.", "vs."}

// splitSentences cuts flattened prose at `.`/`!`/`?` followed by a space or the
// end of the string.
func splitSentences(s string) []string {
	r := []rune(flattenWS(s))
	var out []string
	start := 0
	for i := 0; i < len(r); i++ {
		switch r[i] {
		case '.', '!', '?':
		default:
			continue
		}
		if i+1 < len(r) && r[i+1] != ' ' {
			continue
		}
		cand := strings.TrimSpace(string(r[start : i+1]))
		if cand == "" {
			continue
		}
		abbrev := false
		for _, a := range sentenceAbbrevs {
			if strings.HasSuffix(cand, a) {
				abbrev = true
				break
			}
		}
		if abbrev {
			continue
		}
		out = append(out, cand)
		start = i + 1
	}
	if tail := strings.TrimSpace(string(r[start:])); tail != "" {
		out = append(out, tail)
	}
	return out
}

// preSplitCells parses the fixture (or any rendering of the old table) into
// code → cell text.
func preSplitCells(table string) (map[int]string, error) {
	out := map[int]string{}
	for _, line := range strings.Split(table, "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		rest := line[len("| `"):]
		tick := strings.Index(rest, "`")
		if tick < 0 {
			return nil, fmt.Errorf("malformed row: %.60s…", line)
		}
		var code int
		if _, err := fmt.Sscanf(rest[:tick], "%d", &code); err != nil {
			return nil, fmt.Errorf("row does not open with a code: %.60s…", line)
		}
		cell := rest[tick:]
		cell = strings.TrimPrefix(cell, "` | ")
		cell = strings.TrimSuffix(strings.TrimRight(cell, " "), "|")
		out[code] = strings.TrimSpace(cell)
	}
	return out, nil
}

func readPreSplitFixture(t *testing.T) map[int]string {
	t.Helper()
	b, err := os.ReadFile(preSplitFixture)
	if err != nil {
		t.Fatalf("read %s: %v\nThe pre-split text is a FIXTURE on purpose — CI checks out at depth 1, "+
			"so this guard cannot read it from git.", preSplitFixture, err)
	}
	cells, err := preSplitCells(string(b))
	if err != nil {
		t.Fatalf("parse %s: %v", preSplitFixture, err)
	}
	if len(cells) != len(exitCodeDocs) {
		t.Fatalf("%s holds %d rows, but the contract documents %d codes — the fixture is stale or truncated",
			preSplitFixture, len(cells), len(exitCodeDocs))
	}
	return cells
}

// TestEveryPreSplitClauseSurvives is the guard the whole restructure rests on.
//
// For every code, every sentence the pre-split contract published must still be
// published BY THAT CODE — in its Summary or in one of its Detail bullets. The
// comparison is on flattened whitespace only: no lowercasing, no punctuation
// normalisation, no fuzzy match. A paraphrase fails, a compression fails, a
// dropped clause fails.
//
// Mutation-measured: deleting the two-residuals bullet from code 2's Detail
// reddens it by name with the missing sentence quoted.
func TestEveryPreSplitClauseSurvives(t *testing.T) {
	cells := readPreSplitFixture(t)

	byCode := make(map[int]ExitCodeDoc, len(exitCodeDocs))
	for _, d := range exitCodeDocs {
		byCode[d.Code] = d
	}

	total := 0
	for code, cell := range cells {
		doc, ok := byCode[code]
		if !ok {
			t.Errorf("the pre-split contract documented exit code %d, which exitCodeDocs no longer does", code)
			continue
		}
		published := flattenWS(doc.published())
		sentences := splitSentences(cell)
		total += len(sentences)
		for _, s := range sentences {
			if !strings.Contains(published, s) {
				t.Errorf("exit code %d no longer publishes a clause the pre-split contract did.\n\n"+
					"MISSING:\n  %s\n\nThe restructure is a CONTAINER change: every clause moves, none is\n"+
					"paraphrased, compressed or dropped. Put it back verbatim — into Summary if it is\n"+
					"short enough for a terminal line, otherwise into a Detail bullet.\n\n"+
					"code %d now publishes:\n  %s", code, s, code, published)
			}
		}
	}

	// Positive control: a fixture that parsed to a handful of fragments would
	// report a serene pass. The pre-split code-2 cell alone is 14 sentences.
	if total < 30 {
		t.Fatalf("only %d pre-split sentences were checked — the splitter or the fixture is wrong", total)
	}
}

// TestPreSplitClauseGuardCanFail is the negative control for the comparison
// above. Without it, a green run is indistinguishable from a matcher that
// accepts everything — a flattenWS that returned "" for both sides, say.
func TestPreSplitClauseGuardCanFail(t *testing.T) {
	cells := readPreSplitFixture(t)

	const fabricated = "Exit code 2 is returned when Mercury is in retrograde."
	for code, doc := range func() map[int]ExitCodeDoc {
		m := map[int]ExitCodeDoc{}
		for _, d := range exitCodeDocs {
			m[d.Code] = d
		}
		return m
	}() {
		if strings.Contains(flattenWS(doc.published()), fabricated) {
			t.Errorf("code %d appears to publish a fabricated sentence — the matcher cannot reject anything", code)
		}
	}

	// And the splitter must really be cutting the fixture up, or "every
	// sentence is present" is a claim about one giant string that happens to be
	// a substring of itself.
	two, ok := cells[2]
	if !ok {
		t.Fatal("the fixture has no code-2 row")
	}
	if n := len(splitSentences(two)); n < 10 {
		t.Fatalf("the pre-split code-2 cell split into only %d sentences — the splitter is not cutting", n)
	}
}

// TestPreSplitFixtureMatchesGit is the cross-check that the fixture is really
// the base commit's text rather than something hand-typed to match today's
// source.
//
// 🔴 IT SKIPS LOUDLY IN A SHALLOW CLONE, AND THAT IS WHY IT IS NOT THE PRIMARY
// GUARD. GitHub Actions checks out at depth 1, so the base object is simply not
// present there and no amount of git plumbing will produce it. A guard that
// skipped in CI would be a guard that only ever ran where nothing was at stake;
// TestEveryPreSplitClauseSurvives holds the property everywhere, and this one
// upgrades the fixture from "asserted" to "provenanced" wherever the history
// exists.
func TestPreSplitFixtureMatchesGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("SKIPPED (no git on PATH): the fixture's provenance is unverified here. "+
			"TestEveryPreSplitClauseSurvives still holds the clause-preservation property. (%v)", err)
	}
	root := filepath.Join("..", "..")
	if out, err := exec.Command("git", "-C", root, "cat-file", "-e", preSplitBaseCommit+"^{commit}").CombinedOutput(); err != nil {
		t.Skipf("SKIPPED (commit %s is not in this checkout — a SHALLOW clone, as CI uses): "+
			"the fixture's provenance is unverified here. TestEveryPreSplitClauseSurvives still holds the "+
			"clause-preservation property. (%v: %s)", preSplitBaseCommit, err, strings.TrimSpace(string(out)))
	}

	out, err := exec.Command("git", "-C", root, "show", preSplitBaseCommit+":README.md").Output()
	if err != nil {
		t.Fatalf("git show %s:README.md: %v", preSplitBaseCommit, err)
	}
	base := string(out)

	i := strings.Index(base, "\n## Exit codes\n")
	if i < 0 {
		t.Fatalf("commit %s has no `## Exit codes` README section — the provenance constant is wrong", preSplitBaseCommit)
	}
	rest := base[i:]
	j := strings.Index(rest, "| Code | Meaning |")
	if j < 0 {
		t.Fatalf("commit %s's Exit codes section has no table", preSplitBaseCommit)
	}
	rest = rest[j:]
	var lines []string
	for _, line := range strings.Split(rest, "\n") {
		if strings.TrimSpace(line) == "" {
			break
		}
		lines = append(lines, line)
	}
	wantTable := strings.Join(lines, "\n")

	b, err := os.ReadFile(preSplitFixture)
	if err != nil {
		t.Fatalf("read %s: %v", preSplitFixture, err)
	}
	if got := strings.TrimRight(string(b), "\n"); got != wantTable {
		t.Errorf("%s is not byte-identical to the exit-code table at %s.\n"+
			"The fixture is the PROVENANCE of every clause-preservation assertion — regenerate it from git, "+
			"do not hand-edit it to match.\n\nwant:\n%s\n\ngot:\n%s", preSplitFixture, preSplitBaseCommit, wantTable, got)
	}
}
