// Package pkgzip packages an App project directory into the canonical
// ZIP the platform build recipe expects: the SOURCE tree (manifest + src +
// build config), with build artifacts and VCS/dependency dirs excluded.
//
// Two kinds of exclusion are applied:
//
//   - Directory names (excludedDirs): VCS metadata, dependency installs, build
//     output, tooling caches — see that var.
//   - File names (isExcludedFile): build artifacts (*.zip) and EVERY file whose
//     BASE NAME starts with ".env" — dotted or not, so `.envrc` (direnv, which
//     routinely holds exported credentials) goes too — except a three-name
//     allow-list. The scaffolded page-money template documents pasting a real,
//     Buzz-spending block token into .env.development (VITE_LIVE_BLOCK_TOKEN)
//     for `dev:live`; bundling that file would leak the token to the server, the
//     moderator reviewer, and the built image. We KEEP .env.example /
//     .env.sample (meant to hold documented placeholders) and .env.production
//     (the server build's `vite build` runs in mode=production and reads it).
//     Those three are allow-listed BY NAME — nothing reads their contents, so
//     keeping one is not a claim that it holds no secret. See isExcludedFile and
//     keptEnvFiles for the full rule + rationale.
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

// excludedFilePatterns is the human-readable list of file-level exclusions, for
// CLI messaging. The authoritative matcher is isExcludedFile — keep the two in
// sync. These are matched on the file's BASE NAME anywhere in the tree.
var excludedFilePatterns = []string{
	// Build artifacts. A `--package-only` run drops a *.zip in the project dir;
	// without this, the next package recursively sweeps that zip back in (a real
	// dogfood regression: 22 files/93 KB vs the correct 21 files/47 KB).
	"*.zip",
	// Dev-local / secret-bearing dotenv files. The page-money template tells devs
	// to paste a real VITE_LIVE_BLOCK_TOKEN into .env.development for `dev:live`;
	// these must never reach the server / reviewer / built image.
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
	// like them".
	".env*",
}

// keptEnvFiles are .env* files we deliberately INCLUDE despite the broad
// ".env"-prefix exclusion below. They are allow-listed BY NAME:
//   - .env.example / .env.sample: templates meant to hold documented
//     placeholders a reviewer reads.
//   - .env.production: the server build runs `vite build` (mode=production),
//     which reads .env.production; the SCAFFOLDED file holds one public value,
//     VITE_BLOCK_ALLOWED_PARENT_ORIGINS.
//   - .env.production.local is NOT kept — `.local` is the dev-local override
//     convention and is caught by the `.env.*.local` rule above.
//
// 🔴 THIS IS A NAME ALLOW-LIST, NOT A SAFETY PROPERTY, AND NOTHING HERE READS
// THE CONTENTS. An earlier version of this comment (and of the README section it
// backs) argued that .env.production "by construction cannot hold a secret"
// because Vite inlines every VITE_-prefixed variable into the client bundle.
// That argument is wrong twice over: Vite inlines ONLY the VITE_-prefixed ones,
// so a plain `API_SECRET=…` in this file is not public and is still uploaded;
// and this repo itself treats a VITE_-prefixed value as a spending secret —
// VITE_LIVE_BLOCK_TOKEN is exactly why .env.development is on the exclusion list
// above. So do not re-derive a content guarantee from the prefix rule, and do
// not widen this list on the strength of one. What the CLI can honestly tell an
// author is WHERE the file goes; deciding what belongs in it is theirs.
var keptEnvFiles = map[string]struct{}{
	".env.example":    {},
	".env.sample":     {},
	".env.production": {},
}

// isExcludedFile reports whether a regular file (by base name) must be left out
// of the package. The rule is deliberately CONSERVATIVE toward "never upload a
// secret": every base name starting with ".env" is dropped UNLESS it is one of
// the three names on the keptEnvFiles allow-list. Build-time config for the
// server must come from the manifest / build recipe, not an uploaded dotenv.
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
// Note keptEnvFiles is an allow-list BY NAME. Nothing here reads a file's
// contents, so "kept" is a statement about WHERE the file goes, never a claim
// that it holds no secret — see the 🔴 note on keptEnvFiles.
func isExcludedFile(name string) bool {
	// Build artifacts.
	if strings.HasSuffix(name, ".zip") {
		return true
	}
	// Dotenv handling: allow-list the known-safe template / production files,
	// drop everything else that starts with ".env".
	if strings.HasPrefix(name, ".env") {
		if _, keep := keptEnvFiles[name]; keep {
			return false
		}
		return true
	}
	return false
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
		// Skip excluded files (build artifacts, secret-bearing dotenv files).
		if isExcludedFile(d.Name()) {
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

// IsExcludedFile reports whether a file would be excluded by base name
// (exported for tests + messaging). See isExcludedFile for the rule.
func IsExcludedFile(name string) bool { return isExcludedFile(name) }

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
