package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeLockProject writes a package.json + the given lockfiles alongside a
// page-vite scaffold (which declares `"buildCommand": "npm run build"`).
func scaffoldWithLockfiles(t *testing.T, lockfiles ...string) string {
	t.Helper()
	tmp := t.TempDir()
	chdir(t, tmp)
	if _, _, err := run(t, "app", "init", "lock-block", "--template", "page-vite"); err != nil {
		t.Fatalf("app init page-vite: %v", err)
	}
	dir := filepath.Join(tmp, "lock-block")
	for _, name := range lockfiles {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(lockfileBodyFor(name)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// lockfileBodyFor returns a body the named package manager would actually WRITE.
//
// These fixtures were all the literal `{}`, which is not a lockfile: measured on
// npm 11.17.0, `npm ci` over `{}` exits 1 on any project with a dependency — via
// the sync check ("Missing: <pkg> from lock file"), NOT the empty file's
// "lockfileVersion >= 1" EUSAGE — and validate rejects it too (issue #255). A
// `{}` body under `package-lock.json` would make the PASS-expecting tests below
// assert that a build-breaking project validates clean.
func lockfileBodyFor(name string) string {
	switch name {
	case "package-lock.json":
		return `{"name":"lock-block","version":"0.1.0","lockfileVersion":3,"requires":true,"packages":{}}` + "\n"
	case "pnpm-lock.yaml":
		return "lockfileVersion: '9.0'\n"
	default:
		return "# yarn lockfile v1\n"
	}
}

// The headline case, end to end through the CLI: a pnpm lockfile under the
// scaffold's `npm run build`. It must fail (non-zero exit), and the message must
// name the committed lockfile, the install the platform will actually run, and
// BOTH remedies — including the outputDir the schema couples to buildCommand.
func TestAppValidateFailsOnPnpmLockWithNpmBuildCommand(t *testing.T) {
	dir := scaffoldWithLockfiles(t, "pnpm-lock.yaml")

	stdout, stderr, err := run(t, "app", "validate", dir)
	if err == nil {
		t.Fatalf("a mismatched lockfile must fail validate\nstdout:%s\nstderr:%s", stdout, stderr)
	}
	for _, want := range []string{
		"pnpm-lock.yaml is committed",
		`buildCommand is "npm run build"`,
		"npm ci",
		`"buildCommand": "pnpm run build"`,
		`"outputDir"`,
		"npm install",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("validate stderr missing %q:\n%s", want, stderr)
		}
	}
	// Diagnostics belong on stderr — stdout stays clean for piping.
	if strings.Contains(stdout, "pnpm-lock.yaml") {
		t.Errorf("validation errors must not go to stdout:\n%s", stdout)
	}
}

// --json must stay machine-clean: the lockfile error rides the existing errors
// array on stdout with nothing else, and the exit stays non-zero.
func TestAppValidateJSONCarriesLockfileError(t *testing.T) {
	dir := scaffoldWithLockfiles(t, "pnpm-lock.yaml")

	stdout, _, err := run(t, "app", "validate", dir, "--json")
	if err == nil {
		t.Fatal("a mismatched lockfile must exit non-zero in --json mode")
	}
	var got validateJSON
	if jerr := json.Unmarshal([]byte(stdout), &got); jerr != nil {
		t.Fatalf("--json output is not valid JSON: %v\n%s", jerr, stdout)
	}
	if got.OK {
		t.Error("ok must be false when the lockfile mismatches")
	}
	found := false
	for _, e := range got.Errors {
		if strings.Contains(e.Message, "pnpm-lock.yaml is committed") {
			found = true
		}
	}
	if !found {
		t.Errorf("--json errors should carry the lockfile message: %+v", got.Errors)
	}
}

// A matching lockfile clears the check end to end.
func TestAppValidatePassesWithMatchingLockfile(t *testing.T) {
	dir := scaffoldWithLockfiles(t, "package-lock.json")
	if stdout, stderr, err := run(t, "app", "validate", dir); err != nil {
		t.Fatalf("a matching lockfile must validate: %v\nstdout:%s\nstderr:%s", err, stdout, stderr)
	}
}

// A static block has no package.json, so the platform never installs — it must
// validate clean with no lockfile at all.
func TestAppValidateStaticScaffoldNeedsNoLockfile(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	if _, _, err := run(t, "app", "init", "static-block", "--template", "static"); err != nil {
		t.Fatalf("app init static: %v", err)
	}
	if stdout, stderr, err := run(t, "app", "validate", filepath.Join(tmp, "static-block")); err != nil {
		t.Fatalf("a static block must validate with no lockfile: %v\nstdout:%s\nstderr:%s", err, stdout, stderr)
	}
}

// `civitai app create` must keep working on a machine that has not installed
// anything: the scaffold self-check is about OUR templates, not the author's
// install state. The next steps must nonetheless tell them to commit it.
func TestAppCreateSucceedsWithoutLockfileAndSaysToCommitIt(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "fresh-block")
	stdout, _, err := run(t, "app", "create", "fresh-block", dest, "--template", "page-vite")
	if err != nil {
		t.Fatalf("app create must not fail on a missing lockfile: %v\n%s", err, stdout)
	}
	if _, serr := os.Stat(filepath.Join(dest, "package-lock.json")); serr == nil {
		t.Fatal("the scaffold must not fabricate a lockfile")
	}
	for _, want := range []string{"package-lock.json", "Commit the lockfile"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("create next steps should mention %q:\n%s", want, stdout)
		}
	}
	// And that fresh directory does NOT pass the full validate — the platform
	// build would hard-fail on it.
	if _, stderr, verr := run(t, "app", "validate", dest); verr == nil {
		t.Errorf("an uninstalled scaffold must fail validate:\n%s", stderr)
	}
}
