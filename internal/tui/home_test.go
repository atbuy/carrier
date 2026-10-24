package tui

import "testing"

// setHome points the home directory at dir for one test. os.UserHomeDir reads
// HOME on Unix and USERPROFILE on Windows, so both are set.
func setHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}
