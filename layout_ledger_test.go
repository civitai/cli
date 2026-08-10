package cli_test

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// AGENTS.md's "Layout (the non-obvious parts only)" section is the map a reader
// follows to find out what a package is FOR before touching it. Nothing checked
// that the map matched the territory, and it did not: the list named
// `internal/api` — a package DELETED on 2026-07-20 when #172 promoted it to the
// importable `pkg/civitai` SDK — while five packages that DO exist
// (`antipattern`, `blockproto`, `devtunnel`, `dnsprobe`, `ui`) were absent, three
// of them the subject of numbered items 10, 11 and 18. A reader who went looking
// for `internal/api` found nothing; a reader who wondered what `internal/ui` was
// got no answer from the file that exists to answer it.
//
// Both directions are silences, which is why neither was caught by review, and
// why this guard is BIDIRECTIONAL. A guard that only rejected a missing package
// would have left `api` sitting there; a guard that only rejected a stale entry
// would have left `ui` unmentioned.
//
// # What this guard checks, and what it deliberately does not
//
// It checks PRESENCE — that the set of package names the Layout section spells
// equals the set of directories under `internal/`. It says NOTHING about whether
// the description beside a name is true, or still true. That is not a hole this
// guard can close (no test can read English), and it is stated here so a green
// run is not read as "the Layout section is accurate".
//
// # Design decisions
//
// 1. THE SCOPE IS THE LAYOUT SECTION, NOT THE FILE. AGENTS.md is ~2600 lines and
// names most of these packages somewhere in the numbered items. If the guard
// scanned the whole file, a package would count as "documented" because item 20
// mentions it in passing — which is precisely the reader-hostile state this
// exists to prevent, since the Layout section is where a reader looks. The region
// is delimited structurally, heading to next heading, so ordinary edits inside it
// do not move the bounds.
//
// 2. IT READS CODE SPANS, NOT PROSE. A name only counts when it appears inside
// backticks, which is how every entry in that section is written. This keeps a
// sentence that happens to say "internal/foo" in running text from satisfying the
// ledger, and it is why the section's own note about the departed `api` package
// deliberately does not spell it as a path.
//
// 3. IT WALKS ONLY THE IMMEDIATE CHILDREN OF `internal/`. `go list ./internal/...`
// also reports `internal/scaffold/cmd/bump-pins`, a build-time tool nested inside
// a package that IS ledgered. The Layout section is about the top-level building
// blocks; requiring an entry for every nested tool would make it the exhaustive
// tree its own heading says it is not.
//
// 4. IT HAS A COUNT FLOOR IN BOTH DIRECTIONS. A locator that silently matched
// nothing — a renamed heading, a moved section — would report an empty ledger
// against an empty filesystem read and pass serenely. So both sets must be
// non-trivially populated before the comparison is believed. This is the
// "reassuring zero" the repo's rules warn about: without the floor, the most
// likely way for this guard to break is the way that looks like success.
const layoutSectionHeading = "## Layout (the non-obvious parts only)"

// layoutMinPackages is the floor for BOTH sets — the filesystem read and the
// parse. It is well under the real count (15 at the time of writing) so ordinary
// churn never trips it; it exists only to catch a read or a parse that returned
// nothing at all.
const layoutMinPackages = 8

// internalPkgRe pulls the first path segment after `internal/` out of a code
// span. It matches `internal/ui`, `internal/ui/CONVENTION.md` and each member of
// a brace expansion `internal/{a,b,c}` alike, because the section legitimately
// uses all three spellings.
var internalPkgRe = regexp.MustCompile(`internal/(\{[^}]*\}|[a-z][a-z0-9_]*)`)

