package cli_test

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// AGENTS.md's numbered list is a TRIGGER INDEX. Each item is one line asking
// whether the reader is about to do the thing that item governs, plus a pointer
// at the file holding the item itself. That shape replaced the multi-line STUBS
// waves 1–3 of the evidence split (#290, #305, #310) left behind, and it creates
// a failure mode this repo has not had before — which is what this file guards.
//
// # 🔴 A TRIGGER IS A NEW CLAIM, AND A WRONG ONE SILENTLY HIDES ITS ITEM
//
// A stub was a compressed CONCLUSION: it restated what the item decided, so a
// reader who skipped the evidence still carried away something true. A trigger
// is a claim about the reader's SITUATION — "you are about to touch X" — and it
// is the only thing routing anyone to the item at all. Get it too narrow and the
// item is not reached by the person it was written for; the file is still there,
// the ledger still balances, the digest still matches, and nothing fails. The
// item has been hidden rather than lost, which is the failure that looks most
// like success.
//
// Nothing automated can tell you a trigger is TRUE of its item. That is the same
// limitation agents_xrefs_test.go states for a reference to a valid-but-wrong
// item and agents_index_test.go states for a clause that misdescribes the items
// it names: the SHAPE is checkable, the CLAIM is not. Reading a trigger against
// the item it points at is a manual audit, and an over-narrow trigger is the
// expensive direction — so when an item governs more than one situation, name
// them all.
//
// # WHAT IS CHECKABLE, AND WHY EACH RULE IS HERE
//
//  1. EVERY item has a trigger. Not just the split ones — an item with no
//     routing line is unreachable by the only navigation the list now has.
//
//  2. A TRIGGER ENDS IN `?`. This is the rule that separates a routing signal
//     from a label, and it is structural rather than stylistic: a question is
//     about the reader, a statement is about the subject. The brief's own
//     worked example of the failure —
//     "17. **`DownloadPresigned` exists so a blob fetch carries no credential.**"
//     — is a perfectly good sentence and a useless trigger, and it is caught
//     here by its full stop. It is not sufficient on its own (a label with a
//     question mark bolted on passes), which is why rules 3–5 exist; no one of
//     them subsumes the others.
//
//  3. A LENGTH BAND. The floor catches a blanked or one-word trigger. The
//     ceiling is the more interesting half: without it the list grows a body
//     back one clause at a time, and every individual edit looks reasonable.
//
//  4. A DEGENERATE-PHRASE DENYLIST — "see the evidence file", "TBD", a bare
//     restatement of the item number. These pass rules 2 and 3 while carrying no
//     routing information at all.
//
//  5. NOT THE EVIDENCE FILE'S OWN TITLE. A title is a label by construction, so
//     a trigger that normalises to its file's H1 is a restatement wearing a
//     question mark.
//
//  6. A CONVERTED ITEM IS THE TRIGGER AND THE POINTER, NOTHING ELSE. Any other
//     line in the block is a body creeping back inline.
//
//  7. AN INLINE ITEM STAYS UNDER THE BREAK-EVEN. Two items (2 and 4) are stated
//     in full because a trigger plus a file read costs more than they do; see
//     maxInlineItemBytes. Without this rule the next item is written inline
//     "just this once" and the index silently reverts to a list of bodies.
//
// # 🔴 WHAT THIS FILE SUPERSEDES, AND THE PROPERTY THAT MOVED
//
// It replaces TestEvidenceStubsCarryAThesisAndAPointer, which required 120
// characters of prose before a pointer so that an item could not decay into a
// bare "see the evidence file". That guard was right about the hazard and is
// incompatible with the fix: a trigger is deliberately SHORTER than its floor,
// because the routing information a reader needs is not a compressed thesis. The
// hazard it named is covered here by rules 1, 4 and 5 — a bare pointer now fails
// for having no trigger at all, rather than for being short.
//
// The other property that moved here is ANTI-RE-INLINING. Under the stub regime
// that was the byte ceiling's job: agentsMaxBytes was sized so that restoring a
// large body broke it. Under the trigger index it is structural and byte-free —
// restoring item 6's body means deleting its trigger, which fails rule 1 and
// rule 6 here and orphans an evidence file in agents_evidence_test.go, whatever
// the file's size. That is why agentsMaxBytes can now be a genuine budget for
// ordinary prose edits instead of a second lock.

