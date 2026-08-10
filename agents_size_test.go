package cli_test

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// AGENTS.md is imported wholesale by CLAUDE.md (`@AGENTS.md`), so every byte in
// it is paid on EVERY session, by every agent, whether or not the session is
// about the thing the byte documents. Before the evidence split it stood at
// 186,776 bytes — roughly 47k tokens — and ~95% of that was the "Intentional
// decisions that look wrong" list. Nothing measured it, and nothing stopped it
// growing: the two existing guards (agents_index_test.go, agents_xrefs_test.go)
// police cross-references only, so an item could double in size without a single
// test noticing.
//
// This is the missing ceiling. It is modelled on the shape of the operator's own
// `scripts/tests/test_rules_size.py`: a constant with stated headroom, and a
// failure message that PRINTS THE EVICTION PLAYBOOK rather than just the number,
// because a size guard whose only advice is "make it smaller" gets its constant
// raised instead of its content moved.
//
// 🔴 THE ANTI-LOOSENING CAP IS THE OTHER HALF, AND IT IS THIS REPO'S OWN LESSON.
// AGENTS.md item 3 records a 64 MiB cap that was raised to `1<<62` with the
// entire suite still green — a ceiling nobody bounds is a ceiling that can be
// deleted by editing one number, which is indistinguishable from having no
// guard. So agentsMaxBytes is itself bounded by agentsMaxBytesCeiling, and
// TestAgentsSizeGuardCanStillFire asserts the bound. Raising the ceiling past
// that is a deliberate, reviewable edit to a second constant that says in its
// own comment what it costs.

// agentsMaxBytes is the ceiling on AGENTS.md.
//
// Achieved size: 42,635 bytes, after wave 3 of the evidence split moved items 9,
// 13, 17, 22 and 25 out (11,596 bytes net, 21.4% of the file, in one change).
// The ceiling is 45,135 — 2,500 bytes of headroom.
//
// 🔴 THE HEADROOM IS A BUDGET FOR ORDINARY EDITS, AND THE TWO NUMBERS BEFORE IT
// WERE NOT. 287 bytes, then 300: each evaporated on the very next commit that
// touched this file (#308 consumed 94% of the 300, leaving 19), and a ceiling
// with 19 bytes under it is indistinguishable from a frozen file. It converts
// every routine correction into an eviction project, which is a much worse
// failure than the one the ceiling exists to prevent — the guard stops being a
// budget and becomes a reason not to fix the docs.
//
// 2,500 is DERIVED, not picked. Measured over the last 40 commits touching
// AGENTS.md, the ten docs-only ones changed it by +70, +245, +281, +1,337,
// +1,456, +2,371, +2,760, +2,890, +3,399 and +3,474 bytes — median 1,913. So
// 2,500 buys roughly ONE median docs-only correction, or two or three small
// ones, or one retraction paragraph (300–800 bytes, measured on the ones in the
// file) plus change.
//
// What it deliberately does NOT buy:
//
//   - A new item written as a full body. The modern ones run 1.5–3.7 kB inline
//     and the largest evicted bodies were 8.5–28 kB, so a new decision still has
//     to be paid for by MOVING one out, which is the whole point.
//   - Re-inlining this wave's work. Restoring item 9, 13 or 22 costs 2,284–3,035
//     bytes net and breaks this ceiling on its own. (Items 17 and 25 are the two
//     cheapest at 1,868 and 1,857, and would fit — that is the honest cost of a
//     headroom big enough to be useful, and it is what agentsMaxBytesCeiling is
//     sized to bound.)
//
// 🔴 SQUEEZING IS STILL NOT AN OPTION, which is why the budget has to be real
// rather than aspirational. A full lossless compression pass over every unsplit
// item and every prose section recovered 586 bytes — 0.86% — most of it from ONE
// genuine cross-item duplication (item 19(e) restated item 17's credential
// argument in full). Mechanically the file held only 23 repeated 7-word n-grams
// in 67 kB, every one a deliberate parallel construction rather than accidental
// redundancy. EVICTION is the only lever with anything left in it: wave 2
// recovered ~23x what the compression pass did and wave 3 ~20x.
//
// 🔴 THE CANDIDATE LIST THAT USED TO SIT HERE IS DELETED RATHER THAN UPDATED,
// AND THAT IS THE LESSON. It named items 10 and 19 with their byte sizes and
// asserted "both are byte-identical to agentsSplitBase, so their digests can be
// taken straight from `git show <base>:AGENTS.md`". By the time anyone read it
// that was FALSE — #297's compression pass had rewritten both, 22 bytes and 77
// bytes respectively — so the shortcut it offered would have pinned text that
// was not what was being moved. A hand-maintained ranking in a comment goes
// stale silently while reading as current; evictionPlaybook() computes the same
// ranking from the live file at the moment of failure, which is the copy that
// cannot rot. Do not reintroduce one here.
//
// Do not raise this to make a large item fit. Split the item.
const agentsMaxBytes = 45_135

