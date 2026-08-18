package credscan

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Issue #464: the packager's exclusions are NAME rules, so a credential in an
// ordinarily-named file ships. These tests are the detector's controls in both
// directions — what it must catch, what it must stay silent about, and the one
// property the whole feature exists for: it never carries the VALUE.
//
// 🔴 EVERY EXPECTATION HERE IS A LITERAL. Nothing is derived from the regexes,
// the gate constants or the format table under test: a test whose expectation is
// computed from the value under test cannot see that value change. The fixtures
// are picked so no planted value can coincide with a constant an assertion names.

// mustWrite writes rel (slash-separated) under dir, creating parents.
func mustWrite(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// scanFindings drives the real Scan and returns just the findings, so the tests
// that predate the byte-budget Report read the same way. The Report's own fields
// have their own tests — see TestScanBudgetStopsAndSaysSo.
func scanFindings(dir string, files []string) []Finding {
	return Scan(dir, files).Findings
}

// labelsAt returns the labels reported for rel, so a test can assert what a file
// contributed without depending on the order files were given in.
func labelsAt(findings []Finding, rel string) []string {
	var out []string
	for _, f := range findings {
		if f.Path == rel {
			out = append(out, f.Label)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// POSITIVE CONTROLS — the shapes the issue measured shipping
// ---------------------------------------------------------------------------

// TestScanCatchesTheIssuesOwnThreeFiles is the regression test. #464 measured
// `secrets.json`, `src/credentials.yaml` and `config.toml` all packaged, with
// their marker strings surviving into the bundle bytes, and NOTHING said so —
// the #435 drop-messaging is silent here because nothing was dropped.
func TestScanCatchesTheIssuesOwnThreeFiles(t *testing.T) {
	dir := t.TempDir()
	// 🔴 A JSON KEY IS QUOTED. This file is the reason the key pattern's trailing
	// character class has to include `"`: an earlier revision omitted it and
	// missed this exact file — the issue's headline case — while looking correct.
	mustWrite(t, dir, "secrets.json", "{\n  \"API_SECRET\": \"9f3a2b7c1d4e5f60718293a4b5c6d7e8\"\n}\n")
	mustWrite(t, dir, "src/credentials.yaml", "password: hunter2-Kx9pQ4mZ7vT1\n")
	mustWrite(t, dir, "config.toml", "[auth]\ntoken = \"gA7xQ2mV9pL4zR8nB3kW\"\n")

	files := []string{"config.toml", "secrets.json", "src/credentials.yaml"}
	got := scanFindings(dir, files)
	if len(got) != 3 {
		t.Fatalf("Scan reported %d finding(s), want 3 (one per planted file): %+v", len(got), got)
	}

	for _, want := range []struct {
		path  string
		line  int
		label string
	}{
		{"config.toml", 2, "token"},
		{"secrets.json", 2, "API_SECRET"},
		{"src/credentials.yaml", 1, "password"},
	} {
		var found bool
		for _, f := range got {
			if f.Path == want.path {
				found = true
				if f.Line != want.line {
					t.Errorf("%s reported at line %d, want %d", f.Path, f.Line, want.line)
				}
				if f.Label != want.label {
					t.Errorf("%s labelled %q, want %q", f.Path, f.Label, want.label)
				}
			}
		}
		if !found {
			t.Errorf("nothing reported for %s, which #464 measured shipping with a credential in it", want.path)
		}
	}
}

// TestScanCatchesEnvProductionAtTheRoot covers the one packaged file most likely
// to hold a real secret. `.env.production` is ALLOW-LISTED by name because the
// platform build reads it, and keptEnvFiles' own doc comment says keeping a file
// is a statement about where it goes, never that it is safe. So it is in scope.
func TestScanCatchesEnvProductionAtTheRoot(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, ".env.production",
		"VITE_BLOCK_ALLOWED_PARENT_ORIGINS=https://civitai.com\nAPI_SECRET=7f3Kq9Zx2Lm5Rb8Nv4Tc\n")

	got := scanFindings(dir, []string{".env.production"})
	if len(got) != 1 {
		t.Fatalf("Scan reported %d finding(s) over .env.production, want 1: %+v", len(got), got)
	}
	if got[0].Line != 2 {
		t.Errorf("reported line %d, want 2 — line 1 is an ordinary origin allow-list", got[0].Line)
	}
	if got[0].Label != "API_SECRET" {
		t.Errorf("label = %q, want \"API_SECRET\"", got[0].Label)
	}
}

// TestAssignmentMatchesAQuotedJSONKey pins the defect that a positive control
// caught and review did not: with `"` missing from the key pattern's trailing
// class, a JSON key cannot be matched at all, and the detector misses the file
// the issue is named after. It is asserted at the line level so the failure
// message names the pattern rather than a file listing.
func TestAssignmentMatchesAQuotedJSONKey(t *testing.T) {
	for _, line := range []string{
		`  "API_SECRET": "9f3a2b7c1d4e5f60718293a4b5c6d7e8",`,
		`  'apiToken': 'Q8vN2mR7kX4zL9pW3tB6',`,
		`  "auth.password" : "Zx7Z2pQ9mK4vR8nT1yB3"`,
	} {
		label, ok := match(line)
		if !ok {
			t.Errorf("no match on %q — a quoted key is the ordinary spelling in JSON and YAML", line)
			continue
		}
		if strings.ContainsAny(label, `"'`) {
			t.Errorf("label %q still carries quoting from the source line", label)
		}
	}
}

// TestKnownFormatsAreAllCaught covers detector B end to end: every credential
// shape that identifies itself without any assignment. Values are synthetic
// but SHAPED like the real thing — a textbook fixture (`AKIAIOSFODNN7EXAMPLE`)
// would be allowlisted by half the scanners in the world and prove nothing.
func TestKnownFormatsAreAllCaught(t *testing.T) {
	for _, tc := range []struct {
		name  string
		line  string
		label string
	}{
		{"PEM", "-----BEGIN RSA PRIVATE KEY-----", "PEM private key block"},
		{"PEM openssh", "-----BEGIN OPENSSH PRIVATE KEY-----", "PEM private key block"},
		{"JWT", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiI0MjcxOTgifQ.7Qk3vZ2mXn9RtLpB4wYc1Ud8gHf6Ka0Sj", "JWT"},
		{"AWS", "AKIA3XQ7ZTLMNBVCXZ12", "AWS access key id"},
		{"AWS temporary", "ASIA7HQ2PLMNBVCXZQ44", "AWS access key id"},
		{"GitHub", "ghp_9Kq2mZx7Lb4Rv8Nc3Tp6Wy1Ud5Ha0Sj2GfQ7pR4tE", "GitHub token"},
		// 🔴 ASSEMBLED, NOT WRITTEN OUT — and the reason is a live external control
		// on how realistic these fixtures are: written contiguously, GitHub's own
		// push protection refuses the push ("Slack API Token", this line). The
		// value the detector sees is identical; only the bytes in this file
		// differ. If another row starts being refused, split it the same way
		// rather than allowlisting a secret.
		{"Slack", "xox" + "b-2451234567-1234567890123-AbCdEfGhIjKlMnOpQrSt", "Slack token"},
		{"OpenAI", "sk-9Kq2mZx7Lb4Rv8Nc3Tp6Wy1Ud5Ha0Sj2GfQ7pR4tE1mV6", "OpenAI API key"},
		{"Stripe", "sk_live_9Kq2mZx7Lb4Rv8Nc3Tp6", "Stripe API key"},
		{"npm", "npm_9Kq2mZx7Lb4Rv8Nc3Tp6Wy1Ud5Ha0Sj2GfQ7", "npm token"},
		{"URL with a password", `DATABASE_URL=postgres://admin:Kq9Zx2Lm5Rb8@db.internal:5432/app`, "URL with an embedded password"},
		{"URL with a password, no credential word in the key", `const dsn = "mongodb://svc:Nv4Tc7f3Kq9Z@cluster0.civitai-internal.net/db";`, "URL with an embedded password"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			label, ok := match(tc.line)
			if !ok {
				t.Fatalf("no match on a %s-shaped credential", tc.name)
			}
			if label != tc.label {
				t.Errorf("label = %q, want %q", label, tc.label)
			}
			if strings.Contains(tc.line, label) {
				t.Errorf("label %q is a substring of the credential line — a label must NAME the format, not quote the value", label)
			}
		})
	}
}

// TestEveryKnownFormatIsExercised is the ledger between the format table and the
// corpus above: a format added without a case here would ship untested, and a
// case for a format that no longer exists would look like coverage.
func TestEveryKnownFormatIsExercised(t *testing.T) {
	// The names the table above proves, written out. NOT derived from
	// knownFormats — a list computed from the thing under test agrees with it by
	// construction.
	covered := map[string]bool{
		"PEM private key block":         true,
		"JWT":                           true,
		"AWS access key id":             true,
		"GitHub token":                  true,
		"Slack token":                   true,
		"OpenAI API key":                true,
		"Stripe API key":                true,
		"npm token":                     true,
		"URL with an embedded password": true,
	}
	if len(knownFormats) != 9 {
		t.Errorf("knownFormats holds %d format(s); the corpus in TestKnownFormatsAreAllCaught proves 9. "+
			"Add a case there for the new one (or drop the stale name here) — an unexercised format is not coverage.",
			len(knownFormats))
	}
	for _, f := range knownFormats {
		if !covered[f.name] {
			t.Errorf("format %q has no case in TestKnownFormatsAreAllCaught", f.name)
		}
		delete(covered, f.name)
	}
	for name := range covered {
		t.Errorf("TestKnownFormatsAreAllCaught claims a format %q that knownFormats no longer has", name)
	}
}

// ---------------------------------------------------------------------------
// NEGATIVE CONTROLS — noise is the failure mode
// ---------------------------------------------------------------------------

// TestOrdinaryCodeIsSilent is the rate half of the design. A warning that fires
// on every run trains authors to ignore it, and then it is worse than nothing:
// measured over 244 real project directories, A2 ∪ B fired on ONE. Each line
// here is an ordinary spelling that a keyword-only detector (measured at 86.1%
// of projects) flags and this one must not.
func TestOrdinaryCodeIsSilent(t *testing.T) {
	for _, tc := range []struct {
		why  string
		line string
	}{
		{"a keyword as a literal STRING, not a value", `const label = "password";`},
		{"an env READ, not a value", `const apiKey = process.env.VITE_API_KEY;`},
		{"an import.meta env read", `const token = import.meta.env.VITE_TOKEN;`},
		{"a dotted reference expression", `const token = options.credentials.token;`},
		{"a numeric constant", `const PASSWORD_MIN = 8;`},
		{"a placeholder", `const apiKey = "your-api-key-here";`},
		{"an angle-bracket placeholder", `const secret = "<YOUR_SECRET>";`},
		{"a shell/template interpolation", `const token = "${TOKEN}";`},
		{"a bare shell variable reference, long enough to clear the length floor", `API_SECRET=$CIVITAI_DEPLOY_TOKEN_2`},
		// 🔴 THE CORPUS REGRESSION FROM BOUNDING THE VALUE CAPTURE, distilled.
		// The real line is a JSON fragment inside a markdown table with prose
		// AFTER it: while the capture ran to end of line it swept up that prose's
		// spaces and the value was rejected for holding whitespace, so bounding
		// the capture is what made it fire — the corpus rate went 0.41% → 0.82%
		// on this one line. The fixture is the fragment WITHOUT the trailing
		// prose, so it fires on both sides of that change and pins the fix that
		// closed it: an unquoted value stops at the first structural delimiter.
		{"a JSON fragment inside prose", `call with {"model":"openai/gpt-4o-mini","maxTokens":50,"responseFormat":{"type":"json_object"}}]}`},
		// The same class in a regexp literal, which is the OTHER corpus line.
		{"a regexp literal after an assignment", `expect(out.match(/VITE_LIVE_BLOCK_TOKEN=/g)).toHaveLength(1);`},
		{"a lockfile integrity hash under a non-credential key", `      "integrity": "sha512-9Kq2mZx7Lb4Rv8Nc3Tp6Wy1Ud5Ha0Sj2GfQ7pR4tE1mV6yUxKw==",`},
		{"a test/mock value", `const apiToken = "mock-token-9Kq2mZx7Lb4Rv8";`},
		{"a localhost URL", `const credentialsUrl = "http://localhost:3000/auth";`},
		{"a low-entropy repeat", `const token = "aaaaaaaaaaaaaaaa1";`},
		{"a value with whitespace, i.e. prose", `password: this is not a real value 12345`},
		{"a short value", `token: abc123`},
		{"digits only", `access_key = "1234567890123456"`},
		{"letters only", `secret = "abcdefghijklmnop"`},

		// 🔴 THE ISOLATING ROWS. Every row above is rejected by SOME clause, and a
		// measured mutation battery showed that is not the same as rejecting it by
		// the clause it was written for: with the ordinary spellings alone, three
		// mutants SURVIVED a fully green run (the length floor, the interpolation
		// markers and the dotted-reference rule) because an earlier clause —
		// usually "must contain a digit", or the entropy floor — got there first,
		// so the clause under test never executed. Each row below is built to pass
		// every clause EXCEPT the one it names.
		{"under the length floor, and high-entropy so only length can reject it", `token: aB3xQ9zK7`},
		{"one character under the length floor", `token: aB3xQ9zK7mW`},
		{"a shell interpolation the length floor cannot reach", "apiToken = \"${VITE_TOKEN}9f3a2b7c1d\""},
		{"a process.env read concatenated, so it is not a dotted reference", `apiKey = process.env.VITE_KEY+"9f3a2b7c1d4e"`},
		{"an import.meta read concatenated", `apiToken = import.meta.env.VITE_T+"9f3a2b7c1d4e"`},
		{"a dotted reference carrying a digit", `const token = opts.auth2.credentialsToken;`},
		{"a placeholder carrying a digit", `const apiSecret = "YourSecret9f3a2b7c1d4e";`},
		{"an angle-bracket placeholder carrying a digit", `const apiSecret = "<YOUR_SECRET_9f3a2b7c1d4e>";`},
		{"a high-entropy value that names itself a test value", `const apiSecret = "K9x2Vq7Lm4Rb8-test-Nc3Tp6";`},
		{"a high-entropy value pointing at a dev host", `const apiToken = "K9x2Vq7Lm4Rb8dev.Nc3Tp6";`},
	} {
		t.Run(tc.why, func(t *testing.T) {
			if label, ok := match(tc.line); ok {
				t.Errorf("fired on %s:\n  %s\n  labelled %q — this is ordinary code, and a warning that fires on it gets ignored",
					tc.why, tc.line, label)
			}
		})
	}
}

// TestGateDiscriminatesWithinOneFile is the reachability control for A2's gate.
// Two assignments, same file, same key shape, differing ONLY in the value: one
// the gate rejects and one it accepts. A gate that fired on both, or on neither,
// would satisfy a one-sided test while being vacuous.
func TestGateDiscriminatesWithinOneFile(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "src/config.ts", strings.Join([]string{
		`export const LOW_ENTROPY_TOKEN = "aaaaaaaaaaaa1";`,         // rejected: 0.39 bits/char
		`export const HIGH_ENTROPY_TOKEN = "q7W2xE9rT4yU1iO6pA3s";`, // accepted: ~4.3 bits/char
	}, "\n")+"\n")

	got := scanFindings(dir, []string{"src/config.ts"})
	if len(got) != 1 {
		t.Fatalf("Scan reported %d finding(s), want exactly 1 — the fixture holds one value the gate must "+
			"reject and one it must accept: %+v", len(got), got)
	}
	if got[0].Line != 2 {
		t.Errorf("reported line %d, want 2. Line 1 is the low-entropy value the gate must reject and line 2 "+
			"the high-entropy one it must accept; reporting line 1 means the gate is inverted, and reporting "+
			"both means it is not running.", got[0].Line)
	}
	if got[0].Label != "HIGH_ENTROPY_TOKEN" {
		t.Errorf("label = %q, want \"HIGH_ENTROPY_TOKEN\"", got[0].Label)
	}
}

// TestNoTestFileCarveOut pins the absence of the carve-out that would have taken
// the measured false-positive rate to zero. Both known corpus false positives
// are in a `.test.ts`, and excluding test files would zero them — which is the
// reason NOT to: a credential in a test file is uploaded, reviewed and deployed
// identically to one anywhere else, and a rule shaped like "the files where our
// false positives happen to live" is how a guard is hollowed out later.
func TestNoTestFileCarveOut(t *testing.T) {
	dir := t.TempDir()
	body := "const apiSecret = \"7f3Kq9Zx2Lm5Rb8Nv4Tc\";\n"
	mustWrite(t, dir, "src/thing.test.ts", body)
	mustWrite(t, dir, "src/thing.spec.js", body)
	mustWrite(t, dir, "tests/fixture.json", "{\"API_SECRET\": \"7f3Kq9Zx2Lm5Rb8Nv4Tc\"}\n")
	mustWrite(t, dir, "__mocks__/auth.ts", body)

	files := []string{"__mocks__/auth.ts", "src/thing.spec.js", "src/thing.test.ts", "tests/fixture.json"}
	got := scanFindings(dir, files)
	if len(got) != len(files) {
		t.Fatalf("Scan reported %d finding(s) over %d test-shaped files, want one each. A carve-out for test "+
			"files is deliberately absent: %+v", len(got), len(files), got)
	}
}

// ---------------------------------------------------------------------------
// SCOPE, VALUE-SAFETY, BINARY
// ---------------------------------------------------------------------------

// TestScanReadsOnlyTheFilesItWasGiven is the scoping control. The scan runs over
// pkgzip.Result.Files precisely so a warning can never name a file the packager
// DROPPED — the alternative, re-deriving the exclusion rules here, is a second
// copy that drifts from the first (the whole #435 class).
func TestScanReadsOnlyTheFilesItWasGiven(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "src/App.tsx", "export default 1\n")
	// On disk, excluded by the packager, and carrying a value that WOULD match.
	mustWrite(t, dir, ".env.local", "API_SECRET=7f3Kq9Zx2Lm5Rb8Nv4Tc\n")
	mustWrite(t, dir, "node_modules/leak.js", "const apiToken = \"Q8vN2mR7kX4zL9pW3tB6\";\n")

	got := scanFindings(dir, []string{"src/App.tsx"})
	if len(got) != 0 {
		t.Fatalf("Scan looked outside the file list it was given: %+v", got)
	}

	// POSITIVE CONTROL: the two files it must not read DO hold values it would
	// otherwise report, so the zero above is a fact about the scope and not about
	// the fixture.
	for _, rel := range []string{".env.local", "node_modules/leak.js"} {
		if n := len(scanFindings(dir, []string{rel})); n != 1 {
			t.Errorf("control: scanning %s directly reported %d finding(s), want 1 — the zero above would "+
				"otherwise prove nothing", rel, n)
		}
	}
}

// TestScanNeverReturnsTheValue is the leak-prevention property of the feature,
// asserted on the data rather than on a renderer. Finding has no field that can
// hold the matched text; this pins that no field ACQUIRES it either (a label
// built from the whole line would).
func TestScanNeverReturnsTheValue(t *testing.T) {
	// A value that appears nowhere else in this package, so a substring hit can
	// only come from the fixture.
	const planted = "Zq4TvW9mB2xR7kP1nJ6y"
	dir := t.TempDir()
	mustWrite(t, dir, "src/config.ts", "export const apiSecret = \""+planted+"\";\n")
	mustWrite(t, dir, "keys.pem", "-----BEGIN RSA PRIVATE KEY-----\n")

	got := scanFindings(dir, []string{"keys.pem", "src/config.ts"})
	if len(got) != 2 {
		t.Fatalf("Scan reported %d finding(s), want 2: %+v", len(got), got)
	}
	for _, f := range got {
		if strings.Contains(f.Label, planted) {
			t.Errorf("%s:%d label carries the matched VALUE (%q). This output lands in CI logs and terminal "+
				"scrollback: printing it moves the credential into a second durable place.", f.Path, f.Line, f.Label)
		}
	}
	if labels := labelsAt(got, "src/config.ts"); len(labels) != 1 || labels[0] != "apiSecret" {
		t.Errorf("src/config.ts labels = %v, want [apiSecret]", labels)
	}
}

// TestBinaryFilesAreSkipped pins the coverage decision, in both directions: a
// file with a NUL in its first bytes is not scanned, and the SAME bytes without
// the NUL are. Without the second half this is indistinguishable from a fixture
// that never held a credential.
func TestBinaryFilesAreSkipped(t *testing.T) {
	dir := t.TempDir()
	const line = "apiToken = \"Q8vN2mR7kX4zL9pW3tB6\"\n"
	mustWrite(t, dir, "assets/logo.ico", "\x00\x00\x01\x00"+line)
	mustWrite(t, dir, "assets/logo.txt", line)

	if got := scanFindings(dir, []string{"assets/logo.ico"}); len(got) != 0 {
		t.Errorf("scanned a binary file: %+v", got)
	}
	if got := scanFindings(dir, []string{"assets/logo.txt"}); len(got) != 1 {
		t.Errorf("control: the same line WITHOUT a NUL byte reported %d finding(s), want 1 — otherwise the "+
			"skip above proves nothing about binary detection", len(got))
	}
}

// TestPerFileCapHoldsAndTheFileStillCounts covers the pathological input: a file
// with many matching lines contributes at most maxFindingsPerFile, and the file
// is still reported. The expectations are LITERAL (5, 8) so a change to the cap
// turns this red on purpose rather than moving with it.
func TestPerFileCapHoldsAndTheFileStillCounts(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	for _, v := range []string{
		"q7W2xE9rT4yU1iO6pA3s", "Zq4TvW9mB2xR7kP1nJ6y", "7f3Kq9Zx2Lm5Rb8Nv4Tc",
		"gA7xQ2mV9pL4zR8nB3kW", "Q8vN2mR7kX4zL9pW3tB6", "Kx9pQ4mZ7vT1hunter2",
		"Lm5Rb8Nv4Tc7f3Kq9Zx2", "R8nB3kWgA7xQ2mV9pL4z",
	} {
		b.WriteString("apiSecret = \"" + v + "\"\n")
	}
	mustWrite(t, dir, "src/keys.ts", b.String())

	got := scanFindings(dir, []string{"src/keys.ts"})
	if len(got) != 5 {
		t.Fatalf("Scan reported %d finding(s) over a file with 8 matching lines, want 5 (the per-file cap): %+v",
			len(got), got)
	}
	for i, f := range got {
		if f.Line != i+1 {
			t.Errorf("finding %d is at line %d, want %d — the cap must keep the FIRST matches, in order", i, f.Line, i+1)
		}
	}
}

// TestMissingFileIsAdvisoryNotFatal pins the never-fail contract at the package
// boundary: this warning may not turn a submit into a failure under any input.
func TestMissingFileIsAdvisoryNotFatal(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "src/config.ts", "export const apiSecret = \"Q8vN2mR7kX4zL9pW3tB6\";\n")
	got := scanFindings(dir, []string{"gone.json", "src/config.ts", "also/gone.yaml"})
	if len(got) != 1 {
		t.Fatalf("Scan over a list holding two unreadable paths reported %d finding(s), want 1: %+v", len(got), got)
	}
}

