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
)

// AGENTS.md's numbered list is loaded into every session through CLAUDE.md's
// `@AGENTS.md` import, so its size is a per-session cost. The nine largest items
// keep their THESIS in AGENTS.md and have their bodies — the RSS tables, the
// mutation matrices, the retractions, the enumerated residuals — in
// `claudedocs/decisions/NN-<slug>.md`, reached through a `→ evidence: <path>`
// pointer. That evidence costs nothing until someone opens the code it is about.
//
// A split like that creates exactly one new failure mode, and it is silent: the
// pointer and the file drift apart. This file is the ledger that closes it.
//
// 🔴 IT MUST FAIL IN BOTH DIRECTIONS, AND ONE DIRECTION IS THE EASY ONE TO MISS.
// A guard that only checks "every pointer resolves" is satisfied by DELETING an
// evidence file together with its pointer — the map stays internally consistent
// while the content is gone, which is precisely the loss this whole change is
// supposed to be impossible. So the assertion is set EQUALITY: every pointer
// names a file that exists, AND every file in the directory is named by exactly
// one pointer. Growth fails (an orphan file nobody can reach from AGENTS.md),
// shrinkage fails (a dangling pointer), and a duplicate fails (two items
// claiming one body). The idiom is the repo's own —
// TestEveryValidateDirCallerGatesOnResolveProjectDir (item 26) and
// neterr_ledger_test.go (item 24) are the same shape, for the same reason.
//
// 🔴 WHAT THIS GUARD DOES NOT DO. It does not read the evidence. A pointer at a
// file whose body documents a different item passes byte-for-byte, exactly as
// the sibling xrefs guard passes a reference to a valid-but-wrong item. What
// pins the CONTENT is agents_split_preserved_test.go, which digests each body
// against the text it was moved from. Neither subsumes the other: this one sees
// a file that vanished, that one sees a file that was quietly edited.

// evidenceDir is the one place a split item's body may live. It is a constant
// rather than a glob over `claudedocs/` because `claudedocs/` also holds handoff
// docs, which are not evidence files and must not be swept into the ledger.
const evidenceDir = "claudedocs/decisions"

// minEvidencePointers is the POSITIVE CONTROL. Without it, a pointer regex that
// has stopped matching — a changed arrow glyph, a reflowed line, a wrong working
// directory — finds zero pointers, compares the empty set with an empty
// expectation and reports a serene pass while the ledger checks nothing. Nine
// items are split today; 5 leaves room for re-inlining a couple without letting
// a wired-to-nothing scan through.
const minEvidencePointers = 5

// evidencePointerRe matches the pointer line a stub ends with. The path is
// captured as a non-space run, so a pointer that has been wrapped across a line
// break (which would break the link for a human reader too) is not silently
// half-matched into something that resolves.
var evidencePointerRe = regexp.MustCompile(`→ evidence: (\S+)`)

// evidenceFilePrefixRe pulls the item number back off a file NAME. The number in
// the filename is the third independent copy of the item's identity (the stub's
// heading, the pointer path, the file name), and it is checked against the first
// so a file cannot be pointed at from the wrong item.
var evidenceFilePrefixRe = regexp.MustCompile(`^([0-9]+)-`)

type evidencePointer struct {
	item int    // the AGENTS.md item whose stub carries the pointer
	line int    // 1-based line in AGENTS.md, so a failure names where to look
	path string // exactly as written in AGENTS.md
}

// collectEvidencePointers walks AGENTS.md and returns every `→ evidence:`
// pointer with the item it belongs to. The item is tracked from the same
// `^N. **` heading anchor the sibling guards use, so a pointer that has floated
// out of the numbered list (into the preamble, or below the closing paragraph)
// carries the zero sentinel and fails loudly rather than being attributed to
// whatever heading happened to precede it.
func collectEvidencePointers(t *testing.T) []evidencePointer {
	t.Helper()
	b, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatalf("read AGENTS.md: %v (CONTROL failure — nothing below this line checked anything)", err)
	}
	var out []evidencePointer
	cur := 0
	for i, line := range strings.Split(string(b), "\n") {
		if m := itemHeadingRe.FindStringSubmatch(line); m != nil {
			cur, _ = strconv.Atoi(m[1])
		}
		for _, m := range evidencePointerRe.FindAllStringSubmatch(line, -1) {
			out = append(out, evidencePointer{item: cur, line: i + 1, path: m[1]})
		}
	}
	return out
}