// agentsMaxBytesCeiling bounds agentsMaxBytes itself, so the budget above cannot
// be turned into unlimited slack by editing one number.
//
// 48,600 is ~14% above the achieved size, and it is chosen against a property
// that can be re-derived rather than a round percentage: re-inlining any THREE
// of wave 3's five bodies costs at least 6,009 bytes net, landing at 48,644 —
// just above this bound. So agentsMaxBytes cannot be raised far enough to undo
// the majority of this wave without a second, deliberate, reviewable edit to
// this constant. (Any two of them, at 3,725–5,587 net, would still fit; a bound
// tight enough to block a pair would sit 1,165 bytes above agentsMaxBytes and
// leave the next wave no room to re-derive anything.)
//
// 🔴 IT MOVES DOWN WITH THE ACHIEVED SIZE, OR IT STOPS BOUNDING ANYTHING. Left
// at the 64,000 that was ~19% above the PRE-wave-3 size, it would sit ~50% above
// the achieved one — enough slack to re-inline all five items just moved out
// (54,231) and still pass, which is the same "a ceiling nobody bounds" failure
// one level up. Whoever completes the next wave re-derives this from the new
// achieved size in the same commit.
const agentsMaxBytesCeiling = 48_600

// agentsMinBytes is the POSITIVE CONTROL. A truncated, empty or wrongly-located
// AGENTS.md is comfortably under any ceiling, and "0 bytes, well within budget"
// is the reassuring-zero shape: the guard reports success having measured a file
// that no longer contains the content it is guarding. 20,000 is far below the
// achieved size and far above anything a truncation would leave.
const agentsMinBytes = 20_000

// evidencePathRe recognises an item that has ALREADY been evicted, so the
// playbook does not tell a maintainer to split something that is already split.
var evidencePathRe = regexp.MustCompile(`→ evidence: \S+`)

// agentsItemSizes returns each item's byte size in AGENTS.md, largest first,
// together with whether it already has an evidence pointer.
//
// 🔴 THE LAST ITEM ENDS AT THE LIST, NOT AT THE END OF THE FILE, AND FORGETTING
// THAT MADE THE PLAYBOOK RANK A STUB FIRST. There is no item heading after the
// final one, so a naive slice runs to EOF and swallows the closing
// vendored-mirrors paragraph plus the Permission boundaries, Release process and
// License sections. Measured on the wave-3 tree: item 27 is a 683-byte STUB and
// was reported at 4,789 bytes — 7x its real size, and top of the ranking, so the
// first advice a maintainer got at the ceiling was to evict something already
// evicted. The cut is the same one baseItemBody in agents_split_preserved_test.go
// already applies: stop at the first line that leaves the list's indentation.
// TestEvictionPlaybookSizesTheLastItemCorrectly pins it.
func agentsItemSizes(t *testing.T) []struct {
	num    int
	bytes  int
	split  bool
	firstL string
} {
	t.Helper()
	b, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	body := string(b)
	i := strings.Index(body, itemsSectionHeading)
	if i < 0 {
		t.Fatalf("CONTROL failure: AGENTS.md has no %q heading, so no item sizes could be measured", itemsSectionHeading)
	}
	lines := strings.Split(body, "\n")
	type head struct {
		num, line int
	}
	var heads []head
	for n, line := range lines {
		if m := itemHeadingRe.FindStringSubmatch(line); m != nil {
			num, _ := strconv.Atoi(m[1])
			heads = append(heads, head{num, n})
		}
	}
	var out []struct {
		num    int
		bytes  int
		split  bool
		firstL string
	}
	for idx, h := range heads {
		end := len(lines)
		if idx+1 < len(heads) {
			end = heads[idx+1].line
		}
		item := lines[h.line:end]
		// See the 🔴 note above: the final item has no following heading, so it
		// is bounded by the first line that leaves the list's indentation.
		for i, l := range item {
			if i > 0 && l != "" && !strings.HasPrefix(l, " ") {
				item = item[:i]
				break
			}
		}
		for len(item) > 0 && strings.TrimSpace(item[len(item)-1]) == "" {
			item = item[:len(item)-1]
		}
		block := strings.Join(item, "\n")
		out = append(out, struct {
			num    int
			bytes  int
			split  bool
			firstL string
		}{h.num, len(block), evidencePathRe.MatchString(block), strings.TrimSpace(lines[h.line])})
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].bytes > out[b].bytes })
	return out
}

