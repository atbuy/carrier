package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

// pickerRows is the most sessions the picker shows at once.
const pickerRows = 10

// SessionItem is a session offered by the picker, with tags such as
// "pinned here" that say why it may be the one wanted.
type SessionItem struct {
	store.SessionChoice
	Tags []string
}

// PickerModel is the bubbletea model of the session picker that "carrier
// attach" opens when no session is given: typing filters the sessions by ID,
// label, or tag, and enter picks the selected one.
type PickerModel struct {
	items   []SessionItem
	visible []int
	cursor  int
	filter  textinput.Model
	styles  styles
	width   int
	height  int
	picked  int64
	done    bool
}

// NewPicker builds the picker for items, newest first.
func NewPicker(items []SessionItem, theme config.ThemeColors) PickerModel {
	filter := textinput.New()
	filter.Prompt = "› "
	filter.Placeholder = "type to filter by ID or label"
	filter.Focus()
	m := PickerModel{items: items, filter: filter, styles: newStyles(theme)}
	m.applyFilter()
	return m
}

// PickSession runs the picker and returns the session picked, or false when
// it was cancelled.
func PickSession(items []SessionItem, theme config.ThemeColors) (int64, bool, error) {
	final, err := tea.NewProgram(NewPicker(items, theme)).Run()
	if err != nil {
		return 0, false, err
	}
	m, ok := final.(PickerModel)
	if !ok || m.picked == 0 {
		return 0, false, nil
	}
	return m.picked, true, nil
}

// Picked returns the session picked, or 0.
func (m PickerModel) Picked() int64 { return m.picked }

func (m *PickerModel) applyFilter() {
	words := strings.Fields(strings.ToLower(m.filter.Value()))
	m.visible = m.visible[:0]
	for i, item := range m.items {
		hay := strings.ToLower(strconv.FormatInt(item.ID, 10) + " " + item.Label + " " + strings.Join(item.Tags, " "))
		match := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
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

func (m PickerModel) Init() tea.Cmd { return textinput.Blink }

func (m PickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.done = true
			return m, tea.Quit
		case "enter":
			if m.cursor < len(m.visible) {
				m.picked = m.items[m.visible[m.cursor]].ID
				m.done = true
				return m, tea.Quit
			}
			return m, nil
		case "up", "ctrl+p", "ctrl+k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "ctrl+n", "ctrl+j", "tab":
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

// rows is how many sessions fit.
func (m PickerModel) rows() int {
	if m.height <= 0 {
		return pickerRows
	}
	return max(1, min(pickerRows, m.height-4))
}

func (m PickerModel) View() string {
	if m.done {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.styles.paneTitle.Render("Attach to a session") + "\n")
	b.WriteString(m.filter.View() + "\n")
	if len(m.visible) == 0 {
		b.WriteString(m.styles.muted.Render("  no sessions match") + "\n")
	}
	rows := m.rows()
	start := max(0, m.cursor-rows+1)
	end := min(len(m.visible), start+rows)
	width := m.width
	if width <= 0 {
		width = 80
	}
	idWidth := 1
	for _, item := range m.items {
		idWidth = max(idWidth, len(strconv.FormatInt(item.ID, 10)))
	}
	labelWidth := max(12, min(40, width-idWidth-40))
	for i := start; i < end; i++ {
		item := m.items[m.visible[i]]
		id := fmt.Sprintf("%*d", idWidth, item.ID)
		label := padRight(truncate(item.Label, labelWidth), labelWidth)
		labelStyle := m.styles.label
		if item.Label == "" {
			label = padRight("no label", labelWidth)
			labelStyle = m.styles.muted
		}
		details := []string{fmt.Sprintf("%d %s", item.Runs, pluralRuns(item.Runs)), formatRelativeTime(item.StartedAt)}
		switch {
		case item.Kept:
			details = append(details, "kept")
		case item.Ended:
			details = append(details, "ended")
		}
		line := "  "
		if i == m.cursor {
			line = m.styles.accent.Render("❯ ")
			id = m.styles.id.Render(id)
		} else {
			id = m.styles.accent.Render(id)
		}
		line += id + "  " + labelStyle.Render(label) + "  " + m.styles.muted.Render(strings.Join(details, " · "))
		for _, tag := range item.Tags {
			line += m.styles.muted.Render(" · ") + m.styles.warning.Render(tag)
		}
		b.WriteString(line + "\n")
	}
	if more := len(m.visible) - end; more > 0 {
		b.WriteString(m.styles.muted.Render(fmt.Sprintf("  … %d more", more)) + "\n")
	}
	b.WriteString(m.styles.help.Render("↑/↓ move · enter attach · esc cancel"))
	return b.String()
}

func pluralRuns(n int) string {
	if n == 1 {
		return "run"
	}
	return "runs"
}
