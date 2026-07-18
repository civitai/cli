package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These characterization tests pin the seams extracted from the download*
// decomposition so the behavior-preserving refactor stays honest: the part-file
// streaming+hashing lifecycle (writePart), the idempotency skip decision
// (presentTargetSatisfies), and — end-to-end through the refactored
// runDownload → downloadSelected → downloadOne path — the SSRF dial/scheme guard.

// TestWritePartHashesAndInstalls proves writePart streams the bytes into
// "<target>.part", returns the byte count and (when verifying) the lowercase-hex
// SHA256 of exactly those bytes, and leaves the finished .part in place for the
// caller to rename.
func TestWritePartHashesAndInstalls(t *testing.T) {
	dir := t.TempDir()
	part := filepath.Join(dir, "m.safetensors.part")
	body := "the-streamed-bytes"

	var errW bytes.Buffer
	written, got, err := writePart(strings.NewReader(body), part, &errW, "m.safetensors", int64(len(body)), true)
	if err != nil {
		t.Fatalf("writePart: %v", err)
	}
	if written != int64(len(body)) {
		t.Errorf("written = %d, want %d", written, len(body))
	}
	if got != sha256hex(body) {
		t.Errorf("hash = %q, want %q", got, sha256hex(body))
	}
	// The finished .part exists and holds exactly the streamed bytes.
	onDisk, rerr := os.ReadFile(part)
	if rerr != nil {
		t.Fatalf("finished .part should remain for the caller to rename: %v", rerr)
	}
	if string(onDisk) != body {
		t.Errorf(".part content = %q, want %q", onDisk, body)
	}
}

// TestWritePartNoVerifyReturnsEmptyHash proves that with verify=false writePart
// returns an empty hash (the caller skips the SHA256 comparison) but still writes
// the file.
func TestWritePartNoVerifyReturnsEmptyHash(t *testing.T) {
	dir := t.TempDir()
	part := filepath.Join(dir, "x.bin.part")
	var errW bytes.Buffer
	_, got, err := writePart(strings.NewReader("abc"), part, &errW, "x.bin", 3, false)
	if err != nil {
		t.Fatalf("writePart: %v", err)
	}
	if got != "" {
		t.Errorf("verify=false should return an empty hash, got %q", got)
	}
	if _, e := os.Stat(part); e != nil {
		t.Errorf(".part should still be written: %v", e)
	}
}

// errReader fails partway through the stream, exercising writePart's failure
// cleanup path.
type errReader struct{ n int }

func (e *errReader) Read(p []byte) (int, error) {
	if e.n <= 0 {
		return 0, errors.New("boom")
	}
	c := copy(p, strings.Repeat("a", e.n))
	e.n = 0
	return c, nil
}

// TestWritePartRemovesPartOnStreamError proves a failed transfer never leaves a
// truncated ".part" behind — writePart deletes it and returns a "streaming"
// error.
func TestWritePartRemovesPartOnStreamError(t *testing.T) {
	dir := t.TempDir()
	part := filepath.Join(dir, "half.bin.part")
	var errW bytes.Buffer
	_, _, err := writePart(&errReader{n: 4}, part, &errW, "half.bin", 100, true)
	if err == nil || !strings.Contains(err.Error(), "streaming") {
		t.Fatalf("expected a streaming error, got %v", err)
	}
	if _, e := os.Stat(part); e == nil {
		t.Error("a failed transfer must remove the partial .part file")
	}
}

// TestWritePartSelfHealsStalePart proves a stale ".part" from a previous aborted
// run is truncated/replaced, not appended to (there is no resume).
func TestWritePartSelfHealsStalePart(t *testing.T) {
	dir := t.TempDir()
	part := filepath.Join(dir, "s.bin.part")
	if err := os.WriteFile(part, []byte("STALE-LEFTOVER-BYTES"), 0o644); err != nil {
		t.Fatal(err)
	}
	var errW bytes.Buffer
	body := "fresh"
	_, got, err := writePart(strings.NewReader(body), part, &errW, "s.bin", int64(len(body)), true)
	if err != nil {
		t.Fatalf("writePart: %v", err)
	}
	onDisk, _ := os.ReadFile(part)
	if string(onDisk) != body {
		t.Errorf("stale .part should be replaced, got %q", onDisk)
	}
	if got != sha256hex(body) {
		t.Errorf("hash should cover only the fresh bytes: %q", got)
	}
}

