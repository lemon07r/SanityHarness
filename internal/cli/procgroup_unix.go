//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

// setupProcessGroup configures the command to run in its own process group so
// the entire tree can be killed on timeout or interrupt, preventing orphaned
// child processes.
func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// Kill the entire process group (negative PID).
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
}
