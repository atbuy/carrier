package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Storage   StorageConfig   `toml:"storage"`
	Redaction RedactionConfig `toml:"redaction"`
	Notify    NotifyConfig    `toml:"notify"`
	Shell     ShellConfig     `toml:"shell"`
	UI        UIConfig        `toml:"ui"`
}

type StorageConfig struct {
	DataDir           string `toml:"data_dir"`
	MaxOutputMB       int64  `toml:"max_output_mb"`
	StaleRunThreshold string `toml:"stale_run_threshold"`
	CaptureEnv        bool   `toml:"capture_env"`
}

type RedactionConfig struct {
	Enabled  bool     `toml:"enabled"`
	Patterns []string `toml:"patterns"`
}

type NotifyConfig struct {
	MinDuration string `toml:"min_duration"`
	Success     bool   `toml:"success"`
	Failure     bool   `toml:"failure"`
}

type ShellConfig struct {
	Program        string   `toml:"program"`
	IgnoreCommands []string `toml:"ignore_commands"`
}

// UIConfig controls terminal presentation: when to emit ANSI color and which
// colors to use for each semantic style.
type UIConfig struct {
	// Color selects when to colorize output: "auto" (color only on a TTY,
	// honoring NO_COLOR), "always" (force color), or "never" (plain text).
	Color string      `toml:"color"`
	Theme ThemeColors `toml:"theme"`
}

// ThemeColors holds hex color overrides for each semantic style. Empty fields
// fall back to the builtin defaults.
type ThemeColors struct {
	Muted   string `toml:"muted"`
	Command string `toml:"command"`
	Success string `toml:"success"`
	Danger  string `toml:"danger"`
	Warning string `toml:"warning"`
	Accent  string `toml:"accent"`
	Label   string `toml:"label"`
}

// Color mode values for UIConfig.Color.
const (
	ColorAuto   = "auto"
	ColorAlways = "always"
	ColorNever  = "never"
)

func Default() Config {
	return Config{
		Storage: StorageConfig{DataDir: "~/.local/share/carrier", MaxOutputMB: 20, StaleRunThreshold: "24h", CaptureEnv: true},
		Redaction: RedactionConfig{
			Enabled: true,
			Patterns: []string{
				`Bearer [A-Za-z0-9._-]+`,
				`(?i)(password|passwd|token|api[_-]?key|secret|access[_-]?token|refresh[_-]?token)\s*[:=]\s*\S+`,
				`AKIA[0-9A-Z]{16}`,
				`-----BEGIN PRIVATE KEY-----[\s\S]*?-----END PRIVATE KEY-----`,
			},
		},
		Notify: NotifyConfig{MinDuration: "10s", Success: true, Failure: true},
		Shell: ShellConfig{
			IgnoreCommands: []string{"nvim", "vim", "less", "man", "fzf", "yazi", "lazygit", "tmux"},
		},
		UI: UIConfig{
			Color: ColorAuto,
			Theme: ThemeColors{
				Muted:   "#A8A8A8",
				Command: "#6AAF6A",
				Success: "#6AAF6A",
				Danger:  "#D75F5F",
				Warning: "#D7AF5F",
				Accent:  "#5B8DEF",
				Label:   "#7AADF4",
			},
		},
	}
}

func Load() (Config, error) {
	cfg := Default()
	path, err := Path()
	if err != nil {
		return cfg, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		cfg.Storage.DataDir = Expand(cfg.Storage.DataDir)
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, err
	}
	cfg.Storage.DataDir = Expand(cfg.Storage.DataDir)
	return cfg, nil
}

func Path() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "carrier", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "carrier", "config.toml"), nil
}

func Expand(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

func (c Config) NotifyMinDuration() time.Duration {
	d, err := time.ParseDuration(c.Notify.MinDuration)
	if err != nil {
		return 10 * time.Second
	}
	return d
}

func (c Config) StaleRunThreshold() time.Duration {
	d, err := time.ParseDuration(c.Storage.StaleRunThreshold)
	if err != nil {
		return 24 * time.Hour
	}
	return d
}
