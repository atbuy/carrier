package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
)

func createNoteSessionRun(t *testing.T, st *store.Store, sessID int64, command string) int64 {
	t.Helper()
	started := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	id, err := st.CreateRun(store.CreateRun{
		Status:    store.StatusRunning,
		Mode:      store.ModeShell,
		Command:   command,
		ArgvJSON:  fmt.Sprintf(`[%q]`, command),
		CWD:       "/tmp",
		StartedAt: started,
		SessionID: &sessID,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := st.FinishRun(id, store.StatusSuccess, 0, started.Add(time.Second)); err != nil {
		t.Fatalf("finish run: %v", err)
	}
	return id
}

func runNote(t *testing.T, a *app, flags map[string]string, args ...string) string {
	t.Helper()
	cmd := a.noteCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	for k, v := range flags {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("set flag %s: %v", k, err)
		}
	}
	if err := cmd.RunE(cmd, args); err != nil {
		t.Fatalf("noteCmd RunE failed: %v", err)
	}
	return out.String()
}

func TestNoteCmdAttachesToLatestRun(t *testing.T) {
	t.Setenv("CARRIER_SESSION_ID", "")
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	createFinishedRun(t, st, store.StatusSuccess, "echo one", `["echo","one"]`, "/tmp")
	latest := createFinishedRun(t, st, store.StatusFailed, "make test", `["make","test"]`, "/tmp")

	out := runNote(t, &app{st: st}, nil, "flaky", "on", "CI")
	if !strings.Contains(out, fmt.Sprintf("added to run %d", latest)) {
		t.Fatalf("unexpected output: %s", out)
	}
	notes, err := st.ListNotes(store.NoteFilter{RunIDs: []int64{latest}}, 10)
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	if len(notes) != 1 || notes[0].Text != "flaky on CI" {
		t.Fatalf("unexpected notes: %+v", notes)
	}
}

func TestNoteCmdRunFlag(t *testing.T) {
	t.Setenv("CARRIER_SESSION_ID", "")
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	first := createFinishedRun(t, st, store.StatusSuccess, "echo one", `["echo","one"]`, "/tmp")
	createFinishedRun(t, st, store.StatusSuccess, "echo two", `["echo","two"]`, "/tmp")

	runNote(t, &app{st: st}, map[string]string{"run": fmt.Sprint(first)}, "first run")
	notes, _ := st.ListNotes(store.NoteFilter{RunIDs: []int64{first}}, 10)
	if len(notes) != 1 {
		t.Fatalf("expected note on run %d, got %+v", first, notes)
	}
}

func TestNoteCmdSessionFlagByLabel(t *testing.T) {
	t.Setenv("CARRIER_SESSION_ID", "")
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	sessID, _ := st.CreateSession(store.CreateSession{Label: "backend-debug", StartedAt: time.Now()})

	out := runNote(t, &app{st: st}, map[string]string{"session": "backend-debug"}, "starting")
	if !strings.Contains(out, fmt.Sprintf("added to session %d", sessID)) {
		t.Fatalf("unexpected output: %s", out)
	}
	notes, _ := st.ListNotes(store.NoteFilter{SessionIDs: []int64{sessID}}, 10)
	if len(notes) != 1 || notes[0].RunID != nil {
		t.Fatalf("expected one session note without run, got %+v", notes)
	}
}

func TestNoteCmdNoRuns(t *testing.T) {
	t.Setenv("CARRIER_SESSION_ID", "")
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	cmd := (&app{st: st}).noteCmd()
	if err := cmd.RunE(cmd, []string{"hello"}); err == nil || !strings.Contains(err.Error(), "no runs") {
		t.Fatalf("expected no runs error, got %v", err)
	}
}

func TestNoteCmdInShellDropsOwnRun(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	sessID, _ := st.CreateSession(store.CreateSession{StartedAt: time.Now()})
	prev := createNoteSessionRun(t, st, sessID, "dig example.com")

	// Simulate the shell hook having recorded the note command itself.
	noteRun, err := st.CreateRun(store.CreateRun{
		Status: store.StatusRunning, Mode: store.ModeShell, Command: `c note "suspect DNS; flushing"`,
		ArgvJSON: `["c note"]`, CWD: "/tmp", StartedAt: time.Now(), SessionID: &sessID,
	})
	if err != nil {
		t.Fatalf("create note run: %v", err)
	}
	logPath := filepath.Join(t.TempDir(), "terminal.log")
	if err := os.WriteFile(logPath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	_ = st.UpdatePaths(noteRun, "", "", logPath)
	statePath := filepath.Join(t.TempDir(), "state.json")
	sf := &carriershell.StateFile{Path: statePath}
	if err := sf.Write(carriershell.State{CurrentID: noteRun, CurrentLog: logPath, SessionID: sessID}); err != nil {
		t.Fatalf("write state: %v", err)
	}
	t.Setenv("CARRIER_SESSION_ID", fmt.Sprint(sessID))
	t.Setenv("CARRIER_SHELL_STATE", statePath)

	runNote(t, &app{st: st}, nil, "suspect DNS; flushing")

	if _, err := st.GetRun(noteRun); err == nil {
		t.Fatal("note command run should be deleted")
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("note run log should be removed, stat err = %v", err)
	}
	if got := (&carriershell.StateFile{Path: statePath}).Read(); got.CurrentID != 0 || got.SessionID != sessID {
		t.Fatalf("state should be reset to session only, got %+v", got)
	}
	notes, _ := st.ListNotes(store.NoteFilter{SessionIDs: []int64{sessID}}, 10)
	if len(notes) != 1 || notes[0].RunID == nil || *notes[0].RunID != prev {
		t.Fatalf("note should follow run %d, got %+v", prev, notes)
	}
}

func TestNoteCmdInShellKeepsCompoundRun(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	sessID, _ := st.CreateSession(store.CreateSession{StartedAt: time.Now()})
	compound, err := st.CreateRun(store.CreateRun{
		Status: store.StatusRunning, Mode: store.ModeShell, Command: "make test && carrier note done",
		ArgvJSON: `["make test && carrier note done"]`, CWD: "/tmp", StartedAt: time.Now(), SessionID: &sessID,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	statePath := filepath.Join(t.TempDir(), "state.json")
	_ = (&carriershell.StateFile{Path: statePath}).Write(carriershell.State{CurrentID: compound, SessionID: sessID})
	t.Setenv("CARRIER_SESSION_ID", fmt.Sprint(sessID))
	t.Setenv("CARRIER_SHELL_STATE", statePath)

	runNote(t, &app{st: st}, nil, "done")

	if _, err := st.GetRun(compound); err != nil {
		t.Fatalf("compound run must be kept: %v", err)
	}
	notes, _ := st.ListNotes(store.NoteFilter{RunIDs: []int64{compound}}, 10)
	if len(notes) != 1 {
		t.Fatalf("note should attach to the compound run, got %+v", notes)
	}
}

func TestIsNoteInvocation(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"carrier note hello", true},
		{"c note hello world", true},
		{"/usr/local/bin/carrier -q note hi", true},
		{"c nt quick", true},
		{`c note "a; b | c"`, true},
		{`c note 'it''s $HOME'`, true},
		{"c note a; rm -rf x", false},
		{"make test && c note done", false},
		{"c note $(whoami)", false},
		{"echo note | c note", false},
		{"c show 1", false},
		{"note", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := isNoteInvocation(tc.line); got != tc.want {
			t.Errorf("isNoteInvocation(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}

func TestNotesCmdListFilterAndRemove(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	t.Setenv("CARRIER_SESSION_ID", "")
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	sessID, _ := st.CreateSession(store.CreateSession{Label: "dbg", StartedAt: time.Now()})
	runID := createFinishedRun(t, st, store.StatusSuccess, "echo hi", `["echo","hi"]`, "/tmp")
	sessNote, _ := st.CreateNote(store.CreateNote{SessionID: &sessID, Text: "session thought", CreatedAt: time.Now()})
	_, _ = st.CreateNote(store.CreateNote{RunID: &runID, Text: "run thought", CreatedAt: time.Now()})

	a := &app{st: st}
	list := func(flags map[string]string) string {
		cmd := a.notesCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		for k, v := range flags {
			_ = cmd.Flags().Set(k, v)
		}
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("notesCmd failed: %v", err)
		}
		return out.String()
	}

	all := list(nil)
	if !strings.Contains(all, "session thought") || !strings.Contains(all, "run thought") {
		t.Fatalf("expected both notes, got:\n%s", all)
	}
	if got := list(map[string]string{"session": "dbg"}); strings.Contains(got, "run thought") || !strings.Contains(got, "session thought") {
		t.Fatalf("session filter wrong, got:\n%s", got)
	}
	if got := list(map[string]string{"run": fmt.Sprint(runID)}); strings.Contains(got, "session thought") || !strings.Contains(got, "run thought") {
		t.Fatalf("run filter wrong, got:\n%s", got)
	}
	t.Setenv("CARRIER_SESSION_ID", fmt.Sprint(sessID))
	if got := list(nil); strings.Contains(got, "run thought") {
		t.Fatalf("inside a session only that session's notes should be listed, got:\n%s", got)
	}
	t.Setenv("CARRIER_SESSION_ID", "")

	var views []noteView
	if err := json.Unmarshal([]byte(list(map[string]string{"json": "true"})), &views); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if len(views) != 2 || views[0].Text != "session thought" {
		t.Fatalf("unexpected JSON notes: %+v", views)
	}

	rm := a.notesRmCmd()
	var out bytes.Buffer
	rm.SetOut(&out)
	if err := rm.RunE(rm, []string{fmt.Sprint(sessNote)}); err != nil {
		t.Fatalf("notes rm failed: %v", err)
	}
	if !strings.Contains(out.String(), "deleted") {
		t.Fatalf("unexpected rm output: %s", out.String())
	}
	if err := rm.RunE(rm, []string{fmt.Sprint(sessNote)}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestHistoryCmdShowsNotes(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	sessID, _ := st.CreateSession(store.CreateSession{Label: "dbg", StartedAt: time.Now()})
	_, _ = st.CreateNote(store.CreateNote{SessionID: &sessID, Text: "before anything", CreatedAt: time.Now()})
	first := createNoteSessionRun(t, st, sessID, "dig example.com")
	_, _ = st.CreateNote(store.CreateNote{SessionID: &sessID, RunID: &first, Text: "suspect DNS", CreatedAt: time.Now()})
	createNoteSessionRun(t, st, sessID, "flush-dns")
	solo := createFinishedRun(t, st, store.StatusSuccess, "make test", `["make","test"]`, "/tmp")
	_, _ = st.CreateNote(store.CreateNote{RunID: &solo, Text: "passes now", CreatedAt: time.Now()})

	cmd := (&app{st: st}).historyCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("historyCmd failed: %v", err)
	}
	got := out.String()
	order := []string{"dbg", "note: before anything", "dig example.com", "│  note: suspect DNS", "flush-dns", "make test", "note: passes now"}
	pos := 0
	for _, want := range order {
		idx := strings.Index(got[pos:], want)
		if idx < 0 {
			t.Fatalf("expected %q after position %d in:\n%s", want, pos, got)
		}
		pos += idx + len(want)
	}
}

func TestShowCmdShowsNotes(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	runID := createFinishedRun(t, st, store.StatusFailed, "make test", `["make","test"]`, "/tmp")
	_, _ = st.CreateNote(store.CreateNote{RunID: &runID, Text: "fails only on main", CreatedAt: time.Now()})

	a := &app{st: st}
	cmd := a.showCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, []string{fmt.Sprint(runID)}); err != nil {
		t.Fatalf("showCmd failed: %v", err)
	}
	if !strings.Contains(out.String(), "notes") || !strings.Contains(out.String(), "fails only on main") {
		t.Fatalf("expected notes section, got:\n%s", out.String())
	}

	cmd = a.showCmd()
	out.Reset()
	cmd.SetOut(&out)
	_ = cmd.Flags().Set("json", "true")
	if err := cmd.RunE(cmd, []string{fmt.Sprint(runID)}); err != nil {
		t.Fatalf("showCmd --json failed: %v", err)
	}
	var view runView
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if len(view.Notes) != 1 || view.Notes[0].Text != "fails only on main" {
		t.Fatalf("expected note in JSON, got %+v", view.Notes)
	}
}
