// Package pkgzip packages an App project directory into the canonical
// ZIP the platform build recipe expects: the SOURCE tree (manifest + src +
// build config), with build artifacts and VCS/dependency dirs excluded.
//
// Two kinds of exclusion are applied:
//
//   - Directory names (excludedDirs): VCS metadata, dependency installs, build
//     output, tooling caches — see that var. The VCS names among them
//     (vcsMetadataNames) are excluded whether the entry is a directory or a
//     regular file, because in a linked worktree or a submodule `.git` is a
//     FILE — see that var for the incident.
//
//   - File names (isExcludedFile): VCS metadata that is a regular file
//     (a worktree's / submodule's `.git`), build artifacts (*.zip), and dotenv
//     files matched from BOTH ends — every base name that STARTS with ".env"
//     (dotted or not, so `.envrc`, which routinely holds exported credentials,
//     goes too) and every base name that ENDS in ".env" (`db.env`, `prod.env` —
//     the shape tooling writes, invisible to a prefix rule; #435). The DOTENV
//     and *.zip matching ignores case; the VCS names in this same bullet do
//     NOT — a file called `.GIT` is packaged. Except a three-name allow-list,
//     which is matched EXACTLY.
//
//     The scaffolded page-money template points a real,
//     Buzz-spending block token (VITE_LIVE_BLOCK_TOKEN) at the git-ignored
//     .env.development.local for `dev:live` — vite's dev-local override, which
//     the catch-all drops; bundling any of vite's dev-local files would leak the
//     token to the server, the moderator reviewer, and the built image.
//     (This said ".env.development" until #380. The scaffolded .env.development
//     never carried that instruction; the file that did was .env.example — and
//     that one is ALLOW-LISTED and UPLOADED, see keptEnvFiles.) At the PROJECT
//     ROOT we KEEP .env.example / .env.sample (meant to hold documented
//     placeholders) and .env.production (the server build's `vite build` runs in
//     mode=production and reads it). At any depth below the root those same three
//     names fall to the catch-all and are DROPPED.
//     Those three are allow-listed BY NAME AT THE ROOT — nothing reads their
//     contents, so keeping one is not a claim that it holds no secret. See
//     isExcludedFile and keptEnvFiles for the full rule + rationale.
package pkgzip

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/civitai/cli/internal/manifest"
)

// 🔴 THESE ARE THE CLI'S OWN SANITY CAPS. THEY ARE NOT A MIRROR OF THE SERVER,
// AND A PACKAGE THAT PASSES THEM CAN STILL BE REJECTED ON SIZE GROUNDS.
//
// This comment used to assert the opposite — that these numbers mirrored the
// platform's, so a package the CLI accepted would not be refused on size — and
// issue #423 is the measured counterexample. An 8.20 MB compressed bundle
// passed every cap here and the server refused it; a 2.32 MB one was accepted.
// The false claim is what made that refusal unreadable: the CLI's own preflight
// reported success about a bundle that could never land, and the server's
// answer (`400: Invalid JSON`, an error about the PARSE) names nothing
// size-shaped, so nothing anywhere pointed at the size.
//
// The exact sentence, and the four error messages that carried the same claim,
// are in #423 and in this change's commit — deliberately not requoted here,
// because caps_claim_test.go bans those phrasings from this file outright, and
// a quotation the guard has to make an exception for is a guard with a hole.
//
// 🔴 THE SERVER'S REAL BUNDLE CEILING IS NOT KNOWN HERE, AND MUST NOT BE
// GUESSED. #423's measurement bounds it to the interval (2.32 MB, 8.20 MB] and
// no further — each additional probe costs a real submission. Vendoring a
// number picked from inside that bracket would refuse bundles the server
// accepts, with no appeal and nothing to tell the author it was the CLI's guess
// rather than a real limit: strictly worse than the failure it would be fixing.
// (Same reasoning as AGENTS.md item 25, one layer up.)
//
// So these caps stay generous, and the CLI REPORTS instead of refusing:
// appapi.SubmitBodySize is the size a body limit would actually apply to (the
// zip is base64-encoded into a JSON document before it is sent, so the server
// sees ~4/3 of the number below), and LargestEntries names what to delete
// first. Both are printed by `civitai app submit` — the second only when a
// submit has already failed.
const (
	MaxFiles            = 2000
	MaxFileSizeBytes    = 10 * 1024 * 1024  // 10 MiB per file
	MaxBundleSizeBytes  = 50 * 1024 * 1024  // 50 MiB total (compressed upload)
	MaxDecompressedSize = 200 * 1024 * 1024 // 200 MiB decompressed total
)

// excludedDirs are directory names skipped anywhere in the tree. The platform
// rebuilds from source, so VCS metadata, dependency installs, build output, and
// tooling caches must not be shipped — they bloat the bundle (a stray Python
// .venv alone added ~860 files / ~11 MiB to a Vite/TS block) and slow the
// review-repo push without ever being used by the server build recipe.
var excludedDirs = map[string]struct{}{
	// VCS metadata
	".git": {},
	".hg":  {},
	".svn": {},
	// dependency installs
	"node_modules": {},
	".venv":        {},
	"venv":         {},
	".pnpm-store":  {},
	// build output
	"dist":  {},
	"build": {},
	"out":   {},
	".next": {},
	// tooling / test caches
	".vite":         {},
	".cache":        {},
	"coverage":      {},
	".pytest_cache": {},
	".mypy_cache":   {},
	".ruff_cache":   {},
	".turbo":        {},
}

