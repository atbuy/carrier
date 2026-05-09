package logs

import (
	"fmt"
	"path/filepath"
)

func StdoutPath(dataDir string, id int64) string {
	return filepath.Join(dataDir, "runs", fmt.Sprintf("%06d.stdout.log", id))
}

func StderrPath(dataDir string, id int64) string {
	return filepath.Join(dataDir, "runs", fmt.Sprintf("%06d.stderr.log", id))
}

func TerminalPath(dataDir string, id int64) string {
	return filepath.Join(dataDir, "runs", fmt.Sprintf("%06d.terminal.log", id))
}
