// Package termbg stops carrier from asking the terminal for its background
// color at startup.
//
// Bubble Tea 1.x calls lipgloss.HasDarkBackground in an init function, which
// writes an OSC 11 query to the terminal and waits up to 5 seconds for the
// reply. Terminals that never reply, such as some tmux setups, Emacs shells,
// and serial consoles, made every carrier command wait that long. carrier
// uses no adaptive colors, so the answer is never needed.
//
// Go initializes packages in import path order once their imports are ready
// (the Go spec since 1.21), and this package imports only lipgloss, so its
// init runs before Bubble Tea's: github.com/atbuy sorts before
// github.com/charmbracelet. Import it from package main for the effect.
package termbg

import "github.com/charmbracelet/lipgloss"

func init() {
	lipgloss.SetHasDarkBackground(true)
}
