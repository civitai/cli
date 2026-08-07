package cli_test

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// This is the COMPANION to agents_xrefs_test.go, and it closes the hole that
// file names in its own doc comment.
//
// The sibling guard walks the repo and asserts every `item N` reference resolves
// to an item AGENTS.md actually has. It is a check on the POINTERS. It is
// structurally blind to the opposite direction: an item that nothing points at.
// A `24. **…` heading with no clause naming it passes the sibling byte-for-byte,
// because the sibling only ever validates numbers it FOUND — an item nobody
// mentions produces no reference, so there is nothing for it to reject.
//
// That is not hypothetical either. AGENTS.md opens its numbered list with a
// paragraph of cluster clauses ("items 5–9 cover `civitai app metrics`…") that is
// the list's own index — the navigation a reader follows to find the item
// relevant to what they are about to change. Item 20 shipped absent from every
// clause and stayed absent until it was fixed by hand in the sibling's PR. A
// reader following the file's own navigation never reached it, and no test
// noticed, because a missing pointer is a silence rather than a wrong value.
//
// So this file asserts the coverage direction: every item 1..N is named by at
// least one clause in the index.
//
// # Design decisions
//
// # 1. WHAT COUNTS AS "THE INDEX": the preamble, and only the preamble
//
// The region is everything between the `## Intentional decisions that look
// wrong` heading and the first item heading (`1. **`). Both anchors are
// structural, so the locator does not depend on paragraph counts or on prose
// wording that a normal edit moves.
//
// Two regions were deliberately EXCLUDED, and the exclusions cost something:
//
//   - The Layout section's `internal/genapi` bullet ("Read items 12–17, 19, 21
//     and 22 before touching it") is scoped to one package. An item named only
//     there is reachable only by a reader who is already reading about that
//     package — which is the opposite of an index. Counting it would let an item
//     be "indexed" by a clause most readers never see, so an item covered ONLY
//     by that bullet is reported here as an orphan. That is the intended answer,
//     not a false positive.
//   - The item BODIES are outside the region entirely, which is what stops the
//     guard being vacuous. If the region ran past the first heading, item 21's
//     own body could satisfy the requirement that item 21 be indexed. (Belt and
//     braces: an item heading is spelled `21. **`, with no `item` token, so
//     itemRefRe would not match it as a self-reference even if it were in
//     scope. The region bound is the real guarantee; the spelling is luck.)
//
// # 2. THE REGION IS MATCHED AS ONE JOINED STRING, NEVER LINE BY LINE
//
// 🔴 This is the whole reason the guard has a shape rather than being three
// lines of grep, and it was learned by getting it wrong. AGENTS.md is
// hard-wrapped at ~80 columns, so a clause can straddle a line break —
// `…and item` / `24 covers…`, or `items 18 and` / `20 cover…`. A per-line scan
// finds nothing there, and "no match" is indistinguishable from "not present":
// a hand analysis run that way concluded an item was unreferenced when the
// clause naming it was sitting in the file, split across two lines. A guard
// built line-by-line inherits exactly that blind spot and reports the false
// orphan with full confidence. `agentsIndexRegion` therefore collapses all
// whitespace (`strings.Fields` + join) before a single match pass.
// TestIndexClausesSurviveLineWraps pins it with wrapped fixtures.
//
// # 3. RANGES ARE EXPANDED; THE SIBLING'S ENDPOINT-ONLY READING IS NOT ENOUGH
//
// The sibling validates `items 12–17` at its ENDPOINTS, which is correct for
// "does this number exist" and useless here: read that way, items 13, 14, 15 and
// 16 are named by nothing and every range clause manufactures four orphans.
// expandItemCluster fills the interior, on any of the three dash spellings
// (`-`, `–`, `—`). The EN-DASH is the form AGENTS.md actually uses; a guard that
// handled only the ASCII hyphen would report the whole generate cluster as
// orphaned. `and`, `or`, `,` and `+` are separators, NOT ranges — `items 18 and
// 20` must never expand to 19.
//
// # 4. N IS PARSED, NEVER HARDCODED
//
// The item count comes from parseAgentsItems (shared with the sibling), so
// appending the next item to AGENTS.md requires no edit here — it just has to be
// added to a clause, which is the point. The only hardcoded number is a
// positive-control FLOOR, which the append-only list can only grow past.
//
// # 🔴 WHAT THIS GUARD DOES NOT DO
//
// It checks that an item is NAMED. It does not check that the clause naming it
// DESCRIBES it correctly, and it cannot — the clause "items 5–9 cover
// `civitai app metrics`" would satisfy this guard while covering nine items
// about something else entirely. This is the same limitation class the sibling
// states for its own direction (a reference to a valid-but-wrong item passes
// there; a clause making a false claim about the item it names passes here).
// Both are "the number is well-formed, the claim it makes is unverified".
// Reading a clause against the item it points at is a manual audit; nothing
// automated will tell you they diverged.
//
// Two smaller residuals, stated so they are not mistaken for coverage:
//   - Any `item N` mention anywhere in the preamble counts, including a passing
//     one. "Read item 13 before adding any check to that path" is a sentence
//     about precedence, not an index clause, but it satisfies the requirement
//     for item 13. Narrowing to "the first paragraph only" was considered and
//     rejected: it needs a blank-line heuristic that breaks the day someone
//     splits the paragraph, which is a false failure at correct content — the
//     worst outcome for a docs guard, because it trains people to delete it.
//   - Sub-item letters are not modelled, here or in the sibling. A clause can
//     name an item without naming the sub-item a reader needs.

