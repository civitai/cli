package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/scaffold"
)

// listingMediaCmdRe matches a documented `civitai app listing set-icon|set-cover|
// add-screenshot <path>` invocation and captures the path argument.
//
// It is deliberately run over the RAW README, not the fenced-code-stripped copy
// the anchor guards use: every one of these invocations lives inside a ```bash or
// ```text block, so stripping fences would leave the guard matching nothing —
// which is indistinguishable from "the README is correct".
var listingMediaCmdRe = regexp.MustCompile(`civitai app listing (?:set-icon|set-cover|add-screenshot)\s+(\S+)`)

// TestREADMEListingMediaPathsExistInEveryScaffoldedProject is the seam guard
// between two surfaces that were each individually fine and broken together.
//
// The README's quickstart told authors to run
// `civitai app listing set-icon ./assets/icon.png` as step 7. No template
// created `assets/`. Measured on the binary built at 1eb4095, in a project
// freshly scaffolded by the CLI itself:
//
//	$ civitai app listing set-icon ./assets/icon.png
//	Error: no such file: ./assets/icon.png
//	exit=2
//
// Nothing was wrong with the command, and nothing was wrong with the scaffold.
// The defect lived in the relationship, so the guard has to pin the
// RELATIONSHIP: every relative path the README hands an author must have its
// parent directory present in a project the CLI actually scaffolds — for EVERY
// template, since an author picks one and the quickstart does not say which.
//
// 🔴 It asserts the DIRECTORY, never the file. The images themselves must not be
// scaffolded (a placeholder uploads cleanly to a public listing — see
// internal/scaffold/assets_dir_test.go and AGENTS.md item 25), so a guard
// demanding `assets/icon.png` would be a guard demanding the hazard.
func TestREADMEListingMediaPathsExistInEveryScaffoldedProject(t *testing.T) {
	matches := listingMediaCmdRe.FindAllStringSubmatch(readREADME(t), -1)

	// Positive control. A zero-match run is what a regex typo, a renamed
	// command or a wrong working directory all look like, and it would report a
	// serene pass over nothing.
	if len(matches) < 4 {
		t.Fatalf("found only %d `civitai app listing set-*` invocations in README.md, want >= 4 — "+
			"the extractor is reading the wrong text (pattern: %s)", len(matches), listingMediaCmdRe)
	}

	// Collect the distinct parent directories the README names. A path that is
	// obviously a stand-in (`<file>`, `$ICON`) is not a promise to the reader.
	dirs := map[string][]string{}
	for _, m := range matches {
		p := m[1]
		if strings.ContainsAny(p, "<>$*") || filepath.IsAbs(p) {
			continue
		}
		dir := filepath.Dir(filepath.Clean(p))
		if dir == "." {
			// The project root always exists; such a path makes no claim about
			// the scaffold. Recorded rather than silently dropped so the count
			// below stays honest.
			continue
		}
		dirs[dir] = append(dirs[dir], p)
	}
	if len(dirs) == 0 {
		t.Fatalf("every documented listing-media path resolved to the project root, so this guard checked nothing. "+
			"Either the README stopped naming a directory (in which case the quickstart no longer shows authors where artwork lives), "+
			"or the extractor is wrong. Paths seen: %v", matches)
	}

	var wantDirs []string
	for d := range dirs {
		wantDirs = append(wantDirs, d)
	}
	sort.Strings(wantDirs)

	for _, tmpl := range scaffold.AllTemplates() {
		t.Run(string(tmpl), func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "app")
			if _, err := scaffold.Render(tmpl, dest, scaffold.Data{Slug: "app", Name: "App"}); err != nil {
				t.Fatalf("scaffold %s: %v", tmpl, err)
			}
			for _, dir := range wantDirs {
				info, err := os.Stat(filepath.Join(dest, filepath.FromSlash(dir)))
				if err != nil || !info.IsDir() {
					t.Errorf("README documents %v, but template %q scaffolds no %q directory — "+
						"a reader who copies that command gets `Error: no such file` (exit 2). "+
						"Either scaffold the directory or stop naming the path.",
						dirs[dir], tmpl, dir)
				}
			}
		})
	}
}

