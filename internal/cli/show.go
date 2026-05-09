package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/user/carrier/internal/store"
)

func (a *app) lastCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "last",
		Short: "show latest run",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := a.st.Latest()
			if err != nil {
				return err
			}
			printRun(r)
			return nil
		},
	}
}

func (a *app) showCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "show full run details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			r, err := a.st.GetRun(id)
			if err != nil {
				return err
			}
			printRun(r)
			if r.TerminalOutputPath != "" {
				fmt.Println("\nterminal")
				fmt.Print(readText(r.TerminalOutputPath))
				return nil
			}
			if r.StdoutPath != "" {
				fmt.Println("\nstdout")
				fmt.Print(readText(r.StdoutPath))
			}
			if r.StderrPath != "" {
				_, _ = fmt.Fprintln(os.Stdout, "\nstderr")
				fmt.Print(readText(r.StderrPath))
			}
			return nil
		},
	}
}

func (a *app) failedCmd() *cobra.Command {
	return listStatusCmd("failed", store.StatusFailed, a)
}

func (a *app) runningCmd() *cobra.Command {
	return listStatusCmd("running", store.StatusRunning, a)
}

func listStatusCmd(name, status string, a *app) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: "list " + name + " runs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			runs, err := a.st.ListByStatus(status, 100)
			if err != nil {
				return err
			}
			for _, r := range runs {
				age := ""
				if status == store.StatusRunning {
					ms := timeSinceMS(r.StartedAt)
					age = "  " + formatDuration(&ms)
				}
				fmt.Printf("%d  %s  %s  %s%s\n", r.ID, r.Status, r.Command, r.CWD, age)
			}
			return nil
		},
	}
}

func timeSinceMS(t time.Time) int64 {
	return time.Since(t).Milliseconds()
}
