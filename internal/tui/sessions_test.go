package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

func pickerItems() []SessionItem {
	started := time.Now().Add(-2 * time.Hour)
	return []SessionItem{
		{SessionChoice: store.SessionChoice{ID: 12, Label: "BUG-1234: fix login", StartedAt: started, Runs: 14}, Tags: []string{"pinned here"}},
		{SessionChoice: store.SessionChoice{ID: 9, StartedAt: started, Runs: 1, Ended: true}},
		{SessionChoice: store.SessionChoice{ID: 3, Label: "api refactor", StartedAt: started, Runs: 2, Kept: true}},
	}
}

func pick(m PickerModel, keys ...tea.KeyMsg) (PickerModel, tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		var tm tea.Model
		tm, cmd = m.Update(k)
		m = tm.(PickerModel)
	}
	return m, cmd
}

func typed(s string) []tea.KeyMsg {
	var keys []tea.KeyMsg
	for _, r := range s {
		keys = append(keys, runeKey(r))
	}
	return keys
}

func TestPickerShowsSessions(t *testing.T) {
	m := NewPicker(pickerItems(), config.Default().UI.Theme)
	view := m.View()
	for _, want := range []string{"Attach to a session", "❯ 12  BUG-1234: fix login", "14 runs · 2h ago · pinned here", "no label", "1 run · 2h ago · ended", "2 runs · 2h ago · kept"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
}

func TestPickerFiltersAndPicks(t *testing.T) {
	m := NewPicker(pickerItems(), config.Default().UI.Theme)
	m, _ = pick(m, typed("api ref")...)
	if len(m.visible) != 1 || strings.Contains(m.View(), "BUG-1234") {
		t.Fatalf("filtering by label words:\n%s", m.View())
	}
	m, cmd := pick(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.Picked() != 3 || cmd == nil || m.View() != "" {
		t.Fatalf("enter should pick session 3 and quit, picked %d", m.Picked())
	}

	m = NewPicker(pickerItems(), config.Default().UI.Theme)
	m, _ = pick(m, typed("9")...)
	if len(m.visible) != 1 {
		t.Fatalf("filtering by ID: %d sessions", len(m.visible))
	}
	m, _ = pick(m, typed("x")...)
	if !strings.Contains(m.View(), "no sessions match") {
		t.Fatalf("no match:\n%s", m.View())
	}
	if m, _ = pick(m, tea.KeyMsg{Type: tea.KeyEnter}); m.Picked() != 0 {
		t.Fatal("enter with nothing to pick should pick nothing")
	}
}

func TestPickerMovesAndCancels(t *testing.T) {
	m := NewPicker(pickerItems(), config.Default().UI.Theme)
	m, _ = pick(m, tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.cursor)
	}
	m, cmd := pick(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.Picked() != 0 || cmd == nil {
		t.Fatal("esc should quit without picking")
	}
}

// TestPickerFitsTheTerminal checks that the picker shows up to 20 sessions,
// fewer in a short terminal, and never a line wider than the terminal.
func TestPickerFitsTheTerminal(t *testing.T) {
	var items []SessionItem
	for i := 30; i > 0; i-- {
		items = append(items, SessionItem{
			SessionChoice: store.SessionChoice{ID: int64(i), Label: strings.Repeat("long label ", 6), StartedAt: time.Now(), Runs: i},
			Tags:          []string{"pinned here"},
		})
	}
	for _, tc := range []struct{ width, height, rows int }{{120, 50, 20}, {120, 15, 10}, {40, 4, 1}} {
		tm, _ := NewPicker(items, config.Default().UI.Theme).Update(tea.WindowSizeMsg{Width: tc.width, Height: tc.height})
		view := tm.(PickerModel).View()
		lines := strings.Split(view, "\n")
		if got := strings.Count(view, " runs · "); got != tc.rows {
			t.Errorf("%dx%d: %d sessions shown, want %d", tc.width, tc.height, got, tc.rows)
		}
		if tc.height > pickerChrome && len(lines) > tc.height-1 {
			t.Errorf("%dx%d: %d lines, more than fit below the prompt", tc.width, tc.height, len(lines))
		}
		for _, line := range lines {
			if w := ansi.StringWidth(line); w >= tc.width {
				t.Errorf("%dx%d: line is %d wide: %q", tc.width, tc.height, w, line)
			}
		}
	}
}
