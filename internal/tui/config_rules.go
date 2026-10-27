package tui

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/gitmeta"
)

// The auto-label rules are a row of their own after the settings, in a group
// of their own; Enter opens them as a list of rules.

// rulesRow is the index in ConfigModel.visible that stands for the rules.
var rulesRow = len(config.Settings)

// rulesName names the rules in the status line and on exit.
const rulesName = "auto-label rules"

const rulesSummary = "Rules that label runs by their command, directory, or Git branch, when no --label is given. The first rule that matches labels the run."

// ruleFields are the fields of the rule form, in order.
var ruleFields = []struct{ title, placeholder string }{
	{"Label", "such as tests, or feature ${branch}"},
	{"Command", "regular expression, such as go test.*"},
	{"Directory", "regular expression, matched from the start"},
	{"Git branch", "regular expression, such as feat/(?P<branch>.+)"},
}

// ruleState is what the rules screens keep.
type ruleState struct {
	// rules are the main config's rules, which can be edited; included
	// counts the rules from includes after them.
	rules    []config.AutoLabel
	included int
	cursor   int
	// editing is the rule in the form, or -1 for a new one; field is the
	// field focused.
	editing int
	field   int
	inputs  []textinput.Model
	// deleting is set while "y" would delete the rule selected.
	deleting bool
	try      textinput.Model
	// tryDir and tryBranch are where a command tried runs: here.
	tryDir, tryBranch string
}

func newRuleState() ruleState {
	r := ruleState{inputs: make([]textinput.Model, len(ruleFields))}
	for i, f := range ruleFields {
		in := textinput.New()
		in.Prompt = ""
		in.Placeholder = f.placeholder
		r.inputs[i] = in
	}
	r.try = textinput.New()
	r.try.Prompt = "Try › "
	r.try.Placeholder = "type a command to see the label it gets here"
	return r
}

func (m *ConfigModel) loadRules() {
	rules, err := config.MainAutoLabels(m.path)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.rule.rules = rules
	m.rule.included = max(0, len(m.cfg.AutoLabels)-len(rules))
	m.rule.cursor = max(0, min(m.rule.cursor, len(rules)-1))
}

func (m ConfigModel) onRules() bool {
	return m.cursor >= 0 && m.cursor < len(m.visible) && m.visible[m.cursor] == rulesRow
}

func rulesMatch(q string) bool {
	return q == "" || strings.Contains("auto labels auto-label rules auto_label", q)
}

// groupOf is the group of row idx in visible, for its title.
func groupOf(idx int) string {
	if idx == rulesRow {
		return "auto_label"
	}
	return config.Settings[idx].Group()
}

func groupTitleOf(idx int) string {
	if idx == rulesRow {
		return "Auto labels"
	}
	return config.GroupTitle(config.Settings[idx].Group())
}

// rulesChanged records a change to the rules, as saved does for a setting.
func (m *ConfigModel) rulesChanged(status string) {
	if !slices.Contains(m.changed, rulesName) {
		m.changed = append(m.changed, rulesName)
	}
	if !slices.Contains(m.files, m.path) {
		m.files = append(m.files, m.path)
	}
	if err := m.reload(); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.setStatus(status, false)
}

func (m ConfigModel) openRules() (tea.Model, tea.Cmd) {
	m.mode = configRules
	m.rule.deleting = false
	m.loadRules()
	m.setStatus("", false)
	return m, nil
}

func (m ConfigModel) updateRules(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	r := &m.rule
	if r.deleting {
		r.deleting = false
		if msg.String() == "y" && r.cursor < len(r.rules) {
			label := r.rules[r.cursor].Label
			if err := config.RemoveAutoLabel(m.path, r.cursor); err != nil {
				m.setStatus(err.Error(), true)
				return m, nil
			}
			m.rulesChanged(fmt.Sprintf("deleted rule %d, %q", r.cursor+1, label))
			return m, nil
		}
		m.setStatus("kept the rule", false)
		return m, nil
	}
	switch msg.String() {
	case "esc", "q", "left", "h":
		m.mode = configBrowse
		m.setStatus("", false)
	case "up", "k":
		if r.cursor > 0 {
			r.cursor--
		}
	case "down", "j":
		if r.cursor < len(r.rules)-1 {
			r.cursor++
		}
	case "a", "o":
		return m.editRule(-1)
	case "enter", " ", "right", "l":
		if r.cursor < len(r.rules) {
			return m.editRule(r.cursor)
		}
		if len(r.rules) == 0 {
			return m.editRule(-1)
		}
	case "d", "x", "delete", "backspace":
		if r.cursor < len(r.rules) {
			r.deleting = true
			m.setStatus(fmt.Sprintf("delete rule %d, %q? y deletes it, any other key keeps it", r.cursor+1, r.rules[r.cursor].Label), true)
		}
	case "K", "shift+up":
		return m.moveRule(-1)
	case "J", "shift+down":
		return m.moveRule(1)
	case "t", "/":
		m.mode = configRuleTry
		if r.tryDir == "" {
			r.tryDir, _ = os.Getwd()
			r.tryBranch = gitmeta.Collect(r.tryDir).Branch
		}
		r.try.SetValue("")
		r.try.Focus()
		m.setStatus("", false)
		return m, textinput.Blink
	}
	return m, nil
}

