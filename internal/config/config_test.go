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

func TestExpandHomePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got, want := Expand("~/data"), filepath.Join(home, "data"); got != want {
		t.Fatalf("Expand = %q, want %q", got, want)
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

func hasIssue(issues []Issue, field string) bool {
	for _, issue := range issues {
		if issue.Field == field {
			return true
		}
	}
	return false
}
