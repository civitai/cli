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
