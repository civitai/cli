package redact

import (
	"strings"
	"testing"
)

// Every fixture below is INVENTED, and shaped like the real thing rather than
// like a textbook example: internal/credscan stays quiet about values that name
// themselves as fake (`example`, `test`, `dummy`, `localhost`, …), so a fixture
// borrowed from documentation would pass through un-redacted and the row would
// prove nothing about the rule. The secret in every row is distinct from the
// secret in every other row, and from the home directory and the configured
// tokens, so a rule that fired for the wrong row cannot satisfy this one.

const (
	testHome  = "/home/mkestrel"
	testToken = "c1v1tai4f6b8d0a2c4e6f8091a2b3c4d5"
	// testEnvToken stands in for $CIVITAI_TOKEN when it differs from the stored
	// credential.
	testEnvToken = "9e8d7c6b5a4f3e2d1c0b9a8f7e6d5c4b"
)

func newTestRedactor() *Redactor {
	return New(Options{Home: testHome, Tokens: []string{testToken, testEnvToken}})
}

// redactionRow is one rule's positive control: `in` carries `secret`, and after
// redaction the secret is gone, `wantOut` is the exact result, and exactly
// `wantCount` redactions of `kind` were counted — and of no other kind.
type redactionRow struct {
	name      string
	in        string
	secret    string
	wantOut   string
	kind      string
	wantCount int
}

