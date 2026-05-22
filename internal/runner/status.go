package runner

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"

	"github.com/atbuy/carrier/internal/config"
)

const statusColorReset = "\x1b[0m"

func runStartedLine(w io.Writer, id int64, theme config.ThemeColors) string {
	color := runnerShouldColor(w)
	return runnerPaint(color, runnerANSIColor(theme.Muted), "carrier:") + " " +
		runnerPaint(color, runnerANSIColor(theme.Success), "run") + " " +
		runnerPaint(color, runnerANSIColor(theme.Accent), strconvID(id)) + "\n"
}

func runnerPaint(enabled bool, color, text string) string {
	if !enabled || color == "" {
		return text
	}
	return color + text + statusColorReset
}

func runnerANSIColor(color string) string {
	color = strings.TrimSpace(color)
	if color == "" {
		return ""
	}
	if r, g, b, ok := parseRunnerHexColor(color); ok {
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
	}
	if n, err := strconv.Atoi(color); err == nil && n >= 0 && n <= 255 {
		return fmt.Sprintf("\x1b[38;5;%dm", n)
	}
	if code, ok := runnerANSIColorCode(strings.ToLower(color)); ok {
		return fmt.Sprintf("\x1b[%dm", code)
	}
	return ""
}

func parseRunnerHexColor(color string) (int64, int64, int64, bool) {
	if len(color) != 7 || color[0] != '#' {
		return 0, 0, 0, false
	}
	r, e1 := strconv.ParseInt(color[1:3], 16, 0)
	g, e2 := strconv.ParseInt(color[3:5], 16, 0)
	b, e3 := strconv.ParseInt(color[5:7], 16, 0)
	return r, g, b, e1 == nil && e2 == nil && e3 == nil
}

func runnerANSIColorCode(name string) (int, bool) {
	codes := map[string]int{
		"black": 30, "red": 31, "green": 32, "yellow": 33, "blue": 34, "magenta": 35, "cyan": 36, "white": 37,
		"gray": 90, "grey": 90, "brightblack": 90, "brightred": 91, "brightgreen": 92, "brightyellow": 93,
		"brightblue": 94, "brightmagenta": 95, "brightcyan": 96, "brightwhite": 97,
	}
	code, ok := codes[name]
	return code, ok
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
