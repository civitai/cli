package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/cmd"
	"github.com/civitai/cli/pkg/civitai"
)

// THE EXIT CODE OF THE DIRTY-WORK-TREE REFUSAL (issue #411).
//
// The classification and the process contract are two different claims, and the
// second is the one scripts read: `cmd` can tag an error perfectly while
// `exitCode` routes it somewhere else.
//
// The verdict is 1, not 2, and it is the same DECISION #412's refusal made for
// the same reason — exit 2 is documented as a mistake about the INVOCATION, and
// every flag, argument and path in `civitai app submit --yes` is well-formed
// when this fires. What is wrong is the PROJECT: its working tree against its
// own history, the same shape exitCodeDocs publishes under code 1 as a
// validation verdict.
//
// 🔴 The refusal is left UNTAGGED for the exit mapper on purpose, so this test
// is what makes that silence deliberate rather than accidental: tagging it with
// civitai.ErrBadRequest (the only route from a command error to exit 2) would
// move it, and nothing else in the suite would notice.

// realDirtyRefusal runs the REAL `civitai app submit` against a REAL dirty git
// repository and returns the error the command produced.
//
// 🔴 IT EXISTS BECAUSE A HAND-BUILT `civitai.Tag(cmd.ErrDirtyWorkTree, …)`
// FIXTURE CANNOT SEE A WRONG SENTINEL. This test asserted the exit code of an
// error it constructed itself, so `dirtyWorkTreeError` tagging its refusal with
// something else — or forgetting to tag it — died only to the `internal/cmd`
// tests, and this package's claim ("the dirty refusal exits 1") stayed green
// while being about nothing. The classification and the process contract are
// the two halves of one seam; taking the real error is what joins them.
func realDirtyRefusal(t *testing.T) error {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH — the real-repo fixture cannot be built")
	}
	// Hermetic git: a developer's global core.excludesFile could otherwise hide
	// the uncommitted file and turn the refusal into a clean submit.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_AUTHOR_NAME", "cli-test")
	t.Setenv("GIT_AUTHOR_EMAIL", "cli-test@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "cli-test")
	t.Setenv("GIT_COMMITTER_EMAIL", "cli-test@example.invalid")

	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		c := exec.Command("git", append([]string{"-C", dir}, args...)...)
		var errb bytes.Buffer
		c.Stderr = &errb
		if err := c.Run(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, errb.String())
		}
	}
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "fixture-main")
	write("block.manifest.json", `{
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
}`)
	write("index.html", "<html></html>")
	git("add", "--", "block.manifest.json", "index.html")
	git("commit", "-q", "-m", "app")
	// The uncommitted bundle file the guard must refuse over.
	write("src/App.tsx", "export default 1")

	// 🔴 The server must never be reached: the guard runs before any upload. A
	// handler that fails the test is the reachability control — if the refusal
	// stopped firing, this would go red here rather than silently pass.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the dirty-tree guard must refuse before any request; got %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-test") // the guard only runs on the upload path
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "1")

	root := cmd.NewRootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs([]string{"app", "submit", dir, "--yes"})
	runErr := root.Execute()
	if runErr == nil {
		t.Fatalf("`app submit` must refuse a dirty tree; stdout:\n%s\nstderr:\n%s", out.String(), errb.String())
	}
	if !strings.Contains(runErr.Error(), "from a dirty git work tree") {
		t.Fatalf("the error is not the dirty-tree refusal (so this test would be about the wrong "+
			"error entirely): %v", runErr)
	}
	return runErr
}

func TestDirtyWorkTreeExitsGeneric(t *testing.T) {
	// THE REAL ERROR, from the real command — not a fixture this test tagged
	// itself, which could not see a wrong sentinel.
	err := realDirtyRefusal(t)

	if !errors.Is(err, cmd.ErrDirtyWorkTree) {
		t.Fatal("the REAL refusal does not carry ErrDirtyWorkTree — the exit-code claim below " +
			"would be about a sentinel nothing produces")
	}
	if got := exitCode(err); got != exitGeneric {
		t.Errorf("exitCode(dirty work tree) = %d, want %d (generic — a verdict about the project).\n"+
			"2 is a mistake about the INVOCATION; the flags and paths here are all well-formed.", got, exitGeneric)
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
		t.Fatalf("negative control: exitCode(ErrBadRequest) = %d, want %d — the instrument is not discriminating", got, exitUsage)
	}
}

// TestDirtyWorkTreeSentinelIsNotAnAPIKind pins the other half: the sentinel must
// not accidentally satisfy an API classification kind, which would silently move
// the refusal onto 2/3/4/5/6.
func TestDirtyWorkTreeSentinelIsNotAnAPIKind(t *testing.T) {
	err := civitai.Tag(cmd.ErrDirtyWorkTree, errors.New("refusing to submit demo@0.6.1"))
	kinds := map[string]error{
		"ErrBadRequest":   civitai.ErrBadRequest,
		"ErrUnauthorized": civitai.ErrUnauthorized,
		"ErrNotFound":     civitai.ErrNotFound,
		"ErrRateLimited":  civitai.ErrRateLimited,
		"ErrNetwork":      civitai.ErrNetwork,
	}
	for name, kind := range kinds {
		if errors.Is(err, kind) {
			t.Errorf("a dirty-work-tree error must not match civitai.%s — that would move its exit code", name)
		}
	}
	// POSITIVE CONTROL on the loop.
	if !errors.Is(civitai.Tag(civitai.ErrNotFound, errors.New("x")), kinds["ErrNotFound"]) {
		t.Fatal("the errors.Is walk cannot see a kind it should — the negatives above prove nothing")
	}
}

// TestDirtyWorkTreeAndVersionRegressionAreDistinctSentinels — the two submit
// refusals must be separately identifiable. A single shared sentinel (or one
// wrapping the other) would make `errors.Is` unable to tell a release script
// which escape hatch applies: --allow-dirty and --allow-downgrade are not
// interchangeable.
func TestDirtyWorkTreeAndVersionRegressionAreDistinctSentinels(t *testing.T) {
	dirty := civitai.Tag(cmd.ErrDirtyWorkTree, errors.New("dirty"))
	regression := civitai.Tag(cmd.ErrVersionRegression, errors.New("regression"))

	if errors.Is(dirty, cmd.ErrVersionRegression) {
		t.Error("a dirty-tree refusal must not read as a version regression")
	}
	if errors.Is(regression, cmd.ErrDirtyWorkTree) {
		t.Error("a version regression must not read as a dirty-tree refusal")
	}
	// Both still land on the same exit code, and that is deliberate: they are
	// the same KIND of verdict, distinguished by sentinel rather than by code.
	if exitCode(dirty) != exitCode(regression) {
		t.Errorf("both submit refusals are project verdicts and share exit %d; got dirty=%d regression=%d",
			exitGeneric, exitCode(dirty), exitCode(regression))
	}
}
