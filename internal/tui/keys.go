package tui

import tea "github.com/charmbracelet/bubbletea"

// eachKey splits a key message holding several characters, as a paste or
// typing faster than the screen reads delivers, into one message per
// character. Each is then handled as if typed, so pasting "/min" where "/"
// opens the search searches for "min".
func eachKey(msg tea.Msg) []tea.Msg {
	k, ok := msg.(tea.KeyMsg)
	if !ok || k.Type != tea.KeyRunes || len(k.Runes) <= 1 {
		return nil
	}
	keys := make([]tea.Msg, len(k.Runes))
	for i, r := range k.Runes {
		keys[i] = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: k.Alt}
	}
	return keys
}

// updateEach sends msgs to m in order, and returns the commands they gave.
func updateEach(m tea.Model, msgs []tea.Msg) (tea.Model, tea.Cmd) {
	cmds := make([]tea.Cmd, 0, len(msgs))
	for _, msg := range msgs {
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}
