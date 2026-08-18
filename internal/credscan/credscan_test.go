package credscan

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
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
		{"URL with a password, no credential word in the key", `const dsn = "mongodb://svc:Nv4Tc7f3Kq9Z@cluster0.example.net/db";`, "URL with an embedded password"},
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