var redactionRows = []redactionRow{
	// --- 1. token: exact match ------------------------------------------------
	{
		name:      "the configured token, bare, in a sentence",
		in:        "login printed " + testToken + " and then hung",
		secret:    testToken,
		wantOut:   "login printed <token> and then hung",
		kind:      KindToken,
		wantCount: 1,
	},
	{
		name:      "the CIVITAI_TOKEN value, twice",
		in:        "tried " + testEnvToken + " then again " + testEnvToken,
		secret:    testEnvToken,
		wantOut:   "tried <token> then again <token>",
		kind:      KindToken,
		wantCount: 2,
	},
	{
		// The exact rule runs first, so a token inside a header is counted as the
		// token it is, not as the bearer shape it also has.
		name:      "the configured token inside an Authorization header",
		in:        "> Authorization: Bearer " + testToken,
		secret:    testToken,
		wantOut:   "> Authorization: Bearer <token>",
		kind:      KindToken,
		wantCount: 1,
	},

	// --- 2. credential: credscan's matchers, plus PEM and Bearer ---------------
	{
		name:      "a GitHub token",
		in:        "git push said bad credentials for ghp_4Hn8Wq2Zr6Tb0Vx3Ly7Kc1Md5Pf9Sg2Uj6Ea",
		secret:    "ghp_4Hn8Wq2Zr6Tb0Vx3Ly7Kc1Md5Pf9Sg2Uj6Ea",
		wantOut:   "git push said bad credentials for <credential>",
		kind:      KindCredential,
		wantCount: 1,
	},
	{
		name:      "an npm token",
		in:        "npm ERR! 401 with npm_7Rt2Yq9Lm4Zx8Bv1Nc6Kd3Wf0Hs5Pg2Tj8Ua",
		secret:    "npm_7Rt2Yq9Lm4Zx8Bv1Nc6Kd3Wf0Hs5Pg2Tj8Ua",
		wantOut:   "npm ERR! 401 with <credential>",
		kind:      KindCredential,
		wantCount: 1,
	},
	{
		name:      "a JWT",
		in:        "dev token was eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiI4MTkzMDIiLCJhdWQiOiJibG9jayJ9.Zk4Tn8Wq1Xc7Lb3Rv9Hy2Md6Pf0Sg5Uj",
		secret:    "eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiI4MTkzMDIiLCJhdWQiOiJibG9jayJ9.Zk4Tn8Wq1Xc7Lb3Rv9Hy2Md6Pf0Sg5Uj",
		wantOut:   "dev token was <credential>",
		kind:      KindCredential,
		wantCount: 1,
	},
	{
		name:      "an AWS access key id",
		in:        "upload failed: InvalidAccessKeyId AKIA5RT8WQ2ZLMNB7VCX",
		secret:    "AKIA5RT8WQ2ZLMNB7VCX",
		wantOut:   "upload failed: InvalidAccessKeyId <credential>",
		kind:      KindCredential,
		wantCount: 1,
	},
	{
		name:      "a connection string with an embedded password",
		in:        "connect ECONNREFUSED mysql://svc_blocks:Tq7Zw3Lp9Rc5@db-7.prod-internal.net:3306/blocks",
		secret:    "svc_blocks:Tq7Zw3Lp9Rc5",
		wantOut:   "connect ECONNREFUSED mysql://<credential>@db-7.prod-internal.net:3306/blocks",
		kind:      KindCredential,
		wantCount: 1,
	},
	{
		name:      "a strict KEY=value assignment",
		in:        "ran with BUZZ_API_SECRET=Hq3Zt8Wm2Lx7Rb5Vn9Kc and it 500'd",
		secret:    "Hq3Zt8Wm2Lx7Rb5Vn9Kc",
		wantOut:   "ran with BUZZ_API_SECRET=<credential> and it 500'd",
		kind:      KindCredential,
		wantCount: 1,
	},
	{
		name:      "a quoted JSON credential value",
		in:        `config was {"clientSecret": "Pd6Yk1Vr8Nq4Wz2Lb7Tc0Hs"}`,
		secret:    "Pd6Yk1Vr8Nq4Wz2Lb7Tc0Hs",
		wantOut:   `config was {"clientSecret": "<credential>"}`,
		kind:      KindCredential,
		wantCount: 1,
	},
	{
		name:      "a bearer header carrying some other service's credential",
		in:        "< authorization: bearer 8f2c61d09ab347e5b1c0d4f6a7e89321",
		secret:    "8f2c61d09ab347e5b1c0d4f6a7e89321",
		wantOut:   "< authorization: bearer <credential>",
		kind:      KindCredential,
		wantCount: 1,
	},
	{
		name: "a whole PEM private-key block, not just its header",
		in: "ssh said:\n-----BEGIN OPENSSH PRIVATE KEY-----\n" +
			"b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtz\n" +
			"c2gtZWQyNTUxOQAAACD3Qm7Zk1Lx9Rv4Tn8Wq2Yc6Hb0Md5Pf3Sg7Uj1KaAAAA\n" +
			"-----END OPENSSH PRIVATE KEY-----\nthen exited",
		secret:    "c2gtZWQyNTUxOQAAACD3Qm7Zk1Lx9Rv4Tn8Wq2Yc6Hb0Md5Pf3Sg7Uj1KaAAAA",
		wantOut:   "ssh said:\n<credential>\nthen exited",
		kind:      KindCredential,
		wantCount: 1,
	},
	{
		name: "a PEM block cut off before its END line",
		in: "-----BEGIN EC PRIVATE KEY-----\n" +
			"MHcCAQEEIJ4Xt9Wq2Lm7Zk3Rv8Nc1Hb6Yd0Pf5Sg9Uj2Ka4Te7oAoGCCqGSM49",
		secret:    "MHcCAQEEIJ4Xt9Wq2Lm7Zk3Rv8Nc1Hb6Yd0Pf5Sg9Uj2Ka4Te7oAoGCCqGSM49",
		wantOut:   "<credential>",
		kind:      KindCredential,
		wantCount: 1,
	},

	// --- 3. email -------------------------------------------------------------
	{
		name:      "an e-mail address",
		in:        "the review mail went to priya.raman@northwind-labs.io instead",
		secret:    "priya.raman@northwind-labs.io",
		wantOut:   "the review mail went to <email> instead",
		kind:      KindEmail,
		wantCount: 1,
	},
	{
		name:      "a plus-addressed e-mail in a git author line",
		in:        "Author: Dana Okafor <d.okafor+cli@fieldstone.dev>",
		secret:    "d.okafor+cli@fieldstone.dev",
		wantOut:   "Author: Dana Okafor <<email>>",
		kind:      KindEmail,
		wantCount: 1,
	},

	// --- 4. url-query ---------------------------------------------------------
	{
		name:      "a presigned URL's query string",
		in:        "GET https://blobs.civitai.com/dl/8841.safetensors?X-Amz-Signature=6b1f0c9d2e7a4835&X-Amz-Expires=900 returned 403",
		secret:    "6b1f0c9d2e7a4835",
		wantOut:   "GET https://blobs.civitai.com/dl/8841.safetensors returned 403",
		kind:      KindURLQuery,
		wantCount: 1,
	},
	{
		name:      "a query string is dropped and the fragment kept",
		in:        "see https://developer.civitai.com/apps/guide?ref=ck_41d7e2#scopes",
		secret:    "ck_41d7e2",
		wantOut:   "see https://developer.civitai.com/apps/guide#scopes",
		kind:      KindURLQuery,
		wantCount: 1,
	},

	// --- 5. home-path ---------------------------------------------------------
	{
		name:      "this machine's home directory, as a path prefix",
		in:        "ENOENT: " + testHome + "/work/my-app/block.manifest.json",
		secret:    "mkestrel",
		wantOut:   "ENOENT: ~/work/my-app/block.manifest.json",
		kind:      KindHomePath,
		wantCount: 1,
	},
	{
		name:      "the home directory at the very end of the text",
		in:        "cwd was " + testHome,
		secret:    "mkestrel",
		wantOut:   "cwd was ~",
		kind:      KindHomePath,
		wantCount: 1,
	},

	// --- 6. user-path ---------------------------------------------------------
	{
		name:      "another Linux user's home",
		in:        "cache at /home/tvasquez/.npm/_logs/2026-10-11.log",
		secret:    "tvasquez",
		wantOut:   "cache at /home/<user>/.npm/_logs/2026-10-11.log",
		kind:      KindUserPath,
		wantCount: 1,
	},
	{
		name:      "a macOS home",
		in:        "Error: EACCES /Users/lindqvist/Library/pnpm/store",
		secret:    "lindqvist",
		wantOut:   "Error: EACCES /Users/<user>/Library/pnpm/store",
		kind:      KindUserPath,
		wantCount: 1,
	},
	{
		name:      "a Windows home",
		in:        `cannot find C:\Users\hbrandt\AppData\Roaming\civitai\config.yaml`,
		secret:    "hbrandt",
		wantOut:   `cannot find C:\Users\<user>\AppData\Roaming\civitai\config.yaml`,
		kind:      KindUserPath,
		wantCount: 1,
	},
	{
		name:      "a Windows home with JSON-doubled backslashes",
		in:        `"path":"D:\\Users\\oyelaran\\src\\app"`,
		secret:    "oyelaran",
		wantOut:   `"path":"D:\\Users\\<user>\\src\\app"`,
		kind:      KindUserPath,
		wantCount: 1,
	},
	{
		// The home directory is /home/mkestrel; this is a DIFFERENT user whose
		// name merely starts with it. Rule 5 must not turn it into "~2".
		name:      "a user whose name extends the home directory's",
		in:        "owned by /home/mkestrel2/app",
		secret:    "mkestrel2",
		wantOut:   "owned by /home/<user>/app",
		kind:      KindUserPath,
		wantCount: 1,
	},
}

