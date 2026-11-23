package config

import (
	"path/filepath"
	"testing"
)

// civitai/cli#965: on macOS os.UserConfigDir ignores XDG_CONFIG_HOME, so the
// hundreds of tests that point the config dir at a temp dir by setting
// XDG_CONFIG_HOME alone all shared ONE config dir there — files written where
// the test never looked, and one test's token leaking into the next. CI runs on
// Linux, where XDG is honoured, so this test stands macOS's rule in with
// platformConfigDir and checks what Dir() does INSIDE A TEST BINARY.

// darwinConfigDir is macOS's os.UserConfigDir rule: $HOME/Library/Application
// Support, XDG_CONFIG_HOME not consulted.
func darwinConfigDir(home string) func() (string, error) {
	return func() (string, error) { return filepath.Join(home, "Library", "Application Support"), nil }
}

func TestDirInATestBinaryHonoursXDGWhereThePlatformDoesNot(t *testing.T) {
	home, xdg := t.TempDir(), t.TempDir()
	prev := platformConfigDir
	platformConfigDir = darwinConfigDir(home)
	t.Cleanup(func() { platformConfigDir = prev })

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if want := filepath.Join(xdg, "civitai"); got != want {
		t.Errorf("Dir() = %q, want %q: under macOS's rule a test that sets XDG_CONFIG_HOME "+
			"must still get its own config dir", got, want)
	}
}

// TestDirInAShippedBinaryIgnoresXDGWhereThePlatformDoes pins the other half of
// the contract: OUTSIDE a test binary nothing changes, so a macOS user with
// XDG_CONFIG_HOME set keeps ~/Library/Application Support/civitai.
func TestDirInAShippedBinaryIgnoresXDGWhereThePlatformDoes(t *testing.T) {
	home, xdg := t.TempDir(), t.TempDir()
	prevDir, prevTB := platformConfigDir, inTestBinary
	platformConfigDir = darwinConfigDir(home)
	inTestBinary = func() bool { return false }
	t.Cleanup(func() { platformConfigDir, inTestBinary = prevDir, prevTB })

	t.Setenv("XDG_CONFIG_HOME", xdg)
	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if want := filepath.Join(home, "Library", "Application Support", "civitai"); got != want {
		t.Errorf("Dir() = %q, want the platform path %q: a shipped binary must not honour "+
			"XDG_CONFIG_HOME where the platform does not", got, want)
	}
}

// TestDirInATestBinaryFallsBackToThePlatformWithoutXDG: with XDG_CONFIG_HOME
// unset (or relative, which os.UserConfigDir also refuses) the platform rule
// still decides, so this is the macOS path a test without XDG sees.
func TestDirInATestBinaryFallsBackToThePlatformWithoutXDG(t *testing.T) {
	home := t.TempDir()
	prev := platformConfigDir
	platformConfigDir = darwinConfigDir(home)
	t.Cleanup(func() { platformConfigDir = prev })

	for _, xdg := range []string{"", "relative/dir"} {
		t.Setenv("XDG_CONFIG_HOME", xdg)
		got, err := Dir()
		if err != nil {
			t.Fatalf("Dir: %v", err)
		}
		if want := filepath.Join(home, "Library", "Application Support", "civitai"); got != want {
			t.Errorf("XDG_CONFIG_HOME=%q: Dir() = %q, want the platform path %q", xdg, got, want)
		}
	}
}
