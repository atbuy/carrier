package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/store"
)

type historyItem struct {
	isSession bool
	session   store.Session
	runs      []store.Run // session children, desc order
	run       store.Run   // standalone run
}

func (a *app) historyCmd() *cobra.Command {
	var (
		limit        int
		jsonOutput   bool
		status       string
		since        string
		cwd          string
		branch       string
		command      string
		label        string
		session      string
		sessionsOnly bool
	)
	cmd := &cobra.Command{
		Use:     "history",
		Aliases: []string{"h"},
		Short:   "list recorded runs newest-first",
		Long: `List recorded runs newest-first, one per line.

Pipe to fzf to fuzzy-search and extract an ID for rerun:

  carrier history | fzf | awk '{print $1}' | xargs carrier rerun`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			f := store.HistoryFilter{Status: status, CWD: cwd, Branch: branch, Command: command, Label: label}
			if session != "" {
				sess, err := resolveSession(a.st, session)
				if err != nil {
					return err
				}
				f.SessionID = &sess.ID
			}
			if since != "" {
				d, err := parseAge(since)
				if err != nil {
					return fmt.Errorf("invalid --since duration %q: %w", since, err)
				}
				f.Since = time.Now().Add(-d)
			}
			if sessionsOnly {
				return a.printSessionsOnly(cmd, limit, label)
			}

			runs, err := a.st.ListHistory(limit, f)
			if err != nil {
				return err
			}
			if jsonOutput {
				views := make([]runView, 0, len(runs))
				for _, r := range runs {
					views = append(views, runViewFromStore(&r, false))
				}
				return writeJSON(cmd, views)
			}

			// Batch-fetch sessions for all session_ids in the result set.
			sessIDSet := map[int64]bool{}
			for _, r := range runs {
				if r.SessionID != nil {
					sessIDSet[*r.SessionID] = true
				}
			}
			sessIDs := make([]int64, 0, len(sessIDSet))
			for id := range sessIDSet {
				sessIDs = append(sessIDs, id)
			}
			sessions, err := a.st.GetSessionsByIDs(sessIDs)
			if err != nil {
				return err
			}

			// Build ordered items. Session runs are bucketed under their session
			// header at the position of the first (newest) run seen for that session.
			var items []historyItem
			sessItemIdx := map[int64]int{}

			for _, r := range runs {
				r := r
				if r.SessionID == nil {
					items = append(items, historyItem{run: r})
				} else {
					sessID := *r.SessionID
					if idx, ok := sessItemIdx[sessID]; ok {
						items[idx].runs = append(items[idx].runs, r)
					} else {
						sess := sessions[sessID]
						idx = len(items)
						sessItemIdx[sessID] = idx
						items = append(items, historyItem{
							isSession: true,
							session:   sess,
							runs:      []store.Run{r},
						})
					}
				}
			}

			out := cmd.OutOrStdout()
			t := newTheme(out)

			for _, item := range items {
				if !item.isSession {
					printRunLine(out, t, &item.run, "")
					continue
				}
				sess := item.session
				sessLabel := sess.Label
				if sessLabel == "" {
					sessLabel = "(unlabeled)"
				}
				sessStatusStyle := t.Muted
				sessStatus := "ended"
				if sess.EndedAt == nil {
					sessStatus = "active"
					sessStatusStyle = t.Success
				}
				_, _ = fmt.Fprintf(
					out, "%s %s %s  %s  %s\n",
					t.Accent.Render(fmt.Sprintf("%6d", sess.ID)),
					t.Accent.Render("┬──"),
					sessStatusStyle.Render(padRight(sessStatus, 7)),
					t.Muted.Render(formatTime(sess.StartedAt)),
					t.Label.Render(sessLabel),
				)
				for i, r := range item.runs {
					r := r
					connector := "├──"
					if i == len(item.runs)-1 {
						connector = "└──"
					}
					printRunLine(out, t, &r, connector)
				}
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "l", 500, "maximum number of runs to show")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	cmd.Flags().StringVar(&status, "status", "", "filter by status (success, failed, running, killed)")
	cmd.Flags().StringVar(&since, "since", "", "only runs started within this duration ago (e.g. 24h, 7d)")
	cmd.Flags().StringVar(&cwd, "cwd", "", "filter by working directory (substring match)")
	cmd.Flags().StringVar(&branch, "branch", "", "filter by git branch (exact match)")
	cmd.Flags().StringVarP(&command, "command", "c", "", "filter by command (substring match)")
	cmd.Flags().StringVar(&label, "label", "", "filter by label (substring match)")
	cmd.Flags().StringVar(&session, "session", "", "filter by session ID or label")
	cmd.Flags().BoolVar(&sessionsOnly, "sessions-only", false, "show only session headers, no individual runs")
	return cmd
}

func (a *app) printSessionsOnly(cmd *cobra.Command, limit int, labelFilter string) error {
	sessions, err := a.st.ListSessions(limit)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	t := newTheme(out)
	for _, sess := range sessions {
		if labelFilter != "" && !containsFold(sess.Label, labelFilter) {
			continue
		}
		sessLabel := sess.Label
		if sessLabel == "" {
			sessLabel = "(unlabeled)"
		}
		sessStatusStyle := t.Muted
		sessStatus := "ended"
		if sess.EndedAt == nil {
			sessStatus = "active"
			sessStatusStyle = t.Success
		}
		_, _ = fmt.Fprintf(
			out, "%s %s %s  %s  %s\n",
			t.Accent.Render(fmt.Sprintf("%6d", sess.ID)),
			t.Accent.Render("┬──"),
			sessStatusStyle.Render(padRight(sessStatus, 7)),
			t.Muted.Render(formatTime(sess.StartedAt)),
			t.Label.Render(sessLabel),
		)
	}
	return nil
}

// printRunLine renders one run. connector is one of "├──", "└──", or "" (standalone).
// All three render to the same display width so status/time/command columns stay aligned.
func printRunLine(out io.Writer, t theme, r *store.Run, connector string) {
	labelSuffix := ""
	if r.Label != "" {
		labelSuffix = "  " + t.Label.Render(r.Label)
	}
	conn := "   " // 3 spaces — same display width as ├── / └──
	if connector != "" {
		conn = t.Accent.Render(connector)
	}
	_, _ = fmt.Fprintf(
		out, "%s %s %s  %s  %s  %s%s\n",
		t.ID.Render(fmt.Sprintf("%6d", r.ID)),
		conn,
		t.statusStyle(r.Status).Render(padRight(r.Status, 7)),
		t.Muted.Render(formatTime(r.StartedAt)),
		t.Command.Render(displayCommand(r)),
		t.Muted.Render(r.CWD),
		labelSuffix,
	)
}
