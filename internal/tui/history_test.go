package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

func historyFixture() History {
	at := time.Now().Add(-2 * time.Hour)
	seven := int64(7)
	run := func(id int64, command, status, cwd string, session *int64) store.Run {
		return store.Run{ID: id, Command: command, Status: status, CWD: cwd, StartedAt: at, SessionID: session}
	}
	return History{
		Commands: []store.CommandChoice{
			{Run: run(9, "go test ./...", store.StatusFailed, "/src/api", &seven), Runs: 4},
			{Run: run(8, "kubectl apply -f k8s", store.StatusSuccess, "/src/infra", nil), Runs: 1},
			{Run: run(5, "git status", store.StatusSuccess, "/src/api", nil), Runs: 1},
		},
		Labels:    map[int64]string{7: "BUG-1234: fix login"},
		CWD:       "/src/api",
		SessionID: 7,
	}
}

func historyKeys(m HistoryModel, keys ...tea.KeyMsg) (HistoryModel, tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		var tm tea.Model
		tm, cmd = m.Update(k)
		m = tm.(HistoryModel)
	}
	return m, cmd
}

func TestHistoryListsCommands(t *testing.T) {
	tm, _ := NewHistory(historyFixture(), "", config.Default().UI.Theme).Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	view := tm.(HistoryModel).View()
	for _, want := range []string{"Search history · every terminal", "❯ ✗ go test ./...", "2h ago · /src/api · BUG-1234: fix login · 4 runs", "kubectl apply -f k8s"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
}

func TestHistoryFiltersScopesAndPicks(t *testing.T) {
	// It starts from the line typed so far.
	m := NewHistory(historyFixture(), "git", config.Default().UI.Theme)
	if len(m.visible) != 1 {
		t.Fatalf("query git: %d commands", len(m.visible))
	}
	m, _ = historyKeys(m, tea.KeyMsg{Type: tea.KeyBackspace}, tea.KeyMsg{Type: tea.KeyBackspace}, tea.KeyMsg{Type: tea.KeyBackspace})
	if len(m.visible) != 3 {
		t.Fatalf("empty query: %d commands", len(m.visible))
	}
	m, _ = historyKeys(m, tea.KeyMsg{Type: tea.KeyTab})
	if len(m.visible) != 2 || !strings.Contains(m.View(), "this directory") {
		t.Fatalf("this directory: %d commands\n%s", len(m.visible), m.View())
	}
	m, _ = historyKeys(m, tea.KeyMsg{Type: tea.KeyTab})
	if len(m.visible) != 1 || !strings.Contains(m.View(), "this session") {
		t.Fatalf("this session: %d commands", len(m.visible))
	}
	m, _ = historyKeys(m, tea.KeyMsg{Type: tea.KeyTab})
	if len(m.visible) != 3 {
		t.Fatal("tab should come back to every terminal")
	}
	m, cmd := historyKeys(m, tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyEnter})
	if m.Picked() != "kubectl apply -f k8s" || cmd == nil || m.View() != "" {
		t.Fatalf("enter should pick the second command, picked %q", m.Picked())
	}
}

func TestHistoryCancels(t *testing.T) {
	m, cmd := historyKeys(NewHistory(historyFixture(), "", config.Default().UI.Theme), tea.KeyMsg{Type: tea.KeyEsc})
	if m.Picked() != "" || cmd == nil {
		t.Fatal("esc should quit without picking")
	}
	// Outside a session, Tab skips "this session".
	h := historyFixture()
	h.SessionID = 0
	m = NewHistory(h, "", config.Default().UI.Theme)
	m, _ = historyKeys(m, tea.KeyMsg{Type: tea.KeyTab}, tea.KeyMsg{Type: tea.KeyTab})
	if m.scope != scopeAll {
		t.Fatalf("scope = %v, want every terminal", m.scope)
	}
}