// TestCleanValueStripsWhatTheSpecSaysAndNoMore pins the value normalisation,
// which is where a quoted JSON value and a commented TOML value are reduced to
// the same thing. The URL case is the one that a naive "cut at the first //"
// gets wrong.
func TestCleanValueStripsWhatTheSpecSaysAndNoMore(t *testing.T) {
	for _, tc := range []struct {
		raw        string
		want       string
		wantQuoted bool
	}{
		{`"9f3a2b7c1d4e",`, "9f3a2b7c1d4e", true},
		{`'Q8vN2mR7kX4z';`, "Q8vN2mR7kX4z", true},
		{"`Q8vN2mR7kX4z`", "Q8vN2mR7kX4z", true},
		{`Q8vN2mR7kX4z // the live key`, "Q8vN2mR7kX4z", false},
		{`Q8vN2mR7kX4z # the live key`, "Q8vN2mR7kX4z", false},
		{`https://example.com/a`, "https://example.com/a", false},
		{`opts.token`, "opts.token", false},
	} {
		got, quoted := cleanValue(tc.raw)
		if got != tc.want || quoted != tc.wantQuoted {
			t.Errorf("cleanValue(%q) = (%q, %v), want (%q, %v)", tc.raw, got, quoted, tc.want, tc.wantQuoted)
		}
	}
}

