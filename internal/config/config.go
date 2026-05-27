package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// AutoLabel defines a rule that automatically assigns a label to a run when its
// command, working directory, or git branch match the specified patterns. All
// non-empty fields must match for the rule to fire (AND semantics). The first
// matching rule wins.
//
// Patterns are Go regular expressions. Named capture groups ((?P<name>...)) are
// available in the label template as ${name}. Positional captures are available
// as ${1}, ${2}, etc.
//
// The dir pattern is implicitly anchored at the start of the path, so
// "/home/me/project" matches /home/me/project and all of its subdirectories.
type AutoLabel struct {
	// Label is the text to apply. May contain ${name} or ${N} placeholders
	// referencing capture groups from any matched field pattern.
	Label string `toml:"label"`

	// Cmd matches against the full display command string (e.g. "go test ./...").
	Cmd string `toml:"cmd"`

	// Dir matches against the working directory. Implicitly anchored at start.
	Dir string `toml:"dir"`

	// GitBranch matches against the current git branch name.
	GitBranch string `toml:"git_branch"`

	// compiled at config load time; not serialised to TOML.
	compiledCmd       *regexp.Regexp
	compiledDir       *regexp.Regexp
	compiledGitBranch *regexp.Regexp
}

type Config struct {
	Storage    StorageConfig   `toml:"storage"`
	Redaction  RedactionConfig `toml:"redaction"`
	Notify     NotifyConfig    `toml:"notify"`
	Shell      ShellConfig     `toml:"shell"`
	UI         UIConfig        `toml:"ui"`
	AutoLabels []AutoLabel     `toml:"auto_label"`
	// Includes lists additional TOML files to merge into this config. Paths
	// are resolved relative to the directory containing this config file.
	// Glob patterns (*, ?, [...]) are supported. Included files may not
	// themselves include further files (one level deep only).
	Includes []string `toml:"includes"`
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
		cfg.fillThemeDefaults()
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, err
	}
	configDir := filepath.Dir(path)
	for _, pattern := range cfg.Includes {
		pattern = Expand(pattern)
		if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(configDir, pattern)
		}
		isGlob := strings.ContainsAny(pattern, "*?[")
		matches, globErr := filepath.Glob(pattern)
		if globErr != nil {
			return cfg, fmt.Errorf("includes: invalid glob %q: %w", pattern, globErr)
		}
		if !isGlob && len(matches) == 0 {
			return cfg, fmt.Errorf("includes: file not found: %s", pattern)
		}
		sort.Strings(matches)
		for _, inc := range matches {
			if err := mergeInclude(&cfg, inc); err != nil {
				return cfg, fmt.Errorf("includes: %s: %w", inc, err)
			}
		}
	}
	cfg.Storage.DataDir = Expand(cfg.Storage.DataDir)
	cfg.fillThemeDefaults()
	if err := cfg.CompileAutoLabels(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// mergeInclude decodes the TOML file at path into an overlay Config and merges
// it into base. Arrays are always appended; scalar fields are only overridden
// when explicitly defined in the include file. The overlay's Includes field is
// ignored — recursive includes are not supported.
func mergeInclude(base *Config, path string) error {
	var overlay Config
	meta, err := toml.DecodeFile(path, &overlay)
	if err != nil {
		return err
	}

	// Arrays accumulate across all files.
	base.AutoLabels = append(base.AutoLabels, overlay.AutoLabels...)
	base.Redaction.Patterns = append(base.Redaction.Patterns, overlay.Redaction.Patterns...)
	base.Shell.IgnoreCommands = append(base.Shell.IgnoreCommands, overlay.Shell.IgnoreCommands...)

	// Scalars: only override when the include explicitly defines the key.
	if meta.IsDefined("storage", "data_dir") {
		base.Storage.DataDir = overlay.Storage.DataDir
	}
	if meta.IsDefined("storage", "max_output_mb") {
		base.Storage.MaxOutputMB = overlay.Storage.MaxOutputMB
	}
	if meta.IsDefined("storage", "stale_run_threshold") {
		base.Storage.StaleRunThreshold = overlay.Storage.StaleRunThreshold
	}
	if meta.IsDefined("storage", "capture_env") {
		base.Storage.CaptureEnv = overlay.Storage.CaptureEnv
	}
	if meta.IsDefined("redaction", "enabled") {
		base.Redaction.Enabled = overlay.Redaction.Enabled
	}
	if meta.IsDefined("notify", "min_duration") {
		base.Notify.MinDuration = overlay.Notify.MinDuration
	}
	if meta.IsDefined("notify", "success") {
		base.Notify.Success = overlay.Notify.Success
	}
	if meta.IsDefined("notify", "failure") {
		base.Notify.Failure = overlay.Notify.Failure
	}
	if meta.IsDefined("shell", "program") {
		base.Shell.Program = overlay.Shell.Program
	}
	if meta.IsDefined("ui", "color") {
		base.UI.Color = overlay.UI.Color
	}
	if meta.IsDefined("ui", "theme", "muted") {
		base.UI.Theme.Muted = overlay.UI.Theme.Muted
	}
	if meta.IsDefined("ui", "theme", "command") {
		base.UI.Theme.Command = overlay.UI.Theme.Command
	}
	if meta.IsDefined("ui", "theme", "success") {
		base.UI.Theme.Success = overlay.UI.Theme.Success
	}
	if meta.IsDefined("ui", "theme", "danger") {
		base.UI.Theme.Danger = overlay.UI.Theme.Danger
	}
	if meta.IsDefined("ui", "theme", "warning") {
		base.UI.Theme.Warning = overlay.UI.Theme.Warning
	}
	if meta.IsDefined("ui", "theme", "accent") {
		base.UI.Theme.Accent = overlay.UI.Theme.Accent
	}
	if meta.IsDefined("ui", "theme", "label") {
		base.UI.Theme.Label = overlay.UI.Theme.Label
	}
	return nil
}

func (c *Config) fillThemeDefaults() {
	defaults := Default().UI.Theme
	if c.UI.Theme.Muted == "" {
		c.UI.Theme.Muted = defaults.Muted
	}
	if c.UI.Theme.Command == "" {
		c.UI.Theme.Command = defaults.Command
	}
	if c.UI.Theme.Success == "" {
		c.UI.Theme.Success = defaults.Success
	}
	if c.UI.Theme.Danger == "" {
		c.UI.Theme.Danger = defaults.Danger
	}
	if c.UI.Theme.Warning == "" {
		c.UI.Theme.Warning = defaults.Warning
	}
	if c.UI.Theme.Accent == "" {
		c.UI.Theme.Accent = defaults.Accent
	}
	if c.UI.Theme.Label == "" {
		c.UI.Theme.Label = defaults.Label
	}
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

// labelVarRe matches ${name} and ${N} placeholders in label templates.
var labelVarRe = regexp.MustCompile(`\$\{([^}]+)\}`)

// CompileAutoLabels compiles the regex fields of every AutoLabel rule. It is
// called by Load after the config is decoded from TOML. An error is returned
// if any pattern is an invalid regular expression.
func (c *Config) CompileAutoLabels() error {
	for i := range c.AutoLabels {
		rule := &c.AutoLabels[i]
		var err error
		if rule.Cmd != "" {
			if rule.compiledCmd, err = regexp.Compile(rule.Cmd); err != nil {
				return fmt.Errorf("auto_label[%d].cmd: invalid regex: %w", i, err)
			}
		}
		if rule.Dir != "" {
			// Implicitly anchor at start so the dir pattern matches the path
			// and all of its subdirectories without requiring the user to write ^.
			if rule.compiledDir, err = regexp.Compile("^(?:" + rule.Dir + ")"); err != nil {
				return fmt.Errorf("auto_label[%d].dir: invalid regex: %w", i, err)
			}
		}
		if rule.GitBranch != "" {
			if rule.compiledGitBranch, err = regexp.Compile(rule.GitBranch); err != nil {
				return fmt.Errorf("auto_label[%d].git_branch: invalid regex: %w", i, err)
			}
		}
	}
	return nil
}

// ResolveLabel returns the label from the first AutoLabel rule whose non-empty
// fields all match the supplied command, working directory, and git branch.
// Returns "" when no rule matches or when AutoLabels is empty.
//
// Capture group values from matched fields are merged into a single map and
// used to expand ${name} and ${N} placeholders in the rule's label template.
func (c Config) ResolveLabel(cmd, dir, branch string) string {
	for i := range c.AutoLabels {
		rule := &c.AutoLabels[i]
		captures := map[string]string{}

		if rule.compiledCmd != nil {
			m := rule.compiledCmd.FindStringSubmatch(cmd)
			if m == nil {
				continue
			}
			mergeCaptures(captures, rule.compiledCmd, m)
		}
		if rule.compiledDir != nil {
			m := rule.compiledDir.FindStringSubmatch(dir)
			if m == nil {
				continue
			}
			mergeCaptures(captures, rule.compiledDir, m)
		}
		if rule.compiledGitBranch != nil {
			m := rule.compiledGitBranch.FindStringSubmatch(branch)
			if m == nil {
				continue
			}
			mergeCaptures(captures, rule.compiledGitBranch, m)
		}

		return expandLabelTemplate(rule.Label, captures)
	}
	return ""
}

// mergeCaptures adds all named and positional submatches from match into dst.
func mergeCaptures(dst map[string]string, re *regexp.Regexp, match []string) {
	names := re.SubexpNames()
	for i := 1; i < len(match); i++ {
		dst[strconv.Itoa(i)] = match[i]
		if names[i] != "" {
			dst[names[i]] = match[i]
		}
	}
}

// expandLabelTemplate replaces ${key} placeholders in label with values from captures.
func expandLabelTemplate(label string, captures map[string]string) string {
	return labelVarRe.ReplaceAllStringFunc(label, func(s string) string {
		key := s[2 : len(s)-1]
		if v, ok := captures[key]; ok {
			return v
		}
		return s
	})
}
