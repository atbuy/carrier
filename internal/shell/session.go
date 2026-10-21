package shell

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
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

// WriteSessionInfo writes the session ID and label for the hooks: one per
// line, read with shell builtins so prompts stay fast.
func WriteSessionInfo(statePath string, sessionID int64, label string) error {
	label = strings.Join(strings.Fields(label), " ")
	return os.WriteFile(SessionInfoPath(statePath), []byte(fmt.Sprintf("%d\n%s\n", sessionID, label)), 0o600)
}
