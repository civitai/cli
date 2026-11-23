package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// outputDirError is the ONE wording of "the output directory could not be
// created", shared by the real run (downloadOne's MkdirAll) and the dry-run
// plan (checkOutputDir), so the two cannot drift. `dir` is user-typed: see the
// comment above downloadOne's MkdirAll for why it is not gated.
func outputDirError(dir string, err error) error {
	return fmt.Errorf("create output directory %s: %w", dir, err)
}

// checkOutputDir predicts, WITHOUT creating anything, whether the real run's
// os.MkdirAll(dir) would fail (civitai/cli#963), and returns the error the real
// run would return — the same wording, the same *fs.PathError shape — or nil.
//
// It walks up from dir to the nearest path that exists:
//   - an existing NON-directory there is a path component MkdirAll cannot
//     descend through ("mkdir f: not a directory");
//   - an existing directory with nothing missing below it needs no creation;
//   - otherwise MkdirAll would create the first missing component inside that
//     directory, so the directory must be writable and searchable
//     (dirCreatable; a read-only filesystem fails here too).
//
// It is a prediction, not a lock: the filesystem can change between the dry
// run and the real run. It does not check that the final file can be written
// into an EXISTING directory — the real run fails that at the ".part" create,
// after the transfer has started, which is outside what #963 covers.
func checkOutputDir(dir string) error {
	if dir == "" || dir == "." {
		return nil // matches downloadOne, which skips MkdirAll for these
	}
	p := filepath.Clean(dir)
	firstMissing := ""
	for {
		info, err := os.Stat(p)
		if err == nil {
			if !info.IsDir() {
				return outputDirError(dir, &fs.PathError{Op: "mkdir", Path: p, Err: syscall.ENOTDIR})
			}
			break
		}
		// ENOTDIR: some ancestor is a regular file. Keep walking up so the
		// error names that file, as MkdirAll's own recursion does.
		if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR) {
			return outputDirError(dir, err)
		}
		firstMissing = p
		parent := filepath.Dir(p)
		if parent == p {
			break // no existing ancestor at all (e.g. a missing volume)
		}
		p = parent
	}
	if firstMissing == "" {
		return nil // dir already exists
	}
	if err := dirCreatable(p); err != nil {
		return outputDirError(dir, &fs.PathError{Op: "mkdir", Path: firstMissing, Err: err})
	}
	return nil
}
