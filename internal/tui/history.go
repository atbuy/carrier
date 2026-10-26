package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

type historyScope int

const (
	scopeAll historyScope = iota
	scopeHere
	scopeSession
)

// History is what the history search lists: the commands, and where it was
// opened, for the "here" and "this session" scopes.
type History struct {
	Commands []store.CommandChoice
	// Labels names the sessions the commands ran in.
	Labels    map[int64]string
	CWD       string
	SessionID int64
	// Output returns the last lines of a run's output for the preview, or
	// nil when there is none. Without it there is no preview.
	Output func(r *store.Run, lines int) []string
}

// previewLines is the most output lines the preview shows.
const previewLines = 6

// previewHeight is the lines the preview takes: a rule, the run's details,
// and its output. It keeps its height as the selection moves, so the screen
// does not jump.
const previewHeight = 2 + previewLines

// previewMinRows is the fewest commands the list keeps; a terminal too short
// for them and the preview shows no preview.
const previewMinRows = 5

// HistoryModel is the bubbletea model of history search: a list of commands
// from every terminal, filtered by what is typed, that returns the one picked.
type HistoryModel struct {
	h       History
	scope   historyScope
	visible []int
	cursor  int
	offset  int
	filter  textinput.Model
	styles  styles
	width   int
	height  int
	picked  string
	done    bool
	// hidePreview is set by ctrl+o.
	hidePreview bool
	// outputs caches the preview's output by run ID.
	outputs map[int64][]string
}

// NewHistory builds the history search over h, starting with query.
func NewHistory(h History, query string, theme config.ThemeColors) HistoryModel {
	filter := textinput.New()
	filter.Prompt = "› "
	filter.Placeholder = "type to search commands"
	filter.SetValue(query)
	filter.CursorEnd()
	filter.Focus()
	m := HistoryModel{h: h, filter: filter, styles: newStyles(theme), outputs: map[int64][]string{}}
	m.applyFilter()
	return m
}

// PickCommand runs history search on the terminal, drawing on standard error
// so standard output is left for the command picked. ok is false when it was
// cancelled.
func PickCommand(h History, query string, theme config.ThemeColors) (string, bool, error) {
	final, err := tea.NewProgram(NewHistory(h, query, theme), tea.WithOutput(os.Stderr)).Run()
	if err != nil {
		return "", false, err
	}
	m, ok := final.(HistoryModel)
	if !ok || m.picked == "" {
		return "", false, nil
	}
	return m.picked, true, nil
}

// Picked returns the command picked, or "".
func (m HistoryModel) Picked() string { return m.picked }

func (m *HistoryModel) applyFilter() {
	words := strings.Fields(strings.ToLower(m.filter.Value()))
	m.visible = m.visible[:0]
	for i, c := range m.h.Commands {
		switch m.scope {
		case scopeHere:
			if c.CWD != m.h.CWD {
				continue
			}
		case scopeSession:
			if c.SessionID == nil || *c.SessionID != m.h.SessionID {
				continue
			}
		}
		line := strings.ToLower(c.Command)
		match := true
		for _, w := range words {
			if !strings.Contains(line, w) {
				match = false
				break
			}
		}
		if match {
			m.visible = append(m.visible, i)
		}
	}
	m.cursor = 0
}

// nextScope cycles all → here → this session, skipping the session when the
// search was not opened in one.
func (m *HistoryModel) nextScope() {
	m.scope = (m.scope + 1) % 3
	if m.scope == scopeSession && m.h.SessionID == 0 {
		m.scope = scopeAll
	}
	m.applyFilter()
}

func (m HistoryModel) Init() tea.Cmd { return textinput.Blink }

func (m HistoryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keys := eachKey(msg); keys != nil {
		return updateEach(m, keys)
	}
	tm, cmd := m.update(msg)
	hm := tm.(HistoryModel)
	hm.offset = scrollTo(hm.offset, hm.cursor, hm.rows(), len(hm.visible))
	return hm, cmd
}

func (m HistoryModel) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "ctrl+g":
			m.done = true
			return m, tea.Quit
		case "enter":
			if m.cursor < len(m.visible) {
				m.picked = m.h.Commands[m.visible[m.cursor]].Command
			}
			m.done = true
			return m, tea.Quit
		case "tab":
			m.nextScope()
			return m, nil
		case "ctrl+o":
			m.hidePreview = !m.hidePreview
			return m, nil
		case "up", "ctrl+p", "ctrl+k", "ctrl+r":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "ctrl+n", "ctrl+j", "ctrl+s":
			if m.cursor < len(m.visible)-1 {
				m.cursor++
			}
			return m, nil
		case "pgup":
			m.cursor = max(0, m.cursor-m.rows())
			return m, nil
		case "pgdown":
			m.cursor = max(0, min(len(m.visible)-1, m.cursor+m.rows()))
			return m, nil
		}
		var cmd tea.Cmd
		before := m.filter.Value()
		m.filter, cmd = m.filter.Update(msg)
		if m.filter.Value() != before {
			m.applyFilter()
		}
		return m, cmd
	}
	return m, nil
}

// rows is how many commands fit in the terminal, as in the session picker,
// leaving room for the preview when it is shown.
func (m HistoryModel) rows() int {
	if m.height <= 0 {
		return pickerRows
	}
	space := m.height - pickerChrome
	if m.showsPreview() {
		space -= previewHeight
	}
	return max(1, min(pickerRows, space))
}

