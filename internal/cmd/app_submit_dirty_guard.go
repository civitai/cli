package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/pkgzip"
	"github.com/civitai/cli/pkg/civitai"
)

// THE DIRTY-WORK-TREE GUARD (issue #411, the refusal half).
//
// `civitai app submit` packages whatever is on disk and records nothing about
// where it came from. A bundle built from a tree that was never committed
// submits exactly as cleanly as one built from a tagged release, and afterwards
// there is no way to tell which was which — five first-party apps were live at a
// version their source repo has never held when the issue was filed, and
// `custom-generators@0.5.2` exists in no commit anywhere.
//
// This guard is the PREVENTER: it refuses a submit whose bundle would carry
// bytes that are in no commit, and names them.
//
// 🔴 IT IS NOW ALSO THE FACT-GATHERER, AND THAT IS WHY IT RETURNS A VALUE. The
// issue's other half — the provenance STAMP — needed a server-side column that
// did not exist when the refusal shipped; civitai/civitai#4061 added it, so this
// function now answers WHICH commit and WHETHER dirty as well as pass/fail.
// There is exactly ONE git-invocation seam (gitOutputFunc) and it stays that
// way: a second function asking the same repo the same questions is how the two
// answers start disagreeing — the refusal naming a dirty path while the stamp
// claims `sourceDirty: false`.
//
// 🔴 IT MUST DEGRADE, NOT ENFORCE. A hard refusal on "there is no git repo here"
// would break the scaffold path outright: `civitai app scaffold` produces a
// directory with no repo, and submitting one is completely ordinary. The full
// matrix, from the issue:
//
//	no repo (or no git binary) → proceed, silently
//	repo, clean               → proceed
//	repo, dirty               → REFUSE, naming the paths, --allow-dirty in the text
//	repo, clean, HEAD unpushed → WARN, proceed
//
// The last row is a warning rather than a refusal on purpose: submitting before
// pushing is legitimate, and the thing lost is traceability, not correctness.

// ErrDirtyWorkTree classifies the dirty-tree refusal. Like ErrVersionRegression
// it carries no user-facing text of its own — it is ATTACHED to the
// message-bearing error, so errors.Is reports the KIND while the printed message
// is unchanged.
//
// 🔴 EXIT CODE 1, THE SAME VERDICT #412's REFUSAL TAKES, AND FOR THE SAME
// REASON. Exit 2 is documented as a mistake about the INVOCATION, and every
// flag, argument and path is well-formed when this fires: `civitai app submit
// --yes` in a directory that exists, holding a manifest that validates. What is
// wrong is the PROJECT — its working tree relative to its own history — which is
// the shape exitCodeDocs already publishes under code 1 as a validation verdict.
// Leaving it untagged for the exit mapper's `default` is what produces 1;
// TestDirtyWorkTreeExitsGeneric (cmd/civitai) pins it so the code cannot drift.
var ErrDirtyWorkTree = errors.New("the packaged directory has uncommitted changes")

// errGitUnavailable is what the real runner returns when there is no `git` on
// PATH. It is a DISTINCT error only so the runner does not have to fake an exit
// status; the guard treats it identically to "not a repo", because those two
// states are indistinguishable from the user's point of view and both mean the
// same thing operationally: there is no history here to compare against.
var errGitUnavailable = errors.New("git is not installed or not on PATH")

// gitOutputFunc runs a git subcommand in dir and returns its STDOUT.
//
// It is a second seam beside app_pull.go's gitRunner rather than a reuse of it:
// that one streams git's output straight to the user's terminal and returns only
// an error, which is right for `clone`/`fetch` progress and useless for a
// question whose whole answer is on stdout.
type gitOutputFunc func(dir string, args ...string) (string, error)

