package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/store"
)

type historyItem struct {
	isSession bool
	session   store.Session
	runs      []store.Run // session children, display order
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
		short        bool
	)
	cmd := &cobra.Command{
		Use:     "history",
		Aliases: []string{"h"},
		Short:   "list recorded runs oldest-first",
		Long: `List recorded runs oldest-first, one per line.

Pipe to fzf to fuzzy-search and extract an ID for rerun:

  carrier history | fzf | awk '{print $1}' | xargs carrier rerun`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			f := store.HistoryFilter{Status: status, CWD: cwd, Branch: branch, Command: command, Label: label}
			if session != "" {
				sess, err := resolveSession(cmd, a.st, session)
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
				return a.printSessionsOnly(cmd, limit, label, jsonOutput)
			}

			runs, err := a.st.ListHistory(limit, f)
			if err != nil {
				return err
			}
			slices.Reverse(runs)
			if len(runs) == 0 && !jsonOutput {
				a.printEmptyRunsHint(cmd, "no runs match these filters")
				return nil
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

			runIDs := make([]int64, 0, len(runs))
			for _, r := range runs {
				runIDs = append(runIDs, r.ID)
			}
			notesByRun, sessionNotes, err := a.historyNotes(runIDs, sessIDs)
			if err != nil {
				return err
			}

			// Build ordered items. Session runs are bucketed under their session
			// header at the position of the first (oldest displayed) run seen for that session.
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
					for _, n := range notesByRun[item.run.ID] {
						printNoteLine(out, t, n, "")
					}
					continue
				}
				printSessionHeader(out, t, item.session)
				if short {
					n := len(item.runs)
					s := ""
					if n != 1 {
						s = "s"
					}
					_, _ = fmt.Fprintf(
						out, "%s %s ... (%d run%s)\n",
						strings.Repeat(" ", 6),
						t.Accent.Render("└──"),
						n, s,
					)
				} else {
					for _, n := range sessionNotes[item.session.ID] {
						printNoteLine(out, t, n, "│")
					}
					for i, r := range item.runs {
						r := r
						connector, rail := "├──", "│"
						if i == len(item.runs)-1 {
							connector, rail = "└──", ""
						}
						printRunLine(out, t, &r, connector)
						for _, n := range notesByRun[r.ID] {
							printNoteLine(out, t, n, rail)
						}
					}
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
	cmd.Flags().StringVarP(&session, "session", "s", "", "filter by session ID or label")
	cmd.Flags().BoolVar(&sessionsOnly, "sessions-only", false, "show only session headers, no individual runs")
	cmd.Flags().BoolVar(&short, "short", false, "collapse session children to a single '...' line (or use carrier hs)")
	return cmd
}

// hsCmd returns a history command with --short pre-enabled.
func (a *app) hsCmd() *cobra.Command {
	cmd := a.historyCmd()
	cmd.Use = "hs"
	cmd.Short = "history overview with sessions collapsed (alias for history --short)"
	cmd.Aliases = nil
	_ = cmd.Flags().Set("short", "true")
	return cmd
}

// historyNotes fetches notes for the displayed runs and sessions. Notes linked
// to a run are keyed by run ID; session notes made before the session's first
// run are keyed by session ID.
func (a *app) historyNotes(runIDs, sessIDs []int64) (map[int64][]store.Note, map[int64][]store.Note, error) {
	byRun := map[int64][]store.Note{}
	bySession := map[int64][]store.Note{}
	if len(runIDs) == 0 && len(sessIDs) == 0 {
		return byRun, bySession, nil
	}
	notes, err := a.st.ListNotes(store.NoteFilter{RunIDs: runIDs, SessionIDs: sessIDs}, len(runIDs)*10+len(sessIDs)*10+100)
	if err != nil {
		return nil, nil, err
	}
	for _, n := range notes {
		switch {
		case n.RunID != nil:
			byRun[*n.RunID] = append(byRun[*n.RunID], n)
		case n.SessionID != nil:
			bySession[*n.SessionID] = append(bySession[*n.SessionID], n)
		}
	}
	return byRun, bySession, nil
}

func (a *app) printSessionsOnly(cmd *cobra.Command, limit int, labelFilter string, jsonOutput bool) error {
	sessions, err := a.st.ListSessions(limit)
	if err != nil {
		return err
	}
	slices.Reverse(sessions)
	if jsonOutput {
		matched := make([]store.Session, 0, len(sessions))
		for _, sess := range sessions {
			if labelFilter == "" || containsFold(sess.Label, labelFilter) {
				matched = append(matched, sess)
			}
		}
		return writeSessionsJSON(cmd, matched)
	}
	if len(sessions) == 0 {
		printHint(cmd, noSessionsHint)
		return nil
	}
	out := cmd.OutOrStdout()
	t := newTheme(out)
	shown := 0
	for _, sess := range sessions {
		if labelFilter != "" && !containsFold(sess.Label, labelFilter) {
			continue
		}
		printSessionHeader(out, t, sess)
		shown++
	}
	if shown == 0 {
		printHint(cmd, fmt.Sprintf("no sessions match label %q", labelFilter))
	}
	return nil
}

func printSessionHeader(out io.Writer, t theme, sess store.Session) {
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
		t.Muted.Render(padRight(formatRelativeTime(sess.StartedAt), 9)),
		t.Label.Render(sessLabel),
	)
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
		out, "%s %s %s  %s  %s  %s  %s%s\n",
		t.ID.Render(fmt.Sprintf("%6d", r.ID)),
		conn,
		renderStatus(t, r.Status, 7),
		t.Muted.Render(padRight(formatRelativeTime(r.StartedAt), 9)),
		t.Muted.Render(padRight(formatDuration(r.DurationMS), 9)),
		t.Command.Render(displayCommand(r)),
		t.Muted.Render(collapseHome(r.CWD)),
		labelSuffix,
	)
}
