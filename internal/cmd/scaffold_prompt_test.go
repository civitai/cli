package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestMain forces the interactive-stdin seam OFF by default so NO test can
// accidentally launch the huh scaffold form (which would block on terminal
// input when `go test` inherits a real TTY). Interactive-path tests opt back in
// explicitly by overriding stdinIsTTY + scaffoldPromptFn.
func TestMain(m *testing.M) {
	stdinIsTTY = func() bool { return false }
	os.Exit(m.Run())
}

// TestScaffoldNonInteractiveNoPrompt: with no name and a non-TTY stdin (the
// default), the huh form is NEVER invoked and the command errors asking for a
// name — the scripted/CI path is never blocked on a prompt.
func TestScaffoldNonInteractiveNoPrompt(t *testing.T) {
	orig := scaffoldPromptFn
	t.Cleanup(func() { scaffoldPromptFn = orig })
	scaffoldPromptFn = func(_ *cobra.Command, _ string) (scaffoldInputs, error) {
		t.Fatal("huh prompt must NOT run on a non-TTY stdin")
		return scaffoldInputs{}, nil
	}

	tmp := t.TempDir()
	chdir(t, tmp)
	if _, _, err := run(t, "app", "init"); err == nil {
		t.Fatal("expected an error when no name is given on a non-TTY")
	}
}

// TestScaffoldYesSkipsPromptEvenOnTTY: --yes forces non-interactive even when
// stdin IS a TTY — the prompt must not run, and a missing name errors.
func TestScaffoldYesSkipsPromptEvenOnTTY(t *testing.T) {
	origTTY := stdinIsTTY
	origPrompt := scaffoldPromptFn
	t.Cleanup(func() { stdinIsTTY = origTTY; scaffoldPromptFn = origPrompt })

	stdinIsTTY = func() bool { return true }
	scaffoldPromptFn = func(_ *cobra.Command, _ string) (scaffoldInputs, error) {
		t.Fatal("huh prompt must NOT run when --yes is set")
		return scaffoldInputs{}, nil
	}

	tmp := t.TempDir()
	chdir(t, tmp)
	if _, _, err := run(t, "app", "init", "--yes"); err == nil {
		t.Fatal("expected a missing-name error with --yes and no name")
	}
}

// TestScaffoldInteractiveUsesPromptValues: on an interactive TTY with no name,
// the (stubbed) prompt supplies the name + template and the scaffold is built
// from them.
func TestScaffoldInteractiveUsesPromptValues(t *testing.T) {
	origTTY := stdinIsTTY
	origPrompt := scaffoldPromptFn
	t.Cleanup(func() { stdinIsTTY = origTTY; scaffoldPromptFn = origPrompt })

	stdinIsTTY = func() bool { return true }
	scaffoldPromptFn = func(_ *cobra.Command, _ string) (scaffoldInputs, error) {
		return scaffoldInputs{name: "prompted-block", template: "static"}, nil
	}

	tmp := t.TempDir()
	chdir(t, tmp)
	stdout, _, err := run(t, "app", "init")
	if err != nil {
		t.Fatalf("interactive init: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "prompted-block") || !strings.Contains(stdout, "(static)") {
		t.Errorf("scaffold should use the prompted name + template:\n%s", stdout)
	}
	if _, statErr := os.Stat(filepath.Join(tmp, "prompted-block", "block.manifest.json")); statErr != nil {
		t.Errorf("scaffold dir not created from prompted name: %v", statErr)
	}
}

// TestScaffoldNameArgSkipsPrompt: a supplied positional name skips the prompt
// even on a TTY.
func TestScaffoldNameArgSkipsPrompt(t *testing.T) {
	origTTY := stdinIsTTY
	origPrompt := scaffoldPromptFn
	t.Cleanup(func() { stdinIsTTY = origTTY; scaffoldPromptFn = origPrompt })

	stdinIsTTY = func() bool { return true }
	scaffoldPromptFn = func(_ *cobra.Command, _ string) (scaffoldInputs, error) {
		t.Fatal("huh prompt must NOT run when a name is supplied")
		return scaffoldInputs{}, nil
	}

	tmp := t.TempDir()
	chdir(t, tmp)
	if _, _, err := run(t, "app", "init", "named-block"); err != nil {
		t.Fatalf("named init should not prompt: %v", err)
	}
}
