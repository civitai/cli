package cli_test

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
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
// Achieved size: 25,258 bytes, after wave 4 turned the numbered list into a
// TRIGGER INDEX — twenty-six items reduced to a one-line routing question plus a
// pointer, ten bodies evicted in the process, items 2 and 4 left inline because
// they are smaller than a trigger plus a file read would cost. The item list
// went from 26,571 bytes to 6,109 (−77%) and the file from 45,027 to 25,258
// (−43.9%). The ceiling is 28,758 — 3,500 bytes of headroom.
//
// 🔴 "ITEMS 2 AND 4" IS WAVE-4 HISTORY, NOT THE CURRENT INLINE SET. It was the
// whole set on 2026-08-10 (#317); item 30 arrived inline on 2026-08-16 (#437)
// and the sentence above kept reading present-tense. Do not re-derive today's
// set from this paragraph — ask the file: evictionPlaybook computes it, and
// TestInlineAgentsItemsStayUnderTheBreakEven logs the count.
//
// 🔴 THE HEADROOM IS A BUDGET FOR ORDINARY EDITS, AND THE THREE NUMBERS BEFORE
// IT WERE NOT. 287 bytes, then 300, then 2,500: the first two evaporated on the
// very next commit touching this file (#308 consumed 94% of the 300, leaving 19)
// and the 2,500 was consumed by one ordinary correction (#304, +2,392). A
// ceiling with 19 bytes under it is indistinguishable from a frozen file: it
// converts every routine correction into an eviction project, which is a much
// worse failure than the one the ceiling exists to prevent, because the guard
// stops being a budget and becomes a reason not to fix the docs.
//
// 🔴 3,500 IS DERIVED FROM THE CHURN THAT REMAINS, WHICH IS NOT THE CHURN THAT
// HAPPENED. Every earlier derivation measured WHOLE-FILE deltas — median 3,399
// over the 39 non-zero commits touching AGENTS.md — and that number is now
// irrelevant, because it is dominated by item bodies growing, and item bodies no
// longer live here. #304's +2,392 was an edit to item 28's body; under the
// trigger index the same correction costs this file ZERO bytes.
//
// So the budget is derived from the NON-ITEM region instead — the prose sections
// plus the index preamble, which is what an in-file edit can still touch.
// Measured over the same history, |delta| of that region on the 28 commits that
// moved it: 1, 2, 2, 2, 4, 6, 8, 32, 34, 69, 70, 80, 97, 101, 105, 111, 114,
// 147, 164, 261, 281, 347, 691, 778, 1,127, 1,337, 2,890 and 3,399 — median
// 103, p90 1,127, max 3,399 (#289's Layout-ledger rewrite). 3,500 buys the
// worst prose rewrite this file has ever had, or ~17 new items at one trigger
// block each (162–265 bytes, mean 204), or ~34 median corrections.
//
// 🔴 THE ANTI-RE-INLINING PROPERTY HAS MOVED OUT OF THIS CONSTANT, AND THAT IS
// WHY IT CAN BE A REAL BUDGET. Under the stub regime the ceiling was doing two
// jobs: bounding growth AND making a restored body break CI. It can no longer do
// the second — every item is now small, so any useful headroom absorbs a few of
// them — and it does not need to. MEASURED, restoring item 6's full body over
// its trigger reddens TWO tests and neither is this one:
// TestEvidencePointersAndFilesAreTheSameSet (the evidence file is now an orphan)
// and TestInlineAgentsItemsStayUnderTheBreakEven (an inline item over
// maxInlineItemBytes). Both are structural and byte-free.
//
// 🔴 Say the measured thing, not the tidy one: it does NOT redden
// TestEveryAgentsItemCarriesATrigger, which an earlier revision of this comment
// claimed. That test only judges items that still carry a pointer, so an item
// re-inlined WITH its pointer removed is outside its scope by construction.
// Read a size failure here as "the prose grew", not as "someone re-inlined an
// item" — two different tests say that, by name.
//
// 🔴 SQUEEZING IS STILL NOT AN OPTION. A full lossless compression pass over the
// whole file recovered 586 bytes — 0.86% — most of it from ONE genuine
// cross-item duplication, and mechanically the file held 23 repeated 7-word
// n-grams in 67 kB, every one a deliberate parallel construction. Eviction was
// the lever, and wave 4 has now pulled it as far as it goes: 19,149 of the
// remaining 25,258 bytes are non-item prose, and the twenty-six triggers cost
// 5,308 bytes in total. The next lever is the prose sections themselves, which
// is a different and much more judgement-heavy change.
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
// Do not raise this to make an item fit. Write its trigger and move its body.
const agentsMaxBytes = 28_758