// vcsMetadataNames are the excludedDirs entries whose exclusion is INDEPENDENT
// OF FILE TYPE: they are dropped whether the entry is a directory or a regular
// file, at any depth.
//
// 🔴 THE RULE USED TO BE KEYED ON TYPE, AND THAT IS ISSUE #409. `.git` was on
// excludedDirs from the beginning, but the map was consulted only inside Build's
// `if d.IsDir()` branch — so the exclusion held for the shape its author had in
// mind (a clone) and silently did not hold for two shapes that are just as
// ordinary. In a LINKED WORKTREE and in a SUBMODULE, `.git` is a regular file
// holding `gitdir: <absolute path>`; it passed the directory gate (not a
// directory) and the file gate (isExcludedFile knew only *.zip and .env*), and
// was packaged. `app submit` from a worktree then died on a server error that
// named nothing the author could act on — Forgejo 422, "path contains a
// malformed path component [path: .git]" — while the same commit submitted from
// a clone succeeded. The tell was the file count: 62 from the worktree, 61 from
// the clone.
//
// 🔴 EXACT NAMES, NOT A PREFIX. `.gitignore`, `.gitattributes`, `.gitmodules`,
// `.gitkeep` and a `.github/` directory are all project content that belongs in
// the bundle; a prefix match on ".git" would delete every one of them from a
// submission with no error anywhere. (Contrast the dotenv rule below, which IS a
// prefix — there the failure mode of being too narrow is a leaked credential, so
// it is aimed the other way on purpose.)
//
// 🔴 ONLY VCS METADATA IS PROMOTED, NOT ALL OF excludedDirs. The issue suggested
// having isExcludedFile reject any name in excludedDirs; that would also drop a
// regular file named `build`, `dist`, `out` or `coverage` — a build script
// without an extension is a real thing, and dropping one is a silent content
// loss with no server-side error to catch it. A regular file named exactly
// `.git`, `.hg` or `.svn`, by contrast, is never author content: it is VCS
// plumbing in every case, and the server rejects it outright.
//
// Every name here MUST also be in excludedDirs — the directory half of the rule
// still runs through that map, and TestVCSMetadataNamesAreASubsetOfExcludedDirs
// pins the two together.
var vcsMetadataNames = map[string]struct{}{
	".git": {},
	".hg":  {},
	".svn": {},
}

// isDotenvShaped reports whether a base name follows the dotenv NAMING
// CONVENTION: exactly ".env", or ".env." followed by anything (".env.local",
// ".env.d", ".env.development.local").
//
// 🔴 THIS IS NARROWER THAN isExcludedFile'S DOTENV RULE, ON PURPOSE, AND THE
// DIFFERENCE IS THE WHOLE DESIGN OF issue #420. For a FILE the rule is the bare
// ".env" prefix, undotted names included, because there the cost of matching too
// much is one file the author renames and the cost of matching too little is an
// uploaded credential — so it is aimed wide. A DIRECTORY inverts the first half:
// matching too much removes an entire SUBTREE from the submission, silently,
// with no server-side error to catch it. `.environment/` and `.envoy/` are
// ordinary content directories whose names begin with those four bytes, and the
// bare prefix would delete both.
//
// Requiring the dot keeps every conventional dotenv directory (`.env`,
// `.env.d`, `.env.local`) and excludes no plausible content name, because a
// directory named `.env.<something>` is a dotenv container in every case anyone
// has produced.
//
// 🔴 RESIDUAL, ACCEPTED AND DELIBERATE: a directory named `.env-backup` or
// `.envs` is NOT excluded, though the same name as a FILE would be. That costs
// little now — the FILE rules reach inside such a directory and drop every
// dotenv shape there: `.env.local` by the prefix rule, `db.env` by the `*.env`
// suffix rule, and since the allow-list was scoped to the project root
// (keptEnvFiles) `.env-backup/.env.production` too. What survives is a file
// whose NAME is not dotenv-shaped at all — `.env-backup/credentials.json` — the
// class no name rule can close.
//
// If a real case for excluding those directory names appears, add the exact
// name to excludedDirs rather than widening this predicate; that keeps the loss
// visible in a list someone can read.
// 🔴 CASE-INSENSITIVE since #435: `.ENV.local/` packaged. Case is not a
// meaningful distinction for this convention — nobody names a content directory
// to differ from a dotenv one only by case — so folding costs nothing and
// closes a measured hole. The FIXED-name maps are deliberately NOT folded; see
// isExcludedDir.
func isDotenvShaped(name string) bool {
	lower := strings.ToLower(name)
	return lower == ".env" || strings.HasPrefix(lower, ".env.")
}

// isArchiveArtifact reports whether a base name is a packaged archive. One
// predicate for both file types: a `--package-only` run drops `<app>-<ver>.zip`
// in the project dir, and an author who unpacks one in place leaves a DIRECTORY
// of the same shape. The platform rebuilds from source, so neither ships.
// Case-insensitive since #435: `x.ZIP/` packaged. Same reasoning as
// isDotenvShaped — `.ZIP` is the same convention shouted.
func isArchiveArtifact(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".zip")
}

// isDotenvSuffixed reports whether a FILE base name ends in ".env" —
// `db.env`, `prod.env`, `local.env`, `config.env`.
//
// 🔴 THIS IS THE SHAPE TOOLING ACTUALLY WRITES, AND NO RULE SAW IT UNTIL #435.
// The dotenv rules were all PREFIX rules, so a name that does not START with
// ".env" was invisible to every one of them, at every depth. Measured on
// v0.1.97 through the released binary: `db.env` at the project root,
// `envs/prod.env`, `env/local.env` and `.env-backup/db.env` were all packaged
// into a bundle that is committed to Forgejo and deployed. #420 closed the
// dotenv-shaped DIRECTORY hole and did nothing for this one.
//
// 🔴 FILES ONLY — isExcludedDir does NOT get this rule. The asymmetry is the one
// #420 settled over four review rounds: for a FILE, matching too much costs one
// file the author renames, and matching too little costs an uploaded credential,
// so it is aimed wide. For a DIRECTORY, matching too much removes an entire
// subtree silently, so `config.env/` still ships.
//
// 🔴 THE RESIDUAL IS NOT HYPOTHETICAL, AND IT IS ACCEPTED WITH THAT KNOWN.
// `.env` is also **Babylon.js's environment-texture format**, so a 3D block
// shipping `public/environment.env` loses it — and the loss surfaces at
// RUNTIME in the deployed app, not at submit time, because nothing prints what
// the packager dropped. `sample.env` / `template.env` go the same way, and the
// allow-list has no suffix-shaped counterpart (`.env.sample` is kept,
// `sample.env` is not).
//
// It is still the right trade: an uploaded credential is durably published to
// Forgejo and a human reviewer and cannot be recalled, while a dropped asset is
// recoverable by renaming the file. But it is a real cost, it is called out in
// the README beside the residual table (it cannot BE a row there — that table
// lists what is still uploaded, and this is the opposite residual), and it is the strongest
// argument for the drop-messaging tracked in #435 — a one-line "skipped N
// path(s)" would turn this from a silent runtime break into an obvious one.
//
// Nothing the scaffold emits is affected: its dotenv files are
// `.env.development` / `.env.example` / `.env.production`, all PREFIXED, none
// suffixed. Vite reads only `.env`, `.env.local` and `.env.[mode]` variants, so
// no build-time input is lost either.
func isDotenvSuffixed(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".env")
}

