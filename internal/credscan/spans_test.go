package credscan

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// Spans is Scan's two detectors with the LOCATION kept. These tests pin the two
// things a second consumer relies on: that the span is the credential and
// nothing else, and that Spans and match agree on whether a line holds one.

func TestSpansCoverTheCredentialAndNothingElse(t *testing.T) {
	cases := []struct {
		name, line string
		// want is the exact text each returned span must cover, left to right.
		want []string
	}{
		{"a known format mid-sentence",
			"push failed for ghp_2Xc8Vb4Nm6Lk0Jh3Gf7Ds1Ap5Qw9Er2Ty6Ui today",
			[]string{"ghp_2Xc8Vb4Nm6Lk0Jh3Gf7Ds1Ap5Qw9Er2Ty6Ui"}},
		{"an unquoted assignment value",
			"SERVICE_TOKEN=Jk4Wq9Zx2Lm7Rb5Tn8Vc and more",
			[]string{"Jk4Wq9Zx2Lm7Rb5Tn8Vc"}},
		{"a quoted assignment value keeps its quotes outside the span",
			`{"apiSecret": "Yh6Tg3Rf9Ed2Ws5Qa8Zx1Cv"}`,
			[]string{"Yh6Tg3Rf9Ed2Ws5Qa8Zx1Cv"}},
		{"two credentials on one line are both located",
			"A=npm_3Df7Gh1Jk5Lz9Xc2Vb6Nm0Qw4Er8Ty2Ui6Op and DEPLOY_PASSWORD=Mn5Bv9Cx3Zl7Kj1Hg4Fd",
			[]string{"npm_3Df7Gh1Jk5Lz9Xc2Vb6Nm0Qw4Er8Ty2Ui6Op", "Mn5Bv9Cx3Zl7Kj1Hg4Fd"}},
		{"a URL with a password spans scheme through host",
			"dial redis://cache_rw:Vb7Nm2Xc9Zl4Kj@cache-3.prod-internal.net:6379/0 timed out",
			[]string{"redis://cache_rw:Vb7Nm2Xc9Zl4Kj@cache-3.prod-internal.net:6379"}},
		{"an assignment whose value is itself a known format is one span, not two",
			"GITHUB_TOKEN=ghp_8Qw2Er6Ty0Ui4Op7As1Df5Gh9Jk3Lz6Xc0Vb",
			[]string{"ghp_8Qw2Er6Ty0Ui4Op7As1Df5Gh9Jk3Lz6Xc0Vb"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spans := Spans(tc.line)
			var got []string
			prevEnd := 0
			for _, sp := range spans {
				if sp.Start < prevEnd || sp.End <= sp.Start || sp.End > len(tc.line) {
					t.Fatalf("span %+v is out of order, empty or out of range (prev end %d, line %d bytes)", sp, prevEnd, len(tc.line))
				}
				prevEnd = sp.End
				got = append(got, tc.line[sp.Start:sp.End])
			}
			if strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
				t.Errorf("spans cover %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSpansAgreeWithMatch: Spans must not become a second, drifting opinion
// about what a credential is. For every line, "Spans found something" and
// "match fired" are the same answer — including on the lines the detectors are
// deliberately quiet about.
func TestSpansAgreeWithMatch(t *testing.T) {
	lines := []string{
		"SERVICE_TOKEN=Jk4Wq9Zx2Lm7Rb5Tn8Vc",
		"push failed for ghp_2Xc8Vb4Nm6Lk0Jh3Gf7Ds1Ap5Qw9Er2Ty6Ui today",
		"dial redis://cache_rw:Vb7Nm2Xc9Zl4Kj@cache-3.prod-internal.net:6379/0",
		// Quiet by design:
		"API_TOKEN=your-token-goes-here-123",
		"token: ${CIVITAI_TOKEN}",
		`const token = opts.credentials.token`,
		"postgres://user:password@localhost:5432/mydb",
		"VITE_FIREBASE_API_KEY=AIzaSyB4Xk9Qm2Lz7Rv3Tn8Wc1Yd6Hf0Pg5Sj2U",
		"nothing here but prose about a token",
		"",
	}
	fired, quiet := 0, 0
	for _, line := range lines {
		_, matched := match(line)
		located := len(Spans(line)) > 0
		if matched != located {
			t.Errorf("match=%v but Spans located=%v for %q", matched, located, line)
		}
		if matched {
			fired++
		} else {
			quiet++
		}
	}
	// Both arms must have been exercised, or agreement is a fact about one arm.
	if fired < 3 || quiet < 3 {
		t.Fatalf("CONTROL failure: %d line(s) fired and %d stayed quiet — the comparison needs both", fired, quiet)
	}
}

// TestSpansCoverAValueLongerThanTheCaptureBound: assignRe bounds its value
// capture at 512 characters — a complexity fix (see assignRe) that is right for a
// detector, which only needs to know the line holds a secret. A consumer that
// REMOVES the secret needs all of it: redacting the first 512 bytes of a
// 700-byte value sends the other 188.
func TestSpansCoverAValueLongerThanTheCaptureBound(t *testing.T) {
	var b strings.Builder
	for i := 0; b.Len() < 700; i++ {
		b.WriteString("Kq9Zx2Lm7Rb5Tn8Vc4Hd6Yw1Pf3Sg0Uj"[i%7:])
	}
	secret := b.String()[:700]
	for name, line := range map[string]string{
		"unquoted": "SESSION_SECRET=" + secret + " trailing words",
		"quoted":   `{"sessionSecret": "` + secret + `", "other": 1}`,
	} {
		t.Run(name, func(t *testing.T) {
			spans := Spans(line)
			if len(spans) != 1 {
				t.Fatalf("want 1 span, got %d: %+v", len(spans), spans)
			}
			if got := line[spans[0].Start:spans[0].End]; got != secret {
				t.Errorf("the span covers %d of the value's %d bytes", len(got), len(secret))
			}
		})
	}
	// CONTROL: a value exactly at the bound is one whole span too.
	at := secret[:512]
	if spans := Spans("SESSION_SECRET=" + at + " x"); len(spans) != 1 || spans[0].End-spans[0].Start != 512 {
		t.Errorf("a 512-byte value: %+v", spans)
	}
}

// testSecret returns a value of exactly n CHARACTERS that passes A2's gate.
// seed shifts where in its alphabet it starts, so two parts of one line differ.
// With accented set, every eighth character is the two-byte `é`, so the value's
// byte length is well above n.
func testSecret(n, seed int, accented bool) string {
	const alphabet = "Kq9Zx2Lm7Rb5Tn8Vc4Hd6Yw1Pf3Sg0Uj"
	var b strings.Builder
	for i := 0; i < n; i++ {
		if accented && i%8 == 7 {
			b.WriteRune('é')
			continue
		}
		b.WriteByte(alphabet[((i+seed)*7)%len(alphabet)])
	}
	return b.String()
}

// assertOneSpan requires Spans(line) to return exactly one span covering want.
func assertOneSpan(t *testing.T, line, want string) {
	t.Helper()
	spans := Spans(line)
	if len(spans) != 1 {
		t.Fatalf("want 1 span covering %d characters, got %d: %+v", utf8.RuneCountInString(want), len(spans), spans)
	}
	got := line[spans[0].Start:spans[0].End]
	if got == want {
		return
	}
	if strings.HasPrefix(want, got) {
		t.Errorf("the span covers the first %d of the value's %d characters; the other %d are left outside it",
			utf8.RuneCountInString(got), utf8.RuneCountInString(want),
			utf8.RuneCountInString(want)-utf8.RuneCountInString(got))
		return
	}
	t.Errorf("the span covers %d characters (%.24q…), want the value's %d characters (%.24q…)",
		utf8.RuneCountInString(got), got, utf8.RuneCountInString(want), want)
}

// TestSpansCoverANonASCIIValueLongerThanTheCaptureBound: assignRe's `{1,512}`
// counts CHARACTERS (Go's regexp matches runes), so the capture of a value
// carrying non-ASCII text stops at 512 characters, which is more than 512 bytes.
// A check that compared the capture's BYTE length to the bound never saw the
// capture as cut, so the span ended at character 512 and redaction sent the
// rest of the secret. The ASCII test above cannot see this: there, characters
// and bytes are the same number.
func TestSpansCoverANonASCIIValueLongerThanTheCaptureBound(t *testing.T) {
	secret := testSecret(700, 0, true)
	if utf8.RuneCountInString(secret) != 700 || len(secret) <= 700 {
		t.Fatalf("CONTROL failure: fixture is %d characters / %d bytes; it must be 700 characters and more bytes",
			utf8.RuneCountInString(secret), len(secret))
	}
	for name, line := range map[string]string{
		"unquoted": "SECRET=" + secret + " trailing words",
		"quoted":   `{"sessionSecret": "` + secret + `", "other": 1}`,
	} {
		t.Run(name, func(t *testing.T) { assertOneSpan(t, line, secret) })
	}
	// CONTROL: values exactly at the bound and one character either side of it
	// are each one whole span.
	for _, n := range []int{511, 512, 513} {
		v := testSecret(n, 0, true)
		t.Run(fmt.Sprintf("%d characters", n), func(t *testing.T) { assertOneSpan(t, "SECRET="+v+" x", v) })
	}
}

// TestSpansCoverACutValueWhoseEndTheCleanerMoved: when assignRe's capture is
// cut at the bound, cleanValue can still move the value's end BEFORE the cut —
// it trims a trailing `;`, and an unclosed quote sends a quoted value through
// the unquoted rules, which end it at `,` `}` `]` or `)`. Spans used to extend a
// cut value only when its end sat exactly at the cut, so in each of these
// shapes the rest of the secret was left outside the span and sent.
func TestSpansCoverACutValueWhoseEndTheCleanerMoved(t *testing.T) {
	for _, accented := range []bool{false, true} {
		head511, head510, head300 := testSecret(511, 0, accented), testSecret(510, 0, accented), testSecret(300, 0, accented)
		tail200, tail300 := testSecret(200, 11, accented), testSecret(300, 11, accented)
		cases := []struct{ name, line, want string }{
			{"(a) unquoted, `;` at the bound",
				"SECRET=" + head511 + ";" + tail200 + " trailing words",
				head511 + ";" + tail200},
			{"(c) quoted, `;` at the bound",
				`{"sessionSecret": "` + head510 + ";" + tail200 + `", "other": 1}`,
				head510 + ";" + tail200},
		}
		for _, d := range []string{",", "}", "]", ")"} {
			cases = append(cases, struct{ name, line, want string }{
				"(b) quoted, `" + d + "` inside the first 512",
				`{"sessionSecret": "` + head300 + d + tail300 + `", "other": 1}`,
				head300 + d + tail300,
			})
		}
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s accented=%v", tc.name, accented), func(t *testing.T) {
				assertOneSpan(t, tc.line, tc.want)
			})
		}
	}
}

// TestSpansStopAtADelimiterThatEndsTheValue is the other side of the test
// above: extending a cut value must not run past a delimiter that really ends
// it, and a value under the bound is never extended at all.
func TestSpansStopAtADelimiterThatEndsTheValue(t *testing.T) {
	short, head511, next := testSecret(20, 0, false), testSecret(511, 0, false), testSecret(30, 11, false)
	cases := []struct{ name, line, want string }{
		{"short unquoted, trailing `;`", "SECRET=" + short + "; next", short},
		{"short unquoted, `,` then more", "SECRET=" + short + ",next=" + next, short},
		{"short unquoted, `}` then more", "{SECRET=" + short + "}" + next, short},
		{"short quoted", `{"sessionSecret": "` + short + `", "next": "` + next + `"}`, short},
		{"cut unquoted, `,` at the bound", "SECRET=" + head511 + ",next=" + next, head511},
		{"cut unquoted, CJK text at the bound", "SECRET=" + head511 + "中文说明" + next, head511},
		// Minified JSON: the capture runs past the value's closing quote into
		// the next key and is cut there, but the value itself is closed.
		{"cut capture, quoted value closed before the cut",
			`{"secret":"` + short + `","other":"` + testSecret(600, 11, false) + `"}`, short},
		{"cut capture, unquoted value ended by `,` before the cut",
			"{SECRET=" + short + ",other=" + testSecret(600, 11, false) + "}", short},
		{"exactly at the bound, then a space", "SECRET=" + head511 + "Q " + next, head511 + "Q"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertOneSpan(t, tc.line, tc.want) })
	}
}

// TestNonASCIIValuesPassTheGateLikeASCIIOnes: the A2 gate's letter and entropy
// tests once read BYTES, so a password written in a non-Latin script was
// judged by its UTF-8 encoding — no ASCII letter at all, or a byte entropy far
// below its character entropy — and went unreported, and unredacted.
func TestNonASCIIValuesPassTheGateLikeASCIIOnes(t *testing.T) {
	cases := []struct{ name, value string }{
		// 12 distinct characters (entropy 3.58 bits each), but every Cyrillic
		// letter here shares its lead byte, so the BYTE entropy is ~2.95.
		{"character entropy above the floor, byte entropy below it", "a1ЖЩЯФЫВАПРО"},
		// No ASCII letter anywhere.
		{"letters only outside ASCII", "Пароль2024Секрет"},
		// No ASCII digit anywhere (Arabic-Indic digits).
		{"digits only outside ASCII", "KqZxLmRbTnVc٢٠٢٤"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			line := "DB_PASSWORD=" + tc.value
			if _, ok := match(line); !ok {
				t.Errorf("match is silent about %q", line)
			}
			assertOneSpan(t, line, tc.value)
		})
	}

	// CONTROL: the fixtures sit where they claim to, so the cases above exercise
	// the byte/character difference rather than passing on some other ground.
	if h := byteEntropy(cases[0].value); h >= minEntropyBitsPerChar {
		t.Errorf("CONTROL failure: %q has byte entropy %.2f, not below the %.1f floor", cases[0].value, h, minEntropyBitsPerChar)
	}
	if strings.ContainsFunc(cases[1].value, func(r rune) bool { return r < utf8.RuneSelf && unicode.IsLetter(r) }) {
		t.Errorf("CONTROL failure: %q has an ASCII letter", cases[1].value)
	}
	if strings.ContainsAny(cases[2].value, "0123456789") {
		t.Errorf("CONTROL failure: %q has an ASCII digit", cases[2].value)
	}
	// CONTROL: a low-entropy value is still silent whatever its script.
	for _, v := range []string{"a1ЖЖЖЖЖЖЖЖЖЖЖ", "a1aaaaaaaaaaa"} {
		if _, ok := match("DB_PASSWORD=" + v); ok {
			t.Errorf("a low-entropy value %q fired", v)
		}
	}
}

