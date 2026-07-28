package validate

// lockfile.go implements the LOCKFILE ↔ buildCommand consistency check.
//
// WHY THIS EXISTS. The platform build recipe used to install with
//
//	npm ci --ignore-scripts || npm install --ignore-scripts
//
// That `||` fallback silently converted "no lockfile" / "lockfile out of sync"
// into a FRESH REGISTRY RESOLVE, so a build stopped being reproducible with
// nobody noticing, and then broke later — with zero code change — the moment an
// unrelated upstream published a new version.
//
// The fallback is GONE. The recipe now installs strictly from the committed
// lockfile, and a missing one is a hard build failure:
//
//	PM="${BUILD_CMD%% *}"            # the FIRST WORD of buildCommand
//	pnpm) [ -f pnpm-lock.yaml ]   || exit 1 ; pnpm install --frozen-lockfile …
//	yarn) [ -f yarn.lock ]        || exit 1 ; yarn  install --frozen-lockfile/--immutable …
//	*)    [ -f package-lock.json ]|| exit 1 ; npm ci …
//
// The dominant real-world misconfiguration (it hit FIVE live first-party apps at
// once) is a committed `pnpm-lock.yaml` next to `"buildCommand": "npm run build"`:
// the recipe picks npm from the first word, looks for `package-lock.json`, does
// not find it, and dies. Nothing about the project looks wrong locally — `pnpm
// run build` works fine on the author's machine.
//
// So this check is a FATAL error, not an advisory: the platform build hard-fails
// on it, and a green `validate` followed by a failed build is worse than no
// check at all. It fires ONLY when `package.json` exists — a static block with no
// `package.json` skips the recipe's install step entirely and must never be
// flagged.
//
// SCOPE. This is a presence check, not a freshness check: verifying a lockfile is
// actually IN SYNC with package.json requires running the package manager, which
// `validate` deliberately does not do. `npm ci` / `--frozen-lockfile` still catch
// a stale lockfile server-side (loudly, which is the point of removing the
// fallback).

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/civitai/cli/internal/manifest"
)

// legacyDefaultBuildCommand is what the platform build recipe falls back to when
// the manifest omits buildCommand. Its first word (npm) is what selects the
// required lockfile in that case.
const legacyDefaultBuildCommand = "npm run build"

// packageManager is one of the three package managers the platform build recipe
// knows how to install with, plus the strings needed to explain a mismatch.
type packageManager struct {
	// name is the token the recipe matches on — the FIRST WORD of buildCommand.
	name string
	// lockfile is the file the recipe requires (and installs strictly from).
	lockfile string
	// installCmd is the strict install the recipe runs (flags trimmed to the
	// part that matters to an author reading the message). For yarn the recipe
	// is VERSION-AWARE, so this names both forms rather than misstating one.
	installCmd string
	// refreshCmd is what the author runs LOCALLY to (re)generate the lockfile.
	refreshCmd string
	// buildCommand is the manifest buildCommand that selects this manager, used
	// when suggesting "switch the manifest to match the lockfile you have".
	buildCommand string
}

var (
	pmNpm  = packageManager{"npm", "package-lock.json", "npm ci", "npm install", "npm run build"}
	pmPnpm = packageManager{"pnpm", "pnpm-lock.yaml", "pnpm install --frozen-lockfile", "pnpm install", "pnpm run build"}
	// The yarn branch of the recipe dispatches on `yarn --version`:
	//   1.*) yarn install --frozen-lockfile --ignore-scripts
	//   *)   YARN_ENABLE_SCRIPTS=false yarn install --immutable
	// so the message names both instead of claiming --frozen-lockfile always.
	pmYarn = packageManager{"yarn", "yarn.lock", "yarn install --frozen-lockfile (yarn 1) / --immutable (yarn 2+)", "yarn install", "yarn run build"}
)

// packageManagers is the scan order for reporting which lockfiles are committed.
// Fixed (npm, pnpm, yarn) so messages are deterministic.
var packageManagers = []packageManager{pmNpm, pmPnpm, pmYarn}

// packageManagerFor mirrors the recipe's `PM="${BUILD_CMD%% *}"` exactly: take
// the FIRST WORD of buildCommand and match it against pnpm / yarn; EVERYTHING
// else falls through to the npm branch. Given the server's buildCommand
// allowlist (`(npm|pnpm|yarn) run <script>` | `vite build` | `npx vite build`)
// that "everything else" is `npm`, `vite`, `npx`, and the empty string — the
// omitted-buildCommand case, where the recipe uses the legacy `npm run build`
// default. All four therefore require package-lock.json.
func packageManagerFor(buildCommand string) packageManager {
	first := buildCommand
	if i := strings.IndexByte(first, ' '); i >= 0 {
		first = first[:i]
	}
	switch first {
	case pmPnpm.name:
		return pmPnpm
	case pmYarn.name:
		return pmYarn
	default:
		return pmNpm
	}
}