// codeSpanRe matches a single-backtick code span. AGENTS.md uses no
// triple-backtick fences inside the Layout section, so the simple form is
// sufficient there.
//
// 🔴 IT ALLOWS ONE NEWLINE INSIDE THE SPAN, AND IT DID NOT FOR A RELEASE. The
// Layout section is hard-wrapped at ~79 columns like the rest of the file, and
// markdown lets a code span straddle a wrap (the line ending renders as a
// space). A pattern that excluded `\n` therefore stopped seeing a span the
// moment a reflow broke it — and the failure is SILENT IN THE DANGEROUS
// DIRECTION: the package drops out of the ledgered set, and the guard then
// reports "internal/<pkg> exists but the Layout section never names it" about a
// bullet that is sitting right there. A maintainer meeting that message reads it
// as a false positive and the cheapest fix is to delete the guard — which is the
// outcome this file's own doc comment says a docs guard must never invite.
// Worse, a span whose opening backtick lost its partner re-pairs with the NEXT
// backtick on the line, so one wrap can also invent a span that was never
// written.
//
// The newline is bounded to ONE on purpose. A span that wraps twice does not
// occur in this section, and an unbounded `[^`]+` would let a single stray
// backtick swallow whole paragraphs — which would satisfy the ledger from PROSE
// and quietly repeal design decision 2, the rule that a name counts only inside
// backticks. TestLayoutLedgerParserHandlesEverySpelling carries both the wrapped
// spellings and that bound.
var codeSpanRe = regexp.MustCompile("`([^`\n]+(?:\n[^`\n]+)?)`")

// spanWhitespaceRe and afterInternalRe normalise a span that wrapped. Markdown
// renders the line ending as a space, so the whitespace run (including the
// continuation line's indent) collapses to one — which is all `internal/{a, b}`
// needs, since the parser already accepts a space after the comma. The second
// rule handles the harsher case where the break landed INSIDE the path
// (`internal/` / `devtunnel`): a space there defeats internalPkgRe entirely, and
// re-joining is unambiguous because no real package name begins with a space.
var (
	spanWhitespaceRe = regexp.MustCompile(`\s+`)
	afterInternalRe  = regexp.MustCompile(`internal/\s+`)
)

// normaliseSpan undoes a hard wrap inside a code span. It is applied by
// ledgeredPackages and by the spelling corpus alike, so a narrowing of it fails
// the corpus by name instead of silently shrinking the ledger.
func normaliseSpan(span string) string {
	return afterInternalRe.ReplaceAllString(spanWhitespaceRe.ReplaceAllString(span, " "), "internal/")
}

func layoutSection(t *testing.T) string {
	t.Helper()

	b, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	doc := string(b)

	start := strings.Index(doc, layoutSectionHeading)
	if start < 0 {
		t.Fatalf("AGENTS.md no longer contains the heading %q — this guard's locator is broken, "+
			"which is NOT the same as the ledger being clean. Fix the locator or the heading.",
			layoutSectionHeading)
	}
	rest := doc[start+len(layoutSectionHeading):]

	end := strings.Index(rest, "\n## ")
	if end < 0 {
		t.Fatalf("no heading follows %q — the section is unbounded, so the region would swallow "+
			"the rest of the file and this guard would stop being scoped", layoutSectionHeading)
	}
	return rest[:end]
}

// parseInternalRefs pulls the internal/ package names out of one code span. It
// is the ONE parser: both the ledger below and the spelling corpus call it, so a
// narrowing of it fails the corpus by name rather than silently shrinking the
// ledger and blaming the filesystem.
func parseInternalRefs(span string, into map[string]bool) {
	for _, m := range internalPkgRe.FindAllStringSubmatch(span, -1) {
		name := m[1]
		if !strings.HasPrefix(name, "{") {
			into[name] = true
			continue
		}
		// Brace expansion: `internal/{scaffold,validate,…}`.
		for _, part := range strings.Split(strings.Trim(name, "{}"), ",") {
			if p := strings.TrimSpace(part); p != "" {
				into[p] = true
			}
		}
	}
}

