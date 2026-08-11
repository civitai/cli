package cmd

import "strings"

// safeTerm strips terminal control characters from a SERVER-ORIGIN string before
// it is printed to the user's terminal, so a malicious uploader cannot inject
// ANSI/OSC escape sequences through model/version names, usernames, tags,
// creators, base-model labels, descriptions, download/image URLs, article
// titles/authors, trained words, or file names.
//
// Without this, a hostile field can perform terminal OUTPUT FORGERY: cursor
// moves + line-clears (\x1b[1A\x1b[2K) to overwrite the CLI's own "SHA256
// verified" line or hide a warning, an OSC-52 clipboard-set (\x1b]52;c;...\x07),
// or a window-title spoof.
//
// Removed: every C0 control byte (0x00–0x1F) and DEL (0x7F) EXCEPT newline and
// tab (legitimate layout), plus the C1 control range (U+0080–U+009F) — the 8-bit
// forms of the CSI/OSC escape introducers (0x9B = CSI, 0x9D = OSC, …). Ordinary
// printable and multibyte UTF-8 text passes through byte-for-byte.
//
// Only the HUMAN (table/detail) renderers call this. `--json` output is emitted
// RAW: control characters are already \uXXXX-escaped in spec-compliant JSON, so a
// pipe is safe and sanitizing would corrupt machine-readable bytes.
func safeTerm(s string) string {
	if !strings.ContainsFunc(s, isStrippedControl) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, ch := range s {
		if isStrippedControl(ch) {
			continue
		}
		b.WriteRune(ch)
	}
	return b.String()
}

// indentContinuation prefixes every line of s AFTER the first with pad, so a
// multi-line SERVER string stays visibly inside the list item or block that
// introduced it.
//
// 🔴 THIS IS THE SECOND HALF OF safeTerm, NOT COSMETICS (civitai/cli#367
// review). safeTerm keeps `\n` deliberately — legitimate layout — which means a
// server-supplied string is free to contain them, and until #367 no server field
// this CLI printed was unbounded free text. It now is: the orchestrator's
// failure reason. A reason of
//
//	"Workflow ID:\tspoofed\nStatus:\tsucceeded"
//
// rendered its continuation flush against the left margin, where it was
// indistinguishable from the real `civitai workflows get` header two lines
// above — output forgery achieved without a single control byte, so safeTerm
// cannot see it. Indenting is what makes a continuation line unable to occupy
// column zero, and therefore unable to impersonate a line the CLI itself wrote.
//
// It deliberately does NOT collapse the newlines: the reason is passed through
// verbatim (AGENTS.md item 13), so the fix is where the text SITS, not what it
// says. A trailing newline is left alone rather than padded into a line of
// whitespace.
func indentContinuation(s, pad string) string {
	if !strings.Contains(s, "\n") {
		return s
	}
	lines := strings.Split(s, "\n")
	for i := 1; i < len(lines); i++ {
		if lines[i] == "" {
			continue
		}
		lines[i] = pad + lines[i]
	}
	return strings.Join(lines, "\n")
}

// isStrippedControl reports whether r is a terminal control character safeTerm
// removes: any C0 control or DEL (other than newline and tab), or any C1 control
// (U+0080–U+009F).
func isStrippedControl(r rune) bool {
	if r == '\n' || r == '\t' {
		return false
	}
	if r < 0x20 || r == 0x7f {
		return true
	}
	return r >= 0x80 && r <= 0x9f
}
