package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

// Audit round 1 on civitai/cli#830. Each test here was watched to fail against
// the code the finding was filed on (a26141d) before the fix.

// pfUTF16 encodes s as UTF-16 in the given byte order, with or without a BOM.
func pfUTF16(s string, bigEndian, bom bool) []byte {
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

// pfSentStrings decodes a create body and returns the message and excerpt as
// the SERVER would see them after JSON decoding.
func pfSentStrings(t *testing.T, body string) (message, excerpt string) {
	t.Helper()
	var sent struct {
		JSON struct {
			Message string `json:"message"`
			Report  struct {
				ErrorExcerpt string `json:"errorExcerpt"`
			} `json:"report"`
		} `json:"json"`
	}
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatalf("the request body is not JSON: %v\n%s", err, body)
	}
	return sent.JSON.Message, sent.JSON.Report.ErrorExcerpt
}

// pfPowerShellLog is what a failing run leaves in a log on Windows: the
// configured token, an e-mail address and a user's home path.
const pfPowerShellLog = "PS C:\\Users\\Ingrid Solberg\\tide-chart> civitai app submit\r\n" +
	"login printed " + pfTestToken + "\r\n" +
	"contact ingrid.solberg@northwind-labs.io\r\n"

// pfNotUTF8 is every kind of excerpt that must be refused: the bytes are not
// valid UTF-8, or they hold a NUL. Each carries the configured token, so a case
// that is NOT refused shows up as a leak rather than as a tidy pass.
func pfNotUTF8() map[string][]byte {
	binary := make([]byte, 900)
	for i := range binary {
		binary[i] = byte(i*37 + 11)
	}
	copy(binary[100:], pfTestToken)

	// Audit round 3: a byte-order-marked UTF-16 file with one stray byte in the
	// middle (a lone LF, as a UTF-8 tool appending a newline writes), so
	// everything after it is shifted by one: each ASCII character now reads as
	// the HIGH byte of a unit over a zero low byte.
	shifted := append(pfUTF16("header line\r\n", false, true), '\n')
	shifted = append(shifted, pfUTF16(pfPowerShellLog, false, false)...)
	shifted = append(shifted, '\n')

	// Audit round 2: a UTF-8 log with a UTF-16LE tail appended (PowerShell 5.1 `>>`).
	mixed := append([]byte("2026-10-14 login printed "+pfTestToken+"\n"),
		pfUTF16(strings.Repeat("PS> civitai app submit failed with 401\r\n", 3), false, false)...)

	return map[string][]byte{
		"UTF-16LE with a BOM (PowerShell 5.1 redirect)":   pfUTF16(pfPowerShellLog, false, true),
		"UTF-16BE with a BOM":                             pfUTF16(pfPowerShellLog, true, true),
		"UTF-16LE without a BOM":                          pfUTF16(pfPowerShellLog, false, false),
		"UTF-16BE without a BOM":                          pfUTF16(pfPowerShellLog, true, false),
		"UTF-16LE with a BOM and no NUL byte anywhere":    pfUTF16("中文日本語中文日本語", false, true),
		"a BOM'd UTF-16 file with a byte-shifted section": shifted,
		"UTF-8 with a UTF-16 tail appended":               mixed,
		"Latin-1":                                         []byte("caf\xe9 failed, login printed " + pfTestToken + "\n"),
		"a lone NUL":                                      []byte("login printed " + pfTestToken + "\x00 and hung\n"),
		"random binary":                                   binary,
	}
}

