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
	return issues
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
