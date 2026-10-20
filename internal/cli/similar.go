package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/hints"
)

func (a *app) similarCmd() *cobra.Command {
	var limit int
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:     "similar <id>",
		Aliases: []string{"sim"},
		Short:   "list earlier failed runs with the same error",
		Long: `List earlier failed runs whose output contains the same error as a run.

Runs with notes or a label come first, since they record what happened.

  carrier similar 57
  carrier similar 57 --json

Set hints.similar_failures = true in the config to be told about the best
match automatically when a run fails.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if _, err := a.getRun(id); err != nil {
				return err
			}
			matches, err := a.st.SimilarFailures(id, time.Now(), limit)
			if err != nil {
				return err
			}
			if jsonOutput {
				type similarView struct {
					runView
					NoteCount int `json:"note_count"`
				}
				views := make([]similarView, 0, len(matches))
				for _, m := range matches {
					views = append(views, similarView{runView: runViewFromStore(&m.Run, false), NoteCount: m.NoteCount})
				}
				return writeJSON(cmd, views)
			}
			if len(matches) == 0 {
				printHint(cmd, fmt.Sprintf("no earlier failed runs share the error in run %d", id))
				return nil
			}
			out := cmd.OutOrStdout()
			t := newTheme(out)
			for _, m := range matches {
				r := m.Run
				_, _ = fmt.Fprintf(
					out, "%s  %s  %s  %s\n",
					t.ID.Render(fmt.Sprintf("%d", r.ID)),
					t.statusStyle(r.Status).Render(r.Status),
					t.Command.Render(displayCommand(&r)),
					t.Muted.Render(collapseHome(r.CWD)),
				)
				_, _ = fmt.Fprintf(out, "    %s\n", t.Muted.Render(strings.Join(hints.Details(a.st, m), " · ")))
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "l", 20, "maximum number of runs to show")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	return cmd
}