// TestEntropyFloorIsWhereItSays pins the floor's EFFECT ON THE GATE, with
// literal inputs on both sides of it.
//
// 🔴 IT ASSERTS THROUGH valueLooksLikeSecret, NOT AGAINST A HARDCODED 3.0. The
// first version compared `shannonBitsPerChar(v) >= 3.0` — a statement about the
// ENTROPY FUNCTION that never read the constant the gate uses, so lowering
// minEntropyBitsPerChar to 2.0 widened acceptance invisibly, in the noise
// direction, with the suite green. The 2.0–2.9 band fixture is what catches
// that: it must stay SILENT.
func TestEntropyFloorIsWhereItSays(t *testing.T) {
	for _, tc := range []struct {
		why   string
		value string
		want  bool
		band  bool
	}{
		{why: "a repeated character", value: "aaaaaaaaaaaa1", want: false},
		{why: "a two-character alphabet", value: "ababababab1b", want: false},
		{why: "between a lowered floor and this one", value: "aab1aab2aabc3", want: false, band: true},
		{why: "a mixed-case random token", value: "q7W2xE9rT4yU1iO6pA3s", want: true},
		{why: "a hex token", value: "9f3a2b7c1d4e5f60718293a4", want: true},
	} {
		t.Run(tc.why, func(t *testing.T) {
			h := shannonBitsPerChar(tc.value)
			if got := valueLooksLikeSecret(tc.value, true); got != tc.want {
				t.Errorf("valueLooksLikeSecret(%q) = %v (entropy %.2f bits/char), want %v",
					tc.value, got, h, tc.want)
			}
			// The band fixture is only a control while it really sits in the band.
			if tc.band && (h < 2.0 || h >= 3.0) {
				t.Errorf("CONTROL failure: %q has %.2f bits/char, which is not in the 2.0–3.0 band this case "+
					"exists to cover, so it cannot see the floor being lowered to 2.0", tc.value, h)
			}
		})
	}
}

// TestLengthFloorIsPinnedOnBOTHSides is the boundary the first battery missed:
// 11 was pinned as rejected and 12 was never pinned as ACCEPTED, so raising
// minValueLen to 13 shipped green while the detector went blind to 12-character
// secrets. Both fixtures are the same alphabet, so length is the only variable.
func TestLengthFloorIsPinnedOnBOTHSides(t *testing.T) {
	const twelve = "aB3xQ9zK7mWp"
	const eleven = "aB3xQ9zK7mW"
	if len(twelve) != 12 || len(eleven) != 11 {
		t.Fatalf("CONTROL failure: fixtures are %d and %d characters, want 12 and 11", len(twelve), len(eleven))
	}
	if !valueLooksLikeSecret(twelve, true) {
		t.Errorf("a 12-character random value was REJECTED (%q). The floor is 12 and it is inclusive — "+
			"without this assertion, raising it to 13 is invisible.", twelve)
	}
	if valueLooksLikeSecret(eleven, true) {
		t.Errorf("an 11-character value was accepted (%q); the floor is 12", eleven)
	}
}

// ---------------------------------------------------------------------------
// THE LABEL IS NEVER SECRET MATERIAL
// ---------------------------------------------------------------------------

// canonicalWords is the closed set a DEGRADED label may come from — the
// upper-case forms of assignRe's own alternation, written out here rather than
// derived from the code, so this test can disagree with the implementation.
var canonicalWords = map[string]bool{
	"SECRET": true, "TOKEN": true, "PASSWORD": true, "PASSWD": true,
	"APIKEY": true, "API_KEY": true, "PRIVATEKEY": true, "PRIVATE_KEY": true,
	"ACCESSKEY": true, "ACCESS_KEY": true, "CREDENTIAL": true, "CREDENTIALS": true,
}