// parseLayoutPackages is the WHOLE extraction — find the code spans, undo any
// hard wrap inside one, pull the package names out. It is the ONE pipeline: the
// ledger and the spelling corpus both call it, so a narrowing anywhere along it
// (the span pattern, the normaliser, the name regex) fails the corpus by name
// rather than silently shrinking the ledger and blaming the filesystem. Splitting
// the corpus off the first two stages is exactly how the newline blind spot
// survived — the corpus tested parseInternalRefs alone, and the defect was in
// the stage above it.
func parseLayoutPackages(md string, into map[string]bool) {
	for _, span := range codeSpanRe.FindAllStringSubmatch(md, -1) {
		parseInternalRefs(normaliseSpan(span[1]), into)
	}
}

// ledgeredPackages parses the package names the Layout section spells.
func ledgeredPackages(t *testing.T) map[string]bool {
	t.Helper()

	got := map[string]bool{}
	parseLayoutPackages(layoutSection(t), got)
	return got
}

// realPackages reads the immediate children of internal/ that hold Go code.
func realPackages(t *testing.T) map[string]bool {
	t.Helper()

	entries, err := os.ReadDir("internal")
	if err != nil {
		t.Fatalf("read internal/: %v", err)
	}

	got := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		files, err := os.ReadDir("internal/" + e.Name())
		if err != nil {
			t.Fatalf("read internal/%s: %v", e.Name(), err)
		}
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".go") {
				got[e.Name()] = true
				break
			}
		}
	}
	return got
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestLayoutSectionLedgersEveryInternalPackage(t *testing.T) {
	real := realPackages(t)
	ledgered := ledgeredPackages(t)

	// Positive controls. Either set coming back empty (or near-empty) means the
	// instrument is broken, and a broken instrument reports a clean ledger.
	if len(real) < layoutMinPackages {
		t.Fatalf("only %d package(s) found under internal/ (%v) — expected at least %d. "+
			"The filesystem read is broken; a comparison against it would pass for the wrong reason.",
			len(real), sortedKeys(real), layoutMinPackages)
	}
	if len(ledgered) < layoutMinPackages {
		t.Fatalf("only %d package name(s) parsed out of the Layout section (%v) — expected at least %d. "+
			"The parse is broken; every 'missing' report below would be noise and every 'stale' "+
			"report would be silence.", len(ledgered), sortedKeys(ledgered), layoutMinPackages)
	}

	for _, name := range sortedKeys(real) {
		if !ledgered[name] {
			t.Errorf("internal/%s exists but the Layout section of AGENTS.md never names it. "+
				"Add a bullet saying what it is FOR — that section is where a reader looks "+
				"before touching a package.", name)
		}
	}

	for _, name := range sortedKeys(ledgered) {
		if !real[name] {
			t.Errorf("the Layout section of AGENTS.md names internal/%s, which does not exist. "+
				"A map pointing at a deleted package is worse than no map: a reader greps for it, "+
				"finds nothing, and cannot tell a rename from their own mistake.", name)
		}
	}
}

// TestLayoutLedgerParserHandlesEverySpelling drives the parser against a control
// corpus rather than against AGENTS.md, so a future narrowing of the regex fails
// here by name instead of silently shrinking the ledger. Without it the parser
// could quietly stop understanding brace expansion — dropping six real packages
// from the ledgered set — and the guard above would then report six spurious
// "exists but is never named" errors that a maintainer would most cheaply
// "fix" by deleting this file.
func TestLayoutLedgerParserHandlesEverySpelling(t *testing.T) {
	for _, tc := range []struct {
		name string
		span string
		want []string
	}{
		{"plain", "internal/ui", []string{"ui"}},
		{"with a file suffix", "internal/ui/CONVENTION.md", []string{"ui"}},
		{"with an underscore", "internal/foo_bar", []string{"foo_bar"}},
		{"brace expansion", "internal/{a,b,c}", []string{"a", "b", "c"}},
		{"brace expansion with spaces", "internal/{a, b}", []string{"a", "b"}},
		{"two in one span", "internal/a + internal/b", []string{"a", "b"}},
		{"none", "pkg/civitai", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := map[string]bool{}
			parseInternalRefs(tc.span, got)
			if strings.Join(sortedKeys(got), ",") != strings.Join(tc.want, ",") {
				t.Errorf("parsing %q: got %v, want %v", tc.span, sortedKeys(got), tc.want)
			}
		})
	}
}

