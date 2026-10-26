package tui

import (
	"fmt"
	"os"
	"strings"

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
}

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
}

// NewHistory builds the history search over h, starting with query.
func NewHistory(h History, query string, theme config.ThemeColors) HistoryModel {
	filter := textinput.New()
	filter.Prompt = "› "
	filter.Placeholder = "type to search commands"
	filter.SetValue(query)
	filter.CursorEnd()
	filter.Focus()
	m := HistoryModel{h: h, filter: filter, styles: newStyles(theme)}
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

// rows is how many commands fit in the terminal, as in the session picker.
func (m HistoryModel) rows() int {
	if m.height <= 0 {
		return pickerRows
	}
	return max(1, min(pickerRows, m.height-pickerChrome))
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
	b.WriteString(m.styles.help.Render("↑/↓ move · enter pick · tab every terminal / this directory / this session · esc cancel"))
	return b.String()
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
