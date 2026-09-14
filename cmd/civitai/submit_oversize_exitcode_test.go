package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/cmd"
	"github.com/civitai/cli/pkg/civitai"
)

// THE EXIT CODE OF THE OVERSIZE-BODY REFUSAL (issue #585).
//
// 🔴 THE CONTRACT WAS PUBLISHED AND NOT ASSERTED. internal/cmd/exitcodes_doc.go
// publishes two things about this refusal: that it exits `1`, and — under code 2
// — that "a script branching on `2` for that case must branch on `1`", i.e. a
// documented BREAKING CHANGE for anyone who scripted against the old behaviour,
// where the server's `400: Invalid JSON` produced a 2. appapi's own comment on
// ErrBundleTooLarge says the code is "now assertable".
//
// It was assertable and not asserted. Measured at the commit that shipped #585:
// no row in exitCodeContractClaims(), no entry in exitCodeClaimsFloor, and no
// reference to ErrBundleTooLarge anywhere under cmd/civitai/. The behaviour was
// correct, but only by falling through exitCode's `default` — which is exactly
// the accident AGENTS.md item 7 says an untagged return produces, and which the
// dirty-tree refusal one file over has a test for precisely because being right
// by accident is not the same as being pinned.
//
// This is the missing half, shaped like its sibling
// (submit_dirty_guard_exitcode_test.go): the REAL command, the REAL error, a
// negative control on the instrument.

// realOversizeRefusal runs the REAL `civitai app submit` against a project whose
// bundle cannot fit the submit body, and returns the error the command produced.
//
// 🔴 IT TAKES THE REAL ERROR, NOT A HAND-TAGGED FIXTURE. A test that builds its
// own `fmt.Errorf("%w", appapi.ErrBundleTooLarge)` asserts the exit mapping of an
// error it invented; it cannot see the guard returning something else, or
// wrapping it in a way that breaks the errors.Is walk. That is the seam this
// package exists to join — classification in internal/cmd, process contract
// here — and the sibling test's header says so after the same defect.
//
// NO GIT REPO ON PURPOSE. The dirty-work-tree guard degrades silently outside a
// repository (the scaffolded-app path), so this fixture reaches the size guard
// without the dirty guard firing first — and the submit carries zero provenance,
// which is the shape --package-only and the no-token fallback also send.
func realOversizeRefusal(t *testing.T) (error, string, string) {
	t.Helper()

	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	write := func(rel string, content []byte) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("block.manifest.json", []byte(`{
  "$schema": "https://civitai.com/schemas/app-block/v1.json",
  "blockId": "custom-generators",
  "version": "0.6.1",
  "name": "Custom Generators",
  "type": "block",
  "scopes": [],
  "page": { "path": "/", "title": "Custom Generators", "icon": "bolt" },
  "iframe": { "minHeight": 400, "maxHeight": 4000, "resizable": true, "sandbox": "allow-scripts allow-forms" },
  "contentRating": "g",
  "minApiVersion": "1.0"
}`))
	write("index.html", []byte("<html></html>"))

	// INCOMPRESSIBLE, so the zip cannot shrink back under the ceiling. 3/4 of the
	// ceiling in random bytes base64-encodes to about the ceiling; the extra MiB
	// puts it unambiguously over.
	big := make([]byte, (appapi.MaxSubmitBodyBytes/4)*3+(1<<20))
	if _, err := rand.Read(big); err != nil {
		t.Fatalf("CONTROL failure, not a finding: could not build the fixture asset: %v", err)
	}
	write("big.bin", big)

	// 🔴 THE SUBMIT ROUTE MUST NEVER BE REACHED — the refusal is a PREFLIGHT, and
	// a handler that fails the test is the reachability control: if the guard
	// stopped firing, this goes red here rather than passing quietly.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "submit-version") {
			t.Errorf("the oversize guard must refuse before any upload; got %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// The version-regression guard's listing read, which happens first.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"submissions": []any{}})
	}))
	t.Cleanup(srv.Close)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-test") // the guard only runs on the upload path
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	t.Setenv("CIVITAI_SUBMIT_PATH", "")
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "1")

	root := cmd.NewRootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs([]string{"app", "submit", dir, "--yes"})
	runErr := root.Execute()
	if runErr == nil {
		t.Fatalf("`app submit` must refuse a bundle over the submit-body ceiling; stdout:\n%s\nstderr:\n%s",
			out.String(), errb.String())
	}
	if !strings.Contains(runErr.Error(), "the server can receive") {
		t.Fatalf("the error is not the size refusal (so this test would be about the wrong error "+
			"entirely): %v", runErr)
	}
	return runErr, out.String(), errb.String()
}

func TestOversizeBundleExitsGeneric(t *testing.T) {
	err, _, _ := realOversizeRefusal(t)

	// The sentinel must be the one the contract names, on the REAL error.
	if !errors.Is(err, appapi.ErrBundleTooLarge) {
		t.Fatal("the REAL refusal does not carry appapi.ErrBundleTooLarge — the exit-code claim below " +
			"would be about a sentinel nothing produces, and `app submit` would also print the " +
			"post-UPLOAD diagnosis, which claims bytes went on the wire")
	}
	if got := exitCode(err); got != exitGeneric {
		t.Errorf("exitCode(bundle over the submit-body ceiling) = %d, want %d (generic — a verdict about "+
			"the project).\n2 is a mistake about the INVOCATION; every flag, argument and path here is "+
			"well-formed. The contract publishes this reclassification explicitly: a script branching on "+
			"2 for this case must branch on 1.", got, exitGeneric)
	}

	// Wrapped by an outer message (cobra/RunE chains do this) it must keep its
	// code — an errors.Is walk, never a top-level type check.
	wrapped := fmt.Errorf("app submit: %w", err)
	if got := exitCode(wrapped); got != exitGeneric {
		t.Errorf("wrapped: exitCode = %d, want %d", got, exitGeneric)
	}

	// NEGATIVE CONTROL: exitCode CAN return something other than exitGeneric, so
	// the assertion above is not a fact about a function that always says 1.
	if got := exitCode(civitai.Tag(civitai.ErrBadRequest, errors.New("bad enum"))); got != exitUsage {
		t.Fatalf("negative control: exitCode(ErrBadRequest) = %d, want %d — the instrument is not discriminating",
			got, exitUsage)
	}
}

// TestOversizeSentinelIsNotAnAPIKind pins the other half: the sentinel must not
// accidentally satisfy an API classification kind, which would silently move the
// refusal onto 2/3/4/5/6 — and 2 is the code the contract specifically promises
// it moved AWAY from, so a drift there re-breaks the migration notice rather
// than merely being wrong.
func TestOversizeSentinelIsNotAnAPIKind(t *testing.T) {
	err := fmt.Errorf("%w: submit body is too big", appapi.ErrBundleTooLarge)
	kinds := map[string]error{
		"ErrBadRequest":   civitai.ErrBadRequest,
		"ErrUnauthorized": civitai.ErrUnauthorized,
		"ErrNotFound":     civitai.ErrNotFound,
		"ErrRateLimited":  civitai.ErrRateLimited,
		"ErrNetwork":      civitai.ErrNetwork,
	}
	for name, kind := range kinds {
		if errors.Is(err, kind) {
			t.Errorf("a bundle-too-large error must not match civitai.%s — that would move its exit code", name)
		}
	}
	// POSITIVE CONTROL on the loop.
	if !errors.Is(civitai.Tag(civitai.ErrNotFound, errors.New("x")), kinds["ErrNotFound"]) {
		t.Fatal("the errors.Is walk cannot see a kind it should — the negatives above prove nothing")
	}
}
