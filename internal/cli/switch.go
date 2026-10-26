package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
)

// pinFileName is the file that pins a directory to a session, like
// .python-version for pyenv and uv. New automatic sessions started in the
// directory, or below it, join the pinned session instead.
const pinFileName = ".carrier-session"

// recordedShellState returns the state file of the recorded shell this runs
// in, or "" outside one. A terminal that only inherited the variables of a
// recorded shell, such as a tmux pane, is outside.
func recordedShellState() string {
	statePath := os.Getenv("CARRIER_SHELL_STATE")
	if statePath == "" || carriershell.OtherTerminal() {
		return ""
	}
	if _, err := os.Stat(statePath); err != nil {
		return ""
	}
	return statePath
}

// switchSession makes the recorded shell this runs in record into sess from
// its next command on. The session it leaves is ended, and deleted when it is
// an empty automatic session. The prompt follows at the next prompt.
func (a *app) switchSession(cmd *cobra.Command, statePath string, sess *store.Session) error {
	// The switching command itself is bookkeeping, not part of either session.
	a.dropShellRun(isSessionCommand)
	sf := &carriershell.StateFile{Path: statePath}
	state := sf.Read()
	out := cmd.OutOrStdout()
	if state.SessionID == sess.ID {
		_, _ = fmt.Fprintf(out, "carrier: already in session %s\n", sessionName(sess))
		return nil
	}
	if err := a.moveShell(sf, state, sess); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "carrier: switched to session %s\n", sessionName(sess))
	if hooksIgnoreSessionInfo(statePath) {
		printOldHooksHint(cmd, state.SessionID)
	}
	return nil
}

// moveShell points the recorded shell with state at sess: its run in progress
// is kept, its session file is updated for the prompt, and the session it
// leaves is ended, or deleted when it is an empty automatic session.
func (a *app) moveShell(sf *carriershell.StateFile, state carriershell.State, sess *store.Session) error {
	if err := a.st.ReopenSession(sess.ID); err != nil {
		return err
	}
	if err := sf.Write(carriershell.State{CurrentID: state.CurrentID, CurrentLog: state.CurrentLog, SessionID: sess.ID}); err != nil {
		return err
	}
	// A shell from a carrier older than 1.1.0 has no session file to update.
	if !hooksIgnoreSessionInfo(sf.Path) {
		if err := carriershell.WriteSessionInfo(sf.Path, sess.ID, sess.Label); err != nil {
			return err
		}
	}
	a.leaveSession(state.SessionID)
	return nil
}

// hooksIgnoreSessionInfo reports whether this shell was started by a carrier
// older than 1.1.0, whose hooks never read the session file: switching
// sessions works there, but the prompt keeps showing the old session.
func hooksIgnoreSessionInfo(statePath string) bool {
	_, err := os.Stat(carriershell.SessionInfoPath(statePath))
	return errors.Is(err, os.ErrNotExist)
}

func printOldHooksHint(cmd *cobra.Command, oldID int64) {
	printHint(cmd, fmt.Sprintf("this shell was started by an older carrier, so its prompt still shows session %d; "+
		"commands are recorded into the new session, and new terminals show it", oldID))
}

// leaveSession ends a session a shell stopped recording into. An automatic
// session with nothing in it is deleted, as when its terminal closes.
func (a *app) leaveSession(id int64) {
	if id == 0 {
		return
	}
	_ = a.st.EndSession(id, time.Now())
	a.deleteIfEmptyAuto(id)
}

func (a *app) deleteIfEmptyAuto(id int64) {
	sess, err := a.st.GetSession(id)
	if err != nil || !sess.Auto || sess.Kept {
		return
	}
	if empty, err := a.st.SessionIsEmpty(id); err == nil && empty {
		_, _ = a.st.DeleteSession(id)
	}
}

// isSessionCommand reports whether a shell command line is a single command
// that moves the shell between sessions: "carrier attach", "carrier session
// reload", or "carrier session end". Such commands are not recorded.
func isSessionCommand(line string) bool {
	words := carrierCommandWords(line)
	switch {
	case len(words) == 0:
		return false
	case words[0] == "attach" || words[0] == "a" || words[0] == "detach" || words[0] == "dt":
		return true
	default:
		return len(words) > 1 && (words[0] == "session" || words[0] == "se") && (words[1] == "reload" || words[1] == "end")
	}
}

func sessionName(sess *store.Session) string {
	if sess.Label == "" {
		return strconv.FormatInt(sess.ID, 10)
	}
	return fmt.Sprintf("%d %q", sess.ID, sess.Label)
}

