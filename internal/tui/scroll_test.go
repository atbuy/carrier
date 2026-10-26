package tui

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

func TestScrollTo(t *testing.T) {
	for _, tc := range []struct{ offset, cursor, rows, total, want int }{
		{0, 3, 5, 20, 0},    // in view: stays
		{0, 7, 5, 20, 3},    // below: the cursor becomes the last row
		{5, 6, 5, 20, 5},    // moving up inside the window: stays
		{5, 2, 5, 20, 2},    // above: the cursor becomes the first row
		{15, 19, 5, 20, 15}, // at the end
		{15, 2, 5, 4, 0},    // fewer rows than fit
		{3, 3, 0, 20, 0},
	} {
		if got := scrollTo(tc.offset, tc.cursor, tc.rows, tc.total); got != tc.want {
			t.Errorf("scrollTo(%d, %d, %d, %d) = %d, want %d", tc.offset, tc.cursor, tc.rows, tc.total, got, tc.want)
		}
	}
}

var (
	keyDown = tea.KeyMsg{Type: tea.KeyDown}
	keyUp   = tea.KeyMsg{Type: tea.KeyUp}
)

// press sends key n times to m.
func pressN(m tea.Model, key tea.KeyMsg, n int) tea.Model {
	for range n {
		m, _ = m.Update(key)
	}
	return m
}

func TestPickersScrollOnlyAtTheEdges(t *testing.T) {
	var items []SessionItem
	var commands []store.CommandChoice
	for i := 30; i > 0; i-- {
		items = append(items, SessionItem{SessionChoice: store.SessionChoice{ID: int64(i), StartedAt: time.Now()}})
		commands = append(commands, store.CommandChoice{Run: store.Run{ID: int64(i), Command: fmt.Sprintf("make step-%d", i)}})
	}
	// 10 rows tall: 5 rows of entries.
	size := tea.WindowSizeMsg{Width: 120, Height: 10}
	picker, _ := NewPicker(items, config.Default().UI.Theme).Update(size)
	history, _ := NewHistory(History{Commands: commands}, "", config.Default().UI.Theme).Update(size)
	for name, m := range map[string]tea.Model{"session picker": picker, "history": history} {
		m = pressN(m, keyDown, 9) // cursor 9: rows 5–9 show
		m = pressN(m, keyUp, 2)   // cursor 7: still rows 5–9
		var offset int
		switch v := m.(type) {
		case PickerModel:
			offset = v.offset
		case HistoryModel:
			offset = v.offset
		}
		if offset != 5 {
			t.Errorf("%s: offset = %d after moving up inside the window, want 5", name, offset)
		}
		m = pressN(m, keyUp, 3) // cursor 4: the window moves up one
		switch v := m.(type) {
		case PickerModel:
			offset = v.offset
		case HistoryModel:
			offset = v.offset
		}
		if offset != 4 {
			t.Errorf("%s: offset = %d above the window, want 4", name, offset)
		}
	}
}

func TestBrowserScrollsOnlyAtTheEdges(t *testing.T) {
	st, cfg := seed(t)
	for range 20 {
		if _, err := st.CreateRun(store.CreateRun{Status: store.StatusSuccess, Mode: store.ModeRun, Command: "true", ArgvJSON: `["true"]`, StartedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	m := newModel(t, st, cfg)
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 8}) // 6 rows of runs
	tm = pressN(tm, keyDown, 10)
	before := tm.(Model).runOffset
	tm = pressN(tm, keyUp, 2)
	if got := tm.(Model).runOffset; got != before || before == 0 {
		t.Fatalf("offset %d → %d after moving up inside the window", before, got)
	}
}

func TestConfigScrollsOnlyAtTheEdges(t *testing.T) {
	m, _ := configModel(t, "")
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 16}) // 8 rows of the list
	tm = pressN(tm, keyDown, 12)
	before := tm.(ConfigModel).offset
	tm = pressN(tm, keyUp, 2)
	if got := tm.(ConfigModel).offset; got != before || before == 0 {
		t.Fatalf("offset %d → %d after moving up inside the window", before, got)
	}
	// Scrolling back up to a group's first setting shows its title too.
	tm = pressN(tm, keyUp, 20)
	if got := tm.(ConfigModel).offset; got != 0 {
		t.Fatalf("offset = %d at the top, want 0 so the Storage title shows", got)
	}
}
