package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/atbuy/carrier/internal/config"
)

type configMode int

const (
	configBrowse configMode = iota
	configSearch
	configEdit
	configList
	configListEdit
	configRules
	configRuleEdit
	configRuleTry
)

// titleWidth is the width of the settings' titles, before their values.
const titleWidth = 30

// ConfigModel is the bubbletea model of "carrier config": a list of every
// setting, grouped like the config file, that saves each change to the file
// as it is made.
type ConfigModel struct {
	path    string
	cfg     config.Config
	sources map[string]string
	styles  styles
	// visible holds the indices in config.Settings that match the search.
	visible []int
	cursor  int
	// offset is the first row of the list on screen.
	offset int
	mode   configMode
	search textinput.Model
	input  textinput.Model
	// list is the list being edited in configList mode, listCursor the item
	// selected in it, and listEditing the item being edited, or -1 for a new
	// one.
	list        []string
	listCursor  int
	listEditing int
	// rule holds the auto-label rules screens.
	rule      ruleState
	status    string
	statusErr bool
	// changed lists the settings saved, for the line printed on exit.
	changed  []string
	files    []string
	width    int
	height   int
	quitting bool
	// editor is the command that opens the config file; tests replace it.
	editor func(path string) (*exec.Cmd, error)
}

// NewConfig builds the model for the config file at path. It fails when the
// config does not load, since a change could not be checked against it.
func NewConfig(path string) (ConfigModel, error) {
	cfg, err := config.LoadFrom(path)
	if err != nil {
		return ConfigModel{}, err
	}
	search := textinput.New()
	search.Prompt = "/"
	search.Placeholder = "search settings"
	input := textinput.New()
	input.Prompt = ""
	m := ConfigModel{path: path, cfg: cfg, styles: newStyles(cfg.UI.Theme), search: search, input: input, editor: editorCommand, rule: newRuleState()}
	m.loadSources()
	m.loadRules()
	m.applySearch()
	return m, nil
}

// RunConfig runs "carrier config" in the terminal and returns the line to
// print after it, naming what was saved.
func RunConfig(path string) (string, error) {
	m, err := NewConfig(path)
	if err != nil {
		return "", err
	}
	final, err := tea.NewProgram(m).Run()
	if err != nil {
		return "", err
	}
	if fm, ok := final.(ConfigModel); ok {
		return fm.Summary(), nil
	}
	return "", nil
}

// Summary says which settings were saved, and where.
func (m ConfigModel) Summary() string {
	if len(m.changed) == 0 {
		return ""
	}
	files := make([]string, len(m.files))
	for i, f := range m.files {
		files[i] = collapseHome(f)
	}
	settings := len(m.changed)
	if slices.Contains(m.changed, rulesName) {
		settings--
	}
	what, applies := "1 setting", "it applies"
	if settings > 1 {
		what, applies = fmt.Sprintf("%d settings", settings), "they apply"
	}
	switch {
	case settings == len(m.changed):
	case settings == 0:
		what, applies = "the "+rulesName, "they apply"
	default:
		what, applies = what+" and the "+rulesName, "they apply"
	}
	return fmt.Sprintf("carrier: saved %s to %s; %s from the next carrier command", what, strings.Join(files, ", "), applies)
}

func (m *ConfigModel) loadSources() {
	m.sources = map[string]string{}
	for _, s := range config.Settings {
		if src, err := config.Source(m.path, s); err == nil {
			m.sources[s.Name()] = src
		}
	}
}

// reload reads the config again after a change, here or in an editor.
func (m *ConfigModel) reload() error {
	cfg, err := config.LoadFrom(m.path)
	if err != nil {
		return err
	}
	m.cfg = cfg
	m.styles = newStyles(cfg.UI.Theme)
	m.loadSources()
	m.loadRules()
	return nil
}

func (m *ConfigModel) applySearch() {
	q := strings.ToLower(strings.TrimSpace(m.search.Value()))
	selected := -1
	if m.cursor >= 0 && m.cursor < len(m.visible) {
		selected = m.visible[m.cursor]
	}
	m.visible = m.visible[:0]
	for i, s := range config.Settings {
		hay := strings.ToLower(s.Title + " " + s.Name() + " " + config.GroupTitle(s.Group()))
		if q == "" || strings.Contains(hay, q) {
			m.visible = append(m.visible, i)
		}
	}
	if rulesMatch(q) {
		m.visible = append(m.visible, rulesRow)
	}
	m.cursor = 0
	for i, idx := range m.visible {
		if idx == selected {
			m.cursor = i
		}
	}
}