// isExcludedDir reports whether a DIRECTORY (by base name) must be left out of
// the package, at any depth. Three rules:
//
//   - the fixed names in excludedDirs (VCS metadata, dependency installs, build
//     output, tooling caches);
//   - dotenv-shaped names (isDotenvShaped) — issue #420;
//   - archive artifacts (isArchiveArtifact) — issue #420.
//
// 🔴 THE RULE USED TO BE KEYED ON TYPE, AND THAT IS ISSUE #420 — the same defect
// as #409 with the shapes swapped. `.env.local` and `*.zip` were excluded as
// FILES only, so the same names as DIRECTORIES had their contents packaged. It
// is the credential-bearing direction: measured, a planted `.env.d/db.env`
// holding `API_SECRET=leak` was packaged into a bundle that is committed to
// Forgejo `civitai-apps/<slug>` and deployed, i.e. durably published.
//
// 🔴 THIS RULE IS STILL LOAD-BEARING, BUT NOT FOR THE REASON IT ORIGINALLY WAS.
// Until #435 the argument was that the file rule could not see `db.env`,
// `api.env` or `local.env` inside a `.env.d/` — those names do not START with ".env" — so the
// directory was the only thing stopping them. The `*.env` suffix rule now drops
// all three wherever they sit, so that argument is spent.
//
// What the directory rule still does, and nothing else does: it drops
// everything in a dotenv container REGARDLESS OF NAME. The file rules are name
// rules — they reach `.env.local` (prefix) and `db.env` (suffix) and nothing
// else. Measured by deleting isDotenvShaped from isExcludedDir and packaging a
// `.env.d/`: SIX files became bundle content, and only ONE of them was the
// allow-list case — read off TestBuildExcludesDotenvShapedDirectories' own
// failure output, so this list is the fixture's, not a recollection:
//
//	.env.d/credentials.json   .env.d/id_rsa    .env.local/token
//	.env.local/value          .env/value       .env.d/.env.production
//
// — i.e. the names a secrets directory actually holds are exactly the ones no
// file rule can see. `.env.d/db.env` stayed dropped, by the suffix rule.
//
// So: do NOT remove this as redundant with the suffix rule. The suffix rule
// covers a NAMING convention; this covers a CONTAINER, and a container's
// contents are named whatever its author liked.
// TestBuildExcludesDotenvShapedDirectories pins both the allow-list path and
// the arbitrary-name paths.
//
// 🔴 keptEnvFiles DOES NOT APPLY HERE. That allow-list exists because the server
// build's `vite build` READS `.env.production`, and because `.env.example` /
// `.env.sample` are templates a human reviewer reads. Those are claims about
// FILES; nothing reads a DIRECTORY named `.env.production`, so it is dotenv-
// shaped like any other and goes.
//
// 🔴 NOT A BLANKET PROMOTION OF isExcludedFile. Doing that would also drop a
// directory for every name isExcludedFile matches, and the mirror-image mistake
// is real in the other direction too: #418 records that promoting excludedDirs
// into the file rule wholesale would drop an extensionless `build` SCRIPT. Each
// rule is decided per name, in the direction its failure mode points.
func isExcludedDir(name string) bool {
	_, skip := excludeDirReason(name)
	return skip
}

// Rule tags name the PATTERN that dropped a path, for the `Skipped …` line
// `app submit` prints (#435). They are the author-facing half of drop-messaging:
// the tag is what says "rename it and it travels" rather than "this file is
// gone for a reason you must go read the source to learn".
//
// 🔴 A FIXED NAME GETS NO TAG, ON PURPOSE. `node_modules/`, `dist/`, `.git/` are
// self-explanatory and `app submit --help` already prints excludedDirs verbatim,
// so a tag there is noise on the line an author reads every submit. Only a
// PATTERN rule — where the author cannot tell from the name alone which rule
// reached it — carries one. RuleNonRegular is the exception that proves the
// shape: a symlink's name says nothing about why it was dropped either.
const (
	// RuleArchive is isArchiveArtifact's tag. One constant for both file and
	// directory because they are one predicate (see isArchiveArtifact).
	RuleArchive = "*.zip"
	// RuleDotenvPrefix is isExcludedFile's ".env"-prefix catch-all.
	RuleDotenvPrefix = ".env*"
	// RuleDotenvSuffix is isDotenvSuffixed, the #435 rule that costs a
	// Babylon.js `public/environment.env` its place in the bundle.
	RuleDotenvSuffix = "*.env"
	// RuleDotenvDir is isDotenvShaped — DIRECTORIES only, and deliberately
	// spelled out rather than reusing RuleDotenvPrefix: the directory rule
	// requires the dot, so `.envrc/` and `.env-backup/` are NOT dropped and a
	// `.env*` tag here would assert a rule that does not exist.
	RuleDotenvDir = ".env or .env.*"
	// RuleNonRegular is the walk's `!d.Type().IsRegular()` drop — symlinks,
	// sockets, fifos, devices. Not "symlink": the rule is wider than that, and
	// IsExcludedEntry's own doc records what naming only the common case cost.
	RuleNonRegular = "not a regular file"
)

// excludeDirReason is isExcludedDir with the matching rule kept, for #435's
// drop-messaging.
//
// 🔴 IT IS THE ONE COPY OF THE DIRECTORY RULE — isExcludedDir is a wrapper over
// it, not a sibling of it. A second copy of the matching ORDER alongside the
// original is exactly the drift that produced #409 and #420 (one shape of a name
// fixed, the other left behind), so the reason-returning form is the
// implementation and the bool is derived from it.
//
// An empty tag with true means "excluded, by a fixed name" — see the rule-tag
// block above for why those are deliberately untagged.
func excludeDirReason(name string) (string, bool) {
	if _, skip := excludedDirs[name]; skip {
		return "", true
	}
	if isDotenvShaped(name) {
		return RuleDotenvDir, true
	}
	if isArchiveArtifact(name) {
		return RuleArchive, true
	}
	return "", false
}

