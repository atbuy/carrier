package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/store"
)

func TestStatsCommand(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st := openStatsStore(t)
	defer func() { _ = st.Close() }()
	seedStatsRuns(t, st)

	app := &app{st: st}
	cmd := app.statsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("stats command failed: %v", err)
	}
	for _, want := range []string{
		"Runs:        3",
		"Completed:   2",
		"Success:     1",
		"Failed:      1",
		"Running:     1",
		"Runs/day:    1.50",
		"Failure:     50.0%",
		"Avg duration: 1.75s",
		"Slowest:",
		"2  2.5s  failed  make lint  /tmp/project",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("stats output missing %q:\n%s", want, out.String())
		}
	}
}

func TestStatsCommandColorsHumanOutput(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "always")
	t.Setenv("NO_COLOR", "")
	st := openStatsStore(t)
	defer func() { _ = st.Close() }()
	seedStatsRuns(t, st)

	app := &app{st: st}
	cmd := app.statsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("stats command failed: %v", err)
	}
	for _, want := range []string{"Runs:", "Success:", "Failed:", "Slowest:"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("stats output missing %q:\n%s", want, out.String())
		}
	}
}

func TestStatsCommandJSON(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "always")
	st := openStatsStore(t)
	defer func() { _ = st.Close() }()
	seedStatsRuns(t, st)

	app := &app{st: st}
	cmd := app.statsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("json", "true"); err != nil {
		t.Fatalf("set json flag: %v", err)
	}
	if err := cmd.Flags().Set("slowest", "1"); err != nil {
		t.Fatalf("set slowest flag: %v", err)
	}

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("stats command failed: %v", err)
	}
	if strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("stats JSON should not contain color codes:\n%s", out.String())
	}
	var got statsView
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode stats json: %v\n%s", err, out.String())
	}
	if got.TotalRuns != 3 || got.CompletedRuns != 2 || got.RunsPerDay != 1.5 || got.FailureRate != 50 {
		t.Fatalf("stats JSON mismatch: %#v", got)
	}
	if got.AvgDurationMS == nil || *got.AvgDurationMS != 1750 {
		t.Fatalf("avg duration JSON mismatch: %#v", got.AvgDurationMS)
	}
	if len(got.SlowestRuns) != 1 || got.SlowestRuns[0].Command != "make lint" || got.SlowestRuns[0].DurationMS != 2500 {
		t.Fatalf("slowest JSON mismatch: %#v", got.SlowestRuns)
	}
}

func TestStatsCommandEmptyStore(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st := openStatsStore(t)
	defer func() { _ = st.Close() }()

	app := &app{st: st}
	cmd := app.statsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("stats command failed: %v", err)
	}
	for _, want := range []string{
		"Runs:        0",
		"Runs/day:    0.00",
		"Failure:     0.0%",
		"Slowest:\n  none",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("empty stats output missing %q:\n%s", want, out.String())
		}
	}
}

func openStatsStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return st
}

func seedStatsRuns(t *testing.T, st *store.Store) {
	t.Helper()
	day1 := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	createStatsCommandRun(t, st, store.StatusSuccess, "go test ./...", `["go","test","./..."]`, day1, 1000)
	createStatsCommandRun(t, st, store.StatusFailed, "make lint", `["make","lint"]`, day1.Add(time.Hour), 2500)
	if _, err := st.CreateRun(store.CreateRun{
		Status:    store.StatusRunning,
		Mode:      store.ModeRun,
		Command:   "npm run dev",
		ArgvJSON:  `["npm","run","dev"]`,
		CWD:       "/tmp/project",
		StartedAt: day2,
	}); err != nil {
		t.Fatalf("create running run: %v", err)
	}
}

func createStatsCommandRun(t *testing.T, st *store.Store, status, command, argv string, started time.Time, durationMS int64) {
	t.Helper()
	id, err := st.CreateRun(store.CreateRun{
		Status:    store.StatusRunning,
		Mode:      store.ModeRun,
		Command:   command,
		ArgvJSON:  argv,
		CWD:       "/tmp/project",
		StartedAt: started,
	})
	if err != nil {
		t.Fatalf("create stats run: %v", err)
	}
	exitCode := 0
	if status != store.StatusSuccess {
		exitCode = 1
	}
	if err := st.FinishRun(id, status, exitCode, started.Add(time.Duration(durationMS)*time.Millisecond)); err != nil {
		t.Fatalf("finish stats run: %v", err)
	}
}
