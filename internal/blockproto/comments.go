package blockproto

// comments.go is the ONE comment stripper in this repo.
//
// It had three copies — `internal/validate/readyack.go`,
// `internal/scaffold/ready_ack_contract_test.go`, and the entry-graph resolver
// next door — asking the identical question: "does this file MENTION the token
// outside a comment?" A predicate open-coded at N sites is typically wrong at
// N-1 of them, so it lives here once, with the two rules that were each earned
// by a false warning at a correct project.

import "strings"

// MarkupExts are the extensions whose comments are `<!-- -->`. HTML comment
// stripping is applied ONLY to these.
//
// 🔴 Applying it to `.js`/`.ts` was a false-warning source: StripHTMLComments
// has no string awareness, so a perfectly ordinary `var OPEN = '<!--';` in a
// sanitiser started a "comment" that ran to end of file and deleted the emitter
// below it. Reproduced live on a `static` scaffold. JS has no HTML comments
// worth stripping here — `//` and `/* */` cover it, and Annex B's HTML-like
// comment form is vanishingly rare next to the literal appearing in a string.
var MarkupExts = map[string]bool{
	".html": true, ".htm": true, ".vue": true, ".svelte": true, ".astro": true,
}

// StripCommentsForExt removes comments from src, given its file extension. JS
// line/block comments are always stripped, leaving string and template literals
// intact (so `'https://x'` is not read as the start of a comment, and an emitter
// that posts from inside a string still counts). HTML comments are stripped only
// for markup files — see MarkupExts for why that gate exists.
func StripCommentsForExt(src, ext string) string {
	if MarkupExts[strings.ToLower(ext)] {
		src = StripHTMLComments(src)
	}
	return StripJSComments(src)
}

// StripHTMLComments removes `<!-- … -->` pairs.
func StripHTMLComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	for i := 0; i < len(src); {
		if strings.HasPrefix(src[i:], "<!--") {
			end := strings.Index(src[i+4:], "-->")
			if end < 0 {
				// 🔴 UNTERMINATED. Keep the remainder VERBATIM rather than
				// discarding it. Dropping the tail is lossy in the expensive
				// direction — everything below an unclosed `<!--` disappears,
				// including an emitter, and the author gets a warning about a
				// correct project. There is no upside to the lossy branch: text
				// after an unclosed marker is not evidence of a comment, it is
				// evidence the file is not what we assumed.
				b.WriteString(src[i:])
				return b.String()
			}
			i += 4 + end + 3
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}

// StripJSComments removes // and /* */ comments while respecting string and
// template literals, so a URL like 'https://x' is not mistaken for a comment
// and a comment mentioning a filename cannot satisfy a wiring check.
func StripJSComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			i += 2
			for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
				i++
			}
			i = min(i+2, len(src))
		case c == '\'' || c == '"' || c == '`':
			quote := c
			b.WriteByte(c)
			i++
			for i < len(src) {
				if src[i] == '\\' && i+1 < len(src) {
					b.WriteString(src[i : i+2])
					i += 2
					continue
				}
				b.WriteByte(src[i])
				if src[i] == quote {
					i++
					break
				}
				i++
			}
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}
