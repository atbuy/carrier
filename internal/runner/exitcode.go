package runner

import (
	"os/exec"
	"syscall"
)

func ExitCode(err error) (int, string) {
	if err == nil {
		return 0, "success"
	}
	if ee, ok := err.(*exec.ExitError); ok {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok {
			if ws.Signaled() {
				return 128 + int(ws.Signal()), "killed"
			}
			return ws.ExitStatus(), "failed"
		}
	}
	return 1, "unknown"
}
