package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

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
// bytes that are in no commit, and names them. It is not the provenance stamp
// the issue also asks for — that needs a server-side field on Submission
// (internal/appapi/appblocks.go carries no commit/tree/dirty column), so the CLI
// cannot send what the API will not accept, and #411 stays open for it.
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

// gitOutput is the real implementation, and the package var the submit command
// reads — tests swap it (or pass their own func to checkWorkTreeClean directly).
var gitOutput gitOutputFunc = func(dir string, args ...string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", errGitUnavailable
	}
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
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
//   - --allow-dirty: returns immediately, before any subprocess.
//   - no repo, or no `git` on PATH: proceed SILENTLY. The scaffold path, and the
//     one the issue says must not break.
//   - `git status` failed on a tree that IS a repo: WARN and proceed. This is an
//     accident preventer, not an authorization check.
//   - clean, HEAD on no remote: WARN and proceed.
func checkWorkTreeClean(git gitOutputFunc, warn io.Writer, dir, slug, version string, allowDirty bool) error {
	if allowDirty {
		return nil
	}
	if git == nil {
		return nil
	}

	// One call answers both "is this a work tree at all" and "where is it inside
	// the repo". A non-zero exit here is the no-repo case (git prints `fatal: not
	// a git repository`), and errGitUnavailable arrives the same way.
	//
	// `--is-inside-work-tree` prints `false` inside a bare repo or a .git
	// directory; neither has a working tree to be dirty, so both proceed.
	out, err := git(dir, "rev-parse", "--is-inside-work-tree", "--show-prefix")
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "true" {
		return nil
	}
	prefix := ""
	if len(lines) > 1 {
		prefix = strings.TrimSpace(lines[1])
	}

	// `-c status.relativePaths=false` pins the path spelling: `git status` shows
	// paths relative to the CWD when status.relativePaths is on, and a user CAN
	// turn it on in their config. Pinning it means the prefix arithmetic below is
	// arithmetic and not a guess about someone's ~/.gitconfig.
	//
	// `-z` rather than plain --porcelain: the default format C-quotes any path
	// with a space or a non-ASCII byte, so an author with `My Component.tsx`
	// would see a mangled path in a refusal message. -z emits raw bytes.
	//
	// `-- .` scopes the answer to the packaged subtree (see the note above).
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
		return nil
	}

	dirty := bundleDirtyPaths(raw, prefix)
	if len(dirty) > 0 {
		return dirtyWorkTreeError(slug, version, dirty)
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
	return nil
}

// gitStatusEntry is one porcelain record: its two status letters and the path.
type gitStatusEntry struct {
	code string
	path string
}

// bundleDirtyPaths parses `git status --porcelain -z` output and returns the
// entries that are BUNDLE content, with paths made relative to the packaged
// directory.
//
// The -z record is `XY<space><path>NUL`, and a rename/copy (`R`/`C` in either
// column) appends the ORIGINAL path as its own NUL-terminated field — verified
// against git 2.x rather than read off the man page, because getting it backwards
// silently turns one rename into two bogus entries whose second path is a file
// that no longer exists.
func bundleDirtyPaths(raw, prefix string) []gitStatusEntry {
	fields := strings.Split(raw, "\x00")
	var out []gitStatusEntry
	for i := 0; i < len(fields); i++ {
		rec := fields[i]
		// "XY " plus at least one path byte.
		if len(rec) < 4 {
			continue
		}
		code := rec[:2]
		path := rec[3:]
		if strings.ContainsAny(code, "RC") {
			i++ // consume the original path; it is the same change, not a second one
		}

		rel := path
		if prefix != "" {
			if !strings.HasPrefix(rel, prefix) {
				// Outside the packaged subtree. The pathspec should already have
				// excluded it; this is the belt to that braces.
				continue
			}
			rel = strings.TrimPrefix(rel, prefix)
		}
		if rel == "" || pkgzip.IsExcludedPath(rel) {
			continue
		}
		out = append(out, gitStatusEntry{code: code, path: rel})
	}
	return out
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
