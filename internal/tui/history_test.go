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

func TestHistoryPreview(t *testing.T) {
	h := historyFixture()
	exit, ms := 1, int64(2400)
	h.Commands[0].ExitCode, h.Commands[0].DurationMS, h.Commands[0].GitBranch = &exit, &ms, "fix-login"
	reads := map[int64]int{}
	h.Output = func(r *store.Run, lines int) []string {
		reads[r.ID]++
		if r.ID == 9 {
			return []string{"--- FAIL: TestLogin (0.02s)", "\tlogin_test.go:42: token expired"}
		}
		return nil
	}
	tm, _ := NewHistory(h, "", config.Default().UI.Theme).Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m := tm.(HistoryModel)
	view := m.View()
	for _, want := range []string{
		"✗ exit 1 · 2.4s · 2h ago · /src/api · fix-login · run 9",
		"│ --- FAIL: TestLogin (0.02s)",
		"│     login_test.go:42: token expired",
		"ctrl+o preview",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("preview is missing %q:\n%s", want, view)
		}
	}
	// The preview keeps its height, so moving to a run with no output does not
	// move the help line.
	height := strings.Count(view, "\n")
	m, _ = historyKeys(m, tea.KeyMsg{Type: tea.KeyDown})
	view = m.View()
	if !strings.Contains(view, "✓ success · 2h ago · /src/infra · run 8") || !strings.Contains(view, "no output recorded") {
		t.Errorf("preview of run 8:\n%s", view)
	}
	if got := strings.Count(view, "\n"); got != height {
		t.Errorf("view is %d lines, was %d", got, height)
	}
	// Output is read once per run.
	m, _ = historyKeys(m, tea.KeyMsg{Type: tea.KeyUp})
	_ = m.View()
	if reads[9] != 1 || reads[8] != 1 {
		t.Errorf("output reads = %v", reads)
	}

	// ctrl+o hides it and shows it again.
	m, _ = historyKeys(m, tea.KeyMsg{Type: tea.KeyCtrlO})
	if view := m.View(); strings.Contains(view, "run 9") {
		t.Errorf("ctrl+o should hide the preview:\n%s", view)
	}
	m, _ = historyKeys(m, tea.KeyMsg{Type: tea.KeyCtrlO})
	if view := m.View(); !strings.Contains(view, "run 9") {
		t.Errorf("ctrl+o again should show the preview:\n%s", view)
	}
}

func TestHistoryPreviewNeedsRoom(t *testing.T) {
	h := historyFixture()
	h.Output = func(*store.Run, int) []string { return []string{"output"} }
	tm, _ := NewHistory(h, "", config.Default().UI.Theme).Update(tea.WindowSizeMsg{Width: 120, Height: 14})
	view := tm.(HistoryModel).View()
	if strings.Contains(view, "run 9") || strings.Contains(view, "ctrl+o") {
		t.Errorf("a short terminal has no room for the preview:\n%s", view)
	}
	if !strings.Contains(view, "kubectl apply") {
		t.Errorf("the commands should still be listed:\n%s", view)
	}
	// Without Output there is no preview at all.
	tm, _ = NewHistory(historyFixture(), "", config.Default().UI.Theme).Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if view := tm.(HistoryModel).View(); strings.Contains(view, "run 9") || strings.Contains(view, "ctrl+o") {
		t.Errorf("no preview without Output:\n%s", view)
	}
}
