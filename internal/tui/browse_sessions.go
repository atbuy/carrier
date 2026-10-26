package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/atbuy/carrier/internal/store"
)

type tab int

const (
	tabRuns tab = iota
	tabSessions
)

// Sessions supplies the Sessions tab of the browser. The functions come from
// the caller, which knows how to show a timeline and what else changes with
// a label, such as the prompts of shells recording into the session.
type Sessions struct {
	// List returns the sessions to show, newest first.
	List func() ([]store.SessionChoice, error)
	// Timeline renders a session as "carrier session show" does, for a pane
	// width columns wide.
	Timeline func(id int64, width int) string
	// Label sets or clears a session's label.
	Label func(id int64, label string) error
	// Keep keeps a session or lets it expire again.
	Keep func(id int64, kept bool) error
}

// SetSessions turns on the Sessions tab.
func (m *Model) SetSessions(s Sessions) {
	m.src = s
	m.reloadSessions()
}

func (m Model) hasSessions() bool { return m.src.List != nil }

func (m *Model) reloadSessions() {
	if !m.hasSessions() {
		return
	}
	sessions, err := m.src.List()
	if err != nil {
		m.statusMsg = "loading sessions failed: " + err.Error()
		return
	}
	m.sessions = sessions
	m.applySessionFilter()
}

func (m *Model) applySessionFilter() {
	q := strings.ToLower(strings.TrimSpace(m.filterFor(tabSessions)))
	m.filteredSessions = m.filteredSessions[:0]
	for _, s := range m.sessions {
		hay := strings.ToLower(strconv.FormatInt(s.ID, 10) + " " + s.Label)
		if q == "" || strings.Contains(hay, q) {
			m.filteredSessions = append(m.filteredSessions, s)
		}
	}
	m.sessionCursor = max(0, min(m.sessionCursor, len(m.filteredSessions)-1))
}

func (m Model) selectedSession() (store.SessionChoice, bool) {
	if m.sessionCursor < 0 || m.sessionCursor >= len(m.filteredSessions) {
		return store.SessionChoice{}, false
	}
	return m.filteredSessions[m.sessionCursor], true
}

// filterFor is the filter text of tab: the one being typed for the tab on
// screen, the one kept for the other.
func (m Model) filterFor(t tab) string {
	if t == m.tab {
		return m.filter.Value()
	}
	return m.otherFilter
}

// switchTab shows the other tab, keeping each tab's filter.
func (m *Model) switchTab(to tab) {
	if to == m.tab || (to == tabSessions && !m.hasSessions()) {
		return
	}
	current := m.filter.Value()
	m.filter.SetValue(m.otherFilter)
	m.otherFilter = current
	m.tab = to
	m.filter.Placeholder = "filter by command, status, cwd, label"
	if to == tabSessions {
		m.filter.Placeholder = "filter by session ID or label"
	}
	m.statusMsg = ""
	m.refreshPreview()
}

// showSessionRuns shows the Runs tab with only the runs of session id.
func (m *Model) showSessionRuns(id int64) {
	m.switchTab(tabRuns)
	m.runSession = id
	m.filter.SetValue("")
	m.cursor = 0
	m.applyFilter()
	m.refreshPreview()
}

// updateSessions handles the keys of the Sessions tab; handled is false for
// the keys both tabs share.
func (m Model) updateSessions(key string) (_ Model, cmd tea.Cmd, handled bool) {
	sess, ok := m.selectedSession()
	switch key {
	case "up", "k":
		if m.sessionCursor > 0 {
			m.sessionCursor--
			m.refreshPreview()
		}
	case "down", "j":
		if m.sessionCursor < len(m.filteredSessions)-1 {
			m.sessionCursor++
			m.refreshPreview()
		}
	case "g", "home":
		m.sessionCursor = 0
		m.refreshPreview()
	case "G", "end":
		m.sessionCursor = max(0, len(m.filteredSessions)-1)
		m.refreshPreview()
	case "enter":
		if ok {
			m.result = Result{Action: ActionAttach, SessionID: sess.ID}
			return m, tea.Quit, true
		}
	case "o", "right":
		if ok {
			m.showSessionRuns(sess.ID)
		}
	case "K":
		if ok {
			m = m.toggleKeep(sess)
		}
	case "l":
		if ok && m.src.Label != nil {
			m.mode = modeLabel
			m.statusMsg = ""
			m.label.SetValue(sess.Label)
			m.label.CursorEnd()
			m.label.Focus()
			return m, textinput.Blink, true
		}
	default:
		return m, nil, false
	}
	return m, nil, true
}

