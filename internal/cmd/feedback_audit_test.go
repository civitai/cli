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

// pfPowerShellLog is what `civitai app submit *> err.log` leaves behind on
// Windows: the configured token, an e-mail address and a user's home path.
const pfPowerShellLog = "PS C:\\Users\\Ingrid Solberg\\tide-chart> civitai app submit\r\n" +
	"Authorization: Bearer " + pfTestToken + "\r\n" +
	"contact ingrid.solberg@northwind-labs.io\r\n"

// TestFeedbackSendDecodesUTF16BeforeRedacting: a UTF-16 file is NUL-interleaved
// ASCII as far as a byte-oriented matcher is concerned, so every rule — and the
// token backstop — saw nothing, and the request carried the token, the e-mail
// address and the user name with `redactions: []`.
func TestFeedbackSendDecodesUTF16BeforeRedacting(t *testing.T) {
	encodings := []struct {
		name           string
		bigEndian, bom bool
	}{
		{"UTF-16LE with a BOM", false, true},
		{"UTF-16BE with a BOM", true, true},
		{"UTF-16LE without a BOM", false, false},
		{"UTF-16BE without a BOM", true, false},
	}
	for _, enc := range encodings {
		for _, via := range []string{"--error-file", "--error -"} {
			t.Run(enc.name+" via "+via, func(t *testing.T) {
				f := &pfFake{t: t, create: pfCreated(pfReceived)}
				f.serve()
				raw := pfUTF16(pfPowerShellLog, enc.bigEndian, enc.bom)
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
				out, _, err := runWithStdin(t, stdin, append(args, "submit fails on Windows")...)
				if err != nil {
					t.Fatalf("feedback send: %v", err)
				}
				body := pfSentBody(t, f)
				_, excerpt := pfSentStrings(t, body)

				// Asserted on the DECODED excerpt and with NULs removed, so
				// neither JSON escaping nor interleaving can hide a survivor.
				flat := strings.ReplaceAll(excerpt, "\x00", "")
				for name, secret := range map[string]string{
					"the configured token": pfTestToken, "the e-mail address": "ingrid.solberg@northwind-labs.io", "the user name": "Solberg",
				} {
					if strings.Contains(flat, secret) {
						t.Errorf("%s is in the request: %q", name, excerpt)
					}
				}
				if strings.Contains(body, `\u0000`) {
					t.Errorf("the request body carries a NUL, which the server rejects:\n%s", body)
				}
				const want = "PS C:\\Users\\<user>\\tide-chart> civitai app submit\r\nAuthorization: Bearer <token>\r\ncontact <email>"
				if excerpt != want {
					t.Errorf("excerpt:\n got: %q\nwant: %q", excerpt, want)
				}
				for _, kind := range []string{`"kind": "token"`, `"kind": "email"`, `"kind": "user-path"`} {
					if !strings.Contains(out, kind) {
						t.Errorf("the envelope does not count %s:\n%s", kind, out)
					}
				}
			})
		}
	}
}

// TestFeedbackSendRefusesAnExcerptThatIsNotText: what cannot be decoded cannot
// be redacted, so it is refused before any request rather than sent as-is.
func TestFeedbackSendRefusesAnExcerptThatIsNotText(t *testing.T) {
	binary := make([]byte, 900)
	for i := range binary {
		binary[i] = byte(i*37 + 11)
	}
	// The token is inside, NUL-separated in no encoding's shape.
	copy(binary[100:], "c7a1\x00\x00e94d2b6f4083a5d1c9e7b3f20846")

	for _, via := range []string{"--error-file", "--error -"} {
		t.Run(via, func(t *testing.T) {
			f := &pfFake{t: t}
			f.serve()
			args := []string{"feedback", "send", "--about", "cli"}
			stdin := ""
			if via == "--error-file" {
				p := filepath.Join(f.projectDir, "core.bin")
				if err := os.WriteFile(p, binary, 0o600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--error-file", p)
			} else {
				stdin = string(binary)
				args = append(args, "--error", "-")
			}
			_, _, err := runWithStdin(t, stdin, append(args, "it crashed")...)
			assertOnlyKind(t, err, "ErrUsage")
			if !strings.Contains(err.Error(), "is not text") {
				t.Errorf("the refusal must say why, got: %v", err)
			}
			if n := len(f.seen()); n != 0 {
				t.Errorf("the server saw %d request(s); binary input is refused first", n)
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
