package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/store"
)

func openHistoryStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return st
}

func seedHistoryRuns(t *testing.T, st *store.Store) {
	t.Helper()
	day := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	for i, cmd := range []struct {
		command, argv, status string
	}{
		{"go test ./...", `["go","test","./..."]`, store.StatusSuccess},
		{"make lint", `["make","lint"]`, store.StatusFailed},
		{"npm run dev", `["npm","run","dev"]`, store.StatusSuccess},
	} {
		id, err := st.CreateRun(store.CreateRun{
			Status:    store.StatusRunning,
			Mode:      store.ModeRun,
			Command:   cmd.command,
			ArgvJSON:  cmd.argv,
			CWD:       "/tmp/project",
			StartedAt: day.Add(time.Duration(i) * time.Hour),
		})
		if err != nil {
			t.Fatalf("create run %d: %v", i, err)
		}
		if err := st.FinishRun(id, cmd.status, 0, day.Add(time.Duration(i)*time.Hour+time.Second)); err != nil {
			t.Fatalf("finish run %d: %v", i, err)
		}
	}
}

func TestHistoryCmd(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st := openHistoryStore(t)
	defer func() { _ = st.Close() }()
	seedHistoryRuns(t, st)

	a := &app{st: st}
	cmd := a.historyCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("historyCmd failed: %v", err)
	}
	for _, want := range []string{"go test ./...", "make lint", "npm run dev"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("history output missing %q:\n%s", want, out.String())
		}
	}
	assertInOrder(t, out.String(), "go test ./...", "make lint", "npm run dev")
}

func assertInOrder(t *testing.T, haystack string, needles ...string) {
	t.Helper()
	pos := -1
	for _, needle := range needles {
		next := strings.Index(haystack[pos+1:], needle)
		if next < 0 {
			t.Fatalf("missing %q in output:\n%s", needle, haystack)
		}
		pos += next + 1
	}
}

func TestHistoryCmdJSON(t *testing.T) {
	st := openHistoryStore(t)
	defer func() { _ = st.Close() }()
	seedHistoryRuns(t, st)

	a := &app{st: st}
	cmd := a.historyCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("json", "true"); err != nil {
		t.Fatalf("set json flag: %v", err)
	}

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("historyCmd JSON failed: %v", err)
	}

	var views []runView
	if err := json.Unmarshal(out.Bytes(), &views); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out.String())
	}
	if len(views) != 3 {
		t.Fatalf("expected 3 views, got %d", len(views))
	}
	if got := views[0].Command; got != "go test ./..." {
		t.Fatalf("first JSON history command = %q, want oldest run", got)
	}
	if got := views[2].Command; got != "npm run dev" {
		t.Fatalf("last JSON history command = %q, want newest run", got)
	}
}

func TestHistoryCmdFilterByStatus(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st := openHistoryStore(t)
	defer func() { _ = st.Close() }()
	seedHistoryRuns(t, st)

	a := &app{st: st}
	cmd := a.historyCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("status", "failed"); err != nil {
		t.Fatalf("set status flag: %v", err)
	}

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("historyCmd status filter failed: %v", err)
	}
	if !strings.Contains(out.String(), "make lint") {
		t.Fatalf("filtered history should contain 'make lint':\n%s", out.String())
	}
	if strings.Contains(out.String(), "go test") {
		t.Fatalf("filtered history should NOT contain 'go test':\n%s", out.String())
	}
}

func TestHistoryCmdFilterByCommand(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st := openHistoryStore(t)
	defer func() { _ = st.Close() }()
	seedHistoryRuns(t, st)

	a := &app{st: st}
	cmd := a.historyCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("command", "npm"); err != nil {
		t.Fatalf("set command flag: %v", err)
	}

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("historyCmd command filter failed: %v", err)
	}
	if !strings.Contains(out.String(), "npm run dev") {
		t.Fatalf("should contain 'npm run dev':\n%s", out.String())
	}
	if strings.Contains(out.String(), "make lint") {
		t.Fatalf("should NOT contain 'make lint':\n%s", out.String())
	}
}

func TestHistoryCmdShowsLabelSuffix(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st := openHistoryStore(t)
	defer func() { _ = st.Close() }()
	seedHistoryRuns(t, st)

	// Set a label on the first run.
	runs, err := st.ListHistory(10, store.HistoryFilter{})
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("expected runs")
	}
	if err := st.SetLabel(runs[0].ID, "my-label"); err != nil {
		t.Fatalf("set label: %v", err)
	}

	a := &app{st: st}
	cmd := a.historyCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("historyCmd failed: %v", err)
	}
	if !strings.Contains(out.String(), "my-label") {
		t.Fatalf("history should show label suffix:\n%s", out.String())
	}
}

func TestHistoryCmdShortCollapsesSession(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st := openHistoryStore(t)
	defer func() { _ = st.Close() }()

	day := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	// Create a session with two runs.
	sessID, err := st.CreateSession(store.CreateSession{StartedAt: day})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	for i, cmd := range []string{"echo one", "echo two"} {
		argv := fmt.Sprintf(`[%q]`, cmd)
		id, err := st.CreateRun(store.CreateRun{
			Status:    store.StatusRunning,
			Mode:      store.ModeRun,
			Command:   cmd,
			ArgvJSON:  argv,
			CWD:       "/tmp",
			StartedAt: day.Add(time.Duration(i) * time.Minute),
			SessionID: &sessID,
		})
		if err != nil {
			t.Fatalf("create run: %v", err)
		}
		if err := st.FinishRun(id, store.StatusSuccess, 0, day.Add(time.Duration(i)*time.Minute+time.Second)); err != nil {
			t.Fatalf("finish run: %v", err)
		}
	}

	a := &app{st: st}
	cmd := a.historyCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("short", "true"); err != nil {
		t.Fatalf("set short flag: %v", err)
	}

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("historyCmd --short failed: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "... (2 runs)") {
		t.Fatalf("short mode should show '... (2 runs)', got:\n%s", got)
	}
	if strings.Contains(got, "echo one") || strings.Contains(got, "echo two") {
		t.Fatalf("short mode should NOT show individual run commands, got:\n%s", got)
	}
}

func TestHistoryCmdSinceInvalidDuration(t *testing.T) {
	st := openHistoryStore(t)
	defer func() { _ = st.Close() }()

	a := &app{st: st}
	cmd := a.historyCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("since", "notaduration"); err != nil {
		t.Fatalf("set since flag: %v", err)
	}

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error for invalid --since")
	}
	if !strings.Contains(err.Error(), "invalid --since") {
		t.Fatalf("unexpected error: %v", err)
	}
}
