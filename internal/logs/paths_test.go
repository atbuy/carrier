package logs

import (
	"path/filepath"
	"testing"
)

func TestRunLogPaths(t *testing.T) {
	dataDir := "/tmp/carrier"

	tests := map[string]string{
		StdoutPath(dataDir, 7):   filepath.Join(dataDir, "runs", "000007.stdout.log"),
		StderrPath(dataDir, 7):   filepath.Join(dataDir, "runs", "000007.stderr.log"),
		TerminalPath(dataDir, 7): filepath.Join(dataDir, "runs", "000007.terminal.log"),
	}

	for got, want := range tests {
		if got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
	}
}
