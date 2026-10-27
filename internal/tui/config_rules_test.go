package tui

import (
	"strings"
	"testing"
)

const rulesConfig = `# Tests in any project.
[[auto_label]]
label = "tests" # shown in history
cmd = 'go test.*'

[[auto_label]]
label = "feature ${branch}"
git_branch = 'feat/(?P<branch>.+)'
`

// openRules selects the rules row by searching for it, and opens it.
func openRulesScreen(t *testing.T, m ConfigModel) ConfigModel {
	t.Helper()
	m = press(m, "/", "auto label", "enter")
	if !m.onRules() {
		t.Fatalf("searching for auto label did not select the rules")
	}
	view := m.View()
	if !strings.Contains(view, "Rules") || !strings.Contains(view, "2 · tests, feature ${branch}") {
		t.Fatalf("rules row:\n%s", view)
	}
	m = press(m, "enter")
	if m.mode != configRules {
		t.Fatalf("enter did not open the rules")
	}
	return m
}

func TestConfigListsRules(t *testing.T) {
	m, _ := configModel(t, rulesConfig)
	m = openRulesScreen(t, m)
	view := m.View()
	for _, want := range []string{
		"Auto-label rules · the first rule that matches labels the run",
		"1  tests",
		"cmd go test.*",
		"2  feature ${branch}",
		"branch feat/(?P<branch>.+)",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("rules view is missing %q:\n%s", want, view)
		}
	}
}

func TestConfigAddsEditsMovesAndDeletesRules(t *testing.T) {
	m, path := configModel(t, rulesConfig)
	m = openRulesScreen(t, m)

	// Add a rule: label, then the command after tab.
	m = press(m, "a", "deploy", "tab", "make deploy", "enter")
	if m.mode != configRules || !strings.Contains(m.status, `added rule 3, "deploy"`) {
		t.Fatalf("add: mode %v, status %q", m.mode, m.status)
	}
	if got := fileContent(t, path); !strings.HasSuffix(got, "\n[[auto_label]]\nlabel = \"deploy\"\ncmd = \"make deploy\"\n") {
		t.Fatalf("add wrote:\n%s", got)
	}

	// Move it before the second rule.
	m = press(m, "K")
	if m.rule.cursor != 1 || m.rule.rules[1].Label != "deploy" || m.rule.rules[2].Label != "feature ${branch}" {
		t.Fatalf("move: cursor %d, rules %+v", m.rule.cursor, m.rule.rules)
	}

	// Edit the first rule's label; its comment and pattern stay as written.
	m = press(m, "up", "enter", "ctrl+u", "unit tests", "enter")
	got := fileContent(t, path)
	if !strings.Contains(got, "# Tests in any project.\n[[auto_label]]\nlabel = \"unit tests\" # shown in history\ncmd = 'go test.*'\n") {
		t.Fatalf("edit wrote:\n%s", got)
	}

	// A bad pattern is refused, and the form stays open.
	m = press(m, "enter", "tab", "(", "enter")
	if m.mode != configRuleEdit || !m.statusErr || !strings.Contains(m.status, "not a regular expression") {
		t.Fatalf("bad pattern: mode %v, status %q", m.mode, m.status)
	}
	m = press(m, "esc")

	// Delete asks first; only y deletes.
	m = press(m, "down", "d", "n")
	if len(m.rule.rules) != 3 {
		t.Fatalf("n should keep the rule: %+v", m.rule.rules)
	}
	m = press(m, "d", "y")
	if len(m.rule.rules) != 2 || strings.Contains(fileContent(t, path), "deploy") {
		t.Fatalf("y should delete the rule:\n%s", fileContent(t, path))
	}
	if got := m.Summary(); !strings.Contains(got, "saved the auto-label rules to") {
		t.Errorf("summary = %q", got)
	}
}

func TestConfigTriesACommand(t *testing.T) {
	m, _ := configModel(t, rulesConfig)
	m = openRulesScreen(t, m)
	m = press(m, "t", "go test ./...")
	if view := m.View(); !strings.Contains(view, "→ tests from rule 1") {
		t.Errorf("try go test:\n%s", view)
	}
	m = press(m, "ctrl+u", "ls")
	if view := m.View(); !strings.Contains(view, "no rule matches") {
		t.Errorf("try ls:\n%s", view)
	}
	m = press(m, "esc")
	if m.mode != configRules {
		t.Errorf("esc should go back to the rules")
	}
}

func TestConfigAddsTheFirstRule(t *testing.T) {
	m, path := configModel(t, "")
	m = press(m, "/", "rules", "enter", "enter")
	if view := m.View(); !strings.Contains(view, "no rules yet; press a to add one") {
		t.Fatalf("empty rules:\n%s", view)
	}
	m = press(m, "enter", "build", "tab", "make", "enter")
	if got := fileContent(t, path); !strings.Contains(got, "[[auto_label]]\nlabel = \"build\"\ncmd = \"make\"\n") {
		t.Fatalf("first rule:\n%s", got)
	}
	if len(m.cfg.AutoLabels) != 1 {
		t.Fatalf("the config was not reloaded: %+v", m.cfg.AutoLabels)
	}
}
