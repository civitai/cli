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
