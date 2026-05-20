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

func printRun(w io.Writer, r *store.Run) {
	t := newTheme(w)
	printField(w, t, "ID", fmt.Sprintf("%d", r.ID), t.ID)
	printField(w, t, "Status", r.Status, t.statusStyle(r.Status))
	printField(w, t, "Command", displayCommand(r), t.Command)
	printField(w, t, "CWD", r.CWD, t.Muted)
	if r.ExitCode != nil {
		exitStyle := t.Success
		if *r.ExitCode != 0 {
			exitStyle = t.Danger
		}
		printField(w, t, "Exit", fmt.Sprintf("%d", *r.ExitCode), exitStyle)
	}
	printField(w, t, "Duration", formatDuration(r.DurationMS), t.Muted)
	printField(w, t, "Started", formatTime(r.StartedAt), t.Muted)
	if r.FinishedAt != nil {
		printField(w, t, "Finished", formatTime(*r.FinishedAt), t.Muted)
	}
	if r.GitRoot != "" {
		printField(w, t, "Git", fmt.Sprintf("%s %s %s dirty=%s", r.GitRoot, r.GitBranch, short(r.GitCommit), dirtyString(r.GitDirty)), t.Muted)
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
