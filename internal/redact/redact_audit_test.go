package redact

import (
	"strings"
	"testing"
)

// These are the audit-round-1 findings on civitai/cli#830, each written as a
// test that was watched to fail against the code it was filed on. Fixtures are
// invented and pairwise distinct, as in redact_test.go.

// --- a URL is handled on the ORIGINAL text, before any placeholder exists ----

// TestURLQueryIsStrippedWholeWhateverItContains: the query strip used to run
// AFTER the rules that leave `<token>` / `<email>` behind, and its terminator
// class excluded `<`, so a placeholder inside a query ended the strip and the
// rest of the query was sent — while the count still said the query was
// dropped. And a URL carrying `user:pass@` lost its scheme to the credential
// span, so the query rule never saw a URL at all.
func TestURLQueryIsStrippedWholeWhateverItContains(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		leaks   []string
		wantOut string
		want    []Count
	}{
		{
			name:    "the configured token is one of several query parameters",
			in:      "GET https://civitai.com/api/v1/models/1?token=" + testToken + "&sig=Zx9Kq2Lm7Rv4Tn8Wc1 failed",
			leaks:   []string{testToken, "Zx9Kq2Lm7Rv4Tn8Wc1", "sig="},
			wantOut: "GET https://civitai.com/api/v1/models/1 failed",
			want:    []Count{{KindURLQuery, 1}},
		},
		{
			name:    "an e-mail address is one of several query parameters",
			in:      "open https://app.northwind-labs.io/reset?email=bo.tanaka@corp-mail.io&session=Hd5Yq8Lw2Zr6Tb0Vx3 now",
			leaks:   []string{"bo.tanaka@corp-mail.io", "Hd5Yq8Lw2Zr6Tb0Vx3", "session="},
			wantOut: "open https://app.northwind-labs.io/reset now",
			want:    []Count{{KindURLQuery, 1}},
		},
		{
			name:    "a URL with userinfo AND a query",
			in:      "fetch https://deploy:Pw7Zk3Lq9Rc5Xm@registry.fieldstone.dev/v2/pkg?x=1&y=Nb4Vc8Xz2Lk6Jh0Gf5 timed out",
			leaks:   []string{"Pw7Zk3Lq9Rc5Xm", "Nb4Vc8Xz2Lk6Jh0Gf5", "deploy:", "x=1"},
			wantOut: "fetch https://<credential>@registry.fieldstone.dev/v2/pkg timed out",
			want:    []Count{{KindCredential, 1}, {KindURLQuery, 1}},
		},
		{
			name:    "the user's own angle bracket inside a query does not end the strip",
			in:      "see https://docs.quietharbor.org/p?id=<id>&key=Tg6Yh2Uj8Ik4Ol0Pq9 for it",
			leaks:   []string{"Tg6Yh2Uj8Ik4Ol0Pq9", "key="},
			wantOut: "see https://docs.quietharbor.org/p for it",
			want:    []Count{{KindURLQuery, 1}},
		},
		{
			name:    "two URLs, each with a query, are two redactions",
			in:      "a https://one.saltmarsh.studio/x?p=Qa1Ws2Ed3Rf4 b https://two.saltmarsh.studio/y?q=Zx5Cv6Bn7Mm8#frag",
			leaks:   []string{"Qa1Ws2Ed3Rf4", "Zx5Cv6Bn7Mm8"},
			wantOut: "a https://one.saltmarsh.studio/x b https://two.saltmarsh.studio/y#frag",
			want:    []Count{{KindURLQuery, 2}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestRedactor()
			got := r.Apply(tc.in)
			for _, leak := range tc.leaks {
				if !strings.Contains(tc.in, leak) {
					t.Fatalf("fixture defect: the input does not contain %q", leak)
				}
				if strings.Contains(got, leak) {
					t.Errorf("%q survived:\n in: %s\nout: %s", leak, tc.in, got)
				}
			}
			if got != tc.wantOut {
				t.Errorf("redacted text:\n got: %q\nwant: %q", got, tc.wantOut)
			}
			assertCounts(t, r.Counts(), tc.want)
		})
	}
}

func assertCounts(t *testing.T, got, want []Count) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("counts = %+v, want %+v", got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("counts = %+v, want %+v", got, want)
			return
		}
	}
}

// --- a Windows user name can contain a space ---------------------------------

func TestWindowsUserNameWithASpaceIsRemovedWhole(t *testing.T) {
	cases := []struct{ name, in, leak, want string }{
		{"doubled backslashes", `"cwd":"C:\\Users\\Ingrid Solberg\\src\\app"`, "Solberg", `"cwd":"C:\\Users\\<user>\\src\\app"`},
		{"single backslashes", `cannot open C:\Users\Tomas Achterberg\AppData\x.log`, "Achterberg", `cannot open C:\Users\<user>\AppData\x.log`},
		// With no separator after it the name's extent is unknowable; the
		// first word is still removed, and a following word is left (stated).
		{"the control: a one-word name still ends at whitespace", `in C:\Users\wkowalczyk and elsewhere`, "wkowalczyk", `in C:\Users\<user> and elsewhere`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestRedactor()
			got := r.Apply(tc.in)
			if strings.Contains(got, tc.leak) {
				t.Errorf("%q survived: %q", tc.leak, got)
			}
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
			assertCounts(t, r.Counts(), []Count{{KindUserPath, 1}})
		})
	}
}

// --- header-borne credentials -------------------------------------------------

