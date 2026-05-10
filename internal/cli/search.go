package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func (a *app) searchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <text>",
		Short: "search commands and output",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			results, err := a.st.SearchRuns(args[0], 100)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, result := range results {
				r := result.Run
				_, _ = fmt.Fprintf(out, "%d  %s  %s  %s\n", r.ID, r.Status, displayCommand(&r), r.CWD)
				if snippet := cleanSnippet(result.Snippet); snippet != "" {
					_, _ = fmt.Fprintf(out, "    %s\n", snippet)
				}
			}
			return nil
		},
	}
}

func cleanSnippet(snippet string) string {
	return strings.Join(strings.Fields(snippet), " ")
}
