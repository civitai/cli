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
//
// 🔴 "PRESENCE" IS ABOUT THE FILE, NOT ABOUT ITS BYTES — AND AN EMPTY FILE IS NOT
// A FRESHNESS QUESTION, IT IS "NOT A LOCKFILE AT ALL" (issue #255). The check used
// to ask os.Lstat and nothing else, so a 0-byte `package-lock.json` validated
// clean, exited 0, and the platform build failed anyway: measured on npm 11.17.0,
// `npm ci` over an empty package-lock.json dies with EUSAGE — "can only install
// with an existing package-lock.json or npm-shrinkwrap.json with lockfileVersion
// >= 1" — the SAME class of failure as a missing one. Worse, the missing-lockfile
// message names the filename, which makes `touch package-lock.json` a natural
// reading and a silently wrong one: the check invited the exact input that
// defeated it. So the required lockfile is now read, and the content rule is
// PER-MANAGER — see lockfileContentDefect.

import (
	"bytes"
	"encoding/json"
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
	// lockfileIsJSON selects the CONTENT rule. Only npm's lockfile is JSON with a
	// version key the CLI can check without a parser it does not have; see
	// lockfileContentDefect for why pnpm and yarn deliberately stop at "not
	// empty".
	lockfileIsJSON bool
	// contentRule states, in the author's terms, what the strict install requires
	// of the lockfile's CONTENT. It is appended to the "this is not a lockfile"
	// message so the remedy is checkable rather than a bare assertion.
	contentRule string
}