func (m ConfigModel) selected() (config.Setting, bool) {
	if m.cursor < 0 || m.cursor >= len(m.visible) || m.visible[m.cursor] == rulesRow {
		return config.Setting{}, false
	}
	return config.Settings[m.visible[m.cursor]], true
}

func (m *ConfigModel) setStatus(msg string, isErr bool) {
	m.status, m.statusErr = msg, isErr
}

// save writes value for s and reports the result in the status line.
func (m *ConfigModel) save(s config.Setting, value any) bool {
	file, err := config.Set(m.path, s, value)
	if err != nil {
		m.setStatus(s.Name()+": "+err.Error(), true)
		return false
	}
	m.saved(s, file)
	m.setStatus(fmt.Sprintf("saved %s = %s", s.Name(), config.Literal(value)), false)
	return true
}

func (m *ConfigModel) saved(s config.Setting, file string) {
	if !slices.Contains(m.changed, s.Name()) {
		m.changed = append(m.changed, s.Name())
	}
	if !slices.Contains(m.files, file) {
		m.files = append(m.files, file)
	}
	if err := m.reload(); err != nil {
		m.setStatus(err.Error(), true)
	}
}

func (m *ConfigModel) reset(s config.Setting) {
	file, err := config.Reset(m.path, s)
	switch {
	case err != nil:
		m.setStatus(s.Name()+": "+err.Error(), true)
	case file == "":
		m.setStatus(s.Name()+" already has its default value", false)
	default:
		m.saved(s, file)
		m.setStatus(fmt.Sprintf("reset %s to %s", s.Name(), config.Literal(s.Value(m.cfg))), false)
	}
}

// editorCommand opens path in $VISUAL or $EDITOR.
func editorCommand(path string) (*exec.Cmd, error) {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		return nil, errors.New("set $EDITOR to open the config file")
	}
	args := append(strings.Fields(editor), path)
	return exec.Command(args[0], args[1:]...), nil
}

type editorDoneMsg struct{ err error }

func (m ConfigModel) Init() tea.Cmd { return nil }

func (m ConfigModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keys := eachKey(msg); keys != nil {
		return updateEach(m, keys)
	}
	tm, cmd := m.update(msg)
	cm := tm.(ConfigModel)
	cm.scrollSettings()
	return cm, cmd
}

func (m ConfigModel) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case editorDoneMsg:
		if msg.err != nil {
			m.setStatus("editor: "+msg.err.Error(), true)
		} else if err := m.reload(); err != nil {
			m.setStatus("the config does not load: "+err.Error(), true)
		} else {
			m.setStatus("reloaded "+collapseHome(m.path), false)
		}
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		switch m.mode {
		case configSearch:
			return m.updateSearch(msg)
		case configEdit:
			return m.updateEdit(msg)
		case configList:
			return m.updateList(msg)
		case configListEdit:
			return m.updateListEdit(msg)
		case configRules:
			return m.updateRules(msg)
		case configRuleEdit:
			return m.updateRuleEdit(msg)
		case configRuleTry:
			return m.updateRuleTry(msg)
		default:
			return m.updateBrowse(msg)
		}
	}
	return m, nil
}

func (m ConfigModel) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s, ok := m.selected()
	switch msg.String() {
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		if m.search.Value() != "" {
			m.search.SetValue("")
			m.applySearch()
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.visible)-1 {
			m.cursor++
		}
	case "pgup", "ctrl+u":
		m.cursor = max(0, m.cursor-m.pageSize())
	case "pgdown", "ctrl+d":
		m.cursor = max(0, min(len(m.visible)-1, m.cursor+m.pageSize()))
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = max(0, len(m.visible)-1)
	case "/":
		m.mode = configSearch
		m.search.SetValue("")
		m.applySearch()
		m.search.Focus()
		return m, textinput.Blink
	case "e":
		cmd, err := m.editor(m.path)
		if err != nil {
			m.setStatus(err.Error(), true)
			return m, nil
		}
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return editorDoneMsg{err} })
	case "r":
		if ok {
			m.reset(s)
		}
	case "enter", " ":
		if m.onRules() {
			return m.openRules()
		}
		if ok {
			return m.change(s, 1)
		}
	case "right", "l":
		if m.onRules() {
			return m.openRules()
		}
		if ok && (s.Kind == config.KindBool || s.Kind == config.KindChoice) {
			return m.change(s, 1)
		}
	case "left", "h":
		if ok && (s.Kind == config.KindBool || s.Kind == config.KindChoice) {
			return m.change(s, -1)
		}
	}
	return m, nil
}

