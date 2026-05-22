// Package tui implements the interactive run browser (`carrier tui`). It renders
// a scrollable run list with a live output preview and supports rerunning,
// labeling, and deleting runs. Rerun is executed by the caller after the program
// exits (see Result) so the command runs in the normal terminal, not nested
// inside the alt-screen.
package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/atbuy/carrier/internal/command"
	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

// Action is what the caller should do after the TUI exits.
type Action int

const (
	ActionNone Action = iota
	ActionRerun
)

// Result reports the action selected when the TUI exited.
type Result struct {
	Action Action
	RunID  int64
}

type mode int

const (
	modeBrowse mode = iota
	modeFilter
	modeLabel
	modeConfirmDelete
)

const (
	listWidth        = 52
	previewTailLines = 400
)

type styles struct {
	listSel   lipgloss.Style
	id        lipgloss.Style
	muted     lipgloss.Style
	command   lipgloss.Style
	label     lipgloss.Style
	success   lipgloss.Style
	danger    lipgloss.Style
	warning   lipgloss.Style
	accent    lipgloss.Style
	help      lipgloss.Style
	paneTitle lipgloss.Style
}

func newStyles(c config.ThemeColors) styles {
	s := func(hex string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)) }
	return styles{
		listSel:   selectionStyle(c.Accent),
		id:        lipgloss.NewStyle().Bold(true),
		muted:     s(c.Muted),
		command:   s(c.Command),
		label:     s(c.Label),
		success:   s(c.Success),
		danger:    s(c.Danger),
		warning:   s(c.Warning),
		accent:    s(c.Accent),
		help:      s(c.Muted),
		paneTitle: lipgloss.NewStyle().Bold(true),
	}
}

// selectionStyle builds the highlight bar for the selected row from the user's
// accent color: the accent becomes the background and a black/white foreground
// is chosen for contrast. For non-hex accents (ANSI names/indices) it falls back
// to reverse video, which adapts to any palette.
func selectionStyle(accent string) lipgloss.Style {
	base := lipgloss.NewStyle().Bold(true)
	if fg, ok := contrastFor(accent); ok {
		return base.Background(lipgloss.Color(accent)).Foreground(lipgloss.Color(fg))
	}
	return base.Reverse(true)
}

// contrastFor returns "#000000" or "#FFFFFF" — whichever reads better on the
// given #RRGGBB color. ok is false when hex is not a #RRGGBB string.
func contrastFor(hex string) (string, bool) {
	if len(hex) != 7 || hex[0] != '#' {
		return "", false
	}
	r, e1 := strconv.ParseInt(hex[1:3], 16, 0)
	g, e2 := strconv.ParseInt(hex[3:5], 16, 0)
	b, e3 := strconv.ParseInt(hex[5:7], 16, 0)
	if e1 != nil || e2 != nil || e3 != nil {
		return "", false
	}
	// Perceived luminance (ITU-R BT.601).
	lum := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
	if lum > 150 {
		return "#000000", true
	}
	return "#FFFFFF", true
}

// Model is the bubbletea model for the run browser.
type Model struct {
	st        *store.Store
	dataDir   string
	styles    styles
	runs      []store.Run // all runs, newest-first
	filtered  []store.Run // runs matching the active filter
	cursor    int
	vp        viewport.Model
	filter    textinput.Model
	label     textinput.Model
	mode      mode
	width     int
	height    int
	ready     bool
	statusMsg string
	result    Result
}

// New builds a Model. runs should be newest-first. dataDir is used to remove log
// files on delete.
func New(st *store.Store, cfg config.Config, runs []store.Run) Model {
	fi := textinput.New()
	fi.Placeholder = "filter by command, status, cwd, label"
	fi.Prompt = "/"

	li := textinput.New()
	li.Placeholder = "label (empty to clear)"
	li.Prompt = "label: "

	m := Model{
		st:       st,
		dataDir:  cfg.Storage.DataDir,
		styles:   newStyles(cfg.UI.Theme),
		runs:     runs,
		filtered: runs,
		filter:   fi,
		label:    li,
	}
	return m
}