// agentsMaxBytesCeiling bounds agentsMaxBytes itself, so the budget above cannot
// be turned into unlimited slack by editing one number.
//
// 30,600 is ~21% above the achieved size, and it is chosen against a property
// that can be re-derived rather than a round percentage: re-inlining the THREE
// largest bodies wave 4 evicted — items 28, 15 and 8, which were 2,323 + 1,827 +
// 1,809 bytes inline against trigger blocks of 231 + 180 + 167 — costs 5,381
// bytes net and lands the file at 30,639, just above this bound. So
// agentsMaxBytes cannot be raised far enough to undo the bulk of this wave
// without a second, deliberate, reviewable edit to this constant.
//
// Stated honestly rather than hidden: the two largest (3,739 net, landing at
// 28,997) WOULD still fit. That is the cost of headroom big enough to be useful,
// and it is why the byte ceiling is no longer the thing standing between this
// file and a restored body — see the 🔴 note on agentsMaxBytes. The structural
// guards catch a re-inline at one item, not three.
//
// 🔴 IT MOVES DOWN WITH THE ACHIEVED SIZE, OR IT STOPS BOUNDING ANYTHING. Left
// at the 48,600 that was ~14% above the PRE-wave-4 size, it would sit ~92% above
// the achieved one — enough slack to re-inline every body this wave moved out
// and still pass, which is the same "a ceiling nobody bounds" failure one level
// up. Whoever changes the shape of this file again re-derives this from the new
// achieved size in the same commit.
const agentsMaxBytesCeiling = 30_600

// agentsMinBytes is the POSITIVE CONTROL. A truncated, empty or wrongly-located
// AGENTS.md is comfortably under any ceiling, and "0 bytes, well within budget"
// is the reassuring-zero shape: the guard reports success having measured a file
// that no longer contains the content it is guarding.
//
// It was set when the achieved size was 42,635 and is deliberately UNCHANGED at
// 25,258, which makes it tighter than it was — it now catches any truncation of
// more than 21%. That is a real bar rather than a formality, and the cost is
// explicit: a future change that legitimately takes this file below 20,000 (the
// only lever left is the prose sections) has to lower this constant on purpose,
// which is exactly the review that shrinking the file that far deserves. It is
// also the floor baseDocFor applies to a BASE commit's AGENTS.md, all four of
// which are 45 kB or larger.
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

