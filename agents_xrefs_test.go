package cli_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// AGENTS.md carries a numbered list, "Intentional decisions that look wrong
// (read before 'fixing')", that is explicitly APPEND-ONLY and never renumbered —
// precisely because ~50 comments, workflow files and README paragraphs point at
// its entries by number. Nothing enforced that those pointers resolve, so this
// file is that enforcement.
//
// 🔴 WHAT THIS GUARD DOES NOT DO — READ THIS BEFORE TRUSTING A GREEN RUN.
//
// It checks that every `item N` / `items N–M` reference names an item that
// EXISTS (1 ≤ N ≤ the number of items parsed out of AGENTS.md). That catches the
// renumber/deletion class: delete item 7 or renumber the list and the dangling
// pointers fail by name.
//
// It does NOT check that a reference points at the RIGHT item, and it cannot.
// A comment saying "see AGENTS.md item 18" beside code that item 19 documents
// passes this guard byte-for-byte, because 18 exists. That is not hypothetical:
// it is exactly the bug this file shipped with. Six references — in
// internal/genapi/graph.go, internal/cmd/generate.go (×4) and README.md —
// pointed at valid-but-wrong items, and two range clauses inside AGENTS.md
// itself had gone stale (the Layout paragraph omitted items 21 and 22; the
// preamble omitted item 20 entirely). Every one of them would pass the guard
// below. This is the "spelled rather than structural" shape the repo's own rules
// warn about: the number is well-formed, the claim it makes is false.
//
// So: the ONE-TIME MANUAL AUDIT in the PR that added this file is what
// establishes semantic correctness, and it establishes it only as of that
// commit. A maintainer adding or editing a cross-reference gets NO help here
// beyond "the number exists". Re-audit by hand — read the item, read the code
// around the reference, and confirm the subjects match. Nothing automated will
// tell you they diverged.
//
// Two smaller limits, stated so they are not mistaken for coverage:
//   - A range is validated at its ENDPOINTS. `items 12–17` checks 12 and 17;
//     it does not assert that 13, 14, 15 and 16 are all on-topic (or that the
//     range is even the right shape).
//   - Sub-item letters (`item 19(e)`) are NOT validated. Nothing parses the
//     `(a)`/`(b)` bullets out of AGENTS.md, so a reference to a sub-item that
//     does not exist passes.
//
// # 🔴 THE SCAN IS PARAGRAPH-WISE, NOT LINE-WISE, AND IT WAS LINE-WISE FOR A
// # RELEASE
//
// This is the same blind spot agents_index_test.go's design decision 2 records,
// regenerated at the sibling call site — which is exactly the "one rule, one
// place" shape the repo's own rules warn about, and the reason it went unnoticed
// is that the index guard was fixed and this one was not. Every corpus this
// walks is hard-wrapped: AGENTS.md and the evidence files at ~79 columns, Go doc
// comments at ~77 after the `// `. So a reference straddles a line break
// routinely — `…and item` / `24 covers…`, `items 12–17, 19, 21` / `and 22`. A
// per-line scan finds NOTHING there, and finding nothing is indistinguishable
// from there being nothing.
//
// Measured at the commit that fixed it: the line-wise scan saw 390 references
// and the paragraph-wise scan sees 402. TWELVE references across TEN files were
// invisible — including four inside this test suite's own siblings — and the
// suite was green throughout, because a lost reference is a silence and the
// floor below (40) is nowhere near tight enough to notice twelve. A reflow that
// splits `item 27` across a wrap does not just go unchecked, it goes unSEEN.
//
// collapseParagraphs is the repair. It joins each maximal run of non-blank lines
// into one whitespace-collapsed string before matching, which is precisely the
// inverse of a hard wrap, and it strips a leading comment marker (`//`, `#`, `*`)
// from each line first — without that, `…21` / `// and 22` collapses to
// `21 // and 22` and the cluster still stops at 21, so the fix would have been
// half a fix on the Go corpus that carries most of these references.
// TestItemRefsSurviveLineWraps pins it with fixtures a per-line scan loses, each
// carrying its own positive control that it really does lose them.
//
// The residual, in the widening direction: joining also merges adjacent list
// items inside one paragraph, so `- …about item` / `- 7 things` yields a
// spurious reference to 7. Harmless here for the same reason scannedExts can be
// widened freely — the assertion is "does this number resolve", so an extra
// reference to an item that EXISTS cannot manufacture a failure, and one to an
// item that does not is a finding worth having anyway.