// keyNameShape is what a printable key may look like, spelled independently of
// the implementation's own regexp.
var keyNameShape = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$.\-]*$`)

// TestLabelNeverCarriesSecretBytes is the regression test for the audited leak.
//
// 🔴 MEASURED ON THE SHIPPED BUILD: a line reading
// `AbCdEfLEAKMARK9SecretKey123: <token>` printed all 27 of those characters as
// its "key", one line above the sentence promising the value was not printed.
// The key group is unanchored, so when a credential word sits INSIDE
// credential-shaped material the label was built out of value bytes.
//
// The marker appears in no other fixture, so a hit can only come from the line.
func TestLabelNeverCarriesSecretBytes(t *testing.T) {
	const marker = "LEAKMARK"
	for _, tc := range []struct {
		why  string
		line string
	}{
		{"a credential word inside a secret-shaped token, YAML", `AbCdEfLEAKMARK9SecretKey123: Zq4TvW9mB2xR7kP1nJ6y`},
		{"a credential word inside a secret-shaped token, shell", `export sk_live_TOKENLEAKMARK42=AbCdEf9Zq4TvW9mB2xR7`},
		{"a base64-ish run carrying a credential word", `Zm9vYmFyLEAKMARKtokenX9y=Q8vN2mR7kX4zL9pW3tB6`},
	} {
		t.Run(tc.why, func(t *testing.T) {
			label, ok := match(tc.line)
			if !ok {
				t.Skipf("line did not match at all, so there is no label to check: %q", tc.line)
			}
			if strings.Contains(label, marker) {
				t.Fatalf("the label %q carries bytes of the line's credential-shaped material — and it prints "+
					"directly above a sentence saying values are not printed.", label)
			}
			if canonicalWords[label] {
				return // degraded to a constant from our own table: safe by construction
			}
			if !keyNameShape.MatchString(label) {
				t.Errorf("label %q is neither a canonical credential word nor an identifier-shaped key name", label)
			}
			if looksLikeCredential(label) {
				t.Errorf("label %q would itself be reported as a credential by this very package — it must "+
					"have degraded to the credential word instead", label)
			}
		})
	}
}

// TestOrdinaryKeysAreStillPrinted is the other half: the safety gate must not
// swallow the useful case. A label that degraded to `SECRET` on every finding
// would pass the leak test above and be nearly useless.
func TestOrdinaryKeysAreStillPrinted(t *testing.T) {
	for _, tc := range []struct{ line, want string }{
		{`  "API_SECRET": "9f3a2b7c1d4e5f60718293a4b5c6d7e8"`, "API_SECRET"},
		{`password: hunter2-Kx9pQ4mZ7vT1`, "password"},
		{`token = "gA7xQ2mV9pL4zR8nB3kW"`, "token"},
		{`export const apiSecret = "Q8vN2mR7kX4zL9pW3tB6";`, "apiSecret"},
		{`VITE_LIVE_BLOCK_TOKEN=7f3Kq9Zx2Lm5Rb8Nv4Tc`, "VITE_LIVE_BLOCK_TOKEN"},
		{`  "auth.password" : "Zx7Z2pQ9mK4vR8nT1yB3"`, "auth.password"},
		{`x-api-token: "9f3a2b7c1d4e5f6071"`, "x-api-token"},
		// 🔴 THE COMMA IS INSIDE THE QUOTES. The unquoted path cuts a value at the
		// first structural delimiter, so only the closing-quote reader can see
		// this value whole — without it the value becomes `"9f3a` and the finding
		// is lost. That mutant SURVIVED until this row existed.
		{`  "API_SECRET": "9f3a,2b7c1d4e5f60718293a4"`, "API_SECRET"},
		// 🔴 NOT identifier-shaped, so it DEGRADES even though it is plainly not a
		// secret. This is the second half of the label gate — the shape test —
		// and without a case for it, dropping that half survives the whole suite:
		// the "does it look like a credential" half alone accepts any punctuation
		// soup that happens to contain no digits.
		{`config["auth"].token = "9f3a2b7c1d4e5f6071"`, "TOKEN"},
		{`obj['auth'].apiSecret = "9f3a2b7c1d4e5f6071"`, "SECRET"},
	} {
		label, ok := match(tc.line)
		if !ok {
			t.Errorf("no match on %q", tc.line)
			continue
		}
		if label != tc.want {
			t.Errorf("label for %q = %q, want %q — the label-safety gate must not degrade an ordinary key name",
				tc.line, label, tc.want)
		}
	}
}

// TestAssignmentLabelWinsOverFormatName pins the order inside match(): when a
// line satisfies BOTH detectors the KEY is the more useful label. Swapping the
// two calls is otherwise invisible.
func TestAssignmentLabelWinsOverFormatName(t *testing.T) {
	label, ok := match(`AWS_SECRET=AKIA3XQ7ZTLMNBVCXZ12`)
	if !ok {
		t.Fatal("no match on a line that satisfies both detectors")
	}
	if label != "AWS_SECRET" {
		t.Errorf("label = %q, want \"AWS_SECRET\" — A2 runs first so the key names the finding; the format "+
			"name would mean the order flipped", label)
	}
}

// ---------------------------------------------------------------------------
// PUBLIC MATERIAL, AND THE HOLES THAT WERE CHOSEN
// ---------------------------------------------------------------------------

// TestPublicMaterialIsNotReported pins two deliberate false negatives. Both
// patterns were removed because they fire on material that is public BY DESIGN,
// so a project using them would warn on EVERY submit — the "trains authors to
// ignore it" failure the whole detector is shaped around.
//
// It is a decision, not an oversight, and it is written down so re-adding either
// pattern turns this red and forces the trade-off to be argued again.
func TestPublicMaterialIsNotReported(t *testing.T) {
	for _, tc := range []struct{ why, line string }{
		{"a Firebase web API key in .env.production, which Google documents as public",
			`VITE_FIREBASE_API_KEY=AIzaSyD9Kq2mZx7Lb4Rv8Nc3Tp6Wy1Ud5Ha0Sj2`},
		{"the same key bare, with no assignment",
			`AIzaSyD9Kq2mZx7Lb4Rv8Nc3Tp6Wy1Ud5Ha0Sj2`},
		{"an X.509 certificate body, which is not a private key",
			`MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC7Vn`},
		{"a PUBLIC key header",
			`-----BEGIN PUBLIC KEY-----`},
	} {
		if label, ok := match(tc.line); ok {
			t.Errorf("fired on %s, labelled %q:\n  %s", tc.why, label, tc.line)
		}
	}
	// CONTROL: a PRIVATE key header — one word away from the public one above —
	// still fires, so the four silences are evidence about public material and
	// not about a detector that has stopped working.
	if _, ok := match("-----BEGIN RSA " + "PRIVATE KEY-----"); !ok {
		t.Error("CONTROL failure: a PEM PRIVATE key header is not reported either, so the silences above " +
			"prove nothing")
	}
}

// TestURLPasswordGateKeepsDocumentationQuiet covers the one format with a
// validator. The connection-string shape is both the most common real leak in a
// web project and the most common example in a README, and only the userinfo
// password tells them apart.
func TestURLPasswordGateKeepsDocumentationQuiet(t *testing.T) {
	for _, tc := range []struct {
		why  string
		line string
		want bool
	}{
		{"a real-looking password", `DATABASE_URL=postgres://admin:Kq9Zx2Lm5Rb8@db.internal:5432/app`, true},
		{"the README spelling", `postgres://user:password@localhost:5432/mydb`, false},
		{"a short doc placeholder", `mysql://root:pass@127.0.0.1/db`, false},
		{"an interpolated password", `postgres://admin:${DB_PASSWORD}@db.internal/app`, false},
		{"a printf placeholder", `postgres://admin:%s@db.internal/app`, false},
		// 🔴 THE INTERPOLATION IS IN THE MIDDLE, so no prefix rule can see it —
		// this is the only fixture the `$`/`%` content test catches, and without
		// it that test is dead code every other clause reaches first.
		{"an interpolation inside the password", `postgres://admin:tok${DB_PASS}en@db.internal/app`, false},
		// 🔴 THE HOST HALF. The match used to END at the `@`, so the rejectRe
		// clause inside the validator — the whole reason `localhost`, `example`,
		// `test` and `demo` are in that word list — could never see the hostname
		// it exists to read. All three of these fired.
		{"a real-looking password against localhost", `postgres://admin:Kq9Zx2Lm5Rb8@localhost:5432/app`, false},
		{"a real-looking password against a documentation domain", `postgres://admin:Kq9Zx2Lm5Rb8@example.com:5432/app`, false},
		{"a real-looking password against loopback", `redis://svc:Kq9Zx2Lm5Rb8@127.0.0.1:6379`, false},
		{"a real-looking password against a test host", `postgres://admin:Kq9Zx2Lm5Rb8@db.test.internal/app`, false},
		// And the gates on the password itself: a default credential, and a
		// repeated character, are not leaks.
		{"a default credential", `amqp://guest:guest@rabbitmq:5672`, false},
		{"a repeated-character password", `mongodb://root:aaaaaaaa@mongo:27017`, false},
		{"an all-zero password", `postgres://u:00000000@db.internal/app`, false},
		{"a password under the length floor", `postgres://admin:aB3x@db.internal/app`, false},
		// 🔴 ONLY THE EQUALITY TEST CAN REJECT THIS ONE. `guest:guest` is rejected
		// by entropy long before it, so with that fixture alone the
		// password-equals-username clause was unreachable and its mutant
		// SURVIVED. `service` clears the entropy floor (2.52 bits/char).
		{"a default credential whose password IS the username", `mongodb://service:service@mongo:27017`, false},
		// CONTROL for that group: the same URL with a real-looking password
		// against a real-looking host still fires, so the eight silences above
		// are facts about the gates and not about a dead pattern.
		{"the control: a real password against a real host", `postgres://admin:Kq9Zx2Lm5Rb8@db.internal:5432/app`, true},
		// 🔴 THE CORPUS REGRESSION. This exact line, in the App Blocks starter
		// template's README, was 12 of the 14 findings in a 244-project run and
		// took the measured firing rate from 0.41% to 5.33% on its own. A `$VAR`
		// password is a REFERENCE, like `process.env.X` — not a secret.
		{"a shell variable as the password, from the starter template's README",
			`git remote add origin "https://civitai-admin:$FORGEJO_TOKEN@forgejo.civitai.com/civitai-apps/starter.git"`, false},
		{"no password at all", `postgres://admin@db.internal:5432/app`, false},
		{"an ordinary URL", `https://civitai.com/api/v1/models?limit=10`, false},
	} {
		t.Run(tc.why, func(t *testing.T) {
			if _, ok := match(tc.line); ok != tc.want {
				t.Errorf("match(%q) = %v, want %v", tc.line, ok, tc.want)
			}
		})
	}
}

