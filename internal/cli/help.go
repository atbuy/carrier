package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func installHelp(root *cobra.Command) {
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		printHelp(cmd.OutOrStdout(), cmd)
	})
}

func printHelp(w io.Writer, cmd *cobra.Command) {
	t := newTheme(w)
	if cmd.HasParent() {
		printCommandHelp(w, t, cmd)
		return
	}
	printRootHelp(w, t, cmd)
}

func printRootHelp(w io.Writer, t theme, cmd *cobra.Command) {
	line(w, t.Accent.Bold(true).Render("carrier"))
	line(w, t.Muted.Render("  local developer command logger"))
	line(w)

	line(w, t.Header.Render("USAGE"))
	line(w, t.Muted.Render("  carrier")+" "+t.Muted.Render("[flags]")+" "+t.Label.Render("<command>"))
	line(w)

	printCommandGroup(w, t, "RUN", cmd, []string{"run", "shell", "attach", "rerun", "watch"})
	printCommandGroup(w, t, "INSPECT", cmd, []string{"tui", "last", "running", "show", "tail", "failed", "search", "history", "stats", "diff", "export"})
	printCommandGroup(w, t, "MANAGE", cmd, []string{"label", "session", "clean", "config", "completion", "doctor", "version", "help"})

	line(w, t.Header.Render("EXAMPLES"))
	line(w)
	printExample(w, t, "carrier run go test ./...", "record and log a command")
	printExample(w, t, "carrier -n run docker compose build", "notify when done")
	printExample(w, t, "carrier tui", "browse runs interactively")
	printExample(w, t, "carrier show 42", "inspect run #42")
	printExample(w, t, "carrier tail 42", "stream output of run #42")
	printExample(w, t, `carrier search "connection refused"`, "full-text search logs")
	printExample(w, t, "carrier shell", "start a tracked shell session")
	printExample(w, t, "carrier attach mylabel", "rejoin a labeled session")
	line(w)

	printFlagSet(w, t, "FLAGS", cmd.PersistentFlags())
}

func printCommandHelp(w io.Writer, t theme, cmd *cobra.Command) {
	line(w, t.Accent.Bold(true).Render(cmd.CommandPath()))
	if cmd.Short != "" {
		linef(w, "  %s\n", t.Muted.Render(cmd.Short))
	}
	line(w)

	line(w, t.Header.Render("USAGE"))
	linef(w, "  %s\n", t.Muted.Render(cmd.UseLine()))

	if cmd.Long != "" {
		line(w)
		for _, l := range strings.Split(strings.TrimSpace(cmd.Long), "\n") {
			if l == "" {
				line(w)
				continue
			}
			if isLongHeader(l) {
				linef(w, "  %s\n", t.Header.Render(l))
				continue
			}
			stripped := strings.TrimLeft(l, " ")
			if strings.HasPrefix(stripped, "-") && len(l)-len(stripped) >= 2 {
				parts := strings.SplitN(stripped, "  ", 2)
				flagName := parts[0]
				desc := ""
				if len(parts) == 2 {
					desc = strings.TrimSpace(parts[1])
				}
				linef(w, "    %s  %s\n", t.Label.Render(padRight(flagName, 24)), t.Muted.Render(desc))
				continue
			}
			linef(w, "  %s\n", t.Muted.Render(l))
		}
	}

	printSubcommands(w, t, cmd)

	if cmd.Example != "" {
		line(w)
		line(w, t.Header.Render("EXAMPLES"))
		line(w)
		for _, ex := range strings.Split(cmd.Example, "\n") {
			trimmed := strings.TrimSpace(ex)
			if trimmed == "" {
				continue
			}
			linef(w, "  %s\n", t.Command.Render(trimmed))
		}
	}

	printFlagSet(w, t, "FLAGS", cmd.LocalFlags())
	printFlagSet(w, t, "GLOBAL FLAGS", cmd.InheritedFlags())
}

func printSubcommands(w io.Writer, t theme, cmd *cobra.Command) {
	printed := false
	for _, child := range cmd.Commands() {
		if child.Hidden {
			continue
		}
		if !printed {
			line(w)
			line(w, t.Header.Render("COMMANDS"))
			line(w)
			printed = true
		}
		linef(w, "  %s  %s\n", t.Label.Render(padRight(child.Name(), 10)), t.Muted.Render(child.Short))
	}
}

func printCommandGroup(w io.Writer, t theme, title string, root *cobra.Command, names []string) {
	line(w, t.Header.Render(title))
	line(w)
	for _, name := range names {
		child, _, err := root.Find([]string{name})
		if err != nil || child == nil || child.Hidden {
			continue
		}
		linef(w, "  %s  %s\n", t.Label.Render(padRight(child.Name(), 10)), t.Muted.Render(child.Short))
	}
	line(w)
}

func printFlagSet(w io.Writer, t theme, title string, flags *pflag.FlagSet) {
	if flags == nil || !flags.HasAvailableFlags() {
		return
	}
	line(w)
	line(w, t.Header.Render(title))
	line(w)
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}
		name := "--" + flag.Name
		if flag.Shorthand != "" {
			name = "-" + flag.Shorthand + ", " + name
		}
		desc := flag.Usage
		if flag.DefValue != "false" && flag.DefValue != "" {
			desc += " " + t.Muted.Render("(default "+flag.DefValue+")")
		}
		linef(w, "  %s  %s\n", t.Label.Render(padRight(name, 22)), t.Muted.Render(desc))
	})
}

func printExample(w io.Writer, t theme, cmd, desc string) {
	linef(w, "  %s  %s\n", t.Command.Render(padRight(cmd, 42)), t.Muted.Render(desc))
}

func line(w io.Writer, args ...any) {
	_, _ = fmt.Fprintln(w, args...)
}

func linef(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// isLongHeader reports whether a Long description line is a section header:
// starts with an uppercase letter and contains no lowercase letters.
func isLongHeader(s string) bool {
	if s == "" || s[0] < 'A' || s[0] > 'Z' {
		return false
	}
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			return false
		}
	}
	return true
}