// truncateRunes shortens a display string to n RUNES, never n bytes.
//
// 🔴 THE BYTE VERSION EMITTED INVALID UTF-8, AND IT WAS LIVE. The playbook used
// `title[:72]`, which splits a multi-byte rune whenever one straddles the
// boundary — item 13's trigger is 73 bytes and 71 runes with an em-dash across
// byte 72, so the advice a maintainer reads at the ceiling was mojibake, and a
// harness reading the test output could not decode it at all. The old stubs
// happened to carry no multi-byte character at that offset, which is the only
// reason the defect was latent rather than visible; it went live the moment the
// list changed shape, without anything about the truncation changing.
//
// It is a named function rather than three inline lines so the property can be
// driven over a corpus that ALWAYS contains the hazard —
// TestPlaybookTruncationIsRuneSafe — instead of depending on whichever live
// heading happens to straddle the boundary this week. That dependency was real:
// pinning it against AGENTS.md made rewording item 13's trigger fail a UTF-8
// control, which is a false failure at correct content.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
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
			state = "trigger only (body already split out)"
		}
		fmt.Fprintf(&b, "  item %-2d %7d bytes  %-36s  %s\n", s.num, s.bytes, state, truncateRunes(s.firstL, 72))
		shown++
	}
	// 🔴 THE INLINE SET IS COMPUTED, NOT SPELLED, AND THAT IS A CORRECTION.
	// This line read "Every item but 2 and 4" as a literal from wave 4 (#317,
	// 2026-08-10) until #491 — but item 30 arrived inline six days later in
	// #437, so for months the advice a maintainer read AT THE CEILING named the
	// wrong set with full confidence. A comment is a claim; one that reads as
	// coverage while being wrong is worse than none, because it stops the next
	// person looking. Deriving the set from the same `→ evidence:` predicate
	// agentsItems uses means it cannot drift from the count
	// TestInlineAgentsItemsStayUnderTheBreakEven reports, and
	// TestPlaybookNamesTheLiveInlineSet cross-checks it against that OTHER
	// slicer so a future re-hardcoding fails by name.
	var inline []int
	for _, s := range sizes {
		if !s.split {
			inline = append(inline, s.num)
		}
	}
	sort.Ints(inline)
	subject := "EVERY item is"
	if len(inline) > 0 {
		parts := make([]string, 0, len(inline))
		for _, n := range inline {
			parts = append(parts, strconv.Itoa(n))
		}
		subject = "Every item but " + strings.Join(parts, ", ") + " is"
	}

	fmt.Fprintf(&b, "\n🔴 THE LIST IS A TRIGGER INDEX, SO AN ITEM OVER ~%d BYTES IS ALREADY THE BUG.\n"+
		"%s one routing question plus a pointer. If the ranking above\n"+
		"shows a large item, convert it; if it shows only trigger-sized ones, the growth is in\n"+
		"the PROSE sections and no eviction will fix it — see the note on agentsMaxBytes.\n"+
		"\nTo convert an item:\n"+
		"  1. Move its BODY verbatim to %s/NN-<slug>.md — byte-for-byte, nothing\n"+
		"     summarised and nothing dropped. The measurements, mutation matrices,\n"+
		"     retractions and residuals are the point of the item; the size problem is\n"+
		"     PLACEMENT, not existence.\n"+
		"  2. Leave a TRIGGER in AGENTS.md: `NN. **…?**` — ONE question asking whether the\n"+
		"     reader is about to do what the item governs, naming EVERY situation it\n"+
		"     governs rather than the most obvious one — then a line reading\n"+
		"     `→ evidence: %s/NN-<slug>.md`. It is not a label and not a thesis: derive it\n"+
		"     from the item's own argument, and remember an over-narrow trigger HIDES the\n"+
		"     item with every other guard still green. agents_trigger_test.go pins the shape.\n"+
		"  3. Keep the NUMBER. The list is append-only and is never renumbered — 300+\n"+
		"     cross-references point into it, and agents_xrefs_test.go enforces that.\n"+
		"  4. Add the item to agents_split_preserved_test.go's table with the sha256 of\n"+
		"     its non-blank body lines at the commit you moved it from, so the move is\n"+
		"     proven lossless rather than asserted. That commit goes in the row's own\n"+
		"     `base` field — it is NOT shared with the earlier waves, because a body's\n"+
		"     base is fixed at the moment it was evicted and cannot be re-pointed.\n"+
		"\nDo NOT raise agentsMaxBytes to make this pass. It is currently %d, the file is\n"+
		"%d bytes, and agentsMaxBytesCeiling (%d) bounds how far it may ever be raised.\n",
		maxInlineItemBytes, subject, evidenceDir, evidenceDir, agentsMaxBytes, size, agentsMaxBytesCeiling)
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
		// The playbook has to teach the SHAPE, not just the mechanics: a
		// maintainer who evicts a body and leaves a label behind has moved the
		// bytes and hidden the item.
		//
		// 🔴 THESE ARE PHRASES, NOT THE WORD "TRIGGER", AND THAT IS A MEASURED
		// CORRECTION. The first version asked for the bare token; reverting step
		// 2 to the old stub instruction left it SATISFIED by the unrelated
		// "TRIGGER INDEX" sentence at the top of the same message, so the mutant
		// survived with the advice reverted. A guard answered by a bystander is
		// the shape this repo keeps meeting; assert the instruction, not a word
		// another sentence can spell.
		"Leave a TRIGGER in AGENTS.md",
		"ONE question asking whether",
		"over-narrow trigger HIDES the",
		"agents_trigger_test.go",
	} {
		if !strings.Contains(pb, want) {
			t.Errorf("the eviction playbook no longer mentions %q; a size failure that does not say where the bytes GO gets the ceiling raised instead\n---\n%s", want, pb)
		}
	}

	// Belt-and-braces on the live message. The property is PINNED by
	// TestPlaybookTruncationIsRuneSafe, which supplies the hazard itself; this
	// only says the rendered advice is readable today.
	if !utf8.ValidString(pb) {
		t.Errorf("the eviction playbook is not valid UTF-8 — it is truncating a title mid-rune, and the advice a maintainer reads at the ceiling is mojibake\n---\n%q", pb)
	}
}

