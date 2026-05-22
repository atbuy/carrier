package cli

import (
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

// activeUI holds the color settings resolved from config at startup. It is read
// by newTheme on every call. configureTheme replaces it once during startup;
// the zero value (auto mode, default colors) is safe before that runs.
var activeUI = config.Default().UI

// configureTheme records the resolved UI config so newTheme can honor the user's
// color mode and palette. Called once after config load.
func configureTheme(cfg config.Config) {
	activeUI = cfg.UI
}

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
	applyColorProfile(r, w)
	c := activeUI.Theme
	return theme{
		Bold:    r.NewStyle().Bold(true),
		Header:  r.NewStyle().Bold(true),
		Muted:   r.NewStyle().Foreground(lipgloss.Color(c.Muted)),
		ID:      r.NewStyle().Bold(true),
		Command: r.NewStyle().Foreground(lipgloss.Color(c.Command)),
		Success: r.NewStyle().Foreground(lipgloss.Color(c.Success)),
		Danger:  r.NewStyle().Foreground(lipgloss.Color(c.Danger)),
		Warning: r.NewStyle().Foreground(lipgloss.Color(c.Warning)),
		Accent:  r.NewStyle().Foreground(lipgloss.Color(c.Accent)),
		Label:   r.NewStyle().Foreground(lipgloss.Color(c.Label)),
	}
}

// applyColorProfile forces the renderer's color profile based on the resolved
// color mode. In auto mode it leaves lipgloss's per-writer TTY detection in
// place.
func applyColorProfile(r *lipgloss.Renderer, w io.Writer) {
	switch resolveColorMode() {
	case config.ColorNever:
		r.SetColorProfile(termenv.Ascii)
	case config.ColorAlways:
		r.SetColorProfile(termenv.TrueColor)
	default: // auto: keep per-writer TTY detection
	}
}

// resolveColorMode collapses environment overrides and config into a final
// color mode. Environment variables take precedence over config so a single
// invocation can override the persistent setting. NO_COLOR and TERM=dumb are
// absolute (matching the run status line and the no-color.org convention).
func resolveColorMode() string {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return config.ColorNever
	}
	switch strings.ToLower(os.Getenv("CARRIER_COLOR")) {
	case "always", "1", "true":
		return config.ColorAlways
	case "never", "0", "false":
		return config.ColorNever
	}
	switch activeUI.Color {
	case config.ColorAlways:
		return config.ColorAlways
	case config.ColorNever:
		return config.ColorNever
	default:
		return config.ColorAuto
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
