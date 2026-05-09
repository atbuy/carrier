package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

func (a *app) cleanCmd() *cobra.Command {
	var olderThan string
	cmd := &cobra.Command{
		Use:   "clean --older-than 30d",
		Short: "delete old records and logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := parseAge(olderThan)
			if err != nil {
				return err
			}
			runs, err := a.st.DeleteOlderThan(time.Now().Add(-d))
			if err != nil {
				return err
			}
			for _, r := range runs {
				removeIfSet(r.StdoutPath)
				removeIfSet(r.StderrPath)
				removeIfSet(r.TerminalOutputPath)
			}
			fmt.Printf("deleted %d runs\n", len(runs))
			return nil
		},
	}
	cmd.Flags().StringVar(&olderThan, "older-than", "", "delete runs older than duration, e.g. 30d")
	_ = cmd.MarkFlagRequired("older-than")
	return cmd
}

func parseAge(s string) (time.Duration, error) {
	if len(s) > 1 && s[len(s)-1] == 'd' {
		h := s[:len(s)-1] + "h"
		d, err := time.ParseDuration(h)
		return d * 24, err
	}
	return time.ParseDuration(s)
}

func removeIfSet(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}
