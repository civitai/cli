package credscan

import (
	"os"
	"path/filepath"
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
	got := Scan(dir, files)
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

	got := Scan(dir, []string{".env.production"})
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

// TestKnownFormatsAreAllCaught covers detector B end to end: ten credential
// shapes that identify themselves without any assignment. Values are synthetic
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
		{"Google", "AIzaSyD9Kq2mZx7Lb4Rv8Nc3Tp6Wy1Ud5Ha0Sj2", "Google API key"},
		{"npm", "npm_9Kq2mZx7Lb4Rv8Nc3Tp6Wy1Ud5Ha0Sj2GfQ7", "npm token"},
		{"PKCS8", "MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC7Vn", "PKCS#8 key body"},
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
		"PEM private key block": true,
		"JWT":                   true,
		"AWS access key id":     true,
		"GitHub token":          true,
		"Slack token":           true,
		"OpenAI API key":        true,
		"Stripe API key":        true,
		"Google API key":        true,
		"npm token":             true,
		"PKCS#8 key body":       true,
	}
	if len(knownFormats) != 10 {
		t.Errorf("knownFormats holds %d format(s); the corpus in TestKnownFormatsAreAllCaught proves 10. "+
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

	got := Scan(dir, []string{"src/config.ts"})
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
	got := Scan(dir, files)
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

	got := Scan(dir, []string{"src/App.tsx"})
	if len(got) != 0 {
		t.Fatalf("Scan looked outside the file list it was given: %+v", got)
	}

	// POSITIVE CONTROL: the two files it must not read DO hold values it would
	// otherwise report, so the zero above is a fact about the scope and not about
	// the fixture.
	for _, rel := range []string{".env.local", "node_modules/leak.js"} {
		if n := len(Scan(dir, []string{rel})); n != 1 {
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

	got := Scan(dir, []string{"keys.pem", "src/config.ts"})
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

	if got := Scan(dir, []string{"assets/logo.ico"}); len(got) != 0 {
		t.Errorf("scanned a binary file: %+v", got)
	}
	if got := Scan(dir, []string{"assets/logo.txt"}); len(got) != 1 {
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

	got := Scan(dir, []string{"src/keys.ts"})
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
	got := Scan(dir, []string{"gone.json", "src/config.ts", "also/gone.yaml"})
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

// TestEntropyFloorIsWhereItSays pins the constant's EFFECT with literal inputs
// on both sides of it, so a change to the floor fails here by name.
func TestEntropyFloorIsWhereItSays(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"aaaaaaaaaaaa1", false},       // 0.39 bits/char
		{"ababababab1b", false},        // ~1.1 bits/char
		{"q7W2xE9rT4yU1iO6pA3s", true}, // ~4.3 bits/char
		{"9f3a2b7c1d4e5f60718293a4", true},
	} {
		if got := shannonBitsPerChar(tc.value) >= 3.0; got != tc.want {
			t.Errorf("value %q: above the 3.0 bits/char floor = %v (%.2f), want %v",
				tc.value, got, shannonBitsPerChar(tc.value), tc.want)
		}
	}
}
