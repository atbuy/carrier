package cli

import (
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
