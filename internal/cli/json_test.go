package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/store"
)

func seedJSONSessions(t *testing.T, st *store.Store) (manual, auto int64) {
	t.Helper()
	start := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	manual, err := st.CreateSession(store.CreateSession{Label: "BUG-1234: fix login", StartedAt: start})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := st.EndSession(manual, start.Add(time.Hour)); err != nil {
		t.Fatalf("end session: %v", err)
	}
	auto, err = st.CreateSession(store.CreateSession{StartedAt: start.Add(2 * time.Hour), Auto: true})
	if err != nil {
		t.Fatalf("create auto session: %v", err)
	}
	if err := st.SetSessionKept(auto, true); err != nil {
		t.Fatalf("keep session: %v", err)
	}
	return manual, auto
}

func TestRunJSONIncludesSessionID(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	sessID, _ := seedJSONSessions(t, st)
	now := time.Now()
	if _, err := st.CreateRun(store.CreateRun{
		Status: store.StatusRunning, Mode: store.ModeShell, Command: "make test",
		CWD: "/tmp", StartedAt: now, SessionID: &sessID,
	}); err != nil {
		t.Fatalf("create session run: %v", err)
	}
	if _, err := st.CreateRun(store.CreateRun{
		Status: store.StatusRunning, Mode: store.ModeRun, Command: "ls", CWD: "/tmp", StartedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	a := &app{st: st}
	out, _, err := runEmpty(t, a.historyCmd(), nil, map[string]string{"json": "true"})
	if err != nil {
		t.Fatalf("history --json: %v", err)
	}
	var views []runView
	if err := json.Unmarshal([]byte(out), &views); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out)
	}
	if len(views) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(views))
	}
	if views[0].SessionID == nil || *views[0].SessionID != sessID {
		t.Fatalf("session run session_id = %v, want %d", views[0].SessionID, sessID)
	}
	if views[1].SessionID != nil {
		t.Fatalf("standalone run session_id = %v, want none", *views[1].SessionID)
	}
	if strings.Count(out, `"session_id"`) != 1 {
		t.Fatalf("session_id should be omitted for standalone runs:\n%s", out)
	}
}

func TestFailedCmdJSON(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	a := &app{st: st}

	out, errOut, err := runEmpty(t, listStatusCmd("failed", store.StatusFailed, a), nil, map[string]string{"json": "true"})
	if err != nil {
		t.Fatalf("failed --json: %v", err)
	}
	if strings.TrimSpace(out) != "[]" || errOut != "" {
		t.Fatalf("empty failed --json: stdout=%q stderr=%q", out, errOut)
	}

	createFinishedRun(t, st, store.StatusFailed, "make lint", `["make","lint"]`, "/tmp")
	createFinishedRun(t, st, store.StatusSuccess, "make build", `["make","build"]`, "/tmp")
	out, _, err = runEmpty(t, listStatusCmd("failed", store.StatusFailed, a), nil, map[string]string{"json": "true"})
	if err != nil {
		t.Fatalf("failed --json: %v", err)
	}
	var views []runView
	if err := json.Unmarshal([]byte(out), &views); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out)
	}
	if len(views) != 1 || views[0].Command != "make lint" || views[0].Status != store.StatusFailed {
		t.Fatalf("unexpected views: %#v", views)
	}
}

func TestSessionListJSON(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	a := &app{st: st}

	out, errOut, err := runEmpty(t, a.sessionListCmd(), nil, map[string]string{"json": "true"})
	if err != nil {
		t.Fatalf("session list --json: %v", err)
	}
	if strings.TrimSpace(out) != "[]" || errOut != "" {
		t.Fatalf("empty session list --json: stdout=%q stderr=%q", out, errOut)
	}

	manual, auto := seedJSONSessions(t, st)
	out, _, err = runEmpty(t, a.sessionListCmd(), nil, map[string]string{"json": "true"})
	if err != nil {
		t.Fatalf("session list --json: %v", err)
	}
	var views []sessionView
	if err := json.Unmarshal([]byte(out), &views); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out)
	}
	if len(views) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(views))
	}
	// Newest first, like the text output.
	if views[0].ID != auto || !views[0].Auto || !views[0].Kept || views[0].EndedAt != nil {
		t.Fatalf("auto session view = %#v", views[0])
	}
	if views[1].ID != manual || views[1].Label != "BUG-1234: fix login" || views[1].Auto || views[1].EndedAt == nil {
		t.Fatalf("manual session view = %#v", views[1])
	}
}

func TestSessionsOnlyJSON(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	manual, _ := seedJSONSessions(t, st)
	a := &app{st: st}

	out, errOut, err := runEmpty(t, a.historyCmd(), nil, map[string]string{"sessions-only": "true", "json": "true"})
	if err != nil {
		t.Fatalf("history --sessions-only --json: %v", err)
	}
	var views []sessionView
	if err := json.Unmarshal([]byte(out), &views); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out)
	}
	// Oldest first, like history.
	if len(views) != 2 || views[0].ID != manual || errOut != "" {
		t.Fatalf("unexpected views: %#v stderr=%q", views, errOut)
	}

	out, errOut, err = runEmpty(t, a.historyCmd(), nil, map[string]string{"sessions-only": "true", "json": "true", "label": "web"})
	if err != nil {
		t.Fatalf("history --sessions-only --json --label: %v", err)
	}
	if strings.TrimSpace(out) != "[]" || errOut != "" {
		t.Fatalf("no matching sessions: stdout=%q stderr=%q", out, errOut)
	}
}
