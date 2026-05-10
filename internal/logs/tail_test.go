package logs

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// TailFile — no follow
// ---------------------------------------------------------------------------

func TestTailFileNoFollow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")
	content := "hello\nworld\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var buf bytes.Buffer
	if err := TailFile(path, &buf, false); err != nil {
		t.Fatalf("TailFile: %v", err)
	}
	if buf.String() != content {
		t.Errorf("TailFile output = %q, want %q", buf.String(), content)
	}
}

func TestTailFileNoFollowEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.log")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var buf bytes.Buffer
	if err := TailFile(path, &buf, false); err != nil {
		t.Fatalf("TailFile on empty file: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty output, got %q", buf.String())
	}
}

func TestTailFileMissing(t *testing.T) {
	var buf bytes.Buffer
	err := TailFile("/does/not/exist/file.log", &buf, false)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// ---------------------------------------------------------------------------
// TailFile — with follow
// ---------------------------------------------------------------------------

func TestTailFileFollow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "follow.log")
	initial := "line1\n"
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var buf bytes.Buffer
	done := make(chan error, 1)

	// Run TailFile in a goroutine with follow=true.
	// We use a pipe-trick via the buffer; since TailFile blocks forever in
	// follow mode we cancel it by closing the underlying file from outside,
	// which causes the watcher channel to close and TailFile to return nil.
	//
	// Simpler approach: write additional content shortly after TailFile starts,
	// wait for it to appear in buf, then verify and let the goroutine run in
	// the background (the test itself terminates the process via t.Cleanup).
	go func() {
		done <- TailFile(path, &buf, true)
	}()

	// Give the watcher a moment to arm itself, then append more content.
	time.Sleep(100 * time.Millisecond)
	extra := "line2\n"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	if _, err := f.WriteString(extra); err != nil {
		_ = f.Close()
		t.Fatalf("write extra: %v", err)
	}
	_ = f.Close()

	// Poll until both lines appear in buf (max 3 s).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), initial) && strings.Contains(buf.String(), extra) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	got := buf.String()
	if !strings.Contains(got, initial) {
		t.Errorf("missing initial content %q in output %q", initial, got)
	}
	if !strings.Contains(got, extra) {
		t.Errorf("missing extra content %q in output %q", extra, got)
	}

	// The goroutine is still blocking on the watcher; that is fine — the test
	// process will clean up on exit. We do not block waiting for done here.
}

// ---------------------------------------------------------------------------
// drainFile
// ---------------------------------------------------------------------------

func TestDrainFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "drain.log")
	content := "draining content\nsecond line\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()

	var buf bytes.Buffer
	bufSlice := make([]byte, 32*1024)
	if err := drainFile(f, bufSlice, &buf); err != nil {
		t.Fatalf("drainFile: %v", err)
	}
	if buf.String() != content {
		t.Errorf("drainFile output = %q, want %q", buf.String(), content)
	}
}

func TestDrainFileEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.log")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()

	var buf bytes.Buffer
	bufSlice := make([]byte, 32*1024)
	if err := drainFile(f, bufSlice, &buf); err != nil {
		t.Fatalf("drainFile on empty file: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty output, got %q", buf.String())
	}
}

// ---------------------------------------------------------------------------
// tailPoll
// ---------------------------------------------------------------------------

func TestTailPollReturnsErrorOnClosedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "poll.log")
	if err := os.WriteFile(path, []byte("initial\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Seek to end so the first Read returns EOF immediately and tailPoll sleeps.
	if _, err := f.Seek(0, 2 /*io.SeekEnd*/); err != nil {
		t.Fatalf("seek: %v", err)
	}

	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- tailPoll(f, &buf) }()

	// Close the file after tailPoll enters the 500ms sleep — it will then
	// get a non-EOF error on the next Read and return it.
	time.Sleep(600 * time.Millisecond)
	_ = f.Close()

	select {
	case err := <-done:
		if err == nil {
			t.Error("tailPoll should return an error when underlying file is closed")
		}
	case <-time.After(3 * time.Second):
		t.Error("tailPoll did not return within timeout")
	}
}

func TestTailPollDrainsThenErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "poll2.log")
	content := "drain me\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Do NOT seek — tailPoll will drain the content first, then hit EOF and sleep.

	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- tailPoll(f, &buf) }()

	time.Sleep(600 * time.Millisecond)
	_ = f.Close()

	select {
	case <-done:
		if !strings.Contains(buf.String(), content) {
			t.Errorf("expected drained content %q in output %q", content, buf.String())
		}
	case <-time.After(3 * time.Second):
		t.Error("tailPoll did not return within timeout")
	}
}

func TestDrainFileWriteError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.log")
	if err := os.WriteFile(path, []byte("data to drain\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, 32*1024)
	ew := &tailErrorWriter{}
	err = drainFile(f, buf, ew)
	if err == nil {
		t.Fatal("expected error from drainFile when writer fails")
	}
}

type tailErrorWriter struct{}

func (e *tailErrorWriter) Write(_ []byte) (int, error) {
	return 0, os.ErrClosed
}

func TestDrainFilePartialRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "partial.log")
	// Use content larger than a small buffer to exercise the loop.
	content := strings.Repeat("abcdefghij", 100) // 1000 bytes
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()

	var buf bytes.Buffer
	// Use a small buffer (16 bytes) to exercise multiple iterations.
	bufSlice := make([]byte, 16)
	if err := drainFile(f, bufSlice, &buf); err != nil {
		t.Fatalf("drainFile: %v", err)
	}
	if buf.String() != content {
		t.Errorf("drainFile partial-read output length = %d, want %d", buf.Len(), len(content))
	}
}
