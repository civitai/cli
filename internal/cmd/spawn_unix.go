//go:build !windows

package cmd

import (
	"os"
	"os/exec"
	"syscall"
)

// detachAndStart launches cmd as a detached background process and returns
// immediately WITHOUT waiting for it. On unix we put it in its own process
// group (Setpgid) so it survives the parent exiting and isn't killed by a
// signal sent to the parent's group. stdio is redirected to /dev/null so it
// can never write to the user's terminal.
//
// The returned error reflects only the Start() (fork/exec) failure; we never
// Wait on the child, so the parent returns instantly.
func detachAndStart(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err == nil {
		cmd.Stdin = devnull
		cmd.Stdout = devnull
		cmd.Stderr = devnull
	}
	startErr := cmd.Start()
	// The child has inherited the fd; close our copy so we don't leak it.
	if devnull != nil {
		_ = devnull.Close()
	}
	// Deliberately NOT calling cmd.Wait() — fire and forget. The child reaps as
	// an orphan once the parent exits.
	return startErr
}

// detachFromTerminal runs cmd in a new session, so it has no controlling
// terminal. Used for the guard's `git ls-remote` (app_submit_dirty_guard.go):
// ssh reads a passphrase or a host-key confirmation from /dev/tty, not stdin, so
// without this a local-only commit would stop `app submit` on a prompt the user
// never asked for. With no tty the prompt fails, the call fails, and the guard
// keeps its pre-existing warning.
func detachFromTerminal(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
