package cmd

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// htmlToText converts the sanitized article-body HTML (TipTap/ProseMirror
// output) into readable plain text / lightweight markdown. It targets the tags
// Civitai article content uses: headings (h1–h6), paragraphs, line breaks,
// ordered/unordered lists, links, images, inline/blocked code, blockquotes,
// horizontal rules, and bold/italic emphasis. Any other tag is stripped but its
// text is kept; <script>/<style> content and HTML comments are dropped entirely.
//
// It parses HTML with the golang.org/x/net/html tokenizer rather than
// hand-rolled regexes, so malformed / unclosed / deeply-nested markup can never
// panic or loop, entities decode correctly (once — the tokenizer decodes text),
// and script/style bodies are not mistaken for renderable text.
//
// The final rendered string is passed through safeTerm (the shared terminal
// control-character sanitizer) so that no raw terminal control bytes an author
// embedded in the article can reach the user's terminal as ANSI/OSC escapes.
func htmlToText(body string) string {
	r := &htmlRenderer{}
	r.run(body)
	return safeTerm(r.result())
}

// listState tracks an open list's kind + item counter for nesting.
type listState struct {
	ordered bool
	index   int
}

// emphMark records an open emphasis marker and where it was written on the line,
// so its close can find the span's inner text (to drop empty spans) and coalesce
// with an immediately-adjacent same-kind span (to avoid stray "****"/"*****").
//
// mergePrev / prevMk / prevAt snapshot the merge state AT OPEN TIME so that
// nested emphasis (e.g. an empty <em> inside a <strong>) can't clobber the outer
// span's eligibility to coalesce with its preceding same-kind sibling.
type emphMark struct {
	marker    string
	pos       int  // index in r.line where this open marker begins
	mergePrev bool // opened exactly where a matching close ended (adjacent sibling)
	prevMk    string
	prevAt    int
}

type htmlRenderer struct {
	out    strings.Builder // finished block-level lines
	line   strings.Builder // the current in-progress line (inline content)
	lists  []listState     // open list stack (for nesting + ordered numbering)
	inItem bool            // currently between <li> and </li> (item text accumulates on line)
	quote  int             // blockquote nesting depth
	inPre  bool            // inside <pre> (preserve/monospace-fence)
	preBuf strings.Builder // buffered <pre> content
	href   string          // href of an open <a>, emitted as [text](href) on close
	aStart int             // len(line) when <a> opened, to wrap its text
	skip   int             // >0 while inside a <script>/<style> subtree (drop content)

	// pendingAnchorHref holds the href of the just-closed <a>, so an
	// immediately-following bare "[<href>]" text token (a common TipTap /
	// markdown-conversion artifact that duplicates the link's URL) can be
	// dropped instead of rendered a second time. One-shot: consumed by the next
	// text token, and invalidated by any intervening tag other than <br>.
	pendingAnchorHref string

	emph        []emphMark // open emphasis markers (**, *, `), innermost last
	lastCloseMk string     // marker of the most recently emitted close, "" if none/invalidated
	lastCloseAt int        // len(line) right after that close marker was written
}

// run drives the html tokenizer over body and dispatches text/tags. On EOF (or a
// read error) it flushes any open <pre>/list/paragraph so buffered content is
// never silently dropped.
func (r *htmlRenderer) run(body string) {
	z := html.NewTokenizer(strings.NewReader(body))
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			// EOF (or an unrecoverable read error): flush open state and stop.
			r.flushOpen()
			return
		case html.TextToken:
			if r.skip == 0 {
				// z.Text() returns already-entity-decoded text — do NOT decode again.
				r.text(string(z.Text()))
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			self := tt == html.SelfClosingTagToken
			name, hasAttr := z.TagName()
			n := string(name)
			if n == "script" || n == "style" {
				// Skip the entire raw-text subtree; a self-closing form has no body.
				if !self {
					r.skip++
				}
				continue
			}
			if r.skip > 0 {
				continue
			}
			r.tag(n, r.readAttrs(z, hasAttr), false, self)
		case html.EndTagToken:
			name, _ := z.TagName()
			n := string(name)
			if n == "script" || n == "style" {
				if r.skip > 0 {
					r.skip--
				}
				continue
			}
			if r.skip > 0 {
				continue
			}
			r.tag(n, nil, true, false)
		case html.CommentToken, html.DoctypeToken:
			// Dropped: never emit comment/doctype text.
		}
	}
}

// readAttrs collects a start tag's attributes into a lowercased-key map. The
// tokenizer returns already-decoded attribute values.
func (r *htmlRenderer) readAttrs(z *html.Tokenizer, hasAttr bool) map[string]string {
	if !hasAttr {
		return nil
	}
	m := map[string]string{}
	for {
		k, v, more := z.TagAttr()
		m[strings.ToLower(string(k))] = string(v)
		if !more {
			return m
		}
	}
}

