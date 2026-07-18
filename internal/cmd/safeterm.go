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
