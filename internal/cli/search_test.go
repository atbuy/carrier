package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/store"
)

func TestSearchCommandUsesFTSResults(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	started := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	id, err := st.CreateRun(store.CreateRun{
		Status:    store.StatusRunning,
		Mode:      store.ModeRun,
		Command:   "go test ./...",
		ArgvJSON:  `["go","test","./..."]`,
		CWD:       "/tmp/project",
		StartedAt: started,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	stdoutPath := filepath.Join(t.TempDir(), "stdout.log")
	if err := os.WriteFile(stdoutPath, []byte("needle output line\n"), 0o600); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := st.UpdatePaths(id, stdoutPath, "", ""); err != nil {
		t.Fatalf("update paths: %v", err)
	}
	if err := st.FinishRun(id, store.StatusSuccess, 0, started.Add(time.Second)); err != nil {
		t.Fatalf("finish run: %v", err)
	}

	app := &app{st: st}
	cmd := app.searchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.RunE(cmd, []string{"needle"}); err != nil {
		t.Fatalf("search command failed: %v", err)
	}
	for _, want := range []string{
		"1  success  go test ./...  /tmp/project",
		"needle output line",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("search output missing %q:\n%s", want, out.String())
		}
	}
}

func TestSearchCommandVariadicArgs(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	started := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	id, err := st.CreateRun(store.CreateRun{
		Status:    store.StatusRunning,
		Mode:      store.ModeRun,
		Command:   "go test ./...",
		ArgvJSON:  `["go","test","./..."]`,
		CWD:       "/tmp/project",
		StartedAt: started,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := st.FinishRun(id, store.StatusSuccess, 0, started.Add(time.Second)); err != nil {
		t.Fatalf("finish run: %v", err)
	}

	app := &app{st: st}
	cmd := app.searchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	// multi-word search without quoting
	if err := cmd.RunE(cmd, []string{"go", "test"}); err != nil {
		t.Fatalf("variadic search failed: %v", err)
	}
	if !strings.Contains(out.String(), "go test ./...") {
		t.Fatalf("variadic search missing result:\n%s", out.String())
	}
}
