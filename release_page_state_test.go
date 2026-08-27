package cli_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// A release page in claudedocs/ states, in its FIRST LINE, whether that release
// has shipped. That heading is the only thing distinguishing an open page from a
// closed one — the body of a DRAFT and the body of a SHIPPED page are otherwise
// indistinguishable in an editor.
//
// 🔴 THIS GUARD EXISTS BECAUSE THAT HEADING WENT STALE THREE TIMES IN TWO DAYS,
// AND THE THIRD TIME COST A WHOLE PR TO UNDO.
//
//   - v0.1.101 shipped 2026-08-26T03:56:29Z. Its page kept saying `DRAFT` /
//     "Not yet tagged". Twenty hours later an audit reported the release notes
//     were stale about #498, and the fix was written INTO that closed page — a
//     table row, a prose section, a jq probe, every word of it false. #500
//     reverted it and moved the content to a v0.1.102 page.
//   - v0.1.99 shipped 2026-08-18T23:20:40Z and its page STILL said `DRAFT` when
//     this guard was written, eight releases later. Nobody noticed, because
//     nothing was looking. This test found it.
//
// The lesson generalises past release pages: a document that asserts a state its
// artifact has moved past reads as authoritative while being wrong, which is
// worse than saying nothing. Prose asking people to remember did not hold — the
// v0.1.102 page carried an explicit numbered step telling its author to flip the
// heading, and that step is exactly what a guard makes unnecessary to remember.
//
// 🔴 WHY THIS DOES NOT USE `git tag`, WHICH IS THE OBVIOUS IMPLEMENTATION.
//
// "Does tag vX.Y.Z exist?" is the question a reader expects this guard to ask.
// It is the WRONG primary mechanism here, and the reason is measured, not
// assumed: **CI checks out at depth 1 and a depth-1 clone carries NO TAGS.**
// `.github/workflows/ci.yml` sets no `fetch-depth:`, so `actions/checkout@v4`
// takes its default of 1; a `git clone --depth 1 file://...` of this repo
// resolves `git tag -l` to **0 entries** and cannot verify `refs/tags/v0.1.99`.
// A tag-based guard would therefore find every page "not yet tagged" and pass
// vacuously in the one tier that gates a merge, while looking green and
// authoritative in a developer's full clone. That is the config-blind suite
// failure: the environment silently decides which assertions can execute.
//
// So the PRIMARY invariant below is tag-free and depth-agnostic:
//
//	ONLY THE HIGHEST-VERSIONED RELEASE PAGE MAY BE HEADED `DRAFT`.
//
// A page is only opened for release N+1 once N has shipped, so any page with a
// higher-versioned sibling describes a release that is over. This runs
// identically at depth 1 and in a full clone.
//
// 🔴 AND HERE IS WHAT THE PRIMARY INVARIANT CANNOT SEE, stated plainly so no
// one reads this guard as wider than it is: it cannot catch the NEWEST page
// still saying DRAFT after that release ships, because there is no successor
// page yet. That is exactly the v0.1.101 case above. The tag tier below closes
// that gap wherever tags exist, and REPORTS ITS OWN COVERAGE so a run where it
// saw nothing is distinguishable from a run where it saw everything and found
// nothing. A skip that announces itself is recoverable; a silent one is not.
const releasePageDir = "claudedocs"

// releasePageRe matches the release-page filenames this guard governs.
var releasePageRe = regexp.MustCompile(`^release-v(\d+)\.(\d+)\.(\d+)-draft\.md$`)

// releaseHeadingRe pins the WHOLE first line, not a substring of it. A guard
// that merely greps for "DRAFT" is walkable by rewording the heading; pinning
// the entire normalised line means a cosmetic reword fails this test loudly
// instead of silently disabling it. The em dash is U+2014 and is deliberate —
// every existing page uses it.
var releaseHeadingRe = regexp.MustCompile(`^# v(\d+\.\d+\.\d+) — (DRAFT|SHIPPED)$`)

// minReleasePages is a positive control on the glob. If the directory moves or
// the naming convention changes, this guard would otherwise inspect zero files
// and pass — "no matches" and "nothing wrong" are the same output. There were
// 10 pages when this was written; requiring 8 leaves room to prune old ones
// without making the control a maintenance tax.
const minReleasePages = 8

type releasePage struct {
	file    string
	version [3]int
	verStr  string
	state   string // "DRAFT" or "SHIPPED"
}

