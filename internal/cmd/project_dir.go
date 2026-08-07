package cmd

import (
	"fmt"
	"os"

	"github.com/civitai/cli/internal/manifest"
)

// resolveProjectDir classifies the path the user NAMED as an App project root,
// before any validation runs. It is the one gate `app validate` and `app submit`
// share; both take the same optional positional `[dir]`.
//
// The full rationale, the measured mutants, and the residual the published
// contract knowingly ships with are AGENTS.md item 25 — read it before changing
// any branch here. (It is deliberately NOT item 24: that is the
// transport-vs-filesystem predicate, a different rule on the same contract.)
//
// 🔴 IT STATS THE PATH THE USER TYPED — NEVER the joined manifest path, and that
// is the whole fix (issue #256). `validate.Dir` stats `<dir>/block.manifest.json`
// and branches on os.IsNotExist, which collapses two different mistakes into one
// verdict:
//
//   - `civitai app validate /nope` reported "block.manifest.json not found at
//     project root /nope" and exited 1 — a validation FINDING about a directory
//     that is not there. The published contract says a path that does not exist
//     is a mistake about the invocation and exits 2 (see exitCodeDocs code 2,
//     and readGraphInput in generate_input.go, which already honours it for
//     `generate --input`).
//   - `civitai app validate README.md` fell through to the raw syscall and
//     printed `stat README.md/block.manifest.json: not a directory` — a path the
//     user never named, assembled by us, on exit 1.
//
// So the branch is three-way on the NAMED path:
//
//	does not exist        -> ErrUsage (exit 2)
//	exists, not a dir     -> ErrUsage (exit 2)
//	a directory           -> nil; validate.Dir decides, and a directory with no
//	                         manifest keeps its existing finding and exit 1.
//
// That last row is deliberate and must stay: "this directory is not an App
// project" is a validation verdict about a real place the user pointed at, not a
// malformed invocation. Do not tag it.
//
// 🔴 IT LIVES IN internal/cmd, NOT internal/validate. ErrUsage is this package's
// sentinel (item 7), and `validate.Dir` returns a validation VERDICT — a Result
// — not an opinion about the command line. Pushing the tag down there would make
// internal/validate import the usage sentinel and would leak the exit-code
// contract into a package that has no business holding it. It also must not move
// into validate.ManifestOnly's path: `app init` self-checks a directory it just
// created, and has no user-named path to classify.
//
// Any OTHER stat failure (EACCES on a parent, ENOTDIR partway down a longer
// path) is returned UNTAGGED, so it exits 1 — the generic/filesystem code item
// 24 already assigns to exactly those shapes. `app validate <regular-file>/x.json`
// is one of the six invocations measured in #241, and 1 is the answer that issue
// settled on.
// The two exit-2 arms carry DIFFERENT remedies, and which arm gets which is
// part of the contract rather than cosmetic: "the path is not there" sends you
// to `app init`, while "you pointed at a file" sends you to the parent
// directory. Swapped, the CLI tells someone whose path is simply missing to
// pass "the ROOT, not a file" — advice about a file that does not exist — and
// tells someone who pointed at their manifest to scaffold a project they
// already have.
//
// 🔴 They are named constants because a swap is otherwise INVISIBLE: measured
// on this branch, exchanging the two format strings passed the ENTIRE suite
// (0 failures). Every classification assertion uses errors.Is and both arms
// carry the same ErrUsage sentinel, so nothing downstream can tell them apart,
// and the one message test only asks that the path the user typed appears —
// which both spellings do. That is AGENTS item 21(f)'s recorded shape: the
// operand ORDER of a message is a contract no exit-code assertion can see.
// TestProjectDirRemediesMatchTheirArm requires each arm to carry its own AND
// NOT the other's, and pins that the two are non-empty and distinct — an empty
// or duplicated remedy would make `strings.Contains` vacuously true and disarm
// the whole guard.
const (
	remedyNoSuchDir = "%s: no such directory — pass the path to an App project root, or scaffold one with `civitai app init <name>`"
	remedyNotADir   = "%s is not a directory — pass the App project ROOT (the directory holding %s), not a file"
)

func resolveProjectDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return asUsageError(fmt.Errorf(remedyNoSuchDir, dir))
		}
		// Returned BARE, and that is not laziness. os.Stat's error is an
		// *fs.PathError whose Error() already reads `stat <path>: <reason>`, so
		// a `fmt.Errorf("stat %s: %w", dir, err)` wrapper printed the op and the
		// path TWICE — measured on this branch before the fix:
		// `Error: stat …/file.txt/x.json: stat …/file.txt/x.json: not a
		// directory`, where the base binary printed one `stat`. There is no
		// context left to add: we stat the path the USER typed, never the joined
		// manifest path, so the PathError already names the right thing.
		//
		// Do not re-add a prefix; if you ever need one, it must not repeat `stat`
		// or the path. The classification is unchanged either way — untagged is
		// exit 1, per the comment above.
		return err
	}
	if !info.IsDir() {
		return asUsageError(fmt.Errorf(remedyNotADir, dir, manifest.Filename))
	}
	return nil
}
