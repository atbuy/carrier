package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/store"
)

func TestParseAge(t *testing.T) {
	tests := map[string]time.Duration{
		"30d": 30 * 24 * time.Hour,
		"2h":  2 * time.Hour,
		"15m": 15 * time.Minute,
	}

	for input, want := range tests {
		got, err := parseAge(input)
		if err != nil {
			t.Fatalf("parseAge(%q) failed: %v", input, err)
		}
		if got != want {
			t.Fatalf("parseAge(%q) = %s, want %s", input, got, want)
		}
	}
}

func TestParseAgeRejectsInvalidInput(t *testing.T) {
	if _, err := parseAge("bad"); err == nil {
		t.Fatalf("parseAge accepted invalid input")
	}
}

func TestCleanRequiresAtLeastOneFlag(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	app := &app{st: st}
	cmd := app.cleanCmd()
	err = cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatalf("clean without any filter flag succeeded")
	}
	if !strings.Contains(err.Error(), "--older-than or --keep-last") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCleanKeepLastDryRun(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	started := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 5 {
		id, err := st.CreateRun(store.CreateRun{
			Status: store.StatusRunning, Mode: store.ModeRun,
			Command: fmt.Sprintf("cmd%d", i), ArgvJSON: `["cmd"]`,
			CWD: "/tmp", StartedAt: started.Add(time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatalf("create run: %v", err)
		}
		if err := st.FinishRun(id, store.StatusSuccess, 0, started.Add(time.Duration(i)*time.Minute+time.Second)); err != nil {
			t.Fatalf("finish run: %v", err)
		}
	}

	app := &app{st: st}
	cmd := app.cleanCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("keep-last", "3"); err != nil {
		t.Fatalf("set keep-last: %v", err)
	}
	if err := cmd.Flags().Set("dry-run", "true"); err != nil {
		t.Fatalf("set dry-run: %v", err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("dry-run keep-last failed: %v", err)
	}
	if !strings.Contains(out.String(), "would delete") {
		t.Fatalf("missing 'would delete' in output: %s", out.String())
	}
}

func TestCleanWithYesDeletesOldRuns(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	old := time.Now().Add(-48 * time.Hour)
	for i := 0; i < 2; i++ {
		id, err := st.CreateRun(store.CreateRun{
			Status: store.StatusRunning, Mode: store.ModeRun,
			Command: fmt.Sprintf("old%d", i), ArgvJSON: `["old"]`,
			CWD: "/tmp", StartedAt: old,
		})
		if err != nil {
			t.Fatalf("create old run: %v", err)
		}
		if err := st.FinishRun(id, store.StatusSuccess, 0, old.Add(time.Second)); err != nil {
			t.Fatalf("finish run: %v", err)
		}
	}

	app := &app{st: st}
	cmd := app.cleanCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("older-than", "24h"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatalf("set yes: %v", err)
	}

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("clean --yes failed: %v", err)
	}
	if !strings.Contains(out.String(), "deleted") {
		t.Fatalf("expected 'deleted' in output: %s", out.String())
	}
}

func TestPreviewCleanBothCriteria(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	// Create 5 recent runs (within 1h) — none qualify for 48h olderThan.
	started := time.Now().Add(-30 * time.Minute)
	for i := 0; i < 5; i++ {
		ts := started.Add(time.Duration(i) * time.Minute)
		id, err := st.CreateRun(store.CreateRun{
			Status: store.StatusRunning, Mode: store.ModeRun,
			Command: fmt.Sprintf("cmd%d", i), ArgvJSON: `["cmd"]`,
			CWD: "/tmp", StartedAt: ts,
		})
		if err != nil {
			t.Fatalf("create run: %v", err)
		}
		if err := st.FinishRun(id, store.StatusSuccess, 0, ts.Add(time.Second)); err != nil {
			t.Fatalf("finish run: %v", err)
		}
	}

	a := &app{st: st}
	// olderThan returns empty (all recent); keepLast returns 2 oldest (outside last 3).
	// Both criteria combined — the keepLast results go through !seen[r.ID] path.
	runs, err := a.previewClean("48h", 3)
	if err != nil {
		t.Fatalf("previewClean: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("expected some runs to preview-delete")
	}
	// Verify no duplicates (dedup worked).
	seen := map[int64]bool{}
	for _, r := range runs {
		if seen[r.ID] {
			t.Fatalf("duplicate run ID %d", r.ID)
		}
		seen[r.ID] = true
	}
}

func TestExecCleanBothCriteria(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	started := time.Now().Add(-30 * time.Minute)
	for i := 0; i < 5; i++ {
		ts := started.Add(time.Duration(i) * time.Minute)
		id, err := st.CreateRun(store.CreateRun{
			Status: store.StatusRunning, Mode: store.ModeRun,
			Command: fmt.Sprintf("cmd%d", i), ArgvJSON: `["cmd"]`,
			CWD: "/tmp", StartedAt: ts,
		})
		if err != nil {
			t.Fatalf("create run: %v", err)
		}
		if err := st.FinishRun(id, store.StatusSuccess, 0, ts.Add(time.Second)); err != nil {
			t.Fatalf("finish run: %v", err)
		}
	}

	a := &app{st: st}
	runs, err := a.execClean("48h", 3)
	if err != nil {
		t.Fatalf("execClean: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("expected some runs to be deleted")
	}
}

func TestCleanRequiresYesUnlessDryRun(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	app := &app{st: st}
	cmd := app.cleanCmd()
	if err := cmd.Flags().Set("older-than", "30d"); err != nil {
		t.Fatalf("set older-than: %v", err)
	}
	err = cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatalf("clean without --yes succeeded")
	}
	if !strings.Contains(err.Error(), "without --yes") {
		t.Fatalf("unexpected error: %v", err)
	}

	dryRun := app.cleanCmd()
	if err := dryRun.Flags().Set("older-than", "30d"); err != nil {
		t.Fatalf("set older-than: %v", err)
	}
	if err := dryRun.Flags().Set("dry-run", "true"); err != nil {
		t.Fatalf("set dry-run: %v", err)
	}
	if err := dryRun.RunE(dryRun, nil); err != nil {
		t.Fatalf("dry-run clean failed: %v", err)
	}
}
