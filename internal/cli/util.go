package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/atbuy/carrier/internal/command"
	"github.com/atbuy/carrier/internal/store"
)

const fieldLabelWidth = 10

func parseID(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

func parseArgv(s string) ([]string, error) {
	var argv []string
	err := json.Unmarshal([]byte(s), &argv)
	return argv, err
}

func formatDuration(ms *int64) string {
	if ms == nil {
		return ""
	}
	return (time.Duration(*ms) * time.Millisecond).Round(10 * time.Millisecond).String()
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func printRun(w io.Writer, r *store.Run) {
	c := outputColors(w)
	printField(w, c, "ID", fmt.Sprintf("%d", r.ID), colorCyan)
	printField(w, c, "Status", r.Status, statusColor(r.Status))
	printField(w, c, "Command", displayCommand(r), colorGreen)
	printField(w, c, "CWD", r.CWD, colorGray)
	if r.ExitCode != nil {
		exitColor := colorGreen
		if *r.ExitCode != 0 {
			exitColor = colorRed
		}
		printField(w, c, "Exit", fmt.Sprintf("%d", *r.ExitCode), exitColor)
	}
	printField(w, c, "Duration", formatDuration(r.DurationMS), colorCyan)
	printField(w, c, "Started", formatTime(r.StartedAt), colorGray)
	if r.FinishedAt != nil {
		printField(w, c, "Finished", formatTime(*r.FinishedAt), colorGray)
	}
	if r.GitRoot != "" {
		printField(w, c, "Git", fmt.Sprintf("%s %s %s dirty=%s", r.GitRoot, r.GitBranch, short(r.GitCommit), dirtyString(r.GitDirty)), colorGray)
	}
}

func outputColors(w io.Writer) helpColors {
	return helpColors{enabled: shouldColor(w)}
}

func printField(w io.Writer, c helpColors, label, value, valueColor string) {
	if valueColor != "" {
		value = c.paint(valueColor, value)
	}
	_, _ = fmt.Fprintf(w, "%s%s\n", c.paint(colorBold+colorCyan, padRight(label+":", fieldLabelWidth)), value)
}

func statusColor(status string) string {
	switch status {
	case store.StatusSuccess:
		return colorGreen
	case store.StatusFailed, store.StatusKilled:
		return colorRed
	case store.StatusRunning:
		return colorYellow
	default:
		return colorGray
	}
}

func dirtyString(v *bool) string {
	if v == nil {
		return "unknown"
	}
	if *v {
		return "true"
	}
	return "false"
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func readText(path string) string {
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

func displayCommand(r *store.Run) string {
	if r.Mode != store.ModeShell {
		if argv, err := parseArgv(r.ArgvJSON); err == nil && len(argv) > 0 {
			return command.Display(argv)
		}
	}
	return r.Command
}
