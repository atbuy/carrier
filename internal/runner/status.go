package runner

import (
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

const (
	statusColorReset = "\x1b[0m"
	statusColorCyan  = "\x1b[36m"
	statusColorGreen = "\x1b[32m"
	statusColorGray  = "\x1b[90m"
)

func runStartedLine(w io.Writer, id int64) string {
	color := runnerShouldColor(w)
	return runnerPaint(color, statusColorGray, "carrier:") + " " +
		runnerPaint(color, statusColorGreen, "run") + " " +
		runnerPaint(color, statusColorCyan, strconvID(id)) + "\n"
}

func runnerPaint(enabled bool, color, text string) string {
	if !enabled {
		return text
	}
	return color + text + statusColorReset
}

func runnerShouldColor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	switch strings.ToLower(os.Getenv("CARRIER_COLOR")) {
	case "always", "1", "true":
		return true
	case "never", "0", "false":
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
