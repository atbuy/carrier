package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/atbuy/carrier/internal/config"
)

// withUI sets the package UI config for a test and restores it afterward.
func withUI(t *testing.T, ui config.UIConfig) {
	t.Helper()
	prev := activeUI
	t.Cleanup(func() { activeUI = prev })
	activeUI = ui
}

func hasANSI(s string) bool { return strings.Contains(s, "\x1b[") }

// clearColorEnv neutralizes ambient color env vars so color-expecting tests are
// hermetic regardless of the surrounding shell.
func clearColorEnv(t *testing.T) {
	t.Helper()
	t.Setenv("NO_COLOR", "")
	t.Setenv("CARRIER_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
}

func TestThemeColorAlwaysForcesColorOnNonTTY(t *testing.T) {
	clearColorEnv(t)
	withUI(t, config.Default().UI)
	activeUI.Color = config.ColorAlways

	var buf bytes.Buffer
	out := newTheme(&buf).Danger.Render("boom")
	if !hasANSI(out) {
		t.Fatalf("expected ANSI color with color=always, got %q", out)
	}
}

func TestThemeColorNeverDisablesColor(t *testing.T) {
	withUI(t, config.Default().UI)
	activeUI.Color = config.ColorNever

	var buf bytes.Buffer
	out := newTheme(&buf).Danger.Render("boom")
	if hasANSI(out) {
		t.Fatalf("expected plain text with color=never, got %q", out)
	}
	if out != "boom" {
		t.Fatalf("output = %q, want %q", out, "boom")
	}
}

func TestThemeAutoHonorsNoColorEnv(t *testing.T) {
	withUI(t, config.Default().UI)
	activeUI.Color = config.ColorAuto
	t.Setenv("NO_COLOR", "1")

	var buf bytes.Buffer
	out := newTheme(&buf).Danger.Render("boom")
	if hasANSI(out) {
		t.Fatalf("expected plain text with NO_COLOR set in auto mode, got %q", out)
	}
}

func TestThemeNoColorOverridesConfigAlways(t *testing.T) {
	// NO_COLOR is absolute: it wins even over an explicit color=always config,
	// matching the run status line and no-color.org.
	withUI(t, config.Default().UI)
	activeUI.Color = config.ColorAlways
	t.Setenv("NO_COLOR", "1")

	var buf bytes.Buffer
	out := newTheme(&buf).Danger.Render("boom")
	if hasANSI(out) {
		t.Fatalf("NO_COLOR should override color=always, got %q", out)
	}
}

func TestThemeTermDumbDisablesColor(t *testing.T) {
	withUI(t, config.Default().UI)
	activeUI.Color = config.ColorAlways
	t.Setenv("TERM", "dumb")

	var buf bytes.Buffer
	out := newTheme(&buf).Danger.Render("boom")
	if hasANSI(out) {
		t.Fatalf("TERM=dumb should disable color, got %q", out)
	}
}

func TestThemeCarrierColorAlwaysForcesColor(t *testing.T) {
	clearColorEnv(t)
	withUI(t, config.Default().UI)
	activeUI.Color = config.ColorNever // env override beats config
	t.Setenv("CARRIER_COLOR", "always")

	var buf bytes.Buffer
	out := newTheme(&buf).Danger.Render("boom")
	if !hasANSI(out) {
		t.Fatalf("CARRIER_COLOR=always should force color, got %q", out)
	}
}

func TestThemeCarrierColorNeverDisablesColor(t *testing.T) {
	withUI(t, config.Default().UI)
	activeUI.Color = config.ColorAlways
	t.Setenv("CARRIER_COLOR", "never")

	var buf bytes.Buffer
	out := newTheme(&buf).Danger.Render("boom")
	if hasANSI(out) {
		t.Fatalf("CARRIER_COLOR=never should disable color, got %q", out)
	}
}

func TestThemeCustomColorApplied(t *testing.T) {
	clearColorEnv(t)
	withUI(t, config.Default().UI)
	activeUI.Color = config.ColorAlways
	activeUI.Theme.Danger = "#FF0000"

	var buf bytes.Buffer
	out := newTheme(&buf).Danger.Render("boom")
	// TrueColor profile renders #FF0000 as 38;2;255;0;0.
	if !strings.Contains(out, "255;0;0") {
		t.Fatalf("expected custom red in output, got %q", out)
	}
}

func TestConfigureThemeUpdatesActiveUI(t *testing.T) {
	prev := activeUI
	t.Cleanup(func() { activeUI = prev })

	cfg := config.Default()
	cfg.UI.Color = config.ColorNever
	configureTheme(cfg)
	if activeUI.Color != config.ColorNever {
		t.Fatalf("configureTheme did not update activeUI: %q", activeUI.Color)
	}
}

func TestResolveColorModePrecedenceAndAliases(t *testing.T) {
	withUI(t, config.Default().UI)

	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("CARRIER_COLOR", "always")
	activeUI.Color = config.ColorAlways
	if got := resolveColorMode(); got != config.ColorNever {
		t.Fatalf("NO_COLOR resolveColorMode = %q, want never", got)
	}

	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "dumb")
	if got := resolveColorMode(); got != config.ColorNever {
		t.Fatalf("TERM=dumb resolveColorMode = %q, want never", got)
	}

	t.Setenv("TERM", "xterm-256color")
	for _, value := range []string{"always", "1", "true"} {
		t.Setenv("CARRIER_COLOR", value)
		activeUI.Color = config.ColorNever
		if got := resolveColorMode(); got != config.ColorAlways {
			t.Fatalf("CARRIER_COLOR=%s resolveColorMode = %q, want always", value, got)
		}
	}
	for _, value := range []string{"never", "0", "false"} {
		t.Setenv("CARRIER_COLOR", value)
		activeUI.Color = config.ColorAlways
		if got := resolveColorMode(); got != config.ColorNever {
			t.Fatalf("CARRIER_COLOR=%s resolveColorMode = %q, want never", value, got)
		}
	}

	t.Setenv("CARRIER_COLOR", "bogus")
	activeUI.Color = "bogus"
	if got := resolveColorMode(); got != config.ColorAuto {
		t.Fatalf("fallback resolveColorMode = %q, want auto", got)
	}
}
