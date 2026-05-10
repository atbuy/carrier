package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/store"
)

func openLabelStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return st
}

func createLabelRun(t *testing.T, st *store.Store) int64 {
	t.Helper()
	id, err := st.CreateRun(store.CreateRun{
		Status:    store.StatusRunning,
		Mode:      store.ModeRun,
		Command:   "echo hi",
		ArgvJSON:  `["echo","hi"]`,
		CWD:       "/tmp",
		StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	return id
}

func TestLabelCmd_SetLabel(t *testing.T) {
	st := openLabelStore(t)
	defer func() { _ = st.Close() }()

	id := createLabelRun(t, st)

	a := &app{st: st}
	cmd := a.labelCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.RunE(cmd, []string{fmt.Sprintf("%d", id), "prod", "deploy"}); err != nil {
		t.Fatalf("labelCmd RunE failed: %v", err)
	}
	if !strings.Contains(out.String(), `labeled "prod deploy"`) {
		t.Fatalf("expected label confirmation, got: %s", out.String())
	}

	r, err := st.GetRun(id)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if r.Label != "prod deploy" {
		t.Fatalf("expected label %q, got %q", "prod deploy", r.Label)
	}
}

func TestLabelCmd_ClearLabel(t *testing.T) {
	st := openLabelStore(t)
	defer func() { _ = st.Close() }()

	id := createLabelRun(t, st)
	// First set a label.
	if err := st.SetLabel(id, "initial"); err != nil {
		t.Fatalf("SetLabel: %v", err)
	}

	a := &app{st: st}
	cmd := a.labelCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Call with just the ID to clear the label.
	if err := cmd.RunE(cmd, []string{fmt.Sprintf("%d", id)}); err != nil {
		t.Fatalf("labelCmd RunE (clear) failed: %v", err)
	}
	if !strings.Contains(out.String(), "label cleared") {
		t.Fatalf("expected 'label cleared', got: %s", out.String())
	}

	r, err := st.GetRun(id)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if r.Label != "" {
		t.Fatalf("expected empty label, got %q", r.Label)
	}
}

func TestLabelCmd_BadID(t *testing.T) {
	st := openLabelStore(t)
	defer func() { _ = st.Close() }()

	a := &app{st: st}
	cmd := a.labelCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.RunE(cmd, []string{"not-a-number"})
	if err == nil {
		t.Fatalf("expected error for bad ID, got nil")
	}
}

func TestLabelCmd_NonExistentID(t *testing.T) {
	// SetLabel does an UPDATE which silently affects 0 rows for a non-existent ID;
	// the command succeeds without error (same as SQLite UPDATE semantics).
	st := openLabelStore(t)
	defer func() { _ = st.Close() }()

	a := &app{st: st}
	cmd := a.labelCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.RunE(cmd, []string{"99999", "some", "label"})
	if err != nil {
		t.Fatalf("expected no error for non-existent run ID (UPDATE is a no-op), got: %v", err)
	}
	if !strings.Contains(out.String(), "labeled") {
		t.Fatalf("expected confirmation output, got: %s", out.String())
	}
}
