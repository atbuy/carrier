package cli

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

// ansiEscRE matches ANSI/VT escape sequences so output can be stripped to plain
// text before diffing. Covers CSI (colors, modes), OSC, and DCS sequences.
var ansiEscRE = regexp.MustCompile(
	`\x1b(?:` +
		`\[[0-9;?]*[ -/]*[@-~]` + // CSI sequences: ESC [ ... final-byte
		`|\][^\x07\x1b]*(?:\x07|\x1b\\)` + // OSC sequences: ESC ] ... BEL or ST
		`|P[^\x1b]*\x1b\\` + // DCS sequences: ESC P ... ST
		`|[@-Z\\-_]` + // single-char ESC sequences: ESC @..Z \ ].._ (FS, GS, RS, US, SS2, SS3, etc.)
		`)`,
)

func (a *app) diffCmd() *cobra.Command {
	var stream string
	var raw bool
	cmd := &cobra.Command{
		Use:   "diff <id1> <id2>",
		Short: "diff the output of two runs",
		Long: `Diff the captured output of two runs using unified diff format.

  carrier diff 41 42
  carrier diff --stream stderr 41 42
  carrier diff --raw 41 42`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id1, err := parseID(args[0])
			if err != nil {
				return err
			}
			id2, err := parseID(args[1])
			if err != nil {
				return err
			}
			r1, err := a.st.GetRun(id1)
			if err != nil {
				return err
			}
			r2, err := a.st.GetRun(id2)
			if err != nil {
				return err
			}

			path1, err := diffStreamPath(r1, stream)
			if err != nil {
				return fmt.Errorf("run %d: %w", id1, err)
			}
			path2, err := diffStreamPath(r2, stream)
			if err != nil {
				return fmt.Errorf("run %d: %w", id2, err)
			}

			if raw {
				if path1 == "" {
					path1 = os.DevNull
				}
				if path2 == "" {
					path2 = os.DevNull
				}
				return execDiff(cmd, id1, id2, path1, path2)
			}

			// Strip ANSI escape sequences so diffs show meaningful text.
			tmp1, err := writeDiffTmp(id1, ansiEscRE.ReplaceAllString(readText(path1), ""))
			if err != nil {
				return err
			}
			defer func() { _ = os.Remove(tmp1) }()

			tmp2, err := writeDiffTmp(id2, ansiEscRE.ReplaceAllString(readText(path2), ""))
			if err != nil {
				return err
			}
			defer func() { _ = os.Remove(tmp2) }()

			return execDiff(cmd, id1, id2, tmp1, tmp2)
		},
	}
	cmd.Flags().StringVar(&stream, "stream", "auto", "stream to diff: auto, stdout, stderr, terminal")
	cmd.Flags().BoolVar(&raw, "raw", false, "skip ANSI stripping (diff raw log files)")
	return cmd
}

// diffStreamPath resolves the log file path for a run given the chosen stream.
// "auto" picks terminal output for shell runs, stdout for run-mode runs.
func diffStreamPath(r *store.Run, stream string) (string, error) {
	switch stream {
	case "stdout":
		return r.StdoutPath, nil
	case "stderr":
		return r.StderrPath, nil
	case "terminal":
		if r.TerminalOutputPath == "" {
			return "", fmt.Errorf("no terminal output log (not a shell run)")
		}
		return r.TerminalOutputPath, nil
	default: // "auto"
		if r.TerminalOutputPath != "" {
			return r.TerminalOutputPath, nil
		}
		return r.StdoutPath, nil
	}
}

func execDiff(cmd *cobra.Command, id1, id2 int64, path1, path2 string) error {
	colorFlag := "--color=auto"
	switch resolveColorMode() {
	case config.ColorAlways:
		colorFlag = "--color=always"
	case config.ColorNever:
		colorFlag = "--color=never"
	}
	c := exec.Command(
		"diff",
		colorFlag,
		"--label", fmt.Sprintf("run #%d", id1),
		"--label", fmt.Sprintf("run #%d", id2),
		"-u", path1, path2,
	)
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.ErrOrStderr()
	if err := c.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return nil // exit 1 = files differ; that's a valid diff result, not an error
		}
		return fmt.Errorf("diff: %w", err)
	}
	return nil
}

func writeDiffTmp(id int64, content string) (string, error) {
	f, err := os.CreateTemp("", fmt.Sprintf("carrier-diff-%d-*", id))
	if err != nil {
		return "", err
	}
	_, werr := f.WriteString(content)
	_ = f.Close()
	if werr != nil {
		_ = os.Remove(f.Name())
		return "", werr
	}
	return f.Name(), nil
}
