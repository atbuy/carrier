package store

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestSessionAutoKeptAndEmpty(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	auto, err := st.CreateSession(CreateSession{StartedAt: time.Now(), Auto: true})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	manual, _ := st.CreateSession(CreateSession{StartedAt: time.Now()})
	sess, err := st.GetSession(auto)
	if err != nil || !sess.Auto || sess.Kept {
		t.Fatalf("auto session = %+v, %v", sess, err)
	}
	if sess, _ := st.GetSession(manual); sess.Auto {
		t.Fatal("manual session should not be auto")
	}

	if err := st.SetSessionKept(auto, true); err != nil {
		t.Fatalf("SetSessionKept: %v", err)
	}
	if sess, _ := st.GetSession(auto); !sess.Kept {
		t.Fatal("session should be kept")
	}
	if err := st.SetSessionKept(auto, false); err != nil {
		t.Fatalf("unkeep: %v", err)
	}
	if err := st.SetSessionKept(999, true); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing session: err = %v", err)
	}

	empty, err := st.SessionIsEmpty(auto)
	if err != nil || !empty {
		t.Fatalf("new session should be empty: %v, %v", empty, err)
	}
	if _, err := st.CreateRun(CreateRun{
		Status: StatusSuccess, Mode: ModeShell, Command: "ls", ArgvJSON: `["ls"]`,
		CWD: "/tmp", StartedAt: time.Now(), SessionID: &auto,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if empty, _ := st.SessionIsEmpty(auto); empty {
		t.Fatal("session with a run is not empty")
	}
	if _, err := st.CreateNote(CreateNote{SessionID: &manual, Text: "idea", CreatedAt: time.Now()}); err != nil {
		t.Fatalf("create note: %v", err)
	}
	if empty, _ := st.SessionIsEmpty(manual); empty {
		t.Fatal("session with a note is not empty")
	}
	labeled, _ := st.CreateSession(CreateSession{Label: "x", StartedAt: time.Now(), Auto: true})
	if empty, _ := st.SessionIsEmpty(labeled); empty {
		t.Fatal("labeled session is not empty")
	}
}

func TestExpiredAutoSessionsAndDelete(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	old := time.Now().Add(-60 * 24 * time.Hour)
	mk := func(label string, auto bool, ended *time.Time) int64 {
		t.Helper()
		id, err := st.CreateSession(CreateSession{Label: label, StartedAt: old, Auto: auto})
		if err != nil {
			t.Fatalf("create session: %v", err)
		}
		if ended != nil {
			if err := st.EndSession(id, *ended); err != nil {
				t.Fatalf("end session: %v", err)
			}
		}
		return id
	}
	recent := time.Now().Add(-time.Hour)
	expired := mk("", true, &old)
	mk("", false, &old)          // manual: never expires
	mk("labeled", true, &old)    // labeled: kept
	mk("", true, nil)            // still open
	mk("", true, &recent)        // ended recently
	pinned := mk("", true, &old) // kept
	_ = st.SetSessionKept(pinned, true)
	noted := mk("", true, &old)
	_, _ = st.CreateNote(CreateNote{SessionID: &noted, Text: "why", CreatedAt: old})

	runID, err := st.CreateRun(CreateRun{
		Status: StatusFailed, Mode: ModeShell, Command: "make", ArgvJSON: `["make"]`,
		CWD: "/tmp", StartedAt: old, SessionID: &expired,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := st.UpdatePaths(runID, "", "", "/tmp/run.log"); err != nil {
		t.Fatalf("update paths: %v", err)
	}
	if _, err := st.CreateNote(CreateNote{RunID: &runID, Text: "run note", CreatedAt: old}); err != nil {
		t.Fatalf("create run note: %v", err)
	}

	got, err := st.ExpiredAutoSessions(time.Now().Add(-30 * 24 * time.Hour))
	if err != nil {
		t.Fatalf("ExpiredAutoSessions: %v", err)
	}
	if len(got) != 1 || got[0].ID != expired {
		t.Fatalf("expired sessions = %+v, want only %d", got, expired)
	}

	runs, err := st.DeleteSession(expired)
	if err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if len(runs) != 1 || runs[0].TerminalOutputPath != "/tmp/run.log" {
		t.Fatalf("deleted runs = %+v", runs)
	}
	if _, err := st.GetSession(expired); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("session should be gone: %v", err)
	}
	if _, err := st.GetRun(runID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("session runs should be gone: %v", err)
	}
	if notes, _ := st.ListNotes(NoteFilter{RunIDs: []int64{runID}}, 10); len(notes) != 0 {
		t.Fatalf("notes on deleted runs should be gone: %+v", notes)
	}
	if res, _ := st.SearchRuns("make", 10, SearchFilter{}); len(res) != 0 {
		t.Fatalf("search entries should be gone: %+v", res)
	}
	if notes, _ := st.ListNotes(NoteFilter{SessionIDs: []int64{noted}}, 10); len(notes) != 1 {
		t.Fatal("other sessions' notes must survive")
	}
}
