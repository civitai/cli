// Package pkgzip packages an App Block project directory into the canonical
// ZIP the platform build recipe expects: the SOURCE tree (manifest + src +
// build config), with build artifacts and VCS/dependency dirs excluded.
package pkgzip

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/civitai/cli/internal/manifest"
)

// These caps mirror the server-side submitVersion service so a package that
// the CLI accepts will not be rejected on size grounds.
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

// Result describes a produced package.
type Result struct {
	Files []string // archive-relative paths included, sorted
	Bytes
}

// Bytes carries the produced archive bytes and decompressed total.
type Bytes struct {
	Zip            []byte
	DecompressedBy int64
}

// Build packages dir into an in-memory ZIP. It requires block.manifest.json at
// the root and enforces the server caps.
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

	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p == dir {
				return nil
			}
			if _, skip := excludedDirs[d.Name()]; skip {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip non-regular files (symlinks, sockets, devices).
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		// Normalize to forward slashes for the archive.
		rel = filepath.ToSlash(rel)
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
		return nil, fmt.Errorf("package has %d files (server max %d)", len(entries), MaxFiles)
	}

	// Deterministic ordering.
	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })

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
			return nil, fmt.Errorf("%s is %d bytes (server per-file max %d)", e.rel, info.Size(), MaxFileSizeBytes)
		}
		decompressed += info.Size()
		if decompressed > MaxDecompressedSize {
			return nil, fmt.Errorf("package decompressed size exceeds server max %d bytes", MaxDecompressedSize)
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
		return nil, fmt.Errorf("package is %d bytes compressed (server max %d)", len(buf.b), MaxBundleSizeBytes)
	}

	return &Result{
		Files: files,
		Bytes: Bytes{Zip: buf.b, DecompressedBy: decompressed},
	}, nil
}

// IsExcluded reports whether a path component would be excluded (exported for
// tests + messaging).
func IsExcluded(name string) bool {
	_, ok := excludedDirs[name]
	return ok
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

// zipBuffer is a tiny io.Writer collecting bytes (avoids bytes.Buffer's extra
// API surface; keeps the produced []byte addressable).
type zipBuffer struct{ b []byte }

func (z *zipBuffer) Write(p []byte) (int, error) {
	z.b = append(z.b, p...)
	return len(p), nil
}

// JoinExcluded is a small helper for messages.
func JoinExcluded() string { return strings.Join(ExcludedNames(), ", ") }
