package shell

import "syscall"

// ttyProcAttr has no Windows equivalent; the tests that start a shell under a
// terminal skip themselves on Windows.
func ttyProcAttr() *syscall.SysProcAttr { return nil }
