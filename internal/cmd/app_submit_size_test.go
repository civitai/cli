package cmd

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Issue #423: an oversized bundle was packaged, uploaded and refused with
// `400: Invalid JSON` — the server's own words, about the PARSE, naming nothing
// the author could act on. The CLI's `Packaged …` line reported the COMPRESSED
// zip size and never the base64-in-JSON body it actually sends, so no number
// the author could see corresponded to the number that was refused.
//
// These tests pin the two things that changed. Neither of them asserts a server
// limit: the CLI does not know one, and #423 bounds it only to
// (2.32 MB, 8.20 MB].

// pseudoRandomBytes returns n deterministic, essentially incompressible bytes.
//
// Incompressible is load-bearing twice over: it keeps every fixture's
// COMPRESSED size distinct (a file of zeroes deflates to the same handful of
// bytes whatever its length, which would make the ranking assertions below
// vacuous), and it keeps the zip's size a real function of the input sizes.
// Deterministic so a failure reproduces.
func pseudoRandomBytes(n int, seed uint32) []byte {
	b := make([]byte, n)
	x := seed | 1
	for i := range b {
		// xorshift32 — no math/rand dependency, no global state.
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		b[i] = byte(x)
	}
	return b
}

// sizedProject writes a valid app project whose files have PAIRWISE DISTINCT,
// incompressible payloads, and returns the base names largest-first.
//
// The sizes are deliberately unround and share no value with any constant an
// assertion here names (there are none — every expectation below is derived
// from the bytes the test itself observes).
func sizedProject(t *testing.T, dir string) {
	t.Helper()
	writeStaticManifest(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, "docs", "screenshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, size := range map[string]int{
		"index.html":                    811,
		"src.js":                        2503,
		"docs/screenshots/one.png":      6337,
		"docs/screenshots/two.png":      4099,
		"docs/screenshots/three.png":    1237,
		"docs/screenshots/tiniest.webp": 97,
	} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.WriteFile(p, pseudoRandomBytes(size, uint32(size)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// packagedLineRe parses the `Packaged …` line into its four numbers. Parsing
// rather than substring-matching, so the assertions below are about the VALUES
// the CLI printed and not about a phrase appearing somewhere near them.
var packagedLineRe = regexp.MustCompile(
	`Packaged (\d+) file\(s\) \((\d+) bytes compressed, (\d+) decompressed; (\d+) bytes as the base64 JSON submit body\)`)

// TestPackagedLineNamesTheJSONSubmitBodySize is the arithmetic pin.
//
// 🔴 THE EXPECTED VALUE IS NOT HAND-COMPUTED AND NOT TAKEN FROM THE
// IMPLEMENTATION. It is re-derived here from the real .zip on disk with
// encoding/json and encoding/base64 — the same two stdlib packages
// SubmitVersion goes through — so a wrong envelope constant, a wrong encoding
// or a dropped term all show up as a mismatch.
func TestPackagedLineNamesTheJSONSubmitBodySize(t *testing.T) {
	tmp := t.TempDir()
	sizedProject(t, tmp)
	out := filepath.Join(t.TempDir(), "bundle.zip")

	stdout, _, err := run(t, "app", "submit", tmp, "--package-only", "--out", out)
	if err != nil {
		t.Fatalf("submit --package-only: %v\n%s", err, stdout)
	}

	m := packagedLineRe.FindStringSubmatch(stdout)
	if m == nil {
		t.Fatalf("the `Packaged …` line did not report a base64 JSON submit body size.\n"+
			"An author reading this line is trying to find the number a request-body limit applies to, "+
			"and the compressed size is not it (#423).\nstdout:\n%s", stdout)
	}
	gotCompressed := atoiOrFatal(t, m[2])
	gotBody := atoiOrFatal(t, m[4])

	zipBytes, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if gotCompressed != len(zipBytes) {
		t.Fatalf("the line reports %d bytes compressed but the .zip it wrote is %d bytes — "+
			"the rest of this test would be measuring the wrong artifact", gotCompressed, len(zipBytes))
	}

	// Independent derivation: build the request body the way SubmitVersion does
	// and measure it. A map marshals to the identical document a
	// single-string-field struct with this json tag does.
	wantBody, err := json.Marshal(map[string]string{
		"bundleBase64": base64.StdEncoding.EncodeToString(zipBytes),
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotBody != len(wantBody) {
		t.Errorf("reported submit-body size %d, but marshalling the real .zip into the real body shape gives %d",
			gotBody, len(wantBody))
	}

	// Positive control on the fixture itself: if the two numbers were equal the
	// assertion above could pass while the CLI printed the zip size twice.
	if gotBody <= gotCompressed {
		t.Errorf("submit-body size %d is not larger than the compressed size %d — the fixture cannot "+
			"distinguish the two quantities, so this test proves nothing about which one was printed",
			gotBody, gotCompressed)
	}
}

// entryLineRe parses one row of the failure diagnosis's entry table.
var entryLineRe = regexp.MustCompile(`(?m)^\s+(\d+) / (\d+)\s+(\S+)\s*$`)

type sizedEntry struct {
	compressed   uint64
	uncompressed uint64
	name         string
}

// TestSubmitFailureNamesWhatItSentAndTheLargestEntries pins the failure path.
//
// 🔴 EVERY EXPECTATION IS MEASURED AT THE SERVER, NOT COMPUTED FROM THE CLI.
// The httptest handler records how many bytes actually arrived and keeps the
// bundle it was given; the assertions compare the CLI's printed account against
// those observations. So the printed size is checked against the real
// on-the-wire byte count, and the printed entry table against the real contents
// of the archive that was really uploaded.
func TestSubmitFailureNamesWhatItSentAndTheLargestEntries(t *testing.T) {
	tmp := t.TempDir()
	sizedProject(t, tmp)

	var wireBytes int
	var uploaded []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet { // the monotonic-version guard's read
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"submissions":[]}`))
			return
		}
		raw, _ := io.ReadAll(r.Body)
		wireBytes = len(raw)
		var body struct {
			BundleBase64 string `json:"bundleBase64"`
		}
		if err := json.Unmarshal(raw, &body); err == nil {
			uploaded, _ = base64.StdEncoding.DecodeString(body.BundleBase64)
		}
		// Exactly what #423 measured: a 400 whose body names nothing at all.
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid JSON"}`))
	}))
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-423")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	t.Setenv("CIVITAI_SUBMIT_PATH", "/api/blocks/submit-version")

	stdout, errOut, err := run(t, "app", "submit", tmp, "--yes")
	if err == nil {
		t.Fatalf("expected the 400 to fail the submit\nstdout:\n%s", stdout)
	}
	if !strings.Contains(err.Error(), "Invalid JSON") {
		t.Fatalf("the server's own message must still reach the user verbatim, got %q", err)
	}
	if wireBytes == 0 || len(uploaded) == 0 {
		t.Fatalf("the fake server observed no upload (%d wire bytes, %d bundle bytes) — "+
			"every assertion below would be about nothing", wireBytes, len(uploaded))
	}

	// 1. The size the CLI claims to have sent is the size that arrived.
	wantSize := fmt.Sprintf("%d bytes on the wire", wireBytes)
	if !strings.Contains(errOut, wantSize) {
		t.Errorf("the failure output does not name the bytes that went on the wire (%q).\n"+
			"That number is the one any request-body limit applies to, and it is the one the author "+
			"cannot obtain any other way (#423).\nstderr:\n%s", wantSize, errOut)
	}
	// The zip size is the number that was ALREADY visible and did not explain
	// the failure; naming it alongside is what makes the first number legible.
	if !strings.Contains(errOut, fmt.Sprintf("a %d-byte zip", len(uploaded))) {
		t.Errorf("the failure output does not name the zip size behind that body\nstderr:\n%s", errOut)
	}

	// 2. The entry table matches the archive the server really received.
	got := parseEntryTable(t, errOut)
	if len(got) == 0 {
		t.Fatalf("the failure output named no bundle entries. Finding the oversized directory took "+
			"`--package-only` plus `unzip -l` in #423.\nstderr:\n%s", errOut)
	}
	want := largestOfUploadedZip(t, uploaded, len(got))
	if len(got) != len(want) {
		t.Fatalf("entry table has %d rows, the archive's top %d are %v", len(got), len(want), want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("entry row %d = %+v, want %+v (ranked by COMPRESSED size, largest first, "+
				"read back from the uploaded archive)", i, got[i], want[i])
		}
	}

	// 3. The biggest thing in the fixture is a screenshot, which is #423's own
	// shape: the row that tells the author which DIRECTORY to look at.
	if !strings.HasPrefix(got[0].name, "docs/screenshots/") {
		t.Errorf("largest entry is %q; the fixture's largest file is under docs/screenshots/, so the "+
			"ranking is not putting the offending directory in front of the author", got[0].name)
	}
}

