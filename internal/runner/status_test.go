package runner

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/atbuy/carrier/internal/config"
)

func TestRunStartedLinePlain(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "")
	t.Setenv("CARRIER_COLOR", "never")

	if got := runStartedLine(&bytes.Buffer{}, 42, config.Default().UI.Theme); got != "carrier: run 42\n" {
		t.Fatalf("runStartedLine = %q", got)
	}
}

func TestRunStartedLineColorForced(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "")
	t.Setenv("CARRIER_COLOR", "always")

	got := runStartedLine(&bytes.Buffer{}, 42, config.Default().UI.Theme)
	for _, want := range []string{
		"\x1b[38;2;168;168;168mcarrier:" + statusColorReset,
		"\x1b[38;2;106;175;106mrun" + statusColorReset,
		"\x1b[38;2;91;141;239m42" + statusColorReset,
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

func TestRunnerANSIColorFormats(t *testing.T) {
	cases := map[string]string{
		"#01020A": "\x1b[38;2;1;2;10m",
		"110":     "\x1b[38;5;110m",
		"red":     "\x1b[31m",
		"bogus":   "",
		"":        "",
	}
	for color, want := range cases {
		if got := runnerANSIColor(color); got != want {
			t.Fatalf("runnerANSIColor(%q) = %q, want %q", color, got, want)
		}
	}
}
