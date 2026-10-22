package shell

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

type State struct {
	CurrentID  int64  `json:"current_id"`
	CurrentLog string `json:"current_log"`
	SessionID  int64  `json:"session_id"`
}

type StateFile struct {
	Path    string
	mu      sync.Mutex
	lastMod time.Time
	cached  State
}

// Read returns the current State. It only re-parses the file when its mtime
// has changed since the last read, making it cheap to call on every PTY chunk.
func (s *StateFile) Read() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	info, err := os.Stat(s.Path)
	if err != nil {
		return State{}
	}
	if !s.lastMod.IsZero() && info.ModTime().Equal(s.lastMod) {
		return s.cached
	}
	b, err := os.ReadFile(s.Path)
	if err != nil {
		return State{}
	}
	var state State
	_ = json.Unmarshal(b, &state)
	s.lastMod = info.ModTime()
	s.cached = state
	return state
}

func (s *StateFile) Write(state State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(state)
	return os.WriteFile(s.Path, b, 0o600)
}

// SessionInfoPath is the file the shell hooks read before every prompt to
// learn the current session's ID and label. Switching sessions rewrites it,
// so the prompt, title, and CARRIER_SESSION_ID follow without a new shell.
func SessionInfoPath(statePath string) string { return statePath + ".session" }

// StartSessionInfo writes a new shell's session file: the session ID and label
// for the hooks, one per line, read with shell builtins so prompts stay fast,
// then the data directory, since session IDs only mean something within one
// database.
func StartSessionInfo(statePath, dataDir string, sessionID int64, label string) error {
	return writeSessionInfo(statePath, sessionID, label, filepath.Clean(dataDir))
}

// WriteSessionInfo updates the session ID and label in a shell's session file,
// keeping its data directory.
func WriteSessionInfo(statePath string, sessionID int64, label string) error {
	return writeSessionInfo(statePath, sessionID, label, sessionInfoDataDir(statePath))
}

func writeSessionInfo(statePath string, sessionID int64, label, dataDir string) error {
	label = strings.Join(strings.Fields(label), " ")
	text := fmt.Sprintf("%d\n%s\n", sessionID, label)
	if dataDir != "" {
		text += dataDir + "\n"
	}
	return os.WriteFile(SessionInfoPath(statePath), []byte(text), 0o600)
}

func sessionInfoDataDir(statePath string) string {
	b, err := os.ReadFile(SessionInfoPath(statePath))
	if err != nil {
		return ""
	}
	if lines := strings.Split(string(b), "\n"); len(lines) > 2 {
		return lines[2]
	}
	return ""
}

// OtherTerminal reports whether this process runs in a terminal other than
// the recorded shell named by CARRIER_TTY: for example a tmux pane, which
// copies the carrier variables of the shell tmux was started from although
// nothing records it. It is false when CARRIER_TTY is unset (shells started
// by carrier before 1.2.2) or when none of stdin, stdout, and stderr is a
// terminal, so redirected commands keep working.
func OtherTerminal() bool {
	path := os.Getenv("CARRIER_TTY")
	if path == "" {
		return false
	}
	recorded, statErr := os.Stat(path)
	sawTerminal := false
	for _, f := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
		if !term.IsTerminal(int(f.Fd())) {
			continue
		}
		sawTerminal = true
		if info, err := f.Stat(); err == nil && statErr == nil && os.SameFile(info, recorded) {
			return false
		}
	}
	return sawTerminal
}

// RelabelShells rewrites the session file of each of this user's recorded
// shells that records into sessionID of the database in dataDir, so a label
// change shows in all their prompts from their next prompt, such as several
// tmux windows joined to one pinned session.
func RelabelShells(dataDir string, sessionID int64, label string) {
	states, _ := filepath.Glob(filepath.Join(os.TempDir(), "carrier-shell-*.json"))
	for _, statePath := range states {
		// The temp directory is shared; only touch this user's own files.
		if !ownRegularFile(statePath) || !ownRegularFile(SessionInfoPath(statePath)) {
			continue
		}
		if sessionInfoDataDir(statePath) != filepath.Clean(dataDir) {
			continue
		}
		if (&StateFile{Path: statePath}).Read().SessionID == sessionID {
			_ = WriteSessionInfo(statePath, sessionID, label)
		}
	}
}
