//go:build !windows

package cmd

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The hardening of the guard's one network call (`git ls-remote`), checked
// against the real runner, gitOutput. Unix-only: the fixtures are shell scripts
// and the process-group kill is unix behaviour.

func writeScript(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestLsRemoteNeverRunsAskpass — an https remote that answers 401 makes git ask
// for a username. GIT_TERMINAL_PROMPT=0 alone does not stop git from running
// GIT_ASKPASS / SSH_ASKPASS for it (a dialog in VS Code or on a desktop); the
// positive control proves this fixture really reaches askpass, and the real
// runner must not.
func TestLsRemoteNeverRunsAskpass(t *testing.T) {
	requireGit(t)
	gitFixtureEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="fixture"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	url := srv.URL + "/repo.git"

	marker := filepath.Join(t.TempDir(), "askpass-ran")
	askpass := writeScript(t, "askpass", "echo ran >> '"+marker+"'\necho fixture-user\n")

	// POSITIVE CONTROL: plain git with only GIT_TERMINAL_PROMPT=0 does run it.
	ctl := exec.Command("git", "ls-remote", url)
	ctl.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS="+askpass)
	_ = ctl.Run()
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("control: git never ran GIT_ASKPASS for a 401 remote, so this fixture cannot see the hazard (%v)", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GIT_ASKPASS", askpass)
	t.Setenv("SSH_ASKPASS", askpass)
	if _, err := gitOutput(t.TempDir(), "ls-remote", url); err == nil {
		t.Fatal("a 401 remote with no credentials must fail the call")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the guard's ls-remote ran the user's askpass program; it must never prompt")
	}
}

// TestLsRemoteTimeoutKillsTheSshChild — a remote whose ssh hangs. The call must
// return at its timeout AND take the hung ssh with it: the default cancel kills
// only git, which leaves the child running under init.
func TestLsRemoteTimeoutKillsTheSshChild(t *testing.T) {
	requireGit(t)
	gitFixtureEnv(t)
	prev := gitNetworkTimeout
	gitNetworkTimeout = time.Second
	t.Cleanup(func() { gitNetworkTimeout = prev })

	pidFile := filepath.Join(t.TempDir(), "ssh.pid")
	hung := writeScript(t, "hung-ssh", "echo $$ > '"+pidFile+"'\nexec sleep 30\n")
	t.Setenv("GIT_SSH_COMMAND", hung)

	start := time.Now()
	if _, err := gitOutput(t.TempDir(), "ls-remote", "ssh://fixture.invalid/repo.git"); err == nil {
		t.Fatal("a hung ssh must make the call fail")
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Errorf("the call took %v; the timeout is %v", took, gitNetworkTimeout)
	}

	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("control: the fake ssh never ran, so nothing here was tested (%v)", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("pid file: %q", raw)
	}
	deadline := time.Now().Add(5 * time.Second)
	for processAlive(pid) {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("the hung ssh child (pid %d) outlived the timed-out call", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// processAlive reports whether pid exists and is not a zombie waiting to be
// reaped (a killed process whose new parent has not collected it yet).
func processAlive(pid int) bool {
	if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
		return false
	}
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		// No /proc (macOS): signal 0 succeeding is the only evidence there is.
		return true
	}
	// The state field follows the parenthesised command name.
	if i := strings.LastIndexByte(string(stat), ')'); i >= 0 && i+2 < len(stat) {
		return stat[i+2] != 'Z'
	}
	return true
}
