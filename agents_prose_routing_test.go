package cli_test

import (
	"os"
	"strings"
	"testing"
)

// AGENTS.md's numbered list has a preservation proof for every body it ever
// evicted: agents_split_preserved_test.go digests each one against the commit it
// was moved from, so "the move was verbatim" is mechanical rather than asserted.
//
// 🔴 ITS PROSE SECTIONS HAVE NO SUCH PROOF, AND PROSE IS NOW THE ONLY LEVER
// LEFT. agents_size_test.go's own playbook says so: with every item at
// trigger size, a ceiling failure can only be answered by moving a prose
// section. Two waves have now done that (the `newWhoAmICmd` fence, and
// `## Shell & CI gotchas` → CONTRIBUTING.md), and neither left anything behind
// that fails when the destination loses the content.
//
// That is a real gap, not a theoretical one, and it is shaped exactly like the
// failure the evidence ledger exists to catch one level up: the routing line
// stays, the destination quietly empties, and the line reads as coverage while
// routing a reader to nothing. Nobody re-reads a pointer they have already seen.
//
// This is the seam guard for the one prose eviction that has a named
// destination. It pins the RELATIONSHIP — AGENTS.md promises a set of traps live
// in CONTRIBUTING.md, so CONTRIBUTING.md must still carry them — rather than
// either file on its own, because each is perfectly coherent alone while the
// pair is broken.
//
// # 🔴 WHAT THE BODY BELOW ACTUALLY CHECKS — READ THIS BEFORE COUNTING IT AS
// # COVERAGE
//
// Four things, and no more:
//
//  1. AGENTS.md still has a `## Shell & CI gotchas` section (else there is no
//     promise to hold anyone to, and this guard reports a CONTROL failure rather
//     than a pass).
//  2. That section still spells the literal anchor `CONTRIBUTING.md#shell--ci-gotchas`.
//  3. CONTRIBUTING.md still has a `## Shell & CI gotchas` heading, and the
//     section under it is at least minSectionBytes long.
//  4. FOUR WORDS appear somewhere in that section: `gofmt`, `go test`,
//     `gh pr checks`, `ci-shallow.sh`.
//
// That set catches the realistic RELOCATION failures — the destination heading
// deleted or renamed, the anchor mistyped, a whole trap dropped — and it catches
// nothing else.
//
// 🔴 IT DOES NOT PRESERVE THE TRAPS' CONTENT, AND THE GAP IS MEASURED, NOT
// ESTIMATED. An audit replaced the ENTIRE section with one sentence naming the
// four tools plus filler to ~1,739 bytes — every control, every mechanism, all
// four traps' actual text gone — and this guard stayed GREEN. Re-measured here
// rather than inherited from that report, which said 2,617 and 43%: the section
// is 2,594 bytes as the slicer below cuts it, so the 1,500-byte floor permits
// deleting 42.2% of it outright. So: four words and a length floor. Not a
// preservation proof, and not comparable to
// agents_split_preserved_test.go, which digests an item body against the commit
// it was moved from. There is no base digest for prose, and inventing one would
// pin a section that is legitimately edited far more often than an item body.
//
// If that residual ever needs closing, close it by digesting the section against
// a recorded base the way splitItems does — not by adding more words to
// shellGotchaSubjects, which only moves the summarisable boundary.
//
// # 🔴 A GENERIC ANCHOR WALKER LIVED HERE AND WAS DELETED. DO NOT RE-ADD IT.
//
// It scanned AGENTS.md for every `](CONTRIBUTING.md#…)` link and re-implemented
// GitHub's heading-slug rules to check each resolved, on the argument that a
// FUTURE prose eviction would need it. Measured at the commit that removed it:
// AGENTS.md makes exactly ONE such link, and the anchor-typo mutant that walker
// existed to catch is already killed by rule 2 above — a DOUBLE kill, confirmed
// by applying the mutant in an isolated copy. So it was ~75 lines, including a
// second implementation of somebody else's slug algorithm that can itself be
// wrong, whose only live instance was already covered.
//
// When a second prose eviction happens, give it its own seam assertion here —
// naming its own destination and its own subjects — the way this one does.
// That is cheaper than a generic walker and it fails with a message about the
// eviction rather than about a slug.

// shellGotchasHeading is the destination heading. It is spelled once, here,
// because both halves of the assertion below need it and a second spelling is a
// second thing to keep in step.
const shellGotchasHeading = "## Shell & CI gotchas"

// shellGotchasAnchor is the GitHub anchor AGENTS.md's routing line links to.
// Derived by hand from the heading above rather than computed, because getting
// it wrong is precisely the defect: `&` collapses to nothing and leaves a DOUBLE
// hyphen, which is the part a reader writing the link by eye gets wrong.
const shellGotchasAnchor = "CONTRIBUTING.md#shell--ci-gotchas"

// shellGotchaSubjects are the four tools AGENTS.md's routing line promises are
// covered at the destination. Each is a distinct trap with a distinct control,
// so losing any one of them is a real loss and not a rewording.
var shellGotchaSubjects = []string{
	"gofmt",
	"go test",
	"gh pr checks",
	"ci-shallow.sh",
}

