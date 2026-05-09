package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/user/carrier/internal/store"
)

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

func printRun(r *store.Run) {
	fmt.Printf("ID:       %d\n", r.ID)
	fmt.Printf("Status:   %s\n", r.Status)
	fmt.Printf("Command:  %s\n", r.Command)
	fmt.Printf("CWD:      %s\n", r.CWD)
	if r.ExitCode != nil {
		fmt.Printf("Exit:     %d\n", *r.ExitCode)
	}
	fmt.Printf("Duration: %s\n", formatDuration(r.DurationMS))
	fmt.Printf("Started:  %s\n", formatTime(r.StartedAt))
	if r.FinishedAt != nil {
		fmt.Printf("Finished: %s\n", formatTime(*r.FinishedAt))
	}
	if r.GitRoot != "" {
		fmt.Printf("Git:      %s %s %s dirty=%s\n", r.GitRoot, r.GitBranch, short(r.GitCommit), dirtyString(r.GitDirty))
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
