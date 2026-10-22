package cli

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
)

func (a *app) sessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "session",
		Aliases: []string{"se"},
		Short:   "manage shell session labels and history",
	}
	cmd.AddCommand(a.sessionLabelCmd(), a.sessionListCmd(), a.sessionKeepCmd(true), a.sessionKeepCmd(false),
		a.sessionPinCmd(), a.sessionUnpinCmd(), a.sessionReloadCmd(),
		a.sessionShowCmd(), a.sessionExportCmd(), a.sessionEndCmd())
	return cmd
}

func (a *app) sessionLabelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "label [<id>] [text...]",
		Short: "set or clear a session label",
		Long: `Attach a label to a shell session for grouping runs.

  carrier session label 3 debug run     # set label on session 3
  carrier session label debug run       # set label on $CARRIER_SESSION_ID
  carrier session label 3               # clear label on session 3
  carrier session label                 # clear label on $CARRIER_SESSION_ID

A label that starts with a number is read as a session ID; pass the ID first
("carrier session label $CARRIER_SESSION_ID 1234 fix") to use one.

Labeled automatic sessions never expire.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			id, labelParts, err := parseSessionIDAndLabel(args)
			if err != nil {
				return err
			}
			label := strings.Join(labelParts, " ")
			if err := a.st.UpdateSessionLabel(id, label); err != nil {
				return err
			}
			// The prompt of this shell shows the new label from its next prompt.
			if statePath := recordedShellState(); statePath != "" && id == currentSessionID() {
				_ = carriershell.WriteSessionInfo(statePath, id, label)
			}
			out := cmd.OutOrStdout()
			if label == "" {
				_, _ = fmt.Fprintf(out, "carrier: session %d label cleared\n", id)
			} else {
				_, _ = fmt.Fprintf(out, "carrier: session %d labeled %q\n", id, label)
				if sess, err := a.st.GetSession(id); err == nil && sess.Auto && !sess.Kept {
					printHint(cmd, "labeled automatic sessions are kept and never expire")
				}
			}
			return nil
		},
	}
}

func (a *app) sessionListCmd() *cobra.Command {
	var (
		limit      int
		jsonOutput bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list shell sessions newest-first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sessions, err := a.st.ListSessions(limit)
			if err != nil {
				return err
			}
			if jsonOutput {
				return writeSessionsJSON(cmd, sessions)
			}
			if len(sessions) == 0 {
				printHint(cmd, noSessionsHint)
				return nil
			}
			out := cmd.OutOrStdout()
			t := newTheme(out)
			for _, sess := range sessions {
				label := sess.Label
				if label == "" {
					label = "(unlabeled)"
				}
				duration := ""
				if sess.EndedAt != nil {
					duration = " " + t.Muted.Render(formatSessionDuration(sess.EndedAt.Sub(sess.StartedAt)))
				} else {
					duration = " " + t.Success.Render("active")
				}
				flags := ""
				if sess.Auto {
					flags += " " + t.Muted.Render("auto")
				}
				if sess.Kept {
					flags += " " + t.Accent.Render("kept")
				}
				_, _ = fmt.Fprintf(
					out, "%s  %s  %s%s%s\n",
					t.Accent.Render(fmt.Sprintf("%6d", sess.ID)),
					t.Muted.Render(padRight(formatRelativeTime(sess.StartedAt), 9)),
					t.Label.Render(label),
					duration,
					flags,
				)
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "l", 100, "maximum number of sessions to show")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	return cmd
}

type sessionView struct {
	ID        int64      `json:"id"`
	Label     string     `json:"label,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	Auto      bool       `json:"auto"`
	Kept      bool       `json:"kept"`
}

func writeSessionsJSON(cmd *cobra.Command, sessions []store.Session) error {
	views := make([]sessionView, 0, len(sessions))
	for _, s := range sessions {
		views = append(views, sessionViewFromStore(s))
	}
	return writeJSON(cmd, views)
}

func sessionViewFromStore(s store.Session) sessionView {
	return sessionView{ID: s.ID, Label: s.Label, StartedAt: s.StartedAt, EndedAt: s.EndedAt, Auto: s.Auto, Kept: s.Kept}
}

// sessionKeepCmd builds "session keep" (keep=true) or "session unkeep".
func (a *app) sessionKeepCmd(keep bool) *cobra.Command {
	use, short, done := "keep", "keep a session so it never expires", "kept"
	if !keep {
		use, short, done = "unkeep", "let an automatic session expire again", "no longer kept"
	}
	return &cobra.Command{
		Use:   use + " [<id-or-label>]",
		Short: short,
		Long: fmt.Sprintf(`Automatic sessions (started by "carrier shell --auto") expire after
shell.auto_session_retention unless they have a label, notes, or are kept.

  carrier session %[1]s           # the current session ($CARRIER_SESSION_ID)
  carrier session %[1]s 12        # session 12
  carrier session %[1]s api-debug # by label`, use),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var id int64
			if len(args) == 1 {
				sess, err := resolveSession(cmd, a.st, args[0])
				if err != nil {
					return err
				}
				id = sess.ID
			} else if id = currentSessionID(); id == 0 {
				return fmt.Errorf("no session given and $CARRIER_SESSION_ID is not set")
			}
			if err := a.st.SetSessionKept(id, keep); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return fmt.Errorf("session %d not found", id)
				}
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "carrier: session %d %s\n", id, done)
			return nil
		},
	}
}

// parseSessionIDAndLabel splits args into (sessionID, labelParts).
// If the first arg is an integer, it's treated as the session ID.
// Otherwise $CARRIER_SESSION_ID is used and all args are label parts.
func parseSessionIDAndLabel(args []string) (int64, []string, error) {
	if len(args) > 0 {
		if id, err := strconv.ParseInt(args[0], 10, 64); err == nil {
			return id, args[1:], nil
		}
	}
	envID := os.Getenv("CARRIER_SESSION_ID")
	if envID == "" {
		return 0, nil, fmt.Errorf("no session ID provided and $CARRIER_SESSION_ID is not set")
	}
	id, err := strconv.ParseInt(envID, 10, 64)
	if err != nil {
		return 0, nil, fmt.Errorf("invalid $CARRIER_SESSION_ID %q", envID)
	}
	return id, args, nil
}

func formatSessionDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