// excludedFilePatterns is the human-readable list of file-level exclusions, for
// CLI messaging. The authoritative matcher is isExcludedFile — keep the two in
// sync. These are matched on the file's BASE NAME anywhere in the tree. (The
// allow-list that carves three names out of them is NOT any-depth: see
// keptEnvFiles. Every pattern here holds at every depth; the exception to them
// does not.)
var excludedFilePatterns = []string{
	// Build artifacts. A `--package-only` run drops a *.zip in the project dir;
	// without this, the next package recursively sweeps that zip back in (a real
	// dogfood regression: 22 files/93 KB vs the correct 21 files/47 KB).
	"*.zip",
	// Dev-local / secret-bearing dotenv files. The page-money template points a
	// real VITE_LIVE_BLOCK_TOKEN at the git-ignored .env.development.local for
	// `dev:live`; these must never reach the server / reviewer / built image.
	".env",
	".env.local",
	".env.*.local", // e.g. .env.development.local, .env.production.local
	".env.development",
	".env.test",
	// 🔴 THE CATCH-ALL, AND THE REASON THE FIVE NAMES ABOVE ARE ILLUSTRATION
	// RATHER THAN THE RULE. isExcludedFile drops every base name starting with
	// ".env", dotted or not — `.envrc`, `.env-local`, `.env.staging`. Printing
	// only the enumeration let a reader conclude an unlisted name ships; it
	// does not. Keep this LAST so the message reads "these, and everything else
	// like them" — and keep the pair together: `*.env` below is the other end
	// of the same rule, so the two read as one statement about dotenv files.
	".env*",
	// 🔴 THE OTHER HALF OF THE CATCH-ALL (#435). Every rule above is a PREFIX
	// rule, so the shape tooling actually writes — `db.env`, `prod.env` — was
	// invisible to all of them and packaged at any depth. A reader who sees
	// only `.env*` concludes their `db.env` ships; it does not.
	"*.env",
}

// keptEnvFiles are .env* files we deliberately INCLUDE despite the broad
// ".env"-prefix exclusion below. They are allow-listed BY NAME, AT THE PROJECT
// ROOT ONLY:
//   - .env.example / .env.sample: templates meant to hold documented
//     placeholders a reviewer reads.
//   - .env.production: the server build runs `vite build` (mode=production),
//     which reads .env.production; the SCAFFOLDED file holds one public value,
//     VITE_BLOCK_ALLOWED_PARENT_ORIGINS.
//   - .env.production.local is NOT kept — `.local` is the dev-local override
//     convention and is caught by the `.env.*.local` rule above.
//
// 🔴 ROOT-SCOPED, BECAUSE EVERY JUSTIFICATION ABOVE IS ROOT-SPECIFIC. Vite reads
// env files from `envDir`, which defaults to the project root, so the file
// `vite build` consumes is the ROOT `.env.production` and nothing else; and a
// template a reviewer reads is the one at the root, not a copy buried in
// `.env-backup/` or `old/`. Applying the allow-list at any depth granted the
// exemption to files that can be NEITHER — measured on a binary built at
// 8617a68, a planted `.env-backup/.env.production` holding `API_SECRET=…` was
// packaged, and the bundle is committed to Forgejo `civitai-apps/<slug>` and
// read by a human moderator reviewer. `backups/.env.production` and
// `old/.env.production` went the same way. So at depth these three names fall to
// the catch-all like any other dotenv file.
//
// 🔴 THE DIRECTION OF FAILURE IS DELIBERATE, AND IT IS CHEAP NOW IN A WAY IT WAS
// NOT BEFORE. Dropping is the safe direction: an uploaded credential is durably
// published and cannot be recalled, while a dropped file is recovered by moving
// it to the root. And since the drop-messaging of #458 the packager PRINTS what
// it left out with the rule that matched — an author who loses a nested
// `.env.production` reads `Skipped 1 path(s): app/.env.production (.env*)` on
// that very run, rather than finding out at build time on the server. That line
// is what makes this trade affordable; without it the same change would have
// been a silent loss.
//
// 🔴 RESIDUAL, STATED RATHER THAN HIDDEN: any project whose build reads its
// dotenv from somewhere OTHER than the project root keeps its real build-input
// dotenv there, and this rule drops it.
//
// 🔴 AND THE POPULATION IS WIDER THAN "SETS ONE OF TWO VITE KEYS" (#465). An
// earlier wording scoped this to an app whose `vite.config` sets `root:` or
// `envDir:` to a subdirectory, which reads as "the scaffolds are safe, therefore
// essentially everyone is safe". They are not the same set. Vite's `envDir`
// defaults to `config.root`, which defaults to `process.cwd()` — the directory
// the build is INVOKED in, not "the project root" in the bundle sense. The
// page-money template makes that explicit by calling
// `loadEnv(mode, process.cwd(), …)`. So the affected set also includes:
//   - a package.json script that relocates the root — `vite build --root app`.
//     buildCommandRe (internal/validate/validate.go) admits
//     `npm|pnpm|yarn run <script>`, and the script BODY is arbitrary and read by
//     nothing here, so none of that is visible to this CLI.
//   - a workspace-filtered build, or any script that changes directory before
//     building — `pnpm --filter web build`, `npm --prefix web run build`,
//     `cd web && vite build` — reachable the same way.
//   - a non-Vite toolchain entirely (Next / Astro / webpack + dotenv), whose
//     `.env.production` resolution is likewise cwd-anchored.
//
// The shape is first-class in this repo's own contract rather than exotic:
// `outputDir` (schema/app-block.manifest.schema.json) permits a nested relative
// path — only a leading `/`, a `..` segment, a backslash and a Windows drive
// prefix are rejected — so `"outputDir": "packages/web/dist"` validates today.
// What is NOT known, and is not observable from this repo: how many live App
// Blocks projects actually build from a subdirectory. The schema permits it and
// the scaffolds do not produce it, which brackets the question without answering
// it.
//
// What the scaffolds do is still worth saying, but only as a statement about the
// scaffolds: neither `internal/scaffold/templates/page-vite/vite.config.js.tmpl`
// nor `internal/scaffold/templates/page-money/vite.config.ts.tmpl` sets `root:`
// or `envDir:`, and both are built from the project root, so anything
// `civitai app create` produces and leaves alone is unaffected.
//
// The loss is named in the Skipped line, and the author's fix is one flag or one
// move. Do not "fix" it by re-widening the allow-list: that restores the leak
// above for every project to buy a build-input for the few that relocate the
// build's working directory.
//
// 🔴 THIS IS A NAME ALLOW-LIST, NOT A SAFETY PROPERTY, AND NOTHING HERE READS
// THE CONTENTS. An earlier version of this comment (and of the README section it
// backs) argued that .env.production "by construction cannot hold a secret"
// because Vite inlines every VITE_-prefixed variable into the client bundle.
// That argument is wrong twice over: Vite inlines ONLY the VITE_-prefixed ones,
// so a plain `API_SECRET=…` in this file is not public and is still uploaded;
// and this repo itself treats a VITE_-prefixed value as a spending secret —
// VITE_LIVE_BLOCK_TOKEN is exactly why the dev-local dotenv files are on the
// exclusion list above. So do not re-derive a content guarantee from the prefix
// rule, and do not widen this list on the strength of one. What the CLI can
// honestly tell an author is WHERE the file goes; deciding what belongs in it is
// theirs.
var keptEnvFiles = map[string]struct{}{
	".env.example":    {},
	".env.sample":     {},
	".env.production": {},
}