// gitEnvOverrides are the environment variables that RELOCATE git's idea of
// which repository it is looking at. Every one of them BEATS `-C`, so leaving
// them in place makes the "which directory's repo" decision above a lie.
//
// 🔴 THIS IS NOT HYPOTHETICAL: git EXPORTS GIT_DIR and GIT_INDEX_FILE to its
// hooks. A release script that runs `civitai app submit` from a `pre-push` /
// `post-commit` hook, or under `git rebase -x`, therefore inherits a GIT_DIR
// pointing at the repo that invoked the hook. Measured on git 2.55.0: a dirty
// app directory exits 1 with the refusal, and the SAME invocation with GIT_DIR
// and GIT_WORK_TREE aimed at an unrelated clean repo exits 5 with zero
// refusals. It is the "gitOutput dropped its -C" defect reached through the
// environment instead of argv.
//
// 🔴 THEY MUST BE REMOVED FROM THE ENVIRONMENT, NEVER SET TO "". git does not
// read an empty value as unset: `GIT_DIR= git status` fails with
// `fatal: not a git repository` naming the empty string (measured, git 2.55.0),
// which this guard
// classifies as "no repo here" and proceeds — so the obvious one-line fix,
// appending `GIT_DIR=` to os.Environ(), would fail the guard OPEN on EVERY
// invocation while looking exactly like a hardening change.
var gitEnvOverrides = []string{
	"GIT_DIR",
	"GIT_COMMON_DIR",
	"GIT_WORK_TREE",
	"GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_CEILING_DIRECTORIES",
}

// gitScrubbedEnv drops gitEnvOverrides from env, leaving everything else — in
// particular the user's git config, which this guard deliberately does NOT
// neutralise: `status.renames`, `core.excludesFile` and friends are the user's
// answer to what their own tree looks like.
func gitScrubbedEnv(env []string) []string {
	drop := make(map[string]struct{}, len(gitEnvOverrides))
	for _, k := range gitEnvOverrides {
		drop[k] = struct{}{}
	}
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, ok := strings.Cut(kv, "=")
		if ok {
			if _, skip := drop[name]; skip {
				continue
			}
		}
		out = append(out, kv)
	}
	return out
}

// gitOutput is the real implementation, and the package var the submit command
// reads — tests swap it (or pass their own func to checkWorkTreeClean directly).
//
// `--no-optional-locks` is what makes "every call here is read-only" true rather
// than nearly true: `git status` otherwise takes the index lock to write back a
// refreshed stat cache, which is a write to someone else's repository from a
// command that only asked a question — and can fail on a tree another git
// process holds.
var gitOutput gitOutputFunc = func(dir string, args ...string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", errGitUnavailable
	}
	c := exec.Command("git", append([]string{"-C", dir, "--no-optional-locks"}, args...)...)
	c.Env = gitScrubbedEnv(os.Environ())
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

// dirtyPathListCap bounds how many paths the refusal prints. A tree with 400
// uncommitted files should say so and show a sample, not emit 400 lines into a
// terminal the reader then has to scroll back through to find the fix.
const dirtyPathListCap = 10

