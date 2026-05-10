package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

func (a *app) cleanCmd() *cobra.Command {
	var olderThan string
	var dryRun bool
	var yes bool
	cmd := &cobra.Command{
		Use:   "clean --older-than 30d",
		Short: "delete old records and logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := parseAge(olderThan)
			if err != nil {
				return err
			}
			cutoff := time.Now().Add(-d)
			out := cmd.OutOrStdout()
			c := outputColors(out)
			if dryRun {
				runs, err := a.st.ListOlderThan(cutoff)
				if err != nil {
					return err
				}
				for _, r := range runs {
					_, _ = fmt.Fprintf(
						out, "%s  %s  %s  %s\n",
						c.paint(colorCyan, fmt.Sprintf("%d", r.ID)),
						c.paint(statusColor(r.Status), r.Status),
						c.paint(colorGreen, displayCommand(&r)),
						c.paint(colorGray, r.CWD),
					)
				}
				_, _ = fmt.Fprintf(out, "%s %s %s\n", c.paint(colorYellow, "would delete"), c.paint(colorCyan, fmt.Sprintf("%d", len(runs))), "runs")
				return nil
			}
			if !yes {
				return fmt.Errorf("refusing to delete without --yes; use --dry-run to preview")
			}
			runs, err := a.st.DeleteOlderThan(cutoff)
			if err != nil {
				return err
			}
			for _, r := range runs {
				removeIfSet(r.StdoutPath)
				removeIfSet(r.StderrPath)
				removeIfSet(r.TerminalOutputPath)
			}
			_, _ = fmt.Fprintf(out, "%s %s %s\n", c.paint(colorRed, "deleted"), c.paint(colorCyan, fmt.Sprintf("%d", len(runs))), "runs")
			return nil
		},
	}
	cmd.Flags().StringVar(&olderThan, "older-than", "", "delete runs older than duration, e.g. 30d")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "show records that would be deleted without deleting")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "confirm deletion")
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
