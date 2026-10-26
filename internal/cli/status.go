package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
)

// terminalStatus is what "carrier status" and "carrier doctor" report about
// the terminal they run in.
type terminalStatus struct {
	// Recorded is true inside a recorded shell that is recording.
	Recorded bool
	// Detached is true inside a recorded shell that "carrier detach" stopped.
	Detached bool
	// Inherited is true when this terminal only inherited the variables of a
	// recorded shell in another terminal, such as a tmux pane started from one.
	Inherited bool
	// InheritedID is the session named by those variables.
	InheritedID int64
	// OldHooks is true when the recorded shell was started by a carrier older
	// than 1.1.0, whose prompt cannot follow a session switch.
	OldHooks bool
	Session  *store.Session
	Runs     int
	Failed   int
	Notes    int
	Outcome  *store.Note
	Pin      pinStatus
	// Branch is the Git branch checked out here, and BranchSession the
	// session it is linked to, if any.
	Branch        string
	BranchSession *store.Session
	TTY           string
	Shell         string
	// TmuxStale is the problem with the tmux server's environment, if any.
	TmuxStale string
}

// pinStatus is the .carrier-session file that applies to the current directory.
type pinStatus struct {
	Path    string
	Ref     string
	Session *store.Session
	Err     error
}

// shellHealthy reports whether nothing is wrong with how this terminal is
// recorded, or not recorded.
func (s terminalStatus) shellHealthy() bool {
	sessionGone := s.Recorded && s.Session == nil
	return !s.Inherited && !s.OldHooks && !sessionGone
}

func (a *app) terminalStatus() terminalStatus {
	var s terminalStatus
	s.TTY = os.Getenv("CARRIER_TTY")
	s.Shell = os.Getenv("SHELL")
	if statePath := recordedShellState(); statePath != "" && (&carriershell.StateFile{Path: statePath}).Read().Detached {
		s.Detached = true
	} else if statePath != "" {
		s.Recorded = true
		s.OldHooks = hooksIgnoreSessionInfo(statePath)
		if id := currentSessionID(); id != 0 {
			if sess, err := a.st.GetSession(id); err == nil {
				s.Session = sess
			}
		}
	} else if os.Getenv("CARRIER_SESSION_ID") != "" && carriershell.OtherTerminal() {
		s.Inherited = true
		s.InheritedID, _ = strconv.ParseInt(os.Getenv("CARRIER_SESSION_ID"), 10, 64)
	}
	if s.Session != nil {
		if runs, err := a.st.SessionRuns(s.Session.ID); err == nil {
			s.Runs = len(runs)
			for _, r := range runs {
				if r.Status == store.StatusFailed {
					s.Failed++
				}
			}
		}
		if notes, err := a.st.ListNotes(store.NoteFilter{SessionIDs: []int64{s.Session.ID}}, 100000); err == nil {
			s.Notes = len(notes)
		}
		s.Outcome, _ = a.st.SessionOutcome(s.Session.ID)
	}
	if ok, detail, applies := tmuxEnvironment(); applies && !ok {
		s.TmuxStale = detail
	}
	if cwd, err := os.Getwd(); err == nil {
		if root, branch, ok := currentBranch(cwd); ok {
			s.Branch = branch
			s.BranchSession, _ = a.st.BranchSession(root, branch)
		}
		path, ref, err := findPin(cwd)
		s.Pin = pinStatus{Path: path, Ref: ref, Err: err}
		if path != "" && err == nil {
			s.Pin.Session, s.Pin.Err = resolveSession(nil, a.st, ref)
		}
	}
	return s
}

