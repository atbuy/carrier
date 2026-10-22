package store

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

func createNoteTestRun(t *testing.T, st *Store, command string, sessionID *int64) int64 {
	t.Helper()
	started := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	id, err := st.CreateRun(CreateRun{
		Status:    StatusRunning,
		Mode:      ModeShell,
		Command:   command,
		ArgvJSON:  `["` + command + `"]`,
		CWD:       "/tmp",
		StartedAt: started,
		SessionID: sessionID,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := st.FinishRun(id, StatusSuccess, 0, started.Add(time.Second)); err != nil {
		t.Fatalf("finish run: %v", err)
	}
	return id
}

func TestCreateAndListNotes(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	sessID, err := st.CreateSession(CreateSession{StartedAt: time.Now()})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	runID := createNoteTestRun(t, st, "dig example.com", &sessID)
	otherRun := createNoteTestRun(t, st, "make test", nil)

	created := time.Date(2026, 5, 10, 13, 0, 0, 0, time.UTC)
	first, err := st.CreateNote(CreateNote{SessionID: &sessID, RunID: &runID, Text: "suspect DNS", CreatedAt: created})
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if _, err := st.CreateNote(CreateNote{SessionID: &sessID, Text: "session start", CreatedAt: created}); err != nil {
		t.Fatalf("create session note: %v", err)
	}
	if _, err := st.CreateNote(CreateNote{RunID: &otherRun, Text: "flaky", CreatedAt: created}); err != nil {
		t.Fatalf("create run note: %v", err)
	}

	n, err := st.GetNote(first)
	if err != nil {
		t.Fatalf("get note: %v", err)
	}
	if n.Text != "suspect DNS" || n.RunID == nil || *n.RunID != runID || n.SessionID == nil || *n.SessionID != sessID {
		t.Fatalf("unexpected note: %+v", n)
	}
	if !n.CreatedAt.Equal(created) {
		t.Fatalf("created_at = %v, want %v", n.CreatedAt, created)
	}

	all, err := st.ListNotes(NoteFilter{}, 100)
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	if len(all) != 3 || all[0].ID != first {
		t.Fatalf("expected 3 notes oldest-first, got %+v", all)
	}

	bySession, err := st.ListNotes(NoteFilter{SessionIDs: []int64{sessID}}, 100)
	if err != nil {
		t.Fatalf("list session notes: %v", err)
	}
	if len(bySession) != 2 {
		t.Fatalf("expected 2 session notes, got %d", len(bySession))
	}

	byRun, err := st.ListNotes(NoteFilter{RunIDs: []int64{otherRun}}, 100)
	if err != nil {
		t.Fatalf("list run notes: %v", err)
	}
	if len(byRun) != 1 || byRun[0].Text != "flaky" {
		t.Fatalf("unexpected run notes: %+v", byRun)
	}

	limited, err := st.ListNotes(NoteFilter{}, 2)
	if err != nil {
		t.Fatalf("list limited notes: %v", err)
	}
	if len(limited) != 2 || limited[1].Text != "flaky" {
		t.Fatalf("limit should keep the newest notes oldest-first, got %+v", limited)
	}
}

func TestCreateNoteValidation(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	runID := createNoteTestRun(t, st, "echo hi", nil)
	if _, err := st.CreateNote(CreateNote{RunID: &runID, Text: "  ", CreatedAt: time.Now()}); err == nil {
		t.Fatal("expected error for empty note text")
	}
	if _, err := st.CreateNote(CreateNote{Text: "orphan", CreatedAt: time.Now()}); err == nil {
		t.Fatal("expected error for note without session or run")
	}
}

func TestNotesAreSearchable(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	runID := createNoteTestRun(t, st, "curl localhost", nil)
	noteID, err := st.CreateNote(CreateNote{RunID: &runID, Text: "proxy certificate expired", CreatedAt: time.Now()})
	if err != nil {
		t.Fatalf("create note: %v", err)
	}

	results, err := st.SearchRuns("certificate", 10, SearchFilter{})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 || results[0].Run.ID != runID {
		t.Fatalf("expected note text to find run %d, got %+v", runID, results)
	}

	if _, err := st.DeleteNote(noteID); err != nil {
		t.Fatalf("delete note: %v", err)
	}
	results, err = st.SearchRuns("certificate", 10, SearchFilter{})
	if err != nil {
		t.Fatalf("search after delete: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results after deleting note, got %+v", results)
	}
	if _, err := st.DeleteNote(noteID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows deleting missing note, got %v", err)
	}
}

func TestLatestSessionRun(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	sessID, err := st.CreateSession(CreateSession{StartedAt: time.Now()})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := st.LatestSessionRun(sessID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows for empty session, got %v", err)
	}
	createNoteTestRun(t, st, "echo one", &sessID)
	second := createNoteTestRun(t, st, "echo two", &sessID)
	createNoteTestRun(t, st, "echo outside", nil)

	r, err := st.LatestSessionRun(sessID)
	if err != nil {
		t.Fatalf("latest session run: %v", err)
	}
	if r.ID != second {
		t.Fatalf("latest session run = %d, want %d", r.ID, second)
	}
}

func TestPruneOrphanedNotes(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	sessID, err := st.CreateSession(CreateSession{StartedAt: time.Now()})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	sessRun := createNoteTestRun(t, st, "echo session", &sessID)
	soloRun := createNoteTestRun(t, st, "echo solo", nil)
	keptRun := createNoteTestRun(t, st, "echo kept", nil)

	sessNote, _ := st.CreateNote(CreateNote{SessionID: &sessID, RunID: &sessRun, Text: "in session", CreatedAt: time.Now()})
	soloNote, _ := st.CreateNote(CreateNote{RunID: &soloRun, Text: "solo", CreatedAt: time.Now()})
	keptNote, _ := st.CreateNote(CreateNote{RunID: &keptRun, Text: "kept", CreatedAt: time.Now()})

	for _, id := range []int64{sessRun, soloRun} {
		if _, err := st.DeleteByID(id); err != nil {
			t.Fatalf("delete run %d: %v", id, err)
		}
	}
	pruned, err := st.PruneOrphanedNotes()
	if err != nil {
		t.Fatalf("prune notes: %v", err)
	}
	if pruned != 1 {
		t.Fatalf("pruned = %d, want 1", pruned)
	}

	n, err := st.GetNote(sessNote)
	if err != nil {
		t.Fatalf("session note should be kept: %v", err)
	}
	if n.RunID != nil || n.SessionID == nil {
		t.Fatalf("session note should be detached from its run, got %+v", n)
	}
	if _, err := st.GetNote(soloNote); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("solo note should be deleted, got %v", err)
	}
	if _, err := st.GetNote(keptNote); err != nil {
		t.Fatalf("note on existing run should be kept: %v", err)
	}
}

func TestSessionOutcome(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	id, err := st.CreateSession(CreateSession{Label: "x", StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if o, err := st.SessionOutcome(id); o != nil || err != nil {
		t.Fatalf("no outcome yet: %+v, %v", o, err)
	}
	for _, n := range []CreateNote{
		{SessionID: &id, Text: "plain note"},
		{SessionID: &id, Text: "first guess", Kind: NoteOutcome},
		{SessionID: &id, Text: "the real cause", Kind: NoteOutcome},
	} {
		n.CreatedAt = time.Now()
		if _, err := st.CreateNote(n); err != nil {
			t.Fatal(err)
		}
	}
	o, err := st.SessionOutcome(id)
	if err != nil || o == nil || o.Text != "the real cause" || o.Kind != NoteOutcome {
		t.Fatalf("latest outcome = %+v, %v", o, err)
	}
	notes, _ := st.ListNotes(NoteFilter{SessionIDs: []int64{id}}, 10)
	if len(notes) != 3 || notes[0].Kind != "" || notes[1].Kind != NoteOutcome {
		t.Fatalf("notes = %+v", notes)
	}
}

func TestErrorLines(t *testing.T) {
	out := "building\nwarning: deprecated\nerror: cannot connect\nretrying\nfatal: giving up\ndone\n"
	if got := ErrorLines(out, 2); strings.Join(got, "|") != "error: cannot connect|fatal: giving up" {
		t.Fatalf("ErrorLines = %q", got)
	}
	if got := ErrorLines("all good\n\n", 3); strings.Join(got, "|") != "all good" {
		t.Fatalf("ErrorLines without errors = %q", got)
	}
}
