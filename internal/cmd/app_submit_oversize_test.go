package cmd

import (
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
)

// flattenWrapped collapses every run of whitespace to a single space.
//
// 🔴 REQUIRED, NOT COSMETIC. The size blocks are wrapped at 78 runes by
// wrapRunes, so any phrase long enough to be worth asserting on is liable to
// land across a line break — and a raw strings.Contains then reports the phrase
// ABSENT, which is the same answer it gives when the sentence really is gone.
// That is a guard that passes for the wrong reason in one direction and fails
// for the wrong reason in the other.
func flattenWrapped(s string) string { return strings.Join(strings.Fields(s), " ") }

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
		// 🔴 THE POSITIVE HALF OF THE SPINNER PREDICATE, AND THE `allowOversize`
		// TERM OF IT SPECIFICALLY. The refused case above asserts `Submitting …`
		// is ABSENT; nothing asserted it is ever PRESENT, so
		// `spin := allowOversize || …` could be mutated to `spin := false` with
		// the whole package green. Here the body is over the ceiling, so the
		// second term of that disjunction is false and only `allowOversize`
		// keeps the announcement — dropping the term fails exactly this line.
		if !strings.Contains(stdout, "Submitting") {
			t.Errorf("an upload really happened (%d bytes reached the server) and stdout never announced "+
				"it. `Submitting …` is the only feedback during the transfer, and on a body this size "+
				"that is a long silent wait.\nstdout:\n%s", rec.nbytes, stdout)
		}
	})

	// 🔴 THE OTHER TERM OF THE SAME PREDICATE, ON THE ORDINARY PATH. A bundle
	// under the ceiling announces because `SubmitBodySize(...) < MaxSubmitBodyBytes`
	// is true, not because of any flag, so mutating THAT comparison away is caught
	// here and not by the subtest above. The two cases together mean neither term
	// can be deleted silently.
	t.Run("an ordinary submit announces the upload", func(t *testing.T) {
		f := cleanAppFixture(t, "")

		rec, srv := newSubmitRecorder(t)
		defer srv.Close()
		stdout, stderr, err := runSubmitIn(t, f.root, srv.URL)
		if err != nil {
			t.Fatalf("submit: %v\n%s\n%s", err, stdout, stderr)
		}
		// CONTROL: this fixture must be on the under-the-ceiling side of the
		// predicate, or the case is a duplicate of the one above.
		if rec.nbytes == 0 || rec.nbytes >= appapi.MaxSubmitBodyBytes {
			t.Fatalf("CONTROL failure, not a finding: the server received %d bytes, which is not a body "+
				"strictly under the %d ceiling — this case is not exercising the size term of the "+
				"predicate", rec.nbytes, appapi.MaxSubmitBodyBytes)
		}
		if !strings.Contains(stdout, "Submitting") {
			t.Errorf("a plain submit uploaded %d bytes and never announced it on stdout. Every submit "+
				"under the ceiling is a real network wait and `Submitting …` is the only sign it is "+
				"happening.\nstdout:\n%s", rec.nbytes, stdout)
		}
	})

	// 🔴 THE ESCAPE HATCH MUST BE PASTEABLE, WHICH MEANS CARRYING THE FLAGS THIS
	// RUN NEEDED. This is the ONLY shape that reaches the size refusal on a real
	// project: the oversize file is uncommitted, so the dirty-tree guard — which
	// runs BEFORE the size guard — has to have been waived to get here at all. A
	// suggestion of the bare `civitai app submit --allow-oversize` is therefore
	// refused again, for the dirty tree, and the author has been handed a command
	// that cannot work.
	t.Run("the escape-hatch suggestion carries the flags this run needed", func(t *testing.T) {
		f := cleanAppFixture(t, "")
		// NOT committed: the tree is dirty and the bundle carries the big file.
		oversizeAsset(t, f.root)

		rec, srv := newSubmitRecorder(t)
		defer srv.Close()
		stdout, stderr, err := runSubmitIn(t, f.root, srv.URL, "--allow-dirty")
		if err == nil {
			t.Fatalf("an over-ceiling bundle submitted without a refusal\n%s\n%s", stdout, stderr)
		}
		// CONTROL: it must be the SIZE guard that refused, not the dirty guard —
		// otherwise this case never reaches the block whose wording it asserts.
		if !strings.Contains(err.Error(), "the server can receive") {
			t.Fatalf("CONTROL failure, not a finding: refused by something other than the size guard, so "+
				"the refusal block under test never printed: %v", err)
		}
		if rec.nbytes != 0 {
			t.Fatalf("CONTROL failure, not a finding: the server received %d bytes, so this was not the "+
				"preflight refusal", rec.nbytes)
		}

		// The suggestion is the line carrying --allow-oversize.
		var line string
		for _, l := range strings.Split(stderr, "\n") {
			if strings.Contains(l, "--allow-oversize") && strings.Contains(l, "civitai app submit") {
				line = strings.TrimSpace(l)
			}
		}
		if line == "" {
			t.Fatalf("CONTROL failure, not a finding: the refusal printed no `civitai app submit "+
				"--allow-oversize` suggestion at all.\nstderr:\n%s", stderr)
		}
		if !strings.Contains(line, "--allow-dirty") {
			t.Errorf("the suggested escape hatch is %q, which drops --allow-dirty. The dirty-tree guard "+
				"runs BEFORE the size guard, so this command is refused again — for a different reason — "+
				"and the author is no closer to the upload the message promised.", line)
		}
		if !strings.Contains(line, f.root) {
			t.Errorf("the suggested escape hatch is %q, which drops the directory this run was pointed "+
				"at (%s). Pasted as printed it packages the current directory instead.", line, f.root)
		}
	})

	// 🔴 THE DIAGNOSIS BLOCK MUST NOT TELL AN OVERSIZE-OVERRIDE AUTHOR THAT THEIR
	// BODY WAS UNDER THE CEILING. This is #423's own scenario — a body the
	// platform will not receive, answered `400: Invalid JSON` — reached by the one
	// invocation that is certainly over the ceiling. The unconditional sentence
	// printed "up to N bytes on the wire" and then, two lines down the same
	// stream, that the guard refuses at a smaller number "so this body was under
	// that".
	t.Run("--allow-oversize must not claim the body was under the ceiling", func(t *testing.T) {
		f := cleanAppFixture(t, "")
		oversizeAsset(t, f.root)
		f.commit("big", "big.bin")

		// The platform's real answer to an over-limit body: the proxy truncates
		// it and the handler fails to parse what arrives, so the error names the
		// PARSE and nothing about size.
		var nbytes int64
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "submit-version") {
				nbytes, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"Invalid JSON"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"submissions": []any{}})
		}))
		defer srv.Close()

		stdout, stderr, err := runSubmitIn(t, f.root, srv.URL, "--allow-oversize")
		if err == nil {
			t.Fatalf("the server answered 400 and the submit reported success\n%s\n%s", stdout, stderr)
		}
		// CONTROL: the body must really have been over the ceiling and really
		// have been sent, or this is not the path under test.
		if nbytes <= int64(appapi.MaxSubmitBodyBytes) {
			t.Fatalf("CONTROL failure, not a finding: the server received %d bytes, not over the %d "+
				"ceiling — the override was not exercised", nbytes, appapi.MaxSubmitBodyBytes)
		}
		// CONTROL: the diagnosis block must have printed at all, or the
		// assertions below are reading a stream nothing was written to.
		if !strings.Contains(stderr, "What this CLI sent") {
			t.Fatalf("CONTROL failure, not a finding: the post-upload diagnosis did not print, so this "+
				"case is asserting nothing about its wording.\nstderr:\n%s", stderr)
		}
		flat := flattenWrapped(stderr)
		if strings.Contains(flat, "so this body was under that") {
			t.Errorf("the diagnosis told an --allow-oversize author that their body was under the "+
				"%d-byte ceiling. The guard did not fire because the flag made it INERT, not because "+
				"the body was small: %d bytes reached the server. The claim is false, it sits two "+
				"lines under a size line that contradicts it, and it points the one author who is "+
				"certainly over the limit away from the likeliest cause (issue #423).\nstderr:\n%s",
				appapi.MaxSubmitBodyBytes, nbytes, stderr)
		}
		if !strings.Contains(flat, "--allow-oversize waived that refusal") {
			t.Errorf("the diagnosis never says the size guard was waived for this run, so nothing on "+
				"this path names the override as a possible cause of the failure.\nstderr:\n%s", stderr)
		}
	})
}
