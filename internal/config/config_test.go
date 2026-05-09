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