// text appends already-decoded text to the current line (or the <pre> buffer).
func (r *htmlRenderer) text(decoded string) {
	if r.inPre {
		r.preBuf.WriteString(decoded)
		return
	}
	// Collapse runs of whitespace (incl. newlines) to single spaces outside <pre>.
	collapsed := wsRe.ReplaceAllString(decoded, " ")
	if collapsed == "" {
		return
	}
	// Drop a bare "[<href>]" that duplicates the URL of the link just closed
	// (TipTap article bodies commonly emit <a href="U">T</a><br />[U], which
	// would otherwise render as "[T](U) [U]"). Consume the one-shot flag first
	// so a non-matching token clears it too.
	if r.pendingAnchorHref != "" {
		bare := "[" + r.pendingAnchorHref + "]"
		r.pendingAnchorHref = ""
		if rest, ok := strings.CutPrefix(strings.TrimLeft(collapsed, " "), bare); ok {
			// Drop the separator space a <br>/whitespace inserted before the dup.
			if cur := r.line.String(); strings.HasSuffix(cur, " ") {
				r.line.Reset()
				r.line.WriteString(strings.TrimRight(cur, " "))
			}
			rest = strings.TrimLeft(rest, " ")
			if rest == "" {
				return
			}
			if r.line.Len() > 0 {
				rest = " " + rest
			}
			collapsed = rest
		}
	}
	// Avoid a leading space at the very start of a fresh line.
	if r.line.Len() == 0 {
		collapsed = strings.TrimLeft(collapsed, " ")
		if collapsed == "" {
			return
		}
	}
	r.line.WriteString(collapsed)
}

var wsRe = regexp.MustCompile(`\s+`)

func (r *htmlRenderer) tag(name string, attrs map[string]string, closing, selfClose bool) {
	// A pending bare-URL dedup only survives an intervening <br> (TipTap emits
	// <a>…</a><br />[url]); any other tag ends the anchor's immediate context.
	// closeAnchor re-arms it below for the <a>-closing case.
	if name != "br" {
		r.pendingAnchorHref = ""
	}
	switch name {
	case "br":
		if r.inItem {
			// Inside a list item, a <br> must not split the item into a plain line
			// + a mislabeled remainder — keep the text on the item's line, like <p>.
			if r.line.Len() > 0 && !strings.HasSuffix(r.line.String(), " ") {
				r.line.WriteString(" ")
			}
			return
		}
		r.flushLine()
	case "hr":
		r.flushLine()
		r.block("---")
	case "p", "div":
		if r.inItem {
			// Inside a list item, TipTap wraps the item's text in a <p>. Don't flush
			// it as a standalone paragraph (that dropped the list marker — bug #1);
			// keep the text on the item's line and separate stacked <p>s with a space.
			if r.line.Len() > 0 && !strings.HasSuffix(r.line.String(), " ") {
				r.line.WriteString(" ")
			}
			return
		}
		r.flushLine()
	case "h1", "h2", "h3", "h4", "h5", "h6":
		if closing {
			text := strings.TrimSpace(r.line.String())
			r.line.Reset()
			r.lastCloseMk = ""
			level := int(name[1] - '0')
			r.block(strings.Repeat("#", level) + " " + text)
		} else {
			r.flushLine()
		}
	case "ul", "ol":
		if closing {
			if r.inItem {
				// An <li> left open at list close (</li> is optional HTML and the
				// tokenizer does no implicit close): emit its text with a marker and
				// clear inItem, so later blocks don't glue onto the item's line.
				r.flushListItem()
			}
			if len(r.lists) > 0 {
				r.lists = r.lists[:len(r.lists)-1]
			}
			if len(r.lists) == 0 {
				r.flushLine()
			}
		} else {
			if r.inItem {
				// A nested list inside an <li>: flush the parent item's text (with its
				// marker) before descending, so it isn't lost or glued to the children.
				r.flushListItem()
			} else {
				r.flushLine()
			}
			r.lists = append(r.lists, listState{ordered: name == "ol"})
		}
	case "li":
		if closing {
			r.flushListItem()
		} else {
			r.flushLine()
			r.startListItem()
			r.inItem = true
		}
	case "a":
		if closing {
			r.closeAnchor()
		} else {
			r.href = attrs["href"]
			r.aStart = r.line.Len()
			// An anchor boundary must not coalesce emphasis across it (the anchor
			// rewrites the line on close, which would invalidate merged positions).
			r.lastCloseMk = ""
		}
	case "img":
		alt := attrs["alt"]
		src := attrs["src"]
		if src != "" {
			if alt == "" {
				alt = "image"
			}
			r.line.WriteString(fmt.Sprintf("![%s](%s)", alt, src))
		}
	case "strong", "b":
		r.emphasis("**", closing)
	case "em", "i":
		r.emphasis("*", closing)
	case "code":
		if !r.inPre { // inline code; a <code> inside <pre> is part of the block
			r.emphasis("`", closing)
		}
	case "pre":
		if closing {
			r.closePre()
		} else {
			r.flushLine()
			r.inPre = true
			r.preBuf.Reset()
		}
	case "blockquote":
		if closing {
			r.flushLine()
			if r.quote > 0 {
				r.quote--
			}
		} else {
			r.flushLine()
			r.quote++
		}
	default:
		// Unknown/other tag: strip it, keep its text (handled by text runs).
	}
}