// isExcludedFile reports whether a regular file (by base name) must be left out
// of the package. Four rules, in order:
//
//   - VCS metadata by EXACT name (vcsMetadataNames): a linked worktree's or a
//     submodule's `.git` is a regular file, and issue #409 is what happens when
//     only the directory shape is excluded. See that var.
//   - Build artifacts (*.zip).
//   - Dotenv by PREFIX. That rule is deliberately CONSERVATIVE toward "never
//     upload a secret": every base name starting with ".env" is dropped UNLESS
//     it is one of the three names on the keptEnvFiles allow-list AND it sits at
//     the PROJECT ROOT. Build-time config for the server must come from the
//     manifest / build recipe, not an uploaded dotenv.
//   - Dotenv by SUFFIX (#435): every base name ENDING in ".env" — `db.env`,
//     `prod.env`, `backup.db.env`. The shape tooling writes, invisible to every
//     prefix rule above. See isDotenvSuffixed for why it is files-only and what
//     it costs.
//
// 🔴 THE PREFIX IS ".env", NOT ".env." — UNDOTTED NAMES ARE EXCLUDED TOO. The
// match was `name == ".env" || HasPrefix(name, ".env.")` until the README
// started (correctly) telling authors "every file whose base name starts with
// `.env` is excluded". That left `.envrc` — the direnv convention, which
// routinely holds exported credentials — UPLOADED to the platform and to a human
// moderator reviewer, while the sentence an author reads said it was not. The
// prefix now matches the sentence. Widening cost nothing: no file the scaffold
// writes and no file tracked in this repo has a `.env`-prefixed base name that
// is not `.env.`-dotted (`.env.development` / `.env.production` /
// `.env.example` are all it emits). Residual, accepted: a base name that merely
// begins with those four bytes for an unrelated reason (`.envoy.yaml`) is
// dropped too. The failure mode of the other direction is a silent credential
// upload; this one is a missing file the author can rename.
//
// Note keptEnvFiles is an allow-list BY NAME, and it is scoped to the PROJECT
// ROOT — hence atRoot, which the caller must supply because a base name carries
// no depth. Nothing here reads a file's contents, so "kept" is a statement about
// WHERE the file goes, never a claim that it holds no secret — see the 🔴 note
// on keptEnvFiles.
//
// atRoot is true only for a file directly in the project directory. Passing true
// for a file at depth re-opens the leak keptEnvFiles' 🔴 note measures; passing
// false at the root drops the file `vite build` reads.
func isExcludedFile(name string, atRoot bool) bool {
	_, skip := excludeFileReason(name, atRoot)
	return skip
}

// excludeFileReason is isExcludedFile with the matching rule kept, for #435's
// drop-messaging. Same relationship as excludeDirReason to isExcludedDir: this
// is THE copy of the file rule and its ORDER, and the bool predicate is a
// wrapper over it. Read isExcludedFile's doc comment for the rules themselves —
// it is not repeated here, because two accounts of one order is the drift #409
// and #420 are made of.
//
// An empty tag with true means "excluded, by a fixed name" (vcsMetadataNames);
// see the rule-tag const block for why those carry no tag.
func excludeFileReason(name string, atRoot bool) (string, bool) {
	// VCS metadata that is a regular FILE, not a directory: a linked worktree's
	// or a submodule's `.git` (issue #409). Matched on the EXACT name, so
	// `.gitignore` / `.gitattributes` / `.gitmodules` still ship.
	if _, vcs := vcsMetadataNames[name]; vcs {
		return "", true
	}
	// Build artifacts. Same predicate the directory rule uses (isExcludedDir),
	// so the two shapes of one name cannot drift apart again — that drift is
	// #409 and #420.
	if isArchiveArtifact(name) {
		return RuleArchive, true
	}
	// Dotenv handling. Two rules, one allow-list:
	//
	//   - PREFIX (case-insensitive since #435): every base name starting with
	//     ".env" — `.env`, `.env.local`, `.envrc`, `.ENV.LOCAL`.
	//   - SUFFIX (#435): every base name ending in ".env" — `db.env`,
	//     `prod.env`. See isDotenvSuffixed for why this is files-only.
	//
	// 🔴 THE ALLOW-LIST IS CHECKED BY EXACT NAME, AND STAYS THAT WAY WHILE THE
	// MATCH ABOVE IS FOLDED. `vite build` reads `.env.production` by its exact
	// name, so `.ENV.PRODUCTION` is not the file the server build consumes and
	// keeping it would be a claim nothing supports. It therefore falls to the
	// catch-all and is DROPPED — the safe direction: the author sees a missing
	// file rather than an uploaded one. None of the three kept names ends in
	// ".env", so the suffix rule cannot reach them either.
	//
	// 🔴 AND BY POSITION: at the PROJECT ROOT ONLY. The allow-list exists for the
	// ROOT build input (`vite build`'s envDir defaults to the project root) and
	// the ROOT template a reviewer reads; a copy at depth can be neither, so
	// granting it the exemption uploaded `.env-backup/.env.production` verbatim.
	// At depth these names fall through to the prefix rule below and are dropped
	// with the `.env*` tag, which is what tells the author it happened. See
	// keptEnvFiles for the measurement and the accepted residual.
	if _, keep := keptEnvFiles[name]; keep && atRoot {
		return "", false
	}
	if strings.HasPrefix(strings.ToLower(name), ".env") {
		return RuleDotenvPrefix, true
	}
	if isDotenvSuffixed(name) {
		return RuleDotenvSuffix, true
	}
	return "", false
}

