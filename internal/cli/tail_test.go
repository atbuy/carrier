package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atbuy/carrier/internal/store"
)

func TestTailCmdBadID(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()

	a := &app{st: st}
	cmd := a.tailCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.RunE(cmd, []string{"notanid"})
	if err == nil {
		t.Fatal("expected error for bad ID")
	}
}

func TestTailCmdTerminalStreamError(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	id := createFinishedRun(t, st, store.StatusSuccess, "echo hi", `["echo","hi"]`, "/tmp")
	// Set TerminalOutputPath so it's treated as a shell run.
	termPath := filepath.Join(dir, "term.log")
	if err := os.WriteFile(termPath, []byte("output\n"), 0o600); err != nil {
		t.Fatalf("write term: %v", err)
	}
	if err := st.UpdatePaths(id, "", "", termPath); err != nil {
		t.Fatalf("update paths: %v", err)
	}

	a := &app{st: st}
	cmd := a.tailCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	// stream "stdout" not available for shell run
	if err := cmd.Flags().Set("stream", "stdout"); err != nil {
		t.Fatalf("set stream: %v", err)
	}

	err = cmd.RunE(cmd, []string{fmt.Sprintf("%d", id)})
	if err == nil {
		t.Fatal("expected error when wrong stream for shell run")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTailCmdInvalidStream(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	id := createFinishedRun(t, st, store.StatusSuccess, "echo hi", `["echo","hi"]`, "/tmp")
	stdoutPath := filepath.Join(dir, "out.log")
	if err := os.WriteFile(stdoutPath, []byte("hi\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := st.UpdatePaths(id, stdoutPath, "", ""); err != nil {
		t.Fatalf("update paths: %v", err)
	}

	a := &app{st: st}
	cmd := a.tailCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("stream", "invalid"); err != nil {
		t.Fatalf("set stream: %v", err)
	}

	err = cmd.RunE(cmd, []string{fmt.Sprintf("%d", id)})
	if err == nil {
		t.Fatal("expected error for invalid stream")
	}
	if !strings.Contains(err.Error(), "invalid stream") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLockedWriterWrite(t *testing.T) {
	var buf bytes.Buffer
	lw := &lockedWriter{w: &buf}
	data := []byte("hello world\n")
	n, err := lw.Write(data)
	if err != nil {
		t.Fatalf("locked write: %v", err)
	}
	if n != len(data) {
		t.Fatalf("n = %d, want %d", n, len(data))
	}
	if buf.String() != string(data) {
		t.Fatalf("output = %q, want %q", buf.String(), string(data))
	}
}

func TestTailRunOutputWritesFiles(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "stdout.log")
	stderrPath := filepath.Join(dir, "stderr.log")
	if err := os.WriteFile(stdoutPath, []byte("out line\n"), 0o600); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := os.WriteFile(stderrPath, []byte("err line\n"), 0o600); err != nil {
		t.Fatalf("write stderr: %v", err)
	}

	// tailRunOutput writes to os.Stdout which we can't easily capture,
	// but we can verify it doesn't error on valid input.
	// To avoid writing to the real stdout, redirect via a pipe.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	done := make(chan struct{})
	var captured bytes.Buffer
	go func() {
		defer close(done)
		_, _ = captured.ReadFrom(r)
	}()

	if err := tailRunOutput(stdoutPath, stderrPath, false); err != nil {
		_ = w.Close()
		os.Stdout = oldStdout
		t.Fatalf("tailRunOutput: %v", err)
	}
	_ = w.Close()
	os.Stdout = oldStdout
	<-done
	_ = r.Close()

	out := captured.String()
	if !strings.Contains(out, "out line") {
		t.Errorf("tailRunOutput missing 'out line':\n%s", out)
	}
	if !strings.Contains(out, "err line") {
		t.Errorf("tailRunOutput missing 'err line':\n%s", out)
	}
}

func TestTailCmdNoLogs(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()

	id := createFinishedRun(t, st, store.StatusSuccess, "echo hi", `["echo","hi"]`, "/tmp")

	a := &app{st: st}
	cmd := a.tailCmd()

	// Run with no stdout/stderr/terminal paths → hits the "no output logs" warning path.
	err := cmd.RunE(cmd, []string{fmt.Sprintf("%d", id)})
	if err != nil {
		t.Fatalf("tailCmd no logs: %v", err)
	}
}

func TestTailCmdTerminalStream(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	id := createFinishedRun(t, st, store.StatusSuccess, "echo hi", `["echo","hi"]`, "/tmp")
	termPath := filepath.Join(dir, "term.log")
	if err := os.WriteFile(termPath, []byte("terminal output\n"), 0o600); err != nil {
		t.Fatalf("write term: %v", err)
	}
	if err := st.UpdatePaths(id, "", "", termPath); err != nil {
		t.Fatalf("update paths: %v", err)
	}

	a := &app{st: st}
	cmd := a.tailCmd()
	if err := cmd.Flags().Set("stream", "terminal"); err != nil {
		t.Fatalf("set stream: %v", err)
	}

	// logs.TailFile writes to os.Stdout; follow=false so it returns quickly.
	if err := cmd.RunE(cmd, []string{fmt.Sprintf("%d", id)}); err != nil {
		t.Fatalf("terminal stream: %v", err)
	}
}

func TestTailCmdStdoutStream(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	id := createFinishedRun(t, st, store.StatusSuccess, "echo hi", `["echo","hi"]`, "/tmp")
	stdoutPath := filepath.Join(dir, "out.log")
	if err := os.WriteFile(stdoutPath, []byte("stdout output\n"), 0o600); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := st.UpdatePaths(id, stdoutPath, "", ""); err != nil {
		t.Fatalf("update paths: %v", err)
	}

	a := &app{st: st}
	cmd := a.tailCmd()
	if err := cmd.Flags().Set("stream", "stdout"); err != nil {
		t.Fatalf("set stream: %v", err)
	}

	if err := cmd.RunE(cmd, []string{fmt.Sprintf("%d", id)}); err != nil {
		t.Fatalf("stdout stream: %v", err)
	}
}

func TestTailCmdStderrStream(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	id := createFinishedRun(t, st, store.StatusSuccess, "echo hi", `["echo","hi"]`, "/tmp")
	stderrPath := filepath.Join(dir, "err.log")
	if err := os.WriteFile(stderrPath, []byte("stderr output\n"), 0o600); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	if err := st.UpdatePaths(id, "", stderrPath, ""); err != nil {
		t.Fatalf("update paths: %v", err)
	}

	a := &app{st: st}
	cmd := a.tailCmd()
	if err := cmd.Flags().Set("stream", "stderr"); err != nil {
		t.Fatalf("set stream: %v", err)
	}

	if err := cmd.RunE(cmd, []string{fmt.Sprintf("%d", id)}); err != nil {
		t.Fatalf("stderr stream: %v", err)
	}
}

func TestPrefixWriterPrefixesEachLine(t *testing.T) {
	var buf bytes.Buffer
	writer := newPrefixWriter(&buf, "stdout | ")

	n, err := writer.Write([]byte("one\ntwo"))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != len("one\ntwo") {
		t.Fatalf("byte count = %d", n)
	}
	if _, err := writer.Write([]byte("\nthree\n")); err != nil {
		t.Fatalf("second write failed: %v", err)
	}

	want := "stdout | one\nstdout | two\nstdout | three\n"
	if got := buf.String(); got != want {
		t.Fatalf("prefixed output = %q, want %q", got, want)
	}
}
