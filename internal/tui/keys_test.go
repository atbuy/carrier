package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/atbuy/carrier/internal/config"
)

// pasted is text arriving in one key message: a paste in a terminal with
// bracketed paste, or without it, characters typed faster than read.
func pasted(text string, bracketed bool) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: bracketed}
}

func TestPastedTextActsAsTyped(t *testing.T) {
	for _, bracketed := range []bool{false, true} {
		// "/" opens the search in carrier config; the rest is searched.
		m, _ := configModel(t, "")
		tm, _ := m.Update(pasted("/min", bracketed))
		cm := tm.(ConfigModel)
		if cm.mode != configSearch || cm.search.Value() != "min" {
			t.Errorf("config, bracketed=%v: mode %v search %q, want the search for min", bracketed, cm.mode, cm.search.Value())
		}

		// In the run browser too.
		st, cfg := seed(t)
		bm, _ := newModel(t, st, cfg).Update(pasted("/lint", bracketed))
		if b := bm.(Model); b.mode != modeFilter || b.filter.Value() != "lint" || len(b.filtered) != 1 {
			t.Errorf("browse, bracketed=%v: mode %v filter %q, %d runs", bracketed, b.mode, b.filter.Value(), len(b.filtered))
		}

		// The pickers type it into their filter.
		hm, _ := NewHistory(historyFixture(), "", config.Default().UI.Theme).Update(pasted("kubectl", bracketed))
		if h := hm.(HistoryModel); h.filter.Value() != "kubectl" || len(h.visible) != 1 {
			t.Errorf("history, bracketed=%v: filter %q, %d commands", bracketed, h.filter.Value(), len(h.visible))
		}
		pm, _ := NewPicker(pickerItems(), config.Default().UI.Theme).Update(pasted("api ref", bracketed))
		if p := pm.(PickerModel); p.filter.Value() != "api ref" || len(p.visible) != 1 {
			t.Errorf("session picker, bracketed=%v: filter %q, %d sessions", bracketed, p.filter.Value(), len(p.visible))
		}
	}
}