// checkWorkTreeClean refuses a submit whose bundle would contain uncommitted
// content, and warns when the committed content it is built from is on no
// remote.
//
// 🔴 WHICH DIRECTORY'S REPO: `dir`, THE TREE BEING PACKAGED — never the process
// cwd. `app submit [dir]` takes an optional positional, so a release script that
// runs `civitai app submit ./packages/my-block` from the monorepo root has a
// manifest directory and a cwd that are different places, and the dirtiness
// question is about the bytes that go into the zip. Every git call here goes
// through gitOutputFunc, whose only contract is that it runs `git -C dir` — a
// guard that inspected the cwd's repo would answer confidently about a tree
// nobody is submitting, which is worse than no guard at all. Pinned by
// TestDirtyGuardInspectsThePackagedDirNotTheCwd.
//
// The pathspec is scoped for the same reason: `dir` may be a SUBDIRECTORY of the
// repo, and a sibling package's uncommitted work is not in this bundle.
//
// 🔴 WHAT COUNTS AS DIRTY: a path is dirty when git reports it as differing from
// HEAD — staged, unstaged, or untracked — AND the packager would put it in the
// bundle. That is one rule with one consequence: the guard fires exactly when
// the bundle contains bytes that are in no commit.
//
// The two filters that rule implies, and neither is a special case:
//   - Files git IGNORES never appear, because --porcelain without --ignored does
//     not list them. Not a decision made here; it is what "the repo does not
//     track this" already means.
//   - Files the PACKAGER excludes are dropped (pkgzip.IsExcludedPath). An
//     untracked `dist/`, `node_modules/`, a `.env.local`, or the `*.zip` a
//     previous `--package-only` run left in the directory are not differences
//     between the bundle and HEAD — the bundle does not contain them. Refusing
//     over those would fire on nearly every real project and teach everyone to
//     pass --allow-dirty by reflex, which is the failure mode that makes a guard
//     worth less than nothing.
//
// Deliberately IN scope, because the bundle really does carry them: a modified
// submodule (Build walks the filesystem and excludes only `.git`, so submodule
// CONTENT is packaged), and an untracked editor scratch file the packager does
// not exclude — `notes.md`, `App.tsx.orig`, a `.swp` — which genuinely ships.
//
// Every non-refusal branch:
//   - --allow-dirty: never refuses and never warns, but still gathers the facts.
//   - no repo, or no `git` on PATH: proceed SILENTLY. The scaffold path, and the
//     one the issue says must not break.
//   - `git status` failed on a tree that IS a repo: WARN and proceed. This is an
//     accident preventer, not an authorization check.
//   - clean, HEAD on no remote: WARN and proceed.
//
// 🔴 IT ALSO RETURNS THE PROVENANCE (#411's stamp half), AND THE MATRIX IS THE
// SAME ONE THE REFUSAL DEGRADES THROUGH — every branch that cannot establish a
// fact reports it as UNKNOWN rather than as a default:
//
//	no repo / no git / bare repo / inside .git → Provenance{} (send nothing)
//	unborn HEAD (rev-parse HEAD fails)         → Provenance{} (send nothing)
//	`git status` failed                        → commit only, Dirty nil
//	clean                                      → commit + Dirty=false
//	dirty + --allow-dirty                      → commit + Dirty=TRUE
//
// 🔴 THE LAST ROW IS THE ONE WITH THE MOST DIAGNOSTIC VALUE, so --allow-dirty no
// longer short-circuits before the subprocesses. It used to return before making
// a single git call, on the reasoning that a release script which has already
// decided should not pay for an answer it discards — but that answer is no
// longer discarded: `--allow-dirty` is exactly the invocation that produces a
// bundle carrying bytes in no commit, which is the state #411 was filed about,
// and refusing to record it is refusing to record the only case anyone will need
// to explain later. WHAT DID NOT CHANGE, and is the published contract of #415:
// the flag still cannot refuse, and it still prints nothing. Both are enforced
// here by construction — the warn writer is discarded and the dirty branch
// returns a nil error — rather than by remembering to check the flag twice.
func checkWorkTreeClean(git gitOutputFunc, warn io.Writer, dir, slug, version string, allowDirty bool) (appapi.Provenance, error) {
	var prov appapi.Provenance
	if git == nil {
		return prov, nil
	}
	if allowDirty {
		// The flag's whole meaning: no verdict, no advisories. Facts only.
		warn = io.Discard
	}

	// One call answers both "is this a work tree at all" and "where is it inside
	// the repo". A non-zero exit here is the no-repo case (git prints `fatal: not
	// a git repository`), and errGitUnavailable arrives the same way.
	//
	// `--is-inside-work-tree` prints `false` inside a bare repo or a .git
	// directory; neither has a working tree to be dirty, so both proceed.
	out, err := git(dir, "rev-parse", "--is-inside-work-tree", "--show-prefix")
	if err != nil {
		return prov, nil
	}
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "true" {
		return prov, nil
	}
	prefix := ""
	if len(lines) > 1 {
		// 🔴 TRIM THE LINE ENDING, NOT WHITESPACE. `--show-prefix` prints a path,
		// and a path may legitimately BEGIN with a space: a package directory
		// named ` spacey/app` yields the prefix " spacey/app/", which TrimSpace
		// silently turns into "spacey/app/". Every real status path then fails the
		// HasPrefix test in bundleDirtyPaths and is dropped — the guard reports
		// CLEAN on a tree with uncommitted bundle files. Reproduced on git 2.55.0.
		// Only \r can be here at all: \r\n was normalised above and Split ate the
		// \n, so a lone \r from a CRLF-ish environment is the whole risk.
		prefix = strings.TrimSuffix(lines[1], "\r")
	}

	// The commit half of the provenance, asked through the SAME seam. A failure
	// is the unborn-HEAD case (a repo with no commits, which `app init` +
	// `git init` produces before the first commit) and every other reason git
	// cannot name HEAD; all of them mean "there is no commit to claim", which is
	// UNKNOWN and never a default. Whatever comes back is trimmed and nothing
	// else: the shape gate is appapi.Provenance.sanitised, one place, so this
	// function cannot disagree with the wire about what is sendable.
	prov.Commit = headCommit(git, dir)

	// `-c status.relativePaths=false` pins the path spelling AS FUTURE-PROOFING,
	// and the honest scope of that is narrower than it used to read here.
	// MEASURED on git 2.55.0: `--porcelain` ignores status.relativePaths
	// entirely — unset, `true` and `false` produce byte-identical output — so the
	// flag buys nothing TODAY and removing it is an equivalent mutation. It stays
	// because the prefix arithmetic below depends on repo-root-relative paths and
	// nothing else in this file would notice if a future git honoured the setting.
	// Do not restate it as a live hazard; do not delete it as dead weight.
	//
	// `-z` rather than plain --porcelain: the default format C-quotes any path
	// with a space or a non-ASCII byte, so an author with `My Component.tsx`
	// would see a mangled path in a refusal message. -z emits raw bytes.
	//
	// 🔴 `-- .` SCOPES THE ANSWER TO THE PACKAGED SUBTREE, AND IT IS A
	// CORRECTNESS FLAG, NOT A COST ONE. An earlier version of this comment (and
	// of the test that recorded it) claimed the pathspec and the prefix filter in
	// bundleDirtyPaths "select the same set — proven, measured". THAT CLAIM WAS
	// FALSE. Without the pathspec, git's rename detection runs over the WHOLE
	// repo and can pair a subtree file with a destination outside it. Measured on
	// git 2.55.0, after `git mv packages/my-block/src/App.tsx packages/other/App.tsx`
	// with the packaged dir at packages/my-block:
	//
	//	WITH `-- .` :  [D  packages/my-block/src/App.tsx]
	//	WITHOUT     :  [R  packages/other/App.tsx][packages/my-block/src/App.tsx]
	//
	// The second spelling reports a path OUTSIDE the subtree, which the prefix
	// filter drops — so the guard called a tree clean while the bundle had lost a
	// file that is in HEAD. What made that survivable was that no fixture crossed
	// the subtree boundary; TestDirtyGuardRefusesARenameOutOfThePackagedSubtree
	// now does.
	//
	// 🔴 That hole is closed TWICE over as of the same change: bundleDirtyPaths
	// now also judges a rename's ORIGINAL path, which catches the boundary
	// crossing even without the pathspec (see its own note). So a mutation
	// battery will report REMOVING THE PATHSPEC AS A SURVIVOR — measured, and
	// expected. That is redundancy, not deadness: the pathspec is the only thing
	// bounding the `--untracked-files=all` walk in a monorepo, and it is the
	// mechanism that does not depend on getting rename records right. Keep both.
	//
	// 🔴 `--untracked-files=all`, NOT the default `normal`, and it is a
	// CORRECTNESS choice rather than a verbosity one. `normal` COLLAPSES an
	// untracked directory to a single `src/` entry, which breaks this guard in
	// both directions: the refusal names `src/` instead of the file the author
	// has to go and commit, and the packager-exclusion filter below is handed a
	// directory name when the decision belongs to the files inside it — an
	// untracked directory holding nothing but a stray `.zip` would be refused
	// over content the bundle does not carry. The cost is enumerating a large
	// untracked tree (a `node_modules` that is somehow not gitignored); the
	// verdict is the same either way, and precision on the path that IS reported
	// is worth more than that.
	raw, err := git(dir, "-c", "status.relativePaths=false", "status", "--porcelain", "-z", "--untracked-files=all", "--", ".")
	if err != nil {
		warnf(warn, "could not check whether %s is a clean git work tree (%v) — submitting without the dirty-tree guard.", dir, err)
		// The commit is still known; the dirtiness is not. Provenance.Dirty
		// stays nil — UNKNOWN — rather than defaulting to the reassuring answer
		// for a question this run failed to ask.
		return prov, nil
	}

	dirty := bundleDirtyPaths(raw, prefix, dir)
	isDirty := len(dirty) > 0
	prov.Dirty = &isDirty
	if isDirty {
		if allowDirty {
			// Facts recorded, verdict withheld — see the 🔴 note above.
			return prov, nil
		}
		return prov, dirtyWorkTreeError(slug, version, dirty)
	}

	// Clean. The remaining question is whether the commit it is clean AGAINST
	// exists anywhere but this machine — a warning, never a refusal: submitting
	// before pushing is legitimate, and what is lost is traceability.
	//
	// A failure here (an unborn HEAD in a repo with no commits, an ancient git)
	// is silent: it is a question about a nicety, and a warning nobody can act on
	// is noise on the happy path.
	remotes, err := git(dir, "for-each-ref", "--count=1", "--contains", "HEAD", "--format=%(refname:short)", "refs/remotes")
	if err == nil && strings.TrimSpace(remotes) == "" {
		warnf(warn, "the work tree is clean, but HEAD is on no remote — this bundle's source exists only on this machine. "+
			"Push the branch so the submitted version can be traced back to a commit.")
	}
	return prov, nil
}

