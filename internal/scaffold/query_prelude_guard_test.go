package scaffold

// Structural guard: no scaffold template may put a `var()` inside a media or
// container query PRELUDE.
//
// Why this exists at all. Both width-adaptive examples the scaffold now ships
// sit right next to the `--civitai-bp-*` tokens, and putting one of those tokens
// in the query condition is the FIRST thing a reader tries. It does not work,
// and — this is the whole problem — it does not fail either. The prelude is
// evaluated before custom properties are substituted, so the at-rule is inert:
// green build, no console warning, no test failure, and a layout silently frozen
// on one branch. Measured in Chromium with a literal-value control in the same
// document (`--bp-sm` = 768px, 1000px window): the literal form applied, the
// `var()` form did not.
//
// Why it is a CLASS guard and not a grep. `grep -F 'var(--civitai-bp-'` pins a
// SPELLING: it passes the moment the tokens are renamed, says nothing about a
// `var()` naming some other property, and would have to be re-derived for every
// future token namespace. This asserts the RELATIONSHIP the browser actually
// enforces — a prelude may not depend on the cascade — so it holds however the
// property is named, including for properties that do not exist yet.
//
// `@supports` is deliberately out of scope: its condition tests DECLARATIONS,
// where `var()` is legitimate. See queryprelude.go.

import (
	"strings"
	"testing"
	"testing/fstest"
)

// TestScaffoldTemplatesHaveNoVarInQueryPreludes is the guard.
func TestScaffoldTemplatesHaveNoVarInQueryPreludes(t *testing.T) {
	scan, err := ScanQueryPreludes(templatesFS, "templates")
	if err != nil {
		t.Fatalf("scanning templates for query preludes: %v", err)
	}

	// ── POSITIVE CONTROLS ───────────────────────────────────────────────────
	// The reassuring answer here is "no violations", which is also what a
	// scanner wired to an empty tree, or one whose regex stopped matching,
	// returns. Assert it read this repo's templates AND found real preludes
	// before believing the zero.
	t.Logf("scanned %d template file(s); %d media/container prelude(s) survived comment-stripping; %d violation(s)",
		scan.FilesRead, scan.Preludes, len(scan.Violations))

	if scan.FilesRead < 20 {
		t.Fatalf("read only %d template file(s) — the scanner is not looking at this repo's templates, so a clean verdict here would be a statement about nothing", scan.FilesRead)
	}
	// Measured on this tree: 4 real preludes — the two boot-skeleton
	// `prefers-color-scheme` blocks (page-money + page-vite index.html),
	// page-vite's `@container` responsive rule, and the fenced copy of that rule
	// in page-vite's README. Every other textual occurrence is either inside a
	// comment (stripped) or prose that opens no block (not a prelude).
	//
	// This floor is what makes the comment-stripper's failure visible: a
	// stripper that ate too much would leave 0 here rather than reporting a
	// clean tree. If a change legitimately removes a query, lower this
	// DELIBERATELY — do not let the guard become a guard over nothing.
	if scan.Preludes < 3 {
		t.Fatalf("found only %d media/container prelude(s) in the templates — expected at least 3 (two boot-skeleton prefers-color-scheme blocks + page-vite's @container responsive rule). Either the templates lost their queries (lower this floor on purpose), QueryPreludeRe stopped matching, or StripCommentsForQueryScan is eating real code", scan.Preludes)
	}

	// ── THE INVARIANT ───────────────────────────────────────────────────────
	for _, v := range scan.Violations {
		t.Errorf(
			"DEAD QUERY CONDITION — this at-rule can NEVER match, silently.\n"+
				"  file:    %s\n"+
				"  prelude: %s\n"+
				"  A media/container query prelude is evaluated BEFORE custom properties are\n"+
				"  substituted, so the var() never resolves and the whole block is inert. There is\n"+
				"  no error, no warning and no build failure — the scaffolded app just renders one\n"+
				"  branch of the layout forever.\n"+
				"  fix: write the pixel value out (the block breakpoint scale is\n"+
				"       xs 480 · sm 768 · md 1024 · lg 1184 · xl 1440), or make the decision in JS\n"+
				"       with useBlockBreakpoint() from @civitai/blocks-react.",
			v.File, v.Prelude)
	}
}

