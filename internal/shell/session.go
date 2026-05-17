package shell

import (
	"encoding/json"
	"os"
	"sync"
)

type State struct {
	CurrentID  int64  `json:"current_id"`
	CurrentLog string `json:"current_log"`
	SessionID  int64  `json:"session_id"`
}

type StateFile struct {
	Path string
	mu   sync.Mutex
}

func (s *StateFile) Read() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.Path)
	if err != nil {
		return State{}
	}
	var state State
	_ = json.Unmarshal(b, &state)
	return state
}

func (s *StateFile) Write(state State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(state)
	return os.WriteFile(s.Path, b, 0o600)
}
