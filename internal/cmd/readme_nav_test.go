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

// TestREADMETableOfContentsCoversEverySection requires each top-level (`##`)
// section to be reachable from the "## Contents" list. A TOC that silently stops
// covering the document is the state this whole change set exists to end, and it
// degrades one section at a time rather than all at once — so the check has to
// be per-section, not "a TOC exists".
func TestREADMETableOfContentsCoversEverySection(t *testing.T) {
	md := stripFencedCode(readREADME(t))

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
	if len(linked) < 20 {
		t.Fatalf("the Contents section holds only %d links — the extractor is reading the wrong block", len(linked))
	}

	var missing []string
	sections := 0
	for _, m := range readmeHeadingRe.FindAllStringSubmatch(md, -1) {
		if m[1] != "##" {
			continue
		}
		slug := readmeAnchorSlug(m[2])
		if slug == "contents" {
			continue // the TOC does not list itself
		}
		sections++
		if !linked[slug] {
			missing = append(missing, m[2]+"  (#"+slug+")")
		}
	}
	if sections < 15 {
		t.Fatalf("found only %d top-level sections — the heading scan is wrong", sections)
	}
	if len(missing) > 0 {
		t.Errorf("README sections missing from `## Contents`:\n  %s", strings.Join(missing, "\n  "))
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