// minTriggerChars / maxTriggerChars bound the trigger text (the bolded question,
// whitespace-collapsed, excluding the `N. ` numbering and the `**` markers).
//
// Measured over the twenty-six triggers in the file: min 77 (item 7), mean 120,
// max 169 (item 13).
//
// The floor catches the degenerate shapes rule 4 does not enumerate — a blank, a
// single word, a bare "Why?". At 40 it sits well under the shortest real trigger,
// so it is a floor an honest one clears without being written to clear it.
//
// The ceiling is what stops the list growing bodies back. 220 leaves the longest
// trigger 51 characters of room — enough for an item that genuinely governs four
// situations, not enough for a paragraph. 🔴 Raising it is not a formatting
// decision; it is a decision to let the per-session cost grow, which is the whole
// thing the index exists to bound.
const (
	minTriggerChars = 40
	maxTriggerChars = 220
)

// maxInlineItemBytes is the break-even, expressed as a rule rather than as a
// judgement made once.
//
// A trigger block (the wrapped question plus its pointer line) measures 162–265
// bytes in AGENTS.md today, mean 204. So converting an item recovers
// (its size − ~204) bytes and costs one file read whenever its subject comes up.
// The two items left inline are 301 and 500 bytes — they would recover 97 and
// 296 — and they are also the only two whose bodies carry nothing to defer: no
// measurement, no mutation matrix, no retraction, no enumerated residual, no
// second sub-rule. Their evidence files would hold exactly the text the list
// already shows, so the trigger would promise something to open and deliver a
// restatement.
//
// 600 sits above item 4 (500) and below the smallest converted body (item 11's
// 638-byte stub, which recovered 441). The two tests agree at this line, which
// is why it is drawn here: everything under it saves less than 300 bytes AND has
// nothing to defer; everything over it fails both ways.
//
// 🔴 THIS IS THE RULE THAT KEEPS THE INDEX AN INDEX. A new item written inline
// as a full body fails here by name, and the fix is to write the trigger and
// move the body — not to raise this number.
const maxInlineItemBytes = 600

// minTriggersRequired is the POSITIVE CONTROL. A parser that has stopped seeing
// the bolded question — a changed wrap, a lost `**`, the wrong working directory
// — returns an empty set, iterates over nothing and reports a serene pass. 26
// items carry a trigger today; 20 leaves room to re-inline six without letting a
// scanner wired to nothing through.
const minTriggersRequired = 20

// triggerRe captures the bolded question that opens a converted item. It matches
// against the item block with newlines already collapsed, because AGENTS.md is
// hard-wrapped at ~79 columns and every trigger longer than about seventy
// characters straddles a line break — the same hazard agents_index_test.go's
// design decision 2 and agents_xrefs_test.go's paragraph-wise scan record, which
// is why neither of them reads a line at a time either.
var triggerRe = regexp.MustCompile(`^[0-9]+\. \*\*(.+?)\*\*`)

// degenerateTriggers are shapes that satisfy the question mark and the length
// band while routing nobody. Matched against the whitespace-collapsed,
// lower-cased trigger.
var degenerateTriggers = []string{
	"see the evidence",
	"see below",
	"see the file",
	"read the evidence",
	"tbd",
	"todo",
	"this item",
}

// itemNumberRestatementRe catches "item 17?" and its dressings — a trigger whose
// entire content is the number a reader would have to already know to look up.
var itemNumberRestatementRe = regexp.MustCompile(`^(agents\.?md )?items? [0-9]+\??$`)

