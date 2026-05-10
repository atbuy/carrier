package cli

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/store"
)

func TestRunToCSVRow(t *testing.T) {
	finishedAt := time.Date(2026, 5, 10, 12, 0, 5, 0, time.UTC)
	duration := int64(5000)
	exitCode := 0
	r := &store.Run{
		ID:         42,
		Status:     store.StatusSuccess,
		Mode:       store.ModeRun,
		Command:    "go test ./...",
		ArgvJSON:   `["go","test","./..."]`,
		CWD:        "/home/user/project",
		StartedAt:  time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
		FinishedAt: &finishedAt,
		DurationMS: &duration,
		ExitCode:   &exitCode,
		Hostname:   "myhost",
		GitBranch:  "main",
	}

	row := runToCSVRow(r)
	if len(row) != len(csvHeader) {
		t.Fatalf("row length = %d, want %d (len of csvHeader)", len(row), len(csvHeader))
	}

	checks := map[string]string{
		"id":          "42",
		"status":      "success",
		"mode":        "run",
		"command":     "go test ./...",
		"cwd":         "/home/user/project",
		"started_at":  "2026-05-10T12:00:00Z",
		"finished_at": "2026-05-10T12:00:05Z",
		"duration_ms": "5000",
		"exit_code":   "0",
		"hostname":    "myhost",
		"git_branch":  "main",
	}
	for i, col := range csvHeader {
		want, ok := checks[col]
		if !ok {
			continue
		}
		if row[i] != want {
			t.Errorf("column %q = %q, want %q", col, row[i], want)
		}
	}
}

func TestRunToCSVRow_NullableFields(t *testing.T) {
	r := &store.Run{
		ID:        7,
		Status:    store.StatusRunning,
		Mode:      store.ModeRun,
		Command:   "npm run dev",
		ArgvJSON:  `["npm","run","dev"]`,
		CWD:       "/tmp",
		StartedAt: time.Date(2026, 5, 10, 8, 0, 0, 0, time.UTC),
	}

	row := runToCSVRow(r)
	if len(row) != len(csvHeader) {
		t.Fatalf("row length = %d, want %d", len(row), len(csvHeader))
	}

	// nullable fields should be empty string when nil
	colIndex := func(name string) int {
		for i, h := range csvHeader {
			if h == name {
				return i
			}
		}
		t.Fatalf("column %q not found in csvHeader", name)
		return -1
	}

	if row[colIndex("finished_at")] != "" {
		t.Errorf("finished_at should be empty, got %q", row[colIndex("finished_at")])
	}
	if row[colIndex("duration_ms")] != "" {
		t.Errorf("duration_ms should be empty, got %q", row[colIndex("duration_ms")])
	}
	if row[colIndex("exit_code")] != "" {
		t.Errorf("exit_code should be empty, got %q", row[colIndex("exit_code")])
	}
}

func TestExportCSVCommandAllRuns(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	started := time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC)
	id, err := st.CreateRun(store.CreateRun{
		Status:    store.StatusRunning,
		Mode:      store.ModeRun,
		Command:   "make build",
		ArgvJSON:  `["make","build"]`,
		CWD:       "/tmp/project",
		StartedAt: started,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := st.FinishRun(id, store.StatusSuccess, 0, started.Add(2*time.Second)); err != nil {
		t.Fatalf("finish run: %v", err)
	}

	a := &app{st: st}
	cmd := a.exportCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("format", "csv"); err != nil {
		t.Fatalf("set format flag: %v", err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("export csv failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "id,status,mode") {
		t.Fatalf("CSV missing header: %s", output)
	}
	if !strings.Contains(output, "make build") {
		t.Fatalf("CSV missing command: %s", output)
	}
}

func TestExportCSVCommandSingleRun(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	started := time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC)
	id, err := st.CreateRun(store.CreateRun{
		Status:    store.StatusRunning,
		Mode:      store.ModeRun,
		Command:   "go test",
		ArgvJSON:  `["go","test"]`,
		CWD:       "/tmp/project",
		StartedAt: started,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := st.FinishRun(id, store.StatusSuccess, 0, started.Add(time.Second)); err != nil {
		t.Fatalf("finish run: %v", err)
	}

	a := &app{st: st}
	cmd := a.exportCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("format", "csv"); err != nil {
		t.Fatalf("set format flag: %v", err)
	}
	if err := cmd.RunE(cmd, []string{strconv.FormatInt(id, 10)}); err != nil {
		t.Fatalf("export csv single failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "id,status,mode") || !strings.Contains(output, "go test") {
		t.Fatalf("CSV output mismatch: %s", output)
	}
}

func TestExportCmdJSONSingleRun(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	id := createFinishedRun(t, st, store.StatusSuccess, "make test", `["make","test"]`, "/tmp/project")
	a := &app{st: st}
	cmd := a.exportCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("format", "json"); err != nil {
		t.Fatalf("set format flag: %v", err)
	}
	if err := cmd.RunE(cmd, []string{strconv.FormatInt(id, 10)}); err != nil {
		t.Fatalf("export json failed: %v", err)
	}
	if !strings.Contains(out.String(), `"command": "make test"`) {
		t.Fatalf("JSON output missing command: %s", out.String())
	}
}

func TestExportCmdRequiresIDForSingleRunFormats(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	a := &app{st: st}
	cmd := a.exportCmd()
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("expected missing id error")
	}
}

func TestExportCSVCommandBadID(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	a := &app{st: st}
	cmd := a.exportCmd()
	if err := cmd.Flags().Set("format", "csv"); err != nil {
		t.Fatalf("set format flag: %v", err)
	}
	if err := cmd.RunE(cmd, []string{"not-an-id"}); err == nil {
		t.Fatal("expected bad id error")
	}
}

func TestExportCSVCommandMissingRun(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	a := &app{st: st}
	cmd := a.exportCmd()
	if err := cmd.Flags().Set("format", "csv"); err != nil {
		t.Fatalf("set format flag: %v", err)
	}
	if err := cmd.RunE(cmd, []string{"999"}); err == nil {
		t.Fatal("expected missing run error")
	}
}
