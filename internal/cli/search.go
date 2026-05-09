package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (a *app) searchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <text>",
		Short: "search commands and output",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := args[0]
			runs, err := a.st.All(10000)
			if err != nil {
				return err
			}
			for _, r := range runs {
				if containsFold(r.Command, q) || containsFold(r.CWD, q) || containsFold(readText(r.StdoutPath), q) || containsFold(readText(r.StderrPath), q) || containsFold(readText(r.TerminalOutputPath), q) {
					fmt.Printf("%d  %s  %s  %s\n", r.ID, r.Status, r.Command, r.CWD)
				}
			}
			return nil
		},
	}
}
