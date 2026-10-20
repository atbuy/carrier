package store

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestNthLatestRunAndLabeledLists(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	var ids []int64
	for _, label := range []string{"", "deploy", "", "BUG-1234 fix"} {
		id, err := st.CreateRun(CreateRun{
			Status: StatusSuccess, Mode: ModeRun, Command: "echo", ArgvJSON: `["echo"]`,
			CWD: "/tmp", Label: label, StartedAt: time.Now(),
		})
		if err != nil {
			t.Fatalf("create run: %v", err)
		}
		ids = append(ids, id)
	}
	for n, want := range map[int]int64{1: ids[3], 2: ids[2], 4: ids[0]} {
		r, err := st.NthLatestRun(n)
		if err != nil || r.ID != want {
			t.Fatalf("NthLatestRun(%d) = %v, %v; want %d", n, r, err, want)
		}
	}
	for _, n := range []int{0, 5} {
		if _, err := st.NthLatestRun(n); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("NthLatestRun(%d) err = %v, want sql.ErrNoRows", n, err)
		}
	}

	labeled, err := st.ListLabeledRuns()
	if err != nil {
		t.Fatalf("ListLabeledRuns: %v", err)
	}
	if len(labeled) != 2 || labeled[0].ID != ids[3] || labeled[1].ID != ids[1] {
		t.Fatalf("labeled runs = %+v, want newest-first labeled only", labeled)
	}

	for _, label := range []string{"BUG-1", "", "PROJ-2"} {
		if _, err := st.CreateSession(CreateSession{Label: label, StartedAt: time.Now()}); err != nil {
			t.Fatalf("create session: %v", err)
		}
	}
	sessions, err := st.ListLabeledSessions()
	if err != nil {
		t.Fatalf("ListLabeledSessions: %v", err)
	}
	if len(sessions) != 2 || sessions[0].Label != "PROJ-2" || sessions[1].Label != "BUG-1" {
		t.Fatalf("labeled sessions = %+v", sessions)
	}
}
