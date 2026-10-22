//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

// detach starts cmd in its own session, so it outlives the process that
// started it and never touches its terminal.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