// TestFeedbackSendRefusesAnExcerptThatIsNotUTF8: --error-file and --error -
// take valid UTF-8 with no NUL, and nothing else. Whatever else arrives is
// refused with exit 2 BEFORE a request is built — it is not decoded, and it is
// not repaired.
//
// This replaces a UTF-16 decoder that was rewritten twice and leaked each time:
// undecoded UTF-16 matched no rule (round 1); a UTF-8 file with a UTF-16 tail
// was decoded whole as UTF-16 (round 2); a marked UTF-16 file with a
// byte-shifted section passed the unit test (round 3). In every case the token
// left as characters no rule and no backstop recognised.
func TestFeedbackSendRefusesAnExcerptThatIsNotUTF8(t *testing.T) {
	for name, raw := range pfNotUTF8() {
		for _, via := range []string{"--error-file", "--error -"} {
			t.Run(name+" via "+via, func(t *testing.T) {
				f := &pfFake{t: t, create: pfCreated(pfReceived)}
				f.serve()
				args := []string{"feedback", "send", "--about", "cli"}
				stdin := ""
				if via == "--error-file" {
					p := filepath.Join(f.projectDir, "err.log")
					if err := os.WriteFile(p, raw, 0o600); err != nil {
						t.Fatal(err)
					}
					args = append(args, "--error-file", p)
				} else {
					stdin = string(raw)
					args = append(args, "--error", "-")
				}
				_, _, err := runWithStdin(t, stdin, append(args, "submit fails on Windows")...)

				// Zero requests is the property; it is asserted first and
				// whatever the error turned out to be.
				if reqs := f.seen(); len(reqs) != 0 {
					_, excerpt := pfSentStrings(t, reqs[0].body)
					var b []byte
					for _, u := range utf16.Encode([]rune(excerpt)) {
						b = append(b, byte(u), byte(u>>8))
					}
					t.Fatalf("the server saw %d request(s). Sent excerpt: %q\ntoken present as text: %v; token recoverable by re-encoding it as UTF-16: %v",
						len(reqs), excerpt, strings.Contains(excerpt, pfTestToken),
						strings.Contains(strings.ReplaceAll(string(b), "\x00", ""), pfTestToken))
				}
				assertOnlyKind(t, err, "ErrUsage")
				for _, want := range []string{"is not UTF-8 text", "Set-Content -Encoding utf8", "PowerShell 5.1", "nothing was sent"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("the refusal does not say %q:\n%v", want, err)
					}
				}
				if strings.Contains(err.Error(), pfTestToken) {
					t.Errorf("the refusal echoes the excerpt: %v", err)
				}
			})
		}
	}
}

// TestFeedbackSendAcceptsUTF8WithOrWithoutABOM is the positive control for the
// test above: UTF-8 is read, redacted and sent — including the characters an
// earlier UTF-16 rule refused (… • ™ ❌ ═), a Han character, an accented letter
// and an emoji — and a UTF-8 byte-order mark is dropped.
func TestFeedbackSendAcceptsUTF8WithOrWithoutABOM(t *testing.T) {
	const log = "… • ™ ❌ ═ 中 é 😀\r\nlogin printed " + pfTestToken + "\r\ncontact ingrid.solberg@northwind-labs.io\r\n"
	const want = "… • ™ ❌ ═ 中 é 😀\r\nlogin printed <token>\r\ncontact <email>"
	for name, raw := range map[string][]byte{
		"UTF-8":            []byte(log),
		"UTF-8 with a BOM": append([]byte{0xEF, 0xBB, 0xBF}, log...),
	} {
		for _, via := range []string{"--error-file", "--error -"} {
			t.Run(name+" via "+via, func(t *testing.T) {
				f := &pfFake{t: t, create: pfCreated(pfReceived)}
				f.serve()
				args := []string{"--no-color", "feedback", "send", "--about", "cli", "--json"}
				stdin := ""
				if via == "--error-file" {
					p := filepath.Join(f.projectDir, "err.log")
					if err := os.WriteFile(p, raw, 0o600); err != nil {
						t.Fatal(err)
					}
					args = append(args, "--error-file", p)
				} else {
					stdin = string(raw)
					args = append(args, "--error", "-")
				}
				out, _, err := runWithStdin(t, stdin, append(args, "submit fails")...)
				if err != nil {
					t.Fatalf("feedback send: %v", err)
				}
				_, excerpt := pfSentStrings(t, pfSentBody(t, f))
				if excerpt != want {
					t.Errorf("excerpt:\n got: %q\nwant: %q", excerpt, want)
				}
				for _, kind := range []string{`"kind": "token"`, `"kind": "email"`} {
					if !strings.Contains(out, kind) {
						t.Errorf("the envelope does not count %s:\n%s", kind, out)
					}
				}
			})
		}
	}
}