func TestEachRedactionRuleRemovesItsSecret(t *testing.T) {
	// The fixtures are only a table of independent controls if their secrets are
	// pairwise distinct; otherwise one rule firing could satisfy two rows.
	seen := map[string]string{}
	for _, row := range redactionRows {
		if row.secret == testToken || row.secret == testEnvToken || row.kind == KindHomePath {
			// The token rows share the two configured values, and the home rows
			// the one configured home directory, by construction: those rules
			// match what the Redactor was TOLD, so there is one value to vary.
			continue
		}
		if prev, dup := seen[row.secret]; dup {
			t.Fatalf("fixture defect: rows %q and %q share the secret %q", prev, row.name, row.secret)
		}
		seen[row.secret] = row.name
	}

	kinds := map[string]int{}
	for _, row := range redactionRows {
		t.Run(row.name, func(t *testing.T) {
			if !strings.Contains(row.in, row.secret) {
				t.Fatalf("fixture defect: the input does not contain the secret it claims to")
			}
			r := newTestRedactor()
			got := r.Apply(row.in)
			if strings.Contains(got, row.secret) {
				t.Errorf("the secret survived redaction.\n in: %s\nout: %s", row.in, got)
			}
			if got != row.wantOut {
				t.Errorf("redacted text:\n got: %q\nwant: %q", got, row.wantOut)
			}
			counts := r.Counts()
			if len(counts) != 1 || counts[0].Kind != row.kind || counts[0].Count != row.wantCount {
				t.Errorf("counts = %+v, want exactly [{%s %d}]", counts, row.kind, row.wantCount)
			}
		})
		kinds[row.kind]++
	}
	// Every kind the package can report has at least one positive control here.
	for _, k := range kindOrder {
		if kinds[k] == 0 {
			t.Errorf("no row exercises the %q rule — a rule with no positive control is unproven", k)
		}
	}
}

// TestOrdinaryTextIsLeftAlone is the other direction: a report is prose, and a
// redactor that eats prose makes the report useless. Each line is something a
// developer would plausibly write, chosen because it sits next to a rule.
func TestOrdinaryTextIsLeftAlone(t *testing.T) {
	for _, s := range []string{
		"app submit exits 1 with no message when the bundle has no index.html",
		"Bearer authentication is required, but the docs never say which scope",
		"@civitai/app-sdk@0.69.0 has no typed helper for the buzz balance call",
		"ssh: connect to host tunnel port 22: user@host is how the docs spell it",
		"see https://developer.civitai.com/apps/guide#scopes for the page I mean",
		"the token field is empty: token=\"\" in the generated config",
		"exit code 3 from `civitai app status my-app --json`",
		"~/work/my-app is where I ran it",
	} {
		r := newTestRedactor()
		if got := r.Apply(s); got != s {
			t.Errorf("ordinary text was rewritten:\n in: %q\nout: %q\ncounts: %+v", s, got, r.Counts())
		}
		if n := len(r.Counts()); n != 0 {
			t.Errorf("%q: %d kind(s) counted for text nothing was removed from", s, n)
		}
	}
}