// minIndexedItems is a positive-control FLOOR on the parsed item count, and is
// deliberately a different (higher) number from the sibling's minAgentsItems:
// that floor was written when 23 items existed, this one when 24 did. It is not
// a pin — the list is append-only, so the real count only grows. Its job is to
// fail loudly if parseAgentsItems ever returns a small number, because a small N
// makes the loop below check almost nothing and pass.
const minIndexedItems = 24

// minIndexClauseRefs is the second positive control. If the region locator is
// broken — the heading renamed, the end anchor never found, the file read from
// the wrong working directory — the region comes back empty, zero clauses match,
// and EVERY item is reported orphaned. That failure is real but its message is a
// lie: it blames AGENTS.md's prose for a broken locator. So a zero-match region
// is reported as a CONTROL failure before the orphan check runs at all.
//
// The floor is above 1 on purpose. A single surviving clause is enough to make
// "we matched something" true while an entire spelling — every range, say — has
// stopped being seen. AGENTS.md carries 9 matching clauses today; 5 leaves room
// for consolidation without letting a mostly-dead regex through.
const minIndexClauseRefs = 5

// maxIndexRangeSpan bounds range expansion. A malformed clause (a typo'd
// endpoint, a year read as an item number) must not turn into a multi-gigabyte
// slice; past the bound only the endpoints are taken, which degrades to the
// sibling's reading rather than exploding. Such a clause fails the sibling guard
// anyway, since its endpoint would not resolve.
const maxIndexRangeSpan = 64

// indexRegionEndRe anchors the END of the index region at the first top-level
// item heading. Anchored to the line start, matching the sibling's
// itemHeadingRe, so an indented sub-bullet or a wrapped line beginning with "1."
// cannot truncate the region early.
var indexRegionEndRe = regexp.MustCompile(`(?m)^1\. \*\*`)

// indexNumberRe finds the individual numbers inside a captured cluster. It is
// separate from the sibling's numberRe because this one is used with
// FindAllStringIndex: the expander needs the SEPARATOR text between two adjacent
// numbers to decide range-vs-list, which a values-only match discards.
var indexNumberRe = regexp.MustCompile(`[0-9]+`)

// agentsIndexRegion returns AGENTS.md's index prose — the preamble between the
// numbered list's heading and its first item — as a SINGLE whitespace-collapsed
// string. See design decision 2: the collapse is the load-bearing part.
func agentsIndexRegion(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatalf("read AGENTS.md: %v (CONTROL failure — nothing below this line checked anything)", err)
	}
	body := string(b)
	start := strings.Index(body, itemsSectionHeading)
	if start < 0 {
		t.Fatalf("CONTROL failure, not a finding: AGENTS.md has no %q heading, so the index region could not be located. "+
			"Every item would be reported orphaned by a guard that had read nothing. Fix the locator (or the heading) before reading any result here.",
			itemsSectionHeading)
	}
	rest := body[start+len(itemsSectionHeading):]
	loc := indexRegionEndRe.FindStringIndex(rest)
	if loc == nil {
		t.Fatalf("CONTROL failure, not a finding: found the %q heading but no following item heading matching %s, "+
			"so the end of the index region is unknown. The numbered list was renamed, reformatted or removed.",
			itemsSectionHeading, indexRegionEndRe)
	}
	return strings.Join(strings.Fields(rest[:loc[0]]), " ")
}