// itemsSectionHeading is where the numbered list begins. Items are matched only
// below it, so an ordinary numbered list elsewhere in AGENTS.md cannot inflate
// the count.
const itemsSectionHeading = "## Intentional decisions that look wrong"

// minAgentsItems is a floor on the parsed item count. It is a POSITIVE CONTROL,
// not a pin: the list is append-only so the real count only grows, and a parser
// that silently matched one heading (or none) would otherwise make every
// reference below "out of range" — or, worse, make a heading regex that matches
// nothing report a serene pass. 23 items existed when this guard landed.
const minAgentsItems = 23

// minItemRefs is the second positive control. A regex that silently stops
// matching — a changed comment convention, a bad character class, a walk rooted
// at the wrong directory — finds zero references and validates zero of them,
// which is indistinguishable from "everything resolves". 46+ references existed
// when this landed and 402 do now; requiring 40 leaves room for deletions
// without letting a wired-to-nothing scan pass.
//
// 🔴 IT IS A FLOOR AGAINST A DEAD SCAN, NOT A PIN, AND IT CANNOT SEE A PARTIAL
// ONE. The line-wise scan this file shipped with lost twelve references and
// still returned 390, so a floor of 40 said nothing about the twelve. Do not
// read a comfortable margin here as evidence that the corpus is fully seen;
// that claim rests on TestItemRefsSurviveLineWraps, not on this number.
const minItemRefs = 40

// itemHeadingRe matches a top-level numbered item in the list: `1. **…`.
// Anchored to the line start so an indented sub-bullet or a wrapped line that
// happens to begin with a digit is not counted.
var itemHeadingRe = regexp.MustCompile(`(?m)^([0-9]+)\. \*\*`)

// itemRefRe matches a cross-reference and captures the whole number cluster, so
// the plural and range spellings the repo actually uses are covered:
//
//	item 7 · items 6 and 9 · items 12–17 · items 12-17 · items 17 + 18
//	items 13 and 19(c) · items 12–17, 19, 21 and 22 · the item-11 handshake
//
// The separator class deliberately does NOT include `.` or a bare space, so a
// sentence like "item 7. The errors.Is half" stops at 7 and "item 21,
// `modelSubstitutions`" stops at 21. Sub-item letters are left outside the
// capture and ignored (see the doc comment).
var itemRefRe = regexp.MustCompile(`(?i)items?[\s-]+([0-9]+(?:\s*(?:[-–—+,]|and|or)\s*[0-9]+)*)`)

// numberRe pulls the individual item numbers back out of a captured cluster.
var numberRe = regexp.MustCompile(`[0-9]+`)

// scannedExts are the file types that carry cross-references today: Go source
// and test comments, markdown docs, and the workflow YAML. Widening this can
// only ADD coverage — the check is "does the number resolve", so scanning an
// extra file cannot manufacture a failure at a correct reference.
var scannedExts = map[string]bool{
	".go": true, ".md": true, ".yml": true, ".yaml": true,
}

// skippedDirs are never source. `.git` in particular holds packed objects that
// would be read as text.
var skippedDirs = map[string]bool{
	".git": true, "node_modules": true, "dist": true, "bin": true,
}

// parseAgentsItems returns the highest item number in AGENTS.md's numbered list
// and Fatals if the list is not contiguous 1..N.
//
// The count is DERIVED, never hardcoded: hardcoding it means the guard rots the
// day the next item lands, reporting a false failure at a correct reference —
// which is the worst outcome for a docs guard, because it trains people to
// delete it. (Note this comment cannot name that next number in the `item N`
// form: the scan reads its own source file, so the guard would flag itself.)
func parseAgentsItems(t *testing.T) int {
	t.Helper()
	b, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	body := string(b)
	i := strings.Index(body, itemsSectionHeading)
	if i < 0 {
		t.Fatalf("AGENTS.md has no %q heading — the numbered list this guard validates against is gone or was renamed", itemsSectionHeading)
	}
	matches := itemHeadingRe.FindAllStringSubmatch(body[i:], -1)
	if len(matches) < minAgentsItems {
		t.Fatalf("parsed %d items out of AGENTS.md, want >= %d — the heading regex %q has stopped matching the list's spelling; every reference check below is vacuous until it does",
			len(matches), minAgentsItems, itemHeadingRe)
	}
	for idx, m := range matches {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("unparseable item heading %q: %v", m[0], err)
		}
		if n != idx+1 {
			t.Fatalf("AGENTS.md item list is not contiguous: the %d%s heading is numbered %d. The list is APPEND-ONLY and must never be renumbered — every `item N` reference in this repo is a pointer into it",
				idx+1, ordinalSuffix(idx+1), n)
		}
	}
	return len(matches)
}

