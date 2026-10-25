package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/atbuy/carrier/internal/store"
)

// fakeSessions is a Sessions source over sessions in memory.
type fakeSessions struct {
	sessions []store.SessionChoice
	labeled  map[int64]string
}

func (f *fakeSessions) source() Sessions {
	return Sessions{
		List: func() ([]store.SessionChoice, error) { return f.sessions, nil },
		Timeline: func(id int64, width int) string {
			return fmt.Sprintf("timeline of session %d", id)
		},
		Label: func(id int64, label string) error {
			f.labeled[id] = label
			for i := range f.sessions {
				if f.sessions[i].ID == id {
					f.sessions[i].Label = label
				}
			}
			return nil
		},
		Keep: func(id int64, kept bool) error {
			for i := range f.sessions {
				if f.sessions[i].ID == id {
					f.sessions[i].Kept = kept
				}
			}
			return nil
		},
	}
}

// browseModel is the browser over the seeded runs, the first two in session
// 7, with sessions 7 and 3.
func browseModel(t *testing.T) (Model, *fakeSessions) {
	t.Helper()
	st, cfg := seed(t)
	runs, err := st.AllRuns()
	if err != nil {
		t.Fatal(err)
	}
	seven := int64(7)
	runs[0].SessionID, runs[1].SessionID = &seven, &seven
	f := &fakeSessions{labeled: map[int64]string{}, sessions: []store.SessionChoice{
		{ID: 7, Label: "BUG-1234: fix login", StartedAt: time.Now(), Runs: 2, Ended: true},
		{ID: 3, StartedAt: time.Now().Add(-time.Hour), Runs: 1},
	}}
	m := New(st, cfg, runs)
	m.SetSessions(f.source())
	m, _ = send(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	return m, f
}

func keys(m Model, ks ...tea.KeyMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range ks {
		m, cmd = send(m, k)
	}
	return m, cmd
}

func typeText(s string) []tea.KeyMsg {
	var out []tea.KeyMsg
	for _, r := range s {
		out = append(out, runeKey(r))
	}
	return out
}

var tabKey = tea.KeyMsg{Type: tea.KeyTab}

func TestBrowseSessionsTab(t *testing.T) {
	m, _ := browseModel(t)
	if view := m.View(); !strings.Contains(view, "Runs") || !strings.Contains(view, "Sessions") || !strings.Contains(view, "tab sessions") {
		t.Fatalf("the browser should show its tabs:\n%s", view)
	}
	m, _ = keys(m, tabKey)
	view := m.View()
	for _, want := range []string{"BUG-1234: fix login · 2 runs", "no label · 1 run", "timeline of session 7", "enter attach"} {
		if !strings.Contains(view, want) {
			t.Errorf("sessions tab is missing %q:\n%s", want, view)
		}
	}
	m, _ = keys(m, tea.KeyMsg{Type: tea.KeyDown})
	if !strings.Contains(m.View(), "timeline of session 3") {
		t.Fatalf("moving should preview the next session:\n%s", m.View())
	}
	m, _ = keys(m, tabKey)
	if m.tab != tabRuns || !strings.Contains(m.View(), "go build") {
		t.Fatal("tab should go back to the runs")
	}
}

func TestBrowseKeepsFiltersPerTab(t *testing.T) {
	m, _ := browseModel(t)
	m, _ = keys(m, append(append([]tea.KeyMsg{runeKey('/')}, typeText("make")...), tea.KeyMsg{Type: tea.KeyEnter})...)
	if len(m.filtered) != 1 {
		t.Fatalf("run filter: %d runs", len(m.filtered))
	}
	m, _ = keys(m, tabKey)
	if len(m.filteredSessions) != 2 || m.filter.Value() != "" {
		t.Fatal("the sessions tab should start without the runs' filter")
	}
	m, _ = keys(m, append(append([]tea.KeyMsg{runeKey('/')}, typeText("bug")...), tea.KeyMsg{Type: tea.KeyEnter})...)
	if len(m.filteredSessions) != 1 || m.filteredSessions[0].ID != 7 {
		t.Fatalf("session filter: %+v", m.filteredSessions)
	}
	m, _ = keys(m, tabKey)
	if m.filter.Value() != "make" || len(m.filtered) != 1 {
		t.Fatalf("the runs' filter should come back, got %q", m.filter.Value())
	}
}

func TestBrowseAttachesToSession(t *testing.T) {
	m, _ := browseModel(t)
	m, cmd := keys(m, tabKey, tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyEnter})
	if !isQuit(cmd) || m.Result() != (Result{Action: ActionAttach, SessionID: 3}) {
		t.Fatalf("enter should attach to session 3: %+v", m.Result())
	}
}

func TestBrowseOpensSessionRuns(t *testing.T) {
	m, _ := browseModel(t)
	m, _ = keys(m, tabKey, runeKey('o'))
	if m.tab != tabRuns || m.runSession != 7 || len(m.filtered) != 2 {
		t.Fatalf("o should show the 2 runs of session 7: tab=%v session=%d runs=%d", m.tab, m.runSession, len(m.filtered))
	}
	if view := m.View(); !strings.Contains(view, "Runs (session 7)") || !strings.Contains(view, "esc all runs") {
		t.Fatalf("view:\n%s", view)
	}
	m, cmd := keys(m, tea.KeyMsg{Type: tea.KeyEsc})
	if isQuit(cmd) || m.runSession != 0 || len(m.filtered) != 3 {
		t.Fatal("esc should go back to all runs before quitting")
	}
	if _, cmd = keys(m, tea.KeyMsg{Type: tea.KeyEsc}); !isQuit(cmd) {
		t.Fatal("a second esc should quit")
	}
}

func TestBrowseLabelsAndKeepsSessions(t *testing.T) {
	m, f := browseModel(t)
	m, _ = keys(m, tabKey, tea.KeyMsg{Type: tea.KeyDown}, runeKey('l'))
	m, _ = keys(m, append(typeText("deploy"), tea.KeyMsg{Type: tea.KeyEnter})...)
	if f.labeled[3] != "deploy" || !strings.Contains(m.View(), "deploy · 1 run") {
		t.Fatalf("labeling: %v\n%s", f.labeled, m.View())
	}
	m, _ = keys(m, runeKey('K'))
	if !f.sessions[1].Kept || !strings.Contains(m.statusMsg, "kept session 3") {
		t.Fatalf("K should keep session 3: %+v %q", f.sessions[1], m.statusMsg)
	}
	m, _ = keys(m, runeKey('K'))
	if f.sessions[1].Kept || !strings.Contains(m.statusMsg, "expire again") {
		t.Fatalf("K again should let it expire: %q", m.statusMsg)
	}
}

func TestBrowseWithoutSessionsHasNoTabs(t *testing.T) {
	st, cfg := seed(t)
	m := newModel(t, st, cfg)
	m, _ = keys(m, tabKey)
	if m.tab != tabRuns || strings.Contains(m.View(), "Sessions") {
		t.Fatal("without a sessions source the browser shows only runs")
	}
}