// TestCountsAccumulateAcrossFieldsInRuleOrder: one Redactor covers every field
// of a report, and Counts reports in the order the rules run.
func TestCountsAccumulateAcrossFieldsInRuleOrder(t *testing.T) {
	r := newTestRedactor()
	r.Apply("from " + testHome + "/a and " + testHome + "/b")
	r.Apply("mail rosalind.achterberg@quietharbor.org about " + testToken)
	r.Apply("also /Users/nwosu/x")

	got := r.Counts()
	want := []Count{{KindToken, 1}, {KindEmail, 1}, {KindHomePath, 2}, {KindUserPath, 1}}
	if len(got) != len(want) {
		t.Fatalf("counts = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("counts[%d] = %+v, want %+v (full: %+v)", i, got[i], want[i], got)
		}
	}
}

// TestCountsIsNeverNil: the --json envelope publishes `redactions` as an array,
// and a nil slice marshals as null.
func TestCountsIsNeverNil(t *testing.T) {
	if c := New(Options{}).Counts(); c == nil {
		t.Fatal("Counts() returned nil for a redactor that removed nothing; it must be an empty slice")
	}
}

// TestShortTokenIsSkippedAndReported: below MinTokenLen the exact rule would
// shred ordinary words, so it does not run — and says so, rather than leaving
// the caller to assume the token was covered.
func TestShortTokenIsSkippedAndReported(t *testing.T) {
	r := New(Options{Tokens: []string{"ab12", testToken}})
	if n := r.SkippedShortTokens(); n != 1 {
		t.Fatalf("SkippedShortTokens = %d, want 1", n)
	}
	in := "cab12 is a taxi; the long one is " + testToken
	got := r.Apply(in)
	if !strings.Contains(got, "cab12") {
		t.Errorf("a %d-character token was substring-replaced inside an ordinary word: %q", len("ab12"), got)
	}
	if strings.Contains(got, testToken) {
		t.Errorf("the long token survived: %q", got)
	}
	// Positive control on the floor itself: a token exactly MinTokenLen long IS
	// matched.
	edge := "Zq7Lm2Xv"
	if len(edge) != MinTokenLen {
		t.Fatalf("fixture defect: edge token is %d long, want %d", len(edge), MinTokenLen)
	}
	r2 := New(Options{Tokens: []string{edge}})
	if got := r2.Apply("x " + edge + " y"); got != "x <token> y" || r2.SkippedShortTokens() != 0 {
		t.Errorf("a token at the floor was not matched: %q (skipped %d)", got, r2.SkippedShortTokens())
	}
}

// TestHomeOfRootOrEmptyDisablesTheHomeRule: a home of "/" would rewrite every
// absolute path to "~".
func TestHomeOfRootOrEmptyDisablesTheHomeRule(t *testing.T) {
	for _, home := range []string{"", "/", `\`} {
		r := New(Options{Home: home})
		in := "/etc/hosts and /var/log/syslog"
		if got := r.Apply(in); got != in {
			t.Errorf("home %q: %q became %q", home, in, got)
		}
	}
}

// TestLongerTokenIsRemovedWhole: when one configured credential contains
// another, the longer must go first or its tail is left behind.
func TestLongerTokenIsRemovedWhole(t *testing.T) {
	short := "Wq8Zt3Lm6Xr1"
	long := short + "Bv5Nc9Kd2Hf7"
	r := New(Options{Tokens: []string{short, long}})
	got := r.Apply("value " + long + " end")
	if got != "value <token> end" {
		t.Errorf("got %q, want the long token replaced by one placeholder", got)
	}
}

// TestABareMentionOfAKeyHeaderDoesNotEatTheSentence pins why pemBlockRe demands
// the five dashes. credscan's own pattern matches the header WORDS anywhere, so
// those words are removed (a stated over-redaction) — but the block rule, which
// runs to the end of the text when no END line follows, must not open on prose.
func TestABareMentionOfAKeyHeaderDoesNotEatTheSentence(t *testing.T) {
	r := newTestRedactor()
	got := r.Apply("the BEGIN RSA PRIVATE KEY line is missing from the docs example, and step 4 then fails")
	want := "the <credential> line is missing from the docs example, and step 4 then fails"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}