// TestFeedbackSendReadCapDoesNotSplitACharacter: only the first megabyte of an
// excerpt is read. When that cut lands inside a multi-byte character the
// incomplete tail is dropped, so a large valid UTF-8 log is not refused as
// "not UTF-8" for a byte this command itself cut off.
func TestFeedbackSendReadCapDoesNotSplitACharacter(t *testing.T) {
	f := &pfFake{t: t, create: pfCreated(pfReceived)}
	f.serve()
	// One ASCII byte, then two-byte characters: every odd offset — the cap
	// included — is the middle of a character.
	big := "x" + strings.Repeat("é", feedbackErrorReadMax)
	if feedbackErrorReadMax%2 != 0 {
		t.Fatal("fixture defect: the read cap is odd, so this layout does not split a character")
	}
	p := filepath.Join(f.projectDir, "big.log")
	if err := os.WriteFile(p, []byte(big), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "feedback", "send", "--about", "cli", "--error-file", p, "it broke"); err != nil {
		t.Fatalf("a valid UTF-8 file longer than the read cap was refused: %v", err)
	}
	if _, excerpt := pfSentStrings(t, pfSentBody(t, f)); !strings.HasPrefix(excerpt, "xééé") || strings.ContainsRune(excerpt, 0xFFFD) {
		t.Errorf("unexpected excerpt start: %q", excerpt[:20])
	}

	// The same input on stdin: `--error -` has its own read and its own trim.
	g := &pfFake{t: t, create: pfCreated(pfReceived)}
	g.serve()
	if _, _, err := runWithStdin(t, big, "feedback", "send", "--about", "cli", "--error", "-", "it broke"); err != nil {
		t.Fatalf("valid UTF-8 on stdin longer than the read cap was refused: %v", err)
	}
	if _, excerpt := pfSentStrings(t, pfSentBody(t, g)); !strings.HasPrefix(excerpt, "xééé") || strings.ContainsRune(excerpt, 0xFFFD) {
		t.Errorf("stdin: unexpected excerpt start: %q", excerpt[:20])
	}
}

// TestFeedbackSendTrimsASplitCharacterOnlyAtTheReadCap: the trim exists for the
// byte the read cap cut off, and for nothing else. A short excerpt that ENDS in
// an incomplete multi-byte character is not UTF-8, and is refused.
func TestFeedbackSendTrimsASplitCharacterOnlyAtTheReadCap(t *testing.T) {
	short := "login printed " + pfTestToken + " at the caf\xc3"
	for _, via := range []string{"--error-file", "--error -"} {
		t.Run(via, func(t *testing.T) {
			f := &pfFake{t: t, create: pfCreated(pfReceived)}
			f.serve()
			args := []string{"feedback", "send", "--about", "cli"}
			stdin := ""
			if via == "--error-file" {
				p := filepath.Join(f.projectDir, "short.log")
				if err := os.WriteFile(p, []byte(short), 0o600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--error-file", p)
			} else {
				stdin = short
				args = append(args, "--error", "-")
			}
			_, _, err := runWithStdin(t, stdin, append(args, "it broke")...)
			if n := len(f.seen()); n != 0 {
				t.Fatalf("the server saw %d request(s) for an excerpt that is not valid UTF-8", n)
			}
			assertOnlyKind(t, err, "ErrUsage")
			if !strings.Contains(err.Error(), "is not UTF-8 text") {
				t.Errorf("the refusal does not say why: %v", err)
			}
		})
	}
}

// TestFeedbackSendSanitisesTheMessage: the server rejects a NUL or an unpaired
// surrogate in any free-text field, so neither may be sent in the message.
func TestFeedbackSendSanitisesTheMessage(t *testing.T) {
	f := &pfFake{t: t, create: pfCreated(pfReceived)}
	f.serve()
	if _, _, err := run(t, "feedback", "send", "--about", "cli", "it\x00 broke \xed\xa0\x80 here \xff"); err != nil {
		t.Fatalf("feedback send: %v", err)
	}
	body := pfSentBody(t, f)
	message, _ := pfSentStrings(t, body)
	if want := "it broke \uFFFD here \uFFFD"; message != want {
		t.Errorf("message = %q, want %q", message, want)
	}
	if strings.Contains(body, `\u0000`) {
		t.Errorf("a NUL reached the request body:\n%s", body)
	}
}

// TestFeedbackBackstopChecksTheDecodedRequest: the last gate must not be
// defeated by how the request happens to be encoded. A token that JSON escapes
// (`<`, `&`, a quote) never appears byte-for-byte in the marshalled body.
func TestFeedbackBackstopChecksTheDecodedRequest(t *testing.T) {
	const awkward = `Zq7<Lm2&Xv9"Kd4>Rb6`
	if err := feedbackRefuseIfTokenInBody([]byte(`{"json":{"message":"m","report":{"appSlug":"Zq7\u003cLm2\u0026Xv9\"Kd4\u003eRb6"}}}`), []string{awkward}); !errors.Is(err, ErrUsage) {
		t.Errorf("a token that is only present after JSON decoding was not caught: %v", err)
	}
	// CONTROL: the same body without the token passes.
	if err := feedbackRefuseIfTokenInBody([]byte(`{"json":{"message":"m","report":{"appSlug":"tide-chart"}}}`), []string{awkward}); err != nil {
		t.Errorf("a clean body was refused: %v", err)
	}
}

// TestFeedbackErrorFlagTakesOnlyADash: `--error ./submit.log` used to send the
// PATH as the excerpt. The flag now means stdin and nothing else.
func TestFeedbackErrorFlagTakesOnlyADash(t *testing.T) {
	f := &pfFake{t: t, create: pfCreated(pfReceived)}
	f.serve()
	for _, value := range []string{"./submit.log", "boom: it broke", ""} {
		_, _, err := run(t, "feedback", "send", "--about", "cli", "--error", value, "it broke")
		assertOnlyKind(t, err, "ErrUsage")
		for _, want := range []string{"--error -", "--error-file <path>"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("--error %q: the refusal must name %q, got: %v", value, want, err)
			}
		}
	}
	if n := len(f.seen()); n != 0 {
		t.Fatalf("the server saw %d request(s)", n)
	}
	// CONTROL: `--error -` is accepted and reads stdin.
	if _, _, err := runWithStdin(t, "panic: nil map\n", "feedback", "send", "--about", "cli", "--error", "-", "it broke"); err != nil {
		t.Fatalf("--error -: %v", err)
	}
	if _, excerpt := pfSentStrings(t, pfSentBody(t, f)); excerpt != "panic: nil map" {
		t.Errorf("excerpt = %q, want the stdin text", excerpt)
	}
}