// change acts on the selected setting: a toggle flips, a choice moves by
// step, a list opens, and anything else is edited as text.
func (m ConfigModel) change(s config.Setting, step int) (tea.Model, tea.Cmd) {
	value := s.Value(m.cfg)
	switch s.Kind {
	case config.KindBool:
		m.save(s, !value.(bool))
	case config.KindChoice:
		i := slices.Index(s.Choices, value.(string))
		if i < 0 && step < 0 {
			i = 0
		}
		m.save(s, s.Choices[(i+step+len(s.Choices))%len(s.Choices)])
	case config.KindList:
		m.mode = configList
		m.list = slices.Clone(value.([]string))
		m.listCursor = 0
		m.setStatus("", false)
	default:
		m.mode = configEdit
		m.input.SetValue(m.displayValue(s))
		m.input.CursorEnd()
		m.input.Focus()
		m.setStatus("", false)
		return m, textinput.Blink
	}
	return m, nil
}

func (m ConfigModel) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "down", "up":
		m.mode = configBrowse
		m.search.Blur()
		return m, nil
	case "esc":
		m.mode = configBrowse
		m.search.Blur()
		m.search.SetValue("")
		m.applySearch()
		return m, nil
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	m.applySearch()
	return m, cmd
}

func (m ConfigModel) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s, _ := m.selected()
	switch msg.String() {
	case "esc":
		m.mode = configBrowse
		m.input.Blur()
		return m, nil
	case "enter":
		text := strings.TrimSpace(m.input.Value())
		var value any = text
		if s.Kind == config.KindInt {
			n, err := strconv.ParseInt(text, 10, 64)
			if err != nil {
				m.setStatus(s.Name()+": enter a whole number", true)
				return m, nil
			}
			value = n
		}
		if text == m.displayValue(s) {
			m.mode = configBrowse
			m.input.Blur()
			return m, nil
		}
		if m.save(s, value) {
			m.mode = configBrowse
			m.input.Blur()
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m ConfigModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s, _ := m.selected()
	switch msg.String() {
	case "esc", "q", "left", "h":
		m.mode = configBrowse
		return m, nil
	case "up", "k":
		if m.listCursor > 0 {
			m.listCursor--
		}
	case "down", "j":
		if m.listCursor < len(m.list)-1 {
			m.listCursor++
		}
	case "a", "o":
		m.mode = configListEdit
		m.listEditing = -1
		m.input.SetValue("")
		m.input.Focus()
		m.setStatus("", false)
		return m, textinput.Blink
	case "enter", " ":
		if m.listCursor < len(m.list) {
			m.mode = configListEdit
			m.listEditing = m.listCursor
			m.input.SetValue(m.list[m.listCursor])
			m.input.CursorEnd()
			m.input.Focus()
			m.setStatus("", false)
			return m, textinput.Blink
		}
	case "d", "x", "delete", "backspace":
		if m.listCursor < len(m.list) {
			list := slices.Delete(slices.Clone(m.list), m.listCursor, m.listCursor+1)
			if m.save(s, list) {
				m.list = list
				m.listCursor = max(0, min(m.listCursor, len(list)-1))
			}
		}
	case "r":
		m.reset(s)
		m.list = slices.Clone(s.Value(m.cfg).([]string))
		m.listCursor = 0
	}
	return m, nil
}

func (m ConfigModel) updateListEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s, _ := m.selected()
	switch msg.String() {
	case "esc":
		m.mode = configList
		m.input.Blur()
		return m, nil
	case "enter":
		item := m.input.Value()
		if strings.TrimSpace(item) == "" {
			m.setStatus("enter a value, or press esc", true)
			return m, nil
		}
		list := slices.Clone(m.list)
		cursor := m.listEditing
		if cursor < 0 {
			list = append(list, item)
			cursor = len(list) - 1
		} else {
			list[cursor] = item
		}
		if m.save(s, list) {
			m.list, m.listCursor = list, cursor
			m.mode = configList
			m.input.Blur()
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// displayValue is a text setting's value as it is edited: a data directory
// under home is shown with ~, as written in the default config.
func (m ConfigModel) displayValue(s config.Setting) string {
	switch v := s.Value(m.cfg).(type) {
	case int64:
		return strconv.FormatInt(v, 10)
	case string:
		if s.Name() == "storage.data_dir" {
			return collapseHome(v)
		}
		return v
	}
	return ""
}

func (m ConfigModel) contentWidth() int {
	if m.width <= 0 {
		return 80
	}
	return m.width
}

// pageSize is how many settings fit on the screen.
func (m ConfigModel) pageSize() int {
	if m.height <= 0 {
		return len(config.Settings)
	}
	return max(3, m.height-9)
}

func (m ConfigModel) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.styles.paneTitle.Render("Settings") + "  " + m.styles.muted.Render(collapseHome(m.path)) + "\n")
	switch m.mode {
	case configList, configListEdit:
		b.WriteString(m.renderList())
	case configRules, configRuleTry:
		b.WriteString(m.renderRules())
	case configRuleEdit:
		b.WriteString(m.renderRuleForm())
	default:
		if m.mode == configSearch || m.search.Value() != "" {
			b.WriteString(m.search.View() + "\n")
		}
		b.WriteString(m.renderSettings())
	}
	b.WriteString(m.renderDetail())
	b.WriteString(m.renderConfigHelp())
	return b.String()
}

// settingRows lays out the list of settings: a title row for each group, a
// blank row between groups, and a row for each setting. It returns the row of
// each visible setting, which rows are titles, and how many rows there are.
func (m ConfigModel) settingRows() (settingRow []int, titles map[int]bool, total int) {
	titles = map[int]bool{}
	group := ""
	for _, idx := range m.visible {
		if g := groupOf(idx); g != group {
			group = g
			if total > 0 {
				total++
			}
			titles[total] = true
			total++
		}
		settingRow = append(settingRow, total)
		total++
	}
	return settingRow, titles, total
}

// settingsHeight is how many rows of settings fit on the screen.
func (m ConfigModel) settingsHeight(total int) int {
	if m.height <= 0 {
		return total
	}
	return min(total, max(3, m.height-8))
}

// scrollSettings keeps the selected setting in view, moving the window only
// when it would leave it. A group's first setting brings its title along.
func (m *ConfigModel) scrollSettings() {
	settingRow, titles, total := m.settingRows()
	if m.cursor >= len(settingRow) {
		m.offset = 0
		return
	}
	cursorRow, height := settingRow[m.cursor], m.settingsHeight(total)
	m.offset = scrollTo(m.offset, cursorRow, height, total)
	if m.offset == cursorRow && titles[cursorRow-1] && height > 1 {
		m.offset--
	}
}

func (m ConfigModel) renderSettings() string {
	if len(m.visible) == 0 {
		return m.styles.muted.Render("  no settings match") + "\n"
	}
	settingRow, titles, total := m.settingRows()
	rows := make([]string, total)
	for row := range titles {
		// The setting after a title names its group.
		for i, r := range settingRow {
			if r == row+1 {
				rows[row] = " " + m.styles.paneTitle.Render(groupTitleOf(m.visible[i]))
				break
			}
		}
	}
	for i, idx := range m.visible {
		if idx == rulesRow {
			rows[settingRow[i]] = m.renderRulesRow(i == m.cursor)
			continue
		}
		rows[settingRow[i]] = m.renderRow(config.Settings[idx], i == m.cursor)
	}
	start := m.offset
	end := min(total, start+m.settingsHeight(total))
	return strings.Join(rows[start:end], "\n") + "\n"
}

func (m ConfigModel) renderRow(s config.Setting, selected bool) string {
	gutter := "  "
	title := padRight(s.Title, titleWidth)
	if selected {
		gutter = m.styles.accent.Render("❯ ")
		title = m.styles.id.Render(title)
	}
	value := m.renderValue(s, selected)
	if selected && m.mode == configEdit {
		value = m.input.View()
	}
	return " " + gutter + title + value
}

func (m ConfigModel) renderValue(s config.Setting, selected bool) string {
	room := max(10, m.contentWidth()-titleWidth-4)
	switch v := s.Value(m.cfg).(type) {
	case bool:
		if v {
			return m.styles.success.Render("● on")
		}
		return m.styles.muted.Render("○ off")
	case []string:
		text := fmt.Sprintf("%d · %s", len(v), strings.Join(v, ", "))
		if len(v) == 0 {
			text = "none"
		}
		return m.styles.muted.Render(truncate(text, room))
	}
	text := m.displayValue(s)
	switch s.Kind {
	case config.KindChoice:
		parts := make([]string, len(s.Choices))
		for i, c := range s.Choices {
			if c == text || (text == "" && i == 0) {
				parts[i] = m.styles.accent.Bold(true).Render("● " + c)
			} else {
				parts[i] = m.styles.muted.Render("○ " + c)
			}
		}
		return strings.Join(parts, "  ")
	case config.KindColor:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(text)).Render("■") + " " + text
	}
	if text == "" {
		if s.Name() == "shell.program" {
			return m.styles.muted.Render("$SHELL")
		}
		return m.styles.muted.Render("(empty)")
	}
	return truncate(text, room)
}

