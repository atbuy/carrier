package shell

import (
	"path/filepath"
	"testing"
)

func TestStateFileReadWrite(t *testing.T) {
	sf := &StateFile{Path: filepath.Join(t.TempDir(), "state.json")}

	if got := sf.Read(); got.CurrentID != 0 || got.CurrentLog != "" {
		t.Fatalf("missing state should read zero value: %#v", got)
	}

	want := State{CurrentID: 42, CurrentLog: "/tmp/run.log"}
	if err := sf.Write(want); err != nil {
		t.Fatalf("write state: %v", err)
	}
	if got := sf.Read(); got != want {
		t.Fatalf("state mismatch: got %#v want %#v", got, want)
	}
}
