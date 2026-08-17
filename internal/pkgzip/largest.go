package pkgzip

import (
	"archive/zip"
	"bytes"
	"sort"
)

// Entry is one file in a built bundle, with the two sizes the archive itself
// records for it.
type Entry struct {
	// Name is the archive-relative path, exactly as it appears in the zip.
	Name string
	// Compressed is the stored (deflated) size. This is the one that matters
	// for a request-body limit: it is what the upload is made of.
	Compressed uint64
	// Uncompressed is the original file size, kept because it is what an author
	// recognises when they go looking for the file on disk.
	Uncompressed uint64
}

// LargestEntries returns the n largest entries of zipBytes by COMPRESSED size,
// largest first, ties broken by name so the order is deterministic. Fewer than
// n are returned when the archive holds fewer.
//
// 🔴 IT READS THE ARCHIVE, NOT THE DIRECTORY. The caller in `app submit` has
// the exact bytes it tried to upload, and those are the honest witness: the
// packager drops directories and file names (see isExcludedDir /
// isExcludedFile), so a walk of the project would report entries that were
// never sent, and would miss nothing the archive can miss.
//
// Ranking is on the COMPRESSED size, not the original: a 40 MB text file that
// deflates to 200 KB contributes 200 KB to the body a size limit is applied to,
// and naming it first would send an author to delete the wrong thing. #423's
// real cause was a directory of PNGs — already-compressed bytes, which is
// exactly the case where the two orders agree and the reason to be explicit
// about which one is meant.
//
// A zip that cannot be read returns an error; callers on a failure path should
// simply print less rather than replacing one unhelpful message with another.
func LargestEntries(zipBytes []byte, n int) ([]Entry, error) {
	if n <= 0 {
		return nil, nil
	}
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(zr.File))
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		entries = append(entries, Entry{
			Name:         f.Name,
			Compressed:   f.CompressedSize64,
			Uncompressed: f.UncompressedSize64,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Compressed != entries[j].Compressed {
			return entries[i].Compressed > entries[j].Compressed
		}
		return entries[i].Name < entries[j].Name
	})
	if len(entries) > n {
		entries = entries[:n]
	}
	return entries, nil
}