type agentsItem struct {
	num       int
	line      int      // 1-based line of the heading, so a failure names where to look
	block     []string // the item's lines, as agentsItemSizes slices them
	trigger   string   // the bolded question, whitespace-collapsed ("" if none)
	pointer   string   // the `→ evidence:` path ("" if the item is inline)
	converted bool
}

// agentsItems slices AGENTS.md's numbered list into items, using the same
// boundary rule as agentsItemSizes and baseItemBody — the two slicers whose
// disagreement TestEvictionPlaybookSizesTheLastItemCorrectly exists to catch. A
// third spelling of that rule would be a third thing to keep in step, so this
// one is written to match them exactly and is asserted to agree below.
func agentsItems(t *testing.T) []agentsItem {
	t.Helper()
	b, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatalf("read AGENTS.md: %v (CONTROL failure — nothing below this line checked anything)", err)
	}
	body := string(b)
	if !strings.Contains(body, itemsSectionHeading) {
		t.Fatalf("CONTROL failure, not a finding: AGENTS.md has no %q heading, so no items could be located",
			itemsSectionHeading)
	}
	lines := strings.Split(body, "\n")
	type head struct{ num, line int }
	var heads []head
	for n, line := range lines {
		if m := itemHeadingRe.FindStringSubmatch(line); m != nil {
			num, _ := strconv.Atoi(m[1])
			heads = append(heads, head{num, n})
		}
	}
	var out []agentsItem
	for idx, h := range heads {
		end := len(lines)
		if idx+1 < len(heads) {
			end = heads[idx+1].line
		}
		block := lines[h.line:end]
		for i, l := range block {
			if i > 0 && l != "" && !strings.HasPrefix(l, " ") {
				block = block[:i]
				break
			}
		}
		for len(block) > 0 && strings.TrimSpace(block[len(block)-1]) == "" {
			block = block[:len(block)-1]
		}
		it := agentsItem{num: h.num, line: h.line + 1, block: block}
		joined := strings.Join(strings.Fields(strings.Join(block, " ")), " ")
		if m := triggerRe.FindStringSubmatch(joined); m != nil {
			it.trigger = strings.TrimSpace(m[1])
		}
		for _, l := range block {
			if m := evidencePointerRe.FindStringSubmatch(l); m != nil {
				it.pointer = m[1]
				it.converted = true
			}
		}
		out = append(out, it)
	}
	return out
}

// triggerDefect returns the reason a trigger is not a routing signal, or "" if
// it is one. It is the ONE classifier: the live check and the control corpus
// below both call it, so a narrowing fails the corpus by name instead of quietly
// shrinking what the live check rejects.
func triggerDefect(trigger, evidenceTitle string) string {
	t := strings.Join(strings.Fields(trigger), " ")
	if t == "" {
		return "there is no trigger at all — the item opens with no bolded question, so nothing routes a reader to it"
	}
	low := strings.ToLower(t)
	if itemNumberRestatementRe.MatchString(low) {
		return "the trigger is a restatement of the item number; a reader who already knew the number would not need the index"
	}
	for _, d := range degenerateTriggers {
		if strings.Contains(low, d) {
			return fmt.Sprintf("the trigger says %q, which routes nobody — it names the file, not the situation that should send someone to it", d)
		}
	}
	if n := len([]rune(t)); n < minTriggerChars {
		return fmt.Sprintf("the trigger is %d characters, under the %d-character floor — too short to name a situation", n, minTriggerChars)
	} else if n > maxTriggerChars {
		return fmt.Sprintf("the trigger is %d characters, over the %d-character ceiling — that is a body growing back one clause at a time", n, maxTriggerChars)
	}
	if !strings.HasSuffix(t, "?") {
		return "the trigger is a LABEL, not a trigger: it states what the item concluded instead of asking whether the reader is about to do the thing it governs. " +
			"A trigger ends in `?` because it is a question about the reader's situation"
	}
	if evidenceTitle != "" && normaliseForTitle(t) == normaliseForTitle(evidenceTitle) {
		return "the trigger is the evidence file's own title with a question mark on it — a title is a label by construction, so this restates the conclusion rather than naming the situation"
	}
	return ""
}