// contributingSection slices one `## ` section out of CONTRIBUTING.md.
func contributingSection(t *testing.T, heading string) string {
	t.Helper()
	b, err := os.ReadFile("CONTRIBUTING.md")
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot read CONTRIBUTING.md: %v — "+
			"the destination of AGENTS.md's routing line could not be read, so nothing below checked anything", err)
	}
	doc := string(b)
	i := strings.Index(doc, heading+"\n")
	if i < 0 {
		t.Fatalf("CONTRIBUTING.md has no %q heading.\n"+
			"AGENTS.md's `%s` section is a ROUTING LINE that sends the reader here by anchor. "+
			"With the heading gone the anchor resolves to the top of the file and the reader finds nothing, "+
			"while AGENTS.md still reads as though the content exists. Restore the heading or repoint the line.",
			heading, heading)
	}
	body := doc[i+len(heading)+1:]
	if j := strings.Index(body, "\n## "); j >= 0 {
		body = body[:j]
	}
	return body
}

// minRoutingSectionBytes is the anti-vacuity floor on the AGENTS.md side. A
// slicer that returns "" makes the anchor check fail for the wrong reason and
// blames the prose for a broken extractor.
const minRoutingSectionBytes = 300

// agentsSection slices one `## ` section out of AGENTS.md. It is the mirror of
// contributingSection and exists for the same reason: an assertion about "this
// section" must be made against that section, not against the whole file.
func agentsSection(doc, heading string) string {
	i := strings.Index(doc, heading+"\n")
	if i < 0 {
		return ""
	}
	body := doc[i+len(heading)+1:]
	if j := strings.Index(body, "\n## "); j >= 0 {
		body = body[:j]
	}
	return body
}

// TestShellGotchasRoutingLineResolves is the seam described above.
func TestShellGotchasRoutingLineResolves(t *testing.T) {
	b, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot read AGENTS.md: %v", err)
	}
	agents := string(b)

	// Direction 1 — AGENTS.md still routes. If the routing line is gone the
	// assertion below would be checking CONTRIBUTING.md against a promise nobody
	// makes, which passes while meaning nothing.
	if !strings.Contains(agents, shellGotchasHeading) {
		t.Fatalf("CONTROL failure, not a finding: AGENTS.md has no %q section, so there is no routing line "+
			"to hold CONTRIBUTING.md to. If the section was deliberately retired, retire this guard in the "+
			"same commit rather than leaving it green over nothing.", shellGotchasHeading)
	}
	// 🔴 SCOPED TO THE SECTION, BECAUSE A WHOLE-FILE CONTAINS WAS WALKABLE AND WAS
	// WALKED. This read `strings.Contains(agents, shellGotchasAnchor)` over the
	// entire file while its own failure message said "section". An audit removed
	// the link from the routing paragraph, leaving a bare `CONTRIBUTING.md`, and
	// spelled the anchor in an HTML comment under a different heading — the guard
	// reported ok, which is precisely the failure the message describes. The
	// anchor has to be in the paragraph that does the routing, or it routes nobody.
	routing := agentsSection(agents, shellGotchasHeading)
	if len(routing) < minRoutingSectionBytes {
		t.Fatalf("CONTROL failure, not a finding: AGENTS.md's %q section slices to %d bytes, under the %d-byte "+
			"floor. The section slicer is reading the wrong block — check that before reading the anchor verdict below.",
			shellGotchasHeading, len(routing), minRoutingSectionBytes)
	}
	if !strings.Contains(routing, shellGotchasAnchor) {
		t.Errorf("AGENTS.md's %q section no longer links %q.\n"+
			"The section is a routing line and nothing else — the traps themselves were moved to CONTRIBUTING.md. "+
			"Without the link IN THAT SECTION the reader is told a hazard exists and not where to read it; the "+
			"anchor appearing somewhere else in the file does not route them.",
			shellGotchasHeading, shellGotchasAnchor)
	}

	// Direction 2 — the destination still carries what the routing line promises.
	section := contributingSection(t, shellGotchasHeading)

	// POSITIVE CONTROL on the slicer, before any verdict. An empty or truncated
	// slice makes every subject below report missing, which blames the prose for a
	// broken extractor.
	const minSectionBytes = 1500
	if len(section) < minSectionBytes {
		t.Fatalf("CONTROL failure, not a finding: CONTRIBUTING.md's %q section is %d bytes, under the "+
			"%d-byte floor. The section extractor is reading the wrong block, or the content was gutted — "+
			"check which before reading the subject list below as a finding.",
			shellGotchasHeading, len(section), minSectionBytes)
	}

	for _, subject := range shellGotchaSubjects {
		if !strings.Contains(section, subject) {
			t.Errorf("CONTRIBUTING.md's %q section no longer mentions %q.\n"+
				"AGENTS.md's routing line names that tool as one of the traps enumerated here, so the promise "+
				"and the content have come apart — and the reader who follows the link finds a section that "+
				"does not answer the question that sent them.", shellGotchasHeading, subject)
		}
	}

	// The routing line keeps ONE trap inline, because it fires on a command in
	// this repo's own Makefile and a reader who never opens CONTRIBUTING.md still
	// has to know it. Pin that it is still inline: dropping it is a silent
	// widening of what the eviction cost.
	//
	// Scoped to the section for the same reason the anchor check above is: over the
	// whole file this passes on the phrase appearing anywhere, and "dirty tree" is
	// exactly the sort of phrase another section can grow.
	if !strings.Contains(routing, "dirty tree") {
		t.Errorf("AGENTS.md's routing line no longer carries the ci-shallow dirty-tree warning inline.\n" +
			"That one is kept in AGENTS.md on purpose — it fires on `./scripts/ci-shallow.sh`, a command in " +
			"this repo's Makefile, and it reads as a GREEN about the change you just made. Everything else " +
			"about it is at the destination; this sentence is not.")
	}
}