func TestHeaderCredentialsAreRemoved(t *testing.T) {
	cases := []struct{ name, in, secret, want string }{
		{"Authorization: Basic", "> Authorization: Basic ZGVwbG95OnN3b3JkZmlzaGJsdWU=", "ZGVwbG95OnN3b3JkZmlzaGJsdWU=", "> Authorization: Basic <credential>"},
		{"Authorization: Bearer with no digit in the value", "authorization: Bearer QwErTyUiOpAsDfGhJkLzXcVbNm", "QwErTyUiOpAsDfGhJkLzXcVbNm", "authorization: Bearer <credential>"},
		{"a JSON headers object", `{"Authorization":"Token k8Hs3Lq9Zx2Vb7Nm"}`, "k8Hs3Lq9Zx2Vb7Nm", `{"Authorization":"Token <credential>"}`},
		{"x-api-key header", "< x-api-key: f3a91c07d2b84e65a0c9", "f3a91c07d2b84e65a0c9", "< x-api-key: <credential>"},
		{"api-key in a form body", "body was api-key=7d1e4b9a2c6f8035e1d7 and nothing else", "7d1e4b9a2c6f8035e1d7", "body was api-key=<credential> and nothing else"},
	}
	seen := map[string]bool{}
	for _, tc := range cases {
		if seen[tc.secret] {
			t.Fatalf("fixture defect: %q is used twice", tc.secret)
		}
		seen[tc.secret] = true
		t.Run(tc.name, func(t *testing.T) {
			r := newTestRedactor()
			got := r.Apply(tc.in)
			if strings.Contains(got, tc.secret) {
				t.Errorf("the secret survived: %q", got)
			}
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
			assertCounts(t, r.Counts(), []Count{{KindCredential, 1}})
		})
	}
	// And prose that merely names those headers is left alone.
	for _, s := range []string{
		"the x-api-key header is required but the docs never say so",
		"api-key: required",
		"Authorization is checked before the scope",
	} {
		r := newTestRedactor()
		if got := r.Apply(s); got != s {
			t.Errorf("prose was rewritten: %q -> %q", s, got)
		}
	}
}

// TestStatedMissesAreStillMisses pins the residual list in the package doc. If
// one of these starts being redacted, the doc is stale in the safe direction —
// update it. It exists so the list is a measurement and not a guess.
func TestStatedMissesAreStillMisses(t *testing.T) {
	for name, s := range map[string]string{
		"a bare bearer value with no digit": "sent Bearer QwErTyUiOpAsDfGhJkLzXcVbNm to it",
		"a scheme-less URL with a query":    "curl cdn.fieldstone.dev/f.bin?sig=Yt6Ru8Ei0Ow2Pq4La",
		"an api key with no digit":          "x-api-key: QwErTyUiOpAsDfGhJk",
	} {
		r := newTestRedactor()
		if got := r.Apply(s); got != s {
			t.Errorf("%s is documented as NOT covered, but was rewritten (%q). Remove it from the residual list in the package doc.", name, got)
		}
	}
}

// --- counts describe what is SENT ----------------------------------------------

// TestApplyClippedCountsOnlyWhatSurvivesTheCut: the error excerpt is cut after
// redaction, and the counts used to cover everything that was READ — up to a
// megabyte — rather than the few hundred characters that are sent.
func TestApplyClippedCountsOnlyWhatSurvivesTheCut(t *testing.T) {
	// Three home paths in the first 40 characters, then 300 more far past the cut.
	head := testHome + "/a " + testHome + "/b " + testHome + "/c\n"
	tail := strings.Repeat("mail audit-"+"log@quietharbor.org about "+testHome+"/z\n", 300)

	r := newTestRedactor()
	got, cut := r.ApplyClipped(head+tail, 40)
	if !cut {
		t.Fatal("the text is far longer than 40 characters after redaction; cut must be true")
	}
	if want := "~/a ~/b ~/c\nmail <email> about ~/z\nmail"; got != want {
		t.Errorf("clipped text:\n got: %q\nwant: %q", got, want)
	}
	// Four home paths and one e-mail are IN the 40 characters. 299 more of each
	// were redacted beyond the cut and must not be counted.
	assertCounts(t, r.Counts(), []Count{{KindEmail, 1}, {KindHomePath, 4}})

	// A cut that would land inside a placeholder backs up to the placeholder's
	// start: half a placeholder is neither counted nor sent.
	r2 := newTestRedactor()
	got2, cut2 := r2.ApplyClipped("ab "+testToken+" cd", 6)
	if got2 != "ab" || !cut2 {
		t.Errorf("got %q, %v — want %q, true (never `ab <to`)", got2, cut2, "ab")
	}
	assertCounts(t, r2.Counts(), []Count{})

	// CONTROL: with room for everything nothing is cut and the counts are the
	// full ones.
	r3 := newTestRedactor()
	got3, cut3 := r3.ApplyClipped("ab "+testToken+" cd", 500)
	if got3 != "ab <token> cd" || cut3 {
		t.Errorf("got %q, %v — want the whole redacted text, false", got3, cut3)
	}
	assertCounts(t, r3.Counts(), []Count{{KindToken, 1}})
}

// TestInternalMarkersNeverReachTheOutput: redaction tracks what it replaced with
// noncharacter code points. None may survive into the result, and one already in
// the input must not be mistaken for a redaction.
func TestInternalMarkersNeverReachTheOutput(t *testing.T) {
	r := newTestRedactor()
	in := "x \uFDD0\uFDD4 " + testHome + "/p y"
	got := r.Apply(in)
	for _, c := range got {
		if c >= 0xFDD0 && c <= 0xFDEF {
			t.Fatalf("a marker code point is in the output: %q", got)
		}
	}
	if got != "x  ~/p y" {
		t.Errorf("got %q", got)
	}
	assertCounts(t, r.Counts(), []Count{{KindHomePath, 1}})
}