// TestListingMediaShortsQuoteTheirOwnCap covers the surface #274's cap guard
// does not: `TestListingHelpQuotesTheEnforcedCaps` reads `c.Long` only, so a
// Short could quote a cap the command does not enforce and stay green.
//
// The Shorts carry a cap because they replaced "a square-ish image" / "a
// landscape hero image" (#270) — one-liners that stated a SHAPE the CLI does not
// check while omitting the format and size it does. Both are built by
// interpolating listingSourceRule(kind), so this asserts the interpolation picked
// the right kind, which is exactly the copy-paste #274 caught in the Long bodies.
//
// Scoped to icon and cover on purpose: their caps DIFFER (2.0 MiB vs 4.0 MiB), so
// "contains mine" and "does not contain the sibling's" are independent
// assertions. add-screenshot's Short carries no cap, and its cap string is
// byte-identical to the icon's today, so asserting on it would be #274's
// declared equivalent mutant — a guard that cannot fail.
func TestListingMediaShortsQuoteTheirOwnCap(t *testing.T) {
	root := NewRootCmd()
	listing, _, err := root.Find([]string{"app", "listing"})
	if err != nil {
		t.Fatalf("`app listing` is not in the command tree: %v", err)
	}

	cases := []struct {
		name    string
		kind    mediaKind
		foreign mediaKind
	}{
		{"set-icon", kindIcon, kindCover},
		{"set-cover", kindCover, kindIcon},
	}
	var checked int
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var short string
			for _, c := range listing.Commands() {
				if c.Name() == tc.name {
					short = c.Short
				}
			}
			if short == "" {
				t.Fatalf("`app listing %s` is not registered (or has no Short) — a rename must "+
					"update this test, not silently drop the row", tc.name)
			}
			checked++

			mine := humanBytes(int64(kindByteCap(tc.kind)))
			theirs := humanBytes(int64(kindByteCap(tc.foreign)))
			if mine == theirs {
				t.Fatalf("%s and %s render the same cap (%s), so this row cannot distinguish them — "+
					"pick a differently-capped sibling", tc.kind, tc.foreign, mine)
			}
			if !strings.Contains(short, mine) {
				t.Errorf("`app listing %s` Short (%q) does not quote the cap it enforces (%s) — "+
					"the one-liner in `app listing --help` is the only thing many readers see",
					tc.name, short, mine)
			}
			if strings.Contains(short, theirs) {
				t.Errorf("`app listing %s` Short (%q) quotes %s, the %s cap — "+
					"listingSourceRule was interpolated with the wrong kind",
					tc.name, short, theirs, tc.foreign)
			}
			if !strings.Contains(short, listingImageFormats) {
				t.Errorf("`app listing %s` Short (%q) does not state the accepted formats (%q)",
					tc.name, short, listingImageFormats)
			}
			// The regression itself: a bare shape adjective, stated as though the
			// CLI enforced it. Shape is the platform's and is attributed in Long.
			for _, banned := range []string{"square-ish", "landscape hero"} {
				if strings.Contains(short, banned) {
					t.Errorf("`app listing %s` Short (%q) is back to describing a SHAPE the CLI does "+
						"not check (%q). Shape is the platform's, validated at attach; the Short "+
						"states what the CLI enforces locally. See AGENTS.md item 25.",
						tc.name, short, banned)
				}
			}
		})
	}
	if checked != 2 {
		t.Fatalf("checked %d Shorts, want 2 — the command walk is wrong", checked)
	}
}

// readmeSectionSlice returns the README text of the section anchored at slug,
// raw (fences intact) so tables and sample output are visible to the caller.
func readmeSectionSlice(t *testing.T, md, heading string) string {
	t.Helper()
	i := strings.Index(md, heading)
	if i < 0 {
		t.Fatalf("README.md has no %q heading — the listing-media requirements have nowhere to live", strings.TrimSpace(heading))
	}
	body := md[i+len(heading):]
	// Bound at the next heading of the same or a higher level.
	for _, next := range []string{"\n## ", "\n### "} {
		if j := strings.Index(body, next); j >= 0 {
			body = body[:j]
		}
	}
	return body
}