// TestPlaceholderTestIsAPrefixNotASubstring is the fixture the corrected comment
// names. A substring test would silence any real token that happens to spell a
// short placeholder word inside itself — and `the`, `my` and `xxx` turn up in
// random strings all the time.
func TestPlaceholderTestIsAPrefixNotASubstring(t *testing.T) {
	const line = `const apiSecret = "Q8vN2mR7kX4zL9pWtheB6";`
	if _, ok := match(line); !ok {
		t.Errorf("a real-looking value containing the placeholder word \"the\" in its MIDDLE was not reported:\n"+
			"  %s\nThe placeholder rule is anchored at the START of the value for exactly this reason.", line)
	}
	// CONTROL: the same word at the START is still a placeholder.
	if _, ok := match(`const apiSecret = "theQ8vN2mR7kX4zL9pWB6";`); ok {
		t.Error("CONTROL failure: a value BEGINNING with a placeholder word was reported, so the case above " +
			"does not discriminate a prefix test from a substring test")
	}
}

// ---------------------------------------------------------------------------
// READING: BUDGET, LONG LINES, RETRY
// ---------------------------------------------------------------------------

// TestScanBudgetStopsAndSaysSo covers the latency bound. Scanning costs
// ~230 ms/MiB and pkgzip legally permits a 200 MiB bundle — ~45 s of silent
// stall between the confirmation prompt and the upload.
//
// 🔴 THE REPORT MUST SAY IT STOPPED. A truncated scan returning an empty finding
// list is indistinguishable from a clean bundle, which is the one conclusion
// this feature must never invite.
func TestScanBudgetStopsAndSaysSo(t *testing.T) {
	dir := t.TempDir()
	filler := strings.Repeat("const x = 1; // ordinary source line\n", 40_000) // ~1.4 MiB each
	var files []string
	for i := 0; i < 14; i++ {
		name := fmt.Sprintf("src/filler%02d.ts", i)
		mustWrite(t, dir, name, filler)
		files = append(files, name)
	}
	mustWrite(t, dir, "zz-last.json", "{\n  \"API_SECRET\": \"9f3a2b7c1d4e5f60718293a4b5c6d7e8\"\n}\n")
	files = append(files, "zz-last.json")

	rep := Scan(dir, files)
	if !rep.Truncated {
		t.Fatalf("a ~20 MiB bundle against a %d MiB budget did not report truncation (scanned %d of %d files)",
			MaxScanBytes>>20, rep.FilesScanned, rep.FilesTotal)
	}
	if rep.FilesScanned >= rep.FilesTotal {
		t.Errorf("FilesScanned=%d of FilesTotal=%d — a truncated scan must have read fewer files than it was given",
			rep.FilesScanned, rep.FilesTotal)
	}
	if len(rep.Findings) != 0 {
		t.Errorf("the filler holds no credential, so the findings must be empty: %+v", rep.Findings)
	}

	// CONTROL: the file the budget cut off DOES hold a credential, so the empty
	// result above is a fact about the budget and not about the fixture.
	if n := len(scanFindings(dir, []string{"zz-last.json"})); n != 1 {
		t.Errorf("control: scanning the cut-off file alone reported %d finding(s), want 1", n)
	}
	// And an ordinary bundle is never truncated.
	if small := Scan(dir, []string{"zz-last.json"}); small.Truncated {
		t.Error("a one-file bundle reported truncation")
	}
}

// TestLongLineSkipsTheLINEAndNotTheREST is the audited blindness: bufio.Scanner
// returns ErrTooLong and never resumes, so one long line — a `vendor.js`, a
// bundled asset, a minified blob at the top of a file — silently blinded every
// line after it.
func TestLongLineSkipsTheLINEAndNotTheREST(t *testing.T) {
	dir := t.TempDir()
	long := strings.Repeat("a", (1<<20)+16) // one line over the 1 MiB cap
	mustWrite(t, dir, "vendor.js", long+"\nconst apiSecret = \"Q8vN2mR7kX4zL9pW3tB6\";\n")
	// The identical credential line with no long line in front of it.
	mustWrite(t, dir, "control.js", "const apiSecret = \"Q8vN2mR7kX4zL9pW3tB6\";\n")

	got := scanFindings(dir, []string{"control.js", "vendor.js"})
	if len(got) != 2 {
		t.Fatalf("reported %d finding(s), want 2 — the credential AFTER a too-long line must still be seen, "+
			"and control.js proves the line itself is detectable: %+v", len(got), got)
	}
	for _, f := range got {
		if f.Path == "vendor.js" && f.Line != 2 {
			t.Errorf("the vendor.js finding is at line %d, want 2 — a skipped long line must still be COUNTED, "+
				"or every line number after it is wrong", f.Line)
		}
	}
}

// TestSecondCredentialWordOnALineGetsATurn covers the retry. The value capture
// runs to end of line, so one line yields one regexp match however many
// credential words it holds — and on minified single-line JSON the first word
// was the only one ever put in front of the gate.
func TestSecondCredentialWordOnALineGetsATurn(t *testing.T) {
	const line = `{"token":"","auth":{"API_SECRET":"9f3a2b7c1d4e5f60718293a4"}}`
	label, ok := match(line)
	if !ok {
		t.Fatalf("no match on a minified line whose SECOND credential word carries the secret:\n  %s", line)
	}
	if label != "API_SECRET" {
		t.Errorf("label = %q, want \"API_SECRET\" — the first word on the line (`token`, empty value) is "+
			"rejected by the gate, and the retry is what gives the second one its turn", label)
	}
}

// TestBinarySniffWindowIsWhereItSays pins the 8 KiB window on both sides: a NUL
// inside it classifies the file binary, and one past it does not. Without the
// second half, widening the window to the whole file — which would silence any
// text file with a stray NUL anywhere in it — is invisible.
func TestBinarySniffWindowIsWhereItSays(t *testing.T) {
	dir := t.TempDir()
	const credential = "apiSecret = \"Q8vN2mR7kX4zL9pW3tB6\"\n"
	mustWrite(t, dir, "inside.dat", strings.Repeat("x", 4000)+"\x00"+strings.Repeat("y", 100)+"\n"+credential)
	mustWrite(t, dir, "outside.dat", strings.Repeat("x", 9000)+"\n"+credential+strings.Repeat("z", 10)+"\x00")

	if got := scanFindings(dir, []string{"inside.dat"}); len(got) != 0 {
		t.Errorf("a NUL at byte 4000 (inside the 8 KiB window) did not classify the file binary: %+v", got)
	}
	if got := scanFindings(dir, []string{"outside.dat"}); len(got) != 1 {
		t.Errorf("a NUL at ~byte 9040 (outside the window) reported %d finding(s), want 1 — the window must "+
			"stay bounded, or one stray NUL silences a whole text file", len(got))
	}
}

// TestQuotedExemptionFromTheDottedRule pins the `!quoted &&` clause, which no
// earlier fixture could reach — the entropy floor rejected them first. An
// UNQUOTED dotted value is a reference expression (`opts.auth2.token`); a QUOTED
// one is data. Both calls take the same characters, so quoting is the only
// variable.
func TestQuotedExemptionFromTheDottedRule(t *testing.T) {
	const dotted = "a1.b2.c3.d4.e5.f6"
	if !valueLooksLikeSecret(dotted, true) {
		t.Errorf("a QUOTED dotted value (%q) was rejected — in a config file that is data, and the "+
			"dotted-reference rule is exempted for quoted values precisely for it", dotted)
	}
	if valueLooksLikeSecret(dotted, false) {
		t.Errorf("an UNQUOTED dotted value (%q) was accepted — that is a reference expression", dotted)
	}
}

// ---------------------------------------------------------------------------
// BOUNDED TIME — the class no correctness assertion can see
// ---------------------------------------------------------------------------