func (m ConfigModel) moveRule(step int) (tea.Model, tea.Cmd) {
	r := &m.rule
	to := r.cursor + step
	if r.cursor >= len(r.rules) || to < 0 || to >= len(r.rules) {
		return m, nil
	}
	if err := config.MoveAutoLabel(m.path, r.cursor, to); err != nil {
		m.setStatus(err.Error(), true)
		return m, nil
	}
	label := r.rules[r.cursor].Label
	r.cursor = to
	m.rulesChanged(fmt.Sprintf("moved %q to rule %d", label, to+1))
	return m, nil
}

// editRule opens the form for rule i, or for a new rule when i is -1.
func (m ConfigModel) editRule(i int) (tea.Model, tea.Cmd) {
	r := &m.rule
	r.editing, r.field = i, 0
	var rule config.AutoLabel
	if i >= 0 {
		rule = r.rules[i]
	}
	for f, v := range []string{rule.Label, rule.Cmd, rule.Dir, rule.GitBranch} {
		r.inputs[f].SetValue(v)
		r.inputs[f].CursorEnd()
		r.inputs[f].Blur()
	}
	r.inputs[0].Focus()
	m.mode = configRuleEdit
	m.setStatus("", false)
	return m, textinput.Blink
}

func (m ConfigModel) updateRuleEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	r := &m.rule
	focus := func(f int) (tea.Model, tea.Cmd) {
		r.inputs[r.field].Blur()
		r.field = (f + len(r.inputs)) % len(r.inputs)
		r.inputs[r.field].Focus()
		return m, textinput.Blink
	}
	switch msg.String() {
	case "esc":
		m.mode = configRules
		m.setStatus("", false)
		return m, nil
	case "tab", "down":
		return focus(r.field + 1)
	case "shift+tab", "up":
		return focus(r.field - 1)
	case "enter":
		rule := config.AutoLabel{
			Label:     strings.TrimSpace(r.inputs[0].Value()),
			Cmd:       r.inputs[1].Value(),
			Dir:       r.inputs[2].Value(),
			GitBranch: r.inputs[3].Value(),
		}
		if err := rule.Validate(); err != nil {
			m.setStatus(err.Error(), true)
			return m, nil
		}
		var err error
		status := ""
		if r.editing < 0 {
			err = config.AddAutoLabel(m.path, rule)
			status = fmt.Sprintf("added rule %d, %q", len(r.rules)+1, rule.Label)
		} else {
			err = config.SetAutoLabel(m.path, r.editing, rule)
			status = fmt.Sprintf("saved rule %d, %q", r.editing+1, rule.Label)
		}
		if err != nil {
			m.setStatus(err.Error(), true)
			return m, nil
		}
		if r.editing < 0 {
			r.cursor = len(r.rules)
		}
		m.mode = configRules
		m.rulesChanged(status)
		return m, nil
	}
	var cmd tea.Cmd
	r.inputs[r.field], cmd = r.inputs[r.field].Update(msg)
	return m, cmd
}

func (m ConfigModel) updateRuleTry(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter":
		m.mode = configRules
		m.rule.try.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.rule.try, cmd = m.rule.try.Update(msg)
	return m, cmd
}

// renderRulesRow is the rules' row in the list of settings.
func (m ConfigModel) renderRulesRow(selected bool) string {
	gutter, title := "  ", padRight("Rules", titleWidth)
	if selected {
		gutter, title = m.styles.accent.Render("❯ "), m.styles.id.Render(title)
	}
	n := len(m.cfg.AutoLabels)
	text := "none"
	if n > 0 {
		labels := make([]string, n)
		for i, rule := range m.cfg.AutoLabels {
			labels[i] = rule.Label
		}
		text = fmt.Sprintf("%d · %s", n, strings.Join(labels, ", "))
	}
	return " " + gutter + title + m.styles.muted.Render(truncate(text, max(10, m.contentWidth()-titleWidth-4)))
}

