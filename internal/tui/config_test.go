package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// configModel opens the editor on a config holding content, or on no config
// when content is empty.
func configModel(t *testing.T, content string) (ConfigModel, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "carrier", "config.toml")
	if content != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m, err := NewConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 60})
	return tm.(ConfigModel), path
}

// press sends keys: names such as "enter" and "down", or text typed one
// character at a time.
func press(m ConfigModel, keys ...string) ConfigModel {
	for _, k := range keys {
		var msgs []tea.KeyMsg
		switch k {
		case "enter":
			msgs = []tea.KeyMsg{{Type: tea.KeyEnter}}
		case "esc":
			msgs = []tea.KeyMsg{{Type: tea.KeyEsc}}
		case "up":
			msgs = []tea.KeyMsg{{Type: tea.KeyUp}}
		case "down":
			msgs = []tea.KeyMsg{{Type: tea.KeyDown}}
		case "left":
			msgs = []tea.KeyMsg{{Type: tea.KeyLeft}}
		case "right":
			msgs = []tea.KeyMsg{{Type: tea.KeyRight}}
		case "space":
			msgs = []tea.KeyMsg{{Type: tea.KeySpace, Runes: []rune{' '}}}
		case "backspace":
			msgs = []tea.KeyMsg{{Type: tea.KeyBackspace}}
		case "ctrl+u":
			msgs = []tea.KeyMsg{{Type: tea.KeyCtrlU}}
		default:
			for _, r := range k {
				msgs = append(msgs, runeKey(r))
			}
		}
		for _, msg := range msgs {
			tm, _ := m.Update(msg)
			m = tm.(ConfigModel)
		}
	}
	return m
}

// find selects the setting named name by searching for it.
func find(t *testing.T, m ConfigModel, name string) ConfigModel {
	t.Helper()
	m = press(m, "/", name, "enter")
	if s, ok := m.selected(); !ok || s.Name() != name {
		t.Fatalf("searching %q selected %+v", name, s)
	}
	return m
}

func fileContent(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestConfigListsSettingsInGroups(t *testing.T) {
	m, _ := configModel(t, "")
	view := m.View()
	for _, want := range []string{"Storage", "Notifications", "Appearance", "Data directory", "Branch sessions", "○ off", "● on", "storage.data_dir · default"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
	m = press(m, "/", "theme")
	if view := m.View(); strings.Contains(view, "Storage") || !strings.Contains(view, "Accent color") {
		t.Fatalf("search for theme:\n%s", view)
	}
	if m = press(m, "esc"); len(m.visible) == 0 || m.search.Value() != "" {
		t.Fatal("esc should clear the search")
	}
}

func TestConfigTogglesAndChoices(t *testing.T) {
	m, path := configModel(t, "# mine\n[shell]\nask_outcome = false # ask me\n")
	m = press(find(t, m, "shell.ask_outcome"), "space")
	if got := fileContent(t, path); got != "# mine\n[shell]\nask_outcome = true # ask me\n" {
		t.Fatalf("after toggling:\n%s", got)
	}
	m = press(find(t, m, "shell.branch_sessions"), "enter")
	if !strings.Contains(fileContent(t, path), `branch_sessions = "follow"`) {
		t.Fatalf("enter should pick the next choice:\n%s", fileContent(t, path))
	}
	m = press(m, "left", "left")
	if !strings.Contains(fileContent(t, path), `branch_sessions = "off"`) {
		t.Fatalf("left should go back and wrap around:\n%s", fileContent(t, path))
	}
	if !strings.Contains(m.View(), "set in "+collapseHome(path)) {
		t.Errorf("the view should say where the setting is set:\n%s", m.View())
	}
	m = press(m, "r")
	if strings.Contains(fileContent(t, path), "branch_sessions") || m.cfg.Shell.BranchSessions != "suggest" {
		t.Fatalf("reset should remove the setting:\n%s", fileContent(t, path))
	}
	if want := "saved 2 settings to " + collapseHome(path); !strings.Contains(m.Summary(), want) {
		t.Errorf("summary = %q, want %q", m.Summary(), want)
	}
}

func TestConfigEditsText(t *testing.T) {
	m, path := configModel(t, "")
	m = press(find(t, m, "notify.min_duration"), "enter", "ctrl+u", "soon", "enter")
	if m.mode != configEdit || !m.statusErr || !strings.Contains(m.status, "invalid duration") {
		t.Fatalf("an invalid duration should stay in the editor with an error: mode=%v status=%q", m.mode, m.status)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("an invalid value should not be written")
	}
	m = press(m, "ctrl+u", "2m", "enter")
	if m.mode != configBrowse || !strings.Contains(fileContent(t, path), `min_duration = "2m"`) {
		t.Fatalf("mode=%v config:\n%s", m.mode, fileContent(t, path))
	}
	m = press(find(t, m, "storage.max_output_mb"), "enter", "ctrl+u", "lots", "enter")
	if !strings.Contains(m.status, "whole number") {
		t.Fatalf("status = %q", m.status)
	}
	m = press(m, "esc", "enter", "ctrl+u", "50", "enter")
	if !strings.Contains(fileContent(t, path), "max_output_mb = 50") {
		t.Fatalf("config:\n%s", fileContent(t, path))
	}
}

func TestConfigEditsLists(t *testing.T) {
	m, path := configModel(t, "[shell]\nignore_commands = [\"vim\", \"less\"]\n")
	m = press(find(t, m, "shell.ignore_commands"), "enter")
	if m.mode != configList || !strings.Contains(m.View(), "less") {
		t.Fatalf("enter should open the list:\n%s", m.View())
	}
	m = press(m, "a", "k9s", "enter")
	if !strings.Contains(fileContent(t, path), `ignore_commands = ["vim", "less", "k9s"]`) {
		t.Fatalf("after adding:\n%s", fileContent(t, path))
	}
	m = press(m, "up", "up", "d")
	if !strings.Contains(fileContent(t, path), `ignore_commands = ["less", "k9s"]`) {
		t.Fatalf("after deleting:\n%s", fileContent(t, path))
	}
	m = press(m, "enter", "ctrl+u", "most", "enter", "esc")
	if !strings.Contains(fileContent(t, path), `ignore_commands = ["most", "k9s"]`) || m.mode != configBrowse {
		t.Fatalf("after editing:\n%s", fileContent(t, path))
	}
}

func TestConfigOpensEditor(t *testing.T) {
	m, path := configModel(t, "[ui]\ncolor = \"auto\"\n")
	var opened string
	m.editor = func(p string) (*exec.Cmd, error) {
		opened = p
		return exec.Command("true"), nil
	}
	tm, cmd := m.Update(runeKey('e'))
	if cmd == nil || opened != path {
		t.Fatalf("e should open %s, opened %q", path, opened)
	}
	// The editor changed the file; the model reads it again.
	if err := os.WriteFile(path, []byte("[ui]\ncolor = \"never\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tm, _ = tm.Update(editorDoneMsg{})
	if m = tm.(ConfigModel); m.cfg.UI.Color != "never" || !strings.Contains(m.status, "reloaded") {
		t.Fatalf("after the editor: color=%q status=%q", m.cfg.UI.Color, m.status)
	}
}

func TestNewConfigRejectsBrokenConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[ui\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewConfig(path); err == nil {
		t.Fatal("a config that does not load should not open")
	}
}