// Skip is ONE SKIP DECISION POINT in Build's walk — not one dropped file.
//
// 🔴 THE DISTINCTION IS THE WHOLE DESIGN (#435). When Build meets an excluded
// DIRECTORY it returns filepath.SkipDir, so the walk never descends and the
// number of files under it is never learned. A Skip for `node_modules/` is
// therefore honest about what happened (the walk stopped here) and says nothing
// about how many files that cost — because nothing measured it. Reporting a
// per-file count would mean walking a subtree the packager exists to avoid.
type Skip struct {
	// Path is project-relative with forward slashes, and carries NO trailing
	// slash — Dir is the machine-readable answer and the trailing "/" is a
	// rendering decision, kept out of the data so a consumer comparing paths
	// does not have to strip it.
	Path string
	// Dir reports whether the skipped entry was a directory, i.e. whether this
	// one decision truncated a whole subtree.
	Dir bool
	// Rule is the PATTERN that matched, e.g. "*.env" — empty when the entry was
	// dropped by a fixed name (excludedDirs / vcsMetadataNames). See the rule-tag
	// const block for why a fixed name is deliberately untagged.
	Rule string
}

// Result describes a produced package.
type Result struct {
	Files []string // archive-relative paths included, sorted
	// Skipped is every decision point at which Build left something out, in
	// render order: TAGGED entries first, then untagged, each group sorted by
	// path (see sortSkips). Nil when nothing was skipped.
	//
	// 🔴 IT IS DATA, NOT A MESSAGE. The rendering — the cap, the "…and K more",
	// the trailing slash — lives in exactly one place, the caller in
	// `internal/cmd` (renderSkippedLine). pkgzip does not know how wide a
	// terminal is and must not grow a second opinion about it.
	Skipped []Skip
	Bytes
}

// sortSkips puts the render order in ONE place: entries carrying a rule tag
// first, then the rest, each group sorted by path.
//
// TAGGED FIRST because the tag is the actionable half. `node_modules/` needs no
// explanation; `public/environment.env (*.env)` is the line that tells a
// Babylon.js author their environment texture was dropped by a dotenv rule and
// that renaming it recovers the file. Burying that under four fixed names an
// author already expects is how the message gets skimmed.
//
// Sorted WITHIN each group because determinism is the point of a line tests can
// pin: filepath.WalkDir is lexical today, but that is its behaviour, not its
// contract, and the tagged/untagged split reorders it anyway.
func sortSkips(skips []Skip) {
	sort.SliceStable(skips, func(i, j int) bool {
		ti, tj := skips[i].Rule != "", skips[j].Rule != ""
		if ti != tj {
			return ti
		}
		return skips[i].Path < skips[j].Path
	})
}

// Bytes carries the produced archive bytes and decompressed total.
type Bytes struct {
	Zip            []byte
	DecompressedBy int64
}

// Build packages dir into an in-memory ZIP. It requires block.manifest.json at
// the root and enforces the CLI's own caps — which are NOT the server's; see
// the const block for why clearing them is not a prediction that the submit
// will be accepted.
func Build(dir string) (*Result, error) {
	// Manifest must be at the root.
	if _, err := os.Stat(manifest.Path(dir)); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no %s at project root %s — nothing to submit", manifest.Filename, dir)
		}
		return nil, err
	}

	type entry struct {
		abs string
		rel string
	}
	var entries []entry
	// Every decision point at which this walk leaves something out (#435). The
	// caller prints it, so a drop stops being invisible until runtime.
	var skipped []Skip

	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		// Normalize to forward slashes for the archive (and for Skip.Path).
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if p == dir {
				return nil
			}
			if rule, skip := excludeDirReason(d.Name()); skip {
				// ONE entry for the whole subtree: SkipDir means the files
				// under it are never enumerated, so a per-file count here would
				// be invented rather than measured. See Skip's doc comment.
				skipped = append(skipped, Skip{Path: rel, Dir: true, Rule: rule})
				return filepath.SkipDir
			}
			return nil
		}
		// Skip non-regular files (symlinks, sockets, devices).
		if !d.Type().IsRegular() {
			skipped = append(skipped, Skip{Path: rel, Rule: RuleNonRegular})
			return nil
		}
		// Skip excluded files (build artifacts, secret-bearing dotenv files).
		//
		// rel is already project-relative and slash-normalised, so it holds a
		// separator exactly when the file sits below the project root — which is
		// the only depth information the keptEnvFiles allow-list needs.
		if rule, skip := excludeFileReason(d.Name(), !strings.Contains(rel, "/")); skip {
			skipped = append(skipped, Skip{Path: rel, Rule: rule})
			return nil
		}
		entries = append(entries, entry{abs: p, rel: rel})
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no files to package in %s", dir)
	}
	if len(entries) > MaxFiles {
		return nil, fmt.Errorf("package has %d files (the CLI's own cap is %d)", len(entries), MaxFiles)
	}

	// Deterministic ordering.
	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })
	sortSkips(skipped)

	var buf zipBuffer
	zw := zip.NewWriter(&buf)
	var (
		decompressed int64
		files        []string
	)
	for _, e := range entries {
		info, err := os.Stat(e.abs)
		if err != nil {
			return nil, err
		}
		if info.Size() > MaxFileSizeBytes {
			return nil, fmt.Errorf("%s is %d bytes (the CLI's own per-file cap is %d)", e.rel, info.Size(), MaxFileSizeBytes)
		}
		decompressed += info.Size()
		if decompressed > MaxDecompressedSize {
			return nil, fmt.Errorf("package decompressed size exceeds the CLI's own cap of %d bytes", MaxDecompressedSize)
		}

		w, err := zw.CreateHeader(&zip.FileHeader{
			Name:     e.rel,
			Method:   zip.Deflate,
			Modified: info.ModTime(),
		})
		if err != nil {
			return nil, err
		}
		f, err := os.Open(e.abs)
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(w, f); err != nil {
			f.Close()
			return nil, err
		}
		f.Close()
		files = append(files, e.rel)
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}

	if int64(len(buf.b)) > MaxBundleSizeBytes {
		// MaxBundleSizeBytes is this CLI's own cap, and the server's real
		// ceiling is LOWER and unknown here (#423). The wording this replaced
		// attributed the number to the platform, which told an author that
		// clearing this bar meant the server would take the bundle — the claim
		// #423 disproved. caps_claim_test.go keeps that attribution out.
		return nil, fmt.Errorf("package is %d bytes compressed (the CLI's own cap is %d; the server's own limit is lower and is not known to this CLI — see issue #423)", len(buf.b), MaxBundleSizeBytes)
	}

	return &Result{
		Files:   files,
		Skipped: skipped,
		Bytes:   Bytes{Zip: buf.b, DecompressedBy: decompressed},
	}, nil
}

