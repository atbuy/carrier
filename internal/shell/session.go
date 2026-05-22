package shell

import (
	"encoding/json"
	"os"
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