// evictionPlaybook renders the advice a size failure has to carry: which items
// are largest, which are already split, and the mechanical steps to move one.
func evictionPlaybook(t *testing.T, size int) string {
	sizes := agentsItemSizes(t)
	var b strings.Builder
	fmt.Fprintf(&b, "\nEVICTION PLAYBOOK — the largest items in the numbered list right now:\n")
	shown := 0
	for _, s := range sizes {
		if shown == 8 {
			break
		}
		state := "IN AGENTS.md"
		if s.split {
			state = "already split (stub only)"
		}
		title := s.firstL
		if len(title) > 72 {
			title = title[:72] + "…"
		}
		fmt.Fprintf(&b, "  item %-2d %7d bytes  %-25s  %s\n", s.num, s.bytes, state, title)
		shown++
	}
	fmt.Fprintf(&b, "\nTo evict an item:\n"+
		"  1. Move its BODY verbatim to %s/NN-<slug>.md — byte-for-byte, nothing\n"+
		"     summarised and nothing dropped. The measurements, mutation matrices,\n"+
		"     retractions and residuals are the point of the item; the size problem is\n"+
		"     PLACEMENT, not existence.\n"+
		"  2. Leave a 4–8 line stub in AGENTS.md: the `NN. **…**` heading with its thesis\n"+
		"     sentence (lift it from the item's own opening), then a final line reading\n"+
		"     `→ evidence: %s/NN-<slug>.md`.\n"+
		"  3. Keep the NUMBER. The list is append-only and is never renumbered — 300+\n"+
		"     cross-references point into it, and agents_xrefs_test.go enforces that.\n"+
		"  4. Add the item to agents_split_preserved_test.go's table with the sha256 of\n"+
		"     its non-blank body lines at the commit you moved it from, so the move is\n"+
		"     proven lossless rather than asserted. That commit goes in the row's own\n"+
		"     `base` field — it is NOT shared with the earlier waves, because a body's\n"+
		"     base is fixed at the moment it was evicted and cannot be re-pointed.\n"+
		"\nDo NOT raise agentsMaxBytes to make this pass. It is currently %d, the file is\n"+
		"%d bytes, and agentsMaxBytesCeiling (%d) bounds how far it may ever be raised.\n",
		evidenceDir, evidenceDir, agentsMaxBytes, size, agentsMaxBytesCeiling)
	return b.String()
}

// TestAgentsMDStaysUnderItsCeiling is the guard AGENTS.md did not have.
func TestAgentsMDStaysUnderItsCeiling(t *testing.T) {
	fi, err := os.Stat("AGENTS.md")
	if err != nil {
		t.Fatalf("stat AGENTS.md: %v (CONTROL failure — the file this guard measures was not found)", err)
	}
	size := int(fi.Size())

	// POSITIVE CONTROL first. An empty or truncated file passes any ceiling, and
	// reporting that as "within budget" is a claim about a file that is no longer
	// there.
	if size < agentsMinBytes {
		t.Fatalf("CONTROL failure, not a finding: AGENTS.md is %d bytes, below the %d-byte floor.\n"+
			"A file this small is not a lean AGENTS.md, it is a truncated or wrong one — and it would pass the ceiling check below "+
			"while guarding nothing. Check the file before reading this as good news.", size, agentsMinBytes)
	}

	if size > agentsMaxBytes {
		t.Fatalf("AGENTS.md is %d bytes, over the %d-byte ceiling by %d.\n"+
			"Every byte here is loaded into EVERY session through CLAUDE.md's `@AGENTS.md` import, whether or not the session is about "+
			"the thing the byte documents.%s",
			size, agentsMaxBytes, size-agentsMaxBytes, evictionPlaybook(t, size))
	}
	t.Logf("AGENTS.md is %d bytes, %d under the %d-byte ceiling", size, agentsMaxBytes-size, agentsMaxBytes)
}