func ordinalSuffix(n int) string {
	switch {
	case n%100 >= 11 && n%100 <= 13:
		return "th"
	case n%10 == 1:
		return "st"
	case n%10 == 2:
		return "nd"
	case n%10 == 3:
		return "rd"
	}
	return "th"
}

// itemRef is one parsed cross-reference, kept with enough provenance that a
// failure names the exact file and line a maintainer has to open.
type itemRef struct {
	file string
	line int
	text string
	num  int
}

// refMarkerRe strips the leading comment marker off a line before it is joined
// to its predecessor. `//` covers Go, `#` YAML, `*` a C-style block comment's
// continuation. It is deliberately NOT extended to `-`: a markdown bullet starts
// a new list item rather than continuing a wrapped sentence, and stripping it
// would only make the merged-list-items residual noisier.
var refMarkerRe = regexp.MustCompile(`^\s*(?://+|#+|\*)\s*`)

// collapsedPara is one paragraph — a maximal run of non-blank lines — flattened
// to a single whitespace-collapsed string, with enough provenance to map a match
// back to the source line a maintainer has to open. Without that map the fix
// would trade a blind spot for an unactionable failure message.
type collapsedPara struct {
	text     string
	tokStart []int // byte offset in text where each token begins
	tokLine  []int // 1-based source line each token came from
}

// lineAt maps a byte offset in the collapsed text back to a source line: the
// line of the last token that begins at or before it.
func (p collapsedPara) lineAt(off int) int {
	i := sort.Search(len(p.tokStart), func(i int) bool { return p.tokStart[i] > off }) - 1
	if i < 0 {
		i = 0
	}
	if len(p.tokLine) == 0 {
		return 1
	}
	return p.tokLine[i]
}

// collapseParagraphs is the inverse of a hard wrap. See the 🔴 section of this
// file's doc comment for why a line-wise scan is not an option here.
func collapseParagraphs(src string) []collapsedPara {
	var out []collapsedPara
	var b strings.Builder
	var tokStart, tokLine []int

	flush := func() {
		if b.Len() > 0 {
			out = append(out, collapsedPara{text: b.String(), tokStart: tokStart, tokLine: tokLine})
		}
		b.Reset()
		tokStart, tokLine = nil, nil
	}

	for i, line := range strings.Split(src, "\n") {
		fields := strings.Fields(refMarkerRe.ReplaceAllString(line, ""))
		if len(fields) == 0 {
			// A blank line (or a line that was nothing but a comment marker)
			// ends the paragraph. Joining across one would merge unrelated
			// prose, which is a widening this guard does not need.
			flush()
			continue
		}
		for _, f := range fields {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			tokStart = append(tokStart, b.Len())
			tokLine = append(tokLine, i+1)
			b.WriteString(f)
		}
	}
	flush()
	return out
}

// refsIn returns every item number referenced in one collapsed paragraph,
// attributed to the source line the reference STARTS on. It is the ONE matcher:
// the collector and the wrap corpus both call it, so a narrowing fails the
// corpus by name rather than silently shrinking the set the collector validates.
func refsIn(p collapsedPara) []itemRef {
	var out []itemRef
	for _, loc := range itemRefRe.FindAllStringSubmatchIndex(p.text, -1) {
		whole := p.text[loc[0]:loc[1]]
		cluster := p.text[loc[2]:loc[3]]
		line := p.lineAt(loc[0])
		for _, ns := range numberRe.FindAllString(cluster, -1) {
			n, err := strconv.Atoi(ns)
			if err != nil {
				continue
			}
			out = append(out, itemRef{line: line, text: strings.TrimSpace(whole), num: n})
		}
	}
	return out
}

