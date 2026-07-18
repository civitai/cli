package cmd

import (
	"strings"
	"testing"
)

// TestHTMLToTextRendersCommonTags exercises the article-body converter across the
// tag set Civitai content uses: headings, paragraphs, lists, links, emphasis,
// code, blockquotes, images, entities.
func TestHTMLToTextRendersCommonTags(t *testing.T) {
	body := `<h1>Getting Started</h1>` +
		`<p>Install <strong>ComfyUI</strong> &amp; download the model.</p>` +
		`<h2>Steps</h2>` +
		`<ul><li>Clone the repo</li><li>Run <code>main.py</code></li></ul>` +
		`<p>See the <a href="https://civitai.com/guide">full guide</a> for more.</p>` +
		`<blockquote>Tip: use a venv.</blockquote>` +
		`<pre><code>python main.py --listen</code></pre>`
	out := htmlToText(body)

	checks := map[string]string{
		"h1 heading":     "# Getting Started",
		"h2 heading":     "## Steps",
		"bold":           "**ComfyUI**",
		"decoded entity": "& download the model", // &amp; → &
		"list item 1":    "- Clone the repo",
		"inline code":    "`main.py`",
		"link":           "[full guide](https://civitai.com/guide)",
		"blockquote":     "> Tip: use a venv.",
		"code fence":     "```",
		"code body":      "python main.py --listen",
	}
	for name, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("%s: rendered text missing %q\n---\n%s", name, want, out)
		}
	}
	// No raw tags should survive.
	for _, tag := range []string{"<h1>", "<p>", "<strong>", "<ul>", "<li>", "<a ", "<code>", "&amp;"} {
		if strings.Contains(out, tag) {
			t.Errorf("raw markup %q leaked into output:\n%s", tag, out)
		}
	}
}

// TestHTMLToTextOrderedList proves ordered lists number their items.
func TestHTMLToTextOrderedList(t *testing.T) {
	out := htmlToText(`<ol><li>first</li><li>second</li><li>third</li></ol>`)
	for _, want := range []string{"1. first", "2. second", "3. third"} {
		if !strings.Contains(out, want) {
			t.Errorf("ordered list missing %q:\n%s", want, out)
		}
	}
}

// TestHTMLToTextImageAndBreak proves img → markdown image and <br> → newline.
func TestHTMLToTextImageAndBreak(t *testing.T) {
	out := htmlToText(`<p>line one<br>line two</p><p><img src="https://x/y.png" alt="diagram"></p>`)
	if !strings.Contains(out, "![diagram](https://x/y.png)") {
		t.Errorf("image not rendered as markdown:\n%s", out)
	}
	if !strings.Contains(out, "line one\nline two") {
		t.Errorf("<br> should produce a line break:\n%s", out)
	}
}

// TestHTMLToTextStripsUnknownTagsKeepsText proves unknown tags are dropped but
// their text content is retained, and numeric entities decode.
func TestHTMLToTextStripsUnknownTagsKeepsText(t *testing.T) {
	out := htmlToText(`<p><span class="x">kept</span> text &#39;quoted&#39; &lt;ok&gt;</p>`)
	if !strings.Contains(out, "kept text 'quoted' <ok>") {
		t.Errorf("unknown-tag text / entity decoding wrong:\n%s", out)
	}
	if strings.Contains(out, "<span") {
		t.Errorf("span tag leaked:\n%s", out)
	}
}

// TestHTMLToTextEmpty proves an empty body yields empty output (no panic).
func TestHTMLToTextEmpty(t *testing.T) {
	if got := htmlToText(""); got != "" {
		t.Errorf("empty body → %q, want empty", got)
	}
	if got := htmlToText("   \n  "); got != "" {
		t.Errorf("whitespace body → %q, want empty", got)
	}
}

// TestHTMLToTextSkipsScriptAndStyle proves the tokenizer drops the ENTIRE body of
// <script>/<style> elements — their inner text must never render — while the
// surrounding prose survives.
func TestHTMLToTextSkipsScriptAndStyle(t *testing.T) {
	body := `<p>before</p>` +
		`<script>alert('xss'); var leak = "SECRET_SCRIPT_BODY";</script>` +
		`<style>.evil{content:"SECRET_STYLE_BODY"}</style>` +
		`<p>after</p>`
	out := htmlToText(body)
	for _, want := range []string{"before", "after"} {
		if !strings.Contains(out, want) {
			t.Errorf("surrounding prose missing %q:\n%s", want, out)
		}
	}
	for _, bad := range []string{"SECRET_SCRIPT_BODY", "SECRET_STYLE_BODY", "alert", "content:"} {
		if strings.Contains(out, bad) {
			t.Errorf("script/style body leaked %q into output:\n%s", bad, out)
		}
	}
}