// expandItemCluster turns one captured number cluster into every item it names,
// expanding dash ranges and leaving list separators alone. See design decision 3.
//
//	"1–3, 10 and 11"        -> 1 2 3 10 11
//	"12–17, 19, 21 and 22"  -> 12 13 14 15 16 17 19 21 22
//	"18 and 20"             -> 18 20   (NOT 19)
//	"17 + 18"               -> 17 18
func expandItemCluster(cluster string) []int {
	locs := indexNumberRe.FindAllStringIndex(cluster, -1)
	var out []int
	for i, loc := range locs {
		n, err := strconv.Atoi(cluster[loc[0]:loc[1]])
		if err != nil {
			continue
		}
		out = append(out, n)
		if i+1 >= len(locs) {
			break
		}
		// The separator decides range vs list. Only a dash makes a range; a
		// hyphen, an en-dash and an em-dash all count, because AGENTS.md uses
		// the en-dash and a maintainer typing the ASCII form must not silently
		// lose the interior.
		sep := cluster[loc[1]:locs[i+1][0]]
		if !strings.ContainsAny(sep, "-–—") {
			continue
		}
		hi, err := strconv.Atoi(cluster[locs[i+1][0]:locs[i+1][1]])
		if err != nil || hi <= n || hi-n > maxIndexRangeSpan {
			continue
		}
		// The upper endpoint is appended by the next iteration; only the
		// interior is filled here, so nothing is emitted twice.
		for v := n + 1; v < hi; v++ {
			out = append(out, v)
		}
	}
	return out
}

// TestEveryAgentsItemIsNamedByTheIndex asserts that every item in AGENTS.md's
// append-only list is named by at least one clause in the list's own index
// paragraph, so a reader following the file's navigation can reach all of them.
//
// 🔴 NAMED ONLY. That a clause describes the item CORRECTLY is not checked. See
// this file's package-level doc comment.
func TestEveryAgentsItemIsNamedByTheIndex(t *testing.T) {
	count := parseAgentsItems(t)
	if count < minIndexedItems {
		t.Fatalf("CONTROL failure, not a finding: parsed %d items out of AGENTS.md, want >= %d. "+
			"The list is append-only so the count only grows; a smaller number means the heading parser has stopped seeing the list, "+
			"and this guard would then certify coverage of almost nothing.",
			count, minIndexedItems)
	}

	region := agentsIndexRegion(t)
	matches := itemRefRe.FindAllStringSubmatch(region, -1)
	if len(matches) < minIndexClauseRefs {
		t.Fatalf("CONTROL failure, not a finding: the index region yielded only %d clause(s) matching %s, want >= %d.\n"+
			"A region that matches nothing names nothing, so every item below would be reported orphaned by a guard that read nothing. "+
			"Check the region locator, and check that the clause spellings still match the regex — do NOT edit AGENTS.md on the strength of this failure.\n"+
			"region (whitespace-collapsed, first 200 chars): %.200s",
			len(matches), itemRefRe, minIndexClauseRefs, region)
	}

	named := make(map[int][]string)
	for _, m := range matches {
		for _, n := range expandItemCluster(m[1]) {
			named[n] = append(named[n], strings.TrimSpace(m[0]))
		}
	}

	var orphans []string
	for n := 1; n <= count; n++ {
		if len(named[n]) == 0 {
			orphans = append(orphans, fmt.Sprintf("  item %d is named by no clause in the index paragraph", n))
		}
	}
	if len(orphans) > 0 {
		t.Fatalf("%d AGENTS.md item(s) that the file's own index never names:\n%s\n\n"+
			"The paragraph under %q is the list's index — the navigation a reader follows to find the item relevant to what they are changing. "+
			"An item missing from it is unreachable by that route and effectively invisible, which is how item 20 went unindexed until it was fixed by hand.\n"+
			"FIX THE CLAUSE, NOT THE ITEM: add the number to the cluster clause that describes its subject (or write a new clause). "+
			"Never renumber or reorder the list — 46+ cross-references point into it by number.",
			len(orphans), strings.Join(orphans, "\n"), itemsSectionHeading)
	}
	t.Logf("all %d AGENTS.md items are named by the index (%d clauses parsed from the preamble)", count, len(matches))
}