// Result returns the action chosen when the program exited.
func (m Model) Result() Result { return m.result }

// Init hides the hardware cursor so the rendered selection bar is the only
// cursor on screen. The text inputs draw their own cursor when focused.
func (m Model) Init() tea.Cmd { return tea.HideCursor }

func (m Model) selected() (store.Run, bool) {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return store.Run{}, false
	}
	return m.filtered[m.cursor], true
}

func (m *Model) layout() {
	previewW := m.width - listWidth - 1
	if previewW < 10 {
		previewW = 10
	}
	previewH := m.height - 2 // leave room for help line
	if previewH < 3 {
		previewH = 3
	}
	if !m.ready {
		m.vp = viewport.New(previewW, previewH)
		m.ready = true
	} else {
		m.vp.Width = previewW
		m.vp.Height = previewH
	}
}

func (m *Model) refreshPreview() {
	r, ok := m.selected()
	if !ok {
		m.vp.SetContent(m.styles.muted.Render("no run selected"))
		return
	}
	m.vp.SetContent(m.renderPreview(r))
	m.vp.GotoTop()
}

// applyFilter recomputes filtered from the current filter text and clamps cursor.
func (m *Model) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	if q == "" {
		m.filtered = m.runs
	} else {
		out := make([]store.Run, 0, len(m.runs))
		for _, r := range m.runs {
			hay := strings.ToLower(strings.Join([]string{
				displayCommand(r), r.Status, r.CWD, r.Label,
			}, " "))
			if strings.Contains(hay, q) {
				out = append(out, r)
			}
		}
		m.filtered = out
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// reload re-reads all runs from the store after a mutation.
func (m *Model) reload() {
	runs, err := m.st.AllRuns()
	if err != nil {
		m.statusMsg = "reload failed: " + err.Error()
		return
	}
	m.runs = runs
	m.applyFilter()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		m.refreshPreview()
		return m, nil
	case tea.KeyMsg:
		switch m.mode {
		case modeFilter:
			return m.updateFilter(msg)
		case modeLabel:
			return m.updateLabel(msg)
		case modeConfirmDelete:
			return m.updateConfirmDelete(msg)
		default:
			return m.updateBrowse(msg)
		}
	}
	return m, nil
}

func (m Model) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.refreshPreview()
		}
	case "down", "j":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
			m.refreshPreview()
		}
	case "g", "home":
		m.cursor = 0
		m.refreshPreview()
	case "G", "end":
		m.cursor = len(m.filtered) - 1
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.refreshPreview()
	case "ctrl+d", "pgdown":
		m.vp.HalfPageDown()
	case "ctrl+u", "pgup":
		m.vp.HalfPageUp()
	case "/":
		m.mode = modeFilter
		m.statusMsg = ""
		m.filter.Focus()
		return m, textinput.Blink
	case "l":
		if r, ok := m.selected(); ok {
			m.mode = modeLabel
			m.statusMsg = ""
			m.label.SetValue(r.Label)
			m.label.CursorEnd()
			m.label.Focus()
			return m, textinput.Blink
		}
	case "d":
		if _, ok := m.selected(); ok {
			m.mode = modeConfirmDelete
			m.statusMsg = ""
		}
	case "enter":
		if r, ok := m.selected(); ok {
			m.result = Result{Action: ActionRerun, RunID: r.ID}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.mode = modeBrowse
		m.filter.Blur()
		m.refreshPreview()
		return m, nil
	case "esc":
		m.mode = modeBrowse
		m.filter.Blur()
		m.filter.SetValue("")
		m.applyFilter()
		m.refreshPreview()
		return m, nil
	}
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	m.applyFilter()
	m.refreshPreview()
	return m, cmd
}

func (m Model) updateLabel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if r, ok := m.selected(); ok {
			if err := m.st.SetLabel(r.ID, strings.TrimSpace(m.label.Value())); err != nil {
				m.statusMsg = "label failed: " + err.Error()
			} else {
				m.statusMsg = fmt.Sprintf("labeled run %d", r.ID)
				m.reload()
			}
		}
		m.mode = modeBrowse
		m.label.Blur()
		m.refreshPreview()
		return m, nil
	case "esc":
		m.mode = modeBrowse
		m.label.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.label, cmd = m.label.Update(msg)
	return m, cmd
}