// evidenceFilesOnDisk lists the evidence directory. A missing directory is a
// CONTROL failure, not an empty set: "there are no evidence files" and "the
// ledger is looking in the wrong place" produce the same empty slice, and only
// one of them is a finding.
func evidenceFilesOnDisk(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(evidenceDir)
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot read %s: %v.\n"+
			"Every pointer below would be reported dangling by a guard that had read nothing. "+
			"Fix the path (or restore the directory) before reading any result here.", evidenceDir, err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		out = append(out, filepath.ToSlash(filepath.Join(evidenceDir, e.Name())))
	}
	sort.Strings(out)
	return out
}

// TestEvidencePointersAndFilesAreTheSameSet is the bidirectional ledger.
//
// It fails when the set GROWS (an evidence file nothing points at — unreachable
// from AGENTS.md, so invisible to the reader the split is for) and when it
// SHRINKS (a pointer whose file is gone — the body was deleted, which is the
// loss this change exists to make impossible).
func TestEvidencePointersAndFilesAreTheSameSet(t *testing.T) {
	pointers := collectEvidencePointers(t)
	if len(pointers) < minEvidencePointers {
		t.Fatalf("CONTROL failure, not a finding: found %d `→ evidence:` pointer(s) in AGENTS.md, want >= %d.\n"+
			"A scan that finds nothing validates nothing and would otherwise pass. Check evidencePointerRe (%s) "+
			"and that this test's working directory is the module root — do NOT delete evidence files on the strength of this failure.",
			len(pointers), minEvidencePointers, evidencePointerRe)
	}

	files := evidenceFilesOnDisk(t)
	if len(files) == 0 {
		t.Fatalf("CONTROL failure, not a finding: %s holds no .md files at all, so every one of the %d pointer(s) is dangling. "+
			"That is either a wholesale deletion or a wrong path; check which before editing AGENTS.md.", evidenceDir, len(pointers))
	}

	claimedBy := map[string][]evidencePointer{}
	for _, p := range pointers {
		claimedBy[p.path] = append(claimedBy[p.path], p)
	}
	onDisk := map[string]bool{}
	for _, f := range files {
		onDisk[f] = true
	}

	var problems []string

	// Direction 1 — every pointer resolves, is in the right directory, and the
	// number in the filename is the item that points at it.
	for _, p := range pointers {
		if p.item == 0 {
			problems = append(problems, fmt.Sprintf(
				"  AGENTS.md:%d: pointer %q sits outside the numbered list (no `N. **` heading above it)", p.line, p.path))
			continue
		}
		if !strings.HasPrefix(p.path, evidenceDir+"/") {
			problems = append(problems, fmt.Sprintf(
				"  AGENTS.md:%d: item %d points at %q, which is outside %s — evidence lives in one place so the ledger can enumerate it",
				p.line, p.item, p.path, evidenceDir))
			continue
		}
		if !onDisk[p.path] {
			problems = append(problems, fmt.Sprintf(
				"  AGENTS.md:%d: item %d points at %q, which does not exist. The body was deleted or the file was renamed; "+
					"restore it — the stub in AGENTS.md is deliberately not the whole item", p.line, p.item, p.path))
			continue
		}
		name := filepath.Base(p.path)
		m := evidenceFilePrefixRe.FindStringSubmatch(name)
		if m == nil {
			problems = append(problems, fmt.Sprintf(
				"  AGENTS.md:%d: item %d points at %q, whose name does not begin with its item number (`NN-<slug>.md`)",
				p.line, p.item, p.path))
			continue
		}
		n, _ := strconv.Atoi(m[1])
		if n != p.item {
			problems = append(problems, fmt.Sprintf(
				"  AGENTS.md:%d: item %d points at %q, which is numbered %d. The list is APPEND-ONLY and never renumbered, "+
					"so a mismatch here means the pointer, not the file, is wrong", p.line, p.item, p.path, n))
		}
	}

	// Direction 2 — every file on disk is claimed by exactly one item.
	for _, f := range files {
		switch n := len(claimedBy[f]); {
		case n == 0:
			problems = append(problems, fmt.Sprintf(
				"  %s exists but no AGENTS.md item points at it. It is unreachable from the file a reader actually loads, "+
					"which is the same as not existing — add the `→ evidence:` line to its stub (or delete the file deliberately)", f))
		case n > 1:
			var where []string
			for _, p := range claimedBy[f] {
				where = append(where, fmt.Sprintf("item %d (AGENTS.md:%d)", p.item, p.line))
			}
			problems = append(problems, fmt.Sprintf(
				"  %s is claimed by %d items — %s. One body, one owner: two items pointing at one file means one of them has lost its evidence",
				f, n, strings.Join(where, ", ")))
		}
	}

	if len(problems) > 0 {
		t.Fatalf("%d evidence-ledger problem(s):\n%s\n\n"+
			"The ledger is set EQUALITY between the `→ evidence:` pointers in AGENTS.md and the files in %s. "+
			"It fails when the set grows and when it shrinks, because a guard that only caught growth would let a silent deletion "+
			"turn the ledger into a false map.",
			len(problems), strings.Join(problems, "\n"), evidenceDir)
	}
	t.Logf("evidence ledger: %d pointer(s) and %d file(s) agree", len(pointers), len(files))
}