// headCommit asks the one git seam for HEAD's sha, and returns "" for every
// reason it cannot be established.
//
// 🔴 IT TRIMS AND JUDGES NOTHING ELSE. `git rev-parse HEAD` emits a lowercase
// 40-hex sha and a trailing newline, so the trim is the whole of the normalising
// this layer is entitled to do: a value that arrives in any other shape is a
// signal that this is not the answer we asked for, and appapi's
// `^[0-9a-f]{40}$` gate drops it. Uppercasing, truncating or lower-casing it
// here would turn a value we do not understand into one that LOOKS right on the
// wire, which is the only way this feature could make a submit worse.
func headCommit(git gitOutputFunc, dir string) string {
	out, err := git(dir, "rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// gitStatusEntry is one porcelain record: its two status letters and the path.
type gitStatusEntry struct {
	code string
	path string
}

// bundleDirtyPaths parses `git status --porcelain -z` output and returns the
// entries that are BUNDLE content, with paths made relative to the packaged
// directory. dir is the packaged directory itself, used to ask the filesystem
// what TYPE an entry is; pass "" to judge by path alone (the literal-record
// tests do).
//
// The -z record is `XY<space><path>NUL`, and a rename/copy (`R`/`C` in either
// column) appends the ORIGINAL path as its own NUL-terminated field — verified
// against git 2.x rather than read off the man page, because getting it backwards
// silently turns one rename into two bogus entries whose second path is a file
// that no longer exists.
//
// 🔴 A RENAME HAS TWO SIDES AND BOTH ARE BUNDLE DIFFERENCES. Judging only the
// destination made two real fail-opens, because a rename REMOVES bytes at the
// original path while the destination decides the verdict. Measured on git
// 2.55.0:
//
//	git mv src/App.tsx dist/App.tsx  →  [R  dist/App.tsx][src/App.tsx]
//
// `dist/App.tsx` is packager-excluded, so the entry was dropped and the guard
// reported CLEAN — while `src/App.tsx` is in HEAD and is no longer in the
// bundle. The same shape crossing the packaged subtree (`git mv
// packages/my-block/src/App.tsx packages/other/App.tsx`) failed open the same
// way through the prefix filter. So an `R` contributes BOTH paths.
//
// 🔴 A COPY CONTRIBUTES ONLY THE DESTINATION, and that is not symmetry for its
// own sake. A copy leaves the original exactly as HEAD has it, so the original
// is not a difference — and git's status copy detection only pairs against a
// source that is ITSELF modified, in which case git emits a separate record for
// it. Measured on git 2.55.0 with status.renames=copies:
//
//	[C  src/copy.html][src/index.html][M  src/index.html]
//
// Adding the source from the `C` record would therefore either double-count a
// path git already reported (an inflated "N path(s)" count) or, in the case git
// does not produce today, refuse over a file identical to HEAD. Entries are
// de-duplicated by path for the same reason.
func bundleDirtyPaths(raw, prefix, dir string) []gitStatusEntry {
	fields := strings.Split(raw, "\x00")
	var out []gitStatusEntry
	seen := map[string]struct{}{}

	add := func(code, path string) {
		rel := path
		if prefix != "" {
			if !strings.HasPrefix(rel, prefix) {
				// Outside the packaged subtree — a sibling package's work is not
				// in this bundle. For a rename ORIGINAL this is also the arm that
				// the `-- .` pathspec makes unreachable in practice.
				return
			}
			rel = strings.TrimPrefix(rel, prefix)
		}
		if rel == "" || pkgzip.IsExcludedPath(rel) {
			return
		}
		if !bundlesOnDisk(dir, code, rel) {
			return
		}
		if _, dup := seen[rel]; dup {
			return
		}
		seen[rel] = struct{}{}
		out = append(out, gitStatusEntry{code: code, path: rel})
	}

	for i := 0; i < len(fields); i++ {
		rec := fields[i]
		// "XY " plus at least one path byte.
		if len(rec) < 4 {
			continue
		}
		code := rec[:2]
		path := rec[3:]
		orig := ""
		if strings.ContainsAny(code, "RC") {
			// Consume the original path; it is the same change, not a second one.
			i++
			if i < len(fields) {
				orig = fields[i]
			}
		}
		add(code, path)
		if strings.Contains(code, "R") && orig != "" {
			add(code, orig)
		}
	}
	return out
}

// bundlesOnDisk is the FILE-TYPE half of "would the packager put this in the
// bundle", which pkgzip.IsExcludedPath cannot answer from a string.
//
// 🔴 IT APPLIES ONLY TO PATHS THAT ARE IN NO COMMIT — untracked (`??`) or
// staged-new (`A…`). For those, "the packager drops it" really does mean "not a
// difference between the bundle and HEAD": an untracked symlink ships nowhere,
// and refusing over it is a false refusal whose message ("path(s) that go into
// the bundle") is factually wrong. For anything git reports as tracked the
// opposite holds — a regular file replaced by a symlink (` T`) makes the bundle
// LOSE content that is in HEAD, which is exactly the difference this guard
// exists to catch, so the type check must not silence it.
//
// dir == "" means "no filesystem to ask" (the literal-record tests): judge by
// path alone, which is what the caller already did.
func bundlesOnDisk(dir, code, rel string) bool {
	if dir == "" || !absentFromHEAD(code) {
		return true
	}
	info, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		// Gone, or unreadable. Not something to fail open over.
		return true
	}
	return !pkgzip.IsExcludedEntry(rel, info.Mode())
}

// absentFromHEAD reports whether the porcelain code says this path exists in no
// commit: `??` (untracked) or an `A` in the index column (added, incl. the `AA`
// both-added conflict). Every other code implies HEAD has the path.
func absentFromHEAD(code string) bool {
	return strings.HasPrefix(code, "?") || strings.HasPrefix(code, "A")
}

// dirtyWorkTreeError builds the refusal.
//
// It states the CONSEQUENCE rather than the rule — "approving this deploys code
// that exists in no commit" is the thing that actually went wrong for five apps,
// and "your work tree is dirty" is a fact the author can already see. The escape
// hatch is named on its own line because the reader of this message is, roughly
// always, someone who now has to decide between committing and overriding.
func dirtyWorkTreeError(slug, version string, dirty []gitStatusEntry) error {
	var b strings.Builder
	name := strings.TrimSpace(slug)
	if name == "" {
		name = "this app"
	} else {
		name += "@" + version
	}
	fmt.Fprintf(&b, "refusing to submit %s from a dirty git work tree — %d path(s) that go into the bundle are not committed:\n",
		name, len(dirty))
	shown := dirty
	if len(shown) > dirtyPathListCap {
		shown = shown[:dirtyPathListCap]
	}
	for _, e := range shown {
		fmt.Fprintf(&b, "  %s %s\n", e.code, e.path)
	}
	if len(dirty) > len(shown) {
		fmt.Fprintf(&b, "  … and %d more\n", len(dirty)-len(shown))
	}
	b.WriteString("The bundle is packaged from what is on disk, so approving this would deploy code that exists in no commit and cannot be traced back to one.\n")
	b.WriteString("Commit them, or pass --allow-dirty to submit the working tree exactly as it is")
	return civitai.Tag(ErrDirtyWorkTree, errors.New(b.String()))
}
