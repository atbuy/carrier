//go:build !windows

package shell

import "syscall"

// ttyProcAttr starts a shell as the leader of a new session with its
// terminal as the controlling one, as a terminal emulator does.
func ttyProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true, Setctty: true}
}