// TestEvidenceStubsCarryAThesisAndAPointer asserts the shape of a stub: the
// pointer must not be the whole of what AGENTS.md says about a split item.
//
// This is the guard against the failure mode the split most invites — replacing
// an item with a bare "see the evidence file". A reader deciding whether an item
// bears on the change they are making must be able to decide it from AGENTS.md;
// a pointer alone forces nine file reads per session, which is worse than the
// cost the split removed.
func TestEvidenceStubsCarryAThesisAndAPointer(t *testing.T) {
	b, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	lines := strings.Split(string(b), "\n")

	// The minimum is deliberately low: it is a floor against a DEGENERATE stub,
	// not a style rule. A thesis sentence hard-wrapped at ~79 columns runs to
	// several lines, so 120 characters of prose before the pointer is a bar an
	// honest stub clears without trying and a bare pointer cannot clear at all.
	const minThesisChars = 120

	starts := map[int]int{}
	for i, line := range lines {
		if m := itemHeadingRe.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			starts[n] = i
		}
	}

	pointers := collectEvidencePointers(t)
	if len(pointers) < minEvidencePointers {
		t.Fatalf("CONTROL failure, not a finding: %d pointer(s) found, want >= %d — see TestEvidencePointersAndFilesAreTheSameSet",
			len(pointers), minEvidencePointers)
	}

	var thin []string
	for _, p := range pointers {
		s, ok := starts[p.item]
		if !ok {
			continue // reported by the ledger test
		}
		body := strings.Join(lines[s:p.line-1], " ")
		if n := len(strings.TrimSpace(body)); n < minThesisChars {
			thin = append(thin, fmt.Sprintf(
				"  item %d (AGENTS.md:%d) carries only %d character(s) of prose before its pointer, want >= %d",
				p.item, s+1, n, minThesisChars))
		}
	}
	if len(thin) > 0 {
		t.Fatalf("%d stub(s) that are a pointer and little else:\n%s\n\n"+
			"A stub has to carry the item's THESIS — enough for a reader to know the item exists and whether it bears on what they are "+
			"about to change. The evidence file carries the measurements. A bare pointer moves the cost instead of removing it.",
			len(thin), strings.Join(thin, "\n"))
	}
}
