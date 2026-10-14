package redact

import (
	"bytes"
	"errors"
	"strings"
	"unicode/utf8"
)

// ErrNotUTF8 is returned by UTF8Text for bytes that are not valid UTF-8, or
// that contain a NUL. A caller must treat it as "do not send this".
var ErrNotUTF8 = errors.New("not UTF-8 text")

// utf8BOM is the UTF-8 byte-order mark, the one mark UTF8Text accepts.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// UTF8Text is the READ BOUNDARY for anything that came from a file or a pipe.
// It returns raw as a string when raw is valid UTF-8 and contains no NUL byte,
// after dropping one leading UTF-8 byte-order mark if there is one. Otherwise
// it returns "" and ErrNotUTF8.
//
// That is the whole contract. It converts nothing and repairs nothing: an
// invalid byte is a refusal, not a U+FFFD.
//
// 🔴 THERE IS NO UTF-16 DECODER HERE, AND ONE MUST NOT BE ADDED BACK. This
// function was one for three audit rounds, and each version let a credential
// out. With no decoding, UTF-16 matched none of this package's byte-oriented
// rules. With a "mostly UTF-16" test, a UTF-8 log with a UTF-16 tail was
// decoded whole as UTF-16. With a per-code-unit test, a marked UTF-16 file with
// one stray byte in the middle still passed. Each time the secret left as
// characters no rule and no "is the token in the request" check recognised —
// and the strictest version also refused ordinary logs. Telling encodings
// apart from bytes is guesswork; UTF-8 validity is not. The caller's error
// message says how to convert a file.
func UTF8Text(raw []byte) (string, error) {
	raw = bytes.TrimPrefix(raw, utf8BOM)
	if bytes.IndexByte(raw, 0) >= 0 {
		return "", ErrNotUTF8
	}
	if !utf8.Valid(raw) {
		return "", ErrNotUTF8
	}
	return string(raw), nil
}

// CleanText makes s acceptable as a free-text field: invalid UTF-8 becomes
// U+FFFD and NUL is removed. It is for the MESSAGE, which arrives as a
// command-line argument; file and pipe content goes through UTF8Text, which
// refuses instead.
//
// The server rejects a request whose text carries a NUL or an unpaired
// surrogate. In a Go string an unpaired surrogate can only exist as invalid
// UTF-8 (its three-byte encoding is not valid), so the one replacement covers
// both; each run of invalid bytes becomes a single U+FFFD.
func CleanText(s string) string {
	s = strings.ToValidUTF8(s, "�")
	if strings.IndexByte(s, 0) >= 0 {
		s = strings.ReplaceAll(s, "\x00", "")
	}
	return s
}
