// Shared logic for the "a custom property in a query condition is silently
// dead" guard.
//
// A CSS media/container query PRELUDE is not a declaration. It is evaluated
// against the environment before custom properties are substituted, so a
// `var()` inside one never resolves and the whole at-rule simply never applies:
//
//	@media (min-width: var(--civitai-bp-sm)) { … }   ← the block NEVER matches
//
// Nothing errors. No console warning, no build failure, no red test — the rule
// is just inert, and the layout is quietly stuck on whichever branch is outside
// the at-rule. Measured in Chromium against a literal-value control in the same
// document: with `--bp-sm` resolving to `768px` and a 1000px window, the literal
// `@media (min-width: 768px)` applied and the `var()` form did not.
//
// That makes it the same defect CLASS as a dead design token (designtokens.go):
// a plausible-looking reference that produces no signal at all. And it is the
// mistake a reader makes FIRST, because the `--civitai-bp-*` tokens exist and
// look exactly like the thing to put there. The scaffold now ships width-adaptive
// examples in two templates, so the wrong form is one copy-paste away.
//
// The invariant is therefore structural, not spelled: NO `var()` may appear in
// any `@media`/`@container` prelude anywhere under templates/. That catches the
// hazard however the token is named — including a token that does not exist yet,
// and including `--civitai-bp-*`'s eventual replacement — which a grep for the
// literal `var(--civitai-bp-` would not.
//
// Scope, stated rather than implied:
//   - `@supports` is deliberately NOT covered. Its condition tests DECLARATIONS,
//     where `var()` is legitimate (`@supports (color: var(--x))`).
//   - Comments are stripped before scanning, so documentation may spell the dead
//     form out — the examples in the templates do exactly that. See
//     StripCommentsForQueryScan for the one shape it does not strip.
package scaffold

import (
	"io/fs"
	"regexp"
	"strings"
)

// QueryPreludeRe matches a media/container query PRELUDE: the at-keyword up to
// the `{` that opens its block, which is captured in group 1.
//
// Two deliberate narrowings, both aimed at the same failure — a stray `@media`
// or `@container` in PROSE (a README sentence, a markdown mention) being read as
// code and swallowing an unrelated `var()` on its way to some distant brace:
//
//   - The trailing `\{` is REQUIRED. An at-rule always opens a block; a sentence
//     naming one almost never does. Measured on this tree, requiring it drops a
//     prose mention of "`@container` support" that was otherwise counted as a
//     prelude, without losing any real rule — including the one inside a
//     markdown ```css fence, which IS code and is checked.
//   - The length cap bounds the damage if a prose mention does happen to be
//     followed by a brace. A genuine prelude is a few dozen characters; 200 is
//     far above any of them and far below a runaway.
var QueryPreludeRe = regexp.MustCompile(`(?s)(@(?:media|container)\b[^{;]{0,200})\{`)

// QueryPreludeViolation is one prelude that carries a var() reference.
type QueryPreludeViolation struct {
	File    string // template-relative path
	Prelude string // the offending prelude, whitespace-collapsed for reporting
}

// QueryPreludeScan carries the coverage counts a caller needs to assert a
// POSITIVE CONTROL. This scanner's reassuring answer is an empty violation list
// — exactly what a scanner pointed at an empty tree, or one whose regex stopped
// matching, also produces. So "how many preludes did you actually see?" is part
// of the result, not a debug aside.
type QueryPreludeScan struct {
	Violations []QueryPreludeViolation
	FilesRead  int // every file walked
	Preludes   int // total @media/@container preludes seen, post-comment-strip
}

// ExtractQueryPreludes returns every media/container query prelude in src, with
// comments stripped first and whitespace collapsed. Order is source order.
func ExtractQueryPreludes(src string) []string {
	var out []string
	for _, m := range QueryPreludeRe.FindAllStringSubmatch(StripCommentsForQueryScan(src), -1) {
		out = append(out, collapseSpace(m[1]))
	}
	return out
}

// PreludeUsesVar reports whether a prelude references a custom property — the
// inert form.
func PreludeUsesVar(prelude string) bool {
	return strings.Contains(prelude, "var(")
}

// StripCommentsForQueryScan removes comment text so the prelude scan reads CODE,
// not prose. Without it this guard could not coexist with its own documentation:
// the templates deliberately spell the dead form out as a warning, and a scanner
// that could not tell a warning from a use would red on the very comment telling
// you not to do it.
//
// Two forms, chosen to strip LESS rather than more — an over-eager stripper
// creates false NEGATIVES, which is the failure direction that matters here:
//
//   - `/* … */` block comments, wherever they appear. This is the CSS form and
//     also the multi-line JS form.
//   - `// … EOL`, but ONLY when the line's first non-space characters are `//`.
//     A whole-line comment is unambiguous; a TRAILING `//` is not, because
//     `url(https://…)` and a string literal both contain one, and treating those
//     as comment starts would blind the scanner to the rest of that line.
//
// The residual, stated rather than papered over: a violation written as a
// TRAILING comment (`foo; // @media (min-width: var(--x))`) is NOT stripped and
// will be reported. That is fail-closed and the fix is to move the note onto its
// own line — the opposite bias to hiding a real one.
//
// Newlines inside a stripped block are preserved so line-based reasoning about
// the remaining text stays meaningful.
func StripCommentsForQueryScan(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	for _, line := range strings.SplitAfter(src, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "//") {
			// Drop the comment, keep the line break.
			if strings.HasSuffix(line, "\n") {
				b.WriteByte('\n')
			}
			continue
		}
		b.WriteString(line)
	}
	return stripBlockComments(b.String())
}

// stripBlockComments removes `/* … */` spans, preserving newlines. An unclosed
// `/*` swallows the remainder, which matches how a CSS parser reads it.
func stripBlockComments(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for {
		i := strings.Index(s, "/*")
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		rest := s[i+2:]
		j := strings.Index(rest, "*/")
		if j < 0 {
			// Unclosed: keep the newlines, drop the text.
			b.WriteString(strings.Repeat("\n", strings.Count(rest, "\n")))
			return b.String()
		}
		b.WriteString(strings.Repeat("\n", strings.Count(rest[:j], "\n")))
		s = rest[j+2:]
	}
}

var spaceRunRe = regexp.MustCompile(`\s+`)

func collapseSpace(s string) string {
	return strings.TrimSpace(spaceRunRe.ReplaceAllString(s, " "))
}

// ScanQueryPreludes walks fsys from root and reports every media/container query
// prelude that references a custom property, plus the scan's coverage counts.
func ScanQueryPreludes(fsys fs.FS, root string) (QueryPreludeScan, error) {
	var s QueryPreludeScan
	err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		raw, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		s.FilesRead++
		for _, prelude := range ExtractQueryPreludes(string(raw)) {
			s.Preludes++
			if PreludeUsesVar(prelude) {
				s.Violations = append(s.Violations, QueryPreludeViolation{File: p, Prelude: prelude})
			}
		}
		return nil
	})
	if err != nil {
		return QueryPreludeScan{FilesRead: s.FilesRead}, err
	}
	return s, nil
}