// TestLayoutLedgerSeesSpansThatWrap drives the FULL pipeline over markdown, not
// just the name regex, because that is where the blind spot was: the corpus
// above exercised parseInternalRefs and was serenely green while a code span
// broken across a wrap was invisible to the stage in front of it.
//
// Each case carries its own positive control asserting that the single-line span
// pattern this replaced really does lose the package — so a fixture that is not
// actually a wrap hazard fails here instead of sitting in the table looking like
// coverage.
func TestLayoutLedgerSeesSpansThatWrap(t *testing.T) {
	// singleLineSpanRe is the pattern that shipped: the control, kept here
	// deliberately so the hazard is pinned rather than described.
	singleLineSpanRe := regexp.MustCompile("`([^`\n]+)`")

	for _, tc := range []struct {
		name string
		md   string
		want []string
	}{
		{
			"wrap between two names in one brace expansion",
			"- `internal/{scaffold,validate,pkgzip,manifest,\n  config,auth}` — the building blocks.",
			[]string{"auth", "config", "manifest", "pkgzip", "scaffold", "validate"},
		},
		{
			"wrap after a comma-space inside a brace expansion",
			"- `internal/{devtunnel, \n  dnsprobe}` — `app dev-tunnel`.",
			[]string{"devtunnel", "dnsprobe"},
		},
		{
			"wrap inside the path itself",
			"- `internal/\n  blockproto` — the vendored ready-ack.",
			[]string{"blockproto"},
		},
		{
			"wrap between the path and its file suffix",
			"- `internal/ui/\n  CONVENTION.md` is the rule set.",
			[]string{"ui"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := map[string]bool{}
			parseLayoutPackages(tc.md, got)
			if strings.Join(sortedKeys(got), ",") != strings.Join(tc.want, ",") {
				t.Errorf("parsing %q\n got: %v\nwant: %v", tc.md, sortedKeys(got), tc.want)
			}

			// POSITIVE CONTROL for the control.
			old := map[string]bool{}
			for _, span := range singleLineSpanRe.FindAllStringSubmatch(tc.md, -1) {
				parseInternalRefs(normaliseSpan(span[1]), old)
			}
			if strings.Join(sortedKeys(old), ",") == strings.Join(tc.want, ",") {
				t.Errorf("fixture %q is not a wrap hazard: the single-line span pattern already returns the full answer %v, so this case proves nothing",
					tc.md, tc.want)
			}
		})
	}
}

// TestLayoutCodeSpanBoundIsOneNewline pins the bound in design decision 2's
// direction. An unbounded span pattern would let one stray backtick swallow
// paragraphs of prose and satisfy the ledger from running text — repealing the
// "backticks only" rule while every existing test stayed green, since widening
// can only ever ADD names to the ledgered set and the ledger's growth direction
// only fires on names that do not exist on disk.
func TestLayoutCodeSpanBoundIsOneNewline(t *testing.T) {
	// A lone backtick, then two paragraph breaks, then a closing backtick. A
	// pattern allowing unlimited newlines pairs these and reads `internal/nope`
	// out of the prose between them.
	md := "an opening ` that never closes on its line\n\nprose naming internal/nope in running text\n\nand a stray ` much later"
	got := map[string]bool{}
	parseLayoutPackages(md, got)
	if got["nope"] {
		t.Errorf("the span pattern paired backticks across %d lines and read a package name out of PROSE. "+
			"Design decision 2 says a name counts only inside a code span; an unbounded pattern repeals that silently.",
			strings.Count(md, "\n")+1)
	}
}
