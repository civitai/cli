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
func resolveProjectDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return asUsageError(fmt.Errorf(
				"%s: no such directory — pass the path to an App project root, or scaffold one with `civitai app init <name>`", dir))
		}
		return fmt.Errorf("stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return asUsageError(fmt.Errorf(
			"%s is not a directory — pass the App project ROOT (the directory holding %s), not a file", dir, manifest.Filename))
	}
	return nil
}
