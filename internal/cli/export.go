package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (a *app) exportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "export <id>",
		Short: "export run as Markdown",
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
			fmt.Printf("# carrier run %d\n\n", r.ID)
			fmt.Printf("**Command:** `%s`  \n", displayCommand(r))
			fmt.Printf("**CWD:** `%s`  \n", r.CWD)
			fmt.Printf("**Status:** %s  \n", r.Status)
			if r.ExitCode != nil {
				fmt.Printf("**Exit code:** %d  \n", *r.ExitCode)
			}
			fmt.Printf("**Duration:** %s  \n", formatDuration(r.DurationMS))
			fmt.Printf("**Started:** %s  \n", formatTime(r.StartedAt))
			if r.TerminalOutputPath != "" {
				fmt.Printf("\n## terminal\n\n```text\n%s\n```\n", readText(r.TerminalOutputPath))
				return nil
			}
			fmt.Printf("\n## stdout\n\n```text\n%s\n```\n", readText(r.StdoutPath))
			fmt.Printf("\n## stderr\n\n```text\n%s\n```\n", readText(r.StderrPath))
			return nil
		},
	}
}
