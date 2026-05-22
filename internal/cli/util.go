package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

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

// formatRelativeTime renders an approximate "time ago" string for list views.
// Returns "" for the zero time. Future times are clamped to "just now".
func formatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < 5*time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d.Hours()/(24*7)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy ago", int(d.Hours()/(24*365)))
	}
}

// timeWithRelative pairs an absolute timestamp with its relative form, e.g.
// "2026-05-22 14:03:01 (2m ago)". Used in detail views.
func timeWithRelative(ts time.Time) string {
	abs := formatTime(ts)
	if abs == "" {
		return ""
	}
	if rel := formatRelativeTime(ts); rel != "" {
		return abs + " (" + rel + ")"
	}
	return abs
}

// statusGlyph returns a single-width icon for a run status.
func statusGlyph(status string) string {
	switch status {
	case store.StatusSuccess:
		return "✓"
	case store.StatusFailed:
		return "✗"
	case store.StatusRunning:
		return "●"
	case store.StatusKilled:
		return "⊘"
	default:
		return "•"
	}
}

// renderStatus returns the colored glyph plus status word, with the word padded
// to pad columns for alignment in list views (pass 0 for no padding).
func renderStatus(t theme, status string, pad int) string {
	return t.statusStyle(status).Render(statusGlyph(status) + " " + padRight(status, pad))
}

// collapseHome rewrites a path under the user's home directory to start with ~.
func collapseHome(path string) string {
	if path == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	sep := string(os.PathSeparator)
	if strings.HasPrefix(path, home+sep) {
		return "~" + path[len(home):]
	}
	return path
}

func printRun(w io.Writer, r *store.Run) {
	t := newTheme(w)
	printField(w, t, "ID", fmt.Sprintf("%d", r.ID), t.ID)
	printField(w, t, "Status", statusGlyph(r.Status)+" "+r.Status, t.statusStyle(r.Status))
	printField(w, t, "Command", displayCommand(r), t.Command)
	printField(w, t, "CWD", collapseHome(r.CWD), t.Muted)
	if r.ExitCode != nil {
		exitStyle := t.Success
		if *r.ExitCode != 0 {
			exitStyle = t.Danger
		}
		printField(w, t, "Exit", fmt.Sprintf("%d", *r.ExitCode), exitStyle)
	}
	printField(w, t, "Duration", formatDuration(r.DurationMS), t.Muted)
	printField(w, t, "Started", timeWithRelative(r.StartedAt), t.Muted)
	if r.FinishedAt != nil {
		printField(w, t, "Finished", timeWithRelative(*r.FinishedAt), t.Muted)
	}
	if r.GitRoot != "" {
		printField(w, t, "Git", fmt.Sprintf("%s %s %s dirty=%s", collapseHome(r.GitRoot), r.GitBranch, short(r.GitCommit), dirtyString(r.GitDirty)), t.Muted)
	}
	if r.Label != "" {
		printField(w, t, "Label", r.Label, t.Label)
	}
}

func printField(w io.Writer, t theme, label, value string, style lipgloss.Style) {
	_, _ = fmt.Fprintf(w, "%s%s\n", t.Bold.Render(padRight(label+":", fieldLabelWidth)), style.Render(value))
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

// vtResponseRE matches terminal sequences that should be stripped before
// replaying captured PTY output:
//   - Terminal query responses (DA, DECRQM, OSC color, XTVERSION/DCS, APC)
//     that flow back via PTY echo from the outer terminal.
//   - Mode-switching private DEC modes (alt-screen, bracketed paste, cursor
//     keys, mouse, etc.) that would mutate the user's terminal state if
//     replayed — e.g. a TUI using ?1049h/l would flash an alternate screen.
var vtResponseRE = regexp.MustCompile(
	`\x1b(?:` +
		`P[^\x1b]*\x1b\\` + // DCS ... ST  (XTVERSION: "ghostty 1.3.1", etc.)
		`|_[^\x1b]*\x1b\\` + // APC ... ST
		`|\[[?>][\d;]*c` + // CSI DA1/DA2 response  (ESC[?1;2;4c, ESC[>1;2;4c)
		`|\[[?>]?[\d;]*\$y` + // CSI DECRQM response   (ESC[?2031;2$y)
		`|\]1[0-9];(?:rgba?:[0-9a-fA-F/:,]+|#[0-9a-fA-F]+)(?:\x07|\x1b\\)` + // OSC 10-19 color response
		`|\[\?[\d;]+[hl]` + // CSI ? Pm h/l — DEC private mode set/reset (alt-screen, mouse, bracketed-paste, app-cursor/keypad, etc.)
		`)`,
)

// stripVTResponses removes terminal query responses and mode-switching
// sequences from s before display. Applied to terminal log content to prevent
// escape sequences captured via PTY from being replayed to the user's terminal.
func stripVTResponses(s string) string {
	return vtResponseRE.ReplaceAllString(s, "")
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

// containsTUIOutput reports whether s contains an alternate-screen enter
// sequence, which indicates the log was captured from a TUI application.
// Such logs cannot be safely replayed as plain text.
func containsTUIOutput(s string) bool {
	return strings.Contains(s, "\x1b[?1049h") ||
		strings.Contains(s, "\x1b[?47h") ||
		strings.Contains(s, "\x1b[?1047h")
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
