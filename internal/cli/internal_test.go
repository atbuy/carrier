package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/config"
	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
)

func TestShouldIgnoreCarrierInternalCommands(t *testing.T) {
	tests := []string{
		"",
		"_carrier_begin",
		"_carrier_end",
		"carrier internal begin --state /tmp/state --cmd ls",
		"/usr/local/bin/carrier internal end --state /tmp/state --exit 0",
		"vim file.go",
		"/usr/bin/vim file.go",
	}

	for _, input := range tests {
		if !shouldIgnore(input, []string{"vim"}) {
			t.Fatalf("shouldIgnore(%q) = false, want true", input)
		}
	}
}

func TestShouldIgnoreAllowsNormalCommands(t *testing.T) {
	for _, input := range []string{"go test ./...", "carrier status", "echo carrier internal"} {
		if shouldIgnore(input, []string{"vim"}) {
			t.Fatalf("shouldIgnore(%q) = true, want false", input)
		}
	}
}

func TestInternalCmdRegistersHiddenSubcommands(t *testing.T) {
	cmd := (&app{}).internalCmd()
	if !cmd.Hidden {
		t.Fatal("internal command should be hidden")
	}
	if got := len(cmd.Commands()); got != 2 {
		t.Fatalf("subcommand count = %d, want 2", got)
	}
	if cmd.Commands()[0].Use != "begin" || cmd.Commands()[1].Use != "end" {
		t.Fatalf("unexpected subcommands: %#v", cmd.Commands())
	}
}

func TestInternalBeginEndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	statePath := filepath.Join(dir, "state.json")
	a := &app{st: st, cfg: config.Default()}

	// Run begin.
	beginCmd := a.internalBeginCmd()
	if err := beginCmd.Flags().Set("state", statePath); err != nil {
		t.Fatalf("set state flag: %v", err)
	}
	if err := beginCmd.Flags().Set("cmd", "go test ./..."); err != nil {
		t.Fatalf("set cmd flag: %v", err)
	}
	if err := beginCmd.Flags().Set("cwd", dir); err != nil {
		t.Fatalf("set cwd flag: %v", err)
	}
	if err := beginCmd.RunE(beginCmd, nil); err != nil {
		t.Fatalf("internalBeginCmd: %v", err)
	}

	// Verify state was written.
	state := (&carriershell.StateFile{Path: statePath}).Read()
	if state.CurrentID == 0 {
		t.Fatal("expected non-zero CurrentID after begin")
	}
	if state.CurrentLog == "" {
		t.Fatal("expected non-empty CurrentLog after begin")
	}

	// Verify run is in the store.
	run, err := st.GetRun(state.CurrentID)
	if err != nil {
		t.Fatalf("GetRun after begin: %v", err)
	}
	if run.Status != store.StatusRunning {
		t.Fatalf("status after begin = %q", run.Status)
	}

	// Run end.
	endCmd := a.internalEndCmd()
	if err := endCmd.Flags().Set("state", statePath); err != nil {
		t.Fatalf("set state: %v", err)
	}
	if err := endCmd.Flags().Set("exit", "0"); err != nil {
		t.Fatalf("set exit: %v", err)
	}
	if err := endCmd.RunE(endCmd, nil); err != nil {
		t.Fatalf("internalEndCmd: %v", err)
	}

	// Verify run is finished.
	run, err = st.GetRun(state.CurrentID)
	if err != nil {
		t.Fatalf("GetRun after end: %v", err)
	}
	if run.Status != store.StatusSuccess {
		t.Fatalf("status after end = %q", run.Status)
	}
}

func TestInternalBeginIgnoresFilteredCommand(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	statePath := filepath.Join(dir, "state.json")
	cfg := config.Default()
	cfg.Shell.IgnoreCommands = []string{"ls"}
	a := &app{st: st, cfg: cfg}

	beginCmd := a.internalBeginCmd()
	if err := beginCmd.Flags().Set("state", statePath); err != nil {
		t.Fatalf("set state flag: %v", err)
	}
	if err := beginCmd.Flags().Set("cmd", "ls -la"); err != nil {
		t.Fatalf("set cmd flag: %v", err)
	}
	if err := beginCmd.RunE(beginCmd, nil); err != nil {
		t.Fatalf("internalBeginCmd: %v", err)
	}

	// State should be empty since command was ignored.
	state := (&carriershell.StateFile{Path: statePath}).Read()
	if state.CurrentID != 0 {
		t.Fatalf("expected no run created for ignored command, got ID=%d", state.CurrentID)
	}
}