// ── INSTRUMENT VALIDATION ───────────────────────────────────────────────────
// Everything above reads a verdict out of ExtractQueryPreludes /
// StripCommentsForQueryScan / ScanQueryPreludes. Until these controls have been
// watched to work, that verdict is a fact about the extractor.

// TestExtractQueryPreludes is the POSITIVE control on the extractor: fed input
// that MUST produce preludes, it produces exactly them — and fed the shapes that
// must NOT count, it produces none.
func TestExtractQueryPreludes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "a literal media query",
			in:   "@media (min-width: 768px) { .a { color: red } }",
			want: []string{"@media (min-width: 768px)"},
		},
		{
			name: "a named container query",
			in:   "@container block (min-width: 480px) {\n  .a { display: flex }\n}",
			want: []string{"@container block (min-width: 480px)"},
		},
		{
			name: "the DEAD form is extracted, so the guard can see it",
			in:   "@media (min-width: var(--civitai-bp-sm)) { .a { color: red } }",
			want: []string{"@media (min-width: var(--civitai-bp-sm))"},
		},
		{
			name: "@supports is NOT a query prelude — var() is legitimate there",
			in:   "@supports (color: var(--civitai-color-text)) { .a { color: red } }",
			want: nil,
		},
		{
			name: "a multi-line prelude is one prelude, whitespace collapsed",
			in:   "@media\n  (min-width: 768px)\n  and (orientation: landscape) {\n}",
			want: []string{"@media (min-width: 768px) and (orientation: landscape)"},
		},
		{
			name: "two preludes in one file",
			in:   "@media (min-width: 768px){}\n@container (min-width: 480px){}",
			want: []string{"@media (min-width: 768px)", "@container (min-width: 480px)"},
		},
		{
			name: "no at-rules at all",
			in:   ".a { color: var(--civitai-color-text) }",
			want: nil,
		},
		{
			// The narrowing that keeps README prose out. A sentence naming the
			// at-rule opens no block, so it is not a prelude — otherwise the
			// scanner would run 200 characters into the surrounding paragraph
			// hunting for a brace and could pick up an unrelated var() there.
			name: "prose naming an at-rule is not a prelude",
			in:   "a browser without `@container` support falls back to var(--civitai-color-text).",
			want: nil,
		},
		{
			// ...but a fenced CSS example in a README IS code, and a dead one
			// there teaches the mistake. It must still be checked.
			name: "a markdown-fenced rule is still a prelude",
			in:   "```css\n@container block (min-width: 480px) {\n  .a { color: red }\n}\n```",
			want: []string{"@container block (min-width: 480px)"},
		},
		{
			// `var()` in an ordinary declaration is the CORRECT use and must not
			// be confused with a prelude.
			name: "a declaration using var() is not a prelude",
			in:   "@media (min-width: 768px) { .a { color: var(--civitai-color-text) } }",
			want: []string{"@media (min-width: 768px)"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExtractQueryPreludes(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("ExtractQueryPreludes(%q) = %v, want %v", c.in, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("ExtractQueryPreludes(%q) = %v, want %v", c.in, got, c.want)
				}
			}
		})
	}
}

// TestPreludeUsesVar covers the predicate the guard branches on, in both
// directions, so a mutant that always answers one way dies here.
func TestPreludeUsesVar(t *testing.T) {
	if !PreludeUsesVar("@media (min-width: var(--civitai-bp-sm))") {
		t.Error("the dead form was not recognised as using var() — the guard would pass over every real violation")
	}
	if PreludeUsesVar("@container block (min-width: 480px)") {
		t.Error("a literal-value prelude was reported as using var() — the guard would red on correct code")
	}
}