// TestEntropyCountsInvalidBytesApart: entropy is measured over characters, but
// a byte that is not valid UTF-8 stays its own symbol, as it was when entropy
// was measured over bytes — four distinct invalid bytes must not collapse into
// one U+FFFD and read as zero entropy.
func TestEntropyCountsInvalidBytesApart(t *testing.T) {
	if h := shannonBitsPerChar("\xff\xfe\xfd\xfc"); h != 2 {
		t.Errorf("four distinct invalid bytes: entropy %.2f, want 2", h)
	}
	// CONTROL: the same count of distinct CHARACTERS reads the same.
	if h := shannonBitsPerChar("ЖЩЯФ"); h != 2 {
		t.Errorf("four distinct Cyrillic letters: entropy %.2f, want 2", h)
	}
}

// unspacedSample is one character from each script written without spaces
// between words, keyed by its unicode table name.
var unspacedSample = map[string]rune{
	"Han": '中', "Hiragana": 'あ', "Katakana": 'ア', "Thai": 'ก',
	"Lao": 'ກ', "Khmer": 'ក', "Myanmar": 'က',
}

// TestProseInAScriptWithoutSpacesIsSilent: Chinese, Japanese, Thai and the
// other scripts written without spaces between words put a whole sentence in
// one non-space run, so assignRe's value capture takes the sentence and a
// letter/digit/entropy gate that reads every script passes it. So:
//   - a QUOTED value containing any character of such a script is prose;
//   - an UNQUOTED value ends at its first such character (or at CJK /
//     full-width punctuation), and the part before it is judged as usual.
//
// An i18n sentence is then silent (nothing, or too little, comes before its
// first CJK character), while an ASCII secret with CJK text glued after it is
// still caught — and only the ASCII part is located.
func TestProseInAScriptWithoutSpacesIsSilent(t *testing.T) {
	silent := []string{
		// Scripts written without spaces.
		"token_expired: トークンの有効期限（24時間）が切れました",
		`"password_hint": "密码长度至少8位，包含字母和数字",`,
		"secret_note: 秘密鍵は2048ビット以上を推奨します",
		"password_policy: รหัสผ่านต้องมีอย่างน้อย8ตัวอักษร",
		// Spaced scripts: the capture is one word, as it is for English.
		"password_hint: Пароль должен содержать не менее 8 символов",
		"password_hint: يجب أن تحتوي كلمة المرور على 8 أحرف على الأقل",
		"password_hint: Ο κωδικός πρέπει να έχει τουλάχιστον 8 χαρακτήρες",
		"password_hint: must be at least 8 characters long",
		"token: ${CIVITAI_TOKEN}",
	}
	for _, line := range silent {
		if _, ok := match(line); ok {
			t.Errorf("match fires on prose: %q", line)
		}
		if spans := Spans(line); len(spans) != 0 {
			t.Errorf("Spans locates %+v in prose: %q", spans, line)
		}
	}

	const secret = "Kq9Zx2Lm7Rb5Tn8Vc"
	for script, r := range unspacedSample {
		// QUOTED: one character of the script makes the value prose. CONTROL:
		// the same quoted value without it is a secret.
		quoted := `{"password": "` + secret + string(r) + `"}`
		if _, ok := match(quoted); ok {
			t.Errorf("%s: match fires on %q", script, quoted)
		}
		if spans := Spans(quoted); len(spans) != 0 {
			t.Errorf("%s: Spans locates %+v in %q", script, spans, quoted)
		}
		// UNQUOTED: the value ends at that character; the ASCII before it is
		// caught, and nothing after it is located.
		unquoted := "DB_PASSWORD=" + secret + string(r) + "备注"
		if _, ok := match(unquoted); !ok {
			t.Errorf("%s: match is silent about %q", script, unquoted)
		}
		t.Run(script+" unquoted", func(t *testing.T) { assertOneSpan(t, unquoted, secret) })
	}
	assertOneSpan(t, `{"password": "`+secret+`"}`, secret)

	// Still caught: spaced non-Latin scripts are judged like ASCII.
	for _, v := range []string{"Пароль2024Секрет", "KqZxLmRbTnVc٢٠٢٤", "a1ЖЩЯФЫВАПРО"} {
		assertOneSpan(t, "DB_PASSWORD="+v, v)
	}
}