// TestPresentTargetSatisfies pins the idempotency-skip decision seam.
func TestPresentTargetSatisfies(t *testing.T) {
	body := "present-bytes"
	sha := sha256hex(body)

	writeTarget := func(t *testing.T) string {
		p := filepath.Join(t.TempDir(), "f.bin")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("force never skips", func(t *testing.T) {
		var out bytes.Buffer
		if presentTargetSatisfies(&out, writeTarget(t), sha, true, true) {
			t.Error("--force must never skip a present file")
		}
	})

	t.Run("missing file does not skip", func(t *testing.T) {
		var out bytes.Buffer
		missing := filepath.Join(t.TempDir(), "nope.bin")
		if presentTargetSatisfies(&out, missing, sha, true, false) {
			t.Error("an absent target must not skip")
		}
	})

	t.Run("no verify trusts a present file", func(t *testing.T) {
		var out bytes.Buffer
		if !presentTargetSatisfies(&out, writeTarget(t), "", false, false) {
			t.Error("a present file with verify off should skip")
		}
		if !strings.Contains(out.String(), "already present") {
			t.Errorf("skip line missing: %q", out.String())
		}
	})

	t.Run("verify skips only on hash match", func(t *testing.T) {
		var out bytes.Buffer
		if !presentTargetSatisfies(&out, writeTarget(t), sha, true, false) {
			t.Error("a present file whose SHA256 matches should skip")
		}
		if !strings.Contains(out.String(), "SHA256 verified") {
			t.Errorf("verified-skip line missing: %q", out.String())
		}
	})

	t.Run("verify re-downloads on hash mismatch", func(t *testing.T) {
		var out bytes.Buffer
		if presentTargetSatisfies(&out, writeTarget(t), strings.Repeat("0", 64), true, false) {
			t.Error("a present file whose SHA256 does NOT match must re-download (not skip)")
		}
	})

	t.Run("a directory at target never skips", func(t *testing.T) {
		var out bytes.Buffer
		if presentTargetSatisfies(&out, t.TempDir(), "", false, false) {
			t.Error("a directory at the target path must not skip")
		}
	})
}

// TestDownloadSSRFGuardTriggersThroughCommand proves the SSRF guard still fires
// through the refactored runDownload → downloadSelected → downloadOne → api
// download path: with the guard at its default (bypass OFF), a file whose
// downloadUrl points at an internal address over plain http is refused before any
// bytes move. (The api-layer guard internals are exhaustively covered in
// download_ssrf_test.go; this asserts the cmd wiring the refactor moved through.)
func TestDownloadSSRFGuardTriggersThroughCommand(t *testing.T) {
	// The version detail is fetched by the ordinary reader (fine over loopback
	// http); only the file's downloadUrl is the SSRF target the guard must block.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/model-versions/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":128713,"modelId":4384,"name":"v","baseModel":"SD 1.5","files":[{"id":1,"name":"m.safetensors","type":"Model","primary":true,"sizeKB":1,"downloadUrl":"http://169.254.169.254/latest/meta-data/"}]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// NOTE: deliberately do NOT call allowPrivateDownloadHostsInTest — the guard
	// must be ON for this test.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	chdir(t, t.TempDir())

	_, _, err := run(t, "download", "128713")
	if err == nil {
		t.Fatal("SSRF guard should refuse a download URL pointing at an internal address")
	}
	// Plain-http internal target is refused at the scheme check ("https").
	if !strings.Contains(err.Error(), "https") && !strings.Contains(err.Error(), "private/loopback") {
		t.Errorf("refusal should cite the https/SSRF guard, got %v", err)
	}
	if _, e := os.Stat("m.safetensors.part"); e == nil {
		t.Error("a guard-refused download must not leave a .part file")
	}
}
