package cmd

import (
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"
)

// The README is ~1.6k lines and is navigated almost entirely through in-document
// anchor links (a table of contents, plus ~20 cross-references between the
// command table and the sections that hold the detail). A broken anchor is
// invisible: GitHub renders it as an ordinary link that silently goes nowhere.
// These guards make the navigation checkable.

// readmeHeadingRe matches a `##`/`###` ATX heading. Fenced code blocks are
// stripped before this runs — a `# comment` line inside a bash block is not a
// heading, and counting one would invent an anchor that does not exist.
var readmeHeadingRe = regexp.MustCompile(`(?m)^(#{2,3})[ \t]+(.+?)[ \t]*$`)

// readmeAnchorRe matches an in-document link target: the `#foo` in `[x](#foo)`.
var readmeAnchorRe = regexp.MustCompile(`\]\(#([^)]*)\)`)

// readmeAnchorSlug reproduces GitHub's heading→anchor algorithm: lowercase,
// drop every character that is not a letter, digit, space, hyphen or
// underscore (so emoji, em dashes, arrows, backticks, parentheses, colons,
// commas, apostrophes and `&` all vanish), then turn each remaining space into
// a hyphen. Runs of dropped punctuation therefore leave DOUBLED hyphens
// (`Submit & auth` → `submit--auth`), which is exactly what GitHub produces and
// what the README's pre-existing links already spell.
//
// 🔴 This is a REIMPLEMENTATION of somebody else's rule, so it is only worth
// what its control corpus is worth — see
// TestREADMEAnchorSlugMatchesKnownGitHubAnchors, which pins it against anchors
// that shipped and worked before this file existed. Without that control, this
// function and the TOC could agree with each other and both be wrong.
func readmeAnchorSlug(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(heading)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		case r == '-' || r == '_':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// stripFencedCode removes ``` blocks so headings and links inside sample output
// are not mistaken for real ones.
func stripFencedCode(md string) string {
	var out []string
	fenced := false
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if !fenced {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// readmeHeadings returns slug → heading text for every `##`/`###` heading, and
// fails on a duplicate slug: two headings that collide get `-1` suffixes from
// GitHub, which this model does not reproduce, so a collision would make every
// anchor claim below unreliable rather than merely incomplete.
func readmeHeadings(t *testing.T, md string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, m := range readmeHeadingRe.FindAllStringSubmatch(stripFencedCode(md), -1) {
		slug := readmeAnchorSlug(m[2])
		if prev, dup := out[slug]; dup {
			t.Fatalf("two README headings share the anchor %q (%q and %q). "+
				"GitHub disambiguates with a -1 suffix, which this guard does not model — rename one.",
				slug, prev, m[2])
		}
		out[slug] = m[2]
	}
	if len(out) < 20 {
		t.Fatalf("found only %d README headings — the heading regex is reading the wrong text", len(out))
	}
	return out
}

// anyHeadingRe matches an ATX heading of ANY level, capturing its `#` run and
// its text. Used to bound a section at the next same-or-higher-level heading.
var anyHeadingRe = regexp.MustCompile(`^(#{1,6})[ \t]+(.+?)[ \t]*$`)

// readmeSectionByAnchor returns the body of the README section whose heading
// slugs to `slug`, up to the next heading at the same or a higher level.
//
// It exists so a guard can follow a command-table row's OWN cross-reference and
// assert against the text a reader reaches, rather than against the row alone.
// That makes relocating detail out of an unreadable table cell a supported move
// instead of a silent loss of coverage — and it fails when the link is broken,
// which is the failure mode the relocation introduces.
func readmeSectionByAnchor(t *testing.T, md, slug string) string {
	t.Helper()
	lines := strings.Split(stripFencedCode(md), "\n")
	start, level := -1, 0
	for i, line := range lines {
		m := anyHeadingRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if readmeAnchorSlug(m[2]) == slug {
			start, level = i+1, len(m[1])
			break
		}
	}
	if start < 0 {
		t.Fatalf("README.md has no heading anchored at #%s", slug)
	}
	for i := start; i < len(lines); i++ {
		if m := anyHeadingRe.FindStringSubmatch(lines[i]); m != nil && len(m[1]) <= level {
			return strings.Join(lines[start:i], "\n")
		}
	}
	return strings.Join(lines[start:], "\n")
}

// readmeRowPlusLinkedSections returns a command-table row concatenated with
// every README section its cross-reference links point at — i.e. everything a
// reader who follows the row actually reads.
func readmeRowPlusLinkedSections(t *testing.T, md, row string) string {
	t.Helper()
	parts := []string{row}
	for _, m := range readmeAnchorRe.FindAllStringSubmatch(row, -1) {
		parts = append(parts, readmeSectionByAnchor(t, md, m[1]))
	}
	if len(parts) == 1 {
		t.Fatalf("command-table row carries no cross-reference to follow:\n%s", row)
	}
	return strings.Join(parts, "\n")
}

// TestREADMEAnchorSlugMatchesKnownGitHubAnchors is the positive control for the
// slug algorithm. Every pair below is a heading plus the anchor the README
// ALREADY linked it by before this guard existed — i.e. anchors observed to
// work on github.com. They exercise the four cases the algorithm gets wrong if
// it is written naively: a dropped `&` leaving a double hyphen, backticks
// around a `--flag`, a leading emoji, and an underscore that must SURVIVE.
//
// 🔴 THE CORPUS IS OBSERVED ANCHORS ONLY. Adding a pair derived from this same
// function would make the control agree with itself, which is the exact failure
// readmeAnchorSlug's own doc comment warns about — so a newly-written heading
// does not belong here (see TestREADMEWidenedTOCEntriesSlugAsExpected, which
// pins those separately and says what it is and is not worth).
//
// # `###` is not an untested case
//
// The heading LEVEL is not an input: GitHub computes the slug from the heading
// text alone, and so does readmeAnchorSlug — which takes the text and nothing
// else. The corpus bears that out empirically rather than by assertion: seven of
// the eight pairs below are `###` headings in this very README, and only
// `Submit & auth` is a `##`. So the `###` entries the table of contents gained
// are not a new anchor FORM, and each one's distinguishing character class is
// already pinned by an observed anchor — a leading emoji dropping to a bare
// hyphen by `🔴 Silent model substitution`, and an apostrophe vanishing without
// a separator by `there aren't twelve` → `there-arent-twelve`.
func TestREADMEAnchorSlugMatchesKnownGitHubAnchors(t *testing.T) {
	cases := map[string]string{
		"Submit & auth":                                  "submit--auth",
		"The `--json` result shape":                      "the---json-result-shape",
		"🔴 Silent model substitution":                    "-silent-model-substitution",
		"The host handshake (`BLOCK_READY`)":             "the-host-handshake-block_ready",
		"Local dev loop (harness: mock vs live)":         "local-dev-loop-harness-mock-vs-live",
		"After you submit: review → approve → deploy":    "after-you-submit-review--approve--deploy",
		"Exit codes specific to `generate`":              "exit-codes-specific-to-generate",
		"The content flags, and why there aren't twelve": "the-content-flags-and-why-there-arent-twelve",
	}
	for heading, want := range cases {
		if got := readmeAnchorSlug(heading); got != want {
			t.Errorf("readmeAnchorSlug(%q) = %q, want %q", heading, got, want)
		}
	}
}

// TestREADMEWidenedTOCEntriesSlugAsExpected pins the anchors of the two
// subsections the widened TOC gate found missing, so the exact strings written
// into the Contents list are asserted rather than assumed.
//
// 🔴 THIS IS NOT A CONTROL, AND IT IS DELIBERATELY NOT IN THE CORPUS ABOVE.
// These anchors were derived from readmeAnchorSlug, not observed on github.com,
// so they cannot testify that the algorithm is RIGHT. What they are worth is the
// other failure: TestREADMEAnchorLinksResolve compares a heading and a link
// through this same function, so a change to the algorithm moves both sides
// together and stays green while every anchor in the file silently changes. A
// literal expected value is the only thing that notices.
func TestREADMEWidenedTOCEntriesSlugAsExpected(t *testing.T) {
	cases := map[string]string{
		"🔴 A checkpoint does not carry its ecosystem": "-a-checkpoint-does-not-carry-its-ecosystem",
		"Reading a workflow's Buzz transactions":      "reading-a-workflows-buzz-transactions",
	}
	for heading, want := range cases {
		if got := readmeAnchorSlug(heading); got != want {
			t.Errorf("readmeAnchorSlug(%q) = %q, want %q", heading, got, want)
		}
	}
	// The pin is only worth anything if the README really carries these
	// headings under those anchors — otherwise it pins two strings nothing uses.
	headings := readmeHeadings(t, readREADME(t))
	for heading, slug := range cases {
		if got, ok := headings[slug]; !ok || got != heading {
			t.Errorf("README.md has no heading %q at #%s (found %q, present=%v) — this pin is asserting a string the document does not use",
				heading, slug, got, ok)
		}
	}
}

// TestREADMEAnchorLinksResolve is the guard that keeps the table of contents —
// and every cross-reference in the command table — honest. Every `](#…)` target
// in the README must be a heading that exists in the README.
//
// Mutation-verified when it landed: adding a single `[x](#no-such-heading)` line
// reddens it by name; removing that line greens it again.
func TestREADMEAnchorLinksResolve(t *testing.T) {
	md := readREADME(t)
	headings := readmeHeadings(t, md)

	links := readmeAnchorRe.FindAllStringSubmatch(stripFencedCode(md), -1)
	// Positive control: a zero-link result is indistinguishable from a guard
	// wired to nothing. The README carries a full TOC plus the command table's
	// cross-references, so the real number is far above this floor.
	if len(links) < 40 {
		t.Fatalf("found only %d in-document anchor links — the link regex is reading the wrong text", len(links))
	}

	var broken []string
	for _, m := range links {
		if _, ok := headings[m[1]]; !ok {
			broken = append(broken, m[1])
		}
	}
	if len(broken) > 0 {
		sort.Strings(broken)
		var known []string
		for slug := range headings {
			known = append(known, slug)
		}
		sort.Strings(known)
		t.Errorf("README anchor link(s) point at no heading: %v\n\nheadings that DO exist:\n  %s",
			broken, strings.Join(known, "\n  "))
	}
}

// readmeContentsLinks returns the set of anchor slugs the "## Contents" list
// links to. The block is delimited structurally — the `## Contents` heading to
// the next `## ` — so ordinary edits inside it do not move the bounds.
func readmeContentsLinks(t *testing.T, md string) map[string]bool {
	t.Helper()
	const heading = "\n## Contents\n"
	i := strings.Index(md, heading)
	if i < 0 {
		t.Fatal("README.md has no `## Contents` section — the table of contents is gone")
	}
	toc := md[i+len(heading):]
	if j := strings.Index(toc, "\n## "); j >= 0 {
		toc = toc[:j]
	}

	linked := map[string]bool{}
	for _, m := range readmeAnchorRe.FindAllStringSubmatch(toc, -1) {
		linked[m[1]] = true
	}
	// Positive control: a Contents block that extracted to nothing would report
	// every section as missing (loud) — but it would report every TOC entry as
	// legitimate (silent), which is the reverse direction below.
	if len(linked) < 20 {
		t.Fatalf("the Contents section holds only %d links — the extractor is reading the wrong block", len(linked))
	}
	return linked
}

// readmeSection is one `##` or `###` heading with the `##` section that contains
// it. `Parent` is the empty string for a `##` heading (it is its own section).
type readmeSection struct {
	Level  int
	Text   string
	Slug   string
	Parent string // the `##` heading text a `###` sits under
}

// readmeSections walks the README top to bottom and returns every `##`/`###`
// heading in document order, each `###` tagged with the `##` it belongs to.
//
// The walk is line-by-line rather than a whole-file regex because the PARENT is
// the whole point: scope below is decided per `##` section, and a regex over the
// flat text cannot say which section a `###` fell in.
func readmeSections(t *testing.T, md string) []readmeSection {
	t.Helper()
	var out []readmeSection
	parent := ""
	for _, line := range strings.Split(stripFencedCode(md), "\n") {
		m := anyHeadingRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		switch len(m[1]) {
		case 2:
			parent = m[2]
			out = append(out, readmeSection{Level: 2, Text: m[2], Slug: readmeAnchorSlug(m[2])})
		case 3:
			if parent == "" {
				t.Fatalf("README.md has a `### %s` before any `##` heading — the section walk cannot attribute it", m[2])
			}
			out = append(out, readmeSection{Level: 3, Text: m[2], Slug: readmeAnchorSlug(m[2]), Parent: parent})
		}
	}
	return out
}

// readmeTOCExemptSections names the `##` sections whose `###` subsections are
// deliberately NOT table-of-contents entries. The exemption is keyed by PARENT
// SECTION, never by individual heading: a per-heading allowlist grows one line
// per omission and reads as a to-do list, whereas naming the section states a
// rule a future author can apply to a subsection that does not exist yet.
//
// Both entries are the same shape — a section whose `###` children are an
// ENUMERATION of the index directly above them, not topics a reader navigates
// to. Everything else in the README is in scope.
var readmeTOCExemptSections = map[string]string{
	"Exit codes": "its `### Exit code N` subsections are GENERATED from exitCodeDocs " +
		"(internal/cmd/exitcodes_doc.go) and asserted byte-for-byte by " +
		"TestREADMEExitCodeSectionsAreGenerated. Their number and their headings follow " +
		"that table, so listing them in a hand-written TOC would put a generated set " +
		"behind a hand-maintained index — the exact drift that generator exists to end. " +
		"Each is already reachable by name from the `[Detail](#exit-code-N)` link in the " +
		"table the generator renders alongside them. Putting them in the TOC is a " +
		"decision to generate the TOC too, not a line to add by hand.",
	"Troubleshooting": "its `###` children are the column headers of ONE lookup table — " +
		"the section's own first line is \"Look up the message you got\", and the TOC " +
		"entry for it already says \"look the error message up here\". A reader arrives " +
		"by searching for their error string, not by picking `Generating` over " +
		"`Everything else`, so the buckets are an ordering of the index rather than " +
		"destinations. Every row already carries its own cross-reference into the " +
		"section that holds the detail, and those ARE in the TOC.",
}

// readmeTOCMinSubsections is the anti-vacuity floor for the widened gate: the
// number of `###` headings that must survive the exemptions above and really be
// checked.
//
// 🔴 It is what makes the exemption map safe to hold a rule instead of a list.
// The failure mode of a parent-keyed exemption is that it is TOO BROAD — naming
// `Generate` would silently retire 12 subsections and the gate would go on
// printing a green PASS over a document it had stopped reading. This floor turns
// that into a red run: the real in-scope count is 33, so exempting any of the
// substantial sections (`Generate` 12, `Command reference` 6, `Install` 5,
// `Scripting with --json` 5) drops it under the floor and the guard says so by
// name. It is set well below 33 so ordinary additions and deletions never trip
// it.
const readmeTOCMinSubsections = 25

// TestREADMETableOfContentsCoversEverySection requires each top-level (`##`)
// section AND each in-scope second-level (`###`) subsection to be reachable from
// the "## Contents" list. A TOC that silently stops covering the document is the
// state this whole change set exists to end, and it degrades one section at a
// time rather than all at once — so the check has to be per-section, not "a TOC
// exists".
//
// # Why `###` is in scope at all
//
// The gate used to stop at `##`, and the document rotted underneath it exactly
// where it could not look. `### Is your repo behind what you shipped?` shipped in
// #416 while its sibling `### Deployed is not the same as listed in the store`
// was listed, and nothing noticed the pair had gone half-covered. Widening it
// found two more of the same in one pass: `### 🔴 A checkpoint does not carry its
// ecosystem` — a money-safety subsection linked from NOWHERE in the file — and
// `### Reading a workflow's Buzz transactions`, both live in the README and both
// absent from a TOC that lists all ten of their siblings.
//
// # The rule, and what it deliberately does not cover
//
// Every `##` and every `###` is a TOC entry, EXCEPT the `###` children of the
// sections named in readmeTOCExemptSections — with a reason recorded there per
// section, and a count floor that reddens if the exemption set grows enough to
// hollow the gate out.
//
// Depth stops at `###`. `#### Which dotenv files end up in the bundle` is the
// file's only fourth-level heading and it is NOT required, because the TOC is a
// map of the document and a map that reproduces every leaf is the document. That
// bound comes from readmeHeadingRe/readmeSections, so a `####` cannot be required
// here and cannot be counted either.
//
// This asserts PRESENCE — that a heading has a TOC line pointing at its anchor.
// It says nothing about whether the TOC's ORDER matches the document's, or about
// whether the label beside the link still describes the section. Neither is a
// hole this guard can close, and both are stated so a green run is not read as
// "the TOC is correct".
func TestREADMETableOfContentsCoversEverySection(t *testing.T) {
	md := readREADME(t)
	linked := readmeContentsLinks(t, md)

	// A renamed or deleted exempt section must not silently widen the scope back
	// out (or, worse, narrow it: a stale key exempting nothing reads as a rule
	// still being applied). Both are checked before the exemption is honoured.
	sections := readmeSections(t, md)
	for name := range readmeTOCExemptSections {
		found, children := false, 0
		for _, s := range sections {
			if s.Level == 2 && s.Text == name {
				found = true
			}
			if s.Level == 3 && s.Parent == name {
				children++
			}
		}
		if !found {
			t.Errorf("readmeTOCExemptSections names the `## %s` section, which no longer exists in README.md. "+
				"Rename the key or drop it — an exemption for a section that is gone is a rule nobody can check.", name)
			continue
		}
		if children == 0 {
			t.Errorf("readmeTOCExemptSections exempts `## %s` from the `###` TOC requirement, but that section has no "+
				"`###` subsections. The exemption is exempting nothing — drop it.", name)
		}
	}

	var missing []string
	tops, subs := 0, 0
	for _, s := range sections {
		if s.Slug == "contents" {
			continue // the TOC does not list itself
		}
		switch s.Level {
		case 2:
			tops++
		case 3:
			if _, exempt := readmeTOCExemptSections[s.Parent]; exempt {
				continue
			}
			subs++
		}
		if !linked[s.Slug] {
			missing = append(missing, strings.Repeat("#", s.Level)+" "+s.Text+"  (#"+s.Slug+")")
		}
	}

	// Two floors, because the two levels fail independently: a heading scan that
	// lost `###` entirely would keep the `##` half green.
	if tops < 15 {
		t.Fatalf("found only %d top-level sections — the heading scan is wrong", tops)
	}
	if subs < readmeTOCMinSubsections {
		t.Fatalf("only %d `###` subsections are in scope, want at least %d — either the heading scan is wrong or "+
			"readmeTOCExemptSections has grown until this gate checks almost nothing. Exempting a section is a "+
			"decision about the README's navigation; making the gate vacuous is not.", subs, readmeTOCMinSubsections)
	}

	if len(missing) > 0 {
		t.Errorf("README sections missing from `## Contents`:\n  %s\n\n"+
			"Every `##` and every `###` needs a line in the TOC, except the `###` children of %v.",
			strings.Join(missing, "\n  "), readmeTOCExemptSectionNames())
	}
}

// readmeTOCExemptSectionNames returns the exempt section names, sorted, for use
// in a failure message.
func readmeTOCExemptSectionNames() []string {
	var names []string
	for name := range readmeTOCExemptSections {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// TestREADMETableOfContentsListsNothingElse is the REVERSE direction of the gate
// above, and it exists for the reason the layout ledger is bidirectional: a
// one-directional guard leaves half the rot in place. The forward half catches a
// section that grew without a TOC line; this half catches a TOC line that
// outlived its section, or that reaches into a section the rule says is not
// navigated that way.
//
// It is NOT a duplicate of TestREADMEAnchorLinksResolve. That guard asks whether
// a link resolves to SOME heading anywhere in the file — so it is satisfied by a
// TOC entry pointing at `### Exit code 3` or at a Troubleshooting bucket, both of
// which are the half-covered state readmeTOCExemptSections exists to forbid. This
// one asks whether the target is a heading the TOC is allowed to list at all.
func TestREADMETableOfContentsListsNothingElse(t *testing.T) {
	md := readREADME(t)
	linked := readmeContentsLinks(t, md)

	inScope := map[string]bool{}
	exempt := map[string]string{} // slug → the section it belongs to
	for _, s := range readmeSections(t, md) {
		if _, off := readmeTOCExemptSections[s.Parent]; off && s.Level == 3 {
			exempt[s.Slug] = s.Parent
			continue
		}
		inScope[s.Slug] = true
	}
	// Positive control: an empty in-scope set would report every TOC entry as
	// stale, which is loud — but an in-scope set built from the wrong text would
	// report a handful, which is not. The floor pins the set to the real document.
	if len(inScope) < readmeTOCMinSubsections {
		t.Fatalf("only %d headings are TOC-eligible — the section walk is reading the wrong text", len(inScope))
	}

	var stale, offLimits []string
	for slug := range linked {
		if inScope[slug] {
			continue
		}
		if parent, ok := exempt[slug]; ok {
			// The recorded reason is printed here rather than only living in the
			// map, because this is where a maintainer collides with the rule: a
			// message that says "not allowed" without saying why is a message
			// whose cheapest resolution is to delete the guard.
			offLimits = append(offLimits, slug+"\n      a `###` under `## "+parent+"`, which is exempt because "+
				readmeTOCExemptSections[parent])
			continue
		}
		stale = append(stale, slug)
	}
	sort.Strings(stale)
	sort.Strings(offLimits)

	if len(stale) > 0 {
		t.Errorf("the `## Contents` list links %d anchor(s) that are no longer a `##`/`###` heading: %v\n\n"+
			"A TOC entry that outlived its section renders as an ordinary link that goes nowhere. "+
			"Delete the line, or restore the heading.", len(stale), stale)
	}
	if len(offLimits) > 0 {
		t.Errorf("the `## Contents` list links %d subsection(s) of an exempt section:\n    %s\n\n"+
			"readmeTOCExemptSections holds those sections' `###` children OUT of the TOC wholesale, so a single "+
			"one listed there is the half-covered state the exemption exists to prevent. Either drop the line, or "+
			"drop the exemption and list every sibling.", len(offLimits), strings.Join(offLimits, "\n    "))
	}
}

// readmeCommandTableSubjects returns the first-column cell of every
// `| `civitai …` |` row inside the "## Command reference" table.
//
// Scoped to that table on purpose: the strings `civitai app list` and
// `civitai app pull` also appear in prose and in shell examples, so a
// whole-file search would report a command as "documented in the table" when it
// is merely mentioned somewhere.
func readmeCommandTableSubjects(t *testing.T, md string) []string {
	t.Helper()
	const heading = "\n## Command reference\n"
	i := strings.Index(md, heading)
	if i < 0 {
		t.Fatal("README.md has no `## Command reference` heading")
	}
	body := md[i+len(heading):]
	if j := strings.Index(body, "\n### "); j >= 0 {
		body = body[:j]
	}

	var subjects []string
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "| `civitai") {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) < 3 {
			continue
		}
		subjects = append(subjects, strings.TrimSpace(cells[1]))
	}
	return subjects
}

// TestREADMECommandTableDocumentsEveryAppSubcommand derives the expected rows
// from the COBRA TREE rather than a hardcoded list, so a command cannot be added
// to `civitai app` without being documented.
//
// It exists because three already had been: `app list`, `app view` and
// `app pull` were all shipped and reachable from `civitai app --help` while the
// README's command table named none of them (issue #260). `app pull` in
// particular carries a token-in-URL security warning that a README-only reader
// had no way to reach.
func TestREADMECommandTableDocumentsEveryAppSubcommand(t *testing.T) {
	root := NewRootCmd()
	appCmd, _, err := root.Find([]string{"app"})
	if err != nil {
		t.Fatalf("the `app` command group is not in the tree: %v", err)
	}

	subjects := readmeCommandTableSubjects(t, readREADME(t))
	// Positive control: zero extracted rows would make every lookup below fail
	// for the wrong reason (or, with a Contains-any check, pass for one).
	if len(subjects) < 15 {
		t.Fatalf("extracted only %d command-reference rows — the table extractor is reading the wrong block", len(subjects))
	}

	documented := func(want string) bool {
		for _, s := range subjects {
			if strings.Contains(s, want) {
				return true
			}
		}
		return false
	}

	checked := 0
	for _, sub := range appCmd.Commands() {
		name := sub.Name()
		if sub.Hidden || sub.IsAdditionalHelpTopicCommand() || name == "help" || name == "completion" {
			continue
		}
		checked++
		want := "civitai app " + name
		if !documented(want) {
			t.Errorf("`%s` is registered on the app command tree but has no row in the README's "+
				"command-reference table. Add one (and a section it can link to) — a command a "+
				"README-only reader cannot see is a command that does not exist for them.", want)
		}
	}
	// A tree that resolved to nothing would report a serene pass above.
	if checked < 10 {
		t.Fatalf("only %d app subcommands were checked — the tree walk is wrong", checked)
	}
}

// TestREADMEDocumentsTheAppStoreCredentialRequirement pins the specific false
// claim from issue #260: the README told readers that "reads are anonymous"
// while `civitai app list` / `app view` exit 3 without a token. The correction
// has to survive future edits to the quickstart, so it is asserted where a
// reader meets it — a dedicated section, reachable from the TOC.
func TestREADMEDocumentsTheAppStoreCredentialRequirement(t *testing.T) {
	md := readREADME(t)
	headings := readmeHeadings(t, md)
	if _, ok := headings["browse-the-app-store"]; !ok {
		t.Fatal("README.md has no `Browse the App store` section — `app list` / `app view` " +
			"and their login requirement have nowhere to be documented")
	}

	i := strings.Index(md, "\n## Browse the App store\n")
	body := md[i:]
	if j := strings.Index(body[1:], "\n## "); j >= 0 {
		body = body[:j+1]
	}
	for _, want := range []string{"civitai app list", "civitai app view", "civitai login"} {
		if !strings.Contains(body, want) {
			t.Errorf("the App-store section does not mention %q:\n%s", want, body)
		}
	}
	// The claim that matters: these reads are NOT anonymous.
	if !strings.Contains(body, "not anonymous") && !strings.Contains(body, "require a\ncredential") {
		t.Errorf("the App-store section does not state that these reads need a credential:\n%s", body)
	}
}
