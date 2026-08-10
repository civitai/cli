package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// not_empty_remedy_test.go pins the REMEDY on the non-empty-directory refusal
// (issue #260, item 7) and, in the same file, the refusal itself.
//
// Both halves are needed and neither subsumes the other:
//
//   - Delete the remedy and the refusal tests stay green — the directory is
//     still not clobbered, the author just has nowhere to go.
//   - Delete the REFUSAL (scaffold on top of an existing tree) and a
//     message-only test stays green too, because there would be no message.
//
// The refusal half is labelled CONTROL: it passed before this change and is an
// invariant guard, not regression coverage. It is here because the remedy is
// only correct advice while the refusal is real — an author told "there is no
// --force" by a build that silently overwrites has been lied to twice.

// TestNotEmptyRemedyIsWellFormed is the precondition. Every assertion below
// derives its expectation from NotEmptyRemedy, and strings.Contains(x, "") is
// always true — an emptied constant would disarm the whole file silently.
func TestNotEmptyRemedyIsWellFormed(t *testing.T) {
	if strings.TrimSpace(NotEmptyRemedy) == "" {
		t.Fatal("NotEmptyRemedy is empty — every assertion in this file would be vacuous")
	}
	// The three things the remedy has to say, each because a reader who does
	// not get it takes a different wrong turn:
	//   - somewhere else / another name: the fix that needs no deletion
	//   - remove the directory:          the fix when the path is the one they want
	//   - no --force:                    stops the search for an override that
	//                                    does not exist, which is what made this
	//                                    a documentation-only remedy for so long
	for _, want := range []string{"--dir", "remove the directory", "no --force"} {
		if !strings.Contains(NotEmptyRemedy, want) {
			t.Errorf("NotEmptyRemedy no longer offers %q: %q", want, NotEmptyRemedy)
		}
	}
}

func TestRenderIntoNonEmptyDirNamesTheRemedy(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("mine\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	written, err := Render(Static, dir, Data{Slug: "ok-app", Name: "ok"})
	if err == nil {
		t.Fatal("Render must refuse a non-empty directory")
	}
	msg := err.Error()

	// It still names the directory — a remedy about "the directory" with no
	// path is not actionable when the path came from a default.
	if !strings.Contains(msg, dir) {
		t.Errorf("refusal does not name the directory:\n%s", msg)
	}
	// It still says WHAT happened, in the words it always used. The remedy is
	// appended to the refusal, not substituted for it.
	if !strings.Contains(msg, "is not empty") {
		t.Errorf("refusal lost its own reason:\n%s", msg)
	}
	// 🔴 The change: the CLI now carries the remedy the README already had.
	// Derived from the constant so a reword moves both together.
	if !strings.Contains(msg, NotEmptyRemedy) {
		t.Errorf("refusal carries no remedy\n  want: %s\n  got:  %s", NotEmptyRemedy, msg)
	}

	// CONTROL — the refusal is REAL. Nothing was written, and the file that was
	// already there is untouched.
	if len(written) != 0 {
		t.Errorf("Render reported %d written files while refusing: %v", len(written), written)
	}
	entries, rerr := os.ReadDir(dir)
	if rerr != nil {
		t.Fatalf("read back: %v", rerr)
	}
	if len(entries) != 1 || entries[0].Name() != "existing.txt" {
		t.Errorf("the directory was modified by a refused render: %v", entries)
	}
	body, rerr := os.ReadFile(filepath.Join(dir, "existing.txt"))
	if rerr != nil || string(body) != "mine\n" {
		t.Errorf("the pre-existing file changed: %q (%v)", body, rerr)
	}
}

// TestRenderIntoEmptyDirStillWorks is the POSITIVE CONTROL for the test above.
// "Render refuses and writes nothing" is also satisfied by a Render that
// refuses everything; this is what makes the refusal row evidence.
func TestRenderIntoEmptyDirStillWorks(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fresh")
	written, err := Render(Static, dir, Data{Slug: "ok-app", Name: "ok"})
	if err != nil {
		t.Fatalf("Render into a fresh directory: %v", err)
	}
	if len(written) == 0 {
		t.Fatal("Render wrote nothing into an empty directory — the control observes nothing")
	}
	if _, err := os.Stat(filepath.Join(dir, "block.manifest.json")); err != nil {
		t.Errorf("scaffold is missing its manifest: %v", err)
	}
}
