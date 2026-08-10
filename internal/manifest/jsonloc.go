package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
)

// jsonloc.go turns encoding/json's byte OFFSET into the line:column an author
// can actually navigate to (issue #260, item 7).
//
// The message used to be:
//
//	block.manifest.json is not valid JSON: invalid character '}' looking for
//	beginning of object key string
//
// which on a 30-line manifest is a hunt: every `}` in the file is a candidate.
// `encoding/json` already knows where it stopped — `*json.SyntaxError` and
// `*json.UnmarshalTypeError` both carry an `Offset` — and the CLI was throwing
// it away.
//
// 🔴 THE OFFSET IS A BYTE COUNT AND THE COLUMN IS A CHARACTER COUNT, AND THAT
// IS THE WHOLE TRAP. A manifest with any non-ASCII text before the defect —
// an accented display name, an em dash in a tagline, an emoji — has more BYTES
// than characters on that line, so:
//
//   - slicing the buffer by anything other than the raw byte offset finds the
//     wrong place entirely (the line number goes wrong, not just the column);
//   - counting the column in BYTES reports a column no editor agrees with.
//
// So: slice by BYTES, then count RUNES within the line. `TestJSONErrorLocation`
// pins it with a pair of fixtures identical in CHARACTERS and different in
// BYTES, which must report the same column — a byte-counted column makes them
// differ, and a rune-sliced offset makes them differ too.

// invalidJSON builds the "<manifest> is not valid JSON" error, adding a
// `line L, column C` location whenever encoding/json reported an offset.
//
// It is the ONE place that message is composed. Every JSON decode of the
// manifest goes through it, so the location cannot be present on one path and
// missing on the next.
//
// It fails soft: an error carrying no offset (a decoder error with no
// position — `errors.As` finds neither shape) yields exactly the message the
// CLI printed before, rather than a fabricated location.
func invalidJSON(raw []byte, err error) error {
	if loc, ok := jsonErrorLocation(raw, err); ok {
		return fmt.Errorf("%s is not valid JSON at %s: %w", Filename, loc, err)
	}
	return fmt.Errorf("%s is not valid JSON: %w", Filename, err)
}

// jsonErrorLocation renders the position of err within raw as
// "line L, column C", reporting false when err carries no offset.
//
// Both offset-bearing decoder errors are handled: a SYNTAX error (the file does
// not parse at all) and a TYPE error (it parses, but a value has the wrong Go
// type — e.g. `"blockId": 123` decoding into a string field). The second is
// reachable here because LoadRaw decodes the same bytes twice, once generically
// and once into Manifest.
//
// `errors.As` rather than a type switch: the decode sites are free to wrap.
func jsonErrorLocation(raw []byte, err error) (string, bool) {
	var offset int64
	var syn *json.SyntaxError
	var typ *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syn):
		offset = syn.Offset
	case errors.As(err, &typ):
		offset = typ.Offset
	default:
		return "", false
	}
	line, col := lineColumn(raw, offset)
	return fmt.Sprintf("line %d, column %d", line, col), true
}

// lineColumn converts a decoder byte offset into a 1-based line and a 1-based
// CHARACTER column.
//
// encoding/json reports the offset AFTER the byte it choked on, so the
// offending byte sits at offset-1. Both ends are clamped: a zero offset (no
// bytes read) and an end-of-input offset (`unexpected end of JSON input`
// reports len(raw)) are real shapes, and neither should index out of range.
func lineColumn(raw []byte, offset int64) (line, col int) {
	idx := int(offset) - 1
	if idx < 0 {
		idx = 0
	}
	if idx > len(raw) {
		idx = len(raw)
	}
	// 🔴 BYTE slice, then RUNE count. See the header.
	before := raw[:idx]
	line = 1 + bytes.Count(before, []byte("\n"))
	lineStart := bytes.LastIndexByte(before, '\n') + 1 // 0 when there is none
	col = utf8.RuneCount(raw[lineStart:idx]) + 1
	return line, col
}