// TestAnASCIISecretFollowedByCJKTextIsCaught: an unquoted ASCII secret with CJK
// text joined to it by something other than `,` `}` `]` `)` is still a secret.
// Reading the whole run as prose dropped every one of these, which main caught.
func TestAnASCIISecretFollowedByCJKTextIsCaught(t *testing.T) {
	cases := []struct{ line, want string }{
		{"登录失败 token=Wq8Zt3Lm6Xr1Bv5，请重试", "Wq8Zt3Lm6Xr1Bv5"}, // full-width comma
		{"token=Wq8Zt3Lm6Xr1Bv5&lang=中文", "Wq8Zt3Lm6Xr1Bv5&lang="},
		{"password=Wq8Zt3Lm6Xr1Bv5;备注", "Wq8Zt3Lm6Xr1Bv5"},
		{"password: Wq8Zt3Lm6Xr1Bv5|用户", "Wq8Zt3Lm6Xr1Bv5|"},
		{"DB_PASSWORD=Kq9Zx2Lm7Rb5Tn8Vc中", "Kq9Zx2Lm7Rb5Tn8Vc"},
		// CJK and full-width punctuation end the value too.
		{"token=Wq8Zt3Lm6Xr1Bv5、次へ", "Wq8Zt3Lm6Xr1Bv5"},  // U+3001
		{"token=Wq8Zt3Lm6Xr1Bv5　次へ", "Wq8Zt3Lm6Xr1Bv5"},  // U+3000 ideographic space
		{"token=Wq8Zt3Lm6Xr1Bv5：次へ", "Wq8Zt3Lm6Xr1Bv5"},  // U+FF1A
		{"token=Wq8Zt3Lm6Xr1Bv5（説明）", "Wq8Zt3Lm6Xr1Bv5"}, // U+FF08
	}
	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			if _, ok := match(tc.line); !ok {
				t.Errorf("match is silent about %q", tc.line)
			}
			assertOneSpan(t, tc.line, tc.want)
		})
	}
}