// emphasis dispatches an emphasis tag (**, *, `) to its open/close handler.
func (r *htmlRenderer) emphasis(marker string, closing bool) {
	if closing {
		r.closeEmphasis(marker)
	} else {
		r.openEmphasis(marker)
	}
}

// openEmphasis writes an opening emphasis marker and remembers where it went so
// its close can inspect the span's inner text. It snapshots the merge anchor so a
// close can coalesce this span with an immediately-preceding same-kind span.
func (r *htmlRenderer) openEmphasis(marker string) {
	m := emphMark{
		marker: marker,
		pos:    r.line.Len(),
		prevMk: r.lastCloseMk,
		prevAt: r.lastCloseAt,
	}
	// Adjacent to a matching close (nothing but ignored tags between) → eligible to
	// merge on close, coalescing "</b><b>" instead of emitting "****".
	if r.lastCloseMk == marker && r.lastCloseAt == r.line.Len() {
		m.mergePrev = true
	}
	r.emph = append(r.emph, m)
	r.line.WriteString(marker)
}

// closeEmphasis finalises an emphasis span. It coalesces adjacent runs and drops
// empty/whitespace-only spans so abutting or empty <strong>/<em> never produce a
// malformed "****"/"*****" run (bug #2):
//   - a span whose inner text is empty or all-whitespace emits NO markers (the
//     bare whitespace, if any, is kept);
//   - a span opening exactly where the previous same-kind span closed is merged
//     into it, yielding one "**A B**" rather than "**A****B**".
func (r *htmlRenderer) closeEmphasis(marker string) {
	// Pop the matching open marker (emphasis nests, so it's the nearest one).
	open, ok := emphMark{}, false
	for i := len(r.emph) - 1; i >= 0; i-- {
		if r.emph[i].marker == marker {
			open = r.emph[i]
			r.emph = append(r.emph[:i], r.emph[i+1:]...)
			ok = true
			break
		}
	}
	if !ok {
		// Unbalanced close with no matching open: drop it (never emit a lone marker).
		return
	}
	line := r.line.String()
	if open.pos+len(marker) > len(line) {
		return
	}
	inner := line[open.pos+len(marker):]
	if strings.TrimSpace(inner) == "" {
		// Empty or whitespace-only span: strip the open marker, keep the whitespace,
		// emit no close marker. The span contributed nothing, so RESTORE the merge
		// anchor it saw at open time — a following same-kind sibling can still merge
		// with the span that preceded this empty one.
		r.line.Reset()
		r.line.WriteString(line[:open.pos] + inner)
		r.lastCloseMk = open.prevMk
		r.lastCloseAt = open.prevAt
		return
	}
	before := line[:open.pos]
	if open.mergePrev && strings.HasSuffix(before, marker) {
		// The previous same-kind span closed exactly where this one opened: merge the
		// two into a single span instead of emitting "</b><b>" as "****".
		merged := before[:len(before)-len(marker)] + inner + marker
		r.line.Reset()
		r.line.WriteString(merged)
		r.lastCloseMk = marker
		r.lastCloseAt = len(merged)
		return
	}
	r.line.WriteString(marker)
	r.lastCloseMk = marker
	r.lastCloseAt = r.line.Len()
}

// closeAnchor rewrites the anchor's inner text into [text](href).
func (r *htmlRenderer) closeAnchor() {
	full := r.line.String()
	if r.aStart > len(full) {
		r.aStart = len(full)
	}
	text := strings.TrimSpace(full[r.aStart:])
	base := full[:r.aStart]
	r.line.Reset()
	r.line.WriteString(base)
	switch {
	case r.href == "" && text == "":
		// nothing
	case r.href == "":
		r.line.WriteString(text)
	case text == "":
		r.line.WriteString(r.href)
	default:
		if bare, ok := selfLinkBare(text, r.href); ok {
			// Anchor text is just its own URL (bare link the editor auto-anchored).
			// Emit a single bare URL instead of doubling it as "[U](U)".
			r.line.WriteString(bare)
		} else {
			r.line.WriteString(fmt.Sprintf("[%s](%s)", text, r.href))
		}
	}
	// Arm bare-URL dedup: if this link carried a URL, a bare "[<href>]" text
	// token immediately after it is a duplicate to be dropped (see text()).
	r.pendingAnchorHref = r.href
	r.href = ""
	r.aStart = 0
	r.lastCloseMk = "" // line was rewritten — invalidate any pending merge anchor
}