// TestHTMLToTextDropsComments proves HTML comments are dropped, never printed.
func TestHTMLToTextDropsComments(t *testing.T) {
	out := htmlToText(`<p>visible</p><!-- SECRET_COMMENT do not show --><p>tail</p>`)
	if !strings.Contains(out, "visible") || !strings.Contains(out, "tail") {
		t.Errorf("visible text lost around a comment:\n%s", out)
	}
	for _, bad := range []string{"SECRET_COMMENT", "<!--", "-->"} {
		if strings.Contains(out, bad) {
			t.Errorf("comment content %q leaked:\n%s", bad, out)
		}
	}
}

// TestHTMLToTextFlushesUnclosedPre proves an unclosed <pre> at EOF still flushes
// its buffered content as a fenced code block rather than silently dropping it.
func TestHTMLToTextFlushesUnclosedPre(t *testing.T) {
	out := htmlToText(`<p>intro</p><pre><code>never = closed()`)
	if !strings.Contains(out, "never = closed()") {
		t.Errorf("unclosed <pre> content was dropped:\n%s", out)
	}
	if !strings.Contains(out, "```") {
		t.Errorf("unclosed <pre> should still be fenced:\n%s", out)
	}
}

// TestHTMLToTextFlushesUnclosedList proves an unclosed <li>/<ul> at EOF flushes
// the pending item (with its marker) instead of dropping it.
func TestHTMLToTextFlushesUnclosedList(t *testing.T) {
	out := htmlToText(`<ul><li>closed item</li><li>dangling item`)
	for _, want := range []string{"- closed item", "- dangling item"} {
		if !strings.Contains(out, want) {
			t.Errorf("unclosed list dropped %q:\n%s", want, out)
		}
	}
}

// TestHTMLToTextFlushesUnclosedParagraph proves trailing text with no closing tag
// is still emitted.
func TestHTMLToTextFlushesUnclosedParagraph(t *testing.T) {
	out := htmlToText(`<p>first</p><p>trailing with no close`)
	if !strings.Contains(out, "trailing with no close") {
		t.Errorf("unclosed paragraph dropped:\n%s", out)
	}
}

// TestHTMLToTextMalformedNoPanic proves malformed / unclosed / deeply-nested /
// garbage input never panics or hangs — the tokenizer handles it by construction.
func TestHTMLToTextMalformedNoPanic(t *testing.T) {
	inputs := []string{
		`<p><b><i><u>unbalanced`,
		`<<>><p>>>weird<<`,
		`<a href=`,
		`<h1>heading without close`,
		`<ul><ol><ul><li>mixed nesting`,
		`<p>` + strings.Repeat("<div>", 500) + "deep" + strings.Repeat("</div>", 200),
		`<img src=`,
		`</p></p></p>stray closes`,
		`<pre><pre><pre>nested pre`,
		`&#xZZ; &notreal; &#; bad entities`,
		"\x00\x01\x02 raw controls in markup",
	}
	for _, in := range inputs {
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("panic on input %q: %v", in, p)
				}
			}()
			_ = htmlToText(in) // must not panic or hang
		}()
	}
	// Deeply nested content should still surface its text.
	deep := `<p>` + strings.Repeat("<div>", 500) + "buried" + strings.Repeat("</div>", 500) + "</p>"
	if !strings.Contains(htmlToText(deep), "buried") {
		t.Error("deeply-nested text was dropped")
	}
}

// TestHTMLToTextStripsControlChars is the security assertion for Fix 2: decoded
// article text carrying raw ESC / other C0 control bytes (entity-encoded, literal
// escape, or a raw byte) must not reach the terminal — only newline/tab survive.
func TestHTMLToTextStripsControlChars(t *testing.T) {
	// &#27; decodes to ESC (0x1b); \x1b[2J is a literal escape sequence; a raw ESC
	// byte is embedded directly. Visible text surrounds each.
	body := "<p>start &#27;[31mRED and \x1b[2J clear and " + "\x1b" + "raw ESC end</p>" +
		"<pre><code>code \x1b[1m bold \x07 bell</code></pre>" +
		`<p><a href="https://x/y` + "\x1b" + `[0m">link \x01 text</a></p>`
	out := htmlToText(body)

	// No C0 control chars (other than \n and \t) may survive anywhere.
	for _, ch := range out {
		if ch == '\n' || ch == '\t' {
			continue
		}
		if ch < 0x20 || ch == 0x7f {
			t.Errorf("control char %#x survived sanitization in:\n%q", ch, out)
		}
	}
	if strings.ContainsRune(out, 0x1b) {
		t.Errorf("ESC byte survived:\n%q", out)
	}
	// The surrounding VISIBLE text must survive the strip.
	for _, want := range []string{"start", "[31mRED", "[2J clear", "raw ESC end", "bold", "link", "text"} {
		if !strings.Contains(out, want) {
			t.Errorf("visible text %q lost to sanitization:\n%s", want, out)
		}
	}
	// Newlines (block structure) and tabs are preserved.
	if got := sanitizeControl("keep\tthis\nand\rdrop\x1bthat"); got != "keep\tthis\nanddropthat" {
		t.Errorf("sanitizeControl newline/tab/CR handling wrong: %q", got)
	}
}