// loadReleasePages parses every release page, failing loudly if the set or any
// heading cannot be parsed rather than skipping what it cannot read.
func loadReleasePages(t *testing.T) []releasePage {
	t.Helper()

	entries, err := os.ReadDir(releasePageDir)
	if err != nil {
		t.Fatalf("read %s: %v", releasePageDir, err)
	}

	var pages []releasePage
	for _, e := range entries {
		m := releasePageRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		path := filepath.Join(releasePageDir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		first, _, _ := strings.Cut(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")

		hm := releaseHeadingRe.FindStringSubmatch(first)
		if hm == nil {
			t.Fatalf("%s: first line %q does not match the pinned release-page heading %q.\n"+
				"Every release page must open with exactly `# vX.Y.Z — DRAFT` or `# vX.Y.Z — SHIPPED` "+
				"(em dash U+2014). This guard reads that line to decide whether the page is open or "+
				"closed, so a heading it cannot parse disables the check for that page — which is why "+
				"this is a hard failure and not a skip.",
				path, first, releaseHeadingRe)
		}

		var v [3]int
		for i := range 3 {
			n, err := strconv.Atoi(m[i+1])
			if err != nil {
				t.Fatalf("%s: unparseable version component %q: %v", path, m[i+1], err)
			}
			v[i] = n
		}
		verStr := fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2])

		// Cross-check: the heading must name the same version as the filename.
		// A page copied from its predecessor and half-edited is a real shape —
		// #500's whole subject was content landing on the wrong release page.
		if hm[1] != verStr {
			t.Errorf("%s: filename says v%s but the heading says `# v%s — %s`.\n"+
				"A page whose heading names a different release than its filename is how content "+
				"lands on the wrong release's notes.", path, verStr, hm[1], hm[2])
		}

		pages = append(pages, releasePage{file: path, version: v, verStr: verStr, state: hm[2]})
	}

	if len(pages) < minReleasePages {
		t.Fatalf("found %d release pages under %s/, want >= %d — POSITIVE CONTROL FAILED.\n"+
			"The glob %q matched almost nothing, so every assertion in this file is vacuous. "+
			"Either the pages moved, or the naming convention changed and this guard needs updating. "+
			"A zero here is indistinguishable from a clean run, which is why it fails instead.",
			len(pages), releasePageDir, minReleasePages, releasePageRe)
	}

	// Numeric semver ordering. Lexical sort is WRONG here and this repo has
	// already paid for that class once: "0.1.99" sorts after "0.1.100" as a
	// string, which would make v0.1.99 look like the newest page and exempt it
	// from the invariant below — silently un-finding the very bug that
	// motivated this guard.
	sort.Slice(pages, func(i, j int) bool {
		for k := range 3 {
			if pages[i].version[k] != pages[j].version[k] {
				return pages[i].version[k] < pages[j].version[k]
			}
		}
		return false
	})
	return pages
}

// TestOnlyTheNewestReleasePageMayBeDraft is the primary, depth-agnostic
// invariant. It runs identically in a full clone and in CI's depth-1 checkout.
func TestOnlyTheNewestReleasePageMayBeDraft(t *testing.T) {
	pages := loadReleasePages(t)
	newest := pages[len(pages)-1]

	var stale []string
	for _, p := range pages[:len(pages)-1] {
		if p.state == "DRAFT" {
			stale = append(stale, fmt.Sprintf("  %s: headed `# v%s — DRAFT`, but v%s is a newer release page",
				p.file, p.verStr, newest.verStr))
		}
	}

	if len(stale) > 0 {
		t.Fatalf("%d release page(s) still headed DRAFT despite a newer release existing:\n%s\n\n"+
			"A page is only opened for the next release once the previous one has shipped, so a page "+
			"with a higher-versioned sibling describes a release that is OVER.\n\n"+
			"Fix by flipping the heading to `# vX.Y.Z — SHIPPED` and folding in the measured outcome "+
			"(see release-v0.1.100-draft.md and release-v0.1.102-draft.md for the established table).\n"+
			"Do NOT fix it by deleting the page: the heading is the only marker distinguishing an open "+
			"release page from a closed one, and a closed page that still says DRAFT is what caused #500 "+
			"— a whole PR of false content written into a release that had already shipped.",
			len(stale), strings.Join(stale, "\n"))
	}

	t.Logf("checked %d release pages; newest is v%s (state %s, which may legitimately be either)",
		len(pages), newest.verStr, newest.state)
}

