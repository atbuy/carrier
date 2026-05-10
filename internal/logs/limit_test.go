package logs

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestCappedWriterWritesWithinLimit(t *testing.T) {
	var buf bytes.Buffer
	writer := NewCappedWriter(&buf, 10)

	n, err := writer.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != 5 {
		t.Fatalf("byte count = %d", n)
	}
	if got := buf.String(); got != "hello" {
		t.Fatalf("buffer = %q", got)
	}
}

func TestCappedWriterTruncatesAndReportsOriginalByteCount(t *testing.T) {
	var buf bytes.Buffer
	writer := NewCappedWriter(&buf, 5)

	n, err := writer.Write([]byte("hello world"))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != len("hello world") {
		t.Fatalf("byte count = %d", n)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "hello") {
		t.Fatalf("buffer missing capped content: %q", got)
	}
	if !strings.Contains(got, "output truncated") {
		t.Fatalf("buffer missing truncation notice: %q", got)
	}
}

func TestCappedAppendWriterHonorsAlreadyWrittenBytes(t *testing.T) {
	var buf bytes.Buffer
	writer := NewCappedAppendWriter(&buf, 5, 5)

	n, err := writer.Write([]byte("more"))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != 4 {
		t.Fatalf("byte count = %d", n)
	}
	if got := buf.String(); !strings.Contains(got, "output truncated") || strings.Contains(got, "more") {
		t.Fatalf("append cap mismatch: %q", got)
	}
}

func TestCappedAppendWriterDoesNotRepeatNoticeAfterLimitExceeded(t *testing.T) {
	var buf bytes.Buffer
	writer := NewCappedAppendWriter(&buf, 5, 64)

	n, err := writer.Write([]byte("more"))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != 4 {
		t.Fatalf("byte count = %d", n)
	}
	if got := buf.String(); got != "" {
		t.Fatalf("expected repeated overflow to be discarded, got %q", got)
	}
}

func TestCappedWriterUnlimitedPassesErrorThrough(t *testing.T) {
	// limit=0 → writes directly; exercise the error path.
	ew := &errorWriter{}
	writer := NewCappedWriter(ew, 0)
	_, err := writer.Write([]byte("data"))
	if err == nil {
		t.Fatal("expected error from errorWriter")
	}
}

func TestCappedWriterPartialWriteError(t *testing.T) {
	// Writer that accepts only 3 bytes then errors.
	pw := &partialWriter{limit: 3}
	writer := NewCappedWriter(pw, 100)
	n, err := writer.Write([]byte("hello"))
	if err == nil {
		t.Fatal("expected error from partialWriter")
	}
	if n != 3 {
		t.Fatalf("n = %d, want 3", n)
	}
}

func TestCappedWriterTruncationNoticeError(t *testing.T) {
	fw := &failOnSecondWrite{}
	writer := NewCappedWriter(fw, 2)

	_, err := writer.Write([]byte("abc"))
	if err == nil {
		t.Fatal("expected truncation notice write error")
	}
}

// errorWriter always returns an error.
type errorWriter struct{}

func (e *errorWriter) Write(p []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

// partialWriter accepts up to `limit` bytes then returns an error.
type partialWriter struct {
	limit   int
	written int
}

func (pw *partialWriter) Write(p []byte) (int, error) {
	remaining := pw.limit - pw.written
	if remaining <= 0 {
		return 0, io.ErrUnexpectedEOF
	}
	n := len(p)
	if n > remaining {
		n = remaining
	}
	pw.written += n
	if pw.written >= pw.limit && len(p) > n {
		return n, io.ErrUnexpectedEOF
	}
	return n, nil
}

type failOnSecondWrite struct {
	calls int
}

func (w *failOnSecondWrite) Write(p []byte) (int, error) {
	w.calls++
	if w.calls == 2 {
		return 0, io.ErrUnexpectedEOF
	}
	return len(p), nil
}

func TestMaxOutputBytes(t *testing.T) {
	if got := MaxOutputBytes(2); got != 2*1024*1024 {
		t.Fatalf("MaxOutputBytes = %d", got)
	}
	if got := MaxOutputBytes(0); got != 0 {
		t.Fatalf("zero should disable limit, got %d", got)
	}
}