func (m ConfigModel) renderList() string {
	s, _ := m.selected()
	var b strings.Builder
	b.WriteString(" " + m.styles.paneTitle.Render(s.Title) + "\n")
	if len(m.list) == 0 && m.mode != configListEdit {
		b.WriteString(m.styles.muted.Render("  empty; press a to add one") + "\n")
	}
	room := max(10, m.contentWidth()-4)
	for i, item := range m.list {
		switch {
		case m.mode == configListEdit && i == m.listEditing:
			b.WriteString(" " + m.styles.accent.Render("❯ ") + m.input.View() + "\n")
		case i == m.listCursor && m.mode == configList:
			b.WriteString(" " + m.styles.accent.Render("❯ ") + m.styles.id.Render(truncate(item, room)) + "\n")
		default:
			b.WriteString("   " + truncate(item, room) + "\n")
		}
	}
	if m.mode == configListEdit && m.listEditing < 0 {
		b.WriteString(" " + m.styles.accent.Render("+ ") + m.input.View() + "\n")
	}
	return b.String()
}

func (m ConfigModel) renderDetail() string {
	if m.onRules() {
		return m.renderRulesDetail()
	}
	s, ok := m.selected()
	if !ok {
		return "\n"
	}
	source := "default"
	if src := m.sources[s.Name()]; src != "" {
		source = "set in " + collapseHome(src)
	}
	width := m.contentWidth() - 2
	var b strings.Builder
	b.WriteString("\n " + m.styles.label.Render(s.Name()) + m.styles.muted.Render(" · "+source) + "\n")
	for _, line := range strings.Split(ansi.Wordwrap(s.Summary, width, ""), "\n") {
		b.WriteString(" " + line + "\n")
	}
	return b.String()
}

func (m ConfigModel) renderConfigHelp() string {
	var help string
	switch m.mode {
	case configSearch:
		help = "type to search · enter done · esc clear"
	case configEdit, configListEdit:
		help = "enter save · esc cancel"
	case configList:
		help = "a add · enter edit · d delete · r reset to default · esc back"
	case configRules:
		help = "a add · enter edit · d delete · J/K move down/up · t try a command · esc back"
	case configRuleEdit:
		help = "tab next field · shift+tab back · enter save · esc cancel"
	case configRuleTry:
		help = "type a command · esc back"
	default:
		help = "↑/↓ move · enter change · ←/→ choose · r reset · / search · e edit file · q quit"
		if s, ok := m.selected(); ok && s.Kind == config.KindList {
			help = "↑/↓ move · enter edit list · r reset · / search · e edit file · q quit"
		}
		if m.onRules() {
			help = "↑/↓ move · enter edit rules · / search · e edit file · q quit"
		}
	}
	line := "\n" + m.styles.help.Render(" "+help)
	if m.status != "" {
		style := m.styles.success
		if m.statusErr {
			style = m.styles.danger
		}
		line = "\n " + style.Render(m.status) + line
	}
	return line
}
