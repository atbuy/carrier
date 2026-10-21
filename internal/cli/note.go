package cli

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
)

type noteView struct {
	ID        int64     `json:"id"`
	SessionID *int64    `json:"session_id,omitempty"`
	RunID     *int64    `json:"run_id,omitempty"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

func noteViewFromStore(n store.Note) noteView {
	return noteView{ID: n.ID, SessionID: n.SessionID, RunID: n.RunID, Text: n.Text, CreatedAt: n.CreatedAt}
}

func (a *app) noteCmd() *cobra.Command {
	var session string
	var runRef string
	cmd := &cobra.Command{
		Use:     "note <text...>",
		Aliases: []string{"nt"},
		Short:   "add a note to the current session or a run",
		Long: `Write down what you are thinking while debugging, so you can find out
why you did something later.

Inside carrier shell the note is added to the current session, after the
last command. The note command itself is not recorded as a run.

  carrier note suspect stale DNS cache, flushing next
  carrier note -r 42 "flaky: passes on rerun"   # attach to run 42
  carrier note -r -2 "this one hung"             # the run before the latest
  carrier note -s backend-debug "root cause: expired proxy cert"

Outside a session, without --run or --session, the note is attached to the
latest run.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			text := strings.TrimSpace(strings.Join(args, " "))
			if text == "" {
				return fmt.Errorf("note text must not be empty")
			}
			n := store.CreateNote{Text: text, CreatedAt: time.Now()}
			switch {
			case runRef != "":
				r, err := a.resolveRun(cmd, runRef)
				if err != nil {
					return err
				}
				n.RunID = &r.ID
				n.SessionID = r.SessionID
			case session != "":
				sess, err := resolveSession(cmd, a.st, session)
				if err != nil {
					return err
				}
				if err := a.attachToSession(&n, sess.ID); err != nil {
					return err
				}
			case currentSessionID() != 0:
				a.dropShellRun(isNoteInvocation)
				if err := a.attachToSession(&n, currentSessionID()); err != nil {
					return err
				}
			default:
				r, err := a.st.Latest()
				if errors.Is(err, sql.ErrNoRows) {
					return fmt.Errorf("no runs to attach the note to; record one first, use carrier note inside carrier shell, or pass --session")
				}
				if err != nil {
					return err
				}
				n.RunID = &r.ID
				n.SessionID = r.SessionID
			}
			id, err := a.st.CreateNote(n)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "carrier: note %d added to %s\n", id, noteTarget(n.SessionID, n.RunID))
			return nil
		},
	}
	cmd.Flags().StringVarP(&session, "session", "s", "", "attach to a session by ID or label")
	cmd.Flags().StringVarP(&runRef, "run", "r", "", "attach to a run (ID, last, -N, or label)")
	cmd.MarkFlagsMutuallyExclusive("session", "run")
	return cmd
}

func (a *app) notesCmd() *cobra.Command {
	var session string
	var runRef string
	var limit int
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:     "notes",
		Aliases: []string{"ns"},
		Short:   "list notes oldest-first",
		Long: `List notes oldest-first. Inside carrier shell, only notes for the current
session are shown unless --session or --run is given.

  carrier notes                    # all notes, or the current session's
  carrier notes -s backend-debug   # notes for one session
  carrier notes -r 42              # notes attached to run 42
  carrier notes rm 7               # delete note 7`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var f store.NoteFilter
			switch {
			case runRef != "":
				r, err := a.resolveRun(cmd, runRef)
				if err != nil {
					return err
				}
				f.RunIDs = []int64{r.ID}
			case session != "":
				sess, err := resolveSession(cmd, a.st, session)
				if err != nil {
					return err
				}
				f.SessionIDs = []int64{sess.ID}
			case currentSessionID() != 0:
				f.SessionIDs = []int64{currentSessionID()}
			}
			notes, err := a.st.ListNotes(f, limit)
			if err != nil {
				return err
			}
			if len(notes) == 0 && !jsonOutput {
				printHint(cmd, emptyNotesHint(f))
				return nil
			}
			if jsonOutput {
				views := make([]noteView, 0, len(notes))
				for _, n := range notes {
					views = append(views, noteViewFromStore(n))
				}
				return writeJSON(cmd, views)
			}
			out := cmd.OutOrStdout()
			t := newTheme(out)
			for _, n := range notes {
				_, _ = fmt.Fprintf(
					out, "%s  %s  %s  %s\n",
					t.ID.Render(fmt.Sprintf("%6d", n.ID)),
					t.Muted.Render(padRight(formatRelativeTime(n.CreatedAt), 9)),
					t.Accent.Render(padRight(noteTarget(n.SessionID, n.RunID), 22)),
					n.Text,
				)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&session, "session", "s", "", "only notes for a session ID or label")
	cmd.Flags().StringVarP(&runRef, "run", "r", "", "only notes attached to a run (ID, last, -N, or label)")
	cmd.Flags().IntVarP(&limit, "limit", "l", 500, "maximum number of notes to show")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	cmd.MarkFlagsMutuallyExclusive("session", "run")
	cmd.AddCommand(a.notesRmCmd())
	return cmd
}