// normaliseForTitle strips the decoration a title and a trigger can differ by
// without differing in content: markup, punctuation and case.
func normaliseForTitle(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// evidenceTitleFor reads the H1 out of an evidence file, for rule 5.
func evidenceTitleFor(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return "" // the evidence ledger reports a missing file; do not double-report it
	}
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "# ") {
			return strings.TrimPrefix(l, "# ")
		}
	}
	return ""
}

// TestEveryAgentsItemCarriesATrigger is the guard for the failure the trigger
// index introduces: an item nobody is routed to.
func TestEveryAgentsItemCarriesATrigger(t *testing.T) {
	count := parseAgentsItems(t)
	items := agentsItems(t)
	if len(items) != count {
		t.Fatalf("CONTROL failure, not a finding: this file's slicer found %d item(s) and parseAgentsItems found %d. "+
			"Two slicers disagreeing about where the list is means one of them is reading something else; fix that before reading any result below.",
			len(items), count)
	}

	converted := 0
	for _, it := range items {
		if it.converted {
			converted++
		}
	}
	if converted < minTriggersRequired {
		t.Fatalf("CONTROL failure, not a finding: only %d of %d items carry an `→ evidence:` pointer, want >= %d.\n"+
			"A scan that finds almost no triggers validates almost nothing and would otherwise pass. Check triggerRe (%s) and evidencePointerRe (%s), "+
			"and that this test's working directory is the module root.",
			converted, len(items), minTriggersRequired, triggerRe, evidencePointerRe)
	}

	var problems []string
	for _, it := range items {
		if !it.converted {
			// An inline item is allowed to be a statement — it IS the rule, and
			// there is nothing to route to. It is bounded by size instead.
			continue
		}
		if d := triggerDefect(it.trigger, evidenceTitleFor(t, it.pointer)); d != "" {
			problems = append(problems, fmt.Sprintf("  AGENTS.md:%d: item %d — %s\n      trigger: %q", it.line, it.num, d, it.trigger))
		}
		// Rule 6: the block is the trigger and the pointer, nothing else.
		if extra := blockResidue(it); len(extra) > 0 {
			problems = append(problems, fmt.Sprintf(
				"  AGENTS.md:%d: item %d carries %d line(s) that are neither its trigger nor its pointer:\n      %s",
				it.line, it.num, len(extra), strings.Join(extra, "\n      ")))
		}
	}
	if len(problems) > 0 {
		t.Fatalf("%d item(s) whose trigger does not route anyone:\n%s\n\n"+
			"The numbered list is a TRIGGER INDEX: each line asks whether the reader is about to do the thing the item governs, and the pointer says "+
			"where the item itself lives. A line that states a conclusion instead is a label — it reads fine and routes nobody, and the item it hides "+
			"stays hidden with every other guard green.",
			len(problems), strings.Join(problems, "\n"))
	}
	t.Logf("all %d items carry a trigger or stay inline (%d converted, %d inline)", len(items), converted, len(items)-converted)
}

// blockResidue returns the lines of a converted item that belong to neither the
// trigger heading nor the pointer — rule 6.
func blockResidue(it agentsItem) []string {
	// The trigger heading runs from the first line to the line closing the `**`.
	closed := false
	var extra []string
	for _, l := range it.block {
		s := strings.TrimSpace(l)
		if s == "" {
			continue
		}
		if !closed {
			// Every line up to and including the one closing the bold marker
			// belongs to the trigger. A heading that never closes yields no
			// trigger at all, which triggerDefect reports rather than this.
			if strings.HasSuffix(s, "**") {
				closed = true
			}
			continue
		}
		if evidencePointerRe.MatchString(l) {
			continue
		}
		extra = append(extra, s)
	}
	return extra
}

