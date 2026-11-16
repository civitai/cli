package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// RenderKeeping's two promises, at the package that makes them: a directory
// holding only kept files counts as empty, and nothing kept is ever written.
// The end-to-end half — the real `agent-setup` files, through `app create` —
// lives in internal/cmd/app_create_after_agent_setup_test.go.

func seed(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRenderKeepingLeavesKeptFilesByteIdentical(t *testing.T) {
	dir := t.TempDir()
	seed(t, filepath.Join(dir, "KEEP.md"), "kept one\n")
	seed(t, filepath.Join(dir, ".tool", "cfg.json"), "kept two\n")
	keep := [][]string{{"KEEP.md"}, {".tool", "cfg.json"}}

	written, err := RenderKeeping(Static, dir, Data{Slug: "ok-app", Name: "ok"}, keep)
	if err != nil {
		t.Fatalf("RenderKeeping refused a directory holding only kept files: %v", err)
	}
	if len(written) == 0 {
		t.Fatal("RenderKeeping reported success and wrote nothing")
	}
	for rel, want := range map[string]string{"KEEP.md": "kept one\n", ".tool/cfg.json": "kept two\n"} {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil || string(got) != want {
			t.Errorf("%s was not left as it was: %q (%v)", rel, got, err)
		}
	}
}

// TestRenderKeepingRefusesWhatKeepDoesNotCover: the same directory, with the
// keep list one entry short each time. Plain Render (no keep) is the first row —
// it must still refuse a kept-looking directory, because "kept" is the caller's
// claim and Render makes none.
func TestRenderKeepingRefusesWhatKeepDoesNotCover(t *testing.T) {
	cases := []struct {
		name string
		keep [][]string
	}{
		{"no keep list at all", nil},
		{"the top-level file is not kept", [][]string{{".tool", "cfg.json"}}},
		{"the nested file is not kept", [][]string{{"KEEP.md"}}},
		{"another file sits inside the kept directory", [][]string{{"KEEP.md"}, {".tool", "other.json"}}},
		{"the directory is kept as if it were a file", [][]string{{"KEEP.md"}, {".tool"}}},
		{"the file is kept as if it were a directory", [][]string{{"KEEP.md", "x"}, {".tool", "cfg.json"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			seed(t, filepath.Join(dir, "KEEP.md"), "kept one\n")
			seed(t, filepath.Join(dir, ".tool", "cfg.json"), "kept two\n")

			written, err := RenderKeeping(Static, dir, Data{Slug: "ok-app", Name: "ok"}, tc.keep)
			if err == nil {
				t.Fatalf("RenderKeeping scaffolded over a directory its keep list %v does not account for", tc.keep)
			}
			if want := dir + " is not empty — " + NotEmptyRemedy; err.Error() != want {
				t.Errorf("refusal\n got: %s\nwant: %s", err.Error(), want)
			}
			if len(written) != 0 {
				t.Errorf("a refused render reported written files: %v", written)
			}
			if _, statErr := os.Stat(filepath.Join(dir, "index.html")); statErr == nil {
				t.Error("a refused render wrote index.html")
			}
		})
	}
}

// TestRenderKeepingNeverWritesOverAKeptFile reaches the second guard directly:
// a keep list naming a path the template ITSELF writes. ensureEmptyDir accepts
// the directory (the file is kept), so the only thing between the template and
// the author's bytes is refuseIfOutputExists — the "already holds" in the
// message is what shows it was that guard and not the emptiness check.
func TestRenderKeepingNeverWritesOverAKeptFile(t *testing.T) {
	dir := t.TempDir()
	const mine = "<!-- mine, not the template's -->\n"
	seed(t, filepath.Join(dir, "index.html"), mine)

	written, err := RenderKeeping(Static, dir, Data{Slug: "ok-app", Name: "ok"}, [][]string{{"index.html"}})
	if err == nil {
		t.Fatal("RenderKeeping wrote a template over a file it was told to keep")
	}
	if !strings.Contains(err.Error(), "already holds index.html") {
		t.Errorf("refused, but not by the overwrite guard: %s", err.Error())
	}
	if len(written) != 0 {
		t.Errorf("a refused render reported written files: %v", written)
	}
	got, rerr := os.ReadFile(filepath.Join(dir, "index.html"))
	if rerr != nil || string(got) != mine {
		t.Errorf("the kept file changed: %q (%v)", got, rerr)
	}
	// Refused BEFORE the first write: nothing else from the template is there.
	entries, rerr := os.ReadDir(dir)
	if rerr != nil || len(entries) != 1 {
		t.Errorf("a refused render left a partial scaffold behind: %v (%v)", entries, rerr)
	}
}