func (m ConfigModel) renderRules() string {
	r := m.rule
	var b strings.Builder
	b.WriteString(" " + m.styles.paneTitle.Render("Auto-label rules") + m.styles.muted.Render(" · the first rule that matches labels the run") + "\n")
	if len(r.rules) == 0 {
		b.WriteString(m.styles.muted.Render("  no rules yet; press a to add one") + "\n")
	}
	width := m.contentWidth()
	for i, rule := range r.rules {
		num := fmt.Sprintf("%2d  ", i+1)
		label := padRight(truncate(rule.Label, 24), 24)
		line := "   " + m.styles.muted.Render(num) + label
		if i == r.cursor && m.mode == configRules {
			line = " " + m.styles.accent.Render("❯ ") + m.styles.muted.Render(num) + m.styles.id.Render(label)
		}
		line += "  " + m.styles.muted.Render(ruleConditions(rule))
		b.WriteString(ansi.Truncate(line, width-1, "…") + "\n")
	}
	if r.included > 0 {
		b.WriteString(m.styles.muted.Render(fmt.Sprintf("  + %d after these from includes, edited in their files", r.included)) + "\n")
	}
	if m.mode == configRuleTry {
		b.WriteString("\n " + r.try.View() + "\n " + m.tryResult() + "\n")
	}
	return b.String()
}

// ruleConditions says what a rule matches: "cmd go test.* · dir /src".
func ruleConditions(rule config.AutoLabel) string {
	var parts []string
	for _, c := range []struct{ name, pattern string }{{"cmd", rule.Cmd}, {"dir", rule.Dir}, {"branch", rule.GitBranch}} {
		if c.pattern != "" {
			parts = append(parts, c.name+" "+c.pattern)
		}
	}
	if len(parts) == 0 {
		return "every run"
	}
	return strings.Join(parts, " · ")
}

// tryResult says which rule labels the command typed, run here.
func (m ConfigModel) tryResult() string {
	r := m.rule
	command := strings.TrimSpace(r.try.Value())
	if command == "" {
		return m.styles.muted.Render("  in " + collapseHome(r.tryDir) + branchSuffix(r.tryBranch))
	}
	label, i := m.cfg.MatchAutoLabel(command, r.tryDir, r.tryBranch)
	where := " in " + collapseHome(r.tryDir) + branchSuffix(r.tryBranch)
	if i < 0 {
		return m.styles.muted.Render("  no rule matches" + where)
	}
	from := fmt.Sprintf("rule %d", i+1)
	if i >= len(r.rules) {
		from = "a rule from an include"
	}
	return "  → " + m.styles.label.Render(label) + m.styles.muted.Render(" from "+from+where)
}

func branchSuffix(branch string) string {
	if branch == "" {
		return ""
	}
	return " on " + branch
}

func (m ConfigModel) renderRuleForm() string {
	r := m.rule
	title := "New rule"
	if r.editing >= 0 {
		title = fmt.Sprintf("Rule %d", r.editing+1)
	}
	var b strings.Builder
	b.WriteString(" " + m.styles.paneTitle.Render(title) + "\n")
	for i, f := range ruleFields {
		name := padRight(f.title, 14)
		gutter := "   "
		if i == r.field {
			gutter = " " + m.styles.accent.Render("❯ ")
			name = m.styles.id.Render(name)
		}
		b.WriteString(gutter + name + r.inputs[i].View() + "\n")
	}
	return b.String()
}

func (m ConfigModel) renderRulesDetail() string {
	width := m.contentWidth() - 2
	text := rulesSummary
	switch m.mode {
	case configRuleEdit:
		text = "Patterns are regular expressions, and every one that is set must match. The label can use ${name} for a (?P<name>...) group in a pattern, or ${1} for the first group."
	case configRules, configRuleTry:
		text += " Rules are saved to " + collapseHome(m.path) + " as [[auto_label]] tables, keeping its comments."
	}
	var b strings.Builder
	b.WriteString("\n " + m.styles.label.Render("auto_label") + m.styles.muted.Render(fmt.Sprintf(" · %d %s", len(m.cfg.AutoLabels), pluralWord(len(m.cfg.AutoLabels), "rule"))) + "\n")
	for _, line := range strings.Split(ansi.Wordwrap(text, width, ""), "\n") {
		b.WriteString(" " + line + "\n")
	}
	return b.String()
}

func pluralWord(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