// IsExcluded reports whether a DIRECTORY component would be excluded (exported
// for tests + messaging). See isExcludedDir for the rule — it is more than the
// excludedDirs map, because dotenv-shaped and archive-shaped directory names go
// too (#420).
//
// 🔴 NEITHER ExcludedNames NOR ExcludedFilePatterns REPORTS THE DIRECTORY
// PATTERN RULES. ExcludedNames lists the fixed excludedDirs entries;
// ExcludedFilePatterns lists the FILE patterns and is matched against a file's
// base name (see its own doc). So `JoinExcluded()`, which concatenates the two,
// is not a complete account of what a directory walk drops, and
// DirectoryPatternSummary exists to say the rest.
//
// (`app submit --help` no longer prints JoinExcluded() — it prints the two
// lists separately under their own headings, because a flat list of both reads
// as one rule and the help asserted the wrong shape for 15 of 18 entries. That
// leaves JoinExcluded with no production caller; it is kept as messaging API,
// not because anything currently renders it.)
//
// 🔴 WHAT IS AND IS NOT GATED, precisely, because the previous wording here
// overstated it and a maintainer would have trusted a gate that cannot see
// their change:
//   - A new FIXED name in excludedDirs needs no ledger: `app submit --help`
//     prints ExcludedNames() verbatim, so it documents itself. It does still turn
//     the help golden red, which is a deliberate re-approval step, not a gate
//     you have to design.
//   - A new PATTERN rule does NOT document itself. Add it to
//     DirectoryPatternSummary by hand. Two different guards then apply, and
//     neither is a behavioural pin on this prose:
//     TestDirectoryPatternSummaryIsTheReviewedCopy freezes the summary's exact
//     normalised text, so any reword must be re-read against the code;
//     TestDirectoryRuleIsDocumentedForAuthors pins a fixed ROSTER OF NAMES
//     against isExcludedDir, so it catches a rule that changes the answer for
//     one of those and cannot see a brand-new pattern matching a name nobody
//     listed. Add a roster row when you add a rule.
func IsExcluded(name string) bool { return isExcludedDir(name) }

// DirectoryPatternSummary is the human-readable account of the directory rules
// that are PATTERNS rather than fixed names (#420), for CLI messaging.
//
// It exists because the file patterns read as a complete list and are not one:
// a reader of `*.zip, .env, .env.local, …` reasonably concludes those are the
// rules, and would be wrong twice — those entries are matched against FILE base
// names, so they understate (nothing there says `.env.d/` is dropped WHOLE) and
// overstate (`.env*` reads as covering `.envrc/`, `.env-backup/` and `.envs/`
// as directories, and it does not).
//
// The wording is deliberately concrete about what still ships. This is the only
// exclusion documentation most authors will ever read, and the direction that
// costs them is believing a directory is dropped when it is uploaded.
// Line-wrapped to sit under the rest of the command's Long text, which is hard
// wrapped; TestDirectoryPatternSummaryIsTheReviewedCopy pins the whole string
// NORMALISED, so re-wrapping it is free but rewording it is not.
func DirectoryPatternSummary() string {
	return "a DIRECTORY named .env or .env.<anything> (e.g.\n" +
		"  .env.d/, .env.local/, .env.production/), or ending in .zip, is dropped\n" +
		"  whole at any depth; matching ignores case, so .ENV.D/ and x.ZIP/ go\n" +
		"  too. Directories whose names merely start with .env — .envrc/,\n" +
		"  .env-backup/, .envs/ — are NOT dropped, but the FILE rules still reach\n" +
		"  inside them: a db.env or prod.env there is dropped by the *.env rule,\n" +
		"  and so is a .env.production, because the allow-list applies at the\n" +
		"  PROJECT ROOT only. .env.example, .env.sample and .env.production are\n" +
		"  uploaded from the root and dropped everywhere else —\n" +
		"  .env-backup/.env.production, backups/.env.production and\n" +
		"  .env.d/.env.production all go."
}

// IsExcludedPath answers, for a whole project-relative PATH, the question Build
// answers one walk step at a time: would this path be left OUT of the bundle?
//
// It exists because the dirty-tree guard (issue #411, app_submit_dirty_guard.go)
// is handed paths by `git status`, not by a directory walk, and has to decide
// whether each one is bundle content. A path git reports that the packager drops
// — an untracked `dist/`, a stray `.zip` from a previous --package-only run, a
// `.env.local` — is not a difference between the bundle and HEAD, and refusing a
// submit over it would train everyone to pass --allow-dirty.
//
// 🔴 IT REPRODUCES Build'S DECISIONS RATHER THAN SHARING THEM, because Build
// walks a live filesystem and this takes a string. That is a seam, and a seam is
// where the two drift: adding an exclusion to Build without adding it here makes
// the guard fire on files that are not in the bundle. TestIsExcludedPathAgreesWithBuild
// pins the two against each other over one tree rather than trusting the
// reading, and it fails when either side gains an exclusion the other lacks.
//
// The rules, matching Build exactly:
//   - any DIRECTORY component is excluded by name (excludedDirs), and that
//     truncates the walk — everything under it is out.
//   - the final component, when it is a FILE, is excluded by isExcludedFile.
//     Build applies the directory rule only to directories, so a plain file
//     named `dist` is bundle content, and so it is here. The exception is
//     vcsMetadataNames (`.git`/`.hg`/`.svn`), which isExcludedFile drops too —
//     in a linked worktree or a submodule `.git` IS a regular file (#409), so
//     for those three names the answer is the same either way.
//   - that file rule is DEPTH-AWARE, because the keptEnvFiles allow-list is
//     scoped to the project root: `.env.production` is bundle content and
//     `app/.env.production` is not. The loop below already knows how many
//     directory components it walked past, and hands that to the file rule —
//     the same fact Build reads off its own `rel`.
//
// A trailing "/" marks the path as a DIRECTORY — git's porcelain spelling for an
// untracked directory (`?? dist/`) — so the final component takes the directory
// rule instead of the file rule.
//
// 🔴 A PATH CANNOT ANSWER THE FILE-TYPE HALF OF Build'S RULE. Build also drops
// every non-regular file, and no spelling of a path reveals that it is a
// symlink. A caller that HAS the filesystem must use IsExcludedEntry instead —
// this one answers "the packager keeps it" for a symlink the packager silently
// drops.
func IsExcludedPath(rel string) bool {
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "./")
	isDir := strings.HasSuffix(rel, "/")
	rel = strings.Trim(rel, "/")
	if rel == "" {
		return false
	}
	parts := strings.Split(rel, "/")
	// dirs counts the DIRECTORY components above the final one, so the file rule
	// below learns whether the file sits at the project root — the depth
	// keptEnvFiles is scoped to. Counted from the components actually consumed
	// rather than from len(parts), so "./" and empty segments cannot fake depth.
	dirs := 0
	for i, p := range parts {
		if p == "" || p == "." {
			continue
		}
		if i < len(parts)-1 || isDir {
			if IsExcluded(p) {
				return true
			}
			dirs++
			continue
		}
		if isExcludedFile(p, dirs == 0) {
			return true
		}
	}
	return false
}