// TestSpansFindTheRealClosingQuoteOfAnEscapedValue: a quoted value can carry
// escaped quotes (`\"`) — JSON written into a log line as a string. When the
// capture is cut before the real closing quote, the span must run to THAT
// quote; an escaped one is part of the value.
func TestSpansFindTheRealClosingQuoteOfAnEscapedValue(t *testing.T) {
	head510, tail200 := testSecret(510, 0, false), testSecret(200, 11, false)
	t.Run("cut between a backslash and its quote", func(t *testing.T) {
		assertOneSpan(t, `{"sessionSecret": "`+head510+`\"`+tail200+`", "other": 1}`, head510+`\"`+tail200)
	})
	// CONTROL: an escaped quote in a value under the bound already ends at the
	// real closing quote.
	assertOneSpan(t, `{"sessionSecret": "Kq9\"Zx2Lm7Rb5Tn8", "other": 1}`, `Kq9\"Zx2Lm7Rb5Tn8`)
}

// TestACutQuotedValueIsJudgedWhole: when a quoted value's closing quote lies
// past the 512-character cut, the unquoted rules would end it at its first
// `,`, and the gate would judge only that fragment. Escaped JSON in a log line
// is exactly that shape: the fragment is `{\"client_id\":\"<id>\"`, and an id
// with no digit failed the gate — so nothing was located and the 600-character
// client_secret after it was sent. The value is judged to its real closing
// quote instead, by match and Spans alike.
func TestACutQuotedValueIsJudgedWhole(t *testing.T) {
	secret := testSecret(600, 0, false)
	for _, id := range []string{"abc", "civitai-cli", "my-web-app", "abc123def456"} {
		t.Run(id, func(t *testing.T) {
			inner := `{\"client_id\":\"` + id + `\",\"client_secret\":\"` + secret + `\"}`
			line := `{"credentials":"` + inner + `"}`
			if _, ok := match(line); !ok {
				t.Errorf("match is silent about the escaped JSON with client_id %q", id)
			}
			assertOneSpan(t, line, inner)
		})
	}
}

// byteEntropy is Shannon entropy over BYTES — the measure the gate used to
// apply — kept here only to show the fixture above sits below the floor on it.
func byteEntropy(s string) float64 {
	var counts [256]int
	for i := 0; i < len(s); i++ {
		counts[s[i]]++
	}
	var h float64
	for _, c := range counts {
		if c > 0 {
			p := float64(c) / float64(len(s))
			h -= p * math.Log2(p)
		}
	}
	return h
}