var (
	pmNpm = packageManager{
		name: "npm", lockfile: "package-lock.json",
		installCmd: "npm ci", refreshCmd: "npm install", buildCommand: "npm run build",
		lockfileIsJSON: true,
		contentRule: "`npm ci` requires a package-lock.json that parses as JSON and declares a " +
			`numeric "lockfileVersion" of 1 or more`,
	}
	pmPnpm = packageManager{
		name: "pnpm", lockfile: "pnpm-lock.yaml",
		installCmd: "pnpm install --frozen-lockfile", refreshCmd: "pnpm install", buildCommand: "pnpm run build",
		contentRule: "`pnpm install --frozen-lockfile` needs the file `pnpm install` wrote, and that file is never empty",
	}
	// The yarn branch of the recipe dispatches on `yarn --version`:
	//   1.*) yarn install --frozen-lockfile --ignore-scripts
	//   *)   YARN_ENABLE_SCRIPTS=false yarn install --immutable
	// so the message names both instead of claiming --frozen-lockfile always.
	pmYarn = packageManager{
		name: "yarn", lockfile: "yarn.lock",
		installCmd: "yarn install --frozen-lockfile (yarn 1) / --immutable (yarn 2+)",
		refreshCmd: "yarn install", buildCommand: "yarn run build",
		contentRule: "a strict `yarn install` needs the file `yarn install` wrote, and that file is never empty",
	}
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
//
// FIELD: FieldProject on both. These are findings about a FILE that is or is
// not committed, not about a manifest key — the manifest can be byte-perfect
// and still trip them. `buildCommand` appears in the remedy (one way out is to
// set it to match the lockfile you already have) but the other way out touches
// no manifest field at all, so pinning the finding there would mis-group every
// project that runs the install instead.
func lockfileChecks(dir string, m *manifest.Manifest) (errs []Finding, warns []Finding) {
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
	wantedDefect := ""
	for _, pm := range packageManagers {
		path := filepath.Join(dir, pm.lockfile)
		if !regularFileExists(path) {
			continue
		}
		committed = append(committed, pm)
		if pm.name == want.name {
			haveWanted = true
			// Only the REQUIRED lockfile's content is judged. A foreign one is
			// reported for what it is — a file that is committed and tells us
			// which package manager this project really uses — and that reading
			// does not depend on its bytes.
			wantedDefect = lockfileContentDefect(path, pm)
		} else {
			foreign = append(foreign, pm)
		}
	}

	if !haveWanted {
		return []Finding{newFinding(FieldProject, missingLockfileError(want, foreign, build))}, nil
	}
	if wantedDefect != "" {
		// The file is there and is provably not a lockfile, so the build fails
		// exactly as it would with nothing committed. Same tier, different
		// message: telling this author "no lockfile is committed" when one is
		// sitting in their tree is what makes `touch` look like the fix.
		return []Finding{newFinding(FieldProject, unusableLockfileError(want, wantedDefect))}, nil
	}
	// The required lockfile IS there, so the build installs strictly and
	// reproducibly — any extra lockfile is unused by the platform. That is not
	// build-breaking, so it belongs in the advisory tier, but it is worth saying:
	// an unused lockfile drifts, and the next contributor who runs the other
	// package manager lands right back in the mismatch above.
	if len(foreign) > 0 {
		warns = append(warns, newFinding(FieldProject, extraLockfileWarning(want, committed, build)))
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

// maxLockfileBytes bounds the read. Real lockfiles are kilobytes to a few
// megabytes (a big npm monorepo lock is single-digit MB), so 64 MiB is roughly
// two orders of magnitude above anything a package manager writes and cannot be
// reached by a genuine lockfile — while still bounding what `validate` will pull
// into memory for a file whose only job is to be checked for one key.
// `internal/validate` has been here before: before the ready-ack scan grew caps,
// one 88 MB `.js` took peak RSS to 316 MB (AGENTS.md item 18).
//
// 🔴 THE CAP IS NOT THE CEILING — IT IS ABOUT 2.2x THE CAP, AND SAYING "64 MiB"
// WOULD BE THE WRONG NUMBER. `os.ReadFile` holds the bytes and
// `json.Unmarshal` then allocates a `map[string]json.RawMessage` whose values
// alias that buffer but whose KEYS and map overhead do not. Peak RSS of
// `civitai app validate`, 3 runs each, base (8ed4d69) vs HEAD:
//
//	| lockfile                        | base            | HEAD              |
//	|---------------------------------|-----------------|-------------------|
//	| 104 B                           | 18.0 – 18.3 MB  | 17.7 – 17.9 MB    |
//	| 10 MB   (35,242 entries)        | 18.1 – 18.2 MB  | 37.4 – 37.7 MB    |
//	| 66 MB   (232,595 entries, UNDER)| 17.9 – 18.0 MB  | 146.9 – 147.3 MB  |
//	| 73 MB   (258,675 entries, OVER) | 17.9 – 18.2 MB  | 17.9 – 18.1 MB    |
//
// 🔴 QUOTE THE FIXTURE WITH THE NUMBER. These are REALISTIC lockfiles — real
// `packages` entries, each with a resolved registry URL, a sha512 integrity
// hash, a license and an engines object. A whitespace-padded fixture of the
// same byte count measures nothing, because the decoder allocates per KEY and
// not per byte, and would report a reassuring number while exercising none of
// the cost.
//
// The last row is what proves the cap is checked BEFORE the read rather than
// after: at 73 MB, HEAD costs the same as base. At realistic sizes (<= 10 MB)
// the whole thing costs about +20 MB, which is the trade being accepted here.
const maxLockfileBytes = 64 << 20

// lockfileContentDefect reports WHY the file at path is not a usable lockfile,
// or "" if it is one — or if we could not tell.
//
// 🔴 THIS CHECK IS FATAL, SO AN UNOBSERVABLE STATE MUST FALL BACK TO THE OLD
// PRESENCE-ONLY PASS AND NEVER TO AN ERROR. A read failure, or a file over
// maxLockfileBytes, means we did not look — and manufacturing a hard error that
// blocks a submit out of a gap is the expensive direction (AGENTS.md item 18:
// "reading nothing is not finding nothing"). The rule the check adds is "a file
// we READ and that provably is not a lockfile", never "a file we could not
// vouch for".
//
// 🔴 THE Lstat/IsRegular GATE STAYS IN FRONT OF THE READ, and the order is
// load-bearing rather than incidental. regularFileExists mirrors pkgzip.Build,
// which skips every non-regular entry, so a SYMLINKED lockfile is dropped from
// the submitted bundle. os.ReadFile follows symlinks; reading through one would
// vouch for content the bundle does not carry. Callers therefore call
// regularFileExists FIRST and only then reach this.
//
// The rule is per-manager, and the asymmetry is deliberate:
//
//   - npm: parse as JSON and require a NUMERIC `lockfileVersion` >= 1. That is
//     not a guess at npm's intent — it is npm's own precondition, quoted in
//     pmNpm.contentRule, so this mirrors the platform build recipe exactly the
//     way the rest of this file does.
//   - pnpm / yarn: non-empty after a whitespace trim, and nothing more.
//     `pnpm-lock.yaml` would need a YAML parser (a new third-party dependency,
//     which is an "ask first" in AGENTS.md) and a yarn v1 `yarn.lock` carries no
//     version key at all — it is a comment header and a flat list. "Not empty"
//     is the whole of what can be said here without inventing authority, and it
//     is exactly the reported defect from issue #255.
//
// 🔴 TWO RESIDUALS, STATED RATHER THAN HIDDEN — both are cases where this check
// is STRICTER than `npm ci`, which is the direction that can block a working
// project:
//
//   - A ZERO-DEPENDENCY project. Measured on npm 11.17.0: with no dependencies
//     at all, `npm ci` over `{}`, over an object with no version, over a JSON
//     array, over a string version and over version 0 all SUCCEED (rc 0) —
//     there is nothing to be out of sync about, so the sync check that rejects
//     them on a real project never fires. Only empty / whitespace / bare-BOM /
//     YAML / garbage still fail there. So the CLI refuses five shapes npm would
//     accept on a dependency-free app. Kept deliberately: npm never WRITES any
//     of them, an app with a buildCommand and zero dependencies (not even
//     devDependencies — a `vite` build has them) is close to hypothetical, and
//     accepting `{}` would reopen the headline defect with `echo '{}' >` in
//     place of `touch`.
//   - An `npm-shrinkwrap.json`-ONLY project is reported as having no lockfile
//     at all. `npm ci` accepts one — measured rc 0, node_modules populated, and
//     `npm shrinkwrap` is what renames package-lock.json into it. This is
//     PRE-EXISTING (the missing-lockfile check never knew the filename) and not
//     changed here; it is recorded because the shrinkwrap gap and this check now
//     live in the same paragraph of anyone's reading.
func lockfileContentDefect(path string, pm packageManager) string {
	info, err := os.Lstat(path)
	if err != nil || info.Size() > maxLockfileBytes {
		return "" // unobservable — degrade to presence-only
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "" // unobservable — degrade to presence-only
	}
	// 🔴 A UTF-8 BOM MUST BE STRIPPED BEFORE PARSING, OR THIS CHECK HARD-BLOCKS A
	// PROJECT THAT BUILDS. npm's parser tolerates one; Go's `encoding/json` does
	// not. Measured on npm 11.17.0: a real package-lock.json prefixed with
	// EF BB BF installs cleanly (`npm ci` rc 0, node_modules populated), while
	// this check called it "does not parse as a JSON object" and exited 1 — and
	// because the finding is FATAL it also blocks `app submit`. That is the
	// false-error-at-a-correct-project class this file argues against for the
	// unobservable case, arriving through the parser instead.
	// Honest scope: npm never WRITES a BOM, so reaching this needs an editor or
	// a gitattribute to have rewritten the file (VS Code's `files.encoding:
	// utf8bom`, Visual Studio, a `working-tree-encoding` attribute). Nobody has
	// measured how common that is, so the claim here is only that it is possible
	// and cheap to tolerate — not that it is frequent.
	// Known EQUIVALENT mutant, recorded so nobody re-derives it as a gap:
	// replacing this TrimPrefix with a strip-anywhere ReplaceAll survives the
	// suite. It is genuinely equivalent HERE — the only thing these bytes feed is
	// a decoder we ask for one key, and a U+FEFF inside some string VALUE cannot
	// change whether "lockfileVersion" is a number >= 1. Prefix-only is kept
	// because it is the narrower claim, not because a test distinguishes them.
	raw := body
	body = bytes.TrimPrefix(body, []byte("\xef\xbb\xbf"))
	hadBOM := len(body) != len(raw)
	if len(bytes.TrimSpace(body)) == 0 {
		switch {
		case len(raw) == 0:
			return "it is EMPTY (0 bytes)"
		case hadBOM && len(body) == 0:
			// A file holding nothing but a BOM. npm rejects it exactly as it
			// rejects a 0-byte one (measured: the lockfileVersion >= 1 EUSAGE),
			// so the strip above must not turn it into a pass.
			return "it is EMPTY (a byte-order mark and nothing else)"
		}
		return "it is EMPTY (whitespace only)"
	}
	if !pm.lockfileIsJSON {
		return ""
	}
	// A lockfile is a JSON OBJECT; an array, a string or a bare number decodes
	// without error and is still not one, so decode into a map rather than into
	// `any`.
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		return "it does not parse as a JSON object"
	}
	raw, ok := doc["lockfileVersion"]
	if !ok {
		return `it declares no "lockfileVersion"`
	}
	// 🔴 UNMARSHALLING THE VALUE STRAIGHT INTO A json.Number DOES NOT DISCRIMINATE:
	// json.Number is `type Number string`, so the JSON STRING "3" decodes into one
	// happily and `{"lockfileVersion": "3"}` sailed through. Decoding into `any`
	// with UseNumber is what keeps the two apart — a JSON number arrives as
	// json.Number, a JSON string as a string.
	var val any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&val); err != nil {
		return `its "lockfileVersion" is not a number`
	}
	num, ok := val.(json.Number)
	if !ok {
		return `its "lockfileVersion" is not a number`
	}
	v, err := num.Float64()
	if err != nil {
		// Float64 fails only on a number literal Go cannot represent (`1e999`).
		// That is not "below 1" — it is a number we could not read — so it gets
		// its own clause rather than a wrong one. Unreachable from any real
		// lockfile; the wording matters only because the old text was false if
		// it ever fired.
		return `its "lockfileVersion" is not a number this tool can read`
	}
	if v < 1 {
		return `its "lockfileVersion" is below 1`
	}
	return ""
}

// unusableLockfileError is the message for "the file is there and is not a
// lockfile". It is deliberately NOT the missing-lockfile message.
//
// 🔴 THE MESSAGE MUST NOT RE-INVITE THE WRONG TURN. missingLockfileError names
// the filename the build wants, and issue #255 is what an author does with that:
// `touch package-lock.json`, a green `validate`, and the identical opaque
// server-side build failure. So this one says the file EXISTS, says what is
// wrong with it, and says in as many words that a lockfile is generated by the
// package manager rather than created by hand.
// 🔴 IT MUST NOT SAY "EXACTLY AS IF NOTHING WERE COMMITTED", WHICH IS WHAT IT
// USED TO SAY AND IS MEASURABLY FALSE FOR MOST OF THESE SHAPES. Measured on npm
// 11.17.0 against a project with one real dependency, `npm ci` splits into two
// different EUSAGE failures:
//
//   - EMPTY, whitespace, a bare BOM, a YAML body and outright garbage get
//     "can only install with an existing package-lock.json or npm-shrinkwrap.json
//     with lockfileVersion >= 1" — genuinely the missing-lockfile failure.
//   - `{}`, an object with no version, a JSON array, a string version and
//     version 0 PARSE, so npm gets past that gate and fails the sync check
//     instead: "…package.json and package-lock.json…are in sync… Missing:
//     <pkg>@<ver> from lock file".
//
// Both are rc 1 on any project that has a dependency, so every verdict here
// stands — but the REASON differs, and stating one reason for both is the kind
// of confident-and-wrong sentence the next maintainer would trust instead of
// re-measuring.
func unusableLockfileError(want packageManager, defect string) string {
	return fmt.Sprintf(
		"%s is committed but %s, so it is not a lockfile the platform build can install from — it will run `%s`, which hard-fails on it. A lockfile is GENERATED by the package manager, never hand-written and never created with `touch`: delete %s, run `%s`, and commit the %s it writes. %s.",
		want.lockfile, defect, want.installCmd, want.lockfile, want.refreshCmd, want.lockfile, want.contentRule)
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
