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
// Achieved size at the split: 63,063 bytes. The ceiling is 68,000 — 4,937 bytes
// (~7.8%) of headroom. That number is chosen against what it has to absorb:
//
//   - A new item written as a STUB runs ~600–700 bytes, so the headroom is
//     roughly seven more items before anyone has to think about this again.
//   - Ordinary prose edits — a retraction paragraph, a re-measured number, a new
//     bullet in a small item — are tens to a few hundred bytes each.
//   - A new item written as a FULL BODY is 8.5–28 kB (the nine that were split
//     spanned exactly that range), so it blows the ceiling immediately and by a
//     wide margin. That is the case this guard exists to catch, and the margin
//     is why the headroom can be generous without the guard going slack.
//
// Do not raise this to make a large item fit. Split the item.
const agentsMaxBytes = 68_000

// agentsMaxBytesCeiling bounds agentsMaxBytes itself. 80,000 is ~28% above the
// achieved size: past that the numbered list is back to being a per-session cost
// large enough that the split bought nothing, so raising agentsMaxBytes beyond
// it is a decision about the whole approach, not a bump.
const agentsMaxBytesCeiling = 80_000

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
		block := strings.Join(lines[h.line:end], "\n")
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
		"     proven lossless rather than asserted.\n"+
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