func TestInternalEndWithZeroStateIsNoop(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	statePath := filepath.Join(dir, "state.json")
	// Don't write anything to state — it has CurrentID=0.
	a := &app{st: st, cfg: config.Default()}

	endCmd := a.internalEndCmd()
	if err := endCmd.Flags().Set("state", statePath); err != nil {
		t.Fatalf("set state: %v", err)
	}
	// Should return nil without error.
	if err := endCmd.RunE(endCmd, nil); err != nil {
		t.Fatalf("end with zero state: %v", err)
	}
}

func TestInternalEndFailedRun(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	// Create a running run.
	id, err := st.CreateRun(store.CreateRun{
		Status:    store.StatusRunning,
		Mode:      store.ModeShell,
		Command:   "make build",
		ArgvJSON:  `["make","build"]`,
		CWD:       "/tmp",
		StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	statePath := filepath.Join(dir, "state.json")
	if err := (&carriershell.StateFile{Path: statePath}).Write(carriershell.State{CurrentID: id, CurrentLog: ""}); err != nil {
		t.Fatalf("write state: %v", err)
	}

	a := &app{st: st, cfg: config.Default()}
	endCmd := a.internalEndCmd()
	if err := endCmd.Flags().Set("state", statePath); err != nil {
		t.Fatalf("set state: %v", err)
	}
	if err := endCmd.Flags().Set("exit", "2"); err != nil {
		t.Fatalf("set exit: %v", err)
	}

	if err := endCmd.RunE(endCmd, nil); err != nil {
		t.Fatalf("end failed run: %v", err)
	}

	run, err := st.GetRun(id)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != store.StatusFailed {
		t.Fatalf("status = %q, want failed", run.Status)
	}
	if run.ExitCode == nil || *run.ExitCode != 2 {
		t.Fatalf("exit code = %v", run.ExitCode)
	}
}

func TestExecCleanAndRemoveIfSet(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	// Create several old runs.
	old := time.Now().Add(-48 * time.Hour)
	for i := 0; i < 3; i++ {
		id, err := st.CreateRun(store.CreateRun{
			Status:    store.StatusRunning,
			Mode:      store.ModeRun,
			Command:   fmt.Sprintf("cmd%d", i),
			ArgvJSON:  fmt.Sprintf(`["cmd%d"]`, i),
			CWD:       "/tmp",
			StartedAt: old,
		})
		if err != nil {
			t.Fatalf("create run: %v", err)
		}
		if err := st.FinishRun(id, store.StatusSuccess, 0, old.Add(time.Second)); err != nil {
			t.Fatalf("finish run: %v", err)
		}
	}

	a := &app{st: st, cfg: config.Default()}
	runs, err := a.execClean("24h", 0)
	if err != nil {
		t.Fatalf("execClean: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("execClean returned %d runs, want 3", len(runs))
	}
}

func TestExecCleanKeepLast(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	started := time.Now().Add(-time.Hour)
	for i := 0; i < 5; i++ {
		id, err := st.CreateRun(store.CreateRun{
			Status:    store.StatusRunning,
			Mode:      store.ModeRun,
			Command:   fmt.Sprintf("cmd%d", i),
			ArgvJSON:  fmt.Sprintf(`["cmd%d"]`, i),
			CWD:       "/tmp",
			StartedAt: started.Add(time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatalf("create run: %v", err)
		}
		if err := st.FinishRun(id, store.StatusSuccess, 0, started.Add(time.Duration(i)*time.Minute+time.Second)); err != nil {
			t.Fatalf("finish run: %v", err)
		}
	}

	a := &app{st: st, cfg: config.Default()}
	runs, err := a.execClean("", 3)
	if err != nil {
		t.Fatalf("execClean keepLast: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 deleted runs (5-3=2), got %d", len(runs))
	}
}

func TestRemoveIfSetDeletesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	removeIfSet(path)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file to be deleted, stat: %v", err)
	}
}

func TestRemoveIfSetNoopOnEmpty(t *testing.T) {
	removeIfSet("")
}
