package config

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Kind is the type of value a setting holds, which decides how it is edited.
type Kind int

const (
	KindBool Kind = iota
	KindChoice
	KindInt
	KindDuration
	KindText
	KindColor
	KindList
)

// Setting describes one config key that can be changed without editing the
// file by hand, for "carrier config". auto_label rules and includes are left
// to the file.
type Setting struct {
	// Section is the TOML table, such as "shell" or "ui.theme", and Key the
	// key in it.
	Section, Key string
	// Title names the setting in "carrier config".
	Title string
	Kind  Kind
	// Choices are the values a KindChoice setting accepts.
	Choices []string
	// Summary says what the setting does, in a sentence or two.
	Summary string
	// Value reads the setting from a loaded config: a bool, int64, string,
	// or []string.
	Value func(Config) any
}

// Name is the setting's full key, such as "shell.ask_outcome".
func (s Setting) Name() string { return s.Section + "." + s.Key }

// Group is the top-level table the setting belongs to, such as "ui" for
// "ui.theme.accent".
func (s Setting) Group() string {
	group, _, _ := strings.Cut(s.Section, ".")
	return group
}

// Settings lists every setting "carrier config" edits, in the order of the
// config file.
var Settings = []Setting{
	{
		Section: "storage", Key: "data_dir", Kind: KindText,
		Title:   "Data directory",
		Summary: "Directory for the database and log files. ~ is your home directory. Changing it does not move what is already recorded.",
		Value:   func(c Config) any { return c.Storage.DataDir },
	},
	{
		Section: "storage", Key: "max_output_mb", Kind: KindInt,
		Title:   "Output cap (MiB)",
		Summary: "Most output stored per log file, in MiB. 0 stores all of it. Terminal output is never cut.",
		Value:   func(c Config) any { return c.Storage.MaxOutputMB },
	},
	{
		Section: "storage", Key: "stale_run_threshold", Kind: KindDuration,
		Title:   "Stale run threshold",
		Summary: "Runs still running after this long, such as after a crash, are marked killed.",
		Value:   func(c Config) any { return c.Storage.StaleRunThreshold },
	},
	{
		Section: "storage", Key: "capture_env", Kind: KindBool,
		Title:   "Capture environment",
		Summary: "Store each run's environment, redacted, for \"carrier show --env\".",
		Value:   func(c Config) any { return c.Storage.CaptureEnv },
	},
	{
		Section: "redaction", Key: "enabled", Kind: KindBool,
		Title:   "Redact stored output",
		Summary: "Redact secrets from stored output. Terminal output is never redacted, and environments always are.",
		Value:   func(c Config) any { return c.Redaction.Enabled },
	},
	{
		Section: "redaction", Key: "patterns", Kind: KindList,
		Title:   "Redaction patterns",
		Summary: "Regular expressions redacted from stored output. Setting them replaces the defaults.",
		Value:   func(c Config) any { return c.Redaction.Patterns },
	},
	{
		Section: "notify", Key: "min_duration", Kind: KindDuration,
		Title:   "Minimum duration",
		Summary: "With -n, only commands that take at least this long send a notification.",
		Value:   func(c Config) any { return c.Notify.MinDuration },
	},
	{
		Section: "notify", Key: "success", Kind: KindBool,
		Title:   "Notify on success",
		Summary: "Notify when a command asked to notify succeeds.",
		Value:   func(c Config) any { return c.Notify.Success },
	},
	{
		Section: "notify", Key: "failure", Kind: KindBool,
		Title:   "Notify on failure",
		Summary: "Notify when a command asked to notify fails.",
		Value:   func(c Config) any { return c.Notify.Failure },
	},
	{
		Section: "shell", Key: "program", Kind: KindText,
		Title:   "Shell program",
		Summary: "Shell started by \"carrier shell\". Empty uses $SHELL.",
		Value:   func(c Config) any { return c.Shell.Program },
	},
	{
		Section: "shell", Key: "ignore_commands", Kind: KindList,
		Title:   "Commands without output",
		Summary: "Programs recorded without their output, such as editors and pagers.",
		Value:   func(c Config) any { return c.Shell.IgnoreCommands },
	},
	{
		Section: "shell", Key: "auto_session_retention", Kind: KindDuration,
		Title:   "Automatic session retention",
		Summary: "How long automatic sessions are kept after they end. 0 keeps them all. Labeled, noted, and kept sessions never expire.",
		Value:   func(c Config) any { return c.Shell.AutoSessionRetention },
	},
	{
		Section: "shell", Key: "ask_outcome", Kind: KindBool,
		Title:   "Ask for the outcome",
		Summary: "When a shell in a labeled session exits, ask what the session found.",
		Value:   func(c Config) any { return c.Shell.AskOutcome },
	},
	{
		Section: "shell", Key: "branch_sessions", Kind: KindChoice, Choices: []string{BranchSuggest, BranchFollow, BranchOff},
		Title:   "Branch sessions",
		Summary: "When a checkout moves a recorded shell to a branch linked to another session: suggest says so, follow switches to it, off does nothing.",
		Value:   func(c Config) any { return c.Shell.BranchSessions },
	},
	{
		Section: "hints", Key: "similar_failures", Kind: KindBool,
		Title:   "Similar failures",
		Summary: "When a run fails, point at an earlier failed run with the same error.",
		Value:   func(c Config) any { return c.Hints.SimilarFailures },
	},
	{
		Section: "hints", Key: "similar_min_age", Kind: KindDuration,
		Title:   "Similar failure minimum age",
		Summary: "Ignore similar failures newer than this, so a retry does not point at the attempt just before it.",
		Value:   func(c Config) any { return c.Hints.SimilarMinAge },
	},
	{
		Section: "ui", Key: "color", Kind: KindChoice, Choices: []string{ColorAuto, ColorAlways, ColorNever},
		Title:   "Color",
		Summary: "When to color output: auto colors terminals unless NO_COLOR is set, always also colors pipes, never is plain text.",
		Value:   func(c Config) any { return c.UI.Color },
	},
	themeSetting("muted", "Secondary text: times, directories, hints.", func(t ThemeColors) string { return t.Muted }),
	themeSetting("command", "Commands and examples.", func(t ThemeColors) string { return t.Command }),
	themeSetting("success", "Successful runs.", func(t ThemeColors) string { return t.Success }),
	themeSetting("danger", "Failed runs and errors.", func(t ThemeColors) string { return t.Danger }),
	themeSetting("warning", "Killed runs and warnings.", func(t ThemeColors) string { return t.Warning }),
	themeSetting("accent", "Session IDs and the selection.", func(t ThemeColors) string { return t.Accent }),
	themeSetting("label", "Labels, and flag and command names.", func(t ThemeColors) string { return t.Label }),
}