// showsPreview reports whether the preview is on screen: it can be, it is
// not hidden, and the terminal is tall enough for it and a few commands.
func (m HistoryModel) showsPreview() bool { return m.canPreview() && !m.hidePreview }

// canPreview reports whether there is a preview and room for it.
func (m HistoryModel) canPreview() bool {
	return m.h.Output != nil && (m.height <= 0 || m.height-pickerChrome-previewHeight >= previewMinRows)
}

func (m HistoryModel) View() string {
	if m.done {
		return ""
	}
	width := m.width
	if width <= 0 {
		width = 80
	}
	scope := map[historyScope]string{scopeAll: "every terminal", scopeHere: "this directory", scopeSession: "this session"}[m.scope]
	var b strings.Builder
	b.WriteString(m.styles.paneTitle.Render("Search history") + m.styles.muted.Render(" · "+scope) + "\n")
	b.WriteString(m.filter.View() + "\n")
	if len(m.visible) == 0 {
		b.WriteString(m.styles.muted.Render("  no commands match") + "\n")
	}
	rows := m.rows()
	start := scrollTo(m.offset, m.cursor, rows, len(m.visible))
	end := min(len(m.visible), start+rows)
	commandWidth := max(20, min(60, width-40))
	for i := start; i < end; i++ {
		c := m.h.Commands[m.visible[i]]
		glyph := statusGlyph(c.Status)
		command := padRight(truncate(strings.Join(strings.Fields(c.Command), " "), commandWidth), commandWidth)
		details := []string{formatRelativeTime(c.StartedAt), collapseHome(c.CWD)}
		if c.SessionID != nil {
			if label := m.h.Labels[*c.SessionID]; label != "" {
				details = append(details, label)
			}
		}
		if c.Runs > 1 {
			details = append(details, fmt.Sprintf("%d runs", c.Runs))
		}
		line := "  "
		commandStyle := m.styles.command
		if i == m.cursor {
			line = m.styles.accent.Render("❯ ")
			commandStyle = commandStyle.Bold(true)
		}
		line += m.statusStyle(c.Status).Render(glyph) + " " + commandStyle.Render(command) + "  " + m.styles.muted.Render(strings.Join(details, " · "))
		b.WriteString(ansi.Truncate(line, width-1, "…") + "\n")
	}
	if more := len(m.visible) - end; more > 0 {
		b.WriteString(m.styles.muted.Render(fmt.Sprintf("  … %d more", more)) + "\n")
	}
	if m.showsPreview() {
		b.WriteString(m.renderPreview(width))
	}
	// The title says which of every terminal, this directory, and this
	// session tab has chosen.
	help := "↑/↓ move · enter pick · tab scope · esc cancel"
	if m.canPreview() {
		help = "↑/↓ move · enter pick · tab scope · ctrl+o preview · esc cancel"
	}
	b.WriteString(ansi.Truncate(m.styles.help.Render(help), width-1, "…"))
	return b.String()
}

// renderPreview shows the last run of the selected command: how it ended,
// when and where it ran, and the end of its output. It is always
// previewHeight lines.
func (m HistoryModel) renderPreview(width int) string {
	lines := []string{m.styles.muted.Render(strings.Repeat("─", max(1, min(width-1, 80))))}
	if m.cursor < len(m.visible) {
		c := m.h.Commands[m.visible[m.cursor]]
		lines = append(lines, m.previewDetails(c))
		output, ok := m.outputs[c.ID]
		if !ok {
			output = m.h.Output(&c.Run, previewLines)
			m.outputs[c.ID] = output
		}
		if len(output) == 0 {
			lines = append(lines, m.styles.muted.Render("  no output recorded"))
		}
		for _, line := range output {
			lines = append(lines, "  "+m.styles.muted.Render("│")+" "+ansi.Truncate(strings.ReplaceAll(line, "\t", "    "), max(1, width-5), "…"))
		}
	}
	for len(lines) < previewHeight {
		lines = append(lines, "")
	}
	return strings.Join(lines[:previewHeight], "\n") + "\n"
}

// previewDetails is the line about the last run: "✗ exit 1 · 2.4s · 3h ago ·
// ~/src/api · main · run 482".
func (m HistoryModel) previewDetails(c store.CommandChoice) string {
	status := c.Status
	switch {
	case c.Status == store.StatusRunning:
		status = "running"
	case c.ExitCode != nil:
		status = fmt.Sprintf("exit %d", *c.ExitCode)
	}
	details := []string{m.statusStyle(c.Status).Render(statusGlyph(c.Status) + " " + status)}
	if c.DurationMS != nil {
		details = append(details, (time.Duration(*c.DurationMS) * time.Millisecond).Round(10*time.Millisecond).String())
	}
	details = append(details, formatRelativeTime(c.StartedAt), collapseHome(c.CWD))
	if c.GitBranch != "" {
		details = append(details, c.GitBranch)
	}
	details = append(details, fmt.Sprintf("run %d", c.ID))
	line := "  " + details[0] + m.styles.muted.Render(" · "+strings.Join(details[1:], " · "))
	if m.width > 0 {
		line = ansi.Truncate(line, m.width-1, "…")
	}
	return line
}

func (m HistoryModel) statusStyle(status string) lipgloss.Style {
	switch status {
	case store.StatusSuccess:
		return m.styles.success
	case store.StatusFailed:
		return m.styles.danger
	case store.StatusKilled:
		return m.styles.warning
	default:
		return m.styles.muted
	}
}
