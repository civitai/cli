//go:build !windows

package cmd

import (
	"os/exec"
	"testing"
)

// TestDetachAndStartTrivial fire-and-forgets a trivial command (`true`, which
// exits immediately) and asserts Start() succeeded. The child is a harmless
// no-op that reaps itself; we never Wait on it.
func TestDetachAndStartTrivial(t *testing.T) {
	path, err := exec.LookPath("true")
	if err != nil {
		t.Skip("no `true` binary available")
	}
	if err := detachAndStart(exec.Command(path)); err != nil {
		t.Fatalf("detachAndStart(true): %v", err)
	}
}

// TestDetachAndStartMissingBinary surfaces the fork/exec failure for a
// nonexistent binary.
func TestDetachAndStartMissingBinary(t *testing.T) {
	if err := detachAndStart(exec.Command("/nonexistent/civitai-does-not-exist")); err == nil {
		t.Fatal("expected an error starting a nonexistent binary")
	}
}