func themeSetting(key, use string, get func(ThemeColors) string) Setting {
	return Setting{
		Section: "ui.theme", Key: key, Kind: KindColor,
		Title:   strings.ToUpper(key[:1]) + key[1:] + " color",
		Summary: "Used for " + strings.ToLower(use[:1]) + use[1:] + " A #hex color, an ANSI color name such as red, or an ANSI index such as 9.",
		Value:   func(c Config) any { return get(c.UI.Theme) },
	}
}

// GroupTitle names a group of settings, as Setting.Group returns it.
func GroupTitle(group string) string {
	switch group {
	case "notify":
		return "Notifications"
	case "ui":
		return "Appearance"
	}
	return strings.ToUpper(group[:1]) + group[1:]
}

// LookupSetting returns the setting named like "shell.ask_outcome".
func LookupSetting(name string) (Setting, bool) {
	for _, s := range Settings {
		if s.Name() == name {
			return s, true
		}
	}
	return Setting{}, false
}

// Literal returns v, as Setting.Value returns it, written as a TOML value.
func Literal(v any) string {
	switch v := v.(type) {
	case bool:
		return strconv.FormatBool(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case string:
		return quote(v)
	case []string:
		items := make([]string, len(v))
		for i, item := range v {
			items[i] = quote(item)
		}
		return "[" + strings.Join(items, ", ") + "]"
	}
	panic(fmt.Sprintf("config: no TOML literal for %T", v))
}

// quote writes s as a TOML string, in double quotes like "carrier config
// init" writes, or in single quotes when it holds a backslash or a double
// quote, which keeps regular expressions readable.
func quote(s string) string {
	control := strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f })
	switch {
	case control:
	case !strings.ContainsAny(s, `"\`):
		return `"` + s + `"`
	case !strings.Contains(s, "'"):
		return "'" + s + "'"
	}
	var buf bytes.Buffer
	_ = toml.NewEncoder(&buf).Encode(map[string]string{"v": s})
	_, value, _ := strings.Cut(strings.TrimSpace(buf.String()), " = ")
	return value
}

// ParseValue reads the value of s from the command line, for "carrier config
// set": true or false (also on, off, yes, no) for a toggle, one of the
// choices, a whole number, or text. A list takes each argument as an item;
// anything else takes exactly one.
func ParseValue(s Setting, args []string) (any, error) {
	if s.Kind == KindList {
		if len(args) == 0 {
			return nil, fmt.Errorf("%s is a list; give one or more items", s.Name())
		}
		return args, nil
	}
	if len(args) != 1 {
		return nil, fmt.Errorf("%s takes one value; quote it if it has spaces", s.Name())
	}
	arg := args[0]
	switch s.Kind {
	case KindBool:
		switch strings.ToLower(arg) {
		case "true", "on", "yes":
			return true, nil
		case "false", "off", "no":
			return false, nil
		}
		return nil, fmt.Errorf("%s is on or off; use true or false", s.Name())
	case KindChoice:
		if !slices.Contains(s.Choices, arg) {
			return nil, fmt.Errorf("%s must be %s", s.Name(), orList(s.Choices))
		}
	case KindInt:
		n, err := strconv.ParseInt(arg, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s must be a whole number", s.Name())
		}
		return n, nil
	}
	return arg, nil
}

// orList quotes values as `"a", "b", or "c"`.
func orList(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = strconv.Quote(v)
	}
	if len(quoted) <= 2 {
		return strings.Join(quoted, " or ")
	}
	return strings.Join(quoted[:len(quoted)-1], ", ") + ", or " + quoted[len(quoted)-1]
}
