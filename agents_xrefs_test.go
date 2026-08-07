package cli_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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
// when this landed; requiring 40 leaves room for deletions without letting a
// wired-to-nothing scan pass.
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
		for i, line := range strings.Split(string(b), "\n") {
			for _, m := range itemRefRe.FindAllStringSubmatch(line, -1) {
				for _, ns := range numberRe.FindAllString(m[1], -1) {
					n, convErr := strconv.Atoi(ns)
					if convErr != nil {
						continue
					}
					refs = append(refs, itemRef{
						file: filepath.ToSlash(path),
						line: i + 1,
						text: strings.TrimSpace(m[0]),
						num:  n,
					})
				}
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
