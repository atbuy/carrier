package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/atbuy/carrier/internal/duration"
	"github.com/atbuy/carrier/internal/store"
	"github.com/atbuy/carrier/internal/tui"
)

func (a *app) searchCmd() *cobra.Command {
	var jsonOutput bool
	var limit int
	var since string
	var status string
	var pick bool
	cmd := &cobra.Command{
		Use:     "search [<text>...]",
		Aliases: []string{"sr"},
		Short:   "search commands and output",
		Long: `Search the commands, directories, and output of recorded runs, and list the
runs that match.

Without text, or with --pick, search opens history search instead: the
commands of every terminal, newest first, filtered as you type. Enter prints
the command picked, Tab switches between every terminal, this directory, and
this session, and Esc cancels. With shell.history_search on, a key in the
shell (Ctrl+R by default, see shell.history_search_key) opens it and puts the
command on the command line.

  carrier search "connection refused"
  carrier search --since 7d --status failed OOM
  carrier search                 # pick a command from history`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if pick || len(args) == 0 {
				return a.pickCommand(cmd, strings.Join(args, " "))
			}
			f := store.SearchFilter{Status: status}
			if since != "" {
				d, err := duration.Parse(since)
				if err != nil {
					return fmt.Errorf("invalid --since duration %q: %w", since, err)
				}
				t := time.Now().Add(-d)
				f.Since = &t
			}
			query := strings.Join(args, " ")
			results, err := a.st.SearchRuns(query, limit, f)
			if err != nil {
				return err
			}
			if len(results) == 0 && !jsonOutput {
				a.printEmptyRunsHint(cmd, fmt.Sprintf("no runs match %q", query))
				return nil
			}
			if jsonOutput {
				type searchView struct {
					runView
					Snippet string `json:"snippet,omitempty"`
				}
				views := make([]searchView, 0, len(results))
				for _, result := range results {
					views = append(views, searchView{
						runView: runViewFromStore(&result.Run, false),
						Snippet: cleanSnippet(result.Snippet),
					})
				}
				return writeJSON(cmd, views)
			}
			out := cmd.OutOrStdout()
			t := newTheme(out)
			for _, result := range results {
				r := result.Run
				_, _ = fmt.Fprintf(
					out, "%s  %s  %s  %s\n",
					t.ID.Render(fmt.Sprintf("%d", r.ID)),
					t.statusStyle(r.Status).Render(r.Status),
					t.Command.Render(displayCommand(&r)),
					t.Muted.Render(r.CWD),
				)
				if snippet := cleanSnippet(result.Snippet); snippet != "" {
					_, _ = fmt.Fprintf(out, "    %s\n", t.Muted.Render(snippet))
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	cmd.Flags().IntVarP(&limit, "limit", "l", 100, "maximum number of results to show")
	cmd.Flags().StringVar(&since, "since", "", "only show runs started within duration (e.g. 1h, 24h, 7d)")
	cmd.Flags().StringVar(&status, "status", "", "only show runs with this status (success, failed, killed, running)")
	cmd.Flags().BoolVar(&pick, "pick", false, "pick a command from history, starting with the text, and print it")
	return cmd
}

// pickCommand opens history search, starting with query, and prints the
// command picked. It fails, printing nothing, when cancelled, so a shell
// binding leaves its command line as it was.
func (a *app) pickCommand(cmd *cobra.Command, query string) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stderr.Fd())) {
		return errors.New("give text to search for; in a terminal, carrier search without text opens history search")
	}
	commands, err := a.st.RecentCommands(5000)
	if err != nil {
		return err
	}
	h := tui.History{Commands: commands, Labels: map[int64]string{}, SessionID: currentSessionID()}
	h.CWD, _ = os.Getwd()
	// Many commands share a session; asking for each once keeps the lookup quick.
	var ids []int64
	seen := map[int64]bool{}
	for _, c := range commands {
		if c.SessionID != nil && !seen[*c.SessionID] {
			seen[*c.SessionID] = true
			ids = append(ids, *c.SessionID)
		}
	}
	if sessions, err := a.st.GetSessionsByIDs(ids); err == nil {
		for id, sess := range sessions {
			h.Labels[id] = sess.Label
		}
	}
	picked, ok, err := tui.PickCommand(h, query, a.cfg.UI.Theme)
	if err != nil {
		return err
	}
	if !ok {
		return errPickCancelled
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), picked)
	return err
}

// errPickCancelled ends a cancelled history search with exit status 1 and no
// message.
var errPickCancelled = errors.New("cancelled")

func cleanSnippet(snippet string) string {
	return strings.Join(strings.Fields(snippet), " ")
}