func (m Model) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if r, ok := m.selected(); ok {
			m.deleteRun(r)
		}
		m.mode = modeBrowse
		m.refreshPreview()
		return m, nil
	default: // any other key cancels
		m.mode = modeBrowse
		return m, nil
	}
}

func (m *Model) deleteRun(r store.Run) {
	deleted, err := m.st.DeleteByID(r.ID)
	if err != nil {
		m.statusMsg = "delete failed: " + err.Error()
		return
	}
	removeIfSet(deleted.StdoutPath)
	removeIfSet(deleted.StderrPath)
	removeIfSet(deleted.TerminalOutputPath)
	_, _ = m.st.PruneOrphanedEnvironments()
	m.statusMsg = fmt.Sprintf("deleted run %d", r.ID)
	m.reload()
}

func (m Model) View() string {
	if !m.ready || m.width == 0 {
		return "loading..."
	}
	list := m.renderList()
	preview := m.vp.View()
	body := lipgloss.JoinHorizontal(lipgloss.Top, list, " ", preview)
	return lipgloss.JoinVertical(lipgloss.Left, body, m.renderHelp())
}

func (m Model) renderList() string {
	var b strings.Builder
	height := m.height - 2
	if height < 1 {
		height = 1
	}
	// Scroll the list window to keep the cursor visible.
	start := 0
	if m.cursor >= height {
		start = m.cursor - height + 1
	}
	end := start + height
	if end > len(m.filtered) {
		end = len(m.filtered)
	}
	const cmdWidth = listWidth - 20 // gutter(2)+id(5)+sp+glyph(1)+sp+when(9)+sp
	for i := start; i < end; i++ {
		r := m.filtered[i]
		idStr := fmt.Sprintf("%5d", r.ID)
		glyph := statusGlyph(r.Status)
		when := padRight(formatRelativeTime(r.StartedAt), 9)
		cmd := truncate(displayCommand(r), cmdWidth)
		if i == m.cursor {
			// Selected: uniform full-width highlight bar. Width() is
			// display-aware, so multibyte glyphs pad correctly.
			raw := fmt.Sprintf("▌ %s %s %s %s", idStr, glyph, when, cmd)
			b.WriteString(m.styles.listSel.Width(listWidth).Render(raw))
		} else {
			fmt.Fprintf(
				&b,
				"  %s %s %s %s",
				m.styles.id.Render(idStr),
				m.statusStyle(r.Status).Render(glyph),
				m.styles.muted.Render(when),
				m.styles.command.Render(cmd),
			)
		}
		b.WriteString("\n")
	}
	if len(m.filtered) == 0 {
		b.WriteString(m.styles.muted.Render("  no runs match"))
	}
	return lipgloss.NewStyle().Width(listWidth).Render(b.String())
}

