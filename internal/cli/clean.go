package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/store"
)

func (a *app) cleanCmd() *cobra.Command {
	var olderThan string
	var keepLast int
	var dryRun bool
	var yes bool
	cmd := &cobra.Command{
		Use:     "clean",
		Aliases: []string{"cl"},
		Short:   "delete old records and logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			if olderThan == "" && keepLast == 0 {
				return fmt.Errorf("at least one of --older-than or --keep-last is required")
			}
			out := cmd.OutOrStdout()
			t := newTheme(out)

			if dryRun {
				runs, err := a.previewClean(olderThan, keepLast)
				if err != nil {
					return err
				}
				for _, r := range runs {
					_, _ = fmt.Fprintf(
						out, "%s  %s  %s  %s\n",
						t.ID.Render(fmt.Sprintf("%d", r.ID)),
						t.statusStyle(r.Status).Render(r.Status),
						t.Command.Render(displayCommand(&r)),
						t.Muted.Render(r.CWD),
					)
				}
				_, _ = fmt.Fprintf(out, "%s %s %s\n", t.Warning.Render("would delete"), t.ID.Render(fmt.Sprintf("%d", len(runs))), "runs")
				return nil
			}
			if !yes {
				return fmt.Errorf("refusing to delete without --yes; use --dry-run to preview")
			}
			runs, err := a.execClean(olderThan, keepLast)
			if err != nil {
				return err
			}
			for _, r := range runs {
				removeIfSet(r.StdoutPath)
				removeIfSet(r.StderrPath)
				removeIfSet(r.TerminalOutputPath)
			}
			_, _ = fmt.Fprintf(out, "%s %s %s\n", t.Danger.Render("deleted"), t.ID.Render(fmt.Sprintf("%d", len(runs))), "runs")
			if pruned, err := a.st.PruneOrphanedEnvironments(); err == nil && pruned > 0 {
				_, _ = fmt.Fprintf(out, "%s %s %s\n", t.Danger.Render("pruned"), t.ID.Render(fmt.Sprintf("%d", pruned)), "orphaned environments")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&olderThan, "older-than", "", "delete runs older than this duration (e.g. 30d, 24h)")
	cmd.Flags().IntVar(&keepLast, "keep-last", 0, "keep only the N most recent runs, delete the rest")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "show records that would be deleted without deleting")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "confirm deletion")
	return cmd
}

// previewClean returns runs that would be deleted without deleting them.
func (a *app) previewClean(olderThan string, keepLast int) ([]store.Run, error) {
	seen := map[int64]bool{}
	var result []store.Run
	if olderThan != "" {
		d, err := parseAge(olderThan)
		if err != nil {
			return nil, err
		}
		runs, err := a.st.ListOlderThan(time.Now().Add(-d))
		if err != nil {
			return nil, err
		}
		for _, r := range runs {
			seen[r.ID] = true
			result = append(result, r)
		}
	}
	if keepLast > 0 {
		runs, err := a.st.ListOutsideKeepLast(keepLast)
		if err != nil {
			return nil, err
		}
		for _, r := range runs {
			if !seen[r.ID] {
				seen[r.ID] = true
				result = append(result, r)
			}
		}
	}
	return result, nil
}

// execClean deletes runs according to the given criteria.
func (a *app) execClean(olderThan string, keepLast int) ([]store.Run, error) {
	seen := map[int64]bool{}
	var result []store.Run
	if olderThan != "" {
		d, err := parseAge(olderThan)
		if err != nil {
			return nil, err
		}
		runs, err := a.st.DeleteOlderThan(time.Now().Add(-d))
		if err != nil {
			return nil, err
		}
		for _, r := range runs {
			seen[r.ID] = true
			result = append(result, r)
		}
	}
	if keepLast > 0 {
		runs, err := a.st.DeleteKeepLast(keepLast)
		if err != nil {
			return nil, err
		}
		for _, r := range runs {
			if !seen[r.ID] {
				seen[r.ID] = true
				result = append(result, r)
			}
		}
	}
	return result, nil
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
