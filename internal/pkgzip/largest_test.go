package pkgzip

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// incompressible returns n deterministic, essentially incompressible bytes, so
// each fixture's COMPRESSED size is a real function of its length. A file of
// zeroes deflates to the same handful of bytes at any size, which would make
// every ranking assertion here vacuous.
func incompressible(n int, seed uint32) []byte {
	b := make([]byte, n)
	x := seed | 1
	for i := range b {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		b[i] = byte(x)
	}
	return b
}

func buildFixtureZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(`{"blockId":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	return res.Zip
}

// TestLargestEntriesRanksByCompressedSize drives LargestEntries over a REAL
// archive produced by Build — not a hand-assembled one — and checks the ranking
// against sizes read back out of that same archive with archive/zip.
//
// Fixture lengths are pairwise distinct and unround, and no assertion here
// names a size constant, so nothing can pass by a fixture happening to equal a
// literal in the implementation.
func TestLargestEntriesRanksByCompressedSize(t *testing.T) {
	zipBytes := buildFixtureZip(t, map[string][]byte{
		"index.html":                 incompressible(811, 811),
		"src/app.js":                 incompressible(2503, 2503),
		"docs/screenshots/one.png":   incompressible(6337, 6337),
		"docs/screenshots/two.png":   incompressible(4099, 4099),
		"docs/screenshots/three.png": incompressible(1237, 1237),
	})

	got, err := LargestEntries(zipBytes, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("asked for 3 entries, got %d: %+v", len(got), got)
	}

	wantNames := []string{
		"docs/screenshots/one.png",
		"docs/screenshots/two.png",
		"src/app.js",
	}
	for i, want := range wantNames {
		if got[i].Name != want {
			t.Errorf("entry %d = %q, want %q (largest compressed first)", i, got[i].Name, want)
		}
	}

	// Sizes must be the archive's own, not the packager's or the caller's idea
	// of them — read them back independently.
	actual := map[string]Entry{}
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		actual[f.Name] = Entry{Name: f.Name, Compressed: f.CompressedSize64, Uncompressed: f.UncompressedSize64}
	}
	for _, e := range got {
		if actual[e.Name] != e {
			t.Errorf("%s reported %+v, the archive records %+v", e.Name, e, actual[e.Name])
		}
		// The two columns must hold DIFFERENT values in this fixture, or a swap
		// of the struct fields would go unseen. (Incompressible bytes make the
		// stored size slightly LARGER, which is a difference either way.)
		if e.Uncompressed == e.Compressed {
			t.Errorf("%s reports compressed == uncompressed == %d — the two columns are not "+
				"distinguishable here, so this row cannot see a swap", e.Name, e.Compressed)
		}
	}

	// Descending, strictly: equal neighbours would let a reversed comparator pass.
	for i := 1; i < len(got); i++ {
		if got[i-1].Compressed <= got[i].Compressed {
			t.Errorf("entries not strictly descending by compressed size: %+v", got)
		}
	}
}

// TestLargestEntriesRanksByCompressedNotUncompressed is the discriminating case
// for the doc comment's claim, and the two orders DISAGREE here on purpose: a
// large, highly compressible file contributes almost nothing to the body a size
// limit is applied to, and naming it first would send the author to delete the
// wrong file.
func TestLargestEntriesRanksByCompressedNotUncompressed(t *testing.T) {
	zipBytes := buildFixtureZip(t, map[string][]byte{
		"fixtures/huge.txt": []byte(strings.Repeat("a", 200_003)),
		"assets/photo.png":  incompressible(5171, 5171),
	})

	got, err := LargestEntries(zipBytes, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %+v", got)
	}
	if got[0].Name != "assets/photo.png" {
		t.Errorf("largest = %q, want assets/photo.png: it is 5171 compressed bytes against a 200003-byte "+
			"text file that deflates to a few hundred, so ranking by UNCOMPRESSED size inverts this", got[0].Name)
	}
	if got[0].Uncompressed >= got[1].Uncompressed {
		t.Fatalf("the fixture no longer distinguishes the two orders — the winner is also the larger "+
			"uncompressed file, so this test would pass under either comparator: %+v", got)
	}
}

// TestLargestEntriesCapAndDegradation covers the edges the failure path hits.
func TestLargestEntriesCapAndDegradation(t *testing.T) {
	zipBytes := buildFixtureZip(t, map[string][]byte{
		"a.bin": incompressible(311, 311),
		"b.bin": incompressible(733, 733),
	})

	// n larger than the archive returns what there is, not an error.
	got, err := LargestEntries(zipBytes, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 { // a.bin, b.bin, block.manifest.json
		t.Errorf("want every entry when n exceeds the archive, got %+v", got)
	}

	if got, err := LargestEntries(zipBytes, 0); err != nil || got != nil {
		t.Errorf("LargestEntries(_, 0) = %+v, %v; want nil, nil", got, err)
	}

	// A caller on a failure path must get an error rather than a panic or a
	// confident empty list when the bytes are not an archive at all.
	if _, err := LargestEntries([]byte("not a zip"), 3); err == nil {
		t.Error("LargestEntries on non-zip bytes returned no error — a silent empty list on the failure " +
			"path reads as `the bundle is empty`, which is a worse claim than saying nothing")
	}
}
