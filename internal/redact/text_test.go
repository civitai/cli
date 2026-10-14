package redact

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf16"
)

// utf16Bytes encodes s as UTF-16 in the given byte order, with or without a BOM.
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

const decodeSample = "PS C:\\Users\\Ingrid> civitai app submit\nError: 401 for ingrid.solberg@northwind-labs.io — naïve café ☕ 😀\n"

func TestDecodeTextReadsEveryEncodingAnErrorLogArrivesIn(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
	}{
		{"UTF-8", []byte(decodeSample)},
		{"UTF-8 with a BOM", append([]byte{0xEF, 0xBB, 0xBF}, decodeSample...)},
		{"UTF-16LE with a BOM (PowerShell's > redirect)", utf16Bytes(decodeSample, false, true)},
		{"UTF-16BE with a BOM", utf16Bytes(decodeSample, true, true)},
		{"UTF-16LE without a BOM", utf16Bytes(decodeSample, false, false)},
		{"UTF-16BE without a BOM", utf16Bytes(decodeSample, true, false)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodeText(tc.raw)
			if err != nil {
				t.Fatalf("DecodeText: %v", err)
			}
			if got != decodeSample {
				t.Errorf("decoded text:\n got: %q\nwant: %q", got, decodeSample)
			}
			if strings.ContainsRune(got, 0) {
				t.Error("the decoded text contains a NUL")
			}
		})
	}
}

func TestDecodeTextRefusesWhatIsNotText(t *testing.T) {
	binary := make([]byte, 600)
	for i := range binary {
		binary[i] = byte(i*37 + 11) // every byte value, NULs at no regular stride
	}
	cases := map[string][]byte{
		"arbitrary binary":                  binary,
		"a PNG header":                      []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x02\x00\x00\x00\x02\x00\x08\x06\x00\x00\x00"),
		"a UTF-32 BOM":                      append([]byte{0xFF, 0xFE, 0x00, 0x00}, []byte("E\x00\x00\x00r\x00\x00\x00")...),
		"text with NULs in no UTF-16 shape": []byte("abc\x00\x00\x00defgh\x00ijk\x00\x00lmnop"),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := DecodeText(raw)
			if !errors.Is(err, ErrNotText) {
				t.Errorf("want ErrNotText, got %q, %v", got, err)
			}
		})
	}
	// CONTROL: text with a few stray control bytes and one bad byte is still
	// text — it is cleaned, not refused.
	got, err := DecodeText([]byte("\x1b[31mError:\x1b[0m bad byte \xff here\r\n"))
	if err != nil {
		t.Fatalf("an ANSI-coloured log line with one invalid byte must decode, got %v", err)
	}
	if want := "\x1b[31mError:\x1b[0m bad byte \uFFFD here\r\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
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

// --- audit round 2: one file, two encodings ---------------------------------------

// mixedSecret stands in for anything a matcher must see: here, a credential.
const mixedSecret = "c1v1tai4f6b8d0a2c4e6f8091a2b3c4d5"

