package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRootHelpGroupsCommandsAndFlags(t *testing.T) {
	root := testRootCommand()
	var buf bytes.Buffer

	printRootHelp(&buf, helpColors{}, root)
	out := buf.String()

	for _, want := range []string{
		"carrier",
		"Run",
		"Inspect",
		"Maintenance",
		"run        run and record one command",
		"show       show full run details",
		"-n, --notify",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("help output missing %q\n%s", want, out)
		}
	}
}

func TestCommandHelpShowsSubcommands(t *testing.T) {
	cmd := (&app{}).configCmd()
	var buf bytes.Buffer

	printCommandHelp(&buf, helpColors{}, cmd)
	out := buf.String()

	for _, want := range []string{
		"Usage",
		"config <command>",
		"Commands",
		"path       show config file path",
		"show       show active config",
		"init       write default config file",
		"check      validate active config",
		"carrier config check",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("config help output missing %q\n%s", want, out)
		}
	}
}

func TestHelpColorsCanBeForced(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "always")
	t.Setenv("NO_COLOR", "")

	if !shouldColor(&bytes.Buffer{}) {
		t.Fatalf("expected forced color for non-terminal writer")
	}
	if got := (helpColors{enabled: true}).paint(colorGreen, "run"); got != colorGreen+"run"+colorReset {
		t.Fatalf("color paint mismatch: %q", got)
	}
}

func TestNoColorDisablesColor(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "always")
	t.Setenv("NO_COLOR", "1")

	if shouldColor(&bytes.Buffer{}) {
		t.Fatalf("NO_COLOR should disable color")
	}
}

func testRootCommand() *cobra.Command {
	root := &cobra.Command{Use: "carrier"}
	root.PersistentFlags().BoolP("notify", "n", false, "request notification when command finishes")
	root.AddCommand(
		&cobra.Command{Use: "run <command...>", Short: "run and record one command"},
		&cobra.Command{Use: "shell", Short: "start tracked shell (alpha)"},
		&cobra.Command{Use: "rerun <id>", Short: "rerun original command from original cwd"},
		&cobra.Command{Use: "last", Short: "show latest run"},
		&cobra.Command{Use: "running", Short: "list running runs"},
		&cobra.Command{Use: "show <id>", Short: "show full run details"},
		&cobra.Command{Use: "tail <id>", Short: "stream captured output"},
		&cobra.Command{Use: "failed", Short: "list failed runs"},
		&cobra.Command{Use: "search <text>", Short: "search commands and output"},
		&cobra.Command{Use: "stats", Short: "show run totals and slow commands"},
		&cobra.Command{Use: "export <id>", Short: "export run as Markdown"},
		&cobra.Command{Use: "clean --older-than 30d", Short: "delete old records and logs"},
		&cobra.Command{Use: "config", Short: "inspect and create config"},
		&cobra.Command{Use: "doctor", Short: "check local carrier setup"},
		&cobra.Command{Use: "version", Short: "show carrier version"},
	)
	installHelp(root)
	return root
}
