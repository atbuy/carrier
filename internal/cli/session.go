package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func (a *app) sessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "session",
		Aliases: []string{"se"},
		Short:   "manage shell session labels and history",
	}
	cmd.AddCommand(a.sessionLabelCmd(), a.sessionListCmd())
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
  carrier session label                 # clear label on $CARRIER_SESSION_ID`,
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
			out := cmd.OutOrStdout()
			if label == "" {
				_, _ = fmt.Fprintf(out, "carrier: session %d label cleared\n", id)
			} else {
				_, _ = fmt.Fprintf(out, "carrier: session %d labeled %q\n", id, label)
			}
			return nil
		},
	}
}

func (a *app) sessionListCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list shell sessions newest-first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sessions, err := a.st.ListSessions(limit)
			if err != nil {
				return err
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
				_, _ = fmt.Fprintf(
					out, "%s  %s  %s%s\n",
					t.Accent.Render(fmt.Sprintf("%6d", sess.ID)),
					t.Muted.Render(formatTime(sess.StartedAt)),
					t.Label.Render(label),
					duration,
				)
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "l", 100, "maximum number of sessions to show")
	return cmd
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