// minifiedLine builds one line of the shape a bundler emits: many short quoted
// credential-word keys with empty values, so every one of them reaches the gate
// and is rejected. That is the shape that makes the per-line retry loop run to
// exhaustion, and it is the COMMON case — a clean file, nothing to report.
func minifiedLine(n int, tail string) string {
	var b strings.Builder
	b.WriteString(`var x=[`)
	for b.Len() < n {
		b.WriteString(`{"token":"","refreshToken":"","apiSecret":""},`)
	}
	b.WriteString(tail)
	b.WriteString(`];`)
	return b.String()
}

// runWithin runs fn and returns how long it took, failing the test if it has not
// finished within budget.
//
// 🔴 IT FAILS AT THE DEADLINE RATHER THAN WAITING. A quadratic regression takes
// MINUTES on the larger fixture, and a test that simply waits would blow the
// package timeout and truncate the whole suite's output — which reads as an
// infrastructure problem rather than as this defect. The goroutine is left
// running on purpose; the process is about to exit.
func runWithin(t *testing.T, what string, budget time.Duration, fn func()) time.Duration {
	t.Helper()
	done := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		fn()
		done <- time.Since(start)
	}()
	select {
	case d := <-done:
		return d
	case <-time.After(budget):
		t.Fatalf("%s did not finish within %v.\n\n"+
			"🔴 This is the quadratic-scan regression. The per-line retry loop re-runs the assignment "+
			"regexp from each rejected match, and if the value group captures to END OF LINE then every "+
			"attempt costs the length of the line — O(n²) on exactly the input where nothing matches, "+
			"i.e. an ordinary minified bundle in a clean project. Measured before the value capture was "+
			"bounded: 8 KB 0.40 s, 16 KB 1.64 s, 32 KB 6.47 s, 64 KB 26.09 s (4.0× per doubling), which "+
			"pkgzip's own 1 MiB line cap extrapolates to over an hour for ONE line.", what, budget)
		return 0
	}
}

// TestMinifiedLineScansInBoundedTime is the regression test for that class.
//
// 🔴 IT IS THE ONLY TEST THAT CAN SEE THIS DEFECT. Every correctness assertion in
// this package passes with a quadratic scanner — the ANSWER is right, it just
// takes 26 seconds a line — and CI was green on it. Watched failing against the
// implementation it was written for: the 64 KB case exceeded its 2 s budget.
//
// The budgets are ~100× the measured post-fix cost (single-digit ms), so this is
// a class detector rather than a benchmark: it cannot go red on ordinary CI
// noise, and it cannot stay green under an O(n²) scan.
func TestMinifiedLineScansInBoundedTime(t *testing.T) {
	for _, tc := range []struct {
		size   int
		budget time.Duration
	}{
		{64 << 10, 2 * time.Second},
		{256 << 10, 5 * time.Second},
	} {
		t.Run(fmt.Sprintf("%dKB", tc.size>>10), func(t *testing.T) {
			line := minifiedLine(tc.size, "")
			d := runWithin(t, fmt.Sprintf("scanning a %d KB minified line", tc.size>>10), tc.budget, func() {
				if _, ok := match(line); ok {
					t.Errorf("the filler line holds only EMPTY values and must not match")
				}
			})
			t.Logf("%d KB minified line scanned in %v (budget %v)", tc.size>>10, d.Round(time.Millisecond), tc.budget)
		})
	}
}

// TestMinifiedLineStillFindsACredentialAtTheEnd is the correctness half, and it
// is what stops the bounded-time test being satisfied by a scanner that simply
// GIVES UP on long lines. The secret sits in the last object, so it is found
// only by a scan that traversed the whole line.
func TestMinifiedLineStillFindsACredentialAtTheEnd(t *testing.T) {
	line := minifiedLine(64<<10, `{"apiSecret":"9f3a2b7c1d4e5f60718293a4"}`)
	label, ok := match(line)
	if !ok {
		t.Fatalf("a credential in the LAST object of a 64 KB minified line was not found — a bounded-time "+
			"scan must still be a complete one (line length %d)", len(line))
	}
	if label != "apiSecret" {
		t.Errorf("label = %q, want \"apiSecret\"", label)
	}
}

// TestWholeFileOfMinifiedLinesIsBounded is the same guard one level up: the
// per-line cost is what the file loop multiplies, and pkgzip packages
// `src/*.min.js` (only `dist/` and `node_modules/` are excluded), so this is a
// real bundle rather than a synthetic worst case.
func TestWholeFileOfMinifiedLinesIsBounded(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	for i := 0; i < 8; i++ {
		b.WriteString(minifiedLine(32<<10, ""))
		b.WriteByte('\n')
	}
	mustWrite(t, dir, "src/app.min.js", b.String())

	var got []Finding
	d := runWithin(t, "scanning a 256 KB minified file", 5*time.Second, func() {
		got = scanFindings(dir, []string{"src/app.min.js"})
	})
	if len(got) != 0 {
		t.Errorf("the fixture holds only empty values: %+v", got)
	}
	t.Logf("256 KB minified file scanned in %v", d.Round(time.Millisecond))
}

// TestTruncationInsideTheLastFileIsReported is the reassuring zero, reached by
// the guard written to prevent it.
//
// 🔴 MEASURED ON THE PREVIOUS BUILD: with the budget dying inside the FINAL
// entry, the `budget <= 0` test at the top of the next iteration never ran, so
// the report came back Truncated=false, FilesScanned==FilesTotal, no findings —
// and renderCredentialWarning printed NOTHING AT ALL for a bundle whose last
// file holds a credential. The control below proves that file is detectable.
func TestTruncationInsideTheLastFileIsReported(t *testing.T) {
	dir := t.TempDir()
	// 🔴 THE FIXTURE IS SIZED FROM THE BUDGET, NOT FROM A LITERAL. A fixed size
	// stops demonstrating the moment the budget moves: measured against the
	// 16 MiB budget of the commit this fixes, an 11 MiB fixture never truncated
	// and the test failed on its own CONTROL instead of on the defect. Only the
	// SIZE is derived; every assertion below is literal.
	const line = "const x = 1; // ordinary source line\n"
	perFile := int(MaxScanBytes) / 8
	filler := strings.Repeat(line, perFile/len(line))
	var files []string
	for i := 0; i < 6; i++ {
		name := fmt.Sprintf("src/filler%02d.ts", i)
		mustWrite(t, dir, name, filler)
		files = append(files, name)
	}
	// One last file, larger than the remaining budget, with the credential past
	// where the budget dies.
	big := strings.Repeat("const y = 2; // padding\n", int(MaxScanBytes)/2/24)
	mustWrite(t, dir, "src/last.ts", big+"const apiSecret = \"Q8vN2mR7kX4zL9pW3tB6\";\n")
	files = append(files, "src/last.ts")

	rep := Scan(dir, files)
	if len(rep.Findings) != 0 {
		t.Fatalf("CONTROL failure: the credential was reached, so this fixture cannot show the defect: %+v",
			rep.Findings)
	}
	if !rep.Truncated {
		t.Errorf("the budget ran out INSIDE the last file and the report says Truncated=false "+
			"(FilesScanned=%d, FilesTotal=%d, findings=0). renderCredentialWarning then prints nothing at "+
			"all — a scan that stopped early, reporting exactly what a clean bundle reports.",
			rep.FilesScanned, rep.FilesTotal)
	}
	if rep.FilesScanned >= rep.FilesTotal {
		t.Errorf("FilesScanned=%d of FilesTotal=%d — a file cut short by the budget must not count as scanned, "+
			"or `stopped after N of M` reads as `N of N`", rep.FilesScanned, rep.FilesTotal)
	}
	// CONTROL: that last file DOES hold a credential when it is scanned whole.
	if n := len(scanFindings(dir, []string{"src/last.ts"})); n != 1 {
		t.Errorf("control: scanning the last file alone reported %d finding(s), want 1", n)
	}
}

// TestManyTinyBinariesDoNotEatTheBudget pins the sniff CHARGE. Charging every
// binary a flat 8 KiB meant 2,000 three-byte files could spend 97.6% of the
// budget having read 6 KB — while the comment beside it said assets do not eat
// the budget.
func TestManyTinyBinariesDoNotEatTheBudget(t *testing.T) {
	dir := t.TempDir()
	// Enough three-byte binaries to blow the WHOLE budget if each is charged a
	// flat sniff window — derived from the budget so it keeps demonstrating when
	// that constant moves (a fixed 1500 stopped demonstrating against the 16 MiB
	// budget this replaced). The assertions stay literal.
	n := int(MaxScanBytes/binarySniffBytes) + 200
	var files []string
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("assets/i%05d.ico", i)
		mustWrite(t, dir, name, "\x00\x01\x02")
		files = append(files, name)
	}
	mustWrite(t, dir, "zz.json", "{\n  \"API_SECRET\": \"9f3a2b7c1d4e5f60718293a4b5c6d7e8\"\n}\n")
	files = append(files, "zz.json")

	rep := Scan(dir, files)
	if rep.Truncated {
		t.Errorf("%d three-byte binaries (%d KB in total) truncated a %d MiB budget — the sniff is being "+
			"charged as a flat window rather than as what it read", n, n*3/1024, MaxScanBytes>>20)
	}
	if len(rep.Findings) != 1 {
		t.Errorf("the credential after the binaries was not reported (%d finding(s)) — the budget ate it",
			len(rep.Findings))
	}
}

