package redact

import (
	"strings"
	"testing"
)

// These are the audit-round-1 findings on civitai/cli#830, each written as a
// test that was watched to fail against the code it was filed on. Fixtures are
// invented and pairwise distinct, as in redact_test.go.

// --- a URL is handled on the ORIGINAL text, before any placeholder exists ----

// TestURLTokenLosesItsUserinfoAndEverythingFromItsFirstQueryOrFragmentMark: a
// URL is one token — from its scheme to the next white space — and two cuts are
// made on it: everything between `://` and its LAST `@`, and everything from
// its first `?` or `#` to its end.
//
// Round 1 (the first five rows): the query strip ran after the rules that leave
// `<token>` / `<email>` behind and stopped at the `<`; and a URL carrying
// `user:pass@` lost its scheme to the credential span, so its query was never
// seen. Round 2 (the rest): the rewritten rule still named the characters a
// query and a userinfo "end" at, so a quote or backtick in a query, an `@` in a
// password, or a `?` in one, each left the remainder in place under a count
// that said it was gone.
func TestURLTokenLosesItsUserinfoAndEverythingFromItsFirstQueryOrFragmentMark(t *testing.T) {
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
			// The `@` of the address is the token's last `@`, and a `?` precedes
			// it — so (see the password rows below) the whole token goes.
			name:    "an e-mail address is one of several query parameters",
			in:      "open https://app.northwind-labs.io/reset?email=bo.tanaka@corp-mail.io&session=Hd5Yq8Lw2Zr6Tb0Vx3 now",
			leaks:   []string{"bo.tanaka", "corp-mail.io", "Hd5Yq8Lw2Zr6Tb0Vx3", "session="},
			wantOut: "open https://<credential>@ now",
			want:    []Count{{KindCredential, 1}, {KindURLQuery, 1}},
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
			name:    "two URLs, one with a query and one with a query and a fragment, are two redactions",
			in:      "a https://one.saltmarsh.studio/x?p=Qa1Ws2Ed3Rf4 b https://two.saltmarsh.studio/y?q=Zx5Cv6Bn7Mm8#frag",
			leaks:   []string{"Qa1Ws2Ed3Rf4", "Zx5Cv6Bn7Mm8", "frag"},
			wantOut: "a https://one.saltmarsh.studio/x b https://two.saltmarsh.studio/y",
			want:    []Count{{KindURLQuery, 2}},
		},
		// ---- audit round 2 ----
		{
			name:    "a single quote inside the query",
			in:      "fetch https://h.harborlight.dev/p?name='x'&sig=Rk3Vt9Wm2Xq7Zb5Lc1 failed",
			leaks:   []string{"Rk3Vt9Wm2Xq7Zb5Lc1", "sig=", "'x'"},
			wantOut: "fetch https://h.harborlight.dev/p failed",
			want:    []Count{{KindURLQuery, 1}},
		},
		{
			name:    "a double quote inside the query",
			in:      `fetch https://h.harborlight.dev/q?name="y"&sig=Jn6Bh2Gy8Tf4Rd0Es3 failed`,
			leaks:   []string{"Jn6Bh2Gy8Tf4Rd0Es3", "sig="},
			wantOut: "fetch https://h.harborlight.dev/q failed",
			want:    []Count{{KindURLQuery, 1}},
		},
		{
			name:    "a backtick inside the query",
			in:      "fetch https://h.harborlight.dev/r?name=`z`&sig=Pl9Ok8Ij7Uh6Yg5Tf4 failed",
			leaks:   []string{"Pl9Ok8Ij7Uh6Yg5Tf4", "sig="},
			wantOut: "fetch https://h.harborlight.dev/r failed",
			want:    []Count{{KindURLQuery, 1}},
		},
		{
			name:    "a secret in the fragment",
			in:      "redirected to https://app.harborlight.dev/cb#access_token=Mz4Nx8Bc2Vd6Fg0Hj5 then closed",
			leaks:   []string{"Mz4Nx8Bc2Vd6Fg0Hj5", "access_token"},
			wantOut: "redirected to https://app.harborlight.dev/cb then closed",
			want:    []Count{{KindURLQuery, 1}},
		},
		{
			name:    "an @ inside the password",
			in:      "dial postgres://svc_tides:p@ssw0rdQv7Lm3@db.harborlight.dev:5432/tides refused",
			leaks:   []string{"ssw0rdQv7Lm3", "svc_tides"},
			wantOut: "dial postgres://<credential>@db.harborlight.dev:5432/tides refused",
			want:    []Count{{KindCredential, 1}},
		},
		{
			// A `?` cannot legally sit in a userinfo, so once one precedes the
			// last `@` nothing after that `@` can be trusted to be a host: the
			// same shape is `…/p?to=a@SECRET`. The rest of the token goes too.
			name:    "a ? inside the password",
			in:      "dial postgres://deploy:Xk9?mQ2vLp7wHs4@db2.harborlight.dev:5432/app refused",
			leaks:   []string{"Xk9", "mQ2vLp7wHs4", "deploy"},
			wantOut: "dial postgres://<credential>@ refused",
			want:    []Count{{KindCredential, 1}, {KindURLQuery, 1}},
		},
		{
			name:    "a / and a # inside the password",
			in:      "dial redis://cache:Aa1/Bb2#Cc3Dd4Ee5@cache.harborlight.dev:6379/0 refused",
			leaks:   []string{"Aa1", "Bb2", "Cc3Dd4Ee5"},
			wantOut: "dial redis://<credential>@ refused",
			want:    []Count{{KindCredential, 1}, {KindURLQuery, 1}},
		},
		{
			name:    "an @ after the ? keeps nothing that followed it",
			in:      "GET https://api.harborlight.dev/v1/n?cc=@Ty5Ru3Ei1Ow9Pq7La done",
			leaks:   []string{"Ty5Ru3Ei1Ow9Pq7La", "cc="},
			wantOut: "GET https://<credential>@ done",
			want:    []Count{{KindCredential, 1}, {KindURLQuery, 1}},
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
		{"Authorization: Basic", "> Authorization: Basic ZGVwbG95OnN3b3JkZmlzaGJsdWU=", "ZGVwbG95OnN3b3JkZmlzaGJsdWU=", "> Authorization: <credential>"},
		{"Authorization: Bearer with no digit in the value", "authorization: Bearer QwErTyUiOpAsDfGhJkLzXcVbNm", "QwErTyUiOpAsDfGhJkLzXcVbNm", "authorization: <credential>"},
		{"a JSON headers object", `{"Authorization":"Token k8Hs3Lq9Zx2Vb7Nm"}`, "k8Hs3Lq9Zx2Vb7Nm", `{"Authorization":<credential>`},
		// Round 2: a Digest value is a comma-separated list with spaces. Stopping
		// at the first space or comma removed `username=` and sent the response
		// hash, under docs that said Digest was redacted.
		{"Authorization: Digest, every parameter",
			`Authorization: Digest username="Mufasa", realm="api@harborlight.dev", nonce="dcd98b7102dd2f0e8b11d0f6", response="6629fae49393a05397450978507c4ef1"`,
			"6629fae49393a05397450978507c4ef1", "Authorization: <credential>"},
		{"an Authorization scheme this package has never heard of", "Authorization: AWS4-HMAC-SHA256 Credential=AKIDharbor/20261014, Signature=fe5f80f77d5fa3beca038a248ff027d0",
			"fe5f80f77d5fa3beca038a248ff027d0", "Authorization: <credential>"},
		{"only that line: the next one is kept", "Authorization: Bearer Hy6Tg5Rf4Ed3Ws2Qa1\nHost: api.harborlight.dev", "Hy6Tg5Rf4Ed3Ws2Qa1", "Authorization: <credential>\nHost: api.harborlight.dev"},
		// Round 3: the round-2 rewrite allowed only spaces and tabs between the
		// separator and the value, so a value on the NEXT line — how a
		// pretty-printed JSON headers object and a folded header are written —
		// went out untouched, where the round-1 rule had removed it.
		{"the value on the next line, JSON-quoted, LF",
			"  \"Authorization\":\n    \"Basic dXNlcjpwYXNzd29yZA==\",\n  \"Accept\": \"*/*\"", "dXNlcjpwYXNzd29yZA==",
			"  \"Authorization\":\n    <credential>\n  \"Accept\": \"*/*\""},
		{"the value on the next line, JSON-quoted, CRLF",
			"  \"Authorization\":\r\n    \"Basic c3ZjOnRpZGVwb29sczEyMw==\",\r\n  \"Accept\": \"*/*\"", "c3ZjOnRpZGVwb29sczEyMw==",
			"  \"Authorization\":\r\n    <credential>\r\n  \"Accept\": \"*/*\""},
		{"the value on the next line, bare, no digit in it",
			"Authorization:\n  Bearer ZxCvBnMaSdFgHjKlQwErTyUi\nHost: api.harborlight.dev", "ZxCvBnMaSdFgHjKlQwErTyUi",
			"Authorization:\n  <credential>\nHost: api.harborlight.dev"},
		{"the value on the next line, bare, CRLF",
			"Authorization:\r\n\tBearer Lk8Jh7Gf6Ds5Aq4Pw3Oe2\r\nHost: api.harborlight.dev", "Lk8Jh7Gf6Ds5Aq4Pw3Oe2",
			"Authorization:\r\n\t<credential>\r\nHost: api.harborlight.dev"},
		{"the value after a blank line", "Authorization =\n\n  Token Mn2Bv3Cx4Zl5Kj6Hg7\ndone", "Mn2Bv3Cx4Zl5Kj6Hg7", "Authorization =\n\n  <credential>\ndone"},
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
	// Text these rules do leave alone. (What they wrongly take is in
	// knownOverRedactions, not asserted away here.)
	for _, s := range []string{
		"the x-api-key header is required but the docs never say so",
		"api-key: required",
		"Authorization is checked before the scope",
	} {
		r := newTestRedactor()
		if got := r.Apply(s); got != s {
			t.Errorf("rewritten: %q -> %q", s, got)
		}
	}
}

// TestAuthorizationSeparatorMustBeOnTheHeaderLine (audit round 4): line breaks
// are allowed AFTER the `:` or `=`, not before it. With them allowed before, a
// setext heading underline and a definition on a later line were read as the
// header's separator.
func TestAuthorizationSeparatorMustBeOnTheHeaderLine(t *testing.T) {
	for _, in := range []string{
		"Authorization\n=====\nSome text",
		"needs authorization\n\n: definition text",
	} {
		r := newTestRedactor()
		if got := r.Apply(in); got != in {
			t.Errorf("rewritten:\n in: %q\nout: %q", in, got)
		}
		assertCounts(t, r.Counts(), []Count{})
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
