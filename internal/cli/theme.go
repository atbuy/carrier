package cli

import (
	"io"

	"github.com/charmbracelet/lipgloss"

	"github.com/atbuy/carrier/internal/store"
)

type theme struct {
	// Typography
	Bold   lipgloss.Style
	Header lipgloss.Style // bold ALL-CAPS section headers
	Muted  lipgloss.Style // secondary info: time, CWD, descriptions

	// Semantic
	ID      lipgloss.Style // run and session IDs
	Command lipgloss.Style // command text, example commands
	Success lipgloss.Style // success status
	Danger  lipgloss.Style // failed status, errors
	Warning lipgloss.Style // killed status, caution notices

	// Structural
	Accent lipgloss.Style // session IDs, tree connectors, primary accent
	Label  lipgloss.Style // user-defined labels, flag/command names in help
}

func newTheme(w io.Writer) theme {
	r := lipgloss.NewRenderer(w)
	return theme{
		Bold:    r.NewStyle().Bold(true),
		Header:  r.NewStyle().Bold(true),
		Muted:   r.NewStyle().Foreground(lipgloss.Color("#A8A8A8")),
		ID:      r.NewStyle().Bold(true),
		Command: r.NewStyle().Foreground(lipgloss.Color("#6AAF6A")),
		Success: r.NewStyle().Foreground(lipgloss.Color("#6AAF6A")),
		Danger:  r.NewStyle().Foreground(lipgloss.Color("#D75F5F")),
		Warning: r.NewStyle().Foreground(lipgloss.Color("#D7AF5F")),
		Accent:  r.NewStyle().Foreground(lipgloss.Color("#5B8DEF")),
		Label:   r.NewStyle().Foreground(lipgloss.Color("#7AADF4")),
	}
}

func (t theme) statusStyle(status string) lipgloss.Style {
	switch status {
	case store.StatusSuccess:
		return t.Success
	case store.StatusFailed:
		return t.Danger
	case store.StatusKilled:
		return t.Warning
	default:
		return t.Muted
	}
}
