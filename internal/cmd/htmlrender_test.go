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

// TestHTMLToTextListItemsWrappedInParagraphs is the regression for bug #1: TipTap
// wraps each <li>'s text in a <p> (`<ul><li><p>…</p></li>…`). The inner <p> must
// not be flushed as a standalone paragraph — that dropped the list marker so items
// rendered as bare, run-together lines. Each item must keep its marker on its own
// line, for both unordered and ordered lists.
func TestHTMLToTextListItemsWrappedInParagraphs(t *testing.T) {
	ul := htmlToText(`<ul><li><p>Text-to-Image</p></li><li><p>Simple latent upscale</p></li></ul>`)
	if want := "- Text-to-Image\n- Simple latent upscale"; !strings.Contains(ul, want) {
		t.Errorf("<ul><li><p> items not marked / not on own lines:\ngot:\n%s\nwant substring:\n%s", ul, want)
	}
	ol := htmlToText(`<ol><li><p>first</p></li><li><p>second</p></li><li><p>third</p></li></ol>`)
	if want := "1. first\n2. second\n3. third"; !strings.Contains(ol, want) {
		t.Errorf("<ol><li><p> items not numbered / not on own lines:\ngot:\n%s\nwant substring:\n%s", ol, want)
	}
	// A multi-paragraph <li> keeps its single marker and doesn't glue words together.
	multi := htmlToText(`<ul><li><p>one</p><p>two</p></li></ul>`)
	if !strings.Contains(multi, "- one two") {
		t.Errorf("multi-paragraph <li> mishandled:\n%s", multi)
	}
}

// TestHTMLToTextCoalescesAdjacentEmphasis is the regression for bug #2: abutting or
// empty <strong>/<em> spans each emitted their own marker, producing malformed
// "****" / "*****" runs that broke mid-word. Adjacent same-kind spans must coalesce
// and empty / whitespace-only spans must emit no markers at all.
func TestHTMLToTextCoalescesAdjacentEmphasis(t *testing.T) {
	// No case may ever leave a "****" (or longer) run.
	noStray := []string{
		`<strong>A</strong><strong>B</strong>`,
		`text<strong></strong>`,
		`<strong></strong>text`,
		`<strong>does not</strong><strong> </strong>do:`,
		`<strong>We're up to </strong><strong>32 </strong><strong>LoRAs!</strong>`,
		`<strong><em>If helpful, </em></strong><strong>consider donating<em> </em></strong>`,
		`<em>a</em><em>b</em>`,
	}
	for _, body := range noStray {
		if out := htmlToText(body); strings.Contains(out, "****") {
			t.Errorf("stray '****' from %q:\n%s", body, out)
		}
	}

	// Adjacent bold runs merge into one span.
	if out := htmlToText(`<strong>A</strong><strong>B</strong>`); out != "**AB**" {
		t.Errorf("adjacent bold: got %q, want %q", out, "**AB**")
	}
	// Three adjacent bolds (with an ignored <span> shape) coalesce.
	if out := htmlToText(`<strong>We're up to </strong><strong>32 </strong><strong>LoRAs!</strong>`); out != "**We're up to 32 LoRAs!**" {
		t.Errorf("three adjacent bolds: got %q, want %q", out, "**We're up to 32 LoRAs!**")
	}
	// A trailing empty <strong> drops entirely — no marker survives.
	if out := htmlToText(`text<strong></strong>`); out != "text" {
		t.Errorf("empty trailing <strong>: got %q, want %q", out, "text")
	}
	// A whitespace-only <strong> between spans: no marker, words stay separated.
	if out := htmlToText(`<strong>does not</strong><strong> </strong>do:`); out != "**does not** do:" {
		t.Errorf("whitespace-only <strong>: got %q, want %q", out, "**does not** do:")
	}
}

// TestHTMLToTextNestedEmphasis proves nested emphasis renders correctly: <em>
// inside <strong> becomes ***…***, and a real-article shape mixing nested and
// empty emphasis never produces a stray "****"/"*****".
func TestHTMLToTextNestedEmphasis(t *testing.T) {
	if out := htmlToText(`<p><strong><em>bold italic</em></strong></p>`); out != "***bold italic***" {
		t.Errorf("strong>em: got %q, want %q", out, "***bold italic***")
	}
	if out := htmlToText(`<p><em><strong>italic bold</strong></em></p>`); out != "***italic bold***" {
		t.Errorf("em>strong: got %q, want %q", out, "***italic bold***")
	}
	// Real donation-line shape: nested + trailing whitespace-only emphasis.
	body := `<strong><em>If helpful, </em></strong><strong>consider donating<em> </em></strong>`
	if out := htmlToText(body); strings.Contains(out, "****") {
		t.Errorf("nested+empty emphasis produced a stray run:\n%s", out)
	}
}

