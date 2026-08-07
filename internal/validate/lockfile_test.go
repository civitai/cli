package validate

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/scaffold"
)

// lockManifest is an otherwise-valid manifest with the given buildCommand
// spliced in (plus the outputDir the schema requires alongside it). An empty
// buildCommand omits BOTH fields — the legacy-default case.
func lockManifest(buildCommand string) string {
	build := ""
	if buildCommand != "" {
		build = `, "buildCommand": ` + strconv.Quote(buildCommand) + `, "outputDir": "dist"`
	}
	return `{
		"blockId": "lock-block", "version": "0.1.0", "name": "Lock Block",
		"contentRating": "g", "scopes": [],
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts"}` + build + `
	}`
}

// project writes a manifest plus the given extra files (name -> contents) into a
// temp dir and validates it.
func project(t *testing.T, manifestJSON string, files map[string]string) Result {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(manifestJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Dir(dir)
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	return res
}

func lockErrors(res Result) []string {
	var out []string
	for _, e := range res.Errors {
		if strings.Contains(e.Message, "lockfile") || strings.Contains(e.Message, "-lock.") {
			out = append(out, e.Message)
		}
	}
	return out
}

func wantLockError(t *testing.T, res Result, substrs ...string) {
	t.Helper()
	errs := lockErrors(res)
	if len(errs) != 1 {
		t.Fatalf("want exactly 1 lockfile error, got %d: %v (all errors: %v)", len(errs), errs, res.Errors)
	}
	for _, s := range substrs {
		if !strings.Contains(errs[0], s) {
			t.Errorf("lockfile error missing %q:\n  %s", s, errs[0])
		}
	}
}

func wantNoLockError(t *testing.T, res Result) {
	t.Helper()
	if errs := lockErrors(res); len(errs) != 0 {
		t.Fatalf("want no lockfile error, got: %v", errs)
	}
}

// ---------------------------------------------------------------------------
// The check must NEVER fire on a static block.
// ---------------------------------------------------------------------------

func TestLockfileSkipsStaticApp(t *testing.T) {
	// No package.json at all: the platform recipe's `if [ -f package.json ]`
	// guard is false, so it never installs and never needs a lockfile.
	res := project(t, lockManifest(""), nil)
	if !res.OK() {
		t.Fatalf("static app must validate clean, got: %v", res.Errors)
	}
	if res.HasWarnings() {
		t.Fatalf("static app must be warning-free, got: %v", res.Warnings)
	}
}

func TestLockfileSkipsStaticScaffold(t *testing.T) {
	res, err := Dir(scaffoldRaw(t, scaffold.Static))
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if !res.OK() {
		t.Fatalf("static scaffold must validate clean with no lockfile, got: %v", res.Errors)
	}
}

// A static block that (oddly) ships a lockfile but no package.json is still
// skipped — the recipe keys off package.json, not the lockfile.
func TestLockfileSkipsStaticAppWithStrayLockfile(t *testing.T) {
	res := project(t, lockManifest(""), map[string]string{"pnpm-lock.yaml": "lockfileVersion: '9.0'\n"})
	if !res.OK() {
		t.Fatalf("static app with a stray lockfile must validate clean, got: %v", res.Errors)
	}
}

// ---------------------------------------------------------------------------
// Matching lockfile -> pass, for every package manager and every legal
// buildCommand shape the server allowlist permits.
// ---------------------------------------------------------------------------

func TestLockfileMatchingLockfilePasses(t *testing.T) {
	cases := []struct {
		name     string
		build    string // "" = omitted (legacy `npm run build` default)
		lockfile string
	}{
		{"npm run build + package-lock.json", "npm run build", "package-lock.json"},
		{"pnpm run build + pnpm-lock.yaml", "pnpm run build", "pnpm-lock.yaml"},
		{"yarn run build + yarn.lock", "yarn run build", "yarn.lock"},
		{"vite build + package-lock.json", "vite build", "package-lock.json"},
		{"npx vite build + package-lock.json", "npx vite build", "package-lock.json"},
		{"buildCommand omitted + package-lock.json", "", "package-lock.json"},
		// Non-"build" script names are allowlisted too (`run [a-zA-Z0-9:_-]+`).
		{"pnpm run build:prod + pnpm-lock.yaml", "pnpm run build:prod", "pnpm-lock.yaml"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := project(t, lockManifest(tc.build), map[string]string{
				"package.json": `{"name":"lock-block","private":true}`,
				tc.lockfile:    "{}\n",
			})
			if !res.OK() {
				t.Fatalf("should validate clean, got: %v", res.Errors)
			}
			if res.HasWarnings() {
				t.Errorf("should be warning-free, got: %v", res.Warnings)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Mismatched lockfile -> FATAL, with the specific remedy.
// ---------------------------------------------------------------------------

// The dominant real-world shape: a pnpm lockfile under an npm buildCommand.
// Five live first-party apps had exactly this.
func TestLockfilePnpmLockWithNpmBuildCommandIsFatal(t *testing.T) {
	res := project(t, lockManifest("npm run build"), map[string]string{
		"package.json":   `{"name":"lock-block","private":true}`,
		"pnpm-lock.yaml": "lockfileVersion: '9.0'\n",
	})
	if res.OK() {
		t.Fatal("a pnpm lockfile under `npm run build` must be a hard error")
	}
	wantLockError(t, res,
		"pnpm-lock.yaml is committed",
		`buildCommand is "npm run build"`,
		"npm ci",
		"package-lock.json, which is not committed",
		`"buildCommand": "pnpm run build"`,
		// Setting buildCommand requires outputDir (schema allOf) — a remedy that
		// omits this sends the author into a second validation failure.
		`"outputDir"`,
		"npm install",
	)
}

func TestLockfileMismatchIsFatalForEveryPairing(t *testing.T) {
	cases := []struct {
		name       string
		build      string
		lockfile   string
		wantLock   string
		wantRemedy string
	}{
		{"npm buildCommand + yarn.lock", "npm run build", "yarn.lock", "package-lock.json", `"buildCommand": "yarn run build"`},
		{"pnpm buildCommand + package-lock.json", "pnpm run build", "package-lock.json", "pnpm-lock.yaml", `"buildCommand": "npm run build"`},
		{"yarn buildCommand + pnpm-lock.yaml", "yarn run build", "pnpm-lock.yaml", "yarn.lock", `"buildCommand": "pnpm run build"`},
		// `vite build` / `npx vite build` take the recipe's npm branch.
		{"vite build + pnpm-lock.yaml", "vite build", "pnpm-lock.yaml", "package-lock.json", `"buildCommand": "pnpm run build"`},
		{"npx vite build + yarn.lock", "npx vite build", "yarn.lock", "package-lock.json", `"buildCommand": "yarn run build"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := project(t, lockManifest(tc.build), map[string]string{
				"package.json": `{"name":"lock-block","private":true}`,
				tc.lockfile:    "{}\n",
			})
			if res.OK() {
				t.Fatal("mismatched lockfile must be a hard error")
			}
			wantLockError(t, res, tc.lockfile+" is committed", tc.wantLock, tc.wantRemedy, `"outputDir"`)
		})
	}
}

// buildCommand OMITTED: the recipe falls back to the legacy `npm run build`, so a
// pnpm lockfile still fails. The remedy here MUST name outputDir explicitly —
// this is the case where the author has neither field yet.
func TestLockfileOmittedBuildCommandWithPnpmLockIsFatal(t *testing.T) {
	res := project(t, lockManifest(""), map[string]string{
		"package.json":   `{"name":"lock-block","private":true}`,
		"pnpm-lock.yaml": "lockfileVersion: '9.0'\n",
	})
	if res.OK() {
		t.Fatal("omitted buildCommand + pnpm lockfile must be a hard error")
	}
	wantLockError(t, res,
		"pnpm-lock.yaml is committed",
		"the manifest sets no buildCommand",
		"legacy default",
		"npm ci",
		`"buildCommand": "pnpm run build"`,
		`"outputDir"`,
	)
}

// ---------------------------------------------------------------------------
// No lockfile at all -> FATAL (the recipe hard-errors before installing).
// ---------------------------------------------------------------------------

func TestLockfileMissingEntirelyIsFatal(t *testing.T) {
	cases := []struct {
		name  string
		build string
		// want is the set of substrings the message must carry: the lockfile the
		// recipe requires plus the strict install it will actually run.
		want []string
	}{
		{"npm run build", "npm run build", []string{"package-lock.json", "npm ci"}},
		{"pnpm run build", "pnpm run build", []string{"pnpm-lock.yaml", "pnpm install --frozen-lockfile"}},
		// The recipe's yarn branch is VERSION-AWARE — the message must not claim
		// --frozen-lockfile is what yarn 2+ runs.
		{"yarn run build", "yarn run build", []string{"yarn.lock", "yarn install --frozen-lockfile", "--immutable"}},
		{"vite build", "vite build", []string{"package-lock.json", "npm ci"}},
		{"npx vite build", "npx vite build", []string{"package-lock.json", "npm ci"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := project(t, lockManifest(tc.build), map[string]string{
				"package.json": `{"name":"lock-block","private":true}`,
			})
			if res.OK() {
				t.Fatal("package.json with no lockfile must be a hard error")
			}
			wantLockError(t, res, append([]string{"no lockfile is committed"}, tc.want...)...)
		})
	}
}

// The remedy ("run <pm> install and commit the lockfile") is a dead end in a
// pnpm/yarn WORKSPACE package: that command writes the lockfile at the workspace
// ROOT, and pkgzip.Build walks only the manifest directory, so the root copy is
// never bundled. The message must say where the lockfile has to live.
func TestLockfileMissingEntirelyNamesTheBundleRoot(t *testing.T) {
	res := project(t, lockManifest("pnpm run build"), map[string]string{
		"package.json": `{"name":"lock-block","private":true}`,
	})
	if res.OK() {
		t.Fatal("package.json with no lockfile must be a hard error")
	}
	wantLockError(t, res,
		"no lockfile is committed",
		"rooted at the manifest directory",
		"beside block.manifest.json",
		"workspace ROOT",
	)
}

// With buildCommand omitted AND no lockfile, the message also has to offer the
// "I actually use pnpm/yarn" branch — and that branch must name outputDir.
func TestLockfileMissingEntirelyWithOmittedBuildCommand(t *testing.T) {
	res := project(t, lockManifest(""), map[string]string{
		"package.json": `{"name":"lock-block","private":true}`,
	})
	if res.OK() {
		t.Fatal("package.json with no lockfile must be a hard error")
	}
	wantLockError(t, res,
		"no lockfile is committed",
		"the manifest sets no buildCommand",
		"npm install",
		`"buildCommand"`,
		`"outputDir"`,
	)
}

// A freshly scaffolded npm project has no lockfile yet, so the full `validate`
// reports it — that project genuinely cannot be built by the platform. The
// scaffold self-check (ManifestOnly) must NOT.
func TestLockfileFreshScaffoldFailsDirButNotManifestOnly(t *testing.T) {
	for _, tmpl := range []scaffold.Template{scaffold.PageVite, scaffold.PageMoney} {
		t.Run(string(tmpl), func(t *testing.T) {
			dir := scaffoldRaw(t, tmpl)
			res, err := Dir(dir)
			if err != nil {
				t.Fatalf("Dir: %v", err)
			}
			if res.OK() {
				t.Fatal("an uninstalled npm scaffold must fail validate (the platform build would hard-fail)")
			}
			wantLockError(t, res, "no lockfile is committed", "npm install", "package-lock.json")

			mres, err := ManifestOnly(dir)
			if err != nil {
				t.Fatalf("ManifestOnly: %v", err)
			}
			if !mres.OK() {
				t.Fatalf("ManifestOnly must ignore install state, got: %v", mres.Errors)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// The check must agree with the PACKAGER about what counts as a file.
// ---------------------------------------------------------------------------

// A SYMLINKED lockfile is not bundled: pkgzip.Build skips every non-regular
// entry, so `app submit` would upload a bundle with no lockfile and the platform
// build would hard-fail — the exact opaque failure this check exists to prevent,
// with a green `validate` in front of it. So the check must treat a symlink as
// absent (os.Lstat + IsRegular), matching the packager.
func TestLockfileSymlinkedLockfileIsNotAccepted(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(lockManifest("npm run build")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"lock-block","private":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// A REAL lockfile outside the project, symlinked in — the shape that used to
	// pass because os.Stat follows symlinks.
	outside := filepath.Join(t.TempDir(), "package-lock.json")
	if err := os.WriteFile(outside, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "package-lock.json")
	if err := os.Symlink(outside, link); err != nil {
		// Symlink creation is not available everywhere (e.g. Windows without
		// developer mode) — skip rather than fail CI on an unrelated platform.
		t.Skipf("cannot create a symlink here: %v", err)
	}
	// Sanity: the symlink resolves, so an os.Stat-based check WOULD accept it.
	if _, err := os.Stat(link); err != nil {
		t.Fatalf("symlink should resolve: %v", err)
	}

	res, err := Dir(dir)
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if res.OK() {
		t.Fatal("a symlinked lockfile is dropped from the bundle, so it must NOT pass validation")
	}
	wantLockError(t, res, "no lockfile is committed", "package-lock.json", "npm ci")
}

// The same rule applied to package.json keeps validate and the bundle in
// agreement the other way: a symlinked package.json is not bundled either, so
// the uploaded app really is static and the check correctly stands down.
func TestLockfileSymlinkedPackageJSONIsTreatedAsStatic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(lockManifest("")), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "package.json")
	if err := os.WriteFile(outside, []byte(`{"name":"lock-block","private":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "package.json")); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}

	res, err := Dir(dir)
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if !res.OK() {
		t.Fatalf("a symlinked package.json is not bundled, so the app is static: %v", res.Errors)
	}
}

// ---------------------------------------------------------------------------
// Multiple lockfiles.
// ---------------------------------------------------------------------------

// The required lockfile IS present, so the build installs strictly and succeeds:
// advisory, not fatal.
func TestLockfileMultipleWithRequiredPresentIsWarning(t *testing.T) {
	res := project(t, lockManifest("npm run build"), map[string]string{
		"package.json":      `{"name":"lock-block","private":true}`,
		"package-lock.json": "{}\n",
		"pnpm-lock.yaml":    "lockfileVersion: '9.0'\n",
	})
	if !res.OK() {
		t.Fatalf("extra lockfiles must not fail the build-critical tier, got: %v", res.Errors)
	}
	if !res.HasWarnings() {
		t.Fatal("extra lockfiles should raise an advisory")
	}
	joined := strings.Join(Messages(res.Warnings), "\n")
	for _, want := range []string{"more than one lockfile is committed", "package-lock.json and pnpm-lock.yaml", "installs only from package-lock.json"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warning missing %q:\n%s", want, joined)
		}
	}
}

// Several lockfiles, none of them the required one: still fatal, and the message
// lists what IS there.
func TestLockfileMultipleWithRequiredAbsentIsFatal(t *testing.T) {
	res := project(t, lockManifest("npm run build"), map[string]string{
		"package.json":   `{"name":"lock-block","private":true}`,
		"pnpm-lock.yaml": "lockfileVersion: '9.0'\n",
		"yarn.lock":      "# yarn lockfile v1\n",
	})
	if res.OK() {
		t.Fatal("no matching lockfile must be a hard error")
	}
	wantLockError(t, res,
		"pnpm-lock.yaml and yarn.lock are committed",
		"none of them is the package-lock.json",
		"npm ci",
		`"outputDir"`,
	)
}

// ---------------------------------------------------------------------------
// Interaction with the existing buildCommand allowlist check.
// ---------------------------------------------------------------------------

// A buildCommand outside the server allowlist is already reported by
// buildCoherence. Deriving a package manager from it would stack a second,
// confusing message on top of the real one, so the lockfile check stands down.
func TestLockfileSilentOnDisallowedBuildCommand(t *testing.T) {
	res := project(t, lockManifest("make build"), map[string]string{
		"package.json":   `{"name":"lock-block","private":true}`,
		"pnpm-lock.yaml": "lockfileVersion: '9.0'\n",
	})
	if res.OK() {
		t.Fatal("a disallowed buildCommand must still fail")
	}
	wantNoLockError(t, res)
}

// ---------------------------------------------------------------------------
// Unit coverage of the recipe mirror itself.
// ---------------------------------------------------------------------------

// packageManagerFor must mirror `PM="${BUILD_CMD%% *}"` for every shape the
// server's BUILD_COMMAND_RE admits, plus the omitted case.
func TestPackageManagerForMirrorsRecipe(t *testing.T) {
	cases := map[string]string{
		"":                    "package-lock.json", // legacy default `npm run build`
		"npm run build":       "package-lock.json",
		"npm run build:prod":  "package-lock.json",
		"pnpm run build":      "pnpm-lock.yaml",
		"pnpm run build_prod": "pnpm-lock.yaml",
		"yarn run build":      "yarn.lock",
		"yarn run build-all":  "yarn.lock",
		"vite build":          "package-lock.json",
		"npx vite build":      "package-lock.json",
	}
	for build, wantLock := range cases {
		if got := packageManagerFor(build).lockfile; got != wantLock {
			t.Errorf("packageManagerFor(%q).lockfile = %q, want %q", build, got, wantLock)
		}
	}
}

// Every buildCommand the allowlist admits must map to a known package manager —
// a new allowed shape that this check does not understand would silently pick
// the npm branch, so keep the two in lockstep.
func TestEveryAllowedBuildCommandIsCoveredByTheAllowlist(t *testing.T) {
	for _, build := range []string{"npm run build", "pnpm run build", "yarn run build", "vite build", "npx vite build"} {
		if !buildCommandRe.MatchString(build) {
			t.Errorf("%q should match the buildCommand allowlist", build)
		}
	}
	for _, build := range []string{"make build", "npm install && npm run build", "pnpm build", "yarn build"} {
		if buildCommandRe.MatchString(build) {
			t.Errorf("%q should NOT match the buildCommand allowlist", build)
		}
	}
}
