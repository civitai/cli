//go:build windows

package cmd

import (
	"os"
	"os/exec"
	"syscall"
)

// detachAndStart launches cmd as a detached background process on Windows and
// returns immediately WITHOUT waiting. CREATE_NEW_PROCESS_GROUP detaches the
// child from the parent's console process group so it survives the parent
// exiting; stdio is redirected to NUL so it can never write to the console.
func detachAndStart(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err == nil {
		cmd.Stdin = devnull
		cmd.Stdout = devnull
		cmd.Stderr = devnull
	}
	startErr := cmd.Start()
	if devnull != nil {
		_ = devnull.Close()
	}
	// Fire and forget — never Wait.
	return startErr
}
