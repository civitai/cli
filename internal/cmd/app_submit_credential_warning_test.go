package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

// Issue #464: `secrets.json`, `src/credentials.yaml` and `config.toml` were all
// PACKAGED — measured, with their marker strings surviving into the bundle bytes
// — and nothing said so. #435's `Skipped …` line is structurally silent here,
// because nothing was skipped: this is the opposite residual, files that DID
// ship and should not have.
//
// 🔴 THESE TESTS ASSERT A WARNING AND AN UNCHANGED EXIT CODE, TOGETHER. A false
// positive that dropped app source, or that started failing a `--yes`/CI submit,
// would cost far more than the message is worth, so "it warned" and "it did not
// fail" are one contract and are asserted in the same run.

// credentialWarningBlock returns the lines of the credential warning, from the
// header to the last entry — the block a scoping assertion must be made against.
// It is separated from the rest of stdout on purpose: `.env.local` legitimately
// appears in the `Skipped …` line, so a whole-stdout Contains check would be a
// spelled guard that another feature satisfies (see the sibling skipped-line
// test for the same trap, hit for real).
func credentialWarningBlock(t *testing.T, stdout string) []string {
	t.Helper()
	lines := strings.Split(stdout, "\n")
	start := -1
	for i, l := range lines {
		if strings.Contains(l, "look like they hold credentials:") {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}
	block := []string{lines[start]}
	for _, l := range lines[start+1:] {
		if !strings.HasPrefix(l, "    ") {
			break
		}
		block = append(block, l)
	}
	return block
}

// TestSubmitWarnsAboutPackagedCredentials is the regression test: the real
// command, over the issue's own three files plus a root `.env.production`, must
// print one warning naming all four with a line number — and must still exit 0.
func TestSubmitWarnsAboutPackagedCredentials(t *testing.T) {
	dir := t.TempDir()
	writeStaticManifest(t, dir)
	mustWrite(t, dir, "secrets.json", "{\n  \"API_SECRET\": \"9f3a2b7c1d4e5f60718293a4b5c6d7e8\"\n}\n")
	mustWrite(t, dir, "src/credentials.yaml", "password: hunter2-Kx9pQ4mZ7vT1\n")
	mustWrite(t, dir, "config.toml", "[auth]\ntoken = \"gA7xQ2mV9pL4zR8nB3kW\"\n")
	// 🔴 ALLOW-LISTED BY NAME AND THEREFORE IN SCOPE. keptEnvFiles keeps this file
	// because the platform build reads it; its own doc comment says that is a
	// statement about where the file GOES, never that it is safe.
	mustWrite(t, dir, ".env.production",
		"VITE_BLOCK_ALLOWED_PARENT_ORIGINS=https://civitai.com\nAPI_SECRET=7f3Kq9Zx2Lm5Rb8Nv4Tc\n")
	mustWrite(t, dir, "src/App.tsx", "export default 1\n")

	out := filepath.Join(t.TempDir(), "bundle.zip")
	stdout, stderr, err := run(t, "app", "submit", dir, "--package-only", "--skip-validate", "--out", out)
	// 🔴 EXIT CODE FIRST. The warning must never gate a submit.
	if err != nil {
		t.Fatalf("app submit --package-only returned an error (%v) — this warning must never change the exit "+
			"code.\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Packaged ") {
		t.Fatalf("CONTROL failure: no `Packaged …` line, so the run never reached the printing code:\n%s", stdout)
	}

	block := credentialWarningBlock(t, stdout)
	if len(block) == 0 {
		t.Fatalf("no credential warning printed although four packaged files hold credential-shaped values:\n%s", stdout)
	}
	if want := "⚠ 4 packaged file(s) look like they hold credentials:"; strings.TrimSpace(block[0]) != want {
		t.Errorf("header =\n  %s\nwant\n  %s", strings.TrimSpace(block[0]), want)
	}

	// Each entry is `path:line  label`, asserted field-wise so the column padding
	// (which depends on the longest path) is not part of the contract.
	gotEntries := map[string]string{}
	for _, l := range block[1:] {
		f := strings.Fields(l)
		if len(f) != 2 {
			t.Errorf("entry %q does not read as `path:line  label`", l)
			continue
		}
		gotEntries[f[0]] = f[1]
	}
	for loc, label := range map[string]string{
		".env.production:2":      "API_SECRET",
		"config.toml:2":          "token",
		"secrets.json:2":         "API_SECRET",
		"src/credentials.yaml:1": "password",
	} {
		got, ok := gotEntries[loc]
		if !ok {
			t.Errorf("no entry for %s — that file is packaged and holds a credential-shaped value:\n%s",
				loc, strings.Join(block, "\n"))
			continue
		}
		if got != label {
			t.Errorf("%s labelled %q, want %q", loc, got, label)
		}
	}
	if len(gotEntries) != 4 {
		t.Errorf("the warning lists %d entr(ies), want 4:\n%s", len(gotEntries), strings.Join(block, "\n"))
	}

	// The advice has to name what to DO, say the values were withheld on purpose,
	// and — the honest part — say that on a real submit this REPORTS rather than
	// prevents. Verified ordering in RunE: confirmSubmit, then this warning, then
	// doUpload, with no gate between; `--package-only` is the only path where an
	// author can still act. "cannot be recalled" alone reads as advice you can
	// act on, which on the submit path is false.
	for _, want := range []string{
		"read by a reviewer",
		"not printed here",
		"it does not stop the upload",
		"--package-only",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the warning does not say %q:\n%s", want, stdout)
		}
	}
}

// TestSubmitWarningLabelIsNeverSecretMaterial is the end-to-end half of the
// audited leak: the key group is unanchored, so a credential word sitting INSIDE
// credential-shaped material built the label out of value bytes. Measured on the
// first shipped build, `AbCdEfLEAKMARK9SecretKey123` was printed as a "key" one
// line above the sentence promising values are not printed.
//
// The whole rendered output is searched, not just the label column: this asserts
// the property the user sees.
func TestSubmitWarningLabelIsNeverSecretMaterial(t *testing.T) {
	const marker = "LEAKMARK"
	dir := t.TempDir()
	writeStaticManifest(t, dir)
	mustWrite(t, dir, "keys.yaml", "AbCdEf"+marker+"9SecretKey123: Zq4TvW9mB2xR7kP1nJ6y\n")
	mustWrite(t, dir, "deploy.sh", "export sk_live_TOKEN"+marker+"42=AbCdEf9Zq4TvW9mB2xR7\n")

	stdout := submitPackageOnly(t, dir)
	block := credentialWarningBlock(t, stdout)
	if len(block) < 2 {
		t.Fatalf("CONTROL failure: nothing was reported for two credential-shaped lines, so the absence of "+
			"the marker below proves nothing:\n%s", stdout)
	}
	if strings.Contains(stdout, marker) {
		t.Errorf("the rendered warning carries bytes of the credential-shaped material it matched "+
			"(marker %q). It prints directly above a sentence saying values are not printed:\n%s",
			marker, strings.Join(block, "\n"))
	}
}

// TestSubmitCredentialWarningNeverPrintsTheValue is the leak-prevention property
// of the feature itself. This CLI's output lands in CI logs and terminal
// scrollback, so a warning that printed the matched secret would move the
// credential into a SECOND durable place while telling the author off for the
// first.
//
// The planted values are distinctive strings that appear nowhere else, so a hit
// can only come from the fixture — and each is planted in a file the test also
// proves was REPORTED, so the absence is a fact about the renderer and not about
// a scan that never ran.
func TestSubmitCredentialWarningNeverPrintsTheValue(t *testing.T) {
	const (
		jsonValue = "Zq4TvW9mB2xR7kP1nJ6y"
		envValue  = "Vt8Wc3Nk6Zp2Lr9Qm4Xy"
	)
	dir := t.TempDir()
	writeStaticManifest(t, dir)
	mustWrite(t, dir, "secrets.json", "{\n  \"API_SECRET\": \""+jsonValue+"\"\n}\n")
	mustWrite(t, dir, ".env.production", "API_SECRET="+envValue+"\n")

	out := filepath.Join(t.TempDir(), "bundle.zip")
	stdout, stderr, err := run(t, "app", "submit", dir, "--package-only", "--skip-validate", "--out", out)
	if err != nil {
		t.Fatalf("app submit --package-only: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	block := credentialWarningBlock(t, stdout)
	if len(block) != 3 {
		t.Fatalf("CONTROL failure: want a header and two entries, got %d line(s) — if the scan did not fire, "+
			"the absence of the values below proves nothing:\n%s", len(block), stdout)
	}
	for name, value := range map[string]string{"secrets.json": jsonValue, ".env.production": envValue} {
		if strings.Contains(stdout, value) {
			t.Errorf("the value planted in %s was printed to stdout. This output lands in CI logs and terminal "+
				"scrollback: printing the secret moves it into a second durable place.\n%s", name, stdout)
		}
		if strings.Contains(stderr, value) {
			t.Errorf("the value planted in %s was printed to stderr:\n%s", name, stderr)
		}
	}
}

// TestSubmitCredentialWarningNamesOnlyPackagedFiles is the scoping control. The
// scan runs over pkgzip's own file list precisely so a warning can never name a
// file the packager DROPPED — re-deriving the exclusion rules here would be a
// second copy that drifts from the first, which is the whole #435 class.
//
// Its positive control is inside the same run: `secrets.json` carries the SAME
// value shape and IS reported, so the two silences below are facts about scope
// rather than about a detector that fires on nothing.
func TestSubmitCredentialWarningNamesOnlyPackagedFiles(t *testing.T) {
	dir := t.TempDir()
	writeStaticManifest(t, dir)
	mustWrite(t, dir, "secrets.json", "{\n  \"API_SECRET\": \"9f3a2b7c1d4e5f60718293a4b5c6d7e8\"\n}\n")
	mustWrite(t, dir, ".env.local", "API_SECRET=Rk7Yb2Wm9Tx4Ld6Pq3Zn\n")
	mustWrite(t, dir, "node_modules/leak.js", "const apiToken = \"Q8vN2mR7kX4zL9pW3tB6\";\n")

	stdout := submitPackageOnly(t, dir)
	block := credentialWarningBlock(t, stdout)
	if len(block) != 2 {
		t.Fatalf("CONTROL failure: want a header and exactly one entry (secrets.json), got %d line(s):\n%s",
			len(block), stdout)
	}
	if !strings.Contains(block[1], "secrets.json:2") {
		t.Errorf("the packaged file is not the one reported: %q", block[1])
	}
	for _, dropped := range []string{".env.local", "node_modules"} {
		for _, l := range block {
			if strings.Contains(l, dropped) {
				t.Errorf("the warning names %s, which the packager DROPPED — the scan must read only what "+
					"shipped:\n%s", dropped, strings.Join(block, "\n"))
			}
		}
	}
	// And the values in the dropped files never reach the output by any route.
	for _, v := range []string{"Rk7Yb2Wm9Tx4Ld6Pq3Zn", "Q8vN2mR7kX4zL9pW3tB6"} {
		if strings.Contains(stdout, v) {
			t.Errorf("a value from a file that was never packaged appears in stdout:\n%s", stdout)
		}
	}
}

// TestSubmitPrintsNoCredentialWarningOnACleanProject pins the empty case: no
// "0 packaged file(s) look like…" line. A zero-line on every clean submit is
// noise, and noise is what makes the non-zero line get skimmed.
//
// 🔴 INVARIANT GUARD, NOT A REGRESSION TEST, AND LABELLED THAT WAY BECAUSE IT
// WAS MEASURED: run verbatim against origin/main it PASSES — trivially, because
// that build prints no such line under any circumstances.
func TestSubmitPrintsNoCredentialWarningOnACleanProject(t *testing.T) {
	dir := t.TempDir()
	writeStaticManifest(t, dir)
	mustWrite(t, dir, "src/App.tsx", "export default 1\n")
	// Ordinary code that a keyword-only detector would flag.
	mustWrite(t, dir, "src/auth.ts", strings.Join([]string{
		`const label = "password";`,
		`const apiKey = process.env.VITE_API_KEY;`,
		`const token = options.credentials.token;`,
		`const PASSWORD_MIN = 8;`,
		`const placeholder = "your-api-key-here";`,
	}, "\n")+"\n")
	mustWrite(t, dir, "package-lock.json",
		"{\n  \"integrity\": \"sha512-9Kq2mZx7Lb4Rv8Nc3Tp6Wy1Ud5Ha0Sj2GfQ7pR4tE1mV6yUxKw==\"\n}\n")
	mustWrite(t, dir, ".env.production", "VITE_BLOCK_ALLOWED_PARENT_ORIGINS=https://civitai.com\n")

	stdout := submitPackageOnly(t, dir)
	if block := credentialWarningBlock(t, stdout); len(block) != 0 {
		t.Errorf("warned over a project holding only ordinary code:\n%s", strings.Join(block, "\n"))
	}
	for _, l := range strings.Split(stdout, "\n") {
		if strings.Contains(l, "hold credentials") {
			t.Errorf("line %q looks like a credential warning although nothing matched:\n%s", l, stdout)
		}
	}
}
