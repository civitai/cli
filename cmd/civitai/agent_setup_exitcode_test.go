package main

// THE EXIT CODES OF `civitai agent-setup`, pinned at BOTH levels.
//
// 🔴 THE MAPPER AND THE COMMAND ARE TWO DIFFERENT CLAIMS, and the second is the
// one a script reads. `internal/cmd` can classify the verdict perfectly while
// `exitCode` routes it somewhere else — the seam `doctor_e2e_exitcode_test.go`
// in this package exists to close for a different path, and which a mutation
// battery caught leaving the whole suite green. So the sentinel is exercised
// through the mapper AND the real binary is driven to its process status.
//
// 🔴 THE ROW THAT MATTERS MOST IS `--check` ON A COMPLETE BUT UNAUTHENTICATED
// SETUP. It must exit 0. Setup stops before auth on purpose, so that is exactly
// the state the documented flow leaves a user in; an implementation that folded
// `authenticated` into the verdict would exit 1 for every one of them, and
// developer.civitai.com's setup prompt — instructed not to report success when a
// check fails — would report a failure for a run that did everything right.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/cmd"
	"github.com/civitai/cli/pkg/civitai"
)

func TestAgentSetupCheckFailureExitsGeneric(t *testing.T) {
	err := fmt.Errorf("%w: 3 check(s) failed — the report above lists them", cmd.ErrAgentSetupIncomplete)

	if !errors.Is(err, cmd.ErrAgentSetupIncomplete) {
		t.Fatal("fixture is not tagged with ErrAgentSetupIncomplete — the test would prove nothing")
	}
	if got := exitCode(err); got != exitGeneric {
		t.Errorf("exitCode(agent-setup --check verdict) = %d, want %d (generic — a verdict about the SETUP).\n"+
			"2 is a mistake about the INVOCATION; every flag here is well-formed.", got, exitGeneric)
	}
	// Wrapped by an outer message it must keep its code — an errors.Is walk,
	// never a top-level type check.
	if got := exitCode(fmt.Errorf("agent-setup: %w", err)); got != exitGeneric {
		t.Errorf("wrapped: exitCode = %d, want %d", got, exitGeneric)
	}
	// NEGATIVE CONTROL: exitCode CAN return something else, so the assertion
	// above is not a fact about a function that always says 1.
	if got := exitCode(civitai.Tag(civitai.ErrBadRequest, errors.New("bad enum"))); got != exitUsage {
		t.Fatalf("negative control: exitCode(ErrBadRequest) = %d, want %d — the instrument is not discriminating",
			got, exitUsage)
	}
}

// TestAgentSetupSentinelIsNotAnAPIKind pins the other half: the sentinel must not
// accidentally satisfy an API classification kind, which would silently move the
// verdict onto 2/3/4/5/6.
func TestAgentSetupSentinelIsNotAnAPIKind(t *testing.T) {
	err := fmt.Errorf("%w: 1 check(s) failed", cmd.ErrAgentSetupIncomplete)
	kinds := map[string]error{
		"ErrBadRequest":   civitai.ErrBadRequest,
		"ErrUnauthorized": civitai.ErrUnauthorized,
		"ErrNotFound":     civitai.ErrNotFound,
		"ErrRateLimited":  civitai.ErrRateLimited,
		"ErrNetwork":      civitai.ErrNetwork,
	}
	for name, kind := range kinds {
		if errors.Is(err, kind) {
			t.Errorf("the agent-setup verdict must not match civitai.%s — that would move its exit code", name)
		}
	}
	if errors.Is(err, cmd.ErrUsage) {
		t.Error("the agent-setup verdict must not match cmd.ErrUsage — an incomplete setup is not a bad invocation")
	}
	// POSITIVE CONTROL on the loop: an error that IS one of these kinds must be
	// seen, or the negatives above are a fact about a walk that matches nothing.
	if !errors.Is(civitai.Tag(civitai.ErrNotFound, errors.New("x")), kinds["ErrNotFound"]) {
		t.Fatal("the errors.Is walk cannot see a kind it should — the negatives above prove nothing")
	}
}