// selfLinkBare reports whether an anchor's visible text is just its own URL —
// the extremely common case where an author pastes a bare link and the editor
// auto-anchors it as <a href="U">U</a>. Rendering that as "[U](U)" doubles the
// whole URL back-to-back, so on a match we collapse to a single bare href.
//
// The match tolerates (a) trailing sentence punctuation on the text (e.g. a
// "." the author typed after the pasted link) and (b) a trivial "https://" vs
// bare-host scheme difference. On a match it returns the href with the text's
// trailing punctuation re-appended (so "<a href=U>U.</a>" → "U." once), and
// reports false for a genuine "[text](url)" where the text differs from the URL.
func selfLinkBare(text, href string) (string, bool) {
	if href == "" || text == "" {
		return "", false
	}
	core := strings.TrimRight(text, ".,;:!?")
	trailer := text[len(core):]
	if normalizeURL(core) != normalizeURL(href) {
		return "", false
	}
	return href + trailer, true
}

// normalizeURL strips a leading http(s):// scheme and a trailing slash so two
// spellings of the same URL compare equal (used only for the self-link check).
func normalizeURL(u string) string {
	u = strings.TrimSpace(u)
	if rest, ok := strings.CutPrefix(u, "https://"); ok {
		u = rest
	} else if rest, ok := strings.CutPrefix(u, "http://"); ok {
		u = rest
	}
	return strings.TrimRight(u, "/")
}

// startListItem begins a list item, incrementing the ordered counter.
func (r *htmlRenderer) startListItem() {
	if len(r.lists) == 0 {
		r.lists = append(r.lists, listState{})
	}
	r.lists[len(r.lists)-1].index++
}

// flushListItem emits the current line as a list item with the right marker +
// indentation for its nesting depth.
func (r *htmlRenderer) flushListItem() {
	text := strings.TrimSpace(r.line.String())
	r.line.Reset()
	r.inItem = false
	r.lastCloseMk = "" // line reset — any pending merge anchor is now invalid
	if text == "" {
		return
	}
	depth := len(r.lists) - 1
	if depth < 0 {
		depth = 0
	}
	indent := strings.Repeat("  ", depth)
	ls := listState{}
	if len(r.lists) > 0 {
		ls = r.lists[len(r.lists)-1]
	}
	marker := "- "
	if ls.ordered {
		marker = fmt.Sprintf("%d. ", ls.index)
	}
	r.appendLine(indent + marker + text)
}

// closePre emits the buffered <pre> content as a fenced code block.
func (r *htmlRenderer) closePre() {
	r.inPre = false
	code := strings.Trim(r.preBuf.String(), "\n")
	r.preBuf.Reset()
	r.block("```\n" + code + "\n```")
}

// flushOpen flushes whatever block is in progress at EOF so unclosed markup never
// silently drops content: an open <pre> is fenced, an open list item is emitted
// with its marker, otherwise the in-progress paragraph line is flushed.
func (r *htmlRenderer) flushOpen() {
	switch {
	case r.inPre:
		r.closePre()
	case len(r.lists) > 0:
		r.flushListItem()
	default:
		r.flushLine()
	}
}

// flushLine trims trailing spaces and drops empty lines; a hanging emphasis /
// anchor marker on an otherwise-empty span is discarded.
func (r *htmlRenderer) flushLine() {
	text := strings.TrimRight(r.line.String(), " ")
	r.line.Reset()
	r.lastCloseMk = "" // line reset — any pending merge anchor is now invalid
	if strings.TrimSpace(text) == "" {
		return
	}
	if r.quote > 0 {
		text = strings.Repeat("> ", r.quote) + text
	}
	r.appendLine(text)
}

// block emits a standalone block separated by blank lines from its neighbours.
func (r *htmlRenderer) block(s string) {
	if r.out.Len() > 0 && !strings.HasSuffix(r.out.String(), "\n\n") {
		r.out.WriteString("\n")
	}
	r.out.WriteString(s)
	r.out.WriteString("\n\n")
}

// appendLine writes a single finished line.
func (r *htmlRenderer) appendLine(s string) {
	r.out.WriteString(s)
	r.out.WriteString("\n")
}

// result returns the rendered text with collapsed blank runs + a trailing
// newline stripped.
func (r *htmlRenderer) result() string {
	s := r.out.String()
	s = blankRunRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

var blankRunRe = regexp.MustCompile(`\n{3,}`)
