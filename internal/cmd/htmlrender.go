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

type htmlRenderer struct {
	out    strings.Builder // finished block-level lines
	line   strings.Builder // the current in-progress line (inline content)
	lists  []listState     // open list stack (for nesting + ordered numbering)
	quote  int             // blockquote nesting depth
	inPre  bool            // inside <pre> (preserve/monospace-fence)
	preBuf strings.Builder // buffered <pre> content
	href   string          // href of an open <a>, emitted as [text](href) on close
	aStart int             // len(line) when <a> opened, to wrap its text
	skip   int             // >0 while inside a <script>/<style> subtree (drop content)
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
	switch name {
	case "br":
		r.flushLine()
	case "hr":
		r.flushLine()
		r.block("---")
	case "p", "div":
		r.flushLine()
	case "h1", "h2", "h3", "h4", "h5", "h6":
		if closing {
			text := strings.TrimSpace(r.line.String())
			r.line.Reset()
			level := int(name[1] - '0')
			r.block(strings.Repeat("#", level) + " " + text)
		} else {
			r.flushLine()
		}
	case "ul", "ol":
		if closing {
			if len(r.lists) > 0 {
				r.lists = r.lists[:len(r.lists)-1]
			}
			if len(r.lists) == 0 {
				r.flushLine()
			}
		} else {
			r.flushLine()
			r.lists = append(r.lists, listState{ordered: name == "ol"})
		}
	case "li":
		if closing {
			r.flushListItem()
		} else {
			r.flushLine()
			r.startListItem()
		}
	case "a":
		if closing {
			r.closeAnchor()
		} else {
			r.href = attrs["href"]
			r.aStart = r.line.Len()
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
		r.emphasis("**")
	case "em", "i":
		r.emphasis("*")
	case "code":
		if !r.inPre { // inline code; a <code> inside <pre> is part of the block
			r.emphasis("`")
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

// emphasis writes an emphasis marker; the tokenizer emits the open then the close
// tag so the surrounding pair balances.
func (r *htmlRenderer) emphasis(marker string) {
	r.line.WriteString(marker)
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
		r.line.WriteString(fmt.Sprintf("[%s](%s)", text, r.href))
	}
	r.href = ""
	r.aStart = 0
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
