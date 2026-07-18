package cmd

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// htmlToText converts the sanitized article-body HTML (TipTap/ProseMirror
// output) into readable plain text / lightweight markdown. It is a deliberately
// small, dependency-free converter for the tags Civitai article content uses:
// headings (h1–h6), paragraphs, line breaks, ordered/unordered lists, links,
// images, inline/blocked code, blockquotes, horizontal rules, and bold/italic
// emphasis. Any other tag is stripped but its text is kept; HTML entities are
// decoded. It is NOT a general-purpose HTML parser — it targets the constrained
// subset the API emits, and degrades gracefully (unknown tags → their text) on
// anything else.
func htmlToText(body string) string {
	r := &htmlRenderer{}
	r.run(body)
	return r.result()
}

// tokenizer splits HTML into text runs and tags. A tag token has name+attrs and
// a closing flag; a text token carries decoded text.
var tagRe = regexp.MustCompile(`(?s)<(/?)([a-zA-Z][a-zA-Z0-9]*)((?:[^<>"']|"[^"]*"|'[^']*')*?)(/?)>`)

// hrefRe / srcRe pull an attribute value out of a tag's raw attribute string.
var (
	hrefRe = regexp.MustCompile(`(?i)\bhref\s*=\s*("([^"]*)"|'([^']*)'|(\S+))`)
	srcRe  = regexp.MustCompile(`(?i)\bsrc\s*=\s*("([^"]*)"|'([^']*)'|(\S+))`)
	altRe  = regexp.MustCompile(`(?i)\balt\s*=\s*("([^"]*)"|'([^']*)'|(\S+))`)
)

func attrValue(re *regexp.Regexp, attrs string) string {
	m := re.FindStringSubmatch(attrs)
	if m == nil {
		return ""
	}
	for _, g := range m[2:] {
		if g != "" {
			return html.UnescapeString(g)
		}
	}
	return ""
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
}

// run tokenizes body and dispatches text/tags.
func (r *htmlRenderer) run(body string) {
	idx := tagRe.FindAllStringSubmatchIndex(body, -1)
	pos := 0
	for _, m := range idx {
		if m[0] > pos {
			r.text(body[pos:m[0]])
		}
		closing := body[m[2]:m[3]] == "/"
		name := strings.ToLower(body[m[4]:m[5]])
		attrs := body[m[6]:m[7]]
		selfClose := body[m[8]:m[9]] == "/"
		r.tag(name, attrs, closing, selfClose)
		pos = m[1]
	}
	if pos < len(body) {
		r.text(body[pos:])
	}
	r.flushLine()
}

// text appends decoded text to the current line (or the <pre> buffer).
func (r *htmlRenderer) text(s string) {
	decoded := html.UnescapeString(s)
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

func (r *htmlRenderer) tag(name, attrs string, closing, selfClose bool) {
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
			r.href = attrValue(hrefRe, attrs)
			r.aStart = r.line.Len()
		}
	case "img":
		alt := attrValue(altRe, attrs)
		src := attrValue(srcRe, attrs)
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

// emphasis wraps subsequent inline text with a marker on open and close by simply
// writing the marker; the tokenizer emits open then close so the pair balances.
func (r *htmlRenderer) emphasis(marker string) {
	// Don't emphasize across an empty span.
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

// emphasis/anchor markers may leave a hanging marker with no content when a span
// is empty; flushLine trims trailing spaces and drops empty lines.
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