// TestDecodeTextRefusesAFileThatMixesEncodings: PowerShell 5.1's `>>` appends
// UTF-16LE to whatever is there, so a UTF-8 log gains a UTF-16 tail with no
// byte-order mark. Decoded whole as UTF-16 — which a "most of the bytes look
// like UTF-16" test did once the tail was long enough — the UTF-8 part becomes
// CJK-looking characters that still ENCODE its bytes: no rule matches them, the
// token backstop does not see the token, and the text is sent. So a file is
// UTF-16 only when EVERY code unit says so, at every ratio of prefix to tail.
func TestDecodeTextRefusesAFileThatMixesEncodings(t *testing.T) {
	prefix := "2026-10-14 token=" + mixedSecret + " rejected\n"
	line := "PS> civitai app submit failed with 401\r\n"

	for _, tailLines := range []int{1, 2, 5, 20, 200} {
		tail := strings.Repeat(line, tailLines)
		cases := map[string][]byte{
			"UTF-8 then UTF-16LE":            append([]byte(prefix), utf16Bytes(tail, false, false)...),
			"UTF-8 then UTF-16BE":            append([]byte(prefix), utf16Bytes(tail, true, false)...),
			"UTF-16LE then UTF-8":            append(utf16Bytes(tail, false, false), prefix...),
			"UTF-16LE with a BOM then UTF-8": append(utf16Bytes(tail, false, true), prefix...),
			"UTF-16BE with a BOM then UTF-8": append(utf16Bytes(tail, true, true), prefix...),
			// An odd-length prefix shifts every later unit by one byte.
			"odd-length UTF-8 then UTF-16LE": append([]byte("x"+prefix), utf16Bytes(tail, false, false)...),
		}
		for name, raw := range cases {
			got, err := DecodeText(raw)
			if !errors.Is(err, ErrNotText) {
				// Say what a matcher would have been handed.
				t.Errorf("%s, %d tail line(s) (%d%% of the bytes are the UTF-16 part): want ErrNotText, got err=%v and %d rune(s) of text; "+
					"the secret is visible to a matcher: %v",
					name, tailLines, 100*(len(raw)-len(prefix))/len(raw), err, len([]rune(got)), strings.Contains(got, mixedSecret))
			}
		}
	}
}

// TestDecodeTextStillReadsPureUTF16: the positive controls for the test above.
// Each is one encoding throughout, carries non-ASCII text (an accented letter, a
// symbol outside Latin-1, an emoji that needs a surrogate pair), and decodes
// exactly.
func TestDecodeTextStillReadsPureUTF16(t *testing.T) {
	want := "naïve café ☕ 😀 token=" + mixedSecret + "\r\nline two\r\n"
	for name, raw := range map[string][]byte{
		"UTF-16LE with a BOM":    utf16Bytes(want, false, true),
		"UTF-16BE with a BOM":    utf16Bytes(want, true, true),
		"UTF-16LE without a BOM": utf16Bytes(want, false, false),
		"UTF-16BE without a BOM": utf16Bytes(want, true, false),
	} {
		got, err := DecodeText(raw)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("%s:\n got: %q\nwant: %q", name, got, want)
		}
	}
}

// TestDecodeTextUTF16RuleAtItsEdges pins the rule's stated limits, so the doc
// comment on utf16Units is a measurement.
func TestDecodeTextUTF16RuleAtItsEdges(t *testing.T) {
	// 中 is U+4E2D: bytes 2D 4E, both printable ASCII ("-N"). It is refused in
	// either form, because those two bytes are also two characters of UTF-8.
	han := "error: 中 failed\n"
	for name, raw := range map[string][]byte{
		"with a BOM":    utf16Bytes(han, false, true),
		"without a BOM": utf16Bytes(han, false, false),
	} {
		if _, err := DecodeText(raw); !errors.Is(err, ErrNotText) {
			t.Errorf("a code unit whose two bytes are both printable ASCII, %s: want ErrNotText, got %v", name, err)
		}
	}
	// 一 is U+4E00: a zero LOW byte. With a byte-order mark the order is known
	// and it decodes; without one, the zero byte is in the wrong half and the
	// order cannot be established.
	one := "error: 一 failed\n"
	if got, err := DecodeText(utf16Bytes(one, false, true)); err != nil || got != one {
		t.Errorf("U+4E00 with a BOM: got %q, %v", got, err)
	}
	if _, err := DecodeText(utf16Bytes(one, false, false)); !errors.Is(err, ErrNotText) {
		t.Errorf("U+4E00 without a BOM: want ErrNotText, got %v", err)
	}
	// An odd number of bytes is not UTF-16.
	odd := append(utf16Bytes("abc\n", false, true), 'x')
	if _, err := DecodeText(odd); !errors.Is(err, ErrNotText) {
		t.Errorf("an odd-length UTF-16 file: want ErrNotText, got %v", err)
	}
}