// TestPlaybookNamesTheLiveInlineSet is the drift guard for the correction in
// evictionPlaybook's 🔴 note. Computing the set fixes TODAY; this fixes
// tomorrow, by asserting the rendered advice names exactly the inline set the
// file actually has — so re-hardcoding it fails here by name rather than going
// stale in silence, which is what "2 and 4" did for months.
//
// 🔴 IT DELIBERATELY CROSSES SLICERS. agentsItemSizes (what the playbook reads)
// and agentsItems (what agents_trigger_test.go reads to decide which items are
// exempt from the trigger rule) each answer "is this item inline" from their own
// parse of the `→ evidence:` pointer. Asserting one against the other is the
// same seam TestEvictionPlaybookSizesTheLastItemCorrectly pins for item
// BOUNDARIES; a guard built on the playbook's own slicer alone would agree with
// itself whatever that slicer decided.
func TestPlaybookNamesTheLiveInlineSet(t *testing.T) {
	var want []int
	for _, it := range agentsItems(t) {
		if !it.converted {
			want = append(want, it.num)
		}
	}
	// POSITIVE CONTROL. With no inline items the phrase below never renders and
	// the assertion would pass having compared nothing.
	if len(want) == 0 {
		t.Fatalf("CONTROL failure, not a finding: agentsItems reports no inline items, so the assertion below would " +
			"check for a phrase the playbook never emits. Either every item is now converted (in which case retire this " +
			"guard deliberately) or the pointer scan is broken.")
	}
	sort.Ints(want)
	parts := make([]string, 0, len(want))
	for _, n := range want {
		parts = append(parts, strconv.Itoa(n))
	}
	// Anchored at both ends, so a SUPERSET or a stale SUBSET both fail: the
	// phrase pins the whole set, not merely that some inline item is mentioned.
	phrase := "Every item but " + strings.Join(parts, ", ") + " is"

	pb := evictionPlaybook(t, agentsMaxBytes+1)
	if !strings.Contains(pb, phrase) {
		t.Errorf("the eviction playbook does not name the live inline set %v.\nwant the phrase: %q\n\n"+
			"A maintainer reads this message AT THE CEILING, so a wrong set here is advice that reads as coverage while "+
			"naming the wrong items — \"2 and 4\" was right for the six days between wave 4 (#317) and item 30 landing "+
			"inline (#437), and wrong for months after. Compute the set from the `→ evidence:` pointer; do not spell it.\n---\n%s",
			want, phrase, pb)
	}
	t.Logf("the playbook names the live inline set %v, agreeing with agentsItems", want)
}

// TestPlaybookTruncationIsRuneSafe is the negative control for truncateRunes.
//
// 🔴 THE CORPUS CARRIES THE HAZARD RATHER THAN BORROWING IT FROM AGENTS.md, and
// that is a correction, not a preference. The first version asserted the live
// playbook was valid UTF-8 and controlled it by requiring some heading in the
// file to be a byte-slice hazard — which made REWORDING item 13's trigger fail a
// UTF-8 control, a false failure at correct content. Every row here supplies its
// own input and ASSERTS ITS OWN PREMISE: that a byte slice at the same offset
// really would produce invalid UTF-8. Without that premise a row could sit here
// looking like coverage while exercising nothing.
func TestPlaybookTruncationIsRuneSafe(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		n      int
		hazard bool // a byte slice at n would split a rune
	}{
		{"item 13's real shape: 73 bytes, 71 runes",
			"13. **Adding any check, table, enum or default to the generation path —", 72, true},
		{"a 3-byte rune straddling the cut",
			strings.Repeat("a", 71) + "—" + strings.Repeat("b", 20), 72, true},
		{"a 4-byte rune straddling the cut",
			strings.Repeat("a", 70) + "🔴" + strings.Repeat("b", 20), 72, true},
		{"pure ASCII, longer than the cut",
			strings.Repeat("a", 200), 72, false},
		{"shorter than the cut is returned unchanged",
			"3. **Touching `internal/validate/lockfile.go` —", 72, false},
	}
	hazards := 0
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			byteHazard := len(tc.in) > tc.n && !utf8.ValidString(tc.in[:tc.n])
			if byteHazard != tc.hazard {
				t.Fatalf("PREMISE BROKEN: the row claims hazard=%v, but a byte slice at %d yields %s. "+
					"The row is not testing what it says it is.", tc.hazard, tc.n,
					map[bool]string{true: "invalid UTF-8", false: "valid UTF-8"}[byteHazard])
			}
			if byteHazard {
				hazards++
			}
			got := truncateRunes(tc.in, tc.n)
			if !utf8.ValidString(got) {
				t.Errorf("truncateRunes(%q, %d) is not valid UTF-8: %q", tc.in, tc.n, got)
			}
			if r := []rune(tc.in); len(r) <= tc.n {
				if got != tc.in {
					t.Errorf("a string shorter than the cut must be returned unchanged, got %q", got)
				}
			} else if want := string(r[:tc.n]) + "…"; got != want {
				t.Errorf("truncateRunes(%q, %d)\n got: %q\nwant: %q", tc.in, tc.n, got, want)
			}
		})
	}
	// POSITIVE CONTROL for the corpus: at least two rows must be genuine
	// byte-slice hazards, or every assertion above passed on input a byte-slicing
	// implementation would have survived too.
	if hazards < 2 {
		t.Fatalf("CONTROL failure, not a finding: only %d row(s) are byte-slice hazards. "+
			"A byte-slicing implementation would pass this table, so it pins nothing.", hazards)
	}
}
