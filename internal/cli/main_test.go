package cli

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// flakyEnv makes the test binary a flaky command for retry tests:
// "<counter file>:<n>" fails with exit 3 until its nth run, which succeeds.
const flakyEnv = "CARRIER_TEST_FLAKY"

// TestMain gives the tests their own temp directory: commands that relabel a
// session update the session files of recorded shells found there, and must
// not touch shells running on the machine.
func TestMain(m *testing.M) {
	if spec := os.Getenv(flakyEnv); spec != "" {
		os.Exit(flaky(spec))
	}
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

// flaky counts its runs in a file and fails until the nth.
func flaky(spec string) int {
	// The count follows the last colon: a Windows path has one of its own.
	i := strings.LastIndex(spec, ":")
	path, n := spec[:i], spec[i+1:]
	succeedOn, _ := strconv.Atoi(n)
	data, _ := os.ReadFile(path)
	runs, _ := strconv.Atoi(string(data))
	runs++
	_ = os.WriteFile(path, []byte(strconv.Itoa(runs)), 0o600)
	if succeedOn > 0 && runs >= succeedOn {
		return 0
	}
	return 3
}
