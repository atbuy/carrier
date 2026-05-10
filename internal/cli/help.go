package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"
)

const (
	colorReset = "\x1b[0m"
	colorBold  = "\x1b[1m"
	colorDim   = "\x1b[2m"
	colorCyan  = "\x1b[36m"
	colorGreen = "\x1b[32m"
	colorGray  = "\x1b[90m"
)

func installHelp(root *cobra.Command) {
	root.Long = "carrier records local command executions, output logs, timing, exit codes, and Git context."
	root.Example = strings.Join([]string{
		"  carrier run go test ./...",
		"  carrier -n run docker compose build",
		"  carrier show 42",
		"  carrier tail 42",
		`  carrier search "connection refused"`,
	}, "\n")
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		printHelp(cmd.OutOrStdout(), cmd)
	})
}

func printHelp(w io.Writer, cmd *cobra.Command) {
	c := helpColors{enabled: shouldColor(w)}
	if cmd.HasParent() {
		printCommandHelp(w, c, cmd)
		return
	}
	printRootHelp(w, c, cmd)
}

type helpColors struct {
	enabled bool
}

func (c helpColors) paint(color, text string) string {
	if !c.enabled {
		return text
	}
	return color + text + colorReset
}

func printRootHelp(w io.Writer, c helpColors, cmd *cobra.Command) {
	line(w, c.paint(colorBold+colorCyan, "carrier"))
	line(w, "  local developer command logger")
	line(w)
	line(w, c.paint(colorBold, "Usage"))
	line(w, "  carrier [flags] <command>")
	line(w)
	printCommandGroup(w, c, "Run", cmd, []string{"run", "shell", "rerun"})
	printCommandGroup(w, c, "Inspect", cmd, []string{"last", "running", "show", "tail", "failed", "search", "export"})
	printCommandGroup(w, c, "Maintenance", cmd, []string{"clean", "config", "doctor", "version"})
	line(w, c.paint(colorBold, "Examples"))
	line(w, cmd.Example)
	printFlagSet(w, c, "Global Flags", cmd.PersistentFlags())
}

func printCommandHelp(w io.Writer, c helpColors, cmd *cobra.Command) {
	line(w, c.paint(colorBold+colorCyan, cmd.CommandPath()))
	if cmd.Short != "" {
		linef(w, "  %s\n", cmd.Short)
	}
	line(w)
	line(w, c.paint(colorBold, "Usage"))
	linef(w, "  %s\n", cmd.UseLine())
	if cmd.Example != "" {
		line(w)
		line(w, c.paint(colorBold, "Examples"))
		line(w, cmd.Example)
	}
	printFlagSet(w, c, "Flags", cmd.LocalFlags())
	printFlagSet(w, c, "Global Flags", cmd.InheritedFlags())
}

func printCommandGroup(w io.Writer, c helpColors, title string, root *cobra.Command, names []string) {
	line(w, c.paint(colorBold, title))
	for _, name := range names {
		child, _, err := root.Find([]string{name})
		if err != nil || child == nil || child.Hidden {
			continue
		}
		linef(w, "  %s  %s\n", c.paint(colorGreen, padRight(child.Name(), 9)), child.Short)
	}
	line(w)
}

func printFlagSet(w io.Writer, c helpColors, title string, flags *pflag.FlagSet) {
	if flags == nil || !flags.HasAvailableFlags() {
		return
	}
	line(w)
	line(w, c.paint(colorBold, title))
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}
		name := "--" + flag.Name
		if flag.Shorthand != "" {
			name = "-" + flag.Shorthand + ", " + name
		}
		if flag.DefValue != "false" && flag.DefValue != "" {
			linef(w, "  %s  %s %s\n", c.paint(colorGreen, padRight(name, 22)), flag.Usage, c.paint(colorGray, "(default "+flag.DefValue+")"))
			return
		}
		linef(w, "  %s  %s\n", c.paint(colorGreen, padRight(name, 22)), flag.Usage)
	})
}

func line(w io.Writer, args ...any) {
	_, _ = fmt.Fprintln(w, args...)
}

func linef(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func shouldColor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	switch strings.ToLower(os.Getenv("CARRIER_COLOR")) {
	case "always", "1", "true":
		return true
	case "never", "0", "false":
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
