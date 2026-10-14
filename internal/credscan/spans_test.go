package credscan

import (
	"strings"
	"testing"
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

// nonASCIISecret returns a value of exactly n CHARACTERS that passes A2's gate
// and carries a two-byte character in every eighth position, so its byte length
// is well above n.
func nonASCIISecret(n int) string {
	const alphabet = "Kq9Zx2Lm7Rb5Tn8Vc4Hd6Yw1Pf3Sg0Uj"
	var b strings.Builder
	for i := 0; i < n; i++ {
		if i%8 == 7 {
			b.WriteRune('é')
			continue
		}
		b.WriteByte(alphabet[(i*7)%len(alphabet)])
	}
	return b.String()
}

// TestSpansCoverANonASCIIValueLongerThanTheCaptureBound: assignRe's `{1,512}`
// counts CHARACTERS (Go's regexp matches runes), so the capture of a value
// carrying non-ASCII text stops at 512 characters, which is more than 512 bytes.
// A check that compared the capture's BYTE length to the bound never saw the
// capture as cut, so the span ended at character 512 and redaction sent the
// rest of the secret. The ASCII test above cannot see this: there, characters
// and bytes are the same number.
func TestSpansCoverANonASCIIValueLongerThanTheCaptureBound(t *testing.T) {
	secret := nonASCIISecret(700)
	if utf8.RuneCountInString(secret) != 700 || len(secret) <= 700 {
		t.Fatalf("CONTROL failure: fixture is %d characters / %d bytes; it must be 700 characters and more bytes",
			utf8.RuneCountInString(secret), len(secret))
	}
	for name, line := range map[string]string{
		"unquoted": "SECRET=" + secret + " trailing words",
		"quoted":   `{"sessionSecret": "` + secret + `", "other": 1}`,
	} {
		t.Run(name, func(t *testing.T) {
			spans := Spans(line)
			if len(spans) != 1 {
				t.Fatalf("want 1 span, got %d: %+v", len(spans), spans)
			}
			if got := line[spans[0].Start:spans[0].End]; got != secret {
				t.Errorf("the span covers %d of the value's %d characters; %d characters of the secret are left outside it",
					utf8.RuneCountInString(got), utf8.RuneCountInString(secret),
					utf8.RuneCountInString(secret)-utf8.RuneCountInString(got))
			}
		})
	}
	// CONTROL: values exactly at the bound and one character either side of it
	// are each one whole span.
	for _, n := range []int{511, 512, 513} {
		v := nonASCIISecret(n)
		spans := Spans("SECRET=" + v + " x")
		if len(spans) != 1 || spans[0].End-spans[0].Start != len(v) {
			t.Errorf("a %d-character value (%d bytes): %+v", n, len(v), spans)
		}
	}
}