// lockfileChecks reports the lockfile ↔ buildCommand inconsistencies for the
// project in dir. Hard errors are returned first, advisories second.
func lockfileChecks(dir string, m *manifest.Manifest) (errs []string, warns []string) {
	// A block with no package.json is STATIC: the recipe's `if [ -f package.json ]`
	// guard is false, it never installs anything, and the bundle is served as-is.
	// Never flag it.
	if !regularFileExists(filepath.Join(dir, "package.json")) {
		return nil, nil
	}

	build := strings.TrimSpace(m.BuildCommand)
	// A buildCommand outside the allowlist is already a hard error from
	// buildCoherence, and the server rejects it before any build runs. Deriving a
	// package manager from it would only stack a second, more confusing message
	// on top of the real one.
	if build != "" && !buildCommandRe.MatchString(build) {
		return nil, nil
	}
	want := packageManagerFor(build)

	var committed, foreign []packageManager
	haveWanted := false
	for _, pm := range packageManagers {
		if !regularFileExists(filepath.Join(dir, pm.lockfile)) {
			continue
		}
		committed = append(committed, pm)
		if pm.name == want.name {
			haveWanted = true
		} else {
			foreign = append(foreign, pm)
		}
	}

	if !haveWanted {
		return []string{missingLockfileError(want, foreign, build)}, nil
	}
	// The required lockfile IS there, so the build installs strictly and
	// reproducibly — any extra lockfile is unused by the platform. That is not
	// build-breaking, so it belongs in the advisory tier, but it is worth saying:
	// an unused lockfile drifts, and the next contributor who runs the other
	// package manager lands right back in the mismatch above.
	if len(foreign) > 0 {
		warns = append(warns, extraLockfileWarning(want, committed, build))
	}
	return nil, warns
}

// pmClause explains, in prose, WHY a particular package manager was selected —
// either the buildCommand says so, or the manifest omits it and the recipe falls
// back to the legacy default.
func pmClause(build string) string {
	if build == "" {
		return fmt.Sprintf("the manifest sets no buildCommand (the platform build then falls back to the legacy default %q)", legacyDefaultBuildCommand)
	}
	return fmt.Sprintf("buildCommand is %q", build)
}

// outputDirNote is appended to every remedy that tells an author to SET
// buildCommand. The manifest schema couples the two with an `allOf`, so a
// buildCommand without an outputDir fails validation — suggesting one without
// the other just walks the author into a second failure.
const outputDirNote = `plus the "outputDir" the manifest schema requires alongside buildCommand`

// workspaceNote covers the monorepo/workspace shape, where the remedy above is
// otherwise a dead end: run inside a pnpm/yarn workspace PACKAGE, `pnpm install`
// writes the lockfile at the workspace ROOT, not beside the manifest — and
// pkgzip.Build walks only the manifest directory, so a parent-level lockfile is
// never bundled. This shape used to build under the old `|| npm install`
// fallback, so these authors are hitting a real platform behaviour change and
// need to know WHERE the lockfile has to live.
func workspaceNote(want packageManager) string {
	return fmt.Sprintf(
		"The submitted bundle is rooted at the manifest directory, so the %s must sit beside %s — in a workspace package `%s` writes it at the workspace ROOT, which is never bundled; generate one here instead.",
		want.lockfile, manifest.Filename, want.refreshCmd)
}

func missingLockfileError(want packageManager, foreign []packageManager, build string) string {
	switch len(foreign) {
	case 0:
		// No lockfile of any kind. Common right after `civitai app create`, before
		// the first install — still fatal, because submitting it fails the build.
		msg := fmt.Sprintf(
			"package.json is present but no lockfile is committed — %s, so the platform build will run `%s`, which hard-fails without %s. Run `%s` and commit the %s it writes: the platform installs strictly from the committed lockfile so builds are reproducible. %s",
			pmClause(build), want.installCmd, want.lockfile, want.refreshCmd, want.lockfile, workspaceNote(want))
		if build == "" {
			msg += fmt.Sprintf(
				" If this app uses pnpm or yarn instead, set \"buildCommand\" to it (e.g. %q) — %s — and commit that lockfile instead.",
				pmPnpm.buildCommand, outputDirNote)
		}
		return msg
	case 1:
		// The dominant real shape: a lockfile for a DIFFERENT package manager.
		have := foreign[0]
		return fmt.Sprintf(
			"%s is committed but %s, so the platform build will run `%s` and fail — it installs strictly from %s, which is not committed. Either set \"buildCommand\": %q (%s) to match the lockfile you already have, or run `%s` and commit the %s it writes.",
			have.lockfile, pmClause(build), want.installCmd, want.lockfile,
			have.buildCommand, outputDirNote, want.refreshCmd, want.lockfile)
	default:
		return fmt.Sprintf(
			"%s are committed, but none of them is the %s the platform build needs — %s, so it will run `%s` and fail. Keep the lockfile for the package manager this app actually uses and set \"buildCommand\" to match it (e.g. %q, %s), deleting the others; or run `%s` and commit the %s it writes.",
			joinLockfiles(foreign), want.lockfile, pmClause(build), want.installCmd,
			foreign[0].buildCommand, outputDirNote, want.refreshCmd, want.lockfile)
	}
}

func extraLockfileWarning(want packageManager, committed []packageManager, build string) string {
	return fmt.Sprintf(
		"more than one lockfile is committed (%s) — %s, so the platform build installs only from %s and ignores the rest. The unused lockfiles will silently drift out of sync; delete the ones for package managers this app does not use.",
		joinLockfiles(committed), pmClause(build), want.lockfile)
}

// joinLockfiles renders "a", "a and b", or "a, b and c".
func joinLockfiles(pms []packageManager) string {
	names := make([]string, len(pms))
	for i, pm := range pms {
		names[i] = pm.lockfile
	}
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
}

// regularFileExists reports whether path is a REGULAR file — deliberately using
// os.Lstat (which does NOT follow symlinks) plus an IsRegular test, so this
// mirrors the packager exactly.
//
// pkgzip.Build walks the project with filepath.WalkDir and skips every
// non-regular entry (`if !d.Type().IsRegular() { return nil }`), i.e. symlinks,
// sockets and devices are never bundled. os.Stat would FOLLOW a symlink, so a
// symlinked `package-lock.json` would look present here while being dropped
// from the submitted bundle — a green `validate` in front of exactly the opaque
// server-side "no lockfile" build failure this check exists to prevent. The two
// must agree; if pkgzip's rule changes, change this with it.
func regularFileExists(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}
