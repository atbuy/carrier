package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPathUsesXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")

	got, err := Path()
	if err != nil {
		t.Fatalf("Path failed: %v", err)
	}

	want := filepath.Join("/tmp/xdg", "carrier", "config.toml")
	if got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestPathUsesHomeWhenXDGConfigHomeUnset(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path failed: %v", err)
	}

	want := filepath.Join(home, ".config", "carrier", "config.toml")
	if got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestExpandHomePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got, want := Expand("~/data"), filepath.Join(home, "data"); got != want {
		t.Fatalf("Expand = %q, want %q", got, want)
	}
	if got := Expand("~"); got != home {
		t.Fatalf("Expand(~) = %q, want %q", got, home)
	}
	if got := Expand("/var/tmp"); got != "/var/tmp" {
		t.Fatalf("absolute path changed: %q", got)
	}
}

func TestLoadDefaultsWhenConfigMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Storage.DataDir != filepath.Join(home, ".local", "share", "carrier") {
		t.Fatalf("data dir = %q", cfg.Storage.DataDir)
	}
	if !cfg.Redaction.Enabled {
		t.Fatalf("redaction should default to enabled")
	}
	if len(cfg.Redaction.Patterns) < 4 {
		t.Fatalf("expected expanded default redaction patterns: %#v", cfg.Redaction.Patterns)
	}
	if cfg.NotifyMinDuration() != 10*time.Second {
		t.Fatalf("default notify duration mismatch: %s", cfg.NotifyMinDuration())
	}
}

func TestLoadConfigFile(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, ".config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	configDir := filepath.Join(xdg, "carrier")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	configPath := filepath.Join(configDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(`
[storage]
data_dir = "~/carrier-data"
max_output_mb = 42

[redaction]
enabled = false
patterns = ["secret"]

[notify]
min_duration = "3s"
success = false
failure = true

[shell]
program = "/bin/zsh"
ignore_commands = ["vim"]
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Storage.DataDir != filepath.Join(home, "carrier-data") {
		t.Fatalf("data dir = %q", cfg.Storage.DataDir)
	}
	if cfg.Storage.MaxOutputMB != 42 {
		t.Fatalf("max output = %d", cfg.Storage.MaxOutputMB)
	}
	if cfg.Redaction.Enabled {
		t.Fatalf("redaction should be disabled")
	}
	if cfg.NotifyMinDuration() != 3*time.Second {
		t.Fatalf("notify duration mismatch: %s", cfg.NotifyMinDuration())
	}
	if cfg.Shell.Program != "/bin/zsh" || len(cfg.Shell.IgnoreCommands) != 1 {
		t.Fatalf("shell config mismatch: %#v", cfg.Shell)
	}
}

func TestLoadInvalidTomlReturnsError(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, ".config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	configDir := filepath.Join(xdg, "carrier")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("[storage\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("expected invalid TOML error")
	}
}

func TestNotifyMinDurationFallback(t *testing.T) {
	cfg := Default()
	cfg.Notify.MinDuration = "not-a-duration"

	if got := cfg.NotifyMinDuration(); got != 10*time.Second {
		t.Fatalf("fallback duration = %s", got)
	}
}

func TestCheckDefaultConfigPasses(t *testing.T) {
	if issues := Check(Default()); len(issues) != 0 {
		t.Fatalf("default config issues: %#v", issues)
	}
}

func TestCheckReportsInvalidConfig(t *testing.T) {
	cfg := Default()
	cfg.Storage.DataDir = ""
	cfg.Storage.MaxOutputMB = -1
	cfg.Redaction.Patterns = []string{"[", ""}
	cfg.Notify.MinDuration = "soon"
	cfg.Shell.Program = " "
	cfg.Shell.IgnoreCommands = append(cfg.Shell.IgnoreCommands, "")

	issues := Check(cfg)
	errors, warnings := CountIssues(issues)
	if errors != 5 || warnings != 2 {
		t.Fatalf("issue counts = %d errors, %d warnings: %#v", errors, warnings, issues)
	}
	for _, want := range []string{
		"storage.data_dir",
		"storage.max_output_mb",
		"redaction.patterns[0]",
		"redaction.patterns[1]",
		"notify.min_duration",
		"shell.program",
		"shell.ignore_commands[8]",
	} {
		if !hasIssue(issues, want) {
			t.Fatalf("missing issue for %s: %#v", want, issues)
		}
	}
}

func TestCheckWarnsOnDisabledOutputCapAndEmptyRedactionPatterns(t *testing.T) {
	cfg := Default()
	cfg.Storage.MaxOutputMB = 0
	cfg.Redaction.Patterns = nil

	issues := Check(cfg)
	errors, warnings := CountIssues(issues)
	if errors != 0 || warnings != 2 {
		t.Fatalf("issue counts = %d errors, %d warnings: %#v", errors, warnings, issues)
	}
}

func TestCheckRejectsNegativeNotifyDuration(t *testing.T) {
	cfg := Default()
	cfg.Notify.MinDuration = "-1s"

	issues := Check(cfg)
	if !hasIssue(issues, "notify.min_duration") {
		t.Fatalf("missing notify.min_duration issue: %#v", issues)
	}
}

