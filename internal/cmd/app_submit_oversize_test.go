package cmd

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
)

// TestAppSubmitOversizeEndToEnd drives `app submit` through the REAL command,
// with a bundle genuinely over appapi.MaxSubmitBodyBytes, and reads what the
// server received.
//
// 🔴 WHY THIS EXISTS WHEN internal/appapi ALREADY TESTS THE FIELD. Round 1 of
// #585's audit mutated the command layer and the whole repo stayed green:
//
//	internal/cmd/app_submit.go  client.AllowOversizeBody = allowOversize -> = false
//	                            SURVIVED — the escape hatch is entirely dead
//	internal/cmd/app_submit.go  "allow-oversize" -> "allow-oversized"
//	                            SURVIVED — the flag named by the error message,
//	                            the README command row, the Troubleshooting row
//	                            and the command's own Example ceases to exist
//
// Both leave a user with `unknown flag` and no way past a refusal the CLI's own
// message tells them to pass a flag for. The appapi test asserts the library
// FIELD works; nothing asserted the flag reaches it. That test's own header says
// "a flag that is wired but inert reads exactly like a working one" — and then
// tested the field. This is the other half.
//
// It also pins F1: the refusal must NOT print the post-upload diagnosis, whose
// first line claims bytes went on the wire.
func TestAppSubmitOversizeEndToEnd(t *testing.T) {
	// An incompressible asset, so the zip cannot shrink under the ceiling. 3/4
	// of the ceiling of RANDOM bytes base64-encodes to ~the ceiling and then
	// some, once the JSON envelope is added.
	oversizeAsset := func(t *testing.T, dir string) {
		t.Helper()
		buf := make([]byte, (appapi.MaxSubmitBodyBytes/4)*3+(1<<20))
		if _, err := rand.Read(buf); err != nil {
			t.Fatalf("could not build the fixture asset: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "big.bin"), buf, 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}

	t.Run("refused by default, and nothing reaches the server", func(t *testing.T) {
		f := cleanAppFixture(t, "")
		oversizeAsset(t, f.root)
		f.commit("big", "big.bin")

		rec, srv := newSubmitRecorder(t)
		defer srv.Close()
		stdout, stderr, err := runSubmitIn(t, f.root, srv.URL)
		if err == nil {
			t.Fatalf("an over-ceiling bundle submitted without a refusal\n%s\n%s", stdout, stderr)
		}
		if !strings.Contains(err.Error(), "the server can receive") {
			t.Errorf("refused, but not by the size guard: %v", err)
		}
		// 🔴 THE PREFLIGHT PROMISE. A refusal that still uploads is worse than
		// no refusal — it costs the author the transfer AND fails.
		if rec.nbytes != 0 {
			t.Errorf("the server received %d bytes; the refusal must happen BEFORE the upload", rec.nbytes)
		}
		// The message must name the way out, or the flag is undiscoverable from
		// the only place it is needed.
		if !strings.Contains(err.Error(), "--allow-oversize") {
			t.Errorf("the refusal does not name --allow-oversize:\n  %v", err)
		}
		// F1: the post-upload diagnosis lies on this path.
		if strings.Contains(stderr, "What this CLI sent") {
			t.Errorf("the refusal printed the post-UPLOAD diagnosis, which opens \"What this CLI sent\" and "+
				"states bytes went on the wire. Nothing was sent — the server received %d bytes. Use "+
				"printSubmitSizeRefusal, whose tense is correct for a decision made before contacting "+
				"the server.\nstderr:\n%s", rec.nbytes, stderr)
		}
		if !strings.Contains(stderr, "What this CLI would have sent") {
			t.Errorf("the refusal printed no size block at all. Its own message tells the author that "+
				"`civitai app submit` lists the largest entries, so dropping the list makes that "+
				"sentence false.\nstderr:\n%s", stderr)
		}
		// 🔴 STDOUT IS A SEPARATE SURFACE AND THIS TEST WAS BLIND TO IT. Every
		// assertion above reads stderr. `Submitting demo@1.0.0` is written to
		// STDOUT by the spinner, before SubmitVersion is called — so the CLI
		// announced an upload on one stream and reported on the other that
		// nothing had been uploaded, and the whole suite was green. A reader of
		// a captured stdout log saw only the announcement.
		if strings.Contains(stdout, "Submitting") {
			t.Errorf("stdout announced the upload for a bundle that was never uploaded. `Submitting …` "+
				"goes to stdout and the refusal goes to stderr, so this line is contradicted by a stream "+
				"the reader may not have.\nstdout:\n%s", stdout)
		}
		// POSITIVE CONTROL on that assertion: it must be able to see the word at
		// all, or it is a check against a stream nothing is written to.
		if !strings.Contains(stdout, "Packaged") {
			t.Fatalf("CONTROL failure, not a finding: stdout carries no `Packaged …` line, so the "+
				"`Submitting` check above is reading an empty stream and proves nothing.\nstdout:\n%s", stdout)
		}
	})

	t.Run("--allow-oversize reaches the client and the upload happens", func(t *testing.T) {
		f := cleanAppFixture(t, "")
		oversizeAsset(t, f.root)
		f.commit("big", "big.bin")

		rec, srv := newSubmitRecorder(t)
		defer srv.Close()
		stdout, stderr, err := runSubmitIn(t, f.root, srv.URL, "--allow-oversize")
		if err != nil {
			t.Fatalf("--allow-oversize did not reach the client — the flag is not wired to "+
				"Client.AllowOversizeBody, or it is named differently than the error message says: %v\n%s\n%s",
				err, stdout, stderr)
		}
		// 🔴 POSITIVE CONTROL. Without it this passes if the override made the
		// command succeed for any other reason — the over-ceiling body must
		// actually have been uploaded.
		if rec.nbytes <= appapi.MaxSubmitBodyBytes {
			t.Errorf("the server received %d bytes, not over the %d ceiling — this case is not exercising "+
				"the override at all", rec.nbytes, appapi.MaxSubmitBodyBytes)
		}
	})
}