// TestHTMLToTextWellFormedUnchanged is the golden: a clean, well-formed snippet
// (heading, paragraph with a single bold/italic/link, a wrapped list) renders to
// exactly the expected markdown — so the bug fixes don't regress good content.
func TestHTMLToTextWellFormedUnchanged(t *testing.T) {
	body := `<h2>Overview</h2>` +
		`<p>This uses <strong>ComfyUI</strong> and a <em>custom</em> workflow. See the <a href="https://civitai.com/g">guide</a>.</p>` +
		`<ul><li><p>step one</p></li><li><p>step two</p></li></ul>`
	want := "## Overview\n\n" +
		"This uses **ComfyUI** and a *custom* workflow. See the [guide](https://civitai.com/g).\n" +
		"- step one\n- step two"
	if out := htmlToText(body); out != want {
		t.Errorf("well-formed render changed:\ngot:\n%q\nwant:\n%q", out, want)
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
	if got := safeTerm("keep\tthis\nand\rdrop\x1bthat"); got != "keep\tthis\nanddropthat" {
		t.Errorf("safeTerm newline/tab/CR handling wrong: %q", got)
	}
}

// TestHTMLToTextSeparateBoldsStaySplit guards the highest-risk direction of the
// emphasis-coalescing fix: bolds with any intervening text/space must NOT merge.
func TestHTMLToTextSeparateBoldsStaySplit(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`<strong>A</strong> and <strong>B</strong>`, "**A** and **B**"},
		{`<strong>bold</strong> word <strong>bold</strong>`, "**bold** word **bold**"},
		{`<strong>A</strong> <strong>B</strong>`, "**A** **B**"},
	} {
		if out := htmlToText(tc.in); out != tc.want {
			t.Errorf("separate bolds from %q: got %q, want %q", tc.in, out, tc.want)
		}
	}
}

// TestHTMLToTextMalformedListState covers the two robustness gaps a #159 audit
// found: an <li> left open at list close must not leak `inItem` onto following
// blocks, and a <br> inside a list item must not split off a mislabeled tail.
func TestHTMLToTextMalformedListState(t *testing.T) {
	// Missing </li>: content after the list must land on its own lines, not glue.
	out := htmlToText(`<ul><li><p>x</p></ul><p>A</p><p>B</p>`)
	for _, want := range []string{"- x", "A", "B"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing-</li> leak: %q absent:\n%s", want, out)
		}
	}
	if strings.Contains(out, "A B") {
		t.Errorf("blocks after a malformed list must not glue onto one line:\n%s", out)
	}
	// <br> inside a list item keeps the item together (no bullet on the tail).
	if got := htmlToText(`<ul><li><p>a<br>b</p></li></ul>`); got != "- a b" {
		t.Errorf("<br> in <li>: got %q, want %q", got, "- a b")
	}
}

// TestHTMLToTextDropsDuplicateBareURL guards the fix for the double-URL bug:
// TipTap article bodies commonly emit <a href="U">T</a><br />[U], where the bare
// [U] duplicates the link's URL. The link must render exactly once as [T](U) with
// NO trailing [U] appended.
func TestHTMLToTextDropsDuplicateBareURL(t *testing.T) {
	const u = "https://medium.com/x-c88a8658beb7"

	// The exact article-2054 shape: anchor, <br>, bare [url], all inside <li><p>.
	got := htmlToText(`<li><p><a href="` + u + `">Part 1</a><br />[` + u + `]</p></li>`)
	if want := "- [Part 1](" + u + ")"; got != want {
		t.Fatalf("duplicate bare URL not dropped:\ngot:  %q\nwant: %q", got, want)
	}
	if strings.Contains(got, "["+u+"]") {
		t.Errorf("bare [url] survived:\n%q", got)
	}

	// Plain paragraph form: <a>…</a> [U] with a space separator.
	if out := htmlToText(`<p><a href="` + u + `">text</a> [` + u + `]</p>`); out != "[text]("+u+")" {
		t.Errorf("space-separated dup: got %q, want %q", out, "[text]("+u+")")
	}

	// The exact-once assertion from the task: <a href="U">T</a> renders [T](U),
	// never with a trailing " [U]".
	if out := htmlToText(`<a href="` + u + `">T</a>[` + u + `]`); out != "[T]("+u+")" {
		t.Errorf("adjacent dup (no separator): got %q, want %q", out, "[T]("+u+")")
	}

	// A link whose visible text already IS the URL collapses to a single bare URL
	// (not "[U](U)"), and the trailing bare-URL dup is still dropped.
	if out := htmlToText(`<p><a href="` + u + `">` + u + `</a><br />[` + u + `]</p>`); out != u {
		t.Errorf("text==url dup: got %q, want %q", out, u)
	}
	// …and the same link with no trailing bare URL also renders as a single URL.
	if out := htmlToText(`<p><a href="` + u + `">` + u + `</a></p>`); out != u {
		t.Errorf("text==url no-dup: got %q, want %q", out, u)
	}
}