func (m Model) renderPreview(r store.Run) string {
	var b strings.Builder
	field := func(label, value string, style lipgloss.Style) {
		b.WriteString(m.styles.paneTitle.Render(padRight(label+":", 10)))
		b.WriteString(style.Render(value))
		b.WriteString("\n")
	}
	field("ID", fmt.Sprintf("%d", r.ID), m.styles.id)
	field("Status", statusGlyph(r.Status)+" "+r.Status, m.statusStyle(r.Status))
	field("Command", displayCommand(r), m.styles.command)
	field("CWD", collapseHome(r.CWD), m.styles.muted)
	if r.ExitCode != nil {
		es := m.styles.success
		if *r.ExitCode != 0 {
			es = m.styles.danger
		}
		field("Exit", fmt.Sprintf("%d", *r.ExitCode), es)
	}
	field("Duration", formatDuration(r.DurationMS), m.styles.muted)
	field("Started", timeWithRelative(r.StartedAt), m.styles.muted)
	if r.Label != "" {
		field("Label", r.Label, m.styles.label)
	}
	if r.GitRoot != "" {
		field("Git", fmt.Sprintf("%s %s", r.GitBranch, short(r.GitCommit)), m.styles.muted)
	}
	b.WriteString("\n")

	out := cleanOutput(readTail(r.StdoutPath))
	errOut := cleanOutput(readTail(r.StderrPath))
	hasTerm := readTail(r.TerminalOutputPath) != ""
	if hasTerm {
		// PTY terminal logs lay out via cursor-positioning escapes and include
		// the shell prompt, so they can't render faithfully as plain text here.
		// Point the user at commands that replay them in a real terminal.
		b.WriteString(m.styles.paneTitle.Render("terminal"))
		b.WriteString("\n")
		b.WriteString(m.styles.muted.Render(
			fmt.Sprintf("(shell session — run `carrier show %d` or `carrier tail %d` to view)", r.ID, r.ID),
		))
		b.WriteString("\n")
	}
	if out != "" {
		b.WriteString(m.styles.success.Render("stdout"))
		b.WriteString("\n")
		b.WriteString(out)
		b.WriteString("\n")
	}
	if errOut != "" {
		b.WriteString(m.styles.danger.Render("stderr"))
		b.WriteString("\n")
		b.WriteString(errOut)
	}
	if out == "" && errOut == "" && !hasTerm {
		b.WriteString(m.styles.muted.Render("(no output captured)"))
	}
	content := b.String()
	if m.vp.Width > 0 {
		content = ansi.Hardwrap(content, m.vp.Width, false)
	}
	return content
}

func (m Model) renderHelp() string {
	switch m.mode {
	case modeFilter:
		return m.styles.help.Render("filter: ") + m.filter.View()
	case modeLabel:
		return m.label.View()
	case modeConfirmDelete:
		if r, ok := m.selected(); ok {
			return m.styles.warning.Render(fmt.Sprintf("delete run %d? (y/N)", r.ID))
		}
		return m.styles.warning.Render("delete run? (y/N)")
	default:
		help := "↑/↓ move  enter rerun  / filter  l label  d delete  q quit"
		if m.statusMsg != "" {
			return m.styles.accent.Render(m.statusMsg) + "  " + m.styles.help.Render(help)
		}
		return m.styles.help.Render(help)
	}
}

func (m Model) statusStyle(status string) lipgloss.Style {
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

// Run launches the interactive browser and returns the chosen Result. It returns
// an empty Result (ActionNone) when the user quits without selecting an action.
func Run(st *store.Store, cfg config.Config, runs []store.Run) (Result, error) {
	final, err := tea.NewProgram(New(st, cfg, runs), tea.WithAltScreen()).Run()
	if err != nil {
		return Result{}, err
	}
	if fm, ok := final.(Model); ok {
		return fm.Result(), nil
	}
	return Result{}, nil
}

// --- small display helpers (mirror the cli package's plain-text formatters) ---

func displayCommand(r store.Run) string {
	if r.Mode != store.ModeShell {
		var argv []string
		if json.Unmarshal([]byte(r.ArgvJSON), &argv) == nil && len(argv) > 0 {
			return command.Display(argv)
		}
	}
	return r.Command
}

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

func formatDuration(ms *int64) string {
	if ms == nil {
		return ""
	}
	return (time.Duration(*ms) * time.Millisecond).Round(10 * time.Millisecond).String()
}

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

func timeWithRelative(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	abs := ts.Local().Format("2006-01-02 15:04:05")
	if rel := formatRelativeTime(ts); rel != "" {
		return abs + " (" + rel + ")"
	}
	return abs
}

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

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func truncate(s string, n int) string {
	if lipgloss.Width(s) <= n {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(n).Render(s)
}

func stripANSI(s string) string { return ansi.Strip(s) }

// cleanOutput strips ANSI escape sequences and normalizes carriage returns so
// captured stdout/stderr logs render as plain text in the preview pane.
func cleanOutput(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "")
	return stripANSI(s)
}

func readTail(path string) string {
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > previewTailLines {
		lines = lines[len(lines)-previewTailLines:]
	}
	return strings.Join(lines, "\n")
}

func removeIfSet(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}