// TestInlineAgentsItemsStayUnderTheBreakEven is rule 7. Without it the index
// reverts to a list of bodies one reasonable-looking commit at a time, and the
// byte ceiling would report that as a budget being spent rather than as a
// convention being abandoned.
func TestInlineAgentsItemsStayUnderTheBreakEven(t *testing.T) {
	items := agentsItems(t)
	sizes := agentsItemSizes(t)
	byNum := map[int]int{}
	for _, s := range sizes {
		byNum[s.num] = s.bytes
	}

	inline := 0
	var over []string
	for _, it := range items {
		if it.converted {
			continue
		}
		inline++
		n, ok := byNum[it.num]
		if !ok {
			t.Fatalf("CONTROL failure: item %d has no size from agentsItemSizes — the two slicers disagree about the list", it.num)
		}
		if n > maxInlineItemBytes {
			over = append(over, fmt.Sprintf(
				"  item %d is %d bytes inline, over the %d-byte break-even by %d (AGENTS.md:%d)",
				it.num, n, maxInlineItemBytes, n-maxInlineItemBytes, it.line))
		}
	}
	if len(over) > 0 {
		t.Fatalf("%d item(s) written inline that are large enough to pay for a trigger:\n%s\n\n"+
			"An item this size costs every session its full length whether or not the session touches it. Write a TRIGGER — one line asking whether "+
			"the reader is about to do what the item governs — and move the body to %s/NN-<slug>.md verbatim, with a row in splitItems pinning it. "+
			"Do NOT raise maxInlineItemBytes: it is the point at which converting starts paying, not a style preference.",
			len(over), strings.Join(over, "\n"), evidenceDir)
	}

	// POSITIVE CONTROL. If every item were converted this test would iterate
	// over nothing and pass having measured nothing, so the inline path has to
	// be exercised by at least one real item — and if a future wave converts
	// them both, this control is the thing that says so out loud.
	if inline == 0 {
		t.Fatalf("CONTROL failure, not a finding: no inline items were found, so the break-even rule was applied to nothing. " +
			"Either every item is now converted (in which case delete this control deliberately) or the pointer scan is broken.")
	}

	// 🔴 THE CONSTANT IS BOUNDED BY A MEASUREMENT, NOT BY A SECOND MAGIC NUMBER,
	// AND IT WAS UNBOUNDED FOR A ROUND. Raising maxInlineItemBytes to 99,999 was
	// a SURVIVING mutant: nothing exceeded the new value, so the whole suite
	// stayed green while the rule that keeps this list an index had been
	// deleted — the "a ceiling nobody bounds" failure agentsMaxBytesCeiling
	// exists to stop, regenerated one file over.
	//
	// The bound is re-derived from the live file rather than pinned: a trigger
	// block is what an item costs once converted, so an inline item worth
	// keeping must be within a small multiple of one. 3x is deliberately loose —
	// it is a bound on ABSURDITY, not a restatement of the break-even — and at
	// today's largest trigger block it puts the ceiling at ~795 against the
	// constant's 600.
	largestTrigger := 0
	for _, it := range items {
		if !it.converted {
			continue
		}
		if n := byNum[it.num]; n > largestTrigger {
			largestTrigger = n
		}
	}
	if largestTrigger == 0 {
		t.Fatalf("CONTROL failure, not a finding: no converted item had a measurable size, so the bound below was computed from nothing")
	}
	if bound := 3 * largestTrigger; maxInlineItemBytes > bound {
		t.Errorf("maxInlineItemBytes is %d, above %d — three times the largest trigger block in the file (%d bytes).\n"+
			"At that size an item can be written inline as a BODY and this guard will wave it through, which is the whole thing it exists to prevent. "+
			"If the break-even genuinely moved, re-derive it from the measured trigger cost and say so in the constant's comment; do not raise it to make one item fit.",
			maxInlineItemBytes, bound, largestTrigger)
	}
	t.Logf("%d inline item(s), all within the %d-byte break-even", inline, maxInlineItemBytes)
}