// collectItemRefs walks the repo from the module root (which `go test` makes the
// working directory for this package) and returns every item number referenced.
func collectItemRefs(t *testing.T) []itemRef {
	t.Helper()
	var refs []itemRef
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skippedDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !scannedExts[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			// An unreadable file is a gap, not a pass: fail rather than
			// silently validating a smaller corpus than the repo contains.
			t.Fatalf("read %s: %v", path, readErr)
		}
		for _, para := range collapseParagraphs(string(b)) {
			for _, r := range refsIn(para) {
				r.file = filepath.ToSlash(path)
				refs = append(refs, r)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repo: %v", err)
	}
	return refs
}

// TestAgentsItemCrossReferencesResolve asserts that every `item N` reference in
// the repo names an item that exists in AGENTS.md.
//
// 🔴 EXISTENCE ONLY. A reference to a valid-but-WRONG item passes. See this
// file's package-level doc comment for what that means for a maintainer; do not
// read a green run here as "the cross-references are correct".
func TestAgentsItemCrossReferencesResolve(t *testing.T) {
	count := parseAgentsItems(t)
	refs := collectItemRefs(t)

	// POSITIVE CONTROL. Without this, a regex matching nothing validates
	// nothing and reports success — the reassuring zero.
	if len(refs) < minItemRefs {
		t.Fatalf("found only %d item references across the repo, want >= %d.\n"+
			"This is a POSITIVE-CONTROL failure, not a reference failure: a scan that finds nothing\n"+
			"validates nothing and would otherwise pass. Check itemRefRe (%s), scannedExts, and that\n"+
			"the walk is rooted at the module root.",
			len(refs), minItemRefs, itemRefRe)
	}

	var bad []string
	for _, r := range refs {
		if r.num < 1 || r.num > count {
			bad = append(bad, fmt.Sprintf("  %s:%d: %q references item %d, but AGENTS.md has items 1..%d",
				r.file, r.line, r.text, r.num, count))
		}
	}
	if len(bad) > 0 {
		t.Fatalf("%d dangling AGENTS.md item cross-reference(s):\n%s\n\n"+
			"AGENTS.md's numbered list is APPEND-ONLY — never renumber it. Either the reference is a typo,\n"+
			"or the item it named was renumbered/removed (which is itself the bug).",
			len(bad), strings.Join(bad, "\n"))
	}
	t.Logf("validated %d item references against AGENTS.md items 1..%d", len(refs), count)
}

// TestItemRefsSurviveLineWraps is the negative control for the paragraph-wise
// scan, and the direct analogue of agents_index_test.go's
// TestIndexClausesSurviveLineWraps — the guard that already existed for the
// index region and did not exist for the repo-wide walk, which is why this
// blind spot outlived its own documentation.
//
// Every fixture is a reference split across a hard wrap in a shape this repo
// actually produces, and every one of them yields an INCOMPLETE answer to a
// per-line scan. Each case carries its own positive control proving that, so a
// future implementation cannot satisfy this test by being lucky about where
// today's references happen to wrap.
func TestItemRefsSurviveLineWraps(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []int
	}{
		{"markdown wrap between the token and the number",
			"every `--json` consumer groups on; and item\n24 covers the ONE transport predicate", []int{24}},
		{"markdown wrap inside a list cluster",
			"Items 18 and\n20 cover the checks telling an author", []int{18, 20}},
		// A range is captured at its endpoints here; filling the interior is
		// agents_index_test.go's expandItemCluster, deliberately not this
		// guard's job (see the "two smaller limits" note above).
		{"markdown wrap inside a range cluster",
			"read path. Items 12–\n17, 19, 21 and 22 cover `civitai generate`", []int{12, 17, 19, 21, 22}},
		{"go doc comment wrap, marker on the continuation line",
			"// bullet (\"Read items 12–17, 19, 21\n//     and 22 before touching it\") is scoped", []int{12, 17, 19, 21, 22}},
		{"go line comment wrap between the token and the number",
			"\t// resolveProjectDir — see AGENTS.md item\n\t// 24 for why the walk lives in one place", []int{24}},
		{"yaml comment wrap",
			"# the ready-ack runtime guard, items\n# 11 and 18", []int{11, 18}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []int
			for _, para := range collapseParagraphs(tc.in) {
				for _, r := range refsIn(para) {
					got = append(got, r.num)
				}
			}
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("paragraph-wise scan of %q\n got: %v\nwant: %v", tc.in, got, tc.want)
			}

			// POSITIVE CONTROL for the control. Prove the same input read line
			// by line really does lose part of the reference, so this case is
			// exercising the hazard it claims to rather than restating the
			// happy path. Without it, a fixture that wraps somewhere harmless
			// would sit here looking like coverage.
			var perLine []int
			for _, line := range strings.Split(tc.in, "\n") {
				for _, m := range itemRefRe.FindAllStringSubmatch(line, -1) {
					for _, ns := range numberRe.FindAllString(m[1], -1) {
						n, _ := strconv.Atoi(ns)
						perLine = append(perLine, n)
					}
				}
			}
			if fmt.Sprint(perLine) == fmt.Sprint(tc.want) {
				t.Errorf("fixture %q is not a wrap hazard: a per-line scan already returns the full answer %v, so this case proves nothing",
					tc.in, tc.want)
			}
		})
	}
}