// TestEvictionPlaybookSizesTheLastItemCorrectly pins the fix described in
// agentsItemSizes' 🔴 note, and pins it as a SEAM rather than as a number.
//
// Two slicers in this package answer "where does item N end": agentsItemSizes
// (which decides what the ceiling failure tells a maintainer to evict) and
// baseItemBody (which decides what the verbatim digest covers). They had
// diverged on exactly one input — the final item, which has no following heading
// — and the divergence was invisible because each looked right on its own. So
// the assertion is that they AGREE, for every item, which is the property that
// was actually broken. A hardcoded size for whichever item is currently last
// would go stale the day the next one is appended — and could not even be
// written here, since the repo-wide xrefs guard reads this file and would flag
// the number as a dangling reference.
func TestEvictionPlaybookSizesTheLastItemCorrectly(t *testing.T) {
	b, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	doc := string(b)
	sizes := agentsItemSizes(t)
	if len(sizes) < minAgentsItems {
		t.Fatalf("CONTROL failure: agentsItemSizes returned %d item(s), want >= %d — it is not seeing the list",
			len(sizes), minAgentsItems)
	}

	last := 0
	for _, s := range sizes {
		if s.num > last {
			last = s.num
		}
	}
	for _, s := range sizes {
		want := len(strings.Join(baseItemBody(t, doc, s.num), "\n"))
		if s.bytes != want {
			t.Errorf("item %d: the playbook measures %d bytes, baseItemBody slices %d (delta %+d).\n"+
				"The two slicers disagree about where this item ends. Whichever is wrong, the cost is real: the playbook's number "+
				"is the ranking a maintainer evicts by, and baseItemBody's slice is what the verbatim digest covers.",
				s.num, s.bytes, want, s.bytes-want)
		}
	}

	// POSITIVE CONTROL. The agreement above is only evidence if the last item is
	// genuinely the hazard case — i.e. if a naive to-EOF slice really would
	// swallow the sections below the list. Without this, a document whose final
	// item happened to sit at EOF would satisfy the loop while proving nothing.
	lines := strings.Split(doc, "\n")
	start := -1
	for i, l := range lines {
		if m := itemHeadingRe.FindStringSubmatch(l); m != nil {
			if n, _ := strconv.Atoi(m[1]); n == last {
				start = i
			}
		}
	}
	if start < 0 {
		t.Fatalf("CONTROL failure: item %d's heading was not found", last)
	}
	naive := strings.Join(lines[start:], "\n")
	if !strings.Contains(naive, "\n## ") {
		t.Fatalf("CONTROL failure, not a finding: a naive to-EOF slice of the last item (%d) contains no `## ` heading, "+
			"so this test is not exercising the over-count it exists to pin. AGENTS.md's structure changed; re-derive the control.", last)
	}
	for _, s := range sizes {
		if s.num != last {
			continue
		}
		if strings.Contains(naive[:s.bytes], "\n## ") {
			t.Errorf("item %d is measured at %d bytes, which still reaches a `## ` section heading — the playbook is counting "+
				"the document's trailing sections as part of the last item and will rank it first whatever its real size", last, s.bytes)
		}
		t.Logf("last item %d measures %d bytes; the naive to-EOF slice would have measured %d", last, s.bytes, len(naive))
	}
}

// TestAgentsSizeGuardCanStillFire is the "can it go red" control for the
// constant itself.
//
// 🔴 A ceiling that nobody bounds is one edit away from being deleted, and the
// edit reads as a chore. AGENTS.md item 3 records exactly that outcome on a
// different constant: a 64 MiB cap raised to `1<<62` left the whole suite green.
// So the ceiling is bounded, and the bound is asserted here rather than left as
// a comment nobody has to obey.
func TestAgentsSizeGuardCanStillFire(t *testing.T) {
	if agentsMaxBytes > agentsMaxBytesCeiling {
		t.Fatalf("agentsMaxBytes is %d, above agentsMaxBytesCeiling (%d).\n"+
			"At that size the numbered list is back to being the per-session cost the evidence split removed, so the guard is no longer "+
			"guarding anything. If the ceiling genuinely has to move, move BOTH constants deliberately and say in agentsMaxBytesCeiling's "+
			"comment what the new number costs — do not raise one to silence a failure.",
			agentsMaxBytes, agentsMaxBytesCeiling)
	}
	if agentsMinBytes >= agentsMaxBytes {
		t.Fatalf("agentsMinBytes (%d) is not below agentsMaxBytes (%d) — the guard's band is empty and it cannot report anything meaningful",
			agentsMinBytes, agentsMaxBytes)
	}

	// The playbook is part of the contract, not decoration: a size failure that
	// only prints a number gets the number raised. Render it against the live
	// file and require it to name the mechanics, so a refactor that guts the
	// message fails here instead of at the next person to hit the ceiling.
	pb := evictionPlaybook(t, agentsMaxBytes+1)
	for _, want := range []string{
		"EVICTION PLAYBOOK",
		evidenceDir,
		"→ evidence:",
		"append-only",
		"Do NOT raise agentsMaxBytes",
		"item ",
	} {
		if !strings.Contains(pb, want) {
			t.Errorf("the eviction playbook no longer mentions %q; a size failure that does not say where the bytes GO gets the ceiling raised instead\n---\n%s", want, pb)
		}
	}
}
