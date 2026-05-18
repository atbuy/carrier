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

	printRootHelp(&buf, newTheme(&buf), root)
	out := buf.String()

	for _, want := range []string{
		"carrier",
		"RUN",
		"INSPECT",
		"MANAGE",
		"run",
		"show",
		"-n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("help output missing %q\n%s", want, out)
		}
	}
}

func TestCommandHelpShowsSubcommands(t *testing.T) {
	cmd := (&app{}).configCmd()
	var buf bytes.Buffer

	printCommandHelp(&buf, newTheme(&buf), cmd)
	out := buf.String()

	for _, want := range []string{
		"USAGE",
		"config",
		"COMMANDS",
		"path",
		"show",
		"init",
		"check",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("config help output missing %q\n%s", want, out)
		}
	}
}

func TestPrintHelp(t *testing.T) {
	root := testRootCommand()
	var buf bytes.Buffer
	printHelp(&buf, root)
	if !strings.Contains(buf.String(), "carrier") {
		t.Fatalf("printHelp root missing 'carrier':\n%s", buf.String())
	}

	buf.Reset()
	subCmd := &cobra.Command{Use: "show <id>", Short: "show full run details"}
	root.AddCommand(subCmd)
	subCmd.AddCommand(&cobra.Command{Use: "sub", Short: "a sub sub command"})
	printHelp(&buf, subCmd)
	if !strings.Contains(buf.String(), "show full run details") {
		t.Fatalf("printHelp sub missing description:\n%s", buf.String())
	}
}

func TestInstallHelpFuncIsInvoked(t *testing.T) {
	root := testRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.HelpFunc()(root, nil)
	if !strings.Contains(buf.String(), "carrier") {
		t.Fatalf("help func output missing 'carrier':\n%s", buf.String())
	}
}

func TestPrintFlagSetWithDefaultValue(t *testing.T) {
	var buf bytes.Buffer
	fs := (&cobra.Command{}).Flags()
	fs.String("output", "text", "output format")
	printFlagSet(&buf, newTheme(&buf), "FLAGS", fs)
	out := buf.String()
	if !strings.Contains(out, "--output") {
		t.Fatalf("missing --output flag:\n%s", out)
	}
	if !strings.Contains(out, "(default text)") {
		t.Fatalf("missing default value annotation:\n%s", out)
	}
}

func TestPrintFlagSetWithShorthandAndDefault(t *testing.T) {
	var buf bytes.Buffer
	fs := (&cobra.Command{}).Flags()
	fs.StringP("format", "f", "json", "output format")
	printFlagSet(&buf, newTheme(&buf), "FLAGS", fs)
	out := buf.String()
	if !strings.Contains(out, "-f,") {
		t.Fatalf("missing shorthand -f:\n%s", out)
	}
	if !strings.Contains(out, "(default json)") {
		t.Fatalf("missing default json:\n%s", out)
	}
}

func TestPrintFlagSetNilFlags(t *testing.T) {
	var buf bytes.Buffer
	printFlagSet(&buf, newTheme(&buf), "FLAGS", nil)
	if buf.Len() != 0 {
		t.Fatalf("expected no output for nil flags, got %q", buf.String())
	}
}

func testRootCommand() *cobra.Command {
	root := &cobra.Command{Use: "carrier"}
	root.PersistentFlags().BoolP("notify", "n", false, "request notification when command finishes")
	root.AddCommand(
		&cobra.Command{Use: "run <command...>", Short: "run and record one command"},
		&cobra.Command{Use: "shell", Short: "start tracked shell (alpha)"},
		&cobra.Command{Use: "attach <id-or-label>", Short: "attach to an existing shell session"},
		&cobra.Command{Use: "rerun <id>", Short: "rerun original command from original cwd"},
		&cobra.Command{Use: "last", Short: "show latest run"},
		&cobra.Command{Use: "running", Short: "list running runs"},
		&cobra.Command{Use: "show <id>", Short: "show full run details"},
		&cobra.Command{Use: "tail <id>", Short: "stream captured output"},
		&cobra.Command{Use: "failed", Short: "list failed runs"},
		&cobra.Command{Use: "search <text>", Short: "search commands and output"},
		&cobra.Command{Use: "history", Short: "list recorded runs oldest-first"},
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