// IsExcludedEntry answers IsExcludedPath's question for an entry whose FILE
// TYPE is known, and it is the predicate a caller holding a real filesystem
// should use.
//
// 🔴 IT EXISTS BECAUSE IsExcludedPath TAKES A STRING AND Build DOES NOT.
// Build's walk drops every non-regular file — symlinks, sockets, fifos, devices
// (pkgzip.go, "Skip non-regular files") — and a path alone cannot carry that.
// `git status` DOES report a symlink, so the dirty-tree guard (#411) asking
// IsExcludedPath about one was told "the packager bundles this" and refused a
// submit over a file that is in no zip. Measured against the built binary: a
// repo clean but for one untracked symlink `alias.js` printed
// "1 path(s) that go into the bundle are not committed: ?? alias.js" and exited
// 1, while the zip that run would have produced was
// [app.js block.manifest.json index.html] — the refusal named a path the bundle
// does not carry, so both the verdict and the sentence were wrong.
//
// mode is the entry's own mode as reported by Lstat/DirEntry.Type() — NOT the
// mode of a symlink's target, because Build does not follow the link either.
func IsExcludedEntry(rel string, mode fs.FileMode) bool {
	if mode.IsDir() {
		// Build applies the directory rule to directories; the trailing "/" is
		// how IsExcludedPath is told to do the same.
		if !strings.HasSuffix(rel, "/") {
			rel += "/"
		}
		return IsExcludedPath(rel)
	}
	if !mode.IsRegular() {
		return true
	}
	return IsExcludedPath(rel)
}

// ExcludedNames returns the excluded directory names, sorted.
func ExcludedNames() []string {
	out := make([]string, 0, len(excludedDirs))
	for k := range excludedDirs {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// IsExcludedFile reports whether a file with this base name, AT THE PROJECT
// ROOT, would be excluded (exported for tests + messaging). See isExcludedFile
// for the rule.
//
// 🔴 IT ANSWERS THE ROOT QUESTION, AND THAT IS NOT THE SAME QUESTION AT DEPTH.
// The keptEnvFiles allow-list is root-scoped, so this returns false for
// `.env.production` while the packager DROPS `app/.env.production`. A caller
// holding a path must ask IsExcludedPath (or IsExcludedEntry, which also judges
// file type) — those are the depth-aware forms and the ones Build agrees with.
// A base name alone cannot answer it: three names out of everything this rule
// matches depend on where the file sits.
//
// 🔴 IT HAS NO PRODUCTION CALLER, AND THAT IS THE POINT (#466). It had one —
// internal/antipattern's ScanDir, which walks by base name and so asked the root
// question at every depth. That was inert (the three names it could differ on
// end in `.example`, `.sample` and `.production`, none of which is in that
// package's scannedExts, so the extension gate skipped the file either way) but
// nothing pinned it, and the divergence was newly introduced by the root scope.
// That call now asks IsExcludedPath on the walk's own rel, so the two agree by
// construction rather than by coincidence of an extension table. What is left
// here is the base-name form the tests and the help messaging want: a caller
// that HAS a path must not use it, because for three names out of everything
// this rule matches, a base name is not enough to answer with.
func IsExcludedFile(name string) bool { return isExcludedFile(name, true) }

// KeptEnvFileNames returns the dotenv FILE names the packager deliberately
// uploads despite the ".env" catch-all, sorted.
//
// 🔴 EXPORTED SO THE HELP CAN SAY IT. ExcludedFilePatterns ends at `.env*`,
// which reads as "every dotenv file is dropped" — and `.env.production` being
// SHIPPED is, in the README's words, "the one worth knowing about, because it
// is the least expected". An author who reads only the exclusion list puts a
// token in the one file that is uploaded.
//
// These are allow-listed BY NAME AT THE PROJECT ROOT, so the exception does NOT
// cross a directory: `.env-backup/.env.production` is dropped like any other
// dotenv file, which is stated in DirectoryPatternSummary. Nothing reads their
// contents — see keptEnvFiles for why that is not a safety property, and for why
// the root scope is the whole justification rather than a detail of it.
func KeptEnvFileNames() []string {
	out := make([]string, 0, len(keptEnvFiles))
	for k := range keptEnvFiles {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ExcludedFilePatterns returns the human-readable file-exclusion patterns, in
// declaration order (build artifacts first, then dotenv).
func ExcludedFilePatterns() []string {
	out := make([]string, len(excludedFilePatterns))
	copy(out, excludedFilePatterns)
	return out
}

// zipBuffer is a tiny io.Writer collecting bytes (avoids bytes.Buffer's extra
// API surface; keeps the produced []byte addressable).
type zipBuffer struct{ b []byte }

func (z *zipBuffer) Write(p []byte) (int, error) {
	z.b = append(z.b, p...)
	return len(p), nil
}

// JoinExcluded is a small helper for messages: the excluded directory names
// followed by the file-level patterns (dotenv secrets + *.zip artifacts).
func JoinExcluded() string {
	return strings.Join(append(ExcludedNames(), ExcludedFilePatterns()...), ", ")
}