// TestStripCommentsForQueryScan pins the stripper's contract in BOTH directions.
// It exists because the guard and its own documentation share a file: the
// templates deliberately spell the dead form out as a warning, so a stripper
// that under-strips reds on correct prose, and one that over-strips goes blind
// to real code.
func TestStripCommentsForQueryScan(t *testing.T) {
	t.Run("a whole-line // comment naming the dead form is stripped", func(t *testing.T) {
		src := "// never write @media (min-width: var(--civitai-bp-sm))\n.a { color: red }\n"
		if got := ExtractQueryPreludes(src); len(got) != 0 {
			t.Errorf("prose in a // comment was read as code: %v", got)
		}
	})

	t.Run("a /* */ block naming the dead form is stripped", func(t *testing.T) {
		src := "/* never write\n   @container block (min-width: var(--civitai-bp-xs))\n   here */\n.a { color: red }\n"
		if got := ExtractQueryPreludes(src); len(got) != 0 {
			t.Errorf("prose in a block comment was read as code: %v", got)
		}
	})

	t.Run("a JSX {/* */} comment is stripped", func(t *testing.T) {
		src := "{/* see the @container rule; never @media (min-width: var(--x)) */}\n"
		if got := ExtractQueryPreludes(src); len(got) != 0 {
			t.Errorf("prose in a JSX comment was read as code: %v", got)
		}
	})

	t.Run("REAL code after a stripped comment survives", func(t *testing.T) {
		// The over-stripping direction. If this ever returns nothing, the guard
		// has gone silently blind and its clean verdict means nothing.
		src := "/* a note about @media */\n@media (min-width: var(--civitai-bp-sm)) { .a { color: red } }\n"
		got := ExtractQueryPreludes(src)
		if len(got) != 1 || !PreludeUsesVar(got[0]) {
			t.Fatalf("real code following a comment was lost or misread: %v", got)
		}
	})

	t.Run("a URL is not a comment start", func(t *testing.T) {
		// `//` inside `https://` must not eat the rest of the line — that is the
		// exact over-strip that would hide a violation written after a URL.
		src := "/* https://developer.civitai.com/apps/responsive */\n" +
			"a { background: url(https://x/y.png) } @media (min-width: var(--x)) { .b { color: red } }\n"
		got := ExtractQueryPreludes(src)
		if len(got) != 1 || !PreludeUsesVar(got[0]) {
			t.Fatalf("a violation on the same line as a URL was lost: %v", got)
		}
	})

	t.Run("newlines are preserved through a stripped block", func(t *testing.T) {
		src := "a{}\n/* one\ntwo\nthree */\nb{}\n"
		if got, want := strings.Count(StripCommentsForQueryScan(src), "\n"), strings.Count(src, "\n"); got != want {
			t.Errorf("stripper changed the line count: %d, want %d", got, want)
		}
	})
}

// TestScanQueryPreludesNegativeControl is the NEGATIVE control: the scanner must
// be able to REPORT a violation. A scanner that returns nothing whatever you
// feed it is indistinguishable from a clean tree, so the clean verdict the real
// guard reports would mean nothing without this.
//
// It drives the same ScanQueryPreludes the guard uses, over a synthetic FS
// holding one correct rule, one dead one, and one comment that merely NAMES the
// dead one — and asserts it separates all three.
func TestScanQueryPreludesNegativeControl(t *testing.T) {
	fsys := fstest.MapFS{
		"templates/fake/ok.css.tmpl": {
			Data: []byte("@container block (min-width: 480px) { .a { color: var(--civitai-color-text) } }"),
		},
		"templates/fake/dead.css.tmpl": {
			Data: []byte("@media (min-width: var(--civitai-bp-sm)) { .a { color: red } }"),
		},
		"templates/fake/documented.css.tmpl": {
			Data: []byte("/* never write @media (min-width: var(--civitai-bp-sm)) */\n@media (min-width: 768px) { .a { color: red } }"),
		},
	}
	scan, err := ScanQueryPreludes(fsys, "templates")
	if err != nil {
		t.Fatalf("scanning synthetic FS: %v", err)
	}
	if scan.FilesRead != 3 {
		t.Fatalf("read %d file(s) from the synthetic FS, want 3", scan.FilesRead)
	}
	if scan.Preludes != 3 {
		t.Fatalf("found %d prelude(s) in the synthetic FS, want exactly 3 (one per file; the commented one must not count): %+v", scan.Preludes, scan.Violations)
	}
	if len(scan.Violations) != 1 {
		t.Fatalf("reported %d violation(s), want exactly 1 (only dead.css.tmpl) — the control did not separate live from dead: %+v", len(scan.Violations), scan.Violations)
	}
	if scan.Violations[0].File != "templates/fake/dead.css.tmpl" {
		t.Errorf("violation attributed to %q, want templates/fake/dead.css.tmpl", scan.Violations[0].File)
	}
	if !strings.Contains(scan.Violations[0].Prelude, "var(--civitai-bp-sm)") {
		t.Errorf("violation does not report the offending prelude: %q", scan.Violations[0].Prelude)
	}
}
