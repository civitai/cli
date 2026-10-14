package redact

import (
	"bytes"
	"errors"
	"strings"
	"unicode/utf16"
)

// ErrNotText is returned by DecodeText for input that is not text in ONE
// encoding it reads: binary content, or a file that mixes UTF-8 and UTF-16. A
// caller must treat it as "do not send this": what cannot be decoded cannot be
// redacted.
var ErrNotText = errors.New("not text in a single encoding")

// DecodeText is the READ BOUNDARY for anything that came from a file or a pipe:
// it returns the content as UTF-8 with NULs removed, or ErrNotText.
//
// 🔴 THE RULES IN THIS PACKAGE ARE BYTE-ORIENTED MATCHERS, AND UTF-16 DOES NOT
// MATCH THEM. PowerShell's `>` redirect writes UTF-16LE, so an error log
// captured on Windows is ASCII with a NUL after every character: the token, the
// e-mail address and the `C:\Users\…` path in it match nothing, and the
// caller's "is the token still in the request" check fails the same way.
// Measured before this existed: such a file went out with `redactions: []` and
// the configured token in it. So the bytes are decoded HERE, before redaction
// is asked to look at them.
//
// What it reads, exactly:
//
//   - UTF-8, with or without a byte-order mark. Chosen when the input has no
//     UTF-16 byte-order mark and contains no NUL byte.
//   - UTF-16 announced by a byte-order mark, when every code unit passes
//     utf16UnitOK for that byte order.
//   - UTF-16 with NO byte-order mark — chosen only when the input contains a
//     NUL byte — when every code unit passes utf16UnitOK in exactly ONE of the
//     two byte orders.
//
// Everything else is ErrNotText: an odd number of bytes offered as UTF-16, a
// UTF-32 byte-order mark, a code unit that fails the test (see utf16UnitOK for
// what that means and what it costs), a byte-order-mark-less input that passes
// in both orders or in neither, and decoded content that is mostly control or
// replacement characters (looksLikeText).
//
// 🔴 THERE IS NO "MOSTLY UTF-16". An earlier version accepted mark-less input as
// UTF-16 once enough of its bytes fitted the pattern. PowerShell 5.1's `>>`
// appends UTF-16LE to an existing UTF-8 log, and such a file passed that test
// as soon as the tail outweighed the head. Decoded whole as UTF-16, the UTF-8
// head turns into CJK-range characters that still ENCODE its bytes: no rule
// matches them, the caller's token check does not see the token, and it is
// sent. The test is therefore applied to every code unit, with no proportion
// anywhere in it.
func DecodeText(raw []byte) (string, error) {
	switch {
	case bytes.HasPrefix(raw, []byte{0xFF, 0xFE, 0x00, 0x00}), bytes.HasPrefix(raw, []byte{0x00, 0x00, 0xFE, 0xFF}):
		// UTF-32. Checked before UTF-16, whose little-endian mark is a prefix of
		// the little-endian UTF-32 one.
		return "", ErrNotText
	case bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}):
		raw = raw[3:]
	case bytes.HasPrefix(raw, []byte{0xFF, 0xFE}):
		return decodeUTF16(raw[2:], false, true)
	case bytes.HasPrefix(raw, []byte{0xFE, 0xFF}):
		return decodeUTF16(raw[2:], true, true)
	}
	if bytes.IndexByte(raw, 0) < 0 {
		return finishDecode(string(raw))
	}
	le, leErr := decodeUTF16(raw, false, false)
	be, beErr := decodeUTF16(raw, true, false)
	switch {
	case leErr == nil && beErr != nil:
		return le, nil
	case beErr == nil && leErr != nil:
		return be, nil
	}
	// Both orders pass, or neither does: the order cannot be established.
	return "", ErrNotText
}

// isPrintableASCII reports whether b is a printable ASCII byte, 0x20–0x7E.
func isPrintableASCII(b byte) bool { return b >= 0x20 && b <= 0x7E }