func (a *app) notesRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id>",
		Short: "delete a note",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if _, err := a.st.DeleteNote(id); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return fmt.Errorf("note %d not found", id)
				}
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "carrier: note %d deleted\n", id)
			return nil
		},
	}
}

func emptyNotesHint(f store.NoteFilter) string {
	switch {
	case len(f.RunIDs) == 1:
		return fmt.Sprintf("no notes for run %d", f.RunIDs[0])
	case len(f.SessionIDs) == 1:
		return fmt.Sprintf(`no notes for session %d; add one with "carrier note <text>"`, f.SessionIDs[0])
	default:
		return `no notes yet; add one with "carrier note <text>"`
	}
}

// attachToSession sets the note's session and links it to the session's latest
// run, so the note shows up after that run in history and show.
func (a *app) attachToSession(n *store.CreateNote, sessionID int64) error {
	n.SessionID = &sessionID
	r, err := a.st.LatestSessionRun(sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	n.RunID = &r.ID
	return nil
}

// dropShellRun removes the run the shell hooks recorded for this carrier
// invocation, such as "carrier note", so session bookkeeping doesn't clutter
// the session with runs. It only acts when the in-flight run is a plain
// command that match accepts; compound lines like "make test && carrier note
// done" are kept.
func (a *app) dropShellRun(match func(line string) bool) {
	statePath := os.Getenv("CARRIER_SHELL_STATE")
	if statePath == "" {
		return
	}
	sf := &carriershell.StateFile{Path: statePath}
	state := sf.Read()
	if state.CurrentID == 0 {
		return
	}
	r, err := a.st.GetRun(state.CurrentID)
	if err != nil || r.Status != store.StatusRunning || !match(r.Command) {
		return
	}
	if _, err := a.st.DeleteByID(r.ID); err != nil {
		return
	}
	_ = sf.Write(carriershell.State{SessionID: state.SessionID})
	removeIfSet(r.TerminalOutputPath)
}

// isNoteInvocation reports whether a shell command line is a single note
// command, e.g. "carrier note ..." or an alias like "c note ...".
func isNoteInvocation(line string) bool {
	words := carrierCommandWords(line)
	return len(words) > 0 && (words[0] == "note" || words[0] == "nt")
}

// carrierCommandWords returns the words after the program name of a single
// command line, skipping flags, e.g. ["note", "fixed"] for "c -q note fixed".
// It returns nil for lines with unquoted control operators or substitutions,
// which are never treated as one carrier command.
func carrierCommandWords(line string) []string {
	var quote rune
	escaped := false
	for _, ch := range line {
		switch {
		case escaped:
			escaped = false
		case ch == '\\' && quote != '\'':
			escaped = true
		case quote != 0:
			if ch == quote {
				quote = 0
			}
		case ch == '\'' || ch == '"':
			quote = ch
		case strings.ContainsRune(";&|`$()<>\n", ch):
			return nil
		}
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil
	}
	var words []string
	for _, field := range fields[1:] {
		if !strings.HasPrefix(field, "-") {
			words = append(words, field)
		}
	}
	return words
}

// currentSessionID is the session of the recorded shell this runs in, or 0.
// The shell's state file is checked first, since switching sessions updates
// it before the shell's next prompt updates CARRIER_SESSION_ID.
func currentSessionID() int64 {
	if statePath := os.Getenv("CARRIER_SHELL_STATE"); statePath != "" {
		if state := (&carriershell.StateFile{Path: statePath}).Read(); state.SessionID != 0 {
			return state.SessionID
		}
	}
	id, err := strconv.ParseInt(os.Getenv("CARRIER_SESSION_ID"), 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func noteTarget(sessionID, runID *int64) string {
	var parts []string
	if sessionID != nil {
		parts = append(parts, fmt.Sprintf("session %d", *sessionID))
	}
	if runID != nil {
		parts = append(parts, fmt.Sprintf("run %d", *runID))
	}
	return strings.Join(parts, ", ")
}

// printNoteLine renders a note under a run or session header in history.
// rail is "│" when more session runs follow, otherwise blank.
func printNoteLine(out io.Writer, t theme, n store.Note, rail string) {
	railStr := " "
	if rail != "" {
		railStr = t.Accent.Render(rail)
	}
	_, _ = fmt.Fprintf(
		out, "%s %s  %s %s\n",
		strings.Repeat(" ", 6),
		railStr,
		t.Label.Render("note:"),
		n.Text,
	)
}
