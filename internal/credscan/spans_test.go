package credscan

import (
	"strings"
	"testing"
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