func TestCheckRejectsInvalidColorMode(t *testing.T) {
	cfg := Default()
	cfg.UI.Color = "rainbow"

	issues := Check(cfg)
	if !hasIssue(issues, "ui.color") {
		t.Fatalf("missing ui.color issue: %#v", issues)
	}
}

func TestCheckAllowsEmptyColorMode(t *testing.T) {
	cfg := Default()
	cfg.UI.Color = "" // treated as auto

	if hasIssue(Check(cfg), "ui.color") {
		t.Fatal("empty color mode should be allowed")
	}
}

func TestCheckRejectsInvalidThemeHex(t *testing.T) {
	cfg := Default()
	cfg.UI.Theme.Danger = "#ZZZ"
	cfg.UI.Theme.Accent = "#12345" // wrong length

	issues := Check(cfg)
	for _, want := range []string{"ui.theme.danger", "ui.theme.accent"} {
		if !hasIssue(issues, want) {
			t.Fatalf("missing %s issue: %#v", want, issues)
		}
	}
}

func TestCheckAllowsNonHexThemeColorNames(t *testing.T) {
	cfg := Default()
	cfg.UI.Theme.Danger = "red" // ANSI name, not hex — allowed
	cfg.UI.Theme.Accent = "9"   // ANSI index — allowed

	if hasIssue(Check(cfg), "ui.theme.danger") || hasIssue(Check(cfg), "ui.theme.accent") {
		t.Fatalf("non-hex color names should be allowed: %#v", Check(cfg))
	}
}

func TestDefaultUIColorIsAuto(t *testing.T) {
	if got := Default().UI.Color; got != ColorAuto {
		t.Fatalf("default UI color = %q, want %q", got, ColorAuto)
	}
}

func hasIssue(issues []Issue, field string) bool {
	for _, issue := range issues {
		if issue.Field == field {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// StaleRunThreshold
// ---------------------------------------------------------------------------

func TestStaleRunThresholdDefault(t *testing.T) {
	cfg := Default()
	if got := cfg.StaleRunThreshold(); got != 24*time.Hour {
		t.Fatalf("StaleRunThreshold = %s, want 24h", got)
	}
}

func TestStaleRunThresholdInvalidFallsBackTo24h(t *testing.T) {
	cfg := Default()
	cfg.Storage.StaleRunThreshold = "not-a-duration"
	if got := cfg.StaleRunThreshold(); got != 24*time.Hour {
		t.Fatalf("StaleRunThreshold fallback = %s, want 24h", got)
	}
}

func TestStaleRunThresholdCustom(t *testing.T) {
	cfg := Default()
	cfg.Storage.StaleRunThreshold = "1h"
	if got := cfg.StaleRunThreshold(); got != time.Hour {
		t.Fatalf("StaleRunThreshold = %s, want 1h", got)
	}
}

// ---------------------------------------------------------------------------
// Load — additional paths
// ---------------------------------------------------------------------------

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	// Point XDG_CONFIG_HOME at a temp dir that has no config.toml inside it.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with missing file should not error: %v", err)
	}
	def := Default()
	if cfg.Storage.MaxOutputMB != def.Storage.MaxOutputMB {
		t.Fatalf("MaxOutputMB = %d, want %d", cfg.Storage.MaxOutputMB, def.Storage.MaxOutputMB)
	}
	if cfg.Notify.MinDuration != def.Notify.MinDuration {
		t.Fatalf("MinDuration = %q, want %q", cfg.Notify.MinDuration, def.Notify.MinDuration)
	}
}

func TestLoadValidTomlFile(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, "xdg")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	configDir := filepath.Join(xdg, "carrier")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	const tomlContent = `
[storage]
data_dir = "~/mydata"
max_output_mb = 5
stale_run_threshold = "6h"

[notify]
min_duration = "30s"
success = true
failure = false
`
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(tomlContent), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Storage.MaxOutputMB != 5 {
		t.Fatalf("MaxOutputMB = %d, want 5", cfg.Storage.MaxOutputMB)
	}
	if cfg.StaleRunThreshold() != 6*time.Hour {
		t.Fatalf("StaleRunThreshold = %s, want 6h", cfg.StaleRunThreshold())
	}
	if cfg.NotifyMinDuration() != 30*time.Second {
		t.Fatalf("NotifyMinDuration = %s, want 30s", cfg.NotifyMinDuration())
	}
	if cfg.Notify.Failure {
		t.Fatalf("Notify.Failure should be false")
	}
	wantDir := filepath.Join(home, "mydata")
	if cfg.Storage.DataDir != wantDir {
		t.Fatalf("DataDir = %q, want %q", cfg.Storage.DataDir, wantDir)
	}
}

func TestLoadThemeEmptyFieldsFallBackToDefaults(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, ".config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	configDir := filepath.Join(xdg, "carrier")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(`
[ui]
color = "always"

[ui.theme]
label = ""
accent = "#112233"
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defaults := Default().UI.Theme
	if cfg.UI.Theme.Label != defaults.Label {
		t.Fatalf("label fallback = %q, want %q", cfg.UI.Theme.Label, defaults.Label)
	}
	if cfg.UI.Theme.Accent != "#112233" {
		t.Fatalf("accent = %q, want custom", cfg.UI.Theme.Accent)
	}
	if cfg.UI.Theme.Success != defaults.Success {
		t.Fatalf("success fallback = %q, want %q", cfg.UI.Theme.Success, defaults.Success)
	}
}