// utf16UnitOK is the test EVERY code unit of UTF-16 input must pass. hi and lo
// are the unit's high and low bytes in the byte order being tried; marked says
// whether a byte-order mark established that order.
//
// A unit passes when it is one of:
//
//	(a) hi == 0x00 and lo != 0x00  — U+0001–U+00FF: ASCII and Latin-1, which is
//	    what an error log is made of. This is the NUL-interleave.
//	(b) hi != 0x00, and hi and lo are NOT both printable ASCII bytes — and,
//	    when no byte-order mark established the order, NOT (hi < 0x80 and
//	    lo == 0x00) either.
//
// What (b) refuses, and why:
//
//   - BOTH BYTES PRINTABLE ASCII. Those two bytes are also two characters of
//     UTF-8, and that is what a UTF-8 region inside a "UTF-16" file looks
//     like. Three consecutive printable bytes always contain one whole code
//     unit, so a UTF-8 run of three or more printable characters refuses the
//     file. This is the rule that stops a mixed file.
//   - AN ASCII HIGH BYTE OVER A ZERO LOW BYTE, WITHOUT A MARK. In the WRONG
//     byte order every ASCII character reads as exactly that, so allowing it
//     would let both orders pass. (A zero low byte under a high byte of 0x80 or
//     more is allowed: the low half of a surrogate pair is often one — 😀 ends
//     in `DE00` — and the wrong order cannot produce it from ASCII.)
//
// 🔴 THE COST IS REAL AND IS STATED. Genuine UTF-16 text can contain a
// character whose two bytes are both printable ASCII — 中 is U+4E2D, bytes
// `2D 4E` — and many common CJK characters are in that range. A file holding
// one is REFUSED, with or without a byte-order mark, because it cannot be told
// apart from a file with UTF-8 spliced into it. And without a mark, a character
// below U+8000 whose low byte is zero (一, U+4E00; the ideographic space,
// U+3000) refuses the file too. The user is told to save the log as UTF-8.
// TestDecodeTextUTF16RuleAtItsEdges pins all three.
func utf16UnitOK(hi, lo byte, marked bool) bool {
	if hi == 0 {
		return lo != 0
	}
	if isPrintableASCII(hi) && isPrintableASCII(lo) {
		return false
	}
	return marked || lo != 0 || hi >= 0x80
}

// decodeUTF16 decodes raw as UTF-16 in the given byte order, or returns
// ErrNotText when the length is odd or any code unit fails utf16UnitOK. An
// unpaired surrogate becomes U+FFFD, which is utf16.Decode's rule.
func decodeUTF16(raw []byte, bigEndian, marked bool) (string, error) {
	if len(raw)%2 != 0 {
		return "", ErrNotText
	}
	units := make([]uint16, 0, len(raw)/2)
	for i := 0; i < len(raw); i += 2 {
		hi, lo := raw[i+1], raw[i]
		if bigEndian {
			hi, lo = raw[i], raw[i+1]
		}
		if !utf16UnitOK(hi, lo, marked) {
			return "", ErrNotText
		}
		units = append(units, uint16(hi)<<8|uint16(lo))
	}
	return finishDecode(string(utf16.Decode(units)))
}

func finishDecode(s string) (string, error) {
	s = CleanText(s)
	if !looksLikeText(s) {
		return "", ErrNotText
	}
	return s, nil
}

// CleanText makes s acceptable as a free-text field: invalid UTF-8 becomes
// U+FFFD and NUL is removed.
//
// The server rejects a request whose text carries a NUL or an unpaired
// surrogate. In a Go string an unpaired surrogate can only exist as invalid
// UTF-8 (its three-byte encoding is not valid), so the one replacement covers
// both; each run of invalid bytes becomes a single U+FFFD.
func CleanText(s string) string {
	s = strings.ToValidUTF8(s, "\uFFFD")
	if strings.IndexByte(s, 0) >= 0 {
		s = strings.ReplaceAll(s, "\x00", "")
	}
	return s
}

// looksLikeText reports whether decoded content reads as text: at most one rune
// in ten is a replacement character or a control character other than the ones
// logs are full of (tab, line breaks, form feed, backspace, and ESC — a
// colourised build log is still a log).
//
// This is a proportion, and it is the only one here: it sorts text from binary
// AFTER the encoding has been established unit by unit. It plays no part in
// choosing an encoding.
func looksLikeText(s string) bool {
	total, bad := 0, 0
	for _, r := range s {
		total++
		switch {
		case r == '\t', r == '\n', r == '\r', r == '\f', r == '\b', r == 0x1b:
		case r == 0xFFFD, r < 0x20, r >= 0x7f && r < 0xa0:
			bad++
		}
	}
	return bad*10 <= total
}