// TestDegradedLabelIsAlwaysOneOfOurConstants pins NF-5. Go's `(?i)` uses Unicode
// simple folding, so a KELVIN SIGN (U+212A) matches `K` in `TOKEN` — and
// strings.ToUpper leaves U+212A intact, which put three bytes of the LINE into a
// label documented as a closed set of constants.
func TestDegradedLabelIsAlwaysOneOfOurConstants(t *testing.T) {
	// `to<KELVIN>en` inside credential-shaped material, so the label degrades.
	line := "AbCdEf9toKen42=Q8vN2mR7kX4zL9pW3tB6"
	label, ok := match(line)
	if !ok {
		t.Fatalf("CONTROL failure: the fold case did not match at all, so there is no label to check: %q", line)
	}
	if !canonicalWords[label] {
		t.Errorf("degraded label = %q, which is not one of the constants this package documents. It carries "+
			"bytes from the line (%q), because Unicode case folding matched a character ToUpper does not "+
			"normalise.", label, line)
	}
	for _, r := range label {
		if r > 127 {
			t.Errorf("label %q holds a non-ASCII rune (%U) — it cannot be one of our own constants", label, r)
		}
	}
}

// TestURLValidatorHandlesAMalformedMatch pins NF-4: the bounds test runs before
// the slice. Unreachable through the regexp today, which is exactly why it was
// wrong — it read as protection it did not provide, and a panic here would break
// this package's never-fail contract from the inside.
func TestURLValidatorHandlesAMalformedMatch(t *testing.T) {
	for _, s := range []string{"", "no-at-sign", "postgres://user", ":", "@", "a:b"} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("urlPasswordLooksReal(%q) panicked: %v", s, r)
				}
			}()
			if urlPasswordLooksReal(s) {
				t.Errorf("urlPasswordLooksReal(%q) = true, want false", s)
			}
		}()
	}
}

// TestValueCaptureIsANonSpaceRun is where the whitespace invariant lives now.
//
// 🔴 IT REPLACED A DEAD GUARD. valueLooksLikeSecret used to test the value for
// spaces, and a mutation battery scored that clause SURVIVED — assignRe's value
// group is `\S{1,512}`, so a value carrying a space cannot reach the gate at
// all. Dead code in a gate reads as protection it is not providing, so the
// clause went and the invariant is asserted here, on the thing that actually
// enforces it. The prose row in TestOrdinaryCodeIsSilent is the behavioural
// half: widen the capture and it fires.
func TestValueCaptureIsANonSpaceRun(t *testing.T) {
	for _, line := range []string{
		`password: this is not a real value 12345`,
		`API_SECRET = 9f3a2b7c1d4e 5f60718293a4`,
		`token: "hunter two 9f3a2b7c1d4e"`,
		`  "API_SECRET": "9f3a2b7c 1d4e5f60718293a4",`,
	} {
		m := assignRe.FindStringSubmatch(line)
		if m == nil {
			t.Errorf("CONTROL failure: %q did not match at all, so it says nothing about the capture", line)
			continue
		}
		if strings.ContainsAny(m[4], " \t") {
			t.Errorf("the value capture for %q is %q, which carries whitespace. The gate no longer tests for "+
				"that — this pattern is the only thing enforcing it.", line, m[4])
		}
		if len(m[4]) > 512 {
			t.Errorf("the value capture for %q is %d bytes; the bound is 512, and that bound is what keeps "+
				"the per-line scan linear", line, len(m[4]))
		}
	}
}

// ---------------------------------------------------------------------------
// THE LONG-LINE CAP — pinned by BEHAVIOUR, not by restating the constant
// ---------------------------------------------------------------------------

// lineWithACredentialAtItsEnd returns a SINGLE line of exactly n bytes whose
// tail is an assignment the detector must report. The padding is a run of one
// character followed by a space; a space cannot appear in the key pattern or in
// the value capture, so the padding can neither match on its own nor be swept
// into the key or the value.
func lineWithACredentialAtItsEnd(t *testing.T, n int) string {
	t.Helper()
	const tail = ` apiSecret="Q8vN2mR7kX4zL9pW3tB6";`
	if n <= len(tail) {
		t.Fatalf("n=%d cannot hold the %d-byte credential tail", n, len(tail))
	}
	return strings.Repeat("x", n-len(tail)) + tail
}

// TestLongLineCapIsPinnedOnBothSides covers maxLineBytes, which was
// FUNCTIONALLY UNPINNED: a mutation battery narrowed it from 1 MiB to 1 KiB — a
// 1024× change — and the whole suite stayed green. It could, because every
// existing long-line fixture either derives its size from the constant (so it
// moves with it) or calls match() directly, which never goes through readLine at
// all. At 1 KiB the scanner would skip any longer line, i.e. go blind across
// exactly the minified/bundled content the quadratic fix was written for.
//
// 🔴 WHICH SIDE OF THE STALE-VS-VACUOUS TRADE THIS PICKS, AND WHY. A test that
// derives BOTH fixtures from maxLineBytes passes at ANY value, 1 KiB included —
// that is precisely the trap that let the mutant live, and it is the same shape
// as the entropy test that hardcoded 3.0 instead of reading the gate's constant
// and let a 3.0→2.0 change ship green. Restating `1 << 20` here has the mirror
// failure: it goes red on a deliberate retune that changes nothing anyone cares
// about, and a permanently-annoying assertion gets edited to match rather than
// argued with. So this splits them deliberately:
//
//   - the UNDER/OVER boundary pair is DERIVED from the constant, so it pins the
//     skip behaviour wherever the boundary sits and a ±1 retune stays green;
//   - one LITERAL 256 KiB case pins the STAKE rather than the number. A real
//     minified vendor bundle is a single line far larger than that, so a cap
//     that cannot see 256 KiB has silently dropped this feature's headline input
//     out of scope. 1 KiB fails it; a retune to 512 KiB does not.
func TestLongLineCapIsPinnedOnBothSides(t *testing.T) {
	// 🔴 THE LITERAL HALF. Nothing here reads maxLineBytes.
	t.Run("a 256 KiB line is scanned", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, dir, "src/vendor.min.js", lineWithACredentialAtItsEnd(t, 256<<10)+"\n")

		got := scanFindings(dir, []string{"src/vendor.min.js"})
		if len(got) != 1 {
			t.Fatalf("a credential inside a 256 KiB line reported %d finding(s), want 1. A minified bundle is "+
				"ONE line of this order, and it is the input the whole bounded-time fix exists for — a line cap "+
				"that cannot reach 256 KiB has taken this feature's headline content out of scope silently, "+
				"because a skipped line reports exactly what a clean line reports: %+v", len(got), got)
		}
		if got[0].Label != "apiSecret" {
			t.Errorf("label = %q, want \"apiSecret\"", got[0].Label)
		}
	})

	// The envelope, in literals, for the one direction no behavioural case can
	// reach: readLine buffers a whole line before deciding anything, so
	// maxLineBytes is also this package's per-line MEMORY bound, and a submit may
	// not be turned into an OOM by a bundled asset.
	//
	// 🔴 IT IS A FATAL GATE, AND IT RUNS BEFORE THE DERIVED CASES BELOW ON
	// PURPOSE. Those cases build a fixture of about maxLineBytes bytes, so an
	// absurd cap makes the TEST the expensive thing: measured at 1 GiB, the
	// package timed out and died by `panic: test timed out` — a hang that reads
	// as CI infrastructure trouble, with this assertion's message never printed.
	// A guard that is right but unreachable behind a neighbour's failure is the
	// died-for-the-wrong-reason shape, so the cheap literal check goes first and
	// stops the run with its own words.
	const floor = 256 << 10 // the 256 KiB case above is the behavioural half
	const ceiling = 16 << 20
	if maxLineBytes < floor {
		t.Fatalf("maxLineBytes = %d, below the %d-byte floor: a minified bundle line is routinely larger "+
			"than that, so the detector would be blind to it", maxLineBytes, floor)
	}
	if maxLineBytes > ceiling {
		t.Fatalf("maxLineBytes = %d, above the %d-byte ceiling. readLine buffers the whole line before it "+
			"decides anything, so this constant is a per-line MEMORY bound as well as a detection one, and "+
			"this package may never turn a submit into an OOM.", maxLineBytes, ceiling)
	}

	// 🔴 THE DERIVED HALF — the boundary, wherever the constant puts it. ±64
	// bytes, not ±1: a hairline retune of the cap is not a defect worth a red,
	// and pinning the exact byte would additionally pin whether the line's
	// terminator counts against the cap, which is not a contract this package
	// states.
	t.Run("just under the cap is scanned", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, dir, "under.js", lineWithACredentialAtItsEnd(t, maxLineBytes-64)+"\n")

		got := scanFindings(dir, []string{"under.js"})
		if len(got) != 1 {
			t.Fatalf("a credential in a line 64 bytes UNDER the cap reported %d finding(s), want 1 — the cap "+
				"must skip only what is over it: %+v", len(got), got)
		}
		if got[0].Line != 1 {
			t.Errorf("reported line %d, want 1", got[0].Line)
		}
	})

	t.Run("just over the cap is skipped and the rest of the file is not", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, dir, "over.js",
			lineWithACredentialAtItsEnd(t, maxLineBytes+64)+"\n"+
				`const apiToken = "Zq4TvW9mB2xR7kP1nJ6y";`+"\n")

		got := scanFindings(dir, []string{"over.js"})
		if len(got) != 1 {
			t.Fatalf("reported %d finding(s), want exactly 1 — the credential in the OVER-cap line 1 must be "+
				"skipped and the ordinary line 2 must still be read: %+v", len(got), got)
		}
		if got[0].Line != 2 {
			t.Errorf("reported line %d, want 2. Line 1 is 64 bytes over the cap and must be skipped; reporting "+
				"it means the cap is not being applied, and reporting nothing at all means a skipped line still "+
				"ends the file.", got[0].Line)
		}
		if got[0].Label != "apiToken" {
			t.Errorf("label = %q, want \"apiToken\" — that is the line-2 key", got[0].Label)
		}
	})
}

