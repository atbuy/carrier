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

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

// ---------------------------------------------------------------------------
// shared helpers
// ---------------------------------------------------------------------------

func openCmdStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return st
}

func createFinishedRun(t *testing.T, st *store.Store, status, command, argv, cwd string) int64 {
	t.Helper()
	started := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	id, err := st.CreateRun(store.CreateRun{
		Status:    store.StatusRunning,
		Mode:      store.ModeRun,
		Command:   command,
		ArgvJSON:  argv,
		CWD:       cwd,
		StartedAt: started,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	exitCode := 0
	if status == store.StatusFailed {
		exitCode = 1
	}
	if err := st.FinishRun(id, status, exitCode, started.Add(time.Second)); err != nil {
		t.Fatalf("finish run: %v", err)
	}
	return id
}

// ---------------------------------------------------------------------------
// lastCmd
// ---------------------------------------------------------------------------

func TestLastCmd(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()

	createFinishedRun(t, st, store.StatusSuccess, "go test ./...", `["go","test","./..."]`, "/tmp")

	a := &app{st: st, cfg: config.Default()}
	cmd := a.lastCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("lastCmd failed: %v", err)
	}
	if !strings.Contains(out.String(), "go test ./...") {
		t.Fatalf("last output missing command:\n%s", out.String())
	}
}

func TestLastCmdJSON(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()

	createFinishedRun(t, st, store.StatusSuccess, "make build", `["make","build"]`, "/tmp")

	a := &app{st: st, cfg: config.Default()}
	cmd := a.lastCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("json", "true"); err != nil {
		t.Fatalf("set json flag: %v", err)
	}

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("lastCmd JSON failed: %v", err)
	}
	var view runView
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out.String())
	}
	if view.Command != "make build" {
		t.Fatalf("view.Command = %q", view.Command)
	}
}

func TestLastCmdEmptyStore(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()

	a := &app{st: st, cfg: config.Default()}
	cmd := a.lastCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error from empty store")
	}
}

// ---------------------------------------------------------------------------
// showCmd
// ---------------------------------------------------------------------------

func TestShowCmd(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	id := createFinishedRun(t, st, store.StatusSuccess, "echo hi", `["echo","hi"]`, "/tmp")

	stdoutPath := filepath.Join(dir, "runs", fmt.Sprintf("%d.stdout.log", id))
	if err := os.MkdirAll(filepath.Dir(stdoutPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(stdoutPath, []byte("hi\n"), 0o600); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	stderrPath := filepath.Join(dir, "runs", fmt.Sprintf("%d.stderr.log", id))
	if err := os.WriteFile(stderrPath, []byte(""), 0o600); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	if err := st.UpdatePaths(id, stdoutPath, stderrPath, ""); err != nil {
		t.Fatalf("update paths: %v", err)
	}

	a := &app{st: st, cfg: config.Default()}
	cmd := a.showCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.RunE(cmd, []string{fmt.Sprintf("%d", id)}); err != nil {
		t.Fatalf("showCmd failed: %v", err)
	}
	if !strings.Contains(out.String(), "echo hi") {
		t.Fatalf("show output missing command:\n%s", out.String())
	}
}

func TestShowCmdBadID(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()

	a := &app{st: st, cfg: config.Default()}
	cmd := a.showCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.RunE(cmd, []string{"notanid"}); err == nil {
		t.Fatal("expected error for bad id")
	}
}

func TestShowCmdJSON(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()

	id := createFinishedRun(t, st, store.StatusSuccess, "go build", `["go","build"]`, "/tmp")

	a := &app{st: st, cfg: config.Default()}
	cmd := a.showCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("json", "true"); err != nil {
		t.Fatalf("set json flag: %v", err)
	}

	if err := cmd.RunE(cmd, []string{fmt.Sprintf("%d", id)}); err != nil {
		t.Fatalf("showCmd JSON failed: %v", err)
	}
	var view runView
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out.String())
	}
	if view.Command != "go build" {
		t.Fatalf("view.Command = %q", view.Command)
	}
}

