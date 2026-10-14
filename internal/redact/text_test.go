package redact

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf16"
)

// utf16Bytes encodes s as UTF-16 in the given byte order, with or without a
// BOM. It exists only to build inputs UTF8Text must REFUSE.
func utf16Bytes(s string, bigEndian, bom bool) []byte {
	units := utf16.Encode([]rune(s))
	if bom {
		units = append([]uint16{0xFEFF}, units...)
	}
	out := make([]byte, 0, len(units)*2)
	for _, u := range units {
		if bigEndian {
			out = append(out, byte(u>>8), byte(u))
		} else {
			out = append(out, byte(u), byte(u>>8))
		}
	}
	return out
}

// textSecret stands in for anything a matcher must see.
const textSecret = "c1v1tai4f6b8d0a2c4e6f8091a2b3c4d5"

// TestUTF8TextAcceptsUTF8WithOrWithoutItsBOM: the whole of what is accepted.
// The sample carries the characters an earlier, stricter UTF-16 rule refused
// (an ellipsis, a bullet, a trade-mark sign, a cross mark, a box-drawing line),
// a Han character, an accented letter and an emoji outside the BMP.
func TestUTF8TextAcceptsUTF8WithOrWithoutItsBOM(t *testing.T) {
	const sample = "… • ™ ❌ ═ 中 é 😀 token=" + textSecret + "\r\nline two\tend\n"
	for name, raw := range map[string][]byte{
		"UTF-8":            []byte(sample),
		"UTF-8 with a BOM": append([]byte{0xEF, 0xBB, 0xBF}, sample...),
	} {
		got, err := UTF8Text(raw)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got != sample {
			t.Errorf("%s:\n got: %q\nwant: %q", name, got, sample)
		}
	}
	// Empty input, and a bare BOM, are empty text.
	for name, raw := range map[string][]byte{"empty": nil, "only a UTF-8 BOM": {0xEF, 0xBB, 0xBF}} {
		if got, err := UTF8Text(raw); err != nil || got != "" {
			t.Errorf("%s: got %q, %v", name, got, err)
		}
	}
	// ASCII control characters other than NUL are valid UTF-8 and are kept: a
	// colourised log is still a log.
	const ansi = "\x1b[31mError:\x1b[0m failed\n"
	if got, err := UTF8Text([]byte(ansi)); err != nil || got != ansi {
		t.Errorf("an ANSI-coloured line: got %q, %v", got, err)
	}
}

// TestUTF8TextRefusesEverythingElse: anything that is not valid UTF-8, or that
// holds a NUL, is refused — it is never "repaired" into text, because the
// repair is where a secret stops matching the rules meant to remove it. Three
// rounds of a UTF-16 decoder each ended in a file that decoded to something no
// rule matched; these are those files, and the ones that used to be accepted
// with a replacement character.
func TestUTF8TextRefusesEverythingElse(t *testing.T) {
	log := "login printed " + textSecret + "\r\n"
	binary := make([]byte, 600)
	for i := range binary {
		binary[i] = byte(i*37 + 11)
	}
	// One stray LF in the middle of a marked UTF-16 file shifts everything
	// after it by a byte.
	shifted := append(utf16Bytes("header line\r\n", false, true), '\n')
	shifted = append(shifted, utf16Bytes(log, false, false)...)
	shifted = append(shifted, '\n')

	cases := map[string][]byte{
		"UTF-16LE with a BOM":                                utf16Bytes(log, false, true),
		"UTF-16BE with a BOM":                                utf16Bytes(log, true, true),
		"UTF-16LE without a BOM":                             utf16Bytes(log, false, false),
		"UTF-16BE without a BOM":                             utf16Bytes(log, true, false),
		"UTF-16LE with a BOM and no NUL byte anywhere":       utf16Bytes("中文日本語中文日本語", false, true),
		"UTF-16BE with a BOM and no NUL byte anywhere":       utf16Bytes("中文日本語中文日本語", true, true),
		"a BOM'd UTF-16 file with a byte-shifted section":    shifted,
		"UTF-8 with a UTF-16LE tail appended":                append([]byte("2026-10-14 "+log), utf16Bytes(log+log, false, false)...),
		"a UTF-32LE BOM":                                     append([]byte{0xFF, 0xFE, 0x00, 0x00}, "E\x00\x00\x00"...),
		"a UTF-32BE BOM":                                     append([]byte{0x00, 0x00, 0xFE, 0xFF}, "\x00\x00\x00E"...),
		"a UTF-16LE byte-order mark in front of valid UTF-8": append([]byte{0xFF, 0xFE}, "plain text\n"...),
		"a UTF-16BE byte-order mark in front of valid UTF-8": append([]byte{0xFE, 0xFF}, "plain text\n"...),
		"Latin-1":                                 []byte("caf\xe9 failed for " + textSecret + "\n"),
		"a lone NUL in otherwise plain ASCII":     []byte("login printed " + textSecret + "\x00 and hung\n"),
		"a NUL as the last byte":                  []byte("plain text\x00"),
		"a stray continuation byte":               []byte("ab\x80cd"),
		"a multi-byte sequence cut short":         []byte("caf\xc3"),
		"an overlong encoding":                    []byte("a\xc0\xafb"),
		"an encoded surrogate half":               []byte("a\xed\xa0\x80b"),
		"a UTF-8 BOM followed by an invalid byte": {0xEF, 0xBB, 0xBF, 'a', 0xFF},
		"arbitrary binary":                        binary,
		"a PNG header":                            []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"),
	}
	for name, raw := range cases {
		got, err := UTF8Text(raw)
		if !errors.Is(err, ErrNotUTF8) {
			t.Errorf("%s: want ErrNotUTF8, got err=%v and %d byte(s) of text (secret visible to a matcher: %v)",
				name, err, len(got), strings.Contains(got, textSecret))
		}
		if got != "" {
			t.Errorf("%s: a refusal must return no text, got %q", name, got)
		}
	}
}

func TestCleanTextRemovesWhatTheServerRejects(t *testing.T) {
	// A NUL, an unpaired surrogate (which in Go can only exist as invalid
	// UTF-8), and a stray continuation byte.
	in := "a\x00b \xed\xa0\x80 c\x80d"
	got := CleanText(in)
	if strings.ContainsRune(got, 0) {
		t.Errorf("a NUL survived: %q", got)
	}
	if want := "ab \uFFFD c\uFFFDd"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := CleanText("plain ☕ text\n"); got != "plain ☕ text\n" {
		t.Errorf("clean text was changed: %q", got)
	}
}