// ---------------------------------------------------------------------------
// THE URL VALIDATOR'S TWO UNPINNED CLAUSES
// ---------------------------------------------------------------------------

// TestURLPlaceholderAndDocWordClausesAreEachSeparatelyReachable pins the two
// clauses of urlPasswordLooksReal that a mutation battery scored SURVIVED:
// deleting the placeholder-prefix loop, and deleting the documentation-word
// switch, each left the whole suite green.
//
// 🔴 THE FIXTURES ARE PICKED SO THAT EXACTLY ONE CLAUSE REJECTS EACH, AND THAT
// IS THE ENTIRE DIFFICULTY. Every obvious documentation URL is rejected three or
// four ways over — `user:password@localhost` is caught by rejectRe, by the
// loopback rule AND by the doc-word switch — so a fixture like that leaves the
// OTHER clauses to reject it and BOTH mutants survive a fully green run. Each
// line below was measured against every clause before it was written down:
//
//   - `your…`/`todo…` — prefixes that are in placeholderPrefixes but NOT in
//     rejectRe's word list, so only the prefix loop can see them. The overlapping
//     entries (`dummy`, `fake`, `sample`, `example`, `placeholder`, `changeme`,
//     `redacted`) are useless here because rejectRe rejects them first, and `$`
//     is taken by the `$`/`%` content test.
//   - `password` (2.75 bits/char), `hunter2` (2.81), `changeit` (3.00) — doc
//     words that CLEAR the 2.5 bits/char floor. The switch's other three arms
//     cannot be used: `secret` and `passwd` sit at 2.25 and `pwd` at 1.58, so
//     the entropy floor gets there first and a fixture built on one of them
//     proves nothing about the switch.
//
// Each row is paired with a control that differs ONLY in the clause's trigger,
// so a silence here is a fact about that clause and not about a dead pattern.
func TestURLPlaceholderAndDocWordClausesAreEachSeparatelyReachable(t *testing.T) {
	for _, tc := range []struct {
		clause  string
		why     string
		quiet   string // must NOT be reported — only `clause` rejects it
		control string // must BE reported — identical but for the clause's trigger
	}{
		{
			clause:  "the placeholder-prefix loop",
			why:     "a `your…` password against a real host",
			quiet:   `postgres://civitai:yourDbPassw0rd@db.civitai-internal.net:5432/app`,
			control: `postgres://civitai:DbPassw0rd7q@db.civitai-internal.net:5432/app`,
		},
		{
			clause:  "the placeholder-prefix loop",
			why:     "a `todo…` password against a real host",
			quiet:   `mongodb://svc:todoKq9Zx2Lm5Rb8@cluster0.civitai-internal.net/db`,
			control: `mongodb://svc:Kq9Zx2Lm5Rb8@cluster0.civitai-internal.net/db`,
		},
		{
			clause:  "the documentation-word switch",
			why:     "the README `admin:password@` spelling, against a REAL host",
			quiet:   `postgres://admin:password@db.internal:5432/app`,
			control: `postgres://admin:passwordX9@db.internal:5432/app`,
		},
		{
			clause:  "the documentation-word switch",
			why:     "`changeit`, the Java keystore default",
			quiet:   `postgres://admin:changeit@db.internal:5432/app`,
			control: `postgres://admin:changeitX9@db.internal:5432/app`,
		},
		{
			clause:  "the documentation-word switch",
			why:     "`hunter2`, the joke password every README borrows",
			quiet:   `amqp://svc:hunter2@rabbitmq.civitai-internal.net:5672`,
			control: `amqp://svc:hunter2X9@rabbitmq.civitai-internal.net:5672`,
		},
	} {
		t.Run(tc.why, func(t *testing.T) {
			if label, ok := match(tc.quiet); ok {
				t.Errorf("fired on %s, labelled %q:\n  %s\n\n"+
					"🔴 %s is the ONLY clause of urlPasswordLooksReal that rejects this line — rejectRe, the "+
					"loopback rule, the password-equals-username test, the `$`/`%%` content test and the "+
					"entropy floor were all measured to PASS it. So this firing means that clause is gone, "+
					"and ordinary documentation now warns on every submit.",
					tc.why, label, tc.quiet, tc.clause)
			}
			// CONTROL: the same URL differing only in the clause's trigger IS
			// reported, so the silence above is a fact about the clause rather
			// than about a pattern that has stopped matching altogether.
			if _, ok := match(tc.control); !ok {
				t.Errorf("CONTROL failure: %q was NOT reported. It differs from the quiet line only in the "+
					"trigger for %s, so without it that silence proves nothing.", tc.control, tc.clause)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// B49 — the advance-by-one termination guard in matchAssignment
// ---------------------------------------------------------------------------

// TestAssignRegexpAlwaysAdvancesTheScanPosition is the property that makes
// matchAssignment's `if next <= pos { next = pos + 1 }` guard UNREACHABLE, which
// is why deleting that guard survives every behavioural test in this package.
//
// 🔴 THE GUARD IS NOT VACUOUS, IT IS UNREACHABLE-BY-CONSTRUCTION, AND THOSE NEED
// DIFFERENT TREATMENT. A dropped termination guard is an infinite-loop hazard
// inside a code path that runs on every packaged line, so the honest thing is to
// pin the PROPERTY the unreachability rests on rather than to write a behavioural
// test that cannot exist. matchAssignment resumes at `pos + loc[7]`, the END of
// assignRe's operator submatch; the operator group `(:=|=>|=|:)` is mandatory and
// at least one byte, and the credential-word group in front of it is mandatory
// and at least five, so loc[7] >= 6 > 0 for every match the pattern can produce
// and `next` can never be <= `pos`.
//
// Widen the pattern and that stops being true, in either of two measured ways —
// at which point the guard becomes load-bearing and this test is what goes red
// first. Both were run against this test and both were caught, each by its own
// branch below: making the operator group OPTIONAL (`(…)?`) lets it fail to
// participate, so loc[6]/loc[7] come back -1; giving BOTH that group and the
// credential-word group an EMPTY ALTERNATIVE lets assignRe match at zero width,
// so loc[7] reaches 0 and matchAssignment advances by nothing at all.
func TestAssignRegexpAlwaysAdvancesTheScanPosition(t *testing.T) {
	// A structured sweep rather than a hand-written list: four tokens drawn from
	// an alphabet built out of the pattern's own moving parts, exhaustively.
	// Deterministic, so a failure reproduces exactly.
	tokens := []string{
		"", " ", "SECRET", "token", "password", "API_KEY",
		":", "=", ":=", "=>", `"`, `'`, "9f3a2b7c1d4e", "[",
	}

	var lines, matches, minEnd int
	minEnd = -1
	check := func(t *testing.T, line string) {
		lines++
		for pos := 0; pos < len(line); {
			loc := assignRe.FindStringSubmatchIndex(line[pos:])
			if loc == nil {
				return
			}
			matches++
			if loc[6] < 0 || loc[7] < 0 {
				t.Fatalf("assignRe matched %q at pos %d without its OPERATOR submatch (indices %d,%d). The "+
					"operator group is what matchAssignment advances past, so a match that omits it makes "+
					"`pos + loc[7]` meaningless and the termination guard load-bearing.",
					line, pos, loc[6], loc[7])
			}
			if loc[7] < 1 {
				t.Fatalf("assignRe matched %q at pos %d with an operator ending at offset %d. matchAssignment "+
					"resumes at `pos + loc[7]`, so this match advances the scan by %d bytes — the loop does "+
					"not terminate, and the `if next <= pos` guard is now the only thing standing between "+
					"`civitai app submit` and an infinite loop on a packaged line.",
					line, pos, loc[7], loc[7])
			}
			if minEnd < 0 || loc[7] < minEnd {
				minEnd = loc[7]
			}
			pos += loc[7]
		}
	}

	for _, a := range tokens {
		for _, b := range tokens {
			for _, c := range tokens {
				for _, d := range tokens {
					check(t, a+b+c+d)
				}
			}
		}
	}
	// The adversarial shapes a token sweep will not assemble on its own.
	for _, line := range []string{
		"",
		":",
		"SECRET:",
		`SECRET:""`,
		"token=:=:=:=",
		"secretsecretsecretsecret::::",
		strings.Repeat("token:", 64),
		strings.Repeat("[", 41) + "SECRET" + strings.Repeat("]", 41) + `:"9f3a2b7c1d4e"`,
		`{"token":"","API_SECRET":"9f3a2b7c1d4e5f60718293a4"}`,
	} {
		check(t, line)
	}

	// 🔴 POSITIVE CONTROL. A sweep that matched NOTHING would report the same
	// clean zero as a sweep that proved the property, so the count has to move.
	if matches < 100 {
		t.Fatalf("CONTROL failure: %d line(s) produced only %d assignRe match(es). This sweep proves the "+
			"advance property only over matches it actually found, so a near-zero here means the alphabet "+
			"stopped assembling assignments and the property is untested.", lines, matches)
	}
	t.Logf("%d lines, %d assignRe matches, smallest operator-end offset %d (the guard fires only at 0)",
		lines, matches, minEnd)
}