// TestFeedbackRedactionCountsDescribeWhatIsSent: the counts printed (and in the
// envelope) are for the message and the ≤500 characters of excerpt that are
// SENT, not for the megabyte that was read.
func TestFeedbackRedactionCountsDescribeWhatIsSent(t *testing.T) {
	f := &pfFake{t: t, create: pfCreated(pfReceived)}
	f.serve()
	// One e-mail address in the first line; 400 more, far past the cut.
	log := "first rosalind.achterberg@quietharbor.org\n" + strings.Repeat("x", 600) + "\n" +
		strings.Repeat("later nwosu.adaeze@harborlight.dev\n", 400)
	p := filepath.Join(f.projectDir, "big.log")
	if err := os.WriteFile(p, []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err := run(t, "--no-color", "feedback", "send", "--about", "cli", "--error-file", p, "it broke")
	if err != nil {
		t.Fatalf("feedback send: %v", err)
	}
	_, excerpt := pfSentStrings(t, pfSentBody(t, f))
	if got := strings.Count(excerpt, "<email>"); got != 1 {
		t.Fatalf("fixture defect: the sent excerpt holds %d <email> placeholder(s), want exactly 1", got)
	}
	if !strings.Contains(out, "Redacted before sending: 1 email.\n") {
		t.Errorf("the count must describe the 1 address removed from what was SENT, not the 401 in the file:\n%s", out)
	}
}

// TestFeedbackSDKNamesMustLookLikePackageNames: a dependency KEY is as
// attacker-shaped as its value. Only an npm-shaped `@civitai/<name>` is sent.
func TestFeedbackSDKNamesMustLookLikePackageNames(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{
		"@civitai/app-sdk":"^0.69.0",
		"@civitai/Has Space":"1.0.0",
		"@civitai/../../etc/passwd":"1.0.0",
		"@civitai/UPPER":"1.0.0",
		"@civitai/":"1.0.0",
		"@civitai/x<script>":"1.0.0",
		"@civitai/.hidden":"1.0.0",
		"@civitai/blocks-react_v2.x":"0.14.2"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := feedbackSDKVersions(dir)
	var names []string
	for _, s := range got {
		names = append(names, s.Name)
	}
	if want := "@civitai/app-sdk @civitai/blocks-react_v2.x"; strings.Join(names, " ") != want {
		t.Errorf("sent names = %q, want %q", strings.Join(names, " "), want)
	}
}