// TestTriggerClassifierRejectsLabelsAndPasses is the NEGATIVE CONTROL for
// triggerDefect: it drives the classifier over shapes that MUST be rejected and
// shapes that must NOT be, so a future narrowing fails here by name instead of
// silently widening what the live list may contain.
//
// Without it, "every trigger in AGENTS.md passes" is satisfiable by a classifier
// that rejects nothing — the reassuring-zero shape this repo keeps meeting.
func TestTriggerClassifierRejectsLabelsAndPasses(t *testing.T) {
	const title = "AGENTS.md item 17 — `DownloadPresigned` exists so a blob fetch carries NO credential"

	reject := []struct {
		name string
		in   string
	}{
		{"blank", ""},
		{"whitespace only", "   \n  "},
		{"the brief's own label example", "`DownloadPresigned` exists so a blob fetch carries no credential."},
		{"a label with no terminator at all", "`DownloadPresigned` exists so a blob fetch carries no credential"},
		{"a bare item-number restatement", "item 17?"},
		{"a dressed item-number restatement", "AGENTS.md item 17?"},
		{"a pointer in prose", "See the evidence file for item 17?"},
		{"a TODO", "TBD — write a trigger for this one later, it is about downloads?"},
		{"one word", "Downloads?"},
		{"under the floor by a hair", strings.Repeat("a", minTriggerChars-2) + "?"},
		{"over the ceiling — a body growing back", strings.Repeat("situation, ", 30) + "?"},
		{"the evidence file's own title with a question mark", title + "?"},
	}
	for _, tc := range reject {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			if d := triggerDefect(tc.in, title); d == "" {
				t.Errorf("triggerDefect accepted %q, which routes nobody", tc.in)
			}
		})
	}

	accept := []struct {
		name string
		in   string
	}{
		{"the real item 17 trigger",
			"Folding a credential-free blob transfer back into the token-carrying one, or touching `isTrustedDownloadHost`?"},
		{"a trigger naming several situations",
			"Touching `--image` — the workflow value, the `--ecosystem` refusal, the image cap, the dimensions, or the upload `--dry-run` still performs?"},
		{"a trigger wrapped across lines (collapsed by the caller)",
			"Asserting an exit code, or testing an error's classification, in ANY command?"},
		{"exactly at the floor", strings.Repeat("a", minTriggerChars-1) + "?"},
	}
	for _, tc := range accept {
		t.Run("accept/"+tc.name, func(t *testing.T) {
			if d := triggerDefect(tc.in, title); d != "" {
				t.Errorf("triggerDefect rejected a good trigger %q: %s", tc.in, d)
			}
		})
	}

	// POSITIVE CONTROL for the corpus itself: the accept set must not be empty
	// and the reject set must not be, or "the classifier agreed with every case"
	// is a claim about an empty table.
	if len(reject) < 8 || len(accept) < 3 {
		t.Fatalf("CONTROL failure: the corpus is %d reject / %d accept case(s); a table this small cannot pin the classifier",
			len(reject), len(accept))
	}
}

// TestEveryTriggerIsDistinct catches the copy-paste failure the shape invites:
// two items whose trigger is the same question route a reader to whichever they
// read first, and the second item is hidden exactly as if it had no trigger.
func TestEveryTriggerIsDistinct(t *testing.T) {
	seen := map[string][]int{}
	for _, it := range agentsItems(t) {
		if !it.converted {
			continue
		}
		k := normaliseForTitle(it.trigger)
		seen[k] = append(seen[k], it.num)
	}
	if len(seen) < minTriggersRequired {
		t.Fatalf("CONTROL failure, not a finding: only %d distinct trigger(s) parsed, want >= %d — the parser is not seeing the list",
			len(seen), minTriggersRequired)
	}
	for _, nums := range seen {
		if len(nums) > 1 {
			t.Errorf("items %v share one trigger. A reader routed by it reaches the first and never learns the others exist", nums)
		}
	}
}