// TestIndexClausesSurviveLineWraps is the negative control for design decision
// 2. Every fixture here is a clause split across a hard wrap exactly as
// AGENTS.md's ~80-column wrapping produces, and every one of them yields NOTHING
// to a per-line scan.
//
// Without this test, "the guard passes on main" is satisfiable by a
// line-by-line implementation that happens to be lucky about where today's
// clauses wrap — and the first reflow of that paragraph turns a correct file
// into a confident false orphan report.
func TestIndexClausesSurviveLineWraps(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []int
	}{
		{"wrap between the token and the number", "…groups on; and item\n24 covers the ONE transport predicate", []int{24}},
		{"wrap inside a list cluster", "items 18 and\n20 cover the checks that tell an author", []int{18, 20}},
		{"wrap inside a range cluster", "analytics read path; items 5–\n9 cover `civitai app metrics`", []int{5, 6, 7, 8, 9}},
		{"wrap between the token and a range cluster", "money irreversibly**; items\n12–17, 19, 21 and 22 cover `civitai generate`", []int{12, 13, 14, 15, 16, 17, 19, 21, 22}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			joined := strings.Join(strings.Fields(tc.in), " ")
			var got []int
			for _, m := range itemRefRe.FindAllStringSubmatch(joined, -1) {
				got = append(got, expandItemCluster(m[1])...)
			}
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("joined scan of %q\n got: %v\nwant: %v", tc.in, got, tc.want)
			}

			// POSITIVE CONTROL for the control: prove the same input read line
			// by line really does lose the clause, so this test is exercising
			// the hazard it claims to and not just restating the happy path.
			var perLine []int
			for _, line := range strings.Split(tc.in, "\n") {
				for _, m := range itemRefRe.FindAllStringSubmatch(line, -1) {
					perLine = append(perLine, expandItemCluster(m[1])...)
				}
			}
			if fmt.Sprint(perLine) == fmt.Sprint(tc.want) {
				t.Errorf("fixture %q is not a wrap hazard: a per-line scan already returns the full answer %v, so this case proves nothing",
					tc.in, tc.want)
			}
		})
	}
}

// TestExpandItemClusterHandlesEveryClauseSpelling is the negative control for
// design decision 3: it drives the expander over the forms AGENTS.md uses and
// over the forms it must NOT expand, so a future narrowing (dropping the
// en-dash) or widening (treating `and` as a range) fails loudly instead of
// quietly changing which items look covered.
func TestExpandItemClusterHandlesEveryClauseSpelling(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []int
	}{
		// Ranges — every dash spelling fills the interior.
		{"en-dash range (the form AGENTS.md uses)", "12–17", []int{12, 13, 14, 15, 16, 17}},
		{"ascii hyphen range", "12-17", []int{12, 13, 14, 15, 16, 17}},
		{"em-dash range", "5—9", []int{5, 6, 7, 8, 9}},
		{"range then list", "1–3, 10 and 11", []int{1, 2, 3, 10, 11}},
		{"range inside a mixed cluster", "12–17, 19, 21 and 22", []int{12, 13, 14, 15, 16, 17, 19, 21, 22}},
		{"adjacent range endpoints add no interior", "8–9", []int{8, 9}},

		// List separators — these must NEVER expand.
		{"and is not a range", "18 and 20", []int{18, 20}},
		{"comma is not a range", "19, 21", []int{19, 21}},
		{"plus is not a range", "17 + 18", []int{17, 18}},
		{"or is not a range", "6 or 9", []int{6, 9}},

		// Degenerate shapes must not explode or invent items.
		{"single number", "23", []int{23}},
		{"descending range is not expanded", "17–12", []int{17, 12}},
		{"equal endpoints", "7-7", []int{7, 7}},
		{"runaway range falls back to endpoints", fmt.Sprintf("1-%d", maxIndexRangeSpan+9000), []int{1, maxIndexRangeSpan + 9000}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := expandItemCluster(tc.in)
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("expandItemCluster(%q)\n got: %v\nwant: %v", tc.in, got, tc.want)
			}
		})
	}
}