// TestHTMLToTextSelfLinkNotDoubled guards the fix for the self-link double-URL
// bug: <a href="U">U</a> (a bare link the editor auto-anchored) must render as a
// single bare "U", never "[U](U)". The match tolerates trailing sentence
// punctuation and a trivial https:// vs bare-host scheme difference, while a
// genuine [text](url) with text != url is left untouched.
func TestHTMLToTextSelfLinkNotDoubled(t *testing.T) {
	const u = "https://civitai.com/models/1234"

	// text == href → single bare URL.
	if out := htmlToText(`<a href="` + u + `">` + u + `</a>`); out != u {
		t.Errorf("self-link doubled: got %q, want %q", out, u)
	}
	// Inside a heading the same collapse applies (no "[U](U)" in ## headings).
	if out := htmlToText(`<h2><a href="` + u + `">` + u + `</a></h2>`); out != "## "+u {
		t.Errorf("self-link in heading doubled: got %q, want %q", out, "## "+u)
	}
	// Trailing punctuation on the text is preserved once, appended to the URL.
	const d = "https://discord.gg/B6BSCbdAJX"
	if out := htmlToText(`<a href="` + d + `">` + d + `.</a>`); out != d+"." {
		t.Errorf("self-link trailing punct: got %q, want %q", out, d+".")
	}
	// Trivial scheme difference (bare host vs https://) still counts as a self-link;
	// the canonical href wins.
	if out := htmlToText(`<a href="` + u + `">civitai.com/models/1234</a>`); out != u {
		t.Errorf("self-link scheme diff: got %q, want %q", out, u)
	}
	// A genuine [text](url) where text != url is left as a proper markdown link.
	if out := htmlToText(`<a href="` + u + `">click here</a>`); out != "[click here]("+u+")" {
		t.Errorf("genuine link altered: got %q, want %q", out, "[click here]("+u+")")
	}
	// An image whose alt happens to equal its src is NOT collapsed (images untouched).
	if out := htmlToText(`<img src="` + u + `" alt="` + u + `">`); out != "!["+u+"]("+u+")" {
		t.Errorf("image collapsed: got %q, want %q", out, "!["+u+"]("+u+")")
	}
}

// TestHTMLToTextDedupPreservesNonDuplicates proves the dedup is surgical: a link
// NOT followed by its own bare URL, and following text that is not the duplicate,
// are both left intact.
func TestHTMLToTextDedupPreservesNonDuplicates(t *testing.T) {
	// A normal link followed by ordinary prose keeps everything.
	if out := htmlToText(`<p>See <a href="https://civitai.com/g">the guide</a> for details.</p>`); out != "See [the guide](https://civitai.com/g) for details." {
		t.Errorf("normal link+prose altered:\n%q", out)
	}
	// A bare bracket that does NOT match the href must be preserved.
	got := htmlToText(`<p><a href="https://a.example/1">link</a> [https://b.example/2]</p>`)
	if !strings.Contains(got, "[link](https://a.example/1)") || !strings.Contains(got, "[https://b.example/2]") {
		t.Errorf("non-matching bracket wrongly dropped:\n%q", got)
	}
	// Trailing text after the dropped duplicate survives with a clean separator.
	if out := htmlToText(`<p><a href="https://x/y">t</a> [https://x/y] and more</p>`); out != "[t](https://x/y) and more" {
		t.Errorf("trailing text after dup lost/glued: got %q, want %q", out, "[t](https://x/y) and more")
	}
	// An image link is unaffected by the dedup.
	if out := htmlToText(`<p><img src="https://x/y.png" alt="diagram"></p>`); out != "![diagram](https://x/y.png)" {
		t.Errorf("image render changed: %q", out)
	}
}
