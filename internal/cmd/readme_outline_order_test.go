package cmd

import (
	"strings"
	"testing"
)

// This file holds the one guard phase 4 of the README reduction exists to make
// possible: the README presents its sections in the order `## Contents` lists
// them.
//
// # Why this needed a guard rather than a one-off edit
//
// Before phase 4 the two disagreed, and nothing anywhere noticed. `## Contents`
// listed `Command reference` last in "Get started" while the document put it
// after two "Author an App" sections; it listed `The blockId`, `Templates`, `The
// host handshake`, `Local dev loop`, `Preview in the real host` and `Examples`
// as flat top-level entries while all six were `###` subsections nested inside
// `## Command reference`; and it grouped `Browse the public API`, `Download
// model files` and `Scripting with --json` under "Use the API" while the
// document had them sandwiched between the authoring sections and `## Validate
// fidelity`. The whole suite was green through all of it, because every
// pre-existing navigation guard in readme_nav_test.go asserts PRESENCE — that a
// heading has a TOC line and a TOC line has a heading. A table of contents can
// satisfy both directions of that and still be a map of a document nobody
// wrote.
//
// # Why the document moved to the TOC rather than the TOC to the document
//
// Both directions would have made the two agree and both are byte-neutral, so
// this needs a reason, and the obvious one does not hold up. 🔴 THE ORIGINAL
// REASON WAS A BYTE-DISTANCE READER MODEL, AND IT REFUTES ITSELF. It said a CI
// author sent to `## Scripting with --json` had 160,391 bytes — 58% of the file
// — before reaching `## Exit codes`, the contract their script branches on
// (22,339 after). But the same argument dismissed the one reader the reorder
// made WORSE (`## Download model files`, 64,782 -> 159,634) on the grounds that
// they still have a Contents link. If a Contents link settles it for that
// reader it settles it for the CI reader too, and the headline number is worth
// nothing. Round 0 on #648 is what caught that; the numbers survive as history,
// not as the justification.
//
// The reason that does hold is structural, and it is the one to keep:
// `## Contents` is not a flat list — it carries four editorial groups (**Get
// started**, **Author an App**, **Use the API**, **Reference**), and on `main`
// the three read-path sections sat INSIDE the authoring run while `## Generate`
// sat between `## App metrics` and `## Upgrading`. So repairing the TOC to match
// the document would have had to SPLIT "Use the API" into fragments interleaved
// with "Author an App" — destroying a real reader affordance to preserve an
// accident. Moving the document is the only direction that keeps the grouping,
// and topical coherence (a reader working the authoring track linearly stopped
// hitting three API sections mid-stream) is the benefit, not scroll distance.
//
// # What it does NOT assert
//
// Nothing about whether the order is GOOD. Contents is the authority here and
// this guard only holds the document to it, so moving a section is still a
// judgement — it is just a judgement that now has to be made in both places at
// once. That is the whole point: the failure it ends is the two drifting apart
// silently, not somebody deliberately reordering the book.

// readmeOutlineOrder returns, in document order, the anchor slug of every
// heading `## Contents` is required to list — every `##` except Contents itself,
// and every `###` whose parent is not in readmeTOCExemptSections.
//
// It is derived from readmeSections and readmeTOCExemptSections rather than from
// a second heading walk, so the set it produces is by construction the same set
// TestREADMETableOfContentsCoversEverySection requires to be present. If those
// two ever disagreed about scope, the order check would be asserting a sequence
// over a population the presence check does not police.
func readmeOutlineOrder(t *testing.T, md string) []string {
	t.Helper()
	var out []string
	for _, s := range readmeSections(t, md) {
		if s.Slug == "contents" {
			continue
		}
		if s.Level == 3 {
			if _, exempt := readmeTOCExemptSections[s.Parent]; exempt {
				continue
			}
		}
		out = append(out, s.Slug)
	}
	return out
}

// TestREADMEContentsListsSectionsInDocumentOrder is the order half of the
// navigation gate. Every heading the TOC must list has to appear in the TOC in
// the position the document gives it.
//
// # Watched to fail
//
// This is a regression test, not an invariant guard, and it was run against the
// tree it was written for. At `4cb7c17` — the commit phase 4 branched from — the
// two sequences hold the SAME 66 slugs and diverge at index 9: the document has
// `set-up-your-coding-agent-agent-setup` where Contents has `command-reference`.
// It goes green at the phase-4 head with no other change. Both halves of that
// matrix were observed, not predicted.
//
// # The failure message names the divergence, not the whole list
//
// A 66-element diff is unreadable and the second element onward is usually a
// consequence of the first. The report is the first position where the two
// disagree plus the few entries around it, which is the edit a maintainer has to
// make.
func TestREADMEContentsListsSectionsInDocumentOrder(t *testing.T) {
	md := readREADME(t)
	doc := readmeOutlineOrder(t, md)
	toc := readmeContentsAnchors(t, md)

	// Positive control. Both extractors have their own floor already, but those
	// floors are about each side in isolation; this one is about the COMPARISON
	// being worth making. Two short lists agree trivially, and a guard that
	// passes by having almost nothing to compare is the vacuous green this arc
	// keeps finding. The real count is 66, so the floor sits well below it.
	if len(doc) < 40 {
		t.Fatalf("CONTROL failure: the document walk found only %d in-scope headings — the section "+
			"walk or the exemption map is reading the wrong text, and an order verdict over that "+
			"is a verdict about nothing", len(doc))
	}
	if len(toc) < 40 {
		t.Fatalf("CONTROL failure: the Contents block yielded only %d anchors — the TOC extractor is "+
			"reading the wrong block", len(toc))
	}

	// Membership is TestREADMETableOfContentsCoversEverySection's job in one
	// direction and TestREADMETableOfContentsListsNothingElse's in the other, and
	// both give far better messages for it than a positional diff can. So when
	// the populations differ, say so and stop rather than reporting a divergence
	// at index 3 that is really a missing entry at index 2.
	if len(doc) != len(toc) {
		t.Fatalf("the document has %d headings the TOC must list and the TOC lists %d anchors — "+
			"that is a PRESENCE failure, not an ordering one. "+
			"TestREADMETableOfContentsCoversEverySection and "+
			"TestREADMETableOfContentsListsNothingElse name the specific entries; fix those first, "+
			"then this guard's verdict is about order again.", len(doc), len(toc))
	}

	for i := range doc {
		if doc[i] == toc[i] {
			continue
		}
		t.Fatalf("README section order and `## Contents` order diverge at position %d.\n\n"+
			"  document: %s\n"+
			"  contents: %s\n\n"+
			"Contents is the authority: it is the map a reader navigates by, and a map that lists "+
			"sections in an order the document does not use sends every reader who trusts it to the "+
			"wrong place. Move the section, or move the Contents line — but the two travel together.",
			i, orderWindow(doc, i), orderWindow(toc, i))
	}
}

// orderWindow renders the entries around index i of a sequence, marking the one
// at i, so the failure above shows the divergence in its neighbourhood instead
// of as two bare slugs with no context for which way a section moved.
func orderWindow(seq []string, i int) string {
	lo, hi := i-2, i+3
	if lo < 0 {
		lo = 0
	}
	if hi > len(seq) {
		hi = len(seq)
	}
	parts := make([]string, 0, hi-lo)
	for j := lo; j < hi; j++ {
		if j == i {
			parts = append(parts, ">>"+seq[j]+"<<")
			continue
		}
		parts = append(parts, seq[j])
	}
	return strings.Join(parts, " → ")
}
