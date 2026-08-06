package blockproto

import "testing"

// The comment stripper decides whether a mention of a token is EVIDENCE or just
// prose. Both directions are expensive:
//
//	strip too little -> a comment naming BLOCK_READY silences the check at a
//	                    genuinely broken app (false negative)
//	strip too much   -> real code disappears and a correct project is warned at
//	                    (false positive, the one AGENTS.md item 10 exists for)
//
// These cases are built so a widening or narrowing of each internal branch
// changes the ANSWER, not just the whitespace. Several exist because the audit's
// sweep found the branch unpinned.

func TestStripHTMLComments(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			// 🔴 The TERMINATOR is `-->`, three characters. Widening it to `->`
			// ends the comment early, so everything after the arrow becomes
			// "code" — a commented-out mention turns into evidence and the check
			// goes silently quiet at a broken app. An `->` inside a comment is
			// ordinary prose (and an arrow function's `=>` is one keystroke away).
			name: "an arrow inside a comment does not terminate it",
			src:  "<!-- the host -> block handshake posts BLOCK_READY -->after",
			want: "after",
		},
		{
			name: "a normal comment is removed",
			src:  "a<!-- b -->c",
			want: "ac",
		},
		{
			// 🔴 UNTERMINATED: keep the tail verbatim. Dropping it is lossy in
			// the expensive direction — everything below an unclosed marker
			// disappears, including an emitter.
			name: "an unterminated comment keeps the rest of the file",
			src:  "a<!-- b\nBLOCK_READY",
			want: "a<!-- b\nBLOCK_READY",
		},
		{
			name: "two comments are both removed",
			src:  "a<!--x-->b<!--y-->c",
			want: "abc",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := StripHTMLComments(c.src); got != c.want {
				t.Fatalf("StripHTMLComments(%q) = %q, want %q", c.src, got, c.want)
			}
		})
	}
}

func TestStripJSComments(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			name: "a line comment goes, the code after it stays",
			src:  "// BLOCK_READY\nkeep;",
			want: "\nkeep;",
		},
		{
			name: "a block comment goes",
			src:  "a;/* BLOCK_READY */b;",
			want: "a;b;",
		},
		{
			// 🔴 UNTERMINATED `/*` — the same rule the HTML half carries, which
			// used to be documented and implemented only there. Swallowing the
			// tail deletes an emitter and warns at a correct project.
			name: "an unterminated block comment keeps the rest of the file",
			src:  "a;/* oops\nBLOCK_READY;",
			want: "a;/* oops\nBLOCK_READY;",
		},
		{
			// The string branch: a URL is not a comment. The ack is on the SAME
			// LINE on purpose — with quote handling defeated the `//` in
			// `https://` swallows everything after it.
			name: "a URL in a string does not open a comment",
			src:  "var u = 'https://x'; BLOCK_READY;",
			want: "var u = 'https://x'; BLOCK_READY;",
		},
		{
			// The BACKTICK branch, which nothing pinned: a template literal is a
			// string too, and a `//` inside one is not a comment.
			name: "a template literal protects its contents",
			src:  "var u = `https://x`; BLOCK_READY;",
			want: "var u = `https://x`; BLOCK_READY;",
		},
		{
			// The BACKSLASH-ESCAPE branch, which nothing pinned. Without it the
			// escaped quote is read as the string's END, so the rest of the line
			// is parsed as code — and the `//` that follows becomes a comment
			// that eats the ack.
			name: "an escaped quote does not end the string early",
			src:  `var s = 'it\'s // not a comment'; BLOCK_READY;`,
			want: `var s = 'it\'s // not a comment'; BLOCK_READY;`,
		},
		{
			// The inverse control for the branch above: an escape must not make
			// the stripper run past a string that really does end.
			name: "an escaped backslash still ends the string",
			src:  `var s = 'x\\'; // gone` + "\nBLOCK_READY;",
			want: `var s = 'x\\'; ` + "\nBLOCK_READY;",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := StripJSComments(c.src); got != c.want {
				t.Fatalf("StripJSComments(%q) = %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// TestStripCommentsForExtGate pins the extension gate in BOTH directions.
//
// 🔴 Running StripHTMLComments over `.js` made an ordinary `var OPEN = '<!--';`
// open a "comment" that ran to EOF and deleted the emitter below it — a false
// warning at a correct project, reproduced live. The inverse control matters as
// much: without it, "don't strip HTML in .js" is indistinguishable from "don't
// strip HTML at all", which would let a real HTML comment count as evidence.
func TestStripCommentsForExtGate(t *testing.T) {
	const js = "var OPEN = '<!--';\nBLOCK_READY;\nvar CLOSE = '-->';\n"
	if got := StripCommentsForExt(js, ".js"); !contains(got, "BLOCK_READY") {
		t.Fatalf(".js lost code between two HTML markers in strings: %q", got)
	}
	const html = "<!-- BLOCK_READY -->\n<script>keep;</script>"
	if got := StripCommentsForExt(html, ".html"); contains(got, "BLOCK_READY") {
		t.Fatalf(".html kept a real HTML comment: %q", got)
	}
	// The extension match is case-insensitive — an `.HTML` file is markup.
	if got := StripCommentsForExt(html, ".HTML"); contains(got, "BLOCK_READY") {
		t.Fatalf(".HTML was not treated as markup: %q", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
