package cli

import (
	"os"
	"testing"
)

// TestMain gives the tests their own temp directory: commands that relabel a
// session update the session files of recorded shells found there, and must
// not touch shells running on the machine.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "carrier-cli-test-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("TMPDIR", dir)
	// The executable is the test binary, so metadata is collected inline.
	_ = os.Setenv(backfillInlineEnv, "1")
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