// TestREADMEListingByteCapsMatchTheCLIConstants is a code↔prose drift guard.
//
// The dimension and aspect bounds in that section are PLATFORM constants the CLI
// deliberately does not vendor (AGENTS.md item 25), so nothing here can check
// them — they are guidance, and the server's rejection message is the authority.
// The BYTE CAPS are different: those are this CLI's own local gate, so the
// README quoting a number the binary does not enforce is a plain lie a test can
// catch. That is the half that is checkable, so it is the half that is checked.
func TestREADMEListingByteCapsMatchTheCLIConstants(t *testing.T) {
	section := readmeSectionSlice(t, readREADME(t), "\n### Listing media requirements\n")

	// Positive control on the extractor before reading anything out of it.
	if len(section) < 500 {
		t.Fatalf("the `Listing media requirements` section is only %d bytes — the section extractor is reading the wrong block:\n%s", len(section), section)
	}

	cases := []struct {
		kind string
		cap  int
	}{
		{"icon", maxIconBytes},
		{"cover", maxCoverBytes},
		{"screenshot", maxScreenshotBytes},
	}
	for _, tc := range cases {
		row := ""
		for _, line := range strings.Split(section, "\n") {
			if strings.HasPrefix(line, "| **"+tc.kind+"**") && strings.Contains(line, "MiB") {
				row = line
				break
			}
		}
		if row == "" {
			t.Errorf("no `| **%s** | … MiB …` row in the Listing media requirements section — "+
				"a reader cannot see the byte cap the CLI will reject their file with:\n%s", tc.kind, section)
			continue
		}
		want := fmt.Sprintf("%d MiB", tc.cap/(1024*1024))
		if !strings.Contains(row, want) {
			t.Errorf("README says %q for %s, but the CLI enforces %s (%d bytes). "+
				"The prose and the gate must agree — a reader sizing to the README should never be refused locally.",
				strings.TrimSpace(row), tc.kind, want, tc.cap)
		}
	}

	// The framing this section exists to establish: the dimension table is NOT a
	// local check. Losing that sentence is how the next reader concludes the CLI
	// is missing a validation and adds one.
	for _, want := range []string{"server-side", "platform"} {
		if !strings.Contains(section, want) {
			t.Errorf("the Listing media requirements section never says %q — the dimension bounds then read as rules the CLI enforces", want)
		}
	}
}

// emphasisedQuotedSentenceRe matches a markdown-emphasised verbatim quotation —
// *"…"* — which in this section is only ever used for one thing: reproducing the
// exact sentence the platform is expected to print back.
//
// It is a SHAPE, not a spelling. Banning the one message that rotted would be a
// guard against re-typing four particular words; banning the shape catches the
// next author who pins a different sentence the same way.
var emphasisedQuotedSentenceRe = regexp.MustCompile(`\*"[^"]+"\*`)

// TestREADMEDoesNotPinAServerSentenceForTheOversizedSource is the doc-rot guard
// for the half of this section that nothing else can check.
//
// The byte caps are the CLI's own constants, so the sibling test above can
// compare prose against code. The platform's REJECTION TEXT is not in this repo
// at all, so the only defence is not to quote it. That is not hypothetical: the
// README documented the oversized-source case by quoting the decoder's
// catch-all, *"That icon couldn't be read"*, and civitai/civitai#3737 replaced
// that message with one that names the pixel limit and the measured value. The
// paragraph was accurate when written and became false on a merge in another
// repo, with nothing here to notice.
//
// So the rule this pins: state what the platform DOES (it names the bound), not
// what it SAYS. A sentence is exactly the thing that moves.
//
// 🔴 The two halves are different kinds of guard and are labelled as such.
// The quotation ban is a REGRESSION guard — it is red on the tree that shipped
// the stale paragraph. The "advice survived" assertions are INVARIANT guards:
// they were already true before this fix, and they are here because deleting the
// paragraph is the other way to satisfy the ban.
func TestREADMEDoesNotPinAServerSentenceForTheOversizedSource(t *testing.T) {
	// Validate the instrument before reading its verdict. A regex that cannot
	// match the shape it exists to find reports "clean" over anything.
	const probe = `it fails the icon decoder with *"That icon couldn't be read"* rather than`
	if !emphasisedQuotedSentenceRe.MatchString(probe) {
		t.Fatalf("the extractor does not match the shape it exists to find (%s) — "+
			"a clean verdict below would be a fact about the regex, not about the README",
			emphasisedQuotedSentenceRe)
	}

	section := readmeSectionSlice(t, readREADME(t), "\n### Listing media requirements\n")
	if len(section) < 500 {
		t.Fatalf("the `Listing media requirements` section is only %d bytes — the section extractor is reading the wrong block:\n%s", len(section), section)
	}

	if hits := emphasisedQuotedSentenceRe.FindAllString(section, -1); len(hits) > 0 {
		t.Errorf("the Listing media requirements section pins %d verbatim server sentence(s): %q.\n"+
			"A quoted message is a claim about another repo's source that this one cannot check, and it "+
			"goes stale silently — civitai/civitai#3737 rewrote exactly such a message. Say what the "+
			"platform names in its rejection; do not reproduce the sentence.", len(hits), hits)
	}

	// Invariant half: the de-pinning must not take the advice with it. An author
	// whose 5000x5000 icon is refused while every byte cap says they are fine has
	// no way to reach the answer from the tables alone.
	for _, want := range []string{"megapixel", "ownscale"} {
		if !strings.Contains(section, want) {
			t.Errorf("the Listing media requirements section no longer mentions %q — the "+
				"pixel-ceiling advice is the reason this paragraph exists; de-pinning the "+
				"server's wording must not delete the guidance", want)
		}
	}
}