// TestAgentSetupProcessExitStatusEndToEnd drives the real binary and reads the
// process's own status — the number a setup script branches on.
func TestAgentSetupProcessExitStatusEndToEnd(t *testing.T) {
	bin := buildCLI(t)

	// A directory that is really a regular file, for the --dir classification.
	fileRoot := t.TempDir()
	notADir := filepath.Join(fileRoot, "notadir.txt")
	if err := os.WriteFile(notADir, []byte("hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	env := func(home string) []string {
		return []string{
			"HOME=" + home,
			"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
			"CIVITAI_TOKEN=",
			"CIVITAI_NO_UPDATE_CHECK=1",
			"NO_COLOR=1",
		}
	}

	t.Run("a write run succeeds", func(t *testing.T) {
		dir, home := t.TempDir(), t.TempDir()
		rc, stdout, stderr := runCLIAnyStatus(t, bin, env(home),
			"agent-setup", "--dir", dir, "--agent", "claude")
		if rc != 0 {
			t.Fatalf("a write run exited %d, want 0.\nstdout:\n%s\nstderr:\n%s", rc, stdout, stderr)
		}
		if !strings.Contains(stdout, "civitai login") {
			t.Errorf("the next-step block must end by naming `civitai login` — this command never runs it:\n%s", stdout)
		}
	})

	t.Run("--check on an empty directory exits 1", func(t *testing.T) {
		dir, home := t.TempDir(), t.TempDir()
		rc, stdout, _ := runCLIAnyStatus(t, bin, env(home),
			"agent-setup", "--dir", dir, "--agent", "claude", "--check")
		if rc != exitGeneric {
			t.Errorf("--check on an unconfigured project exited %d, want %d.\n%s", rc, exitGeneric, stdout)
		}
	})

	// 🔴 THE INVERSION ROW, AT THE PROCESS LEVEL.
	t.Run("--check on a complete but UNAUTHENTICATED setup exits 0", func(t *testing.T) {
		dir, home := t.TempDir(), t.TempDir()
		if rc, _, stderr := runCLIAnyStatus(t, bin, env(home),
			"agent-setup", "--dir", dir, "--agent", "claude"); rc != 0 {
			t.Fatalf("PREMISE BROKEN: the setup run itself exited %d: %s", rc, stderr)
		}
		rc, stdout, stderr := runCLIAnyStatus(t, bin, env(home),
			"agent-setup", "--dir", dir, "--agent", "claude", "--check", "--json")
		if rc != 0 {
			t.Errorf("--check exited %d, want 0. A fresh, correct, UNAUTHENTICATED setup is a SUCCESS — "+
				"`authenticated` is reported and must never fail the verdict.\nstdout:\n%s\nstderr:\n%s",
				rc, stdout, stderr)
		}
		// PREMISE: the run really was unauthenticated, or this row is the
		// authenticated case wearing the unauthenticated one's name.
		if !strings.Contains(stdout, "no token") {
			t.Errorf("PREMISE BROKEN: the payload does not report a missing token, so this row is not "+
				"exercising the unauthenticated path:\n%s", stdout)
		}
		if !strings.Contains(stdout, `"ok": true`) {
			t.Errorf("the payload must report ok:true:\n%s", stdout)
		}
	})

	for _, tc := range []struct {
		name string
		args []string
		why  string
	}{
		{"--track api", []string{"agent-setup", "--track", "api"}, "a deferred track is a mistake about the invocation"},
		{"unknown --agent", []string{"agent-setup", "--agent", "vs-code"}, "an unknown flag value is a usage error"},
		{"--dir that does not exist", []string{"agent-setup", "--dir", filepath.Join(fileRoot, "nope")},
			"a path that is not there is a mistake about the invocation"},
		{"--dir that is a file", []string{"agent-setup", "--dir", notADir},
			"pointing at a file instead of a directory is the same mistake"},
	} {
		t.Run(tc.name+" exits 2", func(t *testing.T) {
			home := t.TempDir()
			rc, stdout, stderr := runCLIAnyStatus(t, bin, env(home), tc.args...)
			if rc != exitUsage {
				t.Errorf("`civitai %s` exited %d, want %d — %s.\nstdout:\n%s\nstderr:\n%s",
					strings.Join(tc.args, " "), rc, exitUsage, tc.why, stdout, stderr)
			}
		})
	}
}