func (m Model) labelSession(label string) Model {
	sess, ok := m.selectedSession()
	if !ok {
		return m
	}
	if err := m.src.Label(sess.ID, label); err != nil {
		m.statusMsg = "label failed: " + err.Error()
		return m
	}
	if label == "" {
		m.statusMsg = fmt.Sprintf("cleared the label of session %d", sess.ID)
	} else {
		m.statusMsg = fmt.Sprintf("labeled session %d", sess.ID)
	}
	m.reloadSessions()
	m.refreshPreview()
	return m
}

func (m Model) toggleKeep(sess store.SessionChoice) Model {
	if m.src.Keep == nil {
		return m
	}
	if err := m.src.Keep(sess.ID, !sess.Kept); err != nil {
		m.statusMsg = "keep failed: " + err.Error()
		return m
	}
	if sess.Kept {
		m.statusMsg = fmt.Sprintf("session %d can expire again", sess.ID)
	} else {
		m.statusMsg = fmt.Sprintf("kept session %d", sess.ID)
	}
	m.reloadSessions()
	m.refreshPreview()
	return m
}

func (m Model) renderSessionList(height int) string {
	var b strings.Builder
	start := scrollTo(m.sessionOffset, m.sessionCursor, height, len(m.filteredSessions))
	end := min(len(m.filteredSessions), start+height)
	const labelWidth = listWidth - 20 // gutter(2)+id(5)+sp+glyph(1)+sp+when(9)+sp
	for i := start; i < end; i++ {
		s := m.filteredSessions[i]
		id := fmt.Sprintf("%5d", s.ID)
		glyph, glyphStyle := sessionGlyph(s), m.styles.muted
		if !s.Ended {
			glyphStyle = m.styles.success
		}
		when := padRight(formatRelativeTime(s.StartedAt), 9)
		label, labelStyle := s.Label, m.styles.label
		if label == "" {
			label, labelStyle = "no label", m.styles.muted
		}
		label = truncate(fmt.Sprintf("%s · %d %s", label, s.Runs, pluralRuns(s.Runs)), labelWidth)
		if i == m.sessionCursor {
			raw := fmt.Sprintf("▌ %s %s %s %s", id, glyph, when, label)
			b.WriteString(m.styles.listSel.Width(listWidth).Render(raw))
		} else {
			fmt.Fprintf(&b, "  %s %s %s %s", m.styles.id.Render(id), glyphStyle.Render(glyph), m.styles.muted.Render(when), labelStyle.Render(label))
		}
		b.WriteString("\n")
	}
	switch {
	case len(m.sessions) == 0:
		b.WriteString(m.styles.muted.Render("  no sessions recorded yet\n  try: carrier shell"))
	case len(m.filteredSessions) == 0:
		b.WriteString(m.styles.muted.Render("  no sessions match"))
	}
	return b.String()
}

// sessionGlyph marks active sessions, kept ones, and ended ones.
func sessionGlyph(s store.SessionChoice) string {
	switch {
	case !s.Ended:
		return "●"
	case s.Kept:
		return "◆"
	default:
		return "○"
	}
}

func (m Model) renderSessionPreview() string {
	sess, ok := m.selectedSession()
	if !ok {
		return m.styles.muted.Render("no session selected")
	}
	content := m.src.Timeline(sess.ID, m.vp.Width)
	if m.vp.Width > 0 {
		content = ansi.Hardwrap(content, m.vp.Width, false)
	}
	return content
}

// renderTabs is the tab bar above the lists.
func (m Model) renderTabs() string {
	name := func(t tab, title string) string {
		if t == m.tab {
			return m.styles.paneTitle.Underline(true).Render(title)
		}
		return m.styles.muted.Render(title)
	}
	bar := " " + name(tabRuns, "Runs")
	if m.runSession != 0 {
		bar += m.styles.accent.Render(fmt.Sprintf(" (session %d)", m.runSession))
	}
	bar += "   " + name(tabSessions, "Sessions") + m.styles.muted.Render("   tab switches")
	return bar
}
