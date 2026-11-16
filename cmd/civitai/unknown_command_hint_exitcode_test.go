package main

// THE UNKNOWN-COMMAND UPGRADE HINT, MEASURED ON THE BUILT BINARY.
//
// internal/cmd pins the error TEXT. What a user or agent sees is the process:
// which stream the hint lands on, that stdout stays empty, and that the exit
// status is still the usage code. Only the built binary (main's errorLine +
// exitCode) can answer those, so this builds one with the version stamped the
// way goreleaser stamps it.

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestUnknownCommandUpgradeHintOnBinary(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "civitai")
	build := exec.Command("go", "build", "-ldflags", "-X main.version=0.1.125", "-o", bin, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the CLI failed: %v\n%s", err, out)
	}

	cmd := exec.Command(bin, "feedbackxyz", "send", "--about", "x")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "CIVITAI_NO_UPDATE_CHECK=1"}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()

	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected a non-zero exit, got err=%v", err)
	}
	if got := ee.ExitCode(); got != exitUsage {
		t.Errorf("exit status: got %d, want %d (usage) — the hint must not change it", got, exitUsage)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout must stay empty, got %q", stdout.String())
	}
	want := `Error: unknown command "feedbackxyz" for "civitai"` + "\n" +
		"Run 'civitai --help' for the available subcommands.\n" +
		`"civitai feedbackxyz" is not in this CLI (civitai 0.1.125); if a doc or prompt told you to run it, ` +
		"this CLI may be older than that command.\n" +
		"Upgrade with 'civitai upgrade', or get the latest release at " +
		"https://github.com/civitai/cli/releases/latest\n"
	if got := stderr.String(); got != want {
		t.Errorf("stderr mismatch\n got: %q\nwant: %q", got, want)
	}
}
