package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/logs"
)

func (a *app) tailCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tail <id>",
		Short: "stream captured output",
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
			follow := r.Status == "running"
			if r.TerminalOutputPath != "" {
				return logs.TailFile(r.TerminalOutputPath, os.Stdout, follow)
			}
			if r.StdoutPath != "" {
				go func() { _ = logs.TailFile(r.StderrPath, os.Stderr, follow) }()
				return logs.TailFile(r.StdoutPath, os.Stdout, follow)
			}
			_, _ = io.WriteString(os.Stderr, "carrier: no output logs\n")
			return nil
		},
	}
}
