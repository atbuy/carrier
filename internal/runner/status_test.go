package runner

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRunStartedLinePlain(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "")
	t.Setenv("CARRIER_COLOR", "never")

	if got := runStartedLine(&bytes.Buffer{}, 42); got != "carrier: run 42\n" {
		t.Fatalf("runStartedLine = %q", got)
	}
}

func TestRunStartedLineColorForced(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "")
	t.Setenv("CARRIER_COLOR", "always")

	got := runStartedLine(&bytes.Buffer{}, 42)
	for _, want := range []string{
		statusColorGray + "carrier:" + statusColorReset,
		statusColorGreen + "run" + statusColorReset,
		statusColorCyan + "42" + statusColorReset,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("colored line missing %q: %q", want, got)
		}
	}
}

func TestRunnerShouldColorDisabled(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CARRIER_COLOR", "always")
	if runnerShouldColor(&bytes.Buffer{}) {
		t.Fatal("NO_COLOR should disable color")
	}

	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "dumb")
	if runnerShouldColor(&bytes.Buffer{}) {
		t.Fatal("TERM=dumb should disable color")
	}

	t.Setenv("TERM", "")
	t.Setenv("CARRIER_COLOR", "")
	f, err := os.CreateTemp(t.TempDir(), "not-a-terminal")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer func() { _ = f.Close() }()
	if runnerShouldColor(f) {
		t.Fatal("regular file should not use color")
	}
}
