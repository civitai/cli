package redact

import (
	"bytes"
	"errors"
	"strings"
	"unicode/utf16"
)

// ErrNotText is returned by DecodeText for input that is not text in an
// encoding it reads. A caller must treat it as "do not send this": what cannot
// be decoded cannot be redacted.
var ErrNotText = errors.New("not text")

// DecodeText is the READ BOUNDARY for anything that came from a file or a pipe:
// it returns the content as clean UTF-8, or ErrNotText.
//
// 🔴 EVERY RULE IN THIS PACKAGE IS A BYTE-ORIENTED MATCHER, AND UTF-16 WALKS
// PAST ALL OF THEM. PowerShell's `>` redirect writes UTF-16LE, so an error log
// captured on Windows is ASCII with a NUL after every character: no token, no
// e-mail address and no `C:\Users\…` path in it matches anything, and the
// caller's "is the token still in the body" check fails the same way. Measured
// before this existed: such a file went out with `redactions: []` and the
// configured token in it. So the bytes are decoded HERE, before redaction can
// be asked to look at them.
//
// What it reads:
//
//   - UTF-8, with or without a byte-order mark;
//   - UTF-16, little- or big-endian, announced by a byte-order mark;
//   - UTF-16 with NO byte-order mark, when the NUL bytes make it unambiguous —
//     see utf16Order.
//
// What it refuses: a UTF-32 byte-order mark, NUL bytes in no UTF-16 pattern,
// and anything whose decoded form is mostly control characters or replacement
// characters (looksLikeText). Those are binary files, and guessing an encoding
// for one is how an unredacted secret gets sent.
func DecodeText(raw []byte) (string, error) {
	switch {
	case bytes.HasPrefix(raw, []byte{0xFF, 0xFE, 0x00, 0x00}), bytes.HasPrefix(raw, []byte{0x00, 0x00, 0xFE, 0xFF}):
		// UTF-32. Checked before UTF-16, whose little-endian mark is a prefix of
		// the little-endian UTF-32 one.
		return "", ErrNotText
	case bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}):
		raw = raw[3:]
	case bytes.HasPrefix(raw, []byte{0xFF, 0xFE}):
		return finishDecode(decodeUTF16(raw[2:], false))
	case bytes.HasPrefix(raw, []byte{0xFE, 0xFF}):
		return finishDecode(decodeUTF16(raw[2:], true))
	}
	if bytes.IndexByte(raw, 0) < 0 {
		return finishDecode(string(raw))
	}
	bigEndian, ok := utf16Order(raw)
	if !ok {
		return "", ErrNotText
	}
	return finishDecode(decodeUTF16(raw, bigEndian))
}

// utf16Order decides whether NUL-bearing bytes with no byte-order mark are
// UTF-16, and in which order.
//
// Text that is mostly ASCII — which an error log is — has a zero HIGH byte in
// nearly every 16-bit unit. So the NULs sit at one parity of the byte offset
// and essentially never at the other: odd offsets for little-endian, even for
// big-endian. The thresholds ask for that shape plainly — at least 30% of the
// units have their NUL at one parity, and at most 2% at the other — and anything
// else is not called UTF-16. NULs scattered across both parities are what a
// binary file looks like.
func utf16Order(raw []byte) (bigEndian, ok bool) {
	units := len(raw) / 2
	if units == 0 {
		return false, false
	}
	var even, odd int
	for i, b := range raw {
		if b != 0 {
			continue
		}
		if i%2 == 0 {
			even++
		} else {
			odd++
		}
	}
	switch {
	case odd*10 >= units*3 && even*50 <= units:
		return false, true
	case even*10 >= units*3 && odd*50 <= units:
		return true, true
	}
	return false, false
}

// decodeUTF16 decodes raw as UTF-16 in the given byte order. A trailing odd
// byte (a file cut mid-unit) is dropped; an unpaired surrogate becomes U+FFFD,
// which is utf16.Decode's rule.
func decodeUTF16(raw []byte, bigEndian bool) string {
	units := make([]uint16, 0, len(raw)/2)
	for i := 0; i+1 < len(raw); i += 2 {
		if bigEndian {
			units = append(units, uint16(raw[i])<<8|uint16(raw[i+1]))
		} else {
			units = append(units, uint16(raw[i+1])<<8|uint16(raw[i]))
		}
	}
	return string(utf16.Decode(units))
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
