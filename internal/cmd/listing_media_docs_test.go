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
	//
	// ⚠ LOWERED 4 → 3 when the README's `app listing` walkthrough moved out to
	// the hosted store-listing guide (the README links it; the URL is deliberately
	// not repeated here — see docsURLSpellingLedger). This is a control on the
	// EXTRACTOR, not a claim that the README owes four invocations: the six
	// matches inside that walkthrough went with it, leaving the quickstart's
	// `set-icon ./assets/icon.png` + `set-cover ./assets/cover.png` and the
	// `set-icon <file>` placeholder in the exit-code-1 prose. The guard's
	// SUBSTANCE is untouched — the walkthrough's paths were all `assets/` too, so
	// the directory set it checks against every template is the same one. Do not
	// lower it again without re-deriving the real count: at 2 it would still see
	// `assets/`, at 1 it would be one edit away from checking nothing.
	if len(matches) < 3 {
		t.Fatalf("found only %d `civitai app listing set-*` invocations in README.md, want >= 3 — "+
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
		t.Fatalf("README.md has no %q heading", strings.TrimSpace(heading))
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

// --- the listing-media requirements prose, and where it now lives -------------

// 🔴 THE SUBJECT MOVED; THE GUARDS DID NOT. Until this change the three
// code↔prose drift assertions below read README.md's
// `### Listing media requirements`. That section is gone: the published
// store-listing guide carries the same two tables, the same behaviours and the
// same guidance-not-a-gate framing (verified by CONTENT before the deletion, not
// by heading), and AGENTS.md item 25 was amended to say so. What item 25 still
// forbids is unchanged and is the half that matters — none of those bounds may
// become a LOCAL CHECK. See claudedocs/decisions/25-listing-media-bounds.md.
//
// A hosted page cannot be an oracle for a hermetic test, so the assertions were
// RE-POINTED rather than deleted, at the copy of that prose which is still in
// this repository: the `assets/README.md` every template scaffolds into the
// author's project. Item 25 already named it as the second documented home.
//
// It is a stronger subject than the README section was in two ways: it is the
// file an author reads standing next to their artwork, and it is a SHIPPED
// artefact — written into somebody else's project and not recallable, which the
// README section was not.
//
// 🔴 IT IS *NOT* STRONGER FOR BEING THREE FILES, AND AN EARLIER VERSION OF THIS
// COMMENT CLAIMED IT WAS. "There are THREE copies, so each guard now covers three
// surfaces where it covered one" is a count of DECLARATIONS, not instances: the
// three `assets/README.md.tmpl` files are byte-identical (md5
// 51ebc5538e3bccbd14b2fbd93a928228, templates and rendered output alike), so the
// nine subtests below are THREE real comparisons run three times. Nothing pins
// that identity, so the claim could not even be checked.
//
// It matters because a simplification is queued: collapsing the three identical
// templates into one authority. A reader who believed the retracted sentence
// would price that at 3x coverage and decline it. It costs none.
//
// ⚠ ONE PROPERTY WAS DOWNGRADED AND IS LABELLED RATHER THAN GLOSSED.
// The quotation ban in TestListingRequirementsDocDoesNotPinAServerSentence was a
// REGRESSION guard — it was red on the tree that shipped the stale
// `*"That icon couldn't be read"*` paragraph. On its new subject it is an
// INVARIANT guard: the scaffolded READMEs never carried that defect. The rule it
// pins is the same rule and is worth pinning on the artefact that now carries the
// prose; the historical redness belongs to a file that no longer exists.

// listingRequirementsDocMinBytes is the anti-vacuity floor, carried over from the
// section extractor it replaces. A truncated or wrong file is comfortably under
// any assertion below and would report every check satisfied having read nothing.
const listingRequirementsDocMinBytes = 500

// listingRequirementsDocs renders each template's scaffolded `assets/README.md`
// — the surviving in-repo copy of the listing-media requirements — keyed by
// template name. It renders rather than reading the `.tmpl` because what an
// author is handed is the rendered file, and that is the artefact the claims are
// about.
func listingRequirementsDocs(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, tmpl := range scaffold.AllTemplates() {
		dest := filepath.Join(t.TempDir(), "app")
		if _, err := scaffold.Render(tmpl, dest, scaffold.Data{Slug: "app", Name: "App"}); err != nil {
			t.Fatalf("scaffold %s: %v", tmpl, err)
		}
		b, err := os.ReadFile(filepath.Join(dest, "assets", "README.md"))
		if err != nil {
			t.Fatalf("CONTROL failure, not a finding: template %q scaffolds no assets/README.md (%v).\n"+
				"That file is where the listing-media requirements live now that the README section was deleted; "+
				"without it every assertion below would be checking nothing.", tmpl, err)
		}
		if len(b) < listingRequirementsDocMinBytes {
			t.Fatalf("CONTROL failure, not a finding: template %q scaffolds an assets/README.md of only %d bytes "+
				"— that is a truncated or wrong file, not a requirements doc:\n%s", tmpl, len(b), b)
		}
		out[string(tmpl)] = string(b)
	}
	// POSITIVE CONTROL. An empty template set makes every loop below iterate over
	// nothing and report a serene pass.
	if len(out) == 0 {
		t.Fatal("CONTROL failure, not a finding: scaffold.AllTemplates() is empty, so no requirements doc was read")
	}
	return out
}

// listingDocCapRow returns the table row in a requirements doc that states one
// kind's byte cap, or "" when there is none.
//
// It anchors on `| <kind> (` rather than on the file-name column, because the
// file name is a suggestion (`icon.png`) while the KIND is the thing the cap
// belongs to. `| icon (` also cannot be satisfied by the `icon.png` in the first
// column, which is what makes the three kinds distinguishable.
func listingDocCapRow(doc, kind string) string {
	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(line, "|") && strings.Contains(line, "| "+kind+" (") && strings.Contains(line, "iB") {
			return line
		}
	}
	return ""
}

// TestScaffoldedListingByteCapsMatchTheCLIConstants is the code↔prose drift
// guard, re-pointed at the scaffolded requirements doc — see the note above.
//
// The dimension and aspect bounds in that doc are PLATFORM constants the CLI
// deliberately does not vendor (AGENTS.md item 25), so nothing here can check
// them — they are guidance, and the server's rejection message is the authority.
// The BYTE CAPS are different: those are this CLI's own local gate, so prose
// quoting a number the binary does not enforce is a plain lie a test can catch.
// That is the half that is checkable, so it is the half that is checked.
//
// 🔴 It is strictly wider than the README version: internal/scaffold's own
// assets_dir_test.go already asserts the caps as LITERALS ("2 MiB", "4 MiB"), so
// the two files agreed with each other and neither was tied to the constant. This
// one derives the expected string from maxIconBytes / maxCoverBytes /
// maxScreenshotBytes, which is the only direction that catches a cap change.
func TestScaffoldedListingByteCapsMatchTheCLIConstants(t *testing.T) {
	cases := []struct {
		kind string
		cap  int
	}{
		{"icon", maxIconBytes},
		{"cover", maxCoverBytes},
		{"screenshot", maxScreenshotBytes},
	}
	checked := 0
	for tmpl, doc := range listingRequirementsDocs(t) {
		t.Run(tmpl, func(t *testing.T) {
			for _, tc := range cases {
				row := listingDocCapRow(doc, tc.kind)
				if row == "" {
					t.Errorf("no `| %s (…) | … MiB …` row in template %q's assets/README.md — "+
						"a reader cannot see the byte cap the CLI will reject their file with:\n%s", tc.kind, tmpl, doc)
					continue
				}
				checked++
				want := fmt.Sprintf("%d MiB", tc.cap/(1024*1024))
				if !strings.Contains(row, want) {
					t.Errorf("template %q says %q for %s, but the CLI enforces %s (%d bytes). "+
						"The prose and the gate must agree — an author sizing to this file should never be refused locally.",
						tmpl, strings.TrimSpace(row), tc.kind, want, tc.cap)
				}
			}

			// The framing this doc exists to establish: the dimension table is NOT
			// a local check. Losing that is how the next reader concludes the CLI is
			// missing a validation and adds one.
			for _, want := range []string{"server-side", "platform"} {
				if !strings.Contains(doc, want) {
					t.Errorf("template %q's assets/README.md never says %q — the dimension bounds then read "+
						"as rules the CLI enforces", tmpl, want)
				}
			}
		})
	}
	// POSITIVE CONTROL on the whole sweep, not on one template: a run that
	// compared zero rows is indistinguishable from one that found them all correct.
	if checked == 0 {
		t.Fatal("CONTROL failure, not a finding: zero byte-cap rows were compared across every template. " +
			"The row matcher is broken, not the prose.")
	}
	t.Logf("compared %d byte-cap row(s) against the CLI's constants", checked)
}

// emphasisedQuotedSentenceRe matches a markdown-emphasised verbatim quotation —
// *"…"* — which in this prose is only ever used for one thing: reproducing the
// exact sentence the platform is expected to print back.
//
// It is a SHAPE, not a spelling. Banning the one message that rotted would be a
// guard against re-typing four particular words; banning the shape catches the
// next author who pins a different sentence the same way.
var emphasisedQuotedSentenceRe = regexp.MustCompile(`\*"[^"]+"\*`)

// TestListingRequirementsDocDoesNotPinAServerSentence is the doc-rot guard for
// the half of this prose that nothing else can check.
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
// 🔴 BOTH HALVES ARE INVARIANT GUARDS ON THIS SUBJECT, AND THAT IS A DOWNGRADE
// RECORDED RATHER THAN HIDDEN. Against the deleted README section the quotation
// ban was a REGRESSION guard — red on the tree that shipped the stale paragraph.
// The scaffolded docs never carried that defect, so here it can only hold a line
// that was already true. It is kept because the rule is the rule wherever the
// prose lives, and because deleting the advice is the other way to satisfy a ban.
func TestListingRequirementsDocDoesNotPinAServerSentence(t *testing.T) {
	// Validate the instrument before reading its verdict. A regex that cannot
	// match the shape it exists to find reports "clean" over anything.
	const probe = `it fails the icon decoder with *"That icon couldn't be read"* rather than`
	if !emphasisedQuotedSentenceRe.MatchString(probe) {
		t.Fatalf("the extractor does not match the shape it exists to find (%s) — "+
			"a clean verdict below would be a fact about the regex, not about the prose",
			emphasisedQuotedSentenceRe)
	}

	for tmpl, doc := range listingRequirementsDocs(t) {
		t.Run(tmpl, func(t *testing.T) {
			if hits := emphasisedQuotedSentenceRe.FindAllString(doc, -1); len(hits) > 0 {
				t.Errorf("template %q's assets/README.md pins %d verbatim server sentence(s): %q.\n"+
					"A quoted message is a claim about another repo's source that this one cannot check, and it "+
					"goes stale silently — civitai/civitai#3737 rewrote exactly such a message. Say what the "+
					"platform names in its rejection; do not reproduce the sentence.", tmpl, len(hits), hits)
			}

			// Invariant half: the de-pinning must not take the advice with it. An
			// author whose 5000×5000 icon is refused while every byte cap says they
			// are fine has no way to reach the answer from the tables alone.
			for _, want := range []string{"megapixel", "ownscale"} {
				if !strings.Contains(doc, want) {
					t.Errorf("template %q's assets/README.md no longer mentions %q — the pixel-ceiling "+
						"advice is the reason that bullet exists; de-pinning the server's wording must not "+
						"delete the guidance", tmpl, want)
				}
			}
		})
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
// `x` a code block prefers or the `×` the prose uses.
var quickstartDimensionRe = regexp.MustCompile(`(\d{3,4})\s*[x×]\s*(\d{3,4})`)

// TestQuickstartListingMinimumsAgreeWithTheirSources is the guard for the README
// copy of the listing-media numbers that SURVIVED the relocation.
//
// Step 7 of the quickstart tells an author to attach an icon and a cover. It
// inlines a summary of what the artwork has to be, which is the right call — the
// step that creates the work should say what the work is — and it creates the
// hazard every inlined constant creates: a copy that can drift from what it
// summarises.
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
//     the quickstart's copy agrees with the in-repo requirements doc it
//     summarises: the `assets/README.md` the same step tells the author to read.
//     🔴 The canonical side moved when `### Listing media requirements` was
//     deleted; the assertion did not. Every dimension the quickstart names must
//     appear in EVERY template's scaffolded doc — "every" rather than "some",
//     because an author picks one template and the quickstart does not say which.
func TestQuickstartListingMinimumsAgreeWithTheirSources(t *testing.T) {
	quick := readmeQuickstartAppBlock(t)
	docs := listingRequirementsDocs(t)

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

	// Half two: the dimensions, against the scaffolded requirements doc.
	dims := quickstartDimensionRe.FindAllStringSubmatch(quick, -1)
	// Positive control. Zero matches is what a reworded step 7, a changed
	// separator or a regex typo all look like, and it would pass over nothing.
	if len(dims) < 2 {
		t.Fatalf("found only %d dimension figures in quickstart step 7, want >= 2 (an icon and a cover "+
			"starting point). Either the inlined guidance is gone — in which case step 7 is back to "+
			"telling authors to 'save your own icon.png' with no idea what shape — or the extractor "+
			"is wrong (pattern: %s).\nquickstart:\n%s", len(dims), quickstartDimensionRe, quick)
	}
	for tmpl, doc := range docs {
		norm := strings.NewReplacer("×", "x", " ", "").Replace(doc)
		for _, d := range dims {
			want := d[1] + "x" + d[2]
			if !strings.Contains(norm, want) {
				t.Errorf("quickstart step 7 names the size %s×%s, which does NOT appear in template %q's "+
					"scaffolded assets/README.md. That file is the in-repo authority for these numbers (they "+
					"are the platform's — AGENTS.md item 25 — so nothing can check them against code); a "+
					"summary that has drifted from it is worse than no summary, because two places an author "+
					"reads in the same step now disagree.", d[1], d[2], tmpl)
			}
		}
	}
}

// TestListingCapRenderingAgreesWithTheDocsUnitSystem is the seam between the two
// surfaces that quote a byte cap at an author: `--help`, which renders it through
// humanBytes, and the requirements doc, which spells it out in a table.
//
// Each was internally consistent and they disagreed with each other: the table
// said "2 MiB" while `civitai app listing set-icon --help` said "at most 2.0 MiB"
// for the same 2,097,152 bytes. Neither TestScaffoldedListingByteCapsMatchTheCLIConstants
// (prose vs constant) nor TestListingHelpQuotesTheEnforcedCaps (help vs constant)
// can see it, because both compare against the constant and never against each
// other — the defect lives in the relationship, so the guard has to pin the
// relationship.
func TestListingCapRenderingAgreesWithTheDocsUnitSystem(t *testing.T) {
	cases := []struct {
		kind string
		cap  int
	}{
		{"icon", maxIconBytes},
		{"cover", maxCoverBytes},
		{"screenshot", maxScreenshotBytes},
	}
	checked := 0
	for tmpl, doc := range listingRequirementsDocs(t) {
		t.Run(tmpl, func(t *testing.T) {
			for _, tc := range cases {
				rendered := humanBytes(int64(tc.cap))
				unit := rendered[strings.LastIndex(rendered, " ")+1:]
				if unit == "" || unit == rendered {
					t.Fatalf("could not split a unit out of humanBytes(%d) = %q — the rest of this "+
						"test would compare against an empty string and pass", tc.cap, rendered)
				}
				row := listingDocCapRow(doc, tc.kind)
				if row == "" {
					// The sibling test owns "the row is missing"; here it means the
					// comparison could not be made, which must not read as agreement.
					t.Errorf("no byte-cap row for %s in template %q's assets/README.md — "+
						"the unit check could not run", tc.kind, tmpl)
					continue
				}
				checked++
				if !strings.Contains(row, unit) {
					t.Errorf("`civitai app listing` renders the %s cap as %q, but template %q's row never says %q — "+
						"the help and the docs quote the same constant in different unit systems, so an "+
						"author who sizes to one is surprised by the other.\nrow: %s",
						tc.kind, rendered, tmpl, unit, strings.TrimSpace(row))
				}
			}
		})
	}
	if checked == 0 {
		t.Fatal("CONTROL failure, not a finding: zero rows were compared — a guard that skipped every row " +
			"is not a guard that passed")
	}
	t.Logf("compared %d cap row(s) against humanBytes' unit system", checked)
}
