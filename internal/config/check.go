package config

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	IssueError = "error"
	IssueWarn  = "warn"
)

type Issue struct {
	Level   string
	Field   string
	Message string
}

func Check(cfg Config) []Issue {
	var issues []Issue
	if strings.TrimSpace(cfg.Storage.DataDir) == "" {
		issues = append(issues, Issue{Level: IssueError, Field: "storage.data_dir", Message: "must not be empty"})
	}
	switch {
	case cfg.Storage.MaxOutputMB < 0:
		issues = append(issues, Issue{Level: IssueError, Field: "storage.max_output_mb", Message: "must be zero or greater"})
	case cfg.Storage.MaxOutputMB == 0:
		issues = append(issues, Issue{Level: IssueWarn, Field: "storage.max_output_mb", Message: "output cap disabled"})
	}
	if cfg.Redaction.Enabled && len(cfg.Redaction.Patterns) == 0 {
		issues = append(issues, Issue{Level: IssueWarn, Field: "redaction.patterns", Message: "redaction enabled with no patterns"})
	}
	for i, pattern := range cfg.Redaction.Patterns {
		if strings.TrimSpace(pattern) == "" {
			issues = append(issues, Issue{Level: IssueWarn, Field: fmt.Sprintf("redaction.patterns[%d]", i), Message: "empty pattern"})
			continue
		}
		if _, err := regexp.Compile(pattern); err != nil {
			issues = append(issues, Issue{
				Level:   IssueError,
				Field:   fmt.Sprintf("redaction.patterns[%d]", i),
				Message: "invalid regex: " + err.Error(),
			})
		}
	}
	if strings.TrimSpace(cfg.Notify.MinDuration) == "" {
		issues = append(issues, Issue{Level: IssueError, Field: "notify.min_duration", Message: "must not be empty"})
	} else if d, err := time.ParseDuration(cfg.Notify.MinDuration); err != nil {
		issues = append(issues, Issue{Level: IssueError, Field: "notify.min_duration", Message: "invalid duration: " + err.Error()})
	} else if d < 0 {
		issues = append(issues, Issue{Level: IssueError, Field: "notify.min_duration", Message: "must be zero or greater"})
	}
	if cfg.Shell.Program != "" && strings.TrimSpace(cfg.Shell.Program) == "" {
		issues = append(issues, Issue{Level: IssueError, Field: "shell.program", Message: "must not be blank"})
	}
	for i, item := range cfg.Shell.IgnoreCommands {
		if strings.TrimSpace(item) == "" {
			issues = append(issues, Issue{Level: IssueWarn, Field: fmt.Sprintf("shell.ignore_commands[%d]", i), Message: "empty command name"})
		}
	}
	switch cfg.UI.Color {
	case "", ColorAuto, ColorAlways, ColorNever:
	default:
		issues = append(issues, Issue{Level: IssueError, Field: "ui.color", Message: `must be "auto", "always", or "never"`})
	}
	for _, tc := range []struct{ field, value string }{
		{"ui.theme.muted", cfg.UI.Theme.Muted},
		{"ui.theme.command", cfg.UI.Theme.Command},
		{"ui.theme.success", cfg.UI.Theme.Success},
		{"ui.theme.danger", cfg.UI.Theme.Danger},
		{"ui.theme.warning", cfg.UI.Theme.Warning},
		{"ui.theme.accent", cfg.UI.Theme.Accent},
		{"ui.theme.label", cfg.UI.Theme.Label},
	} {
		if strings.HasPrefix(tc.value, "#") && !isHexColor(tc.value) {
			issues = append(issues, Issue{Level: IssueError, Field: tc.field, Message: "invalid hex color: " + tc.value})
		}
	}
	return issues
}

// isHexColor reports whether s is a #RGB or #RRGGBB hex color string.
func isHexColor(s string) bool {
	if len(s) != 4 && len(s) != 7 {
		return false
	}
	if s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

func CountIssues(issues []Issue) (int, int) {
	var errors, warnings int
	for _, issue := range issues {
		switch issue.Level {
		case IssueError:
			errors++
		case IssueWarn:
			warnings++
		}
	}
	return errors, warnings
}