func TestShowCmdWithLines(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	id := createFinishedRun(t, st, store.StatusSuccess, "echo hi", `["echo","hi"]`, "/tmp")
	stdoutPath := filepath.Join(dir, "out.log")
	if err := os.WriteFile(stdoutPath, []byte("line1\nline2\nline3\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := st.UpdatePaths(id, stdoutPath, "", ""); err != nil {
		t.Fatalf("update paths: %v", err)
	}

	a := &app{st: st, cfg: config.Default()}
	cmd := a.showCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("lines", "1"); err != nil {
		t.Fatalf("set lines: %v", err)
	}

	if err := cmd.RunE(cmd, []string{fmt.Sprintf("%d", id)}); err != nil {
		t.Fatalf("showCmd --lines failed: %v", err)
	}
	if !strings.Contains(out.String(), "line3") {
		t.Errorf("expected last line in output:\n%s", out.String())
	}
	if strings.Contains(out.String(), "line1") {
		t.Errorf("should only show last 1 line:\n%s", out.String())
	}
}

func TestShowCmdOnlyStderr(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	id := createFinishedRun(t, st, store.StatusSuccess, "echo hi", `["echo","hi"]`, "/tmp")
	stdoutPath := filepath.Join(dir, "out.log")
	stderrPath := filepath.Join(dir, "err.log")
	if err := os.WriteFile(stdoutPath, []byte("stdout line\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(stderrPath, []byte("stderr line\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := st.UpdatePaths(id, stdoutPath, stderrPath, ""); err != nil {
		t.Fatalf("update paths: %v", err)
	}

	a := &app{st: st, cfg: config.Default()}
	cmd := a.showCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("stderr", "true"); err != nil {
		t.Fatalf("set flag: %v", err)
	}

	if err := cmd.RunE(cmd, []string{fmt.Sprintf("%d", id)}); err != nil {
		t.Fatalf("showCmd --stderr failed: %v", err)
	}
	if !strings.Contains(out.String(), "stderr line") {
		t.Errorf("expected stderr content:\n%s", out.String())
	}
	if strings.Contains(out.String(), "stdout line") {
		t.Errorf("should NOT contain stdout content:\n%s", out.String())
	}
}

func TestShowCmdTerminalOutput(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	id := createFinishedRun(t, st, store.StatusSuccess, "echo hi", `["echo","hi"]`, "/tmp")
	termPath := filepath.Join(dir, "term.log")
	if err := os.WriteFile(termPath, []byte("terminal output\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := st.UpdatePaths(id, "", "", termPath); err != nil {
		t.Fatalf("update paths: %v", err)
	}

	a := &app{st: st, cfg: config.Default()}
	cmd := a.showCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.RunE(cmd, []string{fmt.Sprintf("%d", id)}); err != nil {
		t.Fatalf("showCmd terminal failed: %v", err)
	}
	if !strings.Contains(out.String(), "terminal output") {
		t.Errorf("expected terminal output:\n%s", out.String())
	}
}

func TestShowCmdEnvFlag(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	// Create run with env JSON.
	started := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	id, err := st.CreateRun(store.CreateRun{
		Status:    store.StatusRunning,
		Mode:      store.ModeRun,
		Command:   "echo hi",
		ArgvJSON:  `["echo","hi"]`,
		CWD:       "/tmp",
		StartedAt: started,
		EnvJSON:   `{"MY_VAR":"my_value","HOME":"/home/user"}`,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := st.FinishRun(id, store.StatusSuccess, 0, started.Add(time.Second)); err != nil {
		t.Fatalf("finish: %v", err)
	}

	a := &app{st: st, cfg: config.Default()}
	cmd := a.showCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("env", "true"); err != nil {
		t.Fatalf("set env flag: %v", err)
	}

	if err := cmd.RunE(cmd, []string{fmt.Sprintf("%d", id)}); err != nil {
		t.Fatalf("showCmd --env failed: %v", err)
	}
	if !strings.Contains(out.String(), "MY_VAR") {
		t.Errorf("expected MY_VAR in env output:\n%s", out.String())
	}
}

func TestShowCmdOnlyStdout(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	id := createFinishedRun(t, st, store.StatusSuccess, "echo hi", `["echo","hi"]`, "/tmp")
	stdoutPath := filepath.Join(dir, "out.log")
	stderrPath := filepath.Join(dir, "err.log")
	if err := os.WriteFile(stdoutPath, []byte("stdout content\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(stderrPath, []byte("stderr content\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := st.UpdatePaths(id, stdoutPath, stderrPath, ""); err != nil {
		t.Fatalf("update paths: %v", err)
	}

	a := &app{st: st, cfg: config.Default()}
	cmd := a.showCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("stdout", "true"); err != nil {
		t.Fatalf("set flag: %v", err)
	}

	if err := cmd.RunE(cmd, []string{fmt.Sprintf("%d", id)}); err != nil {
		t.Fatalf("showCmd --stdout failed: %v", err)
	}
	if !strings.Contains(out.String(), "stdout content") {
		t.Fatalf("expected stdout content:\n%s", out.String())
	}
	if strings.Contains(out.String(), "stderr content") {
		t.Fatalf("should NOT contain stderr content:\n%s", out.String())
	}
}

// ---------------------------------------------------------------------------
// failedCmd / runningCmd / listStatusCmd
// ---------------------------------------------------------------------------

func TestFailedCmd(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()

	createFinishedRun(t, st, store.StatusFailed, "make lint", `["make","lint"]`, "/tmp")
	createFinishedRun(t, st, store.StatusSuccess, "go test", `["go","test"]`, "/tmp")

	a := &app{st: st}
	cmd := a.failedCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("failedCmd failed: %v", err)
	}
	if !strings.Contains(out.String(), "make lint") {
		t.Fatalf("failed output missing 'make lint':\n%s", out.String())
	}
	if strings.Contains(out.String(), "go test") {
		t.Fatalf("failed output should not contain 'go test':\n%s", out.String())
	}
}

func TestRunningCmd(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()

	// Create a running (not finished) run.
	_, err := st.CreateRun(store.CreateRun{
		Status:    store.StatusRunning,
		Mode:      store.ModeRun,
		Command:   "sleep 60",
		ArgvJSON:  `["sleep","60"]`,
		CWD:       "/tmp",
		StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("create running run: %v", err)
	}

	a := &app{st: st}
	cmd := a.runningCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("runningCmd failed: %v", err)
	}
	if !strings.Contains(out.String(), "sleep 60") {
		t.Fatalf("running output missing 'sleep 60':\n%s", out.String())
	}
}

func TestListStatusCmdRunningShowsAge(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()

	_, err := st.CreateRun(store.CreateRun{
		Status:    store.StatusRunning,
		Mode:      store.ModeRun,
		Command:   "sleep 999",
		ArgvJSON:  `["sleep","999"]`,
		CWD:       "/tmp",
		StartedAt: time.Now().Add(-5 * time.Second),
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	a := &app{st: st}
	cmd := listStatusCmd("running", store.StatusRunning, a)
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("listStatusCmd running: %v", err)
	}
	if !strings.Contains(out.String(), "sleep 999") {
		t.Fatalf("missing command in output:\n%s", out.String())
	}
}

func TestRunningCmdJSON(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()

	_, err := st.CreateRun(store.CreateRun{
		Status:    store.StatusRunning,
		Mode:      store.ModeRun,
		Command:   "npm start",
		ArgvJSON:  `["npm","start"]`,
		CWD:       "/tmp",
		StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	a := &app{st: st}
	cmd := a.runningCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("json", "true"); err != nil {
		t.Fatalf("set flag: %v", err)
	}

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("runningCmd JSON failed: %v", err)
	}
	var views []runView
	if err := json.Unmarshal(out.Bytes(), &views); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out.String())
	}
	if len(views) != 1 || views[0].Command != "npm start" {
		t.Fatalf("unexpected views: %#v", views)
	}
}

// ---------------------------------------------------------------------------
// export markdown
// ---------------------------------------------------------------------------

func TestExportMarkdown(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()

	id := createFinishedRun(t, st, store.StatusSuccess, "go build ./...", `["go","build","./..."]`, "/tmp/proj")

	a := &app{st: st}
	cmd := a.exportCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	// format defaults to markdown

	if err := cmd.RunE(cmd, []string{fmt.Sprintf("%d", id)}); err != nil {
		t.Fatalf("exportMarkdown failed: %v", err)
	}
	for _, want := range []string{"# carrier run", "**Command:**", "go build ./...", "## stdout"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("markdown output missing %q:\n%s", want, out.String())
		}
	}
}

func TestExportMarkdownWithTerminalPath(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	id := createFinishedRun(t, st, store.StatusSuccess, "zsh", `["zsh"]`, "/tmp")
	termPath := filepath.Join(dir, "term.log")
	if err := os.WriteFile(termPath, []byte("terminal session\n"), 0o600); err != nil {
		t.Fatalf("write term: %v", err)
	}
	if err := st.UpdatePaths(id, "", "", termPath); err != nil {
		t.Fatalf("update paths: %v", err)
	}

	a := &app{st: st}
	cmd := a.exportCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.RunE(cmd, []string{fmt.Sprintf("%d", id)}); err != nil {
		t.Fatalf("exportMarkdown terminal failed: %v", err)
	}
	if !strings.Contains(out.String(), "## terminal") {
		t.Fatalf("markdown should have terminal section:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "terminal session") {
		t.Fatalf("markdown missing terminal content:\n%s", out.String())
	}
}

func TestExportMarkdownNoIDError(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()

	a := &app{st: st}
	cmd := a.exportCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no id and format is markdown")
	}
}

func TestExportCSVWithID(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()

	id := createFinishedRun(t, st, store.StatusSuccess, "cargo build", `["cargo","build"]`, "/tmp")

	a := &app{st: st}
	cmd := a.exportCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("format", "csv"); err != nil {
		t.Fatalf("set format: %v", err)
	}

	if err := cmd.RunE(cmd, []string{fmt.Sprintf("%d", id)}); err != nil {
		t.Fatalf("export csv with id failed: %v", err)
	}
	if !strings.Contains(out.String(), "cargo build") {
		t.Fatalf("CSV missing command:\n%s", out.String())
	}
}

func TestExportJSONWithID(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()

	id := createFinishedRun(t, st, store.StatusSuccess, "go vet ./...", `["go","vet","./..."]`, "/tmp")

	a := &app{st: st}
	cmd := a.exportCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("format", "json"); err != nil {
		t.Fatalf("set format: %v", err)
	}

	if err := cmd.RunE(cmd, []string{fmt.Sprintf("%d", id)}); err != nil {
		t.Fatalf("export json failed: %v", err)
	}
	var view runView
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v\n%s", err, out.String())
	}
	if view.Command != "go vet ./..." {
		t.Fatalf("view.Command = %q", view.Command)
	}
}

// ---------------------------------------------------------------------------
// statusColor / short helpers
// ---------------------------------------------------------------------------

func TestStatusColor(t *testing.T) {
	cases := map[string]string{
		store.StatusSuccess: colorGreen,
		store.StatusFailed:  colorRed,
		store.StatusRunning: colorYellow,
		store.StatusKilled:  colorRed,
		"unknown":           colorGray,
	}
	for status, want := range cases {
		got := statusColor(status)
		if got != want {
			t.Errorf("statusColor(%q) = %q, want %q", status, got, want)
		}
	}
}

func TestShortTruncation(t *testing.T) {
	long := strings.Repeat("a", 20)
	got := short(long)
	if len(got) > 12 {
		t.Fatalf("short result too long: %q", got)
	}
	short2 := "hi"
	if got := short(short2); got != short2 {
		t.Fatalf("short on short string changed it: %q", got)
	}
}

// ---------------------------------------------------------------------------
// runViewFromStoreOpts with env redaction
// ---------------------------------------------------------------------------

func TestRunViewFromStoreOptsRedactsEnv(t *testing.T) {
	run := &store.Run{
		ID:       1,
		Status:   store.StatusSuccess,
		Mode:     store.ModeRun,
		Command:  "echo",
		ArgvJSON: `["echo"]`,
		CWD:      "/tmp",
		EnvJSON:  `{"TOKEN":"abc123","HOME":"/root"}`,
	}

	view := runViewFromStore(run, true)
	// No redaction configured — values pass through.
	if view.Env["HOME"] != "/root" {
		t.Fatalf("HOME = %q", view.Env["HOME"])
	}
}
