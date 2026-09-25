package cli_test

import (
	"os"
	"regexp"
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
// 🔴 WHAT IT DOES NOT DO. It does not prove the move was VERBATIM. It has no
// base commit to digest against the way splitItems does, and inventing one for
// prose would pin a section that is legitimately edited far more often than an
// item body. What it proves is that the four subjects the routing line NAMES are
// still discussed where it says they are. A summarised trap passes. That is a
// weaker claim than the item ledger makes, and it is stated here so nobody reads
// this file as the same kind of proof.

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
	if !strings.Contains(agents, shellGotchasAnchor) {
		t.Errorf("AGENTS.md's %q section no longer links %q.\n"+
			"The section is a routing line and nothing else — the traps themselves were moved to CONTRIBUTING.md. "+
			"Without the link the reader is told a hazard exists and not where to read it.",
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
	if !strings.Contains(agents, "dirty tree") {
		t.Errorf("AGENTS.md's routing line no longer carries the ci-shallow dirty-tree warning inline.\n" +
			"That one is kept in AGENTS.md on purpose — it fires on `./scripts/ci-shallow.sh`, a command in " +
			"this repo's Makefile, and it reads as a GREEN about the change you just made. Everything else " +
			"about it is at the destination; this sentence is not.")
	}
}

// agentsProseRoutingRe finds every `](CONTRIBUTING.md#…)` anchor link AGENTS.md
// makes, so a SECOND prose eviction cannot quietly ship without a resolving
// destination.
//
// It is deliberately wider than the one section above: the guard that only knows
// about today's eviction is the guard that is silent about tomorrow's, which is
// the shape agents_evidence_test.go's bidirectional ledger exists to avoid.
var agentsProseRoutingRe = regexp.MustCompile(`\(CONTRIBUTING\.md#([a-z0-9-]+)\)`)

// TestEveryAgentsContributingAnchorResolves walks those links and checks each
// names a heading CONTRIBUTING.md actually has, under GitHub's slug rules.
func TestEveryAgentsContributingAnchorResolves(t *testing.T) {
	ab, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot read AGENTS.md: %v", err)
	}
	cb, err := os.ReadFile("CONTRIBUTING.md")
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot read CONTRIBUTING.md: %v", err)
	}

	have := map[string]bool{}
	headings := 0
	for _, line := range strings.Split(string(cb), "\n") {
		if !strings.HasPrefix(line, "#") {
			continue
		}
		headings++
		have[githubAnchor(strings.TrimLeft(line, "# "))] = true
	}
	// POSITIVE CONTROL on the slugger AND on the walk: a CONTRIBUTING.md with no
	// parsed headings makes every link below report dangling, and a slugger that
	// produced garbage would do the same. Requiring a known heading to round-trip
	// proves both.
	if headings < 5 {
		t.Fatalf("CONTROL failure, not a finding: parsed %d heading(s) out of CONTRIBUTING.md, want >= 5 — "+
			"the heading scan is broken, not the links", headings)
	}
	if !have["shell--ci-gotchas"] {
		t.Fatalf("CONTROL failure, not a finding: githubAnchor did not produce %q from any CONTRIBUTING.md "+
			"heading. Either the section is gone (which TestShellGotchasRoutingLineResolves reports properly) "+
			"or the slugger mangles `&`, in which case every verdict below is about the slugger.",
			"shell--ci-gotchas")
	}

	links := agentsProseRoutingRe.FindAllStringSubmatch(string(ab), -1)
	// POSITIVE CONTROL for the link scan. Zero links is what a reworded pointer,
	// a changed path or a regex typo all look like, and it would pass over
	// nothing.
	if len(links) == 0 {
		t.Fatal("CONTROL failure, not a finding: AGENTS.md makes no `](CONTRIBUTING.md#…)` anchor link at all. " +
			"Prose evicted to CONTRIBUTING.md is reached ONLY by such a link, so either an eviction lost its " +
			"pointer or this scan is broken (pattern: " + agentsProseRoutingRe.String() + ")")
	}
	for _, m := range links {
		if !have[m[1]] {
			t.Errorf("AGENTS.md links CONTRIBUTING.md#%s, which is not a heading in CONTRIBUTING.md.\n"+
				"A prose eviction is only as good as its pointer: an anchor that does not resolve silently "+
				"drops the reader at the top of the file, which looks like arriving.", m[1])
		}
	}
	t.Logf("%d CONTRIBUTING.md anchor link(s) in AGENTS.md, all resolving against %d heading(s)", len(links), headings)
}

// githubAnchor slugs a Markdown heading the way GitHub does: lower-cased,
// punctuation dropped, spaces to hyphens. The `&` case is the one that matters
// here — it is DROPPED rather than replaced, so `Shell & CI gotchas` becomes
// `shell--ci-gotchas` with two hyphens, which is exactly the spelling a human
// writing the link by eye gets wrong.
func githubAnchor(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(heading)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}