// TestItemRefsCarryTheLineTheyStartOn keeps the provenance map honest. A
// paragraph-wise scan that reported every reference at the paragraph's first
// line would pass TestItemRefsSurviveLineWraps unchanged while making every
// failure message point at the wrong place — trading a blind spot for an
// unactionable one, which is how a guard gets ignored rather than fixed.
func TestItemRefsCarryTheLineTheyStartOn(t *testing.T) {
	// Line 1 opens the paragraph; the reference begins on line 3 and its
	// cluster finishes on line 4.
	src := "a paragraph that opens here\nand continues without any reference\nbefore naming items 18 and\n20 across a wrap\n"
	paras := collapseParagraphs(src)
	if len(paras) != 1 {
		t.Fatalf("expected 1 paragraph, got %d — the fixture is not exercising the mapper", len(paras))
	}
	refs := refsIn(paras[0])
	if len(refs) != 2 {
		t.Fatalf("expected 2 references, got %d (%v)", len(refs), refs)
	}
	for _, r := range refs {
		if r.line != 3 {
			t.Errorf("reference to item %d attributed to line %d, want 3 (the line the reference STARTS on)", r.num, r.line)
		}
	}
}

// TestAgentsItemRefRegexMatchesTheSpellingsInUse is the NEGATIVE control for
// itemRefRe: it drives the regex over a corpus of forms that MUST match and
// forms that must NOT, so a future narrowing (or widening) of the pattern fails
// loudly instead of silently shrinking the corpus the guard above validates.
//
// Without it, "found >= 40 references" is satisfiable while a whole spelling —
// every range, say — has stopped being seen.
func TestAgentsItemRefRegexMatchesTheSpellingsInUse(t *testing.T) {
	cases := []struct {
		in   string
		want []int
	}{
		// Spellings that appear in the repo today.
		{"// AGENTS.md item 7.", []int{7}},
		{"see AGENTS.md item 19(e)) — so this fake", []int{19}},
		{"the same fabricated-zero class as items 6 and 9, arrived", []int{6, 9}},
		{"Read items 12–17, 19, 21 and 22 before touching it.", []int{12, 17, 19, 21, 22}},
		{"(AGENTS.md items 12-17) — so the", []int{12, 17}},
		{"(AGENTS.md items 17 + 18): the upload", []int{17, 18}},
		{"see [`AGENTS.md`](AGENTS.md) items 13 and 19(c).", []int{13, 19}},
		{"Items 1–3, 10 and 11 are deliberate mirrors", []int{1, 3, 10, 11}},
		{"items 5–9 cover `civitai app metrics`", []int{5, 9}},
		{"missing the item-11 handshake", []int{11}},
		{"per AGENTS item 14's reasoning", []int{14}},
		{"AGENTS.md item 21(f)), not a", []int{21}},

		// Clusters that must STOP where the sentence stops.
		{"item 7. The errors.Is half is the load-bearing one", []int{7}},
		{"plus, since item 21, `modelSubstitutions` — and no", []int{21}},
		{"items 4 and 8 are deliberate *non*-mirrors", []int{4, 8}},

		// Must not match at all.
		{"before the caps, 20,008 files with one 88 MB `.js`", nil},
		{"MAX_AUTO_RETRIES = 2, backoff [2000, 5000]", nil},
		{"18 of them, gated by `isWorkflowAvailable`", nil},
		{"itemize the 5 things", nil},
	}
	for _, tc := range cases {
		var got []int
		for _, m := range itemRefRe.FindAllStringSubmatch(tc.in, -1) {
			for _, ns := range numberRe.FindAllString(m[1], -1) {
				n, _ := strconv.Atoi(ns)
				got = append(got, n)
			}
		}
		if fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Errorf("itemRefRe over %q\n got: %v\nwant: %v", tc.in, got, tc.want)
		}
	}
}