// TestATaggedReleasePageIsNotStillDraft closes the gap the primary invariant
// structurally cannot see: the NEWEST page still saying DRAFT after its release
// ships. It needs tags, which a depth-1 checkout does not have.
//
// 🔴 IT REPORTS ITS OWN COVERAGE RATHER THAN PASSING QUIETLY. A run that could
// resolve no tags proves nothing, and must not be readable as a clean bill of
// health — so it says so, in those words, in the test log. This is deliberately
// NOT a hard failure when tags are absent: CI is depth-1 by design, and a gate
// that is permanently red is one everyone learns to click through.
func TestATaggedReleasePageIsNotStillDraft(t *testing.T) {
	pages := loadReleasePages(t)

	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not on PATH (%v) — TAG TIER DID NOT RUN. This is NOT a clean result: "+
			"the newest release page could be stale and this run cannot tell.", err)
	}

	resolved, drafts := 0, []string(nil)
	for _, p := range pages {
		cmd := exec.Command("git", "rev-parse", "-q", "--verify", "refs/tags/v"+p.verStr)
		if err := cmd.Run(); err != nil {
			continue // tag not present in this clone (depth-1, or not yet tagged)
		}
		resolved++
		if p.state == "DRAFT" {
			drafts = append(drafts, fmt.Sprintf("  %s: tag v%s EXISTS, but the page is still headed DRAFT",
				p.file, p.verStr))
		}
	}

	if len(drafts) > 0 {
		t.Fatalf("%d release page(s) headed DRAFT for a version that is already TAGGED:\n%s\n\n"+
			"A tag means goreleaser has built that release. Flip the heading to SHIPPED and fold in "+
			"the measured outcome. If the tag exists but the release is still an unpublished draft on "+
			"GitHub, say so in the page body — but the heading must not keep claiming the work is open.",
			len(drafts), strings.Join(drafts, "\n"))
	}

	if resolved == 0 {
		t.Logf("🔴 TAG TIER SAW NOTHING: resolved 0 of %d release versions to a tag. This run is "+
			"NOT evidence that release headings are correct — it is evidence that this clone has no "+
			"tags, which is expected under CI's depth-1 checkout (ci.yml sets no fetch-depth, and a "+
			"depth-1 clone carries zero tags). TestOnlyTheNewestReleasePageMayBeDraft is the tier "+
			"that still ran. Run this in a full clone to exercise the tag check.", len(pages))
		return
	}

	t.Logf("tag tier resolved %d of %d release versions; none headed DRAFT", resolved, len(pages))
}

// TestReleasePageOrderingIsNumericNotLexical pins the comparison that decides
// which page is exempt from the DRAFT rule.
//
// 🔴 THIS EXISTS BECAUSE A MUTATION SURVIVED. Replacing the numeric semver
// compare in loadReleasePages with a plain string compare on verStr leaves
// TestOnlyTheNewestReleasePageMayBeDraft **GREEN while a stale page sits in the
// tree**: lexically "0.1.99" > "0.1.102", so v0.1.99 sorts last, is treated as
// the newest page, and is exempted by the very rule meant to catch it. Measured
// on 2026-08-27 against a tree where v0.1.99 was headed DRAFT — the primary tier
// passed, and only the tag tier caught it.
//
// That is not an acceptable division of labour: the tag tier CANNOT RUN IN CI
// (depth-1 checkouts carry no tags), so under the mutant the guard is fully
// blind exactly where it gates a merge. Hence a direct assertion here rather
// than reliance on a tier that is structurally absent from the gate.
func TestReleasePageOrderingIsNumericNotLexical(t *testing.T) {
	pages := loadReleasePages(t)

	// Positive control: the assertion below is only meaningful while the set
	// contains a pair whose lexical and numeric order DISAGREE (a shorter patch
	// like 0.1.99 against a longer one like 0.1.100). If every version were the
	// same width the two orderings would coincide and this test would pass
	// against a lexical implementation, proving nothing.
	disagrees := false
	for _, a := range pages {
		for _, b := range pages {
			sameMinor := a.version[0] == b.version[0] && a.version[1] == b.version[1]
			if sameMinor && a.version[2] < b.version[2] && a.verStr > b.verStr {
				disagrees = true
			}
		}
	}
	if !disagrees {
		t.Fatalf("POSITIVE CONTROL FAILED: no two release pages order differently under numeric vs "+
			"lexical comparison, so this test cannot distinguish the two implementations and would "+
			"pass against a lexical one. It needs a pair like v0.1.99 vs v0.1.100 to be meaningful; "+
			"the %d pages present do not provide one.", len(pages))
	}

	// Independent numeric max — deliberately NOT reusing the sort under test.
	want := pages[0]
	for _, p := range pages[1:] {
		switch {
		case p.version[0] != want.version[0]:
			if p.version[0] > want.version[0] {
				want = p
			}
		case p.version[1] != want.version[1]:
			if p.version[1] > want.version[1] {
				want = p
			}
		case p.version[2] > want.version[2]:
			want = p
		}
	}

	got := pages[len(pages)-1]
	if got.verStr != want.verStr {
		t.Fatalf("loadReleasePages treats v%s as the newest page, but the numerically highest version "+
			"is v%s.\nThe ordering is not numeric semver. A lexical sort puts v0.1.99 after v0.1.102 and "+
			"exempts it from TestOnlyTheNewestReleasePageMayBeDraft — which is the exact stale page that "+
			"guard was written to find.", got.verStr, want.verStr)
	}

	lexMax := pages[0].verStr
	for _, p := range pages {
		if p.verStr > lexMax {
			lexMax = p.verStr
		}
	}
	t.Logf("ordering is numeric: newest of %d pages is v%s (a lexical sort would have said v%s)",
		len(pages), want.verStr, lexMax)
}
