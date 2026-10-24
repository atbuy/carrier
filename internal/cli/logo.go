package cli

import (
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"golang.org/x/term"

	"github.com/atbuy/carrier/internal/config"
)

// logoPixels is carrier's logo, docs/assets/logo.svg, redrawn as pixel art
// for the terminal: an isometric shipping container with a prompt on its
// side. Each letter is a pixel colored by logoPalette and "." is transparent.
// Two rows make one line of text, using half-block characters.
var logoPixels = []string{
	"...................kk.......",
	".................erkree.....",
	"...............errrrrrree...",
	".............errrrrrrrrrrek.",
	"...........errrrrrrrrrrrrkkc",
	".........errrrrrrrrrrrrrettt",
	".......errrrrrrrrrrrrrettdst",
	".....errrrrrrrrrrrrrettdsdst",
	"...errrrrrrrrrrrrrettdsdsdst",
	".kkrrrrrrrrrrrrrettdsdsdsdst",
	"ckerrrrrrrrrrrettdsdspsdsdst",
	"bbbeerrrrrrrettdsdspppsdsdst",
	"bncbbeerkrettdsdspppppsdsdst",
	"bncnnbbkkttdsdspppppppsdsdst",
	"bncnncnccdsdsdppspppppsdsdst",
	"bncnncnbtdsdsdpppsppppsdsdsc",
	"bncnncnbtdsdsdppspggpdsdsbb.",
	"bncnncnbtdsdsdpppppdsdsbb...",
	"bncnncnbtdsdsdpppdsdsbb.....",
	"bncnncnbtdsdsdpdsdsbb.......",
	"bncnncnbtdsdsdsdsbb.........",
	"cncnncnbtdsdsdsbb...........",
	".mmnncnbtdsdsbb.............",
	"...mmcnbtdsbb...............",
	".....mmbtbb.................",
	".......cc...................",
}

var logoPalette = map[byte]string{
	'r': "#b5d1fa", // roof
	'e': "#9cc0f5", // roof edge
	'k': "#2c3550", // roof corner castings
	's': "#7AADF4", // side
	'd': "#6596e0", // side ribs
	't': "#4f82c9", // side rails
	'b': "#3d6db3", // bottom rail, door-end rails
	'n': "#5a8fd6", // door end
	'm': "#2f5a9a", // door-end bottom rail
	'c': "#1f2f4f", // corner castings, door locking bars
	'p': "#11141c", // prompt plate
	'g': "#6AAF6A", // cursor
}

// logo returns the logo as lines of text for w, or nil when w is not a
// terminal, colors are off, or the terminal has fewer than 256 colors.
func logo(w io.Writer) []string {
	f, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) || resolveColorMode() == config.ColorNever {
		return nil
	}
	return renderLogo(lipgloss.NewRenderer(w).ColorProfile())
}

// renderLogo draws logoPixels with half blocks: the upper pixel of each cell
// is the foreground of "▀" and the lower one its background. The 256-color
// profile maps each color to its nearest palette entry.
func renderLogo(p termenv.Profile) []string {
	if p != termenv.TrueColor && p != termenv.ANSI256 {
		return nil
	}
	color := func(b byte) termenv.Color {
		if b == '.' {
			return nil
		}
		return p.Color(logoPalette[b])
	}
	lines := make([]string, 0, (len(logoPixels)+1)/2)
	for y := 0; y < len(logoPixels); y += 2 {
		top := logoPixels[y]
		bottom := strings.Repeat(".", len(top))
		if y+1 < len(logoPixels) {
			bottom = logoPixels[y+1]
		}
		var line strings.Builder
		for x := 0; x < len(top); x++ {
			up, down := color(top[x]), color(bottom[x])
			switch {
			case up == nil && down == nil:
				line.WriteString(" ")
			case down == nil:
				line.WriteString(p.String("▀").Foreground(up).String())
			case up == nil:
				line.WriteString(p.String("▄").Foreground(down).String())
			case top[x] == bottom[x]:
				line.WriteString(p.String("█").Foreground(up).String())
			default:
				line.WriteString(p.String("▀").Foreground(up).Background(down).String())
			}
		}
		lines = append(lines, line.String())
	}
	return lines
}

// besideLogo writes text to the right of the logo, starting at the logo's
// line first. Without a logo, it writes text alone.
func besideLogo(out io.Writer, logo []string, first int, text ...string) {
	if len(logo) == 0 {
		for _, line := range text {
			_, _ = io.WriteString(out, line+"\n")
		}
		return
	}
	width := 0
	for _, line := range logo {
		width = max(width, ansi.StringWidth(line))
	}
	n := max(len(logo), first+len(text))
	for i := range n {
		line := strings.Repeat(" ", width)
		if i < len(logo) {
			line = logo[i] + strings.Repeat(" ", width-ansi.StringWidth(logo[i]))
		}
		if j := i - first; j >= 0 && j < len(text) {
			line += "   " + text[j]
		}
		_, _ = io.WriteString(out, strings.TrimRight(line, " ")+"\n")
	}
}