// readmeQuickstartAppBlock returns the `## Quickstart: build an App Block`
// section, raw — fences intact, because the listing-media advice this guard
// reads lives inside the quickstart's ```bash block.
func readmeQuickstartAppBlock(t *testing.T) string {
	t.Helper()
	md := readREADME(t)
	const heading = "\n## Quickstart: build an App Block\n"
	i := strings.Index(md, heading)
	if i < 0 {
		t.Fatal("README.md has no `## Quickstart: build an App Block` section")
	}
	body := md[i+len(heading):]
	if j := strings.Index(body, "\n## "); j >= 0 {
		body = body[:j]
	}
	return body
}

// quickstartDimensionRe matches a `512 x 512`-style figure, in either the ASCII
// `x` a code block prefers or the `×` the prose section uses.
var quickstartDimensionRe = regexp.MustCompile(`(\d{3,4})\s*[x×]\s*(\d{3,4})`)

// TestQuickstartListingMinimumsAgreeWithTheirSources is the guard for the FOURTH
// copy of the listing-media numbers.
//
// Step 7 of the quickstart tells an author to attach an icon and a cover. The
// requirements lived ~900 lines further down, so the step that creates the work
// said nothing about what the work is, and an author sized artwork by guessing.
// Inlining a summary fixes that and creates the hazard every inlined constant
// creates: a copy that can drift from what it summarises.
//
// So the summary is tied down from BOTH ends, and the two halves are different
// kinds of claim:
//
//   - The BYTE CAPS are this CLI's own constants (`maxIconBytes`,
//     `maxCoverBytes`), so they are checked against the code. A README quoting a
//     cap the binary does not enforce is a plain lie.
//   - The DIMENSIONS are PLATFORM constants the CLI deliberately does not vendor
//     (AGENTS.md item 25), so nothing here can check them against a gate — there
//     is none, and adding one is explicitly forbidden. What IS checkable is that
//     the quickstart's copy agrees with the canonical section it summarises. That
//     is the drift this inlining could actually introduce, so that is what is
//     pinned: every dimension the quickstart names must also appear in
//     `### Listing media requirements`, which stays the single authority.
func TestQuickstartListingMinimumsAgreeWithTheirSources(t *testing.T) {
	quick := readmeQuickstartAppBlock(t)
	canonical := readmeSectionSlice(t, readREADME(t), "\n### Listing media requirements\n")
	if len(canonical) < 500 {
		t.Fatalf("the `Listing media requirements` section is only %d bytes — the extractor is reading the wrong block", len(canonical))
	}

	// Half one: the byte caps, against the constants the CLI enforces.
	caps := []struct {
		kind string
		cap  int
	}{
		{"icon", maxIconBytes},
		{"cover", maxCoverBytes},
	}
	for _, tc := range caps {
		want := fmt.Sprintf("%d MiB", tc.cap/(1024*1024))
		if !strings.Contains(quick, want) {
			t.Errorf("quickstart step 7 does not quote the %s byte cap the CLI enforces (%s). "+
				"An author sizing artwork from the quickstart should never be refused locally by "+
				"a limit the quickstart did not mention.\nquickstart:\n%s", tc.kind, want, quick)
		}
	}
	// The two caps DIFFER (2 MiB vs 4 MiB), so "quotes both" is a real assertion
	// rather than one number satisfying two rows. Guard that premise: if they ever
	// converge, this test silently stops distinguishing them.
	if maxIconBytes == maxCoverBytes {
		t.Fatalf("maxIconBytes and maxCoverBytes are both %d, so quoting one satisfies both checks "+
			"above — this guard can no longer tell them apart", maxIconBytes)
	}

	// Half two: the dimensions, against the canonical section.
	dims := quickstartDimensionRe.FindAllStringSubmatch(quick, -1)
	// Positive control. Zero matches is what a reworded step 7, a changed
	// separator or a regex typo all look like, and it would pass over nothing.
	if len(dims) < 2 {
		t.Fatalf("found only %d dimension figures in quickstart step 7, want >= 2 (an icon and a cover "+
			"starting point). Either the inlined guidance is gone — in which case step 7 is back to "+
			"telling authors to 'save your own icon.png' with no idea what shape — or the extractor "+
			"is wrong (pattern: %s).\nquickstart:\n%s", len(dims), quickstartDimensionRe, quick)
	}
	canonicalNorm := strings.NewReplacer("×", "x", " ", "").Replace(canonical)
	for _, d := range dims {
		norm := d[1] + "x" + d[2]
		if !strings.Contains(canonicalNorm, norm) {
			t.Errorf("quickstart step 7 names the size %s×%s, which does NOT appear in "+
				"`### Listing media requirements`. That section is the authority (the numbers are the "+
				"platform's — AGENTS.md item 25 — so nothing can check them against code); a summary "+
				"that has drifted from it is worse than no summary, because two places now disagree.",
				d[1], d[2])
		}
	}
}