func (a *app) statusCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "show whether this terminal is recorded, and into which session",
		Long: `Show whether this terminal is recorded and into which session, a summary of
that session, and the session pinned for this directory, if any.

  carrier status
  carrier status --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := a.terminalStatus()
			if jsonOutput {
				return writeJSON(cmd, s.view())
			}
			printStatus(cmd.OutOrStdout(), s)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	return cmd
}

func printStatus(out io.Writer, s terminalStatus) {
	t := newTheme(out)
	line := func(name, value string) {
		_, _ = fmt.Fprintf(out, "%s %s\n", t.Bold.Render(padRight(name, 10)), value)
	}
	more := func(value string) { _, _ = fmt.Fprintf(out, "%s %s\n", padRight("", 10), t.Muted.Render(value)) }

	switch {
	case s.Recorded && s.Session != nil:
		line("Recording", t.Accent.Render(fmt.Sprintf("session %d", s.Session.ID))+labelSuffix(t, s.Session.Label))
		state := "active"
		if s.Session.EndedAt != nil {
			state = "ended"
		}
		summary := fmt.Sprintf("%s, started %s · %d %s", state, formatRelativeTime(s.Session.StartedAt), s.Runs, plural("command", s.Runs))
		if s.Failed > 0 {
			summary += fmt.Sprintf(", %d failed", s.Failed)
		}
		if s.Notes > 0 {
			summary += fmt.Sprintf(" · %d %s", s.Notes, plural("note", s.Notes))
		}
		more(summary)
		if s.Session.Auto && !s.Session.Kept && s.Session.Label == "" {
			more(`automatic; label, note, or keep it to keep it past shell.auto_session_retention`)
		}
	case s.Recorded:
		line("Recording", t.Warning.Render("yes, but its session is gone"))
		more(`it was deleted; "carrier attach <session>" switches this shell to another`)
	case s.Detached:
		line("Recording", t.Muted.Render(`no; "carrier detach" stopped recording this shell`))
		more(`"carrier attach <session>" records into a session again`)
	case s.Inherited:
		line("Recording", t.Warning.Render("no"))
		more(fmt.Sprintf("this terminal inherited session %d's variables from another terminal, such as the shell", s.InheritedID))
		more(`a tmux server was started from; open a new terminal, or "carrier shell" to record this one`)
	default:
		line("Recording", t.Muted.Render("no; this terminal is not a recorded shell"))
		more(`start one with "carrier shell", or record every terminal: see "carrier init"`)
	}
	if s.Outcome != nil {
		line("Outcome", s.Outcome.Text)
	}
	if s.OldHooks {
		line("Prompt", t.Warning.Render("started by a carrier older than 1.1.0; it cannot follow session switches"))
	}

	switch {
	case s.Pin.Path == "":
		line("Pin here", t.Muted.Render("none"))
	case s.Pin.Err != nil:
		line("Pin here", collapseHome(s.Pin.Path)+" "+t.Warning.Render(s.Pin.Err.Error()))
	default:
		line("Pin here", collapseHome(s.Pin.Path)+" → "+t.Accent.Render(fmt.Sprintf("session %d", s.Pin.Session.ID))+labelSuffix(t, s.Pin.Session.Label))
		if s.Recorded && s.Session != nil && s.Session.ID != s.Pin.Session.ID {
			more(`this shell records into another session; "carrier session reload" switches to the pinned one`)
		}
	}
	switch {
	case s.Branch == "":
	case s.BranchSession == nil:
		line("Branch", t.Label.Render(s.Branch)+" "+t.Muted.Render(`not linked to a session; "carrier session branch" links it`))
	default:
		line("Branch", t.Label.Render(s.Branch)+" → "+sessionTitle(t, s.BranchSession))
		if s.Recorded && s.Session != nil && s.Session.ID != s.BranchSession.ID {
			more(fmt.Sprintf(`this shell records into another session; "carrier attach %d" switches to it`, s.BranchSession.ID))
		}
	}
	if s.Recorded && s.TTY != "" {
		line("Terminal", t.Muted.Render(s.TTY))
	}
	if s.TmuxStale != "" {
		line("tmux", t.Warning.Render(s.TmuxStale))
	}
}

func labelSuffix(t theme, label string) string {
	if label == "" {
		return ""
	}
	return t.Muted.Render(" · ") + t.Label.Render(label)
}

// statusView is the JSON form of terminalStatus.
type statusView struct {
	Recorded    bool         `json:"recorded"`
	Detached    bool         `json:"detached,omitempty"`
	Inherited   bool         `json:"inherited"`
	InheritedID int64        `json:"inherited_session_id,omitempty"`
	OldHooks    bool         `json:"old_hooks"`
	Session     *sessionView `json:"session,omitempty"`
	RunCount    int          `json:"run_count"`
	FailedCount int          `json:"failed_count"`
	NoteCount   int          `json:"note_count"`
	Outcome     *noteView    `json:"outcome,omitempty"`
	Pin         *pinView     `json:"pin,omitempty"`
	Branch      *branchView  `json:"branch,omitempty"`
	Terminal    string       `json:"terminal,omitempty"`
	TmuxStale   string       `json:"tmux_problem,omitempty"`
}

// branchView is the Git branch checked out here and its linked session.
type branchView struct {
	Name      string `json:"name"`
	SessionID int64  `json:"session_id,omitempty"`
}

type pinView struct {
	Path      string `json:"path"`
	Ref       string `json:"ref,omitempty"`
	SessionID int64  `json:"session_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (s terminalStatus) view() statusView {
	v := statusView{
		Recorded: s.Recorded, Detached: s.Detached, Inherited: s.Inherited, InheritedID: s.InheritedID, OldHooks: s.OldHooks,
		RunCount: s.Runs, FailedCount: s.Failed, NoteCount: s.Notes, TmuxStale: s.TmuxStale,
	}
	if s.Recorded {
		v.Terminal = s.TTY
	}
	if s.Session != nil {
		sv := sessionViewFromStore(*s.Session)
		v.Session = &sv
	}
	if s.Outcome != nil {
		n := noteViewFromStore(*s.Outcome)
		v.Outcome = &n
	}
	if s.Branch != "" {
		v.Branch = &branchView{Name: s.Branch}
		if s.BranchSession != nil {
			v.Branch.SessionID = s.BranchSession.ID
		}
	}
	if s.Pin.Path != "" {
		v.Pin = &pinView{Path: s.Pin.Path, Ref: s.Pin.Ref}
		if s.Pin.Session != nil {
			v.Pin.SessionID = s.Pin.Session.ID
		}
		if s.Pin.Err != nil {
			v.Pin.Error = s.Pin.Err.Error()
		}
	}
	return v
}