// TestSubmitAuthFailureOmitsTheSizeDiagnosis is the other half of the gate. A
// 403 is the single most common submit failure while Apps is invite-only, and
// it has nothing to do with the bundle: printing an entry table under it is
// noise on an error that already says what to do.
func TestSubmitAuthFailureOmitsTheSizeDiagnosis(t *testing.T) {
	tmp := t.TempDir()
	sizedProject(t, tmp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"submissions":[]}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"no Apps access"}`))
	}))
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-403")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	t.Setenv("CIVITAI_SUBMIT_PATH", "/api/blocks/submit-version")

	stdout, errOut, err := run(t, "app", "submit", tmp, "--yes")
	if err == nil {
		t.Fatalf("expected the 403 to fail the submit\nstdout:\n%s", stdout)
	}
	for _, unwanted := range []string{"bytes on the wire", "largest entries in the bundle"} {
		if strings.Contains(errOut, unwanted) {
			t.Errorf("a credential refusal printed the bundle-size account (%q) — it is unrelated to "+
				"the bundle, and burying the 403 under a file table is how the useful line stops being read"+
				"\nstderr:\n%s", unwanted, errOut)
		}
	}
}

func parseEntryTable(t *testing.T, s string) []sizedEntry {
	t.Helper()
	i := strings.Index(s, "largest entries in the bundle")
	if i < 0 {
		return nil
	}
	var out []sizedEntry
	for _, m := range entryLineRe.FindAllStringSubmatch(s[i:], -1) {
		c, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		u, err := strconv.ParseUint(m[2], 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, sizedEntry{compressed: c, uncompressed: u, name: m[3]})
	}
	return out
}

// largestOfUploadedZip re-derives the expected ranking straight from the bytes
// the fake server received, with archive/zip — not through the packager.
func largestOfUploadedZip(t *testing.T, zipBytes []byte, n int) []sizedEntry {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("the uploaded bundle is not a readable zip: %v", err)
	}
	var all []sizedEntry
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		all = append(all, sizedEntry{
			compressed:   f.CompressedSize64,
			uncompressed: f.UncompressedSize64,
			name:         f.Name,
		})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].compressed != all[j].compressed {
			return all[i].compressed > all[j].compressed
		}
		return all[i].name < all[j].name
	})
	if len(all) > n {
		all = all[:n]
	}
	return all
}

func atoiOrFatal(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
