package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestWriteProjectFileFollowsASymlinkRatherThanReplacingIt is the BEHAVIOURAL
// half of the atomic-write symlink property, and it sits on the HELPER rather
// than on any one caller.
//
// 🔴 MEASURED RED, ON A REAL CONFIG: `~/.codex/config.toml ->
// ~/dotfiles/codex.toml` came back a REGULAR FILE at exit 0, with the dotfiles
// copy orphaned and untouched — so every later edit to the dotfiles repo was
// invisible, and the user's version control no longer tracked the live file. A
// rename onto a link destroys the link; `resolveWriteTarget` is what makes the
// rename land on the destination instead.
//
// 🔴 WHY IT IS HERE AND NOT IN A CALLER'S TEST FILE. The property belongs to
// writeProjectFile -> writeFileAtomic -> resolveWriteTarget, and it is identical
// for every caller, so a copy per caller is the same proof re-run with a
// different fixture: it grows with the caller set and still cannot report a
// caller that has NO coverage. The caller SET is asserted structurally instead,
// by TestWriteHelperCallersAreLedgered at the module root. This test is the half
// that ledger cannot be: a structural check type-checks past a wrong argument
// and never observes a byte being written.
func TestWriteProjectFileFollowsASymlinkRatherThanReplacingIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture")
	}
	root := t.TempDir()

	real := filepath.Join(root, "dotfiles", "profile")
	if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
		t.Fatal(err)
	}
	const owned = "# managed in my dotfiles repo\nexport EDITOR=vi\n"
	if err := os.WriteFile(real, []byte(owned), 0o644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(root, "home", ".profile")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	const added = "# BEGIN marker\nPATH=/x:$PATH\n# END marker\n"
	if err := writeProjectFile(link, owned+added); err != nil {
		t.Fatalf("writeProjectFile through a symlink: %v", err)
	}

	// 1 — the link is still a link. This is the assertion the measured defect
	// violated, and it is checked with Lstat because Stat follows the link and
	// cannot tell the two outcomes apart.
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat %s: %v", link, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("%s is no longer a symlink — the write replaced the link with a regular file, "+
			"orphaning %s and making every future edit to the dotfiles copy invisible", link, real)
	}

	// 2 — the bytes reached the DESTINATION, not a new file at the link's path.
	// Without this, a write that deleted the link and wrote nowhere useful could
	// still satisfy (1) on some orderings.
	got, err := os.ReadFile(real)
	if err != nil {
		t.Fatalf("read %s: %v", real, err)
	}
	if !strings.Contains(string(got), "# BEGIN marker") {
		t.Errorf("the real file %s did not gain the written content:\n%s", real, got)
	}
	// 3 — and it did not eat the user's own lines.
	if !strings.HasPrefix(string(got), "# managed in my dotfiles repo\n") {
		t.Errorf("the real file %s lost its own first line:\n%s", real, got)
	}
}

// TestWriteProjectFileRefusesABrokenSymlinkByName is the other arm, and it is
// the reason "follow the link" is not implemented as "resolve and hope".
//
// Following a link must not degrade into materialising a file where the user
// pointed one somewhere else: a broken link means the destination is absent, and
// writing it would create a file in a directory the user never chose. The
// refusal names the symlink so the operator can see what happened.
func TestWriteProjectFileRefusesABrokenSymlinkByName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture")
	}
	root := t.TempDir()
	link := filepath.Join(root, "home", ".profile")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "gone", "profile"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	err := writeProjectFile(link, "anything\n")
	if err == nil {
		t.Fatal("a broken symlink was silently replaced with a regular file")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("the refusal does not name the symlink: %v", err)
	}
}
