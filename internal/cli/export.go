package cli

import (
	"encoding/csv"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/store"
)

var csvHeader = []string{
	"id", "status", "mode", "command", "cwd",
	"started_at", "finished_at", "duration_ms", "exit_code",
	"hostname", "git_branch",
}

func (a *app) exportCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "export [id]",
		Short: "export runs as markdown, json, or csv",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if format == "csv" {
				return a.exportCSV(cmd, args)
			}
			// single-run formats: require id
			if len(args) == 0 {
				return fmt.Errorf("export requires <id> for markdown/json formats")
			}
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			r, err := a.st.GetRun(id)
			if err != nil {
				return err
			}
			if format == "json" {
				return writeJSON(cmd, runViewFromStore(r, true))
			}
			return exportMarkdown(cmd, r)
		},
	}
	cmd.Flags().StringVarP(&format, "format", "f", "markdown", "output format: markdown, json, csv")
	return cmd
}

func exportMarkdown(cmd *cobra.Command, r *store.Run) error {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "# carrier run %d\n\n", r.ID)
	_, _ = fmt.Fprintf(out, "**Command:** `%s`  \n", displayCommand(r))
	_, _ = fmt.Fprintf(out, "**CWD:** `%s`  \n", r.CWD)
	_, _ = fmt.Fprintf(out, "**Status:** %s  \n", r.Status)
	if r.ExitCode != nil {
		_, _ = fmt.Fprintf(out, "**Exit code:** %d  \n", *r.ExitCode)
	}
	_, _ = fmt.Fprintf(out, "**Duration:** %s  \n", formatDuration(r.DurationMS))
	_, _ = fmt.Fprintf(out, "**Started:** %s  \n", formatTime(r.StartedAt))
	if r.TerminalOutputPath != "" {
		_, _ = fmt.Fprintf(out, "\n## terminal\n\n```text\n%s\n```\n", readText(r.TerminalOutputPath))
		return nil
	}
	_, _ = fmt.Fprintf(out, "\n## stdout\n\n```text\n%s\n```\n", readText(r.StdoutPath))
	_, _ = fmt.Fprintf(out, "\n## stderr\n\n```text\n%s\n```\n", readText(r.StderrPath))
	return nil
}

func (a *app) exportCSV(cmd *cobra.Command, args []string) error {
	w := csv.NewWriter(cmd.OutOrStdout())
	if err := w.Write(csvHeader); err != nil {
		return err
	}
	if len(args) == 1 {
		id, err := parseID(args[0])
		if err != nil {
			return err
		}
		r, err := a.st.GetRun(id)
		if err != nil {
			return err
		}
		if err := w.Write(runToCSVRow(r)); err != nil {
			return err
		}
	} else {
		runs, err := a.st.AllRuns()
		if err != nil {
			return err
		}
		for i := range runs {
			if err := w.Write(runToCSVRow(&runs[i])); err != nil {
				return err
			}
		}
	}
	w.Flush()
	return w.Error()
}

func runToCSVRow(r *store.Run) []string {
	finishedAt := ""
	if r.FinishedAt != nil {
		finishedAt = r.FinishedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	durationMS := ""
	if r.DurationMS != nil {
		durationMS = strconv.FormatInt(*r.DurationMS, 10)
	}
	exitCode := ""
	if r.ExitCode != nil {
		exitCode = strconv.Itoa(*r.ExitCode)
	}
	return []string{
		strconv.FormatInt(r.ID, 10),
		r.Status,
		r.Mode,
		displayCommand(r),
		r.CWD,
		r.StartedAt.UTC().Format("2006-01-02T15:04:05Z"),
		finishedAt,
		durationMS,
		exitCode,
		r.Hostname,
		r.GitBranch,
	}
}