// findPin returns the nearest .carrier-session file in dir or its parents,
// and the session reference in it. path is "" when there is none.
func findPin(dir string) (path, ref string, err error) {
	for {
		candidate := filepath.Join(dir, pinFileName)
		if _, statErr := os.Stat(candidate); statErr == nil {
			ref, err := readPin(candidate)
			return candidate, ref, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", nil
		}
		dir = parent
	}
}

// readPin returns the session ID or label in a pin file: its first line that
// is not empty or a "#" comment.
func readPin(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			return line, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("it names no session")
}

// pinnedSession returns the session pinned for the current directory. When a
// pin file names a session that does not exist, it prints a hint and returns
// nil, so the caller carries on without it.
func (a *app) pinnedSession(cmd *cobra.Command) *store.Session {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	path, ref, err := findPin(cwd)
	if path == "" {
		return nil
	}
	if err == nil {
		var sess *store.Session
		if sess, err = resolveSession(cmd, a.st, ref); err == nil {
			return sess
		}
	}
	printHint(cmd, fmt.Sprintf("ignoring %s: %v", collapseHome(path), err))
	return nil
}

// pinRef is what "session pin" writes for sess: its label when no other
// session has the same label, since a label means more to a reader than an
// ID, and the ID otherwise.
func (a *app) pinRef(sess *store.Session) string {
	if sess.Label == "" || strings.HasPrefix(sess.Label, "#") {
		return strconv.FormatInt(sess.ID, 10)
	}
	sessions, err := a.st.ListLabeledSessions()
	if err != nil {
		return strconv.FormatInt(sess.ID, 10)
	}
	same := 0
	for _, s := range sessions {
		if s.Label == sess.Label {
			same++
		}
	}
	if same != 1 {
		return strconv.FormatInt(sess.ID, 10)
	}
	return sess.Label
}

func (a *app) sessionPinCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pin [<id-or-label>]",
		Short: "pin this directory to a session, so new terminals here join it",
		Long: `Write a .carrier-session file in the current directory naming a session,
like .python-version for pyenv and uv. Automatic sessions started in this
directory or below it (see "carrier init") join the pinned session instead of
starting a new one, so every terminal and tmux window for a task records into
the same session.

  carrier session pin              # pin the session of this shell
  carrier session pin BUG-1234     # pin a session by ID or label

The file holds the session's label, or its ID when the label is missing or
shared by several sessions. Changing directory does not switch sessions; run
"carrier session reload" to switch to the session pinned where you are.

Sessions are local to your machine, so keep .carrier-session out of version
control, for example in a global gitignore.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var sess *store.Session
			var err error
			switch {
			case len(args) == 1:
				sess, err = resolveSession(cmd, a.st, args[0])
			case currentSessionID() != 0:
				sess, err = a.st.GetSession(currentSessionID())
			default:
				return errors.New(`give a session ID or label, or run this inside a recorded shell`)
			}
			if err != nil {
				return err
			}
			path := pinFileName
			if err := os.WriteFile(path, []byte(a.pinRef(sess)+"\n"), 0o644); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "carrier: pinned session %s to this directory; new terminals here join it\n", sessionName(sess))
			if sess.Auto && !sess.Kept {
				if err := a.st.SetSessionKept(sess.ID, true); err == nil {
					printHint(cmd, "pinned automatic sessions are kept and never expire")
				}
			}
			return nil
		},
	}
}

func (a *app) sessionUnpinCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unpin",
		Short: "remove the .carrier-session file in this directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := os.Remove(pinFileName)
			if errors.Is(err, os.ErrNotExist) {
				msg := "no " + pinFileName + " in this directory"
				if cwd, cwdErr := os.Getwd(); cwdErr == nil {
					if path, _, _ := findPin(cwd); path != "" {
						msg += "; the pin here comes from " + collapseHome(path)
					}
				}
				return errors.New(msg)
			}
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "carrier: unpinned this directory")
			return nil
		},
	}
}

func (a *app) sessionReloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reload",
		Short: "switch this shell to the session pinned for this directory",
		Long: `Read the .carrier-session file for the current directory (or the nearest
parent that has one) and switch this recorded shell to that session, without
starting a new shell. Changing directory never switches sessions on its own.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			statePath := recordedShellState()
			if statePath == "" {
				return errors.New(`not in a recorded shell; use "carrier attach" to open the pinned session`)
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			path, ref, err := findPin(cwd)
			if path == "" {
				return errors.New(`no ` + pinFileName + ` in this directory or its parents; create one with "carrier session pin"`)
			}
			if err != nil {
				return fmt.Errorf("%s: %w", collapseHome(path), err)
			}
			sess, err := resolveSession(cmd, a.st, ref)
			if err != nil {
				return fmt.Errorf("%s: %w", collapseHome(path), err)
			}
			return a.switchSession(cmd, statePath, sess)
		},
	}
}
