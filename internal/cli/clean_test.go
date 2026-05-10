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