// TestListingCapRenderingAgreesWithTheREADMEUnitSystem is the seam between the
// two surfaces that quote a byte cap at an author: `--help`, which renders it
// through humanBytes, and the README table, which spells it out in prose.
//
// Each was internally consistent and they disagreed with each other: the table
// said "2 MiB" while `civitai app listing set-icon --help` said "at most 2.0 MiB"
// for the same 2,097,152 bytes. Neither TestREADMEListingByteCapsMatchTheCLIConstants
// (prose vs constant) nor TestListingHelpQuotesTheEnforcedCaps (help vs constant)
// can see it, because both compare against the constant and never against each
// other — the defect lives in the relationship, so the guard has to pin the
// relationship.
func TestListingCapRenderingAgreesWithTheREADMEUnitSystem(t *testing.T) {
	section := readmeSectionSlice(t, readREADME(t), "\n### Listing media requirements\n")
	if len(section) < 500 {
		t.Fatalf("the `Listing media requirements` section is only %d bytes — the section extractor is reading the wrong block:\n%s", len(section), section)
	}

	cases := []struct {
		kind string
		cap  int
	}{
		{"icon", maxIconBytes},
		{"cover", maxCoverBytes},
		{"screenshot", maxScreenshotBytes},
	}
	var checked int
	for _, tc := range cases {
		rendered := humanBytes(int64(tc.cap))
		unit := rendered[strings.LastIndex(rendered, " ")+1:]
		if unit == "" || unit == rendered {
			t.Fatalf("could not split a unit out of humanBytes(%d) = %q — the rest of this "+
				"test would compare against an empty string and pass", tc.cap, rendered)
		}
		row := ""
		for _, line := range strings.Split(section, "\n") {
			if strings.HasPrefix(line, "| **"+tc.kind+"**") && strings.Contains(line, "iB") {
				row = line
				break
			}
		}
		if row == "" {
			// The sibling test owns "the row is missing"; here it means the
			// comparison could not be made, which must not read as agreement.
			t.Errorf("no byte-cap row for %s in the Listing media requirements section — "+
				"the unit check could not run", tc.kind)
			continue
		}
		checked++
		if !strings.Contains(row, unit) {
			t.Errorf("`civitai app listing` renders the %s cap as %q, but the README row never says %q — "+
				"the help and the docs quote the same constant in different unit systems, so an "+
				"author who sizes to one is surprised by the other.\nrow: %s",
				tc.kind, rendered, unit, strings.TrimSpace(row))
		}
	}
	if checked != len(cases) {
		t.Fatalf("compared %d of %d caps — a guard that skipped rows is not a guard that passed", checked, len(cases))
	}
}
